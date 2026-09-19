package application

import (
	"context"
	"errors"
	"net"
)

var (
	ErrInvalidCorrelationID        = errors.New("invalid correlation id")
	ErrNotFound                    = errors.New("not found")
	ErrAlreadyExists               = errors.New("already exists")
	ErrIdempotencyConflict         = errors.New("idempotency conflict")
	ErrExternalTransactionConflict = errors.New("external transaction conflict")
	ErrWalletNotFound              = errors.New("wallet not found")
	ErrWalletAlreadyExists         = errors.New("wallet already exists for player and currency")
	ErrWalletMismatch              = errors.New("wallet does not match wager")
	ErrInvalidWalletReference      = errors.New("invalid wallet reference")
	ErrInboxMessageConflict        = errors.New("inbox message conflict")
	ErrInboxMessageNotFound        = errors.New("inbox message not found")
	ErrConcurrentUpdate            = errors.New("concurrent update detected")

	// ErrUnavailable marks failures caused by a temporarily unavailable dependency.
	// The same request may succeed later without changes.
	ErrUnavailable = errors.New("dependency temporarily unavailable")
)

// IsTransient reports whether an error is worth retrying later with the same input.
func IsTransient(err error) bool {
	if err == nil {
		return false
	}

	if errors.Is(err, ErrUnavailable) ||
		errors.Is(err, context.DeadlineExceeded) {
		return true
	}

	var netErr net.Error

	return errors.As(err, &netErr)
}
