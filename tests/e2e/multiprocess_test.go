//go:build e2e && multiprocess

package e2e

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"sync"
	"testing"

	"github.com/google/uuid"
)

type multiProcessResult struct {
	status int
	body   []byte
	err    error
}

// multiProcessAPIURLs returns the independent API instance URLs.
func multiProcessAPIURLs() []string {
	return []string{
		envOrDefault("E2E_API_URL_1", "http://localhost:8080"),
		envOrDefault("E2E_API_URL_2", "http://localhost:8082"),
		envOrDefault("E2E_API_URL_3", "http://localhost:8083"),
	}
}

// requireMultiProcessInstances verifies all API instances are independently reachable.
func requireMultiProcessInstances(t *testing.T, urls []string) {
	t.Helper()

	seen := make(map[string]bool)

	for _, baseURL := range urls {
		if seen[baseURL] {
			t.Fatalf("duplicate API URL configured: %s", baseURL)
		}

		seen[baseURL] = true

		response, err := http.Get(baseURL + "/health/live")
		if err != nil {
			t.Fatalf("API instance %s is not reachable: %v", baseURL, err)
		}

		response.Body.Close()

		if response.StatusCode != http.StatusOK {
			t.Fatalf("expected %s health status 200, got %d", baseURL, response.StatusCode)
		}
	}
}

// doJSONAt performs a JSON request against a specific API instance.
func doJSONAt(t *testing.T, baseURL string, method string, path string, token string, idempotencyKey string, body any) *http.Response {
	t.Helper()

	var reader io.Reader

	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("failed to encode request: %v", err)
		}

		reader = bytes.NewReader(data)
	}

	request, err := http.NewRequest(method, baseURL+path, reader)
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
		t.Fatalf("request failed against %s: %v", baseURL, err)
	}

	return response
}

// executeConcurrentWagerAt submits a wager to a specific API instance.
func executeConcurrentWagerAt(baseURL string, token string, key string, request wagerRequest) multiProcessResult {
	data, err := json.Marshal(request)
	if err != nil {
		return multiProcessResult{err: err}
	}

	httpRequest, err := http.NewRequest(http.MethodPost, baseURL+"/wagering/transactions", bytes.NewReader(data))
	if err != nil {
		return multiProcessResult{err: err}
	}

	httpRequest.Header.Set("Authorization", "Bearer "+token)
	httpRequest.Header.Set("Idempotency-Key", key)
	httpRequest.Header.Set("Content-Type", "application/json")

	response, err := http.DefaultClient.Do(httpRequest)
	if err != nil {
		return multiProcessResult{err: err}
	}

	defer response.Body.Close()

	body, err := io.ReadAll(response.Body)

	return multiProcessResult{
		status: response.StatusCode,
		body:   body,
		err:    err,
	}
}

// createWalletAt creates a wallet through a specific API instance.
func createWalletAt(t *testing.T, baseURL string, token string, amount string) walletResponse {
	t.Helper()

	body := map[string]any{
		"playerId": uuid.NewString(),
		"initialBalance": map[string]string{
			"amount":   amount,
			"currency": "BRL",
		},
	}

	response := doJSONAt(t, baseURL, http.MethodPost, "/wallets", token, "", body)
	if response.StatusCode != http.StatusCreated {
		response.Body.Close()
		t.Fatalf("expected wallet creation status 201, got %d", response.StatusCode)
	}

	return decodeSuccess[walletResponse](t, response)
}

// getWalletAt retrieves a wallet through a specific API instance.
func getWalletAt(t *testing.T, baseURL string, token string, walletID string) walletResponse {
	t.Helper()

	response := doJSONAt(t, baseURL, http.MethodGet, "/wallets/"+walletID, token, "", nil)
	if response.StatusCode != http.StatusOK {
		response.Body.Close()
		t.Fatalf("expected wallet status 200, got %d", response.StatusCode)
	}

	return decodeSuccess[walletResponse](t, response)
}

// getLedgerAt retrieves a wallet ledger through a specific API instance.
func getLedgerAt(t *testing.T, baseURL string, token string, walletID string) ledgerResponse {
	t.Helper()

	response := doJSONAt(t, baseURL, http.MethodGet, "/wallets/"+walletID+"/ledger?limit=100", token, "", nil)
	if response.StatusCode != http.StatusOK {
		response.Body.Close()
		t.Fatalf("expected ledger status 200, got %d", response.StatusCode)
	}

	return decodeSuccess[ledgerResponse](t, response)
}

