package observability

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"go.uber.org/fx"
)

var Module = fx.Module(
	"observability",
	fx.Provide(
		NewLogger,
		NewRegistry,
		fx.Annotate(
			NewMetrics,
			fx.From(new(*prometheus.Registry)),
		),
	),
)

// NewRegistry creates the Prometheus registry with Go runtime and process metrics.
func NewRegistry() *prometheus.Registry {
	registry := prometheus.NewRegistry()

	registry.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)

	return registry
}
