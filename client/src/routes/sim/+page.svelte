<script lang="ts">
	import { onMount } from 'svelte';
	import StatusBadge from '$lib/components/StatusBadge.svelte';
	import { robot, type Command } from '$lib/robot-state.svelte';
	import { RobotSocket } from '$lib/ws';

	// This page is the temporary robot implementation. Later the Pi process can
	// connect with the same role and consume the same commands instead.
	const simSocket = new RobotSocket('robot');
	const maxSpeedPxS = 220;
	const maxTurnDegS = 180;
	const boxSize = 34;

	let stage: HTMLElement;
	let position = $state({ x: 0, y: 0 });
	let drive = $state({ forward: 0, turn: 0 });
	let heading = $state(0);
	let mode = $state('idle');
	let ready = $state(false);

	let frame = 0;
	let statusTimer: ReturnType<typeof setInterval> | null = null;
	let lastFrameAt = 0;

	let boxStyle = $derived(
		`transform: translate(${position.x}px, ${position.y}px) rotate(${heading}deg);`
	);

	$effect(() => {
		if (robot.connectionStatus !== 'connected') {
			stop('offline');
		}
	});

	onMount(() => {
		const host = window.location.hostname || 'localhost';
		const unsubscribeCommand = simSocket.on('command', (message) => {
			if (message.command) {
				applyCommand(message.command);
			}
		});

		simSocket.connect(`ws://${host}:8080/ws`);
		frame = requestAnimationFrame(tick);
		statusTimer = setInterval(sendStatus, 250);

		return () => {
			unsubscribeCommand();
			cancelAnimationFrame(frame);
			if (statusTimer) {
				clearInterval(statusTimer);
			}
			simSocket.disconnect();
		};
	});

	function applyCommand(command: Command) {
		switch (command.type) {
			case 'move':
				// Commands are interpreted locally; the backend does not know
				// anything about screen size, pixels, or simulated movement.
				drive.forward = command.x ?? 0;
				drive.turn = command.y ?? 0;
				mode = 'moving';
				break;
			case 'reset':
				center();
				stop('idle');
				break;
			case 'stop':
				stop(command.note?.startsWith('failsafe') ? 'failsafe' : 'idle');
				break;
		}
	}

	function tick(now: number) {
		if (!ready) {
			center();
			ready = true;
		}

		const dt = lastFrameAt ? (now - lastFrameAt) / 1000 : 0;
		lastFrameAt = now;

		if (mode === 'moving') {
			// Client-side placeholder physics for testing command flow only.
			const rect = stage.getBoundingClientRect();
			heading = normalizeDegrees(heading + drive.turn * maxTurnDegS * dt);

			const radians = (heading * Math.PI) / 180;
			position.x += Math.sin(radians) * drive.forward * maxSpeedPxS * dt;
			position.y -= Math.cos(radians) * drive.forward * maxSpeedPxS * dt;
			position.x = clamp(position.x, 0, Math.max(0, rect.width - boxSize));
			position.y = clamp(position.y, 0, Math.max(0, rect.height - boxSize));
		}

		frame = requestAnimationFrame(tick);
	}

	function center() {
		const rect = stage.getBoundingClientRect();
		position.x = Math.max(0, rect.width / 2 - boxSize / 2);
		position.y = Math.max(0, rect.height / 2 - boxSize / 2);
		heading = 0;
	}

	function stop(nextMode: string) {
		drive.forward = 0;
		drive.turn = 0;
		mode = nextMode;
	}

	function sendStatus() {
		// Status is optional feedback for the control UI; real hardware can
		// later report its own mode without changing the command protocol.
		simSocket.sendStatus({
			mode,
			x: position.x,
			y: position.y,
			heading
		});
	}

	function clamp(value: number, min: number, max: number) {
		return Math.min(max, Math.max(min, value));
	}

	function normalizeDegrees(value: number) {
		const next = value % 360;
		return next < 0 ? next + 360 : next;
	}
</script>

<svelte:head>
	<title>Robot Sim</title>
</svelte:head>

<main class="grid min-h-screen grid-rows-[auto_1fr] bg-slate-950 text-white">
	<header class="flex flex-col gap-3 border-b border-white/10 bg-slate-900 px-4 py-3 sm:flex-row sm:items-center sm:justify-between">
		<div>
			<h1 class="text-xl font-semibold">Robot sim</h1>
			<div class="mt-1 font-mono text-sm text-slate-300">
				{mode} / x {position.x.toFixed(0)} / y {position.y.toFixed(0)} / h {heading.toFixed(0)}
			</div>
		</div>
		<StatusBadge status={robot.connectionStatus} clients={robot.clients} />
	</header>

	<section bind:this={stage} class="relative min-h-0 overflow-hidden bg-slate-950">
		<div class="absolute inset-0 stage-grid"></div>
		<div class="absolute left-3 top-3 rounded-md bg-white/90 px-2 py-1 font-mono text-xs text-slate-800">
			role {robot.role}
		</div>
		<div
			class="absolute left-0 top-0 h-[34px] w-[34px] origin-center rounded-sm border border-white bg-cyan-400 shadow-[0_0_24px_rgba(34,211,238,0.45)]"
			style={boxStyle}
		>
			<div class="absolute -top-1 left-1/2 h-2 w-1 -translate-x-1/2 rounded-full bg-rose-500"></div>
		</div>
	</section>
</main>

<style>
	.stage-grid {
		background-image:
			linear-gradient(rgba(255, 255, 255, 0.08) 1px, transparent 1px),
			linear-gradient(90deg, rgba(255, 255, 255, 0.08) 1px, transparent 1px);
		background-size: 48px 48px;
	}
</style>
