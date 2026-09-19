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
	"github.com/pablo-banker/junglegaming-test/internal/auth"
	"github.com/pablo-banker/junglegaming-test/internal/domain"
)

type fakeWagerHandlerService struct {
	getByIDResult        *application.WagerResult
	getByIDErr           error
	getByIDProviderID    string
	getByIDTransactionID string

	getByExternalResult        *application.WagerResult
	getByExternalErr           error
	getByExternalProviderID    string
	getByExternalTransactionID string
	getByExternalCalled        bool
}

// Process returns no result because wager processing is not exercised by these tests.
func (f *fakeWagerHandlerService) Process(context.Context, application.ProcessWagerCommand, application.CommandMetadata) (*application.ProcessWagerResult, error) {
	return nil, nil
}

// GetByID returns the configured wager result.
func (f *fakeWagerHandlerService) GetByID(_ context.Context, providerID string, transactionID string) (*application.WagerResult, error) {
	f.getByIDProviderID = providerID
	f.getByIDTransactionID = transactionID

	return f.getByIDResult, f.getByIDErr
}

// GetByExternalTransactionID returns the configured wager result.
func (f *fakeWagerHandlerService) GetByExternalTransactionID(_ context.Context, providerID string, externalTransactionID string) (*application.WagerResult, error) {
	f.getByExternalCalled = true
	f.getByExternalProviderID = providerID
	f.getByExternalTransactionID = externalTransactionID

	return f.getByExternalResult, f.getByExternalErr
}

// newWagerHandlerTestApp creates a Fiber app for wager handler tests.
func newWagerHandlerTestApp(service wagerService, principal auth.Principal) *fiber.App {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	handler := &WagerHandler{
		service: service,
		logger:  logger,
	}

	app := fiber.New(fiber.Config{
		ErrorHandler: func(c fiber.Ctx, err error) error {
			return BuildErrorResponse(c, logger, ResolveError(err))
		},
	})

	app.Use(func(c fiber.Ctx) error {
		c.Locals(principalContextKey, principal)

		return c.Next()
	})

	app.Get("/wagering/transactions/:transactionId", handler.GetByID)
	app.Get("/providers/:providerId/wagering/transactions/:externalTransactionId", handler.GetByExternalTransactionID)

	return app
}

// wagerHandlerTestMoney creates BRL money for handler tests.
func wagerHandlerTestMoney(t *testing.T, amount string) domain.Money {
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

// wagerHandlerTestResult creates a wager result for handler tests.
func wagerHandlerTestResult(t *testing.T, providerID string) *application.WagerResult {
	t.Helper()

	before := wagerHandlerTestMoney(t, "100.00")
	after := wagerHandlerTestMoney(t, "75.00")

	return &application.WagerResult{
		TransactionID:         uuid.New(),
		ProviderID:            providerID,
		ExternalTransactionID: "transaction-123",
		WalletID:              uuid.New(),
		PlayerID:              uuid.New(),
		RoundID:               "round-123",
		GameID:                "game-123",
		Type:                  domain.WagerTransactionTypeBet,
		Amount:                wagerHandlerTestMoney(t, "25.00"),
		Status:                domain.WagerTransactionStatusProcessed,
		BalanceBefore:         &before,
		BalanceAfter:          &after,
		CreatedAt:             time.Now().UTC(),
		UpdatedAt:             time.Now().UTC(),
	}
}

// TestWagerHandlerGetByIDReturnsTransaction verifies provider transaction retrieval.
func TestWagerHandlerGetByIDReturnsTransaction(t *testing.T) {
	result := wagerHandlerTestResult(t, "provider-a")

	service := &fakeWagerHandlerService{
		getByIDResult: result,
	}

	app := newWagerHandlerTestApp(service, auth.Principal{
		ProviderID: "provider-a",
		Roles:      []string{"provider"},
	})

	request := httptest.NewRequest(http.MethodGet, "/wagering/transactions/"+result.TransactionID.String(), nil)

	response, err := app.Test(request)
	if err != nil {
		t.Fatalf("unexpected request error: %v", err)
	}

	defer response.Body.Close()

	if response.StatusCode != fiber.StatusOK {
		t.Fatalf("expected status 200, got %d", response.StatusCode)
	}

	if service.getByIDProviderID != "provider-a" {
		t.Errorf("expected provider-a, got %s", service.getByIDProviderID)
	}

	if service.getByIDTransactionID != result.TransactionID.String() {
		t.Errorf("expected transaction id %s, got %s", result.TransactionID, service.getByIDTransactionID)
	}

	var body struct {
		Data struct {
			TransactionID         string `json:"transactionId"`
			ProviderID            string `json:"providerId"`
			ExternalTransactionID string `json:"externalTransactionId"`
			Status                string `json:"status"`
			BalanceAfter          struct {
				Amount string `json:"amount"`
			} `json:"balanceAfter"`
		} `json:"data"`
	}

	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("unexpected decode error: %v", err)
	}

	if body.Data.TransactionID != result.TransactionID.String() {
		t.Errorf("expected transaction id %s, got %s", result.TransactionID, body.Data.TransactionID)
	}

	if body.Data.ProviderID != "provider-a" {
		t.Errorf("expected provider-a, got %s", body.Data.ProviderID)
	}

	if body.Data.Status != "PROCESSED" {
		t.Errorf("expected PROCESSED, got %s", body.Data.Status)
	}

	if body.Data.BalanceAfter.Amount != "75.00" {
		t.Errorf("expected balance after 75.00, got %s", body.Data.BalanceAfter.Amount)
	}
}

