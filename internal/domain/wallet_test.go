//go:build unit

package domain

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

// TestNewWalletCreatesValidWallet verifies wallet creation with version one.
func TestNewWalletCreatesValidWallet(t *testing.T) {
	brl, _ := NewCurrency("BRL")
	balance, _ := ParseMoney("100.00", brl)
	createdAt := time.Now().UTC()

	wallet, err := NewWallet(
		uuid.New(),
		uuid.New(),
		balance,
		createdAt,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if wallet.Balance().Amount() != "100.00" {
		t.Errorf("expected balance 100.00, got %s", wallet.Balance().Amount())
	}

	if wallet.Version() != 1 {
		t.Errorf("expected version 1, got %d", wallet.Version())
	}

	if !wallet.CreatedAt().Equal(wallet.UpdatedAt()) {
		t.Error("expected createdAt and updatedAt to be equal")
	}
}

// TestWalletCreditIncreasesBalanceAndVersion verifies successful credit.
func TestWalletCreditIncreasesBalanceAndVersion(t *testing.T) {
	brl, _ := NewCurrency("BRL")
	balance, _ := ParseMoney("100.00", brl)
	credit, _ := ParseMoney("25.00", brl)

	now := time.Now().UTC()

	wallet, _ := NewWallet(
		uuid.New(),
		uuid.New(),
		balance,
		now,
	)

	err := wallet.Credit(credit, now.Add(time.Second))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if wallet.Balance().Amount() != "125.00" {
		t.Errorf("expected balance 125.00, got %s", wallet.Balance().Amount())
	}

	if wallet.Version() != 2 {
		t.Errorf("expected version 2, got %d", wallet.Version())
	}
}

// TestWalletDebitDecreasesBalanceAndVersion verifies successful debit.
func TestWalletDebitDecreasesBalanceAndVersion(t *testing.T) {
	brl, _ := NewCurrency("BRL")
	balance, _ := ParseMoney("100.00", brl)
	debit, _ := ParseMoney("25.00", brl)

	now := time.Now().UTC()

	wallet, _ := NewWallet(
		uuid.New(),
		uuid.New(),
		balance,
		now,
	)

	err := wallet.Debit(debit, now.Add(time.Second))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if wallet.Balance().Amount() != "75.00" {
		t.Errorf("expected balance 75.00, got %s", wallet.Balance().Amount())
	}

	if wallet.Version() != 2 {
		t.Errorf("expected version 2, got %d", wallet.Version())
	}
}

// TestWalletRejectsInsufficientFunds verifies debit cannot make balance negative.
func TestWalletRejectsInsufficientFunds(t *testing.T) {
	brl, _ := NewCurrency("BRL")
	balance, _ := ParseMoney("20.00", brl)
	debit, _ := ParseMoney("80.00", brl)

	now := time.Now().UTC()

	wallet, _ := NewWallet(
		uuid.New(),
		uuid.New(),
		balance,
		now,
	)

	err := wallet.Debit(debit, now.Add(time.Second))

	if !errors.Is(err, ErrInsufficientFunds) {
		t.Fatalf("expected ErrInsufficientFunds, got %v", err)
	}

	if wallet.Balance().Amount() != "20.00" {
		t.Errorf("expected balance to remain 20.00, got %s", wallet.Balance().Amount())
	}

	if wallet.Version() != 1 {
		t.Errorf("expected version to remain 1, got %d", wallet.Version())
	}
}

// TestWalletRejectsInvalidMovement verifies movement amount, currency and timestamp rules.
func TestWalletRejectsInvalidMovement(t *testing.T) {
	brl, _ := NewCurrency("BRL")
	usd, _ := NewCurrency("USD")

	balance, _ := ParseMoney("100.00", brl)
	zero, _ := Zero(brl)
	usdAmount, _ := ParseMoney("10.00", usd)
	amount, _ := ParseMoney("10.00", brl)

	now := time.Now().UTC()

	tests := []struct {
		name   string
		amount Money
		time   time.Time
		err    error
	}{
		{
			name:   "zero amount",
			amount: zero,
			time:   now.Add(time.Second),
			err:    ErrAmountMustBePositive,
		},
		{
			name:   "currency mismatch",
			amount: usdAmount,
			time:   now.Add(time.Second),
			err:    ErrCurrencyMismatch,
		},
		{
			name:   "timestamp before last update",
			amount: amount,
			time:   now.Add(-time.Second),
			err:    ErrInvalidTimestamp,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			wallet, _ := NewWallet(
				uuid.New(),
				uuid.New(),
				balance,
				now,
			)

			err := wallet.Credit(test.amount, test.time)

			if !errors.Is(err, test.err) {
				t.Fatalf("expected %v, got %v", test.err, err)
			}

			if wallet.Balance().Amount() != "100.00" {
				t.Errorf("expected balance to remain 100.00, got %s", wallet.Balance().Amount())
			}

			if wallet.Version() != 1 {
				t.Errorf("expected version to remain 1, got %d", wallet.Version())
			}
		})
	}
}

// TestRehydrateWalletPreservesPersistedState verifies wallet state restoration.
func TestRehydrateWalletPreservesPersistedState(t *testing.T) {
	brl, _ := NewCurrency("BRL")
	balance, _ := ParseMoney("250.00", brl)

	createdAt := time.Now().UTC()
	updatedAt := createdAt.Add(time.Minute)

	wallet, err := RehydrateWallet(
		uuid.New(),
		uuid.New(),
		balance,
		7,
		createdAt,
		updatedAt,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if wallet.Balance().Amount() != "250.00" {
		t.Errorf("expected balance 250.00, got %s", wallet.Balance().Amount())
	}

	if wallet.Version() != 7 {
		t.Errorf("expected version 7, got %d", wallet.Version())
	}

	if !wallet.UpdatedAt().Equal(updatedAt) {
		t.Error("expected persisted updatedAt to be preserved")
	}
}
