package sqs

import (
	"github.com/pablo-banker/junglegaming-test/internal/application"
	"go.uber.org/fx"
)

var Module = fx.Module(
	"sqs",
	fx.Provide(
		NewClient,
		NewHealthChecker,
		NewPublisher,
		NewWagerMessageHandler,
		NewConsumer,
		NewWorker,
		fx.Annotate(
			NewOutboxPublisher,
			fx.As(new(application.IntegrationEventPublisher)),
		),
	),
	fx.Invoke(
		RegisterWorker,
	),
)
