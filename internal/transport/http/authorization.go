package httptransport

import (
	"github.com/gofiber/fiber/v3"
)

// RequireProvider allows only authenticated provider identities.
func RequireProvider(c fiber.Ctx) error {
	principal, ok := PrincipalFromContext(c)
	if !ok {
		return unauthorized(c)
	}

	if !principal.IsProvider() {
		return forbidden(c)
	}

	return c.Next()
}

// RequireInternal allows only authenticated internal service identities.
func RequireInternal(c fiber.Ctx) error {
	principal, ok := PrincipalFromContext(c)
	if !ok {
		return unauthorized(c)
	}

	if !principal.IsInternal() {
		return forbidden(c)
	}

	return c.Next()
}

// forbidden returns a standardized authorization error.
func forbidden(c fiber.Ctx) error {
	return c.Status(
		fiber.StatusForbidden,
	).JSON(fiber.Map{
		"error": "forbidden",
	})
}
