package domain

import (
	"strings"
	"time"

	"github.com/google/uuid"
)

type WagerTransactionType string

const (
	WagerTransactionTypeOpening  WagerTransactionType = "OPENING"
	WagerTransactionTypeBet      WagerTransactionType = "BET"
	WagerTransactionTypeWin      WagerTransactionType = "WIN"
	WagerTransactionTypeLoss     WagerTransactionType = "LOSS"
	WagerTransactionTypeRefund   WagerTransactionType = "REFUND"
	WagerTransactionTypeRollback WagerTransactionType = "ROLLBACK"
)

type WagerTransactionStatus string

// maxIdentifierLength bounds provider supplied identifiers such as external ids, idempotency
// keys, rounds and games. It keeps them far below the PostgreSQL index entry limit.
const maxIdentifierLength = 255

const (
	WagerTransactionStatusPending          WagerTransactionStatus = "PENDING"
	WagerTransactionStatusPendingReference WagerTransactionStatus = "PENDING_REFERENCE"
	WagerTransactionStatusProcessed        WagerTransactionStatus = "PROCESSED"
	WagerTransactionStatusRejected         WagerTransactionStatus = "REJECTED"
	WagerTransactionStatusFailed           WagerTransactionStatus = "FAILED"
)

type NewExternalWagerTransactionParams struct {
	ID                             uuid.UUID
	ProviderID                     string
	ExternalTransactionID          string
	IdempotencyKey                 string
	PayloadHash                    string
	WalletID                       uuid.UUID
	PlayerID                       uuid.UUID
	RoundID                        string
	GameID                         string
	Type                           WagerTransactionType
	Amount                         Money
	ReferenceExternalTransactionID string
	CreatedAt                      time.Time
}

type NewOpeningWagerTransactionParams struct {
	ID        uuid.UUID
	WalletID  uuid.UUID
	PlayerID  uuid.UUID
	Amount    Money
	CreatedAt time.Time
}

type RehydrateWagerTransactionParams struct {
	ID                             uuid.UUID
	ProviderID                     string
	ExternalTransactionID          string
	IdempotencyKey                 string
	PayloadHash                    string
	WalletID                       uuid.UUID
	PlayerID                       uuid.UUID
	RoundID                        string
	GameID                         string
	Type                           WagerTransactionType
	Amount                         Money
	ReferenceExternalTransactionID string
	ReferenceTransactionID         uuid.UUID
	ReferenceTransactionType       WagerTransactionType
	Status                         WagerTransactionStatus
	FailureCode                    string
	FailureMessage                 string
	BalanceBefore                  *Money
	BalanceAfter                   *Money
	CreatedAt                      time.Time
	UpdatedAt                      time.Time
	CompletedAt                    *time.Time
}

type WagerTransaction struct {
	id                             uuid.UUID
	providerID                     string
	externalTransactionID          string
	idempotencyKey                 string
	payloadHash                    string
	walletID                       uuid.UUID
	playerID                       uuid.UUID
	roundID                        string
	gameID                         string
	transactionType                WagerTransactionType
	amount                         Money
	referenceExternalTransactionID string
	referenceTransactionID         uuid.UUID
	referenceTransactionType       WagerTransactionType
	status                         WagerTransactionStatus
	failureCode                    string
	failureMessage                 string
	balanceBefore                  *Money
	balanceAfter                   *Money
	createdAt                      time.Time
	updatedAt                      time.Time
	completedAt                    *time.Time
}

// NewExternalWagerTransaction creates a pending provider transaction.
func NewExternalWagerTransaction(params NewExternalWagerTransactionParams) (*WagerTransaction, error) {
	if err := validateExternalWager(params); err != nil {
		return nil, err
	}

	createdAt := params.CreatedAt.UTC()

	return &WagerTransaction{
		id:                             params.ID,
		providerID:                     params.ProviderID,
		externalTransactionID:          params.ExternalTransactionID,
		idempotencyKey:                 params.IdempotencyKey,
		payloadHash:                    params.PayloadHash,
		walletID:                       params.WalletID,
		playerID:                       params.PlayerID,
		roundID:                        params.RoundID,
		gameID:                         params.GameID,
		transactionType:                params.Type,
		amount:                         params.Amount,
		referenceExternalTransactionID: params.ReferenceExternalTransactionID,
		status:                         WagerTransactionStatusPending,
		createdAt:                      createdAt,
		updatedAt:                      createdAt,
	}, nil
}

