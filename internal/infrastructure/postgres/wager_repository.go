package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pablo-banker/junglegaming-test/internal/application"
	"github.com/pablo-banker/junglegaming-test/internal/domain"
)

const wagerSelect = `
	SELECT
		id,
		provider_id,
		external_transaction_id,
		idempotency_key,
		payload_hash,
		wallet_id,
		player_id,
		round_id,
		game_id,
		type::text,
		amount::text,
		currency,
		reference_external_transaction_id,
		reference_transaction_id,
		reference_transaction_type::text,
		status::text,
		failure_code,
		failure_message,
		balance_before::text,
		balance_after::text,
		created_at,
		updated_at,
		completed_at
	FROM wager_transactions
`

type WagerRepository struct {
	pool *pgxpool.Pool
}

var _ application.WagerRepository = (*WagerRepository)(nil)

// NewWagerRepository creates a PostgreSQL wager repository.
func NewWagerRepository(pool *pgxpool.Pool) *WagerRepository {
	return &WagerRepository{
		pool: pool,
	}
}

// Create persists a new wager transaction.
func (r *WagerRepository) Create(ctx context.Context, transaction *domain.WagerTransaction) error {
	const query = `
		INSERT INTO wager_transactions (
			id,
			provider_id,
			external_transaction_id,
			idempotency_key,
			payload_hash,
			wallet_id,
			player_id,
			round_id,
			game_id,
			type,
			amount,
			currency,
			reference_external_transaction_id,
			reference_transaction_id,
			reference_transaction_type,
			status,
			failure_code,
			failure_message,
			balance_before,
			balance_after,
			created_at,
			updated_at,
			completed_at
		)
		VALUES (
			$1,
			$2,
			$3,
			$4,
			$5,
			$6,
			$7,
			$8,
			$9,
			$10::wager_transaction_type,
			$11::text::numeric,
			$12,
			$13,
			$14,
			$15::wager_transaction_type,
			$16::wager_transaction_status,
			$17,
			$18,
			$19::text::numeric,
			$20::text::numeric,
			$21,
			$22,
			$23
		)
		ON CONFLICT DO NOTHING
	`

	result, err := db(ctx, r.pool).Exec(
		ctx,
		query,
		transaction.ID(),
		nullableString(transaction.ProviderID()),
		nullableString(transaction.ExternalTransactionID()),
		nullableString(transaction.IdempotencyKey()),
		nullableString(transaction.PayloadHash()),
		transaction.WalletID(),
		transaction.PlayerID(),
		nullableString(transaction.RoundID()),
		nullableString(transaction.GameID()),
		transaction.Type(),
		transaction.Amount().Amount(),
		transaction.Amount().Currency().Code(),
		nullableString(transaction.ReferenceExternalTransactionID()),
		nullableUUID(transaction.ReferenceTransactionID()),
		nullableWagerType(transaction.ReferenceTransactionType()),
		transaction.Status(),
		nullableString(transaction.FailureCode()),
		nullableString(transaction.FailureMessage()),
		wagerBalanceBefore(transaction),
		wagerBalanceAfter(transaction),
		transaction.CreatedAt(),
		transaction.UpdatedAt(),
		wagerCompletedAt(transaction),
	)
	if err != nil {
		return mapWagerCreateError(err)
	}

	if result.RowsAffected() == 0 {
		return application.ErrAlreadyExists
	}

	return nil
}

