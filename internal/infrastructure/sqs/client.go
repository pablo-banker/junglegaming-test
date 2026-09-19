package sqs

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	awssqs "github.com/aws/aws-sdk-go-v2/service/sqs"

	"github.com/pablo-banker/junglegaming-test/internal/config"
)

// NewClient creates the SQS client used by the application.
func NewClient(cfg config.Config) (*awssqs.Client, error) {
	awsConfig, err := awsconfig.LoadDefaultConfig(context.Background(), awsconfig.WithRegion(cfg.AWSRegion))
	if err != nil {
		return nil, err
	}

	client := awssqs.NewFromConfig(awsConfig, func(options *awssqs.Options) {
		if cfg.SQSEndpoint != "" {
			options.BaseEndpoint = aws.String(cfg.SQSEndpoint)
		}
	})

	return client, nil
}
