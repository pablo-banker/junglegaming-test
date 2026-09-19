package worker

import (
	"context"
	"log/slog"
	"time"

	"go.uber.org/fx"

	"github.com/pablo-banker/junglegaming-test/internal/application"
	"github.com/pablo-banker/junglegaming-test/internal/observability"
)

// Module registers the database driven background workers.
var Module = fx.Module(
	"worker",
	fx.Invoke(
		registerOutboxPublisher,
		registerPendingReferenceRetrier,
	),
)

// pollingOptions is used by workers that poll PostgreSQL for due work.
var pollingOptions = Options{
	IdleDelay:     time.Second,
	MinErrorDelay: time.Second,
	MaxErrorDelay: 30 * time.Second,
}

// registerOutboxPublisher publishes committed outbox events.
func registerOutboxPublisher(
	lifecycle fx.Lifecycle,
	dispatcher *application.OutboxDispatcherService,
	metrics *observability.Metrics,
	logger *slog.Logger,
) {
	Register(lifecycle, NewLoop(
		"outbox-publisher",
		func(_ context.Context, work context.Context) (bool, error) {
			found, err := dispatcher.DispatchNext(work)

			switch {
			case err != nil:
				metrics.CountOutboxPublication("failed")
			case found:
				metrics.CountOutboxPublication("published")
			}

			return found, err
		},
		pollingOptions,
		logger,
	))
}

// registerPendingReferenceRetrier retries PENDING_REFERENCE transactions.
func registerPendingReferenceRetrier(
	lifecycle fx.Lifecycle,
	service *application.WagerService,
	metrics *observability.Metrics,
	logger *slog.Logger,
) {
	Register(lifecycle, NewLoop(
		"pending-reference-retrier",
		func(_ context.Context, work context.Context) (bool, error) {
			start := time.Now()

			retry, err := service.RetryNextPendingReference(work)
			if retry == nil {
				return false, err
			}

			if retry.Status.IsTerminal() {
				metrics.ObserveWager("pending_reference", string(retry.Kind), string(retry.Status), false, time.Since(start))

				logger.Info(
					"pending reference transaction finished",
					slog.String("transactionId", retry.TransactionID.String()),
					slog.String("kind", string(retry.Kind)),
					slog.String("status", string(retry.Status)),
					slog.String("failureCode", retry.FailureCode),
				)
			}

			return true, err
		},
		pollingOptions,
		logger,
	))
}
