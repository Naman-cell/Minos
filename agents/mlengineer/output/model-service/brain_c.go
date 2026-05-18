package main

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

const defaultBrainCURL = "https://your-brain-c-ngrok-url.ngrok-free.app"

type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type LedgerNext struct {
	EndInterview bool
	Topic        string
	Level        int
}

type BrainC interface {
	Health(ctx context.Context) error
	InterviewTurn(ctx context.Context, req InterviewTurnRequest) (InterviewTurnResponse, error)
	Chat(ctx context.Context, messages []ChatMessage, opts ChatOptions) (string, error)
	LedgerStart(ctx context.Context, candidateID string) error
	LedgerNext(ctx context.Context, candidateID string) (LedgerNext, error)
	LedgerRecord(ctx context.Context, candidateID, topic string, score float64) error
	LedgerEnd(ctx context.Context, candidateID string) error
	Softener(ctx context.Context, category, language string) (string, error)
	Analyze(ctx context.Context, transcript, candidateID string) (map[string]any, error)
	Mode() string
}

type ChatOptions struct {
	MaxTokens   int
	Temperature float64
}

type InterviewTurnRequest struct {
	CandidateID    string  `json:"candidate_id"`
	Transcript     string  `json:"transcript"`
	JobDescription string  `json:"job_description,omitempty"`
	Seniority      string  `json:"seniority,omitempty"`
	CandidateName  string  `json:"candidate_name,omitempty"`
	LanguageHint   string  `json:"language_hint,omitempty"`
	Region         string  `json:"region,omitempty"`
	CandidateStyle string  `json:"candidate_style,omitempty"`
	MaxTokens      int     `json:"max_tokens,omitempty"`
	Temperature    float64 `json:"temperature,omitempty"`
	TopP           float64 `json:"top_p,omitempty"`
	ForcedTopic    string  `json:"forced_topic,omitempty"`
}

type Classification struct {
	Category      string   `json:"category"`
	Language      string   `json:"language"`
	Confidence    float64  `json:"confidence"`
	MatchedPhrase string   `json:"matched_phrase"`
	Notes         []string `json:"notes"`
}

type SafetyPayload struct {
	Triggered         bool      `json:"triggered"`
	Severity          string    `json:"severity,omitempty"`
	Category          string    `json:"category,omitempty"`
	MatchedPhrases    []string  `json:"matched_phrases,omitempty"`
	ResponseText      string    `json:"response_text,omitempty"`
	ResponseLanguage  string    `json:"response_language,omitempty"`
	Hotlines          []Hotline `json:"hotlines,omitempty"`
	RecommendedAction string    `json:"recommended_action,omitempty"`
}

type Hotline struct {
	Name  string `json:"name"`
	Phone string `json:"phone"`
	Hours string `json:"hours"`
	URL   string `json:"url"`
}

type TopicHint struct {
	Topic       string `json:"topic"`
	Level       int    `json:"level"`
	Category    string `json:"category"`
	Description string `json:"description,omitempty"`
	Reason      string `json:"reason,omitempty"`
}

type LedgerSummary struct {
	CandidateID     string         `json:"candidate_id"`
	Ended           bool           `json:"ended"`
	Phase           string         `json:"phase,omitempty"`
	CurrentFocus    string         `json:"current_focus,omitempty"`
	DurationSeconds float64        `json:"duration_seconds,omitempty"`
	Counts          map[string]int `json:"counts,omitempty"`
	Verified        []string       `json:"verified,omitempty"`
	Missing         []string       `json:"missing,omitempty"`
	RedFlags        []string       `json:"red_flags,omitempty"`
}

type InterviewTurnResponse struct {
	ResponseText     string         `json:"response_text"`
	Language         string         `json:"language"`
	Phase            string         `json:"phase"`
	PhaseBefore      string         `json:"phase_before,omitempty"`
	PhaseChanged     bool           `json:"phase_changed"`
	TurnCountInPhase int            `json:"turn_count_in_phase,omitempty"`
	Classification   Classification `json:"classification"`
	CandidateStyle   string         `json:"candidate_style,omitempty"`
	ResponseStyle    string         `json:"response_style,omitempty"`
	SoftenerUsed     *string        `json:"softener_used,omitempty"`
	TopicHint        *TopicHint     `json:"topic_hint,omitempty"`
	Safety           SafetyPayload  `json:"safety"`
	Ledger           LedgerSummary  `json:"ledger"`
	SessionShouldEnd bool           `json:"session_should_end"`
	RepeatPrompt     bool           `json:"repeat_prompt,omitempty"`
	OpeningTemplate  bool           `json:"opening_template,omitempty"`
}

type ToneSummary struct {
	TurnCount             int            `json:"turn_count,omitempty"`
	CandidateDistribution map[string]int `json:"candidate_distribution,omitempty"`
	ResponseDistribution  map[string]int `json:"response_distribution,omitempty"`
	Trajectory            string         `json:"trajectory,omitempty"`
	DominantCandidateMood string         `json:"dominant_candidate_mood,omitempty"`
	StressIndicatorsCount int            `json:"stress_indicators_count,omitempty"`
}

type BrainCConfig struct {
	Mode        string
	BaseURL     string
	APIKey      string
	Timeout     time.Duration
	MaxTokens   int
	Temperature float64
	TopP        float64
}

func NewBrainCFromEnv() (BrainC, error) {
	cfg := BrainCConfig{
		Mode:        strings.ToLower(env("BRAIN_C_MODE", "remote")),
		BaseURL:     strings.TrimRight(env("BRAIN_C_URL", defaultBrainCURL), "/"),
		APIKey:      os.Getenv("BRAIN_C_API_KEY"),
		Timeout:     time.Duration(envInt("BRAIN_C_TIMEOUT_SECONDS", 60)) * time.Second,
		MaxTokens:   envInt("BRAIN_C_MAX_TOKENS", 320),
		Temperature: envFloat("BRAIN_C_TEMPERATURE", 0.6),
		TopP:        envFloat("BRAIN_C_TOP_P", 0.9),
	}
	switch cfg.Mode {
	case "mock":
		return NewMockBrainC(), nil
	case "remote", "":
		if cfg.BaseURL == "" {
			return nil, fmt.Errorf("brain C remote mode requires a configured base URL; set BRAIN_C_MODE=mock only for tests")
		}
		brain := NewRemoteBrainC(cfg)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := brain.Health(ctx); err != nil {
			return nil, fmt.Errorf("brain C healthcheck failed at %s: %w", cfg.BaseURL, err)
		}
		return brain, nil
	default:
		return nil, fmt.Errorf("unsupported BRAIN_C_MODE %q; use remote or mock", cfg.Mode)
	}
}

func envInt(key string, fallback int) int {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func envFloat(key string, fallback float64) float64 {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return fallback
	}
	return parsed
}

func envBool(key string, fallback bool) bool {
	value := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
	if value == "" {
		return fallback
	}
	switch value {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return fallback
	}
}

func localized(language, en, hi, hinglish string) string {
	switch language {
	case "hi":
		return hi
	case "hinglish":
		return hinglish
	default:
		return en
	}
}

func streamWords(ctx context.Context, response string, delay time.Duration) <-chan string {
	out := make(chan string)
	go func() {
		defer close(out)
		for _, token := range strings.Fields(response) {
			select {
			case <-ctx.Done():
				return
			case <-time.After(delay):
				out <- token
			}
		}
	}()
	return out
}
