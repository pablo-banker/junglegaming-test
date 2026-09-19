package httptransport

import (
	"log/slog"

	"github.com/gofiber/fiber/v3"
)

// newTestFiberApp creates a Fiber app that uses the production error contract.
func newTestFiberApp() *fiber.App {
	return fiber.New(fiber.Config{
		ErrorHandler: newErrorHandler(slog.New(slog.DiscardHandler)),
	})
}
