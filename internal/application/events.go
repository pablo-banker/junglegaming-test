package application

import (
	"time"

	"github.com/google/uuid"

	"github.com/pablo-banker/junglegaming-test/internal/domain"
)

type EventType string

const (
	EventTypeWagerTransactionProcessed        EventType = "WagerTransactionProcessed"
	EventTypeWagerTransactionRejected         EventType = "WagerTransactionRejected"
	EventTypeWalletBalanceChanged             EventType = "WalletBalanceChanged"
	EventTypeWagerTransactionPendingReference EventType = "WagerTransactionPendingReference"

	EventVersion int64 = 1
)

// EventEnvelope contains the common metadata persisted in the outbox.
type EventEnvelope struct {
	EventID       uuid.UUID `json:"eventId"`
	EventType     EventType `json:"eventType"`
	AggregateID   uuid.UUID `json:"aggregateId"`
	CorrelationID string    `json:"correlationId"`
	CausationID   string    `json:"causationId,omitempty"`
	Version       int64     `json:"version"`
	Payload       any       `json:"payload"`
	OccurredAt    time.Time `json:"occurredAt"`
}

// WagerTransactionProcessedPayload describes a successfully processed wager.
type WagerTransactionProcessedPayload struct {
	TransactionID          uuid.UUID                   `json:"transactionId"`
	ProviderID             string                      `json:"providerId,omitempty"`
	ExternalTransactionID  string                      `json:"externalTransactionId,omitempty"`
	WalletID               uuid.UUID                   `json:"walletId"`
	PlayerID               uuid.UUID                   `json:"playerId"`
	RoundID                string                      `json:"roundId,omitempty"`
	GameID                 string                      `json:"gameId,omitempty"`
	Type                   domain.WagerTransactionType `json:"type"`
	Amount                 domain.Money                `json:"amount"`
	BalanceBefore          domain.Money                `json:"balanceBefore"`
	BalanceAfter           domain.Money                `json:"balanceAfter"`
	ReferenceTransactionID *uuid.UUID                  `json:"referenceTransactionId,omitempty"`
}

// WagerTransactionRejectedPayload describes a wager rejected by a business rule.
type WagerTransactionRejectedPayload struct {
	TransactionID         uuid.UUID                   `json:"transactionId"`
	ProviderID            string                      `json:"providerId"`
	ExternalTransactionID string                      `json:"externalTransactionId"`
	WalletID              uuid.UUID                   `json:"walletId"`
	PlayerID              uuid.UUID                   `json:"playerId"`
	RoundID               string                      `json:"roundId"`
	GameID                string                      `json:"gameId"`
	Type                  domain.WagerTransactionType `json:"type"`
	Amount                domain.Money                `json:"amount"`
	FailureCode           string                      `json:"failureCode"`
	FailureMessage        string                      `json:"failureMessage,omitempty"`
}

// WalletBalanceChangedPayload describes a committed wallet balance movement.
type WalletBalanceChangedPayload struct {
	WalletID      uuid.UUID                    `json:"walletId"`
	PlayerID      uuid.UUID                    `json:"playerId"`
	TransactionID uuid.UUID                    `json:"transactionId"`
	Direction     domain.WalletLedgerDirection `json:"direction"`
	Amount        domain.Money                 `json:"amount"`
	BalanceBefore domain.Money                 `json:"balanceBefore"`
	BalanceAfter  domain.Money                 `json:"balanceAfter"`
	WalletVersion int64                        `json:"walletVersion"`
}

// WagerTransactionPendingReferencePayload describes a wager waiting for a reference.
type WagerTransactionPendingReferencePayload struct {
	TransactionID                  uuid.UUID                   `json:"transactionId"`
	ProviderID                     string                      `json:"providerId"`
	ExternalTransactionID          string                      `json:"externalTransactionId"`
	ReferenceExternalTransactionID string                      `json:"referenceExternalTransactionId"`
	WalletID                       uuid.UUID                   `json:"walletId"`
	PlayerID                       uuid.UUID                   `json:"playerId"`
	RoundID                        string                      `json:"roundId"`
	GameID                         string                      `json:"gameId"`
	Type                           domain.WagerTransactionType `json:"type"`
	Amount                         domain.Money                `json:"amount"`
}
