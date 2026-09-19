package sqs

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/pablo-banker/junglegaming-test/internal/application"
)

type fakeWagerMessageTransactionManager struct {
	called bool
}

// WithinTransaction executes the callback using the current context.
func (f *fakeWagerMessageTransactionManager) WithinTransaction(ctx context.Context, fn func(context.Context) error) error {
	f.called = true

	return fn(ctx)
}

type fakeWagerMessageInbox struct {
	registerCompleted bool
	registerErr       error
	completeErr       error

	registerCalls int
	completeCalls int

	consumerName string
	messageID    string
	messageHash  string
}

// Register records the inbox registration call.
func (f *fakeWagerMessageInbox) Register(_ context.Context, consumerName string, messageID string, messageHash string) (bool, error) {
	f.registerCalls++
	f.consumerName = consumerName
	f.messageID = messageID
	f.messageHash = messageHash

	return f.registerCompleted, f.registerErr
}

// Complete records the inbox completion call.
func (f *fakeWagerMessageInbox) Complete(_ context.Context, consumerName string, messageID string, _ time.Time) error {
	f.completeCalls++
	f.consumerName = consumerName
	f.messageID = messageID

	return f.completeErr
}

type fakeWagerProcessor struct {
	calls int
	err   error

	command  application.ProcessWagerCommand
	metadata application.CommandMetadata
}

// Process records the wager processing call.
func (f *fakeWagerProcessor) Process(_ context.Context, command application.ProcessWagerCommand, metadata application.CommandMetadata) (*application.ProcessWagerResult, error) {
	f.calls++
	f.command = command
	f.metadata = metadata

	return &application.ProcessWagerResult{}, f.err
}

type fakeWagerMessageClock struct {
	now time.Time
	err error
}

// Now returns the configured test time.
func (f *fakeWagerMessageClock) Now(context.Context) (time.Time, error) {
	return f.now, f.err
}

// newWagerMessageHandlerForTest creates a wager message handler with test dependencies.
func newWagerMessageHandlerForTest() (*WagerMessageHandler, *fakeWagerMessageTransactionManager, *fakeWagerMessageInbox, *fakeWagerProcessor, *fakeWagerMessageClock) {
	txManager := &fakeWagerMessageTransactionManager{}
	inbox := &fakeWagerMessageInbox{}
	service := &fakeWagerProcessor{}
	clock := &fakeWagerMessageClock{
		now: time.Date(2026, time.September, 19, 12, 0, 0, 0, time.UTC),
	}

	handler := &WagerMessageHandler{
		txManager: txManager,
		inbox:     inbox,
		service:   service,
		clock:     clock,
		logger:    slog.New(slog.DiscardHandler),

		allowedProviders: map[string]bool{"provider-a": true},
	}

	return handler, txManager, inbox, service, clock
}

// wagerMessageHandlerTestBody returns a valid wager SQS message.
func wagerMessageHandlerTestBody() string {
	return `{
		"messageId":"msg-123",
		"type":"WagerTransactionRequested",
		"occurredAt":"2026-09-08T12:00:00.000Z",
		"data":{
			"providerId":"provider-a",
			"externalTransactionId":"transaction-123",
			"idempotencyKey":"provider-a:transaction-123",
			"walletId":"11111111-1111-1111-1111-111111111111",
			"playerId":"22222222-2222-2222-2222-222222222222",
			"roundId":"round-123",
			"gameId":"game-123",
			"kind":"BET",
			"money":{"amount":"25.00","currency":"BRL"}
		}
	}`
}

// TestWagerMessageHandlerRejectsEmptyMessageID verifies message identity is required.
func TestWagerMessageHandlerRejectsEmptyMessageID(t *testing.T) {
	handler, txManager, _, service, _ := newWagerMessageHandlerForTest()

	err := handler.Handle(context.Background(), "", `{"correlationId":"correlation-1"}`)

	if !errors.Is(err, ErrInvalidMessageID) {
		t.Fatalf("expected ErrInvalidMessageID, got %v", err)
	}

	if txManager.called {
		t.Fatal("expected transaction not to start")
	}

	if service.calls != 0 {
		t.Fatalf("expected service not to be called, got %d calls", service.calls)
	}
}

