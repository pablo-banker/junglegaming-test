<script lang="ts">
	import Json from '$lib/components/Json.svelte';
	import TxStatus from '$lib/components/TxStatus.svelte';
	import { formatAmount, formatMoney } from '$lib/money';

	let { data } = $props();

	const tx = $derived(data.transaction);

	/** Fields of the transaction row worth showing as-is. */
	const fields = $derived([
		['id', tx.id],
		['providerId', tx.providerId],
		['externalTransactionId', tx.externalTransactionId],
		['idempotencyKey', tx.idempotencyKey],
		['walletId', tx.walletId],
		['referenceExternalTransactionId', tx.referenceExternalTransactionId],
		['failureCode', tx.failureCode],
		['balanceAfter', tx.balanceAfter ? formatAmount(tx.balanceAfter) : null],
		['createdAt', new Date(tx.createdAt).toLocaleString('pt-BR')],
		['completedAt', tx.completedAt ? new Date(tx.completedAt).toLocaleString('pt-BR') : null]
	] as [string, string | null][]);
</script>

<div class="breadcrumbs mb-3 text-sm">
	<ul>
		<li><a href="/admin/transactions">Transactions</a></li>
		<li class="font-mono">{tx.kind}</li>
	</ul>
</div>

<section class="card bg-base-200 border-base-300 border">
	<div class="card-body">
		<div class="flex flex-wrap items-center justify-between gap-3">
			<h1 class="text-2xl font-semibold">{tx.kind} {formatMoney(tx.amount, tx.currency)}</h1>
			<TxStatus status={tx.status} />
		</div>

		<dl class="grid gap-x-6 gap-y-3 sm:grid-cols-2 xl:grid-cols-3">
			{#each fields as [name, value] (name)}
				<div>
					<dt class="text-xs opacity-60">{name}</dt>
					<dd class="font-mono text-xs break-all">{value ?? '—'}</dd>
				</div>
			{/each}
		</dl>
	</div>
</section>

<div class="mt-5 grid gap-5 xl:grid-cols-2">
	<section class="card bg-base-200 border-base-300 border">
		<div class="card-body">
			<h2 class="card-title text-base">Wallet</h2>
			{#if data.wallet}
				<table class="table table-sm">
					<tbody>
						<tr>
							<td class="w-28 opacity-60">id</td>
							<td><a class="link link-primary font-mono text-xs" href="/admin/wallets/{data.wallet.id}">{data.wallet.id}</a></td>
						</tr>
						<tr><td class="opacity-60">player</td><td class="font-mono text-xs">{data.wallet.playerId}</td></tr>
						<tr><td class="opacity-60">saldo atual</td><td class="font-mono">{formatMoney(data.wallet.balance, data.wallet.currency)}</td></tr>
						<tr><td class="opacity-60">version</td><td class="font-mono">{data.wallet.version}</td></tr>
						<tr><td class="opacity-60">lançamentos</td><td class="font-mono">{data.wallet.ledgerEntries}</td></tr>
					</tbody>
				</table>
			{:else}
				<p class="opacity-60">Carteira não encontrada.</p>
			{/if}
		</div>
	</section>

	<section class="card bg-base-200 border-base-300 border">
		<div class="card-body">
			<h2 class="card-title text-base">Ledger desta transação</h2>
			{#if data.ledger.length}
				<table class="table table-sm">
					<thead><tr><th>Direção</th><th class="text-right">Valor</th><th class="text-right">Antes</th><th class="text-right">Depois</th><th>Criado</th></tr></thead>
					<tbody>
						{#each data.ledger as entry (entry.id)}
							<tr>
								<td>
									<span class="badge badge-soft badge-sm {entry.direction === 'CREDIT' ? 'badge-success' : 'badge-warning'} font-mono">{entry.direction}</span>
								</td>
								<td class="text-right font-mono">{formatAmount(entry.amount)}</td>
								<td class="text-right font-mono">{formatAmount(entry.balanceBefore)}</td>
								<td class="text-right font-mono">{formatAmount(entry.balanceAfter)}</td>
								<td class="font-mono text-xs">{new Date(entry.createdAt).toLocaleTimeString('pt-BR')}</td>
							</tr>
						{/each}
					</tbody>
				</table>
			{:else}
				<p class="opacity-60">Nenhum lançamento: a operação não movimentou saldo.</p>
			{/if}
		</div>
	</section>
</div>

<section class="card bg-base-200 border-base-300 mt-5 border">
	<div class="card-body">
		<h2 class="card-title text-base">
			Outbox
			<span class="badge badge-soft badge-sm">{data.outbox.length}</span>
		</h2>

		{#if data.outbox.length}
			<div class="space-y-3">
				{#each data.outbox as event (event.eventId)}
					<div class="bg-base-100 border-base-300 rounded-box border p-3">
						<div class="flex flex-wrap items-center justify-between gap-2">
							<h3 class="font-semibold">{event.eventType}</h3>
							{#if event.publishedAt}
								<span class="badge badge-soft badge-success badge-sm">publicado em {event.publishLagMs} ms</span>
							{:else}
								<span class="badge badge-soft badge-warning badge-sm">
									pendente · {event.attempts} tentativa(s){event.claimedBy ? ` · claim de ${event.claimedBy}` : ''}
								</span>
							{/if}
						</div>
						<p class="mt-1 font-mono text-xs break-all opacity-60">
							eventId {event.eventId} · aggregate {event.aggregateId} · correlation {event.correlationId}
						</p>
						<div class="mt-2"><Json title="data" value={event.payload} /></div>
					</div>
				{/each}
			</div>
		{:else}
			<p class="opacity-60">Nenhum evento: rejeições de validação não geram eventos.</p>
		{/if}
	</div>
</section>

<section class="card bg-base-200 border-base-300 mt-5 border">
	<div class="card-body">
		<h2 class="card-title text-base">Inbox</h2>
		{#if data.inbox}
			<table class="table table-sm">
				<tbody>
					<tr><td class="w-32 opacity-60">consumer</td><td class="font-mono text-xs">{data.inbox.consumerName}</td></tr>
					<tr><td class="opacity-60">messageId</td><td class="font-mono text-xs">{data.inbox.messageId}</td></tr>
					<tr><td class="opacity-60">recebida</td><td class="font-mono text-xs">{new Date(data.inbox.receivedAt).toLocaleString('pt-BR')}</td></tr>
					<tr><td class="opacity-60">concluída</td><td class="font-mono text-xs">{data.inbox.completedAt ? new Date(data.inbox.completedAt).toLocaleString('pt-BR') : '—'}</td></tr>
				</tbody>
			</table>
			<p class="text-xs opacity-50">Operação recebida pela fila: o messageId do envelope é o correlationId dos eventos.</p>
		{:else}
			<p class="opacity-60">
				Sem mensagem de inbox{data.correlationId ? ` para o correlationId ${data.correlationId}` : ''}: a operação chegou por HTTP.
			</p>
		{/if}
	</div>
</section>
