package main

import "strings"

type InterviewState struct {
	Phase         string `json:"phase"`
	DeepDiveCount int    `json:"deep_dive_count"`
	Verified      bool   `json:"verified"`
}

type StateMachine struct {
	state InterviewState
}

func NewStateMachine() *StateMachine {
	return &StateMachine{state: InterviewState{Phase: "settling"}}
}

func (s *StateMachine) Next(text string, behavior Behavior) InterviewState {
	lower := strings.ToLower(text)
	if behavior.Directive == DirectiveRephrase || containsAny(lower, "sort of", "somehow", "not sure", "maybe") {
		s.state.DeepDiveCount++
		s.state.Verified = false
	} else if containsAny(lower, "measured", "validated", "tradeoff", "latency", "reliability") {
		s.state.Verified = true
	}
	if s.state.DeepDiveCount >= 2 && s.state.Verified {
		s.state.Phase = "verified"
	}
	return s.state
}