// TestWagerMessageHandlerRejectsEmptyBody verifies empty message payloads are rejected.
func TestWagerMessageHandlerRejectsEmptyBody(t *testing.T) {
	handler, txManager, _, service, _ := newWagerMessageHandlerForTest()

	err := handler.Handle(context.Background(), "message-1", "")

	if !errors.Is(err, ErrInvalidMessageBody) {
		t.Fatalf("expected ErrInvalidMessageBody, got %v", err)
	}

	if txManager.called {
		t.Fatal("expected transaction not to start")
	}

	if service.calls != 0 {
		t.Fatalf("expected service not to be called, got %d calls", service.calls)
	}
}

// TestWagerMessageHandlerRejectsMalformedJSON verifies malformed messages never reach processing.
func TestWagerMessageHandlerRejectsMalformedJSON(t *testing.T) {
	handler, txManager, _, service, _ := newWagerMessageHandlerForTest()

	err := handler.Handle(context.Background(), "message-1", `{"correlationId":`)

	if err == nil {
		t.Fatal("expected malformed JSON error")
	}

	if txManager.called {
		t.Fatal("expected transaction not to start")
	}

	if service.calls != 0 {
		t.Fatalf("expected service not to be called, got %d calls", service.calls)
	}
}

// TestWagerMessageHandlerProcessesNewMessage verifies new messages complete transactionally.
func TestWagerMessageHandlerProcessesNewMessage(t *testing.T) {
	handler, txManager, inbox, service, _ := newWagerMessageHandlerForTest()

	err := handler.Handle(context.Background(), "message-123", wagerMessageHandlerTestBody())
	if err != nil {
		t.Fatalf("unexpected handle error: %v", err)
	}

	if !txManager.called {
		t.Fatal("expected transaction to be started")
	}

	if inbox.registerCalls != 1 {
		t.Fatalf("expected 1 register call, got %d", inbox.registerCalls)
	}

	if service.calls != 1 {
		t.Fatalf("expected 1 service call, got %d", service.calls)
	}

	if inbox.completeCalls != 1 {
		t.Fatalf("expected 1 complete call, got %d", inbox.completeCalls)
	}

	if inbox.consumerName != wagerConsumerName {
		t.Errorf("expected consumer %s, got %s", wagerConsumerName, inbox.consumerName)
	}

	if inbox.messageID != "msg-123" {
		t.Errorf("expected envelope message id msg-123, got %s", inbox.messageID)
	}

	if inbox.messageHash == "" {
		t.Fatal("expected message hash")
	}

	if service.metadata.CorrelationID != "msg-123" {
		t.Errorf("expected correlation msg-123, got %s", service.metadata.CorrelationID)
	}

	if service.command.Type != "BET" || service.command.Amount != "25.00" || service.command.Currency != "BRL" {
		t.Errorf("unexpected command mapping: %+v", service.command)
	}
}

// TestWagerMessageHandlerSkipsCompletedMessage verifies completed inbox messages are not processed twice.
func TestWagerMessageHandlerSkipsCompletedMessage(t *testing.T) {
	handler, _, inbox, service, _ := newWagerMessageHandlerForTest()

	inbox.registerCompleted = true

	err := handler.Handle(context.Background(), "message-123", wagerMessageHandlerTestBody())
	if err != nil {
		t.Fatalf("unexpected handle error: %v", err)
	}

	if service.calls != 0 {
		t.Fatalf("expected service not to be called, got %d calls", service.calls)
	}

	if inbox.completeCalls != 0 {
		t.Fatalf("expected inbox not to be completed again, got %d calls", inbox.completeCalls)
	}
}

// TestWagerMessageHandlerRejectsInboxConflict verifies conflicting message identities stop processing.
func TestWagerMessageHandlerRejectsInboxConflict(t *testing.T) {
	handler, _, inbox, service, _ := newWagerMessageHandlerForTest()

	inbox.registerErr = application.ErrInboxMessageConflict

	err := handler.Handle(context.Background(), "message-123", wagerMessageHandlerTestBody())

	if !errors.Is(err, application.ErrInboxMessageConflict) {
		t.Fatalf("expected ErrInboxMessageConflict, got %v", err)
	}

	if service.calls != 0 {
		t.Fatalf("expected service not to be called, got %d calls", service.calls)
	}

	if inbox.completeCalls != 0 {
		t.Fatalf("expected inbox not to complete, got %d calls", inbox.completeCalls)
	}
}

