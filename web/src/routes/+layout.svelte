<script lang="ts">
	import '../app.css';

	import { page } from '$app/state';

	let { children } = $props();

	const areas = [
		{ href: '/providers', label: 'Providers', hint: 'emular um provider' },
		{ href: '/admin', label: 'Admin', hint: 'observar o sistema' }
	];
</script>

<div class="bg-base-100 min-h-screen">
	<div class="navbar bg-base-200 border-base-300 border-b px-4 sm:px-8">
		<div class="navbar-start">
			<a href="/providers" class="text-lg font-semibold tracking-tight sm:text-xl">
				BetMoney <span class="text-primary">Management</span>
			</a>
		</div>
		<div class="navbar-end">
			<span class="hidden text-xs opacity-60 sm:inline">ferramenta de desenvolvimento do desafio</span>
		</div>
	</div>

	<div class="mx-auto flex max-w-[110rem] flex-col gap-6 p-4 sm:p-6 lg:flex-row">
		<aside class="lg:w-56 lg:shrink-0">
			<ul class="menu menu-horizontal bg-base-200 rounded-box w-full lg:menu-vertical">
				{#each areas as area (area.href)}
					{@const active = page.url.pathname.startsWith(area.href)}
					<li>
						<a href={area.href} class:menu-active={active}>
							<span class="flex flex-col items-start leading-tight">
								<span class="font-medium">{area.label}</span>
								<span class="text-xs opacity-60">{area.hint}</span>
							</span>
						</a>
					</li>
				{/each}
			</ul>
		</aside>

		<main class="min-w-0 flex-1">{@render children()}</main>
	</div>
</div>
