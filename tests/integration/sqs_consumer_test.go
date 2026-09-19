//go:build integration

package integration

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awssqs "github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pablo-banker/junglegaming-test/internal/application"
	"github.com/pablo-banker/junglegaming-test/internal/domain"
	"github.com/pablo-banker/junglegaming-test/internal/infrastructure/postgres"

	"github.com/pablo-banker/junglegaming-test/internal/config"
	infrasqs "github.com/pablo-banker/junglegaming-test/internal/infrastructure/sqs"
)

// createConsumerTestQueue creates an isolated FIFO queue for one integration test.
func createConsumerTestQueue(t *testing.T, client *awssqs.Client) string {
	t.Helper()

	ctx := integrationContext(t)

	name := "consumer-" + strings.ReplaceAll(uuid.NewString(), "-", "") + ".fifo"

	result, err := client.CreateQueue(ctx, &awssqs.CreateQueueInput{
		QueueName: aws.String(name),
		Attributes: map[string]string{
			"FifoQueue":         "true",
			"VisibilityTimeout": "1",
		},
	})
	if err != nil {
		t.Fatalf("failed to create consumer test queue: %v", err)
	}

	queueURL := aws.ToString(result.QueueUrl)

	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		_, _ = client.DeleteQueue(cleanupCtx, &awssqs.DeleteQueueInput{
			QueueUrl: aws.String(queueURL),
		})
	})

	return queueURL
}

// newConsumerSQSClient creates the real MiniStack SQS client.
func newConsumerSQSClient(t *testing.T) (*awssqs.Client, config.Config) {
	t.Helper()

	t.Setenv("AWS_ACCESS_KEY_ID", "test")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "test")

	cfg := config.Config{
		AWSRegion:   "us-east-1",
		SQSEndpoint: "http://localhost:4566",
	}

	client, err := infrasqs.NewClient(cfg)
	if err != nil {
		t.Fatalf("failed to create SQS client: %v", err)
	}

	return client, cfg
}

// TestSQSConsumerProcessesAndDeletesMessage verifies successful SQS processing is committed and acknowledged.
func TestSQSConsumerProcessesAndDeletesMessage(t *testing.T) {
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
		application.DefaultReferenceRetryPolicy(),
	)

	handler := infrasqs.NewWagerMessageHandler(
		txManager,
		inboxRepository,
		wagerService,
		clock, sqsHandlerConfig(), discardLogger(), nil)

	consumer := infrasqs.NewConsumer(client, handler, cfg, discardLogger(), nil)
	publisher := infrasqs.NewPublisher(client, cfg)

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

	wallet, err := domain.NewWallet(uuid.New(), uuid.New(), balance, now)
	if err != nil {
		t.Fatalf("failed to create wallet: %v", err)
	}

	if err := walletRepository.Create(ctx, wallet); err != nil {
		t.Fatalf("failed to persist wallet: %v", err)
	}

	externalTransactionID := "transaction-" + uuid.NewString()
	idempotencyKey := "key-" + uuid.NewString()
	correlationID := "correlation-" + uuid.NewString()

	body := wagerRequestedMessageBody(t, wagerRequestedMessage{
		MessageID:             correlationID,
		ProviderID:            "provider-a",
		ExternalTransactionID: externalTransactionID,
		IdempotencyKey:        idempotencyKey,
		WalletID:              wallet.ID().String(),
		PlayerID:              wallet.PlayerID().String(),
		Kind:                  "BET",
		Amount:                "25.00",
	})

	err = publisher.Send(ctx, body, wallet.ID().String(), uuid.NewString())
	if err != nil {
		t.Fatalf("failed to publish wager message: %v", err)
	}

	_, err = consumer.PollOnce(ctx, ctx)
	if err != nil {
		t.Fatalf("failed to poll wager message: %v", err)
	}

	assertWalletState(t, ctx, pool, wallet.ID(), "75.00", 2)
	assertWagerCount(t, ctx, pool, externalTransactionID, 1)
	assertLedgerCount(t, ctx, pool, wallet.ID(), 1)
	assertOutboxExists(t, ctx, pool, correlationID)

	var inboxCount int
	var completedAt *time.Time

	err = pool.QueryRow(
		ctx,
		`
			SELECT COUNT(*), MAX(completed_at)
			FROM inbox_messages
			WHERE consumer_name = 'wager-transactions'
		`,
	).Scan(&inboxCount, &completedAt)
	if err != nil {
		t.Fatalf("failed to query inbox: %v", err)
	}

	if inboxCount == 0 {
		t.Fatal("expected inbox message")
	}

	if completedAt == nil {
		t.Fatal("expected completed inbox message")
	}

	result, err := client.ReceiveMessage(ctx, &awssqs.ReceiveMessageInput{
		QueueUrl:        aws.String(cfg.SQSQueueURL),
		WaitTimeSeconds: 1,
	})
	if err != nil {
		t.Fatalf("failed to verify queue: %v", err)
	}

	if len(result.Messages) != 0 {
		t.Fatalf("expected consumed message to be deleted, got %d messages", len(result.Messages))
	}
}

