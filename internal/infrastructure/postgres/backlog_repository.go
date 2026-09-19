package postgres

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Backlog is the durable work still waiting in PostgreSQL.
type Backlog struct {
	OutboxPending          int64
	OutboxOldestPendingAge time.Duration
	PendingReferences      int64
}

type BacklogRepository struct {
	pool *pgxpool.Pool
}

// NewBacklogRepository creates the repository used by the backlog metrics.
func NewBacklogRepository(pool *pgxpool.Pool) *BacklogRepository {
	return &BacklogRepository{
		pool: pool,
	}
}

// Read counts unpublished outbox events, the age of the oldest one and pending references.
func (r *BacklogRepository) Read(ctx context.Context) (Backlog, error) {
	const query = `
		SELECT
			(SELECT COUNT(*) FROM outbox_events WHERE published_at IS NULL),
			COALESCE(
				(SELECT EXTRACT(EPOCH FROM clock_timestamp() - MIN(occurred_at)) FROM outbox_events WHERE published_at IS NULL),
				0
			)::float8,
			(SELECT COUNT(*) FROM wager_transactions WHERE status = 'PENDING_REFERENCE')
	`

	var (
		backlog      Backlog
		oldestSecond float64
	)

	err := r.pool.QueryRow(ctx, query).Scan(
		&backlog.OutboxPending,
		&oldestSecond,
		&backlog.PendingReferences,
	)
	if err != nil {
		return Backlog{}, err
	}

	backlog.OutboxOldestPendingAge = time.Duration(oldestSecond * float64(time.Second))

	return backlog, nil
}
