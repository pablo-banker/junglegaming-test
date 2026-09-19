package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/pablo-banker/junglegaming-test/internal/domain"
)

const (
	failureCodeBetInsufficientFunds      = "BET_INSUFFICIENT_FUNDS"
	failureCodeReversalInsufficientFunds = "REVERSAL_INSUFFICIENT_FUNDS"
	failureCodeReferenceNotProcessable   = "REFERENCE_NOT_PROCESSABLE"
	failureCodeReferenceNotFound         = "REFERENCE_NOT_FOUND"
	failureCodeDuplicateReversal         = "DUPLICATE_REVERSAL"

	initialReferenceRetryDelay = 5 * time.Second
	maxReferenceRetryDelay     = 5 * time.Minute
	referenceRetryTTL          = 24 * time.Hour
)

type ProcessWagerResult struct {
	TransactionID         uuid.UUID                     `json:"transactionId"`
	ExternalTransactionID string                        `json:"externalTransactionId"`
	Status                domain.WagerTransactionStatus `json:"status"`
	BalanceBefore         *domain.Money                 `json:"balanceBefore,omitempty"`
	BalanceAfter          *domain.Money                 `json:"balanceAfter,omitempty"`
	FailureCode           string                        `json:"failureCode,omitempty"`
	FailureMessage        string                        `json:"failureMessage,omitempty"`
	IdempotentReplay      bool                          `json:"idempotentReplay"`
}

type WagerResult struct {
	TransactionID                  uuid.UUID                     `json:"transactionId"`
	ProviderID                     string                        `json:"providerId"`
	ExternalTransactionID          string                        `json:"externalTransactionId"`
	WalletID                       uuid.UUID                     `json:"walletId"`
	PlayerID                       uuid.UUID                     `json:"playerId"`
	RoundID                        string                        `json:"roundId"`
	GameID                         string                        `json:"gameId"`
	Type                           domain.WagerTransactionType   `json:"type"`
	Amount                         domain.Money                  `json:"money"`
	ReferenceExternalTransactionID string                        `json:"referenceExternalTransactionId,omitempty"`
	Status                         domain.WagerTransactionStatus `json:"status"`
	FailureCode                    string                        `json:"failureCode,omitempty"`
	FailureMessage                 string                        `json:"failureMessage,omitempty"`
	BalanceBefore                  *domain.Money                 `json:"balanceBefore,omitempty"`
	BalanceAfter                   *domain.Money                 `json:"balanceAfter,omitempty"`
	CreatedAt                      time.Time                     `json:"createdAt"`
	UpdatedAt                      time.Time                     `json:"updatedAt"`
}

type WagerService struct {
	txManager TransactionManager
	clock     Clock
	wallets   WalletRepository
	wagers    WagerRepository
	ledger    WalletLedgerRepository
	outbox    OutboxRepository
}

// NewWagerService creates the wager application service.
func NewWagerService(
	txManager TransactionManager,
	clock Clock,
	wallets WalletRepository,
	wagers WagerRepository,
	ledger WalletLedgerRepository,
	outbox OutboxRepository,
) *WagerService {
	return &WagerService{
		txManager: txManager,
		clock:     clock,
		wallets:   wallets,
		wagers:    wagers,
		ledger:    ledger,
		outbox:    outbox,
	}
}