// Update persists the mutable state of a wager transaction.
func (r *WagerRepository) Update(ctx context.Context, transaction *domain.WagerTransaction) error {
	const query = `
		UPDATE wager_transactions
		SET
			reference_transaction_id = $2,
			reference_transaction_type = $3::wager_transaction_type,
			status = $4::wager_transaction_status,
			failure_code = $5,
			failure_message = $6,
			balance_before = $7::text::numeric,
			balance_after = $8::text::numeric,
			reference_retry_count = 0,
			next_reference_attempt_at = NULL,
			reference_expires_at = NULL,
			reference_correlation_id = NULL,
			reference_causation_id = NULL,
			updated_at = $9,
			completed_at = $10
		WHERE id = $1
	`

	result, err := db(ctx, r.pool).Exec(
		ctx,
		query,
		transaction.ID(),
		nullableUUID(transaction.ReferenceTransactionID()),
		nullableWagerType(transaction.ReferenceTransactionType()),
		transaction.Status(),
		nullableString(transaction.FailureCode()),
		nullableString(transaction.FailureMessage()),
		wagerBalanceBefore(transaction),
		wagerBalanceAfter(transaction),
		transaction.UpdatedAt(),
		wagerCompletedAt(transaction),
	)
	if err != nil {
		return err
	}

	if result.RowsAffected() == 0 {
		return application.ErrNotFound
	}

	return nil
}

// UpdatePendingReference persists reference retry scheduling information.
func (r *WagerRepository) UpdatePendingReference(
	ctx context.Context,
	transaction *domain.WagerTransaction,
	nextAttemptAt time.Time,
	expiresAt time.Time,
	metadata application.CommandMetadata,
) error {
	const query = `
		UPDATE wager_transactions
		SET
			status = $2::wager_transaction_status,
			updated_at = $3,
			reference_retry_count = 0,
			next_reference_attempt_at = $4,
			reference_expires_at = $5,
			reference_correlation_id = $6,
			reference_causation_id = $7
		WHERE id = $1
	`

	result, err := db(ctx, r.pool).Exec(
		ctx,
		query,
		transaction.ID(),
		transaction.Status(),
		transaction.UpdatedAt(),
		nextAttemptAt,
		expiresAt,
		metadata.CorrelationID,
		nullableString(metadata.CausationID),
	)
	if err != nil {
		return err
	}

	if result.RowsAffected() == 0 {
		return application.ErrNotFound
	}

	return nil
}

// FindByID returns a wager transaction by its internal identifier.
func (r *WagerRepository) FindByID(ctx context.Context, id uuid.UUID) (*domain.WagerTransaction, error) {
	query := wagerSelect + `
		WHERE id = $1
	`

	return scanWager(
		db(ctx, r.pool).QueryRow(ctx, query, id),
	)
}

// FindByProviderAndExternalTransactionID returns a provider transaction by external id.
func (r *WagerRepository) FindByProviderAndExternalTransactionID(ctx context.Context, providerID string, externalTransactionID string) (*domain.WagerTransaction, error) {
	query := wagerSelect + `
		WHERE provider_id = $1
		  AND external_transaction_id = $2
	`

	return scanWager(
		db(ctx, r.pool).QueryRow(
			ctx,
			query,
			providerID,
			externalTransactionID,
		),
	)
}

// FindByProviderAndIdempotencyKey returns a transaction by its idempotency key.
func (r *WagerRepository) FindByProviderAndIdempotencyKey(ctx context.Context, providerID string, idempotencyKey string) (*domain.WagerTransaction, error) {
	query := wagerSelect + `
		WHERE provider_id = $1
		  AND idempotency_key = $2
	`

	return scanWager(
		db(ctx, r.pool).QueryRow(
			ctx,
			query,
			providerID,
			idempotencyKey,
		),
	)
}

// HasProcessedDirectReversal reports whether a transaction was already reversed directly.
func (r *WagerRepository) HasProcessedDirectReversal(ctx context.Context, referenceTransactionID uuid.UUID) (bool, error) {
	const query = `
		SELECT EXISTS (
			SELECT 1
			FROM wager_transactions
			WHERE reference_transaction_id = $1
			  AND status = 'PROCESSED'
			  AND type IN ('REFUND', 'ROLLBACK')
		)
	`

	var exists bool

	err := db(ctx, r.pool).QueryRow(ctx, query, referenceTransactionID).
		Scan(&exists)
	if err != nil {
		return false, err
	}

	return exists, nil
}

