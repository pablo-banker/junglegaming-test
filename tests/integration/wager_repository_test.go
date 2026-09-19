//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pablo-banker/junglegaming-test/internal/application"

	"github.com/pablo-banker/junglegaming-test/internal/domain"
	"github.com/pablo-banker/junglegaming-test/internal/infrastructure/postgres"
)

var wagerRepositoryTestTime = time.Date(
	2026,
	9,
	18,
	18,
	0,
	0,
	0,
	time.UTC,
)

// mustWagerRepositoryMoney creates BRL money for wager repository tests.
func mustWagerRepositoryMoney(
	t *testing.T,
	amount string,
) domain.Money {
	t.Helper()

	currency, err := domain.NewCurrency("BRL")
	if err != nil {
		t.Fatalf("failed to create currency: %v", err)
	}

	money, err := domain.ParseMoney(amount, currency)
	if err != nil {
		t.Fatalf("failed to create money: %v", err)
	}

	return money
}

// newWagerRepositoryTransaction creates a valid external wager transaction.
func newWagerRepositoryTransaction(
	t *testing.T,
	wallet *domain.Wallet,
	transactionType domain.WagerTransactionType,
	externalTransactionID string,
	idempotencyKey string,
	referenceExternalTransactionID string,
	createdAt time.Time,
) *domain.WagerTransaction {
	t.Helper()

	amount := "25.00"
	if transactionType == domain.WagerTransactionTypeLoss {
		amount = "0.00"
	}

	transaction, err := domain.NewExternalWagerTransaction(
		domain.NewExternalWagerTransactionParams{
			ID:                             uuid.New(),
			ProviderID:                     "provider-a",
			ExternalTransactionID:          externalTransactionID,
			IdempotencyKey:                 idempotencyKey,
			PayloadHash:                    "hash-" + uuid.NewString(),
			WalletID:                       wallet.ID(),
			PlayerID:                       wallet.PlayerID(),
			RoundID:                        "round-123",
			GameID:                         "game-123",
			Type:                           transactionType,
			Amount:                         mustWagerRepositoryMoney(t, amount),
			ReferenceExternalTransactionID: referenceExternalTransactionID,
			CreatedAt:                      createdAt,
		},
	)
	if err != nil {
		t.Fatalf("failed to create wager transaction: %v", err)
	}

	return transaction
}

// createWagerRepositoryWallet persists a wallet required by wager foreign keys.
func createWagerRepositoryWallet(
	t *testing.T,
	ctx context.Context,
	repository *postgres.WalletRepository,
) *domain.Wallet {
	t.Helper()

	wallet := newWalletRepositoryWallet(
		t,
		uuid.New(),
		"100.00",
	)

	if err := repository.Create(ctx, wallet); err != nil {
		t.Fatalf("failed to create test wallet: %v", err)
	}

	return wallet
}

// cleanupWagerRepositoryWallet removes wager test data.
func cleanupWagerRepositoryWallet(t *testing.T, pool *pgxpool.Pool, walletID uuid.UUID) {
	t.Helper()

	t.Cleanup(func() {
		ctx := context.Background()

		_, _ = pool.Exec(
			ctx,
			`DELETE FROM wager_transactions WHERE wallet_id = $1`,
			walletID,
		)

		_, _ = pool.Exec(
			ctx,
			`DELETE FROM wallets WHERE id = $1`,
			walletID,
		)
	})
}

// TestWagerRepositoryCreateAndFind verifies wager persistence and lookup.
func TestWagerRepositoryCreateAndFind(t *testing.T) {
	ctx := integrationContext(t)
	pool := openIntegrationPool(t)

	walletRepository := postgres.NewWalletRepository(pool)
	wagerRepository := postgres.NewWagerRepository(pool)

	wallet := createWagerRepositoryWallet(
		t,
		ctx,
		walletRepository,
	)

	cleanupWagerRepositoryWallet(
		t,
		pool,
		wallet.ID(),
	)

	transaction := newWagerRepositoryTransaction(
		t,
		wallet,
		domain.WagerTransactionTypeBet,
		"bet-123",
		"key-123",
		"",
		wagerRepositoryTestTime,
	)

	if err := wagerRepository.Create(
		ctx,
		transaction,
	); err != nil {
		t.Fatalf("failed to create wager: %v", err)
	}

	foundByID, err := wagerRepository.FindByID(
		ctx,
		transaction.ID(),
	)
	if err != nil {
		t.Fatalf("failed to find wager by id: %v", err)
	}

	if foundByID.ID() != transaction.ID() {
		t.Errorf(
			"expected transaction id %s, got %s",
			transaction.ID(),
			foundByID.ID(),
		)
	}

	if foundByID.Status() != domain.WagerTransactionStatusPending {
		t.Errorf(
			"expected PENDING, got %s",
			foundByID.Status(),
		)
	}

	foundByExternalID, err :=
		wagerRepository.FindByProviderAndExternalTransactionID(
			ctx,
			"provider-a",
			"bet-123",
		)
	if err != nil {
		t.Fatalf(
			"failed to find wager by external transaction id: %v",
			err,
		)
	}

	if foundByExternalID.ID() != transaction.ID() {
		t.Error("expected external transaction lookup to return original wager")
	}

	foundByIdempotencyKey, err :=
		wagerRepository.FindByProviderAndIdempotencyKey(
			ctx,
			"provider-a",
			"key-123",
		)
	if err != nil {
		t.Fatalf(
			"failed to find wager by idempotency key: %v",
			err,
		)
	}

	if foundByIdempotencyKey.ID() != transaction.ID() {
		t.Error("expected idempotency lookup to return original wager")
	}
}

