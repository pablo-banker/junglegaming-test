<script lang="ts">
	import Card from '$lib/components/Card.svelte';
	import TxStatus from '$lib/components/TxStatus.svelte';
	import { formatAmount, formatMoney } from '$lib/money';

	let { data } = $props();

	const wagerQueue = $derived(data.queues.find((queue) => queue.name === 'wager-transactions.fifo'));
	const deadLetters = $derived(data.queues.filter((queue) => queue.name.includes('dlq')).reduce((total, queue) => total + queue.visible, 0));
	const outboxLag = $derived(Math.round(data.overview.outboxOldestSeconds));

	const alerts = $derived(
		[
			data.overview.inconsistentWallets > 0 ? `${data.overview.inconsistentWallets} carteira(s) com saldo diferente do ledger` : '',
			deadLetters > 0 ? `${deadLetters} mensagem(ns) na DLQ` : '',
			(data.statuses.FAILED ?? 0) > 0 ? `${data.statuses.FAILED} transação(ões) FAILED` : '',
			outboxLag > 10 ? `evento mais antigo da outbox esperando há ${outboxLag} s` : ''
		].filter(Boolean)
	);
</script>

<div class="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
	<Card label="Wallets" value={data.overview.wallets} hint={formatMoney(data.overview.totalBalance)} />
	<Card label="Wagers" value={data.overview.transactions} hint="sem contar as aberturas (OPENING)" />
	<Card label="Pending references" value={data.overview.pendingReferences} hint="aguardando a referência chegar" />
	<Card label="Outbox pending" value={data.overview.outboxPending} hint={`mais antigo há ${outboxLag} s · ${data.overview.outboxClaimed} com claim ativo`} />
	<Card label="SQS ready" value={wagerQueue?.visible ?? '—'} hint="wager-transactions.fifo" />
	<Card label="SQS in flight" value={wagerQueue?.inFlight ?? '—'} hint={`visibility ${wagerQueue?.visibilityTimeout ?? '—'} s`} />
	<Card label="DLQ" value={deadLetters} hint="entrada + eventos" alert={deadLetters > 0} />
	<Card label="Ledger entries" value={data.overview.ledgerEntries} hint={`${data.overview.inconsistentWallets} carteira(s) divergente(s)`} alert={data.overview.inconsistentWallets > 0} />
</div>

{#if alerts.length}
	<div class="alert alert-warning alert-soft mt-4 flex-col items-start gap-1">
		{#each alerts as alert (alert)}
			<span class="text-sm">{alert}</span>
		{/each}
	</div>
{/if}

<div class="mt-5 grid gap-5 xl:grid-cols-2">
	<section class="card bg-base-200 border-base-300 border">
		<div class="card-body">
			<h2 class="card-title text-base">Transações por status</h2>
			<table class="table table-sm">
				<tbody>
					{#each Object.entries(data.statuses) as [status, total] (status)}
						<tr><td><TxStatus {status} /></td><td class="text-right font-mono">{total}</td></tr>
					{:else}
						<tr><td class="opacity-60">Nenhuma transação registrada.</td></tr>
					{/each}
				</tbody>
			</table>

			{#if data.pendingReferences.length}
				<h3 class="mt-3 text-sm font-semibold">Próximas referências pendentes</h3>
				<ul class="space-y-1 font-mono text-xs">
					{#each data.pendingReferences as pending (pending.id)}
						<li>
							<a class="link link-primary" href="/admin/transactions/{pending.id}">{pending.kind} {formatAmount(pending.amount)}</a>
							→ {pending.referenceExternalTransactionId} · retry {pending.retryCount} ·
							{pending.nextAttemptAt ? new Date(pending.nextAttemptAt).toLocaleTimeString('pt-BR') : '—'}
						</li>
					{/each}
				</ul>
			{/if}
		</div>
	</section>

	<section class="card bg-base-200 border-base-300 border">
		<div class="card-body">
			<h2 class="card-title text-base">Filas SQS</h2>
			<div class="overflow-x-auto">
				<table class="table table-sm">
					<thead>
						<tr><th>Fila</th><th>Ready</th><th>In flight</th><th>Delayed</th></tr>
					</thead>
					<tbody>
						{#each data.queues as queue (queue.name)}
							<tr>
								<td>
									<span class="font-mono text-xs">{queue.name}</span>
									<span class="block text-xs opacity-60">{queue.role}</span>
								</td>
								<td class="font-mono">{queue.error ? '—' : queue.visible}</td>
								<td class="font-mono">{queue.error ? '—' : queue.inFlight}</td>
								<td class="font-mono">{queue.error ? '—' : queue.delayed}</td>
							</tr>
							{#if queue.error}
								<tr><td colspan="4" class="text-error text-xs">{queue.error}</td></tr>
							{/if}
						{/each}
					</tbody>
				</table>
			</div>
			<p class="text-xs opacity-50">Lido com GetQueueAttributes: nenhuma mensagem é recebida nem escondida da fila.</p>
		</div>
	</section>
</div>

<section class="card bg-base-200 border-base-300 mt-5 border">
	<div class="card-body">
		<h2 class="card-title text-base">Instâncias da API</h2>
		<div class="overflow-x-auto">
			<table class="table table-sm table-zebra">
				<thead>
					<tr><th>Instância</th><th>Estado</th><th>Wagers</th><th>Replays</th><th>Conflitos</th><th>Retries</th><th>SQS</th><th>Outbox</th><th>Goroutines</th><th>Memória</th><th>Uptime</th></tr>
				</thead>
				<tbody>
					{#each data.instances as instance (instance.url)}
						{@const metrics = data.metrics.find((entry) => entry.url === instance.url)}
						<tr>
							<td class="font-mono text-xs">{instance.url}</td>
							<td>
								{#if instance.ready}
									<span class="badge badge-soft badge-success badge-sm">ready</span>
								{:else}
									<span class="badge badge-soft badge-error badge-sm">down</span>
								{/if}
							</td>
							<td class="font-mono text-xs">
								{#if metrics}
									{Object.entries(metrics.wagersByStatus).map(([status, total]) => `${status} ${total}`).join(' · ') || '—'}
								{:else}—{/if}
							</td>
							<td class="font-mono">{metrics?.replays ?? '—'}</td>
							<td class="font-mono">{metrics?.conflicts ?? '—'}</td>
							<td class="font-mono">{metrics?.transactionRetries ?? '—'}</td>
							<td class="font-mono text-xs">
								{#if metrics}
									{Object.entries(metrics.sqsMessages).map(([result, total]) => `${result} ${total}`).join(' · ') || '—'}
								{:else}—{/if}
							</td>
							<td class="font-mono text-xs">
								{#if metrics}
									{Object.entries(metrics.outboxPublications).map(([result, total]) => `${result} ${total}`).join(' · ') || '—'}
								{:else}—{/if}
							</td>
							<td class="font-mono">{metrics?.goroutines ?? '—'}</td>
							<td class="font-mono text-xs">{metrics ? `${metrics.memoryMb} MB` : '—'}</td>
							<td class="font-mono text-xs">{metrics ? `${metrics.uptimeSeconds} s` : '—'}</td>
						</tr>
					{/each}
				</tbody>
			</table>
		</div>
		<p class="text-xs opacity-50">Contadores lidos de /metrics de cada instância.</p>
	</div>
</section>
