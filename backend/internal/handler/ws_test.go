package handler

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"roboback/internal/model"

	"github.com/gorilla/websocket"
)

func TestHubRoutesCommandsByRobotID(t *testing.T) {
	hub := NewHub()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go hub.Run(ctx)

	server := httptest.NewServer(http.HandlerFunc(hub.HandleWS))
	defer server.Close()

	controller := dialHub(t, server.URL)
	defer controller.Close()
	robotA := dialHub(t, server.URL)
	defer robotA.Close()
	robotB := dialHub(t, server.URL)
	defer robotB.Close()

	sendClientMessage(t, robotA, model.ClientMessage{
		Type:    "hello",
		Role:    model.RoleRobot,
		RobotID: "car-a",
	})
	mustRead(t, robotA, time.Second, func(msg model.ServerMessage) bool {
		return msg.Type == "role" && msg.Role == model.RoleRobot && msg.RobotID == "car-a"
	})

	sendClientMessage(t, robotB, model.ClientMessage{
		Type:    "hello",
		Role:    model.RoleRobot,
		RobotID: "car-b",
	})
	mustRead(t, robotB, time.Second, func(msg model.ServerMessage) bool {
		return msg.Type == "role" && msg.Role == model.RoleRobot && msg.RobotID == "car-b"
	})

	sendClientMessage(t, controller, model.ClientMessage{
		Type: "hello",
		Role: model.RoleController,
	})
	mustRead(t, controller, time.Second, func(msg model.ServerMessage) bool {
		return msg.Type == "role" && msg.Role == model.RoleController
	})

	sendClientMessage(t, controller, model.ClientMessage{
		Type:          "command",
		TargetRobotID: "car-a",
		Command: &model.Command{
			Type: model.CommandMove,
			X:    0.8,
			Y:    -0.2,
		},
	})

	moveToA := mustRead(t, robotA, time.Second, func(msg model.ServerMessage) bool {
		return msg.Type == "command" && msg.RobotID == "car-a" && msg.Command != nil
	})
	if moveToA.Command.Type != model.CommandMove {
		t.Fatalf("robot A command type = %s, want %s", moveToA.Command.Type, model.CommandMove)
	}

	if msg, ok := readMatching(t, robotB, 150*time.Millisecond, func(msg model.ServerMessage) bool {
		return msg.Type == "command"
	}); ok {
		t.Fatalf("robot B received command for %s: %+v", msg.RobotID, msg.Command)
	}

	mustRead(t, controller, time.Second, func(msg model.ServerMessage) bool {
		return msg.Type == "command_ack" && msg.RobotID == "car-a" && msg.Command != nil
	})

	stopToA := mustRead(t, robotA, commandTTL+500*time.Millisecond, func(msg model.ServerMessage) bool {
		return msg.Type == "command" &&
			msg.RobotID == "car-a" &&
			msg.Command != nil &&
			msg.Command.Type == model.CommandStop
	})
	if !strings.Contains(stopToA.Command.Note, "command timeout") {
		t.Fatalf("failsafe note = %q, want command timeout", stopToA.Command.Note)
	}

	if msg, ok := readMatching(t, robotB, 150*time.Millisecond, func(msg model.ServerMessage) bool {
		return msg.Type == "command"
	}); ok {
		t.Fatalf("robot B received failsafe for %s: %+v", msg.RobotID, msg.Command)
	}
}

func dialHub(t *testing.T, serverURL string) *websocket.Conn {
	t.Helper()

	wsURL := "ws" + strings.TrimPrefix(serverURL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial hub: %v", err)
	}
	return conn
}

func sendClientMessage(t *testing.T, conn *websocket.Conn, msg model.ClientMessage) {
	t.Helper()

	if err := conn.WriteJSON(msg); err != nil {
		t.Fatalf("write ws message: %v", err)
	}
}

func mustRead(t *testing.T, conn *websocket.Conn, timeout time.Duration, match func(model.ServerMessage) bool) model.ServerMessage {
	t.Helper()

	msg, ok := readMatching(t, conn, timeout, match)
	if !ok {
		t.Fatalf("timed out waiting for websocket message")
	}
	return msg
}

func readMatching(t *testing.T, conn *websocket.Conn, timeout time.Duration, match func(model.ServerMessage) bool) (model.ServerMessage, bool) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for {
		var msg model.ServerMessage
		if err := conn.SetReadDeadline(deadline); err != nil {
			t.Fatalf("set read deadline: %v", err)
		}
		if err := conn.ReadJSON(&msg); err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				return model.ServerMessage{}, false
			}
			t.Fatalf("read ws message: %v", err)
		}
		if match(msg) {
			return msg, true
		}
	}
}