// NewOpeningWagerTransaction creates a processed internal opening transaction.
func NewOpeningWagerTransaction(params NewOpeningWagerTransactionParams) (*WagerTransaction, error) {
	if params.ID == uuid.Nil {
		return nil, ErrInvalidTransactionID
	}

	if params.WalletID == uuid.Nil {
		return nil, ErrInvalidWalletID
	}

	if params.PlayerID == uuid.Nil {
		return nil, ErrInvalidPlayerID
	}

	if !params.Amount.IsValid() {
		return nil, ErrInvalidMoney
	}

	if !params.Amount.IsPositive() {
		return nil, ErrAmountMustBePositive
	}

	if params.CreatedAt.IsZero() {
		return nil, ErrInvalidTimestamp
	}

	zero, err := Zero(params.Amount.Currency())
	if err != nil {
		return nil, err
	}

	createdAt := params.CreatedAt.UTC()
	after := params.Amount
	completedAt := createdAt

	return &WagerTransaction{
		id:              params.ID,
		walletID:        params.WalletID,
		playerID:        params.PlayerID,
		transactionType: WagerTransactionTypeOpening,
		amount:          params.Amount,
		status:          WagerTransactionStatusProcessed,
		balanceBefore:   &zero,
		balanceAfter:    &after,
		createdAt:       createdAt,
		updatedAt:       createdAt,
		completedAt:     &completedAt,
	}, nil
}

// RehydrateWagerTransaction rebuilds a persisted transaction without applying business actions.
func RehydrateWagerTransaction(params RehydrateWagerTransactionParams) (*WagerTransaction, error) {
	if err := validateRehydratedWager(params); err != nil {
		return nil, err
	}

	transaction := &WagerTransaction{
		id:                             params.ID,
		providerID:                     params.ProviderID,
		externalTransactionID:          params.ExternalTransactionID,
		idempotencyKey:                 params.IdempotencyKey,
		payloadHash:                    params.PayloadHash,
		walletID:                       params.WalletID,
		playerID:                       params.PlayerID,
		roundID:                        params.RoundID,
		gameID:                         params.GameID,
		transactionType:                params.Type,
		amount:                         params.Amount,
		referenceExternalTransactionID: params.ReferenceExternalTransactionID,
		referenceTransactionID:         params.ReferenceTransactionID,
		referenceTransactionType:       params.ReferenceTransactionType,
		status:                         params.Status,
		failureCode:                    params.FailureCode,
		failureMessage:                 params.FailureMessage,
		createdAt:                      params.CreatedAt.UTC(),
		updatedAt:                      params.UpdatedAt.UTC(),
	}

	if params.BalanceBefore != nil {
		before := *params.BalanceBefore
		transaction.balanceBefore = &before
	}

	if params.BalanceAfter != nil {
		after := *params.BalanceAfter
		transaction.balanceAfter = &after
	}

	if params.CompletedAt != nil {
		completedAt := params.CompletedAt.UTC()
		transaction.completedAt = &completedAt
	}

	return transaction, nil
}

// MarkPendingReference moves the transaction into reference waiting state.
func (w *WagerTransaction) MarkPendingReference(updatedAt time.Time) error {
	if w.status.IsTerminal() {
		return ErrWagerTransactionTerminal
	}

	if w.status != WagerTransactionStatusPending {
		return ErrInvalidWagerTransition
	}

	if !w.transactionType.AllowsReference() {
		return ErrInvalidWagerReference
	}

	if strings.TrimSpace(w.referenceExternalTransactionID) == "" {
		return ErrInvalidWagerReference
	}

	if w.referenceTransactionID != uuid.Nil {
		return ErrInvalidWagerReference
	}

	return w.transitionTo(WagerTransactionStatusPendingReference, updatedAt)
}

