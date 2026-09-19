import { listWallets, reconcileWallets } from '$lib/server/queries';

import type { PageServerLoad } from './$types';

/** Wallet table with the ledger reconstruction of each wallet. */
export const load: PageServerLoad = async () => {
	const [wallets, reconciliation] = await Promise.all([listWallets(100), reconcileWallets(undefined, 100)]);

	return {
		wallets: wallets.map((wallet) => ({
			...wallet,
			reconciliation: reconciliation.find((entry) => entry.id === wallet.id)
		}))
	};
};
