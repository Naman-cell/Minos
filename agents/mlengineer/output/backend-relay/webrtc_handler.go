package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/pion/webrtc/v3"
)

type BrowserMessage struct {
	SessionID      string `json:"session_id,omitempty"`
	Type           string `json:"type"`
	Data           string `json:"data,omitempty"`
	Text           string `json:"text,omitempty"`
	CandidateID    string `json:"candidate_id,omitempty"`
	Language       string `json:"language,omitempty"`
	CandidateStyle string `json:"candidate_style,omitempty"`
}

type StreamMessage struct {
	SessionID      string         `json:"session_id,omitempty"`
	Type           string         `json:"type"`
	Text           string         `json:"text,omitempty"`
	State          string         `json:"state,omitempty"`
	Language       string         `json:"language,omitempty"`
	Phase          string         `json:"phase,omitempty"`
	PhaseBefore    string         `json:"phase_before,omitempty"`
	ResponseStyle  string         `json:"response_style,omitempty"`
	CandidateStyle string         `json:"candidate_style,omitempty"`
	Report         map[string]any `json:"report,omitempty"`
	ToneSummary    map[string]any `json:"tone_summary,omitempty"`
	EndedReason    string         `json:"ended_reason,omitempty"`
	Status         string         `json:"status,omitempty"`
	Response       string         `json:"response,omitempty"`
	Style          string         `json:"style,omitempty"`
}

type Relay struct {
	stt     *STTClient
	model   ModelStreamer
	prosody ProsodyDetector
	signals *SignalStore
	upgrade websocket.Upgrader
}

type ModelStreamer interface {
	Stream(ctx context.Context, sessionID, candidateID, text, contextBlock, language, candidateStyle string) (<-chan StreamMessage, error)
}

func NewRelay(stt *STTClient, model ModelStreamer) *Relay {
	return &Relay{
		stt:     stt,
		model:   model,
		prosody: FallbackProsodyDetector{},
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
				_ = r.relayAudio(ctx, audio, "", "", "auto", func(out StreamMessage) error {
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
			_ = conn.WriteJSON(StreamMessage{Type: "error", Text: err.Error(), State: "error"})
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
		return r.streamTranscript(ctx, msg.SessionID, msg.CandidateID, msg.Text, msg.Language, styleOrDefault(msg.CandidateStyle), send)
	}
	if msg.Type == "audio" {
		audio, err := decodeAudioPayload(msg.Data)
		if err != nil {
			return err
		}
		return r.relayAudio(ctx, audio, msg.SessionID, msg.CandidateID, msg.Language, send)
	}
	return nil
}

func (r *Relay) relayAudio(ctx context.Context, audio []byte, sessionID, candidateID, language string, send func(StreamMessage) error) error {
	if !isTextBypass(audio) {
		if err := send(StreamMessage{Type: "filler", Text: fillerFor(candidateID, language), State: "thinking", Language: normalizeRelayLanguage(language)}); err != nil {
			return err
		}
	}
	text, err := r.stt.Transcribe(ctx, audio, language)
	if err != nil {
		return err
	}
	style, err := r.prosody.DetectStyle(ctx, audio)
	if err != nil {
		style = "Default"
	}
	return r.streamTranscript(ctx, sessionID, candidateID, text, language, styleOrDefault(style), send)
}

func isTextBypass(audio []byte) bool {
	return strings.HasPrefix(strings.TrimSpace(string(audio)), "text:")
}

func fillerFor(candidateID, language string) string {
	phrases := map[string][]string{
		"en": {
			"Hmm, interesting, let me think about that.",
			"Got it, give me a moment to process that.",
			"Okay, I am taking that in.",
			"That is useful context, one moment.",
		},
		"hi": {
			"Haan, samajh raha hoon, ek moment.",
			"Achha, main isko process kar raha hoon.",
			"Yeh useful context hai, ek second.",
			"Theek hai, main soch raha hoon.",
		},
		"hinglish": {
			"Got it, yeh useful context hai.",
			"Hmm, interesting, ek moment.",
			"Okay, main isko process kar raha hoon.",
			"Samajh gaya, let me think about that.",
		},
	}
	lang := normalizeRelayLanguage(language)
	options := phrases[lang]
	idx := 0
	for _, r := range candidateID {
		idx += int(r)
	}
	if len(options) == 0 {
		return ""
	}
	return options[idx%len(options)]
}

func normalizeRelayLanguage(language string) string {
	switch strings.ToLower(strings.TrimSpace(language)) {
	case "hi", "hindi":
		return "hi"
	case "hinglish":
		return "hinglish"
	default:
		return "en"
	}
}

func (r *Relay) streamTranscript(ctx context.Context, sessionID, candidateID, text, language, candidateStyle string, send func(StreamMessage) error) error {
	stream, err := r.model.Stream(ctx, sessionID, candidateID, text, "relay context: live candidate interview", language, styleOrDefault(candidateStyle))
	if err != nil {
		return err
	}
	aggregated := StreamMessage{
		SessionID: sessionID,
		Type:      "interview_response",
		Status:    "not_completed",
		Language:  normalizeRelayLanguage(language),
		Style:     "Friendly",
	}
	var words []string
	for msg := range stream {
		switch msg.Type {
		case "style":
			aggregated.Style = responseStyleOrDefault(msg.ResponseStyle)
			aggregated.Language = msg.Language
			aggregated.Phase = msg.Phase
		case "phase":
			aggregated.Phase = msg.Phase
			aggregated.PhaseBefore = msg.PhaseBefore
			if msg.Language != "" {
				aggregated.Language = msg.Language
			}
		case "token":
			if msg.Text != "" {
				words = append(words, msg.Text)
			}
			if msg.Language != "" {
				aggregated.Language = msg.Language
			}
		case "report":
			aggregated.Report = msg.Report
			aggregated.ToneSummary = msg.ToneSummary
			aggregated.Status = "completed"
			aggregated.EndedReason = msg.EndedReason
		case "end":
			if msg.Status != "" {
				aggregated.Status = msg.Status
			}
			if msg.State == "completed" || msg.State == "ended" {
				aggregated.Status = "completed"
			}
			if msg.Response != "" {
				aggregated.Response = msg.Response
			}
			if msg.Language != "" {
				aggregated.Language = msg.Language
			}
		case "error":
			return send(msg)
		}
	}
	if aggregated.Response == "" {
		aggregated.Response = strings.Join(words, " ")
	}
	aggregated.Text = aggregated.Response
	aggregated.ResponseStyle = aggregated.Style
	return send(aggregated)
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
