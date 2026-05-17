package main

import (
	"context"
	"time"

	"github.com/gorilla/websocket"
)

type ModelRequest struct {
	CandidateID string `json:"candidate_id,omitempty"`
	Text        string `json:"text"`
	Context     string `json:"context"`
	Language    string `json:"language,omitempty"`
}

type ModelClient struct {
	url string
}

func NewModelClient(url string) *ModelClient {
	return &ModelClient{url: url}
}

func (m *ModelClient) Stream(ctx context.Context, candidateID, text, contextBlock, language string) (<-chan StreamMessage, error) {
	out := make(chan StreamMessage)
	conn, _, err := websocket.DefaultDialer.DialContext(ctx, m.url, nil)
	if err != nil {
		go mockStream(ctx, out)
		return out, nil
	}
	if err := conn.WriteJSON(ModelRequest{CandidateID: candidateID, Text: text, Context: contextBlock, Language: language}); err != nil {
		_ = conn.Close()
		return nil, err
	}
	go func() {
		defer close(out)
		defer conn.Close()
		for {
			var msg StreamMessage
			if err := conn.ReadJSON(&msg); err != nil {
				return
			}
			out <- msg
			if msg.Type == "end" {
				return
			}
		}
	}()
	return out, nil
}

func mockStream(ctx context.Context, out chan<- StreamMessage) {
	defer close(out)
	messages := []StreamMessage{
		{Type: "ack", Text: "That makes sense.", State: "thinking", Language: "en"},
		{Type: "token", Text: "You", State: "speaking", Language: "en"},
		{Type: "token", Text: "mentioned", State: "speaking", Language: "en"},
		{Type: "token", Text: "latency.", State: "speaking", Language: "en"},
		{Type: "token", Text: "How", State: "speaking", Language: "en"},
		{Type: "token", Text: "did", State: "speaking", Language: "en"},
		{Type: "token", Text: "you", State: "speaking", Language: "en"},
		{Type: "token", Text: "validate", State: "speaking", Language: "en"},
		{Type: "token", Text: "it?", State: "speaking", Language: "en"},
		{Type: "end", State: "listening", Language: "en"},
	}
	for _, msg := range messages {
		select {
		case <-ctx.Done():
			return
		case <-time.After(10 * time.Millisecond):
			out <- msg
		}
	}
}
