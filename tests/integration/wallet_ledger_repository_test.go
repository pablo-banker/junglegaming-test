//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pablo-banker/junglegaming-test/internal/domain"
	"github.com/pablo-banker/junglegaming-test/internal/infrastructure/postgres"
)

// createLedgerFixture creates and persists a wallet, opening wager and ledger entry.
func createLedgerFixture(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
) (*domain.Wallet, *domain.WagerTransaction, *domain.WalletLedgerEntry) {
	t.Helper()

	txManager := postgres.NewTransactionManager(pool)
	walletRepository := postgres.NewWalletRepository(pool)
	wagerRepository := postgres.NewWagerRepository(pool)
	ledgerRepository := postgres.NewWalletLedgerRepository(pool)

	wallet := newWalletRepositoryWallet(
		t,
		uuid.New(),
		"100.00",
	)

	opening, err := domain.NewOpeningWagerTransaction(
		domain.NewOpeningWagerTransactionParams{
			ID:        uuid.New(),
			WalletID:  wallet.ID(),
			PlayerID:  wallet.PlayerID(),
			Amount:    mustWagerRepositoryMoney(t, "100.00"),
			CreatedAt: wallet.CreatedAt(),
		},
	)
	if err != nil {
		t.Fatalf("failed to create opening wager: %v", err)
	}

	zero, err := domain.Zero(wallet.Currency())
	if err != nil {
		t.Fatalf("failed to create zero money: %v", err)
	}

	entry, err := domain.NewWalletLedgerEntry(
		uuid.New(),
		wallet.ID(),
		opening.ID(),
		domain.WalletLedgerDirectionCredit,
		mustWagerRepositoryMoney(t, "100.00"),
		zero,
		mustWagerRepositoryMoney(t, "100.00"),
		wallet.CreatedAt(),
	)
	if err != nil {
		t.Fatalf("failed to create ledger entry: %v", err)
	}

	err = txManager.WithinTransaction(
		ctx,
		func(txCtx context.Context) error {
			if err := walletRepository.Create(txCtx, wallet); err != nil {
				return err
			}

			if err := wagerRepository.Create(txCtx, opening); err != nil {
				return err
			}

			return ledgerRepository.Create(txCtx, entry)
		},
	)
	if err != nil {
		t.Fatalf("failed to persist ledger fixture: %v", err)
	}

	return wallet, opening, entry
}

// postgresErrorCode returns the PostgreSQL error code when available.
func postgresErrorCode(err error) string {
	var pgErr *pgconn.PgError

	if errors.As(err, &pgErr) {
		return pgErr.Code
	}

	return ""
}

// TestWalletLedgerRepositoryCreatesEntry verifies ledger persistence.
func TestWalletLedgerRepositoryCreatesEntry(t *testing.T) {
	ctx := integrationContext(t)
	pool := openIntegrationPool(t)

	wallet, _, entry := createLedgerFixture(
		t,
		ctx,
		pool,
	)

	var (
		direction     string
		amount        string
		balanceBefore string
		balanceAfter  string
	)

	err := pool.QueryRow(
		ctx,
		`
			SELECT
				direction::text,
				amount::text,
				balance_before::text,
				balance_after::text
			FROM wallet_ledger_entries
			WHERE id = $1
		`,
		entry.ID(),
	).Scan(
		&direction,
		&amount,
		&balanceBefore,
		&balanceAfter,
	)
	if err != nil {
		t.Fatalf("failed to read ledger entry: %v", err)
	}

	if direction != "CREDIT" {
		t.Errorf("expected CREDIT, got %s", direction)
	}

	if amount != "100.00" {
		t.Errorf("expected amount 100.00, got %s", amount)
	}

	if balanceBefore != "0.00" {
		t.Errorf("expected balance before 0.00, got %s", balanceBefore)
	}

	if balanceAfter != "100.00" {
		t.Errorf("expected balance after 100.00, got %s", balanceAfter)
	}

	if wallet.Version() != 1 {
		t.Errorf(
			"expected opening wallet version 1, got %d",
			wallet.Version(),
		)
	}
}

