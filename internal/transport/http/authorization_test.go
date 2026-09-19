package httptransport

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v3"

	"github.com/pablo-banker/junglegaming-test/internal/auth"
)

// TestRequireProviderAllowsProvider verifies provider authorization.
func TestRequireProviderAllowsProvider(t *testing.T) {
	app := newTestFiberApp()

	middleware := NewAuthMiddleware(
		&fakeTokenVerifier{
			principal: auth.Principal{
				ClientID:   "provider-a",
				ProviderID: "provider-a",
				Roles: []string{
					"provider",
				},
			},
		}, slog.New(slog.DiscardHandler))

	app.Get(
		"/protected",
		middleware.Authenticate,
		RequireProvider,
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

// TestRequireProviderRejectsInternal verifies internal identities cannot use provider routes.
func TestRequireProviderRejectsInternal(t *testing.T) {
	app := newTestFiberApp()

	middleware := NewAuthMiddleware(
		&fakeTokenVerifier{
			principal: auth.Principal{
				ClientID: "internal-service",
				Roles: []string{
					"internal",
				},
			},
		}, slog.New(slog.DiscardHandler))

	app.Get(
		"/protected",
		middleware.Authenticate,
		RequireProvider,
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
		"Bearer valid-token",
	)

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("failed to execute request: %v", err)
	}

	defer resp.Body.Close()

	if resp.StatusCode != fiber.StatusForbidden {
		t.Fatalf(
			"expected status 403, got %d",
			resp.StatusCode,
		)
	}
}

// TestRequireInternalAllowsInternal verifies internal service authorization.
func TestRequireInternalAllowsInternal(t *testing.T) {
	app := newTestFiberApp()

	middleware := NewAuthMiddleware(
		&fakeTokenVerifier{
			principal: auth.Principal{
				ClientID: "internal-service",
				Roles: []string{
					"internal",
				},
			},
		}, slog.New(slog.DiscardHandler))

	app.Get(
		"/protected",
		middleware.Authenticate,
		RequireInternal,
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

// TestRequireInternalRejectsProvider verifies providers cannot use internal routes.
func TestRequireInternalRejectsProvider(t *testing.T) {
	app := newTestFiberApp()

	middleware := NewAuthMiddleware(
		&fakeTokenVerifier{
			principal: auth.Principal{
				ClientID:   "provider-a",
				ProviderID: "provider-a",
				Roles: []string{
					"provider",
				},
			},
		}, slog.New(slog.DiscardHandler))

	app.Get(
		"/protected",
		middleware.Authenticate,
		RequireInternal,
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
		"Bearer valid-token",
	)

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("failed to execute request: %v", err)
	}

	defer resp.Body.Close()

	if resp.StatusCode != fiber.StatusForbidden {
		t.Fatalf(
			"expected status 403, got %d",
			resp.StatusCode,
		)
	}
}
