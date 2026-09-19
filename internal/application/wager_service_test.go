//go:build unit

package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/pablo-banker/junglegaming-test/internal/domain"
)

var wagerServiceTestTime = time.Date(
	2026,
	9,
	18,
	15,
	0,
	0,
	0,
	time.UTC,
)

// mustApplicationMoney creates BRL money for application tests.
func mustApplicationMoney(t *testing.T, amount string) domain.Money {
	t.Helper()

	currency, err := domain.NewCurrency("BRL")
	if err != nil {
		t.Fatalf("unexpected currency error: %v", err)
	}

	money, err := domain.ParseMoney(amount, currency)
	if err != nil {
		t.Fatalf("unexpected money error: %v", err)
	}

	return money
}

// mustApplicationWallet creates a wallet for application tests.
func mustApplicationWallet(
	t *testing.T,
	balance string,
) *domain.Wallet {
	t.Helper()

	wallet, err := domain.NewWallet(
		uuid.New(),
		uuid.New(),
		mustApplicationMoney(t, balance),
		wagerServiceTestTime.Add(-time.Hour),
	)
	if err != nil {
		t.Fatalf("unexpected wallet error: %v", err)
	}

	return wallet
}

// validProcessWagerCommand creates a valid BET command.
func validProcessWagerCommand(
	wallet *domain.Wallet,
) ProcessWagerCommand {
	return ProcessWagerCommand{
		ProviderID:            "provider-a",
		ExternalTransactionID: "transaction-" + uuid.NewString(),
		IdempotencyKey:        "key-" + uuid.NewString(),
		WalletID:              wallet.ID().String(),
		PlayerID:              wallet.PlayerID().String(),
		RoundID:               "round-123",
		GameID:                "game-123",
		Type:                  "BET",
		Amount:                "25.00",
		Currency:              "BRL",
	}
}

// newWagerServiceForTest creates WagerService dependencies for tests.
func newWagerServiceForTest() (
	*WagerService,
	*fakeTransactionManager,
	*fakeWalletRepository,
	*fakeWagerRepository,
	*fakeLedgerRepository,
	*fakeOutboxRepository,
) {
	txManager := &fakeTransactionManager{}

	clock := &fakeClock{
		now: wagerServiceTestTime,
	}

	wallets := &fakeWalletRepository{}
	wagers := &fakeWagerRepository{}
	ledger := &fakeLedgerRepository{}
	outbox := &fakeOutboxRepository{}

	service := NewWagerService(
		txManager,
		clock,
		wallets,
		wagers,
		ledger,
		outbox,
	)

	return service, txManager, wallets, wagers, ledger, outbox
}

