import { error } from '@sveltejs/kit';

import { findInboxMessage, findTransactionById, findWallet, listLedgerByTransaction, listOutboxForTransaction } from '$lib/server/queries';

import type { PageServerLoad } from './$types';

/** One transaction and everything it caused: wallet, ledger, outbox events and the inbox message. */
export const load: PageServerLoad = async ({ params }) => {
	const transaction = await findTransactionById(params.id);

	if (!transaction) {
		error(404, 'transação não encontrada');
	}

	const [wallet, ledger, outbox] = await Promise.all([
		findWallet(transaction.walletId),
		listLedgerByTransaction(transaction.id),
		listOutboxForTransaction(transaction.id)
	]);

	// Operations that arrived through SQS carry the envelope messageId as correlation id.
	const correlationId = outbox[0]?.correlationId;
	const inbox = correlationId ? await findInboxMessage(correlationId) : undefined;

	return { transaction, wallet, ledger, outbox, inbox, correlationId };
};
