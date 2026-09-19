package sqs

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/pablo-banker/junglegaming-test/internal/application"
	"github.com/pablo-banker/junglegaming-test/internal/config"
	"github.com/pablo-banker/junglegaming-test/internal/observability"
)

const wagerConsumerName = "wager-transactions"

type wagerProcessor interface {
	Process(ctx context.Context, command application.ProcessWagerCommand, metadata application.CommandMetadata) (*application.ProcessWagerResult, error)
}

type WagerMessageHandler struct {
	txManager        application.TransactionManager
	inbox            application.InboxRepository
	service          wagerProcessor
	clock            application.Clock
	allowedProviders map[string]bool
	logger           *slog.Logger
	metrics          *observability.Metrics
}

// NewWagerMessageHandler creates the SQS wager message handler.
func NewWagerMessageHandler(
	txManager application.TransactionManager,
	inbox application.InboxRepository,
	service *application.WagerService,
	clock application.Clock,
	cfg config.Config,
	logger *slog.Logger,
	metrics *observability.Metrics,
) *WagerMessageHandler {
	allowedProviders := make(map[string]bool, len(cfg.SQSAllowedProviders))
	for _, provider := range cfg.SQSAllowedProviders {
		allowedProviders[provider] = true
	}

	return &WagerMessageHandler{
		txManager:        txManager,
		inbox:            inbox,
		service:          service,
		clock:            clock,
		allowedProviders: allowedProviders,
		logger:           logger,
		metrics:          metrics,
	}
}

// Handle processes one SQS wager message transactionally. The inbox registration, the
// financial changes, the outbox events and the inbox completion share one transaction.
func (h *WagerMessageHandler) Handle(ctx context.Context, transportMessageID string, body string) error {
	if strings.TrimSpace(transportMessageID) == "" {
		return ErrInvalidMessageID
	}

	message, err := parseWagerMessage(body)
	if err != nil {
		return err
	}

	ctx = observability.WithAttrs(
		ctx,
		slog.String("messageId", message.MessageID),
		slog.String("correlationId", message.MessageID),
		slog.String("providerId", message.Data.ProviderID),
		slog.String("walletId", message.Data.WalletID),
	)

	// Unlike HTTP, the provider comes from the message body: only trusted providers pass.
	if !h.allowedProviders[message.Data.ProviderID] {
		h.logger.WarnContext(ctx, "wager message from a provider that is not allowed")

		return ErrProviderNotAllowed
	}

	command := wagerMessageToCommand(message.Data)
	messageHash := hashMessage(body)
	start := time.Now()

	var (
		result    *application.ProcessWagerResult
		duplicate bool
	)

	err = h.txManager.WithinTransaction(ctx, func(txCtx context.Context) error {
		result, duplicate = nil, false

		completed, err := h.inbox.Register(
			txCtx,
			wagerConsumerName,
			message.MessageID,
			messageHash,
		)
		if err != nil {
			return err
		}

		if completed {
			duplicate = true

			return nil
		}

		result, err = h.service.Process(txCtx, command, application.CommandMetadata{
			CorrelationID: message.MessageID,
			CausationID:   message.MessageID,
		})
		if err != nil {
			return err
		}

		now, err := h.clock.Now(txCtx)
		if err != nil {
			return err
		}

		return h.inbox.Complete(
			txCtx,
			wagerConsumerName,
			message.MessageID,
			now,
		)
	})
	if err != nil {
		h.countConflict(err)

		return err
	}

	if duplicate {
		h.metrics.CountReplay("sqs_inbox")
		h.logger.InfoContext(ctx, "duplicate wager message ignored by inbox")

		return nil
	}

	h.metrics.ObserveWager("sqs", command.Type, string(result.Status), result.IdempotentReplay, time.Since(start))
	h.logger.InfoContext(
		ctx,
		"wager message processed",
		slog.String("transactionId", result.TransactionID.String()),
		slog.String("kind", command.Type),
		slog.String("status", string(result.Status)),
		slog.String("failureCode", result.FailureCode),
		slog.Bool("idempotentReplay", result.IdempotentReplay),
	)

	return nil
}

// countConflict records idempotency conflicts detected for a message.
func (h *WagerMessageHandler) countConflict(err error) {
	switch {
	case errors.Is(err, application.ErrIdempotencyConflict):
		h.metrics.CountConflict("idempotency_key")

	case errors.Is(err, application.ErrExternalTransactionConflict):
		h.metrics.CountConflict("external_transaction")
	}
}

// wagerMessageToCommand converts the SQS contract into the shared application command.
func wagerMessageToCommand(data WagerMessageData) application.ProcessWagerCommand {
	referenceExternalTransactionID := ""

	if data.ReferenceExternalTransactionID != nil {
		referenceExternalTransactionID = *data.ReferenceExternalTransactionID
	}

	return application.ProcessWagerCommand{
		ProviderID:                     data.ProviderID,
		ExternalTransactionID:          data.ExternalTransactionID,
		IdempotencyKey:                 data.IdempotencyKey,
		WalletID:                       data.WalletID,
		PlayerID:                       data.PlayerID,
		RoundID:                        data.RoundID,
		GameID:                         data.GameID,
		Type:                           data.Kind,
		Amount:                         data.Money.Amount,
		Currency:                       data.Money.Currency,
		ReferenceExternalTransactionID: referenceExternalTransactionID,
	}
}

// hashMessage calculates the persistent message payload hash.
func hashMessage(body string) string {
	sum := sha256.Sum256([]byte(body))

	return hex.EncodeToString(sum[:])
}
