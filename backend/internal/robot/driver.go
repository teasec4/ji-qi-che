package robot

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"roboback/internal/model"

	"github.com/gorilla/websocket"
)

// Driver connects to the Hub as a robot, receives commands, drives the
// MotorController, and reports telemetry back.
type Driver struct {
	hubURL     string
	controller MotorController
}

func NewDriver(hubURL string, controller MotorController) *Driver {
	return &Driver{
		hubURL:     hubURL,
		controller: controller,
	}
}

func (d *Driver) Run(ctx context.Context) error {
	conn, _, err := websocket.DefaultDialer.DialContext(ctx, d.hubURL, nil)
	if err != nil {
		return err
	}
	defer conn.Close()

	// Register as robot so the Hub routes commands to us.
	hello, _ := json.Marshal(model.ClientMessage{
		Type: "hello",
		Role: model.RoleRobot,
	})
	if err := conn.WriteMessage(websocket.TextMessage, hello); err != nil {
		return err
	}
	log.Printf("robot driver connected to %s", d.hubURL)

	// Periodic telemetry push.
	statusTicker := time.NewTicker(250 * time.Millisecond)
	defer statusTicker.Stop()

	// Separate goroutine for reading so status can be sent independently.
	readErr := make(chan error, 1)
	go func() {
		readErr <- d.readLoop(conn)
	}()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-readErr:
			return err
		case <-statusTicker.C:
			d.sendStatus(conn)
		}
	}
}

func (d *Driver) readLoop(conn *websocket.Conn) error {
	for {
		_, payload, err := conn.ReadMessage()
		if err != nil {
			return err
		}

		var msg model.ServerMessage
		if err := json.Unmarshal(payload, &msg); err != nil {
			log.Printf("robot driver: bad message: %v", err)
			continue
		}

		if msg.Command == nil {
			continue
		}

		switch msg.Command.Type {
		case model.CommandMove:
			if err := d.controller.Move(msg.Command.X, msg.Command.Y); err != nil {
				log.Printf("robot driver: move error: %v", err)
			}
		case model.CommandStop:
			if err := d.controller.Stop(); err != nil {
				log.Printf("robot driver: stop error: %v", err)
			}
		case model.CommandReset:
			if err := d.controller.Reset(); err != nil {
				log.Printf("robot driver: reset error: %v", err)
			}
		default:
			log.Printf("robot driver: unknown command: %s", msg.Command.Type)
		}
	}
}

func (d *Driver) sendStatus(conn *websocket.Conn) {
	status := d.controller.Telemetry()
	payload, err := json.Marshal(model.ClientMessage{
		Type:   "status",
		Status: &status,
	})
	if err != nil {
		return
	}

	conn.SetWriteDeadline(time.Now().Add(2 * time.Second))
	if err := conn.WriteMessage(websocket.TextMessage, payload); err != nil {
		log.Printf("robot driver: status write error: %v", err)
	}
}
