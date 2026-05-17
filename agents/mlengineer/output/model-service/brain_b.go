package main

import (
	"strings"
	"unicode"
)

type Directive string

const (
	DirectiveNeutral   Directive = "NEUTRAL"
	DirectiveSafety    Directive = "SAFETY"
	DirectiveInterrupt Directive = "INTERRUPT"
	DirectiveRephrase  Directive = "REPHRASE"
)

type Behavior struct {
	Directive Directive
	Ack       string
	Language  string
}

type BrainB struct{}

func NewBrainB() *BrainB {
	return &BrainB{}
}

func (b *BrainB) Analyze(text string, requestedLanguage string) Behavior {
	lower := strings.ToLower(text)
	language := detectLanguage(text, requestedLanguage)
	directive := DirectiveNeutral

	switch {
	case containsSelfHarmSignal(lower):
		directive = DirectiveSafety
	case len(strings.Fields(lower)) > 70:
		directive = DirectiveInterrupt
	case asksForRephrase(lower):
		directive = DirectiveRephrase
	}

	return Behavior{Directive: directive, Ack: ackFor(directive, language), Language: language}
}

func detectLanguage(text string, requested string) string {
	switch strings.ToLower(strings.TrimSpace(requested)) {
	case "en", "english":
		return "en"
	case "hi", "hindi":
		return "hi"
	case "hinglish":
		return "hinglish"
	}

	hasDevanagari := false
	for _, r := range text {
		if unicode.In(r, unicode.Devanagari) {
			hasDevanagari = true
			break
		}
	}
	if hasDevanagari {
		if hasASCIIWord(text) {
			return "hinglish"
		}
		return "hi"
	}

	lower := strings.ToLower(text)
	hindiRomanHits := 0
	for _, marker := range []string{
		"maine", "mujhe", "mera", "meri", "tha", "thi", "hain", "hai", "kyunki",
		"kaise", "kya", "haan", "nahi", "nhi", "samajh", "lagaya", "ho rahi", "ke baad",
		"ye ", "yeh", "socha", "soccha", "abhi", "tak", "tkk", "ata", "aata",
	} {
		if strings.Contains(lower, marker) {
			hindiRomanHits++
		}
	}
	if hindiRomanHits >= 2 || (hindiRomanHits >= 1 && containsAny(lower, "api", "latency", "cache", "database", "deployment", "rollback", "metrics")) {
		return "hinglish"
	}
	return "en"
}

func hasASCIIWord(text string) bool {
	for _, word := range strings.Fields(text) {
		for _, r := range word {
			if r >= 'A' && r <= 'z' {
				return true
			}
		}
	}
	return false
}

func ackFor(directive Directive, language string) string {
	acks := map[string]map[Directive]string{
		"en": {
			DirectiveSafety:    "I am really sorry you are feeling this way. Your safety matters more than this interview.",
			DirectiveInterrupt: "Let me pause you there so we can keep the interview moving.",
			DirectiveRephrase:  "No problem, I can rephrase that.",
			DirectiveNeutral:   "That makes sense.",
		},
		"hi": {
			DirectiveSafety:    "Mujhe afsos hai ki aap aisa feel kar rahe hain. Aapki safety interview se zyada important hai.",
			DirectiveInterrupt: "Haan, main yahan thoda pause karke interview ko track par rakhna chahta hoon.",
			DirectiveRephrase:  "Koi baat nahi, main isko rephrase kar sakta hoon.",
			DirectiveNeutral:   "Haan, samajh raha hoon.",
		},
		"hinglish": {
			DirectiveSafety:    "I am really sorry aap aisa feel kar rahe hain. Your safety is more important than this interview.",
			DirectiveInterrupt: "Let me pause you there, interview ko track par rakhte hain.",
			DirectiveRephrase:  "Koi baat nahi, main isko rephrase kar deta hoon.",
			DirectiveNeutral:   "Got it, yeh makes sense.",
		},
	}
	if byDirective, ok := acks[language]; ok {
		return byDirective[directive]
	}
	return acks["en"][directive]
}

func asksForRephrase(text string) bool {
	return containsAny(text,
		"can you repeat", "could you repeat", "please repeat", "repeat the question",
		"can you rephrase", "could you rephrase", "rephrase the question",
		"i did not understand the question", "i didn't understand the question",
		"question samajh nahi", "question samajh nhi", "dobara", "phir se",
	)
}

func containsSelfHarmSignal(text string) bool {
	return containsAny(text,
		"kill myself", "suicide", "end my life", "hurt myself", "harm myself",
		"i want to die", "i don't want to live", "dont want to live",
	)
}

func containsAny(text string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(text, needle) {
			return true
		}
	}
	return false
}
