package httptransport

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"log/slog"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"

	"github.com/pablo-banker/junglegaming-test/internal/application"
	"github.com/pablo-banker/junglegaming-test/internal/domain"
)

const (
	defaultLedgerLimit = 50
	maxLedgerLimit     = 100
)

type createWalletRequest struct {
	PlayerID       string       `json:"playerId"`
	InitialBalance moneyRequest `json:"initialBalance"`
}

type moneyRequest struct {
	Amount   string `json:"amount"`
	Currency string `json:"currency"`
}

type createWalletResponse struct {
	ID       uuid.UUID    `json:"id"`
	PlayerID uuid.UUID    `json:"playerId"`
	Balance  domain.Money `json:"balance"`
	Version  int64        `json:"version"`
}

type walletResponse struct {
	ID       uuid.UUID    `json:"id"`
	PlayerID uuid.UUID    `json:"playerId"`
	Balance  domain.Money `json:"balance"`
	Version  int64        `json:"version"`
}

type walletLedgerEntryResponse struct {
	ID            uuid.UUID    `json:"id"`
	WalletID      uuid.UUID    `json:"walletId"`
	TransactionID uuid.UUID    `json:"transactionId"`
	Direction     string       `json:"direction"`
	Money         domain.Money `json:"money"`
	BalanceBefore domain.Money `json:"balanceBefore"`
	BalanceAfter  domain.Money `json:"balanceAfter"`
	CreatedAt     time.Time    `json:"createdAt"`
}

type walletLedgerResponse struct {
	Entries    []walletLedgerEntryResponse `json:"entries"`
	NextCursor string                      `json:"nextCursor,omitempty"`
}

type walletReconciliationResponse struct {
	WalletID          uuid.UUID    `json:"walletId"`
	StoredBalance     domain.Money `json:"storedBalance"`
	CalculatedBalance domain.Money `json:"calculatedBalance"`
	Difference        domain.Money `json:"difference"`
	Consistent        bool         `json:"consistent"`
	CheckedEntries    int64        `json:"checkedEntries"`
}

type ledgerCursor struct {
	CreatedAt time.Time `json:"createdAt"`
	ID        uuid.UUID `json:"id"`
}

// WalletHandler handles wallet HTTP operations.
type WalletHandler struct {
	service walletService
	logger  *slog.Logger
}

type walletService interface {
	Create(ctx context.Context, command application.CreateWalletCommand, metadata application.CommandMetadata) (*application.CreateWalletResult, error)
	Get(ctx context.Context, walletID string) (*application.WalletResult, error)
	ListLedger(ctx context.Context, walletID string, beforeCreatedAt *time.Time, beforeID *uuid.UUID, limit int) (*application.WalletLedgerResult, error)
	Reconcile(ctx context.Context, walletID string) (*application.WalletReconciliationResult, error)
}

// NewWalletHandler creates a wallet HTTP handler.
func NewWalletHandler(service *application.WalletService, logger *slog.Logger) *WalletHandler {
	return &WalletHandler{
		service: service,
		logger:  logger,
	}
}

// Create creates a wallet.
func (h *WalletHandler) Create(c fiber.Ctx) error {
	var request createWalletRequest

	if err := c.Bind().Body(&request); err != nil {
		return BuildErrorResponse(c, h.logger, ResolveError(fiber.ErrBadRequest))
	}

	if request.PlayerID == "" ||
		request.InitialBalance.Amount == "" ||
		request.InitialBalance.Currency == "" {
		return BuildErrorResponse(c, h.logger, ResolveError(fiber.ErrBadRequest))
	}

	result, err := h.service.Create(c.Context(),
		application.CreateWalletCommand{
			PlayerID:       request.PlayerID,
			Currency:       request.InitialBalance.Currency,
			InitialBalance: request.InitialBalance.Amount,
		},
		application.CommandMetadata{
			CorrelationID: uuid.NewString(),
		},
	)
	if err != nil {
		return BuildErrorResponse(c, h.logger, ResolveError(err))
	}

	return BuildSuccessResponse(c, fiber.StatusCreated, createWalletResponse{
		ID:       result.WalletID,
		PlayerID: result.PlayerID,
		Balance:  result.Balance,
		Version:  result.Version,
	})
}