// TestSQSConsumerDelaysTransientFailure verifies an unavailable database delays the message
// with backoff instead of consuming a delivery every visibility timeout or reaching the DLQ.
func TestSQSConsumerDelaysTransientFailure(t *testing.T) {
	ctx := integrationContext(t)

	client, cfg := newConsumerSQSClient(t)

	cfg.SQSQueueURL = createConsumerTestQueue(t, client)

	// Nothing listens on port 1, so every transaction fails as unavailable.
	unreachable, err := pgxpool.New(ctx, "postgres://jungle:jungle@127.0.0.1:1/jungle?sslmode=disable&connect_timeout=1")
	if err != nil {
		t.Fatalf("failed to create unreachable pool: %v", err)
	}

	t.Cleanup(unreachable.Close)

	txManager := postgres.NewTransactionManager(unreachable)
	clock := postgres.NewClock(unreachable)

	wagerService := application.NewWagerService(
		txManager,
		clock,
		postgres.NewWalletRepository(unreachable),
		postgres.NewWagerRepository(unreachable),
		postgres.NewWalletLedgerRepository(unreachable),
		postgres.NewOutboxRepository(),
		application.DefaultReferenceRetryPolicy(),
	)

	handler := infrasqs.NewWagerMessageHandler(
		txManager,
		postgres.NewInboxRepository(unreachable),
		wagerService,
		clock, sqsHandlerConfig(), discardLogger(), nil)

	consumer := infrasqs.NewConsumer(client, handler, cfg, discardLogger(), nil)
	publisher := infrasqs.NewPublisher(client, cfg)

	body := wagerRequestedMessageBody(t, wagerRequestedMessage{
		MessageID:             "message-" + uuid.NewString(),
		ProviderID:            "provider-a",
		ExternalTransactionID: "transaction-" + uuid.NewString(),
		IdempotencyKey:        "key-" + uuid.NewString(),
		WalletID:              uuid.NewString(),
		PlayerID:              uuid.NewString(),
		Kind:                  "BET",
		Amount:                "25.00",
	})

	if err := publisher.Send(ctx, body, "wallet-test", uuid.NewString()); err != nil {
		t.Fatalf("failed to publish message: %v", err)
	}

	if _, err := consumer.PollOnce(ctx, ctx); !application.IsTransient(err) {
		t.Fatalf("expected transient poll error, got %v", err)
	}

	// The queue visibility timeout is 1 second, the first backoff is 5 seconds.
	time.Sleep(1500 * time.Millisecond)

	result, err := client.ReceiveMessage(ctx, &awssqs.ReceiveMessageInput{
		QueueUrl:        aws.String(cfg.SQSQueueURL),
		WaitTimeSeconds: 1,
	})
	if err != nil {
		t.Fatalf("failed to receive: %v", err)
	}

	if len(result.Messages) != 0 {
		t.Fatalf("expected the message to stay invisible during backoff, got %d messages", len(result.Messages))
	}
}
