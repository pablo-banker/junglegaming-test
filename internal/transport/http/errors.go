package httptransport

import (
	"errors"
	"net/http"

	"github.com/gofiber/fiber/v3"

	"github.com/pablo-banker/junglegaming-test/internal/application"
	"github.com/pablo-banker/junglegaming-test/internal/domain"
)

// APIError is the error contract of the HTTP API. Code is stable and machine readable.
type APIError struct {
	Status  int    `json:"-"`
	Code    string `json:"code"`
	Message string `json:"message"`
	Details string `json:"details,omitempty"`

	cause error
}

// Error returns the public error message.
func (e *APIError) Error() string {
	return e.Message
}

// Unwrap returns the internal cause, which is logged but never exposed.
func (e *APIError) Unwrap() error {
	return e.cause
}

// withDetails returns a copy with public details.
func (e *APIError) withDetails(details string) *APIError {
	clone := *e
	clone.Details = details

	return &clone
}

// withCause returns a copy wrapping the internal cause.
func (e *APIError) withCause(err error) *APIError {
	clone := *e
	clone.cause = err

	return &clone
}

// newAPIError declares an error of the API contract.
func newAPIError(status int, code string, message string) *APIError {
	return &APIError{
		Status:  status,
		Code:    code,
		Message: message,
	}
}

var (
	errInvalidPayload              = newAPIError(http.StatusBadRequest, "INVALID_PAYLOAD", "the request payload is invalid")
	errIdempotencyKeyRequired      = newAPIError(http.StatusBadRequest, "IDEMPOTENCY_KEY_REQUIRED", "the Idempotency-Key header is required")
	errUnauthorized                = newAPIError(http.StatusUnauthorized, "UNAUTHORIZED", "authentication is required")
	errForbidden                   = newAPIError(http.StatusForbidden, "FORBIDDEN", "access denied")
	errNotFound                    = newAPIError(http.StatusNotFound, "NOT_FOUND", "resource not found")
	errMethodNotAllowed            = newAPIError(http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
	errWalletAlreadyExists         = newAPIError(http.StatusConflict, "WALLET_ALREADY_EXISTS", "a wallet already exists for this player and currency")
	errIdempotencyConflict         = newAPIError(http.StatusConflict, "IDEMPOTENCY_CONFLICT", "the idempotency key was already used with a different payload")
	errExternalTransactionConflict = newAPIError(http.StatusConflict, "EXTERNAL_TRANSACTION_CONFLICT", "the external transaction was already received with another idempotency key")
	errConflict                    = newAPIError(http.StatusConflict, "CONFLICT", "the resource already exists")
	errPayloadTooLarge             = newAPIError(http.StatusRequestEntityTooLarge, "PAYLOAD_TOO_LARGE", "the request payload is too large")
	errValidation                  = newAPIError(http.StatusUnprocessableEntity, "VALIDATION_FAILED", "one or more fields are invalid")
	errServiceUnavailable          = newAPIError(http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "a dependency is temporarily unavailable, retry later")
	errInternal                    = newAPIError(http.StatusInternalServerError, "INTERNAL_ERROR", "an unexpected error occurred")
)

// validationErrors are corrigible input errors; their message is safe to expose as details.
var validationErrors = []error{
	domain.ErrInvalidCurrency,
	domain.ErrInvalidMoney,
	domain.ErrCurrencyMismatch,
	domain.ErrAmountMustBePositive,
	domain.ErrInvalidWalletID,
	domain.ErrInvalidPlayerID,
	domain.ErrInvalidProviderID,
	domain.ErrInvalidExternalTransactionID,
	domain.ErrInvalidIdempotencyKey,
	domain.ErrInvalidRoundID,
	domain.ErrInvalidGameID,
	domain.ErrInvalidWagerType,
	domain.ErrInvalidWagerAmount,
	domain.ErrInvalidWagerReference,
	application.ErrWalletMismatch,
	application.ErrInvalidWalletReference,
}

// resolveError maps application, domain and Fiber errors into the API contract.
func resolveError(err error) *APIError {
	if apiErr, ok := errors.AsType[*APIError](err); ok {
		return apiErr
	}

	switch {
	case application.IsTransient(err):
		return errServiceUnavailable.withCause(err)

	case errors.Is(err, application.ErrIdempotencyConflict):
		return errIdempotencyConflict

	case errors.Is(err, application.ErrExternalTransactionConflict):
		return errExternalTransactionConflict

	case errors.Is(err, application.ErrWalletAlreadyExists):
		return errWalletAlreadyExists

	case errors.Is(err, application.ErrAlreadyExists):
		return errConflict

	case errors.Is(err, application.ErrNotFound),
		errors.Is(err, application.ErrWalletNotFound):
		return errNotFound
	}

	for _, validationErr := range validationErrors {
		if errors.Is(err, validationErr) {
			return errValidation.withDetails(validationErr.Error())
		}
	}

	if fiberErr, ok := errors.AsType[*fiber.Error](err); ok {
		switch fiberErr.Code {
		case fiber.StatusBadRequest, fiber.StatusUnprocessableEntity:
			return errInvalidPayload

		case fiber.StatusUnauthorized:
			return errUnauthorized

		case fiber.StatusForbidden:
			return errForbidden

		case fiber.StatusNotFound:
			return errNotFound

		case fiber.StatusMethodNotAllowed:
			return errMethodNotAllowed

		case fiber.StatusRequestEntityTooLarge:
			return errPayloadTooLarge
		}
	}

	return errInternal.withCause(err)
}
