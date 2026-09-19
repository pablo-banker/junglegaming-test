package sqs

import (
	"errors"
	"testing"
	"time"
)

// validWagerMessageBody returns a valid SQS wager envelope.
func validWagerMessageBody() string {
	return `{
		"messageId":"msg-123",
		"type":"WagerTransactionRequested",
		"occurredAt":"2026-09-08T12:00:00.000Z",
		"data":{
			"providerId":"provider-a",
			"externalTransactionId":"transaction-123",
			"idempotencyKey":"provider-a:transaction-123",
			"playerId":"0192f28f-5dc0-7d58-bdb2-814ad6a0f4a1",
			"walletId":"0192f291-27dd-7d3f-8071-5f8685deef37",
			"roundId":"round-987",
			"gameId":"fortune-chimp",
			"kind":"BET",
			"money":{
				"amount":"25.00",
				"currency":"BRL"
			}
		}
	}`
}

// TestParseWagerMessageParsesValidEnvelope verifies the challenge SQS contract is parsed correctly.
func TestParseWagerMessageParsesValidEnvelope(t *testing.T) {
	message, err := parseWagerMessage(validWagerMessageBody())
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}

	if message.MessageID != "msg-123" {
		t.Errorf("expected msg-123, got %s", message.MessageID)
	}

	if message.Type != wagerRequestedMessageType {
		t.Errorf("expected %s, got %s", wagerRequestedMessageType, message.Type)
	}

	expectedTime := time.Date(
		2026,
		time.September,
		8,
		12,
		0,
		0,
		0,
		time.UTC,
	)

	if !message.OccurredAt.Equal(expectedTime) {
		t.Errorf("expected %s, got %s", expectedTime, message.OccurredAt)
	}

	if message.Data.ProviderID != "provider-a" {
		t.Errorf("expected provider-a, got %s", message.Data.ProviderID)
	}

	if message.Data.ExternalTransactionID != "transaction-123" {
		t.Errorf("expected transaction-123, got %s", message.Data.ExternalTransactionID)
	}

	if message.Data.IdempotencyKey != "provider-a:transaction-123" {
		t.Errorf("expected idempotency key provider-a:transaction-123, got %s", message.Data.IdempotencyKey)
	}

	if message.Data.Kind != "BET" {
		t.Errorf("expected BET, got %s", message.Data.Kind)
	}

	if message.Data.Money.Amount != "25.00" {
		t.Errorf("expected 25.00, got %s", message.Data.Money.Amount)
	}

	if message.Data.Money.Currency != "BRL" {
		t.Errorf("expected BRL, got %s", message.Data.Money.Currency)
	}
}

// TestParseWagerMessageRejectsInvalidEnvelope verifies invalid transport metadata is rejected.
func TestParseWagerMessageRejectsInvalidEnvelope(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		wantErr error
	}{
		{
			name:    "empty body",
			body:    "",
			wantErr: ErrInvalidMessageBody,
		},
		{
			name: "missing message id",
			body: `{
				"type":"WagerTransactionRequested",
				"occurredAt":"2026-09-08T12:00:00Z",
				"data":{}
			}`,
			wantErr: ErrInvalidWagerMessageID,
		},
		{
			name: "blank message id",
			body: `{
				"messageId":"   ",
				"type":"WagerTransactionRequested",
				"occurredAt":"2026-09-08T12:00:00Z",
				"data":{}
			}`,
			wantErr: ErrInvalidWagerMessageID,
		},
		{
			name: "invalid type",
			body: `{
				"messageId":"msg-123",
				"type":"SomethingElse",
				"occurredAt":"2026-09-08T12:00:00Z",
				"data":{}
			}`,
			wantErr: ErrInvalidWagerMessageType,
		},
		{
			name: "missing occurred at",
			body: `{
				"messageId":"msg-123",
				"type":"WagerTransactionRequested",
				"data":{}
			}`,
			wantErr: ErrInvalidWagerMessageOccurredAt,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseWagerMessage(tt.body)

			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("expected %v, got %v", tt.wantErr, err)
			}
		})
	}
}

// TestParseWagerMessageRejectsInvalidTimestamp verifies occurredAt must use a valid RFC3339 timestamp.
func TestParseWagerMessageRejectsInvalidTimestamp(t *testing.T) {
	body := `{
		"messageId":"msg-123",
		"type":"WagerTransactionRequested",
		"occurredAt":"not-a-date",
		"data":{}
	}`

	_, err := parseWagerMessage(body)

	if err == nil {
		t.Fatal("expected invalid timestamp error")
	}
}
