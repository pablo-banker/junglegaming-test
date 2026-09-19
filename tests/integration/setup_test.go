//go:build integration

package integration

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const integrationTestTimeout = 10 * time.Second

// openIntegrationPool opens the PostgreSQL connection used by integration tests.
func openIntegrationPool(t *testing.T) *pgxpool.Pool {
	t.Helper()

	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not configured")
	}

	ctx, cancel := context.WithTimeout(
		context.Background(),
		integrationTestTimeout,
	)
	defer cancel()

	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatalf("failed to parse TEST_DATABASE_URL: %v", err)
	}

	// Keep enough connections available for concurrency integration tests.
	config.MaxConns = 10

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatalf("failed to create integration postgres pool: %v", err)
	}

	t.Cleanup(pool.Close)

	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("failed to connect to integration postgres: %v", err)
	}

	requireMigrations(t, pool)

	return pool
}

// integrationContext creates a bounded context for an integration test.
func integrationContext(t *testing.T) context.Context {
	t.Helper()

	ctx, cancel := context.WithTimeout(
		context.Background(),
		integrationTestTimeout,
	)

	t.Cleanup(cancel)

	return ctx
}

// requireMigrations verifies the integration database has the project schema.
func requireMigrations(
	t *testing.T,
	pool *pgxpool.Pool,
) {
	t.Helper()

	ctx, cancel := context.WithTimeout(
		context.Background(),
		integrationTestTimeout,
	)
	defer cancel()

	var exists bool

	err := pool.QueryRow(
		ctx,
		`
			SELECT to_regclass('public.wallets') IS NOT NULL
		`,
	).Scan(&exists)
	if err != nil {
		t.Fatalf("failed to verify integration database migrations: %v", err)
	}

	if !exists {
		t.Fatal("integration database is not migrated")
	}
}
