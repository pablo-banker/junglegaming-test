<script lang="ts">
	import { invalidateAll } from '$app/navigation';
	import { page } from '$app/state';

	let { children } = $props();

	const sections = [
		{ href: '/admin', label: 'Visão geral' },
		{ href: '/admin/transactions', label: 'Transactions' },
		{ href: '/admin/wallets', label: 'Wallets' },
		{ href: '/admin/messaging', label: 'Mensageria' }
	];

	let auto = $state(false);

	// Reloads every load function of the page while auto-refresh is on.
	$effect(() => {
		if (!auto) return;

		const timer = setInterval(() => invalidateAll(), 3_000);

		return () => clearInterval(timer);
	});
</script>

<div class="mb-5 flex flex-wrap items-center justify-between gap-3">
	<div role="tablist" class="tabs tabs-box">
		{#each sections as section (section.href)}
			{@const active = section.href === '/admin' ? page.url.pathname === '/admin' : page.url.pathname.startsWith(section.href)}
			<a role="tab" href={section.href} class="tab {active ? 'tab-active' : ''}">{section.label}</a>
		{/each}
	</div>

	<div class="flex items-center gap-3">
		<label class="label text-xs">
			<input type="checkbox" class="toggle toggle-sm toggle-primary" bind:checked={auto} />
			atualizar a cada 3 s
		</label>
		<button class="btn btn-sm" onclick={() => invalidateAll()}>Atualizar</button>
	</div>
</div>

{@render children()}
