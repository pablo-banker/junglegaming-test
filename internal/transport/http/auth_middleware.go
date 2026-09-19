package httptransport

import (
	"log/slog"
	"strings"

	"github.com/gofiber/fiber/v3"

	"github.com/pablo-banker/junglegaming-test/internal/auth"
	"github.com/pablo-banker/junglegaming-test/internal/observability"
)

type authContextKey string

const principalContextKey authContextKey = "principal"

// AuthMiddleware authenticates requests using bearer access tokens.
type AuthMiddleware struct {
	verifier auth.TokenVerifier
	logger   *slog.Logger
}

// NewAuthMiddleware creates an authentication middleware.
func NewAuthMiddleware(verifier auth.TokenVerifier, logger *slog.Logger) *AuthMiddleware {
	return &AuthMiddleware{
		verifier: verifier,
		logger:   logger,
	}
}

// Authenticate validates the bearer token and stores the authenticated principal.
func (m *AuthMiddleware) Authenticate(c fiber.Ctx) error {
	token, ok := bearerToken(c.Get(fiber.HeaderAuthorization))
	if !ok {
		return errUnauthorized
	}

	principal, err := m.verifier.Verify(
		c.Context(),
		token,
	)
	if err != nil {
		// The reason (expired, wrong audience, bad signature) never includes the token itself.
		m.logger.WarnContext(c.Context(), "access token rejected", slog.Any("reason", err))

		return errUnauthorized
	}

	c.Locals(principalContextKey, principal)
	c.SetContext(observability.WithAttrs(
		c.Context(),
		slog.String("clientId", principal.ClientID),
		slog.String("providerId", principal.ProviderID),
	))

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
