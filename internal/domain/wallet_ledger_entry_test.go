//go:build unit

package domain

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

// TestNewWalletLedgerEntryCreatesDebit verifies a valid debit ledger entry.
func TestNewWalletLedgerEntryCreatesDebit(t *testing.T) {
	brl, _ := NewCurrency("BRL")
	amount, _ := ParseMoney("25.00", brl)
	before, _ := ParseMoney("100.00", brl)
	after, _ := ParseMoney("75.00", brl)

	entry, err := NewWalletLedgerEntry(
		uuid.New(),
		uuid.New(),
		uuid.New(),
		WalletLedgerDirectionDebit,
		amount,
		before,
		after,
		time.Now().UTC(),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if entry.Direction() != WalletLedgerDirectionDebit {
		t.Errorf("expected DEBIT, got %s", entry.Direction())
	}

	if entry.BalanceAfter().Amount() != "75.00" {
		t.Errorf("expected balance 75.00, got %s", entry.BalanceAfter().Amount())
	}
}

// TestNewWalletLedgerEntryCreatesCredit verifies a valid credit ledger entry.
func TestNewWalletLedgerEntryCreatesCredit(t *testing.T) {
	brl, _ := NewCurrency("BRL")
	amount, _ := ParseMoney("25.00", brl)
	before, _ := ParseMoney("100.00", brl)
	after, _ := ParseMoney("125.00", brl)

	entry, err := NewWalletLedgerEntry(
		uuid.New(),
		uuid.New(),
		uuid.New(),
		WalletLedgerDirectionCredit,
		amount,
		before,
		after,
		time.Now().UTC(),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if entry.BalanceAfter().Amount() != "125.00" {
		t.Errorf("expected balance 125.00, got %s", entry.BalanceAfter().Amount())
	}
}

// TestWalletLedgerEntryRejectsInvalidFinancialResult verifies ledger math cannot be inconsistent.
func TestWalletLedgerEntryRejectsInvalidFinancialResult(t *testing.T) {
	brl, _ := NewCurrency("BRL")
	amount, _ := ParseMoney("25.00", brl)
	before, _ := ParseMoney("100.00", brl)
	invalidAfter, _ := ParseMoney("90.00", brl)

	_, err := NewWalletLedgerEntry(
		uuid.New(),
		uuid.New(),
		uuid.New(),
		WalletLedgerDirectionDebit,
		amount,
		before,
		invalidAfter,
		time.Now().UTC(),
	)

	if !errors.Is(err, ErrInvalidFinancialResult) {
		t.Fatalf("expected ErrInvalidFinancialResult, got %v", err)
	}
}

// TestWalletLedgerEntryRejectsCurrencyMismatch verifies all monetary values use the same currency.
func TestWalletLedgerEntryRejectsCurrencyMismatch(t *testing.T) {
	brl, _ := NewCurrency("BRL")
	usd, _ := NewCurrency("USD")

	amount, _ := ParseMoney("25.00", brl)
	before, _ := ParseMoney("100.00", brl)
	after, _ := ParseMoney("75.00", usd)

	_, err := NewWalletLedgerEntry(
		uuid.New(),
		uuid.New(),
		uuid.New(),
		WalletLedgerDirectionDebit,
		amount,
		before,
		after,
		time.Now().UTC(),
	)

	if !errors.Is(err, ErrCurrencyMismatch) {
		t.Fatalf("expected ErrCurrencyMismatch, got %v", err)
	}
}

// TestRehydrateWalletLedgerEntryPreservesState verifies persisted ledger restoration.
func TestRehydrateWalletLedgerEntryPreservesState(t *testing.T) {
	brl, _ := NewCurrency("BRL")
	amount, _ := ParseMoney("20.00", brl)
	before, _ := ParseMoney("80.00", brl)
	after, _ := ParseMoney("100.00", brl)

	id := uuid.New()
	walletID := uuid.New()
	transactionID := uuid.New()
	createdAt := time.Now().UTC()

	entry, err := RehydrateWalletLedgerEntry(
		id,
		walletID,
		transactionID,
		WalletLedgerDirectionCredit,
		amount,
		before,
		after,
		createdAt,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if entry.ID() != id {
		t.Error("expected persisted id to be preserved")
	}

	if entry.WalletID() != walletID {
		t.Error("expected persisted wallet id to be preserved")
	}

	if entry.TransactionID() != transactionID {
		t.Error("expected persisted transaction id to be preserved")
	}

	if !entry.CreatedAt().Equal(createdAt) {
		t.Error("expected persisted createdAt to be preserved")
	}
}
