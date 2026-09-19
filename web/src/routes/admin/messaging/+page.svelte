<script lang="ts">
	import Json from '$lib/components/Json.svelte';

	let { data } = $props();
</script>

<section class="card bg-base-200 border-base-300 border">
	<div class="card-body">
		<h2 class="card-title text-base">Filas SQS</h2>
		<div class="overflow-x-auto">
			<table class="table table-sm table-zebra">
				<thead>
					<tr><th>Fila</th><th>Papel</th><th class="text-right">Ready</th><th class="text-right">In flight</th><th class="text-right">Delayed</th><th>Visibility</th><th>maxReceive</th></tr>
				</thead>
				<tbody>
					{#each data.queues as queue (queue.name)}
						<tr>
							<td class="font-mono text-xs">{queue.name}</td>
							<td class="text-xs opacity-70">{queue.role}</td>
							<td class="text-right font-mono">{queue.error ? '—' : queue.visible}</td>
							<td class="text-right font-mono">{queue.error ? '—' : queue.inFlight}</td>
							<td class="text-right font-mono">{queue.error ? '—' : queue.delayed}</td>
							<td class="font-mono text-xs">{queue.error ? '—' : `${queue.visibilityTimeout} s`}</td>
							<td class="font-mono text-xs">{queue.maxReceiveCount ?? '—'}</td>
						</tr>
						{#if queue.error}
							<tr><td colspan="7" class="text-error text-xs">{queue.error}</td></tr>
						{/if}
					{/each}
				</tbody>
			</table>
		</div>
		<p class="text-xs opacity-50">Somente GetQueueAttributes: nenhuma mensagem é recebida, escondida ou consumida.</p>
	</div>
</section>

<section class="card bg-base-200 border-base-300 mt-5 border">
	<div class="card-body">
		<div class="flex flex-wrap items-center justify-between gap-3">
			<h2 class="card-title text-base">Outbox</h2>
			<div role="tablist" class="tabs tabs-box tabs-sm">
				<a role="tab" href="/admin/messaging" class="tab {data.pendingOnly ? '' : 'tab-active'}">todos</a>
				<a role="tab" href="/admin/messaging?outbox=pending" class="tab {data.pendingOnly ? 'tab-active' : ''}">só pendentes</a>
			</div>
		</div>

		<div class="overflow-x-auto">
			<table class="table table-sm table-zebra">
				<thead>
					<tr><th>Ocorrido</th><th>Evento</th><th>Aggregate</th><th class="text-right">Tentativas</th><th>Claim</th><th>Publicado</th><th class="text-right">Atraso</th><th>data</th></tr>
				</thead>
				<tbody>
					{#each data.outbox as event (event.eventId)}
						<tr>
							<td class="font-mono text-xs">{new Date(event.occurredAt).toLocaleString('pt-BR')}</td>
							<td class="text-xs">{event.eventType}</td>
							<td class="font-mono text-xs">{event.aggregateId}</td>
							<td class="text-right font-mono">{event.attempts}</td>
							<td class="font-mono text-xs">{event.claimedBy ?? ''}</td>
							<td>
								{#if event.publishedAt}
									<span class="font-mono text-xs">{new Date(event.publishedAt).toLocaleTimeString('pt-BR')}</span>
								{:else}
									<span class="badge badge-soft badge-warning badge-sm">pendente</span>
								{/if}
							</td>
							<td class="text-right font-mono text-xs">{event.publishLagMs === null ? '—' : `${event.publishLagMs} ms`}</td>
							<td class="w-72"><Json title="payload" value={event.payload} /></td>
						</tr>
					{:else}
						<tr><td colspan="8" class="opacity-60">Nenhum evento.</td></tr>
					{/each}
				</tbody>
			</table>
		</div>
	</div>
</section>

<section class="card bg-base-200 border-base-300 mt-5 border">
	<div class="card-body">
		<h2 class="card-title text-base">Inbox</h2>
		<div class="overflow-x-auto">
			<table class="table table-sm table-zebra">
				<thead><tr><th>Recebida</th><th>Consumer</th><th>messageId</th><th>Concluída</th></tr></thead>
				<tbody>
					{#each data.inbox as message (message.messageId)}
						<tr>
							<td class="font-mono text-xs">{new Date(message.receivedAt).toLocaleString('pt-BR')}</td>
							<td class="text-xs">{message.consumerName}</td>
							<td class="font-mono text-xs">{message.messageId}</td>
							<td class="font-mono text-xs">{message.completedAt ? new Date(message.completedAt).toLocaleTimeString('pt-BR') : '—'}</td>
						</tr>
					{:else}
						<tr><td colspan="4" class="opacity-60">Nenhuma mensagem consumida ainda.</td></tr>
					{/each}
				</tbody>
			</table>
		</div>
		<p class="text-xs opacity-50">A inbox garante que uma reentrega da mesma mensagem não movimente dinheiro de novo.</p>
	</div>
</section>
