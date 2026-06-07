export type ConnectionStatus = 'disconnected' | 'connecting' | 'connected';
export type ClientRole = 'controller' | 'robot';
export type CommandType = 'move' | 'stop' | 'reset';

export type Command = {
	type: CommandType;
	x?: number;
	y?: number;
	note?: string;
};

export type ClientCounts = {
	controllers: number;
	robots: number;
};

export type RobotStatus = {
	mode: string;
	x: number;
	y: number;
	heading: number;
};

export type ServerMessage = {
	type: string;
	role?: ClientRole;
	clients: ClientCounts;
	command?: Command;
	status?: RobotStatus;
	message?: string;
};

export type LogEntry = {
	id: number;
	at: string;
	text: string;
};

type RobotStore = {
	connectionStatus: ConnectionStatus;
	role: ClientRole;
	clients: ClientCounts;
	lastRobotStatus: RobotStatus | null;
	commandLog: LogEntry[];
};

export const robot = $state<RobotStore>({
	connectionStatus: 'disconnected',
	role: 'controller',
	clients: {
		controllers: 0,
		robots: 0
	},
	lastRobotStatus: null,
	commandLog: []
});

let logId = 0;

export function addLog(text: string) {
	const now = new Date().toLocaleTimeString();
	robot.commandLog = [{ id: ++logId, at: now, text }, ...robot.commandLog].slice(0, 14);
}
