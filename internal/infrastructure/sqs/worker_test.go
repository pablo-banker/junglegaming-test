//go:build unit

package sqs

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"
)

type blockingPoller struct {
	mu      sync.Mutex
	calls   int
	started chan struct{}
	once    sync.Once
}

// PollOnce blocks until the worker context is cancelled.
func (p *blockingPoller) PollOnce(ctx context.Context) error {
	p.mu.Lock()
	p.calls++
	p.mu.Unlock()

	p.once.Do(func() {
		close(p.started)
	})

	<-ctx.Done()

	return ctx.Err()
}

// Calls returns the number of polling calls.
func (p *blockingPoller) Calls() int {
	p.mu.Lock()
	defer p.mu.Unlock()

	return p.calls
}

type failingPoller struct {
	mu          sync.Mutex
	calls       int
	firstCalled chan struct{}
	secondCall  chan struct{}
	firstOnce   sync.Once
	secondOnce  sync.Once
	err         error
}

// PollOnce fails once and blocks on the next call.
func (p *failingPoller) PollOnce(ctx context.Context) error {
	p.mu.Lock()
	p.calls++
	call := p.calls
	p.mu.Unlock()

	if call == 1 {
		p.firstOnce.Do(func() {
			close(p.firstCalled)
		})

		return p.err
	}

	p.secondOnce.Do(func() {
		close(p.secondCall)
	})

	<-ctx.Done()

	return ctx.Err()
}

// Calls returns the number of polling calls.
func (p *failingPoller) Calls() int {
	p.mu.Lock()
	defer p.mu.Unlock()

	return p.calls
}

// testWorkerLogger creates a silent logger for worker tests.
func testWorkerLogger() *slog.Logger {
	return slog.New(
		slog.NewTextHandler(
			io.Discard,
			nil,
		),
	)
}

// TestWorkerStartBeginsPolling verifies Start launches the polling loop.
func TestWorkerStartBeginsPolling(t *testing.T) {
	poller := &blockingPoller{
		started: make(chan struct{}),
	}

	worker := &Worker{
		consumer: poller,
		logger:   testWorkerLogger(),
	}

	worker.Start()

	select {
	case <-poller.started:
	case <-time.After(time.Second):
		t.Fatal("expected worker to start polling")
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	if err := worker.Stop(ctx); err != nil {
		t.Fatalf("unexpected stop error: %v", err)
	}
}

// TestWorkerStopCancelsPolling verifies Stop cancels and waits for the polling loop.
func TestWorkerStopCancelsPolling(t *testing.T) {
	poller := &blockingPoller{
		started: make(chan struct{}),
	}

	worker := &Worker{
		consumer: poller,
		logger:   testWorkerLogger(),
	}

	worker.Start()

	select {
	case <-poller.started:
	case <-time.After(time.Second):
		t.Fatal("expected worker to start polling")
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	err := worker.Stop(ctx)
	if err != nil {
		t.Fatalf("unexpected stop error: %v", err)
	}

	if poller.Calls() != 1 {
		t.Fatalf("expected 1 poll call, got %d", poller.Calls())
	}
}

// TestWorkerStartIsIdempotent verifies repeated Start calls do not create multiple workers.
func TestWorkerStartIsIdempotent(t *testing.T) {
	poller := &blockingPoller{
		started: make(chan struct{}),
	}

	worker := &Worker{
		consumer: poller,
		logger:   testWorkerLogger(),
	}

	worker.Start()
	worker.Start()

	select {
	case <-poller.started:
	case <-time.After(time.Second):
		t.Fatal("expected worker to start polling")
	}

	time.Sleep(50 * time.Millisecond)

	if poller.Calls() != 1 {
		t.Fatalf("expected 1 poll call, got %d", poller.Calls())
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	if err := worker.Stop(ctx); err != nil {
		t.Fatalf("unexpected stop error: %v", err)
	}
}

// TestWorkerStopBeforeStartIsSafe verifies stopping an inactive worker succeeds.
func TestWorkerStopBeforeStartIsSafe(t *testing.T) {
	worker := &Worker{
		consumer: &blockingPoller{
			started: make(chan struct{}),
		},
		logger: testWorkerLogger(),
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	if err := worker.Stop(ctx); err != nil {
		t.Fatalf("unexpected stop error: %v", err)
	}
}

// TestWorkerContinuesAfterPollingError verifies transient SQS errors do not stop the worker.
func TestWorkerContinuesAfterPollingError(t *testing.T) {
	poller := &failingPoller{
		firstCalled: make(chan struct{}),
		secondCall:  make(chan struct{}),
		err:         errors.New("temporary SQS failure"),
	}

	worker := &Worker{
		consumer: poller,
		logger:   testWorkerLogger(),
	}

	worker.Start()

	select {
	case <-poller.firstCalled:
	case <-time.After(time.Second):
		t.Fatal("expected first polling attempt")
	}

	select {
	case <-poller.secondCall:
	case <-time.After(2 * time.Second):
		t.Fatal("expected worker to retry polling")
	}

	if poller.Calls() < 2 {
		t.Fatalf("expected at least 2 poll calls, got %d", poller.Calls())
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	if err := worker.Stop(ctx); err != nil {
		t.Fatalf("unexpected stop error: %v", err)
	}
}

// TestWorkerStopInterruptsRetryDelay verifies shutdown does not wait for the retry delay.
func TestWorkerStopInterruptsRetryDelay(t *testing.T) {
	poller := &failingPoller{
		firstCalled: make(chan struct{}),
		secondCall:  make(chan struct{}),
		err:         errors.New("temporary SQS failure"),
	}

	worker := &Worker{
		consumer: poller,
		logger:   testWorkerLogger(),
	}

	worker.Start()

	select {
	case <-poller.firstCalled:
	case <-time.After(time.Second):
		t.Fatal("expected first polling attempt")
	}

	startedAt := time.Now()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	if err := worker.Stop(ctx); err != nil {
		t.Fatalf("unexpected stop error: %v", err)
	}

	if time.Since(startedAt) >= workerRetryDelay {
		t.Fatal("expected shutdown to interrupt retry delay")
	}
}