// ResolveReference validates and stores a referenced processed transaction.
func (w *WagerTransaction) ResolveReference(reference *WagerTransaction, updatedAt time.Time) error {
	if w.status.IsTerminal() {
		return ErrWagerTransactionTerminal
	}

	if w.status != WagerTransactionStatusPending &&
		w.status != WagerTransactionStatusPendingReference {
		return ErrInvalidWagerTransition
	}

	if err := w.validateReference(reference); err != nil {
		return err
	}

	if w.referenceTransactionID != uuid.Nil {
		if w.referenceTransactionID != reference.id ||
			w.referenceTransactionType != reference.transactionType {
			return ErrWagerReferenceMismatch
		}

		return nil
	}

	if err := w.validateTimestamp(updatedAt); err != nil {
		return err
	}

	if w.status == WagerTransactionStatusPendingReference {
		w.status = WagerTransactionStatusPending
	}

	w.referenceTransactionID = reference.id
	w.referenceTransactionType = reference.transactionType
	w.updatedAt = updatedAt.UTC()

	return nil
}

// MovementDirection returns the wallet movement required by the transaction.
func (w *WagerTransaction) MovementDirection() (WalletLedgerDirection, bool, error) {
	switch w.transactionType {
	case WagerTransactionTypeBet:
		return WalletLedgerDirectionDebit, true, nil

	case WagerTransactionTypeWin,
		WagerTransactionTypeRefund:
		return WalletLedgerDirectionCredit, true, nil

	case WagerTransactionTypeLoss:
		return "", false, nil

	case WagerTransactionTypeRollback:
		switch w.referenceTransactionType {
		case WagerTransactionTypeBet:
			return WalletLedgerDirectionCredit, true, nil

		case WagerTransactionTypeWin,
			WagerTransactionTypeRefund:
			return WalletLedgerDirectionDebit, true, nil

		default:
			return "", false, ErrInvalidWagerReference
		}

	default:
		return "", false, ErrInvalidWagerType
	}
}

// MarkProcessed completes a successfully processed external transaction.
func (w *WagerTransaction) MarkProcessed(balanceBefore Money, balanceAfter Money, completedAt time.Time) error {
	if w.status.IsTerminal() {
		return ErrWagerTransactionTerminal
	}

	if w.status != WagerTransactionStatusPending {
		return ErrInvalidWagerTransition
	}

	if err := w.validateReferenceReady(); err != nil {
		return err
	}

	if err := w.validateObservedBalances(balanceBefore, balanceAfter); err != nil {
		return err
	}

	if err := w.transitionTo(WagerTransactionStatusProcessed, completedAt); err != nil {
		return err
	}

	before := balanceBefore
	after := balanceAfter
	completed := completedAt.UTC()

	w.balanceBefore = &before
	w.balanceAfter = &after
	w.completedAt = &completed

	return nil
}

// MarkRejected completes a transaction rejected by a business rule.
func (w *WagerTransaction) MarkRejected(failureCode string, failureMessage string, completedAt time.Time) error {
	return w.completeWithFailure(
		WagerTransactionStatusRejected,
		failureCode,
		failureMessage,
		completedAt,
	)
}

// MarkFailed completes a transaction with a permanent infrastructure failure.
func (w *WagerTransaction) MarkFailed(failureCode string, failureMessage string, completedAt time.Time) error {
	return w.completeWithFailure(
		WagerTransactionStatusFailed,
		failureCode,
		failureMessage,
		completedAt,
	)
}

// completeWithFailure completes an external transaction with a terminal failure.
func (w *WagerTransaction) completeWithFailure(status WagerTransactionStatus, failureCode string, failureMessage string, completedAt time.Time) error {
	if w.status.IsTerminal() {
		return ErrWagerTransactionTerminal
	}

	if w.transactionType == WagerTransactionTypeOpening {
		return ErrInvalidWagerTransition
	}

	if strings.TrimSpace(failureCode) == "" {
		return ErrInvalidFailureCode
	}

	if err := w.transitionTo(status, completedAt); err != nil {
		return err
	}

	completed := completedAt.UTC()

	w.failureCode = failureCode
	w.failureMessage = failureMessage
	w.completedAt = &completed

	return nil
}

