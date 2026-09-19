//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/pablo-banker/junglegaming-test/internal/application"
	"github.com/pablo-banker/junglegaming-test/internal/infrastructure/postgres"
)

// TestInboxRepositoryRegistersNewMessage verifies a new message is registered as incomplete.
func TestInboxRepositoryRegistersNewMessage(t *testing.T) {
	pool := openIntegrationPool(t)
	repository := postgres.NewInboxRepository(pool)

	consumerName := "wager-consumer"
	messageID := "message-" + uuid.NewString()
	messageHash := "hash-" + uuid.NewString()

	alreadyCompleted, err := repository.Register(context.Background(), consumerName, messageID, messageHash)
	if err != nil {
		t.Fatalf("unexpected register error: %v", err)
	}

	if alreadyCompleted {
		t.Fatal("expected new message not to be completed")
	}

	var storedHash string
	var completedAt *time.Time

	err = pool.QueryRow(
		context.Background(),
		`
			SELECT message_hash, completed_at
			FROM inbox_messages
			WHERE consumer_name = $1
			  AND message_id = $2
		`,
		consumerName,
		messageID,
	).Scan(&storedHash, &completedAt)
	if err != nil {
		t.Fatalf("failed to query inbox message: %v", err)
	}

	if storedHash != messageHash {
		t.Errorf("expected hash %s, got %s", messageHash, storedHash)
	}

	if completedAt != nil {
		t.Fatal("expected completed_at to be nil")
	}
}

// TestInboxRepositoryRecognizesCompletedMessage verifies completed messages are treated as duplicates.
func TestInboxRepositoryRecognizesCompletedMessage(t *testing.T) {
	pool := openIntegrationPool(t)
	repository := postgres.NewInboxRepository(pool)

	consumerName := "wager-consumer"
	messageID := "message-" + uuid.NewString()
	messageHash := "hash-" + uuid.NewString()

	alreadyCompleted, err := repository.Register(context.Background(), consumerName, messageID, messageHash)
	if err != nil {
		t.Fatalf("unexpected register error: %v", err)
	}

	if alreadyCompleted {
		t.Fatal("expected first registration not to be completed")
	}

	completedAt := time.Now().UTC()

	err = repository.Complete(context.Background(), consumerName, messageID, completedAt)
	if err != nil {
		t.Fatalf("unexpected complete error: %v", err)
	}

	alreadyCompleted, err = repository.Register(context.Background(), consumerName, messageID, messageHash)
	if err != nil {
		t.Fatalf("unexpected duplicate register error: %v", err)
	}

	if !alreadyCompleted {
		t.Fatal("expected completed message to be recognized")
	}
}

// TestInboxRepositoryRejectsMessageHashConflict verifies message identity cannot represent another payload.
func TestInboxRepositoryRejectsMessageHashConflict(t *testing.T) {
	pool := openIntegrationPool(t)
	repository := postgres.NewInboxRepository(pool)

	consumerName := "wager-consumer"
	messageID := "message-" + uuid.NewString()

	_, err := repository.Register(context.Background(), consumerName, messageID, "hash-original")
	if err != nil {
		t.Fatalf("unexpected first register error: %v", err)
	}

	_, err = repository.Register(context.Background(), consumerName, messageID, "hash-changed")

	if !errors.Is(err, application.ErrInboxMessageConflict) {
		t.Fatalf("expected ErrInboxMessageConflict, got %v", err)
	}
}

// TestInboxRepositoryAllowsIncompleteMessageRetry verifies interrupted messages can be retried.
func TestInboxRepositoryAllowsIncompleteMessageRetry(t *testing.T) {
	pool := openIntegrationPool(t)
	repository := postgres.NewInboxRepository(pool)

	consumerName := "wager-consumer"
	messageID := "message-" + uuid.NewString()
	messageHash := "hash-" + uuid.NewString()

	alreadyCompleted, err := repository.Register(context.Background(), consumerName, messageID, messageHash)
	if err != nil {
		t.Fatalf("unexpected first register error: %v", err)
	}

	if alreadyCompleted {
		t.Fatal("expected first registration not to be completed")
	}

	alreadyCompleted, err = repository.Register(context.Background(), consumerName, messageID, messageHash)
	if err != nil {
		t.Fatalf("unexpected retry register error: %v", err)
	}

	if alreadyCompleted {
		t.Fatal("expected incomplete message to remain processable")
	}
}

