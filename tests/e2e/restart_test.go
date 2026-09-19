//go:build e2e && restart

package e2e

import (
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
)

// buildServerBinary builds an isolated server binary for restart tests.
func buildServerBinary(t *testing.T) string {
	t.Helper()

	workingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}

	root := filepath.Clean(filepath.Join(workingDirectory, "..", ".."))
	binary := filepath.Join(t.TempDir(), "junglegaming-server")

	command := exec.Command("go", "build", "-o", binary, "./cmd/server")
	command.Dir = root

	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("failed to build server: %v\n%s", err, output)
	}

	return binary
}

// startRestartServer starts an independent application process.
func startRestartServer(t *testing.T, binary string, port string) *exec.Cmd {
	t.Helper()

	command := exec.Command(binary)
	command.Env = append(os.Environ(), "HTTP_PORT="+port)
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr

	if err := command.Start(); err != nil {
		t.Fatalf("failed to start server: %v", err)
	}

	waitForRestartServer(t, "http://localhost:"+port)

	return command
}

// waitForRestartServer waits until the application becomes healthy.
func waitForRestartServer(t *testing.T, baseURL string) {
	t.Helper()

	deadline := time.Now().Add(10 * time.Second)

	for time.Now().Before(deadline) {
		response, err := http.Get(baseURL + "/health/live")
		if err == nil {
			response.Body.Close()

			if response.StatusCode == http.StatusOK {
				return
			}
		}

		time.Sleep(100 * time.Millisecond)
	}

	t.Fatalf("server %s did not become healthy", baseURL)
}

// stopRestartServer gracefully stops an application process.
func stopRestartServer(t *testing.T, command *exec.Cmd) {
	t.Helper()

	if command == nil || command.Process == nil {
		return
	}

	if err := command.Process.Signal(os.Interrupt); err != nil {
		t.Fatalf("failed to stop server: %v", err)
	}

	done := make(chan error, 1)

	go func() {
		done <- command.Wait()
	}()

	select {
	case <-done:
		return

	case <-time.After(5 * time.Second):
		if err := command.Process.Kill(); err != nil {
			t.Fatalf("failed to kill server: %v", err)
		}

		<-done
	}
}

// TestE2ERestartPreservesIdempotency verifies financial idempotency survives application restarts.
func TestE2ERestartPreservesIdempotency(t *testing.T) {
	port := envOrDefault("E2E_RESTART_PORT", "8090")
	baseURL := "http://localhost:" + port

	t.Setenv("E2E_API_URL", baseURL)

	binary := buildServerBinary(t)

	internal := internalToken(t)
	provider := providerAToken(t)

	firstServer := startRestartServer(t, binary, port)

	wallet := createRandomWalletE2E(t, internal, "100.00")

	request := newWagerRequestE2E(wallet, "provider-a", "BET", "25.00")
	key := "restart-" + uuid.NewString()

	first := submitWagerE2E(t, provider, key, request, http.StatusOK)

	if first.Status != "PROCESSED" {
		t.Fatalf("expected PROCESSED, got %s", first.Status)
	}

	if first.IdempotentReplay {
		t.Fatal("expected first request not to be a replay")
	}

	if first.Balance == nil || first.Balance.Amount != "75.00" {
		t.Fatal("expected first balance 75.00")
	}

	transactionID := first.TransactionID

	beforeRestart := getWalletE2E(t, internal, wallet.ID)

	if beforeRestart.Balance.Amount != "75.00" {
		t.Errorf("expected balance 75.00 before restart, got %s", beforeRestart.Balance.Amount)
	}

	if beforeRestart.Version != 2 {
		t.Errorf("expected version 2 before restart, got %d", beforeRestart.Version)
	}

	stopRestartServer(t, firstServer)

	secondServer := startRestartServer(t, binary, port)
	defer stopRestartServer(t, secondServer)

	replay := submitWagerE2E(t, provider, key, request, http.StatusOK)

	if !replay.IdempotentReplay {
		t.Fatal("expected request after restart to be an idempotent replay")
	}

	if replay.TransactionID != transactionID {
		t.Errorf("expected transaction %s, got %s", transactionID, replay.TransactionID)
	}

	if replay.Balance == nil || replay.Balance.Amount != "75.00" {
		t.Fatal("expected replay balance 75.00")
	}

	afterRestart := getWalletE2E(t, internal, wallet.ID)

	if afterRestart.Balance.Amount != "75.00" {
		t.Errorf("expected balance 75.00 after restart, got %s", afterRestart.Balance.Amount)
	}

	if afterRestart.Version != 2 {
		t.Errorf("expected version 2 after restart, got %d", afterRestart.Version)
	}

	ledger := getLedgerE2E(t, internal, wallet.ID)

	if len(ledger.Entries) != 2 {
		t.Fatalf("expected opening plus one debit, got %d entries", len(ledger.Entries))
	}

	response := doJSON(t, http.MethodPost, "/wallets/"+wallet.ID+"/reconciliation", internal, "", nil)
	if response.StatusCode != http.StatusOK {
		response.Body.Close()
		t.Fatalf("expected reconciliation status 200, got %d", response.StatusCode)
	}

	reconciliation := decodeSuccess[reconciliationResponse](t, response)

	if !reconciliation.Consistent {
		t.Fatal("expected reconciliation to remain consistent after restart")
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
}
