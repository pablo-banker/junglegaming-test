package integration

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/pablo-banker/junglegaming-test/internal/infrastructure/postgres"

	"github.com/pablo-banker/junglegaming-test/internal/application"
	"github.com/pablo-banker/junglegaming-test/internal/domain"
)

type concurrentWagerExecution struct {
	status        domain.WagerTransactionStatus
	failureCode   string
	transactionID uuid.UUID
	correlationID string
	err           error
}

// TestWagerServiceSerializesConcurrentBets verifies concurrent debits cannot overspend one wallet.
func TestWagerServiceSerializesConcurrentBets(t *testing.T) {
	ctx := integrationContext(t)
	pool := openIntegrationPool(t)

	service := newIntegratedWagerService(pool)

	wallet := createWagerServiceWallet(
		t,
		ctx,
		pool,
		"100.00",
	)

	providerID := "provider-" + uuid.NewString()
	roundID := "round-" + uuid.NewString()

	firstCommand := newProcessWagerCommand(
		wallet,
		domain.WagerTransactionTypeBet,
		"80.00",
	)

	firstCommand.ProviderID = providerID
	firstCommand.RoundID = roundID

	secondCommand := firstCommand
	secondCommand.ExternalTransactionID = "transaction-" + uuid.NewString()
	secondCommand.IdempotencyKey = "idempotency-" + uuid.NewString()

	commands := []application.ProcessWagerCommand{
		firstCommand,
		secondCommand,
	}

	metadata := []application.CommandMetadata{
		wagerMetadata(),
		wagerMetadata(),
	}

	start := make(chan struct{})
	done := make(chan concurrentWagerExecution, 2)

	for i := range commands {
		command := commands[i]
		meta := metadata[i]

		go func() {
			<-start

			result, err := service.Process(
				ctx,
				command,
				meta,
			)
			if err != nil {
				done <- concurrentWagerExecution{
					correlationID: meta.CorrelationID,
					err:           err,
				}

				return
			}

			done <- concurrentWagerExecution{
				status:        result.Status,
				failureCode:   result.FailureCode,
				transactionID: result.TransactionID,
				correlationID: meta.CorrelationID,
			}
		}()
	}

	close(start)

	executions := make(
		[]concurrentWagerExecution,
		0,
		2,
	)

	for len(executions) < 2 {
		select {
		case execution := <-done:
			if execution.err != nil {
				t.Fatalf(
					"concurrent wager failed: %v",
					execution.err,
				)
			}

			executions = append(
				executions,
				execution,
			)

		case <-time.After(5 * time.Second):
			t.Fatal("concurrent wagers did not finish")
		}
	}

	var (
		processedCount int
		rejectedCount  int

		processedExecution concurrentWagerExecution
		rejectedExecution  concurrentWagerExecution
	)

	for _, execution := range executions {
		switch execution.status {
		case domain.WagerTransactionStatusProcessed:
			processedCount++
			processedExecution = execution

		case domain.WagerTransactionStatusRejected:
			rejectedCount++
			rejectedExecution = execution
		}
	}

	if processedCount != 1 {
		t.Fatalf(
			"expected exactly one PROCESSED wager, got %d",
			processedCount,
		)
	}

	if rejectedCount != 1 {
		t.Fatalf(
			"expected exactly one REJECTED wager, got %d",
			rejectedCount,
		)
	}

	if rejectedExecution.failureCode != "BET_INSUFFICIENT_FUNDS" {
		t.Errorf(
			"expected BET_INSUFFICIENT_FUNDS, got %s",
			rejectedExecution.failureCode,
		)
	}

	if processedExecution.transactionID == rejectedExecution.transactionID {
		t.Fatal("expected different wager transaction ids")
	}

	balance, version := readWalletState(
		t,
		ctx,
		pool,
		wallet.ID(),
	)

	if balance != "20.00" {
		t.Errorf(
			"expected final balance 20.00, got %s",
			balance,
		)
	}

	if version != 2 {
		t.Errorf(
			"expected wallet version 2, got %d",
			version,
		)
	}

	if countWalletLedgerEntries(
		t,
		ctx,
		pool,
		wallet.ID(),
	) != 1 {
		t.Fatal("expected exactly one ledger entry")
	}

	var (
		persistedProcessed int
		persistedRejected  int
	)

	err := pool.QueryRow(
		ctx,
		`
			SELECT
				COUNT(*) FILTER (
					WHERE status = 'PROCESSED'
				),
				COUNT(*) FILTER (
					WHERE status = 'REJECTED'
				)
			FROM wager_transactions
			WHERE wallet_id = $1
		`,
		wallet.ID(),
	).Scan(
		&persistedProcessed,
		&persistedRejected,
	)
	if err != nil {
		t.Fatalf(
			"failed to read persisted wager statuses: %v",
			err,
		)
	}

	if persistedProcessed != 1 {
		t.Errorf(
			"expected one persisted PROCESSED wager, got %d",
			persistedProcessed,
		)
	}

	if persistedRejected != 1 {
		t.Errorf(
			"expected one persisted REJECTED wager, got %d",
			persistedRejected,
		)
	}

	if countOutboxEventsByCorrelation(
		t,
		ctx,
		pool,
		processedExecution.correlationID,
	) != 2 {
		t.Error(
			"expected processed wager to produce processed and balance changed events",
		)
	}

	if countOutboxEventsByCorrelation(
		t,
		ctx,
		pool,
		rejectedExecution.correlationID,
	) != 1 {
		t.Error(
			"expected rejected wager to produce one rejected event",
		)
	}
}

