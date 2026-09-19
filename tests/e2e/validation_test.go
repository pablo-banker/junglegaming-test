//go:build e2e

package e2e

import (
	"bytes"
	"io"
	"net/http"
	"testing"

	"github.com/google/uuid"
)

// TestE2ERejectsMalformedJSON verifies malformed request bodies are rejected.
func TestE2ERejectsMalformedJSON(t *testing.T) {
	token := internalToken(t)

	request, err := http.NewRequest(http.MethodPost, apiURL()+"/wallets", bytes.NewBufferString(`{"playerId":`))
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", "application/json")

	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}

	defer response.Body.Close()

	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", response.StatusCode)
	}
}

// TestE2ERejectsInvalidPlayerID verifies invalid UUIDs do not become internal errors.
func TestE2ERejectsInvalidPlayerID(t *testing.T) {
	token := internalToken(t)

	body := map[string]any{
		"playerId": "definitely-not-a-uuid",
		"initialBalance": map[string]string{
			"amount":   "100.00",
			"currency": "BRL",
		},
	}

	response := doJSON(t, http.MethodPost, "/wallets", token, "", body)
	defer response.Body.Close()

	if response.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("expected status 422, got %d", response.StatusCode)
	}
}

// TestE2ERejectsInvalidMoney verifies invalid financial representations are rejected.
func TestE2ERejectsInvalidMoney(t *testing.T) {
	token := internalToken(t)

	tests := []string{
		"-1.00",
		"1.001",
		"1e2",
		"NaN",
		"Infinity",
	}

	for _, amount := range tests {
		t.Run(amount, func(t *testing.T) {
			body := map[string]any{
				"playerId": uuid.NewString(),
				"initialBalance": map[string]string{
					"amount":   amount,
					"currency": "BRL",
				},
			}

			response := doJSON(t, http.MethodPost, "/wallets", token, "", body)
			defer response.Body.Close()

			if response.StatusCode != http.StatusUnprocessableEntity {
				t.Fatalf("expected status 422 for %s, got %d", amount, response.StatusCode)
			}
		})
	}
}

// TestE2ERejectsMissingIdempotencyKey verifies wagers require persistent idempotency.
func TestE2ERejectsMissingIdempotencyKey(t *testing.T) {
	internal := internalToken(t)
	provider := providerAToken(t)

	wallet := createRandomWalletE2E(t, internal, "100.00")
	request := newWagerRequestE2E(wallet, "provider-a", "BET", "10.00")

	response := doJSON(t, http.MethodPost, "/wagering/transactions", provider, "", request)
	defer response.Body.Close()

	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", response.StatusCode)
	}
}

// TestE2ERejectsExternalOpening verifies OPENING cannot enter through provider APIs.
func TestE2ERejectsExternalOpening(t *testing.T) {
	internal := internalToken(t)
	provider := providerAToken(t)

	wallet := createRandomWalletE2E(t, internal, "100.00")
	request := newWagerRequestE2E(wallet, "provider-a", "OPENING", "10.00")

	response := doJSON(t, http.MethodPost, "/wagering/transactions", provider, "key-"+uuid.NewString(), request)
	defer response.Body.Close()

	if response.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("expected status 422, got %d", response.StatusCode)
	}
}

// TestE2ERejectsZeroBet verifies BET requires a positive amount.
func TestE2ERejectsZeroBet(t *testing.T) {
	internal := internalToken(t)
	provider := providerAToken(t)

	wallet := createRandomWalletE2E(t, internal, "100.00")
	request := newWagerRequestE2E(wallet, "provider-a", "BET", "0.00")

	response := doJSON(t, http.MethodPost, "/wagering/transactions", provider, "key-"+uuid.NewString(), request)
	defer response.Body.Close()

	if response.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("expected status 422, got %d", response.StatusCode)
	}
}

// TestE2ERejectsNonZeroLoss verifies LOSS must have zero amount.
func TestE2ERejectsNonZeroLoss(t *testing.T) {
	internal := internalToken(t)
	provider := providerAToken(t)

	wallet := createRandomWalletE2E(t, internal, "100.00")
	request := newWagerRequestE2E(wallet, "provider-a", "LOSS", "1.00")

	response := doJSON(t, http.MethodPost, "/wagering/transactions", provider, "key-"+uuid.NewString(), request)
	defer response.Body.Close()

	if response.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("expected status 422, got %d", response.StatusCode)
	}
}

// TestE2ERejectsCurrencyMismatch verifies wagers cannot cross currencies.
func TestE2ERejectsCurrencyMismatch(t *testing.T) {
	internal := internalToken(t)
	provider := providerAToken(t)

	wallet := createRandomWalletE2E(t, internal, "100.00")
	request := newWagerRequestE2E(wallet, "provider-a", "BET", "10.00")
	request.Money.Currency = "USD"

	response := doJSON(t, http.MethodPost, "/wagering/transactions", provider, "key-"+uuid.NewString(), request)

	if response.StatusCode != http.StatusUnprocessableEntity {
		body, _ := io.ReadAll(response.Body)
		response.Body.Close()

		t.Fatalf("expected status 422, got %d: %s", response.StatusCode, body)
	}

	response.Body.Close()
}
