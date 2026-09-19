//go:build integration

package integration

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pablo-banker/junglegaming-test/internal/application"
	"github.com/pablo-banker/junglegaming-test/internal/domain"
	"github.com/pablo-banker/junglegaming-test/internal/infrastructure/postgres"
)

// newIntegratedWagerService creates a WagerService backed by real PostgreSQL repositories.
func newIntegratedWagerService(
	pool *pgxpool.Pool,
) *application.WagerService {
	return application.NewWagerService(
		postgres.NewTransactionManager(pool),
		postgres.NewClock(pool),
		postgres.NewWalletRepository(pool),
		postgres.NewWagerRepository(pool),
		postgres.NewWalletLedgerRepository(pool),
		postgres.NewOutboxRepository(),
		application.DefaultReferenceRetryPolicy(),
	)
}

// createWagerServiceWallet creates a persisted wallet for service integration tests.
func createWagerServiceWallet(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	balance string,
) *domain.Wallet {
	t.Helper()

	repository := postgres.NewWalletRepository(pool)

	wallet := newWalletRepositoryWallet(
		t,
		uuid.New(),
		balance,
	)

	if err := repository.Create(ctx, wallet); err != nil {
		t.Fatalf("failed to create wallet: %v", err)
	}

	return wallet
}

// newProcessWagerCommand creates a valid wager command for integration tests.
func newProcessWagerCommand(
	wallet *domain.Wallet,
	transactionType domain.WagerTransactionType,
	amount string,
) application.ProcessWagerCommand {
	return application.ProcessWagerCommand{
		ProviderID:            "provider-" + uuid.NewString(),
		ExternalTransactionID: "transaction-" + uuid.NewString(),
		IdempotencyKey:        "idempotency-" + uuid.NewString(),
		WalletID:              wallet.ID().String(),
		PlayerID:              wallet.PlayerID().String(),
		RoundID:               "round-" + uuid.NewString(),
		GameID:                "game-1",
		Type:                  string(transactionType),
		Amount:                amount,
		Currency:              wallet.Currency().Code(),
	}
}

// wagerMetadata creates metadata for an integration command.
func wagerMetadata() application.CommandMetadata {
	return application.CommandMetadata{
		CorrelationID: uuid.NewString(),
	}
}

// readWalletState reads the persisted wallet balance and version.
func readWalletState(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	walletID uuid.UUID,
) (string, int64) {
	t.Helper()

	var (
		balance string
		version int64
	)

	err := pool.QueryRow(
		ctx,
		`
			SELECT balance::text, version
			FROM wallets
			WHERE id = $1
		`,
		walletID,
	).Scan(
		&balance,
		&version,
	)
	if err != nil {
		t.Fatalf("failed to read wallet state: %v", err)
	}

	return balance, version
}

// countWalletLedgerEntries returns the number of ledger entries for a wallet.
func countWalletLedgerEntries(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	walletID uuid.UUID,
) int {
	t.Helper()

	var count int

	err := pool.QueryRow(
		ctx,
		`
			SELECT COUNT(*)
			FROM wallet_ledger_entries
			WHERE wallet_id = $1
		`,
		walletID,
	).Scan(&count)
	if err != nil {
		t.Fatalf("failed to count ledger entries: %v", err)
	}

	return count
}

// countOutboxEventsByCorrelation returns outbox events produced by one command.
func countOutboxEventsByCorrelation(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	correlationID string,
) int {
	t.Helper()

	var count int

	err := pool.QueryRow(
		ctx,
		`
			SELECT COUNT(*)
			FROM outbox_events
			WHERE correlation_id = $1
		`,
		correlationID,
	).Scan(&count)
	if err != nil {
		t.Fatalf("failed to count outbox events: %v", err)
	}

	return count
}

