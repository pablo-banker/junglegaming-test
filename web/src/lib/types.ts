// Types shared between server code and pages. Money is always a decimal string.

export type WagerKind = 'BET' | 'WIN' | 'LOSS' | 'REFUND' | 'ROLLBACK';

export type WagerPayload = {
	providerId: string;
	externalTransactionId: string;
	playerId: string;
	walletId: string;
	roundId: string;
	gameId: string;
	kind: WagerKind;
	money: { amount: string; currency: string };
	referenceExternalTransactionId?: string;
};

export type Instance = {
	url: string;
	ready: boolean;
	checks: Record<string, string>;
	latencyMs: number;
	error?: string;
};

export type ApiCall = {
	instance: string;
	method: string;
	path: string;
	status: number;
	durationMs: number;
	correlationId: string;
	idempotencyKey?: string;
	request?: unknown;
	body: unknown;
	error?: string;
};

export type TokenInfo = {
	issuedAt: string;
	expiresIn: number;
	claims: Record<string, unknown>;
};

/** Wallet as returned by GET /wallets/:id. */
export type WalletView = {
	id: string;
	playerId: string;
	balance: { amount: string; currency: string };
	version: number;
};

/** Ledger entry as returned by GET /wallets/:id/ledger. */
export type LedgerView = {
	id: string;
	transactionId: string;
	direction: string;
	money: { amount: string; currency: string };
	balanceBefore: { amount: string; currency: string };
	balanceAfter: { amount: string; currency: string };
	createdAt: string;
};

/** Transaction as returned by GET /wagering/transactions/:id. */
export type TransactionView = {
	transactionId: string;
	externalTransactionId: string;
	kind: string;
	money: { amount: string; currency: string };
	status: string;
	failureCode?: string;
	failureMessage?: string;
	referenceExternalTransactionId?: string;
	balanceBefore?: { amount: string; currency: string };
	balanceAfter?: { amount: string; currency: string };
};

/** Everything observed around one operation, all read through the API. */
export type OperationResult = {
	at: string;
	request: WagerPayload;
	response: ApiCall;
	walletBefore?: WalletView;
	walletAfter?: WalletView;
	transaction?: TransactionView;
	ledgerEntry?: LedgerView;
};

export type QueueStats = {
	name: string;
	role: string;
	visible: number;
	inFlight: number;
	delayed: number;
	visibilityTimeout: number;
	maxReceiveCount?: number;
	error?: string;
};

export type Wallet = {
	id: string;
	playerId: string;
	currency: string;
	balance: string;
	version: number;
	ledgerEntries: number;
	updatedAt: string;
};

export type Transaction = {
	id: string;
	providerId: string | null;
	externalTransactionId: string | null;
	idempotencyKey: string | null;
	walletId: string;
	kind: string;
	amount: string;
	currency: string;
	status: string;
	failureCode: string | null;
	referenceExternalTransactionId: string | null;
	balanceAfter: string | null;
	retryCount: number;
	nextAttemptAt: string | null;
	expiresAt: string | null;
	createdAt: string;
	completedAt: string | null;
};

export type LedgerEntry = {
	id: string;
	walletId: string;
	transactionId: string;
	kind: string;
	direction: string;
	amount: string;
	balanceBefore: string;
	balanceAfter: string;
	createdAt: string;
};
