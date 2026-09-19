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

// TestPostgresClockUsesTransactionTimestamp verifies stable time inside one transaction.
func TestPostgresClockUsesTransactionTimestamp(t *testing.T) {
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

	if !first.Equal(second) {
		t.Fatalf(
			"expected stable transaction timestamp, got %s and %s",
			first,
			second,
		)
	}
}
