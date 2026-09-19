//go:build integration

package integration

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awssqs "github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pablo-banker/junglegaming-test/internal/application"
	"github.com/pablo-banker/junglegaming-test/internal/domain"
	"github.com/pablo-banker/junglegaming-test/internal/infrastructure/postgres"
	infrasqs "github.com/pablo-banker/junglegaming-test/internal/infrastructure/sqs"
)

// newIntegratedMessageHandler creates the SQS wager handler backed by PostgreSQL.
func newIntegratedMessageHandler(pool *pgxpool.Pool) *infrasqs.WagerMessageHandler {
	return infrasqs.NewWagerMessageHandler(
		postgres.NewTransactionManager(pool),
		postgres.NewInboxRepository(pool),
		newIntegratedWagerService(pool),
		postgres.NewClock(pool),
		sqsHandlerConfig(),
		discardLogger(),
		nil,
	)
}

// sqsBetFor builds a BET command and the equivalent SQS message body.
func sqsBetFor(t *testing.T, wallet *application.CreateWalletResult, amount string) (application.ProcessWagerCommand, string) {
	t.Helper()

	command := wagerCommand(wallet, domain.WagerTransactionTypeBet, amount)
	command.ProviderID = "provider-a"
	command.IdempotencyKey = "provider-a:" + command.ExternalTransactionID
	command.RoundID = "round-123"
	command.GameID = "game-123"

	body := wagerRequestedMessageBody(t, wagerRequestedMessage{
		MessageID:             "message-" + uuid.NewString(),
		ProviderID:            command.ProviderID,
		ExternalTransactionID: command.ExternalTransactionID,
		IdempotencyKey:        command.IdempotencyKey,
		WalletID:              command.WalletID,
		PlayerID:              command.PlayerID,
		Kind:                  command.Type,
		Amount:                command.Amount,
	})

	return command, body
}

// TestHTTPAndSQSDeliverSameOperationOnce verifies the same operation received concurrently
// through HTTP and SQS moves money once and both entry points report success.
func TestHTTPAndSQSDeliverSameOperationOnce(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool := openIntegrationPool(t)
	service := newIntegratedWagerService(pool)
	handler := newIntegratedMessageHandler(pool)

	for range 10 {
		wallet := openWallet(t, ctx, pool, "100.00")
		command, body := sqsBetFor(t, wallet, "25.00")

		var (
			wg         sync.WaitGroup
			httpErr    error
			sqsErr     error
			httpResult *application.ProcessWagerResult
		)

		wg.Go(func() { httpResult, httpErr = service.Process(ctx, command, wagerMetadata()) })
		wg.Go(func() { sqsErr = handler.Handle(ctx, "sqs-"+uuid.NewString(), body) })
		wg.Wait()

		if httpErr != nil || sqsErr != nil {
			t.Fatalf("expected both deliveries to succeed, got http=%v sqs=%v", httpErr, sqsErr)
		}

		if httpResult.Status != domain.WagerTransactionStatusProcessed {
			t.Fatalf("expected PROCESSED, got %s", httpResult.Status)
		}

		balance, _ := readWalletState(t, ctx, pool, wallet.WalletID)
		if balance != "75.00" {
			t.Fatalf("expected a single debit, got balance %s", balance)
		}

		if count := countWalletLedgerEntries(t, ctx, pool, wallet.WalletID); count != 2 {
			t.Fatalf("expected opening plus one debit, got %d ledger entries", count)
		}
	}
}

