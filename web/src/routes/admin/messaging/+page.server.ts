import { listInbox, listOutbox } from '$lib/server/queries';
import { listQueueStats } from '$lib/server/sqs';

import type { PageServerLoad } from './$types';

/** Queues, outbox events and inbox messages. */
export const load: PageServerLoad = async ({ url }) => {
	const pendingOnly = url.searchParams.get('outbox') === 'pending';

	const [queues, outbox, inbox] = await Promise.all([listQueueStats(), listOutbox(50, pendingOnly), listInbox(50)]);

	return { queues, outbox, inbox, pendingOnly };
};