// TestWagerRepositoryRejectsDuplicateIdempotencyKey verifies persisted idempotency uniqueness.
func TestWagerRepositoryRejectsDuplicateIdempotencyKey(t *testing.T) {
	ctx := integrationContext(t)
	pool := openIntegrationPool(t)

	walletRepository := postgres.NewWalletRepository(pool)
	wagerRepository := postgres.NewWagerRepository(pool)

	wallet := createWagerRepositoryWallet(
		t,
		ctx,
		walletRepository,
	)

	cleanupWagerRepositoryWallet(
		t,
		pool,
		wallet.ID(),
	)

	first := newWagerRepositoryTransaction(
		t,
		wallet,
		domain.WagerTransactionTypeBet,
		"bet-1",
		"same-key",
		"",
		wagerRepositoryTestTime,
	)

	second := newWagerRepositoryTransaction(
		t,
		wallet,
		domain.WagerTransactionTypeBet,
		"bet-2",
		"same-key",
		"",
		wagerRepositoryTestTime.Add(time.Second),
	)

	if err := wagerRepository.Create(ctx, first); err != nil {
		t.Fatalf("failed to create first wager: %v", err)
	}

	err := wagerRepository.Create(ctx, second)

	if !errors.Is(err, application.ErrAlreadyExists) {
		t.Fatalf("expected ErrAlreadyExists, got %v", err)
	}

	found, err := wagerRepository.FindByProviderAndIdempotencyKey(ctx, "provider-a", "same-key")
	if err != nil {
		t.Fatalf("failed to find original wager: %v", err)
	}

	if found.ID() != first.ID() {
		t.Error("expected original wager to remain persisted")
	}
}

// TestWagerRepositoryRejectsDuplicateExternalTransactionID verifies provider transaction uniqueness.
func TestWagerRepositoryRejectsDuplicateExternalTransactionID(t *testing.T) {
	ctx := integrationContext(t)
	pool := openIntegrationPool(t)

	walletRepository := postgres.NewWalletRepository(pool)
	wagerRepository := postgres.NewWagerRepository(pool)

	wallet := createWagerRepositoryWallet(t, ctx, walletRepository)
	cleanupWagerRepositoryWallet(t, pool, wallet.ID())

	first := newWagerRepositoryTransaction(
		t,
		wallet,
		domain.WagerTransactionTypeBet,
		"same-external-id",
		"key-1",
		"",
		wagerRepositoryTestTime,
	)

	second := newWagerRepositoryTransaction(
		t,
		wallet,
		domain.WagerTransactionTypeBet,
		"same-external-id",
		"key-2",
		"",
		wagerRepositoryTestTime.Add(time.Second),
	)

	if err := wagerRepository.Create(ctx, first); err != nil {
		t.Fatalf("failed to create first wager: %v", err)
	}

	err := wagerRepository.Create(ctx, second)

	if !errors.Is(err, application.ErrAlreadyExists) {
		t.Fatalf(
			"expected ErrAlreadyExists, got %v",
			err,
		)
	}

	found, err := wagerRepository.FindByProviderAndExternalTransactionID(ctx, "provider-a", "same-external-id")
	if err != nil {
		t.Fatalf("failed to find original wager: %v", err)
	}

	if found.ID() != first.ID() {
		t.Error("expected original external transaction to remain persisted")
	}
}

