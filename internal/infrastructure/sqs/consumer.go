package sqs

import (
	"context"
	"errors"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awssqs "github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"

	"github.com/pablo-banker/junglegaming-test/internal/application"
	"github.com/pablo-banker/junglegaming-test/internal/config"
	"github.com/pablo-banker/junglegaming-test/internal/observability"
)

const (
	receiveBatchSize   = 10
	receiveWaitSeconds = 20

	// Visibility backoff for transient failures: 5s, 10s, 20s ... capped at 5 minutes.
	retryBaseDelay = 5 * time.Second
	retryMaxDelay  = 5 * time.Minute

	// releaseTimeout bounds the SQS calls made after the work context was cancelled.
	releaseTimeout = 5 * time.Second

	failureReasonAttribute = "failureReason"
	maxFailureReasonLength = 256
)

type consumerClient interface {
	ReceiveMessage(ctx context.Context, params *awssqs.ReceiveMessageInput, optFns ...func(*awssqs.Options)) (*awssqs.ReceiveMessageOutput, error)
	DeleteMessage(ctx context.Context, params *awssqs.DeleteMessageInput, optFns ...func(*awssqs.Options)) (*awssqs.DeleteMessageOutput, error)
	ChangeMessageVisibility(ctx context.Context, params *awssqs.ChangeMessageVisibilityInput, optFns ...func(*awssqs.Options)) (*awssqs.ChangeMessageVisibilityOutput, error)
	SendMessage(ctx context.Context, params *awssqs.SendMessageInput, optFns ...func(*awssqs.Options)) (*awssqs.SendMessageOutput, error)
}

type wagerMessageHandler interface {
	Handle(ctx context.Context, messageID string, body string) error
}

// Consumer receives wager messages and deletes, delays or dead-letters each one by its outcome.
type Consumer struct {
	client   consumerClient
	handler  wagerMessageHandler
	queueURL string
	dlqURL   string
	logger   *slog.Logger
	metrics  *observability.Metrics
}

// NewConsumer creates the SQS wager consumer.
func NewConsumer(
	client *awssqs.Client,
	handler *WagerMessageHandler,
	cfg config.Config,
	logger *slog.Logger,
	metrics *observability.Metrics,
) *Consumer {
	return &Consumer{
		client:   client,
		handler:  handler,
		queueURL: cfg.SQSQueueURL,
		dlqURL:   cfg.SQSDLQURL,
		logger:   logger,
		metrics:  metrics,
	}
}

// PollOnce receives one batch and processes it, handing messages back when stop is cancelled.
func (c *Consumer) PollOnce(stop context.Context, work context.Context) (bool, error) {
	result, err := c.client.ReceiveMessage(stop, &awssqs.ReceiveMessageInput{
		QueueUrl:            aws.String(c.queueURL),
		MaxNumberOfMessages: receiveBatchSize,
		WaitTimeSeconds:     receiveWaitSeconds,
		MessageSystemAttributeNames: []types.MessageSystemAttributeName{
			types.MessageSystemAttributeNameApproximateReceiveCount,
			types.MessageSystemAttributeNameMessageGroupId,
		},
	})
	if err != nil {
		if stop.Err() != nil {
			return false, nil
		}

		return false, err
	}

	for i, message := range result.Messages {
		if stop.Err() != nil {
			c.release(work, result.Messages[i:], func(types.Message) time.Duration { return 0 })

			for range result.Messages[i:] {
				c.metrics.CountSQSMessage("released")
			}

			return true, nil
		}

		if err := c.processMessage(work, message); err != nil {
			// A transient failure usually affects the whole batch, so the rest waits too.
			c.release(work, result.Messages[i+1:], retryDelay)

			return true, err
		}
	}

	return len(result.Messages) > 0, nil
}

