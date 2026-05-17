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
	Chat(ctx context.Context, messages []ChatMessage, opts ChatOptions) (string, error)
	LedgerStart(ctx context.Context, candidateID string) error
	LedgerNext(ctx context.Context, candidateID string) (LedgerNext, error)
	LedgerRecord(ctx context.Context, candidateID, topic string, score float64) error
	LedgerEnd(ctx context.Context, candidateID string) error
	Softener(ctx context.Context, category, language string) (string, error)
	Analyze(ctx context.Context, transcript string) (map[string]any, error)
	Mode() string
}

type ChatOptions struct {
	MaxTokens   int
	Temperature float64
}

type BrainCConfig struct {
	Mode        string
	BaseURL     string
	APIKey      string
	Timeout     time.Duration
	MaxTokens   int
	Temperature float64
}

func NewBrainCFromEnv() (BrainC, error) {
	cfg := BrainCConfig{
		Mode:        strings.ToLower(env("BRAIN_C_MODE", "remote")),
		BaseURL:     strings.TrimRight(env("BRAIN_C_URL", defaultBrainCURL), "/"),
		APIKey:      os.Getenv("BRAIN_C_API_KEY"),
		Timeout:     time.Duration(envInt("BRAIN_C_TIMEOUT_SECONDS", 60)) * time.Second,
		MaxTokens:   envInt("BRAIN_C_MAX_TOKENS", 320),
		Temperature: envFloat("BRAIN_C_TEMPERATURE", 0.6),
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
