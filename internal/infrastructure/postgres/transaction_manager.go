package postgres

import (
	"context"
	"fmt"
	"math/rand/v2"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pablo-banker/junglegaming-test/internal/application"
)

// maxTransactionAttempts bounds retries of transactions aborted by deadlocks or serialization failures.
const maxTransactionAttempts = 3

type transactionContextKey struct{}

type TransactionManager struct {
	pool *pgxpool.Pool

	// onRetry, when set, observes the SQLSTATE of every retried transaction.
	onRetry func(sqlstate string)
}

var _ application.TransactionManager = (*TransactionManager)(nil)

// NewTransactionManager creates a PostgreSQL transaction manager.
func NewTransactionManager(pool *pgxpool.Pool) *TransactionManager {
	return &TransactionManager{
		pool: pool,
	}
}

// WithinTransaction executes the function inside a PostgreSQL transaction.
//
// Nested calls join the active transaction. The outermost call retries a bounded number
// of times when PostgreSQL aborts it with a deadlock or serialization failure, so fn must
// not keep state between attempts. Failures of an unavailable database are wrapped with
// application.ErrUnavailable.
func (m *TransactionManager) WithinTransaction(
	ctx context.Context,
	fn func(context.Context) error,
) error {
	if _, ok := txFromContext(ctx); ok {
		return fn(ctx)
	}

	var err error

	for attempt := 1; attempt <= maxTransactionAttempts; attempt++ {
		err = m.run(ctx, fn)

		if !isRetryableConflict(err) || attempt == maxTransactionAttempts {
			break
		}

		if m.onRetry != nil {
			m.onRetry(sqlState(err))
		}

		if !waitRetry(ctx, attempt) {
			break
		}
	}

	if isUnavailable(err) {
		return fmt.Errorf("%w: %w", application.ErrUnavailable, err)
	}

	return err
}

// run executes one transaction attempt.
func (m *TransactionManager) run(ctx context.Context, fn func(context.Context) error) error {
	return pgx.BeginTxFunc(
		ctx,
		m.pool,
		pgx.TxOptions{
			IsoLevel: pgx.ReadCommitted,
		},
		func(tx pgx.Tx) error {
			return fn(context.WithValue(ctx, transactionContextKey{}, tx))
		},
	)
}

// waitRetry waits a short jittered delay before another attempt.
func waitRetry(ctx context.Context, attempt int) bool {
	delay := time.Duration(attempt*10+rand.IntN(20)) * time.Millisecond

	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-timer.C:
		return true

	case <-ctx.Done():
		return false
	}
}

// txFromContext returns the active PostgreSQL transaction when available.
func txFromContext(ctx context.Context) (pgx.Tx, bool) {
	tx, ok := ctx.Value(transactionContextKey{}).(pgx.Tx)

	return tx, ok
}
