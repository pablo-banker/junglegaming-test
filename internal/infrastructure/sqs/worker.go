package sqs

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"
)

const workerRetryDelay = time.Second

type poller interface {
	PollOnce(ctx context.Context) error
}

type Worker struct {
	consumer poller
	logger   *slog.Logger

	mu     sync.Mutex
	cancel context.CancelFunc
	done   chan struct{}
}

// NewWorker creates the background SQS consumer worker.
func NewWorker(consumer *Consumer, logger *slog.Logger) *Worker {
	return &Worker{
		consumer: consumer,
		logger:   logger,
	}
}

// Start starts the SQS polling loop.
func (w *Worker) Start() {
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

// Stop stops the SQS polling loop and waits for it to finish.
func (w *Worker) Stop(ctx context.Context) error {
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

// run continuously polls SQS until the worker is stopped.
func (w *Worker) run(ctx context.Context) {
	defer close(w.done)

	for {
		err := w.consumer.PollOnce(ctx)

		if errors.Is(err, context.Canceled) || ctx.Err() != nil {
			return
		}

		if err != nil {
			w.logger.Error(
				"SQS polling failed",
				"error",
				err,
			)

			select {
			case <-time.After(workerRetryDelay):
				continue
			case <-ctx.Done():
				return
			}
		}
	}
}
