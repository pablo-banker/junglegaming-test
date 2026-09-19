import { error } from '@sveltejs/kit';

import { findWallet, listLedger, listTransactions, reconcileWallets } from '$lib/server/queries';

import type { PageServerLoad } from './$types';

/** One wallet with its ledger, its transactions and its reconciliation. */
export const load: PageServerLoad = async ({ params }) => {
	const wallet = await findWallet(params.id);

	if (!wallet) {
		error(404, 'carteira não encontrada');
	}

	const [ledger, transactions, [reconciliation]] = await Promise.all([
		listLedger(wallet.id, 50),
		listTransactions({ walletId: wallet.id, limit: 50 }),
		reconcileWallets([wallet.id])
	]);

	return { wallet, ledger, transactions, reconciliation };
};
