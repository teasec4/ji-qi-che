package robot

import (
	"log"
	"sync"

	"roboback/internal/model"
)

// MotorController is the hardware abstraction for the robot chassis.
// Today it's a logger. On Pi 5 it will become a GPIO/serial driver.
type MotorController interface {
	Move(forward, turn float64) error
	Stop() error
	Reset() error
	Status() model.RobotStatus
}

type ControllerMode string

const (
	ControllerModeIdle   ControllerMode = "idle"
	ControllerModeMoving ControllerMode = "moving"
)

// MockController logs every command to stdout.
// Replace with GPIOController when the real hardware is wired up.
type MockController struct {
	mu   sync.Mutex
	mode ControllerMode
}

func NewMockController() *MockController {
	return &MockController{mode: ControllerModeIdle}
}

func (m *MockController) Move(forward, turn float64) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.mode = ControllerModeMoving
	log.Printf("[mock] MOVE forward=%.2f turn=%.2f", forward, turn)
	return nil
}

func (m *MockController) Stop() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.mode = ControllerModeIdle
	log.Println("[mock] STOP")
	return nil
}

func (m *MockController) Reset() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.mode = ControllerModeIdle
	log.Println("[mock] RESET")
	return nil
}

// Status reports the current controller state for the Hub to broadcast.
func (m *MockController) Status() model.RobotStatus {
	m.mu.Lock()
	defer m.mu.Unlock()

	return model.RobotStatus{
		Mode: string(m.mode),
	}
}
