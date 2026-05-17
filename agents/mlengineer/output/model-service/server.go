package main

import (
	"context"
	_ "embed"
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

//go:embed manual_test.html
var manualTestHTML string

type ModelRequest struct {
	CandidateID string `json:"candidate_id,omitempty"`
	Text        string `json:"text"`
	Context     string `json:"context"`
	Language    string `json:"language,omitempty"`
}

type StartInterviewResponse struct {
	CandidateID     string         `json:"candidate_id"`
	DurationSeconds int            `json:"duration_seconds"`
	FirstQuestion   string         `json:"first_question"`
	StreamURL       string         `json:"stream_url"`
	StartedAt       time.Time      `json:"started_at"`
	Session         map[string]any `json:"session"`
}

type EndInterviewResponse struct {
	CandidateID string         `json:"candidate_id"`
	Ended       bool           `json:"ended"`
	Report      map[string]any `json:"report"`
	Raw         map[string]any `json:"raw,omitempty"`
}

type StreamMessage struct {
	Type     string `json:"type"`
	Text     string `json:"text,omitempty"`
	State    string `json:"state"`
	Language string `json:"language,omitempty"`
}

type Server struct {
	brainA   *BrainA
	brainB   *BrainB
	brainC   BrainC
	states   *StateMachine
	sessions *SessionStore
	upgrade  websocket.Upgrader
}

func NewServer(a *BrainA, b *BrainB, c BrainC, states *StateMachine) *Server {
	return &Server{
		brainA:   a,
		brainB:   b,
		brainC:   c,
		states:   states,
		sessions: NewSessionStore(),
		upgrade: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		},
	}
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("/interviews", s.handleInterviews)
	mux.HandleFunc("/interviews/", s.handleInterviewByID)
	mux.HandleFunc("/manual-test", s.handleManualTest)
	mux.HandleFunc("/ws", s.handleWS)
	return mux
}

func (s *Server) handleManualTest(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(manualTestHTML))
}

func (s *Server) handleInterviews(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	req, err := decodeStartInterviewRequest(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	session := s.sessions.Start(req)
	if err := s.brainC.LedgerStart(r.Context(), session.CandidateID); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "brain C ledger start failed: " + err.Error()})
		return
	}
	behavior := Behavior{Directive: DirectiveNeutral, Language: normalizeLanguage(req.Language), Ack: ackFor(DirectiveNeutral, normalizeLanguage(req.Language))}
	parts, err := s.askNextQuestion(r.Context(), session, behavior)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "first question failed: " + err.Error()})
		return
	}
	firstQuestion := strings.Join(parts, " ")
	session.AppendAssistant(firstQuestion)
	writeJSON(w, http.StatusCreated, StartInterviewResponse{
		CandidateID:     session.CandidateID,
		DurationSeconds: session.DurationSeconds,
		FirstQuestion:   firstQuestion,
		StreamURL:       "/ws",
		StartedAt:       session.InterviewStartedAt,
		Session: map[string]any{
			"seniority":       session.Seniority,
			"last_topic":      session.LastTopic,
			"last_level":      session.LastLevel,
			"job_description": session.JobDescription,
		},
	})
}

func (s *Server) handleInterviewByID(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/interviews/")
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "missing candidate id"})
		return
	}
	candidateID := parts[0]
	switch {
	case len(parts) == 1 && r.Method == http.MethodGet:
		session, ok := s.sessions.Find(candidateID)
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "interview not found"})
			return
		}
		writeJSON(w, http.StatusOK, session)
	case len(parts) == 2 && parts[1] == "end" && r.Method == http.MethodPost:
		session, ok := s.sessions.Find(candidateID)
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "interview not found"})
			return
		}
		report, raw, err := s.finalizeInterview(r.Context(), session)
		if err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, EndInterviewResponse{CandidateID: candidateID, Ended: true, Report: report, Raw: raw})
	case len(parts) == 2 && parts[1] == "report" && r.Method == http.MethodGet:
		session, ok := s.sessions.Find(candidateID)
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "interview not found"})
			return
		}
		if session.Report == nil {
			writeJSON(w, http.StatusAccepted, map[string]any{"candidate_id": candidateID, "ready": false})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"candidate_id": candidateID, "ready": true, "report": session.Report, "raw": session.ReportRaw})
	default:
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
	}
}

