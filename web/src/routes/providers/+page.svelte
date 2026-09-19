<script lang="ts">
	import { invalidateAll } from '$app/navigation';

	import StatusDot from '$lib/components/StatusDot.svelte';

	let { data } = $props();

	const ready = $derived(data.instances.filter((instance) => instance.ready));
</script>

<div class="mb-6 flex flex-wrap items-end justify-between gap-4">
	<div>
		<h1 class="text-2xl font-semibold">Providers</h1>
		<p class="text-sm opacity-60">Clients do realm que enviam operações pela API.</p>
	</div>

	<div class="stats bg-base-200">
		<div class="stat py-3">
			<div class="stat-title text-xs">Instâncias da API</div>
			<div class="stat-value text-2xl">{ready.length}<span class="text-base opacity-50">/{data.instances.length}</span></div>
			<div class="stat-desc">requisições distribuídas entre as ativas</div>
		</div>
	</div>
</div>

{#if data.providers.length === 0}
	<div class="alert alert-warning">
		<span>Nenhum provider configurado: defina KEYCLOAK_PROVIDER_&lt;X&gt;_CLIENT_ID e _CLIENT_SECRET no .env.</span>
	</div>
{/if}

<div class="grid gap-4 sm:grid-cols-2 xl:grid-cols-3">
	{#each data.providers as provider (provider.id)}
		<div class="card bg-base-200 border-base-300 border">
			<div class="card-body gap-3">
				<div class="flex items-start justify-between gap-3">
					<div>
						<h2 class="card-title">{provider.name}</h2>
						<p class="font-mono text-xs opacity-60">{provider.clientId}</p>
					</div>
					<StatusDot status={provider.status} />
				</div>

				{#if provider.status === 'RUNNING'}
					<p class="text-sm opacity-70">Token obtido no Keycloak com client_credentials.</p>
					<div class="card-actions">
						<a href="/providers/{provider.id}" class="btn btn-primary btn-sm">Abrir</a>
					</div>
				{:else}
					<p class="text-error text-xs">{provider.reason}</p>
					<div class="card-actions">
						<button class="btn btn-sm" onclick={() => invalidateAll()}>Verificar de novo</button>
					</div>
				{/if}
			</div>
		</div>
	{/each}
</div>

<div class="card bg-base-200 border-base-300 mt-6 border">
	<div class="card-body">
		<h2 class="card-title text-base">Instâncias</h2>
		<div class="overflow-x-auto">
			<table class="table table-sm">
				<thead><tr><th>Instância</th><th>Estado</th><th>Latência</th></tr></thead>
				<tbody>
					{#each data.instances as instance (instance.url)}
						<tr>
							<td class="font-mono text-xs">{instance.url}</td>
							<td>
								{#if instance.ready}
									<span class="badge badge-soft badge-success badge-sm">ready</span>
								{:else}
									<span class="badge badge-soft badge-error badge-sm">down</span>
								{/if}
							</td>
							<td class="text-xs">{instance.latencyMs} ms</td>
						</tr>
					{/each}
				</tbody>
			</table>
		</div>
	</div>
</div>
