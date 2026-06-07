package main

import (
	"context"
	"log"
	"net/http"
	"roboback/internal/handler"
)

func main() {
	mux := http.NewServeMux()
	hub := handler.NewHub()

	go hub.Run(context.Background())

	mux.HandleFunc("/health", handler.HandleHealth)
	mux.HandleFunc("/ws", hub.HandleWS)

	srv := http.Server{
		Addr:    ":8080",
		Handler: mux,
	}

	log.Println("robot api listening on :8080")
	if err := srv.ListenAndServe(); err != nil {
		log.Fatalf("Server has failed %v", err)
	}
}
