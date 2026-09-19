package keycloak

import (
	"go.uber.org/fx"

	"github.com/pablo-banker/junglegaming-test/internal/auth"
)

// Module provides Keycloak infrastructure dependencies.
var Module = fx.Module(
	"keycloak",
	fx.Provide(
		fx.Annotate(
			NewVerifier,
			fx.As(new(auth.TokenVerifier)),
		),
	),
)