// TestWagerServiceProcessesBet verifies the complete successful BET flow.
func TestWagerServiceProcessesBet(t *testing.T) {
	ctx := integrationContext(t)
	pool := openIntegrationPool(t)

	service := newIntegratedWagerService(pool)
	wallet := createWagerServiceWallet(
		t,
		ctx,
		pool,
		"100.00",
	)

	command := newProcessWagerCommand(
		wallet,
		domain.WagerTransactionTypeBet,
		"80.00",
	)

	metadata := wagerMetadata()

	result, err := service.Process(
		ctx,
		command,
		metadata,
	)
	if err != nil {
		t.Fatalf("failed to process bet: %v", err)
	}

	if result.Status != domain.WagerTransactionStatusProcessed {
		t.Fatalf(
			"expected PROCESSED, got %s",
			result.Status,
		)
	}

	if result.IdempotentReplay {
		t.Fatal("expected first request not to be a replay")
	}

	if result.BalanceBefore == nil ||
		result.BalanceBefore.Amount() != "100.00" {
		t.Fatalf(
			"expected balance before 100.00, got %v",
			result.BalanceBefore,
		)
	}

	if result.BalanceAfter == nil ||
		result.BalanceAfter.Amount() != "20.00" {
		t.Fatalf(
			"expected balance after 20.00, got %v",
			result.BalanceAfter,
		)
	}

	balance, version := readWalletState(
		t,
		ctx,
		pool,
		wallet.ID(),
	)

	if balance != "20.00" {
		t.Errorf(
			"expected persisted balance 20.00, got %s",
			balance,
		)
	}

	if version != 2 {
		t.Errorf(
			"expected wallet version 2, got %d",
			version,
		)
	}

	if countWalletLedgerEntries(
		t,
		ctx,
		pool,
		wallet.ID(),
	) != 1 {
		t.Fatal("expected exactly one ledger entry")
	}

	if countOutboxEventsByCorrelation(
		t,
		ctx,
		pool,
		metadata.CorrelationID,
	) != 2 {
		t.Fatal("expected processed and balance changed outbox events")
	}
}

// TestWagerServiceRejectsBetWithoutFunds verifies insufficient funds do not move money.
func TestWagerServiceRejectsBetWithoutFunds(t *testing.T) {
	ctx := integrationContext(t)
	pool := openIntegrationPool(t)

	service := newIntegratedWagerService(pool)
	wallet := createWagerServiceWallet(
		t,
		ctx,
		pool,
		"50.00",
	)

	command := newProcessWagerCommand(
		wallet,
		domain.WagerTransactionTypeBet,
		"80.00",
	)

	metadata := wagerMetadata()

	result, err := service.Process(
		ctx,
		command,
		metadata,
	)
	if err != nil {
		t.Fatalf("failed to process bet: %v", err)
	}

	if result.Status != domain.WagerTransactionStatusRejected {
		t.Fatalf(
			"expected REJECTED, got %s",
			result.Status,
		)
	}

	if result.FailureCode != "BET_INSUFFICIENT_FUNDS" {
		t.Errorf(
			"expected BET_INSUFFICIENT_FUNDS, got %s",
			result.FailureCode,
		)
	}

	balance, version := readWalletState(
		t,
		ctx,
		pool,
		wallet.ID(),
	)

	if balance != "50.00" {
		t.Errorf(
			"expected balance 50.00, got %s",
			balance,
		)
	}

	if version != 1 {
		t.Errorf(
			"expected version 1, got %d",
			version,
		)
	}

	if countWalletLedgerEntries(
		t,
		ctx,
		pool,
		wallet.ID(),
	) != 0 {
		t.Fatal("expected rejected bet to create no ledger entry")
	}

	if countOutboxEventsByCorrelation(
		t,
		ctx,
		pool,
		metadata.CorrelationID,
	) != 1 {
		t.Fatal("expected exactly one rejected outbox event")
	}
}

