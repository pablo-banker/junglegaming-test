import { randomUUID } from 'node:crypto';

import { error, fail, type Cookies } from '@sveltejs/kit';

import { createWallet, getTransaction, getWallet, getWalletLedger, listInstances, submitWager } from '$lib/server/api';
import { findProvider, type ProviderConfig } from '$lib/server/config';
import { currentToken, issueToken, providerStatus } from '$lib/server/keycloak';
import type { LedgerView, OperationResult, TransactionView, WagerKind, WagerPayload, WalletView } from '$lib/types';

import type { Actions, PageServerLoad } from './$types';

// This area acts as an external provider: everything goes through Keycloak and the Go API,
// never through the database.

const WALLETS_COOKIE = 'test-wallets';
const MAX_WALLETS = 15;

type TestWallet = { id: string; playerId: string };

/** Wallets remembered by this browser for the tests (ids only; balances always come from the API). */
function readWallets(cookies: Cookies): TestWallet[] {
	try {
		return JSON.parse(cookies.get(WALLETS_COOKIE) ?? '[]') as TestWallet[];
	} catch {
		return [];
	}
}

/** Stores a wallet at the top of the test list. */
function rememberWallet(cookies: Cookies, wallet: TestWallet): void {
	const wallets = [wallet, ...readWallets(cookies).filter((known) => known.id !== wallet.id)].slice(0, MAX_WALLETS);

	cookies.set(WALLETS_COOKIE, JSON.stringify(wallets), { path: '/', httpOnly: true, sameSite: 'lax', maxAge: 60 * 60 * 24 * 30 });
}

/** Returns the provider of the route or a 404. */
function providerOf(providerId: string): ProviderConfig {
	return findProvider(providerId) ?? error(404, `provider ${providerId} não está configurado`);
}

/** Reads a field of a form as a trimmed string. */
function field(form: FormData, name: string): string {
	return String(form.get(name) ?? '').trim();
}

/** Reads a wallet through the API, or undefined when it cannot be read. */
async function readWallet(walletId: string): Promise<WalletView | undefined> {
	if (!walletId) return undefined;

	const call = await getWallet(walletId);

	return call.status === 200 ? (call.body as WalletView) : undefined;
}

export const load: PageServerLoad = async ({ params, cookies }) => {
	const provider = providerOf(params.providerId);

	const [status, instances, wallets] = await Promise.all([
		providerStatus(provider),
		listInstances(),
		Promise.all(
			readWallets(cookies).map(async (wallet) => ({
				...wallet,
				view: await readWallet(wallet.id)
			}))
		)
	]);

	return {
		provider: { id: provider.id, name: provider.name, clientId: provider.clientId },
		status,
		token: currentToken(provider),
		readyInstances: instances.filter((instance) => instance.ready).length,
		wallets
	};
};

export const actions: Actions = {
	/** Requests a fresh client_credentials token and shows its claims. */
	token: async ({ params }) => {
		try {
			return { token: await issueToken(providerOf(params.providerId)) };
		} catch (reason) {
			return fail(502, { tokenError: reason instanceof Error ? reason.message : String(reason) });
		}
	},

	/** Opens a wallet for a new player through POST /wallets (internal service). */
	createWallet: async ({ request, cookies }) => {
		const form = await request.formData();
		const playerId = field(form, 'playerId') || randomUUID();
		const call = await createWallet(playerId, field(form, 'amount'), field(form, 'currency'));

		if (call.status !== 201) {
			return fail(call.status || 502, { walletCall: call });
		}

		rememberWallet(cookies, { id: (call.body as WalletView).id, playerId });

		return { walletCall: call };
	},

	/** Adds an existing wallet to the test list after reading it through the API. */
	loadWallet: async ({ request, cookies }) => {
		const form = await request.formData();
		const call = await getWallet(field(form, 'walletId'));

		if (call.status !== 200) {
			return fail(call.status || 502, { walletCall: call });
		}

		const wallet = call.body as WalletView;
		rememberWallet(cookies, { id: wallet.id, playerId: wallet.playerId });

		return { walletCall: call };
	},

	/** Removes a wallet from the test list; the wallet itself is untouched. */
	forgetWallet: async ({ request, cookies }) => {
		const form = await request.formData();
		const walletId = field(form, 'walletId');
		const wallets = readWallets(cookies).filter((wallet) => wallet.id !== walletId);

		cookies.set(WALLETS_COOKIE, JSON.stringify(wallets), { path: '/', httpOnly: true, sameSite: 'lax', maxAge: 60 * 60 * 24 * 30 });
	},

	/**
	 * Sends a wager operation as the provider and records what the API reports around it:
	 * the wallet before and after, the persisted transaction and its ledger entry.
	 */
	operate: async ({ params, request }) => {
		const provider = providerOf(params.providerId);
		const form = await request.formData();

		const payload: WagerPayload = {
			providerId: provider.id,
			externalTransactionId: field(form, 'externalTransactionId'),
			playerId: field(form, 'playerId'),
			walletId: field(form, 'walletId'),
			roundId: field(form, 'roundId'),
			gameId: field(form, 'gameId'),
			kind: field(form, 'kind') as WagerKind,
			money: { amount: field(form, 'amount'), currency: field(form, 'currency') }
		};

		const reference = field(form, 'referenceExternalTransactionId');
		if (reference) payload.referenceExternalTransactionId = reference;

		const walletBefore = await readWallet(payload.walletId);
		const response = await submitWager(provider, payload, field(form, 'idempotencyKey'));
		const walletAfter = await readWallet(payload.walletId);

		const transactionId = (response.body as { transactionId?: string } | null)?.transactionId;
		let transaction: TransactionView | undefined;
		let ledgerEntry: LedgerView | undefined;

		if (transactionId) {
			const [transactionCall, ledgerCall] = await Promise.all([getTransaction(provider, transactionId), getWalletLedger(payload.walletId)]);

			if (transactionCall.status === 200) transaction = transactionCall.body as TransactionView;

			if (ledgerCall.status === 200) {
				ledgerEntry = (ledgerCall.body as { entries: LedgerView[] }).entries.find((entry) => entry.transactionId === transactionId);
			}
		}

		const operation: OperationResult = {
			at: new Date().toISOString(),
			request: payload,
			response,
			walletBefore,
			walletAfter,
			transaction,
			ledgerEntry
		};

		return { operation };
	}
};
