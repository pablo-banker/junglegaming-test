<script lang="ts">
	import TxStatus from '$lib/components/TxStatus.svelte';
	import { formatAmount, formatMoney } from '$lib/money';

	let { data } = $props();
</script>

<div class="breadcrumbs mb-3 text-sm">
	<ul>
		<li><a href="/admin/wallets">Wallets</a></li>
		<li class="font-mono">{data.wallet.id.slice(0, 8)}…</li>
	</ul>
</div>

<section class="card bg-base-200 border-base-300 border">
	<div class="card-body">
		<div class="flex flex-wrap items-center justify-between gap-3">
			<h1 class="text-2xl font-semibold">{formatMoney(data.wallet.balance, data.wallet.currency)}</h1>
			{#if data.reconciliation?.consistent}
				<span class="badge badge-soft badge-success">ledger confere com o saldo</span>
			{:else}
				<span class="badge badge-soft badge-error">diferença {formatAmount(data.reconciliation?.difference)}</span>
			{/if}
		</div>
		<dl class="grid gap-x-6 gap-y-3 sm:grid-cols-3">
			<div><dt class="text-xs opacity-60">walletId</dt><dd class="font-mono text-xs break-all">{data.wallet.id}</dd></div>
			<div><dt class="text-xs opacity-60">playerId</dt><dd class="font-mono text-xs break-all">{data.wallet.playerId}</dd></div>
			<div>
				<dt class="text-xs opacity-60">version / lançamentos</dt>
				<dd class="font-mono text-xs">{data.wallet.version} · {data.wallet.ledgerEntries}</dd>
			</div>
		</dl>
	</div>
</section>

<div class="mt-5 grid gap-5 xl:grid-cols-2">
	<section class="card bg-base-200 border-base-300 border">
		<div class="card-body">
			<h2 class="card-title text-base">Ledger</h2>
			<div class="overflow-x-auto">
				<table class="table table-sm table-zebra">
					<thead><tr><th>Quando</th><th>Kind</th><th>Direção</th><th class="text-right">Valor</th><th class="text-right">Antes</th><th class="text-right">Depois</th></tr></thead>
					<tbody>
						{#each data.ledger as entry (entry.id)}
							<tr>
								<td class="font-mono text-xs">{new Date(entry.createdAt).toLocaleString('pt-BR')}</td>
								<td class="font-mono text-xs">{entry.kind}</td>
								<td>
									<span class="badge badge-soft badge-sm {entry.direction === 'CREDIT' ? 'badge-success' : 'badge-warning'} font-mono">{entry.direction}</span>
								</td>
								<td class="text-right font-mono">{formatAmount(entry.amount)}</td>
								<td class="text-right font-mono">{formatAmount(entry.balanceBefore)}</td>
								<td class="text-right font-mono">{formatAmount(entry.balanceAfter)}</td>
							</tr>
						{:else}
							<tr><td colspan="6" class="opacity-60">Nenhum lançamento.</td></tr>
						{/each}
					</tbody>
				</table>
			</div>
		</div>
	</section>

	<section class="card bg-base-200 border-base-300 border">
		<div class="card-body">
			<h2 class="card-title text-base">Transações</h2>
			<div class="overflow-x-auto">
				<table class="table table-sm table-zebra">
					<thead><tr><th>Quando</th><th>Kind</th><th class="text-right">Valor</th><th>Status</th><th>failureCode</th><th></th></tr></thead>
					<tbody>
						{#each data.transactions as transaction (transaction.id)}
							<tr>
								<td class="font-mono text-xs">{new Date(transaction.createdAt).toLocaleString('pt-BR')}</td>
								<td class="font-mono text-xs">{transaction.kind}</td>
								<td class="text-right font-mono">{formatAmount(transaction.amount)}</td>
								<td><TxStatus status={transaction.status} /></td>
								<td class="font-mono text-xs">{transaction.failureCode ?? ''}</td>
								<td><a class="btn btn-ghost btn-xs" href="/admin/transactions/{transaction.id}">abrir</a></td>
							</tr>
						{:else}
							<tr><td colspan="6" class="opacity-60">Nenhuma transação.</td></tr>
						{/each}
					</tbody>
				</table>
			</div>
		</div>
	</section>
</div>