// TestSQSRedeliveryAfterCommitMovesMoneyOnce simulates a consumer that commits and dies
// before deleting the message: the redelivery is answered by the inbox and deleted.
func TestSQSRedeliveryAfterCommitMovesMoneyOnce(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool := openIntegrationPool(t)
	client, cfg := newConsumerSQSClient(t)
	cfg.SQSQueueURL = createConsumerTestQueue(t, client)

	handler := newIntegratedMessageHandler(pool)
	consumer := infrasqs.NewConsumer(client, handler, cfg, discardLogger(), nil)

	wallet := openWallet(t, ctx, pool, "100.00")
	_, body := sqsBetFor(t, wallet, "25.00")

	if err := infrasqs.NewPublisher(client, cfg).Send(ctx, body, wallet.WalletID.String(), uuid.NewString()); err != nil {
		t.Fatalf("failed to publish: %v", err)
	}

	// The first consumer receives and commits, then crashes before DeleteMessage.
	received, err := client.ReceiveMessage(ctx, &awssqs.ReceiveMessageInput{
		QueueUrl:        aws.String(cfg.SQSQueueURL),
		WaitTimeSeconds: 1,
	})
	if err != nil || len(received.Messages) != 1 {
		t.Fatalf("expected one message, got %v %v", received, err)
	}

	if err := handler.Handle(ctx, aws.ToString(received.Messages[0].MessageId), body); err != nil {
		t.Fatalf("failed to process the first delivery: %v", err)
	}

	// The queue visibility timeout is 1 second: the message comes back to another consumer.
	time.Sleep(1100 * time.Millisecond)

	found, err := consumer.PollOnce(ctx, ctx)
	if err != nil || !found {
		t.Fatalf("expected the redelivery to be handled, got %v %v", found, err)
	}

	balance, version := readWalletState(t, ctx, pool, wallet.WalletID)
	if balance != "75.00" || version != 2 {
		t.Fatalf("expected a single debit, got balance %s version %d", balance, version)
	}

	time.Sleep(1100 * time.Millisecond)

	remaining, err := client.ReceiveMessage(ctx, &awssqs.ReceiveMessageInput{
		QueueUrl:        aws.String(cfg.SQSQueueURL),
		WaitTimeSeconds: 1,
	})
	if err != nil {
		t.Fatalf("failed to check the queue: %v", err)
	}

	if len(remaining.Messages) != 0 {
		t.Fatalf("expected the redelivered message to be deleted, got %d", len(remaining.Messages))
	}
}

// recordingPublisher records every publication, as a downstream consumer would see it.
type recordingPublisher struct {
	mu        sync.Mutex
	published map[uuid.UUID]int
}

// Publish records the event id.
func (p *recordingPublisher) Publish(_ context.Context, event *application.PendingOutboxEvent) error {
	time.Sleep(time.Millisecond)

	p.mu.Lock()
	defer p.mu.Unlock()

	p.published[event.EventID]++

	return nil
}

// count returns how many times an event was published.
func (p *recordingPublisher) count(eventID uuid.UUID) int {
	p.mu.Lock()
	defer p.mu.Unlock()

	return p.published[eventID]
}

// newOutboxDispatcher creates one publisher instance with its own worker id.
func newOutboxDispatcher(pool *pgxpool.Pool, publisher *recordingPublisher) *application.OutboxDispatcherService {
	return application.NewOutboxDispatcherService(
		postgres.NewOutboxDispatcherRepository(pool),
		publisher,
		postgres.NewClock(pool),
	)
}

// drainOutbox dispatches until no event is available.
func drainOutbox(t *testing.T, ctx context.Context, dispatcher *application.OutboxDispatcherService) {
	t.Helper()

	for ctx.Err() == nil {
		found, err := dispatcher.DispatchNext(ctx)
		if err != nil {
			t.Errorf("dispatch failed: %v", err)

			return
		}

		if !found {
			return
		}
	}
}

// openWalletsWithEvents opens wallets whose outbox events share one correlation id.
func openWalletsWithEvents(t *testing.T, ctx context.Context, pool *pgxpool.Pool, wallets int) []uuid.UUID {
	t.Helper()

	correlationID := "outbox-" + uuid.NewString()

	for range wallets {
		_, err := newIntegratedWalletService(pool).Create(ctx, application.CreateWalletCommand{
			PlayerID:       uuid.NewString(),
			Currency:       "BRL",
			InitialBalance: "10.00",
		}, application.CommandMetadata{CorrelationID: correlationID})
		if err != nil {
			t.Fatalf("failed to open wallet: %v", err)
		}
	}

	rows, err := pool.Query(ctx, `SELECT event_id FROM outbox_events WHERE correlation_id = $1`, correlationID)
	if err != nil {
		t.Fatalf("failed to read outbox events: %v", err)
	}

	defer rows.Close()

	var eventIDs []uuid.UUID

	for rows.Next() {
		var eventID uuid.UUID
		if err := rows.Scan(&eventID); err != nil {
			t.Fatalf("failed to scan event id: %v", err)
		}

		eventIDs = append(eventIDs, eventID)
	}

	return eventIDs
}

// assertPublished verifies the outbox marked the event as published.
func assertPublished(t *testing.T, ctx context.Context, pool *pgxpool.Pool, eventID uuid.UUID) {
	t.Helper()

	var published bool

	err := pool.QueryRow(
		ctx,
		`SELECT published_at IS NOT NULL AND claimed_by IS NULL FROM outbox_events WHERE event_id = $1`,
		eventID,
	).Scan(&published)
	if err != nil || !published {
		t.Fatalf("expected event %s to be published and released, got %v %v", eventID, published, err)
	}
}

