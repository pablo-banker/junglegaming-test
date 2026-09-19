//go:build e2e

package e2e

import (
	"net/http"
	"testing"

	"github.com/google/uuid"
)

// TestE2EDuplicateWalletReturnsConflict verifies player and currency uniqueness.
func TestE2EDuplicateWalletReturnsConflict(t *testing.T) {
	token := internalToken(t)
	playerID := uuid.NewString()

	createWalletE2E(t, token, playerID, "100.00")

	body := map[string]any{
		"playerId": playerID,
		"initialBalance": map[string]string{
			"amount":   "100.00",
			"currency": "BRL",
		},
	}

	response := doJSON(t, http.MethodPost, "/wallets", token, "", body)
	defer response.Body.Close()

	if response.StatusCode != http.StatusConflict {
		t.Fatalf("expected status 409, got %d", response.StatusCode)
	}
}

// TestE2EZeroOpeningCreatesNoLedger verifies zero balance openings have no financial records.
func TestE2EZeroOpeningCreatesNoLedger(t *testing.T) {
	internal := internalToken(t)

	wallet := createRandomWalletE2E(t, internal, "0.00")
	ledger := getLedgerE2E(t, internal, wallet.ID)

	if len(ledger.Entries) != 0 {
		t.Fatalf("expected no ledger entries, got %d", len(ledger.Entries))
	}

	response := doJSON(t, http.MethodPost, "/wallets/"+wallet.ID+"/reconciliation", internal, "", nil)
	if response.StatusCode != http.StatusOK {
		response.Body.Close()
		t.Fatalf("expected reconciliation status 200, got %d", response.StatusCode)
	}

	result := decodeSuccess[reconciliationResponse](t, response)

	if !result.Consistent {
		t.Fatal("expected reconciliation to be consistent")
	}

	if result.CheckedEntries != 0 {
		t.Errorf("expected 0 checked entries, got %d", result.CheckedEntries)
	}
}

// TestE2EInsufficientFundsDoesNotChangeWallet verifies rejected BETs have no financial effect.
func TestE2EInsufficientFundsDoesNotChangeWallet(t *testing.T) {
	internal := internalToken(t)
	provider := providerAToken(t)

	wallet := createRandomWalletE2E(t, internal, "20.00")
	request := newWagerRequestE2E(wallet, "provider-a", "BET", "80.00")

	result := submitWagerE2E(t, provider, "key-"+uuid.NewString(), request, http.StatusOK)

	if result.Status != "REJECTED" {
		t.Fatalf("expected REJECTED, got %s", result.Status)
	}

	if result.FailureCode != "BET_INSUFFICIENT_FUNDS" {
		t.Errorf("expected BET_INSUFFICIENT_FUNDS, got %s", result.FailureCode)
	}

	current := getWalletE2E(t, internal, wallet.ID)

	if current.Balance.Amount != "20.00" {
		t.Errorf("expected balance 20.00, got %s", current.Balance.Amount)
	}

	if current.Version != 1 {
		t.Errorf("expected version 1, got %d", current.Version)
	}

	ledger := getLedgerE2E(t, internal, wallet.ID)

	if len(ledger.Entries) != 1 {
		t.Fatalf("expected only opening ledger entry, got %d", len(ledger.Entries))
	}
}

// TestE2ELossDoesNotMoveBalance verifies LOSS produces no ledger movement.
func TestE2ELossDoesNotMoveBalance(t *testing.T) {
	internal := internalToken(t)
	provider := providerAToken(t)

	wallet := createRandomWalletE2E(t, internal, "100.00")
	request := newWagerRequestE2E(wallet, "provider-a", "LOSS", "0.00")

	result := submitWagerE2E(t, provider, "key-"+uuid.NewString(), request, http.StatusOK)

	if result.Status != "PROCESSED" {
		t.Fatalf("expected PROCESSED, got %s", result.Status)
	}

	current := getWalletE2E(t, internal, wallet.ID)

	if current.Balance.Amount != "100.00" {
		t.Errorf("expected balance 100.00, got %s", current.Balance.Amount)
	}

	if current.Version != 1 {
		t.Errorf("expected version 1, got %d", current.Version)
	}

	ledger := getLedgerE2E(t, internal, wallet.ID)

	if len(ledger.Entries) != 1 {
		t.Fatalf("expected only opening ledger entry, got %d", len(ledger.Entries))
	}
}

// TestE2EIdempotencyKeyConflict verifies a key cannot represent different payloads.
func TestE2EIdempotencyKeyConflict(t *testing.T) {
	internal := internalToken(t)
	provider := providerAToken(t)

	wallet := createRandomWalletE2E(t, internal, "100.00")
	request := newWagerRequestE2E(wallet, "provider-a", "BET", "10.00")
	key := "key-" + uuid.NewString()

	submitWagerE2E(t, provider, key, request, http.StatusOK)

	changed := request
	changed.Money.Amount = "20.00"

	response := doJSON(t, http.MethodPost, "/wagering/transactions", provider, key, changed)
	defer response.Body.Close()

	if response.StatusCode != http.StatusConflict {
		t.Fatalf("expected status 409, got %d", response.StatusCode)
	}

	current := getWalletE2E(t, internal, wallet.ID)

	if current.Balance.Amount != "90.00" {
		t.Errorf("expected balance 90.00, got %s", current.Balance.Amount)
	}
}

// TestE2EExternalTransactionConflict verifies another key cannot reapply the same transaction.
func TestE2EExternalTransactionConflict(t *testing.T) {
	internal := internalToken(t)
	provider := providerAToken(t)

	wallet := createRandomWalletE2E(t, internal, "100.00")
	request := newWagerRequestE2E(wallet, "provider-a", "BET", "10.00")

	submitWagerE2E(t, provider, "key-"+uuid.NewString(), request, http.StatusOK)

	response := doJSON(t, http.MethodPost, "/wagering/transactions", provider, "another-key-"+uuid.NewString(), request)
	defer response.Body.Close()

	if response.StatusCode != http.StatusConflict {
		t.Fatalf("expected status 409, got %d", response.StatusCode)
	}

	current := getWalletE2E(t, internal, wallet.ID)

	if current.Balance.Amount != "90.00" {
		t.Errorf("expected balance 90.00, got %s", current.Balance.Amount)
	}
}

// TestE2EReplayReturnsOriginalObservedBalance verifies replay does not use the current wallet balance.
func TestE2EReplayReturnsOriginalObservedBalance(t *testing.T) {
	internal := internalToken(t)
	provider := providerAToken(t)

	wallet := createRandomWalletE2E(t, internal, "100.00")

	bet := newWagerRequestE2E(wallet, "provider-a", "BET", "25.00")
	betKey := "key-" + uuid.NewString()

	first := submitWagerE2E(t, provider, betKey, bet, http.StatusOK)

	if first.Balance == nil || first.Balance.Amount != "75.00" {
		t.Fatal("expected first balance 75.00")
	}

	win := newWagerRequestE2E(wallet, "provider-a", "WIN", "10.00")
	submitWagerE2E(t, provider, "key-"+uuid.NewString(), win, http.StatusOK)

	current := getWalletE2E(t, internal, wallet.ID)

	if current.Balance.Amount != "85.00" {
		t.Errorf("expected current balance 85.00, got %s", current.Balance.Amount)
	}

	replay := submitWagerE2E(t, provider, betKey, bet, http.StatusOK)

	if !replay.IdempotentReplay {
		t.Fatal("expected idempotent replay")
	}

	if replay.Balance == nil || replay.Balance.Amount != "75.00" {
		t.Fatal("expected replay to preserve original balance 75.00")
	}
}
