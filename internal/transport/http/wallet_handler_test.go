//go:build unit

package httptransport

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"

	"github.com/pablo-banker/junglegaming-test/internal/application"
	"github.com/pablo-banker/junglegaming-test/internal/domain"
)

type fakeWalletHandlerService struct {
	getResult   *application.WalletResult
	getErr      error
	getWalletID string

	ledgerResult          *application.WalletLedgerResult
	ledgerErr             error
	ledgerWalletID        string
	ledgerBeforeCreatedAt *time.Time
	ledgerBeforeID        *uuid.UUID
	ledgerLimit           int
	ledgerCalled          bool

	reconcileResult   *application.WalletReconciliationResult
	reconcileErr      error
	reconcileWalletID string
}

// Create returns no result because wallet creation is not exercised by these tests.
func (f *fakeWalletHandlerService) Create(
	context.Context,
	application.CreateWalletCommand,
	application.CommandMetadata,
) (*application.CreateWalletResult, error) {
	return nil, nil
}

// Get returns the configured wallet result.
func (f *fakeWalletHandlerService) Get(_ context.Context, walletID string) (*application.WalletResult, error) {
	f.getWalletID = walletID

	return f.getResult, f.getErr
}

// ListLedger returns the configured ledger result.
func (f *fakeWalletHandlerService) ListLedger(
	_ context.Context,
	walletID string,
	beforeCreatedAt *time.Time,
	beforeID *uuid.UUID,
	limit int,
) (*application.WalletLedgerResult, error) {
	f.ledgerCalled = true
	f.ledgerWalletID = walletID
	f.ledgerBeforeCreatedAt = beforeCreatedAt
	f.ledgerBeforeID = beforeID
	f.ledgerLimit = limit

	return f.ledgerResult, f.ledgerErr
}

// Reconcile returns the configured reconciliation result.
func (f *fakeWalletHandlerService) Reconcile(_ context.Context, walletID string) (*application.WalletReconciliationResult, error) {
	f.reconcileWalletID = walletID

	return f.reconcileResult, f.reconcileErr
}

// newWalletHandlerTestApp creates a Fiber app for wallet handler tests.
func newWalletHandlerTestApp(service walletService) *fiber.App {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	handler := &WalletHandler{
		service: service,
		logger:  logger,
	}

	app := fiber.New(fiber.Config{
		ErrorHandler: func(c fiber.Ctx, err error) error {
			return BuildErrorResponse(c, logger, ResolveError(err))
		},
	})

	app.Get("/wallets/:walletId", handler.Get)
	app.Get("/wallets/:walletId/ledger", handler.Ledger)
	app.Post("/wallets/:walletId/reconciliation", handler.Reconcile)

	return app
}

// walletHandlerTestMoney creates BRL money for handler tests.
func walletHandlerTestMoney(t *testing.T, amount string) domain.Money {
	t.Helper()

	currency, err := domain.NewCurrency("BRL")
	if err != nil {
		t.Fatalf("unexpected currency error: %v", err)
	}

	money, err := domain.ParseMoney(amount, currency)
	if err != nil {
		t.Fatalf("unexpected money error: %v", err)
	}

	return money
}

// walletHandlerTestLedgerEntry creates a ledger entry for handler tests.
func walletHandlerTestLedgerEntry(t *testing.T, walletID uuid.UUID, createdAt time.Time) *domain.WalletLedgerEntry {
	t.Helper()

	amount := walletHandlerTestMoney(t, "10.00")
	before := walletHandlerTestMoney(t, "0.00")
	after := walletHandlerTestMoney(t, "10.00")

	entry, err := domain.NewWalletLedgerEntry(
		uuid.New(),
		walletID,
		uuid.New(),
		domain.WalletLedgerDirectionCredit,
		amount,
		before,
		after,
		createdAt,
	)
	if err != nil {
		t.Fatalf("unexpected ledger entry error: %v", err)
	}

	return entry
}

