package postgres

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
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

// ListByWallet returns wallet ledger entries using stable cursor pagination.
func (r *WalletLedgerRepository) ListByWallet(
	ctx context.Context,
	walletID uuid.UUID,
	beforeCreatedAt *time.Time,
	beforeID *uuid.UUID,
	limit int,
) ([]*domain.WalletLedgerEntry, error) {
	const baseQuery = `
		SELECT
			id,
			wallet_id,
			transaction_id,
			direction::text,
			amount::text,
			currency,
			balance_before::text,
			balance_after::text,
			created_at
		FROM wallet_ledger_entries
		WHERE wallet_id = $1
	`

	var (
		rows pgx.Rows
		err  error
	)

	if beforeCreatedAt != nil && beforeID != nil {
		rows, err = db(ctx, r.pool).Query(
			ctx,
			baseQuery+`
				AND (created_at, id) < ($2, $3)
				ORDER BY created_at DESC, id DESC
				LIMIT $4
			`,
			walletID,
			*beforeCreatedAt,
			*beforeID,
			limit,
		)
	} else {
		rows, err = db(ctx, r.pool).Query(
			ctx,
			baseQuery+`
				ORDER BY created_at DESC, id DESC
				LIMIT $2
			`,
			walletID,
			limit,
		)
	}

	if err != nil {
		return nil, err
	}

	defer rows.Close()

	entries := make([]*domain.WalletLedgerEntry, 0, limit)

	for rows.Next() {
		var (
			id            uuid.UUID
			storedWallet  uuid.UUID
			transactionID uuid.UUID
			direction     string
			amount        string
			currencyCode  string
			balanceBefore string
			balanceAfter  string
			createdAt     time.Time
		)

		if err := rows.Scan(
			&id,
			&storedWallet,
			&transactionID,
			&direction,
			&amount,
			&currencyCode,
			&balanceBefore,
			&balanceAfter,
			&createdAt,
		); err != nil {
			return nil, err
		}

		currency, err := domain.NewCurrency(currencyCode)
		if err != nil {
			return nil, err
		}

		entryAmount, err := domain.ParseMoney(amount, currency)
		if err != nil {
			return nil, err
		}

		before, err := domain.ParseMoney(balanceBefore, currency)
		if err != nil {
			return nil, err
		}

		after, err := domain.ParseMoney(balanceAfter, currency)
		if err != nil {
			return nil, err
		}

		entry, err := domain.RehydrateWalletLedgerEntry(
			id,
			storedWallet,
			transactionID,
			domain.WalletLedgerDirection(direction),
			entryAmount,
			before,
			after,
			createdAt,
		)
		if err != nil {
			return nil, err
		}

		entries = append(entries, entry)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return entries, nil
}

// CalculateBalance reconstructs the current wallet balance from its ledger entries.
func (r *WalletLedgerRepository) CalculateBalance(ctx context.Context, walletID uuid.UUID, currency domain.Currency) (domain.Money, int64, error) {
	const query = `
		SELECT
			COALESCE(
				SUM(
					CASE
						WHEN direction = 'CREDIT'
							THEN amount
						ELSE -amount
					END
				),
				0
			)::numeric(20, 2)::text,
			COUNT(*)
		FROM wallet_ledger_entries
		WHERE wallet_id = $1
	`

	var (
		amount string
		count  int64
	)

	err := db(ctx, r.pool).QueryRow(
		ctx,
		query,
		walletID,
	).Scan(
		&amount,
		&count,
	)
	if err != nil {
		return domain.Money{}, 0, err
	}

	balance, err := domain.ParseMoney(amount, currency)
	if err != nil {
		return domain.Money{}, 0, err
	}

	return balance, count, nil
}
