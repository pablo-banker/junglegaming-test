<script lang="ts">
	/** Provider state as a badge; the label is always visible. */
	let { status }: { status: 'RUNNING' | 'INACTIVE' } = $props();
</script>

<span class="badge badge-soft {status === 'RUNNING' ? 'badge-success' : 'badge-neutral'} gap-2">
	<span class="inline-block size-2 rounded-full {status === 'RUNNING' ? 'bg-success' : 'bg-base-content/40'}"></span>
	{status}
</span>