// validateReference validates a resolved reference against the transaction.
func (w *WagerTransaction) validateReference(reference *WagerTransaction) error {
	if reference == nil {
		return ErrInvalidWagerReference
	}

	if strings.TrimSpace(w.referenceExternalTransactionID) == "" {
		return ErrInvalidWagerReference
	}

	if reference.status != WagerTransactionStatusProcessed {
		return ErrWagerReferenceNotProcessed
	}

	if w.referenceExternalTransactionID != reference.externalTransactionID {
		return ErrWagerReferenceMismatch
	}

	if w.providerID != reference.providerID ||
		w.playerID != reference.playerID ||
		w.walletID != reference.walletID ||
		w.roundID != reference.roundID ||
		!w.amount.Currency().Equal(reference.amount.Currency()) {
		return ErrWagerReferenceMismatch
	}

	if !isAllowedReferenceType(w.transactionType, reference.transactionType) {
		return ErrInvalidWagerReference
	}

	if w.transactionType.RequiresReference() &&
		!w.amount.Equal(reference.amount) {
		return ErrWagerReferenceMismatch
	}

	return nil
}

// validateReferenceReady verifies required references are already resolved.
func (w *WagerTransaction) validateReferenceReady() error {
	hasExternalReference := strings.TrimSpace(w.referenceExternalTransactionID) != ""
	hasInternalReference := w.referenceTransactionID != uuid.Nil

	if w.transactionType.RequiresReference() && !hasExternalReference {
		return ErrInvalidWagerReference
	}

	if hasExternalReference && !hasInternalReference {
		return ErrInvalidWagerReference
	}

	if hasInternalReference &&
		!isAllowedReferenceType(w.transactionType, w.referenceTransactionType) {
		return ErrInvalidWagerReference
	}

	return nil
}

// validateObservedBalances validates the persisted result used for replay.
func (w *WagerTransaction) validateObservedBalances(balanceBefore Money, balanceAfter Money) error {
	if !balanceBefore.IsValid() || !balanceAfter.IsValid() {
		return ErrInvalidMoney
	}

	if balanceBefore.IsNegative() || balanceAfter.IsNegative() {
		return ErrInvalidFinancialResult
	}

	if !w.amount.Currency().Equal(balanceBefore.Currency()) ||
		!w.amount.Currency().Equal(balanceAfter.Currency()) {
		return ErrCurrencyMismatch
	}

	if w.transactionType == WagerTransactionTypeLoss &&
		!balanceBefore.Equal(balanceAfter) {
		return ErrInvalidFinancialResult
	}

	return nil
}

// transitionTo validates and applies a transaction state transition.
func (w *WagerTransaction) transitionTo(status WagerTransactionStatus, updatedAt time.Time) error {
	if w.status.IsTerminal() {
		return ErrWagerTransactionTerminal
	}

	if !status.IsValid() {
		return ErrInvalidWagerStatus
	}

	if err := w.validateTimestamp(updatedAt); err != nil {
		return err
	}

	if !isAllowedWagerTransition(w.status, status) {
		return ErrInvalidWagerTransition
	}

	w.status = status
	w.updatedAt = updatedAt.UTC()

	return nil
}

// validateTimestamp prevents transaction time from moving backwards.
func (w *WagerTransaction) validateTimestamp(updatedAt time.Time) error {
	if updatedAt.IsZero() || updatedAt.Before(w.updatedAt) {
		return ErrInvalidTimestamp
	}

	return nil
}

