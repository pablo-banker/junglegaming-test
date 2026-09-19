<script lang="ts">
	/** Dashboard counter. */
	let { label, value, hint, alert = false }: { label: string; value: string | number; hint?: string; alert?: boolean } = $props();
</script>

<div class="stats bg-base-200 {alert ? 'outline-error outline-1' : ''} w-full">
	<div class="stat">
		<div class="stat-title text-xs">{label}</div>
		<div class="stat-value text-3xl {alert ? 'text-error' : ''}">{value}</div>
		{#if hint}<div class="stat-desc">{hint}</div>{/if}
	</div>
</div>
