//go:build integration

package integration

import (
	"context"
	"errors"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pablo-banker/junglegaming-test/internal/domain"
)

// TestDatabaseRejectsChangesToTerminalWager verifies terminal wagers are immutable in the database.
func TestDatabaseRejectsChangesToTerminalWager(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	pool := openIntegrationPool(t)
	wallet := openWallet(t, ctx, pool, "100.00")

	result, err := newIntegratedWagerService(pool).Process(
		ctx,
		wagerCommand(wallet, domain.WagerTransactionTypeBet, "10.00"),
		wagerMetadata(),
	)
	if err != nil {
		t.Fatalf("failed to process bet: %v", err)
	}

	statements := map[string]string{
		"status transition": `UPDATE wager_transactions SET status = 'REJECTED', failure_code = 'X', balance_before = NULL, balance_after = NULL WHERE id = $1`,
		"amount change":     `UPDATE wager_transactions SET amount = 99 WHERE id = $1`,
	}

	for name, statement := range statements {
		if _, err := pool.Exec(ctx, statement, result.TransactionID); err == nil {
			t.Errorf("%s: expected terminal wager update to be rejected", name)
		}
	}
}

// TestDatabaseEnforcesWalletVersionRules verifies balance and version change together.
func TestDatabaseEnforcesWalletVersionRules(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	pool := openIntegrationPool(t)
	wallet := openWallet(t, ctx, pool, "100.00")

	statements := map[string]string{
		"balance without version": `UPDATE wallets SET balance = 50 WHERE id = $1`,
		"version without balance": `UPDATE wallets SET version = version + 1 WHERE id = $1`,
		"player change":           `UPDATE wallets SET player_id = gen_random_uuid() WHERE id = $1`,
	}

	for name, statement := range statements {
		if _, err := pool.Exec(ctx, statement, wallet.WalletID); err == nil {
			t.Errorf("%s: expected wallet update to be rejected", name)
		}
	}

	if _, err := pool.Exec(ctx, `UPDATE wallets SET balance = 50, version = version + 1 WHERE id = $1`, wallet.WalletID); err != nil {
		t.Fatalf("expected a versioned balance change to be accepted: %v", err)
	}
}

// openApplicationRolePool connects to the test database as the least privilege application role.
func openApplicationRolePool(t *testing.T) *pgxpool.Pool {
	t.Helper()

	databaseURL, err := url.Parse(os.Getenv("TEST_DATABASE_URL"))
	if err != nil || databaseURL.Host == "" {
		t.Skip("TEST_DATABASE_URL is not configured")
	}

	password := os.Getenv("APP_DB_PASSWORD")
	if password == "" {
		password = "jungle_app"
	}

	databaseURL.User = url.UserPassword("jungle_app", password)

	pool, err := pgxpool.New(context.Background(), databaseURL.String())
	if err != nil {
		t.Fatalf("failed to create application role pool: %v", err)
	}

	t.Cleanup(pool.Close)

	return pool
}

// TestApplicationRoleCannotRewriteHistory verifies the runtime role cannot rewrite financial history.
func TestApplicationRoleCannotRewriteHistory(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	ownerPool := openIntegrationPool(t)
	appPool := openApplicationRolePool(t)

	wallet := openWallet(t, ctx, ownerPool, "100.00")

	result, err := newIntegratedWagerService(appPool).Process(
		ctx,
		wagerCommand(wallet, domain.WagerTransactionTypeBet, "10.00"),
		wagerMetadata(),
	)
	if err != nil || result.Status != domain.WagerTransactionStatusProcessed {
		t.Fatalf("expected the application role to process a bet, got %v %v", result, err)
	}

	statements := map[string]string{
		"update ledger":           `UPDATE wallet_ledger_entries SET amount = amount WHERE wallet_id = $1`,
		"delete ledger":           `DELETE FROM wallet_ledger_entries WHERE wallet_id = $1`,
		"delete wager":            `DELETE FROM wager_transactions WHERE wallet_id = $1`,
		"delete wallet":           `DELETE FROM wallets WHERE id = $1`,
		"truncate ledger":         `TRUNCATE wallet_ledger_entries`,
		"disable ledger triggers": `ALTER TABLE wallet_ledger_entries DISABLE TRIGGER ALL`,
	}

	for name, statement := range statements {
		var args []any
		if strings.Contains(statement, "$1") {
			args = append(args, wallet.WalletID)
		}

		_, err := appPool.Exec(ctx, statement, args...)

		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || (pgErr.Code != "42501" && pgErr.Code != "42809") {
			t.Errorf("%s: expected insufficient privilege, got %v", name, err)
		}
	}

	if _, err := appPool.Exec(ctx, `SET session_replication_role = replica`); err == nil {
		t.Error("expected the application role to be unable to skip triggers")
	}

	assertReconciled(t, ctx, ownerPool, wallet.WalletID)
}
