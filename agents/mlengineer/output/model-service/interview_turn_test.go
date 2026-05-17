package main

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestInterviewTurnStartUsesGreetingPath(t *testing.T) {
	t.Setenv("USE_INTERVIEW_TURN", "true")
	server := NewServer(mustBrainA(t), NewBrainB(), NewMockBrainC(), NewStateMachine())

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	_ = writer.WriteField("candidate_id", "turn_start_001")
	_ = writer.WriteField("candidate_name", "Hridesh")
	_ = writer.WriteField("job_description", "Senior backend role requiring API reliability.")
	_ = writer.WriteField("seniority", "senior")
	_ = writer.WriteField("language", "auto")
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/interviews", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rec := httptest.NewRecorder()
	server.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var decoded StartInterviewResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.FirstQuestion == "" {
		t.Fatal("expected greeting response")
	}
	if decoded.ResponseStyle == "" {
		t.Fatal("expected response_style for frontend TTS handoff")
	}
	assertNotContains(t, decoded.FirstQuestion, "Score:")
	assertNotContains(t, decoded.FirstQuestion, "Issues:")
	assertNotContains(t, decoded.FirstQuestion, "system prompt")
	if decoded.Session["use_turn_api"] != true {
		t.Fatalf("expected use_turn_api marker, got %#v", decoded.Session["use_turn_api"])
	}
}

func TestInterviewTurnWebSocketDoesNotEmitLegacyAck(t *testing.T) {
	t.Setenv("USE_INTERVIEW_TURN", "true")
	server := NewServer(mustBrainA(t), NewBrainB(), NewMockBrainC(), NewStateMachine())
	httpServer := httptest.NewServer(server.Routes())
	defer httpServer.Close()

	startSession(t, httpServer.URL, "turn_ws_001")
	url := "ws" + strings.TrimPrefix(httpServer.URL, "http") + "/ws"
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	if err := conn.WriteJSON(ModelRequest{
		CandidateID: "turn_ws_001",
		Text:        "Mujhe ye nahi pata, honestly.",
		Context:     "Senior backend role requiring API reliability.",
		Language:    "auto",
	}); err != nil {
		t.Fatal(err)
	}

	if err := conn.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	var first StreamMessage
	if err := conn.ReadJSON(&first); err != nil {
		t.Fatal(err)
	}
	if first.Type == "ack" {
		t.Fatalf("new interview turn path must not emit legacy ack: %#v", first)
	}
	if first.Type != "style" {
		t.Fatalf("first message type=%q want style", first.Type)
	}
	if first.ResponseStyle == "" {
		t.Fatalf("expected response_style in style frame: %#v", first)
	}
}

func TestInterviewTurnRequestDefaultsCandidateStyle(t *testing.T) {
	server := NewServer(mustBrainA(t), NewBrainB(), NewMockBrainC(), NewStateMachine())
	req := server.newInterviewTurnRequest(&CandidateSession{
		CandidateID:    "style_default_001",
		JobDescription: "Backend role",
		Seniority:      "senior",
	}, "I used Redis.", "en")
	if req.CandidateStyle != "Default" {
		t.Fatalf("candidate_style=%q want Default", req.CandidateStyle)
	}
}

func TestCleanInterviewResponseRemovesWrapperAndPlaceholder(t *testing.T) {
	got := cleanInterviewResponse("Okay, here's what I'd say:\n\nHi [Candidate Name], thanks for joining.", "Hridesh")
	if got != "Hi Hridesh, thanks for joining." {
		t.Fatalf("cleaned=%q", got)
	}
}

func startSession(t *testing.T, baseURL, candidateID string) {
	t.Helper()
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	_ = writer.WriteField("candidate_id", candidateID)
	_ = writer.WriteField("job_description", "Senior backend role requiring API reliability.")
	_ = writer.WriteField("seniority", "senior")
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest(http.MethodPost, baseURL+"/interviews", body)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("start status=%s", resp.Status)
	}
}

func assertNotContains(t *testing.T, text, forbidden string) {
	t.Helper()
	if strings.Contains(strings.ToLower(text), strings.ToLower(forbidden)) {
		t.Fatalf("%q unexpectedly contains %q", text, forbidden)
	}
}
