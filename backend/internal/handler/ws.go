package handler

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"roboback/internal/model"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const commandTTL = 700 * time.Millisecond

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

type client struct {
	conn *websocket.Conn
	role model.ClientRole
	// Gorilla allows one writer at a time per connection.
	writeMu sync.Mutex
}

type Hub struct {
	mu      sync.Mutex
	clients map[*websocket.Conn]*client
	// lastCommand/moving are the server-side deadman switch. The controller
	// must keep refreshing move commands; otherwise robots get a forced stop.
	lastCommand time.Time
	moving      bool
}

func NewHub() *Hub {
	return &Hub{
		clients: make(map[*websocket.Conn]*client),
	}
}

// tiker.C что значит? просто периодичность срабатывания?
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
		log.Printf("client disconnected: role=%s", removed.role)

		// Losing every controller means no one can intentionally stop the car.
		if removed.role == model.RoleController && h.counts().Controllers == 0 {
			h.forceStop("controller disconnected")
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
			h.setRole(conn, msg.Role)
			return
		case "status":
			if msg.Status != nil {
				// Robot status is reported by the robot client and forwarded to UIs.
				h.broadcastToRole(model.RoleController, model.ServerMessage{
					Type:    "robot_status",
					Clients: h.counts(),
					Status:  msg.Status,
				})
			}
			return
		case "command":
			if msg.Command != nil {
				h.handleCommand(*msg.Command)
			}
			return
		}
	}

	// Backward-compatible path for sending raw command JSON from controllers.
	var cmd model.Command
	if err := json.Unmarshal(payload, &cmd); err != nil {
		log.Printf("bad ws payload: %v", err)
		return
	}
	h.handleCommand(cmd)
}

func (h *Hub) handleCommand(cmd model.Command) {
	cmd.X = clamp(cmd.X, -1, 1)
	cmd.Y = clamp(cmd.Y, -1, 1)

	switch cmd.Type {
	case model.CommandMove:
		h.mu.Lock()
		h.lastCommand = time.Now()
		h.moving = true
		h.mu.Unlock()
	case model.CommandStop, model.CommandReset:
		h.mu.Lock()
		h.moving = false
		h.mu.Unlock()
	default:
		log.Printf("unknown command: %s", cmd.Type)
		return
	}

	log.Printf("command: type=%s x=%.2f y=%.2f note=%s", cmd.Type, cmd.X, cmd.Y, cmd.Note)
	// The backend is a broker: commands go to robots, acknowledgements go to UIs.
	h.broadcastCommand("command", cmd, model.RoleRobot)
	h.broadcastCommand("command_ack", cmd, model.RoleController)
}

func (h *Hub) checkDeadman() {
	h.mu.Lock()
	shouldStop := h.moving && time.Since(h.lastCommand) > commandTTL
	if shouldStop {
		h.moving = false
	}
	h.mu.Unlock()

	if shouldStop {
		h.forceStop("command timeout")
	}
}

func (h *Hub) forceStop(reason string) {
	cmd := model.Command{Type: model.CommandStop, Note: "failsafe: " + reason}
	log.Printf("failsafe stop: %s", reason)
	// Failsafe is also delivered as a normal stop command so real motor code can
	// treat it exactly like a user stop, with the reason kept in Note.
	h.broadcastCommand("command", cmd, model.RoleRobot)
	h.broadcastCommand("failsafe", cmd, model.RoleController)
}

func (h *Hub) add(conn *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.clients[conn] = &client{conn: conn, role: model.RoleController}
}

// если тут remove зачем что-то возвращать?
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

// зачем тут вконце h.broadcastClients() ? 
func (h *Hub) setRole(conn *websocket.Conn, role model.ClientRole) {
	if role != model.RoleRobot {
		role = model.RoleController
	}

	h.mu.Lock()
	if current := h.clients[conn]; current != nil {
		current.role = role
	}
	h.mu.Unlock()

	h.send(conn, model.ServerMessage{
		Type:    "role",
		Role:    role,
		Clients: h.counts(),
	})
	h.broadcastClients()
}

func (h *Hub) broadcastClients() {
	h.broadcast(model.ServerMessage{
		Type:    "clients",
		Clients: h.counts(),
	})
}

func (h *Hub) broadcastCommand(messageType string, cmd model.Command, role model.ClientRole) {
	h.broadcastToRole(role, model.ServerMessage{
		Type:    messageType,
		Clients: h.counts(),
		Command: &cmd,
	})
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
	for _, current := range h.clients {
		switch current.role {
		case model.RoleRobot:
			counts.Robots++
		default:
			counts.Controllers++
		}
	}
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
