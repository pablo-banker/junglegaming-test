package application

// CommandMetadata contains metadata shared by application operations.
type CommandMetadata struct {
	CorrelationID string `json:"correlationId"`
	CausationID   string `json:"causationId,omitempty"`
}

// CreateWalletCommand contains the data required to create a wallet.
type CreateWalletCommand struct {
	PlayerID       string `json:"playerId"`
	Currency       string `json:"currency"`
	InitialBalance string `json:"initialBalance"`
}

// ProcessWagerCommand contains the business data required to process a wager.
type ProcessWagerCommand struct {
	ProviderID                     string `json:"providerId"`
	ExternalTransactionID          string `json:"externalTransactionId"`
	IdempotencyKey                 string `json:"idempotencyKey"`
	WalletID                       string `json:"walletId"`
	PlayerID                       string `json:"playerId"`
	RoundID                        string `json:"roundId"`
	GameID                         string `json:"gameId"`
	Type                           string `json:"type"`
	Amount                         string `json:"amount"`
	Currency                       string `json:"currency"`
	ReferenceExternalTransactionID string `json:"referenceExternalTransactionId,omitempty"`
}
