package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type RemoteBrainC struct {
	cfg    BrainCConfig
	client *http.Client
}

func NewRemoteBrainC(cfg BrainCConfig) *RemoteBrainC {
	return &RemoteBrainC{
		cfg: cfg,
		client: &http.Client{
			Timeout: cfg.Timeout,
		},
	}
}

func (b *RemoteBrainC) Mode() string {
	return "remote"
}

func (b *RemoteBrainC) Health(ctx context.Context) error {
	req, err := b.request(ctx, http.MethodGet, "/health", nil, "")
	if err != nil {
		return err
	}
	resp, err := b.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("health returned %s", resp.Status)
	}
	return nil
}

func (b *RemoteBrainC) Chat(ctx context.Context, messages []ChatMessage, opts ChatOptions) (string, error) {
	if opts.MaxTokens == 0 {
		opts.MaxTokens = b.cfg.MaxTokens
	}
	if opts.Temperature == 0 {
		opts.Temperature = b.cfg.Temperature
	}
	payload := map[string]any{
		"model":       "customllm",
		"messages":    messages,
		"max_tokens":  opts.MaxTokens,
		"temperature": opts.Temperature,
	}
	var decoded struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := b.doJSON(ctx, http.MethodPost, "/v1/chat/completions", payload, &decoded); err != nil {
		return "", err
	}
	if len(decoded.Choices) == 0 {
		return "", fmt.Errorf("brain C returned no choices")
	}
	return decoded.Choices[0].Message.Content, nil
}

func (b *RemoteBrainC) InterviewTurn(ctx context.Context, req InterviewTurnRequest) (InterviewTurnResponse, error) {
	if req.MaxTokens == 0 {
		req.MaxTokens = b.cfg.MaxTokens
	}
	if req.Temperature == 0 {
		req.Temperature = b.cfg.Temperature
	}
	if req.TopP == 0 {
		req.TopP = b.cfg.TopP
	}
	var decoded InterviewTurnResponse
	if err := b.doJSON(ctx, http.MethodPost, "/interview/turn", req, &decoded); err != nil {
		return InterviewTurnResponse{}, err
	}
	return decoded, nil
}

func (b *RemoteBrainC) LedgerStart(ctx context.Context, candidateID string) error {
	payload := map[string]any{"candidate_id": candidateID}
	var decoded map[string]any
	return b.doJSON(ctx, http.MethodPost, "/ledger", payload, &decoded)
}

func (b *RemoteBrainC) LedgerNext(ctx context.Context, candidateID string) (LedgerNext, error) {
	var decoded struct {
		EndInterview bool `json:"end_interview"`
		Next         struct {
			Topic string `json:"topic"`
			Level int    `json:"level"`
		} `json:"next"`
	}
	if err := b.doJSON(ctx, http.MethodGet, "/ledger/"+candidateID+"/next", nil, &decoded); err != nil {
		return LedgerNext{}, err
	}
	return LedgerNext{EndInterview: decoded.EndInterview, Topic: decoded.Next.Topic, Level: decoded.Next.Level}, nil
}

func (b *RemoteBrainC) LedgerRecord(ctx context.Context, candidateID, topic string, score float64) error {
	payload := map[string]any{"topic": topic, "score": score}
	var decoded map[string]any
	return b.doJSON(ctx, http.MethodPost, "/ledger/"+candidateID+"/record", payload, &decoded)
}

func (b *RemoteBrainC) LedgerEnd(ctx context.Context, candidateID string) error {
	var decoded map[string]any
	return b.doJSON(ctx, http.MethodPost, "/ledger/"+candidateID+"/end", map[string]any{}, &decoded)
}

func (b *RemoteBrainC) Softener(ctx context.Context, category, language string) (string, error) {
	payload := map[string]any{"category": category, "language": language}
	var decoded struct {
		Phrase string `json:"phrase"`
	}
	if err := b.doJSON(ctx, http.MethodPost, "/softeners/pick", payload, &decoded); err != nil {
		return "", err
	}
	return decoded.Phrase, nil
}

func (b *RemoteBrainC) Analyze(ctx context.Context, transcript, candidateID string) (map[string]any, error) {
	payload := map[string]any{"transcript": transcript}
	if strings.TrimSpace(candidateID) != "" {
		payload["candidate_id"] = candidateID
	}
	var decoded map[string]any
	if err := b.doJSON(ctx, http.MethodPost, "/analyze", payload, &decoded); err != nil {
		return nil, err
	}
	return decoded, nil
}

func (b *RemoteBrainC) doJSON(ctx context.Context, method, path string, payload any, out any) error {
	var body io.Reader
	if payload != nil {
		raw, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		body = bytes.NewReader(raw)
	}
	req, err := b.request(ctx, method, path, body, "application/json")
	if err != nil {
		return err
	}
	resp, err := b.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("brain C request %s %s failed: %s %s", method, path, resp.Status, strings.TrimSpace(string(raw)))
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func (b *RemoteBrainC) request(ctx context.Context, method, path string, body io.Reader, contentType string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, b.cfg.BaseURL+path, body)
	if err != nil {
		return nil, err
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if b.cfg.APIKey != "" {
		req.Header.Set("X-API-Key", b.cfg.APIKey)
	}
	return req, nil
}
