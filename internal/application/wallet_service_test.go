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

// newWalletForTest creates a wallet with the given balance.
func newWalletForTest(t *testing.T, amount string) *domain.Wallet {
	t.Helper()

	currency, err := domain.NewCurrency("BRL")
	if err != nil {
		t.Fatalf("unexpected currency error: %v", err)
	}

	balance, err := domain.ParseMoney(amount, currency)
	if err != nil {
		t.Fatalf("unexpected money error: %v", err)
	}

	wallet, err := domain.NewWallet(
		uuid.New(),
		uuid.New(),
		balance,
		walletServiceTestTime,
	)
	if err != nil {
		t.Fatalf("unexpected wallet error: %v", err)
	}

	return wallet
}

// newLedgerEntryForTest creates a ledger entry for tests.
func newLedgerEntryForTest(t *testing.T, wallet *domain.Wallet, amount string) *domain.WalletLedgerEntry {
	t.Helper()

	currency := wallet.Balance().Currency()

	entryAmount, err := domain.ParseMoney(
		amount,
		currency,
	)
	if err != nil {
		t.Fatalf("unexpected amount error: %v", err)
	}

	zero, err := domain.Zero(currency)
	if err != nil {
		t.Fatalf("unexpected zero money error: %v", err)
	}

	entry, err := domain.NewWalletLedgerEntry(
		uuid.New(),
		wallet.ID(),
		uuid.New(),
		domain.WalletLedgerDirectionCredit,
		entryAmount,
		zero,
		entryAmount,
		walletServiceTestTime,
	)
	if err != nil {
		t.Fatalf("unexpected ledger entry error: %v", err)
	}

	return entry
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

// TestWalletServiceGetReturnsWallet verifies wallet retrieval.
func TestWalletServiceGetReturnsWallet(t *testing.T) {
	service, _, wallets, _, _, _ := newWalletServiceForTest()

	wallet := newWalletForTest(t, "100.00")
	wallets.findByIDResult = wallet

	result, err := service.Get(
		context.Background(),
		wallet.ID().String(),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.WalletID != wallet.ID() {
		t.Errorf(
			"expected wallet id %s, got %s",
			wallet.ID(),
			result.WalletID,
		)
	}

	if result.PlayerID != wallet.PlayerID() {
		t.Errorf(
			"expected player id %s, got %s",
			wallet.PlayerID(),
			result.PlayerID,
		)
	}

	if result.Balance.Amount() != "100.00" {
		t.Errorf(
			"expected balance 100.00, got %s",
			result.Balance.Amount(),
		)
	}

	if result.Version != 1 {
		t.Errorf("expected version 1, got %d", result.Version)
	}
}

// TestWalletServiceGetRejectsInvalidWalletID verifies invalid wallet identifiers.
func TestWalletServiceGetRejectsInvalidWalletID(t *testing.T) {
	service, _, _, _, _, _ := newWalletServiceForTest()

	result, err := service.Get(
		context.Background(),
		"invalid-wallet-id",
	)

	if !errors.Is(err, ErrWalletNotFound) {
		t.Fatalf("expected ErrWalletNotFound, got %v", err)
	}

	if result != nil {
		t.Fatal("expected nil result")
	}
}

// TestWalletServiceGetPropagatesRepositoryError verifies repository failures.
func TestWalletServiceGetPropagatesRepositoryError(t *testing.T) {
	service, _, wallets, _, _, _ := newWalletServiceForTest()

	expectedErr := errors.New("wallet repository unavailable")
	wallets.findByIDErr = expectedErr

	result, err := service.Get(
		context.Background(),
		uuid.NewString(),
	)

	if !errors.Is(err, expectedErr) {
		t.Fatalf("expected %v, got %v", expectedErr, err)
	}

	if result != nil {
		t.Fatal("expected nil result")
	}
}

// TestWalletServiceListLedgerReturnsEntries verifies ledger retrieval.
func TestWalletServiceListLedgerReturnsEntries(t *testing.T) {
	service, _, wallets, _, ledger, _ := newWalletServiceForTest()

	wallet := newWalletForTest(t, "100.00")
	entry := newLedgerEntryForTest(t, wallet, "100.00")

	wallets.findByIDResult = wallet
	ledger.listed = []*domain.WalletLedgerEntry{
		entry,
	}

	result, err := service.ListLedger(
		context.Background(),
		wallet.ID().String(),
		nil,
		nil,
		50,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Entries) != 1 {
		t.Fatalf(
			"expected 1 ledger entry, got %d",
			len(result.Entries),
		)
	}

	if result.Entries[0].ID() != entry.ID() {
		t.Errorf(
			"expected entry %s, got %s",
			entry.ID(),
			result.Entries[0].ID(),
		)
	}
}

// TestWalletServiceListLedgerReturnsEmptyEntries verifies wallets without ledger entries.
func TestWalletServiceListLedgerReturnsEmptyEntries(t *testing.T) {
	service, _, wallets, _, ledger, _ := newWalletServiceForTest()

	wallet := newWalletForTest(t, "0.00")

	wallets.findByIDResult = wallet
	ledger.listed = []*domain.WalletLedgerEntry{}

	result, err := service.ListLedger(
		context.Background(),
		wallet.ID().String(),
		nil,
		nil,
		50,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Entries) != 0 {
		t.Fatalf(
			"expected no ledger entries, got %d",
			len(result.Entries),
		)
	}
}

// TestWalletServiceListLedgerRejectsInvalidWalletID verifies invalid wallet identifiers.
func TestWalletServiceListLedgerRejectsInvalidWalletID(t *testing.T) {
	service, _, _, _, _, _ := newWalletServiceForTest()

	result, err := service.ListLedger(
		context.Background(),
		"invalid-wallet-id",
		nil,
		nil,
		50,
	)

	if !errors.Is(err, ErrWalletNotFound) {
		t.Fatalf("expected ErrWalletNotFound, got %v", err)
	}

	if result != nil {
		t.Fatal("expected nil result")
	}
}

// TestWalletServiceListLedgerPropagatesWalletError verifies wallet lookup failures.
func TestWalletServiceListLedgerPropagatesWalletError(t *testing.T) {
	service, _, wallets, _, _, _ := newWalletServiceForTest()

	expectedErr := errors.New("wallet lookup failed")
	wallets.findByIDErr = expectedErr

	result, err := service.ListLedger(
		context.Background(),
		uuid.NewString(),
		nil,
		nil,
		50,
	)

	if !errors.Is(err, expectedErr) {
		t.Fatalf("expected %v, got %v", expectedErr, err)
	}

	if result != nil {
		t.Fatal("expected nil result")
	}
}

// TestWalletServiceListLedgerPropagatesLedgerError verifies ledger failures.
func TestWalletServiceListLedgerPropagatesLedgerError(t *testing.T) {
	service, _, wallets, _, ledger, _ := newWalletServiceForTest()

	wallet := newWalletForTest(t, "100.00")
	wallets.findByIDResult = wallet

	expectedErr := errors.New("ledger unavailable")
	ledger.listErr = expectedErr

	result, err := service.ListLedger(
		context.Background(),
		wallet.ID().String(),
		nil,
		nil,
		50,
	)

	if !errors.Is(err, expectedErr) {
		t.Fatalf("expected %v, got %v", expectedErr, err)
	}

	if result != nil {
		t.Fatal("expected nil result")
	}
}

// TestWalletServiceReconcileReturnsConsistent verifies matching wallet and ledger balances.
func TestWalletServiceReconcileReturnsConsistent(t *testing.T) {
	service, txManager, wallets, _, ledger, _ :=
		newWalletServiceForTest()

	wallet := newWalletForTest(t, "100.00")
	wallets.findByIDForUpdateResult = wallet

	ledger.calculatedBalance = wallet.Balance()
	ledger.calculatedCount = 3

	result, err := service.Reconcile(
		context.Background(),
		wallet.ID().String(),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !txManager.called {
		t.Fatal("expected transaction to be started")
	}

	if !result.Consistent {
		t.Fatal("expected reconciliation to be consistent")
	}

	if result.StoredBalance.Amount() != "100.00" {
		t.Errorf(
			"expected stored balance 100.00, got %s",
			result.StoredBalance.Amount(),
		)
	}

	if result.CalculatedBalance.Amount() != "100.00" {
		t.Errorf(
			"expected calculated balance 100.00, got %s",
			result.CalculatedBalance.Amount(),
		)
	}

	if result.CheckedEntries != 3 {
		t.Errorf(
			"expected 3 checked entries, got %d",
			result.CheckedEntries,
		)
	}
}

// TestWalletServiceReconcileReturnsInconsistent verifies balance mismatches.
func TestWalletServiceReconcileReturnsInconsistent(t *testing.T) {
	service, _, wallets, _, ledger, _ :=
		newWalletServiceForTest()

	wallet := newWalletForTest(t, "100.00")
	wallets.findByIDForUpdateResult = wallet

	currency := wallet.Balance().Currency()

	calculatedBalance, err := domain.ParseMoney(
		"90.00",
		currency,
	)
	if err != nil {
		t.Fatalf("unexpected money error: %v", err)
	}

	ledger.calculatedBalance = calculatedBalance
	ledger.calculatedCount = 2

	result, err := service.Reconcile(
		context.Background(),
		wallet.ID().String(),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Consistent {
		t.Fatal("expected reconciliation to be inconsistent")
	}

	if result.StoredBalance.Amount() != "100.00" {
		t.Errorf(
			"expected stored balance 100.00, got %s",
			result.StoredBalance.Amount(),
		)
	}

	if result.CalculatedBalance.Amount() != "90.00" {
		t.Errorf(
			"expected calculated balance 90.00, got %s",
			result.CalculatedBalance.Amount(),
		)
	}
}

// TestWalletServiceReconcileRejectsInvalidWalletID verifies invalid wallet identifiers.
func TestWalletServiceReconcileRejectsInvalidWalletID(t *testing.T) {
	service, txManager, _, _, _, _ :=
		newWalletServiceForTest()

	result, err := service.Reconcile(
		context.Background(),
		"invalid-wallet-id",
	)

	if !errors.Is(err, ErrWalletNotFound) {
		t.Fatalf("expected ErrWalletNotFound, got %v", err)
	}

	if result != nil {
		t.Fatal("expected nil result")
	}

	if txManager.called {
		t.Fatal("expected transaction not to start")
	}
}

// TestWalletServiceReconcilePropagatesWalletError verifies locked wallet lookup failures.
func TestWalletServiceReconcilePropagatesWalletError(t *testing.T) {
	service, txManager, wallets, _, _, _ := newWalletServiceForTest()

	expectedErr := errors.New("wallet lock failed")
	wallets.findByIDForUpdateErr = expectedErr

	result, err := service.Reconcile(
		context.Background(),
		uuid.NewString(),
	)

	if !errors.Is(err, expectedErr) {
		t.Fatalf("expected %v, got %v", expectedErr, err)
	}

	if result != nil {
		t.Fatal("expected nil result")
	}

	if !txManager.called {
		t.Fatal("expected transaction to be started")
	}
}

// TestWalletServiceReconcilePropagatesLedgerError verifies ledger reconstruction failures.
func TestWalletServiceReconcilePropagatesLedgerError(t *testing.T) {
	service, txManager, wallets, _, ledger, _ := newWalletServiceForTest()

	wallet := newWalletForTest(t, "100.00")
	wallets.findByIDForUpdateResult = wallet

	expectedErr := errors.New("ledger calculation failed")
	ledger.calculateErr = expectedErr

	result, err := service.Reconcile(
		context.Background(),
		wallet.ID().String(),
	)

	if !errors.Is(err, expectedErr) {
		t.Fatalf("expected %v, got %v", expectedErr, err)
	}

	if result != nil {
		t.Fatal("expected nil result")
	}

	if !txManager.called {
		t.Fatal("expected transaction to be started")
	}
}
