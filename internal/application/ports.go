package application

import (
	"context"
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
	UpdatePendingReference(ctx context.Context, transaction *domain.WagerTransaction, nextAttemptAt time.Time, expiresAt time.Time) error
	FindByID(ctx context.Context, id uuid.UUID) (*domain.WagerTransaction, error)
	FindByProviderAndExternalTransactionID(ctx context.Context, providerID string, externalTransactionID string) (*domain.WagerTransaction, error)
	FindByProviderAndIdempotencyKey(ctx context.Context, providerID string, idempotencyKey string) (*domain.WagerTransaction, error)
	HasProcessedDirectReversal(ctx context.Context, referenceTransactionID uuid.UUID) (bool, error)
}

// WalletLedgerRepository defines persistence operations required for ledger entries.
type WalletLedgerRepository interface {
	Create(ctx context.Context, entry *domain.WalletLedgerEntry) error
	ListByWallet(ctx context.Context, walletID uuid.UUID, beforeCreatedAt *time.Time, beforeID *uuid.UUID, limit int) ([]*domain.WalletLedgerEntry, error)
	CalculateBalance(ctx context.Context, walletID uuid.UUID, currency domain.Currency) (domain.Money, int64, error)
}

// OutboxRepository defines persistence operations required for application events.
type OutboxRepository interface {
	Create(ctx context.Context, event EventEnvelope) error
}
