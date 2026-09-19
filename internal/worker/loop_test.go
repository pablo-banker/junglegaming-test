package worker

import (
	"context"
	"errors"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"
)

var fastOptions = Options{
	IdleDelay:     time.Millisecond,
	MinErrorDelay: time.Millisecond,
	MaxErrorDelay: 5 * time.Millisecond,
}

// newTestLoop creates a loop with a discarded logger.
func newTestLoop(step Step) *Loop {
	return NewLoop("test", step, fastOptions, slog.New(slog.DiscardHandler))
}

// TestLoopRunsStepsUntilStopped verifies steps run repeatedly and Stop waits for the loop.
func TestLoopRunsStepsUntilStopped(t *testing.T) {
	var calls atomic.Int32

	loop := newTestLoop(func(context.Context, context.Context) (bool, error) {
		calls.Add(1)

		return false, nil
	})

	loop.Start()
	loop.Start()

	time.Sleep(20 * time.Millisecond)

	if err := loop.Stop(context.Background()); err != nil {
		t.Fatalf("unexpected stop error: %v", err)
	}

	stoppedAt := calls.Load()
	if stoppedAt == 0 {
		t.Fatal("expected steps to run")
	}

	time.Sleep(10 * time.Millisecond)

	if calls.Load() != stoppedAt {
		t.Fatal("expected no steps after Stop returned")
	}
}

// TestLoopFinishesInFlightWorkOnStop verifies graceful shutdown lets the current step complete.
func TestLoopFinishesInFlightWorkOnStop(t *testing.T) {
	started := make(chan struct{})

	var completed atomic.Bool

	loop := newTestLoop(func(stop context.Context, work context.Context) (bool, error) {
		close(started)
		<-stop.Done()

		select {
		case <-time.After(20 * time.Millisecond):
			completed.Store(true)
		case <-work.Done():
		}

		return true, nil
	})

	loop.Start()
	<-started

	if err := loop.Stop(context.Background()); err != nil {
		t.Fatalf("unexpected stop error: %v", err)
	}

	if !completed.Load() {
		t.Fatal("expected in-flight work to complete before Stop returned")
	}
}

// TestLoopCancelsInFlightWorkAfterDeadline verifies the stop deadline bounds shutdown.
func TestLoopCancelsInFlightWorkAfterDeadline(t *testing.T) {
	started := make(chan struct{})
	cancelled := make(chan struct{})

	loop := newTestLoop(func(_ context.Context, work context.Context) (bool, error) {
		close(started)
		<-work.Done()
		close(cancelled)

		return false, work.Err()
	})

	loop.Start()
	<-started

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	if err := loop.Stop(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline error, got %v", err)
	}

	select {
	case <-cancelled:
	case <-time.After(time.Second):
		t.Fatal("expected in-flight work to be cancelled")
	}
}

// TestLoopKeepsRunningAfterErrors verifies failures back off instead of stopping the worker.
func TestLoopKeepsRunningAfterErrors(t *testing.T) {
	var calls atomic.Int32

	loop := newTestLoop(func(context.Context, context.Context) (bool, error) {
		calls.Add(1)

		return false, errors.New("temporary failure")
	})

	loop.Start()
	time.Sleep(30 * time.Millisecond)

	if err := loop.Stop(context.Background()); err != nil {
		t.Fatalf("unexpected stop error: %v", err)
	}

	if calls.Load() < 2 {
		t.Fatalf("expected retries after errors, got %d calls", calls.Load())
	}
}

// TestLoopStopBeforeStartIsSafe verifies stopping an idle loop does nothing.
func TestLoopStopBeforeStartIsSafe(t *testing.T) {
	loop := newTestLoop(func(context.Context, context.Context) (bool, error) {
		return false, nil
	})

	if err := loop.Stop(context.Background()); err != nil {
		t.Fatalf("unexpected stop error: %v", err)
	}
}
