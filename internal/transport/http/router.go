package httptransport

import (
	"log/slog"

	"github.com/gofiber/fiber/v3"
	recoverer "github.com/gofiber/fiber/v3/middleware/recover"
)

// NewRouter creates and configures the Fiber application routes and error handling.
func NewRouter(
	healthHandler *HealthHandler,
	authMiddleware *AuthMiddleware,
	walletHandler *WalletHandler,
	wagerHandler *WagerHandler,
	logger *slog.Logger,
) *fiber.App {
	app := fiber.New(fiber.Config{
		AppName: "Jungle Gaming Backend Challenge",
		ErrorHandler: func(c fiber.Ctx, err error) error {
			apiErr := ResolveError(err)

			return BuildErrorResponse(c, logger, apiErr)
		},
	})

	app.Use(recoverer.New())

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
