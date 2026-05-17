//go:build ignore

package main

import (
	"encoding/base64"
	"flag"
	"fmt"
	"log"
	"time"

	"github.com/gorilla/websocket"
)

type clientBrowserMessage struct {
	SessionID      string `json:"session_id,omitempty"`
	Type           string `json:"type"`
	Data           string `json:"data,omitempty"`
	Text           string `json:"text,omitempty"`
	CandidateID    string `json:"candidate_id,omitempty"`
	Language       string `json:"language,omitempty"`
	CandidateStyle string `json:"candidate_style,omitempty"`
}

type clientStreamMessage struct {
	Type          string `json:"type"`
	Text          string `json:"text,omitempty"`
	State         string `json:"state"`
	Language      string `json:"language,omitempty"`
	Phase         string `json:"phase,omitempty"`
	PhaseBefore   string `json:"phase_before,omitempty"`
	ResponseStyle string `json:"response_style,omitempty"`
	Response      string `json:"response,omitempty"`
	Style         string `json:"style,omitempty"`
	Status        string `json:"status,omitempty"`
}

func main() {
	addr := flag.String("addr", "ws://localhost:3000/ws", "relay fallback websocket URL")
	text := flag.String("text", "I measured reliability during the migration with latency and error budgets.", "candidate transcript")
	candidateID := flag.String("candidate-id", "test_001", "candidate session id")
	sessionID := flag.String("session-id", "", "interview session id")
	language := flag.String("language", "auto", "language preference: auto, en, hi, hinglish")
	candidateStyle := flag.String("candidate-style", "Default", "candidate speaking style")
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
	payload := base64.StdEncoding.EncodeToString([]byte("text:" + *text))
	if err := conn.WriteJSON(clientBrowserMessage{SessionID: *sessionID, Type: "audio", Data: payload, CandidateID: *candidateID, Language: *language, CandidateStyle: *candidateStyle}); err != nil {
		log.Fatal(err)
	}
	for {
		var msg clientStreamMessage
		if err := conn.ReadJSON(&msg); err != nil {
			log.Fatal(err)
		}
		if msg.Type == "filler" {
			fmt.Printf("filler in %dms [%s]: %s\n", time.Since(start).Milliseconds(), msg.Language, msg.Text)
		} else if msg.Type == "ack" {
			fmt.Printf("ack in %dms [%s]: %s\n", time.Since(start).Milliseconds(), msg.Language, msg.Text)
		} else if msg.Type == "phase" {
			fmt.Printf("\nphase: %s <- %s\n", msg.Phase, msg.PhaseBefore)
		} else if msg.Type == "style" {
			fmt.Printf("\nstyle: %s [%s %s]\n", msg.ResponseStyle, msg.Language, msg.Phase)
		} else if msg.Type == "interview_response" {
			fmt.Printf("\nresponse (%s, %s): %s\n", msg.Style, msg.Status, msg.Response)
			return
		} else if msg.Type == "token" {
			fmt.Printf("%s ", msg.Text)
		} else if msg.Type == "end" {
			fmt.Printf("\nend: %s\n", msg.State)
			return
		}
	}
}
