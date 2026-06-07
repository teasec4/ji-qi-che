<script lang="ts">
	type MoveValue = {
		x: number;
		y: number;
	};

	type Props = {
		disabled?: boolean;
		onmove?: (value: MoveValue) => void;
		onstop?: () => void;
	};

	let { disabled = false, onmove, onstop }: Props = $props();

	// что есть что пояснения? 
	let pad: HTMLDivElement;
	let active = $state(false);
	let knobX = $state(0);
	let knobY = $state(0);
	let commandX = $state(0);
	let commandY = $state(0);

	const radius = 76;

	// странно выглядит какбудто немного дергается
	let knobStyle = $derived(
		`transform: translate(calc(-50% + ${knobX}px), calc(-50% + ${knobY}px));`
	);

	function start(event: PointerEvent) {
		if (disabled) {
			return;
		}
		active = true;
		pad.setPointerCapture(event.pointerId);
		update(event);
	}

	function update(event: PointerEvent) {
		if (!active || disabled) {
			return;
		}

		const rect = pad.getBoundingClientRect();
		const centerX = rect.left + rect.width / 2;
		const centerY = rect.top + rect.height / 2;
		const rawX = event.clientX - centerX;
		const rawY = event.clientY - centerY;
		const distance = Math.hypot(rawX, rawY);
		const scale = distance > radius ? radius / distance : 1;

		knobX = rawX * scale;
		knobY = rawY * scale;
		commandX = round(-knobY / radius);
		commandY = round(knobX / radius);
		onmove?.({ x: commandX, y: commandY });
	}

	function end(event: PointerEvent) {
		if (!active) {
			return;
		}
		active = false;
		pad.releasePointerCapture(event.pointerId);
		knobX = 0;
		knobY = 0;
		commandX = 0;
		commandY = 0;
		onstop?.();
	}

	function round(value: number) {
		return Math.round(value * 100) / 100;
	}
</script>

<div class="flex flex-col gap-3">
	<div
		bind:this={pad}
		role="slider"
		aria-label="Joystick forward axis"
		aria-valuemin="-1"
		aria-valuemax="1"
		aria-valuenow={commandX}
		tabindex="0"
		class:opacity-50={disabled}
		class="relative aspect-square w-full max-w-72 touch-none select-none rounded-md border border-slate-300 bg-slate-100 shadow-inner"
		onpointerdown={start}
		onpointermove={update}
		onpointerup={end}
		onpointercancel={end}
	>
		<div class="absolute left-1/2 top-0 h-full w-px bg-slate-300"></div>
		<div class="absolute left-0 top-1/2 h-px w-full bg-slate-300"></div>
		<div class="absolute left-1/2 top-3 -translate-x-1/2 text-xs font-medium text-slate-500">FWD</div>
		<div class="absolute bottom-3 left-1/2 -translate-x-1/2 text-xs font-medium text-slate-500">REV</div>
		<div class="absolute left-3 top-1/2 -translate-y-1/2 text-xs font-medium text-slate-500">L</div>
		<div class="absolute right-3 top-1/2 -translate-y-1/2 text-xs font-medium text-slate-500">R</div>
		<div
			class="absolute left-1/2 top-1/2 h-20 w-20 rounded-full border border-slate-900 bg-white shadow-lg transition-transform duration-75"
			style={knobStyle}
		></div>
	</div>

	<div class="grid grid-cols-2 gap-2 text-sm">
		<div class="rounded-md border border-slate-200 bg-white px-3 py-2">
			<span class="text-slate-500">x</span>
			<span class="float-right font-mono text-slate-950">{commandX.toFixed(2)}</span>
		</div>
		<div class="rounded-md border border-slate-200 bg-white px-3 py-2">
			<span class="text-slate-500">y</span>
			<span class="float-right font-mono text-slate-950">{commandY.toFixed(2)}</span>
		</div>
	</div>
</div>