func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := s.upgrade.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("upgrade: %v", err)
		return
	}
	defer conn.Close()

	for {
		var req ModelRequest
		if err := conn.ReadJSON(&req); err != nil {
			return
		}
		if err := s.handleTurn(r.Context(), conn, req); err != nil {
			log.Printf("turn: %v", err)
			return
		}
	}
}

func (s *Server) handleTurn(ctx context.Context, conn *websocket.Conn, req ModelRequest) error {
	start := time.Now()
	req.Text = strings.TrimSpace(req.Text)
	session := s.sessions.Get(req.CandidateID, req.Context)
	behavior := s.brainB.Analyze(req.Text, req.Language)
	if err := conn.WriteJSON(StreamMessage{Type: "ack", Text: behavior.Ack, State: "thinking", Language: behavior.Language}); err != nil {
		return err
	}
	log.Printf("ack_ms=%d", time.Since(start).Milliseconds())

	if behavior.Directive == DirectiveSafety {
		session.AppendUser(req.Text)
		response := localized(behavior.Language,
			"Let's pause the interview. If you might hurt yourself or feel in immediate danger, please contact emergency services or a crisis line now, and reach out to someone near you.",
			"Interview yahin pause karte hain. Agar aap khud ko harm kar sakte hain ya immediate danger feel kar rahe hain, please abhi emergency services ya crisis line se contact karein, aur kisi trusted person ko batayein.",
			"Interview yahin pause karte hain. If you might hurt yourself ya immediate danger feel ho raha hai, please abhi emergency services ya crisis line se contact karein, aur kisi trusted person ko batayein.")
		return s.writeResponse(ctx, conn, response, behavior.Language, session)
	}

	if session.Expired(time.Now()) {
		session.AppendUser(req.Text)
		report, _, err := s.finalizeInterview(ctx, session)
		if err != nil {
			return err
		}
		encoded, _ := json.Marshal(report)
		return s.writeResponse(ctx, conn, "Thanks, we are at the end of the 7 minute interview. "+string(encoded), behavior.Language, session)
	}

	memories, err := s.brainA.Recall(req.Text+" "+req.Context, 3)
	if err != nil {
		return err
	}
	_ = s.states.Next(req.Text, behavior)
	session.RollingSummary = summarizeMemories(memories)
	session.AppendUser(req.Text)

	response, err := s.runOrchestratorTurn(ctx, session, req.Text, behavior)
	if err != nil {
		return err
	}
	if err := s.brainA.StoreTurnWithLanguage(req.Context+"\n"+req.Text, behavior.Language); err != nil {
		return err
	}
	return s.writeResponse(ctx, conn, response, behavior.Language, session)
}

func (s *Server) runOrchestratorTurn(ctx context.Context, session *CandidateSession, candidateText string, behavior Behavior) (string, error) {
	var parts []string
	if session.LastQuestion != "" {
		evalRaw, err := s.brainC.Chat(ctx, session.MessagesWithUserPrompt(WrapEvaluate(session.LastQuestion, candidateText)), ChatOptions{MaxTokens: 400, Temperature: 0.5})
		if err != nil {
			return "", err
		}
		eval := ParseReply(evalRaw)
		if eval.Score0To1 != nil && session.LastTopic != "" {
			if err := s.brainC.LedgerRecord(ctx, session.CandidateID, session.LastTopic, *eval.Score0To1); err != nil {
				log.Printf("ledger record failed: %v", err)
			}
			category := "gentle_correction"
			if *eval.Score0To1 >= 0.75 {
				category = "praise"
			} else if *eval.Score0To1 >= 0.45 {
				category = "encouragement_partial"
			}
			if softener, err := s.brainC.Softener(ctx, category, behavior.Language); err == nil && softener != "" {
				parts = append(parts, softener)
			}
		}
		if eval.Response != "" {
			parts = append(parts, eval.Response)
		}
	}

	if behavior.Directive == DirectiveRephrase {
		if session.LastQuestion != "" {
			raw, err := s.brainC.Chat(ctx, session.MessagesWithUserPrompt(WrapRephrase(session.LastQuestion)), ChatOptions{MaxTokens: 260, Temperature: 0.4})
			if err != nil {
				return "", err
			}
			reply := ParseReply(raw)
			if reply.Filler != "" {
				parts = append(parts, reply.Filler)
			}
			parts = append(parts, reply.Response)
			session.LastQuestion = reply.Response
			return strings.Join(parts, " "), nil
		}
	}

	question, err := s.askNextQuestion(ctx, session, behavior)
	if err != nil {
		return "", err
	}
	parts = append(parts, question...)
	return strings.Join(parts, " "), nil
}

