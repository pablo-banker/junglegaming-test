import { listInstances } from '$lib/server/api';
import { readInstanceMetrics } from '$lib/server/metrics';
import { countByStatus, listPendingReferences, readOverview } from '$lib/server/queries';
import { listQueueStats } from '$lib/server/sqs';

import type { PageServerLoad } from './$types';

/** Dashboard: counters from PostgreSQL, queue depths from SQS and counters from each API instance. */
export const load: PageServerLoad = async () => {
	const [overview, statuses, queues, instances, pendingReferences] = await Promise.all([
		readOverview(),
		countByStatus(),
		listQueueStats(),
		listInstances(true),
		listPendingReferences()
	]);

	const metrics = await Promise.all(instances.filter((instance) => instance.ready).map((instance) => readInstanceMetrics(instance.url)));

	return { overview, statuses, queues, instances, metrics, pendingReferences: pendingReferences.slice(0, 5) };
};