// validateExternalWager validates a new provider transaction.
func validateExternalWager(params NewExternalWagerTransactionParams) error {
	if params.ID == uuid.Nil {
		return ErrInvalidTransactionID
	}

	if !isValidIdentifier(params.ProviderID) {
		return ErrInvalidProviderID
	}

	if !isValidIdentifier(params.ExternalTransactionID) {
		return ErrInvalidExternalTransactionID
	}

	if !isValidIdentifier(params.IdempotencyKey) {
		return ErrInvalidIdempotencyKey
	}

	if strings.TrimSpace(params.PayloadHash) == "" {
		return ErrInvalidPayloadHash
	}

	if params.WalletID == uuid.Nil {
		return ErrInvalidWalletID
	}

	if params.PlayerID == uuid.Nil {
		return ErrInvalidPlayerID
	}

	if !isValidIdentifier(params.RoundID) {
		return ErrInvalidRoundID
	}

	if !isValidIdentifier(params.GameID) {
		return ErrInvalidGameID
	}

	if !params.Type.IsExternal() {
		return ErrInvalidWagerType
	}

	if !params.Amount.IsValid() {
		return ErrInvalidMoney
	}

	if err := validateWagerAmount(params.Type, params.Amount); err != nil {
		return err
	}

	if err := validateExternalReference(
		params.Type,
		params.ReferenceExternalTransactionID,
	); err != nil {
		return err
	}

	if len(params.ReferenceExternalTransactionID) > maxIdentifierLength ||
		params.ReferenceExternalTransactionID == params.ExternalTransactionID {
		return ErrInvalidWagerReference
	}

	if params.CreatedAt.IsZero() {
		return ErrInvalidTimestamp
	}

	return nil
}

// validateRehydratedWager validates persisted transaction state.
func validateRehydratedWager(params RehydrateWagerTransactionParams) error {
	if params.ID == uuid.Nil {
		return ErrInvalidTransactionID
	}

	if params.WalletID == uuid.Nil {
		return ErrInvalidWalletID
	}

	if params.PlayerID == uuid.Nil {
		return ErrInvalidPlayerID
	}

	if !params.Type.IsValid() {
		return ErrInvalidWagerType
	}

	if !params.Status.IsValid() {
		return ErrInvalidWagerStatus
	}

	if !params.Amount.IsValid() {
		return ErrInvalidMoney
	}

	if params.CreatedAt.IsZero() ||
		params.UpdatedAt.IsZero() ||
		params.UpdatedAt.Before(params.CreatedAt) {
		return ErrInvalidTimestamp
	}

	if params.CompletedAt != nil {
		if params.CompletedAt.IsZero() ||
			params.CompletedAt.Before(params.CreatedAt) ||
			params.CompletedAt.After(params.UpdatedAt) {
			return ErrInvalidTimestamp
		}
	}

	if params.Type == WagerTransactionTypeOpening {
		return validateRehydratedOpening(params)
	}

	return validateRehydratedExternal(params)
}

// validateRehydratedOpening validates persisted internal opening state.
func validateRehydratedOpening(params RehydrateWagerTransactionParams) error {
	if strings.TrimSpace(params.ProviderID) != "" ||
		strings.TrimSpace(params.ExternalTransactionID) != "" ||
		strings.TrimSpace(params.IdempotencyKey) != "" ||
		strings.TrimSpace(params.PayloadHash) != "" ||
		strings.TrimSpace(params.RoundID) != "" ||
		strings.TrimSpace(params.GameID) != "" ||
		strings.TrimSpace(params.ReferenceExternalTransactionID) != "" ||
		params.ReferenceTransactionID != uuid.Nil ||
		params.ReferenceTransactionType != "" {
		return ErrInvalidWagerState
	}

	if !params.Amount.IsPositive() {
		return ErrAmountMustBePositive
	}

	if params.Status != WagerTransactionStatusProcessed ||
		params.BalanceBefore == nil ||
		params.BalanceAfter == nil ||
		params.CompletedAt == nil ||
		params.FailureCode != "" ||
		params.FailureMessage != "" {
		return ErrInvalidWagerState
	}

	zero, err := Zero(params.Amount.Currency())
	if err != nil {
		return err
	}

	if !params.BalanceBefore.Equal(zero) ||
		!params.BalanceAfter.Equal(params.Amount) {
		return ErrInvalidFinancialResult
	}

	return nil
}

