package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	"github.com/pion/webrtc/v3"
)

type BrowserMessage struct {
	Type        string `json:"type"`
	Data        string `json:"data,omitempty"`
	Text        string `json:"text,omitempty"`
	CandidateID string `json:"candidate_id,omitempty"`
	Language    string `json:"language,omitempty"`
}

type StreamMessage struct {
	Type     string `json:"type"`
	Text     string `json:"text,omitempty"`
	State    string `json:"state"`
	Language string `json:"language,omitempty"`
}

type Relay struct {
	stt     *STTClient
	model   ModelStreamer
	signals *SignalStore
	upgrade websocket.Upgrader
}

type ModelStreamer interface {
	Stream(ctx context.Context, candidateID, text, contextBlock, language string) (<-chan StreamMessage, error)
}

func NewRelay(stt *STTClient, model ModelStreamer) *Relay {
	return &Relay{
		stt:     stt,
		model:   model,
		signals: &SignalStore{},
		upgrade: websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }},
	}
}

func (r *Relay) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, req *http.Request) { _, _ = w.Write([]byte("ok")) })
	mux.HandleFunc("/offer", r.handleOffer)
	mux.HandleFunc("/answer", r.handleAnswer)
	mux.HandleFunc("/ice-candidate", r.handleICECandidate)
	mux.HandleFunc("/ws", r.handleFallbackWS)
	return mux
}

func (r *Relay) newPeerConnection() (*webrtc.PeerConnection, error) {
	peer, err := webrtc.NewPeerConnection(webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{{URLs: []string{"stun:stun.l.google.com:19302"}}},
	})
	if err != nil {
		return nil, err
	}
	channel, err := peer.CreateDataChannel("ai-stream", nil)
	if err != nil {
		_ = peer.Close()
		return nil, err
	}
	channel.OnMessage(func(msg webrtc.DataChannelMessage) {
		go r.relayBrowserPayload(context.Background(), msg.Data, func(out StreamMessage) error {
			raw, err := json.Marshal(out)
			if err != nil {
				return err
			}
			return channel.SendText(string(raw))
		})
	})
	peer.OnTrack(func(track *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
		go r.consumeAudioTrack(context.Background(), track, channel)
	})
	return peer, nil
}

func (r *Relay) consumeAudioTrack(ctx context.Context, track *webrtc.TrackRemote, channel *webrtc.DataChannel) {
	deadline := time.After(2 * time.Second)
	var audio []byte
	for {
		select {
		case <-ctx.Done():
			return
		case <-deadline:
			if len(audio) > 0 {
				_ = r.relayAudio(ctx, audio, "", "auto", func(out StreamMessage) error {
					raw, _ := json.Marshal(out)
					return channel.SendText(string(raw))
				})
			}
			return
		default:
			packet, _, err := track.ReadRTP()
			if err != nil {
				if err == io.EOF {
					return
				}
				continue
			}
			audio = append(audio, packet.Payload...)
		}
	}
}

func (r *Relay) handleFallbackWS(w http.ResponseWriter, req *http.Request) {
	conn, err := r.upgrade.Upgrade(w, req, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	for {
		_, raw, err := conn.ReadMessage()
		if err != nil {
			return
		}
		err = r.relayBrowserPayload(req.Context(), raw, func(out StreamMessage) error {
			return conn.WriteJSON(out)
		})
		if err != nil {
			return
		}
	}
}

func (r *Relay) relayBrowserPayload(ctx context.Context, raw []byte, send func(StreamMessage) error) error {
	var msg BrowserMessage
	if err := json.Unmarshal(raw, &msg); err != nil {
		return err
	}
	if msg.Text != "" {
		return r.streamTranscript(ctx, msg.CandidateID, msg.Text, msg.Language, send)
	}
	if msg.Type == "audio" {
		audio, err := decodeAudioPayload(msg.Data)
		if err != nil {
			return err
		}
		return r.relayAudio(ctx, audio, msg.CandidateID, msg.Language, send)
	}
	return nil
}

func (r *Relay) relayAudio(ctx context.Context, audio []byte, candidateID, language string, send func(StreamMessage) error) error {
	text, err := r.stt.Transcribe(ctx, audio, language)
	if err != nil {
		return err
	}
	return r.streamTranscript(ctx, candidateID, text, language, send)
}

func (r *Relay) streamTranscript(ctx context.Context, candidateID, text, language string, send func(StreamMessage) error) error {
	stream, err := r.model.Stream(ctx, candidateID, text, "relay context: live candidate interview", language)
	if err != nil {
		return err
	}
	for msg := range stream {
		if err := send(msg); err != nil {
			return err
		}
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
