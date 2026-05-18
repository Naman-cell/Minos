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

func TestInterviewStartSendsOpeningTurnBeforeAudioWithCandidateName(t *testing.T) {
	t.Setenv("USE_INTERVIEW_TURN", "true")
	brainC := &recordingBrainC{MockBrainC: NewMockBrainC()}
	server := NewServer(mustBrainA(t), NewBrainB(), brainC, NewStateMachine())
	httpServer := httptest.NewServer(server.Routes())
	defer httpServer.Close()

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	_ = writer.WriteField("candidate_id", "opening_contract_001")
	_ = writer.WriteField("candidate_name", "Nirav")
	_ = writer.WriteField("job_description", "Senior backend role requiring API reliability.")
	_ = writer.WriteField("seniority", "senior")
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest(http.MethodPost, httpServer.URL+"/interviews", body)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var created CreateInterviewResponse
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}

	startReq, err := http.NewRequest(http.MethodPost, httpServer.URL+"/interviews/"+created.SessionID+"/start", nil)
	if err != nil {
		t.Fatal(err)
	}
	startResp, err := http.DefaultClient.Do(startReq)
	if err != nil {
		t.Fatal(err)
	}
	defer startResp.Body.Close()
	if startResp.StatusCode != http.StatusOK {
		t.Fatalf("start status=%s", startResp.Status)
	}

	if len(brainC.turnRequests) != 1 {
		t.Fatalf("turn requests=%d want 1", len(brainC.turnRequests))
	}
	got := brainC.turnRequests[0]
	if got.Transcript != "" {
		t.Fatalf("opening transcript=%q want empty", got.Transcript)
	}
	if got.CandidateName != "Nirav" {
		t.Fatalf("candidate_name=%q want Nirav", got.CandidateName)
	}
	if got.CandidateStyle != "Default" {
		t.Fatalf("candidate_style=%q want Default", got.CandidateStyle)
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

type recordingBrainC struct {
	*MockBrainC
	turnRequests []InterviewTurnRequest
}

func (b *recordingBrainC) InterviewTurn(ctx context.Context, req InterviewTurnRequest) (InterviewTurnResponse, error) {
	b.turnRequests = append(b.turnRequests, req)
	return b.MockBrainC.InterviewTurn(ctx, req)
}
