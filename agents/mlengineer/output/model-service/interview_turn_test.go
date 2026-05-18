package main

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestInterviewCreateReturnsSessionID(t *testing.T) {
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
	var decoded CreateInterviewResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.SessionID == "" {
		t.Fatal("expected session_id")
	}
	if decoded.Status != "created" {
		t.Fatalf("status=%q want created", decoded.Status)
	}
}

func TestInterviewStartUsesGreetingPath(t *testing.T) {
	t.Setenv("USE_INTERVIEW_TURN", "true")
	server := NewServer(mustBrainA(t), NewBrainB(), NewMockBrainC(), NewStateMachine())
	httpServer := httptest.NewServer(server.Routes())
	defer httpServer.Close()

	sessionID := createSession(t, httpServer.URL, "turn_start_001")
	req, err := http.NewRequest(http.MethodPost, httpServer.URL+"/interviews/"+sessionID+"/start", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("start status=%s", resp.Status)
	}
	var decoded StartInterviewResponse
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Response == "" {
		t.Fatal("expected greeting response")
	}
	if decoded.Style == "" {
		t.Fatal("expected style for frontend TTS handoff")
	}
	assertNotContains(t, decoded.Response, "Score:")
	assertNotContains(t, decoded.Response, "Issues:")
	assertNotContains(t, decoded.Response, "system prompt")
	if decoded.Session["use_turn_api"] != true {
		t.Fatalf("expected use_turn_api marker, got %#v", decoded.Session["use_turn_api"])
	}
}

func TestGetInterviewBySessionIDReturnsFrontendShape(t *testing.T) {
	t.Setenv("USE_INTERVIEW_TURN", "true")
	server := NewServer(mustBrainA(t), NewBrainB(), NewMockBrainC(), NewStateMachine())
	httpServer := httptest.NewServer(server.Routes())
	defer httpServer.Close()

	sessionID := createSession(t, httpServer.URL, "turn_get_001")
	resp, err := http.Get(httpServer.URL + "/interviews/" + sessionID)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%s", resp.Status)
	}
	var decoded GetInterviewResponse
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.SessionID != sessionID {
		t.Fatalf("session_id=%q want %q", decoded.SessionID, sessionID)
	}
	if decoded.CandidateID != "turn_get_001" {
		t.Fatalf("candidate_id=%q", decoded.CandidateID)
	}
	if decoded.Status != "not_completed" {
		t.Fatalf("status=%q want not_completed", decoded.Status)
	}
}

