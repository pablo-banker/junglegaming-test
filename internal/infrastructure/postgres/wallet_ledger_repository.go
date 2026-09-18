package postgres

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pablo-banker/junglegaming-test/internal/application"
	"github.com/pablo-banker/junglegaming-test/internal/domain"
)

type WalletLedgerRepository struct {
	pool *pgxpool.Pool
}

var _ application.WalletLedgerRepository = (*WalletLedgerRepository)(nil)

// NewWalletLedgerRepository creates a PostgreSQL wallet ledger repository.
func NewWalletLedgerRepository(pool *pgxpool.Pool) *WalletLedgerRepository {
	return &WalletLedgerRepository{
		pool: pool,
	}
}

// Create persists an immutable wallet ledger entry.
func (r *WalletLedgerRepository) Create(ctx context.Context, entry *domain.WalletLedgerEntry) error {
	tx, ok := txFromContext(ctx)
	if !ok {
		return ErrTransactionRequired
	}

	const query = `
		INSERT INTO wallet_ledger_entries (
			id,
			wallet_id,
			transaction_id,
			direction,
			amount,
			currency,
			balance_before,
			balance_after,
			created_at
		)
		VALUES (
			$1,
			$2,
			$3,
			$4,
			$5::text::numeric,
			$6,
			$7::text::numeric,
			$8::text::numeric,
			$9
		)
	`

	_, err := tx.Exec(
		ctx,
		query,
		entry.ID(),
		entry.WalletID(),
		entry.TransactionID(),
		entry.Direction().String(),
		entry.Amount().Amount(),
		entry.Amount().Currency().Code(),
		entry.BalanceBefore().Amount(),
		entry.BalanceAfter().Amount(),
		entry.CreatedAt(),
	)
	if err != nil {
		return err
	}

	return nil
}
