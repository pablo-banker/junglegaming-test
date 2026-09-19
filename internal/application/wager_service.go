package application

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/pablo-banker/junglegaming-test/internal/domain"
)

type WagerService struct {
	txManager TransactionManager
	clock     Clock
	wallets   WalletRepository
	wagers    WagerRepository
	ledger    WalletLedgerRepository
	outbox    OutboxRepository
	policy    ReferenceRetryPolicy
}

// wagerRejection is a definitive business outcome persisted as REJECTED.
type wagerRejection struct {
	code    string
	message string
}

// NewWagerService creates the wager application service.
func NewWagerService(
	txManager TransactionManager,
	clock Clock,
	wallets WalletRepository,
	wagers WagerRepository,
	ledger WalletLedgerRepository,
	outbox OutboxRepository,
	policy ReferenceRetryPolicy,
) *WagerService {
	return &WagerService{
		txManager: txManager,
		clock:     clock,
		wallets:   wallets,
		wagers:    wagers,
		ledger:    ledger,
		outbox:    outbox,
		policy:    policy,
	}
}

// Process executes a wager operation atomically: insert, lock the wallet, then read the clock.
func (s *WagerService) Process(ctx context.Context, command ProcessWagerCommand, metadata CommandMetadata) (*ProcessWagerResult, error) {
	if strings.TrimSpace(metadata.CorrelationID) == "" {
		return nil, ErrInvalidCorrelationID
	}

	request, err := parseWagerRequest(command)
	if err != nil {
		return nil, err
	}

	var result *ProcessWagerResult

	err = s.txManager.WithinTransaction(
		ctx,
		func(txCtx context.Context) error {
			createdAt, err := s.clock.Now(txCtx)
			if err != nil {
				return err
			}

			transaction, err := request.newTransaction(createdAt)
			if err != nil {
				return err
			}

			if err := s.wagers.Create(txCtx, transaction); err != nil {
				if !errors.Is(err, ErrAlreadyExists) {
					return err
				}

				result, err = s.resolveExistingTransaction(txCtx, request)

				return err
			}

			wallet, err := s.lockWallet(txCtx, transaction)
			if err != nil {
				return err
			}

			now, err := s.clock.Now(txCtx)
			if err != nil {
				return err
			}

			result, err = s.processNew(txCtx, wallet, transaction, metadata, now)

			return err
		},
	)
	if err != nil {
		return nil, err
	}

	return result, nil
}

// GetByID returns a wager transaction visible to the authenticated provider.
func (s *WagerService) GetByID(ctx context.Context, providerID string, transactionID string) (*WagerResult, error) {
	id, err := uuid.Parse(transactionID)
	if err != nil || id == uuid.Nil {
		return nil, ErrNotFound
	}

	transaction, err := s.wagers.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}

	if transaction.ProviderID() != providerID {
		return nil, ErrNotFound
	}

	return wagerResultFromTransaction(transaction), nil
}

// GetByExternalTransactionID returns a wager transaction by provider and external identifier.
func (s *WagerService) GetByExternalTransactionID(
	ctx context.Context,
	providerID string,
	externalTransactionID string,
) (*WagerResult, error) {
	transaction, err := s.wagers.FindByProviderAndExternalTransactionID(ctx, providerID, externalTransactionID)
	if err != nil {
		return nil, err
	}

	return wagerResultFromTransaction(transaction), nil
}

// processNew applies a freshly persisted transaction, waiting for its reference when needed.
func (s *WagerService) processNew(
	ctx context.Context,
	wallet *domain.Wallet,
	transaction *domain.WagerTransaction,
	metadata CommandMetadata,
	now time.Time,
) (*ProcessWagerResult, error) {
	if transaction.ReferenceExternalTransactionID() != "" {
		outcome, err := s.resolveReference(ctx, transaction, now)
		if err != nil {
			return nil, err
		}

		if outcome.rejection != nil {
			return s.reject(ctx, transaction, *outcome.rejection, metadata, now)
		}

		if !outcome.available {
			return s.markPendingReference(ctx, transaction, metadata, now)
		}
	}

	return s.settle(ctx, wallet, transaction, metadata, now)
}

// resolveExistingTransaction handles persisted idempotency and external transaction conflicts.
func (s *WagerService) resolveExistingTransaction(ctx context.Context, request wagerRequest) (*ProcessWagerResult, error) {
	command := request.command

	existing, err := s.wagers.FindByProviderAndIdempotencyKey(ctx, command.ProviderID, command.IdempotencyKey)
	if err == nil {
		if existing.PayloadHash() != request.payloadHash {
			return nil, ErrIdempotencyConflict
		}

		return resultFromWager(existing, true), nil
	}

	if !errors.Is(err, ErrNotFound) {
		return nil, err
	}

	_, err = s.wagers.FindByProviderAndExternalTransactionID(ctx, command.ProviderID, command.ExternalTransactionID)
	if err == nil {
		return nil, ErrExternalTransactionConflict
	}

	if !errors.Is(err, ErrNotFound) {
		return nil, err
	}

	return nil, ErrAlreadyExists
}

