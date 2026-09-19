package httptransport

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v3"

	"github.com/pablo-banker/junglegaming-test/internal/application"
	"github.com/pablo-banker/junglegaming-test/internal/domain"
)

// TestResolveErrorKeepsSituationsDistinguishable verifies every contract situation has its own status and code.
func TestResolveErrorKeepsSituationsDistinguishable(t *testing.T) {
	tests := []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{"invalid money", domain.ErrInvalidMoney, http.StatusUnprocessableEntity, "VALIDATION_FAILED"},
		{"wallet mismatch", application.ErrWalletMismatch, http.StatusUnprocessableEntity, "VALIDATION_FAILED"},
		{"idempotency conflict", application.ErrIdempotencyConflict, http.StatusConflict, "IDEMPOTENCY_CONFLICT"},
		{"external conflict", application.ErrExternalTransactionConflict, http.StatusConflict, "EXTERNAL_TRANSACTION_CONFLICT"},
		{"wallet exists", application.ErrWalletAlreadyExists, http.StatusConflict, "WALLET_ALREADY_EXISTS"},
		{"not found", application.ErrWalletNotFound, http.StatusNotFound, "NOT_FOUND"},
		{"unavailable", fmt.Errorf("%w: connection refused", application.ErrUnavailable), http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE"},
		{"deadline", context.DeadlineExceeded, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE"},
		{"unexpected", errors.New("boom"), http.StatusInternalServerError, "INTERNAL_ERROR"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			apiErr := resolveError(test.err)

			if apiErr.Status != test.status || apiErr.Code != test.code {
				t.Fatalf("expected %d %s, got %d %s", test.status, test.code, apiErr.Status, apiErr.Code)
			}
		})
	}
}

// TestErrorHandlerWritesContract verifies the error body, hidden causes and Retry-After on 503.
func TestErrorHandlerWritesContract(t *testing.T) {
	app := newTestFiberApp()

	app.Get("/unavailable", func(fiber.Ctx) error {
		return fmt.Errorf("%w: dial tcp: connection refused", application.ErrUnavailable)
	})

	app.Get("/invalid", func(fiber.Ctx) error {
		return domain.ErrInvalidMoney
	})

	response, err := app.Test(httptest.NewRequest(http.MethodGet, "/unavailable", nil))
	if err != nil {
		t.Fatalf("unexpected request error: %v", err)
	}

	if response.StatusCode != http.StatusServiceUnavailable || response.Header.Get("Retry-After") == "" {
		t.Fatalf("expected 503 with Retry-After, got %d %q", response.StatusCode, response.Header.Get("Retry-After"))
	}

	var unavailable map[string]string
	if err := json.NewDecoder(response.Body).Decode(&unavailable); err != nil {
		t.Fatalf("unexpected decode error: %v", err)
	}

	if unavailable["code"] != "SERVICE_UNAVAILABLE" || unavailable["details"] != "" {
		t.Fatalf("expected SERVICE_UNAVAILABLE without internal details, got %v", unavailable)
	}

	response, err = app.Test(httptest.NewRequest(http.MethodGet, "/invalid", nil))
	if err != nil {
		t.Fatalf("unexpected request error: %v", err)
	}

	var invalid map[string]string
	if err := json.NewDecoder(response.Body).Decode(&invalid); err != nil {
		t.Fatalf("unexpected decode error: %v", err)
	}

	if response.StatusCode != http.StatusUnprocessableEntity || invalid["details"] != domain.ErrInvalidMoney.Error() {
		t.Fatalf("expected 422 with field details, got %d %v", response.StatusCode, invalid)
	}
}
