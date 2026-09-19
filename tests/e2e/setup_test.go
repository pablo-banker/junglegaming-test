//go:build e2e

package e2e

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"testing"
	"time"
)

const (
	defaultAPIURL      = "http://localhost:8080"
	defaultKeycloakURL = "http://localhost:8081"
	defaultRealm       = "junglegaming"
)

type errorResponse struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details string `json:"details,omitempty"`
}

type moneyResponse struct {
	Amount   string `json:"amount"`
	Currency string `json:"currency"`
}

type walletResponse struct {
	ID       string        `json:"id"`
	PlayerID string        `json:"playerId"`
	Balance  moneyResponse `json:"balance"`
	Version  int64         `json:"version"`
}

type wagerProcessResponse struct {
	TransactionID    string         `json:"transactionId"`
	Status           string         `json:"status"`
	Balance          *moneyResponse `json:"balance,omitempty"`
	FailureCode      string         `json:"failureCode,omitempty"`
	IdempotentReplay bool           `json:"idempotentReplay"`
}

type wagerResponse struct {
	TransactionID         string         `json:"transactionId"`
	ProviderID            string         `json:"providerId"`
	ExternalTransactionID string         `json:"externalTransactionId"`
	Status                string         `json:"status"`
	BalanceBefore         *moneyResponse `json:"balanceBefore,omitempty"`
	BalanceAfter          *moneyResponse `json:"balanceAfter,omitempty"`
}

type ledgerResponse struct {
	Entries    []ledgerEntryResponse `json:"entries"`
	NextCursor string                `json:"nextCursor,omitempty"`
}

type ledgerEntryResponse struct {
	ID            string        `json:"id"`
	TransactionID string        `json:"transactionId"`
	Direction     string        `json:"direction"`
	Money         moneyResponse `json:"money"`
	CreatedAt     time.Time     `json:"createdAt"`
}

type reconciliationResponse struct {
	WalletID          string        `json:"walletId"`
	StoredBalance     moneyResponse `json:"storedBalance"`
	CalculatedBalance moneyResponse `json:"calculatedBalance"`
	Difference        moneyResponse `json:"difference"`
	Consistent        bool          `json:"consistent"`
	CheckedEntries    int64         `json:"checkedEntries"`
}

// apiURL returns the configured application URL.
func apiURL() string {
	if value := os.Getenv("E2E_API_URL"); value != "" {
		return value
	}

	return defaultAPIURL
}

// keycloakURL returns the configured Keycloak URL.
func keycloakURL() string {
	if value := os.Getenv("E2E_KEYCLOAK_URL"); value != "" {
		return value
	}

	return defaultKeycloakURL
}

// getAccessToken obtains a real client_credentials token from Keycloak.
func getAccessToken(t *testing.T, clientID string, clientSecret string) string {
	t.Helper()

	realm := defaultRealm
	if value := os.Getenv("KEYCLOAK_REALM"); value != "" {
		realm = value
	}

	form := url.Values{}
	form.Set("grant_type", "client_credentials")
	form.Set("client_id", clientID)
	form.Set("client_secret", clientSecret)

	tokenURL := fmt.Sprintf("%s/realms/%s/protocol/openid-connect/token", keycloakURL(), realm)

	request, err := http.NewRequest(http.MethodPost, tokenURL, bytes.NewBufferString(form.Encode()))
	if err != nil {
		t.Fatalf("failed to create token request: %v", err)
	}

	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("failed to request token: %v", err)
	}

	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("expected token status 200, got %d: %s", response.StatusCode, body)
	}

	var result struct {
		AccessToken string `json:"access_token"`
	}

	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode token response: %v", err)
	}

	if result.AccessToken == "" {
		t.Fatal("expected access token")
	}

	return result.AccessToken
}

// doJSON performs an authenticated JSON HTTP request.
func doJSON(t *testing.T, method string, path string, token string, idempotencyKey string, body any) *http.Response {
	t.Helper()

	var reader io.Reader

	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("failed to encode request: %v", err)
		}

		reader = bytes.NewReader(data)
	}

	request, err := http.NewRequest(method, apiURL()+path, reader)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}

	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}

	if idempotencyKey != "" {
		request.Header.Set("Idempotency-Key", idempotencyKey)
	}

	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}

	return response
}

// decodeSuccess decodes a successful response body.
func decodeSuccess[T any](t *testing.T, response *http.Response) T {
	t.Helper()

	defer response.Body.Close()

	var body T

	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	return body
}