// Process executes a wager operation atomically.
func (s *WagerService) Process(ctx context.Context, command ProcessWagerCommand, metadata CommandMetadata) (*ProcessWagerResult, error) {
	if strings.TrimSpace(metadata.CorrelationID) == "" {
		return nil, ErrInvalidCorrelationID
	}

	walletID, err := uuid.Parse(command.WalletID)
	if err != nil || walletID == uuid.Nil {
		return nil, domain.ErrInvalidWalletID
	}

	playerID, err := uuid.Parse(command.PlayerID)
	if err != nil || playerID == uuid.Nil {
		return nil, domain.ErrInvalidPlayerID
	}

	currency, err := domain.NewCurrency(command.Currency)
	if err != nil {
		return nil, err
	}

	amount, err := domain.ParseMoney(command.Amount, currency)
	if err != nil {
		return nil, err
	}

	transactionType := domain.WagerTransactionType(command.Type)
	if !transactionType.IsExternal() {
		return nil, domain.ErrInvalidWagerType
	}

	payloadHash, err := calculateWagerPayloadHash(command, walletID, playerID, transactionType, amount)
	if err != nil {
		return nil, err
	}

	var result *ProcessWagerResult

	err = s.txManager.WithinTransaction(
		ctx,
		func(txCtx context.Context) error {
			now, err := s.clock.Now(txCtx)
			if err != nil {
				return err
			}

			transaction, err := domain.NewExternalWagerTransaction(
				domain.NewExternalWagerTransactionParams{
					ID:                             uuid.New(),
					ProviderID:                     command.ProviderID,
					ExternalTransactionID:          command.ExternalTransactionID,
					IdempotencyKey:                 command.IdempotencyKey,
					PayloadHash:                    payloadHash,
					WalletID:                       walletID,
					PlayerID:                       playerID,
					RoundID:                        command.RoundID,
					GameID:                         command.GameID,
					Type:                           transactionType,
					Amount:                         amount,
					ReferenceExternalTransactionID: command.ReferenceExternalTransactionID,
					CreatedAt:                      now,
				},
			)
			if err != nil {
				return err
			}

			if err := s.wagers.Create(txCtx, transaction); err != nil {
				if !errors.Is(err, ErrAlreadyExists) {
					return err
				}

				existing, err := s.resolveExistingTransaction(txCtx, command, payloadHash)
				if err != nil {
					return err
				}

				result = existing

				return nil
			}

			referenceResult, err := s.resolveReference(txCtx, transaction, metadata, now)
			if err != nil {
				return err
			}

			if referenceResult != nil {
				result = referenceResult

				return nil
			}

			wallet, err := s.wallets.FindByIDForUpdate(txCtx, walletID)
			if err != nil {
				if errors.Is(err, ErrNotFound) {
					return ErrWalletNotFound
				}

				return err
			}

			if wallet.PlayerID() != playerID {
				return ErrWalletMismatch
			}

			if !wallet.Currency().Equal(currency) {
				return domain.ErrCurrencyMismatch
			}

			if transactionType == domain.WagerTransactionTypeRefund ||
				transactionType == domain.WagerTransactionTypeRollback {
				duplicate, err := s.wagers.HasProcessedDirectReversal(txCtx, transaction.ReferenceTransactionID())
				if err != nil {
					return err
				}

				if duplicate {
					result, err = s.reject(
						txCtx,
						transaction,
						failureCodeDuplicateReversal,
						"referenced transaction already has a successful direct reversal",
						metadata,
						now,
					)

					return err
				}
			}

			result, err = s.processWalletMovement(txCtx, wallet, transaction, metadata, now)

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

// RetryNextPendingReference retries one due pending reference transaction.
func (s *WagerService) RetryNextPendingReference(
	ctx context.Context,
) (bool, error) {
	processed := false

	err := s.txManager.WithinTransaction(
		ctx,
		func(txCtx context.Context) error {
			now, err := s.clock.Now(txCtx)
			if err != nil {
				return err
			}

			work, err := s.wagers.FindDuePendingReferenceForUpdate(
				txCtx,
				now,
			)
			if err != nil {
				if errors.Is(err, ErrNotFound) {
					return nil
				}

				return err
			}

			processed = true

			return s.retryPendingReference(
				txCtx,
				work,
				now,
			)
		},
	)
	if err != nil {
		return false, err
	}

	return processed, nil
}

// resolveExistingTransaction handles persisted idempotency and external transaction conflicts.
func (s *WagerService) resolveExistingTransaction(ctx context.Context, command ProcessWagerCommand, payloadHash string) (*ProcessWagerResult, error) {
	existing, err := s.wagers.FindByProviderAndIdempotencyKey(ctx, command.ProviderID, command.IdempotencyKey)
	if err == nil {
		if existing.PayloadHash() != payloadHash {
			return nil, ErrIdempotencyConflict
		}

		return resultFromWager(existing, true), nil
	}

	if !errors.Is(err, ErrNotFound) {
		return nil, err
	}

	existing, err = s.wagers.FindByProviderAndExternalTransactionID(ctx, command.ProviderID, command.ExternalTransactionID)
	if err == nil {
		return nil, ErrExternalTransactionConflict
	}

	if !errors.Is(err, ErrNotFound) {
		return nil, err
	}

	return nil, ErrAlreadyExists
}

// resolveReference resolves or persists a transaction waiting for a reference.
func (s *WagerService) resolveReference(ctx context.Context, transaction *domain.WagerTransaction, metadata CommandMetadata, now time.Time) (*ProcessWagerResult, error) {
	if strings.TrimSpace(transaction.ReferenceExternalTransactionID()) == "" {
		return nil, nil
	}

	reference, err := s.wagers.FindByProviderAndExternalTransactionID(ctx, transaction.ProviderID(), transaction.ReferenceExternalTransactionID())
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return s.markPendingReference(ctx, transaction, metadata, now)
		}

		return nil, err
	}

	switch reference.Status() {
	case domain.WagerTransactionStatusProcessed:
		if err := transaction.ResolveReference(reference, now); err != nil {
			return nil, err
		}

		return nil, nil

	case domain.WagerTransactionStatusPending,
		domain.WagerTransactionStatusPendingReference:
		return s.markPendingReference(ctx, transaction, metadata, now)

	case domain.WagerTransactionStatusRejected,
		domain.WagerTransactionStatusFailed:
		return s.reject(
			ctx,
			transaction,
			failureCodeReferenceNotProcessable,
			"referenced transaction did not complete successfully",
			metadata,
			now,
		)

	default:
		return nil, domain.ErrInvalidWagerState
	}
}

// markPendingReference persists a transaction waiting for its reference.
func (s *WagerService) markPendingReference(ctx context.Context, transaction *domain.WagerTransaction, metadata CommandMetadata, now time.Time) (*ProcessWagerResult, error) {
	if err := transaction.MarkPendingReference(now); err != nil {
		return nil, err
	}

	nextAttemptAt := now.Add(initialReferenceRetryDelay)
	expiresAt := now.Add(referenceRetryTTL)

	if err := s.wagers.UpdatePendingReference(ctx, transaction, nextAttemptAt, expiresAt, metadata); err != nil {
		return nil, err
	}

	event := EventEnvelope{
		EventID:       uuid.New(),
		EventType:     EventTypeWagerTransactionPendingReference,
		AggregateID:   transaction.ID(),
		CorrelationID: metadata.CorrelationID,
		CausationID:   metadata.CausationID,
		Version:       EventVersion,
		OccurredAt:    now,
		Payload: WagerTransactionPendingReferencePayload{
			TransactionID:                  transaction.ID(),
			ProviderID:                     transaction.ProviderID(),
			ExternalTransactionID:          transaction.ExternalTransactionID(),
			ReferenceExternalTransactionID: transaction.ReferenceExternalTransactionID(),
			WalletID:                       transaction.WalletID(),
			PlayerID:                       transaction.PlayerID(),
			RoundID:                        transaction.RoundID(),
			GameID:                         transaction.GameID(),
			Type:                           transaction.Type(),
			Amount:                         transaction.Amount(),
		},
	}

	if err := s.outbox.Create(ctx, event); err != nil {
		return nil, err
	}

	return resultFromWager(transaction, false), nil
}

// processWalletMovement applies the wager operation to the wallet.
func (s *WagerService) processWalletMovement(ctx context.Context, wallet *domain.Wallet, transaction *domain.WagerTransaction, metadata CommandMetadata, now time.Time) (*ProcessWagerResult, error) {
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
		failureCode := failureCodeBetInsufficientFunds
		failureMessage := "insufficient funds for bet"

		if transaction.Type() == domain.WagerTransactionTypeRollback {
			failureCode = failureCodeReversalInsufficientFunds
			failureMessage = "insufficient funds for reversal"
		}

		return s.reject(ctx, transaction, failureCode, failureMessage, metadata, now)
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

	if err := s.createBalanceChangedEvent(ctx, wallet, transaction, entry, metadata, now); err != nil {
		return nil, err
	}

	return resultFromWager(transaction, false), nil
}

// reject marks and persists an auditable business rejection.
func (s *WagerService) reject(
	ctx context.Context,
	transaction *domain.WagerTransaction,
	failureCode string,
	failureMessage string,
	metadata CommandMetadata,
	now time.Time,
) (*ProcessWagerResult, error) {
	if err := transaction.MarkRejected(failureCode, failureMessage, now); err != nil {
		return nil, err
	}

	if err := s.wagers.Update(ctx, transaction); err != nil {
		return nil, err
	}

	event := EventEnvelope{
		EventID:       uuid.New(),
		EventType:     EventTypeWagerTransactionRejected,
		AggregateID:   transaction.ID(),
		CorrelationID: metadata.CorrelationID,
		CausationID:   metadata.CausationID,
		Version:       EventVersion,
		OccurredAt:    now,
		Payload: WagerTransactionRejectedPayload{
			TransactionID:         transaction.ID(),
			ProviderID:            transaction.ProviderID(),
			ExternalTransactionID: transaction.ExternalTransactionID(),
			WalletID:              transaction.WalletID(),
			PlayerID:              transaction.PlayerID(),
			RoundID:               transaction.RoundID(),
			GameID:                transaction.GameID(),
			Type:                  transaction.Type(),
			Amount:                transaction.Amount(),
			FailureCode:           transaction.FailureCode(),
			FailureMessage:        transaction.FailureMessage(),
		},
	}

	if err := s.outbox.Create(ctx, event); err != nil {
		return nil, err
	}

	return resultFromWager(transaction, false), nil
}

// createProcessedEvent persists the wager processed event.
func (s *WagerService) createProcessedEvent(ctx context.Context, transaction *domain.WagerTransaction, metadata CommandMetadata, now time.Time) error {
	before, hasBefore := transaction.BalanceBefore()
	after, hasAfter := transaction.BalanceAfter()

	if !hasBefore || !hasAfter {
		return domain.ErrInvalidWagerState
	}

	var referenceTransactionID *uuid.UUID

	if id := transaction.ReferenceTransactionID(); id != uuid.Nil {
		referenceID := id
		referenceTransactionID = &referenceID
	}

	event := EventEnvelope{
		EventID:       uuid.New(),
		EventType:     EventTypeWagerTransactionProcessed,
		AggregateID:   transaction.ID(),
		CorrelationID: metadata.CorrelationID,
		CausationID:   metadata.CausationID,
		Version:       EventVersion,
		OccurredAt:    now,
		Payload: WagerTransactionProcessedPayload{
			TransactionID:          transaction.ID(),
			ProviderID:             transaction.ProviderID(),
			ExternalTransactionID:  transaction.ExternalTransactionID(),
			WalletID:               transaction.WalletID(),
			PlayerID:               transaction.PlayerID(),
			RoundID:                transaction.RoundID(),
			GameID:                 transaction.GameID(),
			Type:                   transaction.Type(),
			Amount:                 transaction.Amount(),
			BalanceBefore:          before,
			BalanceAfter:           after,
			ReferenceTransactionID: referenceTransactionID,
		},
	}

	return s.outbox.Create(ctx, event)
}

// createBalanceChangedEvent persists the wallet balance changed event.
func (s *WagerService) createBalanceChangedEvent(
	ctx context.Context,
	wallet *domain.Wallet,
	transaction *domain.WagerTransaction,
	entry *domain.WalletLedgerEntry,
	metadata CommandMetadata,
	now time.Time,
) error {
	event := EventEnvelope{
		EventID:       uuid.New(),
		EventType:     EventTypeWalletBalanceChanged,
		AggregateID:   wallet.ID(),
		CorrelationID: metadata.CorrelationID,
		CausationID:   metadata.CausationID,
		Version:       EventVersion,
		OccurredAt:    now,
		Payload: WalletBalanceChangedPayload{
			WalletID:      wallet.ID(),
			PlayerID:      wallet.PlayerID(),
			TransactionID: transaction.ID(),
			Direction:     entry.Direction(),
			Amount:        entry.Amount(),
			BalanceBefore: entry.BalanceBefore(),
			BalanceAfter:  entry.BalanceAfter(),
			WalletVersion: wallet.Version(),
		},
	}

	return s.outbox.Create(ctx, event)
}

// retryPendingReference attempts to resolve one persisted pending reference.
func (s *WagerService) retryPendingReference(
	ctx context.Context,
	work *PendingReferenceWork,
	now time.Time,
) error {
	transaction := work.Transaction
	metadata := work.Metadata

	if !now.Before(work.ExpiresAt) {
		_, err := s.reject(
			ctx,
			transaction,
			failureCodeReferenceNotFound,
			"referenced transaction was not found before expiration",
			metadata,
			now,
		)

		return err
	}

	reference, err := s.wagers.FindByProviderAndExternalTransactionID(
		ctx,
		transaction.ProviderID(),
		transaction.ReferenceExternalTransactionID(),
	)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return s.schedulePendingReferenceRetry(
				ctx,
				transaction,
				work.RetryCount,
				now,
			)
		}

		return err
	}

	switch reference.Status() {
	case domain.WagerTransactionStatusProcessed:
		return s.processResolvedPendingReference(
			ctx,
			transaction,
			reference,
			metadata,
			now,
		)

	case domain.WagerTransactionStatusPending,
		domain.WagerTransactionStatusPendingReference:
		return s.schedulePendingReferenceRetry(
			ctx,
			transaction,
			work.RetryCount,
			now,
		)

	case domain.WagerTransactionStatusRejected,
		domain.WagerTransactionStatusFailed:
		_, err := s.reject(
			ctx,
			transaction,
			failureCodeReferenceNotProcessable,
			"referenced transaction did not complete successfully",
			metadata,
			now,
		)

		return err

	default:
		return domain.ErrInvalidWagerState
	}
}

