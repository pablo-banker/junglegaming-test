import { randomUUID } from 'node:crypto';

import type { ApiCall, Instance, WagerPayload } from '$lib/types';

import { config, type ClientCredentials, type ProviderConfig } from './config';
import { accessToken } from './keycloak';

/** Health results are reused for a short time so every page does not probe every instance. */
const HEALTH_TTL_MS = 2_000;
const HEALTH_TIMEOUT_MS = 1_500;
const REQUEST_TIMEOUT_MS = 10_000;

let healthCache: { at: number; instances: Instance[] } | undefined;
let cursor = 0;

/** Probes /health/ready of every configured API instance. */
export async function listInstances(force = false): Promise<Instance[]> {
	if (!force && healthCache && Date.now() - healthCache.at < HEALTH_TTL_MS) {
		return healthCache.instances;
	}

	const instances = await Promise.all(config.apiUrls.map(probe));

	healthCache = { at: Date.now(), instances };

	return instances;
}

/** Probes one API instance. */
async function probe(url: string): Promise<Instance> {
	const start = performance.now();

	try {
		const response = await fetch(`${url}/health/ready`, { signal: AbortSignal.timeout(HEALTH_TIMEOUT_MS) });
		const body = (await response.json()) as { status: string; checks?: Record<string, string> };

		return {
			url,
			ready: response.ok,
			checks: body.checks ?? {},
			latencyMs: Math.round(performance.now() - start)
		};
	} catch (error) {
		return {
			url,
			ready: false,
			checks: {},
			latencyMs: Math.round(performance.now() - start),
			error: error instanceof Error ? error.message : String(error)
		};
	}
}

/** Returns the next ready instance, rotating requests across all of them. */
export async function nextInstance(): Promise<string> {
	const ready = (await listInstances()).filter((instance) => instance.ready);

	if (ready.length === 0) {
		throw new Error('no API instance is ready');
	}

	return ready[cursor++ % ready.length].url;
}

type CallOptions = {
	instance?: string;
	body?: unknown;
	idempotencyKey?: string;
};

/** Calls the Go API with a client_credentials token and records what happened. */
async function call(client: ClientCredentials, method: string, path: string, options: CallOptions = {}): Promise<ApiCall> {
	const instance = options.instance ?? (await nextInstance());
	const correlationId = `ui-${randomUUID()}`;

	const headers: Record<string, string> = {
		Authorization: `Bearer ${await accessToken(client)}`,
		'X-Correlation-Id': correlationId
	};

	if (options.body !== undefined) headers['Content-Type'] = 'application/json';
	if (options.idempotencyKey) headers['Idempotency-Key'] = options.idempotencyKey;

	const start = performance.now();

	try {
		const response = await fetch(`${instance}${path}`, {
			method,
			headers,
			body: options.body === undefined ? undefined : JSON.stringify(options.body),
			signal: AbortSignal.timeout(REQUEST_TIMEOUT_MS)
		});

		const text = await response.text();

		return {
			instance,
			method,
			path,
			status: response.status,
			durationMs: Math.round(performance.now() - start),
			correlationId,
			idempotencyKey: options.idempotencyKey,
			request: options.body,
			body: text ? JSON.parse(text) : null
		};
	} catch (error) {
		return {
			instance,
			method,
			path,
			status: 0,
			durationMs: Math.round(performance.now() - start),
			correlationId,
			idempotencyKey: options.idempotencyKey,
			request: options.body,
			body: null,
			error: error instanceof Error ? error.message : String(error)
		};
	}
}

/** Sends a wager operation as the provider, like POST /wagering/transactions from a game provider. */
export function submitWager(provider: ProviderConfig, payload: WagerPayload, idempotencyKey: string, instance?: string): Promise<ApiCall> {
	return call(provider, 'POST', '/wagering/transactions', { body: payload, idempotencyKey, instance });
}

/** Opens a wallet as the internal service. */
export function createWallet(playerId: string, amount: string, currency = 'BRL', instance?: string): Promise<ApiCall> {
	return call(config.internalClient, 'POST', '/wallets', {
		body: { playerId, initialBalance: { amount, currency } },
		instance
	});
}

/** Reads a wallet as the internal service (GET /wallets/:id). */
export function getWallet(walletId: string): Promise<ApiCall> {
	return call(config.internalClient, 'GET', `/wallets/${encodeURIComponent(walletId)}`);
}

/** Reads the newest ledger entries of a wallet as the internal service. */
export function getWalletLedger(walletId: string, limit = 20): Promise<ApiCall> {
	return call(config.internalClient, 'GET', `/wallets/${encodeURIComponent(walletId)}/ledger?limit=${limit}`);
}

/** Reads a transaction as the provider that owns it (GET /wagering/transactions/:id). */
export function getTransaction(provider: ProviderConfig, transactionId: string): Promise<ApiCall> {
	return call(provider, 'GET', `/wagering/transactions/${encodeURIComponent(transactionId)}`);
}

/** Downloads the Prometheus metrics of an instance. */
export async function fetchMetrics(url: string): Promise<string> {
	const response = await fetch(`${url}/metrics`, { signal: AbortSignal.timeout(HEALTH_TIMEOUT_MS) });

	if (!response.ok) {
		throw new Error(`metrics returned HTTP ${response.status}`);
	}

	return response.text();
}
