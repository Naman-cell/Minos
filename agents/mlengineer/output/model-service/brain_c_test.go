package main

import (
	"context"
	"strings"
	"testing"
)

func TestWrapGenerateUsesTrainingShape(t *testing.T) {
	got := WrapGenerate("Redis caching", 2, []string{"ttl", "latency"})
	assertContains(t, got, "Generate a mid-level interview question about: Redis caching.")
	assertContains(t, got, "Relevant tags: ttl, latency.")
}

func TestWrapEvaluateUsesTrainingShape(t *testing.T) {
	got := WrapEvaluate("How did you use Redis?", "I used TTL-based caching.")
	assertContains(t, got, "Evaluate this candidate answer.")
	assertContains(t, got, "Question: How did you use Redis?")
	assertContains(t, got, "Candidate answer: I used TTL-based caching.")
}

func TestWrapRephraseUsesTrainingShape(t *testing.T) {
	got := WrapRephrase("How did you validate cache freshness?")
	assertContains(t, got, "Rephrase the following interview question while preserving its intent:")
	assertContains(t, got, "How did you validate cache freshness?")
}

func TestParseReplyExtractsFillerResponseAndScore(t *testing.T) {
	scoreRaw := "<filler>Got it.</filler><response>Score: 7.5/10. Good TTL detail.</response>"
	parsed := ParseReply(scoreRaw)
	if parsed.Filler != "Got it." {
		t.Fatalf("filler = %q", parsed.Filler)
	}
	assertContains(t, parsed.Response, "Good TTL detail")
	if parsed.Score0To1 == nil || *parsed.Score0To1 != 0.75 {
		t.Fatalf("score = %#v", parsed.Score0To1)
	}
}

func TestMockBrainCRecognizesGenerateAndEvaluateShapes(t *testing.T) {
	brain := NewMockBrainC()
	raw, err := brain.Chat(context.Background(), []ChatMessage{{Role: "user", Content: WrapGenerate("API reliability", 3, nil)}}, ChatOptions{})
	if err != nil {
		t.Fatal(err)
	}
	question := ParseReply(raw).Response
	assertContains(t, question, "API reliability")
	assertContains(t, question, "?")

	raw, err = brain.Chat(context.Background(), []ChatMessage{{Role: "user", Content: WrapEvaluate(question, "I used SLOs and rollback thresholds.")}}, ChatOptions{})
	if err != nil {
		t.Fatal(err)
	}
	eval := ParseReply(raw)
	assertContains(t, eval.Response, "Score:")
	if eval.Score0To1 == nil {
		t.Fatal("expected parsed score")
	}
}

func TestSessionSystemPromptCarriesContext(t *testing.T) {
	session := &CandidateSession{
		CandidateID:    "c1",
		ResumeText:     "Backend engineer with Redis and Kafka.",
		JobDescription: "Needs API reliability.",
		Seniority:      "senior",
	}
	got := session.SystemPrompt()
	assertContains(t, got, "Candidate seniority: senior.")
	assertContains(t, got, "Backend engineer")
	assertContains(t, got, "API reliability")
}

func assertContains(t *testing.T, text, want string) {
	t.Helper()
	if !strings.Contains(strings.ToLower(text), strings.ToLower(want)) {
		t.Fatalf("%q does not contain %q", text, want)
	}
}
