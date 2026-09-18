package postgres

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Clock provides PostgreSQL-backed application time.
type Clock struct {
	pool *pgxpool.Pool
}

// NewClock creates a PostgreSQL-backed clock.
func NewClock(pool *pgxpool.Pool) *Clock {
	return &Clock{
		pool: pool,
	}
}

// Now returns the current PostgreSQL transaction timestamp.
func (c *Clock) Now(ctx context.Context) (time.Time, error) {
	var now time.Time

	err := c.db(ctx).QueryRow(
		ctx,
		`SELECT CURRENT_TIMESTAMP`,
	).Scan(&now)
	if err != nil {
		return time.Time{}, err
	}

	return now, nil
}

// db returns the active transaction when available.
func (c *Clock) db(ctx context.Context) dbExecutor {
	if tx, ok := txFromContext(ctx); ok {
		return tx
	}

	return c.pool
}
