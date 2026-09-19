package httptransport

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"

	"github.com/pablo-banker/junglegaming-test/internal/application"
	"github.com/pablo-banker/junglegaming-test/internal/domain"
	"github.com/pablo-banker/junglegaming-test/internal/observability"
)

type createWagerRequest struct {
	ProviderID                     string       `json:"providerId"`
	ExternalTransactionID          string       `json:"externalTransactionId"`
	PlayerID                       string       `json:"playerId"`
	WalletID                       string       `json:"walletId"`
	RoundID                        string       `json:"roundId"`
	GameID                         string       `json:"gameId"`
	Kind                           string       `json:"kind"`
	Money                          moneyRequest `json:"money"`
	ReferenceExternalTransactionID string       `json:"referenceExternalTransactionId,omitempty"`
}

type createWagerResponse struct {
	TransactionID    uuid.UUID                     `json:"transactionId"`
	Status           domain.WagerTransactionStatus `json:"status"`
	Balance          *domain.Money                 `json:"balance,omitempty"`
	FailureCode      string                        `json:"failureCode,omitempty"`
	IdempotentReplay bool                          `json:"idempotentReplay"`
}

type wagerService interface {
	Process(ctx context.Context, command application.ProcessWagerCommand, metadata application.CommandMetadata) (*application.ProcessWagerResult, error)
	GetByID(ctx context.Context, providerID string, transactionID string) (*application.WagerResult, error)
	GetByExternalTransactionID(ctx context.Context, providerID string, externalTransactionID string) (*application.WagerResult, error)
}

// WagerHandler handles wagering HTTP operations.
type WagerHandler struct {
	service wagerService
	logger  *slog.Logger
	metrics *observability.Metrics
}

// NewWagerHandler creates a wagering HTTP handler.
func NewWagerHandler(service *application.WagerService, logger *slog.Logger, metrics *observability.Metrics) *WagerHandler {
	return &WagerHandler{
		service: service,
		logger:  logger,
		metrics: metrics,
	}
}

// Create processes an external wagering transaction.
func (h *WagerHandler) Create(c fiber.Ctx) error {
	principal, ok := PrincipalFromContext(c)
	if !ok {
		return errUnauthorized
	}

	var request createWagerRequest
	if err := c.Bind().Body(&request); err != nil {
		return errInvalidPayload
	}

	if request.ProviderID == "" ||
		request.ExternalTransactionID == "" ||
		request.PlayerID == "" ||
		request.WalletID == "" ||
		request.RoundID == "" ||
		request.GameID == "" ||
		request.Kind == "" ||
		request.Money.Amount == "" ||
		request.Money.Currency == "" {
		return errInvalidPayload.withDetails("providerId, externalTransactionId, playerId, walletId, roundId, gameId, kind and money are required")
	}

	if request.ProviderID != principal.ProviderID {
		return errForbidden
	}

	idempotencyKey := c.Get("Idempotency-Key")
	if idempotencyKey == "" {
		return errIdempotencyKeyRequired
	}

	ctx := observability.WithAttrs(c.Context(), slog.String("walletId", request.WalletID))
	start := time.Now()

	result, err := h.service.Process(
		ctx,
		application.ProcessWagerCommand{
			ProviderID:                     principal.ProviderID,
			ExternalTransactionID:          request.ExternalTransactionID,
			IdempotencyKey:                 idempotencyKey,
			WalletID:                       request.WalletID,
			PlayerID:                       request.PlayerID,
			RoundID:                        request.RoundID,
			GameID:                         request.GameID,
			Type:                           request.Kind,
			Amount:                         request.Money.Amount,
			Currency:                       request.Money.Currency,
			ReferenceExternalTransactionID: request.ReferenceExternalTransactionID,
		},
		application.CommandMetadata{
			CorrelationID: correlationID(c),
		},
	)
	if err != nil {
		h.countConflict(err)

		return err
	}

	h.metrics.ObserveWager("http", request.Kind, string(result.Status), result.IdempotentReplay, time.Since(start))
	h.logger.InfoContext(
		ctx,
		"wager processed",
		slog.String("transactionId", result.TransactionID.String()),
		slog.String("kind", request.Kind),
		slog.String("status", string(result.Status)),
		slog.String("failureCode", result.FailureCode),
		slog.Bool("idempotentReplay", result.IdempotentReplay),
	)

	status := fiber.StatusOK

	if result.Status == domain.WagerTransactionStatusPendingReference ||
		result.Status == domain.WagerTransactionStatusPending {
		status = fiber.StatusAccepted
	}

	return c.Status(status).JSON(createWagerResponse{
		TransactionID:    result.TransactionID,
		Status:           result.Status,
		Balance:          result.BalanceAfter,
		FailureCode:      result.FailureCode,
		IdempotentReplay: result.IdempotentReplay,
	})
}

// GetByID returns a wager transaction visible to the authenticated provider.
func (h *WagerHandler) GetByID(c fiber.Ctx) error {
	principal, ok := PrincipalFromContext(c)
	if !ok {
		return errUnauthorized
	}

	result, err := h.service.GetByID(c.Context(), principal.ProviderID, c.Params("transactionId"))
	if err != nil {
		return err
	}

	return c.JSON(result)
}

// GetByExternalTransactionID returns a provider wager by external transaction id.
func (h *WagerHandler) GetByExternalTransactionID(c fiber.Ctx) error {
	principal, ok := PrincipalFromContext(c)
	if !ok {
		return errUnauthorized
	}

	providerID := c.Params("providerId")

	if providerID != principal.ProviderID {
		return errForbidden
	}

	result, err := h.service.GetByExternalTransactionID(c.Context(), providerID, c.Params("externalTransactionId"))
	if err != nil {
		return err
	}

	return c.JSON(result)
}

// countConflict records idempotency conflicts, which point to misbehaving clients.
func (h *WagerHandler) countConflict(err error) {
	switch {
	case errors.Is(err, application.ErrIdempotencyConflict):
		h.metrics.CountConflict("idempotency_key")

	case errors.Is(err, application.ErrExternalTransactionConflict):
		h.metrics.CountConflict("external_transaction")
	}
}
