package domain

import "errors"

var (
	// Money and currency.
	ErrInvalidCurrency      = errors.New("invalid currency")
	ErrInvalidMoney         = errors.New("invalid money")
	ErrCurrencyMismatch     = errors.New("currency mismatch")
	ErrAmountMustBePositive = errors.New("amount must be positive")

	// Wallet.
	ErrInvalidWalletID      = errors.New("invalid wallet id")
	ErrInvalidPlayerID      = errors.New("invalid player id")
	ErrInvalidWalletVersion = errors.New("invalid wallet version")
	ErrInsufficientFunds    = errors.New("insufficient funds")
	ErrInvalidTimestamp     = errors.New("invalid timestamp")

	// Wager transaction.
	ErrInvalidTransactionID         = errors.New("invalid transaction id")
	ErrInvalidProviderID            = errors.New("invalid provider id")
	ErrInvalidExternalTransactionID = errors.New("invalid external transaction id")
	ErrInvalidIdempotencyKey        = errors.New("invalid idempotency key")
	ErrInvalidPayloadHash           = errors.New("invalid payload hash")
	ErrInvalidRoundID               = errors.New("invalid round id")
	ErrInvalidGameID                = errors.New("invalid game id")
	ErrInvalidWagerType             = errors.New("invalid wager transaction type")
	ErrInvalidWagerStatus           = errors.New("invalid wager transaction status")
	ErrInvalidWagerAmount           = errors.New("invalid wager amount")
	ErrInvalidWagerReference        = errors.New("invalid wager reference")
	ErrWagerReferenceMismatch       = errors.New("wager reference mismatch")
	ErrWagerReferenceNotProcessed   = errors.New("wager reference is not processed")
	ErrInvalidWagerTransition       = errors.New("invalid wager transition")
	ErrWagerTransactionTerminal     = errors.New("wager transaction is terminal")
	ErrInvalidFailureCode           = errors.New("invalid failure code")
	ErrInvalidFinancialResult       = errors.New("invalid financial result")
	ErrInvalidWagerState            = errors.New("invalid wager transaction state")

	// Ledger.
	ErrInvalidLedgerEntry     = errors.New("invalid ledger entry")
	ErrInvalidLedgerDirection = errors.New("invalid ledger direction")
)
