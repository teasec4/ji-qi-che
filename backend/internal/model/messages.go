package model

type CommandType string

const (
	CommandMove  CommandType = "move"
	CommandStop  CommandType = "stop"
	CommandReset CommandType = "reset"
)

type ClientRole string

const (
	// Controllers are browser UIs that produce commands.
	RoleController ClientRole = "controller"
	// Robots are command consumers: today /sim, later the Pi process.
	RoleRobot ClientRole = "robot"
)

type Command struct {
	Type CommandType `json:"type"`
	X    float64     `json:"x,omitempty"` // -1..1 forward/back
	Y    float64     `json:"y,omitempty"` // -1..1 left/right
	Note string      `json:"note,omitempty"`
}

// ClientMessage is the envelope used for role registration and robot status.
// Plain Command JSON is still accepted by the hub to keep the controller simple.
type ClientMessage struct {
	Type    string       `json:"type"`
	Role    ClientRole   `json:"role,omitempty"`
	Command *Command     `json:"command,omitempty"`
	Status  *RobotStatus `json:"status,omitempty"`
}

type ClientCounts struct {
	Controllers int `json:"controllers"`
	Robots      int `json:"robots"`
}

type RobotStatus struct {
	Mode    string  `json:"mode"`
	X       float64 `json:"x"`
	Y       float64 `json:"y"`
	Heading float64 `json:"heading"`
}

// ServerMessage is intentionally transport-level only. The backend does not
// simulate position; it routes commands and publishes status reported by robots.
type ServerMessage struct {
	Type    string       `json:"type"`
	Role    ClientRole   `json:"role,omitempty"`
	Clients ClientCounts `json:"clients"`
	Command *Command     `json:"command,omitempty"`
	Status  *RobotStatus `json:"status,omitempty"`
	Message string       `json:"message,omitempty"`
}
