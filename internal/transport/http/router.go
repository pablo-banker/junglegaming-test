package httptransport

import (
	"log/slog"

	"github.com/gofiber/fiber/v3"
	recoverer "github.com/gofiber/fiber/v3/middleware/recover"
)

// NewRouter creates and configures the Fiber application routes and error handling.
func NewRouter(healthHandler *HealthHandler, logger *slog.Logger) *fiber.App {
	app := fiber.New(fiber.Config{
		AppName: "Jungle Gaming Backend Challenge",
		ErrorHandler: func(c fiber.Ctx, err error) error {
			apiErr := ResolveError(err)

			return BuildErrorResponse(c, logger, apiErr)
		},
	})

	app.Use(
		recoverer.New(),
	)

	registerHealthRoutes(app, healthHandler)

	return app
}

func registerHealthRoutes(app *fiber.App, healthHandler *HealthHandler) {
	health := app.Group("/health")

	health.Get("/live", healthHandler.Live)
	health.Get("/ready", healthHandler.Ready)
}