// lockWallet locks the wager wallet and verifies it belongs to the wager player and currency.
func (s *WagerService) lockWallet(ctx context.Context, transaction *domain.WagerTransaction) (*domain.Wallet, error) {
	wallet, err := s.wallets.FindByIDForUpdate(ctx, transaction.WalletID())
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, ErrWalletNotFound
		}

		return nil, err
	}

	if wallet.PlayerID() != transaction.PlayerID() {
		return nil, ErrWalletMismatch
	}

	if !wallet.Currency().Equal(transaction.Amount().Currency()) {
		return nil, domain.ErrCurrencyMismatch
	}

	return wallet, nil
}

// settle applies a transaction whose reference, when present, is already resolved.
func (s *WagerService) settle(
	ctx context.Context,
	wallet *domain.Wallet,
	transaction *domain.WagerTransaction,
	metadata CommandMetadata,
	now time.Time,
) (*ProcessWagerResult, error) {
	if transaction.Type().RequiresReference() {
		duplicate, err := s.wagers.HasProcessedDirectReversal(ctx, transaction.ReferenceTransactionID())
		if err != nil {
			return nil, err
		}

		if duplicate {
			return s.reject(ctx, transaction, wagerRejection{
				code:    FailureCodeDuplicateReversal,
				message: "referenced transaction already has a successful direct reversal",
			}, metadata, now)
		}
	}

	return s.applyMovement(ctx, wallet, transaction, metadata, now)
}

// applyMovement applies the wager operation to the locked wallet.
func (s *WagerService) applyMovement(
	ctx context.Context,
	wallet *domain.Wallet,
	transaction *domain.WagerTransaction,
	metadata CommandMetadata,
	now time.Time,
) (*ProcessWagerResult, error) {
	direction, hasMovement, err := transaction.MovementDirection()
	if err != nil {
		return nil, err
	}

	before := wallet.Balance()

	if !hasMovement {
		if err := transaction.MarkProcessed(before, before, now); err != nil {
			return nil, err
		}

		if err := s.wagers.Update(ctx, transaction); err != nil {
			return nil, err
		}

		if err := s.createProcessedEvent(ctx, transaction, metadata, now); err != nil {
			return nil, err
		}

		return resultFromWager(transaction, false), nil
	}

	switch direction {
	case domain.WalletLedgerDirectionDebit:
		err = wallet.Debit(transaction.Amount(), now)

	case domain.WalletLedgerDirectionCredit:
		err = wallet.Credit(transaction.Amount(), now)

	default:
		return nil, domain.ErrInvalidLedgerDirection
	}

	if errors.Is(err, domain.ErrInsufficientFunds) {
		return s.reject(ctx, transaction, insufficientFundsRejection(transaction), metadata, now)
	}

	if err != nil {
		return nil, err
	}

	after := wallet.Balance()

	if err := transaction.MarkProcessed(before, after, now); err != nil {
		return nil, err
	}

	if err := s.wallets.Update(ctx, wallet); err != nil {
		return nil, err
	}

	if err := s.wagers.Update(ctx, transaction); err != nil {
		return nil, err
	}

	entry, err := domain.NewWalletLedgerEntry(
		uuid.New(),
		wallet.ID(),
		transaction.ID(),
		direction,
		transaction.Amount(),
		before,
		after,
		now,
	)
	if err != nil {
		return nil, err
	}

	if err := s.ledger.Create(ctx, entry); err != nil {
		return nil, err
	}

	if err := s.createProcessedEvent(ctx, transaction, metadata, now); err != nil {
		return nil, err
	}

	if err := s.createBalanceChangedEvent(ctx, wallet, entry, metadata, now); err != nil {
		return nil, err
	}

	return resultFromWager(transaction, false), nil
}

// reject marks and persists an auditable business rejection.
func (s *WagerService) reject(
	ctx context.Context,
	transaction *domain.WagerTransaction,
	rejection wagerRejection,
	metadata CommandMetadata,
	now time.Time,
) (*ProcessWagerResult, error) {
	if err := transaction.MarkRejected(rejection.code, rejection.message, now); err != nil {
		return nil, err
	}

	if err := s.wagers.Update(ctx, transaction); err != nil {
		return nil, err
	}

	if err := s.createRejectedEvent(ctx, transaction, metadata, now); err != nil {
		return nil, err
	}

	return resultFromWager(transaction, false), nil
}

// insufficientFundsRejection distinguishes a bet without funds from a reversal without funds.
func insufficientFundsRejection(transaction *domain.WagerTransaction) wagerRejection {
	if transaction.Type() == domain.WagerTransactionTypeRollback {
		return wagerRejection{
			code:    FailureCodeReversalInsufficientFunds,
			message: "insufficient funds for reversal",
		}
	}

	return wagerRejection{
		code:    FailureCodeBetInsufficientFunds,
		message: "insufficient funds for bet",
	}
}
