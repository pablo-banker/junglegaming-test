package sqs

import (
	"context"
	"strconv"

	"github.com/aws/aws-sdk-go-v2/aws"
	awssqs "github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"

	"github.com/pablo-banker/junglegaming-test/internal/config"
)

type HealthChecker struct {
	client        *awssqs.Client
	queueURL      string
	eventQueueURL string
	dlqURL        string
}

// NewHealthChecker creates the SQS health checker.
func NewHealthChecker(client *awssqs.Client, cfg config.Config) *HealthChecker {
	return &HealthChecker{
		client:        client,
		queueURL:      cfg.SQSQueueURL,
		eventQueueURL: cfg.SQSEventQueueURL,
		dlqURL:        cfg.SQSDLQURL,
	}
}

// Check verifies that the inbound and outbound queues are reachable.
func (h *HealthChecker) Check(ctx context.Context) error {
	for _, queueURL := range []string{h.queueURL, h.eventQueueURL} {
		if _, err := h.client.GetQueueAttributes(ctx, &awssqs.GetQueueAttributesInput{
			QueueUrl:       aws.String(queueURL),
			AttributeNames: []types.QueueAttributeName{types.QueueAttributeNameQueueArn},
		}); err != nil {
			return err
		}
	}

	return nil
}

// DLQDepth returns the approximate number of messages waiting in the wager DLQ.
func (h *HealthChecker) DLQDepth(ctx context.Context) (int64, error) {
	output, err := h.client.GetQueueAttributes(ctx, &awssqs.GetQueueAttributesInput{
		QueueUrl:       aws.String(h.dlqURL),
		AttributeNames: []types.QueueAttributeName{types.QueueAttributeNameApproximateNumberOfMessages},
	})
	if err != nil {
		return 0, err
	}

	return strconv.ParseInt(output.Attributes[string(types.QueueAttributeNameApproximateNumberOfMessages)], 10, 64)
}
