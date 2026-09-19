package sqs

import (
	"log/slog"
	"time"

	"go.uber.org/fx"

	"github.com/pablo-banker/junglegaming-test/internal/application"
	"github.com/pablo-banker/junglegaming-test/internal/worker"
)

var Module = fx.Module(
	"sqs",
	fx.Provide(
		NewClient,
		NewHealthChecker,
		NewWagerMessageHandler,
		NewConsumer,
		fx.Annotate(
			NewOutboxPublisher,
			fx.As(new(application.IntegrationEventPublisher)),
		),
	),
	fx.Invoke(
		registerConsumer,
	),
)

// registerConsumer runs the wager queue consumer for the application lifetime.
func registerConsumer(lifecycle fx.Lifecycle, consumer *Consumer, logger *slog.Logger) {
	worker.Register(lifecycle, worker.NewLoop(
		"sqs-wager-consumer",
		consumer.PollOnce,
		worker.Options{
			MinErrorDelay: time.Second,
			MaxErrorDelay: 30 * time.Second,
		},
		logger,
	))
}
