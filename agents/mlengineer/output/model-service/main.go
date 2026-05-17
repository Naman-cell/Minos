package main

import (
	"context"
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

	addr := env("MODEL_ADDR", ":8080")
	dbPath := env("CONTEXT_DB", "/tmp/ai-interviewer-context.db")

	brainA, err := NewBrainA(dbPath)
	if err != nil {
		log.Fatalf("brain A: %v", err)
	}
	defer brainA.Close()

	brainC, err := NewBrainCFromEnv()
	if err != nil {
		log.Fatalf("brain C: %v", err)
	}
	log.Printf("brain C mode=%s", brainC.Mode())

	server := NewServer(brainA, NewBrainB(), brainC, NewStateMachine())
	httpServer := &http.Server{
		Addr:              addr,
		Handler:           server.Routes(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	log.Printf("model service listening on %s", addr)
	if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
	_ = httpServer.Shutdown(context.Background())
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
