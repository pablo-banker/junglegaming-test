//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awssqs "github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/google/uuid"
	"github.com/pablo-banker/junglegaming-test/internal/config"
	infrasqs "github.com/pablo-banker/junglegaming-test/internal/infrastructure/sqs"
)

// newSQSTestDependencies creates the real SQS client and a publisher bound to an isolated queue.
func newSQSTestDependencies(t *testing.T) (*awssqs.Client, *infrasqs.Publisher, string) {
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

	cfg.SQSQueueURL = createConsumerTestQueue(t, client)

	return client, infrasqs.NewPublisher(client, cfg), cfg.SQSQueueURL
}

// receiveSQSMessages receives and deletes the expected number of messages.
func receiveSQSMessages(t *testing.T, client *awssqs.Client, queueURL string, expected int) []string {
	t.Helper()

	ctx := context.Background()
	deadline := time.Now().Add(5 * time.Second)

	var messages []string

	for len(messages) < expected && time.Now().Before(deadline) {
		result, err := client.ReceiveMessage(ctx, &awssqs.ReceiveMessageInput{
			QueueUrl:            aws.String(queueURL),
			MaxNumberOfMessages: 10,
			WaitTimeSeconds:     1,
		})
		if err != nil {
			t.Fatalf("failed to receive SQS messages: %v", err)
		}

		for _, message := range result.Messages {
			messages = append(messages, aws.ToString(message.Body))

			_, err := client.DeleteMessage(ctx, &awssqs.DeleteMessageInput{
				QueueUrl:      aws.String(queueURL),
				ReceiptHandle: message.ReceiptHandle,
			})
			if err != nil {
				t.Fatalf("failed to delete SQS message: %v", err)
			}
		}
	}

	return messages
}

// TestSQSPublisherSendsMessage verifies messages reach the real FIFO queue.
func TestSQSPublisherSendsMessage(t *testing.T) {
	client, publisher, queueURL := newSQSTestDependencies(t)

	body := `{"type":"BET","amount":"25.00"}`

	err := publisher.Send(context.Background(), body, "wallet-"+uuid.NewString(), "message-"+uuid.NewString())
	if err != nil {
		t.Fatalf("failed to publish message: %v", err)
	}

	messages := receiveSQSMessages(t, client, queueURL, 1)

	if len(messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(messages))
	}

	if messages[0] != body {
		t.Errorf("expected body %s, got %s", body, messages[0])
	}
}

// TestSQSPublisherDeduplicatesMessages verifies the FIFO deduplication id prevents duplicate delivery.
func TestSQSPublisherDeduplicatesMessages(t *testing.T) {
	client, publisher, queueURL := newSQSTestDependencies(t)

	body := `{"type":"BET","amount":"25.00"}`
	groupID := "wallet-" + uuid.NewString()
	dedupID := "message-" + uuid.NewString()

	err := publisher.Send(context.Background(), body, groupID, dedupID)
	if err != nil {
		t.Fatalf("failed to publish first message: %v", err)
	}

	err = publisher.Send(context.Background(), body, groupID, dedupID)
	if err != nil {
		t.Fatalf("failed to publish duplicate message: %v", err)
	}

	messages := receiveSQSMessages(t, client, queueURL, 1)

	if len(messages) != 1 {
		t.Fatalf("expected 1 delivered message, got %d", len(messages))
	}

	if messages[0] != body {
		t.Errorf("expected body %s, got %s", body, messages[0])
	}

	result, err := client.ReceiveMessage(context.Background(), &awssqs.ReceiveMessageInput{
		QueueUrl:            aws.String(queueURL),
		MaxNumberOfMessages: 10,
		WaitTimeSeconds:     1,
	})
	if err != nil {
		t.Fatalf("failed to check duplicate delivery: %v", err)
	}

	if len(result.Messages) != 0 {
		t.Fatalf("expected no duplicated message, got %d", len(result.Messages))
	}
}

// TestSQSPublisherPreservesGroupOrder verifies FIFO ordering inside the same message group.
func TestSQSPublisherPreservesGroupOrder(t *testing.T) {
	client, publisher, queueURL := newSQSTestDependencies(t)

	groupID := "wallet-" + uuid.NewString()

	expected := []string{
		`{"sequence":1}`,
		`{"sequence":2}`,
		`{"sequence":3}`,
	}

	for _, body := range expected {
		err := publisher.Send(context.Background(), body, groupID, "message-"+uuid.NewString())
		if err != nil {
			t.Fatalf("failed to publish message: %v", err)
		}
	}

	messages := receiveSQSMessages(t, client, queueURL, len(expected))

	if len(messages) != len(expected) {
		t.Fatalf("expected %d messages, got %d", len(expected), len(messages))
	}

	for i := range expected {
		if messages[i] != expected[i] {
			t.Errorf("expected message %d to be %s, got %s", i, expected[i], messages[i])
		}
	}
}
