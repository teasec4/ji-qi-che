package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"os/signal"
	"syscall"
	"time"

	"roboback/internal/model"
	"roboback/internal/robot"
)

func main() {
	hubURL := flag.String("hub", "ws://localhost:8080/ws", "hub websocket url")
	robotID := flag.String("id", model.DefaultRobotID, "robot id")
	flag.Parse()

	ctrl := robot.NewMockController()
	driver := robot.NewDriver(*hubURL, *robotID, ctrl)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	log.Printf("robot driver starting, connecting to %s as %s", *hubURL, model.NormalizeRobotID(*robotID))
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
