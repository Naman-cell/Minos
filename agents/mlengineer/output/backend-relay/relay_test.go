package main

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

type fakeModel struct{}

func (f *fakeModel) Stream(ctx context.Context, candidateID, text, contextBlock, language, candidateStyle string) (<-chan StreamMessage, error) {
	out := make(chan StreamMessage, 4)
	out <- StreamMessage{Type: "phase", Phase: "interview", PhaseBefore: "rapport", Language: "en"}
	out <- StreamMessage{Type: "style", ResponseStyle: "Friendly", CandidateStyle: candidateStyle, Language: "en", Phase: "interview"}
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

	want := []string{"phase:interview:rapport", "style:Friendly", "token:speaking", "end:listening"}
	for _, expected := range want {
		var msg StreamMessage
		if err := conn.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
			t.Fatal(err)
		}
		if err := conn.ReadJSON(&msg); err != nil {
			t.Fatal(err)
		}
		got := msg.Type + ":" + msg.State
		if msg.Type == "phase" {
			got = msg.Type + ":" + msg.Phase + ":" + msg.PhaseBefore
		}
		if msg.Type == "style" {
			got = msg.Type + ":" + msg.ResponseStyle
		}
		if got != expected {
			t.Fatalf("got %s want %s", got, expected)
		}
	}
}

func TestFallbackWebSocketSendsFillerBeforeBinaryAudioSTT(t *testing.T) {
	sttServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/transcribe" {
			t.Fatalf("path=%q want /transcribe", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"transcript":"I measured p95 latency."}`))
	}))
	defer sttServer.Close()

	relay := NewRelay(&STTClient{
		mode:      "brainc",
		brainURL:  sttServer.URL,
		client:    sttServer.Client(),
		mediaName: "candidate.webm",
		mediaType: "audio/webm",
	}, &fakeModel{})
	server := httptest.NewServer(relay.Routes())
	defer server.Close()

	url := "ws" + server.URL[len("http"):] + "/ws"
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	payload := base64.StdEncoding.EncodeToString([]byte{1, 2, 3, 4})
	if err := conn.WriteJSON(BrowserMessage{Type: "audio", Data: payload, CandidateID: "fill_001", Language: "hinglish"}); err != nil {
		t.Fatal(err)
	}

	if err := conn.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	var msg StreamMessage
	if err := conn.ReadJSON(&msg); err != nil {
		t.Fatal(err)
	}
	if msg.Type != "filler" {
		t.Fatalf("first type=%q want filler", msg.Type)
	}
	if msg.Text == "" {
		t.Fatal("expected filler text")
	}
	if msg.Language != "hinglish" {
		t.Fatalf("language=%q want hinglish", msg.Language)
	}
}
