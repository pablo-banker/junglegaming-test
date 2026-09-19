package worker

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"go.uber.org/fx"

	"github.com/pablo-banker/junglegaming-test/internal/application"
)

// Step performs one unit of work; stop is cancelled at shutdown, work only at its deadline.
type Step func(stop context.Context, work context.Context) (bool, error)

// Options configures the pauses of a Loop.
type Options struct {
	// IdleDelay is the pause after a step that found no work.
	IdleDelay time.Duration

	// MinErrorDelay and MaxErrorDelay bound the exponential pause after failures.
	MinErrorDelay time.Duration
	MaxErrorDelay time.Duration
}

// Loop runs a Step repeatedly until it is stopped.
type Loop struct {
	name    string
	step    Step
	options Options
	logger  *slog.Logger

	mu         sync.Mutex
	stop       context.CancelFunc
	cancelWork context.CancelFunc
	done       chan struct{}
}

// NewLoop creates a background loop.
func NewLoop(name string, step Step, options Options, logger *slog.Logger) *Loop {
	return &Loop{
		name:    name,
		step:    step,
		options: options,
		logger:  logger.With(slog.String("worker", name)),
	}
}

// Start runs the loop in the background. Starting a running loop does nothing.
func (l *Loop) Start() {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.done != nil {
		return
	}

	stopCtx, stop := context.WithCancel(context.Background())
	workCtx, cancelWork := context.WithCancel(context.Background())

	l.stop = stop
	l.cancelWork = cancelWork
	l.done = make(chan struct{})

	go l.run(stopCtx, workCtx, l.done)

	l.logger.Info("worker started")
}

// Stop stops fetching new work and waits for the in-flight step until ctx expires.
func (l *Loop) Stop(ctx context.Context) error {
	l.mu.Lock()

	stop, cancelWork, done := l.stop, l.cancelWork, l.done
	l.stop, l.cancelWork, l.done = nil, nil, nil

	l.mu.Unlock()

	if done == nil {
		return nil
	}

	stop()
	defer cancelWork()

	select {
	case <-done:
		l.logger.Info("worker stopped")

		return nil

	case <-ctx.Done():
		l.logger.Warn("worker stop deadline exceeded, cancelling in-flight work")

		return ctx.Err()
	}
}

// run executes steps until stop is cancelled.
func (l *Loop) run(stop context.Context, work context.Context, done chan struct{}) {
	defer close(done)

	errorDelay := l.options.MinErrorDelay

	for stop.Err() == nil {
		found, err := l.step(stop, work)

		if stop.Err() != nil {
			return
		}

		delay := l.options.IdleDelay

		switch {
		case err != nil:
			l.logStepError(err, errorDelay)

			delay = errorDelay
			errorDelay = min(errorDelay*2, l.options.MaxErrorDelay)

		case found:
			errorDelay = l.options.MinErrorDelay

			continue

		default:
			errorDelay = l.options.MinErrorDelay
		}

		if !sleep(stop, delay) {
			return
		}
	}
}

// logStepError logs transient failures as warnings and anything else as errors.
func (l *Loop) logStepError(err error, retryIn time.Duration) {
	level := slog.LevelError
	if application.IsTransient(err) {
		level = slog.LevelWarn
	}

	l.logger.Log(
		context.Background(),
		level,
		"worker step failed",
		slog.Any("error", err),
		slog.Duration("retryIn", retryIn),
	)
}

// sleep waits for the delay unless ctx is cancelled first.
func sleep(ctx context.Context, delay time.Duration) bool {
	if delay <= 0 {
		return ctx.Err() == nil
	}

	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-timer.C:
		return true

	case <-ctx.Done():
		return false
	}
}

// Register ties the loop to the application lifecycle.
func Register(lifecycle fx.Lifecycle, loop *Loop) {
	lifecycle.Append(fx.Hook{
		OnStart: func(context.Context) error {
			loop.Start()

			return nil
		},
		OnStop: loop.Stop,
	})
}
