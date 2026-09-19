package sqs

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"

	"github.com/pablo-banker/junglegaming-test/internal/application"
)

const wagerConsumerName = "wager-transactions"

type wagerProcessor interface {
	Process(ctx context.Context, command application.ProcessWagerCommand, metadata application.CommandMetadata) (*application.ProcessWagerResult, error)
}

type WagerMessageHandler struct {
	txManager application.TransactionManager
	inbox     application.InboxRepository
	service   wagerProcessor
	clock     application.Clock
}

// NewWagerMessageHandler creates the SQS wager message handler.
func NewWagerMessageHandler(txManager application.TransactionManager, inbox application.InboxRepository, service *application.WagerService, clock application.Clock) *WagerMessageHandler {
	return &WagerMessageHandler{
		txManager: txManager,
		inbox:     inbox,
		service:   service,
		clock:     clock,
	}
}

// Handle processes one SQS wager message transactionally.
func (h *WagerMessageHandler) Handle(ctx context.Context, transportMessageID string, body string) error {
	if strings.TrimSpace(transportMessageID) == "" {
		return ErrInvalidMessageID
	}

	message, err := parseWagerMessage(body)
	if err != nil {
		return err
	}

	command := wagerMessageToCommand(message.Data)
	messageHash := hashMessage(body)

	return h.txManager.WithinTransaction(ctx, func(txCtx context.Context) error {
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
			return nil
		}

		_, err = h.service.Process(txCtx, command, application.CommandMetadata{
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
