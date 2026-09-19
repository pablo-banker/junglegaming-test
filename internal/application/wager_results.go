package application

import (
	"time"

	"github.com/google/uuid"

	"github.com/pablo-banker/junglegaming-test/internal/domain"
)

type ProcessWagerResult struct {
	TransactionID         uuid.UUID
	ExternalTransactionID string
	Status                domain.WagerTransactionStatus
	BalanceBefore         *domain.Money
	BalanceAfter          *domain.Money
	FailureCode           string
	FailureMessage        string
	IdempotentReplay      bool
}

type WagerResult struct {
	TransactionID                  uuid.UUID                     `json:"transactionId"`
	ProviderID                     string                        `json:"providerId"`
	ExternalTransactionID          string                        `json:"externalTransactionId"`
	WalletID                       uuid.UUID                     `json:"walletId"`
	PlayerID                       uuid.UUID                     `json:"playerId"`
	RoundID                        string                        `json:"roundId"`
	GameID                         string                        `json:"gameId"`
	Type                           domain.WagerTransactionType   `json:"kind"`
	Amount                         domain.Money                  `json:"money"`
	ReferenceExternalTransactionID string                        `json:"referenceExternalTransactionId,omitempty"`
	Status                         domain.WagerTransactionStatus `json:"status"`
	FailureCode                    string                        `json:"failureCode,omitempty"`
	FailureMessage                 string                        `json:"failureMessage,omitempty"`
	BalanceBefore                  *domain.Money                 `json:"balanceBefore,omitempty"`
	BalanceAfter                   *domain.Money                 `json:"balanceAfter,omitempty"`
	CreatedAt                      time.Time                     `json:"createdAt"`
	UpdatedAt                      time.Time                     `json:"updatedAt"`
}

// resultFromWager builds the application response from persisted wager state.
func resultFromWager(transaction *domain.WagerTransaction, idempotentReplay bool) *ProcessWagerResult {
	result := &ProcessWagerResult{
		TransactionID:         transaction.ID(),
		ExternalTransactionID: transaction.ExternalTransactionID(),
		Status:                transaction.Status(),
		FailureCode:           transaction.FailureCode(),
		FailureMessage:        transaction.FailureMessage(),
		IdempotentReplay:      idempotentReplay,
	}

	if before, ok := transaction.BalanceBefore(); ok {
		result.BalanceBefore = &before
	}

	if after, ok := transaction.BalanceAfter(); ok {
		result.BalanceAfter = &after
	}

	return result
}

// wagerResultFromTransaction builds a query result from persisted wager state.
func wagerResultFromTransaction(transaction *domain.WagerTransaction) *WagerResult {
	result := &WagerResult{
		TransactionID:                  transaction.ID(),
		ProviderID:                     transaction.ProviderID(),
		ExternalTransactionID:          transaction.ExternalTransactionID(),
		WalletID:                       transaction.WalletID(),
		PlayerID:                       transaction.PlayerID(),
		RoundID:                        transaction.RoundID(),
		GameID:                         transaction.GameID(),
		Type:                           transaction.Type(),
		Amount:                         transaction.Amount(),
		ReferenceExternalTransactionID: transaction.ReferenceExternalTransactionID(),
		Status:                         transaction.Status(),
		FailureCode:                    transaction.FailureCode(),
		FailureMessage:                 transaction.FailureMessage(),
		CreatedAt:                      transaction.CreatedAt(),
		UpdatedAt:                      transaction.UpdatedAt(),
	}

	if before, ok := transaction.BalanceBefore(); ok {
		result.BalanceBefore = &before
	}

	if after, ok := transaction.BalanceAfter(); ok {
		result.BalanceAfter = &after
	}

	return result
}
