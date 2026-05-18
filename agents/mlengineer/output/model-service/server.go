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
	SessionID      string `json:"session_id,omitempty"`
	CandidateID    string `json:"candidate_id,omitempty"`
	Text           string `json:"text"`
	Context        string `json:"context"`
	Language       string `json:"language,omitempty"`
	CandidateStyle string `json:"candidate_style,omitempty"`
}

type StartInterviewResponse struct {
	SessionID       string         `json:"session_id,omitempty"`
	CandidateID     string         `json:"candidate_id"`
	DurationSeconds int            `json:"duration_seconds"`
	Response        string         `json:"response,omitempty"`
	FirstQuestion   string         `json:"first_question"`
	ResponseStyle   string         `json:"response_style,omitempty"`
	Style           string         `json:"style,omitempty"`
	Status          string         `json:"status,omitempty"`
	StreamURL       string         `json:"stream_url"`
	StartedAt       time.Time      `json:"started_at"`
	Session         map[string]any `json:"session"`
}

type EndInterviewResponse struct {
	SessionID   string         `json:"session_id,omitempty"`
	CandidateID string         `json:"candidate_id"`
	Ended       bool           `json:"ended"`
	Report      map[string]any `json:"report"`
	ToneSummary map[string]any `json:"tone_summary,omitempty"`
	Raw         map[string]any `json:"raw,omitempty"`
}

type StreamMessage struct {
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
}

type Server struct {
	brainA   *BrainA
	brainB   *BrainB
	brainC   BrainC
	states   *StateMachine
	sessions *SessionStore
	upgrade  websocket.Upgrader
	useTurns bool
}

type CreateInterviewResponse struct {
	SessionID       string `json:"session_id"`
	CandidateID     string `json:"candidate_id"`
	DurationSeconds int    `json:"duration_seconds"`
	Status          string `json:"status"`
	StartURL        string `json:"start_url"`
	StreamURL       string `json:"stream_url"`
}

type GetInterviewResponse struct {
	SessionID         string         `json:"session_id"`
	CandidateID       string         `json:"candidate_id"`
	CandidateName     string         `json:"candidate_name,omitempty"`
	DurationSeconds   int            `json:"duration_seconds"`
	StartedAt         *time.Time     `json:"started_at,omitempty"`
	Status            string         `json:"status"`
	Phase             string         `json:"phase,omitempty"`
	Language          string         `json:"language,omitempty"`
	JobDescription    string         `json:"job_description,omitempty"`
	Seniority         string         `json:"seniority,omitempty"`
	LastResponseStyle string         `json:"last_response_style,omitempty"`
	ReportReady       bool           `json:"report_ready"`
	ToneSummary       map[string]any `json:"tone_summary,omitempty"`
}

func NewServer(a *BrainA, b *BrainB, c BrainC, states *StateMachine) *Server {
	return &Server{
		brainA:   a,
		brainB:   b,
		brainC:   c,
		states:   states,
		sessions: NewSessionStore(),
		useTurns: envBool("USE_INTERVIEW_TURN", true),
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
	return withCORS(mux)
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
	if r.URL.Query().Get("start") != "true" {
		if strings.TrimSpace(req.ResumeText+" "+req.JobDescription) != "" {
			if err := s.brainA.StoreTurnWithLanguage(req.ResumeText+"\n"+req.JobDescription, normalizeLanguage(req.Language)); err != nil {
				log.Printf("brain A store interview create context failed: %v", err)
			}
		}
		writeJSON(w, http.StatusCreated, CreateInterviewResponse{
			SessionID:       session.SessionID,
			CandidateID:     session.CandidateID,
			DurationSeconds: session.DurationSeconds,
			Status:          "created",
			StartURL:        "/interviews/" + session.SessionID + "/start",
			StreamURL:       "/ws",
		})
		return
	}
	if s.useTurns {
		s.startInterviewClock(session)
		if err := s.brainC.LedgerStart(r.Context(), session.CandidateID); err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "brain C ledger start failed: " + err.Error()})
			return
		}
		start := time.Now()
		turn, err := s.brainC.InterviewTurn(r.Context(), s.newInterviewTurnRequest(session, "", normalizeLanguage(req.Language)))
		if err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "brain C interview turn failed: " + err.Error()})
			return
		}
		logInterviewTurnLatency("opening", session, "", normalizeLanguage(req.Language), start, turn)
		turn.ResponseText = cleanInterviewResponse(turn.ResponseText, session.CandidateName)
		s.applyInterviewTurn(session, turn)
		session.AppendAssistant(turn.ResponseText)
		writeJSON(w, http.StatusCreated, StartInterviewResponse{
			SessionID:       session.SessionID,
			CandidateID:     session.CandidateID,
			DurationSeconds: session.DurationSeconds,
			Response:        turn.ResponseText,
			FirstQuestion:   turn.ResponseText,
			ResponseStyle:   responseStyleOrDefault(turn.ResponseStyle),
			Style:           responseStyleOrDefault(turn.ResponseStyle),
			Status:          interviewStatus(session),
			StreamURL:       "/ws",
			StartedAt:       session.InterviewStartedAt,
			Session: map[string]any{
				"seniority":       session.Seniority,
				"job_description": session.JobDescription,
				"phase":           session.Phase,
				"language":        session.Language,
				"response_style":  responseStyleOrDefault(turn.ResponseStyle),
				"use_turn_api":    true,
			},
		})
		return
	}
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

