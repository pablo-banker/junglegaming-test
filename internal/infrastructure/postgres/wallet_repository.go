package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pablo-banker/junglegaming-test/internal/application"
	"github.com/pablo-banker/junglegaming-test/internal/domain"
)

var ErrTransactionRequired = errors.New("transaction required")

type WalletRepository struct {
	pool *pgxpool.Pool
}

var _ application.WalletRepository = (*WalletRepository)(nil)

// NewWalletRepository creates a PostgreSQL wallet repository.
func NewWalletRepository(pool *pgxpool.Pool) *WalletRepository {
	return &WalletRepository{
		pool: pool,
	}
}

// Create persists a new wallet.
func (r *WalletRepository) Create(ctx context.Context, wallet *domain.Wallet) error {
	const query = `
		INSERT INTO wallets (
			id,
			player_id,
			currency,
			balance,
			version,
			created_at,
			updated_at
		)
		VALUES ($1, $2, $3, $4::text::numeric, $5, $6, $7)
	`

	_, err := db(ctx, r.pool).Exec(
		ctx,
		query,
		wallet.ID(),
		wallet.PlayerID(),
		wallet.Currency().Code(),
		wallet.Balance().Amount(),
		wallet.Version(),
		wallet.CreatedAt(),
		wallet.UpdatedAt(),
	)
	if err != nil {
		if isUniqueViolation(err) {
			return application.ErrAlreadyExists
		}

		return err
	}

	return nil
}

// FindByID returns a wallet by its identifier.
func (r *WalletRepository) FindByID(ctx context.Context, id uuid.UUID) (*domain.Wallet, error) {
	const query = `
		SELECT
			id,
			player_id,
			currency,
			balance::text,
			version,
			created_at,
			updated_at
		FROM wallets
		WHERE id = $1
	`

	return scanWallet(
		db(ctx, r.pool).QueryRow(ctx, query, id),
	)
}

// FindByIDForUpdate locks a wallet with FOR NO KEY UPDATE; FOR UPDATE would deadlock with the FK locks.
func (r *WalletRepository) FindByIDForUpdate(ctx context.Context, id uuid.UUID) (*domain.Wallet, error) {
	tx, ok := txFromContext(ctx)
	if !ok {
		return nil, ErrTransactionRequired
	}

	const query = `
		SELECT
			id,
			player_id,
			currency,
			balance::text,
			version,
			created_at,
			updated_at
		FROM wallets
		WHERE id = $1
		FOR NO KEY UPDATE
	`

	return scanWallet(
		tx.QueryRow(ctx, query, id),
	)
}

// FindByPlayerAndCurrency returns a wallet by player and currency.
func (r *WalletRepository) FindByPlayerAndCurrency(ctx context.Context, playerID uuid.UUID, currency domain.Currency) (*domain.Wallet, error) {
	const query = `
		SELECT
			id,
			player_id,
			currency,
			balance::text,
			version,
			created_at,
			updated_at
		FROM wallets
		WHERE player_id = $1
		  AND currency = $2
	`

	return scanWallet(
		db(ctx, r.pool).QueryRow(
			ctx,
			query,
			playerID,
			currency.Code(),
		),
	)
}

// Update persists the mutable financial state of a wallet, rejecting a lost update by version.
func (r *WalletRepository) Update(ctx context.Context, wallet *domain.Wallet) error {
	tx, ok := txFromContext(ctx)
	if !ok {
		return ErrTransactionRequired
	}

	const query = `
		UPDATE wallets
		SET
			balance = $2::text::numeric,
			version = $3,
			updated_at = $4
		WHERE id = $1
		  AND version = $3 - 1
	`

	result, err := tx.Exec(
		ctx,
		query,
		wallet.ID(),
		wallet.Balance().Amount(),
		wallet.Version(),
		wallet.UpdatedAt(),
	)
	if err != nil {
		return err
	}

	if result.RowsAffected() == 0 {
		return application.ErrConcurrentUpdate
	}

	return nil
}

// scanWallet rebuilds a wallet from a PostgreSQL row.
func scanWallet(row pgx.Row) (*domain.Wallet, error) {
	var (
		id        uuid.UUID
		playerID  uuid.UUID
		currency  string
		balance   string
		version   int64
		createdAt time.Time
		updatedAt time.Time
	)

	err := row.Scan(
		&id,
		&playerID,
		&currency,
		&balance,
		&version,
		&createdAt,
		&updatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, application.ErrNotFound
		}

		return nil, err
	}

	walletCurrency, err := domain.NewCurrency(currency)
	if err != nil {
		return nil, err
	}

	walletBalance, err := domain.ParseMoney(
		balance,
		walletCurrency,
	)
	if err != nil {
		return nil, err
	}

	return domain.RehydrateWallet(
		id,
		playerID,
		walletBalance,
		version,
		createdAt,
		updatedAt,
	)
}
