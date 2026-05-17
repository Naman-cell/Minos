package main

import (
	"context"
	"encoding/base64"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

type fakeModel struct{}

func (f *fakeModel) Stream(ctx context.Context, candidateID, text, contextBlock, language string) (<-chan StreamMessage, error) {
	out := make(chan StreamMessage, 3)
	out <- StreamMessage{Type: "ack", Text: "ack", State: "thinking"}
	out <- StreamMessage{Type: "token", Text: "Why?", State: "speaking"}
	out <- StreamMessage{Type: "end", State: "listening"}
	close(out)
	return out, nil
}

func TestFallbackWebSocketRelaysStream(t *testing.T) {
	relay := NewRelay(NewSTTClient(), &fakeModel{})
	server := httptest.NewServer(relay.Routes())
	defer server.Close()

	url := "ws" + server.URL[len("http"):] + "/ws"
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	payload := base64.StdEncoding.EncodeToString([]byte("text:I measured the latency impact."))
	if err := conn.WriteJSON(BrowserMessage{Type: "audio", Data: payload}); err != nil {
		t.Fatal(err)
	}

	want := []string{"ack:thinking", "token:speaking", "end:listening"}
	for _, expected := range want {
		var msg StreamMessage
		if err := conn.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
			t.Fatal(err)
		}
		if err := conn.ReadJSON(&msg); err != nil {
			t.Fatal(err)
		}
		got := msg.Type + ":" + msg.State
		if got != expected {
			t.Fatalf("got %s want %s", got, expected)
		}
	}
}
