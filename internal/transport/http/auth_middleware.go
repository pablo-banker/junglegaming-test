package httptransport

import (
	"strings"

	"github.com/gofiber/fiber/v3"

	"github.com/pablo-banker/junglegaming-test/internal/auth"
)

type authContextKey string

const principalContextKey authContextKey = "principal"

// AuthMiddleware authenticates requests using bearer access tokens.
type AuthMiddleware struct {
	verifier auth.TokenVerifier
}

// NewAuthMiddleware creates an authentication middleware.
func NewAuthMiddleware(verifier auth.TokenVerifier) *AuthMiddleware {
	return &AuthMiddleware{
		verifier: verifier,
	}
}

// Authenticate validates the bearer token and stores the authenticated principal.
func (m *AuthMiddleware) Authenticate(c fiber.Ctx) error {
	token, ok := bearerToken(c.Get(fiber.HeaderAuthorization))
	if !ok {
		return unauthorized(c)
	}

	principal, err := m.verifier.Verify(
		c.Context(),
		token,
	)
	if err != nil {
		return unauthorized(c)
	}

	c.Locals(principalContextKey, principal)

	return c.Next()
}

// PrincipalFromContext returns the authenticated principal stored in the request.
func PrincipalFromContext(c fiber.Ctx) (auth.Principal, bool) {
	principal, ok := c.Locals(
		principalContextKey,
	).(auth.Principal)

	return principal, ok
}

// bearerToken extracts a bearer token from the Authorization header.
func bearerToken(header string) (string, bool) {
	parts := strings.Fields(header)

	if len(parts) != 2 {
		return "", false
	}

	if !strings.EqualFold(parts[0], "Bearer") {
		return "", false
	}

	if parts[1] == "" {
		return "", false
	}

	return parts[1], true
}

// unauthorized returns a standardized authentication error.
func unauthorized(c fiber.Ctx) error {
	return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
		"error": "unauthorized",
	})
}
