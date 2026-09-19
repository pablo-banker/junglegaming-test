package application

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/pablo-banker/junglegaming-test/internal/domain"
)

// createProcessedEvent persists the WagerTransactionProcessed event.
func (s *WagerService) createProcessedEvent(ctx context.Context, transaction *domain.WagerTransaction, metadata CommandMetadata, now time.Time) error {
	data, err := processedEventData(transaction)
	if err != nil {
		return err
	}

	return s.outbox.Create(ctx, NewWagerTransactionProcessedEvent(metadata, now, data))
}

// createBalanceChangedEvent persists the WalletBalanceChanged event.
func (s *WagerService) createBalanceChangedEvent(
	ctx context.Context,
	wallet *domain.Wallet,
	entry *domain.WalletLedgerEntry,
	metadata CommandMetadata,
	now time.Time,
) error {
	return s.outbox.Create(ctx, NewWalletBalanceChangedEvent(metadata, now, balanceChangedEventData(wallet, entry)))
}

// createRejectedEvent persists the WagerTransactionRejected event.
func (s *WagerService) createRejectedEvent(ctx context.Context, transaction *domain.WagerTransaction, metadata CommandMetadata, now time.Time) error {
	return s.outbox.Create(ctx, NewWagerTransactionRejectedEvent(metadata, now, WagerTransactionRejectedData{
		TransactionID:         transaction.ID(),
		ProviderID:            transaction.ProviderID(),
		ExternalTransactionID: transaction.ExternalTransactionID(),
		WalletID:              transaction.WalletID(),
		PlayerID:              transaction.PlayerID(),
		RoundID:               transaction.RoundID(),
		GameID:                transaction.GameID(),
		Kind:                  transaction.Type(),
		Money:                 transaction.Amount(),
		FailureCode:           transaction.FailureCode(),
		FailureMessage:        transaction.FailureMessage(),
	}))
}

// createPendingReferenceEvent persists the WagerTransactionPendingReference event.
func (s *WagerService) createPendingReferenceEvent(ctx context.Context, transaction *domain.WagerTransaction, metadata CommandMetadata, now time.Time) error {
	return s.outbox.Create(ctx, NewWagerTransactionPendingReferenceEvent(metadata, now, WagerTransactionPendingReferenceData{
		TransactionID:                  transaction.ID(),
		ProviderID:                     transaction.ProviderID(),
		ExternalTransactionID:          transaction.ExternalTransactionID(),
		ReferenceExternalTransactionID: transaction.ReferenceExternalTransactionID(),
		WalletID:                       transaction.WalletID(),
		PlayerID:                       transaction.PlayerID(),
		RoundID:                        transaction.RoundID(),
		GameID:                         transaction.GameID(),
		Kind:                           transaction.Type(),
		Money:                          transaction.Amount(),
	}))
}

// processedEventData builds the processed event data of a processed transaction, OPENING included.
func processedEventData(transaction *domain.WagerTransaction) (WagerTransactionProcessedData, error) {
	before, hasBefore := transaction.BalanceBefore()
	after, hasAfter := transaction.BalanceAfter()

	if !hasBefore || !hasAfter {
		return WagerTransactionProcessedData{}, domain.ErrInvalidWagerState
	}

	var referenceTransactionID *uuid.UUID

	if id := transaction.ReferenceTransactionID(); id != uuid.Nil {
		referenceTransactionID = &id
	}

	return WagerTransactionProcessedData{
		TransactionID:          transaction.ID(),
		ProviderID:             transaction.ProviderID(),
		ExternalTransactionID:  transaction.ExternalTransactionID(),
		WalletID:               transaction.WalletID(),
		PlayerID:               transaction.PlayerID(),
		RoundID:                transaction.RoundID(),
		GameID:                 transaction.GameID(),
		Kind:                   transaction.Type(),
		Money:                  transaction.Amount(),
		BalanceBefore:          before,
		BalanceAfter:           after,
		ReferenceTransactionID: referenceTransactionID,
	}, nil
}

// balanceChangedEventData builds the balance changed event data of a ledger movement.
func balanceChangedEventData(wallet *domain.Wallet, entry *domain.WalletLedgerEntry) WalletBalanceChangedData {
	return WalletBalanceChangedData{
		WalletID:      wallet.ID(),
		PlayerID:      wallet.PlayerID(),
		TransactionID: entry.TransactionID(),
		Direction:     entry.Direction(),
		Money:         entry.Amount(),
		BalanceBefore: entry.BalanceBefore(),
		BalanceAfter:  entry.BalanceAfter(),
		WalletVersion: wallet.Version(),
	}
}
