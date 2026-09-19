package sqs

import (
	"context"
	"encoding/json"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awssqs "github.com/aws/aws-sdk-go-v2/service/sqs"

	"github.com/pablo-banker/junglegaming-test/internal/application"
	"github.com/pablo-banker/junglegaming-test/internal/config"
)

type OutboxPublisher struct {
	client   *awssqs.Client
	queueURL string
}

type outboxMessage struct {
	EventID       string          `json:"eventId"`
	EventType     string          `json:"eventType"`
	AggregateID   string          `json:"aggregateId"`
	CorrelationID string          `json:"correlationId"`
	CausationID   string          `json:"causationId,omitempty"`
	OccurredAt    string          `json:"occurredAt"`
	Version       int64           `json:"version"`
	Data          json.RawMessage `json:"data"`
}

// NewOutboxPublisher creates the SQS integration event publisher.
func NewOutboxPublisher(
	client *awssqs.Client,
	cfg config.Config,
) *OutboxPublisher {
	return &OutboxPublisher{
		client:   client,
		queueURL: cfg.SQSEventQueueURL,
	}
}

// Publish publishes one persisted outbox event.
func (p *OutboxPublisher) Publish(
	ctx context.Context,
	event *application.PendingOutboxEvent,
) error {
	body, err := buildOutboxMessage(event)
	if err != nil {
		return err
	}

	_, err = p.client.SendMessage(
		ctx,
		&awssqs.SendMessageInput{
			QueueUrl:               aws.String(p.queueURL),
			MessageBody:            aws.String(body),
			MessageGroupId:         aws.String(event.AggregateID.String()),
			MessageDeduplicationId: aws.String(event.EventID.String()),
		},
	)

	return err
}

func buildOutboxMessage(
	event *application.PendingOutboxEvent,
) (string, error) {
	message := outboxMessage{
		EventID:       event.EventID.String(),
		EventType:     event.EventType,
		AggregateID:   event.AggregateID.String(),
		CorrelationID: event.CorrelationID,
		CausationID:   event.CausationID,
		OccurredAt:    event.OccurredAt.UTC().Format(time.RFC3339Nano),
		Version:       event.Version,
		Data:          event.Payload,
	}

	body, err := json.Marshal(message)
	if err != nil {
		return "", err
	}

	return string(body), nil
}
