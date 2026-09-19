package httptransport

import (
	"github.com/gofiber/fiber/v3"
)

// RequireProvider allows only authenticated provider identities.
func RequireProvider(c fiber.Ctx) error {
	principal, ok := PrincipalFromContext(c)
	if !ok {
		return errUnauthorized
	}

	if !principal.IsProvider() {
		return errForbidden
	}

	return c.Next()
}

// RequireInternal allows only authenticated internal service identities.
func RequireInternal(c fiber.Ctx) error {
	principal, ok := PrincipalFromContext(c)
	if !ok {
		return errUnauthorized
	}

	if !principal.IsInternal() {
		return errForbidden
	}

	return c.Next()
}
