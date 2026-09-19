//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/pablo-banker/junglegaming-test/internal/application"
	"github.com/pablo-banker/junglegaming-test/internal/domain"
	"github.com/pablo-banker/junglegaming-test/internal/infrastructure/postgres"
)

// newWalletRepositoryWallet creates a wallet for repository integration tests.
func newWalletRepositoryWallet(t *testing.T, playerID uuid.UUID, balance string) *domain.Wallet {
	t.Helper()

	currency, err := domain.NewCurrency("BRL")
	if err != nil {
		t.Fatalf("failed to create currency: %v", err)
	}

	money, err := domain.ParseMoney(balance, currency)
	if err != nil {
		t.Fatalf("failed to create money: %v", err)
	}

	wallet, err := domain.NewWallet(
		uuid.New(),
		playerID,
		money,
		time.Date(2026, 9, 18, 17, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatalf("failed to create wallet: %v", err)
	}

	return wallet
}

// TestWalletRepositoryCreateAndFind verifies wallet persistence and rehydration.
func TestWalletRepositoryCreateAndFind(t *testing.T) {
	ctx := integrationContext(t)
	pool := openIntegrationPool(t)

	repository := postgres.NewWalletRepository(pool)

	playerID := uuid.New()
	wallet := newWalletRepositoryWallet(t, playerID, "100.00")

	t.Cleanup(func() {
		_, _ = pool.Exec(
			context.Background(),
			`DELETE FROM wallets WHERE player_id = $1`,
			playerID,
		)
	})

	if err := repository.Create(ctx, wallet); err != nil {
		t.Fatalf("failed to create wallet: %v", err)
	}

	found, err := repository.FindByID(ctx, wallet.ID())
	if err != nil {
		t.Fatalf("failed to find wallet: %v", err)
	}

	if found.ID() != wallet.ID() {
		t.Errorf(
			"expected wallet id %s, got %s",
			wallet.ID(),
			found.ID(),
		)
	}

	if found.PlayerID() != playerID {
		t.Errorf(
			"expected player id %s, got %s",
			playerID,
			found.PlayerID(),
		)
	}

	if found.Currency().Code() != "BRL" {
		t.Errorf(
			"expected currency BRL, got %s",
			found.Currency(),
		)
	}

	if found.Balance().Amount() != "100.00" {
		t.Errorf(
			"expected balance 100.00, got %s",
			found.Balance().Amount(),
		)
	}

	if found.Version() != 1 {
		t.Errorf(
			"expected version 1, got %d",
			found.Version(),
		)
	}
}

// TestWalletRepositoryRejectsDuplicatePlayerCurrency verifies the database uniqueness guarantee.
func TestWalletRepositoryRejectsDuplicatePlayerCurrency(t *testing.T) {
	ctx := integrationContext(t)
	pool := openIntegrationPool(t)

	repository := postgres.NewWalletRepository(pool)

	playerID := uuid.New()

	firstWallet := newWalletRepositoryWallet(
		t,
		playerID,
		"100.00",
	)

	secondWallet := newWalletRepositoryWallet(
		t,
		playerID,
		"200.00",
	)

	t.Cleanup(func() {
		_, _ = pool.Exec(
			context.Background(),
			`DELETE FROM wallets WHERE player_id = $1`,
			playerID,
		)
	})

	if err := repository.Create(ctx, firstWallet); err != nil {
		t.Fatalf("failed to create first wallet: %v", err)
	}

	err := repository.Create(ctx, secondWallet)

	if !errors.Is(err, application.ErrAlreadyExists) {
		t.Fatalf(
			"expected ErrAlreadyExists, got %v",
			err,
		)
	}
}

// TestWalletRepositoryFindByIDForUpdateLocksWallet verifies concurrent transactions serialize access to one wallet.
func TestWalletRepositoryFindByIDForUpdateLocksWallet(t *testing.T) {
	ctx := integrationContext(t)
	pool := openIntegrationPool(t)

	repository := postgres.NewWalletRepository(pool)
	txManager := postgres.NewTransactionManager(pool)

	playerID := uuid.New()
	wallet := newWalletRepositoryWallet(t, playerID, "100.00")

	t.Cleanup(func() {
		_, _ = pool.Exec(
			context.Background(),
			`DELETE FROM wallets WHERE player_id = $1`,
			playerID,
		)
	})

	if err := repository.Create(ctx, wallet); err != nil {
		t.Fatalf("failed to create wallet: %v", err)
	}

	firstLocked := make(chan struct{})
	releaseFirst := make(chan struct{})
	firstDone := make(chan error, 1)

	go func() {
		firstDone <- txManager.WithinTransaction(
			ctx,
			func(txCtx context.Context) error {
				_, err := repository.FindByIDForUpdate(
					txCtx,
					wallet.ID(),
				)
				if err != nil {
					return err
				}

				close(firstLocked)

				<-releaseFirst

				return nil
			},
		)
	}()

	select {
	case <-firstLocked:
	case <-time.After(2 * time.Second):
		t.Fatal("first transaction did not acquire wallet lock")
	}

	secondStarted := make(chan struct{})
	secondAcquired := make(chan struct{})
	secondDone := make(chan error, 1)

	go func() {
		secondDone <- txManager.WithinTransaction(
			ctx,
			func(txCtx context.Context) error {
				close(secondStarted)

				_, err := repository.FindByIDForUpdate(
					txCtx,
					wallet.ID(),
				)
				if err != nil {
					return err
				}

				close(secondAcquired)

				return nil
			},
		)
	}()

	select {
	case <-secondStarted:
	case <-time.After(2 * time.Second):
		close(releaseFirst)
		t.Fatal("second transaction did not start")
	}

	acquiredBeforeRelease := false

	select {
	case <-secondAcquired:
		acquiredBeforeRelease = true

	case <-time.After(200 * time.Millisecond):
		// Expected: the second transaction is blocked by FOR UPDATE.
	}

	close(releaseFirst)

	if acquiredBeforeRelease {
		t.Fatal("second transaction acquired wallet before first transaction released it")
	}

	select {
	case err := <-firstDone:
		if err != nil {
			t.Fatalf("first transaction failed: %v", err)
		}

	case <-time.After(2 * time.Second):
		t.Fatal("first transaction did not finish")
	}

	select {
	case err := <-secondDone:
		if err != nil {
			t.Fatalf("second transaction failed: %v", err)
		}

	case <-time.After(2 * time.Second):
		t.Fatal("second transaction did not acquire wallet after release")
	}
}
