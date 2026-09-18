package application

import "errors"

var (
	ErrInvalidCorrelationID        = errors.New("invalid correlation id")
	ErrNotFound                    = errors.New("not found")
	ErrAlreadyExists               = errors.New("already exists")
	ErrIdempotencyConflict         = errors.New("idempotency conflict")
	ErrExternalTransactionConflict = errors.New("external transaction conflict")
	ErrWalletNotFound              = errors.New("wallet not found")
	ErrWalletMismatch              = errors.New("wallet does not match wager")
)