// TestWagerMessageHandlerDoesNotCompleteInboxWhenProcessingFails verifies failed messages remain retryable.
func TestWagerMessageHandlerDoesNotCompleteInboxWhenProcessingFails(t *testing.T) {
	handler, _, inbox, service, _ := newWagerMessageHandlerForTest()

	expectedErr := errors.New("processing failed")
	service.err = expectedErr

	err := handler.Handle(context.Background(), "message-123", wagerMessageHandlerTestBody())

	if !errors.Is(err, expectedErr) {
		t.Fatalf("expected processing error, got %v", err)
	}

	if service.calls != 1 {
		t.Fatalf("expected 1 service call, got %d", service.calls)
	}

	if inbox.completeCalls != 0 {
		t.Fatalf("expected inbox not to complete, got %d calls", inbox.completeCalls)
	}
}

// TestWagerMessageHandlerReturnsClockErrorBeforeCompletion verifies completion requires a valid database time.
func TestWagerMessageHandlerReturnsClockErrorBeforeCompletion(t *testing.T) {
	handler, _, inbox, service, clock := newWagerMessageHandlerForTest()

	expectedErr := errors.New("clock failed")
	clock.err = expectedErr

	err := handler.Handle(context.Background(), "message-123", wagerMessageHandlerTestBody())

	if !errors.Is(err, expectedErr) {
		t.Fatalf("expected clock error, got %v", err)
	}

	if service.calls != 1 {
		t.Fatalf("expected 1 service call, got %d", service.calls)
	}

	if inbox.completeCalls != 0 {
		t.Fatalf("expected inbox not to complete, got %d calls", inbox.completeCalls)
	}
}

// TestWagerMessageHandlerReturnsCompletionError verifies inbox completion failures abort processing.
func TestWagerMessageHandlerReturnsCompletionError(t *testing.T) {
	handler, _, inbox, service, _ := newWagerMessageHandlerForTest()

	expectedErr := errors.New("complete failed")
	inbox.completeErr = expectedErr

	err := handler.Handle(context.Background(), "message-123", wagerMessageHandlerTestBody())

	if !errors.Is(err, expectedErr) {
		t.Fatalf("expected completion error, got %v", err)
	}

	if service.calls != 1 {
		t.Fatalf("expected 1 service call, got %d", service.calls)
	}

	if inbox.completeCalls != 1 {
		t.Fatalf("expected 1 complete call, got %d", inbox.completeCalls)
	}
}

// TestHashMessageIsDeterministic verifies identical payloads produce identical hashes.
func TestHashMessageIsDeterministic(t *testing.T) {
	body := wagerMessageHandlerTestBody()

	first := hashMessage(body)
	second := hashMessage(body)

	if first != second {
		t.Fatalf("expected identical hashes, got %s and %s", first, second)
	}

	if first == "" {
		t.Fatal("expected non-empty hash")
	}
}

// TestHashMessageChangesWithPayload verifies modified payloads produce another hash.
func TestHashMessageChangesWithPayload(t *testing.T) {
	first := hashMessage(`{"amount":"10.00"}`)
	second := hashMessage(`{"amount":"20.00"}`)

	if first == second {
		t.Fatal("expected different payload hashes")
	}
}

// TestWagerMessageHandlerRejectsProviderNotAllowed verifies SQS messages cannot act for unknown providers.
func TestWagerMessageHandlerRejectsProviderNotAllowed(t *testing.T) {
	handler, txManager, _, service, _ := newWagerMessageHandlerForTest()

	handler.allowedProviders = map[string]bool{"provider-b": true}

	err := handler.Handle(context.Background(), "message-123", wagerMessageHandlerTestBody())

	if !errors.Is(err, ErrProviderNotAllowed) {
		t.Fatalf("expected ErrProviderNotAllowed, got %v", err)
	}

	if txManager.called || service.calls != 0 {
		t.Fatal("expected the message to be rejected before any processing")
	}
}
