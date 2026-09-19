package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pablo-banker/junglegaming-test/internal/application"
)

type OutboxDispatcherRepository struct {
	pool *pgxpool.Pool
}

var _ application.OutboxDispatcherRepository = (*OutboxDispatcherRepository)(nil)

// NewOutboxDispatcherRepository creates the PostgreSQL outbox dispatcher repository.
func NewOutboxDispatcherRepository(pool *pgxpool.Pool) *OutboxDispatcherRepository {
	return &OutboxDispatcherRepository{
		pool: pool,
	}
}

// ClaimPending claims one available outbox event for publication.
func (r *OutboxDispatcherRepository) ClaimPending(
	ctx context.Context,
	workerID string,
	leaseUntil time.Time,
) (*application.PendingOutboxEvent, error) {
	const query = `
		WITH candidate AS (
			SELECT event_id
			FROM outbox_events
			WHERE published_at IS NULL
			  AND next_attempt_at <= CURRENT_TIMESTAMP
			  AND (
				  claimed_until IS NULL
				  OR claimed_until <= CURRENT_TIMESTAMP
			  )
			ORDER BY next_attempt_at, occurred_at
			FOR UPDATE SKIP LOCKED
			LIMIT 1
		)
		UPDATE outbox_events AS o
		SET
			attempts = o.attempts + 1,
			claimed_by = $1,
			claimed_until = $2
		FROM candidate
		WHERE o.event_id = candidate.event_id
		RETURNING
			o.event_id,
			o.event_type,
			o.aggregate_id,
			o.correlation_id,
			o.causation_id,
			o.version,
			o.payload,
			o.occurred_at,
			o.attempts
	`

	var (
		event       application.PendingOutboxEvent
		causationID *string
		payload     []byte
	)

	err := r.pool.QueryRow(
		ctx,
		query,
		workerID,
		leaseUntil,
	).Scan(
		&event.EventID,
		&event.EventType,
		&event.AggregateID,
		&event.CorrelationID,
		&causationID,
		&event.Version,
		&payload,
		&event.OccurredAt,
		&event.Attempts,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, application.ErrNotFound
		}

		return nil, err
	}

	if causationID != nil {
		event.CausationID = *causationID
	}

	event.Payload = json.RawMessage(payload)

	return &event, nil
}

// MarkPublished marks a claimed outbox event as successfully published.
func (r *OutboxDispatcherRepository) MarkPublished(
	ctx context.Context,
	eventID uuid.UUID,
	workerID string,
	publishedAt time.Time,
) error {
	const query = `
		UPDATE outbox_events
		SET
			published_at = $3,
			claimed_by = NULL,
			claimed_until = NULL
		WHERE event_id = $1
		  AND claimed_by = $2
		  AND published_at IS NULL
	`

	result, err := r.pool.Exec(
		ctx,
		query,
		eventID,
		workerID,
		publishedAt,
	)
	if err != nil {
		return err
	}

	if result.RowsAffected() == 0 {
		return application.ErrNotFound
	}

	return nil
}

// ScheduleRetry releases a failed outbox event and schedules another attempt.
func (r *OutboxDispatcherRepository) ScheduleRetry(
	ctx context.Context,
	eventID uuid.UUID,
	workerID string,
	nextAttemptAt time.Time,
) error {
	const query = `
		UPDATE outbox_events
		SET
			next_attempt_at = $3,
			claimed_by = NULL,
			claimed_until = NULL
		WHERE event_id = $1
		  AND claimed_by = $2
		  AND published_at IS NULL
	`

	result, err := r.pool.Exec(
		ctx,
		query,
		eventID,
		workerID,
		nextAttemptAt,
	)
	if err != nil {
		return err
	}

	if result.RowsAffected() == 0 {
		return application.ErrNotFound
	}

	return nil
}
