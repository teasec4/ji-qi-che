<script lang="ts">
	import type { ClientCounts, ConnectionStatus } from '$lib/robot-state.svelte';

	type Props = {
		status: ConnectionStatus;
		clients: ClientCounts;
	};

	let { status, clients }: Props = $props();

	let label = $derived(
		status === 'connected' ? 'online' : status === 'connecting' ? 'connecting' : 'offline'
	);
	let tone = $derived(
		status === 'connected'
			? 'border-emerald-300 bg-emerald-50 text-emerald-800'
			: status === 'connecting'
				? 'border-amber-300 bg-amber-50 text-amber-800'
				: 'border-rose-300 bg-rose-50 text-rose-800'
	);
</script>

<div class={`inline-flex items-center gap-2 rounded-md border px-3 py-2 text-sm font-medium ${tone}`}>
	<span class="h-2.5 w-2.5 rounded-full bg-current"></span>
	<span>{label}</span>
	<span class="text-current/70">ctrl {clients.controllers} / robot {clients.robots}</span>
</div>
