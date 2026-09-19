package application

import "go.uber.org/fx"

// Module provides application services.
var Module = fx.Module(
	"application",
	fx.Provide(
		NewWalletService,
		NewWagerService,
	),
)
