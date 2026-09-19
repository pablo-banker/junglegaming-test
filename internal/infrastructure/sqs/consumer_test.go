package sqs

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awssqs "github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"

	"github.com/pablo-banker/junglegaming-test/internal/application"
)

type visibilityChange struct {
	receipt string
	timeout int32
}

type fakeConsumerClient struct {
	receiveOutput *awssqs.ReceiveMessageOutput
	receiveErr    error
	deleteErr     error
	sendErr       error

	receiveInput      *awssqs.ReceiveMessageInput
	deletedReceipts   []string
	visibilityChanges []visibilityChange
	sent              []*awssqs.SendMessageInput
}

// ReceiveMessage returns the configured SQS batch.
func (f *fakeConsumerClient) ReceiveMessage(_ context.Context, input *awssqs.ReceiveMessageInput, _ ...func(*awssqs.Options)) (*awssqs.ReceiveMessageOutput, error) {
	f.receiveInput = input

	return f.receiveOutput, f.receiveErr
}

// DeleteMessage records the acknowledged SQS message.
func (f *fakeConsumerClient) DeleteMessage(_ context.Context, input *awssqs.DeleteMessageInput, _ ...func(*awssqs.Options)) (*awssqs.DeleteMessageOutput, error) {
	if f.deleteErr != nil {
		return nil, f.deleteErr
	}

	f.deletedReceipts = append(f.deletedReceipts, aws.ToString(input.ReceiptHandle))

	return &awssqs.DeleteMessageOutput{}, nil
}

// ChangeMessageVisibility records the visibility change.
func (f *fakeConsumerClient) ChangeMessageVisibility(_ context.Context, input *awssqs.ChangeMessageVisibilityInput, _ ...func(*awssqs.Options)) (*awssqs.ChangeMessageVisibilityOutput, error) {
	f.visibilityChanges = append(f.visibilityChanges, visibilityChange{
		receipt: aws.ToString(input.ReceiptHandle),
		timeout: input.VisibilityTimeout,
	})

	return &awssqs.ChangeMessageVisibilityOutput{}, nil
}

// SendMessage records the message sent to the DLQ.
func (f *fakeConsumerClient) SendMessage(_ context.Context, input *awssqs.SendMessageInput, _ ...func(*awssqs.Options)) (*awssqs.SendMessageOutput, error) {
	if f.sendErr != nil {
		return nil, f.sendErr
	}

	f.sent = append(f.sent, input)

	return &awssqs.SendMessageOutput{}, nil
}

type fakeWagerMessageHandler struct {
	calls      int
	errorsByID map[string]error
}

// Handle returns the configured result for the message.
func (f *fakeWagerMessageHandler) Handle(_ context.Context, messageID string, _ string) error {
	f.calls++

	return f.errorsByID[messageID]
}

// newConsumerForTest creates a consumer with fake dependencies.
func newConsumerForTest(messages ...types.Message) (*Consumer, *fakeConsumerClient, *fakeWagerMessageHandler) {
	client := &fakeConsumerClient{
		receiveOutput: &awssqs.ReceiveMessageOutput{Messages: messages},
	}

	handler := &fakeWagerMessageHandler{errorsByID: map[string]error{}}

	consumer := &Consumer{
		client:   client,
		handler:  handler,
		queueURL: "http://localhost:4566/000000000000/wager-transactions.fifo",
		dlqURL:   "http://localhost:4566/000000000000/wager-transactions-dlq.fifo",
		logger:   slog.New(slog.DiscardHandler),
	}

	return consumer, client, handler
}

// testMessage creates an SQS message delivered receiveCount times.
func testMessage(id string, receiveCount int) types.Message {
	return types.Message{
		MessageId:     aws.String(id),
		Body:          aws.String(`{"messageId":"` + id + `"}`),
		ReceiptHandle: aws.String("receipt-" + id),
		Attributes: map[string]string{
			string(types.MessageSystemAttributeNameApproximateReceiveCount): fmt.Sprint(receiveCount),
			string(types.MessageSystemAttributeNameMessageGroupId):          "wallet-1",
		},
	}
}

// TestConsumerDeletesProcessedMessages verifies successful messages are acknowledged.
func TestConsumerDeletesProcessedMessages(t *testing.T) {
	consumer, client, _ := newConsumerForTest(testMessage("m1", 1), testMessage("m2", 1))

	found, err := consumer.PollOnce(context.Background(), context.Background())
	if err != nil || !found {
		t.Fatalf("expected processed batch, got %v %v", found, err)
	}

	if len(client.deletedReceipts) != 2 {
		t.Fatalf("expected 2 deleted messages, got %d", len(client.deletedReceipts))
	}
}