func (s *Server) startInterview(ctx context.Context, session *CandidateSession) (StartInterviewResponse, error) {
	s.startInterviewClock(session)
	if err := s.brainC.LedgerStart(ctx, session.CandidateID); err != nil {
		return StartInterviewResponse{}, err
	}
	start := time.Now()
	turn, err := s.brainC.InterviewTurn(ctx, s.newInterviewTurnRequest(session, "", session.Language))
	if err != nil {
		return StartInterviewResponse{}, err
	}
	logInterviewTurnLatency("opening", session, "", session.Language, start, turn)
	turn.ResponseText = cleanInterviewResponse(turn.ResponseText, session.CandidateName)
	s.applyInterviewTurn(session, turn)
	session.AppendAssistant(turn.ResponseText)
	style := responseStyleOrDefault(turn.ResponseStyle)
	return StartInterviewResponse{
		SessionID:       session.SessionID,
		CandidateID:     session.CandidateID,
		DurationSeconds: session.DurationSeconds,
		Response:        turn.ResponseText,
		FirstQuestion:   turn.ResponseText,
		ResponseStyle:   style,
		Style:           style,
		Status:          interviewStatus(session),
		StreamURL:       "/ws",
		StartedAt:       session.InterviewStartedAt,
		Session: map[string]any{
			"session_id":      session.SessionID,
			"seniority":       session.Seniority,
			"job_description": session.JobDescription,
			"phase":           session.Phase,
			"language":        session.Language,
			"response_style":  style,
			"use_turn_api":    true,
		},
	}, nil
}

func (s *Server) startInterviewClock(session *CandidateSession) {
	session.MarkStarted(time.Now())
}

func newGetInterviewResponse(session *CandidateSession) GetInterviewResponse {
	var startedAt *time.Time
	if !session.InterviewStartedAt.IsZero() {
		started := session.InterviewStartedAt
		startedAt = &started
	}
	return GetInterviewResponse{
		SessionID:         session.SessionID,
		CandidateID:       session.CandidateID,
		CandidateName:     session.CandidateName,
		DurationSeconds:   session.DurationSeconds,
		StartedAt:         startedAt,
		Status:            interviewStatus(session),
		Phase:             session.Phase,
		Language:          session.Language,
		JobDescription:    session.JobDescription,
		Seniority:         session.Seniority,
		LastResponseStyle: session.LastResponseStyle,
		ReportReady:       session.Report != nil,
		ToneSummary:       session.ToneSummary,
	}
}

func (s *Server) sessionForRequest(req ModelRequest) *CandidateSession {
	key := strings.TrimSpace(req.SessionID)
	if key == "" {
		key = strings.TrimSpace(req.CandidateID)
	}
	if session, ok := s.sessions.Find(key); ok {
		return session
	}
	return s.sessions.Get(key, req.Context)
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
	case len(parts) == 2 && parts[1] == "start" && r.Method == http.MethodPost:
		session, ok := s.sessions.Find(candidateID)
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "interview not found"})
			return
		}
		resp, err := s.startInterview(r.Context(), session)
		if err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, resp)
	case len(parts) == 1 && r.Method == http.MethodGet:
		session, ok := s.sessions.Find(candidateID)
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "interview not found"})
			return
		}
		writeJSON(w, http.StatusOK, newGetInterviewResponse(session))
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
		writeJSON(w, http.StatusOK, EndInterviewResponse{SessionID: session.SessionID, CandidateID: session.CandidateID, Ended: true, Report: report, ToneSummary: session.ToneSummary, Raw: raw})
	case len(parts) == 2 && parts[1] == "report" && r.Method == http.MethodGet:
		session, ok := s.sessions.Find(candidateID)
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "interview not found"})
			return
		}
		if session.Report == nil {
			writeJSON(w, http.StatusAccepted, map[string]any{"session_id": session.SessionID, "candidate_id": session.CandidateID, "ready": false})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"session_id": session.SessionID, "candidate_id": session.CandidateID, "ready": true, "report": session.Report, "tone_summary": session.ToneSummary, "raw": session.ReportRaw})
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
	if s.useTurns {
		return s.handleInterviewTurn(ctx, conn, req)
	}
	start := time.Now()
	req.Text = strings.TrimSpace(req.Text)
	session := s.sessionForRequest(req)
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

