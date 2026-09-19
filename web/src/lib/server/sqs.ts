import { GetQueueAttributesCommand, SQSClient } from '@aws-sdk/client-sqs';

import type { QueueStats } from '$lib/types';

import { config } from './config';

export const sqs = new SQSClient({
	region: config.aws.region,
	endpoint: config.aws.endpoint,
	credentials: {
		accessKeyId: config.aws.accessKeyId,
		secretAccessKey: config.aws.secretAccessKey
	}
});

const QUEUES = [
	{ name: 'wager-transactions.fifo', role: 'Entrada', url: () => config.queues.wager },
	{ name: 'wager-transactions-dlq.fifo', role: 'DLQ de entrada', url: () => config.queues.wagerDlq },
	{ name: 'integration-events.fifo', role: 'Eventos (outbox)', url: () => config.queues.events },
	{ name: 'integration-events-dlq.fifo', role: 'DLQ de eventos', url: () => config.queues.eventsDlq }
];

/** Reads the approximate depth of every queue with GetQueueAttributes, which never touches the messages. */
export async function listQueueStats(): Promise<QueueStats[]> {
	return Promise.all(
		QUEUES.map(async (queue) => {
			try {
				const output = await sqs.send(
					new GetQueueAttributesCommand({
						QueueUrl: queue.url(),
						AttributeNames: [
							'ApproximateNumberOfMessages',
							'ApproximateNumberOfMessagesNotVisible',
							'ApproximateNumberOfMessagesDelayed',
							'VisibilityTimeout',
							'RedrivePolicy'
						]
					})
				);

				const attributes = output.Attributes ?? {};
				const redrive = attributes.RedrivePolicy ? (JSON.parse(attributes.RedrivePolicy) as { maxReceiveCount?: string }) : undefined;

				return {
					name: queue.name,
					role: queue.role,
					visible: Number(attributes.ApproximateNumberOfMessages ?? 0),
					inFlight: Number(attributes.ApproximateNumberOfMessagesNotVisible ?? 0),
					delayed: Number(attributes.ApproximateNumberOfMessagesDelayed ?? 0),
					visibilityTimeout: Number(attributes.VisibilityTimeout ?? 0),
					maxReceiveCount: redrive?.maxReceiveCount ? Number(redrive.maxReceiveCount) : undefined
				};
			} catch (error) {
				return {
					name: queue.name,
					role: queue.role,
					visible: 0,
					inFlight: 0,
					delayed: 0,
					visibilityTimeout: 0,
					error: error instanceof Error ? error.message : String(error)
				};
			}
		})
	);
}
