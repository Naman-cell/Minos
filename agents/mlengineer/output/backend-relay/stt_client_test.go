package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestBrainCSTTUsesTranscribeEndpoint(t *testing.T) {
	var gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		if r.URL.Path == "/native-audio-chat" {
			t.Fatal("STT_MODE=brainc must not call native audio chat")
		}
		if r.URL.Path != "/transcribe" {
			t.Fatalf("path=%q want /transcribe", r.URL.Path)
		}
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Fatal(err)
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"transcript": "I used Redis with TTLs."})
	}))
	defer server.Close()

	client := &STTClient{
		mode:      "brainc",
		brainURL:  server.URL,
		client:    server.Client(),
		mediaName: "candidate.webm",
		mediaType: "audio/webm",
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	got, err := client.Transcribe(ctx, []byte{1, 2, 3}, "en")
	if err != nil {
		t.Fatal(err)
	}
	if got != "I used Redis with TTLs." {
		t.Fatalf("transcript=%q", got)
	}
	if gotPath != "/transcribe" {
		t.Fatalf("path=%q want /transcribe", gotPath)
	}
}

func TestBrainCSTTAllowsEmptyTranscript(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/transcribe" {
			t.Fatalf("path=%q want /transcribe", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"transcript": ""})
	}))
	defer server.Close()

	client := &STTClient{
		mode:      "brainc",
		brainURL:  server.URL,
		client:    server.Client(),
		mediaName: "candidate.webm",
		mediaType: "audio/webm",
	}
	got, err := client.Transcribe(context.Background(), []byte{1, 2, 3}, "en")
	if err != nil {
		t.Fatal(err)
	}
	if got != "" {
		t.Fatalf("transcript=%q want empty", got)
	}
}

func TestNativeAudioRequiresExplicitMode(t *testing.T) {
	var gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_ = json.NewEncoder(w).Encode(map[string]string{"text": "native transcript"})
	}))
	defer server.Close()

	client := &STTClient{
		mode:      "native_audio",
		brainURL:  server.URL,
		client:    server.Client(),
		mediaName: "candidate.webm",
		mediaType: "audio/webm",
	}
	got, err := client.Transcribe(context.Background(), []byte{1, 2, 3}, "en")
	if err != nil {
		t.Fatal(err)
	}
	if got != "native transcript" {
		t.Fatalf("transcript=%q", got)
	}
	if gotPath != "/native-audio-chat" {
		t.Fatalf("path=%q want /native-audio-chat", gotPath)
	}
}
