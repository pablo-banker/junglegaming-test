package httptransport

import (
	"context"
	"time"

	"github.com/gofiber/fiber/v3"
)

const readinessTimeout = 2 * time.Second

// ReadinessCheck verifies one dependency required to serve traffic.
type ReadinessCheck struct {
	Name  string
	Check func(ctx context.Context) error
}

type HealthHandler struct {
	checks []ReadinessCheck
}

type healthResponse struct {
	Status string            `json:"status"`
	Checks map[string]string `json:"checks,omitempty"`
}

// NewHealthHandler creates the public health handler.
func NewHealthHandler(checks []ReadinessCheck) *HealthHandler {
	return &HealthHandler{
		checks: checks,
	}
}

// Live reports whether the application process is running.
func (h *HealthHandler) Live(c fiber.Ctx) error {
	return c.JSON(healthResponse{
		Status: "ok",
	})
}

// Ready reports whether every required dependency is reachable.
func (h *HealthHandler) Ready(c fiber.Ctx) error {
	ctx, cancel := context.WithTimeout(c.Context(), readinessTimeout)
	defer cancel()

	response := healthResponse{
		Status: "ready",
		Checks: make(map[string]string, len(h.checks)),
	}

	status := fiber.StatusOK

	for _, check := range h.checks {
		if err := check.Check(ctx); err != nil {
			response.Checks[check.Name] = "unavailable"
			response.Status = "not_ready"
			status = fiber.StatusServiceUnavailable

			continue
		}

		response.Checks[check.Name] = "ok"
	}

	return c.Status(status).JSON(response)
}
