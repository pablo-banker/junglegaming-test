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

// Now returns the PostgreSQL clock_timestamp(), which advances inside a transaction.
func (c *Clock) Now(ctx context.Context) (time.Time, error) {
	var now time.Time

	err := db(ctx, c.pool).QueryRow(
		ctx,
		`SELECT clock_timestamp()`,
	).Scan(&now)
	if err != nil {
		return time.Time{}, err
	}

	return now, nil
}
