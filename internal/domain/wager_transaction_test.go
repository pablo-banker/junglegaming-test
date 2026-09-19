package domain

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

var wagerTestTime = time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC)

// mustWagerMoney creates BRL Money for wager tests.
func mustWagerMoney(t *testing.T, amount string) Money {
	t.Helper()

	currency, err := NewCurrency("BRL")
	if err != nil {
		t.Fatalf("unexpected currency error: %v", err)
	}

	money, err := ParseMoney(amount, currency)
	if err != nil {
		t.Fatalf("unexpected money error: %v", err)
	}

	return money
}

// validExternalWagerParams creates valid external transaction parameters.
func validExternalWagerParams(
	t *testing.T,
	transactionType WagerTransactionType,
) NewExternalWagerTransactionParams {
	t.Helper()

	amount := "25.00"
	if transactionType == WagerTransactionTypeLoss {
		amount = "0.00"
	}

	reference := ""
	if transactionType.RequiresReference() {
		reference = "reference-123"
	}

	return NewExternalWagerTransactionParams{
		ID:                             uuid.New(),
		ProviderID:                     "provider-a",
		ExternalTransactionID:          "transaction-" + uuid.NewString(),
		IdempotencyKey:                 "key-" + uuid.NewString(),
		PayloadHash:                    "payload-hash",
		WalletID:                       uuid.New(),
		PlayerID:                       uuid.New(),
		RoundID:                        "round-123",
		GameID:                         "game-123",
		Type:                           transactionType,
		Amount:                         mustWagerMoney(t, amount),
		ReferenceExternalTransactionID: reference,
		CreatedAt:                      wagerTestTime,
	}
}

// mustProcessedReference creates a processed transaction for reference tests.
func mustProcessedReference(
	t *testing.T,
	transactionType WagerTransactionType,
	externalTransactionID string,
	providerID string,
	walletID uuid.UUID,
	playerID uuid.UUID,
	roundID string,
) *WagerTransaction {
	t.Helper()

	before := mustWagerMoney(t, "100.00")
	after := mustWagerMoney(t, "100.00")

	switch transactionType {
	case WagerTransactionTypeBet:
		after = mustWagerMoney(t, "75.00")

	case WagerTransactionTypeWin:
		after = mustWagerMoney(t, "125.00")

	case WagerTransactionTypeRefund:
		before = mustWagerMoney(t, "75.00")
		after = mustWagerMoney(t, "100.00")
	}

	completedAt := wagerTestTime.Add(time.Second)

	params := RehydrateWagerTransactionParams{
		ID:                    uuid.New(),
		ProviderID:            providerID,
		ExternalTransactionID: externalTransactionID,
		IdempotencyKey:        "key-" + uuid.NewString(),
		PayloadHash:           "payload-hash",
		WalletID:              walletID,
		PlayerID:              playerID,
		RoundID:               roundID,
		GameID:                "game-123",
		Type:                  transactionType,
		Amount:                mustWagerMoney(t, "25.00"),
		Status:                WagerTransactionStatusProcessed,
		BalanceBefore:         &before,
		BalanceAfter:          &after,
		CreatedAt:             wagerTestTime,
		UpdatedAt:             completedAt,
		CompletedAt:           &completedAt,
	}

	if transactionType == WagerTransactionTypeRefund {
		params.ReferenceExternalTransactionID = "original-bet"
		params.ReferenceTransactionID = uuid.New()
		params.ReferenceTransactionType = WagerTransactionTypeBet
	}

	transaction, err := RehydrateWagerTransaction(params)
	if err != nil {
		t.Fatalf("unexpected rehydration error: %v", err)
	}

	return transaction
}