// TestConsumerRequestsFIFOAttributes verifies long polling and the attributes used for retries and DLQ moves.
func TestConsumerRequestsFIFOAttributes(t *testing.T) {
	consumer, client, _ := newConsumerForTest()

	if _, err := consumer.PollOnce(context.Background(), context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	input := client.receiveInput

	if input.WaitTimeSeconds != receiveWaitSeconds || input.MaxNumberOfMessages != receiveBatchSize {
		t.Fatalf("unexpected receive settings: wait %d batch %d", input.WaitTimeSeconds, input.MaxNumberOfMessages)
	}

	if len(input.MessageSystemAttributeNames) != 2 {
		t.Fatalf("expected receive count and group id attributes, got %v", input.MessageSystemAttributeNames)
	}
}

// TestConsumerDelaysTransientFailuresWithBackoff verifies transient failures are retried with backoff.
func TestConsumerDelaysTransientFailuresWithBackoff(t *testing.T) {
	consumer, client, handler := newConsumerForTest(testMessage("m1", 3), testMessage("m2", 1))

	handler.errorsByID["m1"] = fmt.Errorf("%w: connection refused", application.ErrUnavailable)

	_, err := consumer.PollOnce(context.Background(), context.Background())
	if !errors.Is(err, application.ErrUnavailable) {
		t.Fatalf("expected transient error, got %v", err)
	}

	if handler.calls != 1 {
		t.Fatalf("expected processing to stop after the transient failure, got %d calls", handler.calls)
	}

	if len(client.deletedReceipts) != 0 || len(client.sent) != 0 {
		t.Fatal("expected transient failures to stay in the queue")
	}

	expected := []visibilityChange{
		{receipt: "receipt-m1", timeout: int32((20 * time.Second).Seconds())},
		{receipt: "receipt-m2", timeout: int32((5 * time.Second).Seconds())},
	}

	if len(client.visibilityChanges) != len(expected) {
		t.Fatalf("expected %d visibility changes, got %v", len(expected), client.visibilityChanges)
	}

	for i, change := range expected {
		if client.visibilityChanges[i] != change {
			t.Errorf("expected %v, got %v", change, client.visibilityChanges[i])
		}
	}
}

// TestConsumerMovesPermanentFailuresToDLQ verifies permanent failures reach the DLQ at once.
func TestConsumerMovesPermanentFailuresToDLQ(t *testing.T) {
	consumer, client, handler := newConsumerForTest(testMessage("m1", 1), testMessage("m2", 1))

	handler.errorsByID["m1"] = application.ErrIdempotencyConflict

	if _, err := consumer.PollOnce(context.Background(), context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(client.sent) != 1 {
		t.Fatalf("expected 1 DLQ message, got %d", len(client.sent))
	}

	dlq := client.sent[0]

	if aws.ToString(dlq.QueueUrl) != consumer.dlqURL ||
		aws.ToString(dlq.MessageGroupId) != "wallet-1" ||
		aws.ToString(dlq.MessageDeduplicationId) != "m1" {
		t.Fatalf("unexpected DLQ message: %+v", dlq)
	}

	if reason := aws.ToString(dlq.MessageAttributes[failureReasonAttribute].StringValue); reason != application.ErrIdempotencyConflict.Error() {
		t.Errorf("expected failure reason, got %q", reason)
	}

	if len(client.deletedReceipts) != 2 {
		t.Fatalf("expected both messages removed from the source queue, got %v", client.deletedReceipts)
	}
}

// TestConsumerReleasesUnstartedMessagesOnShutdown verifies received messages are handed back on shutdown.
func TestConsumerReleasesUnstartedMessagesOnShutdown(t *testing.T) {
	consumer, client, handler := newConsumerForTest(testMessage("m1", 1), testMessage("m2", 1))

	stop, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := consumer.PollOnce(stop, context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if handler.calls != 0 {
		t.Fatalf("expected no processing after shutdown started, got %d", handler.calls)
	}

	if len(client.visibilityChanges) != 2 || client.visibilityChanges[0].timeout != 0 {
		t.Fatalf("expected messages released with visibility 0, got %v", client.visibilityChanges)
	}
}

// TestConsumerReturnsReceiveError verifies SQS receive failures reach the worker backoff.
func TestConsumerReturnsReceiveError(t *testing.T) {
	consumer, client, _ := newConsumerForTest()

	client.receiveErr = errors.New("sqs unavailable")

	if _, err := consumer.PollOnce(context.Background(), context.Background()); err == nil {
		t.Fatal("expected receive error")
	}
}

// TestRetryDelayGrowsExponentially verifies the visibility backoff and its cap.
func TestRetryDelayGrowsExponentially(t *testing.T) {
	tests := map[int]time.Duration{
		1:  5 * time.Second,
		2:  10 * time.Second,
		4:  40 * time.Second,
		20: retryMaxDelay,
	}

	for count, want := range tests {
		if got := retryDelay(testMessage("m", count)); got != want {
			t.Errorf("receive count %d: expected %s, got %s", count, want, got)
		}
	}
}
