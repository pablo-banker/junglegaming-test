package httptransport

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v3"

	"github.com/pablo-banker/junglegaming-test/internal/auth"
)

type fakeTokenVerifier struct {
	principal auth.Principal
	err       error
}

// Verify returns the configured authentication result.
func (f *fakeTokenVerifier) Verify(
	_ context.Context,
	_ string,
) (auth.Principal, error) {
	return f.principal, f.err
}

// TestAuthMiddlewareRejectsMissingToken verifies requests without bearer tokens are unauthorized.
func TestAuthMiddlewareRejectsMissingToken(t *testing.T) {
	app := newTestFiberApp()

	middleware := NewAuthMiddleware(
		&fakeTokenVerifier{}, slog.New(slog.DiscardHandler))

	app.Get(
		"/protected",
		middleware.Authenticate,
		func(c fiber.Ctx) error {
			return c.SendStatus(fiber.StatusOK)
		},
	)

	req := httptest.NewRequest(
		http.MethodGet,
		"/protected",
		nil,
	)

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("failed to execute request: %v", err)
	}

	defer resp.Body.Close()

	if resp.StatusCode != fiber.StatusUnauthorized {
		t.Fatalf(
			"expected status 401, got %d",
			resp.StatusCode,
		)
	}
}

// TestAuthMiddlewareRejectsMalformedAuthorization verifies malformed authorization headers are unauthorized.
func TestAuthMiddlewareRejectsMalformedAuthorization(t *testing.T) {
	app := newTestFiberApp()

	middleware := NewAuthMiddleware(
		&fakeTokenVerifier{}, slog.New(slog.DiscardHandler))

	app.Get(
		"/protected",
		middleware.Authenticate,
		func(c fiber.Ctx) error {
			return c.SendStatus(fiber.StatusOK)
		},
	)

	req := httptest.NewRequest(
		http.MethodGet,
		"/protected",
		nil,
	)

	req.Header.Set(
		fiber.HeaderAuthorization,
		"invalid-token",
	)

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("failed to execute request: %v", err)
	}

	defer resp.Body.Close()

	if resp.StatusCode != fiber.StatusUnauthorized {
		t.Fatalf(
			"expected status 401, got %d",
			resp.StatusCode,
		)
	}
}

// TestAuthMiddlewareRejectsInvalidToken verifies verifier failures are unauthorized.
func TestAuthMiddlewareRejectsInvalidToken(t *testing.T) {
	app := newTestFiberApp()

	middleware := NewAuthMiddleware(
		&fakeTokenVerifier{
			err: auth.ErrInvalidToken,
		}, slog.New(slog.DiscardHandler))

	app.Get(
		"/protected",
		middleware.Authenticate,
		func(c fiber.Ctx) error {
			return c.SendStatus(fiber.StatusOK)
		},
	)

	req := httptest.NewRequest(
		http.MethodGet,
		"/protected",
		nil,
	)

	req.Header.Set(
		fiber.HeaderAuthorization,
		"Bearer invalid-token",
	)

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("failed to execute request: %v", err)
	}

	defer resp.Body.Close()

	if resp.StatusCode != fiber.StatusUnauthorized {
		t.Fatalf(
			"expected status 401, got %d",
			resp.StatusCode,
		)
	}
}

// TestAuthMiddlewareStoresPrincipal verifies authenticated identity reaches the next handler.
func TestAuthMiddlewareStoresPrincipal(t *testing.T) {
	app := newTestFiberApp()

	expected := auth.Principal{
		Subject:    "subject-123",
		ClientID:   "provider-a",
		ProviderID: "provider-a",
		Roles: []string{
			"provider",
		},
	}

	middleware := NewAuthMiddleware(
		&fakeTokenVerifier{
			principal: expected,
		}, slog.New(slog.DiscardHandler))

	app.Get(
		"/protected",
		middleware.Authenticate,
		func(c fiber.Ctx) error {
			principal, ok := PrincipalFromContext(c)
			if !ok {
				return errors.New("principal not found")
			}

			if principal.ProviderID != expected.ProviderID {
				return errors.New("unexpected provider")
			}

			return c.SendStatus(fiber.StatusOK)
		},
	)

	req := httptest.NewRequest(
		http.MethodGet,
		"/protected",
		nil,
	)

	req.Header.Set(
		fiber.HeaderAuthorization,
		"Bearer valid-token",
	)

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("failed to execute request: %v", err)
	}

	defer resp.Body.Close()

	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf(
			"expected status 200, got %d",
			resp.StatusCode,
		)
	}
}
