//go:build e2e

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

type concurrentResult struct {
	status int
	body   []byte
	err    error
}

// executeConcurrentWager sends a wager without calling testing methods from worker goroutines.
func executeConcurrentWager(token string, key string, request wagerRequest) concurrentResult {
	data, err := json.Marshal(request)
	if err != nil {
		return concurrentResult{err: err}
	}

	httpRequest, err := http.NewRequest(http.MethodPost, apiURL()+"/wagering/transactions", bytes.NewReader(data))
	if err != nil {
		return concurrentResult{err: err}
	}

	httpRequest.Header.Set("Authorization", "Bearer "+token)
	httpRequest.Header.Set("Idempotency-Key", key)
	httpRequest.Header.Set("Content-Type", "application/json")

	response, err := http.DefaultClient.Do(httpRequest)
	if err != nil {
		return concurrentResult{err: err}
	}

	defer response.Body.Close()

	body, err := io.ReadAll(response.Body)

	return concurrentResult{
		status: response.StatusCode,
		body:   body,
		err:    err,
	}
}

// TestE2ETwoConcurrentBetsCannotOverspend verifies per-wallet database coordination.
func TestE2ETwoConcurrentBetsCannotOverspend(t *testing.T) {
	internal := internalToken(t)
	provider := providerAToken(t)

	wallet := createRandomWalletE2E(t, internal, "100.00")

	first := newWagerRequestE2E(wallet, "provider-a", "BET", "80.00")
	second := newWagerRequestE2E(wallet, "provider-a", "BET", "80.00")

	results := make([]concurrentResult, 2)

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		results[0] = executeConcurrentWager(provider, "key-"+uuid.NewString(), first)
	}()

	go func() {
		defer wg.Done()
		results[1] = executeConcurrentWager(provider, "key-"+uuid.NewString(), second)
	}()

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

		var envelope successResponse[wagerProcessResponse]

		if err := json.Unmarshal(result.body, &envelope); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		switch envelope.Data.Status {
		case "PROCESSED":
			processed++

		case "REJECTED":
			rejected++

		default:
			t.Fatalf("unexpected wager status %s", envelope.Data.Status)
		}
	}

	if processed != 1 {
		t.Fatalf("expected 1 processed wager, got %d", processed)
	}

	if rejected != 1 {
		t.Fatalf("expected 1 rejected wager, got %d", rejected)
	}

	current := getWalletE2E(t, internal, wallet.ID)

	if current.Balance.Amount != "20.00" {
		t.Errorf("expected final balance 20.00, got %s", current.Balance.Amount)
	}

	if current.Version != 2 {
		t.Errorf("expected wallet version 2, got %d", current.Version)
	}

	ledger := getLedgerE2E(t, internal, wallet.ID)

	if len(ledger.Entries) != 2 {
		t.Fatalf("expected opening plus one debit, got %d entries", len(ledger.Entries))
	}
}

// TestE2ESameBetFiftyTimesMovesMoneyOnce verifies massive concurrent idempotency.
func TestE2ESameBetFiftyTimesMovesMoneyOnce(t *testing.T) {
	internal := internalToken(t)
	provider := providerAToken(t)

	wallet := createRandomWalletE2E(t, internal, "100.00")
	request := newWagerRequestE2E(wallet, "provider-a", "BET", "25.00")
	key := "key-" + uuid.NewString()

	const requests = 50

	results := make([]concurrentResult, requests)

	var wg sync.WaitGroup
	wg.Add(requests)

	for i := 0; i < requests; i++ {
		go func(index int) {
			defer wg.Done()
			results[index] = executeConcurrentWager(provider, key, request)
		}(i)
	}

	wg.Wait()

	firstProcessing := 0
	replays := 0
	transactionIDs := map[string]bool{}

	for _, result := range results {
		if result.err != nil {
			t.Fatalf("concurrent request failed: %v", result.err)
		}

		if result.status != http.StatusOK {
			t.Fatalf("expected status 200, got %d: %s", result.status, result.body)
		}

		var envelope successResponse[wagerProcessResponse]

		if err := json.Unmarshal(result.body, &envelope); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		transactionIDs[envelope.Data.TransactionID] = true

		if envelope.Data.IdempotentReplay {
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

	current := getWalletE2E(t, internal, wallet.ID)

	if current.Balance.Amount != "75.00" {
		t.Errorf("expected balance 75.00, got %s", current.Balance.Amount)
	}

	if current.Version != 2 {
		t.Errorf("expected version 2, got %d", current.Version)
	}

	ledger := getLedgerE2E(t, internal, wallet.ID)

	if len(ledger.Entries) != 2 {
		t.Fatalf("expected opening plus one debit, got %d entries", len(ledger.Entries))
	}
}
