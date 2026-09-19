<script lang="ts">
	import { formatMoney } from '$lib/money';
	import type { OperationResult } from '$lib/types';

	import Json from './Json.svelte';
	import TxStatus from './TxStatus.svelte';

	/** Shows an operation as the backend reported it: response, wallet before/after, transaction and ledger. */
	let { operation }: { operation: OperationResult } = $props();

	type Body = {
		transactionId?: string;
		status?: string;
		failureCode?: string;
		idempotentReplay?: boolean;
		balance?: { amount: string; currency: string };
		code?: string;
		message?: string;
		details?: string;
	};

	const body = $derived((operation.response.body ?? {}) as Body);
	const before = $derived(operation.walletBefore);
	const after = $derived(operation.walletAfter);
	const entry = $derived(operation.ledgerEntry);
	const ok = $derived(operation.response.status >= 200 && operation.response.status < 300);
</script>

<div class="card bg-base-100 border-base-300 border">
	<div class="card-body gap-4">
		<div class="flex flex-wrap items-baseline justify-between gap-2">
			<h3 class="card-title text-lg">
				{operation.request.kind}
				{formatMoney(operation.request.money.amount, operation.request.money.currency)}
			</h3>
			<p class="font-mono text-xs opacity-60">{operation.response.instance} · {operation.response.durationMs} ms</p>
		</div>

		<div class="stats stats-vertical sm:stats-horizontal bg-base-200 w-full">
			<div class="stat py-3">
				<div class="stat-title text-xs">HTTP</div>
				<div class="stat-value text-2xl {ok ? '' : 'text-error'}">{operation.response.status || 'erro'}</div>
			</div>
			<div class="stat py-3">
				<div class="stat-title text-xs">status</div>
				<div class="stat-value text-base"><TxStatus status={body.status} /></div>
				{#if body.failureCode}<div class="stat-desc font-mono">{body.failureCode}</div>{/if}
			</div>
			<div class="stat py-3">
				<div class="stat-title text-xs">saldo devolvido</div>
				<div class="stat-value text-xl">{body.balance ? formatMoney(body.balance.amount, body.balance.currency) : '—'}</div>
				<div class="stat-desc">idempotentReplay: {body.idempotentReplay ?? '—'}</div>
			</div>
		</div>

		{#if body.code}
			<div class="alert alert-warning alert-soft text-sm">
				<span><strong class="font-mono">{body.code}</strong> — {body.message}{body.details ? ` (${body.details})` : ''}</span>
			</div>
		{/if}

		<div>
			<h4 class="mb-1 text-xs font-semibold opacity-60">Efeito observado (lido pela API)</h4>
			<table class="table table-sm">
				<tbody>
					<tr>
						<td class="w-44 opacity-60">Saldo</td>
						<td class="font-mono">
							{#if before && after}
								{formatMoney(before.balance.amount, before.balance.currency)} → {formatMoney(after.balance.amount, after.balance.currency)}
							{:else}
								<span class="opacity-60">carteira não encontrada pela API</span>
							{/if}
						</td>
					</tr>
					<tr>
						<td class="opacity-60">Wallet version</td>
						<td class="font-mono">{#if before && after}{before.version} → {after.version}{:else}—{/if}</td>
					</tr>
					<tr>
						<td class="opacity-60">Transaction</td>
						<td>
							{#if operation.transaction}
								<TxStatus status={operation.transaction.status} />
								{#if operation.transaction.failureCode}
									<span class="ml-2 font-mono text-xs">{operation.transaction.failureCode}</span>
								{/if}
							{:else}
								<span class="opacity-60">nenhuma transação registrada</span>
							{/if}
						</td>
					</tr>
					<tr>
						<td class="opacity-60">Ledger da transação</td>
						<td class="font-mono">
							{#if entry}
								{entry.direction}
								{formatMoney(entry.money.amount, entry.money.currency)}
								<span class="opacity-60">({formatMoney(entry.balanceBefore.amount)} → {formatMoney(entry.balanceAfter.amount)})</span>
							{:else}
								<span class="opacity-60">nenhum lançamento para esta transação</span>
							{/if}
						</td>
					</tr>
					<tr>
						<td class="opacity-60">Outbox e inbox</td>
						<td>
							{#if body.transactionId}
								<a class="link link-primary text-sm" href="/admin/transactions/{body.transactionId}">ver a transação no Admin</a>
							{:else}
								<span class="opacity-60">nada registrado</span>
							{/if}
						</td>
					</tr>
				</tbody>
			</table>
		</div>

		<p class="font-mono text-xs break-all opacity-50">
			Idempotency-Key {operation.response.idempotencyKey} · X-Correlation-Id {operation.response.correlationId}
		</p>

		<div class="grid gap-2 md:grid-cols-2">
			<Json title="Request" value={operation.request} />
			<Json title="Response" value={operation.response.body} />
		</div>
	</div>
</div>