// FindDuePendingReferenceForUpdate locks one pending reference ready for retry.
func (r *WagerRepository) FindDuePendingReferenceForUpdate(
	ctx context.Context,
	now time.Time,
) (*application.PendingReferenceWork, error) {
	const query = `
		SELECT
			id,
			reference_retry_count,
			reference_expires_at,
			reference_correlation_id,
			reference_causation_id
		FROM wager_transactions
		WHERE status = 'PENDING_REFERENCE'
		  AND next_reference_attempt_at <= $1
		ORDER BY next_reference_attempt_at, created_at
		FOR UPDATE SKIP LOCKED
		LIMIT 1
	`

	return r.scanPendingReferenceWork(ctx, db(ctx, r.pool).QueryRow(ctx, query, now))
}

// FindPendingReferenceForUpdate locks a transaction that is still waiting for its reference.
func (r *WagerRepository) FindPendingReferenceForUpdate(
	ctx context.Context,
	transactionID uuid.UUID,
) (*application.PendingReferenceWork, error) {
	const query = `
		SELECT
			id,
			reference_retry_count,
			reference_expires_at,
			reference_correlation_id,
			reference_causation_id
		FROM wager_transactions
		WHERE id = $1
		  AND status = 'PENDING_REFERENCE'
		FOR UPDATE
	`

	return r.scanPendingReferenceWork(ctx, db(ctx, r.pool).QueryRow(ctx, query, transactionID))
}

// scanPendingReferenceWork loads the locked transaction and its retry state.
func (r *WagerRepository) scanPendingReferenceWork(ctx context.Context, row pgx.Row) (*application.PendingReferenceWork, error) {
	var (
		transactionID uuid.UUID
		retryCount    int
		expiresAt     time.Time
		correlationID *string
		causationID   *string
	)

	err := row.Scan(
		&transactionID,
		&retryCount,
		&expiresAt,
		&correlationID,
		&causationID,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, application.ErrNotFound
		}

		return nil, err
	}

	transaction, err := r.FindByID(ctx, transactionID)
	if err != nil {
		return nil, err
	}

	return &application.PendingReferenceWork{
		Transaction: transaction,
		RetryCount:  retryCount,
		ExpiresAt:   expiresAt,
		Metadata: application.CommandMetadata{
			CorrelationID: stringValue(correlationID),
			CausationID:   stringValue(causationID),
		},
	}, nil
}