// TestE2EMultiProcessPreventsOverspending verifies wallet locking across independent processes.
func TestE2EMultiProcessPreventsOverspending(t *testing.T) {
	urls := multiProcessAPIURLs()
	requireMultiProcessInstances(t, urls)

	internal := internalToken(t)
	provider := providerAToken(t)

	wallet := createWalletAt(t, urls[0], internal, "100.00")

	first := newWagerRequestE2E(wallet, "provider-a", "BET", "80.00")
	second := newWagerRequestE2E(wallet, "provider-a", "BET", "80.00")

	results := make([]multiProcessResult, 2)
	start := make(chan struct{})

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		<-start

		results[0] = executeConcurrentWagerAt(urls[0], provider, "key-"+uuid.NewString(), first)
	}()

	go func() {
		defer wg.Done()
		<-start

		results[1] = executeConcurrentWagerAt(urls[1], provider, "key-"+uuid.NewString(), second)
	}()

	close(start)
	wg.Wait()

	processed := 0
	rejected := 0

	for _, result := range results {
		if result.err != nil {
			t.Fatalf("concurrent request failed: %v", result.err)
		}

		if result.status != http.StatusOK {
			t.Fatalf("expected status 200, got %d: %s", result.status, result.body)
		}

		var envelope wagerProcessResponse

		if err := json.Unmarshal(result.body, &envelope); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		switch envelope.Status {
		case "PROCESSED":
			processed++

		case "REJECTED":
			rejected++

		default:
			t.Fatalf("unexpected status %s", envelope.Status)
		}
	}

	if processed != 1 {
		t.Fatalf("expected exactly 1 processed wager, got %d", processed)
	}

	if rejected != 1 {
		t.Fatalf("expected exactly 1 rejected wager, got %d", rejected)
	}

	current := getWalletAt(t, urls[2], internal, wallet.ID)

	if current.Balance.Amount != "20.00" {
		t.Errorf("expected balance 20.00, got %s", current.Balance.Amount)
	}

	if current.Version != 2 {
		t.Errorf("expected version 2, got %d", current.Version)
	}

	ledger := getLedgerAt(t, urls[2], internal, wallet.ID)

	if len(ledger.Entries) != 2 {
		t.Fatalf("expected opening plus one debit, got %d entries", len(ledger.Entries))
	}

	response := doJSONAt(t, urls[2], http.MethodPost, "/wallets/"+wallet.ID+"/reconciliation", internal, "", nil)
	if response.StatusCode != http.StatusOK {
		response.Body.Close()
		t.Fatalf("expected reconciliation status 200, got %d", response.StatusCode)
	}

	reconciliation := decodeSuccess[reconciliationResponse](t, response)

	if !reconciliation.Consistent {
		t.Fatal("expected reconciliation to remain consistent")
	}

	if reconciliation.StoredBalance.Amount != "20.00" {
		t.Errorf("expected stored balance 20.00, got %s", reconciliation.StoredBalance.Amount)
	}

	if reconciliation.CalculatedBalance.Amount != "20.00" {
		t.Errorf("expected calculated balance 20.00, got %s", reconciliation.CalculatedBalance.Amount)
	}

	if reconciliation.Difference.Amount != "0.00" {
		t.Errorf("expected difference 0.00, got %s", reconciliation.Difference.Amount)
	}
}

// TestE2EMultiProcessFiftyReplaysMoveMoneyOnce verifies idempotency across independent processes.
func TestE2EMultiProcessFiftyReplaysMoveMoneyOnce(t *testing.T) {
	urls := multiProcessAPIURLs()
	requireMultiProcessInstances(t, urls)

	internal := internalToken(t)
	provider := providerAToken(t)

	wallet := createWalletAt(t, urls[0], internal, "100.00")
	request := newWagerRequestE2E(wallet, "provider-a", "BET", "25.00")
	key := "key-" + uuid.NewString()

	const requests = 50

	results := make([]multiProcessResult, requests)
	start := make(chan struct{})

	var wg sync.WaitGroup
	wg.Add(requests)

	for i := 0; i < requests; i++ {
		go func(index int) {
			defer wg.Done()
			<-start

			baseURL := urls[index%len(urls)]
			results[index] = executeConcurrentWagerAt(baseURL, provider, key, request)
		}(i)
	}

	close(start)
	wg.Wait()

	firstProcessing := 0
	replays := 0
	transactionIDs := make(map[string]bool)

	for _, result := range results {
		if result.err != nil {
			t.Fatalf("concurrent request failed: %v", result.err)
		}

		if result.status != http.StatusOK {
			t.Fatalf("expected status 200, got %d: %s", result.status, result.body)
		}

		var envelope wagerProcessResponse

		if err := json.Unmarshal(result.body, &envelope); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		transactionIDs[envelope.TransactionID] = true

		if envelope.IdempotentReplay {
			replays++
		} else {
			firstProcessing++
		}
	}

	if firstProcessing != 1 {
		t.Fatalf("expected exactly 1 original processing, got %d", firstProcessing)
	}

	if replays != requests-1 {
		t.Fatalf("expected %d replays, got %d", requests-1, replays)
	}

	if len(transactionIDs) != 1 {
		t.Fatalf("expected exactly 1 transaction id, got %d", len(transactionIDs))
	}

	current := getWalletAt(t, urls[1], internal, wallet.ID)

	if current.Balance.Amount != "75.00" {
		t.Errorf("expected balance 75.00, got %s", current.Balance.Amount)
	}

	if current.Version != 2 {
		t.Errorf("expected version 2, got %d", current.Version)
	}

	ledger := getLedgerAt(t, urls[2], internal, wallet.ID)

	if len(ledger.Entries) != 2 {
		t.Fatalf("expected opening plus one debit, got %d entries", len(ledger.Entries))
	}

	response := doJSONAt(t, urls[2], http.MethodPost, "/wallets/"+wallet.ID+"/reconciliation", internal, "", nil)
	if response.StatusCode != http.StatusOK {
		response.Body.Close()
		t.Fatalf("expected reconciliation status 200, got %d", response.StatusCode)
	}

	reconciliation := decodeSuccess[reconciliationResponse](t, response)

	if !reconciliation.Consistent {
		t.Fatal("expected reconciliation to remain consistent")
	}

	if reconciliation.StoredBalance.Amount != "75.00" {
		t.Errorf("expected stored balance 75.00, got %s", reconciliation.StoredBalance.Amount)
	}

	if reconciliation.CalculatedBalance.Amount != "75.00" {
		t.Errorf("expected calculated balance 75.00, got %s", reconciliation.CalculatedBalance.Amount)
	}
}
