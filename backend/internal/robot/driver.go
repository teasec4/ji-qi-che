package robot

import (
	"context"
	"encoding/json"
	"log"
	"sync"
	"time"

	"roboback/internal/model"

	"github.com/gorilla/websocket"
)

const watchdogTTL = 700 * time.Millisecond

// Driver connects to the Hub as a robot, receives commands, drives the
// MotorController, and reports controller status back.
type Driver struct {
	hubURL     string
	robotID    string
	controller MotorController
	controlMu  sync.Mutex
	moving     bool
}

func NewDriver(hubURL string, robotID string, controller MotorController) *Driver {
	return &Driver{
		hubURL:     hubURL,
		robotID:    model.NormalizeRobotID(robotID),
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
		Type:    "hello",
		Role:    model.RoleRobot,
		RobotID: d.robotID,
	})
	if err := conn.WriteMessage(websocket.TextMessage, hello); err != nil {
		return err
	}
	log.Printf("robot driver connected to %s as %s", d.hubURL, d.robotID)
	defer func() {
		d.controlMu.Lock()
		defer d.controlMu.Unlock()

		d.moving = false
		if err := d.controller.Stop(); err != nil {
			log.Printf("robot driver: stop on shutdown failed: %v", err)
		}
	}()

	// Periodic status push.
	statusTicker := time.NewTicker(250 * time.Millisecond)
	defer statusTicker.Stop()

	// Separate goroutine for reading so status/watchdog can run independently.
	readErr := make(chan error, 1)
	moveSeen := make(chan struct{}, 1)
	stopped := make(chan struct{}, 1)

	go func() {
		readErr <- d.readLoop(conn, moveSeen, stopped)
	}()

	watchdog := time.NewTimer(watchdogTTL)
	if !watchdog.Stop() {
		<-watchdog.C
	}
	defer watchdog.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-readErr:
			return err
		case <-moveSeen:
			resetTimer(watchdog, watchdogTTL)
		case <-stopped:
			stopTimer(watchdog)
		case <-watchdog.C:
			d.stopFromWatchdog()
		case <-statusTicker.C:
			d.sendStatus(conn)
		}
	}
}

func (d *Driver) readLoop(conn *websocket.Conn, moveSeen chan<- struct{}, stopped chan<- struct{}) error {
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
		if msg.RobotID != "" && msg.RobotID != d.robotID {
			log.Printf("robot driver: ignored command for robot %s", msg.RobotID)
			continue
		}

		switch msg.Command.Type {
		case model.CommandMove:
			d.applyMove(msg.Command.X, msg.Command.Y, moveSeen)
		case model.CommandStop:
			d.applyStop(stopped)
		case model.CommandReset:
			d.applyReset(stopped)
		default:
			log.Printf("robot driver: unknown command: %s", msg.Command.Type)
		}
	}
}

func (d *Driver) applyMove(forward, turn float64, moveSeen chan<- struct{}) {
	d.controlMu.Lock()
	d.moving = true
	if err := d.controller.Move(forward, turn); err != nil {
		log.Printf("robot driver: move error: %v", err)
	}
	d.controlMu.Unlock()

	notify(moveSeen)
}

func (d *Driver) applyStop(stopped chan<- struct{}) {
	d.controlMu.Lock()
	d.moving = false
	if err := d.controller.Stop(); err != nil {
		log.Printf("robot driver: stop error: %v", err)
	}
	d.controlMu.Unlock()

	notify(stopped)
}

func (d *Driver) applyReset(stopped chan<- struct{}) {
	d.controlMu.Lock()
	d.moving = false
	if err := d.controller.Reset(); err != nil {
		log.Printf("robot driver: reset error: %v", err)
	}
	d.controlMu.Unlock()

	notify(stopped)
}

func (d *Driver) stopFromWatchdog() {
	d.controlMu.Lock()
	if !d.moving {
		d.controlMu.Unlock()
		return
	}
	d.moving = false
	log.Printf("robot driver: watchdog stop after %s without move command", watchdogTTL)
	if err := d.controller.Stop(); err != nil {
		log.Printf("robot driver: watchdog stop error: %v", err)
	}
	d.controlMu.Unlock()
}

func (d *Driver) sendStatus(conn *websocket.Conn) {
	d.controlMu.Lock()
	status := d.controller.Status()
	d.controlMu.Unlock()

	payload, err := json.Marshal(model.ClientMessage{
		Type:    "status",
		RobotID: d.robotID,
		Status:  &status,
	})
	if err != nil {
		return
	}

	conn.SetWriteDeadline(time.Now().Add(2 * time.Second))
	if err := conn.WriteMessage(websocket.TextMessage, payload); err != nil {
		log.Printf("robot driver: status write error: %v", err)
	}
}

func notify(ch chan<- struct{}) {
	select {
	case ch <- struct{}{}:
	default:
	}
}

func resetTimer(timer *time.Timer, duration time.Duration) {
	stopTimer(timer)
	timer.Reset(duration)
}

func stopTimer(timer *time.Timer) {
	if !timer.Stop() {
		select {
		case <-timer.C:
		default:
		}
	}
}
