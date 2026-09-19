import postgres from 'postgres';

import { config } from './config';

/** Read-only connection: the interface observes the database but never changes it. */
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
