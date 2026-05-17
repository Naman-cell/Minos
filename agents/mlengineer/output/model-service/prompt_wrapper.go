package main

import (
	"fmt"
	"regexp"
	"strings"
)

var (
	fillerRE   = regexp.MustCompile(`(?is)<filler>(.*?)</filler>`)
	responseRE = regexp.MustCompile(`(?is)<response>(.*?)</response>`)
	scoreRE    = regexp.MustCompile(`(?i)Score:\s*(\d+(?:\.\d+)?)\s*/\s*10`)
)

var levelName = map[int]string{
	1: "junior",
	2: "mid",
	3: "senior",
	4: "senior",
}

type ParsedReply struct {
	Filler    string
	Response  string
	Score0To1 *float64
	Raw       string
}

func WrapGenerate(topic string, level int, tags []string) string {
	tagsClause := ""
	if len(tags) > 0 {
		tagsClause = " Relevant tags: " + strings.Join(tags, ", ") + "."
	}
	label := levelName[level]
	if label == "" {
		label = "mid"
	}
	return fmt.Sprintf("Generate a %s-level interview question about: %s.%s", label, strings.TrimSpace(topic), tagsClause)
}

func WrapEvaluate(question string, candidateAnswer string) string {
	return fmt.Sprintf("Evaluate this candidate answer.\n\nQuestion: %s\n\nCandidate answer: %s",
		strings.TrimSpace(question),
		strings.TrimSpace(candidateAnswer),
	)
}

func WrapRephrase(question string) string {
	return "Rephrase the following interview question while preserving its intent:\n\n" + strings.TrimSpace(question)
}

func WrapAnalysis(transcript string) string {
	return "Below is a transcript from a candidate interview. Return a structured post-interview assessment as a single JSON object with these keys: recommendation (Hire|Maybe|Pass), level_match (Below target|At target|Above target), confidence (Low|Medium|High), skill_evaluation (array of {topic, score, label, notes}), strengths (array of strings), growth_areas (array of strings), red_flags (array of strings), next_steps (array of strings). Output JSON only, no prose, no markdown fences.\n\nTranscript:\n\n" + strings.TrimSpace(transcript)
}

func ParseReply(raw string) ParsedReply {
	reply := ParsedReply{Raw: raw, Response: strings.TrimSpace(raw)}
	if match := fillerRE.FindStringSubmatch(raw); len(match) == 2 {
		reply.Filler = strings.TrimSpace(match[1])
	}
	if match := responseRE.FindStringSubmatch(raw); len(match) == 2 {
		reply.Response = strings.TrimSpace(match[1])
	}
	if match := scoreRE.FindStringSubmatch(raw); len(match) == 2 {
		var score float64
		if _, err := fmt.Sscanf(match[1], "%f", &score); err == nil {
			normalized := score / 10
			reply.Score0To1 = &normalized
		}
	}
	return reply
}

func ExtractQuestion(reply ParsedReply) string {
	return strings.TrimSpace(reply.Response)
}
