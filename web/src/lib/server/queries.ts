import type { LedgerEntry, Transaction, Wallet } from '$lib/types';

import { sql } from './db';

// Every query is read-only. NUMERIC columns are returned as text so money never becomes a float.

type TransactionRow = {
	id: string;
	provider_id: string | null;
	external_transaction_id: string | null;
	idempotency_key: string | null;
	wallet_id: string;
	kind: string;
	amount: string;
	currency: string;
	status: string;
	failure_code: string | null;
	reference_external_transaction_id: string | null;
	balance_after: string | null;
	reference_retry_count: number;
	next_reference_attempt_at: Date | null;
	reference_expires_at: Date | null;
	created_at: Date;
	completed_at: Date | null;
};

const transactionColumns = sql`
	id,
	provider_id,
	external_transaction_id,
	idempotency_key,
	wallet_id,
	type::text AS kind,
	amount::text AS amount,
	currency,
	status::text AS status,
	failure_code,
	reference_external_transaction_id,
	balance_after::text AS balance_after,
	reference_retry_count,
	next_reference_attempt_at,
	reference_expires_at,
	created_at,
	completed_at
`;

/** Converts a timestamp column into an ISO string. */
function iso(value: Date | null): string | null {
	return value ? value.toISOString() : null;
}

/** Maps a wager_transactions row. */
function toTransaction(row: TransactionRow): Transaction {
	return {
		id: row.id,
		providerId: row.provider_id,
		externalTransactionId: row.external_transaction_id,
		idempotencyKey: row.idempotency_key,
		walletId: row.wallet_id,
		kind: row.kind,
		amount: row.amount,
		currency: row.currency,
		status: row.status,
		failureCode: row.failure_code,
		referenceExternalTransactionId: row.reference_external_transaction_id,
		balanceAfter: row.balance_after,
		retryCount: row.reference_retry_count,
		nextAttemptAt: iso(row.next_reference_attempt_at),
		expiresAt: iso(row.reference_expires_at),
		createdAt: row.created_at.toISOString(),
		completedAt: iso(row.completed_at)
	};
}

// Wallets and ledger

/** Lists wallets, most recently changed first. */
export async function listWallets(limit = 100): Promise<Wallet[]> {
	const rows = await sql<{ id: string; player_id: string; currency: string; balance: string; version: number; updated_at: Date; ledger_entries: number }[]>`
		SELECT
			w.id,
			w.player_id,
			w.currency,
			w.balance::text AS balance,
			w.version::int AS version,
			w.updated_at,
			(SELECT count(*)::int FROM wallet_ledger_entries l WHERE l.wallet_id = w.id) AS ledger_entries
		FROM wallets w
		ORDER BY w.updated_at DESC
		LIMIT ${limit}
	`;

	return rows.map((row) => ({
		id: row.id,
		playerId: row.player_id,
		currency: row.currency,
		balance: row.balance,
		version: row.version,
		ledgerEntries: row.ledger_entries,
		updatedAt: row.updated_at.toISOString()
	}));
}

/** Lists ledger entries, optionally of one wallet, newest first. */
export async function listLedger(walletId?: string, limit = 50): Promise<LedgerEntry[]> {
	const rows = await sql<{ id: string; wallet_id: string; transaction_id: string; kind: string; direction: string; amount: string; balance_before: string; balance_after: string; created_at: Date }[]>`
		SELECT
			l.id,
			l.wallet_id,
			l.transaction_id,
			t.type::text AS kind,
			l.direction::text AS direction,
			l.amount::text AS amount,
			l.balance_before::text AS balance_before,
			l.balance_after::text AS balance_after,
			l.created_at
		FROM wallet_ledger_entries l
		JOIN wager_transactions t ON t.id = l.transaction_id
		${walletId ? sql`WHERE l.wallet_id = ${walletId}` : sql``}
		ORDER BY l.created_at DESC, l.id DESC
		LIMIT ${limit}
	`;

	return rows.map((row) => ({
		id: row.id,
		walletId: row.wallet_id,
		transactionId: row.transaction_id,
		kind: row.kind,
		direction: row.direction,
		amount: row.amount,
		balanceBefore: row.balance_before,
		balanceAfter: row.balance_after,
		createdAt: row.created_at.toISOString()
	}));
}

