import {
	addLog,
	robot,
	type ClientRole,
	type Command,
	type RobotStatus,
	type ServerMessage
} from './robot-state.svelte';

type MessageHandler = (data: ServerMessage) => void;

type ClientMessage =
  Command |
  { type: 'hello', role: ClientRole } |
  { type: 'status', status: RobotStatus }

export class RobotSocket {
	private ws: WebSocket | null = null;
	private handlers = new Map<string, MessageHandler[]>();
	private reconnectTimer: ReturnType<typeof setTimeout> | null = null;
	private url = '';
	private manualClose = false;

	// The same socket class is used by both sides of the test bench:
	// "/" creates a controller socket, "/sim" creates a robot socket.
	constructor(private readonly role: ClientRole) {}

	connect(url: string) {
		this.url = url;
		this.manualClose = false;

		// check if already connected, and if so, do nothing
		if (
			this.ws &&
			(this.ws.readyState === WebSocket.CONNECTING || this.ws.readyState === WebSocket.OPEN)
		) {
			return;
		}

		robot.connectionStatus = 'connecting';
		this.ws = new WebSocket(url);

		this.ws.onopen = () => {
			robot.connectionStatus = 'connected';
			robot.role = this.role;
			// Role registration lets the backend route commands only to robots.
			this.rawSend({ type: 'hello', role: this.role });
			addLog(`ws connected as ${this.role}`);
		};

		this.ws.onmessage = (event) => {
			const msg = this.parse(event.data);
			if (!msg) {
				return;
			}

			this.applyMessage(msg);
			this.handlers.get(msg.type)?.forEach((handler) => handler(msg));
		};

		this.ws.onerror = () => {
			addLog('ws error');
		};

		this.ws.onclose = () => {
			this.ws = null;
			robot.connectionStatus = 'disconnected';
			addLog('ws disconnected');

			if (!this.manualClose) {
				this.reconnectTimer = setTimeout(() => this.connect(this.url), 1500);
			}
		};
	}

	sendCommand(command: Command, options: { log?: boolean } = {}) {
		if (!this.rawSend(command)) {
			addLog(`skipped ${command.type}: ws offline`);
			return;
		}

		if (options.log !== false) {
			addLog(`send ${command.type} x=${format(command.x)} y=${format(command.y)}`);
		}
	}

	sendStatus(status: RobotStatus) {
		// Only robot clients send status; the server just forwards it to UIs.
		this.rawSend({ type: 'status', status });
	}

	on(type: string, handler: MessageHandler) {
		const handlers = this.handlers.get(type) ?? [];
		handlers.push(handler);
		this.handlers.set(type, handlers);

		return () => {
			const nextHandlers = (this.handlers.get(type) ?? []).filter((item) => item !== handler);
			this.handlers.set(type, nextHandlers);
		};
	}

	disconnect() {
		this.manualClose = true;
		if (this.reconnectTimer) {
			clearTimeout(this.reconnectTimer);
			this.reconnectTimer = null;
		}
		this.ws?.close();
		this.ws = null;
		robot.connectionStatus = 'disconnected';
		robot.commandLog = [];
	}

	// а может сюда можно тип добавить? Command 
	// или ClientMessage с общей типизацией
	private rawSend(payload: ClientMessage) {
		if (!this.ws || this.ws.readyState !== WebSocket.OPEN) {
			return false;
		}
		this.ws.send(JSON.stringify(payload));
		return true;
	}

	// почему СТринг? 
	private parse(data: string): ServerMessage | null {
		try {
			return JSON.parse(data) as ServerMessage;
		} catch {
			addLog('bad ws payload');
			return null;
		}
	}

	private applyMessage(msg: ServerMessage) {
		if (msg.clients) {
			robot.clients = msg.clients;
		}
		if (msg.role) {
			robot.role = msg.role;
		}
		if (msg.status) {
			robot.lastRobotStatus = msg.status;
		}
		if (msg.command && msg.command.note !== 'hold') {
			// Heartbeat move commands are intentionally hidden from the log.
			const prefix = msg.type === 'command' ? 'recv' : msg.type;
			addLog(`${prefix} ${msg.command.type} x=${format(msg.command.x)} y=${format(msg.command.y)}`);
		}
	}
}

function format(value: number | undefined) {
	return typeof value === 'number' ? value.toFixed(2) : '0.00';
}

export const socket = new RobotSocket('controller');
