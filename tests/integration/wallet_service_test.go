package integration

import (
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pablo-banker/junglegaming-test/internal/application"
	"github.com/pablo-banker/junglegaming-test/internal/infrastructure/postgres"
)

// newIntegratedWalletService creates a WalletService backed by real PostgreSQL repositories.
func newIntegratedWalletService(
	pool *pgxpool.Pool,
) *application.WalletService {
	return application.NewWalletService(
		postgres.NewTransactionManager(pool),
		postgres.NewClock(pool),
		postgres.NewWalletRepository(pool),
		postgres.NewWagerRepository(pool),
		postgres.NewWalletLedgerRepository(pool),
		postgres.NewOutboxRepository(),
	)
}

// TestWalletServiceCreatesZeroBalanceWallet verifies zero initial balance creates no financial movement.
func TestWalletServiceCreatesZeroBalanceWallet(t *testing.T) {
	ctx := integrationContext(t)
	pool := openIntegrationPool(t)

	service := newIntegratedWalletService(pool)

	playerID := uuid.New()

	command := application.CreateWalletCommand{
		PlayerID:       playerID.String(),
		Currency:       "BRL",
		InitialBalance: "0.00",
	}

	result, err := service.Create(
		ctx,
		command,
		wagerMetadata(),
	)
	if err != nil {
		t.Fatalf("failed to create wallet: %v", err)
	}

	if result.PlayerID != playerID {
		t.Errorf(
			"expected player id %s, got %s",
			playerID,
			result.PlayerID,
		)
	}

	if result.Balance.Amount() != "0.00" {
		t.Errorf(
			"expected balance 0.00, got %s",
			result.Balance.Amount(),
		)
	}

	if result.Version != 1 {
		t.Errorf(
			"expected version 1, got %d",
			result.Version,
		)
	}

	var (
		wagerCount  int
		ledgerCount int
		outboxCount int
	)

	err = pool.QueryRow(
		ctx,
		`
			SELECT COUNT(*)
			FROM wager_transactions
			WHERE wallet_id = $1
		`,
		result.WalletID,
	).Scan(&wagerCount)
	if err != nil {
		t.Fatalf("failed to count wagers: %v", err)
	}

	err = pool.QueryRow(
		ctx,
		`
			SELECT COUNT(*)
			FROM wallet_ledger_entries
			WHERE wallet_id = $1
		`,
		result.WalletID,
	).Scan(&ledgerCount)
	if err != nil {
		t.Fatalf("failed to count ledger entries: %v", err)
	}

	err = pool.QueryRow(
		ctx,
		`
			SELECT COUNT(*)
			FROM outbox_events
			WHERE aggregate_id = $1
		`,
		result.WalletID,
	).Scan(&outboxCount)
	if err != nil {
		t.Fatalf("failed to count outbox events: %v", err)
	}

	if wagerCount != 0 {
		t.Errorf(
			"expected no opening wager, got %d",
			wagerCount,
		)
	}

	if ledgerCount != 0 {
		t.Errorf(
			"expected no ledger entry, got %d",
			ledgerCount,
		)
	}

	if outboxCount != 0 {
		t.Errorf(
			"expected no financial events, got %d",
			outboxCount,
		)
	}
}