// TestWagerHandlerGetByIDReturnsNotFoundForAnotherProvider verifies provider isolation.
func TestWagerHandlerGetByIDReturnsNotFoundForAnotherProvider(t *testing.T) {
	service := &fakeWagerHandlerService{
		getByIDErr: application.ErrNotFound,
	}

	app := newWagerHandlerTestApp(service, auth.Principal{
		ProviderID: "provider-a",
		Roles:      []string{"provider"},
	})

	request := httptest.NewRequest(http.MethodGet, "/wagering/transactions/"+uuid.NewString(), nil)

	response, err := app.Test(request)
	if err != nil {
		t.Fatalf("unexpected request error: %v", err)
	}

	defer response.Body.Close()

	if response.StatusCode != fiber.StatusNotFound {
		t.Fatalf("expected status 404, got %d", response.StatusCode)
	}
}

// TestWagerHandlerGetByExternalTransactionIDReturnsTransaction verifies external transaction lookup.
func TestWagerHandlerGetByExternalTransactionIDReturnsTransaction(t *testing.T) {
	result := wagerHandlerTestResult(t, "provider-a")

	service := &fakeWagerHandlerService{
		getByExternalResult: result,
	}

	app := newWagerHandlerTestApp(service, auth.Principal{
		ProviderID: "provider-a",
		Roles:      []string{"provider"},
	})

	request := httptest.NewRequest(http.MethodGet, "/providers/provider-a/wagering/transactions/transaction-123", nil)

	response, err := app.Test(request)
	if err != nil {
		t.Fatalf("unexpected request error: %v", err)
	}

	defer response.Body.Close()

	if response.StatusCode != fiber.StatusOK {
		t.Fatalf("expected status 200, got %d", response.StatusCode)
	}

	if !service.getByExternalCalled {
		t.Fatal("expected service to be called")
	}

	if service.getByExternalProviderID != "provider-a" {
		t.Errorf("expected provider-a, got %s", service.getByExternalProviderID)
	}

	if service.getByExternalTransactionID != "transaction-123" {
		t.Errorf("expected transaction-123, got %s", service.getByExternalTransactionID)
	}
}

// TestWagerHandlerGetByExternalTransactionIDRejectsAnotherProvider verifies URL provider isolation.
func TestWagerHandlerGetByExternalTransactionIDRejectsAnotherProvider(t *testing.T) {
	service := &fakeWagerHandlerService{}

	app := newWagerHandlerTestApp(service, auth.Principal{
		ProviderID: "provider-a",
		Roles:      []string{"provider"},
	})

	request := httptest.NewRequest(http.MethodGet, "/providers/provider-b/wagering/transactions/transaction-123", nil)

	response, err := app.Test(request)
	if err != nil {
		t.Fatalf("unexpected request error: %v", err)
	}

	defer response.Body.Close()

	if response.StatusCode != fiber.StatusForbidden {
		t.Fatalf("expected status 403, got %d", response.StatusCode)
	}

	if service.getByExternalCalled {
		t.Fatal("expected service not to be called")
	}
}
