package main

import (
	"context"
	"log"
	"os/signal"
	"syscall"

	"roboback/internal/robot"
)

func main() {
	ctrl := robot.NewMockController()

	driver := robot.NewDriver("ws://localhost:8080/ws", ctrl)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	log.Println("robot driver starting, connecting to ws://localhost:8080/ws")
	if err := driver.Run(ctx); err != nil {
		log.Fatalf("driver stopped: %v", err)
	}
}
