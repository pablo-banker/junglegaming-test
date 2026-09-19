package application

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	"github.com/pablo-banker/junglegaming-test/internal/domain"
)

// ReferenceRetryPolicy controls how PENDING_REFERENCE transactions are retried.
type ReferenceRetryPolicy struct {
	// InitialDelay is the wait before the first retry; every retry doubles it.
	InitialDelay time.Duration

	// MaxDelay caps the exponential delay.
	MaxDelay time.Duration

	// TTL is how long a reference may take to arrive before REFERENCE_NOT_FOUND.
	TTL time.Duration
}

// DefaultReferenceRetryPolicy returns the production retry policy.
func DefaultReferenceRetryPolicy() ReferenceRetryPolicy {
	return ReferenceRetryPolicy{
		InitialDelay: 5 * time.Second,
		MaxDelay:     5 * time.Minute,
		TTL:          24 * time.Hour,
	}
}

// delay returns the exponential wait before the given retry attempt (1-based).
func (p ReferenceRetryPolicy) delay(attempt int) time.Duration {
	delay := p.InitialDelay

	for i := 1; i < attempt && delay < p.MaxDelay; i++ {
		delay *= 2
	}

	return min(delay, p.MaxDelay)
}

// nextAttemptAt schedules a retry without going past the expiration.
func (p ReferenceRetryPolicy) nextAttemptAt(now time.Time, expiresAt time.Time, attempt int) time.Time {
	next := now.Add(p.delay(attempt))

	if next.After(expiresAt) {
		return expiresAt
	}

	return next
}

// referenceOutcome describes whether a transaction reference can be applied now.
type referenceOutcome struct {
	available bool
	rejection *wagerRejection
}

// PendingReferenceRetry is the outcome of one pending reference retry.
// Status stays PENDING_REFERENCE when the retry was rescheduled.
type PendingReferenceRetry struct {
	TransactionID uuid.UUID
	Kind          domain.WagerTransactionType
	Status        domain.WagerTransactionStatus
	FailureCode   string
}

// RetryNextPendingReference retries one due pending reference transaction and returns
// nil when nothing is due.
//
// Transient failures leave the transaction untouched for the next cycle. Any other
// unexpected failure is deterministic and would block the queue forever, so the
// transaction is finished as FAILED for audit.
func (s *WagerService) RetryNextPendingReference(ctx context.Context) (*PendingReferenceRetry, error) {
	var claimed *domain.WagerTransaction

	err := s.txManager.WithinTransaction(
		ctx,
		func(txCtx context.Context) error {
			claimed = nil

			now, err := s.clock.Now(txCtx)
			if err != nil {
				return err
			}

			work, err := s.wagers.FindDuePendingReferenceForUpdate(txCtx, now)
			if err != nil {
				if errors.Is(err, ErrNotFound) {
					return nil
				}

				return err
			}

			claimed = work.Transaction

			return s.retryPendingReference(txCtx, work)
		},
	)

	if claimed == nil {
		return nil, err
	}

	if err == nil {
		return pendingReferenceRetry(claimed), nil
	}

	retry := &PendingReferenceRetry{
		TransactionID: claimed.ID(),
		Kind:          claimed.Type(),
		Status:        domain.WagerTransactionStatusPendingReference,
	}

	if ctx.Err() != nil || IsTransient(err) {
		return retry, err
	}

	failed, failErr := s.failPendingReference(ctx, claimed.ID())
	if failErr != nil {
		return retry, errors.Join(err, failErr)
	}

	if failed != nil {
		retry = pendingReferenceRetry(failed)
	}

	return retry, err
}

// pendingReferenceRetry describes the state of a retried transaction.
func pendingReferenceRetry(transaction *domain.WagerTransaction) *PendingReferenceRetry {
	return &PendingReferenceRetry{
		TransactionID: transaction.ID(),
		Kind:          transaction.Type(),
		Status:        transaction.Status(),
		FailureCode:   transaction.FailureCode(),
	}
}

