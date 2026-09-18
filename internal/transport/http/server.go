package httptransport

import (
	"context"
	"log/slog"
	"net"

	"github.com/gofiber/fiber/v3"
	"github.com/pablo-banker/junglegaming-test/internal/config"
	"go.uber.org/fx"
)

// StartServer registers the Fiber server startup and shutdown hooks.
func StartServer(lifecycle fx.Lifecycle, app *fiber.App, cfg config.Config, logger *slog.Logger) {
	lifecycle.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			listener, err := net.Listen("tcp", cfg.HTTPAddr)
			if err != nil {
				return err
			}

			logger.InfoContext(ctx, "http server listening on", slog.String("address", cfg.HTTPAddr))

			go func() {
				err := app.Listener(listener, fiber.ListenConfig{
					DisableStartupMessage: true,
				})
				if err != nil {
					logger.Error("http server stopped", slog.String("error", err.Error()))
				}
			}()

			return nil
		},

		OnStop: func(ctx context.Context) error {
			logger.InfoContext(ctx, "http server shutting down")

			if err := app.ShutdownWithContext(ctx); err != nil {
				logger.ErrorContext(ctx, "http server shutdown failed", slog.String("error", err.Error()))
				return err
			}

			logger.InfoContext(ctx, "http server stopped")

			return nil
		},
	})

}