func (s *Server) handleInterviewTurn(ctx context.Context, conn *websocket.Conn, req ModelRequest) error {
	receivedAt := time.Now()
	req.Text = strings.TrimSpace(req.Text)
	session := s.sessionForRequest(req)
	s.startInterviewClock(session)
	if session.Ended {
		return s.writeCompletedResponse(ctx, conn, closingResponse(session), session.Language, "Friendly", session)
	}
	if session.ClosingPending || session.Expired(time.Now()) {
		session.AppendUser(req.Text)
		session.ClosingPending = false
		session.Ended = true
		if err := s.writeCompletedResponse(ctx, conn, closingResponse(session), session.Language, "Friendly", session); err != nil {
			return err
		}
		_, _, err := s.finalizeInterview(ctx, session)
		return err
	}

	behavior := s.brainB.Analyze(req.Text, req.Language)
	if strings.TrimSpace(req.CandidateStyle) != "" {
		session.LastCandidateStyle = strings.TrimSpace(req.CandidateStyle)
	}
	session.AppendUser(req.Text)
	memories, err := s.brainA.Recall(req.Text+" "+req.Context, 3)
	if err != nil {
		return err
	}
	session.RollingSummary = summarizeMemories(memories)

	turnStart := time.Now()
	turn, err := s.brainC.InterviewTurn(ctx, s.newInterviewTurnRequest(session, req.Text, behavior.Language))
	if err != nil {
		return err
	}
	logInterviewTurnLatency("candidate_turn", session, req.Text, behavior.Language, turnStart, turn)
	turn.ResponseText = cleanInterviewResponse(turn.ResponseText, session.CandidateName)
	s.applyInterviewTurn(session, turn)
	if err := s.brainA.StoreTurnWithLanguage(req.Context+"\n"+req.Text, turn.Language); err != nil {
		return err
	}
	if turn.PhaseChanged && turn.Phase != "" {
		if err := conn.WriteJSON(StreamMessage{Type: "phase", Phase: turn.Phase, PhaseBefore: turn.PhaseBefore, Language: turn.Language}); err != nil {
			return err
		}
	}
	emitStart := time.Now()
	if err := s.writeStyledResponse(ctx, conn, turn.ResponseText, turn.Language, turn.ResponseStyle, turn.Phase, session); err != nil {
		return err
	}
	log.Printf("ws_emit_latency path=candidate_turn session_id=%s candidate_id=%s received_to_first_emit_ms=%d emit_ms=%d transcript_chars=%d language=%s",
		session.SessionID,
		session.CandidateID,
		emitStart.Sub(receivedAt).Milliseconds(),
		time.Since(emitStart).Milliseconds(),
		len(req.Text),
		turn.Language,
	)
	if session.timeRemaining(time.Now()) <= time.Minute {
		session.ClosingPending = true
	}
	if turn.Safety.Triggered {
		report, _, err := s.finalizeInterview(ctx, session)
		if err == nil {
			_ = conn.WriteJSON(StreamMessage{Type: "report", Report: report, ToneSummary: session.ToneSummary, EndedReason: "safety"})
		}
		session.Ended = true
		return nil
	}
	if turn.Phase == "wrap" && turn.Ledger.Ended {
		report, _, err := s.finalizeInterview(ctx, session)
		if err == nil {
			_ = conn.WriteJSON(StreamMessage{Type: "report", Report: report, ToneSummary: session.ToneSummary, EndedReason: "natural"})
		}
		session.Ended = true
	}
	return nil
}

func logInterviewTurnLatency(path string, session *CandidateSession, transcript string, languageHint string, start time.Time, turn InterviewTurnResponse) {
	log.Printf("interview_turn_latency path=%s session_id=%s candidate_id=%s phase=%s language=%s language_hint=%s transcript_chars=%d turn_ms=%d brainc_debug=%v",
		path,
		session.SessionID,
		session.CandidateID,
		turn.Phase,
		turn.Language,
		normalizeLanguageHint(languageHint),
		len(transcript),
		time.Since(start).Milliseconds(),
		turn.DebugTimings,
	)
}

func (s *Server) newInterviewTurnRequest(session *CandidateSession, transcript string, languageHint string) InterviewTurnRequest {
	candidateStyle := strings.TrimSpace(session.LastCandidateStyle)
	if candidateStyle == "" {
		candidateStyle = "Default"
	}
	return InterviewTurnRequest{
		CandidateID:    session.CandidateID,
		Transcript:     transcript,
		JobDescription: session.JobDescription,
		Seniority:      session.Seniority,
		CandidateName:  session.CandidateName,
		LanguageHint:   normalizeLanguageHint(languageHint),
		Region:         "IN",
		CandidateStyle: candidateStyle,
	}
}