// retryPendingReference attempts to resolve and apply one locked pending reference.
func (s *WagerService) retryPendingReference(ctx context.Context, work *PendingReferenceWork) error {
	transaction := work.Transaction

	wallet, err := s.lockWallet(ctx, transaction)
	if err != nil {
		return err
	}

	now, err := s.clock.Now(ctx)
	if err != nil {
		return err
	}

	if !now.Before(work.ExpiresAt) {
		_, err := s.reject(ctx, transaction, wagerRejection{
			code:    FailureCodeReferenceNotFound,
			message: "referenced transaction was not processed before expiration",
		}, work.Metadata, now)

		return err
	}

	outcome, err := s.resolveReference(ctx, transaction, now)
	if err != nil {
		return err
	}

	if outcome.rejection != nil {
		_, err := s.reject(ctx, transaction, *outcome.rejection, work.Metadata, now)

		return err
	}

	if !outcome.available {
		retryCount := work.RetryCount + 1

		return s.wagers.SchedulePendingReferenceRetry(
			ctx,
			transaction.ID(),
			retryCount,
			s.policy.nextAttemptAt(now, work.ExpiresAt, retryCount+1),
		)
	}

	_, err = s.settle(ctx, wallet, transaction, work.Metadata, now)

	return err
}

// failPendingReference finishes a pending reference as FAILED after a permanent processing
// error. It returns nil when another instance already finished the transaction.
func (s *WagerService) failPendingReference(ctx context.Context, transactionID uuid.UUID) (*domain.WagerTransaction, error) {
	var failed *domain.WagerTransaction

	err := s.txManager.WithinTransaction(
		ctx,
		func(txCtx context.Context) error {
			failed = nil

			work, err := s.wagers.FindPendingReferenceForUpdate(txCtx, transactionID)
			if err != nil {
				if errors.Is(err, ErrNotFound) {
					return nil
				}

				return err
			}

			now, err := s.clock.Now(txCtx)
			if err != nil {
				return err
			}

			if err := work.Transaction.MarkFailed(
				FailureCodeProcessingFailed,
				"transaction could not be processed; see service logs",
				now,
			); err != nil {
				return err
			}

			if err := s.wagers.Update(txCtx, work.Transaction); err != nil {
				return err
			}

			failed = work.Transaction

			return nil
		},
	)

	return failed, err
}

// resolveReference looks up and validates the transaction reference.
func (s *WagerService) resolveReference(ctx context.Context, transaction *domain.WagerTransaction, now time.Time) (referenceOutcome, error) {
	reference, err := s.wagers.FindByProviderAndExternalTransactionID(
		ctx,
		transaction.ProviderID(),
		transaction.ReferenceExternalTransactionID(),
	)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return referenceOutcome{}, nil
		}

		return referenceOutcome{}, err
	}

	switch reference.Status() {
	case domain.WagerTransactionStatusPending,
		domain.WagerTransactionStatusPendingReference:
		return referenceOutcome{}, nil

	case domain.WagerTransactionStatusRejected,
		domain.WagerTransactionStatusFailed:
		return referenceOutcome{rejection: &wagerRejection{
			code:    FailureCodeReferenceNotProcessable,
			message: "referenced transaction did not complete successfully",
		}}, nil

	case domain.WagerTransactionStatusProcessed:

	default:
		return referenceOutcome{}, domain.ErrInvalidWagerState
	}

	err = transaction.ResolveReference(reference, now)

	switch {
	case err == nil:
		return referenceOutcome{available: true}, nil

	case errors.Is(err, domain.ErrWagerReferenceMismatch):
		return referenceOutcome{rejection: &wagerRejection{
			code:    FailureCodeReferenceMismatch,
			message: "referenced transaction does not match provider, player, wallet, currency, round or amount",
		}}, nil

	case errors.Is(err, domain.ErrInvalidWagerReference):
		return referenceOutcome{rejection: &wagerRejection{
			code:    FailureCodeReferenceTypeNotAllowed,
			message: "referenced transaction type cannot be used by this operation",
		}}, nil

	default:
		return referenceOutcome{}, err
	}
}

// markPendingReference persists a transaction waiting for its reference.
func (s *WagerService) markPendingReference(ctx context.Context, transaction *domain.WagerTransaction, metadata CommandMetadata, now time.Time) (*ProcessWagerResult, error) {
	if err := transaction.MarkPendingReference(now); err != nil {
		return nil, err
	}

	expiresAt := now.Add(s.policy.TTL)
	nextAttemptAt := s.policy.nextAttemptAt(now, expiresAt, 1)

	if err := s.wagers.UpdatePendingReference(ctx, transaction, nextAttemptAt, expiresAt, metadata); err != nil {
		return nil, err
	}

	if err := s.createPendingReferenceEvent(ctx, transaction, metadata, now); err != nil {
		return nil, err
	}

	return resultFromWager(transaction, false), nil
}
