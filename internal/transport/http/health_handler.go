package httptransport

import (
	"context"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/pablo-banker/junglegaming-test/internal/apierrors"

	"github.com/jackc/pgx/v5/pgxpool"
)

type HealthHandler struct {
	db *pgxpool.Pool
}

type HealthResponse struct {
	Status string `json:"status"`
}

// NewHealthHandler creates a health handler backed by the PostgreSQL pool.
func NewHealthHandler(db *pgxpool.Pool) *HealthHandler {
	return &HealthHandler{
		db: db,
	}
}

// Live reports whether the application process is running.
func (h *HealthHandler) Live(c fiber.Ctx) error {
	return BuildSuccessResponse(
		c, fiber.StatusOK, HealthResponse{
			Status: "ok",
		},
	)
}

// Ready reports whether the application dependencies are available.
func (h *HealthHandler) Ready(c fiber.Ctx) error {
	ctx, cancel := context.WithTimeout(c.Context(), 2*time.Second)
	defer cancel()

	if err := h.db.Ping(ctx); err != nil {
		return apierrors.ErrPingDB.WithCause(err)
	}

	return BuildSuccessResponse(
		c, fiber.StatusOK, HealthResponse{
			Status: "ready",
		},
	)
}
