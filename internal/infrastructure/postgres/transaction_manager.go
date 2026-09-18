package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pablo-banker/junglegaming-test/internal/application"
)

type transactionContextKey struct{}

type TransactionManager struct {
	pool *pgxpool.Pool
}

var _ application.TransactionManager = (*TransactionManager)(nil)

// NewTransactionManager creates a PostgreSQL transaction manager.
func NewTransactionManager(pool *pgxpool.Pool) *TransactionManager {
	return &TransactionManager{
		pool: pool,
	}
}

// WithinTransaction executes the function inside a PostgreSQL transaction.
func (m *TransactionManager) WithinTransaction(
	ctx context.Context,
	fn func(context.Context) error,
) error {
	if _, ok := txFromContext(ctx); ok {
		return fn(ctx)
	}

	return pgx.BeginTxFunc(
		ctx,
		m.pool,
		pgx.TxOptions{
			IsoLevel: pgx.ReadCommitted,
		},
		func(tx pgx.Tx) error {
			txCtx := context.WithValue(
				ctx,
				transactionContextKey{},
				tx,
			)

			return fn(txCtx)
		},
	)
}

// txFromContext returns the active PostgreSQL transaction when available.
func txFromContext(ctx context.Context) (pgx.Tx, bool) {
	tx, ok := ctx.Value(transactionContextKey{}).(pgx.Tx)

	return tx, ok
}