// TestWalletHandlerGetReturnsWallet verifies wallet retrieval.
func TestWalletHandlerGetReturnsWallet(t *testing.T) {
	walletID := uuid.New()
	playerID := uuid.New()

	service := &fakeWalletHandlerService{
		getResult: &application.WalletResult{
			WalletID: walletID,
			PlayerID: playerID,
			Balance:  walletHandlerTestMoney(t, "100.00"),
			Version:  3,
		},
	}

	app := newWalletHandlerTestApp(service)

	request := httptest.NewRequest(http.MethodGet, "/wallets/"+walletID.String(), nil)

	response, err := app.Test(request)
	if err != nil {
		t.Fatalf("unexpected request error: %v", err)
	}

	defer response.Body.Close()

	if response.StatusCode != fiber.StatusOK {
		t.Fatalf("expected status 200, got %d", response.StatusCode)
	}

	if service.getWalletID != walletID.String() {
		t.Errorf("expected wallet id %s, got %s", walletID, service.getWalletID)
	}

	var body struct {
		Data struct {
			ID       string `json:"id"`
			PlayerID string `json:"playerId"`
			Balance  struct {
				Amount   string `json:"amount"`
				Currency string `json:"currency"`
			} `json:"balance"`
			Version int64 `json:"version"`
		} `json:"data"`
	}

	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("unexpected decode error: %v", err)
	}

	if body.Data.ID != walletID.String() {
		t.Errorf("expected wallet id %s, got %s", walletID, body.Data.ID)
	}

	if body.Data.PlayerID != playerID.String() {
		t.Errorf("expected player id %s, got %s", playerID, body.Data.PlayerID)
	}

	if body.Data.Balance.Amount != "100.00" {
		t.Errorf("expected balance 100.00, got %s", body.Data.Balance.Amount)
	}

	if body.Data.Version != 3 {
		t.Errorf("expected version 3, got %d", body.Data.Version)
	}
}

// TestWalletHandlerGetReturnsNotFound verifies wallet lookup failures.
func TestWalletHandlerGetReturnsNotFound(t *testing.T) {
	service := &fakeWalletHandlerService{
		getErr: application.ErrWalletNotFound,
	}

	app := newWalletHandlerTestApp(service)

	request := httptest.NewRequest(http.MethodGet, "/wallets/"+uuid.NewString(), nil)

	response, err := app.Test(request)
	if err != nil {
		t.Fatalf("unexpected request error: %v", err)
	}

	defer response.Body.Close()

	if response.StatusCode != fiber.StatusNotFound {
		t.Fatalf("expected status 404, got %d", response.StatusCode)
	}
}

// TestWalletHandlerLedgerUsesDefaultLimit verifies the default pagination limit.
func TestWalletHandlerLedgerUsesDefaultLimit(t *testing.T) {
	walletID := uuid.New()
	entry := walletHandlerTestLedgerEntry(t, walletID, time.Now().UTC())

	service := &fakeWalletHandlerService{
		ledgerResult: &application.WalletLedgerResult{
			Entries: []*domain.WalletLedgerEntry{
				entry,
			},
		},
	}

	app := newWalletHandlerTestApp(service)

	request := httptest.NewRequest(http.MethodGet, "/wallets/"+walletID.String()+"/ledger", nil)

	response, err := app.Test(request)
	if err != nil {
		t.Fatalf("unexpected request error: %v", err)
	}

	defer response.Body.Close()

	if response.StatusCode != fiber.StatusOK {
		t.Fatalf("expected status 200, got %d", response.StatusCode)
	}

	if service.ledgerLimit != defaultLedgerLimit+1 {
		t.Errorf("expected repository limit %d, got %d", defaultLedgerLimit+1, service.ledgerLimit)
	}

	var body struct {
		Data struct {
			Entries []struct {
				ID string `json:"id"`
			} `json:"entries"`
			NextCursor string `json:"nextCursor"`
		} `json:"data"`
	}

	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("unexpected decode error: %v", err)
	}

	if len(body.Data.Entries) != 1 {
		t.Fatalf("expected 1 ledger entry, got %d", len(body.Data.Entries))
	}

	if body.Data.NextCursor != "" {
		t.Errorf("expected no next cursor, got %s", body.Data.NextCursor)
	}
}

