package domain

import (
	"time"

	"github.com/google/uuid"
)

const initialWalletVersion int64 = 1

type Wallet struct {
	id        uuid.UUID
	playerID  uuid.UUID
	balance   Money
	version   int64
	createdAt time.Time
	updatedAt time.Time
}

// NewWallet creates a wallet with its initial balance and version.
func NewWallet(
	id uuid.UUID,
	playerID uuid.UUID,
	balance Money,
	createdAt time.Time,
) (*Wallet, error) {
	if id == uuid.Nil {
		return nil, ErrInvalidWalletID
	}

	if playerID == uuid.Nil {
		return nil, ErrInvalidPlayerID
	}

	if !balance.IsValid() || balance.IsNegative() {
		return nil, ErrInvalidMoney
	}

	if createdAt.IsZero() {
		return nil, ErrInvalidTimestamp
	}

	createdAt = createdAt.UTC()

	return &Wallet{
		id:        id,
		playerID:  playerID,
		balance:   balance,
		version:   initialWalletVersion,
		createdAt: createdAt,
		updatedAt: createdAt,
	}, nil
}

// RehydrateWallet rebuilds a wallet from persisted state without changing it.
func RehydrateWallet(
	id uuid.UUID,
	playerID uuid.UUID,
	balance Money,
	version int64,
	createdAt time.Time,
	updatedAt time.Time,
) (*Wallet, error) {
	if id == uuid.Nil {
		return nil, ErrInvalidWalletID
	}

	if playerID == uuid.Nil {
		return nil, ErrInvalidPlayerID
	}

	if !balance.IsValid() || balance.IsNegative() {
		return nil, ErrInvalidMoney
	}

	if version < initialWalletVersion {
		return nil, ErrInvalidWalletVersion
	}

	if createdAt.IsZero() || updatedAt.IsZero() || updatedAt.Before(createdAt) {
		return nil, ErrInvalidTimestamp
	}

	return &Wallet{
		id:        id,
		playerID:  playerID,
		balance:   balance,
		version:   version,
		createdAt: createdAt.UTC(),
		updatedAt: updatedAt.UTC(),
	}, nil
}

// Credit adds a positive amount to the wallet balance.
func (w *Wallet) Credit(amount Money, updatedAt time.Time) error {
	if err := w.validateMovement(amount, updatedAt); err != nil {
		return err
	}

	newBalance, err := w.balance.Add(amount)
	if err != nil {
		return err
	}

	w.applyBalance(newBalance, updatedAt)

	return nil
}

// Debit subtracts a positive amount while preserving a non-negative balance.
func (w *Wallet) Debit(amount Money, updatedAt time.Time) error {
	if err := w.validateMovement(amount, updatedAt); err != nil {
		return err
	}

	comparison, err := w.balance.Compare(amount)
	if err != nil {
		return err
	}

	if comparison < 0 {
		return ErrInsufficientFunds
	}

	newBalance, err := w.balance.Sub(amount)
	if err != nil {
		return err
	}

	w.applyBalance(newBalance, updatedAt)

	return nil
}

// validateMovement validates a wallet balance-changing operation.
func (w *Wallet) validateMovement(amount Money, updatedAt time.Time) error {
	if !amount.IsValid() {
		return ErrInvalidMoney
	}

	if !amount.IsPositive() {
		return ErrAmountMustBePositive
	}

	if !w.balance.Currency().Equal(amount.Currency()) {
		return ErrCurrencyMismatch
	}

	if updatedAt.IsZero() || updatedAt.Before(w.updatedAt) {
		return ErrInvalidTimestamp
	}

	return nil
}

// applyBalance updates the wallet balance, version and modification time.
func (w *Wallet) applyBalance(balance Money, updatedAt time.Time) {
	w.balance = balance
	w.version++
	w.updatedAt = updatedAt.UTC()
}

// ID returns the wallet identifier.
func (w *Wallet) ID() uuid.UUID {
	return w.id
}

// PlayerID returns the wallet owner identifier.
func (w *Wallet) PlayerID() uuid.UUID {
	return w.playerID
}

// Balance returns the current wallet balance.
func (w *Wallet) Balance() Money {
	return w.balance
}

// Currency returns the wallet currency.
func (w *Wallet) Currency() Currency {
	return w.balance.Currency()
}

// Version returns the wallet version.
func (w *Wallet) Version() int64 {
	return w.version
}

// CreatedAt returns when the wallet was created.
func (w *Wallet) CreatedAt() time.Time {
	return w.createdAt
}

// UpdatedAt returns when the wallet was last financially changed.
func (w *Wallet) UpdatedAt() time.Time {
	return w.updatedAt
}