// TestNewExternalWagerTransactionCreatesPendingTransaction verifies external creation.
func TestNewExternalWagerTransactionCreatesPendingTransaction(t *testing.T) {
	params := validExternalWagerParams(t, WagerTransactionTypeBet)

	transaction, err := NewExternalWagerTransaction(params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if transaction.Status() != WagerTransactionStatusPending {
		t.Errorf("expected PENDING, got %s", transaction.Status())
	}

	if transaction.Type() != WagerTransactionTypeBet {
		t.Errorf("expected BET, got %s", transaction.Type())
	}

	if !transaction.Amount().Equal(params.Amount) {
		t.Error("expected transaction amount to be preserved")
	}
}

// TestNewExternalWagerTransactionValidatesTypeRules verifies basic wager rules.
func TestNewExternalWagerTransactionValidatesTypeRules(t *testing.T) {
	tests := []struct {
		name   string
		change func(*NewExternalWagerTransactionParams)
		want   error
	}{
		{
			name: "opening cannot be external",
			change: func(params *NewExternalWagerTransactionParams) {
				params.Type = WagerTransactionTypeOpening
			},
			want: ErrInvalidWagerType,
		},
		{
			name: "bet must be positive",
			change: func(params *NewExternalWagerTransactionParams) {
				params.Amount = mustWagerMoney(t, "0.00")
			},
			want: ErrAmountMustBePositive,
		},
		{
			name: "loss must be zero",
			change: func(params *NewExternalWagerTransactionParams) {
				params.Type = WagerTransactionTypeLoss
				params.Amount = mustWagerMoney(t, "25.00")
			},
			want: ErrInvalidWagerAmount,
		},
		{
			name: "idempotency key is bounded",
			change: func(params *NewExternalWagerTransactionParams) {
				params.IdempotencyKey = strings.Repeat("k", 256)
			},
			want: ErrInvalidIdempotencyKey,
		},
		{
			name: "external transaction id is bounded",
			change: func(params *NewExternalWagerTransactionParams) {
				params.ExternalTransactionID = strings.Repeat("x", 256)
			},
			want: ErrInvalidExternalTransactionID,
		},
		{
			name: "operation cannot reference itself",
			change: func(params *NewExternalWagerTransactionParams) {
				params.Type = WagerTransactionTypeRefund
				params.ReferenceExternalTransactionID = params.ExternalTransactionID
			},
			want: ErrInvalidWagerReference,
		},
		{
			name: "refund requires reference",
			change: func(params *NewExternalWagerTransactionParams) {
				params.Type = WagerTransactionTypeRefund
				params.ReferenceExternalTransactionID = ""
			},
			want: ErrInvalidWagerReference,
		},
		{
			name: "rollback requires reference",
			change: func(params *NewExternalWagerTransactionParams) {
				params.Type = WagerTransactionTypeRollback
				params.ReferenceExternalTransactionID = ""
			},
			want: ErrInvalidWagerReference,
		},
		{
			name: "bet cannot have reference",
			change: func(params *NewExternalWagerTransactionParams) {
				params.ReferenceExternalTransactionID = "reference-123"
			},
			want: ErrInvalidWagerReference,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			params := validExternalWagerParams(t, WagerTransactionTypeBet)
			test.change(&params)

			_, err := NewExternalWagerTransaction(params)

			if !errors.Is(err, test.want) {
				t.Fatalf("expected %v, got %v", test.want, err)
			}
		})
	}
}

// TestNewOpeningWagerTransactionCreatesProcessedOpening verifies internal opening creation.
func TestNewOpeningWagerTransactionCreatesProcessedOpening(t *testing.T) {
	transaction, err := NewOpeningWagerTransaction(
		NewOpeningWagerTransactionParams{
			ID:        uuid.New(),
			WalletID:  uuid.New(),
			PlayerID:  uuid.New(),
			Amount:    mustWagerMoney(t, "100.00"),
			CreatedAt: wagerTestTime,
		},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if transaction.Type() != WagerTransactionTypeOpening {
		t.Errorf("expected OPENING, got %s", transaction.Type())
	}

	if transaction.Status() != WagerTransactionStatusProcessed {
		t.Errorf("expected PROCESSED, got %s", transaction.Status())
	}

	before, ok := transaction.BalanceBefore()
	if !ok || before.Amount() != "0.00" {
		t.Error("expected balance before 0.00")
	}

	after, ok := transaction.BalanceAfter()
	if !ok || after.Amount() != "100.00" {
		t.Error("expected balance after 100.00")
	}
}

// TestWagerTransactionMovementDirection verifies wallet movement by wager type.
func TestWagerTransactionMovementDirection(t *testing.T) {
	tests := []struct {
		name        string
		typ         WagerTransactionType
		direction   WalletLedgerDirection
		hasMovement bool
	}{
		{
			name:        "bet debits",
			typ:         WagerTransactionTypeBet,
			direction:   WalletLedgerDirectionDebit,
			hasMovement: true,
		},
		{
			name:        "win credits",
			typ:         WagerTransactionTypeWin,
			direction:   WalletLedgerDirectionCredit,
			hasMovement: true,
		},
		{
			name:        "loss has no movement",
			typ:         WagerTransactionTypeLoss,
			hasMovement: false,
		},
		{
			name:        "refund credits",
			typ:         WagerTransactionTypeRefund,
			direction:   WalletLedgerDirectionCredit,
			hasMovement: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			params := validExternalWagerParams(t, test.typ)

			transaction, err := NewExternalWagerTransaction(params)
			if err != nil {
				t.Fatalf("unexpected creation error: %v", err)
			}

			direction, hasMovement, err := transaction.MovementDirection()
			if err != nil {
				t.Fatalf("unexpected movement error: %v", err)
			}

			if hasMovement != test.hasMovement {
				t.Errorf("expected movement %v, got %v", test.hasMovement, hasMovement)
			}

			if test.hasMovement && direction != test.direction {
				t.Errorf("expected %s, got %s", test.direction, direction)
			}
		})
	}
}

