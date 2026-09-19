package postgres

import (
	"context"

	"github.com/pablo-banker/junglegaming-test/internal/application"
)

type OutboxRepository struct{}

var _ application.OutboxRepository = (*OutboxRepository)(nil)

// NewOutboxRepository creates a PostgreSQL outbox repository.
func NewOutboxRepository() *OutboxRepository {
	return &OutboxRepository{}
}

// Create persists an event snapshot in the transactional outbox.
func (r *OutboxRepository) Create(ctx context.Context, event application.OutboxEvent) error {
	tx, ok := txFromContext(ctx)
	if !ok {
		return ErrTransactionRequired
	}

	envelope, err := event.Envelope()
	if err != nil {
		return err
	}

	const query = `
		INSERT INTO outbox_events (
			event_id,
			event_type,
			aggregate_id,
			correlation_id,
			causation_id,
			version,
			payload,
			occurred_at
		)
		VALUES (
			$1,
			$2,
			$3,
			$4,
			$5,
			$6,
			$7::jsonb,
			$8
		)
	`

	_, err = tx.Exec(
		ctx,
		query,
		envelope.EventID,
		envelope.EventType,
		envelope.AggregateID,
		envelope.CorrelationID,
		nullableString(envelope.CausationID),
		envelope.Version,
		envelope.Data,
		envelope.OccurredAt,
	)

	return err
}
