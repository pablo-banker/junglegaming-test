//go:build unit

package sqs

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	awssqs "github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"
)

type fakeConsumerClient struct {
	receiveOutput *awssqs.ReceiveMessageOutput
	receiveErr    error
	deleteErr     error

	receiveCalls int
	deleteCalls  int

	receiveInput   *awssqs.ReceiveMessageInput
	deletedReceipt []string
}

// ReceiveMessage returns the configured SQS batch.
func (f *fakeConsumerClient) ReceiveMessage(ctx context.Context, input *awssqs.ReceiveMessageInput, optFns ...func(*awssqs.Options)) (*awssqs.ReceiveMessageOutput, error) {
	f.receiveCalls++
	f.receiveInput = input

	return f.receiveOutput, f.receiveErr
}

// DeleteMessage records the acknowledged SQS message.
func (f *fakeConsumerClient) DeleteMessage(ctx context.Context, input *awssqs.DeleteMessageInput, optFns ...func(*awssqs.Options)) (*awssqs.DeleteMessageOutput, error) {
	f.deleteCalls++
	f.deletedReceipt = append(f.deletedReceipt, aws.ToString(input.ReceiptHandle))

	if f.deleteErr != nil {
		return nil, f.deleteErr
	}

	return &awssqs.DeleteMessageOutput{}, nil
}

type fakeWagerMessageHandler struct {
	calls int

	messageIDs []string
	bodies     []string

	err        error
	errorsByID map[string]error
}

// Handle records the wager message processing call.
func (f *fakeWagerMessageHandler) Handle(_ context.Context, messageID string, body string) error {
	f.calls++
	f.messageIDs = append(f.messageIDs, messageID)
	f.bodies = append(f.bodies, body)

	if err, ok := f.errorsByID[messageID]; ok {
		return err
	}

	return f.err
}

// newConsumerForTest creates a consumer with fake dependencies.
func newConsumerForTest() (*Consumer, *fakeConsumerClient, *fakeWagerMessageHandler) {
	client := &fakeConsumerClient{
		receiveOutput: &awssqs.ReceiveMessageOutput{},
	}

	handler := &fakeWagerMessageHandler{}

	consumer := &Consumer{
		client:   client,
		handler:  handler,
		queueURL: "http://localhost:4566/000000000000/wager-transactions.fifo",
	}

	return consumer, client, handler
}

// TestConsumerProcessesAndDeletesMessage verifies successful messages are acknowledged.
func TestConsumerProcessesAndDeletesMessage(t *testing.T) {
	consumer, client, handler := newConsumerForTest()

	client.receiveOutput.Messages = []types.Message{
		{
			MessageId:     aws.String("message-1"),
			Body:          aws.String(`{"type":"BET"}`),
			ReceiptHandle: aws.String("receipt-1"),
		},
	}

	err := consumer.PollOnce(context.Background())
	if err != nil {
		t.Fatalf("unexpected poll error: %v", err)
	}

	if handler.calls != 1 {
		t.Fatalf("expected 1 handler call, got %d", handler.calls)
	}

	if client.deleteCalls != 1 {
		t.Fatalf("expected 1 delete call, got %d", client.deleteCalls)
	}

	if handler.messageIDs[0] != "message-1" {
		t.Errorf("expected message-1, got %s", handler.messageIDs[0])
	}

	if client.deletedReceipt[0] != "receipt-1" {
		t.Errorf("expected receipt-1, got %s", client.deletedReceipt[0])
	}
}

// TestConsumerDoesNotDeleteFailedMessage verifies failed processing remains available for retry.
func TestConsumerDoesNotDeleteFailedMessage(t *testing.T) {
	consumer, client, handler := newConsumerForTest()

	handler.err = errors.New("processing failed")

	client.receiveOutput.Messages = []types.Message{
		{
			MessageId:     aws.String("message-1"),
			Body:          aws.String(`{"type":"BET"}`),
			ReceiptHandle: aws.String("receipt-1"),
		},
	}

	err := consumer.PollOnce(context.Background())
	if err != nil {
		t.Fatalf("unexpected poll error: %v", err)
	}

	if handler.calls != 1 {
		t.Fatalf("expected 1 handler call, got %d", handler.calls)
	}

	if client.deleteCalls != 0 {
		t.Fatalf("expected no delete calls, got %d", client.deleteCalls)
	}
}

// TestConsumerReturnsReceiveError verifies SQS receive failures abort the poll.
func TestConsumerReturnsReceiveError(t *testing.T) {
	consumer, client, handler := newConsumerForTest()

	expectedErr := errors.New("receive failed")
	client.receiveErr = expectedErr

	err := consumer.PollOnce(context.Background())

	if !errors.Is(err, expectedErr) {
		t.Fatalf("expected receive error, got %v", err)
	}

	if handler.calls != 0 {
		t.Fatalf("expected no handler calls, got %d", handler.calls)
	}

	if client.deleteCalls != 0 {
		t.Fatalf("expected no delete calls, got %d", client.deleteCalls)
	}
}

