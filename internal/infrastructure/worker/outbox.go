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
	outboxIdleDelay  = time.Second
	outboxErrorDelay = time.Second
)

type outboxDispatcher interface {
	DispatchNext(ctx context.Context) (bool, error)
}

type OutboxWorker struct {
	service outboxDispatcher
	logger  *slog.Logger

	mu     sync.Mutex
	cancel context.CancelFunc
	done   chan struct{}
}

// NewOutboxWorker creates the background outbox publishing worker.
func NewOutboxWorker(
	service *application.OutboxDispatcherService,
	logger *slog.Logger,
) *OutboxWorker {
	return &OutboxWorker{
		service: service,
		logger:  logger,
	}
}

// Start starts the outbox publishing loop.
func (w *OutboxWorker) Start() {
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

// Stop stops the outbox worker and waits for completion.
func (w *OutboxWorker) Stop(ctx context.Context) error {
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

// run continuously publishes pending outbox events.
func (w *OutboxWorker) run(ctx context.Context) {
	defer close(w.done)

	for {
		processed, err := w.service.DispatchNext(ctx)

		if errors.Is(err, context.Canceled) || ctx.Err() != nil {
			return
		}

		if err != nil {
			w.logger.Error(
				"outbox publication failed",
				"error",
				err,
			)

			if !waitOutbox(ctx, outboxErrorDelay) {
				return
			}

			continue
		}

		if processed {
			continue
		}

		if !waitOutbox(ctx, outboxIdleDelay) {
			return
		}
	}
}

// waitOutbox waits for another worker iteration or cancellation.
func waitOutbox(ctx context.Context, delay time.Duration) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-timer.C:
		return true

	case <-ctx.Done():
		return false
	}
}

// RegisterOutboxWorker registers the outbox worker with the application lifecycle.
func RegisterOutboxWorker(lifecycle fx.Lifecycle, worker *OutboxWorker) {
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
