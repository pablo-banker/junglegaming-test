//go:build integration

package integration

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/fx"

	"github.com/pablo-banker/junglegaming-test/internal/application"
	"github.com/pablo-banker/junglegaming-test/internal/domain"
	"github.com/pablo-banker/junglegaming-test/internal/infrastructure/postgres"
	infrasqs "github.com/pablo-banker/junglegaming-test/internal/infrastructure/sqs"
)

// waitForWalletState waits until the worker commits the expected wallet state.
func waitForWalletState(t *testing.T, ctx context.Context, pool *pgxpool.Pool, walletID uuid.UUID, expectedBalance string, expectedVersion int64) {
	t.Helper()

	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	for {
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

		if err == nil &&
			balance == expectedBalance &&
			version == expectedVersion {
			return
		}

		select {
		case <-ticker.C:
		case <-ctx.Done():
			t.Fatalf(
				"wallet did not reach balance %s version %d: %v",
				expectedBalance,
				expectedVersion,
				ctx.Err(),
			)
		}
	}
}

// TestSQSWorkerLifecycleStartsAndStopsWithFx verifies Fx controls the real SQS worker lifecycle.
func TestSQSWorkerLifecycleStartsAndStopsWithFx(t *testing.T) {
	pool := openIntegrationPool(t)
	ctx := integrationContext(t)

	client, cfg := newConsumerSQSClient(t)
	cfg.SQSQueueURL = createConsumerTestQueue(t, client)

	txManager := postgres.NewTransactionManager(pool)
	clock := postgres.NewClock(pool)

	walletRepository := postgres.NewWalletRepository(pool)
	wagerRepository := postgres.NewWagerRepository(pool)
	ledgerRepository := postgres.NewWalletLedgerRepository(pool)
	outboxRepository := postgres.NewOutboxRepository()
	inboxRepository := postgres.NewInboxRepository(pool)

	wagerService := application.NewWagerService(
		txManager,
		clock,
		walletRepository,
		wagerRepository,
		ledgerRepository,
		outboxRepository,
	)

	handler := infrasqs.NewWagerMessageHandler(
		txManager,
		inboxRepository,
		wagerService,
		clock,
	)

	consumer := infrasqs.NewConsumer(
		client,
		handler,
		cfg,
	)

	logger := slog.New(
		slog.NewTextHandler(
			io.Discard,
			nil,
		),
	)

	worker := infrasqs.NewWorker(
		consumer,
		logger,
	)

	app := fx.New(
		fx.Supply(worker),
		fx.Invoke(infrasqs.RegisterWorker),
	)

	if err := app.Err(); err != nil {
		t.Fatalf("failed to build Fx app: %v", err)
	}

	started := false
	stopped := false

	t.Cleanup(func() {
		if !started || stopped {
			return
		}

		stopCtx, cancel := context.WithTimeout(
			context.Background(),
			2*time.Second,
		)
		defer cancel()

		_ = app.Stop(stopCtx)
	})

	currency, err := domain.NewCurrency("BRL")
	if err != nil {
		t.Fatalf("failed to create currency: %v", err)
	}

	balance, err := domain.ParseMoney("100.00", currency)
	if err != nil {
		t.Fatalf("failed to create wallet balance: %v", err)
	}

	now, err := clock.Now(ctx)
	if err != nil {
		t.Fatalf("failed to get database time: %v", err)
	}

	wallet, err := domain.NewWallet(
		uuid.New(),
		uuid.New(),
		balance,
		now,
	)
	if err != nil {
		t.Fatalf("failed to create wallet: %v", err)
	}

	if err := walletRepository.Create(ctx, wallet); err != nil {
		t.Fatalf("failed to create wallet: %v", err)
	}

	externalTransactionID := "transaction-" + uuid.NewString()
	idempotencyKey := "idempotency-" + uuid.NewString()
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

	publisher := infrasqs.NewPublisher(
		client,
		cfg,
	)

	// Publish before Fx starts. Starting the app must activate the worker
	// and consume this pending message.
	err = publisher.Send(
		ctx,
		body,
		wallet.ID().String(),
		"message-"+uuid.NewString(),
	)
	if err != nil {
		t.Fatalf("failed to publish wager message: %v", err)
	}

	startCtx, cancelStart := context.WithTimeout(
		context.Background(),
		2*time.Second,
	)
	defer cancelStart()

	if err := app.Start(startCtx); err != nil {
		t.Fatalf("failed to start Fx app: %v", err)
	}

	started = true

	waitCtx, cancelWait := context.WithTimeout(
		context.Background(),
		5*time.Second,
	)
	defer cancelWait()

	waitForWalletState(
		t,
		waitCtx,
		pool,
		wallet.ID(),
		"75.00",
		2,
	)

	assertWalletState(
		t,
		ctx,
		pool,
		wallet.ID(),
		"75.00",
		2,
	)

	assertWagerCount(
		t,
		ctx,
		pool,
		externalTransactionID,
		1,
	)

	assertLedgerCount(
		t,
		ctx,
		pool,
		wallet.ID(),
		1,
	)

	assertOutboxExists(
		t,
		ctx,
		pool,
		correlationID,
	)

	stopCtx, cancelStop := context.WithTimeout(
		context.Background(),
		2*time.Second,
	)
	defer cancelStop()

	if err := app.Stop(stopCtx); err != nil {
		t.Fatalf("failed to stop Fx app: %v", err)
	}

	stopped = true
}
