//go:build e2e

package e2e

import (
	"net/http"
	"os"
	"testing"

	"github.com/google/uuid"
)

// TestHTTPFinancialFlow verifies the authenticated HTTP financial flow end to end.
func TestHTTPFinancialFlow(t *testing.T) {
	internalToken := getAccessToken(t, envOrDefault("KEYCLOAK_INTERNAL_CLIENT_ID", "internal-service"), envOrDefault("KEYCLOAK_INTERNAL_CLIENT_SECRET", "internal-service-secret"))
	providerAToken := getAccessToken(t, envOrDefault("KEYCLOAK_PROVIDER_A_CLIENT_ID", "provider-a"), envOrDefault("KEYCLOAK_PROVIDER_A_CLIENT_SECRET", "provider-a-secret"))
	providerBToken := getAccessToken(t, envOrDefault("KEYCLOAK_PROVIDER_B_CLIENT_ID", "provider-b"), envOrDefault("KEYCLOAK_PROVIDER_B_CLIENT_SECRET", "provider-b-secret"))

	playerID := uuid.NewString()

	createWallet := map[string]any{
		"playerId": playerID,
		"initialBalance": map[string]string{
			"amount":   "100.00",
			"currency": "BRL",
		},
	}

	response := doJSON(t, http.MethodPost, "/wallets", internalToken, "", createWallet)
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("expected wallet creation status 201, got %d", response.StatusCode)
	}

	wallet := decodeSuccess[walletResponse](t, response)

	if wallet.PlayerID != playerID {
		t.Errorf("expected player %s, got %s", playerID, wallet.PlayerID)
	}

	if wallet.Balance.Amount != "100.00" {
		t.Errorf("expected wallet balance 100.00, got %s", wallet.Balance.Amount)
	}

	if wallet.Version != 1 {
		t.Errorf("expected wallet version 1, got %d", wallet.Version)
	}

	response = doJSON(t, http.MethodGet, "/wallets/"+wallet.ID, providerAToken, "", nil)
	if response.StatusCode != http.StatusForbidden {
		response.Body.Close()
		t.Fatalf("expected provider wallet access status 403, got %d", response.StatusCode)
	}
	response.Body.Close()

	externalTransactionID := "transaction-" + uuid.NewString()
	idempotencyKey := "provider-a:" + externalTransactionID

	wager := map[string]any{
		"providerId":            "provider-a",
		"externalTransactionId": externalTransactionID,
		"playerId":              playerID,
		"walletId":              wallet.ID,
		"roundId":               "round-" + uuid.NewString(),
		"gameId":                "fortune-chimp",
		"kind":                  "BET",
		"money": map[string]string{
			"amount":   "25.00",
			"currency": "BRL",
		},
	}

	response = doJSON(t, http.MethodPost, "/wagering/transactions", providerAToken, idempotencyKey, wager)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("expected wager status 200, got %d", response.StatusCode)
	}

	processed := decodeSuccess[wagerProcessResponse](t, response)

	if processed.Status != "PROCESSED" {
		t.Errorf("expected PROCESSED, got %s", processed.Status)
	}

	if processed.IdempotentReplay {
		t.Fatal("expected first request not to be an idempotent replay")
	}

	if processed.Balance == nil || processed.Balance.Amount != "75.00" {
		t.Fatal("expected processed balance 75.00")
	}

	response = doJSON(t, http.MethodPost, "/wagering/transactions", providerAToken, idempotencyKey, wager)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("expected replay status 200, got %d", response.StatusCode)
	}

	replay := decodeSuccess[wagerProcessResponse](t, response)

	if !replay.IdempotentReplay {
		t.Fatal("expected idempotent replay")
	}

	if replay.TransactionID != processed.TransactionID {
		t.Errorf("expected transaction %s, got %s", processed.TransactionID, replay.TransactionID)
	}

	if replay.Balance == nil || replay.Balance.Amount != "75.00" {
		t.Fatal("expected replay balance 75.00")
	}

	response = doJSON(t, http.MethodGet, "/wallets/"+wallet.ID, internalToken, "", nil)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("expected wallet status 200, got %d", response.StatusCode)
	}

	updatedWallet := decodeSuccess[walletResponse](t, response)

	if updatedWallet.Balance.Amount != "75.00" {
		t.Errorf("expected wallet balance 75.00, got %s", updatedWallet.Balance.Amount)
	}

	if updatedWallet.Version != 2 {
		t.Errorf("expected wallet version 2, got %d", updatedWallet.Version)
	}

	response = doJSON(t, http.MethodGet, "/wallets/"+wallet.ID+"/ledger?limit=50", internalToken, "", nil)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("expected ledger status 200, got %d", response.StatusCode)
	}

	ledger := decodeSuccess[ledgerResponse](t, response)

	if len(ledger.Entries) != 2 {
		t.Fatalf("expected 2 ledger entries, got %d", len(ledger.Entries))
	}

	response = doJSON(t, http.MethodPost, "/wallets/"+wallet.ID+"/reconciliation", internalToken, "", nil)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("expected reconciliation status 200, got %d", response.StatusCode)
	}

	reconciliation := decodeSuccess[reconciliationResponse](t, response)

	if !reconciliation.Consistent {
		t.Fatal("expected reconciliation to be consistent")
	}

	if reconciliation.StoredBalance.Amount != "75.00" {
		t.Errorf("expected stored balance 75.00, got %s", reconciliation.StoredBalance.Amount)
	}

	if reconciliation.CalculatedBalance.Amount != "75.00" {
		t.Errorf("expected calculated balance 75.00, got %s", reconciliation.CalculatedBalance.Amount)
	}

	if reconciliation.Difference.Amount != "0.00" {
		t.Errorf("expected difference 0.00, got %s", reconciliation.Difference.Amount)
	}

	if reconciliation.CheckedEntries != 2 {
		t.Errorf("expected 2 checked entries, got %d", reconciliation.CheckedEntries)
	}

	response = doJSON(t, http.MethodGet, "/wagering/transactions/"+processed.TransactionID, providerAToken, "", nil)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("expected wager query status 200, got %d", response.StatusCode)
	}

	transaction := decodeSuccess[wagerResponse](t, response)

	if transaction.ProviderID != "provider-a" {
		t.Errorf("expected provider-a, got %s", transaction.ProviderID)
	}

	if transaction.ExternalTransactionID != externalTransactionID {
		t.Errorf("expected external transaction %s, got %s", externalTransactionID, transaction.ExternalTransactionID)
	}

	response = doJSON(t, http.MethodGet, "/wagering/transactions/"+processed.TransactionID, providerBToken, "", nil)
	if response.StatusCode != http.StatusNotFound {
		response.Body.Close()
		t.Fatalf("expected provider isolation status 404, got %d", response.StatusCode)
	}
	response.Body.Close()

	response = doJSON(t, http.MethodGet, "/providers/provider-a/wagering/transactions/"+externalTransactionID, providerAToken, "", nil)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("expected external transaction query status 200, got %d", response.StatusCode)
	}
	response.Body.Close()

	response = doJSON(t, http.MethodGet, "/providers/provider-a/wagering/transactions/"+externalTransactionID, providerBToken, "", nil)
	if response.StatusCode != http.StatusForbidden {
		response.Body.Close()
		t.Fatalf("expected cross-provider status 403, got %d", response.StatusCode)
	}
	response.Body.Close()
}

// TestHTTPRejectsMissingAuthentication verifies business endpoints require authentication.
func TestHTTPRejectsMissingAuthentication(t *testing.T) {
	response := doJSON(t, http.MethodGet, "/wallets/"+uuid.NewString(), "", "", nil)
	defer response.Body.Close()

	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected status 401, got %d", response.StatusCode)
	}
}

// envOrDefault returns an environment variable or its local default.
func envOrDefault(key string, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}

	return fallback
}
