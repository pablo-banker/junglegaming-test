package integration

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pablo-banker/junglegaming-test/internal/infrastructure/postgres"

	"github.com/pablo-banker/junglegaming-test/internal/application"
	"github.com/pablo-banker/junglegaming-test/internal/domain"
)

var outboxRepositoryTestTime = time.Date(
	2026,
	9,
	18,
	18,
	30,
	0,
	0,
	time.UTC,
)

// newWalletBalanceChangedEvent creates an outbox event for integration tests.
func newWalletBalanceChangedEvent(
	t *testing.T,
	wallet *domain.Wallet,
) application.EventEnvelope {
	t.Helper()

	zero, err := domain.Zero(wallet.Currency())
	if err != nil {
		t.Fatalf("failed to create zero money: %v", err)
	}

	return application.EventEnvelope{
		EventID:       uuid.New(),
		EventType:     application.EventTypeWalletBalanceChanged,
		AggregateID:   wallet.ID(),
		CorrelationID: "correlation-123",
		CausationID:   "wager-123",
		Version:       application.EventVersion,
		OccurredAt:    outboxRepositoryTestTime,
		Payload: application.WalletBalanceChangedPayload{
			WalletID:      wallet.ID(),
			PlayerID:      wallet.PlayerID(),
			TransactionID: uuid.New(),
			Direction:     domain.WalletLedgerDirectionCredit,
			Amount:        wallet.Balance(),
			BalanceBefore: zero,
			BalanceAfter:  wallet.Balance(),
			WalletVersion: wallet.Version(),
		},
	}
}

// cleanupOutboxTest removes data created by an outbox integration test.
func cleanupOutboxTest(t *testing.T, pool *pgxpool.Pool, eventID uuid.UUID, walletID uuid.UUID) {
	t.Helper()

	t.Cleanup(func() {
		ctx := context.Background()

		_, _ = pool.Exec(
			ctx,
			`DELETE FROM outbox_events WHERE event_id = $1`,
			eventID,
		)

		_, _ = pool.Exec(
			ctx,
			`DELETE FROM wallets WHERE id = $1`,
			walletID,
		)
	})
}

// TestOutboxRepositoryCommitsWithFinancialOperation verifies wallet and event commit together.
func TestOutboxRepositoryCommitsWithFinancialOperation(t *testing.T) {
	ctx := integrationContext(t)
	pool := openIntegrationPool(t)

	txManager := postgres.NewTransactionManager(pool)
	walletRepository := postgres.NewWalletRepository(pool)
	outboxRepository := postgres.NewOutboxRepository()

	wallet := newWalletRepositoryWallet(
		t,
		uuid.New(),
		"100.00",
	)

	event := newWalletBalanceChangedEvent(
		t,
		wallet,
	)

	cleanupOutboxTest(
		t,
		pool,
		event.EventID,
		wallet.ID(),
	)

	err := txManager.WithinTransaction(
		ctx,
		func(txCtx context.Context) error {
			if err := walletRepository.Create(
				txCtx,
				wallet,
			); err != nil {
				return err
			}

			return outboxRepository.Create(
				txCtx,
				event,
			)
		},
	)
	if err != nil {
		t.Fatalf("unexpected transaction error: %v", err)
	}

	var walletExists bool

	err = pool.QueryRow(
		ctx,
		`
			SELECT EXISTS (
				SELECT 1
				FROM wallets
				WHERE id = $1
			)
		`,
		wallet.ID(),
	).Scan(&walletExists)
	if err != nil {
		t.Fatalf("failed to check wallet: %v", err)
	}

	if !walletExists {
		t.Fatal("expected wallet to be committed")
	}

	var eventExists bool

	err = pool.QueryRow(
		ctx,
		`
			SELECT EXISTS (
				SELECT 1
				FROM outbox_events
				WHERE event_id = $1
			)
		`,
		event.EventID,
	).Scan(&eventExists)
	if err != nil {
		t.Fatalf("failed to check outbox event: %v", err)
	}

	if !eventExists {
		t.Fatal("expected outbox event to be committed")
	}
}

