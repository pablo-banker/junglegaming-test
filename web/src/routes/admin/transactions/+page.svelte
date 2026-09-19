<script lang="ts">
	import TxStatus from '$lib/components/TxStatus.svelte';
	import { formatAmount } from '$lib/money';

	let { data } = $props();

	const statuses = ['PROCESSED', 'REJECTED', 'PENDING_REFERENCE', 'PENDING', 'FAILED'];
	const kinds = ['OPENING', 'BET', 'WIN', 'LOSS', 'REFUND', 'ROLLBACK'];
</script>

<section class="card bg-base-200 border-base-300 border">
	<div class="card-body">
		<form method="GET" class="flex flex-wrap items-end gap-3">
			<fieldset class="fieldset">
				<legend class="fieldset-legend">provider</legend>
				<select class="select select-sm" name="provider">
					<option value="">todos</option>
					{#each data.providers as provider (provider)}
						<option selected={data.filter.providerId === provider}>{provider}</option>
					{/each}
				</select>
			</fieldset>
			<fieldset class="fieldset">
				<legend class="fieldset-legend">status</legend>
				<select class="select select-sm" name="status">
					<option value="">todos</option>
					{#each statuses as status (status)}
						<option selected={data.filter.status === status}>{status}</option>
					{/each}
				</select>
			</fieldset>
			<fieldset class="fieldset">
				<legend class="fieldset-legend">kind</legend>
				<select class="select select-sm" name="kind">
					<option value="">todos</option>
					{#each kinds as kind (kind)}
						<option selected={data.filter.kind === kind}>{kind}</option>
					{/each}
				</select>
			</fieldset>
			<button class="btn btn-sm btn-primary">Filtrar</button>
			<a class="btn btn-sm btn-ghost" href="/admin/transactions">Limpar</a>
		</form>

		<div class="overflow-x-auto">
			<table class="table table-sm table-zebra">
				<thead>
					<tr>
						<th>Criada</th><th>Provider</th><th>externalTransactionId</th><th>Kind</th><th class="text-right">Valor</th>
						<th>Status</th><th>failureCode</th><th class="text-right">Saldo depois</th><th></th>
					</tr>
				</thead>
				<tbody>
					{#each data.transactions as transaction (transaction.id)}
						<tr>
							<td class="font-mono text-xs">{new Date(transaction.createdAt).toLocaleString('pt-BR')}</td>
							<td class="font-mono text-xs">{transaction.providerId ?? '—'}</td>
							<td class="font-mono text-xs">{transaction.externalTransactionId ?? '—'}</td>
							<td class="font-mono text-xs">{transaction.kind}</td>
							<td class="text-right font-mono">{formatAmount(transaction.amount)}</td>
							<td><TxStatus status={transaction.status} /></td>
							<td class="font-mono text-xs">{transaction.failureCode ?? ''}</td>
							<td class="text-right font-mono">{formatAmount(transaction.balanceAfter)}</td>
							<td><a class="btn btn-ghost btn-xs" href="/admin/transactions/{transaction.id}">abrir</a></td>
						</tr>
					{:else}
						<tr><td colspan="9" class="opacity-60">Nenhuma transação para este filtro.</td></tr>
					{/each}
				</tbody>
			</table>
		</div>
		<p class="text-xs opacity-50">Até 100 transações, das mais recentes para as mais antigas.</p>
	</div>
</section>

<section class="card bg-base-200 border-base-300 mt-5 border">
	<div class="card-body">
		<h2 class="card-title text-base">
			Pending references
			<span class="badge badge-soft badge-info badge-sm">{data.pendingReferences.length}</span>
		</h2>
		<div class="overflow-x-auto">
			<table class="table table-sm">
				<thead>
					<tr><th>Transação</th><th>Provider</th><th>Kind</th><th>Referência esperada</th><th>Tentativas</th><th>Próxima</th><th>Expira</th></tr>
				</thead>
				<tbody>
					{#each data.pendingReferences as pending (pending.id)}
						<tr>
							<td><a class="link link-primary font-mono text-xs" href="/admin/transactions/{pending.id}">{pending.externalTransactionId}</a></td>
							<td class="font-mono text-xs">{pending.providerId}</td>
							<td class="font-mono text-xs">{pending.kind}</td>
							<td class="font-mono text-xs">{pending.referenceExternalTransactionId}</td>
							<td class="font-mono">{pending.retryCount}</td>
							<td class="font-mono text-xs">{pending.nextAttemptAt ? new Date(pending.nextAttemptAt).toLocaleTimeString('pt-BR') : '—'}</td>
							<td class="font-mono text-xs">{pending.expiresAt ? new Date(pending.expiresAt).toLocaleString('pt-BR') : '—'}</td>
						</tr>
					{:else}
						<tr><td colspan="7" class="opacity-60">Nenhuma transação aguardando referência.</td></tr>
					{/each}
				</tbody>
			</table>
		</div>
	</div>
</section>