// schedulePendingReferenceRetry schedules the next exponential reference retry.
func (s *WagerService) schedulePendingReferenceRetry(
	ctx context.Context,
	transaction *domain.WagerTransaction,
	retryCount int,
	now time.Time,
) error {
	nextRetryCount := retryCount + 1
	nextAttemptAt := now.Add(referenceRetryDelay(nextRetryCount))

	return s.wagers.SchedulePendingReferenceRetry(
		ctx,
		transaction.ID(),
		nextRetryCount,
		nextAttemptAt,
	)
}

// processResolvedPendingReference processes a pending wager after its reference becomes available.
func (s *WagerService) processResolvedPendingReference(
	ctx context.Context,
	transaction *domain.WagerTransaction,
	reference *domain.WagerTransaction,
	metadata CommandMetadata,
	now time.Time,
) error {
	if err := transaction.ResolveReference(reference, now); err != nil {
		return err
	}

	wallet, err := s.wallets.FindByIDForUpdate(
		ctx,
		transaction.WalletID(),
	)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return ErrWalletNotFound
		}

		return err
	}

	if wallet.PlayerID() != transaction.PlayerID() {
		return ErrWalletMismatch
	}

	if !wallet.Currency().Equal(transaction.Amount().Currency()) {
		return domain.ErrCurrencyMismatch
	}

	if transaction.Type() == domain.WagerTransactionTypeRefund ||
		transaction.Type() == domain.WagerTransactionTypeRollback {

		duplicate, err := s.wagers.HasProcessedDirectReversal(
			ctx,
			transaction.ReferenceTransactionID(),
		)
		if err != nil {
			return err
		}

		if duplicate {
			_, err := s.reject(
				ctx,
				transaction,
				failureCodeDuplicateReversal,
				"referenced transaction already has a successful direct reversal",
				metadata,
				now,
			)

			return err
		}
	}

	_, err = s.processWalletMovement(
		ctx,
		wallet,
		transaction,
		metadata,
		now,
	)

	return err
}

