package handler

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"roboback/internal/model"
	"sort"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const commandTTL = 700 * time.Millisecond

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

type client struct {
	conn    *websocket.Conn
	role    model.ClientRole
	robotID string
	// Gorilla allows one writer at a time per connection.
	writeMu sync.Mutex
}

type robotRuntime struct {
	lastCommand time.Time
	moving      bool
}

type Hub struct {
	mu          sync.Mutex
	clients     map[*websocket.Conn]*client
	robotStates map[string]robotRuntime
}

func NewHub() *Hub {
	return &Hub{
		clients:     make(map[*websocket.Conn]*client),
		robotStates: make(map[string]robotRuntime),
	}
}

func (h *Hub) Run(ctx context.Context) {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			h.checkDeadman()
		}
	}
}

func (h *Hub) HandleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("ws upgrade failed: %v", err)
		return
	}

	h.add(conn)
	defer func() {
		removed := h.remove(conn)
		conn.Close()
		log.Printf("client disconnected: role=%s robot=%s", removed.role, removed.robotID)

		// Losing every controller means no one can intentionally stop the car.
		if removed.role == model.RoleController && h.counts().Controllers == 0 {
			h.forceStopAll("controller disconnected")
		}
		h.broadcastClients()
	}()

	log.Printf("client connected: %s", r.RemoteAddr)
	h.send(conn, model.ServerMessage{
		Type:    "hello",
		Role:    model.RoleController,
		Clients: h.counts(),
		Message: "connected to robot command hub",
	})
	h.broadcastClients()

	for {
		_, payload, err := conn.ReadMessage()
		if err != nil {
			log.Printf("read error (client disconnected): %v", err)
			break
		}

		h.handlePayload(conn, payload)
	}
}

func (h *Hub) handlePayload(conn *websocket.Conn, payload []byte) {
	var msg model.ClientMessage
	if err := json.Unmarshal(payload, &msg); err == nil {
		switch msg.Type {
		case "hello":
			h.setRole(conn, msg.Role, msg.RobotID)
			return
		case "status":
			h.handleStatus(conn, msg.Status)
			return
		case "command":
			h.handleCommandFrom(conn, msg.TargetRobotID, msg.Command)
			return
		}
	}

	var cmd model.Command
	if err := json.Unmarshal(payload, &cmd); err == nil && cmd.Type != "" {
		h.handleCommandFrom(conn, model.DefaultRobotID, &cmd)
		return
	}

	log.Printf("bad ws payload")
}

func (h *Hub) handleStatus(conn *websocket.Conn, status *model.RobotStatus) {
	if status == nil {
		return
	}
	role, robotID := h.clientInfo(conn)
	if role != model.RoleRobot {
		log.Printf("ignored status from non-robot client")
		return
	}

	// Robot status is reported by the robot client and forwarded to UIs.
	h.broadcastToRole(model.RoleController, model.ServerMessage{
		Type:    "robot_status",
		RobotID: robotID,
		Clients: h.counts(),
		Status:  status,
	})
}

func (h *Hub) handleCommandFrom(conn *websocket.Conn, targetRobotID string, cmd *model.Command) {
	if cmd == nil {
		return
	}
	if h.roleOf(conn) != model.RoleController {
		log.Printf("ignored command from non-controller client")
		return
	}

	h.handleCommand(model.NormalizeRobotID(targetRobotID), *cmd)
}

func (h *Hub) handleCommand(robotID string, cmd model.Command) {
	cmd.X = clamp(cmd.X, -1, 1)
	cmd.Y = clamp(cmd.Y, -1, 1)

	switch cmd.Type {
	case model.CommandMove:
		h.mu.Lock()
		state := h.robotStates[robotID]
		state.lastCommand = time.Now()
		state.moving = true
		h.robotStates[robotID] = state
		h.mu.Unlock()
	case model.CommandStop, model.CommandReset:
		h.mu.Lock()
		state := h.robotStates[robotID]
		state.moving = false
		h.robotStates[robotID] = state
		h.mu.Unlock()
	default:
		log.Printf("unknown command: %s", cmd.Type)
		return
	}

	log.Printf("command: robot=%s type=%s x=%.2f y=%.2f note=%s", robotID, cmd.Type, cmd.X, cmd.Y, cmd.Note)
	// The backend is a broker: commands go to robots, acknowledgements go to UIs.
	h.broadcastCommandToRobot(robotID, "command", cmd)
	h.broadcastCommandToControllers(robotID, "command_ack", cmd)
}

func (h *Hub) checkDeadman() {
	h.mu.Lock()
	now := time.Now()
	robotIDs := make([]string, 0)
	for robotID, state := range h.robotStates {
		if state.moving && now.Sub(state.lastCommand) > commandTTL {
			state.moving = false
			h.robotStates[robotID] = state
			robotIDs = append(robotIDs, robotID)
		}
	}
	h.mu.Unlock()

	sort.Strings(robotIDs)
	for _, robotID := range robotIDs {
		h.forceStop(robotID, "command timeout")
	}
}

