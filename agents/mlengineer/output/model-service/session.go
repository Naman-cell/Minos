package main

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

type TranscriptTurn struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type CandidateSession struct {
	SessionID          string
	CandidateID        string
	CandidateName      string
	ResumeText         string
	JobDescription     string
	Seniority          string
	DurationSeconds    int
	Transcript         []TranscriptTurn
	Phase              string
	Language           string
	LastCandidateStyle string
	LastResponseStyle  string
	LastQuestion       string
	LastTopic          string
	LastLevel          int
	InSettlingPhase    bool
	SettlingStartedAt  time.Time
	InterviewStartedAt time.Time
	RollingSummary     string
	Ended              bool
	ClosingPending     bool
	Report             map[string]any
	ReportRaw          map[string]any
	ToneSummary        map[string]any
}

type SessionStore struct {
	mu       sync.Mutex
	sessions map[string]*CandidateSession
}

func NewSessionStore() *SessionStore {
	return &SessionStore{sessions: make(map[string]*CandidateSession)}
}

type StartInterviewRequest struct {
	CandidateID     string `json:"candidate_id"`
	CandidateName   string `json:"candidate_name"`
	ResumeText      string `json:"resume_text"`
	JobDescription  string `json:"job_description"`
	Seniority       string `json:"seniority"`
	DurationSeconds int    `json:"duration_seconds"`
	Language        string `json:"language"`
}

func (s *SessionStore) Start(req StartInterviewRequest) *CandidateSession {
	return s.Create(req)
}

func (s *SessionStore) Create(req StartInterviewRequest) *CandidateSession {
	candidateID := strings.TrimSpace(req.CandidateID)
	if candidateID == "" {
		candidateID = fmt.Sprintf("candidate_%d", time.Now().UnixNano())
	}
	sessionID := fmt.Sprintf("session_%d", time.Now().UnixNano())
	duration := req.DurationSeconds
	if duration <= 0 {
		duration = 7 * 60
	}
	seniority := strings.TrimSpace(req.Seniority)
	if seniority == "" {
		seniority = "mid"
	}
	now := time.Now()
	session := &CandidateSession{
		SessionID:         sessionID,
		CandidateID:       candidateID,
		CandidateName:     strings.TrimSpace(req.CandidateName),
		ResumeText:        req.ResumeText,
		JobDescription:    req.JobDescription,
		Seniority:         seniority,
		DurationSeconds:   duration,
		Language:          normalizeLanguage(req.Language),
		LastLevel:         2,
		InSettlingPhase:   true,
		SettlingStartedAt: now,
	}
	s.mu.Lock()
	s.sessions[sessionID] = session
	s.sessions[candidateID] = session
	s.mu.Unlock()
	return session
}

func (s *SessionStore) Get(candidateID, context string) *CandidateSession {
	if strings.TrimSpace(candidateID) == "" {
		candidateID = "default"
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if session, ok := s.sessions[candidateID]; ok {
		if strings.TrimSpace(context) != "" {
			session.JobDescription = context
		}
		return session
	}
	now := time.Now()
	session := &CandidateSession{
		SessionID:          candidateID,
		CandidateID:        candidateID,
		JobDescription:     context,
		Seniority:          "mid",
		DurationSeconds:    7 * 60,
		Language:           "en",
		LastLevel:          2,
		InSettlingPhase:    true,
		SettlingStartedAt:  now,
		InterviewStartedAt: now,
	}
	s.sessions[candidateID] = session
	return session
}

func (s *SessionStore) Find(candidateID string) (*CandidateSession, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	session, ok := s.sessions[candidateID]
	return session, ok
}

func (s *CandidateSession) MarkStarted(now time.Time) {
	if s.InterviewStartedAt.IsZero() {
		s.InterviewStartedAt = now
	}
	s.SettlingStartedAt = now
	s.InSettlingPhase = true
}

func (s *CandidateSession) Expired(now time.Time) bool {
	if s.DurationSeconds <= 0 {
		return false
	}
	if s.InterviewStartedAt.IsZero() {
		return false
	}
	return now.Sub(s.InterviewStartedAt) >= time.Duration(s.DurationSeconds)*time.Second
}

func (s *CandidateSession) timeRemaining(now time.Time) time.Duration {
	if s.DurationSeconds <= 0 || s.InterviewStartedAt.IsZero() {
		return time.Hour
	}
	remaining := time.Duration(s.DurationSeconds)*time.Second - now.Sub(s.InterviewStartedAt)
	if remaining < 0 {
		return 0
	}
	return remaining
}

func (s *CandidateSession) AppendUser(content string) {
	s.Transcript = append(s.Transcript, TranscriptTurn{Role: "user", Content: content})
}

func (s *CandidateSession) AppendAssistant(content string) {
	s.Transcript = append(s.Transcript, TranscriptTurn{Role: "assistant", Content: content})
}

func (s *CandidateSession) SystemPrompt() string {
	base := "You are an experienced technical and behavioral interviewer. You ask precise interview questions, evaluate candidate answers fairly, explain scoring with clear reasoning, and tailor difficulty to seniority."
	var extras []string
	if s.Seniority != "" {
		extras = append(extras, "Candidate seniority: "+s.Seniority+".")
	}
	if s.ResumeText != "" {
		extras = append(extras, "Candidate resume excerpt:\n"+truncate(s.ResumeText, 1200))
	}
	if s.JobDescription != "" {
		extras = append(extras, "Job description excerpt:\n"+truncate(s.JobDescription, 1200))
	}
	if s.RollingSummary != "" {
		extras = append(extras, "Earlier in the interview:\n"+s.RollingSummary)
	}
	if len(extras) == 0 {
		return base
	}
	return base + "\n\n" + strings.Join(extras, "\n\n")
}

func (s *CandidateSession) MessagesWithUserPrompt(prompt string) []ChatMessage {
	messages := []ChatMessage{{Role: "system", Content: s.SystemPrompt()}}
	for _, turn := range s.Transcript {
		messages = append(messages, ChatMessage{Role: turn.Role, Content: turn.Content})
	}
	messages = append(messages, ChatMessage{Role: "user", Content: prompt})
	return messages
}

func (s *CandidateSession) TranscriptText() string {
	lines := make([]string, 0, len(s.Transcript))
	for _, turn := range s.Transcript {
		speaker := "Candidate"
		if turn.Role == "assistant" {
			speaker = "Interviewer"
		}
		lines = append(lines, fmt.Sprintf("%s: %s", speaker, turn.Content))
	}
	return strings.Join(lines, "\n")
}

func truncate(text string, limit int) string {
	if len(text) <= limit {
		return text
	}
	return text[:limit]
}
