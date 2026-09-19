//go:build integration

package integration

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pablo-banker/junglegaming-test/internal/application"
	"github.com/pablo-banker/junglegaming-test/internal/domain"
	"github.com/pablo-banker/junglegaming-test/internal/infrastructure/postgres"
	infrasqs "github.com/pablo-banker/junglegaming-test/internal/infrastructure/sqs"
)

type failCompleteInboxRepository struct {
	repository application.InboxRepository
	err        error
}

// Register delegates inbox registration to the real repository.
func (f *failCompleteInboxRepository) Register(ctx context.Context, consumerName string, messageID string, messageHash string) (bool, error) {
	return f.repository.Register(ctx, consumerName, messageID, messageHash)
}

// Complete forces inbox completion to fail.
func (f *failCompleteInboxRepository) Complete(context.Context, string, string, time.Time) error {
	return f.err
}

// TestWagerMessageHandlerRollsBackFinancialProcessing verifies inbox and financial state rollback together.
func TestWagerMessageHandlerRollsBackFinancialProcessing(t *testing.T) {
	pool := openIntegrationPool(t)
	ctx := integrationContext(t)

	txManager := postgres.NewTransactionManager(pool)
	walletRepository := postgres.NewWalletRepository(pool)
	wagerRepository := postgres.NewWagerRepository(pool)
	ledgerRepository := postgres.NewWalletLedgerRepository(pool)
	outboxRepository := postgres.NewOutboxRepository()
	inboxRepository := postgres.NewInboxRepository(pool)

	now := time.Now().UTC()

	currency, err := domain.NewCurrency("BRL")
	if err != nil {
		t.Fatalf("failed to create currency: %v", err)
	}

	balance, err := domain.ParseMoney("100.00", currency)
	if err != nil {
		t.Fatalf("failed to create wallet balance: %v", err)
	}

	wallet, err := domain.NewWallet(uuid.New(), uuid.New(), balance, now)
	if err != nil {
		t.Fatalf("failed to create wallet: %v", err)
	}

	err = walletRepository.Create(ctx, wallet)
	if err != nil {
		t.Fatalf("failed to persist wallet: %v", err)
	}

	clock := postgres.NewClock(pool)

	wagerService := application.NewWagerService(
		txManager,
		clock,
		walletRepository,
		wagerRepository,
		ledgerRepository,
		outboxRepository,
	)

	expectedErr := errors.New("forced inbox completion failure")

	failingInbox := &failCompleteInboxRepository{
		repository: inboxRepository,
		err:        expectedErr,
	}

	handler := infrasqs.NewWagerMessageHandler(
		txManager,
		failingInbox,
		wagerService,
		clock,
	)

	messageID := "message-" + uuid.NewString()
	externalTransactionID := "transaction-" + uuid.NewString()
	idempotencyKey := "key-" + uuid.NewString()
	correlationID := "correlation-" + uuid.NewString()

	body := fmt.Sprintf(
		`{
			"correlationId":"%s",
			"command":{
				"providerId":"provider-a",
				"externalTransactionId":"%s",
				"idempotencyKey":"%s",
				"walletId":"%s",
				"playerId":"%s",
				"roundId":"round-123",
				"gameId":"game-123",
				"type":"BET",
				"amount":"25.00",
				"currency":"BRL"
			}
		}`,
		correlationID,
		externalTransactionID,
		idempotencyKey,
		wallet.ID().String(),
		wallet.PlayerID().String(),
	)

	err = handler.Handle(ctx, messageID, body)

	if !errors.Is(err, expectedErr) {
		t.Fatalf("expected forced inbox completion failure, got %v", err)
	}

	assertWalletState(t, ctx, pool, wallet.ID(), "100.00", 1)
	assertWagerCount(t, ctx, pool, externalTransactionID, 0)
	assertLedgerCount(t, ctx, pool, wallet.ID(), 0)
	assertOutboxCount(t, ctx, pool, correlationID, 0)
	assertInboxCount(t, ctx, pool, messageID, 0)

	retryHandler := infrasqs.NewWagerMessageHandler(
		txManager,
		inboxRepository,
		wagerService,
		clock,
	)

	err = retryHandler.Handle(ctx, messageID, body)
	if err != nil {
		t.Fatalf("expected retry to succeed, got %v", err)
	}

	assertWalletState(t, ctx, pool, wallet.ID(), "75.00", 2)
	assertWagerCount(t, ctx, pool, externalTransactionID, 1)
	assertLedgerCount(t, ctx, pool, wallet.ID(), 1)
	assertOutboxExists(t, ctx, pool, correlationID)
	assertCompletedInbox(t, ctx, pool, messageID)
}

