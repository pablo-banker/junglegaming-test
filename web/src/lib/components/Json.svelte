<script lang="ts">
	/** Collapsible JSON block for raw requests, responses and event payloads. */
	let { title, value }: { title: string; value: unknown } = $props();
</script>

<div class="collapse-arrow bg-base-100 border-base-300 collapse border">
	<input type="checkbox" />
	<div class="collapse-title text-xs font-semibold opacity-70">{title}</div>
	<div class="collapse-content">
		<pre class="overflow-x-auto font-mono text-xs leading-relaxed">{JSON.stringify(value, null, 2)}</pre>
	</div>
</div>
