//go:build e2e

package e2e

import (
	"net/http"
	"testing"

	"github.com/google/uuid"
)

// TestE2ERefundRestoresBet verifies an integral processed BET refund.
func TestE2ERefundRestoresBet(t *testing.T) {
	internal := internalToken(t)
	provider := providerAToken(t)

	wallet := createRandomWalletE2E(t, internal, "100.00")

	bet := newWagerRequestE2E(wallet, "provider-a", "BET", "25.00")
	bet.RoundID = "round-refund"

	submitWagerE2E(t, provider, "key-"+uuid.NewString(), bet, http.StatusOK)

	refund := newWagerRequestE2E(wallet, "provider-a", "REFUND", "25.00")
	refund.RoundID = bet.RoundID
	refund.ReferenceExternalTransactionID = bet.ExternalTransactionID

	result := submitWagerE2E(t, provider, "key-"+uuid.NewString(), refund, http.StatusOK)

	if result.Status != "PROCESSED" {
		t.Fatalf("expected PROCESSED, got %s", result.Status)
	}

	current := getWalletE2E(t, internal, wallet.ID)

	if current.Balance.Amount != "100.00" {
		t.Errorf("expected balance 100.00, got %s", current.Balance.Amount)
	}

	if current.Version != 3 {
		t.Errorf("expected version 3, got %d", current.Version)
	}

	ledger := getLedgerE2E(t, internal, wallet.ID)

	if len(ledger.Entries) != 3 {
		t.Fatalf("expected 3 ledger entries, got %d", len(ledger.Entries))
	}
}

// TestE2EDuplicateRefundIsRejected verifies the same BET cannot be refunded twice.
func TestE2EDuplicateRefundIsRejected(t *testing.T) {
	internal := internalToken(t)
	provider := providerAToken(t)

	wallet := createRandomWalletE2E(t, internal, "100.00")

	bet := newWagerRequestE2E(wallet, "provider-a", "BET", "25.00")
	bet.RoundID = "round-refund"

	submitWagerE2E(t, provider, "key-"+uuid.NewString(), bet, http.StatusOK)

	first := newWagerRequestE2E(wallet, "provider-a", "REFUND", "25.00")
	first.RoundID = bet.RoundID
	first.ReferenceExternalTransactionID = bet.ExternalTransactionID

	submitWagerE2E(t, provider, "key-"+uuid.NewString(), first, http.StatusOK)

	second := newWagerRequestE2E(wallet, "provider-a", "REFUND", "25.00")
	second.RoundID = bet.RoundID
	second.ReferenceExternalTransactionID = bet.ExternalTransactionID

	result := submitWagerE2E(t, provider, "key-"+uuid.NewString(), second, http.StatusOK)

	if result.Status != "REJECTED" {
		t.Fatalf("expected REJECTED, got %s", result.Status)
	}

	if result.FailureCode != "DUPLICATE_REVERSAL" {
		t.Errorf("expected DUPLICATE_REVERSAL, got %s", result.FailureCode)
	}

	current := getWalletE2E(t, internal, wallet.ID)

	if current.Balance.Amount != "100.00" {
		t.Errorf("expected balance 100.00, got %s", current.Balance.Amount)
	}
}

// TestE2EMissingReferenceBecomesPending verifies out-of-order reversals are durable.
func TestE2EMissingReferenceBecomesPending(t *testing.T) {
	internal := internalToken(t)
	provider := providerAToken(t)

	wallet := createRandomWalletE2E(t, internal, "100.00")

	refund := newWagerRequestE2E(wallet, "provider-a", "REFUND", "25.00")
	refund.ReferenceExternalTransactionID = "missing-" + uuid.NewString()

	result := submitWagerE2E(t, provider, "key-"+uuid.NewString(), refund, http.StatusAccepted)

	if result.Status != "PENDING_REFERENCE" {
		t.Fatalf("expected PENDING_REFERENCE, got %s", result.Status)
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
		t.Fatalf("expected only opening entry, got %d", len(ledger.Entries))
	}
}

// TestE2ERollbackReversesWin verifies ROLLBACK applies the opposite movement.
func TestE2ERollbackReversesWin(t *testing.T) {
	internal := internalToken(t)
	provider := providerAToken(t)

	wallet := createRandomWalletE2E(t, internal, "100.00")

	win := newWagerRequestE2E(wallet, "provider-a", "WIN", "20.00")
	win.RoundID = "round-win"

	submitWagerE2E(t, provider, "key-"+uuid.NewString(), win, http.StatusOK)

	rollback := newWagerRequestE2E(wallet, "provider-a", "ROLLBACK", "20.00")
	rollback.RoundID = win.RoundID
	rollback.ReferenceExternalTransactionID = win.ExternalTransactionID

	result := submitWagerE2E(t, provider, "key-"+uuid.NewString(), rollback, http.StatusOK)

	if result.Status != "PROCESSED" {
		t.Fatalf("expected PROCESSED, got %s", result.Status)
	}

	current := getWalletE2E(t, internal, wallet.ID)

	if current.Balance.Amount != "100.00" {
		t.Errorf("expected balance 100.00, got %s", current.Balance.Amount)
	}
}
