//go:build integration

package integration

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pablo-banker/junglegaming-test/internal/application"
	"github.com/pablo-banker/junglegaming-test/internal/domain"
	"github.com/pablo-banker/junglegaming-test/internal/infrastructure/postgres"
)

// newIntegratedWagerServiceWithPolicy creates a wager service with a custom reference retry policy.
func newIntegratedWagerServiceWithPolicy(pool *pgxpool.Pool, policy application.ReferenceRetryPolicy) *application.WagerService {
	return application.NewWagerService(
		postgres.NewTransactionManager(pool),
		postgres.NewClock(pool),
		postgres.NewWalletRepository(pool),
		postgres.NewWagerRepository(pool),
		postgres.NewWalletLedgerRepository(pool),
		postgres.NewOutboxRepository(),
		policy,
	)
}

// openWallet opens a wallet through the application so its OPENING is in the ledger.
func openWallet(t *testing.T, ctx context.Context, pool *pgxpool.Pool, balance string) *application.CreateWalletResult {
	t.Helper()

	wallet, err := newIntegratedWalletService(pool).Create(
		ctx,
		application.CreateWalletCommand{
			PlayerID:       uuid.NewString(),
			Currency:       "BRL",
			InitialBalance: balance,
		},
		wagerMetadata(),
	)
	if err != nil {
		t.Fatalf("failed to open wallet: %v", err)
	}

	return wallet
}

// wagerCommand builds a command for a wallet opened with openWallet.
func wagerCommand(wallet *application.CreateWalletResult, kind domain.WagerTransactionType, amount string) application.ProcessWagerCommand {
	externalTransactionID := "transaction-" + uuid.NewString()

	return application.ProcessWagerCommand{
		ProviderID:            "provider-regression",
		ExternalTransactionID: externalTransactionID,
		IdempotencyKey:        "provider-regression:" + externalTransactionID,
		WalletID:              wallet.WalletID.String(),
		PlayerID:              wallet.PlayerID.String(),
		RoundID:               "round-1",
		GameID:                "game-1",
		Type:                  string(kind),
		Amount:                amount,
		Currency:              "BRL",
	}
}

// assertReconciled verifies the stored balance equals the ledger credits minus debits.
func assertReconciled(t *testing.T, ctx context.Context, pool *pgxpool.Pool, walletID uuid.UUID) {
	t.Helper()

	result, err := newIntegratedWalletService(pool).Reconcile(ctx, walletID.String())
	if err != nil {
		t.Fatalf("failed to reconcile wallet: %v", err)
	}

	if !result.Consistent {
		t.Fatalf(
			"wallet %s is inconsistent: stored %s, ledger %s",
			walletID,
			result.StoredBalance.Amount(),
			result.CalculatedBalance.Amount(),
		)
	}
}

// processConcurrently runs all commands at the same time and returns results and errors.
func processConcurrently(
	ctx context.Context,
	service *application.WagerService,
	commands []application.ProcessWagerCommand,
) ([]*application.ProcessWagerResult, []error) {
	results := make([]*application.ProcessWagerResult, len(commands))
	errs := make([]error, len(commands))

	start := make(chan struct{})

	var wg sync.WaitGroup

	for i, command := range commands {
		wg.Go(func() {
			<-start

			results[i], errs[i] = service.Process(ctx, command, wagerMetadata())
		})
	}

	close(start)
	wg.Wait()

	return results, errs
}