// referenceRetryDelay calculates the exponential pending reference retry delay.
func referenceRetryDelay(retryCount int) time.Duration {
	delay := initialReferenceRetryDelay

	for i := 0; i < retryCount; i++ {
		if delay >= maxReferenceRetryDelay/2 {
			return maxReferenceRetryDelay
		}

		delay *= 2
	}

	return delay
}

// resultFromWager builds the application response from persisted wager state.
func resultFromWager(transaction *domain.WagerTransaction, idempotentReplay bool) *ProcessWagerResult {
	result := &ProcessWagerResult{
		TransactionID:         transaction.ID(),
		ExternalTransactionID: transaction.ExternalTransactionID(),
		Status:                transaction.Status(),
		FailureCode:           transaction.FailureCode(),
		FailureMessage:        transaction.FailureMessage(),
		IdempotentReplay:      idempotentReplay,
	}

	if before, ok := transaction.BalanceBefore(); ok {
		result.BalanceBefore = &before
	}

	if after, ok := transaction.BalanceAfter(); ok {
		result.BalanceAfter = &after
	}

	return result
}

// wagerResultFromTransaction builds a query result from persisted wager state.
func wagerResultFromTransaction(transaction *domain.WagerTransaction) *WagerResult {
	result := &WagerResult{
		TransactionID:                  transaction.ID(),
		ProviderID:                     transaction.ProviderID(),
		ExternalTransactionID:          transaction.ExternalTransactionID(),
		WalletID:                       transaction.WalletID(),
		PlayerID:                       transaction.PlayerID(),
		RoundID:                        transaction.RoundID(),
		GameID:                         transaction.GameID(),
		Type:                           transaction.Type(),
		Amount:                         transaction.Amount(),
		ReferenceExternalTransactionID: transaction.ReferenceExternalTransactionID(),
		Status:                         transaction.Status(),
		FailureCode:                    transaction.FailureCode(),
		FailureMessage:                 transaction.FailureMessage(),
		CreatedAt:                      transaction.CreatedAt(),
		UpdatedAt:                      transaction.UpdatedAt(),
	}

	if before, ok := transaction.BalanceBefore(); ok {
		result.BalanceBefore = &before
	}

	if after, ok := transaction.BalanceAfter(); ok {
		result.BalanceAfter = &after
	}

	return result
}