func TestInterviewTurnWebSocketDoesNotEmitLegacyAck(t *testing.T) {
	t.Setenv("USE_INTERVIEW_TURN", "true")
	server := NewServer(mustBrainA(t), NewBrainB(), NewMockBrainC(), NewStateMachine())
	httpServer := httptest.NewServer(server.Routes())
	defer httpServer.Close()

	sessionID := startSession(t, httpServer.URL, "turn_ws_001")
	url := "ws" + strings.TrimPrefix(httpServer.URL, "http") + "/ws"
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	if err := conn.WriteJSON(ModelRequest{
		SessionID:   sessionID,
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

func TestEmptyTranscriptRepeatPromptDoesNotEndSession(t *testing.T) {
	t.Setenv("USE_INTERVIEW_TURN", "true")
	brainC := &scriptedBrainC{
		MockBrainC: NewMockBrainC(),
		turnResp: InterviewTurnResponse{
			ResponseText:     "Sorry, I didn't catch that. Could you say that again?",
			Language:         "en",
			Phase:            "interview",
			ResponseStyle:    "Friendly",
			RepeatPrompt:     true,
			SessionShouldEnd: false,
			Ledger:           LedgerSummary{CandidateID: "empty_repeat_001", Phase: "interview"},
		},
	}
	server := NewServer(mustBrainA(t), NewBrainB(), brainC, NewStateMachine())
	session := server.sessions.Start(StartInterviewRequest{
		CandidateID:    "empty_repeat_001",
		CandidateName:  "Nirav",
		JobDescription: "Senior backend role",
		Seniority:      "senior",
	})
	initialID := session.CandidateID
	httpServer := httptest.NewServer(server.Routes())
	defer httpServer.Close()

	conn := dialModelWS(t, httpServer.URL)
	defer conn.Close()
	if err := conn.WriteJSON(ModelRequest{SessionID: session.SessionID, CandidateID: session.CandidateID, Text: "", Language: "en"}); err != nil {
		t.Fatal(err)
	}
	messages := readUntilEnd(t, conn)
	if len(messages) == 0 {
		t.Fatal("expected websocket response")
	}
	if session.Ended {
		t.Fatal("session must not end on empty transcript repeat prompt")
	}
	if session.CandidateID != initialID {
		t.Fatalf("candidate_id changed: %q -> %q", initialID, session.CandidateID)
	}
	if brainC.analyzeCalls != 0 {
		t.Fatalf("analyze calls=%d want 0", brainC.analyzeCalls)
	}
	if brainC.endLedgerCalls != 0 {
		t.Fatalf("ledger end calls=%d want 0", brainC.endLedgerCalls)
	}
	if len(brainC.turnRequests) != 1 {
		t.Fatalf("turn requests=%d want 1", len(brainC.turnRequests))
	}
	if brainC.turnRequests[0].Transcript != "" {
		t.Fatalf("turn transcript=%q want empty", brainC.turnRequests[0].Transcript)
	}
}

func TestSessionShouldEndTriggersAnalyzeAndLedgerEnd(t *testing.T) {
	t.Setenv("USE_INTERVIEW_TURN", "true")
	brainC := &scriptedBrainC{
		MockBrainC: NewMockBrainC(),
		turnResp: InterviewTurnResponse{
			ResponseText:     "Thanks for your time, that wraps things up.",
			Language:         "en",
			Phase:            "wrap",
			ResponseStyle:    "Friendly",
			SessionShouldEnd: true,
			Ledger:           LedgerSummary{CandidateID: "should_end_001", Phase: "wrap", Ended: true},
		},
	}
	server := NewServer(mustBrainA(t), NewBrainB(), brainC, NewStateMachine())
	session := server.sessions.Start(StartInterviewRequest{CandidateID: "should_end_001", JobDescription: "Backend role"})
	httpServer := httptest.NewServer(server.Routes())
	defer httpServer.Close()

	conn := dialModelWS(t, httpServer.URL)
	defer conn.Close()
	if err := conn.WriteJSON(ModelRequest{SessionID: session.SessionID, CandidateID: session.CandidateID, Text: "Sounds good.", Language: "en"}); err != nil {
		t.Fatal(err)
	}
	_ = readUntilReport(t, conn)
	if !session.Ended {
		t.Fatal("session should be ended after session_should_end")
	}
	if brainC.analyzeCalls != 1 {
		t.Fatalf("analyze calls=%d want 1", brainC.analyzeCalls)
	}
	if brainC.endLedgerCalls != 1 {
		t.Fatalf("ledger end calls=%d want 1", brainC.endLedgerCalls)
	}
}

func TestCandidateIDStableAcrossWebSocketReconnect(t *testing.T) {
	t.Setenv("USE_INTERVIEW_TURN", "true")
	brainC := &scriptedBrainC{
		MockBrainC: NewMockBrainC(),
		turnResp: InterviewTurnResponse{
			ResponseText:     "That context helps. What metric did you use?",
			Language:         "en",
			Phase:            "interview",
			ResponseStyle:    "Friendly",
			SessionShouldEnd: false,
			Ledger:           LedgerSummary{CandidateID: "stable_reconnect_001", Phase: "interview"},
		},
	}
	server := NewServer(mustBrainA(t), NewBrainB(), brainC, NewStateMachine())
	session := server.sessions.Start(StartInterviewRequest{CandidateID: "stable_reconnect_001", JobDescription: "Backend role"})
	httpServer := httptest.NewServer(server.Routes())
	defer httpServer.Close()

	for i := 0; i < 2; i++ {
		conn := dialModelWS(t, httpServer.URL)
		if err := conn.WriteJSON(ModelRequest{SessionID: session.SessionID, Text: "I measured p95 latency.", Language: "en"}); err != nil {
			t.Fatal(err)
		}
		_ = readUntilEnd(t, conn)
		_ = conn.Close()
	}

	if len(brainC.turnRequests) != 2 {
		t.Fatalf("turn requests=%d want 2", len(brainC.turnRequests))
	}
	for i, req := range brainC.turnRequests {
		if req.CandidateID != "stable_reconnect_001" {
			t.Fatalf("turn %d candidate_id=%q want stable_reconnect_001", i, req.CandidateID)
		}
	}
	if session.CandidateID != "stable_reconnect_001" {
		t.Fatalf("session candidate_id=%q", session.CandidateID)
	}
}

func createSession(t *testing.T, baseURL, candidateID string) string {
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
		t.Fatalf("create status=%s", resp.Status)
	}
	var decoded CreateInterviewResponse
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.SessionID == "" {
		t.Fatal("missing session_id")
	}
	return decoded.SessionID
}

func startSession(t *testing.T, baseURL, candidateID string) string {
	t.Helper()
	sessionID := createSession(t, baseURL, candidateID)
	req, err := http.NewRequest(http.MethodPost, baseURL+"/interviews/"+sessionID+"/start", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("start status=%s", resp.Status)
	}
	return sessionID
}

func assertNotContains(t *testing.T, text, forbidden string) {
	t.Helper()
	if strings.Contains(strings.ToLower(text), strings.ToLower(forbidden)) {
		t.Fatalf("%q unexpectedly contains %q", text, forbidden)
	}
}

func dialModelWS(t *testing.T, baseURL string) *websocket.Conn {
	t.Helper()
	url := "ws" + strings.TrimPrefix(baseURL, "http") + "/ws"
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := conn.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatal(err)
	}
	return conn
}

func readUntilEnd(t *testing.T, conn *websocket.Conn) []StreamMessage {
	t.Helper()
	var messages []StreamMessage
	for {
		var msg StreamMessage
		if err := conn.ReadJSON(&msg); err != nil {
			t.Fatal(err)
		}
		messages = append(messages, msg)
		if msg.Type == "end" {
			return messages
		}
	}
}

func readUntilReport(t *testing.T, conn *websocket.Conn) []StreamMessage {
	t.Helper()
	var messages []StreamMessage
	for {
		var msg StreamMessage
		if err := conn.ReadJSON(&msg); err != nil {
			t.Fatal(err)
		}
		messages = append(messages, msg)
		if msg.Type == "report" {
			return messages
		}
	}
}

type scriptedBrainC struct {
	*MockBrainC
	turnResp       InterviewTurnResponse
	turnRequests   []InterviewTurnRequest
	analyzeCalls   int
	endLedgerCalls int
}

func (b *scriptedBrainC) InterviewTurn(ctx context.Context, req InterviewTurnRequest) (InterviewTurnResponse, error) {
	b.turnRequests = append(b.turnRequests, req)
	resp := b.turnResp
	if resp.Language == "" {
		resp.Language = "en"
	}
	if resp.ResponseStyle == "" {
		resp.ResponseStyle = "Friendly"
	}
	if resp.Ledger.CandidateID == "" {
		resp.Ledger.CandidateID = req.CandidateID
	}
	return resp, nil
}

func (b *scriptedBrainC) Analyze(ctx context.Context, transcript, candidateID string) (map[string]any, error) {
	b.analyzeCalls++
	return b.MockBrainC.Analyze(ctx, transcript, candidateID)
}

func (b *scriptedBrainC) LedgerEnd(ctx context.Context, candidateID string) error {
	b.endLedgerCalls++
	return b.MockBrainC.LedgerEnd(ctx, candidateID)
}
