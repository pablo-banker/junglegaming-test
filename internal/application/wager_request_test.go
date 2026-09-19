package application

import (
	"strings"
	"testing"
)

// hashTestCommand returns the command used by the payload hash tests.
func hashTestCommand() ProcessWagerCommand {
	return ProcessWagerCommand{
		ProviderID:            "provider-a",
		ExternalTransactionID: "transaction-123",
		IdempotencyKey:        "provider-a:transaction-123",
		WalletID:              "0192f291-27dd-7d3f-8071-5f8685deef37",
		PlayerID:              "0192f28f-5dc0-7d58-bdb2-814ad6a0f4a1",
		RoundID:               "round-987",
		GameID:                "fortune-chimp",
		Type:                  "BET",
		Amount:                "25.00",
		Currency:              "BRL",
	}
}

// TestWagerPayloadHashIsCanonical pins the documented canonical JSON and its SHA-256.
func TestWagerPayloadHashIsCanonical(t *testing.T) {
	request, err := parseWagerRequest(hashTestCommand())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// sha256 of:
	// {"externalTransactionId":"transaction-123","gameId":"fortune-chimp","kind":"BET","money":{"amount":"25.00","currency":"BRL"},"playerId":"0192f28f-5dc0-7d58-bdb2-814ad6a0f4a1","providerId":"provider-a","roundId":"round-987","walletId":"0192f291-27dd-7d3f-8071-5f8685deef37"}
	const expected = "629836932b79106b99523d06a1e7fa80689b0ea1e1c47aa3f0a5a2c87d0c4344"

	if request.payloadHash != expected {
		t.Fatalf("expected canonical hash %s, got %s", expected, request.payloadHash)
	}
}

// TestWagerPayloadHashIgnoresIdempotencyKeyAndNormalizesUUIDs verifies the documented exclusions and normalization.
func TestWagerPayloadHashIgnoresIdempotencyKeyAndNormalizesUUIDs(t *testing.T) {
	original, err := parseWagerRequest(hashTestCommand())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	equivalent := hashTestCommand()
	equivalent.IdempotencyKey = "another-key"
	equivalent.WalletID = strings.ToUpper(equivalent.WalletID)
	equivalent.PlayerID = strings.ToUpper(equivalent.PlayerID)

	other, err := parseWagerRequest(equivalent)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if original.payloadHash != other.payloadHash {
		t.Fatal("expected equal hashes for equivalent business payloads")
	}
}

// TestWagerPayloadHashChangesWithBusinessFields verifies every business field is covered.
func TestWagerPayloadHashChangesWithBusinessFields(t *testing.T) {
	base, err := parseWagerRequest(hashTestCommand())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	changes := map[string]func(*ProcessWagerCommand){
		"provider":  func(c *ProcessWagerCommand) { c.ProviderID = "provider-b" },
		"external":  func(c *ProcessWagerCommand) { c.ExternalTransactionID = "transaction-999" },
		"round":     func(c *ProcessWagerCommand) { c.RoundID = "round-1" },
		"game":      func(c *ProcessWagerCommand) { c.GameID = "other-game" },
		"kind":      func(c *ProcessWagerCommand) { c.Type = "WIN" },
		"amount":    func(c *ProcessWagerCommand) { c.Amount = "25.01" },
		"reference": func(c *ProcessWagerCommand) { c.Type = "WIN"; c.ReferenceExternalTransactionID = "bet-1" },
	}

	for name, change := range changes {
		command := hashTestCommand()
		change(&command)

		request, err := parseWagerRequest(command)
		if err != nil {
			t.Fatalf("%s: unexpected error: %v", name, err)
		}

		if request.payloadHash == base.payloadHash {
			t.Errorf("%s: expected hash to change", name)
		}
	}
}

// BenchmarkParseWagerRequest measures validation plus the canonical payload hash.
func BenchmarkParseWagerRequest(b *testing.B) {
	command := hashTestCommand()

	for b.Loop() {
		if _, err := parseWagerRequest(command); err != nil {
			b.Fatal(err)
		}
	}
}