func (s *Server) applyInterviewTurn(session *CandidateSession, turn InterviewTurnResponse) {
	if turn.Phase != "" {
		session.Phase = turn.Phase
	}
	if turn.Language != "" {
		session.Language = turn.Language
	}
	if turn.CandidateStyle != "" {
		session.LastCandidateStyle = turn.CandidateStyle
	}
	if turn.ResponseStyle != "" {
		session.LastResponseStyle = turn.ResponseStyle
	}
	if turn.TopicHint != nil {
		session.LastTopic = turn.TopicHint.Topic
		session.LastLevel = turn.TopicHint.Level
	}
	if turn.Ledger.Ended {
		session.Ended = true
	}
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
	raw, err := s.brainC.Analyze(ctx, session.TranscriptText(), session.CandidateID)
	if err != nil {
		return nil, nil, err
	}
	report := raw
	if nested, ok := raw["analysis"].(map[string]any); ok {
		report = nested
	}
	if tone, ok := raw["tone_summary"].(map[string]any); ok {
		session.ToneSummary = tone
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
	return s.writeTokenResponse(ctx, conn, response, language, session)
}

func (s *Server) writeStyledResponse(ctx context.Context, conn *websocket.Conn, response string, language string, responseStyle string, phase string, session *CandidateSession) error {
	style := responseStyleOrDefault(responseStyle)
	if err := conn.WriteJSON(StreamMessage{
		Type:          "style",
		ResponseStyle: style,
		Language:      language,
		Phase:         phase,
	}); err != nil {
		return err
	}
	return s.writeTokenResponse(ctx, conn, response, language, session)
}

func (s *Server) writeCompletedResponse(ctx context.Context, conn *websocket.Conn, response string, language string, responseStyle string, session *CandidateSession) error {
	style := responseStyleOrDefault(responseStyle)
	if err := conn.WriteJSON(StreamMessage{
		Type:          "style",
		ResponseStyle: style,
		Language:      language,
		Phase:         "wrap",
		Status:        "completed",
	}); err != nil {
		return err
	}
	session.AppendAssistant(response)
	for token := range streamWords(ctx, strings.TrimSpace(response), 5*time.Millisecond) {
		if err := conn.WriteJSON(StreamMessage{Type: "token", Text: token, State: "speaking", Language: language}); err != nil {
			return err
		}
	}
	return conn.WriteJSON(StreamMessage{Type: "end", State: "completed", Language: language, Status: "completed", Response: response})
}

func (s *Server) writeTokenResponse(ctx context.Context, conn *websocket.Conn, response string, language string, session *CandidateSession) error {
	session.AppendAssistant(response)
	for token := range streamWords(ctx, strings.TrimSpace(response), 5*time.Millisecond) {
		if err := conn.WriteJSON(StreamMessage{Type: "token", Text: token, State: "speaking", Language: language}); err != nil {
			return err
		}
	}
	return conn.WriteJSON(StreamMessage{Type: "end", State: "listening", Language: language})
}

func responseStyleOrDefault(style string) string {
	style = strings.TrimSpace(style)
	if style == "" {
		return "Friendly"
	}
	return style
}

func interviewStatus(session *CandidateSession) string {
	if session.Ended {
		return "completed"
	}
	return "not_completed"
}

func closingResponse(session *CandidateSession) string {
	name := strings.TrimSpace(session.CandidateName)
	if name == "" {
		name = "there"
	}
	return "Thanks " + name + " for joining. It was really nice talking to you, and I hope your interview experience was good. Have a nice day."
}

func cleanInterviewResponse(text, candidateName string) string {
	cleaned := strings.TrimSpace(text)
	prefixes := []string{
		"Okay, here's what I'd say:",
		"Here's what I'd say:",
		"Here is what I'd say:",
		"Okay, here is what I'd say:",
	}
	for _, prefix := range prefixes {
		if strings.HasPrefix(strings.ToLower(cleaned), strings.ToLower(prefix)) {
			cleaned = strings.TrimSpace(cleaned[len(prefix):])
			break
		}
	}
	name := strings.TrimSpace(candidateName)
	if name == "" {
		name = "there"
	}
	replacements := []struct {
		old string
		new string
	}{
		{"[Candidate Name]", name},
		{"[candidate name]", name},
		{"Candidate Name", name},
	}
	for _, replacement := range replacements {
		cleaned = strings.ReplaceAll(cleaned, replacement.old, replacement.new)
	}
	return strings.TrimSpace(cleaned)
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

func normalizeLanguageHint(language string) string {
	switch strings.ToLower(strings.TrimSpace(language)) {
	case "hi", "hindi":
		return "hi"
	case "hinglish":
		return "hinglish"
	case "en", "english":
		return "en"
	default:
		return ""
	}
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Requested-With")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
