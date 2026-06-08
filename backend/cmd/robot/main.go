package main

import (
	"context"
	"errors"
	"log"
	"os/signal"
	"syscall"
	"time"

	"roboback/internal/robot"
)

func main() {
	ctrl := robot.NewMockController()
	driver := robot.NewDriver("ws://localhost:8080/ws", ctrl)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	log.Println("robot driver starting, connecting to ws://localhost:8080/ws")
	for {
		err := driver.Run(ctx)
		if err == nil || errors.Is(err, context.Canceled) {
			log.Println("robot driver stopped")
			return
		}

		log.Printf("robot driver disconnected: %v", err)

		select {
		case <-ctx.Done():
			log.Println("robot driver stopped")
			return
		case <-time.After(time.Second):
			log.Println("robot driver reconnecting")
		}
	}
}