export type WalletReconciliation = {
	id: string;
	stored: string;
	calculated: string;
	difference: string;
	entries: number;
	consistent: boolean;
};

/** Rebuilds wallet balances from the ledger in one statement; divergent wallets come first. */
export async function reconcileWallets(walletIds?: string[], limit = 100): Promise<WalletReconciliation[]> {
	const rows = await sql<{ id: string; stored: string; calculated: string; difference: string; entries: number; consistent: boolean }[]>`
		SELECT
			w.id,
			w.balance::text AS stored,
			COALESCE(sum(CASE WHEN l.direction = 'CREDIT' THEN l.amount ELSE -l.amount END), 0)::numeric(20, 2)::text AS calculated,
			(w.balance - COALESCE(sum(CASE WHEN l.direction = 'CREDIT' THEN l.amount ELSE -l.amount END), 0))::numeric(20, 2)::text AS difference,
			count(l.id)::int AS entries,
			w.balance = COALESCE(sum(CASE WHEN l.direction = 'CREDIT' THEN l.amount ELSE -l.amount END), 0) AS consistent
		FROM wallets w
		LEFT JOIN wallet_ledger_entries l ON l.wallet_id = w.id
		${walletIds ? sql`WHERE w.id = ANY(${walletIds}::uuid[])` : sql``}
		GROUP BY w.id, w.balance, w.updated_at
		ORDER BY consistent, w.updated_at DESC
		LIMIT ${limit}
	`;

	return rows;
}

// Transactions

export type TransactionFilter = {
	providerId?: string;
	walletId?: string;
	status?: string;
	kind?: string;
	limit?: number;
};

/** Lists transactions, newest first. */
export async function listTransactions(filter: TransactionFilter = {}): Promise<Transaction[]> {
	const conditions = [
		filter.providerId ? sql`provider_id = ${filter.providerId}` : undefined,
		filter.walletId ? sql`wallet_id = ${filter.walletId}` : undefined,
		filter.status ? sql`status::text = ${filter.status}` : undefined,
		filter.kind ? sql`type::text = ${filter.kind}` : undefined
	].filter((condition) => condition !== undefined);

	const where = conditions.length
		? sql`WHERE ${conditions.reduce((all, condition) => sql`${all} AND ${condition}`)}`
		: sql``;

	const rows = await sql<TransactionRow[]>`
		SELECT ${transactionColumns}
		FROM wager_transactions
		${where}
		ORDER BY created_at DESC
		LIMIT ${filter.limit ?? 100}
	`;

	return rows.map(toTransaction);
}

/** Finds a provider transaction by its external id. */
export async function findTransaction(providerId: string, externalTransactionId: string): Promise<Transaction | undefined> {
	const [row] = await sql<TransactionRow[]>`
		SELECT ${transactionColumns}
		FROM wager_transactions
		WHERE provider_id = ${providerId}
		  AND external_transaction_id = ${externalTransactionId}
	`;

	return row ? toTransaction(row) : undefined;
}

/** Reads one transaction by its internal id. */
export async function findTransactionById(id: string): Promise<Transaction | undefined> {
	const rows = await sql<TransactionRow[]>`
		SELECT ${transactionColumns}
		FROM wager_transactions
		WHERE id = ${id}
	`;

	return rows[0] ? toTransaction(rows[0]) : undefined;
}

/** Reads the wallet of a transaction. */
export async function findWallet(walletId: string): Promise<Wallet | undefined> {
	const rows = await sql<{ id: string; player_id: string; currency: string; balance: string; version: number; updated_at: Date; ledger_entries: number }[]>`
		SELECT
			w.id,
			w.player_id,
			w.currency,
			w.balance::text AS balance,
			w.version::int AS version,
			w.updated_at,
			(SELECT count(*)::int FROM wallet_ledger_entries l WHERE l.wallet_id = w.id) AS ledger_entries
		FROM wallets w
		WHERE w.id = ${walletId}
	`;

	const row = rows[0];

	return row
		? {
				id: row.id,
				playerId: row.player_id,
				currency: row.currency,
				balance: row.balance,
				version: row.version,
				ledgerEntries: row.ledger_entries,
				updatedAt: row.updated_at.toISOString()
			}
		: undefined;
}