// processMessage handles one message and returns an error only for transient failures.
func (c *Consumer) processMessage(ctx context.Context, message types.Message) error {
	logger := c.logger.With(
		slog.String("sqsMessageId", aws.ToString(message.MessageId)),
		slog.Int("receiveCount", receiveCount(message)),
	)

	err := c.handler.Handle(ctx, aws.ToString(message.MessageId), aws.ToString(message.Body))

	switch {
	case err == nil:
		c.metrics.CountSQSMessage("processed")

		if _, err := c.client.DeleteMessage(ctx, &awssqs.DeleteMessageInput{
			QueueUrl:      aws.String(c.queueURL),
			ReceiptHandle: message.ReceiptHandle,
		}); err != nil {
			// The outcome is committed; a redelivery is answered by the inbox.
			logger.WarnContext(ctx, "failed to delete processed message", slog.Any("error", err))

			return err
		}

		return nil

	case isTransient(err):
		delay := retryDelay(message)

		c.metrics.CountSQSMessage("retried")
		logger.WarnContext(ctx, "message processing failed temporarily", slog.Any("error", err), slog.Duration("retryIn", delay))
		c.release(ctx, []types.Message{message}, func(types.Message) time.Duration { return delay })

		return err

	default:
		c.metrics.CountSQSMessage("dead_lettered")
		logger.ErrorContext(ctx, "message rejected permanently, moving to DLQ", slog.Any("error", err))

		return c.moveToDLQ(ctx, message, err)
	}
}

// moveToDLQ copies a permanently failed message to the DLQ and removes it from the queue.
func (c *Consumer) moveToDLQ(ctx context.Context, message types.Message, cause error) error {
	ctx, cancel := detached(ctx)
	defer cancel()

	groupID := message.Attributes[string(types.MessageSystemAttributeNameMessageGroupId)]
	if groupID == "" {
		groupID = aws.ToString(message.MessageId)
	}

	_, err := c.client.SendMessage(ctx, &awssqs.SendMessageInput{
		QueueUrl:               aws.String(c.dlqURL),
		MessageBody:            message.Body,
		MessageGroupId:         aws.String(groupID),
		MessageDeduplicationId: message.MessageId,
		MessageAttributes: map[string]types.MessageAttributeValue{
			failureReasonAttribute: {
				DataType:    aws.String("String"),
				StringValue: aws.String(truncate(cause.Error(), maxFailureReasonLength)),
			},
		},
	})
	if err != nil {
		// The message stays in the queue; the redrive policy still delivers it to the DLQ.
		return err
	}

	_, err = c.client.DeleteMessage(ctx, &awssqs.DeleteMessageInput{
		QueueUrl:      aws.String(c.queueURL),
		ReceiptHandle: message.ReceiptHandle,
	})

	return err
}

// release makes messages visible again after the delay chosen for each of them.
func (c *Consumer) release(ctx context.Context, messages []types.Message, delay func(types.Message) time.Duration) {
	if len(messages) == 0 {
		return
	}

	ctx, cancel := detached(ctx)
	defer cancel()

	for _, message := range messages {
		_, err := c.client.ChangeMessageVisibility(ctx, &awssqs.ChangeMessageVisibilityInput{
			QueueUrl:          aws.String(c.queueURL),
			ReceiptHandle:     message.ReceiptHandle,
			VisibilityTimeout: int32(delay(message).Seconds()),
		})
		if err != nil {
			// The queue visibility timeout still returns the message later.
			c.logger.WarnContext(ctx, "failed to change message visibility",
				slog.String("sqsMessageId", aws.ToString(message.MessageId)),
				slog.Any("error", err),
			)
		}
	}
}

// retryDelay returns the exponential visibility delay for the message's next attempt.
func retryDelay(message types.Message) time.Duration {
	delay := retryBaseDelay

	for i := 1; i < receiveCount(message) && delay < retryMaxDelay; i++ {
		delay *= 2
	}

	return min(delay, retryMaxDelay)
}

// receiveCount returns how many times SQS delivered the message, starting at 1.
func receiveCount(message types.Message) int {
	count, err := strconv.Atoi(message.Attributes[string(types.MessageSystemAttributeNameApproximateReceiveCount)])
	if err != nil || count < 1 {
		return 1
	}

	return count
}

// isTransient reports failures that may succeed later without changing the message.
func isTransient(err error) bool {
	return application.IsTransient(err) || errors.Is(err, context.Canceled)
}

// detached returns a context that survives the cancellation of ctx for a short time.
func detached(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(ctx), releaseTimeout)
}

// truncate limits a string to at most n bytes without splitting a UTF-8 character.
func truncate(value string, n int) string {
	if len(value) <= n {
		return value
	}

	return strings.ToValidUTF8(value[:n], "")
}