// TestWalletHandlerLedgerReturnsNextCursor verifies cursor pagination.
func TestWalletHandlerLedgerReturnsNextCursor(t *testing.T) {
	walletID := uuid.New()

	first := walletHandlerTestLedgerEntry(t, walletID, time.Date(2026, 9, 18, 20, 0, 0, 0, time.UTC))
	second := walletHandlerTestLedgerEntry(t, walletID, time.Date(2026, 9, 18, 19, 0, 0, 0, time.UTC))
	third := walletHandlerTestLedgerEntry(t, walletID, time.Date(2026, 9, 18, 18, 0, 0, 0, time.UTC))

	service := &fakeWalletHandlerService{
		ledgerResult: &application.WalletLedgerResult{
			Entries: []*domain.WalletLedgerEntry{
				first,
				second,
				third,
			},
		},
	}

	app := newWalletHandlerTestApp(service)

	request := httptest.NewRequest(http.MethodGet, "/wallets/"+walletID.String()+"/ledger?limit=2", nil)

	response, err := app.Test(request)
	if err != nil {
		t.Fatalf("unexpected request error: %v", err)
	}

	defer response.Body.Close()

	if response.StatusCode != fiber.StatusOK {
		t.Fatalf("expected status 200, got %d", response.StatusCode)
	}

	if service.ledgerLimit != 3 {
		t.Errorf("expected service limit 3, got %d", service.ledgerLimit)
	}

	var body struct {
		Data struct {
			Entries []struct {
				ID string `json:"id"`
			} `json:"entries"`
			NextCursor string `json:"nextCursor"`
		} `json:"data"`
	}

	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("unexpected decode error: %v", err)
	}

	if len(body.Data.Entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(body.Data.Entries))
	}

	if body.Data.NextCursor == "" {
		t.Fatal("expected next cursor")
	}

	createdAt, id, err := decodeLedgerCursor(body.Data.NextCursor)
	if err != nil {
		t.Fatalf("unexpected cursor decode error: %v", err)
	}

	if !createdAt.Equal(second.CreatedAt()) {
		t.Errorf("expected cursor createdAt %s, got %s", second.CreatedAt(), *createdAt)
	}

	if *id != second.ID() {
		t.Errorf("expected cursor id %s, got %s", second.ID(), *id)
	}
}

// TestWalletHandlerLedgerDecodesCursor verifies incoming cursor parsing.
func TestWalletHandlerLedgerDecodesCursor(t *testing.T) {
	walletID := uuid.New()
	entry := walletHandlerTestLedgerEntry(t, walletID, time.Date(2026, 9, 18, 19, 30, 0, 0, time.UTC))

	cursor, err := encodeLedgerCursor(entry)
	if err != nil {
		t.Fatalf("unexpected cursor encode error: %v", err)
	}

	service := &fakeWalletHandlerService{
		ledgerResult: &application.WalletLedgerResult{
			Entries: []*domain.WalletLedgerEntry{},
		},
	}

	app := newWalletHandlerTestApp(service)

	request := httptest.NewRequest(http.MethodGet, "/wallets/"+walletID.String()+"/ledger?cursor="+cursor, nil)

	response, err := app.Test(request)
	if err != nil {
		t.Fatalf("unexpected request error: %v", err)
	}

	defer response.Body.Close()

	if response.StatusCode != fiber.StatusOK {
		t.Fatalf("expected status 200, got %d", response.StatusCode)
	}

	if service.ledgerBeforeCreatedAt == nil {
		t.Fatal("expected cursor createdAt")
	}

	if service.ledgerBeforeID == nil {
		t.Fatal("expected cursor id")
	}

	if !service.ledgerBeforeCreatedAt.Equal(entry.CreatedAt()) {
		t.Errorf("expected createdAt %s, got %s", entry.CreatedAt(), *service.ledgerBeforeCreatedAt)
	}

	if *service.ledgerBeforeID != entry.ID() {
		t.Errorf("expected id %s, got %s", entry.ID(), *service.ledgerBeforeID)
	}
}

