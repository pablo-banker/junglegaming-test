package application

import (
	"encoding/json"
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

	// eventVersion is the schema version of every event data type defined below.
	eventVersion int64 = 1
)

// Event is an integration event with typed data, built only by the constructors below.
type Event[T any] struct {
	EventID       uuid.UUID
	EventType     EventType
	AggregateID   uuid.UUID
	CorrelationID string
	CausationID   string
	OccurredAt    time.Time
	Version       int64
	Data          T
}

// OutboxEvent is an event ready to be stored in the transactional outbox.
type OutboxEvent interface {
	Envelope() (EventEnvelope, error)
}

// EventEnvelope is the immutable snapshot persisted in the outbox and published later.
type EventEnvelope struct {
	EventID       uuid.UUID
	EventType     EventType
	AggregateID   uuid.UUID
	CorrelationID string
	CausationID   string
	OccurredAt    time.Time
	Version       int64
	Data          json.RawMessage
}

// Envelope serializes the typed data into the outbox snapshot.
func (e Event[T]) Envelope() (EventEnvelope, error) {
	data, err := json.Marshal(e.Data)
	if err != nil {
		return EventEnvelope{}, err
	}

	return EventEnvelope{
		EventID:       e.EventID,
		EventType:     e.EventType,
		AggregateID:   e.AggregateID,
		CorrelationID: e.CorrelationID,
		CausationID:   e.CausationID,
		OccurredAt:    e.OccurredAt,
		Version:       e.Version,
		Data:          data,
	}, nil
}

// newEvent fills the envelope metadata shared by every event.
func newEvent[T any](eventType EventType, aggregateID uuid.UUID, metadata CommandMetadata, occurredAt time.Time, data T) Event[T] {
	return Event[T]{
		EventID:       uuid.New(),
		EventType:     eventType,
		AggregateID:   aggregateID,
		CorrelationID: metadata.CorrelationID,
		CausationID:   metadata.CausationID,
		OccurredAt:    occurredAt.UTC(),
		Version:       eventVersion,
		Data:          data,
	}
}

// WagerTransactionProcessedData describes a successfully processed operation, including LOSS and OPENING.
type WagerTransactionProcessedData struct {
	TransactionID          uuid.UUID                   `json:"transactionId"`
	ProviderID             string                      `json:"providerId,omitempty"`
	ExternalTransactionID  string                      `json:"externalTransactionId,omitempty"`
	WalletID               uuid.UUID                   `json:"walletId"`
	PlayerID               uuid.UUID                   `json:"playerId"`
	RoundID                string                      `json:"roundId,omitempty"`
	GameID                 string                      `json:"gameId,omitempty"`
	Kind                   domain.WagerTransactionType `json:"kind"`
	Money                  domain.Money                `json:"money"`
	BalanceBefore          domain.Money                `json:"balanceBefore"`
	BalanceAfter           domain.Money                `json:"balanceAfter"`
	ReferenceTransactionID *uuid.UUID                  `json:"referenceTransactionId,omitempty"`
}

// WagerTransactionRejectedData describes an operation definitively rejected by a business rule.
type WagerTransactionRejectedData struct {
	TransactionID         uuid.UUID                   `json:"transactionId"`
	ProviderID            string                      `json:"providerId"`
	ExternalTransactionID string                      `json:"externalTransactionId"`
	WalletID              uuid.UUID                   `json:"walletId"`
	PlayerID              uuid.UUID                   `json:"playerId"`
	RoundID               string                      `json:"roundId"`
	GameID                string                      `json:"gameId"`
	Kind                  domain.WagerTransactionType `json:"kind"`
	Money                 domain.Money                `json:"money"`
	FailureCode           string                      `json:"failureCode"`
	FailureMessage        string                      `json:"failureMessage,omitempty"`
}

// WalletBalanceChangedData describes a committed wallet balance movement.
type WalletBalanceChangedData struct {
	WalletID      uuid.UUID                    `json:"walletId"`
	PlayerID      uuid.UUID                    `json:"playerId"`
	TransactionID uuid.UUID                    `json:"transactionId"`
	Direction     domain.WalletLedgerDirection `json:"direction"`
	Money         domain.Money                 `json:"money"`
	BalanceBefore domain.Money                 `json:"balanceBefore"`
	BalanceAfter  domain.Money                 `json:"balanceAfter"`
	WalletVersion int64                        `json:"walletVersion"`
}

// WagerTransactionPendingReferenceData describes an operation waiting for its reference.
type WagerTransactionPendingReferenceData struct {
	TransactionID                  uuid.UUID                   `json:"transactionId"`
	ProviderID                     string                      `json:"providerId"`
	ExternalTransactionID          string                      `json:"externalTransactionId"`
	ReferenceExternalTransactionID string                      `json:"referenceExternalTransactionId"`
	WalletID                       uuid.UUID                   `json:"walletId"`
	PlayerID                       uuid.UUID                   `json:"playerId"`
	RoundID                        string                      `json:"roundId"`
	GameID                         string                      `json:"gameId"`
	Kind                           domain.WagerTransactionType `json:"kind"`
	Money                          domain.Money                `json:"money"`
}

// NewWagerTransactionProcessedEvent creates a WagerTransactionProcessed event.
func NewWagerTransactionProcessedEvent(metadata CommandMetadata, occurredAt time.Time, data WagerTransactionProcessedData) Event[WagerTransactionProcessedData] {
	return newEvent(EventTypeWagerTransactionProcessed, data.TransactionID, metadata, occurredAt, data)
}

// NewWagerTransactionRejectedEvent creates a WagerTransactionRejected event.
func NewWagerTransactionRejectedEvent(metadata CommandMetadata, occurredAt time.Time, data WagerTransactionRejectedData) Event[WagerTransactionRejectedData] {
	return newEvent(EventTypeWagerTransactionRejected, data.TransactionID, metadata, occurredAt, data)
}

// NewWalletBalanceChangedEvent creates a WalletBalanceChanged event.
func NewWalletBalanceChangedEvent(metadata CommandMetadata, occurredAt time.Time, data WalletBalanceChangedData) Event[WalletBalanceChangedData] {
	return newEvent(EventTypeWalletBalanceChanged, data.WalletID, metadata, occurredAt, data)
}

// NewWagerTransactionPendingReferenceEvent creates a WagerTransactionPendingReference event.
func NewWagerTransactionPendingReferenceEvent(metadata CommandMetadata, occurredAt time.Time, data WagerTransactionPendingReferenceData) Event[WagerTransactionPendingReferenceData] {
	return newEvent(EventTypeWagerTransactionPendingReference, data.TransactionID, metadata, occurredAt, data)
}
