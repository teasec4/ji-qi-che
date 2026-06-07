<script lang="ts">
	import { onMount } from 'svelte';
	import Joystick from '$lib/components/Joystick.svelte';
	import StatusBadge from '$lib/components/StatusBadge.svelte';
	import { robot, type Command } from '$lib/robot-state.svelte';
	import { socket } from '$lib/ws';

	let scenarioRunning = $state(false);
	let heldMove = $state<{ x: number; y: number } | null>(null);
	let holdTimer: ReturnType<typeof setInterval> | null = null;

	onMount(() => {
		const host = window.location.hostname || 'localhost';
		socket.connect(`ws://${host}:8080/ws`);

		// когда отрабатывает это?
		return () => {
			clearHeldMove();
			socket.disconnect();
		};
	});

	function sendMove(value: { x: number; y: number }) {
		heldMove = value; 
		sendCommand({ type: 'move', x: value.x, y: value.y });
		ensureHoldLoop();
	}

	function sendStop() {
		clearHeldMove();
		sendCommand({ type: 'stop' });
	}

	function sendReset() {
		clearHeldMove();
		sendCommand({ type: 'reset' });
	}

	function sendCommand(command: Command) {
		socket.sendCommand(command);
	}

	// one of scenario for testing
	async function runScenario() {
		if (scenarioRunning) {
			return;
		}

		scenarioRunning = true;
		const steps: Array<{ command: Command; ms: number }> = [
			{ command: { type: 'move', x: 0.85, y: 0 }, ms: 900 },
			{ command: { type: 'move', x: 0.45, y: 0.8 }, ms: 850 },
			{ command: { type: 'move', x: 0.7, y: -0.65 }, ms: 850 },
			{ command: { type: 'stop' }, ms: 350 }
		];

		for (const step of steps) {
			await runTimedCommand(step.command, step.ms);
		}

		scenarioRunning = false;
	}

	
	async function runTimedCommand(command: Command, ms: number) {
		if (command.type !== 'move') {
			socket.sendCommand(command);
			await sleep(ms);
			return;
		}

		// Move commands are "deadman" commands: one click starts movement, then
		// quiet hold packets keep it alive until the scenario step ends.
		const endAt = Date.now() + ms;
		socket.sendCommand(command);
		await sleep(240);

		while (Date.now() < endAt) {
			socket.sendCommand({ ...command, note: 'hold' }, { log: false });
			await sleep(240);
		}
	}

	function ensureHoldLoop() {
		if (holdTimer) {
			return;
		}

		// While the joystick is held, refresh movement faster than backend TTL.
		// If this loop stops, the backend sends a failsafe stop to the robot.
		holdTimer = setInterval(() => {
			if (!heldMove || robot.connectionStatus !== 'connected') {
				clearHeldMove();
				return;
			}

			socket.sendCommand({ type: 'move', x: heldMove.x, y: heldMove.y, note: 'hold' }, { log: false });
		}, 240);
	}

	function clearHeldMove() {
		heldMove = null;
		if (holdTimer) {
			clearInterval(holdTimer);
			holdTimer = null;
		}
	}

	function sleep(ms: number) {
		return new Promise((resolve) => setTimeout(resolve, ms));
	}
</script>

<svelte:head>
	<title>Robot Control Stand</title>
</svelte:head>