// TestOutboxRepositoryRollsBackWithFinancialOperation verifies wallet and event rollback together.
func TestOutboxRepositoryRollsBackWithFinancialOperation(t *testing.T) {
	ctx := integrationContext(t)
	pool := openIntegrationPool(t)

	txManager := postgres.NewTransactionManager(pool)
	walletRepository := postgres.NewWalletRepository(pool)
	outboxRepository := postgres.NewOutboxRepository()

	wallet := newWalletRepositoryWallet(
		t,
		uuid.New(),
		"100.00",
	)

	event := newWalletBalanceChangedEvent(
		t,
		wallet,
	)

	expectedErr := errors.New("force rollback")

	err := txManager.WithinTransaction(
		ctx,
		func(txCtx context.Context) error {
			if err := walletRepository.Create(
				txCtx,
				wallet,
			); err != nil {
				return err
			}

			if err := outboxRepository.Create(
				txCtx,
				event,
			); err != nil {
				return err
			}

			return expectedErr
		},
	)

	if !errors.Is(err, expectedErr) {
		t.Fatalf(
			"expected %v, got %v",
			expectedErr,
			err,
		)
	}

	var walletExists bool

	err = pool.QueryRow(
		ctx,
		`
			SELECT EXISTS (
				SELECT 1
				FROM wallets
				WHERE id = $1
			)
		`,
		wallet.ID(),
	).Scan(&walletExists)
	if err != nil {
		t.Fatalf("failed to check wallet: %v", err)
	}

	if walletExists {
		t.Fatal("expected wallet to be rolled back")
	}

	var eventExists bool

	err = pool.QueryRow(
		ctx,
		`
			SELECT EXISTS (
				SELECT 1
				FROM outbox_events
				WHERE event_id = $1
			)
		`,
		event.EventID,
	).Scan(&eventExists)
	if err != nil {
		t.Fatalf("failed to check outbox event: %v", err)
	}

	if eventExists {
		t.Fatal("expected outbox event to be rolled back")
	}
}

// TestOutboxRepositoryPersistsPayload verifies event payload JSON serialization.
func TestOutboxRepositoryPersistsPayload(t *testing.T) {
	ctx := integrationContext(t)
	pool := openIntegrationPool(t)

	txManager := postgres.NewTransactionManager(pool)
	walletRepository := postgres.NewWalletRepository(pool)
	outboxRepository := postgres.NewOutboxRepository()

	wallet := newWalletRepositoryWallet(
		t,
		uuid.New(),
		"100.00",
	)

	event := newWalletBalanceChangedEvent(
		t,
		wallet,
	)

	cleanupOutboxTest(
		t,
		pool,
		event.EventID,
		wallet.ID(),
	)

	err := txManager.WithinTransaction(
		ctx,
		func(txCtx context.Context) error {
			if err := walletRepository.Create(
				txCtx,
				wallet,
			); err != nil {
				return err
			}

			return outboxRepository.Create(
				txCtx,
				event,
			)
		},
	)
	if err != nil {
		t.Fatalf("unexpected transaction error: %v", err)
	}

	var (
		eventType        string
		aggregateID      uuid.UUID
		correlationID    string
		version          int64
		walletID         string
		amount           string
		currency         string
		balanceBefore    string
		balanceAfter     string
		publishedAtIsNil bool
		attempts         int
	)

	err = pool.QueryRow(
		ctx,
		`
			SELECT
				event_type,
				aggregate_id,
				correlation_id,
				version,
				payload ->> 'walletId',
				payload -> 'amount' ->> 'amount',
				payload -> 'amount' ->> 'currency',
				payload -> 'balanceBefore' ->> 'amount',
				payload -> 'balanceAfter' ->> 'amount',
				published_at IS NULL,
				attempts
			FROM outbox_events
			WHERE event_id = $1
		`,
		event.EventID,
	).Scan(
		&eventType,
		&aggregateID,
		&correlationID,
		&version,
		&walletID,
		&amount,
		&currency,
		&balanceBefore,
		&balanceAfter,
		&publishedAtIsNil,
		&attempts,
	)
	if err != nil {
		t.Fatalf("failed to read outbox event: %v", err)
	}

	if eventType != string(application.EventTypeWalletBalanceChanged) {
		t.Errorf(
			"expected WalletBalanceChanged, got %s",
			eventType,
		)
	}

	if aggregateID != wallet.ID() {
		t.Errorf(
			"expected aggregate %s, got %s",
			wallet.ID(),
			aggregateID,
		)
	}

	if correlationID != "correlation-123" {
		t.Errorf(
			"expected correlation-123, got %s",
			correlationID,
		)
	}

	if version != application.EventVersion {
		t.Errorf(
			"expected version %d, got %d",
			application.EventVersion,
			version,
		)
	}

	if walletID != wallet.ID().String() {
		t.Errorf(
			"expected wallet id %s, got %s",
			wallet.ID(),
			walletID,
		)
	}

	if amount != "100.00" {
		t.Errorf(
			"expected amount 100.00, got %s",
			amount,
		)
	}

	if currency != "BRL" {
		t.Errorf(
			"expected BRL, got %s",
			currency,
		)
	}

	if balanceBefore != "0.00" {
		t.Errorf(
			"expected balance before 0.00, got %s",
			balanceBefore,
		)
	}

	if balanceAfter != "100.00" {
		t.Errorf(
			"expected balance after 100.00, got %s",
			balanceAfter,
		)
	}

	if !publishedAtIsNil {
		t.Error("expected new event to be unpublished")
	}

	if attempts != 0 {
		t.Errorf(
			"expected 0 publishing attempts, got %d",
			attempts,
		)
	}
}