// TestWalletHandlerLedgerRejectsInvalidLimit verifies invalid pagination limits.
func TestWalletHandlerLedgerRejectsInvalidLimit(t *testing.T) {
	service := &fakeWalletHandlerService{}
	app := newWalletHandlerTestApp(service)

	request := httptest.NewRequest(http.MethodGet, "/wallets/"+uuid.NewString()+"/ledger?limit=101", nil)

	response, err := app.Test(request)
	if err != nil {
		t.Fatalf("unexpected request error: %v", err)
	}

	defer response.Body.Close()

	if response.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", response.StatusCode)
	}

	if service.ledgerCalled {
		t.Fatal("expected service not to be called")
	}
}

// TestWalletHandlerLedgerRejectsInvalidCursor verifies invalid pagination cursors.
func TestWalletHandlerLedgerRejectsInvalidCursor(t *testing.T) {
	service := &fakeWalletHandlerService{}
	app := newWalletHandlerTestApp(service)

	request := httptest.NewRequest(http.MethodGet, "/wallets/"+uuid.NewString()+"/ledger?cursor=invalid!", nil)

	response, err := app.Test(request)
	if err != nil {
		t.Fatalf("unexpected request error: %v", err)
	}

	defer response.Body.Close()

	if response.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", response.StatusCode)
	}

	if service.ledgerCalled {
		t.Fatal("expected service not to be called")
	}
}

// TestWalletHandlerReconcileReturnsResult verifies reconciliation responses.
func TestWalletHandlerReconcileReturnsResult(t *testing.T) {
	walletID := uuid.New()

	service := &fakeWalletHandlerService{
		reconcileResult: &application.WalletReconciliationResult{
			WalletID:          walletID,
			StoredBalance:     walletHandlerTestMoney(t, "100.00"),
			CalculatedBalance: walletHandlerTestMoney(t, "90.00"),
			Difference:        walletHandlerTestMoney(t, "10.00"),
			Consistent:        false,
			CheckedEntries:    3,
		},
	}

	app := newWalletHandlerTestApp(service)

	request := httptest.NewRequest(http.MethodPost, "/wallets/"+walletID.String()+"/reconciliation", nil)

	response, err := app.Test(request)
	if err != nil {
		t.Fatalf("unexpected request error: %v", err)
	}

	defer response.Body.Close()

	if response.StatusCode != fiber.StatusOK {
		t.Fatalf("expected status 200, got %d", response.StatusCode)
	}

	if service.reconcileWalletID != walletID.String() {
		t.Errorf("expected wallet id %s, got %s", walletID, service.reconcileWalletID)
	}

	var body struct {
		Data struct {
			WalletID string `json:"walletId"`

			StoredBalance struct {
				Amount string `json:"amount"`
			} `json:"storedBalance"`

			CalculatedBalance struct {
				Amount string `json:"amount"`
			} `json:"calculatedBalance"`

			Difference struct {
				Amount string `json:"amount"`
			} `json:"difference"`

			Consistent     bool  `json:"consistent"`
			CheckedEntries int64 `json:"checkedEntries"`
		} `json:"data"`
	}

	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("unexpected decode error: %v", err)
	}

	if body.Data.WalletID != walletID.String() {
		t.Errorf("expected wallet id %s, got %s", walletID, body.Data.WalletID)
	}

	if body.Data.StoredBalance.Amount != "100.00" {
		t.Errorf("expected stored balance 100.00, got %s", body.Data.StoredBalance.Amount)
	}

	if body.Data.CalculatedBalance.Amount != "90.00" {
		t.Errorf("expected calculated balance 90.00, got %s", body.Data.CalculatedBalance.Amount)
	}

	if body.Data.Difference.Amount != "10.00" {
		t.Errorf("expected difference 10.00, got %s", body.Data.Difference.Amount)
	}

	if body.Data.Consistent {
		t.Fatal("expected reconciliation to be inconsistent")
	}

	if body.Data.CheckedEntries != 3 {
		t.Errorf("expected 3 checked entries, got %d", body.Data.CheckedEntries)
	}
}