// calculateWagerPayloadHash creates a deterministic hash of the business payload.
func calculateWagerPayloadHash(command ProcessWagerCommand, walletID uuid.UUID, playerID uuid.UUID, transactionType domain.WagerTransactionType, amount domain.Money) (string, error) {
	payload := struct {
		ProviderID                     string `json:"providerId"`
		ExternalTransactionID          string `json:"externalTransactionId"`
		WalletID                       string `json:"walletId"`
		PlayerID                       string `json:"playerId"`
		RoundID                        string `json:"roundId"`
		GameID                         string `json:"gameId"`
		Type                           string `json:"type"`
		Amount                         string `json:"amount"`
		Currency                       string `json:"currency"`
		ReferenceExternalTransactionID string `json:"referenceExternalTransactionId,omitempty"`
	}{
		ProviderID:                     command.ProviderID,
		ExternalTransactionID:          command.ExternalTransactionID,
		WalletID:                       walletID.String(),
		PlayerID:                       playerID.String(),
		RoundID:                        command.RoundID,
		GameID:                         command.GameID,
		Type:                           string(transactionType),
		Amount:                         amount.Amount(),
		Currency:                       amount.Currency().Code(),
		ReferenceExternalTransactionID: command.ReferenceExternalTransactionID,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	hash := sha256.Sum256(data)

	return hex.EncodeToString(hash[:]), nil
}