func (s *Server) askNextQuestion(ctx context.Context, session *CandidateSession, behavior Behavior) ([]string, error) {
	var parts []string
	next, err := s.brainC.LedgerNext(ctx, session.CandidateID)
	if err != nil {
		log.Printf("ledger next failed: %v", err)
		next = fallbackLedgerNext(session)
	}
	if next.EndInterview {
		softener, _ := s.brainC.Softener(ctx, "wrap_up", behavior.Language)
		if softener != "" {
			parts = append(parts, softener)
		}
		report, _, err := s.finalizeInterview(ctx, session)
		if err != nil {
			return nil, err
		}
		encoded, _ := json.Marshal(report)
		parts = append(parts, string(encoded))
		return parts, nil
	}
	if session.LastQuestion != "" {
		if transition, err := s.brainC.Softener(ctx, "topic_transition", behavior.Language); err == nil && transition != "" {
			parts = append(parts, transition)
		}
	}
	raw, err := s.brainC.Chat(ctx, []ChatMessage{
		{Role: "system", Content: session.SystemPrompt()},
		{Role: "user", Content: WrapGenerate(next.Topic, next.Level, nil)},
	}, ChatOptions{MaxTokens: 300, Temperature: 0.7})
	if err != nil {
		return nil, err
	}
	reply := ParseReply(raw)
	if reply.Filler != "" {
		parts = append(parts, reply.Filler)
	}
	question := ExtractQuestion(reply)
	parts = append(parts, question)
	session.LastQuestion = question
	session.LastTopic = next.Topic
	session.LastLevel = next.Level
	return parts, nil
}

func (s *Server) finalizeInterview(ctx context.Context, session *CandidateSession) (map[string]any, map[string]any, error) {
	if session.Report != nil {
		return session.Report, session.ReportRaw, nil
	}
	raw, err := s.brainC.Analyze(ctx, session.TranscriptText())
	if err != nil {
		return nil, nil, err
	}
	report := raw
	if nested, ok := raw["analysis"].(map[string]any); ok {
		report = nested
	}
	session.Report = report
	session.ReportRaw = raw
	session.Ended = true
	if err := s.brainC.LedgerEnd(ctx, session.CandidateID); err != nil {
		log.Printf("ledger end failed: %v", err)
	}
	return report, raw, nil
}

func (s *Server) writeResponse(ctx context.Context, conn *websocket.Conn, response string, language string, session *CandidateSession) error {
	session.AppendAssistant(response)
	for token := range streamWords(ctx, strings.TrimSpace(response), 5*time.Millisecond) {
		if err := conn.WriteJSON(StreamMessage{Type: "token", Text: token, State: "speaking", Language: language}); err != nil {
			return err
		}
	}
	return conn.WriteJSON(StreamMessage{Type: "end", State: "listening", Language: language})
}

func summarizeMemories(memories []Memory) string {
	if len(memories) == 0 {
		return ""
	}
	lines := make([]string, 0, len(memories))
	for _, memory := range memories {
		lines = append(lines, memory.Text)
	}
	return strings.Join(lines, "\n")
}

func fallbackLedgerNext(session *CandidateSession) LedgerNext {
	topics := []string{"Redis caching", "API reliability", "deployment rollback"}
	idx := len(session.Transcript) % len(topics)
	return LedgerNext{Topic: topics[idx], Level: session.LastLevel}
}

func normalizeLanguage(language string) string {
	switch strings.ToLower(strings.TrimSpace(language)) {
	case "hi", "hindi":
		return "hi"
	case "hinglish":
		return "hinglish"
	default:
		return "en"
	}
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
