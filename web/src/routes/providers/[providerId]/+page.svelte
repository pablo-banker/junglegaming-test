<script lang="ts">
	import { untrack } from 'svelte';

	import { enhance } from '$app/forms';

	import OperationEffect from '$lib/components/OperationEffect.svelte';
	import StatusDot from '$lib/components/StatusDot.svelte';
	import TxStatus from '$lib/components/TxStatus.svelte';
	import { AMOUNT_PATTERN, formatMoney } from '$lib/money';
	import type { OperationResult, TokenInfo, WagerKind, WagerPayload, WalletView } from '$lib/types';

	let { data, form } = $props();

	const kinds: WagerKind[] = ['BET', 'WIN', 'LOSS', 'REFUND', 'ROLLBACK'];

	// Reference rules of the domain: BET and LOSS refuse one, WIN accepts a BET, REFUND and ROLLBACK require one.
	const rules: Record<WagerKind, { need: 'none' | 'optional' | 'required'; from: WagerKind[]; hint: string }> = {
		BET: { need: 'none', from: [], hint: 'Debita a aposta da carteira. A API rejeita um BET que traga referência.' },
		WIN: { need: 'optional', from: ['BET'], hint: 'Credita o prêmio. A referência a um BET processado é opcional e o valor pode ser diferente da aposta.' },
		LOSS: { need: 'none', from: [], hint: 'Fecha a rodada sem movimento: amount 0.00, sem referência, saldo e wallet version não mudam.' },
		REFUND: { need: 'required', from: ['BET'], hint: 'Devolve uma aposta: exige um BET processado e o mesmo valor dele.' },
		ROLLBACK: { need: 'required', from: ['BET', 'WIN', 'REFUND'], hint: 'Desfaz uma operação processada com o mesmo valor dela.' }
	};

	/** Short random suffix for generated identifiers. */
	const shortId = () => crypto.randomUUID().slice(0, 8);

	const firstWallet = untrack(() => data.wallets[0]);

	let op = $state({
		walletId: firstWallet?.id ?? '',
		playerId: firstWallet?.playerId ?? '',
		kind: 'BET' as WagerKind,
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

	// The operation picked as reference; while it is set, the fields it filled stay locked.
	let linked = $state<WagerPayload | undefined>(undefined);

	// Remembers the amount typed before a LOSS, which is always zero, to restore it afterwards.
	let typedAmount = '25.00';

	const token = $derived((form && 'token' in form ? (form.token as TokenInfo) : undefined) ?? data.token);

	const rule = $derived(rules[op.kind]);

	/** Requests of this session that the API processed, newest first and without replay duplicates. */
	const processed = $derived([
		...new Map(
			history
				.filter((result) => (result.response.body as { status?: string } | null)?.status === 'PROCESSED')
				.map((result) => [result.request.externalTransactionId, result.request] as const)
		).values()
	]);

	/** Processed operations that the current kind may reference. */
	const candidates = $derived(processed.filter((request) => rule.from.includes(request.kind)));

	// The API compares wallet, player, round, currency and amount with the reference (REFERENCE_MISMATCH).
	const contextLocked = $derived(linked !== undefined);
	const amountLocked = $derived(op.kind === 'LOSS' || (linked !== undefined && rule.need === 'required'));

	/** Wallet movement a rollback produces, which is the opposite of the referenced operation. */
	const rollbackDirection = $derived(linked && linked.kind === 'BET' ? 'CREDIT' : 'DEBIT');

	/** Blocks only what the API would certainly reject; the rest is left to the backend. */
	const blocked = $derived(
		(rule.need === 'required' && !op.referenceExternalTransactionId) ||
			(op.kind === 'LOSS' ? op.amount !== '0.00' : !AMOUNT_PATTERN.test(op.amount) || op.amount === '0.00')
	);

	/** Fills the operation with a wallet of the test list. */
	function useWallet(wallet: { id: string; playerId: string; view?: WalletView }) {
		op.walletId = wallet.id;
		op.playerId = wallet.playerId;
		if (wallet.view) op.currency = wallet.view.balance.currency;
	}

	/** Makes the operation unique again, so the next send is not an idempotent replay. */
	function renewIdentifiers() {
		op.externalTransactionId = `tx-${shortId()}`;
		customKey = null;
	}

	/** Drops the reference and unlocks the fields it had filled. */
	function unlink() {
		linked = undefined;
		op.referenceExternalTransactionId = '';
	}

	/** Switches the operation kind and applies its rules to the form. */
	function chooseKind(kind: WagerKind) {
		op.kind = kind;
		unlink();

		if (kind === 'LOSS') {
			typedAmount = op.amount === '0.00' ? typedAmount : op.amount;
			op.amount = '0.00';
		} else if (op.amount === '0.00') {
			op.amount = typedAmount;
		}

		renewIdentifiers();
	}

	/** Copies the context of a processed operation; REFUND and ROLLBACK also copy its amount. */
	function selectReference(request: WagerPayload) {
		linked = request;
		op.referenceExternalTransactionId = request.externalTransactionId;
		op.walletId = request.walletId;
		op.playerId = request.playerId;
		op.roundId = request.roundId;
		op.currency = request.money.currency;

		// A WIN pays a prize that does not have to match the bet, so only the required references copy the amount.
		if (rule.need === 'required') op.amount = request.money.amount;
	}

	/** Applies the reference chosen in the selector. */
	function pickReference(externalTransactionId: string) {
		const request = candidates.find((candidate) => candidate.externalTransactionId === externalTransactionId);

		if (request) selectReference(request);
		else unlink();
	}

	/** Loads exactly the same request, including its key, to observe the replay. */
	function repeat(result: OperationResult) {
		op.kind = result.request.kind;
		op.walletId = result.request.walletId;
		op.playerId = result.request.playerId;
		op.amount = result.request.money.amount;
		op.currency = result.request.money.currency;
		op.roundId = result.request.roundId;
		op.gameId = result.request.gameId;
		op.externalTransactionId = result.request.externalTransactionId;
		op.referenceExternalTransactionId = result.request.referenceExternalTransactionId ?? '';
		customKey = result.response.idempotencyKey ?? null;

		linked = candidates.find((candidate) => candidate.externalTransactionId === op.referenceExternalTransactionId);
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
							<button
								class="min-w-0 flex-1 text-left disabled:opacity-60"
								disabled={contextLocked}
								title={contextLocked ? 'a carteira vem da operação referenciada' : 'usar nesta operação'}
								onclick={() => useWallet(wallet)}
							>
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
				<div>
					<div class="join">
						{#each kinds as kind (kind)}
							<label class="btn btn-sm join-item {op.kind === kind ? 'btn-primary' : ''}">
								<input type="radio" name="kind" value={kind} checked={op.kind === kind} onchange={() => chooseKind(kind)} class="hidden" />
								{kind}
							</label>
						{/each}
					</div>
					<p class="mt-2 text-xs opacity-60">{rule.hint}</p>
				</div>

				<div class="grid gap-3 md:grid-cols-2">
					<fieldset class="fieldset">
						<legend class="fieldset-legend">walletId</legend>
						<input class="input input-sm w-full font-mono" class:opacity-60={contextLocked} class:cursor-not-allowed={contextLocked} name="walletId" readonly={contextLocked} bind:value={op.walletId} />
					</fieldset>
					<fieldset class="fieldset">
						<legend class="fieldset-legend">playerId</legend>
						<input class="input input-sm w-full font-mono" class:opacity-60={contextLocked} class:cursor-not-allowed={contextLocked} name="playerId" readonly={contextLocked} bind:value={op.playerId} />
					</fieldset>
					<fieldset class="fieldset">
						<legend class="fieldset-legend">amount</legend>
						<input class="input input-sm w-full font-mono" class:opacity-60={amountLocked} class:cursor-not-allowed={amountLocked} name="amount" readonly={amountLocked} bind:value={op.amount} />
						{#if op.kind === 'LOSS'}
							<p class="label text-xs">LOSS só é aceito com 0.00.</p>
						{:else if amountLocked}
							<p class="label text-xs">O valor é o da operação referenciada.</p>
						{/if}
					</fieldset>
					<fieldset class="fieldset">
						<legend class="fieldset-legend">currency</legend>
						<input class="input input-sm w-full font-mono" class:opacity-60={contextLocked} class:cursor-not-allowed={contextLocked} name="currency" readonly={contextLocked} bind:value={op.currency} />
					</fieldset>
					<fieldset class="fieldset">
						<legend class="fieldset-legend">
							roundId
							{#if !contextLocked}
								<button type="button" class="link link-primary ml-1" onclick={() => (op.roundId = `round-${shortId()}`)}>nova</button>
							{/if}
						</legend>
						<input class="input input-sm w-full font-mono" class:opacity-60={contextLocked} class:cursor-not-allowed={contextLocked} name="roundId" readonly={contextLocked} bind:value={op.roundId} />
					</fieldset>
					<fieldset class="fieldset">
						<legend class="fieldset-legend">gameId</legend>
						<input class="input input-sm w-full font-mono" name="gameId" bind:value={op.gameId} />
					</fieldset>
					<fieldset class="fieldset">
						<legend class="fieldset-legend">
							externalTransactionId
							<button type="button" class="link link-primary ml-1" onclick={renewIdentifiers}>novo</button>
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
						<legend class="fieldset-legend">
							referenceExternalTransactionId
							{#if rule.need === 'required'}<span class="badge badge-xs badge-soft badge-warning">obrigatória</span>{/if}
							{#if rule.need === 'optional'}<span class="badge badge-xs badge-soft">opcional</span>{/if}
						</legend>

						<!-- The value travels in a hidden field so a replayed reference survives even outside the options. -->
						<input type="hidden" name="referenceExternalTransactionId" value={rule.need === 'none' ? '' : op.referenceExternalTransactionId} />

						{#if rule.need === 'none'}
							<input class="input input-sm w-full cursor-not-allowed font-mono opacity-60" value="" readonly placeholder="{op.kind} não aceita referência" />
						{:else}
							<select
								class="select select-sm w-full font-mono"
								disabled={candidates.length === 0}
								value={op.referenceExternalTransactionId}
								onchange={(event) => pickReference(event.currentTarget.value)}
								aria-label="operação referenciada"
							>
								{#if rule.need === 'optional'}
									<option value="">sem referência</option>
								{:else if !op.referenceExternalTransactionId}
									<option value="" disabled>escolha a operação referenciada</option>
								{/if}
								{#each candidates as candidate (candidate.externalTransactionId)}
									<option value={candidate.externalTransactionId}>
										{candidate.kind}
										{formatMoney(candidate.money.amount, candidate.money.currency)} · {candidate.externalTransactionId} · {candidate.roundId}
									</option>
								{/each}
							</select>

							{#if candidates.length === 0}
								<p class="label text-warning text-xs">
									Nenhuma {rule.from.join(' ou ')} processada nesta sessão; envie uma primeiro.
								</p>
							{:else if linked}
								<p class="label text-xs">
									Vinculada a {linked.kind} de {formatMoney(linked.money.amount, linked.money.currency)}; walletId, playerId, roundId e currency ficam travados para evitar REFERENCE_MISMATCH.
								</p>
							{/if}

							{#if op.kind === 'ROLLBACK'}
								<p class="label text-xs">
									{#if linked}
										rollback de {linked.kind} → <span class="font-mono">{rollbackDirection}</span> na carteira
									{:else}
										rollback de BET → CREDIT; rollback de WIN ou REFUND → DEBIT
									{/if}
								</p>
							{/if}
						{/if}
					</fieldset>
				</div>

				<button class="btn btn-primary" disabled={sending || blocked || data.status.status !== 'RUNNING'}>
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
							{@const referenceable = candidates.some((candidate) => candidate.externalTransactionId === result.request.externalTransactionId)}
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
									{#if referenceable}
										<button
											class="btn btn-ghost btn-xs"
											title="usar como referência do {op.kind}"
											onclick={(event) => {
												event.stopPropagation();
												selectReference(result.request);
											}}
										>
											referenciar
										</button>
									{/if}
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