// assertWalletState verifies the persisted wallet balance and version.
func assertWalletState(t *testing.T, ctx context.Context, pool *pgxpool.Pool, walletID uuid.UUID, expectedBalance string, expectedVersion int64) {
	t.Helper()

	var balance string
	var version int64

	err := pool.QueryRow(
		ctx,
		`
			SELECT balance::text, version
			FROM wallets
			WHERE id = $1
		`,
		walletID,
	).Scan(&balance, &version)
	if err != nil {
		t.Fatalf("failed to query wallet: %v", err)
	}

	if balance != expectedBalance {
		t.Errorf("expected balance %s, got %s", expectedBalance, balance)
	}

	if version != expectedVersion {
		t.Errorf("expected version %d, got %d", expectedVersion, version)
	}
}

// assertWagerCount verifies how many wager transactions exist for an external transaction.
func assertWagerCount(t *testing.T, ctx context.Context, pool *pgxpool.Pool, externalTransactionID string, expected int) {
	t.Helper()

	var count int

	err := pool.QueryRow(
		ctx,
		`
			SELECT COUNT(*)
			FROM wager_transactions
			WHERE external_transaction_id = $1
		`,
		externalTransactionID,
	).Scan(&count)
	if err != nil {
		t.Fatalf("failed to query wager transactions: %v", err)
	}

	if count != expected {
		t.Fatalf("expected %d wager transactions, got %d", expected, count)
	}
}

// assertLedgerCount verifies how many ledger entries exist for the wallet.
func assertLedgerCount(t *testing.T, ctx context.Context, pool *pgxpool.Pool, walletID uuid.UUID, expected int) {
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
		t.Fatalf("failed to query wallet ledger: %v", err)
	}

	if count != expected {
		t.Fatalf("expected %d ledger entries, got %d", expected, count)
	}
}

// assertOutboxCount verifies how many outbox events exist for the correlation.
func assertOutboxCount(t *testing.T, ctx context.Context, pool *pgxpool.Pool, correlationID string, expected int) {
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
		t.Fatalf("failed to query outbox events: %v", err)
	}

	if count != expected {
		t.Fatalf("expected %d outbox events, got %d", expected, count)
	}
}

// assertOutboxExists verifies successful processing generated outbox events.
func assertOutboxExists(t *testing.T, ctx context.Context, pool *pgxpool.Pool, correlationID string) {
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
		t.Fatalf("failed to query outbox events: %v", err)
	}

	if count == 0 {
		t.Fatal("expected outbox events after successful retry")
	}
}

// assertInboxCount verifies how many inbox records exist for the message.
func assertInboxCount(t *testing.T, ctx context.Context, pool *pgxpool.Pool, messageID string, expected int) {
	t.Helper()

	var count int

	err := pool.QueryRow(
		ctx,
		`
			SELECT COUNT(*)
			FROM inbox_messages
			WHERE consumer_name = 'wager-transactions'
			  AND message_id = $1
		`,
		messageID,
	).Scan(&count)
	if err != nil {
		t.Fatalf("failed to query inbox messages: %v", err)
	}

	if count != expected {
		t.Fatalf("expected %d inbox messages, got %d", expected, count)
	}
}

// assertCompletedInbox verifies the message was committed as completed.
func assertCompletedInbox(t *testing.T, ctx context.Context, pool *pgxpool.Pool, messageID string) {
	t.Helper()

	var completedAt *time.Time

	err := pool.QueryRow(
		ctx,
		`
			SELECT completed_at
			FROM inbox_messages
			WHERE consumer_name = 'wager-transactions'
			  AND message_id = $1
		`,
		messageID,
	).Scan(&completedAt)
	if err != nil {
		t.Fatalf("failed to query completed inbox message: %v", err)
	}

	if completedAt == nil {
		t.Fatal("expected inbox message to be completed")
	}
}
