package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

const defaultBrainCURL = "https://your-brain-c-ngrok-url.ngrok-free.app"

type STTClient struct {
	mode      string
	brainURL  string
	apiKey    string
	client    *http.Client
	mediaName string
	mediaType string
	prompt    string
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

func NewSTTClient() *STTClient {
	mode := strings.ToLower(env("STT_MODE", "brainc"))
	return &STTClient{
		mode:      mode,
		brainURL:  strings.TrimRight(env("BRAIN_C_URL", defaultBrainCURL), "/"),
		apiKey:    os.Getenv("BRAIN_C_API_KEY"),
		client:    &http.Client{Timeout: time.Duration(envInt("STT_TIMEOUT_SECONDS", 60)) * time.Second},
		mediaName: env("STT_MEDIA_NAME", "candidate.webm"),
		mediaType: env("STT_MEDIA_TYPE", "audio/webm"),
		prompt:    env("NATIVE_AUDIO_PROMPT", "Transcribe the candidate audio exactly. Return only the transcript text, with no filler, no response tag, no explanation, and no markdown."),
	}
}

func (s *STTClient) Transcribe(ctx context.Context, audio []byte, language string) (string, error) {
	text := strings.TrimSpace(string(audio))
	if strings.HasPrefix(text, "text:") {
		return strings.TrimSpace(strings.TrimPrefix(text, "text:")), nil
	}
	if s.mode == "mock" {
		if text != "" && strings.IndexFunc(text, func(r rune) bool { return r < 32 || r > 126 }) == -1 {
			return text, nil
		}
		return "I led a reliability migration and measured latency before and after.", nil
	}
	if s.mode == "native_audio" || s.mode == "native-audio" {
		return s.nativeAudioTextWithBrainC(ctx, audio, language)
	}
	return s.transcribeWithBrainC(ctx, audio, language)
}

func (s *STTClient) transcribeWithBrainC(ctx context.Context, audio []byte, language string) (string, error) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", s.mediaName)
	if err != nil {
		return "", err
	}
	if _, err := part.Write(audio); err != nil {
		return "", err
	}
	if language != "" && language != "auto" {
		if err := writer.WriteField("language", language); err != nil {
			return "", err
		}
	}
	if err := writer.Close(); err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.brainURL+"/transcribe", &body)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	if s.apiKey != "" {
		req.Header.Set("X-API-Key", s.apiKey)
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return "", fmt.Errorf("brain C transcribe failed: %s %s", resp.Status, strings.TrimSpace(string(raw)))
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}
	var decoded struct {
		Text       string `json:"text"`
		Transcript string `json:"transcript"`
		Language   string `json:"language"`
	}
	transcript := ""
	if err := json.Unmarshal(raw, &decoded); err == nil {
		transcript = strings.TrimSpace(decoded.Transcript)
		if transcript == "" {
			transcript = strings.TrimSpace(decoded.Text)
		}
	} else {
		transcript = strings.TrimSpace(string(raw))
	}
	if transcript == "" {
		return "", fmt.Errorf("brain C transcribe returned empty transcript")
	}
	return transcript, nil
}

func (s *STTClient) nativeAudioTextWithBrainC(ctx context.Context, audio []byte, language string) (string, error) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", s.mediaName)
	if err != nil {
		return "", err
	}
	if _, err := part.Write(audio); err != nil {
		return "", err
	}
	if err := writer.WriteField("prompt", s.prompt); err != nil {
		return "", err
	}
	if err := writer.WriteField("max_tokens", "256"); err != nil {
		return "", err
	}
	if err := writer.WriteField("temperature", "0.1"); err != nil {
		return "", err
	}
	if language != "" && language != "auto" {
		// The native audio endpoint does not document a language field, but it safely ignores unknown form fields.
		if err := writer.WriteField("language", language); err != nil {
			return "", err
		}
	}
	if err := writer.Close(); err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.brainURL+"/native-audio-chat", &body)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	if s.apiKey != "" {
		req.Header.Set("X-API-Key", s.apiKey)
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return "", fmt.Errorf("brain C native audio failed: %s %s", resp.Status, strings.TrimSpace(string(raw)))
	}
	var decoded struct {
		Text     string `json:"text"`
		Path     string `json:"path"`
		UsedLoRA bool   `json:"used_lora"`
		Model    string `json:"model"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return "", err
	}
	text := cleanupNativeAudioText(decoded.Text)
	if text == "" {
		return "", fmt.Errorf("brain C native audio returned empty text")
	}
	return text, nil
}

func cleanupNativeAudioText(text string) string {
	text = strings.TrimSpace(text)
	text = strings.TrimPrefix(text, "<response>")
	text = strings.TrimSuffix(text, "</response>")
	text = strings.TrimPrefix(text, "<filler>")
	if idx := strings.Index(text, "</filler>"); idx >= 0 {
		text = text[idx+len("</filler>"):]
	}
	return strings.TrimSpace(text)
}

func decodeAudioPayload(raw string) ([]byte, error) {
	if strings.HasPrefix(raw, "text:") {
		return []byte(raw), nil
	}
	decoded, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return []byte(raw), nil
	}
	return decoded, nil
}
