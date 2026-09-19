package sqs

import (
	"context"

	awssqs "github.com/aws/aws-sdk-go-v2/service/sqs"

	"github.com/pablo-banker/junglegaming-test/internal/config"
)

type HealthChecker struct {
	client   *awssqs.Client
	queueURL string
}

// NewHealthChecker creates the SQS health checker.
func NewHealthChecker(client *awssqs.Client, cfg config.Config) *HealthChecker {
	return &HealthChecker{
		client:   client,
		queueURL: cfg.SQSQueueURL,
	}
}

// Check verifies that the wager queue is reachable.
func (h *HealthChecker) Check(ctx context.Context) error {
	_, err := h.client.GetQueueAttributes(ctx, &awssqs.GetQueueAttributesInput{
		QueueUrl: &h.queueURL,
	})

	return err
}
