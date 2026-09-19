package postgres

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pablo-banker/junglegaming-test/internal/config"
	"go.uber.org/fx"
)

// NewPool creates the PostgreSQL connection pool and registers its lifecycle hooks.
func NewPool(
	lifecycle fx.Lifecycle,
	cfg config.Config,
	logger *slog.Logger,
) (*pgxpool.Pool, error) {

	poolConfig, err := pgxpool.ParseConfig(cfg.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse database url: %w", err)
	}

	// Bound every wait so a stuck lock or query surfaces as a retryable 503.
	runtimeParams := poolConfig.ConnConfig.RuntimeParams
	runtimeParams["application_name"] = "junglegaming"
	runtimeParams["lock_timeout"] = "5s"
	runtimeParams["statement_timeout"] = "15s"
	runtimeParams["idle_in_transaction_session_timeout"] = "30s"

	pool, err := pgxpool.NewWithConfig(context.Background(), poolConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	lifecycle.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			if err := pool.Ping(ctx); err != nil {
				pool.Close()

				return fmt.Errorf("ping database failed: %w", err)
			}

			logger.Info(
				"postgres connection pool ready",
			)

			return nil
		},
		OnStop: func(ctx context.Context) error {
			logger.Info("postgres connection pool shutting down")

			pool.Close()

			return nil
		},
	})

	return pool, nil
}
