import { env } from '$env/dynamic/private';

export type ClientCredentials = {
	clientId: string;
	clientSecret: string;
};

export type ProviderConfig = ClientCredentials & {
	id: string;
	name: string;
};

/** Reads a variable, falling back to a default. */
function read(name: string, fallback = ''): string {
	return env[name]?.trim() || fallback;
}

/** Providers are the provider clients already provisioned in the Keycloak realm (KEYCLOAK_PROVIDER_<X>_CLIENT_ID). */
function readProviders(): ProviderConfig[] {
	const providers: ProviderConfig[] = [];

	for (const [key, clientId] of Object.entries(env)) {
		const match = /^KEYCLOAK_PROVIDER_([A-Z0-9]+)_CLIENT_ID$/.exec(key);
		if (!match || !clientId) continue;

		const suffix = match[1];

		providers.push({
			id: clientId,
			name: `Provider ${suffix}`,
			clientId,
			clientSecret: read(`KEYCLOAK_PROVIDER_${suffix}_CLIENT_SECRET`)
		});
	}

	return providers.sort((a, b) => a.name.localeCompare(b.name));
}

const issuer = read('KEYCLOAK_ISSUER_URL', 'http://localhost:8081/realms/junglegaming');
const wagerQueueUrl = read('SQS_WAGER_QUEUE_URL', 'http://localhost:4566/000000000000/wager-transactions.fifo');
const eventQueueUrl = read('SQS_EVENT_QUEUE_URL', 'http://localhost:4566/000000000000/integration-events.fifo');

export const config = {
	databaseUrl: read('WEB_DATABASE_URL', read('DATABASE_URL')),

	apiUrls: read('API_URLS', 'http://localhost:8080,http://localhost:8082,http://localhost:8083')
		.split(',')
		.map((url) => url.trim().replace(/\/$/, ''))
		.filter(Boolean),

	tokenUrl: read('KEYCLOAK_TOKEN_URL', `${issuer}/protocol/openid-connect/token`),
	providers: readProviders(),
	internalClient: {
		clientId: read('KEYCLOAK_INTERNAL_CLIENT_ID', 'internal-service'),
		clientSecret: read('KEYCLOAK_INTERNAL_CLIENT_SECRET', 'internal-service-secret')
	},

	aws: {
		region: read('AWS_REGION', 'us-east-1'),
		endpoint: read('SQS_ENDPOINT', 'http://localhost:4566'),
		accessKeyId: read('AWS_ACCESS_KEY_ID', 'test'),
		secretAccessKey: read('AWS_SECRET_ACCESS_KEY', 'test')
	},

	queues: {
		wager: wagerQueueUrl,
		wagerDlq: read('SQS_WAGER_DLQ_URL', wagerQueueUrl.replace(/\.fifo$/, '-dlq.fifo')),
		events: eventQueueUrl,
		eventsDlq: read('SQS_EVENT_DLQ_URL', eventQueueUrl.replace(/\.fifo$/, '-dlq.fifo'))
	}
};

/** Returns the configured provider or undefined. */
export function findProvider(id: string): ProviderConfig | undefined {
	return config.providers.find((provider) => provider.id === id);
}
