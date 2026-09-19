import { config } from '$lib/server/config';
import { listPendingReferences, listTransactions } from '$lib/server/queries';

import type { PageServerLoad } from './$types';

/** Transaction table with simple filters, plus everything waiting for a reference. */
export const load: PageServerLoad = async ({ url }) => {
	const filter = {
		providerId: url.searchParams.get('provider') ?? undefined,
		status: url.searchParams.get('status') ?? undefined,
		kind: url.searchParams.get('kind') ?? undefined,
		limit: 100
	};

	const [transactions, pendingReferences] = await Promise.all([listTransactions(filter), listPendingReferences()]);

	return {
		transactions,
		pendingReferences,
		filter,
		providers: config.providers.map((provider) => provider.id)
	};
};
