package main

import (
	"context"
	"strings"
)

type MockBrainC struct {
	ledgerCalls int
}

func NewMockBrainC() *MockBrainC {
	return &MockBrainC{}
}

func (b *MockBrainC) Mode() string {
	return "mock"
}

func (b *MockBrainC) Health(ctx context.Context) error {
	return nil
}

func (b *MockBrainC) Chat(ctx context.Context, messages []ChatMessage, opts ChatOptions) (string, error) {
	prompt := ""
	if len(messages) > 0 {
		prompt = messages[len(messages)-1].Content
	}
	lower := strings.ToLower(prompt)
	switch {
	case strings.HasPrefix(lower, "evaluate this candidate answer"):
		return "<filler>Got it.</filler><response>Score: 7/10. The answer is relevant and gives a concrete implementation detail, but it should explain the tradeoff and validation signal.</response>", nil
	case strings.HasPrefix(lower, "rephrase the following interview question"):
		return "<response>Let me ask that more simply: which signal would you check first, and why?</response>", nil
	case strings.HasPrefix(lower, "below is a transcript"):
		return `{"recommendation":"Maybe","level_match":"At target","confidence":"Medium","skill_evaluation":[],"strengths":["Concrete examples"],"growth_areas":["Explain tradeoffs"],"red_flags":[],"next_steps":["Probe system design depth"]}`, nil
	default:
		topic := "system design"
		if idx := strings.Index(lower, "about:"); idx >= 0 {
			topic = strings.Trim(strings.TrimSuffix(prompt[idx+len("about:"):], "."), " ")
		}
		return "<filler>Alright.</filler><response>On " + topic + ", can you walk me through one concrete tradeoff you made and the metric that proved it worked?</response>", nil
	}
}

func (b *MockBrainC) LedgerStart(ctx context.Context, candidateID string) error {
	b.ledgerCalls = 0
	return nil
}

func (b *MockBrainC) LedgerNext(ctx context.Context, candidateID string) (LedgerNext, error) {
	topics := []string{"Redis caching", "API reliability", "deployment rollback"}
	topic := topics[b.ledgerCalls%len(topics)]
	b.ledgerCalls++
	return LedgerNext{Topic: topic, Level: 2}, nil
}

func (b *MockBrainC) LedgerRecord(ctx context.Context, candidateID, topic string, score float64) error {
	return nil
}

func (b *MockBrainC) LedgerEnd(ctx context.Context, candidateID string) error {
	return nil
}

func (b *MockBrainC) Softener(ctx context.Context, category, language string) (string, error) {
	switch category {
	case "praise":
		return localized(language, "Nice, that is a strong signal.", "Achha, yeh strong signal hai.", "Nice, yeh strong signal hai."), nil
	case "gentle_correction":
		return localized(language, "Let us tighten that answer a bit.", "Is answer ko thoda tight karte hain.", "Is answer ko thoda tight karte hain."), nil
	case "topic_transition":
		return localized(language, "Let us move to the next area.", "Ab next area par chalte hain.", "Ab next area par chalte hain."), nil
	case "wrap_up":
		return localized(language, "Thanks, we can wrap up here.", "Thanks, yahin wrap up karte hain.", "Thanks, yahin wrap up karte hain."), nil
	default:
		return localized(language, "That makes sense.", "Haan, samajh raha hoon.", "Got it, yeh makes sense."), nil
	}
}

func (b *MockBrainC) Analyze(ctx context.Context, transcript string) (map[string]any, error) {
	return map[string]any{
		"analysis": map[string]any{
			"recommendation": "Maybe",
			"confidence":     "Medium",
		},
	}, nil
}