// TestOutboxDispatchersShareWorkWithoutDuplicates runs two publishers against the same
// outbox: every committed event is published exactly once.
func TestOutboxDispatchersShareWorkWithoutDuplicates(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool := openIntegrationPool(t)
	eventIDs := openWalletsWithEvents(t, ctx, pool, 15)

	publisher := &recordingPublisher{published: map[uuid.UUID]int{}}

	var wg sync.WaitGroup

	for range 2 {
		dispatcher := newOutboxDispatcher(pool, publisher)

		wg.Go(func() { drainOutbox(t, ctx, dispatcher) })
	}

	wg.Wait()

	for _, eventID := range eventIDs {
		if count := publisher.count(eventID); count != 1 {
			t.Errorf("expected event %s to be published once, got %d", eventID, count)
		}

		assertPublished(t, ctx, pool, eventID)
	}
}

// TestOutboxRecoversAbandonedClaim simulates a publisher that crashed after publishing and
// before confirming: once its lease expires another instance republishes the same eventId.
// An event committed but never claimed (crash between commit and publication) is simply
// published by the surviving instance.
func TestOutboxRecoversAbandonedClaim(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool := openIntegrationPool(t)
	eventIDs := openWalletsWithEvents(t, ctx, pool, 1)

	if len(eventIDs) != 2 {
		t.Fatalf("expected the opening to produce 2 events, got %d", len(eventIDs))
	}

	abandoned, neverClaimed := eventIDs[0], eventIDs[1]
	publisher := &recordingPublisher{published: map[uuid.UUID]int{}}

	// The crashed instance claimed and published the event, but never marked it published.
	_, err := pool.Exec(
		ctx,
		`UPDATE outbox_events
		 SET claimed_by = 'crashed-publisher', claimed_until = clock_timestamp() + interval '1 hour', attempts = attempts + 1
		 WHERE event_id = $1`,
		abandoned,
	)
	if err != nil {
		t.Fatalf("failed to simulate the abandoned claim: %v", err)
	}

	publisher.published[abandoned] = 1

	survivor := newOutboxDispatcher(pool, publisher)
	drainOutbox(t, ctx, survivor)

	if publisher.count(neverClaimed) != 1 {
		t.Fatalf("expected the unclaimed event to be published once, got %d", publisher.count(neverClaimed))
	}

	if publisher.count(abandoned) != 1 {
		t.Fatal("expected the claimed event to wait for its lease")
	}

	// The lease expires.
	_, err = pool.Exec(ctx, `UPDATE outbox_events SET claimed_until = clock_timestamp() - interval '1 second' WHERE event_id = $1`, abandoned)
	if err != nil {
		t.Fatalf("failed to expire the lease: %v", err)
	}

	drainOutbox(t, ctx, survivor)

	if count := publisher.count(abandoned); count != 2 {
		t.Fatalf("expected the abandoned event to be republished with the same eventId, got %d publications", count)
	}

	assertPublished(t, ctx, pool, abandoned)
	assertPublished(t, ctx, pool, neverClaimed)
}

// TestPendingReferenceResumesOnAnotherInstance verifies a PENDING_REFERENCE committed by
// one instance is completed by another one, since the retry state lives in PostgreSQL.
func TestPendingReferenceResumesOnAnotherInstance(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool := openIntegrationPool(t)
	firstInstance := newIntegratedWagerServiceWithPolicy(pool, fastRetryPolicy(time.Hour))
	secondInstance := newIntegratedWagerServiceWithPolicy(pool, fastRetryPolicy(time.Hour))

	wallet := openWallet(t, ctx, pool, "100.00")

	bet := wagerCommand(wallet, domain.WagerTransactionTypeBet, "40.00")
	rollback := wagerCommand(wallet, domain.WagerTransactionTypeRollback, "40.00")
	rollback.ReferenceExternalTransactionID = bet.ExternalTransactionID

	pending, err := firstInstance.Process(ctx, rollback, wagerMetadata())
	if err != nil || pending.Status != domain.WagerTransactionStatusPendingReference {
		t.Fatalf("expected PENDING_REFERENCE, got %v %v", pending, err)
	}

	if _, err := secondInstance.Process(ctx, bet, wagerMetadata()); err != nil {
		t.Fatalf("failed to process bet: %v", err)
	}

	result := drainPendingReferences(t, ctx, secondInstance, pending.TransactionID)[pending.TransactionID]
	if result.Status != domain.WagerTransactionStatusProcessed {
		t.Fatalf("expected the rollback to be processed by the second instance, got %s %s", result.Status, result.FailureCode)
	}

	balance, _ := readWalletState(t, ctx, pool, wallet.WalletID)
	if balance != "100.00" {
		t.Fatalf("expected the rollback to restore 100.00, got %s", balance)
	}

	assertReconciled(t, ctx, pool, wallet.WalletID)
}
