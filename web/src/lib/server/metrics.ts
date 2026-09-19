import { fetchMetrics } from './api';

type Sample = {
	name: string;
	labels: Record<string, string>;
	value: number;
};

export type InstanceMetrics = {
	url: string;
	error?: string;
	wagersByStatus: Record<string, number>;
	wagersBySource: Record<string, number>;
	replays: number;
	conflicts: number;
	transactionRetries: number;
	sqsMessages: Record<string, number>;
	outboxPublications: Record<string, number>;
	goroutines: number;
	memoryMb: number;
	uptimeSeconds: number;
};

/** Parses the Prometheus text exposition format (counters and gauges only). */
function parse(text: string): Sample[] {
	const samples: Sample[] = [];

	for (const line of text.split('\n')) {
		if (!line || line.startsWith('#')) continue;

		const match = /^([a-zA-Z_:][a-zA-Z0-9_:]*)(?:\{(.*)\})?\s+(\S+)/.exec(line);
		if (!match) continue;

		const labels: Record<string, string> = {};

		for (const pair of match[2]?.matchAll(/(\w+)="((?:[^"\\]|\\.)*)"/g) ?? []) {
			labels[pair[1]] = pair[2];
		}

		samples.push({ name: match[1], labels, value: Number(match[3]) });
	}

	return samples;
}

/** Sums a metric grouped by one label. */
function groupBy(samples: Sample[], name: string, label: string): Record<string, number> {
	const groups: Record<string, number> = {};

	for (const sample of samples) {
		if (sample.name !== name) continue;

		const key = sample.labels[label] ?? 'total';
		groups[key] = (groups[key] ?? 0) + sample.value;
	}

	return groups;
}

/** Sums every series of a metric. */
function total(samples: Sample[], name: string): number {
	return samples.filter((sample) => sample.name === name).reduce((sum, sample) => sum + sample.value, 0);
}

/** Reads the counters exposed by one API instance. */
export async function readInstanceMetrics(url: string): Promise<InstanceMetrics> {
	try {
		const samples = parse(await fetchMetrics(url));
		const startedAt = total(samples, 'process_start_time_seconds');

		return {
			url,
			wagersByStatus: groupBy(samples, 'wager_transactions_total', 'status'),
			wagersBySource: groupBy(samples, 'wager_transactions_total', 'source'),
			replays: total(samples, 'wager_idempotent_replays_total'),
			conflicts: total(samples, 'wager_conflicts_total'),
			transactionRetries: total(samples, 'db_transaction_retries_total'),
			sqsMessages: groupBy(samples, 'sqs_messages_total', 'result'),
			outboxPublications: groupBy(samples, 'outbox_publications_total', 'result'),
			goroutines: total(samples, 'go_goroutines'),
			memoryMb: Math.round(total(samples, 'process_resident_memory_bytes') / 1024 / 1024),
			uptimeSeconds: startedAt ? Math.round(Date.now() / 1000 - startedAt) : 0
		};
	} catch (error) {
		return {
			url,
			error: error instanceof Error ? error.message : String(error),
			wagersByStatus: {},
			wagersBySource: {},
			replays: 0,
			conflicts: 0,
			transactionRetries: 0,
			sqsMessages: {},
			outboxPublications: {},
			goroutines: 0,
			memoryMb: 0,
			uptimeSeconds: 0
		};
	}
}
