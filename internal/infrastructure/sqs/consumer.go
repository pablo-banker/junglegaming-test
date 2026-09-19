package sqs

import (
	"context"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awssqs "github.com/aws/aws-sdk-go-v2/service/sqs"

	"github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"github.com/pablo-banker/junglegaming-test/internal/config"
)

type consumerClient interface {
	ReceiveMessage(ctx context.Context, params *awssqs.ReceiveMessageInput, optFns ...func(*awssqs.Options)) (*awssqs.ReceiveMessageOutput, error)
	DeleteMessage(ctx context.Context, params *awssqs.DeleteMessageInput, optFns ...func(*awssqs.Options)) (*awssqs.DeleteMessageOutput, error)
}

type wagerMessageHandler interface {
	Handle(ctx context.Context, messageID string, body string) error
}

type Consumer struct {
	client   consumerClient
	handler  wagerMessageHandler
	queueURL string
}

// NewConsumer creates the SQS wager consumer.
func NewConsumer(client *awssqs.Client, handler *WagerMessageHandler, cfg config.Config) *Consumer {
	return &Consumer{
		client:   client,
		handler:  handler,
		queueURL: cfg.SQSQueueURL,
	}
}

// PollOnce receives and processes one batch of SQS messages.
func (c *Consumer) PollOnce(ctx context.Context) error {
	result, err := c.client.ReceiveMessage(ctx, &awssqs.ReceiveMessageInput{
		QueueUrl:            aws.String(c.queueURL),
		MaxNumberOfMessages: 10,
		WaitTimeSeconds:     10,
	})
	if err != nil {
		return err
	}

	for _, message := range result.Messages {
		if err := c.processMessage(ctx, message); err != nil {
			continue
		}
	}

	return nil
}

// processMessage processes and acknowledges one SQS message.
func (c *Consumer) processMessage(ctx context.Context, message types.Message) error {
	messageID := strings.TrimSpace(aws.ToString(message.MessageId))
	if messageID == "" {
		return ErrInvalidSQSMessageID
	}

	body := aws.ToString(message.Body)
	if strings.TrimSpace(body) == "" {
		return ErrInvalidSQSBody
	}

	receiptHandle := strings.TrimSpace(aws.ToString(message.ReceiptHandle))
	if receiptHandle == "" {
		return ErrInvalidReceipt
	}

	if err := c.handler.Handle(ctx, messageID, body); err != nil {
		return err
	}

	_, err := c.client.DeleteMessage(ctx, &awssqs.DeleteMessageInput{
		QueueUrl:      aws.String(c.queueURL),
		ReceiptHandle: aws.String(receiptHandle),
	})

	return err
}