// validateRehydratedExternal validates persisted provider transaction state.
func validateRehydratedExternal(params RehydrateWagerTransactionParams) error {
	if strings.TrimSpace(params.ProviderID) == "" {
		return ErrInvalidProviderID
	}

	if strings.TrimSpace(params.ExternalTransactionID) == "" {
		return ErrInvalidExternalTransactionID
	}

	if strings.TrimSpace(params.IdempotencyKey) == "" {
		return ErrInvalidIdempotencyKey
	}

	if strings.TrimSpace(params.PayloadHash) == "" {
		return ErrInvalidPayloadHash
	}

	if strings.TrimSpace(params.RoundID) == "" {
		return ErrInvalidRoundID
	}

	if strings.TrimSpace(params.GameID) == "" {
		return ErrInvalidGameID
	}

	if !params.Type.IsExternal() {
		return ErrInvalidWagerType
	}

	if err := validateWagerAmount(params.Type, params.Amount); err != nil {
		return err
	}

	if err := validateExternalReference(
		params.Type,
		params.ReferenceExternalTransactionID,
	); err != nil {
		return err
	}

	if err := validatePersistedReference(params); err != nil {
		return err
	}

	switch params.Status {
	case WagerTransactionStatusPending:
		return validatePendingState(params)

	case WagerTransactionStatusPendingReference:
		return validatePendingReferenceState(params)

	case WagerTransactionStatusProcessed:
		return validateProcessedState(params)

	case WagerTransactionStatusRejected,
		WagerTransactionStatusFailed:
		return validateFailureState(params)

	default:
		return ErrInvalidWagerStatus
	}
}

// validatePendingState validates a persisted pending transaction.
func validatePendingState(params RehydrateWagerTransactionParams) error {
	if params.CompletedAt != nil ||
		params.BalanceBefore != nil ||
		params.BalanceAfter != nil ||
		params.FailureCode != "" ||
		params.FailureMessage != "" {
		return ErrInvalidWagerState
	}

	return nil
}

// validatePendingReferenceState validates a persisted reference waiting state.
func validatePendingReferenceState(params RehydrateWagerTransactionParams) error {
	if strings.TrimSpace(params.ReferenceExternalTransactionID) == "" ||
		params.ReferenceTransactionID != uuid.Nil ||
		params.ReferenceTransactionType != "" {
		return ErrInvalidWagerState
	}

	return validatePendingState(params)
}

// validateProcessedState validates a persisted successful result.
func validateProcessedState(params RehydrateWagerTransactionParams) error {
	if params.CompletedAt == nil ||
		params.BalanceBefore == nil ||
		params.BalanceAfter == nil ||
		params.FailureCode != "" ||
		params.FailureMessage != "" {
		return ErrInvalidWagerState
	}

	hasExternalReference := strings.TrimSpace(params.ReferenceExternalTransactionID) != ""

	if hasExternalReference && params.ReferenceTransactionID == uuid.Nil {
		return ErrInvalidWagerReference
	}

	if params.BalanceBefore.IsNegative() ||
		params.BalanceAfter.IsNegative() {
		return ErrInvalidFinancialResult
	}

	if !params.Amount.Currency().Equal(params.BalanceBefore.Currency()) ||
		!params.Amount.Currency().Equal(params.BalanceAfter.Currency()) {
		return ErrCurrencyMismatch
	}

	if params.Type == WagerTransactionTypeLoss &&
		!params.BalanceBefore.Equal(*params.BalanceAfter) {
		return ErrInvalidFinancialResult
	}

	return nil
}

// validateFailureState validates a persisted terminal failure.
func validateFailureState(params RehydrateWagerTransactionParams) error {
	if strings.TrimSpace(params.FailureCode) == "" ||
		params.CompletedAt == nil ||
		params.BalanceBefore != nil ||
		params.BalanceAfter != nil {
		return ErrInvalidWagerState
	}

	return nil
}

