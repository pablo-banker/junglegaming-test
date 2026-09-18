package httptransport

import "go.uber.org/fx"

var Module = fx.Module(
	"http",
	fx.Provide(
		NewHealthHandler,
		NewRouter,
	),

	fx.Invoke(
		StartServer,
	),
)
