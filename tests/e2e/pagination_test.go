//go:build e2e

package e2e

import (
	"net/http"
	"testing"

	"github.com/google/uuid"
)

// TestE2ELedgerCursorDoesNotDuplicateEntries verifies stable cursor pagination.
func TestE2ELedgerCursorDoesNotDuplicateEntries(t *testing.T) {
	internal := internalToken(t)
	provider := providerAToken(t)

	wallet := createRandomWalletE2E(t, internal, "100.00")

	bet := newWagerRequestE2E(wallet, "provider-a", "BET", "10.00")
	submitWagerE2E(t, provider, "key-"+uuid.NewString(), bet, http.StatusOK)

	win := newWagerRequestE2E(wallet, "provider-a", "WIN", "5.00")
	submitWagerE2E(t, provider, "key-"+uuid.NewString(), win, http.StatusOK)

	response := doJSON(t, http.MethodGet, "/wallets/"+wallet.ID+"/ledger?limit=2", internal, "", nil)
	if response.StatusCode != http.StatusOK {
		response.Body.Close()
		t.Fatalf("expected status 200, got %d", response.StatusCode)
	}

	firstPage := decodeSuccess[ledgerResponse](t, response)

	if len(firstPage.Entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(firstPage.Entries))
	}

	if firstPage.NextCursor == "" {
		t.Fatal("expected next cursor")
	}

	response = doJSON(t, http.MethodGet, "/wallets/"+wallet.ID+"/ledger?limit=2&cursor="+firstPage.NextCursor, internal, "", nil)
	if response.StatusCode != http.StatusOK {
		response.Body.Close()
		t.Fatalf("expected status 200, got %d", response.StatusCode)
	}

	secondPage := decodeSuccess[ledgerResponse](t, response)

	if len(secondPage.Entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(secondPage.Entries))
	}

	firstIDs := map[string]bool{}

	for _, entry := range firstPage.Entries {
		firstIDs[entry.ID] = true
	}

	for _, entry := range secondPage.Entries {
		if firstIDs[entry.ID] {
			t.Fatalf("duplicated ledger entry across pages: %s", entry.ID)
		}
	}
}

// TestE2ELedgerRejectsInvalidPagination verifies abusive pagination parameters.
func TestE2ELedgerRejectsInvalidPagination(t *testing.T) {
	internal := internalToken(t)
	wallet := createRandomWalletE2E(t, internal, "100.00")

	paths := []string{
		"/wallets/" + wallet.ID + "/ledger?limit=0",
		"/wallets/" + wallet.ID + "/ledger?limit=101",
		"/wallets/" + wallet.ID + "/ledger?limit=abc",
		"/wallets/" + wallet.ID + "/ledger?cursor=definitely-not-valid",
	}

	for _, path := range paths {
		response := doJSON(t, http.MethodGet, path, internal, "", nil)

		if response.StatusCode != http.StatusBadRequest {
			response.Body.Close()
			t.Fatalf("expected status 400 for %s, got %d", path, response.StatusCode)
		}

		response.Body.Close()
	}
}
