package application

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"

	"github.com/google/uuid"

	"github.com/pablo-banker/junglegaming-test/internal/domain"
)

// wagerRequest is a validated ProcessWagerCommand shared by the HTTP and SQS entry points.
type wagerRequest struct {
	command         ProcessWagerCommand
	walletID        uuid.UUID
	playerID        uuid.UUID
	transactionType domain.WagerTransactionType
	amount          domain.Money
	payloadHash     string
}

// parseWagerRequest validates the command fields that do not depend on persisted state.
func parseWagerRequest(command ProcessWagerCommand) (wagerRequest, error) {
	walletID, err := uuid.Parse(command.WalletID)
	if err != nil || walletID == uuid.Nil {
		return wagerRequest{}, domain.ErrInvalidWalletID
	}

	playerID, err := uuid.Parse(command.PlayerID)
	if err != nil || playerID == uuid.Nil {
		return wagerRequest{}, domain.ErrInvalidPlayerID
	}

	currency, err := domain.NewCurrency(command.Currency)
	if err != nil {
		return wagerRequest{}, err
	}

	amount, err := domain.ParseMoney(command.Amount, currency)
	if err != nil {
		return wagerRequest{}, err
	}

	transactionType := domain.WagerTransactionType(command.Type)
	if !transactionType.IsExternal() {
		return wagerRequest{}, domain.ErrInvalidWagerType
	}

	request := wagerRequest{
		command:         command,
		walletID:        walletID,
		playerID:        playerID,
		transactionType: transactionType,
		amount:          amount,
	}

	request.payloadHash, err = request.hash()
	if err != nil {
		return wagerRequest{}, err
	}

	return request, nil
}

// newTransaction creates the pending domain transaction for the request.
func (r wagerRequest) newTransaction(createdAt time.Time) (*domain.WagerTransaction, error) {
	return domain.NewExternalWagerTransaction(
		domain.NewExternalWagerTransactionParams{
			ID:                             uuid.New(),
			ProviderID:                     r.command.ProviderID,
			ExternalTransactionID:          r.command.ExternalTransactionID,
			IdempotencyKey:                 r.command.IdempotencyKey,
			PayloadHash:                    r.payloadHash,
			WalletID:                       r.walletID,
			PlayerID:                       r.playerID,
			RoundID:                        r.command.RoundID,
			GameID:                         r.command.GameID,
			Type:                           r.transactionType,
			Amount:                         r.amount,
			ReferenceExternalTransactionID: r.command.ReferenceExternalTransactionID,
			CreatedAt:                      createdAt,
		},
	)
}

// hash returns the SHA-256 (hex) of the canonical JSON of the business fields.
//
// Canonical form: object keys sorted lexicographically at every level, no insignificant
// whitespace, no HTML escaping, UTF-8. Fields: externalTransactionId, gameId, kind,
// money{amount,currency}, playerId, providerId, referenceExternalTransactionId (only when
// present), roundId and walletId. The idempotency key and transport metadata are excluded.
// Normalization: UUIDs use their lowercase canonical form; money is already canonical
// because only fixed two-decimal amounts are accepted; other strings are used as received.
// HTTP and SQS build the same ProcessWagerCommand, so equal operations hash equally.
func (r wagerRequest) hash() (string, error) {
	fields := map[string]any{
		"externalTransactionId": r.command.ExternalTransactionID,
		"gameId":                r.command.GameID,
		"kind":                  string(r.transactionType),
		"money": map[string]string{
			"amount":   r.amount.Amount(),
			"currency": r.amount.Currency().Code(),
		},
		"playerId":   r.playerID.String(),
		"providerId": r.command.ProviderID,
		"roundId":    r.command.RoundID,
		"walletId":   r.walletID.String(),
	}

	if r.command.ReferenceExternalTransactionID != "" {
		fields["referenceExternalTransactionId"] = r.command.ReferenceExternalTransactionID
	}

	var canonical bytes.Buffer

	encoder := json.NewEncoder(&canonical)
	encoder.SetEscapeHTML(false)

	// encoding/json writes map keys in sorted order.
	if err := encoder.Encode(fields); err != nil {
		return "", err
	}

	sum := sha256.Sum256(bytes.TrimSuffix(canonical.Bytes(), []byte("\n")))

	return hex.EncodeToString(sum[:]), nil
}