func (h *Hub) forceStopAll(reason string) {
	h.mu.Lock()
	robotIDSet := make(map[string]struct{})
	for robotID, state := range h.robotStates {
		if state.moving {
			state.moving = false
			h.robotStates[robotID] = state
			robotIDSet[robotID] = struct{}{}
		}
	}
	for _, current := range h.clients {
		if current.role == model.RoleRobot {
			robotIDSet[model.NormalizeRobotID(current.robotID)] = struct{}{}
		}
	}
	h.mu.Unlock()

	robotIDs := make([]string, 0, len(robotIDSet))
	for robotID := range robotIDSet {
		robotIDs = append(robotIDs, robotID)
	}
	sort.Strings(robotIDs)
	for _, robotID := range robotIDs {
		h.forceStop(robotID, reason)
	}
}

func (h *Hub) forceStop(robotID string, reason string) {
	cmd := model.Command{Type: model.CommandStop, Note: "failsafe: " + reason}
	log.Printf("failsafe stop: robot=%s reason=%s", robotID, reason)
	// Failsafe is also delivered as a normal stop command so real motor code can
	// treat it exactly like a user stop, with the reason kept in Note.
	h.broadcastCommandToRobot(robotID, "command", cmd)
	h.broadcastCommandToControllers(robotID, "failsafe", cmd)
}

func (h *Hub) add(conn *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.clients[conn] = &client{conn: conn, role: model.RoleController}
}

func (h *Hub) remove(conn *websocket.Conn) client {
	h.mu.Lock()
	defer h.mu.Unlock()

	current := h.clients[conn]
	delete(h.clients, conn)
	if current == nil {
		return client{role: model.RoleController}
	}
	return *current
}

func (h *Hub) setRole(conn *websocket.Conn, role model.ClientRole, robotID string) {
	if role != model.RoleRobot {
		role = model.RoleController
		robotID = ""
	} else {
		robotID = model.NormalizeRobotID(robotID)
	}

	h.mu.Lock()
	if current := h.clients[conn]; current != nil {
		current.role = role
		current.robotID = robotID
	}
	h.mu.Unlock()

	h.send(conn, model.ServerMessage{
		Type:    "role",
		Role:    role,
		RobotID: robotID,
		Clients: h.counts(),
	})
	h.broadcastClients()
}

func (h *Hub) clientInfo(conn *websocket.Conn) (model.ClientRole, string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if current := h.clients[conn]; current != nil {
		return current.role, current.robotID
	}
	return model.RoleController, ""
}

func (h *Hub) roleOf(conn *websocket.Conn) model.ClientRole {
	role, _ := h.clientInfo(conn)
	return role
}

func (h *Hub) broadcastClients() {
	h.broadcast(model.ServerMessage{
		Type:    "clients",
		Clients: h.counts(),
	})
}

func (h *Hub) broadcastCommandToRobot(robotID string, messageType string, cmd model.Command) {
	h.broadcastToRobot(robotID, model.ServerMessage{
		Type:    messageType,
		RobotID: robotID,
		Clients: h.counts(),
		Command: &cmd,
	})
}

func (h *Hub) broadcastCommandToControllers(robotID string, messageType string, cmd model.Command) {
	h.broadcastToRole(model.RoleController, model.ServerMessage{
		Type:    messageType,
		RobotID: robotID,
		Clients: h.counts(),
		Command: &cmd,
	})
}

func (h *Hub) broadcastToRobot(robotID string, msg model.ServerMessage) {
	h.mu.Lock()
	targets := make([]*websocket.Conn, 0, len(h.clients))
	for conn, current := range h.clients {
		if current.role == model.RoleRobot && current.robotID == robotID {
			targets = append(targets, conn)
		}
	}
	h.mu.Unlock()

	for _, conn := range targets {
		h.send(conn, msg)
	}
}

func (h *Hub) broadcastToRole(role model.ClientRole, msg model.ServerMessage) {
	h.mu.Lock()
	targets := make([]*websocket.Conn, 0, len(h.clients))
	for conn, current := range h.clients {
		if current.role == role {
			targets = append(targets, conn)
		}
	}
	h.mu.Unlock()

	for _, conn := range targets {
		h.send(conn, msg)
	}
}

func (h *Hub) broadcast(msg model.ServerMessage) {
	h.mu.Lock()
	targets := make([]*websocket.Conn, 0, len(h.clients))
	for conn := range h.clients {
		targets = append(targets, conn)
	}
	h.mu.Unlock()

	for _, conn := range targets {
		h.send(conn, msg)
	}
}

func (h *Hub) send(conn *websocket.Conn, msg model.ServerMessage) {
	h.mu.Lock()
	current := h.clients[conn]
	h.mu.Unlock()
	if current == nil {
		return
	}

	current.writeMu.Lock()
	defer current.writeMu.Unlock()

	if err := conn.WriteJSON(msg); err != nil {
		log.Printf("ws write failed: %v", err)
		h.mu.Lock()
		delete(h.clients, conn)
		h.mu.Unlock()
		conn.Close()
	}
}

func (h *Hub) counts() model.ClientCounts {
	h.mu.Lock()
	defer h.mu.Unlock()

	var counts model.ClientCounts
	robotIDs := make(map[string]struct{})
	for _, current := range h.clients {
		switch current.role {
		case model.RoleRobot:
			counts.Robots++
			robotIDs[model.NormalizeRobotID(current.robotID)] = struct{}{}
		default:
			counts.Controllers++
		}
	}

	counts.RobotIDs = make([]string, 0, len(robotIDs))
	for robotID := range robotIDs {
		counts.RobotIDs = append(counts.RobotIDs, robotID)
	}
	sort.Strings(counts.RobotIDs)

	return counts
}

func clamp(value, min, max float64) float64 {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}