// TestWagerTransactionResolvesPendingReference verifies pending reference resolution.
func TestWagerTransactionResolvesPendingReference(t *testing.T) {
	walletID := uuid.New()
	playerID := uuid.New()

	reference := mustProcessedReference(
		t,
		WagerTransactionTypeBet,
		"bet-123",
		"provider-a",
		walletID,
		playerID,
		"round-123",
	)

	params := validExternalWagerParams(t, WagerTransactionTypeRefund)
	params.ProviderID = "provider-a"
	params.WalletID = walletID
	params.PlayerID = playerID
	params.RoundID = "round-123"
	params.ReferenceExternalTransactionID = "bet-123"

	transaction, err := NewExternalWagerTransaction(params)
	if err != nil {
		t.Fatalf("unexpected creation error: %v", err)
	}

	if err := transaction.MarkPendingReference(
		wagerTestTime.Add(time.Second),
	); err != nil {
		t.Fatalf("unexpected pending reference error: %v", err)
	}

	if transaction.Status() != WagerTransactionStatusPendingReference {
		t.Fatalf("expected PENDING_REFERENCE, got %s", transaction.Status())
	}

	if err := transaction.ResolveReference(
		reference,
		wagerTestTime.Add(2*time.Second),
	); err != nil {
		t.Fatalf("unexpected reference resolution error: %v", err)
	}

	if transaction.Status() != WagerTransactionStatusPending {
		t.Errorf("expected PENDING, got %s", transaction.Status())
	}

	if transaction.ReferenceTransactionID() != reference.ID() {
		t.Error("expected resolved reference id")
	}

	if transaction.ReferenceTransactionType() != WagerTransactionTypeBet {
		t.Errorf(
			"expected BET reference, got %s",
			transaction.ReferenceTransactionType(),
		)
	}
}

// TestWagerTransactionRejectsReferenceMismatch verifies reference context must match.
func TestWagerTransactionRejectsReferenceMismatch(t *testing.T) {
	walletID := uuid.New()
	playerID := uuid.New()

	reference := mustProcessedReference(
		t,
		WagerTransactionTypeBet,
		"bet-123",
		"provider-a",
		walletID,
		playerID,
		"round-123",
	)

	tests := []struct {
		name   string
		change func(*NewExternalWagerTransactionParams)
	}{
		{
			name: "provider mismatch",
			change: func(params *NewExternalWagerTransactionParams) {
				params.ProviderID = "provider-b"
			},
		},
		{
			name: "player mismatch",
			change: func(params *NewExternalWagerTransactionParams) {
				params.PlayerID = uuid.New()
			},
		},
		{
			name: "wallet mismatch",
			change: func(params *NewExternalWagerTransactionParams) {
				params.WalletID = uuid.New()
			},
		},
		{
			name: "round mismatch",
			change: func(params *NewExternalWagerTransactionParams) {
				params.RoundID = "another-round"
			},
		},
		{
			name: "external reference mismatch",
			change: func(params *NewExternalWagerTransactionParams) {
				params.ReferenceExternalTransactionID = "another-bet"
			},
		},
		{
			name: "amount mismatch",
			change: func(params *NewExternalWagerTransactionParams) {
				params.Amount = mustWagerMoney(t, "10.00")
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			params := validExternalWagerParams(t, WagerTransactionTypeRefund)

			params.ProviderID = "provider-a"
			params.WalletID = walletID
			params.PlayerID = playerID
			params.RoundID = "round-123"
			params.ReferenceExternalTransactionID = "bet-123"

			test.change(&params)

			transaction, err := NewExternalWagerTransaction(params)
			if err != nil {
				t.Fatalf("unexpected creation error: %v", err)
			}

			err = transaction.ResolveReference(
				reference,
				wagerTestTime.Add(time.Second),
			)

			if !errors.Is(err, ErrWagerReferenceMismatch) {
				t.Fatalf("expected ErrWagerReferenceMismatch, got %v", err)
			}
		})
	}
}

