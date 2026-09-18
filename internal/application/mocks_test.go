package application

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/pablo-banker/junglegaming-test/internal/domain"
)

type fakeTransactionManager struct {
	called bool
	err    error
}

// WithinTransaction executes the provided function immediately.
func (f *fakeTransactionManager) WithinTransaction(
	ctx context.Context,
	fn func(context.Context) error,
) error {
	f.called = true

	if f.err != nil {
		return f.err
	}

	return fn(ctx)
}

type fakeClock struct {
	now time.Time
	err error
}

// Now returns the configured test timestamp.
func (f *fakeClock) Now(context.Context) (time.Time, error) {
	if f.err != nil {
		return time.Time{}, f.err
	}

	return f.now, nil
}

type fakeWalletRepository struct {
	created []*domain.Wallet
	updated []*domain.Wallet

	createErr error
	updateErr error

	findByIDResult *domain.Wallet
	findByIDErr    error

	findByIDForUpdateResult *domain.Wallet
	findByIDForUpdateErr    error

	findByPlayerAndCurrencyResult *domain.Wallet
	findByPlayerAndCurrencyErr    error
}

// Create records a wallet creation.
func (f *fakeWalletRepository) Create(
	_ context.Context,
	wallet *domain.Wallet,
) error {
	if f.createErr != nil {
		return f.createErr
	}

	f.created = append(f.created, wallet)

	return nil
}

// FindByID returns the configured wallet.
func (f *fakeWalletRepository) FindByID(
	context.Context,
	uuid.UUID,
) (*domain.Wallet, error) {
	if f.findByIDErr != nil {
		return nil, f.findByIDErr
	}

	return f.findByIDResult, nil
}

// FindByIDForUpdate returns the configured locked wallet.
func (f *fakeWalletRepository) FindByIDForUpdate(
	context.Context,
	uuid.UUID,
) (*domain.Wallet, error) {
	if f.findByIDForUpdateErr != nil {
		return nil, f.findByIDForUpdateErr
	}

	return f.findByIDForUpdateResult, nil
}

// FindByPlayerAndCurrency returns the configured wallet.
func (f *fakeWalletRepository) FindByPlayerAndCurrency(
	context.Context,
	uuid.UUID,
	domain.Currency,
) (*domain.Wallet, error) {
	if f.findByPlayerAndCurrencyErr != nil {
		return nil, f.findByPlayerAndCurrencyErr
	}

	return f.findByPlayerAndCurrencyResult, nil
}

// Update records a wallet update.
func (f *fakeWalletRepository) Update(
	_ context.Context,
	wallet *domain.Wallet,
) error {
	if f.updateErr != nil {
		return f.updateErr
	}

	f.updated = append(f.updated, wallet)

	return nil
}

type pendingReferenceUpdate struct {
	transaction   *domain.WagerTransaction
	nextAttemptAt time.Time
	expiresAt     time.Time
}

type fakeWagerRepository struct {
	created []*domain.WagerTransaction
	updated []*domain.WagerTransaction

	pendingReferenceUpdates []pendingReferenceUpdate

	createErr                 error
	updateErr                 error
	updatePendingReferenceErr error

	findByIDResult *domain.WagerTransaction
	findByIDErr    error

	findByProviderExternalResult *domain.WagerTransaction
	findByProviderExternalErr    error

	findByProviderIdempotencyResult *domain.WagerTransaction
	findByProviderIdempotencyErr    error

	hasProcessedDirectReversal    bool
	hasProcessedDirectReversalErr error
}

// Create records a wager transaction creation.
func (f *fakeWagerRepository) Create(
	_ context.Context,
	transaction *domain.WagerTransaction,
) error {
	if f.createErr != nil {
		return f.createErr
	}

	f.created = append(f.created, transaction)

	return nil
}

// Update records a wager transaction update.
func (f *fakeWagerRepository) Update(
	_ context.Context,
	transaction *domain.WagerTransaction,
) error {
	if f.updateErr != nil {
		return f.updateErr
	}

	f.updated = append(f.updated, transaction)

	return nil
}

// UpdatePendingReference records reference retry metadata.
func (f *fakeWagerRepository) UpdatePendingReference(
	_ context.Context,
	transaction *domain.WagerTransaction,
	nextAttemptAt time.Time,
	expiresAt time.Time,
) error {
	if f.updatePendingReferenceErr != nil {
		return f.updatePendingReferenceErr
	}

	f.pendingReferenceUpdates = append(
		f.pendingReferenceUpdates,
		pendingReferenceUpdate{
			transaction:   transaction,
			nextAttemptAt: nextAttemptAt,
			expiresAt:     expiresAt,
		},
	)

	return nil
}

// FindByID returns the configured transaction.
func (f *fakeWagerRepository) FindByID(
	context.Context,
	uuid.UUID,
) (*domain.WagerTransaction, error) {
	if f.findByIDErr != nil {
		return nil, f.findByIDErr
	}

	return f.findByIDResult, nil
}

// FindByProviderAndExternalTransactionID returns the configured transaction.
func (f *fakeWagerRepository) FindByProviderAndExternalTransactionID(
	context.Context,
	string,
	string,
) (*domain.WagerTransaction, error) {
	if f.findByProviderExternalErr != nil {
		return nil, f.findByProviderExternalErr
	}

	return f.findByProviderExternalResult, nil
}

// FindByProviderAndIdempotencyKey returns the configured transaction.
func (f *fakeWagerRepository) FindByProviderAndIdempotencyKey(
	context.Context,
	string,
	string,
) (*domain.WagerTransaction, error) {
	if f.findByProviderIdempotencyErr != nil {
		return nil, f.findByProviderIdempotencyErr
	}

	return f.findByProviderIdempotencyResult, nil
}

// HasProcessedDirectReversal returns whether a successful direct reversal exists.
func (f *fakeWagerRepository) HasProcessedDirectReversal(
	context.Context,
	uuid.UUID,
) (bool, error) {
	if f.hasProcessedDirectReversalErr != nil {
		return false, f.hasProcessedDirectReversalErr
	}

	return f.hasProcessedDirectReversal, nil
}

type fakeLedgerRepository struct {
	created []*domain.WalletLedgerEntry
	err     error
}

// Create records a wallet ledger entry.
func (f *fakeLedgerRepository) Create(
	_ context.Context,
	entry *domain.WalletLedgerEntry,
) error {
	if f.err != nil {
		return f.err
	}

	f.created = append(f.created, entry)

	return nil
}

type fakeOutboxRepository struct {
	created []EventEnvelope
	err     error
}

// Create records an outbox event.
func (f *fakeOutboxRepository) Create(
	_ context.Context,
	event EventEnvelope,
) error {
	if f.err != nil {
		return f.err
	}

	f.created = append(f.created, event)

	return nil
}