// TestWalletServiceCreatesWalletWithOpeningTransaction verifies positive initial balance creates opening financial records.
func TestWalletServiceCreatesWalletWithOpeningTransaction(t *testing.T) {
	ctx := integrationContext(t)
	pool := openIntegrationPool(t)

	service := newIntegratedWalletService(pool)

	playerID := uuid.New()
	metadata := wagerMetadata()

	command := application.CreateWalletCommand{
		PlayerID:       playerID.String(),
		Currency:       "BRL",
		InitialBalance: "100.00",
	}

	result, err := service.Create(
		ctx,
		command,
		metadata,
	)
	if err != nil {
		t.Fatalf("failed to create wallet: %v", err)
	}

	if result.Balance.Amount() != "100.00" {
		t.Errorf(
			"expected balance 100.00, got %s",
			result.Balance.Amount(),
		)
	}

	if result.Version != 1 {
		t.Errorf(
			"expected opening wallet version 1, got %d",
			result.Version,
		)
	}

	var (
		persistedBalance string
		persistedVersion int64
	)

	err = pool.QueryRow(
		ctx,
		`
			SELECT balance::text, version
			FROM wallets
			WHERE id = $1
		`,
		result.WalletID,
	).Scan(
		&persistedBalance,
		&persistedVersion,
	)
	if err != nil {
		t.Fatalf("failed to read wallet: %v", err)
	}

	if persistedBalance != "100.00" {
		t.Errorf(
			"expected persisted balance 100.00, got %s",
			persistedBalance,
		)
	}

	if persistedVersion != 1 {
		t.Errorf(
			"expected persisted version 1, got %d",
			persistedVersion,
		)
	}

	var (
		openingID            uuid.UUID
		openingStatus        string
		openingAmount        string
		openingBalanceBefore string
		openingBalanceAfter  string
	)

	err = pool.QueryRow(
		ctx,
		`
			SELECT
				id,
				status::text,
				amount::text,
				balance_before::text,
				balance_after::text
			FROM wager_transactions
			WHERE wallet_id = $1
			  AND type = 'OPENING'
		`,
		result.WalletID,
	).Scan(
		&openingID,
		&openingStatus,
		&openingAmount,
		&openingBalanceBefore,
		&openingBalanceAfter,
	)
	if err != nil {
		t.Fatalf("failed to read opening wager: %v", err)
	}

	if openingStatus != "PROCESSED" {
		t.Errorf(
			"expected opening PROCESSED, got %s",
			openingStatus,
		)
	}

	if openingAmount != "100.00" {
		t.Errorf(
			"expected opening amount 100.00, got %s",
			openingAmount,
		)
	}

	if openingBalanceBefore != "0.00" {
		t.Errorf(
			"expected opening balance before 0.00, got %s",
			openingBalanceBefore,
		)
	}

	if openingBalanceAfter != "100.00" {
		t.Errorf(
			"expected opening balance after 100.00, got %s",
			openingBalanceAfter,
		)
	}

	var (
		direction     string
		ledgerAmount  string
		balanceBefore string
		balanceAfter  string
	)

	err = pool.QueryRow(
		ctx,
		`
			SELECT
				direction::text,
				amount::text,
				balance_before::text,
				balance_after::text
			FROM wallet_ledger_entries
			WHERE wallet_id = $1
			  AND transaction_id = $2
		`,
		result.WalletID,
		openingID,
	).Scan(
		&direction,
		&ledgerAmount,
		&balanceBefore,
		&balanceAfter,
	)
	if err != nil {
		t.Fatalf("failed to read opening ledger entry: %v", err)
	}

	if direction != "CREDIT" {
		t.Errorf(
			"expected CREDIT, got %s",
			direction,
		)
	}

	if ledgerAmount != "100.00" {
		t.Errorf(
			"expected ledger amount 100.00, got %s",
			ledgerAmount,
		)
	}

	if balanceBefore != "0.00" {
		t.Errorf(
			"expected ledger balance before 0.00, got %s",
			balanceBefore,
		)
	}

	if balanceAfter != "100.00" {
		t.Errorf(
			"expected ledger balance after 100.00, got %s",
			balanceAfter,
		)
	}

	var outboxCount int

	err = pool.QueryRow(
		ctx,
		`
			SELECT COUNT(*)
			FROM outbox_events
			WHERE correlation_id = $1
		`,
		metadata.CorrelationID,
	).Scan(&outboxCount)
	if err != nil {
		t.Fatalf("failed to count opening events: %v", err)
	}

	if outboxCount != 2 {
		t.Errorf(
			"expected 2 opening events, got %d",
			outboxCount,
		)
	}

	rows, err := pool.Query(
		ctx,
		`
		SELECT event_type
		FROM outbox_events
		WHERE correlation_id = $1
		ORDER BY event_type
	`,
		metadata.CorrelationID,
	)
	if err != nil {
		t.Fatalf("failed to read opening events: %v", err)
	}
	defer rows.Close()

	eventTypes := make(map[string]bool)

	for rows.Next() {
		var eventType string

		if err := rows.Scan(&eventType); err != nil {
			t.Fatalf("failed to scan event type: %v", err)
		}

		eventTypes[eventType] = true
	}

	if err := rows.Err(); err != nil {
		t.Fatalf("failed while reading event types: %v", err)
	}

	if !eventTypes["WagerTransactionProcessed"] {
		t.Error("expected WagerTransactionProcessed event")
	}

	if !eventTypes["WalletBalanceChanged"] {
		t.Error("expected WalletBalanceChanged event")
	}
}
