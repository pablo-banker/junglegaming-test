import postgres from 'postgres';

import { config } from './config';

/**
 * Read-only connection used by the interface. Every session starts with
 * default_transaction_read_only, so the interface can observe the database but never
 * change it: money only moves through the API and SQS, like a real provider.
 */
export const sql = postgres(config.databaseUrl, {
	max: 5,
	idle_timeout: 30,
	connect_timeout: 5,
	connection: {
		application_name: 'betmoney-management',
		default_transaction_read_only: true,
		statement_timeout: 5000
	}
});
