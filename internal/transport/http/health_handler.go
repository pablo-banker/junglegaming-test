package httptransport

import (
	"context"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/pablo-banker/junglegaming-test/internal/apierrors"
	"github.com/pablo-banker/junglegaming-test/internal/infrastructure/sqs"

	"github.com/jackc/pgx/v5/pgxpool"
)

type HealthHandler struct {
	db  *pgxpool.Pool
	sqs *sqs.HealthChecker
}

type HealthResponse struct {
	Status string `json:"status"`
}

// NewHealthHandler creates a health handler backed by the PostgreSQL pool.
func NewHealthHandler(db *pgxpool.Pool, sqsHealth *sqs.HealthChecker) *HealthHandler {
	return &HealthHandler{
		db:  db,
		sqs: sqsHealth,
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

	if err := h.sqs.Check(ctx); err != nil {
		return apierrors.ErrSQSUnavailable.WithCause(err)
	}

	return BuildSuccessResponse(c, fiber.StatusOK, HealthResponse{
		Status: "ready",
	})
}
