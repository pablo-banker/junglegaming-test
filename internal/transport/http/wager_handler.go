package httptransport

import (
	"context"
	"log/slog"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"

	"github.com/pablo-banker/junglegaming-test/internal/application"
	"github.com/pablo-banker/junglegaming-test/internal/domain"
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
}

// NewWagerHandler creates a wagering HTTP handler.
func NewWagerHandler(service *application.WagerService, logger *slog.Logger) *WagerHandler {
	return &WagerHandler{
		service: service,
		logger:  logger,
	}
}

// Create processes an external wagering transaction.
func (h *WagerHandler) Create(c fiber.Ctx) error {
	principal, ok := PrincipalFromContext(c)
	if !ok {
		return fiber.ErrUnauthorized
	}

	var request createWagerRequest
	if err := c.Bind().Body(&request); err != nil {
		return BuildErrorResponse(c, h.logger, ResolveError(fiber.ErrBadRequest))
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
		return BuildErrorResponse(c, h.logger, ResolveError(fiber.ErrBadRequest))
	}

	if request.ProviderID != principal.ProviderID {
		return fiber.ErrForbidden
	}

	idempotencyKey := c.Get("Idempotency-Key")
	if idempotencyKey == "" {
		return BuildErrorResponse(c, h.logger, ResolveError(fiber.ErrBadRequest))
	}

	result, err := h.service.Process(
		c.Context(),
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
			CorrelationID: uuid.NewString(),
		},
	)
	if err != nil {
		return BuildErrorResponse(c, h.logger, ResolveError(err))
	}

	status := fiber.StatusOK

	if result.Status == domain.WagerTransactionStatusPendingReference ||
		result.Status == domain.WagerTransactionStatusPending {
		status = fiber.StatusAccepted
	}

	return BuildSuccessResponse(c, status, createWagerResponse{
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
		return fiber.ErrUnauthorized
	}

	result, err := h.service.GetByID(c.Context(), principal.ProviderID, c.Params("transactionId"))
	if err != nil {
		return err
	}

	return BuildSuccessResponse(c, fiber.StatusOK, result)
}

// GetByExternalTransactionID returns a provider wager by external transaction id.
func (h *WagerHandler) GetByExternalTransactionID(c fiber.Ctx) error {
	principal, ok := PrincipalFromContext(c)
	if !ok {
		return fiber.ErrUnauthorized
	}

	providerID := c.Params("providerId")

	if providerID != principal.ProviderID {
		return fiber.ErrForbidden
	}

	result, err := h.service.GetByExternalTransactionID(c.Context(), providerID, c.Params("externalTransactionId"))
	if err != nil {
		return err
	}

	return BuildSuccessResponse(c, fiber.StatusOK, result)
}