/** Ledger entries created by one transaction. */
export async function listLedgerByTransaction(transactionId: string): Promise<LedgerEntry[]> {
	const rows = await sql<{ id: string; wallet_id: string; transaction_id: string; kind: string; direction: string; amount: string; balance_before: string; balance_after: string; created_at: Date }[]>`
		SELECT
			l.id,
			l.wallet_id,
			l.transaction_id,
			t.type::text AS kind,
			l.direction::text AS direction,
			l.amount::text AS amount,
			l.balance_before::text AS balance_before,
			l.balance_after::text AS balance_after,
			l.created_at
		FROM wallet_ledger_entries l
		JOIN wager_transactions t ON t.id = l.transaction_id
		WHERE l.transaction_id = ${transactionId}
	`;

	return rows.map((row) => ({
		id: row.id,
		walletId: row.wallet_id,
		transactionId: row.transaction_id,
		kind: row.kind,
		direction: row.direction,
		amount: row.amount,
		balanceBefore: row.balance_before,
		balanceAfter: row.balance_after,
		createdAt: row.created_at.toISOString()
	}));
}

/** Counts transactions by status, optionally for one provider. */
export async function countByStatus(providerId?: string): Promise<Record<string, number>> {
	const rows = await sql<{ status: string; total: number }[]>`
		SELECT status::text AS status, count(*)::int AS total
		FROM wager_transactions
		${providerId ? sql`WHERE provider_id = ${providerId}` : sql``}
		GROUP BY status
	`;

	return Object.fromEntries(rows.map((row) => [row.status, row.total]));
}

/** Counts transactions by kind and status. */
export async function countByKindAndStatus(): Promise<{ kind: string; status: string; total: number }[]> {
	return sql<{ kind: string; status: string; total: number }[]>`
		SELECT type::text AS kind, status::text AS status, count(*)::int AS total
		FROM wager_transactions
		GROUP BY type, status
		ORDER BY type, status
	`;
}

/** Transactions waiting for their reference, next retry first. */
export async function listPendingReferences(): Promise<Transaction[]> {
	const rows = await sql<TransactionRow[]>`
		SELECT ${transactionColumns}
		FROM wager_transactions
		WHERE status = 'PENDING_REFERENCE'
		ORDER BY next_reference_attempt_at
		LIMIT 100
	`;

	return rows.map(toTransaction);
}

// Overview, outbox and inbox

export type Overview = {
	wallets: number;
	totalBalance: string;
	ledgerEntries: number;
	transactions: number;
	outboxPending: number;
	outboxClaimed: number;
	outboxPublished: number;
	outboxOldestSeconds: number;
	inboxTotal: number;
	pendingReferences: number;
	inconsistentWallets: number;
};

/** Reads the global counters of the service. */
export async function readOverview(): Promise<Overview> {
	const [row] = await sql<Overview[]>`
		SELECT
			(SELECT count(*)::int FROM wallets) AS "wallets",
			(SELECT COALESCE(sum(balance), 0)::numeric(24, 2)::text FROM wallets) AS "totalBalance",
			(SELECT count(*)::int FROM wallet_ledger_entries) AS "ledgerEntries",
			(SELECT count(*)::int FROM wager_transactions WHERE type <> 'OPENING') AS "transactions",
			(SELECT count(*)::int FROM outbox_events WHERE published_at IS NULL) AS "outboxPending",
			(SELECT count(*)::int FROM outbox_events WHERE published_at IS NULL AND claimed_until > clock_timestamp()) AS "outboxClaimed",
			(SELECT count(*)::int FROM outbox_events WHERE published_at IS NOT NULL) AS "outboxPublished",
			(SELECT COALESCE(extract(epoch FROM clock_timestamp() - min(occurred_at)), 0)::float8 FROM outbox_events WHERE published_at IS NULL) AS "outboxOldestSeconds",
			(SELECT count(*)::int FROM inbox_messages) AS "inboxTotal",
			(SELECT count(*)::int FROM wager_transactions WHERE status = 'PENDING_REFERENCE') AS "pendingReferences",
			(
				SELECT count(*)::int
				FROM (
					SELECT w.id
					FROM wallets w
					LEFT JOIN wallet_ledger_entries l ON l.wallet_id = w.id
					GROUP BY w.id, w.balance
					HAVING w.balance <> COALESCE(sum(CASE WHEN l.direction = 'CREDIT' THEN l.amount ELSE -l.amount END), 0)
				) AS divergent
			) AS "inconsistentWallets"
	`;

	return row;
}

