package sqs

import (
	"context"
	"strings"

	awssqs "github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/pablo-banker/junglegaming-test/internal/config"
)

type Publisher struct {
	client   *awssqs.Client
	queueURL string
}

// NewPublisher creates the SQS wager publisher.
func NewPublisher(client *awssqs.Client, cfg config.Config) *Publisher {
	return &Publisher{
		client:   client,
		queueURL: cfg.SQSQueueURL,
	}
}

// Send publishes a message to the wager FIFO queue.
func (p *Publisher) Send(ctx context.Context, body string, groupID string, deduplicationID string) error {
	if strings.TrimSpace(body) == "" {
		return ErrInvalidMessageBody
	}

	if strings.TrimSpace(groupID) == "" {
		return ErrInvalidGroupID
	}

	if strings.TrimSpace(deduplicationID) == "" {
		return ErrInvalidDedupID
	}

	_, err := p.client.SendMessage(ctx, &awssqs.SendMessageInput{
		QueueUrl:               &p.queueURL,
		MessageBody:            &body,
		MessageGroupId:         &groupID,
		MessageDeduplicationId: &deduplicationID,
	})

	return err
}