// Get returns a wallet by identifier.
func (h *WalletHandler) Get(c fiber.Ctx) error {
	result, err := h.service.Get(c.Context(), c.Params("walletId"))
	if err != nil {
		return err
	}

	return BuildSuccessResponse(c, fiber.StatusOK, walletResponse{
		ID:       result.WalletID,
		PlayerID: result.PlayerID,
		Balance:  result.Balance,
		Version:  result.Version,
	})
}

// Ledger returns paginated wallet ledger entries.
func (h *WalletHandler) Ledger(c fiber.Ctx) error {
	limit := defaultLedgerLimit

	if rawLimit := c.Query("limit"); rawLimit != "" {
		parsedLimit, err := strconv.Atoi(rawLimit)
		if err != nil || parsedLimit < 1 || parsedLimit > maxLedgerLimit {
			return fiber.ErrBadRequest
		}

		limit = parsedLimit
	}

	beforeCreatedAt, beforeID, err := decodeLedgerCursor(c.Query("cursor"))
	if err != nil {
		return fiber.ErrBadRequest
	}

	result, err := h.service.ListLedger(c.Context(), c.Params("walletId"), beforeCreatedAt, beforeID, limit+1)
	if err != nil {
		return err
	}

	hasMore := len(result.Entries) > limit
	entries := result.Entries

	if hasMore {
		entries = entries[:limit]
	}

	responseEntries := make([]walletLedgerEntryResponse, 0, len(entries))

	for _, entry := range entries {
		responseEntries = append(
			responseEntries,
			walletLedgerEntryResponse{
				ID:            entry.ID(),
				WalletID:      entry.WalletID(),
				TransactionID: entry.TransactionID(),
				Direction:     entry.Direction().String(),
				Money:         entry.Amount(),
				BalanceBefore: entry.BalanceBefore(),
				BalanceAfter:  entry.BalanceAfter(),
				CreatedAt:     entry.CreatedAt(),
			},
		)
	}

	response := walletLedgerResponse{
		Entries: responseEntries,
	}

	if hasMore {
		nextCursor, err := encodeLedgerCursor(
			entries[len(entries)-1],
		)
		if err != nil {
			return err
		}

		response.NextCursor = nextCursor
	}

	return BuildSuccessResponse(c, fiber.StatusOK, response)
}

// Reconcile compares the stored wallet balance with its ledger.
func (h *WalletHandler) Reconcile(c fiber.Ctx) error {
	result, err := h.service.Reconcile(c.Context(), c.Params("walletId"))
	if err != nil {
		return err
	}

	if !result.Consistent {
		h.logger.WarnContext(c.Context(), "wallet reconciliation divergence detected", slog.String("walletId", result.WalletID.String()))
	}

	return BuildSuccessResponse(c, fiber.StatusOK, walletReconciliationResponse{
		WalletID:          result.WalletID,
		StoredBalance:     result.StoredBalance,
		CalculatedBalance: result.CalculatedBalance,
		Difference:        result.Difference,
		Consistent:        result.Consistent,
		CheckedEntries:    result.CheckedEntries,
	})
}

// encodeLedgerCursor creates an opaque ledger pagination cursor.
func encodeLedgerCursor(entry *domain.WalletLedgerEntry) (string, error) {
	raw, err := json.Marshal(ledgerCursor{
		CreatedAt: entry.CreatedAt().UTC(),
		ID:        entry.ID(),
	})
	if err != nil {
		return "", err
	}

	return base64.RawURLEncoding.EncodeToString(raw), nil
}

// decodeLedgerCursor parses an opaque ledger pagination cursor.
func decodeLedgerCursor(value string) (*time.Time, *uuid.UUID, error) {
	if value == "" {
		return nil, nil, nil
	}

	raw, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return nil, nil, err
	}

	var cursor ledgerCursor

	if err := json.Unmarshal(raw, &cursor); err != nil {
		return nil, nil, err
	}

	if cursor.CreatedAt.IsZero() || cursor.ID == uuid.Nil {
		return nil, nil, errors.New("invalid ledger cursor")
	}

	createdAt := cursor.CreatedAt
	id := cursor.ID

	return &createdAt, &id, nil
}
