package wiring

import (
	"context"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/fx"
	"go.uber.org/fx/fxevent"

	"github.com/pablo-banker/junglegaming-test/internal/application"
	"github.com/pablo-banker/junglegaming-test/internal/config"
	"github.com/pablo-banker/junglegaming-test/internal/infrastructure/keycloak"
	"github.com/pablo-banker/junglegaming-test/internal/infrastructure/postgres"
	"github.com/pablo-banker/junglegaming-test/internal/infrastructure/sqs"
	"github.com/pablo-banker/junglegaming-test/internal/observability"
	httptransport "github.com/pablo-banker/junglegaming-test/internal/transport/http"
	"github.com/pablo-banker/junglegaming-test/internal/worker"
)

// App composes the whole service. Modules only declare constructors; lifecycle hooks
// start the pool first and the HTTP server last, and stop them in reverse order.
var App = fx.Options(
	fx.WithLogger(fxLogger),
	config.Module,
	fx.Provide(
		referenceRetryPolicy,
		readinessChecks,
	),
	observability.Module,
	postgres.Module,
	keycloak.Module,
	sqs.Module,
	worker.Module,
	application.Module,
	httptransport.Module,
	fx.Invoke(
		registerBacklogMetrics,
		logConfiguration,
	),
)

// fxLogger writes Fx lifecycle events through the application logger at debug level.
func fxLogger(logger *slog.Logger) fxevent.Logger {
	fxLogger := &fxevent.SlogLogger{Logger: logger}
	fxLogger.UseLogLevel(slog.LevelDebug)

	return fxLogger
}

// referenceRetryPolicy maps configuration into the pending reference retry policy.
func referenceRetryPolicy(cfg config.Config) application.ReferenceRetryPolicy {
	return application.ReferenceRetryPolicy{
		InitialDelay: cfg.ReferenceRetryInitialDelay,
		MaxDelay:     cfg.ReferenceRetryMaxDelay,
		TTL:          cfg.ReferenceTTL,
	}
}

// readinessChecks lists the dependencies that must be reachable to serve traffic.
func readinessChecks(pool *pgxpool.Pool, queue *sqs.HealthChecker) []httptransport.ReadinessCheck {
	return []httptransport.ReadinessCheck{
		{Name: "postgres", Check: pool.Ping},
		{Name: "sqs", Check: queue.Check},
	}
}

// registerBacklogMetrics exposes the outbox delay, pending references and DLQ size.
func registerBacklogMetrics(registry *prometheus.Registry, backlog *postgres.BacklogRepository, queue *sqs.HealthChecker) {
	observability.RegisterBacklog(registry, func(ctx context.Context) (observability.Backlog, error) {
		stored, err := backlog.Read(ctx)
		if err != nil {
			return observability.Backlog{}, err
		}

		deadLettered, err := queue.DLQDepth(ctx)
		if err != nil {
			return observability.Backlog{}, err
		}

		return observability.Backlog{
			OutboxPending:          stored.OutboxPending,
			OutboxOldestPendingAge: stored.OutboxOldestPendingAge,
			PendingReferences:      stored.PendingReferences,
			DeadLetteredMessages:   deadLettered,
		}, nil
	})
}

// logConfiguration logs the effective configuration without credentials.
func logConfiguration(cfg config.Config, logger *slog.Logger) {
	logger.Info(
		"configuration loaded",
		slog.String("httpAddr", cfg.HTTPAddr),
		slog.String("logLevel", cfg.LogLevel.String()),
		slog.String("keycloakIssuer", cfg.KeycloakIssuerURL),
		slog.String("sqsEndpoint", cfg.SQSEndpoint),
		slog.String("wagerQueue", cfg.SQSQueueURL),
		slog.String("eventQueue", cfg.SQSEventQueueURL),
		slog.Duration("referenceTTL", cfg.ReferenceTTL),
	)
}
