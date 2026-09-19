<script lang="ts">
	import { untrack } from 'svelte';

	import { enhance } from '$app/forms';

	import OperationEffect from '$lib/components/OperationEffect.svelte';
	import StatusDot from '$lib/components/StatusDot.svelte';
	import TxStatus from '$lib/components/TxStatus.svelte';
	import { formatMoney } from '$lib/money';
	import type { OperationResult, TokenInfo, WalletView } from '$lib/types';

	let { data, form } = $props();

	const kinds = ['BET', 'WIN', 'LOSS', 'REFUND', 'ROLLBACK'] as const;

	/** Short random suffix for generated identifiers. */
	const shortId = () => crypto.randomUUID().slice(0, 8);

	const firstWallet = untrack(() => data.wallets[0]);

	let op = $state({
		walletId: firstWallet?.id ?? '',
		playerId: firstWallet?.playerId ?? '',
		kind: 'BET' as (typeof kinds)[number],
		amount: '25.00',
		currency: firstWallet?.view?.balance.currency ?? 'BRL',
		roundId: `round-${shortId()}`,
		gameId: 'fortune-chimp',
		externalTransactionId: `tx-${shortId()}`,
		referenceExternalTransactionId: ''
	});

	// The key follows the challenge suggestion ({providerId}:{externalTransactionId}) until edited.
	let customKey = $state<string | null>(null);
	const idempotencyKey = $derived(customKey ?? `${data.provider.id}:${op.externalTransactionId}`);

	let history = $state<OperationResult[]>([]);
	let shown = $state(0);
	let sending = $state(false);

	const token = $derived((form && 'token' in form ? (form.token as TokenInfo) : undefined) ?? data.token);

	/** Fills the operation with a wallet of the test list. */
	function useWallet(wallet: { id: string; playerId: string; view?: WalletView }) {
		op.walletId = wallet.id;
		op.playerId = wallet.playerId;
		if (wallet.view) op.currency = wallet.view.balance.currency;
	}

	/** Prepares a new operation that references a previous one. */
	function reference(result: OperationResult) {
		op.walletId = result.request.walletId;
		op.playerId = result.request.playerId;
		op.roundId = result.request.roundId;
		op.referenceExternalTransactionId = result.request.externalTransactionId;
		op.externalTransactionId = `tx-${shortId()}`;
		customKey = null;
	}

	/** Loads exactly the same request, including its key, to observe the replay. */
	function repeat(result: OperationResult) {
		op.walletId = result.request.walletId;
		op.playerId = result.request.playerId;
		op.kind = result.request.kind;
		op.amount = result.request.money.amount;
		op.currency = result.request.money.currency;
		op.roundId = result.request.roundId;
		op.gameId = result.request.gameId;
		op.externalTransactionId = result.request.externalTransactionId;
		op.referenceExternalTransactionId = result.request.referenceExternalTransactionId ?? '';
		customKey = result.response.idempotencyKey ?? null;
	}
</script>

<div class="mb-5 flex flex-wrap items-center justify-between gap-3">
	<div class="breadcrumbs text-sm">
		<ul>
			<li><a href="/providers">Providers</a></li>
			<li>{data.provider.name}</li>
		</ul>
	</div>
	<div class="flex items-center gap-3">
		<span class="text-xs opacity-60">{data.readyInstances} instância(s) ativas</span>
		<StatusDot status={data.status.status} />
	</div>
</div>