export type OutboxEvent = {
	eventId: string;
	eventType: string;
	aggregateId: string;
	correlationId: string;
	attempts: number;
	claimedBy: string | null;
	occurredAt: string;
	publishedAt: string | null;
	publishLagMs: number | null;
	payload: unknown;
};

/** Lists the most recent outbox events. */
export async function listOutbox(limit = 50, pendingOnly = false): Promise<OutboxEvent[]> {
	const rows = await sql<{ event_id: string; event_type: string; aggregate_id: string; correlation_id: string; attempts: number; claimed_by: string | null; occurred_at: Date; published_at: Date | null; lag_ms: number | null; payload: unknown }[]>`
		SELECT
			event_id,
			event_type,
			aggregate_id,
			correlation_id,
			attempts,
			claimed_by,
			occurred_at,
			published_at,
			(extract(epoch FROM published_at - occurred_at) * 1000)::float8 AS lag_ms,
			payload
		FROM outbox_events
		${pendingOnly ? sql`WHERE published_at IS NULL` : sql``}
		ORDER BY occurred_at DESC
		LIMIT ${limit}
	`;

	return rows.map((row) => ({
		eventId: row.event_id,
		eventType: row.event_type,
		aggregateId: row.aggregate_id,
		correlationId: row.correlation_id,
		attempts: row.attempts,
		claimedBy: row.claimed_by,
		occurredAt: row.occurred_at.toISOString(),
		publishedAt: iso(row.published_at),
		publishLagMs: row.lag_ms === null ? null : Math.round(row.lag_ms),
		payload: row.payload
	}));
}

export type InboxMessage = {
	consumerName: string;
	messageId: string;
	receivedAt: string;
	completedAt: string | null;
};

/** Lists the most recent inbox messages. */
export async function listInbox(limit = 50): Promise<InboxMessage[]> {
	const rows = await sql<{ consumer_name: string; message_id: string; received_at: Date; completed_at: Date | null }[]>`
		SELECT consumer_name, message_id, received_at, completed_at
		FROM inbox_messages
		ORDER BY received_at DESC
		LIMIT ${limit}
	`;

	return rows.map((row) => ({
		consumerName: row.consumer_name,
		messageId: row.message_id,
		receivedAt: row.received_at.toISOString(),
		completedAt: iso(row.completed_at)
	}));
}

/** Outbox events of one transaction: its own aggregate plus the wallet events that name it. */
export async function listOutboxForTransaction(transactionId: string): Promise<OutboxEvent[]> {
	const rows = await sql<{ event_id: string; event_type: string; aggregate_id: string; correlation_id: string; attempts: number; claimed_by: string | null; occurred_at: Date; published_at: Date | null; lag_ms: number | null; payload: unknown }[]>`
		SELECT
			event_id,
			event_type,
			aggregate_id,
			correlation_id,
			attempts,
			claimed_by,
			occurred_at,
			published_at,
			(extract(epoch FROM published_at - occurred_at) * 1000)::float8 AS lag_ms,
			payload
		FROM outbox_events
		WHERE aggregate_id = ${transactionId}::uuid
		   OR payload ->> 'transactionId' = ${transactionId}
		ORDER BY occurred_at
	`;

	return rows.map((row) => ({
		eventId: row.event_id,
		eventType: row.event_type,
		aggregateId: row.aggregate_id,
		correlationId: row.correlation_id,
		attempts: row.attempts,
		claimedBy: row.claimed_by,
		occurredAt: row.occurred_at.toISOString(),
		publishedAt: iso(row.published_at),
		publishLagMs: row.lag_ms === null ? null : Math.round(row.lag_ms),
		payload: row.payload
	}));
}

/** Finds an inbox message by the SQS envelope messageId. */
export async function findInboxMessage(messageId: string): Promise<InboxMessage | undefined> {
	const rows = await sql<{ consumer_name: string; message_id: string; received_at: Date; completed_at: Date | null }[]>`
		SELECT consumer_name, message_id, received_at, completed_at
		FROM inbox_messages
		WHERE message_id = ${messageId}
	`;

	const row = rows[0];

	return row
		? {
				consumerName: row.consumer_name,
				messageId: row.message_id,
				receivedAt: row.received_at.toISOString(),
				completedAt: iso(row.completed_at)
			}
		: undefined;
}
