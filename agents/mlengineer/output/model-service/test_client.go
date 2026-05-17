//go:build ignore

package main

import (
	"flag"
	"fmt"
	"log"
	"time"

	"github.com/gorilla/websocket"
)

type clientModelRequest struct {
	CandidateID string `json:"candidate_id,omitempty"`
	Text        string `json:"text"`
	Context     string `json:"context"`
	Language    string `json:"language,omitempty"`
}

type clientStreamMessage struct {
	Type     string `json:"type"`
	Text     string `json:"text,omitempty"`
	State    string `json:"state"`
	Language string `json:"language,omitempty"`
}

func main() {
	addr := flag.String("addr", "ws://localhost:8080/ws", "model service websocket URL")
	text := flag.String("text", "I led a reliability migration and measured latency before and after.", "candidate transcript")
	candidateID := flag.String("candidate-id", "test_001", "candidate session id")
	language := flag.String("language", "auto", "language preference: auto, en, hi, hinglish")
	flag.Parse()

	conn, resp, err := websocket.DefaultDialer.Dial(*addr, nil)
	if err != nil {
		if resp != nil {
			log.Fatalf("%v (status %s)", err, resp.Status)
		}
		log.Fatal(err)
	}
	defer conn.Close()

	start := time.Now()
	if err := conn.WriteJSON(clientModelRequest{CandidateID: *candidateID, Text: *text, Context: "Senior backend role requiring distributed systems and incident response.", Language: *language}); err != nil {
		log.Fatal(err)
	}
	for {
		var msg clientStreamMessage
		if err := conn.ReadJSON(&msg); err != nil {
			log.Fatal(err)
		}
		if msg.Type == "ack" {
			fmt.Printf("ack in %dms [%s]: %s\n", time.Since(start).Milliseconds(), msg.Language, msg.Text)
		} else if msg.Type == "token" {
			fmt.Printf("%s ", msg.Text)
		} else if msg.Type == "end" {
			fmt.Printf("\nend: %s\n", msg.State)
			return
		}
	}
}
