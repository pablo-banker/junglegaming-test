package wiring

import (
	"github.com/pablo-banker/junglegaming-test/internal/application"
	"github.com/pablo-banker/junglegaming-test/internal/config"
	"github.com/pablo-banker/junglegaming-test/internal/infrastructure/keycloak"
	"github.com/pablo-banker/junglegaming-test/internal/infrastructure/postgres"
	"github.com/pablo-banker/junglegaming-test/internal/observability"
	httptransport "github.com/pablo-banker/junglegaming-test/internal/transport/http"
	"go.uber.org/fx"
)

var App = fx.Options(
	config.Module,
	observability.Module,
	postgres.Module,
	keycloak.Module,
	application.Module,
	httptransport.Module,
)
