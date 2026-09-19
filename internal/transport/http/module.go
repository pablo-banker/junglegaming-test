package httptransport

import "go.uber.org/fx"

var Module = fx.Module(
	"http",
	fx.Provide(
		NewHealthHandler,
		NewAuthMiddleware,
		NewWalletHandler,
		NewWagerHandler,
		NewRouter,
	),

	fx.Invoke(
		StartServer,
	),
)
