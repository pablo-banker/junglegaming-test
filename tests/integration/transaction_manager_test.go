package integration

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/pablo-banker/junglegaming-test/internal/domain"
	"github.com/pablo-banker/junglegaming-test/internal/infrastructure/postgres"
)

type queryRower interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// newIntegrationWallet creates a wallet for PostgreSQL integration tests.
func newIntegrationWallet(t *testing.T) *domain.Wallet {
	t.Helper()

	currency, err := domain.NewCurrency("BRL")
	if err != nil {
		t.Fatalf("failed to create currency: %v", err)
	}

	balance, err := domain.ParseMoney("100.00", currency)
	if err != nil {
		t.Fatalf("failed to create money: %v", err)
	}

	wallet, err := domain.NewWallet(uuid.New(), uuid.New(), balance, time.Now().UTC())
	if err != nil {
		t.Fatalf("failed to create wallet: %v", err)
	}

	return wallet
}

// walletExists reports whether a wallet exists in PostgreSQL.
func walletExists(t *testing.T, ctx context.Context, pool queryRower, walletID uuid.UUID) bool {
	t.Helper()

	var exists bool

	err := pool.QueryRow(ctx,
		`
			SELECT EXISTS (
				SELECT 1
				FROM wallets
				WHERE id = $1
			)
		`,
		walletID,
	).Scan(&exists)
	if err != nil {
		t.Fatalf("failed to check wallet existence: %v", err)
	}

	return exists
}

// TestTransactionManagerCommits verifies successful transactions are committed.
func TestTransactionManagerCommits(t *testing.T) {
	ctx := integrationContext(t)
	pool := openIntegrationPool(t)

	txManager := postgres.NewTransactionManager(pool)
	wallets := postgres.NewWalletRepository(pool)

	wallet := newIntegrationWallet(t)

	t.Cleanup(func() {
		_, _ = pool.Exec(
			context.Background(),
			`DELETE FROM wallets WHERE id = $1`,
			wallet.ID(),
		)
	})

	err := txManager.WithinTransaction(
		ctx,
		func(txCtx context.Context) error {
			return wallets.Create(txCtx, wallet)
		},
	)
	if err != nil {
		t.Fatalf("unexpected transaction error: %v", err)
	}

	if !walletExists(t, ctx, pool, wallet.ID()) {
		t.Fatal("expected wallet to be committed")
	}
}

// TestTransactionManagerRollsBack verifies errors rollback the transaction.
func TestTransactionManagerRollsBack(t *testing.T) {
	ctx := integrationContext(t)
	pool := openIntegrationPool(t)

	txManager := postgres.NewTransactionManager(pool)
	wallets := postgres.NewWalletRepository(pool)

	wallet := newIntegrationWallet(t)
	expectedErr := errors.New("force rollback")

	err := txManager.WithinTransaction(
		ctx,
		func(txCtx context.Context) error {
			if err := wallets.Create(txCtx, wallet); err != nil {
				return err
			}

			return expectedErr
		},
	)

	if !errors.Is(err, expectedErr) {
		t.Fatalf("expected %v, got %v", expectedErr, err)
	}

	if walletExists(t, ctx, pool, wallet.ID()) {
		t.Fatal("expected wallet creation to be rolled back")
	}
}

// TestTransactionManagerReusesExistingTransaction verifies nested operations share the outer transaction.
func TestTransactionManagerReusesExistingTransaction(t *testing.T) {
	ctx := integrationContext(t)
	pool := openIntegrationPool(t)

	txManager := postgres.NewTransactionManager(pool)
	wallets := postgres.NewWalletRepository(pool)

	wallet := newIntegrationWallet(t)
	expectedErr := errors.New("rollback outer transaction")

	err := txManager.WithinTransaction(
		ctx,
		func(outerCtx context.Context) error {
			err := txManager.WithinTransaction(
				outerCtx,
				func(innerCtx context.Context) error {
					return wallets.Create(innerCtx, wallet)
				},
			)
			if err != nil {
				return err
			}

			return expectedErr
		},
	)

	if !errors.Is(err, expectedErr) {
		t.Fatalf("expected %v, got %v", expectedErr, err)
	}

	if walletExists(t, ctx, pool, wallet.ID()) {
		t.Fatal("expected nested operation to rollback with outer transaction")
	}
}