// validatePersistedReference validates persisted reference fields.
func validatePersistedReference(params RehydrateWagerTransactionParams) error {
	hasExternalReference := strings.TrimSpace(params.ReferenceExternalTransactionID) != ""
	hasInternalReference := params.ReferenceTransactionID != uuid.Nil
	hasReferenceType := params.ReferenceTransactionType != ""

	if (hasInternalReference || hasReferenceType) && !hasExternalReference {
		return ErrInvalidWagerReference
	}

	if hasInternalReference != hasReferenceType {
		return ErrInvalidWagerReference
	}

	if hasReferenceType {
		if !params.ReferenceTransactionType.IsValid() {
			return ErrInvalidWagerReference
		}

		if !isAllowedReferenceType(
			params.Type,
			params.ReferenceTransactionType,
		) {
			return ErrInvalidWagerReference
		}
	}

	return nil
}

// validateWagerAmount validates transaction amount rules.
func validateWagerAmount(transactionType WagerTransactionType, amount Money) error {
	if transactionType == WagerTransactionTypeLoss {
		if !amount.IsZero() {
			return ErrInvalidWagerAmount
		}

		return nil
	}

	if !amount.IsPositive() {
		return ErrAmountMustBePositive
	}

	return nil
}

// validateExternalReference validates reference requirements by transaction type.
func validateExternalReference(transactionType WagerTransactionType, referenceExternalTransactionID string) error {
	hasReference := strings.TrimSpace(referenceExternalTransactionID) != ""

	if transactionType.RequiresReference() && !hasReference {
		return ErrInvalidWagerReference
	}

	if !transactionType.AllowsReference() && hasReference {
		return ErrInvalidWagerReference
	}

	return nil
}

// isAllowedReferenceType reports whether a reference type is valid for the transaction.
func isAllowedReferenceType(transactionType WagerTransactionType, referenceType WagerTransactionType) bool {
	switch transactionType {
	case WagerTransactionTypeWin,
		WagerTransactionTypeRefund:
		return referenceType == WagerTransactionTypeBet

	case WagerTransactionTypeRollback:
		return referenceType == WagerTransactionTypeBet ||
			referenceType == WagerTransactionTypeWin ||
			referenceType == WagerTransactionTypeRefund

	default:
		return false
	}
}

// isAllowedWagerTransition reports whether a state transition is valid.
func isAllowedWagerTransition(from WagerTransactionStatus, to WagerTransactionStatus) bool {
	switch from {
	case WagerTransactionStatusPending:
		return to == WagerTransactionStatusPendingReference ||
			to == WagerTransactionStatusProcessed ||
			to == WagerTransactionStatusRejected ||
			to == WagerTransactionStatusFailed

	case WagerTransactionStatusPendingReference:
		return to == WagerTransactionStatusPending ||
			to == WagerTransactionStatusRejected ||
			to == WagerTransactionStatusFailed

	default:
		return false
	}
}

// IsValid reports whether the transaction type is supported.
func (t WagerTransactionType) IsValid() bool {
	switch t {
	case WagerTransactionTypeOpening,
		WagerTransactionTypeBet,
		WagerTransactionTypeWin,
		WagerTransactionTypeLoss,
		WagerTransactionTypeRefund,
		WagerTransactionTypeRollback:
		return true

	default:
		return false
	}
}

// IsExternal reports whether the transaction type may come from a provider.
func (t WagerTransactionType) IsExternal() bool {
	return t.IsValid() && t != WagerTransactionTypeOpening
}

// RequiresReference reports whether the transaction requires a reference.
func (t WagerTransactionType) RequiresReference() bool {
	return t == WagerTransactionTypeRefund ||
		t == WagerTransactionTypeRollback
}

// AllowsReference reports whether the transaction may contain a reference.
func (t WagerTransactionType) AllowsReference() bool {
	return t == WagerTransactionTypeWin || t.RequiresReference()
}