// SchedulePendingReferenceRetry schedules the next durable reference retry.
func (r *WagerRepository) SchedulePendingReferenceRetry(
	ctx context.Context,
	transactionID uuid.UUID,
	retryCount int,
	nextAttemptAt time.Time,
) error {
	const query = `
		UPDATE wager_transactions
		SET
			reference_retry_count = $2,
			next_reference_attempt_at = $3,
			updated_at = clock_timestamp()
		WHERE id = $1
		  AND status = 'PENDING_REFERENCE'
	`

	result, err := db(ctx, r.pool).Exec(
		ctx,
		query,
		transactionID,
		retryCount,
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

// mapWagerCreateError translates known database constraints into application errors.
func mapWagerCreateError(err error) error {
	var pgErr *pgconn.PgError

	if !errors.As(err, &pgErr) {
		return err
	}

	if pgErr.Code == "23503" && pgErr.ConstraintName == "fk_wager_transactions_wallet_currency" {
		return application.ErrInvalidWalletReference
	}

	return err
}

// scanWager rebuilds a wager transaction from a PostgreSQL row.
func scanWager(row pgx.Row) (*domain.WagerTransaction, error) {
	var (
		id                    uuid.UUID
		providerID            *string
		externalTransactionID *string
		idempotencyKey        *string
		payloadHash           *string

		walletID uuid.UUID
		playerID uuid.UUID

		roundID *string
		gameID  *string

		transactionType string
		amount          string
		currencyCode    string

		referenceExternalTransactionID *string
		referenceTransactionID         *uuid.UUID
		referenceTransactionType       *string

		status string

		failureCode    *string
		failureMessage *string

		balanceBefore *string
		balanceAfter  *string

		createdAt   time.Time
		updatedAt   time.Time
		completedAt *time.Time
	)

	err := row.Scan(
		&id,
		&providerID,
		&externalTransactionID,
		&idempotencyKey,
		&payloadHash,
		&walletID,
		&playerID,
		&roundID,
		&gameID,
		&transactionType,
		&amount,
		&currencyCode,
		&referenceExternalTransactionID,
		&referenceTransactionID,
		&referenceTransactionType,
		&status,
		&failureCode,
		&failureMessage,
		&balanceBefore,
		&balanceAfter,
		&createdAt,
		&updatedAt,
		&completedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, application.ErrNotFound
		}

		return nil, err
	}

	currency, err := domain.NewCurrency(currencyCode)
	if err != nil {
		return nil, err
	}

	transactionAmount, err := domain.ParseMoney(amount, currency)
	if err != nil {
		return nil, err
	}

	before, err := nullableMoney(balanceBefore, currency)
	if err != nil {
		return nil, err
	}

	after, err := nullableMoney(balanceAfter, currency)
	if err != nil {
		return nil, err
	}

	return domain.RehydrateWagerTransaction(
		domain.RehydrateWagerTransactionParams{
			ID:                             id,
			ProviderID:                     stringValue(providerID),
			ExternalTransactionID:          stringValue(externalTransactionID),
			IdempotencyKey:                 stringValue(idempotencyKey),
			PayloadHash:                    stringValue(payloadHash),
			WalletID:                       walletID,
			PlayerID:                       playerID,
			RoundID:                        stringValue(roundID),
			GameID:                         stringValue(gameID),
			Type:                           domain.WagerTransactionType(transactionType),
			Amount:                         transactionAmount,
			ReferenceExternalTransactionID: stringValue(referenceExternalTransactionID),
			ReferenceTransactionID:         uuidValue(referenceTransactionID),
			ReferenceTransactionType: domain.WagerTransactionType(
				stringValue(referenceTransactionType),
			),
			Status:         domain.WagerTransactionStatus(status),
			FailureCode:    stringValue(failureCode),
			FailureMessage: stringValue(failureMessage),
			BalanceBefore:  before,
			BalanceAfter:   after,
			CreatedAt:      createdAt,
			UpdatedAt:      updatedAt,
			CompletedAt:    completedAt,
		},
	)
}

// nullableMoney rebuilds optional persisted money.
func nullableMoney(amount *string, currency domain.Currency) (*domain.Money, error) {
	if amount == nil {
		return nil, nil
	}

	money, err := domain.ParseMoney(*amount, currency)
	if err != nil {
		return nil, err
	}

	return &money, nil
}

// nullableString converts an empty domain string into SQL NULL.
func nullableString(value string) any {
	if value == "" {
		return nil
	}

	return value
}

// nullableUUID converts a nil UUID into SQL NULL.
func nullableUUID(value uuid.UUID) any {
	if value == uuid.Nil {
		return nil
	}

	return value
}

// nullableWagerType converts an empty wager type into SQL NULL.
func nullableWagerType(value domain.WagerTransactionType) any {
	if value == "" {
		return nil
	}

	return value
}

// wagerBalanceBefore returns the persisted balance before value.
func wagerBalanceBefore(transaction *domain.WagerTransaction) any {
	value, ok := transaction.BalanceBefore()
	if !ok {
		return nil
	}

	return value.Amount()
}

// wagerBalanceAfter returns the persisted balance after value.
func wagerBalanceAfter(transaction *domain.WagerTransaction) any {
	value, ok := transaction.BalanceAfter()
	if !ok {
		return nil
	}

	return value.Amount()
}

// wagerCompletedAt returns the persisted completion timestamp.
func wagerCompletedAt(transaction *domain.WagerTransaction) any {
	value, ok := transaction.CompletedAt()
	if !ok {
		return nil
	}

	return value
}

// stringValue converts an optional database string into its domain value.
func stringValue(value *string) string {
	if value == nil {
		return ""
	}

	return *value
}

// uuidValue converts an optional database UUID into its domain value.
func uuidValue(value *uuid.UUID) uuid.UUID {
	if value == nil {
		return uuid.Nil
	}

	return *value
}