// TestWagerTransactionRollbackDirection verifies rollback reverses the referenced operation.
func TestWagerTransactionRollbackDirection(t *testing.T) {
	tests := []struct {
		name          string
		referenceType WagerTransactionType
		want          WalletLedgerDirection
	}{
		{
			name:          "rollback bet credits",
			referenceType: WagerTransactionTypeBet,
			want:          WalletLedgerDirectionCredit,
		},
		{
			name:          "rollback win debits",
			referenceType: WagerTransactionTypeWin,
			want:          WalletLedgerDirectionDebit,
		},
		{
			name:          "rollback refund debits",
			referenceType: WagerTransactionTypeRefund,
			want:          WalletLedgerDirectionDebit,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			walletID := uuid.New()
			playerID := uuid.New()
			externalID := "reference-" + uuid.NewString()

			reference := mustProcessedReference(
				t,
				test.referenceType,
				externalID,
				"provider-a",
				walletID,
				playerID,
				"round-123",
			)

			params := validExternalWagerParams(
				t,
				WagerTransactionTypeRollback,
			)

			params.ProviderID = reference.ProviderID()
			params.WalletID = reference.WalletID()
			params.PlayerID = reference.PlayerID()
			params.RoundID = reference.RoundID()
			params.Amount = reference.Amount()
			params.ReferenceExternalTransactionID = reference.ExternalTransactionID()
			params.CreatedAt = reference.UpdatedAt().Add(time.Second)

			transaction, err := NewExternalWagerTransaction(params)
			if err != nil {
				t.Fatalf("unexpected creation error: %v", err)
			}

			if err := transaction.ResolveReference(
				reference,
				params.CreatedAt.Add(time.Second),
			); err != nil {
				t.Fatalf("unexpected reference resolution error: %v", err)
			}

			direction, hasMovement, err := transaction.MovementDirection()
			if err != nil {
				t.Fatalf("unexpected movement error: %v", err)
			}

			if !hasMovement {
				t.Fatal("expected rollback to have a wallet movement")
			}

			if direction != test.want {
				t.Errorf("expected %s, got %s", test.want, direction)
			}
		})
	}
}

// TestWagerTransactionMarkProcessedStoresObservedBalance verifies replay data is preserved.
func TestWagerTransactionMarkProcessedStoresObservedBalance(t *testing.T) {
	params := validExternalWagerParams(t, WagerTransactionTypeBet)

	transaction, err := NewExternalWagerTransaction(params)
	if err != nil {
		t.Fatalf("unexpected creation error: %v", err)
	}

	before := mustWagerMoney(t, "100.00")
	after := mustWagerMoney(t, "75.00")
	completedAt := wagerTestTime.Add(time.Second)

	if err := transaction.MarkProcessed(
		before,
		after,
		completedAt,
	); err != nil {
		t.Fatalf("unexpected processing error: %v", err)
	}

	if transaction.Status() != WagerTransactionStatusProcessed {
		t.Errorf("expected PROCESSED, got %s", transaction.Status())
	}

	storedBefore, ok := transaction.BalanceBefore()
	if !ok || !storedBefore.Equal(before) {
		t.Error("expected balance before to be preserved")
	}

	storedAfter, ok := transaction.BalanceAfter()
	if !ok || !storedAfter.Equal(after) {
		t.Error("expected balance after to be preserved")
	}

	storedCompletedAt, ok := transaction.CompletedAt()
	if !ok || !storedCompletedAt.Equal(completedAt) {
		t.Error("expected completedAt to be preserved")
	}
}

