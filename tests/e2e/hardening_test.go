//go:build e2e

package e2e

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

// decodeError decodes the API error contract.
func decodeError(t *testing.T, response *http.Response) errorResponse {
	t.Helper()

	defer response.Body.Close()

	var body errorResponse

	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode error response: %v", err)
	}

	return body
}

// assertWalletBalance verifies a wallet balance through the API.
func assertWalletBalance(t *testing.T, walletID string, expected string) {
	t.Helper()

	if balance := getWalletE2E(t, internalToken(t), walletID).Balance.Amount; balance != expected {
		t.Fatalf("expected balance %s, got %s", expected, balance)
	}
}

// TestE2ERejectsExpiredToken uses a real Keycloak token that expires after 1 second.
func TestE2ERejectsExpiredToken(t *testing.T) {
	wallet := createRandomWalletE2E(t, internalToken(t), "100.00")

	token := getAccessToken(
		t,
		envOrDefault("KEYCLOAK_SHORT_LIVED_CLIENT_ID", "provider-a-short-lived"),
		envOrDefault("KEYCLOAK_SHORT_LIVED_CLIENT_SECRET", "provider-a-short-lived-secret"),
	)

	time.Sleep(2500 * time.Millisecond)

	request := newWagerRequestE2E(wallet, "provider-a", "BET", "25.00")
	response := doJSON(t, http.MethodPost, "/wagering/transactions", token, "provider-a:"+request.ExternalTransactionID, request)

	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 for an expired token, got %d", response.StatusCode)
	}

	if body := decodeError(t, response); body.Code != "UNAUTHORIZED" {
		t.Fatalf("expected UNAUTHORIZED, got %s", body.Code)
	}

	assertWalletBalance(t, wallet.ID, "100.00")
}

// TestE2EProvidersAreIsolatedOnReplays verifies idempotency keys and external ids are scoped
// by the authenticated provider: provider-b cannot replay or read provider-a's result.
func TestE2EProvidersAreIsolatedOnReplays(t *testing.T) {
	wallet := createRandomWalletE2E(t, internalToken(t), "100.00")

	requestA := newWagerRequestE2E(wallet, "provider-a", "BET", "10.00")
	key := "shared-key-" + uuid.NewString()

	resultA := submitWagerE2E(t, providerAToken(t), key, requestA, http.StatusOK)

	requestB := requestA
	requestB.ProviderID = "provider-b"

	resultB := submitWagerE2E(t, providerBToken(t), key, requestB, http.StatusOK)

	if resultB.IdempotentReplay || resultB.TransactionID == resultA.TransactionID {
		t.Fatalf("expected provider-b to get its own transaction, got %+v", resultB)
	}

	response := doJSON(t, http.MethodGet, "/wagering/transactions/"+resultA.TransactionID, providerBToken(t), "", nil)
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("expected provider-b to be unable to read provider-a's transaction, got %d", response.StatusCode)
	}

	response = doJSON(t, http.MethodGet, "/providers/provider-a/wagering/transactions/"+requestA.ExternalTransactionID, providerBToken(t), "", nil)
	if response.StatusCode != http.StatusForbidden {
		t.Fatalf("expected provider-b to be forbidden from provider-a's namespace, got %d", response.StatusCode)
	}
}

// TestE2ERejectsUnsafeInput verifies float amounts, oversized identifiers and payloads are
// rejected before any financial effect.
func TestE2ERejectsUnsafeInput(t *testing.T) {
	token := providerAToken(t)
	wallet := createRandomWalletE2E(t, internalToken(t), "100.00")
	request := newWagerRequestE2E(wallet, "provider-a", "BET", "25.00")

	numeric := map[string]any{
		"providerId":            request.ProviderID,
		"externalTransactionId": request.ExternalTransactionID,
		"playerId":              request.PlayerID,
		"walletId":              request.WalletID,
		"roundId":               request.RoundID,
		"gameId":                request.GameID,
		"kind":                  request.Kind,
		"money":                 map[string]any{"amount": 25.00, "currency": "BRL"},
	}

	response := doJSON(t, http.MethodPost, "/wagering/transactions", token, "numeric-"+uuid.NewString(), numeric)
	if body := decodeError(t, response); response.StatusCode != http.StatusBadRequest || body.Code != "INVALID_PAYLOAD" {
		t.Fatalf("expected 400 INVALID_PAYLOAD for a numeric amount, got %d %s", response.StatusCode, body.Code)
	}

	response = doJSON(t, http.MethodPost, "/wagering/transactions", token, strings.Repeat("k", 300), request)
	if body := decodeError(t, response); response.StatusCode != http.StatusUnprocessableEntity || body.Code != "VALIDATION_FAILED" {
		t.Fatalf("expected 422 VALIDATION_FAILED for an oversized key, got %d %s", response.StatusCode, body.Code)
	}

	oversized := bytes.Repeat([]byte("a"), 32*1024)

	httpRequest, err := http.NewRequest(http.MethodPost, apiURL()+"/wagering/transactions", bytes.NewReader(oversized))
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	httpRequest.Header.Set("Authorization", "Bearer "+token)
	httpRequest.Header.Set("Content-Type", "application/json")
	httpRequest.Header.Set("Idempotency-Key", "oversized-"+uuid.NewString())

	httpResponse, err := http.DefaultClient.Do(httpRequest)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}

	_, _ = io.Copy(io.Discard, httpResponse.Body)
	_ = httpResponse.Body.Close()

	if httpResponse.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413 for an oversized payload, got %d", httpResponse.StatusCode)
	}

	assertWalletBalance(t, wallet.ID, "100.00")
}

// TestE2EPropagatesCorrelationID verifies the client correlation id is echoed back.
func TestE2EPropagatesCorrelationID(t *testing.T) {
	request, err := http.NewRequest(http.MethodGet, apiURL()+"/health/live", nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	request.Header.Set("X-Correlation-Id", "e2e-correlation-123")

	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}

	_ = response.Body.Close()

	if got := response.Header.Get("X-Correlation-Id"); got != "e2e-correlation-123" {
		t.Fatalf("expected the correlation id to be echoed, got %q", got)
	}
}
