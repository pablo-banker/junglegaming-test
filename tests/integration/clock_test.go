//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/pablo-banker/junglegaming-test/internal/infrastructure/postgres"
)

// TestPostgresClockReturnsDatabaseTime verifies database-backed time retrieval.
func TestPostgresClockReturnsDatabaseTime(t *testing.T) {
	ctx := integrationContext(t)
	pool := openIntegrationPool(t)

	clock := postgres.NewClock(pool)

	before := time.Now().UTC()

	now, err := clock.Now(ctx)
	if err != nil {
		t.Fatalf("failed to get database time: %v", err)
	}

	after := time.Now().UTC()

	if now.Before(before.Add(-time.Second)) {
		t.Fatalf(
			"database time %s is unexpectedly before local time %s",
			now,
			before,
		)
	}

	if now.After(after.Add(time.Second)) {
		t.Fatalf(
			"database time %s is unexpectedly after local time %s",
			now,
			after,
		)
	}
}

// TestPostgresClockAdvancesInsideTransaction verifies the clock is not frozen at BEGIN.
// Reading it after the wallet lock must observe time after the previous committed movement.
func TestPostgresClockAdvancesInsideTransaction(t *testing.T) {
	ctx := integrationContext(t)
	pool := openIntegrationPool(t)

	clock := postgres.NewClock(pool)
	txManager := postgres.NewTransactionManager(pool)

	var first time.Time
	var second time.Time

	err := txManager.WithinTransaction(
		ctx,
		func(txCtx context.Context) error {
			var err error

			first, err = clock.Now(txCtx)
			if err != nil {
				return err
			}

			time.Sleep(50 * time.Millisecond)

			second, err = clock.Now(txCtx)
			return err
		},
	)
	if err != nil {
		t.Fatalf("transaction failed: %v", err)
	}

	if !second.After(first) {
		t.Fatalf(
			"expected clock to advance inside the transaction, got %s and %s",
			first,
			second,
		)
	}
}
