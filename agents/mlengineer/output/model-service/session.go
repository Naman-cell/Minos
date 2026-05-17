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
	candidateID := strings.TrimSpace(req.CandidateID)
	if candidateID == "" {
		candidateID = fmt.Sprintf("candidate_%d", time.Now().UnixNano())
	}
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
		CandidateID:        candidateID,
		CandidateName:      strings.TrimSpace(req.CandidateName),
		ResumeText:         req.ResumeText,
		JobDescription:     req.JobDescription,
		Seniority:          seniority,
		DurationSeconds:    duration,
		Language:           normalizeLanguage(req.Language),
		LastLevel:          2,
		InSettlingPhase:    true,
		SettlingStartedAt:  now,
		InterviewStartedAt: now,
	}
	s.mu.Lock()
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

func (s *CandidateSession) Expired(now time.Time) bool {
	if s.DurationSeconds <= 0 {
		return false
	}
	return now.Sub(s.InterviewStartedAt) >= time.Duration(s.DurationSeconds)*time.Second
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
