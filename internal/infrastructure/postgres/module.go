package postgres

import (
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/fx"

	"github.com/pablo-banker/junglegaming-test/internal/application"
	"github.com/pablo-banker/junglegaming-test/internal/observability"
)

// Module provides PostgreSQL infrastructure dependencies.
var Module = fx.Module(
	"postgres",
	fx.Provide(
		NewPool,

		fx.Annotate(
			newObservedTransactionManager,
			fx.As(new(application.TransactionManager)),
		),

		NewBacklogRepository,

		fx.Annotate(
			NewClock,
			fx.As(new(application.Clock)),
		),

		fx.Annotate(
			NewWalletRepository,
			fx.As(new(application.WalletRepository)),
		),

		fx.Annotate(
			NewWagerRepository,
			fx.As(new(application.WagerRepository)),
		),

		fx.Annotate(
			NewWalletLedgerRepository,
			fx.As(new(application.WalletLedgerRepository)),
		),

		fx.Annotate(
			NewOutboxRepository,
			fx.As(new(application.OutboxRepository)),
		),
		fx.Annotate(
			NewInboxRepository,
			fx.As(new(application.InboxRepository)),
		),

		fx.Annotate(
			NewOutboxDispatcherRepository,
			fx.As(new(application.OutboxDispatcherRepository)),
		),
	),
)

// newObservedTransactionManager counts transactions retried after concurrency conflicts.
func newObservedTransactionManager(pool *pgxpool.Pool, metrics *observability.Metrics) *TransactionManager {
	manager := NewTransactionManager(pool)
	manager.onRetry = metrics.CountTransactionRetry

	return manager
}
