package application

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"

	"github.com/pablo-banker/junglegaming-test/internal/domain"
)

// TransactionManager executes application operations inside a single transaction.
type TransactionManager interface {
	WithinTransaction(ctx context.Context, fn func(ctx context.Context) error) error
}

// Clock provides the authoritative application timestamp.
type Clock interface {
	Now(ctx context.Context) (time.Time, error)
}

// WalletRepository defines persistence operations required for wallets.
type WalletRepository interface {
	Create(ctx context.Context, wallet *domain.Wallet) error
	FindByID(ctx context.Context, id uuid.UUID) (*domain.Wallet, error)
	FindByIDForUpdate(ctx context.Context, id uuid.UUID) (*domain.Wallet, error)
	FindByPlayerAndCurrency(ctx context.Context, playerID uuid.UUID, currency domain.Currency) (*domain.Wallet, error)
	Update(ctx context.Context, wallet *domain.Wallet) error
}

// WagerRepository defines persistence operations required for wager transactions.
type WagerRepository interface {
	Create(ctx context.Context, transaction *domain.WagerTransaction) error
	Update(ctx context.Context, transaction *domain.WagerTransaction) error
	UpdatePendingReference(ctx context.Context, transaction *domain.WagerTransaction, nextAttemptAt time.Time, expiresAt time.Time, metadata CommandMetadata) error
	FindByID(ctx context.Context, id uuid.UUID) (*domain.WagerTransaction, error)
	FindByProviderAndExternalTransactionID(ctx context.Context, providerID string, externalTransactionID string) (*domain.WagerTransaction, error)
	FindByProviderAndIdempotencyKey(ctx context.Context, providerID string, idempotencyKey string) (*domain.WagerTransaction, error)
	HasProcessedDirectReversal(ctx context.Context, referenceTransactionID uuid.UUID) (bool, error)
	FindDuePendingReferenceForUpdate(ctx context.Context, now time.Time) (*PendingReferenceWork, error)
	FindPendingReferenceForUpdate(ctx context.Context, transactionID uuid.UUID) (*PendingReferenceWork, error)
	SchedulePendingReferenceRetry(ctx context.Context, transactionID uuid.UUID, retryCount int, nextAttemptAt time.Time) error
}

// WalletLedgerRepository defines persistence operations required for ledger entries.
type WalletLedgerRepository interface {
	Create(ctx context.Context, entry *domain.WalletLedgerEntry) error
	ListByWallet(ctx context.Context, walletID uuid.UUID, beforeCreatedAt *time.Time, beforeID *uuid.UUID, limit int) ([]*domain.WalletLedgerEntry, error)
	ReconciliationSnapshot(ctx context.Context, walletID uuid.UUID) (*ReconciliationSnapshot, error)
}

// ReconciliationSnapshot is the stored wallet balance and its ledger reconstruction, read together.
type ReconciliationSnapshot struct {
	StoredBalance     domain.Money
	CalculatedBalance domain.Money
	CheckedEntries    int64
}

type InboxRepository interface {
	// Register records an inbox message and reports whether it was already completed.
	Register(ctx context.Context, consumerName string, messageID string, messageHash string) (bool, error)
	Complete(ctx context.Context, consumerName string, messageID string, completedAt time.Time) error
}

// OutboxRepository defines persistence operations required for application events.
type OutboxRepository interface {
	Create(ctx context.Context, event OutboxEvent) error
}

// PendingReferenceWork is a locked PENDING_REFERENCE transaction with its durable retry state.
type PendingReferenceWork struct {
	Transaction *domain.WagerTransaction
	RetryCount  int
	ExpiresAt   time.Time
	Metadata    CommandMetadata
}

type PendingOutboxEvent struct {
	EventID       uuid.UUID
	EventType     string
	AggregateID   uuid.UUID
	CorrelationID string
	CausationID   string
	Version       int64
	Payload       json.RawMessage
	OccurredAt    time.Time
	Attempts      int
}

// OutboxDispatcherRepository manages durable outbox delivery state.
type OutboxDispatcherRepository interface {
	ClaimPending(ctx context.Context, workerID string, leaseUntil time.Time) (*PendingOutboxEvent, error)
	MarkPublished(ctx context.Context, eventID uuid.UUID, workerID string, publishedAt time.Time) error
	ScheduleRetry(ctx context.Context, eventID uuid.UUID, workerID string, nextAttemptAt time.Time) error
}

type IntegrationEventPublisher interface {
	Publish(ctx context.Context, event *PendingOutboxEvent) error
}
