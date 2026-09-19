//go:build integration

package integration

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awssqs "github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/google/uuid"
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
	)

	handler := infrasqs.NewWagerMessageHandler(
		txManager,
		inboxRepository,
		wagerService,
		clock,
	)

	consumer := infrasqs.NewConsumer(client, handler, cfg)
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

	err = publisher.Send(ctx, body, wallet.ID().String(), uuid.NewString())
	if err != nil {
		t.Fatalf("failed to publish wager message: %v", err)
	}

	err = consumer.PollOnce(ctx)
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

// TestSQSConsumerLeavesFailedMessageForRetry verifies failed messages become visible again.
func TestSQSConsumerLeavesFailedMessageForRetry(t *testing.T) {
	pool := openIntegrationPool(t)
	ctx := integrationContext(t)

	client, cfg := newConsumerSQSClient(t)

	cfg.SQSQueueURL = createConsumerTestQueue(t, client)

	txManager := postgres.NewTransactionManager(pool)
	clock := postgres.NewClock(pool)

	wagerService := application.NewWagerService(
		txManager,
		clock,
		postgres.NewWalletRepository(pool),
		postgres.NewWagerRepository(pool),
		postgres.NewWalletLedgerRepository(pool),
		postgres.NewOutboxRepository(),
	)

	handler := infrasqs.NewWagerMessageHandler(
		txManager,
		postgres.NewInboxRepository(pool),
		wagerService,
		clock,
	)

	consumer := infrasqs.NewConsumer(client, handler, cfg)
	publisher := infrasqs.NewPublisher(client, cfg)

	body := `{"correlationId":`

	err := publisher.Send(ctx, body, "wallet-test", uuid.NewString())
	if err != nil {
		t.Fatalf("failed to publish malformed message: %v", err)
	}

	err = consumer.PollOnce(ctx)
	if err != nil {
		t.Fatalf("unexpected poll error: %v", err)
	}

	// The queue visibility timeout is 1 second.
	time.Sleep(1100 * time.Millisecond)

	result, err := client.ReceiveMessage(ctx, &awssqs.ReceiveMessageInput{
		QueueUrl:        aws.String(cfg.SQSQueueURL),
		WaitTimeSeconds: 1,
	})
	if err != nil {
		t.Fatalf("failed to receive retried message: %v", err)
	}

	if len(result.Messages) != 1 {
		t.Fatalf("expected failed message to reappear, got %d messages", len(result.Messages))
	}

	if aws.ToString(result.Messages[0].Body) != body {
		t.Errorf("expected original message body, got %s", aws.ToString(result.Messages[0].Body))
	}

	_, err = client.DeleteMessage(ctx, &awssqs.DeleteMessageInput{
		QueueUrl:      aws.String(cfg.SQSQueueURL),
		ReceiptHandle: result.Messages[0].ReceiptHandle,
	})
	if err != nil {
		t.Fatalf("failed to clean up retried message: %v", err)
	}
}