// TestWalletLedgerRepositoryRejectsDuplicateTransaction verifies one ledger movement per wallet transaction.
func TestWalletLedgerRepositoryRejectsDuplicateTransaction(t *testing.T) {
	ctx := integrationContext(t)
	pool := openIntegrationPool(t)

	_, _, entry := createLedgerFixture(
		t,
		ctx,
		pool,
	)

	txManager := postgres.NewTransactionManager(pool)
	repository := postgres.NewWalletLedgerRepository(pool)

	duplicate, err := domain.NewWalletLedgerEntry(
		uuid.New(),
		entry.WalletID(),
		entry.TransactionID(),
		entry.Direction(),
		entry.Amount(),
		entry.BalanceBefore(),
		entry.BalanceAfter(),
		entry.CreatedAt(),
	)
	if err != nil {
		t.Fatalf("failed to create duplicate entry: %v", err)
	}

	err = txManager.WithinTransaction(
		ctx,
		func(txCtx context.Context) error {
			return repository.Create(txCtx, duplicate)
		},
	)

	if postgresErrorCode(err) != "23505" {
		t.Fatalf(
			"expected PostgreSQL unique violation 23505, got %v",
			err,
		)
	}
}

// TestWalletLedgerDatabaseRejectsInvalidMath verifies the database protects ledger arithmetic.
func TestWalletLedgerDatabaseRejectsInvalidMath(t *testing.T) {
	ctx := integrationContext(t)
	pool := openIntegrationPool(t)

	wallet, _, _ := createLedgerFixture(
		t,
		ctx,
		pool,
	)

	wagerRepository := postgres.NewWagerRepository(pool)

	bet := newWagerRepositoryTransaction(
		t,
		wallet,
		domain.WagerTransactionTypeBet,
		"ledger-invalid-bet-"+uuid.NewString(),
		"ledger-invalid-key-"+uuid.NewString(),
		"",
		time.Now().UTC(),
	)

	if err := wagerRepository.Create(ctx, bet); err != nil {
		t.Fatalf("failed to create wager for invalid ledger test: %v", err)
	}

	_, err := pool.Exec(
		ctx,
		`
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
				'DEBIT',
				25.00,
				'BRL',
				100.00,
				90.00,
				NOW()
			)
		`,
		uuid.New(),
		wallet.ID(),
		bet.ID(),
	)

	if postgresErrorCode(err) != "23514" {
		t.Fatalf(
			"expected PostgreSQL check violation 23514, got %v",
			err,
		)
	}
}

// TestWalletLedgerDatabaseBlocksUpdate verifies ledger entries are append-only.
func TestWalletLedgerDatabaseBlocksUpdate(t *testing.T) {
	ctx := integrationContext(t)
	pool := openIntegrationPool(t)

	_, _, entry := createLedgerFixture(
		t,
		ctx,
		pool,
	)

	_, err := pool.Exec(
		ctx,
		`
			UPDATE wallet_ledger_entries
			SET amount = 200.00
			WHERE id = $1
		`,
		entry.ID(),
	)

	if err == nil {
		t.Fatal("expected ledger UPDATE to be rejected")
	}

	var amount string

	err = pool.QueryRow(
		ctx,
		`
			SELECT amount::text
			FROM wallet_ledger_entries
			WHERE id = $1
		`,
		entry.ID(),
	).Scan(&amount)
	if err != nil {
		t.Fatalf("failed to read ledger after rejected update: %v", err)
	}

	if amount != "100.00" {
		t.Errorf(
			"expected amount to remain 100.00, got %s",
			amount,
		)
	}
}

// TestWalletLedgerDatabaseBlocksDelete verifies ledger history cannot be deleted.
func TestWalletLedgerDatabaseBlocksDelete(t *testing.T) {
	ctx := integrationContext(t)
	pool := openIntegrationPool(t)

	_, _, entry := createLedgerFixture(
		t,
		ctx,
		pool,
	)

	_, err := pool.Exec(
		ctx,
		`
			DELETE FROM wallet_ledger_entries
			WHERE id = $1
		`,
		entry.ID(),
	)

	if err == nil {
		t.Fatal("expected ledger DELETE to be rejected")
	}

	var exists bool

	err = pool.QueryRow(
		ctx,
		`
			SELECT EXISTS (
				SELECT 1
				FROM wallet_ledger_entries
				WHERE id = $1
			)
		`,
		entry.ID(),
	).Scan(&exists)
	if err != nil {
		t.Fatalf("failed to check ledger after rejected delete: %v", err)
	}

	if !exists {
		t.Fatal("expected ledger entry to remain persisted")
	}
}