// IsValid reports whether the transaction status is supported.
func (s WagerTransactionStatus) IsValid() bool {
	switch s {
	case WagerTransactionStatusPending,
		WagerTransactionStatusPendingReference,
		WagerTransactionStatusProcessed,
		WagerTransactionStatusRejected,
		WagerTransactionStatusFailed:
		return true

	default:
		return false
	}
}

// IsTerminal reports whether the transaction can no longer transition.
func (s WagerTransactionStatus) IsTerminal() bool {
	return s == WagerTransactionStatusProcessed ||
		s == WagerTransactionStatusRejected ||
		s == WagerTransactionStatusFailed
}

// ID returns the internal transaction identifier.
func (w *WagerTransaction) ID() uuid.UUID {
	return w.id
}

// ProviderID returns the provider identifier.
func (w *WagerTransaction) ProviderID() string {
	return w.providerID
}

// ExternalTransactionID returns the provider transaction identifier.
func (w *WagerTransaction) ExternalTransactionID() string {
	return w.externalTransactionID
}

// IdempotencyKey returns the persisted idempotency key.
func (w *WagerTransaction) IdempotencyKey() string {
	return w.idempotencyKey
}

// PayloadHash returns the canonical payload hash.
func (w *WagerTransaction) PayloadHash() string {
	return w.payloadHash
}

// WalletID returns the affected wallet identifier.
func (w *WagerTransaction) WalletID() uuid.UUID {
	return w.walletID
}

// PlayerID returns the player identifier.
func (w *WagerTransaction) PlayerID() uuid.UUID {
	return w.playerID
}

// RoundID returns the provider round identifier.
func (w *WagerTransaction) RoundID() string {
	return w.roundID
}

// GameID returns the game identifier.
func (w *WagerTransaction) GameID() string {
	return w.gameID
}

// Type returns the transaction type.
func (w *WagerTransaction) Type() WagerTransactionType {
	return w.transactionType
}

// Amount returns the transaction amount.
func (w *WagerTransaction) Amount() Money {
	return w.amount
}

// ReferenceExternalTransactionID returns the external reference identifier.
func (w *WagerTransaction) ReferenceExternalTransactionID() string {
	return w.referenceExternalTransactionID
}

// ReferenceTransactionID returns the resolved internal reference identifier.
func (w *WagerTransaction) ReferenceTransactionID() uuid.UUID {
	return w.referenceTransactionID
}

// ReferenceTransactionType returns the resolved reference transaction type.
func (w *WagerTransaction) ReferenceTransactionType() WagerTransactionType {
	return w.referenceTransactionType
}

// Status returns the current transaction status.
func (w *WagerTransaction) Status() WagerTransactionStatus {
	return w.status
}

// FailureCode returns the persisted terminal failure code.
func (w *WagerTransaction) FailureCode() string {
	return w.failureCode
}

// FailureMessage returns the persisted terminal failure description.
func (w *WagerTransaction) FailureMessage() string {
	return w.failureMessage
}

// BalanceBefore returns the balance observed before processing.
func (w *WagerTransaction) BalanceBefore() (Money, bool) {
	if w.balanceBefore == nil {
		return Money{}, false
	}

	return *w.balanceBefore, true
}

// BalanceAfter returns the balance observed after processing.
func (w *WagerTransaction) BalanceAfter() (Money, bool) {
	if w.balanceAfter == nil {
		return Money{}, false
	}

	return *w.balanceAfter, true
}

// CreatedAt returns when the transaction was created.
func (w *WagerTransaction) CreatedAt() time.Time {
	return w.createdAt
}

// UpdatedAt returns when the transaction was last changed.
func (w *WagerTransaction) UpdatedAt() time.Time {
	return w.updatedAt
}

// CompletedAt returns when the transaction became terminal.
func (w *WagerTransaction) CompletedAt() (time.Time, bool) {
	if w.completedAt == nil {
		return time.Time{}, false
	}

	return *w.completedAt, true
}

// isValidIdentifier reports whether a provider supplied identifier is present and bounded.
func isValidIdentifier(value string) bool {
	return strings.TrimSpace(value) != "" && len(value) <= maxIdentifierLength
}