// TestConcurrentBetsOnSameWalletNeverFail is a regression test for the FK deadlock and early timestamps.
func TestConcurrentBetsOnSameWalletNeverFail(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	pool := openIntegrationPool(t)
	service := newIntegratedWagerService(pool)

	const (
		wallets    = 10
		concurrent = 8
	)

	for range wallets {
		wallet := openWallet(t, ctx, pool, "100.00")

		commands := make([]application.ProcessWagerCommand, concurrent)
		for i := range commands {
			commands[i] = wagerCommand(wallet, domain.WagerTransactionTypeBet, "10.00")
		}

		results, errs := processConcurrently(ctx, service, commands)

		for i, err := range errs {
			if err != nil {
				t.Fatalf("concurrent bet failed: %v", err)
			}

			if results[i].Status != domain.WagerTransactionStatusProcessed {
				t.Fatalf("expected PROCESSED, got %s", results[i].Status)
			}
		}

		balance, version := readWalletState(t, ctx, pool, wallet.WalletID)

		if balance != "20.00" || version != 1+concurrent {
			t.Fatalf("expected balance 20.00 version %d, got %s version %d", 1+concurrent, balance, version)
		}

		if count := countWalletLedgerEntries(t, ctx, pool, wallet.WalletID); count != 1+concurrent {
			t.Fatalf("expected %d ledger entries, got %d", 1+concurrent, count)
		}

		assertReconciled(t, ctx, pool, wallet.WalletID)
	}
}

// TestCompetingBetsRepeatedly runs the mandatory 100.00 vs 2 x 80.00 race many times.
func TestCompetingBetsRepeatedly(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	pool := openIntegrationPool(t)
	service := newIntegratedWagerService(pool)

	for range 30 {
		wallet := openWallet(t, ctx, pool, "100.00")

		results, errs := processConcurrently(ctx, service, []application.ProcessWagerCommand{
			wagerCommand(wallet, domain.WagerTransactionTypeBet, "80.00"),
			wagerCommand(wallet, domain.WagerTransactionTypeBet, "80.00"),
		})

		processed, rejected := 0, 0

		for i, err := range errs {
			if err != nil {
				t.Fatalf("competing bet failed: %v", err)
			}

			switch {
			case results[i].Status == domain.WagerTransactionStatusProcessed:
				processed++

			case results[i].Status == domain.WagerTransactionStatusRejected &&
				results[i].FailureCode == application.FailureCodeBetInsufficientFunds:
				rejected++
			}
		}

		if processed != 1 || rejected != 1 {
			t.Fatalf("expected 1 processed and 1 rejected, got %d and %d", processed, rejected)
		}

		balance, _ := readWalletState(t, ctx, pool, wallet.WalletID)
		if balance != "20.00" {
			t.Fatalf("expected balance 20.00, got %s", balance)
		}

		if count := countWalletLedgerEntries(t, ctx, pool, wallet.WalletID); count != 2 {
			t.Fatalf("expected opening plus one debit, got %d ledger entries", count)
		}

		assertReconciled(t, ctx, pool, wallet.WalletID)
	}
}

// fastRetryPolicy retries pending references almost immediately.
func fastRetryPolicy(ttl time.Duration) application.ReferenceRetryPolicy {
	return application.ReferenceRetryPolicy{
		InitialDelay: time.Millisecond,
		MaxDelay:     10 * time.Millisecond,
		TTL:          ttl,
	}
}

// drainPendingReferences runs the pending reference worker until the transactions finish.
func drainPendingReferences(
	t *testing.T,
	ctx context.Context,
	service *application.WagerService,
	transactionIDs ...uuid.UUID,
) map[uuid.UUID]*application.WagerResult {
	t.Helper()

	deadline := time.Now().Add(15 * time.Second)

	for time.Now().Before(deadline) {
		results := make(map[uuid.UUID]*application.WagerResult, len(transactionIDs))

		for _, id := range transactionIDs {
			result, err := service.GetByID(ctx, "provider-regression", id.String())
			if err != nil {
				t.Fatalf("failed to read transaction: %v", err)
			}

			if result.Status == domain.WagerTransactionStatusPendingReference {
				break
			}

			results[id] = result
		}

		if len(results) == len(transactionIDs) {
			return results
		}

		if _, err := service.RetryNextPendingReference(ctx); err != nil {
			t.Logf("pending reference retry returned: %v", err)
		}

		time.Sleep(5 * time.Millisecond)
	}

	t.Fatal("pending references did not finish in time")

	return nil
}

