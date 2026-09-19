package httptransport

import (
	"log/slog"

	"github.com/gofiber/fiber/v3"
)

// newErrorHandler writes every error using the API error contract.
// Server errors are logged with their internal cause, which is never returned to clients.
func newErrorHandler(logger *slog.Logger) fiber.ErrorHandler {
	return func(c fiber.Ctx, err error) error {
		apiErr := resolveError(err)

		if apiErr.Status >= fiber.StatusInternalServerError {
			logger.ErrorContext(
				c.Context(),
				"request failed",
				slog.String("code", apiErr.Code),
				slog.String("method", c.Method()),
				slog.String("route", c.Route().Path),
				slog.Any("error", err),
			)
		}

		if apiErr.Status == fiber.StatusServiceUnavailable {
			c.Set(fiber.HeaderRetryAfter, "1")
		}

		return c.Status(apiErr.Status).JSON(apiErr)
	}
}
