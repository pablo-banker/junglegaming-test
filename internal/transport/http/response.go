package httptransport

import (
	"errors"
	"log/slog"

	"github.com/gofiber/fiber/v3"
	"github.com/pablo-banker/junglegaming-test/internal/apierrors"
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

	if fiberErr, ok := errors.AsType[*fiber.Error](err); ok {
		switch fiberErr.Code {
		case fiber.StatusNotFound:
			return apierrors.ErrNotFound

		case fiber.StatusMethodNotAllowed:
			return apierrors.ErrMethodNotAllowed

		case fiber.StatusBadRequest:
			return apierrors.ErrInvalidPayload
		}
	}

	return apierrors.ErrInternal.WithCause(err)
}
