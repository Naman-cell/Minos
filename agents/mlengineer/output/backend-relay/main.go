package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"time"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "--healthcheck" {
		fmt.Println("ok")
		return
	}

	addr := env("RELAY_ADDR", ":3000")
	modelURL := env("MODEL_WS_URL", "ws://localhost:8080/ws")

	relay := NewRelay(NewSTTClient(), NewModelClient(modelURL))
	server := &http.Server{
		Addr:              addr,
		Handler:           relay.Routes(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	log.Printf("backend relay listening on %s", addr)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
