package sqs

import (
	"encoding/json"
	"strings"
	"time"
)

const wagerRequestedMessageType = "WagerTransactionRequested"

type WagerMessageEnvelope struct {
	MessageID  string           `json:"messageId"`
	Type       string           `json:"type"`
	OccurredAt time.Time        `json:"occurredAt"`
	Data       WagerMessageData `json:"data"`
}

type WagerMessageData struct {
	ProviderID                     string            `json:"providerId"`
	ExternalTransactionID          string            `json:"externalTransactionId"`
	IdempotencyKey                 string            `json:"idempotencyKey"`
	PlayerID                       string            `json:"playerId"`
	WalletID                       string            `json:"walletId"`
	RoundID                        string            `json:"roundId"`
	GameID                         string            `json:"gameId"`
	Kind                           string            `json:"kind"`
	Money                          WagerMessageMoney `json:"money"`
	ReferenceExternalTransactionID *string           `json:"referenceExternalTransactionId,omitempty"`
}

type WagerMessageMoney struct {
	Amount   string `json:"amount"`
	Currency string `json:"currency"`
}

// parseWagerMessage parses and validates the SQS wager message envelope.
func parseWagerMessage(body string) (WagerMessageEnvelope, error) {
	var message WagerMessageEnvelope

	if strings.TrimSpace(body) == "" {
		return message, ErrInvalidMessageBody
	}

	if err := json.Unmarshal([]byte(body), &message); err != nil {
		return message, err
	}

	if strings.TrimSpace(message.MessageID) == "" {
		return message, ErrInvalidWagerMessageID
	}

	if message.Type != wagerRequestedMessageType {
		return message, ErrInvalidWagerMessageType
	}

	if message.OccurredAt.IsZero() {
		return message, ErrInvalidWagerMessageOccurredAt
	}

	return message, nil
}
