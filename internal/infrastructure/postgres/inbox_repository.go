package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pablo-banker/junglegaming-test/internal/application"
)

type InboxRepository struct {
	pool *pgxpool.Pool
}

// NewInboxRepository creates the PostgreSQL inbox repository.
func NewInboxRepository(pool *pgxpool.Pool) *InboxRepository {
	return &InboxRepository{
		pool: pool,
	}
}

// Register records an inbox message and reports whether it was already completed.
func (r *InboxRepository) Register(ctx context.Context, consumerName string, messageID string, messageHash string) (bool, error) {
	const insertQuery = `
		INSERT INTO inbox_messages (
			consumer_name,
			message_id,
			message_hash
		)
		VALUES ($1, $2, $3)
		ON CONFLICT (consumer_name, message_id) DO NOTHING
		RETURNING consumer_name
	`

	var inserted string

	err := db(ctx, r.pool).QueryRow(ctx, insertQuery, consumerName, messageID, messageHash).Scan(&inserted)
	if err == nil {
		return false, nil
	}

	if !errors.Is(err, pgx.ErrNoRows) {
		return false, err
	}

	const findQuery = `
		SELECT
			message_hash,
			completed_at
		FROM inbox_messages
		WHERE consumer_name = $1
		  AND message_id = $2
	`

	var existingHash string
	var completedAt *time.Time

	err = db(ctx, r.pool).QueryRow(ctx, findQuery, consumerName, messageID).Scan(&existingHash, &completedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, application.ErrInboxMessageNotFound
		}

		return false, err
	}

	if existingHash != messageHash {
		return false, application.ErrInboxMessageConflict
	}

	return completedAt != nil, nil
}

// Complete marks an inbox message as successfully processed.
func (r *InboxRepository) Complete(ctx context.Context, consumerName string, messageID string, completedAt time.Time) error {
	const query = `
		UPDATE inbox_messages
		SET completed_at = $3
		WHERE consumer_name = $1
		  AND message_id = $2
		  AND completed_at IS NULL
	`

	tag, err := db(ctx, r.pool).Exec(ctx, query, consumerName, messageID, completedAt)
	if err != nil {
		return err
	}

	if tag.RowsAffected() != 1 {
		return application.ErrInboxMessageNotFound
	}

	return nil
}
