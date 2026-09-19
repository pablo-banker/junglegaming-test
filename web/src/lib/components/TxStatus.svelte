<script lang="ts">
	/** Transaction status as a labelled badge. */
	let { status }: { status: string | undefined | null } = $props();

	const tones: Record<string, string> = {
		PROCESSED: 'badge-success',
		REJECTED: 'badge-warning',
		PENDING_REFERENCE: 'badge-info',
		PENDING: 'badge-info',
		FAILED: 'badge-error'
	};
</script>

{#if status}
	<span class="badge badge-soft badge-sm {tones[status] ?? 'badge-neutral'} font-mono">{status}</span>
{:else}
	<span class="opacity-50">—</span>
{/if}
