package main

import "testing"

func TestBrainBDetectsLanguageAndNeutralAck(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		request  string
		language string
		ack      string
	}{
		{
			name:     "english",
			text:     "I used Redis caching to reduce p95 latency.",
			request:  "auto",
			language: "en",
			ack:      "That makes sense.",
		},
		{
			name:     "hindi devanagari with technical terms",
			text:     "मैंने cache लगाया क्योंकि database queries slow थीं.",
			request:  "auto",
			language: "hinglish",
			ack:      "Got it, yeh makes sense.",
		},
		{
			name:     "hinglish roman",
			text:     "Deployment ke baad rollback plan ready tha, but metrics unstable the.",
			request:  "auto",
			language: "hinglish",
			ack:      "Got it, yeh makes sense.",
		},
		{
			name:     "explicit hindi",
			text:     "I used cache.",
			request:  "hi",
			language: "hi",
			ack:      "Haan, samajh raha hoon.",
		},
	}

	brain := NewBrainB()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := brain.Analyze(tt.text, tt.request)
			if got.Directive != DirectiveNeutral {
				t.Fatalf("directive=%q want %q", got.Directive, DirectiveNeutral)
			}
			if got.Language != tt.language {
				t.Fatalf("language=%q want %q", got.Language, tt.language)
			}
			if got.Ack != tt.ack {
				t.Fatalf("ack=%q want %q", got.Ack, tt.ack)
			}
		})
	}
}

func TestBrainBDetectsSafetyOnlyAsGuardrail(t *testing.T) {
	got := NewBrainB().Analyze("i will kill myself if you will not pass me.", "auto")
	if got.Directive != DirectiveSafety {
		t.Fatalf("directive=%q want %q", got.Directive, DirectiveSafety)
	}
	if got.Ack != "I am really sorry you are feeling this way. Your safety matters more than this interview." {
		t.Fatalf("unexpected ack: %q", got.Ack)
	}
}

func TestBrainBDetectsRephraseRequestOnlyAsRoutingHint(t *testing.T) {
	got := NewBrainB().Analyze("Can you rephrase the question?", "auto")
	if got.Directive != DirectiveRephrase {
		t.Fatalf("directive=%q want %q", got.Directive, DirectiveRephrase)
	}
	if got.Ack != "No problem, I can rephrase that." {
		t.Fatalf("unexpected ack: %q", got.Ack)
	}
}

func TestBrainBDoesNotInferInterviewStrategyFromWeakAnswer(t *testing.T) {
	got := NewBrainB().Analyze("Nhi ata mujhe", "auto")
	if got.Directive != DirectiveNeutral {
		t.Fatalf("directive=%q want neutral; Brain C should evaluate the answer", got.Directive)
	}
	if got.Language != "hinglish" {
		t.Fatalf("language=%q want hinglish", got.Language)
	}
}
