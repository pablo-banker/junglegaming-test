//go:build integration

package integration

import (
	"encoding/json"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/pablo-banker/junglegaming-test/internal/config"
)

// sqsHandlerConfig allows the provider used by the SQS integration messages.
func sqsHandlerConfig() config.Config {
	return config.Config{SQSAllowedProviders: []string{"provider-a"}}
}

// discardLogger returns a logger that drops every record.
func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

type wagerRequestedMessage struct {
	MessageID             string
	ProviderID            string
	ExternalTransactionID string
	IdempotencyKey        string
	WalletID              string
	PlayerID              string
	Kind                  string
	Amount                string
	Reference             string
}

// wagerRequestedMessageBody builds a WagerTransactionRequested envelope following the challenge contract.
func wagerRequestedMessageBody(t *testing.T, message wagerRequestedMessage) string {
	t.Helper()

	data := map[string]any{
		"providerId":            message.ProviderID,
		"externalTransactionId": message.ExternalTransactionID,
		"idempotencyKey":        message.IdempotencyKey,
		"playerId":              message.PlayerID,
		"walletId":              message.WalletID,
		"roundId":               "round-123",
		"gameId":                "game-123",
		"kind":                  message.Kind,
		"money": map[string]string{
			"amount":   message.Amount,
			"currency": "BRL",
		},
	}

	if message.Reference != "" {
		data["referenceExternalTransactionId"] = message.Reference
	}

	body, err := json.Marshal(map[string]any{
		"messageId":  message.MessageID,
		"type":       "WagerTransactionRequested",
		"occurredAt": time.Now().UTC().Format(time.RFC3339Nano),
		"data":       data,
	})
	if err != nil {
		t.Fatalf("failed to build wager message: %v", err)
	}

	return string(body)
}