// TestWagerRepositoryPersistsPendingReference verifies durable reference retry scheduling.
func TestWagerRepositoryPersistsPendingReference(t *testing.T) {
	ctx := integrationContext(t)
	pool := openIntegrationPool(t)

	walletRepository := postgres.NewWalletRepository(pool)
	wagerRepository := postgres.NewWagerRepository(pool)

	wallet := createWagerRepositoryWallet(
		t,
		ctx,
		walletRepository,
	)

	cleanupWagerRepositoryWallet(
		t,
		pool,
		wallet.ID(),
	)

	transaction := newWagerRepositoryTransaction(
		t,
		wallet,
		domain.WagerTransactionTypeRefund,
		"refund-123",
		"refund-key-123",
		"missing-bet-123",
		wagerRepositoryTestTime,
	)

	if err := wagerRepository.Create(
		ctx,
		transaction,
	); err != nil {
		t.Fatalf("failed to create refund: %v", err)
	}

	updatedAt := wagerRepositoryTestTime.Add(time.Second)

	if err := transaction.MarkPendingReference(
		updatedAt,
	); err != nil {
		t.Fatalf(
			"failed to mark pending reference: %v",
			err,
		)
	}

	nextAttemptAt := updatedAt.Add(5 * time.Second)
	expiresAt := updatedAt.Add(24 * time.Hour)

	if err := wagerRepository.UpdatePendingReference(
		ctx,
		transaction,
		nextAttemptAt,
		expiresAt,
		application.CommandMetadata{CorrelationID: uuid.NewString()},
	); err != nil {
		t.Fatalf(
			"failed to persist pending reference: %v",
			err,
		)
	}

	var (
		status            string
		referenceAttempts int
		storedNextAttempt time.Time
		storedExpiresAt   time.Time
	)

	err := pool.QueryRow(
		ctx,
		`
			SELECT
				status::text,
				reference_retry_count,
				next_reference_attempt_at,
				reference_expires_at
			FROM wager_transactions
			WHERE id = $1
		`,
		transaction.ID(),
	).Scan(
		&status,
		&referenceAttempts,
		&storedNextAttempt,
		&storedExpiresAt,
	)
	if err != nil {
		t.Fatalf(
			"failed to read pending reference metadata: %v",
			err,
		)
	}

	if status != "PENDING_REFERENCE" {
		t.Errorf(
			"expected PENDING_REFERENCE, got %s",
			status,
		)
	}

	if referenceAttempts != 0 {
		t.Errorf(
			"expected 0 reference attempts, got %d",
			referenceAttempts,
		)
	}

	if !storedNextAttempt.Equal(nextAttemptAt) {
		t.Errorf(
			"expected next attempt %s, got %s",
			nextAttemptAt,
			storedNextAttempt,
		)
	}

	if !storedExpiresAt.Equal(expiresAt) {
		t.Errorf(
			"expected expiration %s, got %s",
			expiresAt,
			storedExpiresAt,
		)
	}

	found, err := wagerRepository.FindByID(
		ctx,
		transaction.ID(),
	)
	if err != nil {
		t.Fatalf(
			"failed to rehydrate pending transaction: %v",
			err,
		)
	}

	if found.Status() != domain.WagerTransactionStatusPendingReference {
		t.Errorf(
			"expected rehydrated PENDING_REFERENCE, got %s",
			found.Status(),
		)
	}

	if found.ReferenceExternalTransactionID() != "missing-bet-123" {
		t.Errorf(
			"expected missing-bet-123, got %s",
			found.ReferenceExternalTransactionID(),
		)
	}
}

// TestWagerRepositoryFindsProcessedDirectReversal verifies successful reversal detection.
func TestWagerRepositoryFindsProcessedDirectReversal(t *testing.T) {
	ctx := integrationContext(t)
	pool := openIntegrationPool(t)

	walletRepository := postgres.NewWalletRepository(pool)
	wagerRepository := postgres.NewWagerRepository(pool)

	wallet := createWagerRepositoryWallet(
		t,
		ctx,
		walletRepository,
	)

	cleanupWagerRepositoryWallet(
		t,
		pool,
		wallet.ID(),
	)

	bet := newWagerRepositoryTransaction(
		t,
		wallet,
		domain.WagerTransactionTypeBet,
		"bet-original",
		"bet-original-key",
		"",
		wagerRepositoryTestTime,
	)

	err := bet.MarkProcessed(
		mustWagerRepositoryMoney(t, "100.00"),
		mustWagerRepositoryMoney(t, "75.00"),
		wagerRepositoryTestTime.Add(time.Second),
	)
	if err != nil {
		t.Fatalf("failed to process bet: %v", err)
	}

	if err := wagerRepository.Create(ctx, bet); err != nil {
		t.Fatalf("failed to persist bet: %v", err)
	}

	exists, err := wagerRepository.HasProcessedDirectReversal(
		ctx,
		bet.ID(),
	)
	if err != nil {
		t.Fatalf(
			"failed to check direct reversal: %v",
			err,
		)
	}

	if exists {
		t.Fatal("expected bet to have no direct reversal")
	}

	refund := newWagerRepositoryTransaction(
		t,
		wallet,
		domain.WagerTransactionTypeRefund,
		"refund-original",
		"refund-original-key",
		bet.ExternalTransactionID(),
		wagerRepositoryTestTime.Add(2*time.Second),
	)

	if err := refund.ResolveReference(
		bet,
		wagerRepositoryTestTime.Add(3*time.Second),
	); err != nil {
		t.Fatalf(
			"failed to resolve bet reference: %v",
			err,
		)
	}

	if err := refund.MarkProcessed(
		mustWagerRepositoryMoney(t, "75.00"),
		mustWagerRepositoryMoney(t, "100.00"),
		wagerRepositoryTestTime.Add(4*time.Second),
	); err != nil {
		t.Fatalf(
			"failed to process refund: %v",
			err,
		)
	}

	if err := wagerRepository.Create(ctx, refund); err != nil {
		t.Fatalf("failed to persist refund: %v", err)
	}

	exists, err = wagerRepository.HasProcessedDirectReversal(
		ctx,
		bet.ID(),
	)
	if err != nil {
		t.Fatalf(
			"failed to check direct reversal: %v",
			err,
		)
	}

	if !exists {
		t.Fatal("expected bet to have a processed direct reversal")
	}
}