// TestInboxRepositoryCompleteReturnsNotFound verifies unknown messages cannot be completed.
func TestInboxRepositoryCompleteReturnsNotFound(t *testing.T) {
	pool := openIntegrationPool(t)
	repository := postgres.NewInboxRepository(pool)

	err := repository.Complete(
		context.Background(),
		"wager-consumer",
		"missing-"+uuid.NewString(),
		time.Now().UTC(),
	)

	if !errors.Is(err, application.ErrInboxMessageNotFound) {
		t.Fatalf("expected ErrInboxMessageNotFound, got %v", err)
	}
}

// TestInboxRepositoryCannotCompleteMessageTwice verifies completion is applied once.
func TestInboxRepositoryCannotCompleteMessageTwice(t *testing.T) {
	pool := openIntegrationPool(t)
	repository := postgres.NewInboxRepository(pool)

	consumerName := "wager-consumer"
	messageID := "message-" + uuid.NewString()
	messageHash := "hash-" + uuid.NewString()

	_, err := repository.Register(context.Background(), consumerName, messageID, messageHash)
	if err != nil {
		t.Fatalf("unexpected register error: %v", err)
	}

	err = repository.Complete(context.Background(), consumerName, messageID, time.Now().UTC())
	if err != nil {
		t.Fatalf("unexpected first complete error: %v", err)
	}

	err = repository.Complete(context.Background(), consumerName, messageID, time.Now().UTC())

	if !errors.Is(err, application.ErrInboxMessageNotFound) {
		t.Fatalf("expected ErrInboxMessageNotFound, got %v", err)
	}
}

// TestInboxRepositoryRollsBackRegistration verifies inbox registration follows the transaction rollback.
func TestInboxRepositoryRollsBackRegistration(t *testing.T) {
	pool := openIntegrationPool(t)

	repository := postgres.NewInboxRepository(pool)
	transactionManager := postgres.NewTransactionManager(pool)

	consumerName := "wager-consumer"
	messageID := "message-" + uuid.NewString()
	messageHash := "hash-" + uuid.NewString()
	expectedErr := errors.New("forced rollback")

	err := transactionManager.WithinTransaction(
		context.Background(),
		func(txCtx context.Context) error {
			alreadyCompleted, err := repository.Register(txCtx, consumerName, messageID, messageHash)
			if err != nil {
				return err
			}

			if alreadyCompleted {
				t.Fatal("expected new message not to be completed")
			}

			return expectedErr
		},
	)

	if !errors.Is(err, expectedErr) {
		t.Fatalf("expected forced rollback error, got %v", err)
	}

	var count int

	err = pool.QueryRow(
		context.Background(),
		`
			SELECT COUNT(*)
			FROM inbox_messages
			WHERE consumer_name = $1
			  AND message_id = $2
		`,
		consumerName,
		messageID,
	).Scan(&count)
	if err != nil {
		t.Fatalf("failed to query inbox messages: %v", err)
	}

	if count != 0 {
		t.Fatalf("expected rollback to remove inbox registration, got %d rows", count)
	}
}

// TestInboxRepositoryCommitsRegistrationAndCompletion verifies inbox state commits atomically.
func TestInboxRepositoryCommitsRegistrationAndCompletion(t *testing.T) {
	pool := openIntegrationPool(t)

	repository := postgres.NewInboxRepository(pool)
	transactionManager := postgres.NewTransactionManager(pool)

	consumerName := "wager-consumer"
	messageID := "message-" + uuid.NewString()
	messageHash := "hash-" + uuid.NewString()
	completedAt := time.Now().UTC()

	err := transactionManager.WithinTransaction(
		context.Background(),
		func(txCtx context.Context) error {
			alreadyCompleted, err := repository.Register(txCtx, consumerName, messageID, messageHash)
			if err != nil {
				return err
			}

			if alreadyCompleted {
				t.Fatal("expected new message not to be completed")
			}

			return repository.Complete(txCtx, consumerName, messageID, completedAt)
		},
	)
	if err != nil {
		t.Fatalf("unexpected transaction error: %v", err)
	}

	alreadyCompleted, err := repository.Register(context.Background(), consumerName, messageID, messageHash)
	if err != nil {
		t.Fatalf("unexpected duplicate register error: %v", err)
	}

	if !alreadyCompleted {
		t.Fatal("expected committed message to be completed")
	}
}