// TestWagerServiceProcessesLossWithoutBalanceMovement verifies LOSS has no financial movement.
func TestWagerServiceProcessesLossWithoutBalanceMovement(t *testing.T) {
	ctx := integrationContext(t)
	pool := openIntegrationPool(t)

	service := newIntegratedWagerService(pool)
	wallet := createWagerServiceWallet(
		t,
		ctx,
		pool,
		"100.00",
	)

	command := newProcessWagerCommand(
		wallet,
		domain.WagerTransactionTypeLoss,
		"0.00",
	)

	metadata := wagerMetadata()

	result, err := service.Process(
		ctx,
		command,
		metadata,
	)
	if err != nil {
		t.Fatalf("failed to process loss: %v", err)
	}

	if result.Status != domain.WagerTransactionStatusProcessed {
		t.Fatalf(
			"expected PROCESSED, got %s",
			result.Status,
		)
	}

	balance, version := readWalletState(
		t,
		ctx,
		pool,
		wallet.ID(),
	)

	if balance != "100.00" {
		t.Errorf(
			"expected balance 100.00, got %s",
			balance,
		)
	}

	if version != 1 {
		t.Errorf(
			"expected version 1, got %d",
			version,
		)
	}

	if countWalletLedgerEntries(
		t,
		ctx,
		pool,
		wallet.ID(),
	) != 0 {
		t.Fatal("expected LOSS to create no ledger entry")
	}

	if countOutboxEventsByCorrelation(
		t,
		ctx,
		pool,
		metadata.CorrelationID,
	) != 1 {
		t.Fatal("expected only WagerTransactionProcessed event")
	}
}

// TestWagerServiceReplaysIdempotentRequest verifies the same request cannot move money twice.
func TestWagerServiceReplaysIdempotentRequest(t *testing.T) {
	ctx := integrationContext(t)
	pool := openIntegrationPool(t)

	service := newIntegratedWagerService(pool)
	wallet := createWagerServiceWallet(
		t,
		ctx,
		pool,
		"100.00",
	)

	command := newProcessWagerCommand(
		wallet,
		domain.WagerTransactionTypeBet,
		"25.00",
	)

	first, err := service.Process(
		ctx,
		command,
		wagerMetadata(),
	)
	if err != nil {
		t.Fatalf("failed to process first request: %v", err)
	}

	second, err := service.Process(
		ctx,
		command,
		wagerMetadata(),
	)
	if err != nil {
		t.Fatalf("failed to replay request: %v", err)
	}

	if first.IdempotentReplay {
		t.Fatal("expected first request not to be replay")
	}

	if !second.IdempotentReplay {
		t.Fatal("expected second request to be replay")
	}

	if second.TransactionID != first.TransactionID {
		t.Errorf(
			"expected transaction %s, got %s",
			first.TransactionID,
			second.TransactionID,
		)
	}

	if second.Status != domain.WagerTransactionStatusProcessed {
		t.Fatalf(
			"expected PROCESSED replay, got %s",
			second.Status,
		)
	}

	if second.BalanceBefore == nil ||
		second.BalanceBefore.Amount() != "100.00" {
		t.Fatal("expected original balance before 100.00")
	}

	if second.BalanceAfter == nil ||
		second.BalanceAfter.Amount() != "75.00" {
		t.Fatal("expected original balance after 75.00")
	}

	balance, version := readWalletState(
		t,
		ctx,
		pool,
		wallet.ID(),
	)

	if balance != "75.00" {
		t.Errorf(
			"expected balance 75.00, got %s",
			balance,
		)
	}

	if version != 2 {
		t.Errorf(
			"expected version 2, got %d",
			version,
		)
	}

	if countWalletLedgerEntries(
		t,
		ctx,
		pool,
		wallet.ID(),
	) != 1 {
		t.Fatal("expected replay not to duplicate ledger")
	}

	var outboxCount int

	err = pool.QueryRow(
		ctx,
		`
			SELECT COUNT(*)
			FROM outbox_events
			WHERE payload ->> 'transactionId' = $1
		`,
		first.TransactionID.String(),
	).Scan(&outboxCount)
	if err != nil {
		t.Fatalf("failed to count transaction events: %v", err)
	}

	if outboxCount != 2 {
		t.Fatalf(
			"expected replay not to duplicate outbox events, got %d",
			outboxCount,
		)
	}
}
