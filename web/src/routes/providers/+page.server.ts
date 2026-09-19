import { listInstances } from '$lib/server/api';
import { config } from '$lib/server/config';
import { providerStatus } from '$lib/server/keycloak';

import type { PageServerLoad } from './$types';

/** Lists the realm provider clients with their status and the API instances that are up. */
export const load: PageServerLoad = async () => {
	const [instances, providers] = await Promise.all([
		listInstances(true),
		Promise.all(
			config.providers.map(async (provider) => ({
				id: provider.id,
				name: provider.name,
				clientId: provider.clientId,
				...(await providerStatus(provider))
			}))
		)
	]);

	return { instances, providers };
};
