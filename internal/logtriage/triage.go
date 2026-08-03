// Package logtriage produces bounded, privacy-safe context packets from
// daemon logs. It has no filesystem, database, browser, email, or Git write
// authority; callers supply log lines and decide where any result is sent.
package logtriage

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/howlcipher/Career_Agent_Core/internal/modelbench"
)

const (
	maxEvents     = 100
	maxEventBytes = 500
	maxOutput     = 1200
)

var (
	emailPattern  = regexp.MustCompile(`(?i)\b[A-Z0-9._%+\-]+@[A-Z0-9.\-]+\.[A-Z]{2,}\b`)
	phonePattern  = regexp.MustCompile(`\b(?:\+?\d{1,2}[ .-]?)?(?:\(?\d{3}\)?[ .-]?)\d{3}[ .-]\d{4}\b`)
	queryPattern  = regexp.MustCompile(`\?[^\s]+`)
	secretPattern = regexp.MustCompile(`(?i)\b(api[_-]?key|token|password|secret)\s*[=:]\s*[^\s,;]+`)
)

// Event is a single already-collected log line. Source is a stable component
// label, never a user identity or application field.
type Event struct {
	Source string
	Line   string
}

// Packet is the strictly bounded, sanitized output that may be passed to a
// local model or shown to an operator.
type Packet struct {
	Summary    string         `json:"summary"`
	Confidence float64        `json:"confidence"`
	UsedModel  bool           `json:"used_model"`
	Fallback   bool           `json:"fallback"`
	Groups     []Group        `json:"groups"`
	Events     []SanitizedLog `json:"events"`
}

type Group struct {
	Class string `json:"class"`
	Count int    `json:"count"`
}

type SanitizedLog struct {
	Source string `json:"source"`
	Class  string `json:"class"`
	Line   string `json:"line"`
}

// Options keeps every inference input and output bounded. Leaving Model
// empty selects deterministic triage only.
type Options struct {
	Host    string
	Model   string
	Timeout time.Duration
}

// Redact removes common direct identifiers and credential-shaped values
// before a line is retained or used for inference.
func Redact(line string) string {
	line = emailPattern.ReplaceAllString(line, "[redacted-email]")
	line = phonePattern.ReplaceAllString(line, "[redacted-phone]")
	line = secretPattern.ReplaceAllString(line, "$1=[redacted]")
	line = queryPattern.ReplaceAllString(line, "?[redacted-query]")
	return truncate(strings.TrimSpace(line), maxEventBytes)
}

// Build deterministically redacts, classifies, deduplicates, and summarizes
// events. It is the safe fallback for unavailable or invalid model output.
func Build(events []Event) Packet {
	if len(events) > maxEvents {
		events = events[:maxEvents]
	}
	packet := Packet{Fallback: true, Confidence: 1}
	counts := map[string]int{}
	for _, event := range events {
		line := Redact(event.Line)
		if line == "" {
			continue
		}
		class := classify(line)
		packet.Events = append(packet.Events, SanitizedLog{Source: truncate(event.Source, 80), Class: class, Line: line})
		counts[class]++
	}
	for class, count := range counts {
		packet.Groups = append(packet.Groups, Group{Class: class, Count: count})
	}
	sort.Slice(packet.Groups, func(i, j int) bool {
		if packet.Groups[i].Count == packet.Groups[j].Count {
			return packet.Groups[i].Class < packet.Groups[j].Class
		}
		return packet.Groups[i].Count > packet.Groups[j].Count
	})
	packet.Summary = fallbackSummary(packet.Groups)
	return packet
}

// Analyze uses the local model only after deterministic redaction. Invalid,
// oversized, or unavailable model output always returns the deterministic
// packet instead of an error-shaped or untrusted partial result.
func Analyze(ctx context.Context, events []Event, opts Options) Packet {
	packet := Build(events)
	if strings.TrimSpace(opts.Model) == "" || len(packet.Events) == 0 {
		return packet
	}
	if opts.Timeout <= 0 {
		opts.Timeout = 30 * time.Second
	}
	prompt, err := json.Marshal(packet.Events)
	if err != nil {
		return packet
	}
	callCtx, cancel := context.WithTimeout(ctx, opts.Timeout)
	defer cancel()
	result, err := modelbench.Generate(callCtx, opts.Host, opts.Model, modelbench.GenerateOptions{
		System: "Summarize sanitized daemon events only. Return strict JSON: {\"summary\":\"at most 80 words\",\"confidence\":0..1}. Do not infer identities, credentials, actions, or recommendations.",
		Prompt: string(prompt), JSONFormat: true, Temperature: 0, NumCtx: 2048, KeepAlive: "0",
	})
	if err != nil || len(result.Content) > maxOutput {
		return packet
	}
	var response struct {
		Summary    string  `json:"summary"`
		Confidence float64 `json:"confidence"`
	}
	if json.Unmarshal([]byte(result.Content), &response) != nil || response.Confidence < 0 || response.Confidence > 1 || strings.TrimSpace(response.Summary) == "" {
		return packet
	}
	response.Summary = truncate(strings.TrimSpace(response.Summary), maxOutput)
	packet.Summary = response.Summary
	packet.Confidence = response.Confidence
	packet.UsedModel = true
	packet.Fallback = false
	return packet
}

func classify(line string) string {
	lower := strings.ToLower(line)
	switch {
	case strings.Contains(lower, "deadline") || strings.Contains(lower, "timeout"):
		return "timeout"
	case strings.Contains(lower, "dial ") || strings.Contains(lower, "connection") || strings.Contains(lower, "no route") || strings.Contains(lower, "dns"):
		return "network"
	case strings.Contains(lower, "sqlite") || strings.Contains(lower, "database") || strings.Contains(lower, "sql:"):
		return "database"
	case strings.Contains(lower, "parse") || strings.Contains(lower, "unmarshal") || strings.Contains(lower, "decode"):
		return "parsing"
	case strings.Contains(lower, "unauthorized") || strings.Contains(lower, "forbidden") || strings.Contains(lower, "authentication"):
		return "auth"
	default:
		return "unknown"
	}
}

func fallbackSummary(groups []Group) string {
	if len(groups) == 0 {
		return "No usable events supplied."
	}
	parts := make([]string, 0, len(groups))
	for _, group := range groups {
		parts = append(parts, fmt.Sprintf("%d %s", group.Count, group.Class))
	}
	return "Deterministic triage: " + strings.Join(parts, ", ") + "."
}

func truncate(value string, max int) string {
	if len(value) <= max {
		return value
	}
	return value[:max] + "…"
}
