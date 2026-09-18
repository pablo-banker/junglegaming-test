package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/pablo-banker/junglegaming-test/internal/domain"
)

var walletServiceTestTime = time.Date(
	2026,
	9,
	18,
	15,
	0,
	0,
	0,
	time.UTC,
)

// newWalletServiceForTest creates WalletService dependencies for tests.
func newWalletServiceForTest() (
	*WalletService,
	*fakeTransactionManager,
	*fakeWalletRepository,
	*fakeWagerRepository,
	*fakeLedgerRepository,
	*fakeOutboxRepository,
) {
	txManager := &fakeTransactionManager{}
	clock := &fakeClock{
		now: walletServiceTestTime,
	}
	wallets := &fakeWalletRepository{}
	wagers := &fakeWagerRepository{}
	ledger := &fakeLedgerRepository{}
	outbox := &fakeOutboxRepository{}

	service := NewWalletService(
		txManager,
		clock,
		wallets,
		wagers,
		ledger,
		outbox,
	)

	return service, txManager, wallets, wagers, ledger, outbox
}

// TestWalletServiceCreateZeroBalanceCreatesOnlyWallet verifies zero-balance creation.
func TestWalletServiceCreateZeroBalanceCreatesOnlyWallet(t *testing.T) {
	service, txManager, wallets, wagers, ledger, outbox :=
		newWalletServiceForTest()

	playerID := uuid.New()

	result, err := service.Create(
		context.Background(),
		CreateWalletCommand{
			PlayerID:       playerID.String(),
			Currency:       "BRL",
			InitialBalance: "0.00",
		},
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

	if len(wallets.created) != 1 {
		t.Fatalf("expected 1 wallet, got %d", len(wallets.created))
	}

	if len(wagers.created) != 0 {
		t.Errorf("expected no wager, got %d", len(wagers.created))
	}

	if len(ledger.created) != 0 {
		t.Errorf("expected no ledger entry, got %d", len(ledger.created))
	}

	if len(outbox.created) != 0 {
		t.Errorf("expected no outbox events, got %d", len(outbox.created))
	}

	if result.PlayerID != playerID {
		t.Errorf("expected player %s, got %s", playerID, result.PlayerID)
	}

	if result.Balance.Amount() != "0.00" {
		t.Errorf("expected balance 0.00, got %s", result.Balance.Amount())
	}

	if result.Version != 1 {
		t.Errorf("expected version 1, got %d", result.Version)
	}
}

// TestWalletServiceCreatePositiveBalanceCreatesOpeningRecords verifies opening flow.
func TestWalletServiceCreatePositiveBalanceCreatesOpeningRecords(t *testing.T) {
	service, _, wallets, wagers, ledger, outbox :=
		newWalletServiceForTest()

	playerID := uuid.New()

	result, err := service.Create(
		context.Background(),
		CreateWalletCommand{
			PlayerID:       playerID.String(),
			Currency:       "BRL",
			InitialBalance: "100.00",
		},
		CommandMetadata{
			CorrelationID: "correlation-123",
			CausationID:   "request-123",
		},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(wallets.created) != 1 {
		t.Fatalf("expected 1 wallet, got %d", len(wallets.created))
	}

	if len(wagers.created) != 1 {
		t.Fatalf("expected 1 opening wager, got %d", len(wagers.created))
	}

	if len(ledger.created) != 1 {
		t.Fatalf("expected 1 ledger entry, got %d", len(ledger.created))
	}

	if len(outbox.created) != 2 {
		t.Fatalf("expected 2 outbox events, got %d", len(outbox.created))
	}

	wallet := wallets.created[0]
	opening := wagers.created[0]
	entry := ledger.created[0]

	if wallet.Balance().Amount() != "100.00" {
		t.Errorf(
			"expected wallet balance 100.00, got %s",
			wallet.Balance().Amount(),
		)
	}

	if wallet.Version() != 1 {
		t.Errorf("expected wallet version 1, got %d", wallet.Version())
	}

	if opening.Type() != domain.WagerTransactionTypeOpening {
		t.Errorf("expected OPENING, got %s", opening.Type())
	}

	if opening.Status() != domain.WagerTransactionStatusProcessed {
		t.Errorf("expected PROCESSED, got %s", opening.Status())
	}

	if entry.Direction() != domain.WalletLedgerDirectionCredit {
		t.Errorf("expected CREDIT, got %s", entry.Direction())
	}

	if entry.BalanceBefore().Amount() != "0.00" {
		t.Errorf(
			"expected ledger balance before 0.00, got %s",
			entry.BalanceBefore().Amount(),
		)
	}

	if entry.BalanceAfter().Amount() != "100.00" {
		t.Errorf(
			"expected ledger balance after 100.00, got %s",
			entry.BalanceAfter().Amount(),
		)
	}

	if outbox.created[0].EventType != EventTypeWagerTransactionProcessed {
		t.Errorf(
			"expected first event %s, got %s",
			EventTypeWagerTransactionProcessed,
			outbox.created[0].EventType,
		)
	}

	if outbox.created[1].EventType != EventTypeWalletBalanceChanged {
		t.Errorf(
			"expected second event %s, got %s",
			EventTypeWalletBalanceChanged,
			outbox.created[1].EventType,
		)
	}

	if result.WalletID != wallet.ID() {
		t.Error("expected result wallet id to match created wallet")
	}

	if result.Version != 1 {
		t.Errorf("expected result version 1, got %d", result.Version)
	}
}

// TestWalletServiceCreatePropagatesTransactionError verifies failures stop the use case.
func TestWalletServiceCreatePropagatesTransactionError(t *testing.T) {
	service, _, _, _, _, outbox := newWalletServiceForTest()

	expectedErr := errors.New("outbox unavailable")
	outbox.err = expectedErr

	result, err := service.Create(
		context.Background(),
		CreateWalletCommand{
			PlayerID:       uuid.NewString(),
			Currency:       "BRL",
			InitialBalance: "100.00",
		},
		CommandMetadata{
			CorrelationID: "correlation-123",
		},
	)

	if !errors.Is(err, expectedErr) {
		t.Fatalf("expected %v, got %v", expectedErr, err)
	}

	if result != nil {
		t.Fatal("expected no result after transaction failure")
	}
}
