package httptransport

import (
	"errors"
	"log/slog"

	"github.com/gofiber/fiber/v3"
	"github.com/pablo-banker/junglegaming-test/internal/apierrors"
	"github.com/pablo-banker/junglegaming-test/internal/application"
	"github.com/pablo-banker/junglegaming-test/internal/domain"
)

type Response[T any] struct {
	Data T `json:"data"`
}

type ErrorResponse struct {
	Status      int    `json:"status"`
	Code        string `json:"code"`
	Error       string `json:"error"`
	Details     string `json:"details,omitempty"`
	SupportCode string `json:"supportCode,omitempty"`
}

// BuildSuccessResponse writes a successful response using the standard response envelope.
func BuildSuccessResponse[T any](c fiber.Ctx, status int, data T) error {
	return c.
		Status(status).
		JSON(Response[T]{
			Data: data,
		})
}

// BuildErrorResponse writes an API error using the standard error response format.
func BuildErrorResponse(c fiber.Ctx, logger *slog.Logger, err *apierrors.APIError) error {
	response := ErrorResponse{
		Status:  err.HTTPStatus,
		Code:    err.Code,
		Error:   err.Message,
		Details: err.Details,
	}

	if supportCode, ok := c.Locals("support_code").(string); ok {
		response.SupportCode = supportCode
	}

	if err.Internal != "" {
		logger.ErrorContext(
			c.Context(),
			err.Message,
			slog.String("code", err.Code),
			slog.String("internal", err.Internal),
			slog.String(
				"supportCode",
				response.SupportCode,
			),
		)
	}

	return c.Status(err.HTTPStatus).JSON(response)
}

// ResolveError converts application and Fiber errors into API errors.
func ResolveError(err error) *apierrors.APIError {
	if apiErr, ok := errors.AsType[*apierrors.APIError](err); ok {
		return apiErr
	}

	if errors.Is(err, application.ErrAlreadyExists) ||
		errors.Is(err, application.ErrIdempotencyConflict) ||
		errors.Is(err, application.ErrExternalTransactionConflict) {
		return apierrors.ErrConflict
	}

	if errors.Is(err, application.ErrNotFound) ||
		errors.Is(err, application.ErrWalletNotFound) {
		return apierrors.ErrNotFound
	}

	if errors.Is(err, application.ErrWalletMismatch) ||
		errors.Is(err, application.ErrInvalidWalletReference) {
		return apierrors.ErrValidation
	}

	if errors.Is(err, domain.ErrInvalidCurrency) ||
		errors.Is(err, domain.ErrInvalidMoney) ||
		errors.Is(err, domain.ErrCurrencyMismatch) ||
		errors.Is(err, domain.ErrAmountMustBePositive) ||
		errors.Is(err, domain.ErrInvalidWalletID) ||
		errors.Is(err, domain.ErrInvalidPlayerID) ||
		errors.Is(err, domain.ErrInvalidProviderID) ||
		errors.Is(err, domain.ErrInvalidExternalTransactionID) ||
		errors.Is(err, domain.ErrInvalidIdempotencyKey) ||
		errors.Is(err, domain.ErrInvalidRoundID) ||
		errors.Is(err, domain.ErrInvalidGameID) ||
		errors.Is(err, domain.ErrInvalidWagerType) ||
		errors.Is(err, domain.ErrInvalidWagerAmount) ||
		errors.Is(err, domain.ErrInvalidWagerReference) ||
		errors.Is(err, domain.ErrWagerReferenceMismatch) {
		return apierrors.ErrValidation
	}

	if fiberErr, ok := errors.AsType[*fiber.Error](err); ok {
		switch fiberErr.Code {
		case fiber.StatusBadRequest:
			return apierrors.ErrInvalidPayload
		case fiber.StatusUnauthorized:
			return apierrors.ErrUnauthorized
		case fiber.StatusForbidden:
			return apierrors.ErrForbidden
		case fiber.StatusNotFound:
			return apierrors.ErrNotFound
		case fiber.StatusMethodNotAllowed:
			return apierrors.ErrMethodNotAllowed
		}
	}

	return apierrors.ErrInternal.WithCause(err)
}
