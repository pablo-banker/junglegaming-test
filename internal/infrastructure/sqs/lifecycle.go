package sqs

import (
	"context"

	"go.uber.org/fx"
)

// RegisterWorker registers the SQS worker with the application lifecycle.
func RegisterWorker(lifecycle fx.Lifecycle, worker *Worker) {
	lifecycle.Append(fx.Hook{
		OnStart: func(context.Context) error {
			worker.Start()

			return nil
		},
		OnStop: func(ctx context.Context) error {
			return worker.Stop(ctx)
		},
	})
}
