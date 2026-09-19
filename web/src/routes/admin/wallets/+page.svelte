<script lang="ts">
	import { formatAmount } from '$lib/money';

	let { data } = $props();
</script>

<section class="card bg-base-200 border-base-300 border">
	<div class="card-body">
		<h2 class="card-title text-base">Wallets</h2>
		<div class="overflow-x-auto">
			<table class="table table-sm table-zebra">
				<thead>
					<tr>
						<th>Wallet</th><th>Player</th><th>Moeda</th><th class="text-right">Saldo</th>
						<th class="text-right">Version</th><th class="text-right">Lançamentos</th><th>Ledger</th><th>Atualizada</th><th></th>
					</tr>
				</thead>
				<tbody>
					{#each data.wallets as wallet (wallet.id)}
						<tr>
							<td class="font-mono text-xs">{wallet.id}</td>
							<td class="font-mono text-xs">{wallet.playerId}</td>
							<td>{wallet.currency}</td>
							<td class="text-right font-mono">{formatAmount(wallet.balance)}</td>
							<td class="text-right font-mono">{wallet.version}</td>
							<td class="text-right font-mono">{wallet.ledgerEntries}</td>
							<td>
								{#if wallet.reconciliation?.consistent}
									<span class="badge badge-soft badge-success badge-sm">confere</span>
								{:else if wallet.reconciliation}
									<span class="badge badge-soft badge-error badge-sm">diferença {formatAmount(wallet.reconciliation.difference)}</span>
								{:else}
									—
								{/if}
							</td>
							<td class="font-mono text-xs">{new Date(wallet.updatedAt).toLocaleString('pt-BR')}</td>
							<td><a class="btn btn-ghost btn-xs" href="/admin/wallets/{wallet.id}">abrir</a></td>
						</tr>
					{:else}
						<tr><td colspan="9" class="opacity-60">Nenhuma carteira ainda.</td></tr>
					{/each}
				</tbody>
			</table>
		</div>
		<p class="text-xs opacity-50">"Ledger" compara o saldo armazenado com créditos menos débitos, numa única consulta.</p>
	</div>
</section>