<div class="grid gap-5 xl:grid-cols-[21rem_1fr]">
	<aside class="space-y-5">
		<section class="card bg-base-200 border-base-300 border">
			<div class="card-body gap-3">
				<h2 class="card-title text-base">Identidade</h2>
				<p class="text-sm opacity-70">
					Client <span class="font-mono">{data.provider.clientId}</span> no Keycloak.
				</p>

				<form method="POST" action="?/token" use:enhance>
					<button class="btn btn-sm btn-outline w-full">Obter token (client_credentials)</button>
				</form>

				{#if form && 'tokenError' in form}
					<div class="alert alert-error alert-soft text-xs">{form.tokenError}</div>
				{/if}

				{#if token}
					<div class="bg-base-100 border-base-300 rounded-box border p-3">
						<p class="mb-1 text-xs font-semibold opacity-60">Claims do access token</p>
						<table class="table table-xs">
							<tbody>
								{#each Object.entries(token.claims) as [name, value] (name)}
									<tr>
										<td class="font-mono opacity-60">{name}</td>
										<td class="font-mono break-all">{Array.isArray(value) ? value.join(', ') : String(value ?? '—')}</td>
									</tr>
								{/each}
							</tbody>
						</table>
						<p class="mt-2 text-xs opacity-50">
							Emitido às {new Date(token.issuedAt).toLocaleTimeString('pt-BR')}, válido por {token.expiresIn} s.
						</p>
					</div>
				{/if}
			</div>
		</section>

		<section class="card bg-base-200 border-base-300 border">
			<div class="card-body gap-3">
				<h2 class="card-title text-base">Wallets de teste</h2>

				<form
					method="POST"
					action="?/createWallet"
					class="join w-full"
					use:enhance={() =>
						async ({ result, update }) => {
							if (result.type === 'success') {
								const created = (result.data?.walletCall as { body: WalletView }).body;
								useWallet({ id: created.id, playerId: created.playerId, view: created });
							}
							await update({ reset: false });
						}}
				>
					<input class="input input-sm join-item w-full font-mono" name="amount" value="100.00" aria-label="saldo inicial" />
					<input class="input input-sm join-item w-20 font-mono" name="currency" value="BRL" aria-label="moeda" />
					<button class="btn btn-sm btn-primary join-item">Criar</button>
				</form>

				<form method="POST" action="?/loadWallet" class="join w-full" use:enhance>
					<input class="input input-sm join-item w-full font-mono" name="walletId" placeholder="carregar walletId" />
					<button class="btn btn-sm join-item">Carregar</button>
				</form>

				{#if form && 'walletCall' in form && form.walletCall && form.walletCall.status >= 300}
					<div class="alert alert-error alert-soft text-xs">
						HTTP {form.walletCall.status}: {JSON.stringify(form.walletCall.body)}
					</div>
				{/if}

				<ul class="space-y-2">
					{#each data.wallets as wallet (wallet.id)}
						<li class="bg-base-100 rounded-box flex items-center justify-between gap-2 border p-2 {op.walletId === wallet.id ? 'border-primary' : 'border-base-300'}">
							<button class="min-w-0 flex-1 text-left" onclick={() => useWallet(wallet)}>
								<span class="block truncate font-mono text-xs opacity-60">{wallet.id}</span>
								{#if wallet.view}
									<span class="font-semibold">{formatMoney(wallet.view.balance.amount, wallet.view.balance.currency)}</span>
									<span class="text-xs opacity-60">· version {wallet.view.version}</span>
								{:else}
									<span class="text-error text-xs">não encontrada pela API</span>
								{/if}
							</button>
							<form method="POST" action="?/forgetWallet" use:enhance>
								<input type="hidden" name="walletId" value={wallet.id} />
								<button class="btn btn-ghost btn-xs" title="Remover da lista">✕</button>
							</form>
						</li>
					{:else}
						<li class="text-sm opacity-60">Crie ou carregue uma wallet para começar.</li>
					{/each}
				</ul>
			</div>
		</section>
	</aside>

	<section class="card bg-base-200 border-base-300 border">
		<div class="card-body gap-4">
			<h2 class="card-title text-base">
				Operação
				<span class="badge badge-soft badge-sm font-mono">POST /wagering/transactions</span>
			</h2>

			<form
				method="POST"
				action="?/operate"
				class="space-y-4"
				use:enhance={() => {
					sending = true;

					return async ({ result, update }) => {
						if (result.type === 'success' && result.data?.operation) {
							history.unshift(result.data.operation as OperationResult);
							shown = 0;
						}

						sending = false;
						await update({ reset: false });
					};
				}}
			>
				<div class="join">
					{#each kinds as kind (kind)}
						<label class="btn btn-sm join-item {op.kind === kind ? 'btn-primary' : ''}">
							<input type="radio" name="kind" value={kind} bind:group={op.kind} class="hidden" />
							{kind}
						</label>
					{/each}
				</div>

				<div class="grid gap-3 md:grid-cols-2">
					<fieldset class="fieldset">
						<legend class="fieldset-legend">walletId</legend>
						<input class="input input-sm w-full font-mono" name="walletId" bind:value={op.walletId} />
					</fieldset>
					<fieldset class="fieldset">
						<legend class="fieldset-legend">playerId</legend>
						<input class="input input-sm w-full font-mono" name="playerId" bind:value={op.playerId} />
					</fieldset>
					<fieldset class="fieldset">
						<legend class="fieldset-legend">amount</legend>
						<input class="input input-sm w-full font-mono" name="amount" bind:value={op.amount} />
					</fieldset>
					<fieldset class="fieldset">
						<legend class="fieldset-legend">currency</legend>
						<input class="input input-sm w-full font-mono" name="currency" bind:value={op.currency} />
					</fieldset>
					<fieldset class="fieldset">
						<legend class="fieldset-legend">
							roundId
							<button type="button" class="link link-primary ml-1" onclick={() => (op.roundId = `round-${shortId()}`)}>nova</button>
						</legend>
						<input class="input input-sm w-full font-mono" name="roundId" bind:value={op.roundId} />
					</fieldset>
					<fieldset class="fieldset">
						<legend class="fieldset-legend">gameId</legend>
						<input class="input input-sm w-full font-mono" name="gameId" bind:value={op.gameId} />
					</fieldset>
					<fieldset class="fieldset">
						<legend class="fieldset-legend">
							externalTransactionId
							<button
								type="button"
								class="link link-primary ml-1"
								onclick={() => {
									op.externalTransactionId = `tx-${shortId()}`;
									customKey = null;
								}}
							>
								novo
							</button>
						</legend>
						<input class="input input-sm w-full font-mono" name="externalTransactionId" bind:value={op.externalTransactionId} />
					</fieldset>
					<fieldset class="fieldset">
						<legend class="fieldset-legend">
							Idempotency-Key
							{#if customKey !== null}
								<button type="button" class="link link-primary ml-1" onclick={() => (customKey = null)}>automática</button>
							{/if}
						</legend>
						<input class="input input-sm w-full font-mono" name="idempotencyKey" value={idempotencyKey} oninput={(event) => (customKey = event.currentTarget.value)} />
					</fieldset>
					<fieldset class="fieldset md:col-span-2">
						<legend class="fieldset-legend">referenceExternalTransactionId</legend>
						<input class="input input-sm w-full font-mono" name="referenceExternalTransactionId" bind:value={op.referenceExternalTransactionId} placeholder="vazio = sem referência" />
						<p class="label text-xs">REFUND e ROLLBACK exigem; WIN aceita.</p>
					</fieldset>
				</div>

				<button class="btn btn-primary" disabled={sending || data.status.status !== 'RUNNING'}>
					{#if sending}<span class="loading loading-spinner loading-sm"></span>{/if}
					Enviar {op.kind}
				</button>
			</form>

			{#if history[shown]}
				<OperationEffect operation={history[shown]} />
			{/if}
		</div>
	</section>
</div>

{#if history.length}
	<section class="card bg-base-200 border-base-300 mt-5 border">
		<div class="card-body">
			<h2 class="card-title text-base">Histórico desta sessão</h2>
			<div class="overflow-x-auto">
				<table class="table table-sm table-zebra">
					<thead>
						<tr>
							<th>Hora</th><th>Operação</th><th>externalTransactionId</th><th>HTTP</th>
							<th>Status</th><th>failureCode</th><th>Replay</th><th>Saldo</th><th></th>
						</tr>
					</thead>
					<tbody>
						{#each history as result, index (result.at)}
							{@const body = (result.response.body ?? {}) as { status?: string; failureCode?: string; idempotentReplay?: boolean; balance?: { amount: string } }}
							<tr class="hover:bg-base-300 cursor-pointer {index === shown ? 'bg-base-300' : ''}" onclick={() => (shown = index)}>
								<td class="font-mono text-xs">{new Date(result.at).toLocaleTimeString('pt-BR')}</td>
								<td class="font-mono text-xs">{result.request.kind} {result.request.money.amount}</td>
								<td class="font-mono text-xs">{result.request.externalTransactionId}</td>
								<td class="font-mono">{result.response.status}</td>
								<td><TxStatus status={body.status} /></td>
								<td class="font-mono text-xs">{body.failureCode ?? ''}</td>
								<td class="text-xs">{body.idempotentReplay ? 'sim' : ''}</td>
								<td class="font-mono text-xs">{body.balance?.amount ?? ''}</td>
								<td class="whitespace-nowrap">
									<button
										class="btn btn-ghost btn-xs"
										onclick={(event) => {
											event.stopPropagation();
											reference(result);
										}}
									>
										referenciar
									</button>
									<button
										class="btn btn-ghost btn-xs"
										onclick={(event) => {
											event.stopPropagation();
											repeat(result);
										}}
									>
										repetir
									</button>
								</td>
							</tr>
						{/each}
					</tbody>
				</table>
			</div>
		</div>
	</section>
{/if}
