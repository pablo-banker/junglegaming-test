package httptransport

import (
	"log/slog"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/adaptor"
	recoverer "github.com/gofiber/fiber/v3/middleware/recover"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// NewRouter creates and configures the Fiber application routes and error handling.
func NewRouter(
	healthHandler *HealthHandler,
	authMiddleware *AuthMiddleware,
	walletHandler *WalletHandler,
	wagerHandler *WagerHandler,
	logger *slog.Logger,
	registry *prometheus.Registry,
) *fiber.App {
	app := fiber.New(fiber.Config{
		AppName:      "Jungle Gaming Backend Challenge",
		ErrorHandler: newErrorHandler(logger),

		// Requests are small; the limits protect the server from slow clients holding connections.
		BodyLimit:    16 * 1024,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	})

	app.Use(recoverer.New())
	app.Use(requestContext(logger))

	// Operational endpoints are public; they expose no business data.
	app.Get("/metrics", adaptor.HTTPHandler(promhttp.HandlerFor(registry, promhttp.HandlerOpts{})))

	// Health
	health := app.Group("/health")
	health.Get("/live", healthHandler.Live)
	health.Get("/ready", healthHandler.Ready)

	// Wallets
	wallets := app.Group("/wallets", authMiddleware.Authenticate, RequireInternal)
	wallets.Post("", walletHandler.Create)
	wallets.Get("/:walletId", walletHandler.Get)
	wallets.Get("/:walletId/ledger", walletHandler.Ledger)
	wallets.Post("/:walletId/reconciliation", walletHandler.Reconcile)

	// Wager Transactions
	wagers := app.Group("/wagering/transactions", authMiddleware.Authenticate, RequireProvider)
	wagers.Post("", wagerHandler.Create)
	wagers.Get("/:transactionId", wagerHandler.GetByID)

	// Provider Wager Transactions
	providers := app.Group("/providers", authMiddleware.Authenticate, RequireProvider)
	providers.Get("/:providerId/wagering/transactions/:externalTransactionId", wagerHandler.GetByExternalTransactionID)

	return app
}
