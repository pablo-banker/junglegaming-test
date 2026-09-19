package worker

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/pablo-banker/junglegaming-test/internal/application"
	"go.uber.org/fx"
)

const (
	pendingReferenceIdleDelay  = time.Second
	pendingReferenceErrorDelay = time.Second
)

type pendingReferenceRetrier interface {
	RetryNextPendingReference(ctx context.Context) (bool, error)
}

type PendingReferenceWorker struct {
	service pendingReferenceRetrier
	logger  *slog.Logger

	mu     sync.Mutex
	cancel context.CancelFunc
	done   chan struct{}
}

// NewPendingReferenceWorker creates the pending reference background worker.
func NewPendingReferenceWorker(service *application.WagerService, logger *slog.Logger) *PendingReferenceWorker {
	return &PendingReferenceWorker{
		service: service,
		logger:  logger,
	}
}

// Start starts the pending reference processing loop.
func (w *PendingReferenceWorker) Start() {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.cancel != nil {
		return
	}

	ctx, cancel := context.WithCancel(context.Background())

	w.cancel = cancel
	w.done = make(chan struct{})

	go w.run(ctx)
}

// Stop stops the pending reference worker and waits for completion.
func (w *PendingReferenceWorker) Stop(ctx context.Context) error {
	w.mu.Lock()

	cancel := w.cancel
	done := w.done

	w.mu.Unlock()

	if cancel == nil {
		return nil
	}

	cancel()

	select {
	case <-done:
		w.mu.Lock()
		w.cancel = nil
		w.done = nil
		w.mu.Unlock()

		return nil

	case <-ctx.Done():
		return ctx.Err()
	}
}

// run continuously processes due pending references.
func (w *PendingReferenceWorker) run(ctx context.Context) {
	defer close(w.done)

	for {
		processed, err := w.service.RetryNextPendingReference(ctx)

		if errors.Is(err, context.Canceled) || ctx.Err() != nil {
			return
		}

		if err != nil {
			w.logger.Error(
				"pending reference retry failed",
				"error",
				err,
			)

			if !waitPendingReference(ctx, pendingReferenceErrorDelay) {
				return
			}

			continue
		}

		if processed {
			continue
		}

		if !waitPendingReference(ctx, pendingReferenceIdleDelay) {
			return
		}
	}
}

// waitPendingReference waits for the next worker iteration or cancellation.
func waitPendingReference(ctx context.Context, delay time.Duration) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-timer.C:
		return true

	case <-ctx.Done():
		return false
	}
}

// RegisterPendingReferenceWorker registers the worker with the application lifecycle.
func RegisterPendingReferenceWorker(
	lifecycle fx.Lifecycle,
	worker *PendingReferenceWorker,
) {
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
