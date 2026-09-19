//go:build e2e

package e2e

import (
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"
)

// tamperJWT corrupts the signature of a JWT.
func tamperJWT(token string) string {
	parts := strings.Split(token, ".")
	if len(parts) != 3 || len(parts[2]) == 0 {
		return token + "invalid"
	}

	if parts[2][0] == 'a' {
		parts[2] = "b" + parts[2][1:]
	} else {
		parts[2] = "a" + parts[2][1:]
	}

	return strings.Join(parts, ".")
}

// TestE2ERejectsTamperedJWT verifies modified JWT signatures are rejected.
func TestE2ERejectsTamperedJWT(t *testing.T) {
	token := tamperJWT(providerAToken(t))

	response := doJSON(t, http.MethodGet, "/wagering/transactions/"+uuid.NewString(), token, "", nil)
	defer response.Body.Close()

	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected status 401, got %d", response.StatusCode)
	}
}

// TestE2EProviderCannotCreateWallet verifies wallet operations are internal only.
func TestE2EProviderCannotCreateWallet(t *testing.T) {
	token := providerAToken(t)

	body := map[string]any{
		"playerId": uuid.NewString(),
		"initialBalance": map[string]string{
			"amount":   "100.00",
			"currency": "BRL",
		},
	}

	response := doJSON(t, http.MethodPost, "/wallets", token, "", body)
	defer response.Body.Close()

	if response.StatusCode != http.StatusForbidden {
		t.Fatalf("expected status 403, got %d", response.StatusCode)
	}
}

// TestE2EInternalServiceCannotSubmitWager verifies provider routes reject internal identities.
func TestE2EInternalServiceCannotSubmitWager(t *testing.T) {
	internal := internalToken(t)
	wallet := createRandomWalletE2E(t, internal, "100.00")

	request := newWagerRequestE2E(wallet, "provider-a", "BET", "10.00")

	response := doJSON(t, http.MethodPost, "/wagering/transactions", internal, "key-"+uuid.NewString(), request)
	defer response.Body.Close()

	if response.StatusCode != http.StatusForbidden {
		t.Fatalf("expected status 403, got %d", response.StatusCode)
	}
}

// TestE2EProviderCannotSpoofProviderID verifies the JWT identity overrides body claims.
func TestE2EProviderCannotSpoofProviderID(t *testing.T) {
	internal := internalToken(t)
	token := providerAToken(t)
	wallet := createRandomWalletE2E(t, internal, "100.00")

	request := newWagerRequestE2E(wallet, "provider-b", "BET", "10.00")

	response := doJSON(t, http.MethodPost, "/wagering/transactions", token, "key-"+uuid.NewString(), request)
	defer response.Body.Close()

	if response.StatusCode != http.StatusForbidden {
		t.Fatalf("expected status 403, got %d", response.StatusCode)
	}

	current := getWalletE2E(t, internal, wallet.ID)

	if current.Balance.Amount != "100.00" {
		t.Errorf("expected balance 100.00, got %s", current.Balance.Amount)
	}

	if current.Version != 1 {
		t.Errorf("expected version 1, got %d", current.Version)
	}
}

// TestE2EProviderCannotReadAnotherProviderTransaction verifies transaction isolation.
func TestE2EProviderCannotReadAnotherProviderTransaction(t *testing.T) {
	internal := internalToken(t)
	providerA := providerAToken(t)
	providerB := providerBToken(t)

	wallet := createRandomWalletE2E(t, internal, "100.00")
	request := newWagerRequestE2E(wallet, "provider-a", "BET", "10.00")

	result := submitWagerE2E(t, providerA, "key-"+uuid.NewString(), request, http.StatusOK)

	response := doJSON(t, http.MethodGet, "/wagering/transactions/"+result.TransactionID, providerB, "", nil)
	defer response.Body.Close()

	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d", response.StatusCode)
	}
}

// TestE2ESQLInjectionPayloadIsTreatedAsData verifies external identifiers cannot alter SQL.
func TestE2ESQLInjectionPayloadIsTreatedAsData(t *testing.T) {
	internal := internalToken(t)
	provider := providerAToken(t)

	wallet := createRandomWalletE2E(t, internal, "100.00")

	request := newWagerRequestE2E(wallet, "provider-a", "BET", "1.00")
	request.ExternalTransactionID = `'; DROP TABLE wallets; -- ` + uuid.NewString()
	request.RoundID = `'; DELETE FROM wallet_ledger_entries; -- ` + uuid.NewString()
	request.GameID = `'; DROP TABLE wager_transactions; -- ` + uuid.NewString()

	result := submitWagerE2E(t, provider, "sql-injection-"+uuid.NewString(), request, http.StatusOK)

	if result.Status != "PROCESSED" {
		t.Fatalf("expected PROCESSED, got %s", result.Status)
	}

	current := getWalletE2E(t, internal, wallet.ID)

	if current.Balance.Amount != "99.00" {
		t.Errorf("expected balance 99.00, got %s", current.Balance.Amount)
	}

	ledger := getLedgerE2E(t, internal, wallet.ID)

	if len(ledger.Entries) != 2 {
		t.Fatalf("expected database to remain intact, got %d entries", len(ledger.Entries))
	}
}
