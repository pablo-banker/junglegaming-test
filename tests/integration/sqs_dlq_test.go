//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awssqs "github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"github.com/google/uuid"

	"github.com/pablo-banker/junglegaming-test/internal/application"
	"github.com/pablo-banker/junglegaming-test/internal/infrastructure/postgres"
	infrasqs "github.com/pablo-banker/junglegaming-test/internal/infrastructure/sqs"
)

const (
	dlqTestMaxReceiveCount = "3"
	dlqVisibilityTimeout   = 1100 * time.Millisecond
	dlqTestTimeout         = 20 * time.Second
)

// createDLQTestQueues creates isolated FIFO source and dead-letter queues.
func createDLQTestQueues(t *testing.T, client *awssqs.Client) (string, string) {
	t.Helper()

	ctx := integrationContext(t)

	suffix := strings.ReplaceAll(uuid.NewString(), "-", "")
	dlqName := "consumer-dlq-" + suffix + ".fifo"
	sourceName := "consumer-source-" + suffix + ".fifo"

	dlqResult, err := client.CreateQueue(ctx, &awssqs.CreateQueueInput{
		QueueName: aws.String(dlqName),
		Attributes: map[string]string{
			"FifoQueue": "true",
		},
	})
	if err != nil {
		t.Fatalf("failed to create DLQ: %v", err)
	}

	dlqURL := aws.ToString(dlqResult.QueueUrl)

	attributes, err := client.GetQueueAttributes(ctx, &awssqs.GetQueueAttributesInput{
		QueueUrl: aws.String(dlqURL),
		AttributeNames: []types.QueueAttributeName{
			types.QueueAttributeNameQueueArn,
		},
	})
	if err != nil {
		t.Fatalf("failed to get DLQ attributes: %v", err)
	}

	dlqARN := attributes.Attributes[string(types.QueueAttributeNameQueueArn)]
	if dlqARN == "" {
		t.Fatal("expected DLQ ARN")
	}

	redrivePolicy, err := json.Marshal(map[string]string{
		"deadLetterTargetArn": dlqARN,
		"maxReceiveCount":     dlqTestMaxReceiveCount,
	})
	if err != nil {
		t.Fatalf("failed to create redrive policy: %v", err)
	}

	sourceResult, err := client.CreateQueue(ctx, &awssqs.CreateQueueInput{
		QueueName: aws.String(sourceName),
		Attributes: map[string]string{
			"FifoQueue":         "true",
			"VisibilityTimeout": "1",
			"RedrivePolicy":     string(redrivePolicy),
		},
	})
	if err != nil {
		t.Fatalf("failed to create source queue: %v", err)
	}

	sourceURL := aws.ToString(sourceResult.QueueUrl)

	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(
			context.Background(),
			5*time.Second,
		)
		defer cancel()

		_, _ = client.DeleteQueue(cleanupCtx, &awssqs.DeleteQueueInput{
			QueueUrl: aws.String(sourceURL),
		})

		_, _ = client.DeleteQueue(cleanupCtx, &awssqs.DeleteQueueInput{
			QueueUrl: aws.String(dlqURL),
		})
	})

	return sourceURL, dlqURL
}

// triggerDLQRedrive forces another source receive after maxReceiveCount is reached.
func triggerDLQRedrive(
	t *testing.T,
	ctx context.Context,
	client *awssqs.Client,
	queueURL string,
) {
	t.Helper()

	for attempt := 1; attempt <= 3; attempt++ {
		result, err := client.ReceiveMessage(ctx, &awssqs.ReceiveMessageInput{
			QueueUrl:            aws.String(queueURL),
			MaxNumberOfMessages: 1,
			WaitTimeSeconds:     1,
		})
		if err != nil {
			t.Fatalf("failed to trigger DLQ redrive: %v", err)
		}

		if len(result.Messages) == 0 {
			return
		}

		time.Sleep(dlqVisibilityTimeout)
	}

	t.Fatal("source message remained available after redrive attempts")
}

// receiveDLQMessage waits until the failed message reaches the dead-letter queue.
func receiveDLQMessage(
	t *testing.T,
	ctx context.Context,
	client *awssqs.Client,
	queueURL string,
) types.Message {
	t.Helper()

	deadline := time.Now().Add(6 * time.Second)

	for time.Now().Before(deadline) {
		result, err := client.ReceiveMessage(ctx, &awssqs.ReceiveMessageInput{
			QueueUrl:            aws.String(queueURL),
			MaxNumberOfMessages: 1,
			WaitTimeSeconds:     1,
		})
		if err != nil {
			t.Fatalf("failed to receive DLQ message: %v", err)
		}

		if len(result.Messages) > 0 {
			return result.Messages[0]
		}
	}

	t.Fatal("expected message to reach DLQ")

	return types.Message{}
}

// TestSQSConsumerMovesPoisonMessageToDLQ verifies repeated processing failures trigger SQS redrive.
func TestSQSConsumerMovesPoisonMessageToDLQ(t *testing.T) {
	pool := openIntegrationPool(t)

	ctx, cancel := context.WithTimeout(
		context.Background(),
		dlqTestTimeout,
	)
	defer cancel()

	client, cfg := newConsumerSQSClient(t)

	sourceURL, dlqURL := createDLQTestQueues(t, client)
	cfg.SQSQueueURL = sourceURL

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

	consumer := infrasqs.NewConsumer(
		client,
		handler,
		cfg,
	)

	publisher := infrasqs.NewPublisher(
		client,
		cfg,
	)

	poisonBody := `{"correlationId":`

	err := publisher.Send(
		ctx,
		poisonBody,
		"wallet-"+uuid.NewString(),
		"message-"+uuid.NewString(),
	)
	if err != nil {
		t.Fatalf("failed to publish poison message: %v", err)
	}

	for attempt := 1; attempt <= 3; attempt++ {
		err = consumer.PollOnce(ctx)
		if err != nil {
			t.Fatalf(
				"unexpected poll error on attempt %d: %v",
				attempt,
				err,
			)
		}

		time.Sleep(dlqVisibilityTimeout)
	}

	// The message has now reached maxReceiveCount.
	// Another receive allows the queue to apply the redrive policy.
	triggerDLQRedrive(
		t,
		ctx,
		client,
		sourceURL,
	)

	message := receiveDLQMessage(
		t,
		ctx,
		client,
		dlqURL,
	)

	if aws.ToString(message.Body) != poisonBody {
		t.Errorf(
			"expected DLQ body %s, got %s",
			poisonBody,
			aws.ToString(message.Body),
		)
	}

	sourceResult, err := client.ReceiveMessage(ctx, &awssqs.ReceiveMessageInput{
		QueueUrl:            aws.String(sourceURL),
		MaxNumberOfMessages: 1,
		WaitTimeSeconds:     1,
	})
	if err != nil {
		t.Fatalf("failed to verify source queue: %v", err)
	}

	if len(sourceResult.Messages) != 0 {
		t.Fatalf(
			"expected source queue to be empty after redrive, got %d messages",
			len(sourceResult.Messages),
		)
	}
}