// TestWagerTransactionFailureTransitions verifies rejected and failed terminal results.
func TestWagerTransactionFailureTransitions(t *testing.T) {
	tests := []struct {
		name   string
		status WagerTransactionStatus
		apply  func(*WagerTransaction, time.Time) error
	}{
		{
			name:   "rejected",
			status: WagerTransactionStatusRejected,
			apply: func(transaction *WagerTransaction, at time.Time) error {
				return transaction.MarkRejected(
					"BET_INSUFFICIENT_FUNDS",
					"insufficient funds",
					at,
				)
			},
		},
		{
			name:   "failed",
			status: WagerTransactionStatusFailed,
			apply: func(transaction *WagerTransaction, at time.Time) error {
				return transaction.MarkFailed(
					"PERMANENT_FAILURE",
					"permanent infrastructure failure",
					at,
				)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			params := validExternalWagerParams(t, WagerTransactionTypeBet)

			transaction, err := NewExternalWagerTransaction(params)
			if err != nil {
				t.Fatalf("unexpected creation error: %v", err)
			}

			if err := test.apply(
				transaction,
				wagerTestTime.Add(time.Second),
			); err != nil {
				t.Fatalf("unexpected terminal transition error: %v", err)
			}

			if transaction.Status() != test.status {
				t.Errorf(
					"expected %s, got %s",
					test.status,
					transaction.Status(),
				)
			}

			if transaction.FailureCode() == "" {
				t.Error("expected failure code to be stored")
			}
		})
	}
}

// TestWagerTransactionTerminalStateIsImmutable verifies terminal transactions cannot change.
func TestWagerTransactionTerminalStateIsImmutable(t *testing.T) {
	params := validExternalWagerParams(t, WagerTransactionTypeBet)

	transaction, err := NewExternalWagerTransaction(params)
	if err != nil {
		t.Fatalf("unexpected creation error: %v", err)
	}

	completedAt := wagerTestTime.Add(time.Second)

	if err := transaction.MarkProcessed(
		mustWagerMoney(t, "100.00"),
		mustWagerMoney(t, "75.00"),
		completedAt,
	); err != nil {
		t.Fatalf("unexpected processing error: %v", err)
	}

	err = transaction.MarkRejected(
		"SHOULD_NOT_CHANGE",
		"terminal transaction",
		completedAt.Add(time.Second),
	)

	if !errors.Is(err, ErrWagerTransactionTerminal) {
		t.Fatalf("expected ErrWagerTransactionTerminal, got %v", err)
	}

	if transaction.Status() != WagerTransactionStatusProcessed {
		t.Errorf("expected status to remain PROCESSED, got %s", transaction.Status())
	}
}

// TestRehydrateWagerTransactionPreservesState verifies persisted state restoration.
func TestRehydrateWagerTransactionPreservesState(t *testing.T) {
	before := mustWagerMoney(t, "100.00")
	after := mustWagerMoney(t, "75.00")
	completedAt := wagerTestTime.Add(time.Second)

	id := uuid.New()
	walletID := uuid.New()
	playerID := uuid.New()

	transaction, err := RehydrateWagerTransaction(
		RehydrateWagerTransactionParams{
			ID:                    id,
			ProviderID:            "provider-a",
			ExternalTransactionID: "transaction-123",
			IdempotencyKey:        "key-123",
			PayloadHash:           "payload-hash",
			WalletID:              walletID,
			PlayerID:              playerID,
			RoundID:               "round-123",
			GameID:                "game-123",
			Type:                  WagerTransactionTypeBet,
			Amount:                mustWagerMoney(t, "25.00"),
			Status:                WagerTransactionStatusProcessed,
			BalanceBefore:         &before,
			BalanceAfter:          &after,
			CreatedAt:             wagerTestTime,
			UpdatedAt:             completedAt,
			CompletedAt:           &completedAt,
		},
	)
	if err != nil {
		t.Fatalf("unexpected rehydration error: %v", err)
	}

	if transaction.ID() != id {
		t.Error("expected persisted id")
	}

	if transaction.WalletID() != walletID {
		t.Error("expected persisted wallet id")
	}

	if transaction.PlayerID() != playerID {
		t.Error("expected persisted player id")
	}

	if transaction.Status() != WagerTransactionStatusProcessed {
		t.Errorf("expected PROCESSED, got %s", transaction.Status())
	}

	if transaction.UpdatedAt() != completedAt {
		t.Error("expected persisted updatedAt")
	}
}