<main class="min-h-screen bg-slate-100 text-slate-950">
	<div class="mx-auto flex min-h-screen w-full max-w-7xl flex-col gap-4 px-4 py-4 lg:px-6">
		<header class="flex flex-col gap-3 border-b border-slate-300 pb-4 sm:flex-row sm:items-center sm:justify-between">
			<div>
				<h1 class="text-2xl font-semibold tracking-normal">Robot control stand</h1>
				<p class="mt-1 text-sm text-slate-600">Pi brain simulator, WebSocket commands, virtual robot state</p>
			</div>
			<StatusBadge status={robot.connectionStatus} clients={robot.clients} />
		</header>

		<div class="grid flex-1 grid-cols-[minmax(0,1fr)] gap-4 lg:grid-cols-[360px_minmax(0,1fr)]">
			<section class="min-w-0 flex flex-col gap-4 rounded-md border border-slate-300 bg-white p-4">
				<div class="flex items-center justify-between">
					<h2 class="text-lg font-semibold">Control</h2>
					<button
						class="rounded-md border border-rose-300 bg-rose-50 px-3 py-2 text-sm font-semibold text-rose-700 hover:bg-rose-100 disabled:cursor-not-allowed disabled:opacity-50"
						disabled={robot.connectionStatus !== 'connected'}
						onclick={sendStop}
					>
						STOP
					</button>
				</div>

				<Joystick disabled={robot.connectionStatus !== 'connected'} onmove={sendMove} onstop={sendStop} />

				<div class="grid grid-cols-2 gap-2">
					<button
						class="rounded-md border border-slate-300 px-3 py-2 text-sm font-medium hover:bg-slate-100 disabled:cursor-not-allowed disabled:opacity-50"
						disabled={robot.connectionStatus !== 'connected'}
						onclick={() => sendCommand({ type: 'move', x: 0.75, y: 0 })}
					>
						Forward
					</button>
					<button
						class="rounded-md border border-slate-300 px-3 py-2 text-sm font-medium hover:bg-slate-100 disabled:cursor-not-allowed disabled:opacity-50"
						disabled={robot.connectionStatus !== 'connected'}
						onclick={() => sendCommand({ type: 'move', x: -0.45, y: 0 })}
					>
						Reverse
					</button>
					<button
						class="rounded-md border border-slate-300 px-3 py-2 text-sm font-medium hover:bg-slate-100 disabled:cursor-not-allowed disabled:opacity-50"
						disabled={robot.connectionStatus !== 'connected'}
						onclick={() => sendCommand({ type: 'move', x: 0.35, y: -0.9 })}
					>
						Turn left
					</button>
					<button
						class="rounded-md border border-slate-300 px-3 py-2 text-sm font-medium hover:bg-slate-100 disabled:cursor-not-allowed disabled:opacity-50"
						disabled={robot.connectionStatus !== 'connected'}
						onclick={() => sendCommand({ type: 'move', x: 0.35, y: 0.9 })}
					>
						Turn right
					</button>
					<button
						class="rounded-md border border-slate-300 px-3 py-2 text-sm font-medium hover:bg-slate-100 disabled:cursor-not-allowed disabled:opacity-50"
						disabled={robot.connectionStatus !== 'connected' || scenarioRunning}
						onclick={runScenario}
					>
						Scenario
					</button>
					<button
						class="rounded-md border border-slate-300 px-3 py-2 text-sm font-medium hover:bg-slate-100 disabled:cursor-not-allowed disabled:opacity-50"
						disabled={robot.connectionStatus !== 'connected'}
						onclick={sendReset}
					>
						Reset
					</button>
				</div>
			</section>

			<section class="grid min-w-0 grid-cols-[minmax(0,1fr)] gap-4 xl:grid-cols-[minmax(0,1fr)_360px]">
				<div class="min-w-0 rounded-md border border-slate-300 bg-white p-4">
					<div class="flex items-center justify-between gap-3">
						<h2 class="text-lg font-semibold">Robot client</h2>
						<a
							class="rounded-md border border-slate-300 px-3 py-2 text-sm font-medium hover:bg-slate-100"
							href="/sim"
							target="_blank"
							rel="noreferrer"
						>
							Open sim
						</a>
					</div>

					<div class="mt-4 grid gap-3 text-sm sm:grid-cols-2">
						<div class="rounded-md border border-slate-200 p-3">
							<div class="text-slate-500">robot clients</div>
							<div class="mt-1 text-2xl font-semibold">{robot.clients.robots}</div>
						</div>
						<div class="rounded-md border border-slate-200 p-3">
							<div class="text-slate-500">last mode</div>
							<div class="mt-1 text-2xl font-semibold">{robot.lastRobotStatus?.mode ?? 'none'}</div>
						</div>
						<div class="rounded-md border border-slate-200 p-3">
							<div class="text-slate-500">sim position</div>
							<div class="mt-1 font-mono text-lg">
								{robot.lastRobotStatus
									? `${robot.lastRobotStatus.x.toFixed(0)}, ${robot.lastRobotStatus.y.toFixed(0)}`
									: 'n/a'}
							</div>
						</div>
						<div class="rounded-md border border-slate-200 p-3">
							<div class="text-slate-500">heading</div>
							<div class="mt-1 font-mono text-lg">
								{robot.lastRobotStatus ? `${robot.lastRobotStatus.heading.toFixed(0)} deg` : 'n/a'}
							</div>
						</div>
					</div>
				</div>

				<div class="min-w-0 flex flex-col gap-4">
					<section class="min-h-72 min-w-0 rounded-md border border-slate-300 bg-white p-4">
						<h2 class="mb-3 text-lg font-semibold">Command log</h2>
						<div class="flex max-h-80 flex-col gap-2 overflow-auto pr-1 text-sm">
							{#if robot.commandLog.length === 0}
								<div class="rounded-md border border-dashed border-slate-300 px-3 py-6 text-center text-slate-500">
									No messages yet
								</div>
							{:else}
								{#each robot.commandLog as item (item.id)}
									<div class="grid grid-cols-[76px_1fr] gap-2 border-b border-slate-100 pb-2">
										<span class="font-mono text-xs text-slate-500">{item.at}</span>
										<span class="break-words font-mono text-xs text-slate-800">{item.text}</span>
									</div>
								{/each}
							{/if}
						</div>
					</section>
				</div>
			</section>
		</div>
	</div>
</main>
