package httptransport

import (
	"log/slog"
	"regexp"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"

	"github.com/pablo-banker/junglegaming-test/internal/observability"
)

const correlationHeader = "X-Correlation-Id"

type correlationContextKey struct{}

// correlationIDPattern accepts client correlation ids that are safe to log and propagate.
var correlationIDPattern = regexp.MustCompile(`^[A-Za-z0-9._:-]{1,128}$`)

// requestContext assigns the request correlation id and writes one access log per request.
// The correlation id is echoed in the response and attached to every log of the request.
func requestContext(logger *slog.Logger) fiber.Handler {
	return func(c fiber.Ctx) error {
		correlationID := c.Get(correlationHeader)
		if !correlationIDPattern.MatchString(correlationID) {
			correlationID = uuid.NewString()
		}

		c.Set(correlationHeader, correlationID)
		c.Locals(correlationContextKey{}, correlationID)
		c.SetContext(observability.WithAttrs(c.Context(), slog.String("correlationId", correlationID)))

		start := time.Now()
		err := c.Next()

		status := c.Response().StatusCode()
		if err != nil {
			status = resolveError(err).Status
		}

		level := slog.LevelInfo
		if c.Route().Path == "/health/live" || c.Route().Path == "/health/ready" || c.Route().Path == "/metrics" {
			level = slog.LevelDebug
		}

		logger.Log(
			c.Context(),
			level,
			"http request",
			slog.String("method", c.Method()),
			slog.String("route", c.Route().Path),
			slog.Int("status", status),
			slog.Duration("duration", time.Since(start)),
		)

		return err
	}
}

// correlationID returns the correlation id assigned by requestContext.
func correlationID(c fiber.Ctx) string {
	if value, ok := c.Locals(correlationContextKey{}).(string); ok {
		return value
	}

	return uuid.NewString()
}
