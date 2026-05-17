package main

import (
	"encoding/json"
	"net/http"
	"sync"

	"github.com/pion/webrtc/v3"
)

type SignalResponse struct {
	SDP  string `json:"sdp"`
	Type string `json:"type,omitempty"`
}

type SignalRequest struct {
	SDP       string `json:"sdp,omitempty"`
	Type      string `json:"type,omitempty"`
	Candidate string `json:"candidate,omitempty"`
}

type SignalStore struct {
	mu   sync.Mutex
	peer *webrtc.PeerConnection
}

func (r *Relay) handleOffer(w http.ResponseWriter, req *http.Request) {
	peer, err := r.newPeerConnection()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	offer, err := peer.CreateOffer(nil)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if err := peer.SetLocalDescription(offer); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	<-webrtc.GatheringCompletePromise(peer)

	r.signals.mu.Lock()
	r.signals.peer = peer
	r.signals.mu.Unlock()
	writeJSON(w, http.StatusOK, SignalResponse{SDP: peer.LocalDescription().SDP, Type: peer.LocalDescription().Type.String()})
}

func (r *Relay) handleAnswer(w http.ResponseWriter, req *http.Request) {
	var body SignalRequest
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	if body.SDP == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing sdp"})
		return
	}
	r.signals.mu.Lock()
	peer := r.signals.peer
	r.signals.mu.Unlock()
	if peer == nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "offer required first"})
		return
	}
	if err := peer.SetRemoteDescription(webrtc.SessionDescription{Type: webrtc.SDPTypeAnswer, SDP: body.SDP}); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (r *Relay) handleICECandidate(w http.ResponseWriter, req *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "accepted"})
}
