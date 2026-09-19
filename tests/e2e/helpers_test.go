//go:build e2e

package e2e

import (
	"net/http"
	"testing"

	"github.com/google/uuid"
)

type wagerRequest struct {
	ProviderID                     string        `json:"providerId"`
	ExternalTransactionID          string        `json:"externalTransactionId"`
	PlayerID                       string        `json:"playerId"`
	WalletID                       string        `json:"walletId"`
	RoundID                        string        `json:"roundId"`
	GameID                         string        `json:"gameId"`
	Kind                           string        `json:"kind"`
	Money                          moneyResponse `json:"money"`
	ReferenceExternalTransactionID string        `json:"referenceExternalTransactionId,omitempty"`
}

// internalToken obtains the internal service token.
func internalToken(t *testing.T) string {
	t.Helper()

	return getAccessToken(
		t,
		envOrDefault("KEYCLOAK_INTERNAL_CLIENT_ID", "internal-service"),
		envOrDefault("KEYCLOAK_INTERNAL_CLIENT_SECRET", "internal-service-secret"),
	)
}

// providerAToken obtains the provider-a token.
func providerAToken(t *testing.T) string {
	t.Helper()

	return getAccessToken(
		t,
		envOrDefault("KEYCLOAK_PROVIDER_A_CLIENT_ID", "provider-a"),
		envOrDefault("KEYCLOAK_PROVIDER_A_CLIENT_SECRET", "provider-a-secret"),
	)
}

// providerBToken obtains the provider-b token.
func providerBToken(t *testing.T) string {
	t.Helper()

	return getAccessToken(
		t,
		envOrDefault("KEYCLOAK_PROVIDER_B_CLIENT_ID", "provider-b"),
		envOrDefault("KEYCLOAK_PROVIDER_B_CLIENT_SECRET", "provider-b-secret"),
	)
}

// createWalletE2E creates a wallet using the real HTTP API.
func createWalletE2E(t *testing.T, token string, playerID string, amount string) walletResponse {
	t.Helper()

	body := map[string]any{
		"playerId": playerID,
		"initialBalance": map[string]string{
			"amount":   amount,
			"currency": "BRL",
		},
	}

	response := doJSON(t, http.MethodPost, "/wallets", token, "", body)
	if response.StatusCode != http.StatusCreated {
		response.Body.Close()
		t.Fatalf("expected wallet creation status 201, got %d", response.StatusCode)
	}

	return decodeSuccess[walletResponse](t, response)
}

// createRandomWalletE2E creates a wallet for a random player.
func createRandomWalletE2E(t *testing.T, token string, amount string) walletResponse {
	t.Helper()

	return createWalletE2E(t, token, uuid.NewString(), amount)
}

// newWagerRequestE2E creates a basic wager request.
func newWagerRequestE2E(wallet walletResponse, providerID string, kind string, amount string) wagerRequest {
	return wagerRequest{
		ProviderID:            providerID,
		ExternalTransactionID: "transaction-" + uuid.NewString(),
		PlayerID:              wallet.PlayerID,
		WalletID:              wallet.ID,
		RoundID:               "round-" + uuid.NewString(),
		GameID:                "fortune-chimp",
		Kind:                  kind,
		Money: moneyResponse{
			Amount:   amount,
			Currency: "BRL",
		},
	}
}

// submitWagerE2E submits a wager and decodes its successful business response.
func submitWagerE2E(t *testing.T, token string, key string, request wagerRequest, expectedStatus int) wagerProcessResponse {
	t.Helper()

	response := doJSON(t, http.MethodPost, "/wagering/transactions", token, key, request)
	if response.StatusCode != expectedStatus {
		response.Body.Close()
		t.Fatalf("expected wager status %d, got %d", expectedStatus, response.StatusCode)
	}

	return decodeSuccess[wagerProcessResponse](t, response)
}

// getWalletE2E retrieves a wallet using the internal service.
func getWalletE2E(t *testing.T, token string, walletID string) walletResponse {
	t.Helper()

	response := doJSON(t, http.MethodGet, "/wallets/"+walletID, token, "", nil)
	if response.StatusCode != http.StatusOK {
		response.Body.Close()
		t.Fatalf("expected wallet status 200, got %d", response.StatusCode)
	}

	return decodeSuccess[walletResponse](t, response)
}

// getLedgerE2E retrieves a wallet ledger.
func getLedgerE2E(t *testing.T, token string, walletID string) ledgerResponse {
	t.Helper()

	response := doJSON(t, http.MethodGet, "/wallets/"+walletID+"/ledger?limit=100", token, "", nil)
	if response.StatusCode != http.StatusOK {
		response.Body.Close()
		t.Fatalf("expected ledger status 200, got %d", response.StatusCode)
	}

	return decodeSuccess[ledgerResponse](t, response)
}