// TestWagerServiceProcessesBet verifies a successful BET financial flow.
func TestWagerServiceProcessesBet(t *testing.T) {
	service, txManager, wallets, wagers, ledger, outbox :=
		newWagerServiceForTest()

	wallet := mustApplicationWallet(t, "100.00")
	wallets.findByIDForUpdateResult = wallet

	command := validProcessWagerCommand(wallet)

	result, err := service.Process(
		context.Background(),
		command,
		CommandMetadata{
			CorrelationID: "correlation-123",
		},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !txManager.called {
		t.Fatal("expected transaction to be started")
	}

	if result.Status != domain.WagerTransactionStatusProcessed {
		t.Errorf("expected PROCESSED, got %s", result.Status)
	}

	if result.IdempotentReplay {
		t.Error("expected first processing not to be a replay")
	}

	if wallet.Balance().Amount() != "75.00" {
		t.Errorf(
			"expected wallet balance 75.00, got %s",
			wallet.Balance().Amount(),
		)
	}

	if wallet.Version() != 2 {
		t.Errorf("expected wallet version 2, got %d", wallet.Version())
	}

	if len(wallets.updated) != 1 {
		t.Fatalf(
			"expected 1 wallet update, got %d",
			len(wallets.updated),
		)
	}

	if len(wagers.created) != 1 {
		t.Fatalf(
			"expected 1 wager creation, got %d",
			len(wagers.created),
		)
	}

	if len(wagers.updated) != 1 {
		t.Fatalf(
			"expected 1 wager update, got %d",
			len(wagers.updated),
		)
	}

	if len(ledger.created) != 1 {
		t.Fatalf(
			"expected 1 ledger entry, got %d",
			len(ledger.created),
		)
	}

	entry := ledger.created[0]

	if entry.Direction() != domain.WalletLedgerDirectionDebit {
		t.Errorf("expected DEBIT, got %s", entry.Direction())
	}

	if entry.BalanceBefore().Amount() != "100.00" {
		t.Errorf(
			"expected balance before 100.00, got %s",
			entry.BalanceBefore().Amount(),
		)
	}

	if entry.BalanceAfter().Amount() != "75.00" {
		t.Errorf(
			"expected balance after 75.00, got %s",
			entry.BalanceAfter().Amount(),
		)
	}

	if len(outbox.created) != 2 {
		t.Fatalf(
			"expected 2 outbox events, got %d",
			len(outbox.created),
		)
	}

	if outbox.created[0].EventType != EventTypeWagerTransactionProcessed {
		t.Errorf(
			"expected WagerTransactionProcessed, got %s",
			outbox.created[0].EventType,
		)
	}

	if outbox.created[1].EventType != EventTypeWalletBalanceChanged {
		t.Errorf(
			"expected WalletBalanceChanged, got %s",
			outbox.created[1].EventType,
		)
	}
}

// TestWagerServiceProcessesLossWithoutWalletMovement verifies LOSS has no financial movement.
func TestWagerServiceProcessesLossWithoutWalletMovement(t *testing.T) {
	service, _, wallets, wagers, ledger, outbox :=
		newWagerServiceForTest()

	wallet := mustApplicationWallet(t, "100.00")
	wallets.findByIDForUpdateResult = wallet

	command := validProcessWagerCommand(wallet)
	command.Type = "LOSS"
	command.Amount = "0.00"

	result, err := service.Process(
		context.Background(),
		command,
		CommandMetadata{
			CorrelationID: "correlation-123",
		},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Status != domain.WagerTransactionStatusProcessed {
		t.Errorf("expected PROCESSED, got %s", result.Status)
	}

	if wallet.Balance().Amount() != "100.00" {
		t.Errorf(
			"expected wallet balance 100.00, got %s",
			wallet.Balance().Amount(),
		)
	}

	if wallet.Version() != 1 {
		t.Errorf(
			"expected wallet version 1, got %d",
			wallet.Version(),
		)
	}

	if len(wallets.updated) != 0 {
		t.Errorf(
			"expected no wallet updates, got %d",
			len(wallets.updated),
		)
	}

	if len(ledger.created) != 0 {
		t.Errorf(
			"expected no ledger entries, got %d",
			len(ledger.created),
		)
	}

	if len(wagers.updated) != 1 {
		t.Fatalf(
			"expected wager to be updated once, got %d",
			len(wagers.updated),
		)
	}

	if len(outbox.created) != 1 {
		t.Fatalf(
			"expected 1 event, got %d",
			len(outbox.created),
		)
	}

	if outbox.created[0].EventType != EventTypeWagerTransactionProcessed {
		t.Errorf(
			"expected WagerTransactionProcessed, got %s",
			outbox.created[0].EventType,
		)
	}
}

// TestWagerServiceRejectsBetWithInsufficientFunds verifies auditable BET rejection.
func TestWagerServiceRejectsBetWithInsufficientFunds(t *testing.T) {
	service, _, wallets, wagers, ledger, outbox :=
		newWagerServiceForTest()

	wallet := mustApplicationWallet(t, "20.00")
	wallets.findByIDForUpdateResult = wallet

	command := validProcessWagerCommand(wallet)
	command.Amount = "80.00"

	result, err := service.Process(
		context.Background(),
		command,
		CommandMetadata{
			CorrelationID: "correlation-123",
		},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Status != domain.WagerTransactionStatusRejected {
		t.Errorf("expected REJECTED, got %s", result.Status)
	}

	if result.FailureCode != failureCodeBetInsufficientFunds {
		t.Errorf(
			"expected %s, got %s",
			failureCodeBetInsufficientFunds,
			result.FailureCode,
		)
	}

	if wallet.Balance().Amount() != "20.00" {
		t.Errorf(
			"expected balance to remain 20.00, got %s",
			wallet.Balance().Amount(),
		)
	}

	if wallet.Version() != 1 {
		t.Errorf(
			"expected version to remain 1, got %d",
			wallet.Version(),
		)
	}

	if len(wallets.updated) != 0 {
		t.Errorf(
			"expected no wallet update, got %d",
			len(wallets.updated),
		)
	}

	if len(ledger.created) != 0 {
		t.Errorf(
			"expected no ledger entry, got %d",
			len(ledger.created),
		)
	}

	if len(wagers.updated) != 1 {
		t.Fatalf(
			"expected rejected wager update, got %d",
			len(wagers.updated),
		)
	}

	if len(outbox.created) != 1 {
		t.Fatalf(
			"expected 1 rejected event, got %d",
			len(outbox.created),
		)
	}

	if outbox.created[0].EventType != EventTypeWagerTransactionRejected {
		t.Errorf(
			"expected WagerTransactionRejected, got %s",
			outbox.created[0].EventType,
		)
	}
}

// TestWagerServiceMarksMissingReferenceAsPending verifies durable missing-reference handling.
func TestWagerServiceMarksMissingReferenceAsPending(t *testing.T) {
	service, _, _, wagers, ledger, outbox :=
		newWagerServiceForTest()

	wallet := mustApplicationWallet(t, "100.00")

	command := validProcessWagerCommand(wallet)
	command.Type = "REFUND"
	command.ReferenceExternalTransactionID = "bet-not-arrived"

	wagers.findByProviderExternalErr = ErrNotFound

	result, err := service.Process(
		context.Background(),
		command,
		CommandMetadata{
			CorrelationID: "correlation-123",
		},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Status != domain.WagerTransactionStatusPendingReference {
		t.Errorf(
			"expected PENDING_REFERENCE, got %s",
			result.Status,
		)
	}

	if len(wagers.pendingReferenceUpdates) != 1 {
		t.Fatalf(
			"expected 1 pending reference update, got %d",
			len(wagers.pendingReferenceUpdates),
		)
	}

	update := wagers.pendingReferenceUpdates[0]

	expectedNextAttempt := wagerServiceTestTime.Add(
		initialReferenceRetryDelay,
	)

	expectedExpiration := wagerServiceTestTime.Add(
		referenceRetryTTL,
	)

	if !update.nextAttemptAt.Equal(expectedNextAttempt) {
		t.Errorf(
			"expected next attempt %s, got %s",
			expectedNextAttempt,
			update.nextAttemptAt,
		)
	}

	if !update.expiresAt.Equal(expectedExpiration) {
		t.Errorf(
			"expected expiration %s, got %s",
			expectedExpiration,
			update.expiresAt,
		)
	}

	if len(ledger.created) != 0 {
		t.Errorf(
			"expected no ledger entry, got %d",
			len(ledger.created),
		)
	}

	if len(outbox.created) != 1 {
		t.Fatalf(
			"expected 1 pending reference event, got %d",
			len(outbox.created),
		)
	}

	if outbox.created[0].EventType !=
		EventTypeWagerTransactionPendingReference {
		t.Errorf(
			"expected pending reference event, got %s",
			outbox.created[0].EventType,
		)
	}
}

// TestWagerServiceProcessesRefund verifies a valid BET refund credits the wallet.
func TestWagerServiceProcessesRefund(t *testing.T) {
	service, _, wallets, wagers, ledger, outbox :=
		newWagerServiceForTest()

	wallet := mustApplicationWallet(t, "75.00")
	wallets.findByIDForUpdateResult = wallet

	betBefore := mustApplicationMoney(t, "100.00")
	betAfter := mustApplicationMoney(t, "75.00")
	completedAt := wagerServiceTestTime.Add(-time.Minute)

	bet, err := domain.RehydrateWagerTransaction(
		domain.RehydrateWagerTransactionParams{
			ID:                    uuid.New(),
			ProviderID:            "provider-a",
			ExternalTransactionID: "bet-123",
			IdempotencyKey:        "original-bet-key",
			PayloadHash:           "original-bet-hash",
			WalletID:              wallet.ID(),
			PlayerID:              wallet.PlayerID(),
			RoundID:               "round-123",
			GameID:                "game-123",
			Type:                  domain.WagerTransactionTypeBet,
			Amount:                mustApplicationMoney(t, "25.00"),
			Status:                domain.WagerTransactionStatusProcessed,
			BalanceBefore:         &betBefore,
			BalanceAfter:          &betAfter,
			CreatedAt: wagerServiceTestTime.Add(
				-2 * time.Minute,
			),
			UpdatedAt:   completedAt,
			CompletedAt: &completedAt,
		},
	)
	if err != nil {
		t.Fatalf("unexpected bet creation error: %v", err)
	}

	wagers.findByProviderExternalResult = bet

	command := validProcessWagerCommand(wallet)
	command.Type = "REFUND"
	command.Amount = "25.00"
	command.ReferenceExternalTransactionID = "bet-123"

	result, err := service.Process(
		context.Background(),
		command,
		CommandMetadata{
			CorrelationID: "correlation-123",
		},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Status != domain.WagerTransactionStatusProcessed {
		t.Errorf("expected PROCESSED, got %s", result.Status)
	}

	if wallet.Balance().Amount() != "100.00" {
		t.Errorf(
			"expected balance 100.00, got %s",
			wallet.Balance().Amount(),
		)
	}

	if len(ledger.created) != 1 {
		t.Fatalf(
			"expected 1 ledger entry, got %d",
			len(ledger.created),
		)
	}

	if ledger.created[0].Direction() !=
		domain.WalletLedgerDirectionCredit {
		t.Errorf(
			"expected CREDIT, got %s",
			ledger.created[0].Direction(),
		)
	}

	if len(outbox.created) != 2 {
		t.Fatalf(
			"expected 2 events, got %d",
			len(outbox.created),
		)
	}
}

// TestWagerServiceReturnsIdempotentReplay verifies same key and payload are replayed.
func TestWagerServiceReturnsIdempotentReplay(t *testing.T) {
	service, _, _, wagers, ledger, outbox :=
		newWagerServiceForTest()

	wallet := mustApplicationWallet(t, "75.00")
	command := validProcessWagerCommand(wallet)

	payloadHash, err := calculateWagerPayloadHash(
		command,
		wallet.ID(),
		wallet.PlayerID(),
		domain.WagerTransactionTypeBet,
		mustApplicationMoney(t, "25.00"),
	)
	if err != nil {
		t.Fatalf("unexpected hash error: %v", err)
	}

	before := mustApplicationMoney(t, "100.00")
	after := mustApplicationMoney(t, "75.00")
	completedAt := wagerServiceTestTime.Add(-time.Minute)

	existing, err := domain.RehydrateWagerTransaction(
		domain.RehydrateWagerTransactionParams{
			ID:                    uuid.New(),
			ProviderID:            command.ProviderID,
			ExternalTransactionID: command.ExternalTransactionID,
			IdempotencyKey:        command.IdempotencyKey,
			PayloadHash:           payloadHash,
			WalletID:              wallet.ID(),
			PlayerID:              wallet.PlayerID(),
			RoundID:               command.RoundID,
			GameID:                command.GameID,
			Type:                  domain.WagerTransactionTypeBet,
			Amount:                mustApplicationMoney(t, "25.00"),
			Status:                domain.WagerTransactionStatusProcessed,
			BalanceBefore:         &before,
			BalanceAfter:          &after,
			CreatedAt: wagerServiceTestTime.Add(
				-2 * time.Minute,
			),
			UpdatedAt:   completedAt,
			CompletedAt: &completedAt,
		},
	)
	if err != nil {
		t.Fatalf("unexpected existing wager error: %v", err)
	}

	wagers.createErr = ErrAlreadyExists
	wagers.findByProviderIdempotencyResult = existing

	result, err := service.Process(
		context.Background(),
		command,
		CommandMetadata{
			CorrelationID: "another-correlation",
		},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.IdempotentReplay {
		t.Fatal("expected idempotent replay")
	}

	if result.TransactionID != existing.ID() {
		t.Error("expected original transaction id")
	}

	if result.BalanceBefore == nil ||
		result.BalanceBefore.Amount() != "100.00" {
		t.Error("expected original balance before 100.00")
	}

	if result.BalanceAfter == nil ||
		result.BalanceAfter.Amount() != "75.00" {
		t.Error("expected original balance after 75.00")
	}

	if len(ledger.created) != 0 {
		t.Errorf(
			"expected no new ledger entry, got %d",
			len(ledger.created),
		)
	}

	if len(outbox.created) != 0 {
		t.Errorf(
			"expected no new events, got %d",
			len(outbox.created),
		)
	}
}

// TestWagerServiceRejectsIdempotencyConflict verifies changed payload cannot reuse a key.
func TestWagerServiceRejectsIdempotencyConflict(t *testing.T) {
	service, _, _, wagers, _, _ :=
		newWagerServiceForTest()

	wallet := mustApplicationWallet(t, "100.00")
	command := validProcessWagerCommand(wallet)

	before := mustApplicationMoney(t, "100.00")
	after := mustApplicationMoney(t, "90.00")
	completedAt := wagerServiceTestTime.Add(-time.Minute)

	existing, err := domain.RehydrateWagerTransaction(
		domain.RehydrateWagerTransactionParams{
			ID:                    uuid.New(),
			ProviderID:            command.ProviderID,
			ExternalTransactionID: command.ExternalTransactionID,
			IdempotencyKey:        command.IdempotencyKey,
			PayloadHash:           "different-payload-hash",
			WalletID:              wallet.ID(),
			PlayerID:              wallet.PlayerID(),
			RoundID:               command.RoundID,
			GameID:                command.GameID,
			Type:                  domain.WagerTransactionTypeBet,
			Amount:                mustApplicationMoney(t, "10.00"),
			Status:                domain.WagerTransactionStatusProcessed,
			BalanceBefore:         &before,
			BalanceAfter:          &after,
			CreatedAt: wagerServiceTestTime.Add(
				-2 * time.Minute,
			),
			UpdatedAt:   completedAt,
			CompletedAt: &completedAt,
		},
	)
	if err != nil {
		t.Fatalf("unexpected existing wager error: %v", err)
	}

	wagers.createErr = ErrAlreadyExists
	wagers.findByProviderIdempotencyResult = existing

	result, err := service.Process(
		context.Background(),
		command,
		CommandMetadata{
			CorrelationID: "correlation-123",
		},
	)

	if !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf(
			"expected ErrIdempotencyConflict, got %v",
			err,
		)
	}

	if result != nil {
		t.Fatal("expected no result after idempotency conflict")
	}
}

// mustApplicationProcessedWager creates a processed wager transaction for application tests.
func mustApplicationProcessedWager(t *testing.T, wallet *domain.Wallet, providerID string, externalTransactionID string) *domain.WagerTransaction {
	t.Helper()

	before := mustApplicationMoney(t, "100.00")
	after := mustApplicationMoney(t, "75.00")
	completedAt := wagerServiceTestTime.Add(-time.Minute)

	transaction, err := domain.RehydrateWagerTransaction(domain.RehydrateWagerTransactionParams{
		ID:                    uuid.New(),
		ProviderID:            providerID,
		ExternalTransactionID: externalTransactionID,
		IdempotencyKey:        "key-" + uuid.NewString(),
		PayloadHash:           "payload-hash",
		WalletID:              wallet.ID(),
		PlayerID:              wallet.PlayerID(),
		RoundID:               "round-123",
		GameID:                "game-123",
		Type:                  domain.WagerTransactionTypeBet,
		Amount:                mustApplicationMoney(t, "25.00"),
		Status:                domain.WagerTransactionStatusProcessed,
		BalanceBefore:         &before,
		BalanceAfter:          &after,
		CreatedAt:             wagerServiceTestTime.Add(-2 * time.Minute),
		UpdatedAt:             completedAt,
		CompletedAt:           &completedAt,
	})
	if err != nil {
		t.Fatalf("unexpected wager creation error: %v", err)
	}

	return transaction
}

// TestWagerServiceGetByIDReturnsTransaction verifies transaction retrieval by internal id.
func TestWagerServiceGetByIDReturnsTransaction(t *testing.T) {
	service, _, _, wagers, _, _ := newWagerServiceForTest()

	wallet := mustApplicationWallet(t, "75.00")
	transaction := mustApplicationProcessedWager(t, wallet, "provider-a", "transaction-123")

	wagers.findByIDResult = transaction

	result, err := service.GetByID(context.Background(), "provider-a", transaction.ID().String())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.TransactionID != transaction.ID() {
		t.Errorf("expected transaction id %s, got %s", transaction.ID(), result.TransactionID)
	}

	if result.ProviderID != "provider-a" {
		t.Errorf("expected provider-a, got %s", result.ProviderID)
	}

	if result.ExternalTransactionID != "transaction-123" {
		t.Errorf("expected transaction-123, got %s", result.ExternalTransactionID)
	}

	if result.WalletID != wallet.ID() {
		t.Errorf("expected wallet id %s, got %s", wallet.ID(), result.WalletID)
	}

	if result.PlayerID != wallet.PlayerID() {
		t.Errorf("expected player id %s, got %s", wallet.PlayerID(), result.PlayerID)
	}

	if result.Type != domain.WagerTransactionTypeBet {
		t.Errorf("expected BET, got %s", result.Type)
	}

	if result.Status != domain.WagerTransactionStatusProcessed {
		t.Errorf("expected PROCESSED, got %s", result.Status)
	}

	if result.Amount.Amount() != "25.00" {
		t.Errorf("expected amount 25.00, got %s", result.Amount.Amount())
	}

	if result.BalanceBefore == nil || result.BalanceBefore.Amount() != "100.00" {
		t.Fatal("expected balance before 100.00")
	}

	if result.BalanceAfter == nil || result.BalanceAfter.Amount() != "75.00" {
		t.Fatal("expected balance after 75.00")
	}
}

// TestWagerServiceGetByIDRejectsInvalidTransactionID verifies invalid transaction identifiers.
func TestWagerServiceGetByIDRejectsInvalidTransactionID(t *testing.T) {
	service, _, _, _, _, _ := newWagerServiceForTest()

	result, err := service.GetByID(context.Background(), "provider-a", "invalid-transaction-id")

	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}

	if result != nil {
		t.Fatal("expected nil result")
	}
}

// TestWagerServiceGetByIDPropagatesRepositoryError verifies repository failures.
func TestWagerServiceGetByIDPropagatesRepositoryError(t *testing.T) {
	service, _, _, wagers, _, _ := newWagerServiceForTest()

	expectedErr := errors.New("wager repository unavailable")
	wagers.findByIDErr = expectedErr

	result, err := service.GetByID(context.Background(), "provider-a", uuid.NewString())

	if !errors.Is(err, expectedErr) {
		t.Fatalf("expected %v, got %v", expectedErr, err)
	}

	if result != nil {
		t.Fatal("expected nil result")
	}
}

// TestWagerServiceGetByIDHidesAnotherProviderTransaction verifies provider isolation.
func TestWagerServiceGetByIDHidesAnotherProviderTransaction(t *testing.T) {
	service, _, _, wagers, _, _ := newWagerServiceForTest()

	wallet := mustApplicationWallet(t, "75.00")
	transaction := mustApplicationProcessedWager(t, wallet, "provider-b", "transaction-123")

	wagers.findByIDResult = transaction

	result, err := service.GetByID(context.Background(), "provider-a", transaction.ID().String())

	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}

	if result != nil {
		t.Fatal("expected nil result")
	}
}

// TestWagerServiceGetByExternalTransactionIDReturnsTransaction verifies provider-scoped external lookup.
func TestWagerServiceGetByExternalTransactionIDReturnsTransaction(t *testing.T) {
	service, _, _, wagers, _, _ := newWagerServiceForTest()

	wallet := mustApplicationWallet(t, "75.00")
	transaction := mustApplicationProcessedWager(t, wallet, "provider-a", "transaction-123")

	wagers.findByProviderExternalResult = transaction

	result, err := service.GetByExternalTransactionID(context.Background(), "provider-a", "transaction-123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.TransactionID != transaction.ID() {
		t.Errorf("expected transaction id %s, got %s", transaction.ID(), result.TransactionID)
	}

	if result.ProviderID != "provider-a" {
		t.Errorf("expected provider-a, got %s", result.ProviderID)
	}

	if result.ExternalTransactionID != "transaction-123" {
		t.Errorf("expected transaction-123, got %s", result.ExternalTransactionID)
	}

	if result.Status != domain.WagerTransactionStatusProcessed {
		t.Errorf("expected PROCESSED, got %s", result.Status)
	}

	if result.BalanceAfter == nil || result.BalanceAfter.Amount() != "75.00" {
		t.Fatal("expected balance after 75.00")
	}
}

// TestWagerServiceGetByExternalTransactionIDPropagatesRepositoryError verifies external lookup failures.
func TestWagerServiceGetByExternalTransactionIDPropagatesRepositoryError(t *testing.T) {
	service, _, _, wagers, _, _ := newWagerServiceForTest()

	expectedErr := errors.New("wager not found")
	wagers.findByProviderExternalErr = expectedErr

	result, err := service.GetByExternalTransactionID(context.Background(), "provider-a", "transaction-123")

	if !errors.Is(err, expectedErr) {
		t.Fatalf("expected %v, got %v", expectedErr, err)
	}

	if result != nil {
		t.Fatal("expected nil result")
	}
}