// TestWagerServiceDoesNotGloballyLockWallets verifies one locked wallet does not block another wallet.
func TestWagerServiceDoesNotGloballyLockWallets(t *testing.T) {
	ctx := integrationContext(t)
	pool := openIntegrationPool(t)

	service := newIntegratedWagerService(pool)
	txManager := postgres.NewTransactionManager(pool)
	walletRepository := postgres.NewWalletRepository(pool)

	firstWallet := createWagerServiceWallet(
		t,
		ctx,
		pool,
		"100.00",
	)

	secondWallet := createWagerServiceWallet(
		t,
		ctx,
		pool,
		"100.00",
	)

	firstLocked := make(chan struct{})
	releaseFirst := make(chan struct{})
	firstDone := make(chan error, 1)

	go func() {
		firstDone <- txManager.WithinTransaction(
			ctx,
			func(txCtx context.Context) error {
				_, err := walletRepository.FindByIDForUpdate(
					txCtx,
					firstWallet.ID(),
				)
				if err != nil {
					return err
				}

				close(firstLocked)

				<-releaseFirst

				return nil
			},
		)
	}()

	select {
	case <-firstLocked:

	case err := <-firstDone:
		t.Fatalf(
			"first wallet lock failed: %v",
			err,
		)

	case <-time.After(2 * time.Second):
		t.Fatal("first wallet was not locked")
	}

	command := newProcessWagerCommand(
		secondWallet,
		domain.WagerTransactionTypeBet,
		"10.00",
	)

	metadata := wagerMetadata()

	secondDone := make(
		chan concurrentWagerExecution,
		1,
	)

	go func() {
		result, err := service.Process(
			ctx,
			command,
			metadata,
		)
		if err != nil {
			secondDone <- concurrentWagerExecution{
				err: err,
			}

			return
		}

		secondDone <- concurrentWagerExecution{
			status:        result.Status,
			transactionID: result.TransactionID,
		}
	}()

	var secondExecution concurrentWagerExecution

	select {
	case secondExecution = <-secondDone:
		close(releaseFirst)

	case <-time.After(2 * time.Second):
		close(releaseFirst)

		<-firstDone

		t.Fatal(
			"second wallet was blocked while unrelated wallet was locked",
		)
	}

	if secondExecution.err != nil {
		t.Fatalf(
			"second wallet wager failed: %v",
			secondExecution.err,
		)
	}

	if secondExecution.status != domain.WagerTransactionStatusProcessed {
		t.Fatalf(
			"expected second wallet wager PROCESSED, got %s",
			secondExecution.status,
		)
	}

	if err := <-firstDone; err != nil {
		t.Fatalf(
			"first wallet transaction failed: %v",
			err,
		)
	}

	firstBalance, firstVersion := readWalletState(
		t,
		ctx,
		pool,
		firstWallet.ID(),
	)

	if firstBalance != "100.00" {
		t.Errorf(
			"expected first wallet balance 100.00, got %s",
			firstBalance,
		)
	}

	if firstVersion != 1 {
		t.Errorf(
			"expected first wallet version 1, got %d",
			firstVersion,
		)
	}

	secondBalance, secondVersion := readWalletState(
		t,
		ctx,
		pool,
		secondWallet.ID(),
	)

	if secondBalance != "90.00" {
		t.Errorf(
			"expected second wallet balance 90.00, got %s",
			secondBalance,
		)
	}

	if secondVersion != 2 {
		t.Errorf(
			"expected second wallet version 2, got %d",
			secondVersion,
		)
	}
}