// TestPendingReferencesResolveOrRejectWithoutBlocking verifies a late reference applies and a mismatch is rejected.
func TestPendingReferencesResolveOrRejectWithoutBlocking(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool := openIntegrationPool(t)
	service := newIntegratedWagerServiceWithPolicy(pool, fastRetryPolicy(time.Hour))
	wallet := openWallet(t, ctx, pool, "100.00")

	mismatchedBet := wagerCommand(wallet, domain.WagerTransactionTypeBet, "30.00")
	mismatchedRefund := wagerCommand(wallet, domain.WagerTransactionTypeRefund, "25.00")
	mismatchedRefund.ReferenceExternalTransactionID = mismatchedBet.ExternalTransactionID

	validBet := wagerCommand(wallet, domain.WagerTransactionTypeBet, "25.00")
	validRefund := wagerCommand(wallet, domain.WagerTransactionTypeRefund, "25.00")
	validRefund.ReferenceExternalTransactionID = validBet.ExternalTransactionID

	var refundIDs []uuid.UUID

	for _, refund := range []application.ProcessWagerCommand{mismatchedRefund, validRefund} {
		result, err := service.Process(ctx, refund, wagerMetadata())
		if err != nil {
			t.Fatalf("failed to process refund: %v", err)
		}

		if result.Status != domain.WagerTransactionStatusPendingReference {
			t.Fatalf("expected PENDING_REFERENCE, got %s", result.Status)
		}

		refundIDs = append(refundIDs, result.TransactionID)
	}

	for _, bet := range []application.ProcessWagerCommand{mismatchedBet, validBet} {
		if _, err := service.Process(ctx, bet, wagerMetadata()); err != nil {
			t.Fatalf("failed to process bet: %v", err)
		}
	}

	results := drainPendingReferences(t, ctx, service, refundIDs...)

	mismatched := results[refundIDs[0]]
	if mismatched.Status != domain.WagerTransactionStatusRejected ||
		mismatched.FailureCode != application.FailureCodeReferenceMismatch {
		t.Fatalf("expected REJECTED %s, got %s %s", application.FailureCodeReferenceMismatch, mismatched.Status, mismatched.FailureCode)
	}

	valid := results[refundIDs[1]]
	if valid.Status != domain.WagerTransactionStatusProcessed {
		t.Fatalf("expected valid refund PROCESSED, got %s %s", valid.Status, valid.FailureCode)
	}

	// 100.00 - 30.00 - 25.00 + 25.00
	balance, _ := readWalletState(t, ctx, pool, wallet.WalletID)
	if balance != "70.00" {
		t.Fatalf("expected balance 70.00, got %s", balance)
	}

	assertReconciled(t, ctx, pool, wallet.WalletID)
}

// TestPendingReferenceExpiresAsReferenceNotFound verifies the TTL rejection and its event.
func TestPendingReferenceExpiresAsReferenceNotFound(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool := openIntegrationPool(t)
	service := newIntegratedWagerServiceWithPolicy(pool, fastRetryPolicy(50*time.Millisecond))
	wallet := openWallet(t, ctx, pool, "100.00")

	rollback := wagerCommand(wallet, domain.WagerTransactionTypeRollback, "10.00")
	rollback.ReferenceExternalTransactionID = "never-arrives-" + uuid.NewString()

	result, err := service.Process(ctx, rollback, wagerMetadata())
	if err != nil {
		t.Fatalf("failed to process rollback: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	expired := drainPendingReferences(t, ctx, service, result.TransactionID)[result.TransactionID]

	if expired.Status != domain.WagerTransactionStatusRejected ||
		expired.FailureCode != application.FailureCodeReferenceNotFound {
		t.Fatalf("expected REJECTED %s, got %s %s", application.FailureCodeReferenceNotFound, expired.Status, expired.FailureCode)
	}

	var rejectedEvents int

	err = pool.QueryRow(
		ctx,
		`SELECT COUNT(*) FROM outbox_events WHERE aggregate_id = $1 AND event_type = 'WagerTransactionRejected'`,
		result.TransactionID,
	).Scan(&rejectedEvents)
	if err != nil {
		t.Fatalf("failed to count rejected events: %v", err)
	}

	if rejectedEvents != 1 {
		t.Fatalf("expected 1 WagerTransactionRejected event, got %d", rejectedEvents)
	}
}
