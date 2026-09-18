package domain

import (
	"time"

	"github.com/google/uuid"
)

type WalletLedgerDirection string

const (
	WalletLedgerDirectionDebit  WalletLedgerDirection = "DEBIT"
	WalletLedgerDirectionCredit WalletLedgerDirection = "CREDIT"
)

type WalletLedgerEntry struct {
	id            uuid.UUID
	walletID      uuid.UUID
	transactionID uuid.UUID
	direction     WalletLedgerDirection
	amount        Money
	balanceBefore Money
	balanceAfter  Money
	createdAt     time.Time
}

// NewWalletLedgerEntry creates an immutable wallet ledger entry.
func NewWalletLedgerEntry(
	id uuid.UUID,
	walletID uuid.UUID,
	transactionID uuid.UUID,
	direction WalletLedgerDirection,
	amount Money,
	balanceBefore Money,
	balanceAfter Money,
	createdAt time.Time,
) (*WalletLedgerEntry, error) {
	return buildWalletLedgerEntry(
		id,
		walletID,
		transactionID,
		direction,
		amount,
		balanceBefore,
		balanceAfter,
		createdAt,
	)
}

// RehydrateWalletLedgerEntry rebuilds a persisted ledger entry without changing it.
func RehydrateWalletLedgerEntry(
	id uuid.UUID,
	walletID uuid.UUID,
	transactionID uuid.UUID,
	direction WalletLedgerDirection,
	amount Money,
	balanceBefore Money,
	balanceAfter Money,
	createdAt time.Time,
) (*WalletLedgerEntry, error) {
	return buildWalletLedgerEntry(
		id,
		walletID,
		transactionID,
		direction,
		amount,
		balanceBefore,
		balanceAfter,
		createdAt,
	)
}

// buildWalletLedgerEntry validates and builds a wallet ledger entry.
func buildWalletLedgerEntry(
	id uuid.UUID,
	walletID uuid.UUID,
	transactionID uuid.UUID,
	direction WalletLedgerDirection,
	amount Money,
	balanceBefore Money,
	balanceAfter Money,
	createdAt time.Time,
) (*WalletLedgerEntry, error) {
	if id == uuid.Nil {
		return nil, ErrInvalidLedgerEntry
	}

	if walletID == uuid.Nil {
		return nil, ErrInvalidWalletID
	}

	if transactionID == uuid.Nil {
		return nil, ErrInvalidTransactionID
	}

	if !direction.IsValid() {
		return nil, ErrInvalidLedgerDirection
	}

	if !amount.IsValid() || !balanceBefore.IsValid() || !balanceAfter.IsValid() {
		return nil, ErrInvalidMoney
	}

	if !amount.IsPositive() {
		return nil, ErrAmountMustBePositive
	}

	if balanceBefore.IsNegative() || balanceAfter.IsNegative() {
		return nil, ErrInvalidFinancialResult
	}

	if !amount.Currency().Equal(balanceBefore.Currency()) ||
		!amount.Currency().Equal(balanceAfter.Currency()) {
		return nil, ErrCurrencyMismatch
	}

	if createdAt.IsZero() {
		return nil, ErrInvalidTimestamp
	}

	if err := validateLedgerBalance(
		direction,
		amount,
		balanceBefore,
		balanceAfter,
	); err != nil {
		return nil, err
	}

	return &WalletLedgerEntry{
		id:            id,
		walletID:      walletID,
		transactionID: transactionID,
		direction:     direction,
		amount:        amount,
		balanceBefore: balanceBefore,
		balanceAfter:  balanceAfter,
		createdAt:     createdAt.UTC(),
	}, nil
}

// validateLedgerBalance validates the financial result of a ledger movement.
func validateLedgerBalance(
	direction WalletLedgerDirection,
	amount Money,
	balanceBefore Money,
	balanceAfter Money,
) error {
	var expected Money
	var err error

	switch direction {
	case WalletLedgerDirectionDebit:
		expected, err = balanceBefore.Sub(amount)

	case WalletLedgerDirectionCredit:
		expected, err = balanceBefore.Add(amount)

	default:
		return ErrInvalidLedgerDirection
	}

	if err != nil {
		return err
	}

	if !expected.Equal(balanceAfter) {
		return ErrInvalidFinancialResult
	}

	return nil
}

// IsValid reports whether the ledger direction is supported.
func (d WalletLedgerDirection) IsValid() bool {
	return d == WalletLedgerDirectionDebit ||
		d == WalletLedgerDirectionCredit
}

// String returns the ledger direction.
func (d WalletLedgerDirection) String() string {
	return string(d)
}

// ID returns the ledger entry identifier.
func (e *WalletLedgerEntry) ID() uuid.UUID {
	return e.id
}

// WalletID returns the affected wallet identifier.
func (e *WalletLedgerEntry) WalletID() uuid.UUID {
	return e.walletID
}

// TransactionID returns the transaction that caused the movement.
func (e *WalletLedgerEntry) TransactionID() uuid.UUID {
	return e.transactionID
}

// Direction returns whether the movement is a debit or credit.
func (e *WalletLedgerEntry) Direction() WalletLedgerDirection {
	return e.direction
}

// Amount returns the movement amount.
func (e *WalletLedgerEntry) Amount() Money {
	return e.amount
}

// BalanceBefore returns the balance before the movement.
func (e *WalletLedgerEntry) BalanceBefore() Money {
	return e.balanceBefore
}

// BalanceAfter returns the balance after the movement.
func (e *WalletLedgerEntry) BalanceAfter() Money {
	return e.balanceAfter
}

// CreatedAt returns when the ledger entry was created.
func (e *WalletLedgerEntry) CreatedAt() time.Time {
	return e.createdAt
}
