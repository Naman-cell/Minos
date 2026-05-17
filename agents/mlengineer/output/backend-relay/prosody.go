package main

import (
	"context"
	"strings"
)

type ProsodyDetector interface {
	DetectStyle(ctx context.Context, audio []byte) (string, error)
}

type FallbackProsodyDetector struct{}

func (FallbackProsodyDetector) DetectStyle(context.Context, []byte) (string, error) {
	return "Default", nil
}

func styleOrDefault(style string) string {
	style = strings.TrimSpace(style)
	if style == "" {
		return "Default"
	}
	return style
}
