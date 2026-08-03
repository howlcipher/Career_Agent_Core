package logtriage

import (
	"context"
	"strings"
	"testing"
)

func TestBuildRedactsAndGroupsBoundedEvents(t *testing.T) {
	packet := Build([]Event{{Source: "agent", Line: "dial tcp jane@example.com?token=abc timeout 555-123-4567"}, {Source: "agent", Line: "dial tcp jane@example.com?token=abc timeout"}})
	if got, want := packet.Groups[0], (Group{Class: "timeout", Count: 2}); got != want {
		t.Fatalf("first group = %#v, want %#v", got, want)
	}
	for _, event := range packet.Events {
		if strings.Contains(event.Line, "jane@example.com") || strings.Contains(event.Line, "abc") || strings.Contains(event.Line, "555-123") {
			t.Fatalf("unredacted sensitive value in %q", event.Line)
		}
	}
}

func TestAnalyzeWithoutModelUsesDeterministicFallback(t *testing.T) {
	packet := Analyze(context.Background(), []Event{{Source: "agent", Line: "database is locked"}}, Options{})
	if !packet.Fallback || packet.UsedModel || packet.Summary != "Deterministic triage: 1 database." {
		t.Fatalf("unexpected fallback packet: %#v", packet)
	}
}
