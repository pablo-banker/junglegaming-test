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

// Now returns the current PostgreSQL wall-clock time.
//
// clock_timestamp() advances inside a transaction, unlike CURRENT_TIMESTAMP, which is
// frozen at BEGIN. Reading it after the wallet lock guarantees that movements of the
// same wallet never observe a time earlier than the previous committed movement.
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
