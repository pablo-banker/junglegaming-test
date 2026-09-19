package postgres

import (
	"go.uber.org/fx"

	"github.com/pablo-banker/junglegaming-test/internal/application"
)

// Module provides PostgreSQL infrastructure dependencies.
var Module = fx.Module(
	"postgres",
	fx.Provide(
		NewPool,

		fx.Annotate(
			NewTransactionManager,
			fx.As(new(application.TransactionManager)),
		),

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
		fx.Annotate(NewInboxRepository,
			fx.As(new(application.InboxRepository)),
		),
	),
)