// TestConsumerUsesFIFOQueueConfiguration verifies the expected queue polling configuration.
func TestConsumerUsesFIFOQueueConfiguration(t *testing.T) {
	consumer, client, _ := newConsumerForTest()

	err := consumer.PollOnce(context.Background())
	if err != nil {
		t.Fatalf("unexpected poll error: %v", err)
	}

	if client.receiveInput == nil {
		t.Fatal("expected receive input")
	}

	if aws.ToString(client.receiveInput.QueueUrl) != consumer.queueURL {
		t.Errorf("expected queue URL %s, got %s", consumer.queueURL, aws.ToString(client.receiveInput.QueueUrl))
	}

	if client.receiveInput.MaxNumberOfMessages != 10 {
		t.Errorf("expected max 10 messages, got %d", client.receiveInput.MaxNumberOfMessages)
	}

	if client.receiveInput.WaitTimeSeconds != 10 {
		t.Errorf("expected wait time 10 seconds, got %d", client.receiveInput.WaitTimeSeconds)
	}
}

// TestConsumerRejectsInvalidMessages verifies malformed SQS metadata never reaches the handler.
func TestConsumerRejectsInvalidMessages(t *testing.T) {
	tests := []struct {
		name    string
		message types.Message
		wantErr error
	}{
		{
			name: "empty message id",
			message: types.Message{
				Body:          aws.String(`{"type":"BET"}`),
				ReceiptHandle: aws.String("receipt-1"),
			},
			wantErr: ErrInvalidSQSMessageID,
		},
		{
			name: "blank message id",
			message: types.Message{
				MessageId:     aws.String("   "),
				Body:          aws.String(`{"type":"BET"}`),
				ReceiptHandle: aws.String("receipt-1"),
			},
			wantErr: ErrInvalidSQSMessageID,
		},
		{
			name: "empty body",
			message: types.Message{
				MessageId:     aws.String("message-1"),
				ReceiptHandle: aws.String("receipt-1"),
			},
			wantErr: ErrInvalidSQSBody,
		},
		{
			name: "blank body",
			message: types.Message{
				MessageId:     aws.String("message-1"),
				Body:          aws.String("   "),
				ReceiptHandle: aws.String("receipt-1"),
			},
			wantErr: ErrInvalidSQSBody,
		},
		{
			name: "empty receipt handle",
			message: types.Message{
				MessageId: aws.String("message-1"),
				Body:      aws.String(`{"type":"BET"}`),
			},
			wantErr: ErrInvalidReceipt,
		},
		{
			name: "blank receipt handle",
			message: types.Message{
				MessageId:     aws.String("message-1"),
				Body:          aws.String(`{"type":"BET"}`),
				ReceiptHandle: aws.String("   "),
			},
			wantErr: ErrInvalidReceipt,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			consumer, client, handler := newConsumerForTest()

			err := consumer.processMessage(context.Background(), tt.message)

			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("expected %v, got %v", tt.wantErr, err)
			}

			if handler.calls != 0 {
				t.Fatalf("expected no handler calls, got %d", handler.calls)
			}

			if client.deleteCalls != 0 {
				t.Fatalf("expected no delete calls, got %d", client.deleteCalls)
			}
		})
	}
}

// TestConsumerContinuesBatchAfterMessageFailure verifies one poison message does not block the batch.
func TestConsumerContinuesBatchAfterMessageFailure(t *testing.T) {
	consumer, client, handler := newConsumerForTest()

	handler.errorsByID = map[string]error{
		"message-2": errors.New("processing failed"),
	}

	client.receiveOutput.Messages = []types.Message{
		{
			MessageId:     aws.String("message-1"),
			Body:          aws.String(`{"sequence":1}`),
			ReceiptHandle: aws.String("receipt-1"),
		},
		{
			MessageId:     aws.String("message-2"),
			Body:          aws.String(`{"sequence":2}`),
			ReceiptHandle: aws.String("receipt-2"),
		},
		{
			MessageId:     aws.String("message-3"),
			Body:          aws.String(`{"sequence":3}`),
			ReceiptHandle: aws.String("receipt-3"),
		},
	}

	err := consumer.PollOnce(context.Background())
	if err != nil {
		t.Fatalf("unexpected poll error: %v", err)
	}

	if handler.calls != 3 {
		t.Fatalf("expected 3 handler calls, got %d", handler.calls)
	}

	if client.deleteCalls != 2 {
		t.Fatalf("expected 2 delete calls, got %d", client.deleteCalls)
	}

	if client.deletedReceipt[0] != "receipt-1" {
		t.Errorf("expected first deleted receipt receipt-1, got %s", client.deletedReceipt[0])
	}

	if client.deletedReceipt[1] != "receipt-3" {
		t.Errorf("expected second deleted receipt receipt-3, got %s", client.deletedReceipt[1])
	}
}

// TestConsumerReturnsDeleteError verifies acknowledgment failures are propagated by message processing.
func TestConsumerReturnsDeleteError(t *testing.T) {
	consumer, client, handler := newConsumerForTest()

	expectedErr := errors.New("delete failed")
	client.deleteErr = expectedErr

	message := types.Message{
		MessageId:     aws.String("message-1"),
		Body:          aws.String(`{"type":"BET"}`),
		ReceiptHandle: aws.String("receipt-1"),
	}

	err := consumer.processMessage(context.Background(), message)

	if !errors.Is(err, expectedErr) {
		t.Fatalf("expected delete error, got %v", err)
	}

	if handler.calls != 1 {
		t.Fatalf("expected 1 handler call, got %d", handler.calls)
	}

	if client.deleteCalls != 1 {
		t.Fatalf("expected 1 delete call, got %d", client.deleteCalls)
	}
}
