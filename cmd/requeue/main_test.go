package main

import (
	"testing"

	"github.com/howlcipher/Career_Agent_Core/pkg/storage"
)

func TestCountForStatus(t *testing.T) {
	stat := storage.SourceOutcomeStat{Total: 100, Applied: 39, Captcha: 5, Failed: 10, Manual: 2}

	tests := []struct {
		status  string
		want    int
		wantErr bool
	}{
		{status: "BLOCKED_CAPTCHA", want: 5},
		{status: "FAILED_SUBMIT", want: 10},
		{status: "APPLIED", want: 39},
		{status: "NOT_A_REAL_STATUS", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.status, func(t *testing.T) {
			got, err := countForStatus(stat, tt.status)
			if tt.wantErr {
				if err == nil {
					t.Errorf("countForStatus(%q) expected an error, got nil", tt.status)
				}
				return
			}
			if err != nil {
				t.Fatalf("countForStatus(%q) unexpected error: %v", tt.status, err)
			}
			if got != tt.want {
				t.Errorf("countForStatus(%q) = %d, want %d", tt.status, got, tt.want)
			}
		})
	}
}

func TestResolvePatterns_NamedSources(t *testing.T) {
	patterns, err := resolvePatterns("lever,greenhouse", "")
	if err != nil {
		t.Fatalf("resolvePatterns failed: %v", err)
	}
	if len(patterns) != 2 || patterns["lever"] != "%lever.co%" || patterns["greenhouse"] != "%greenhouse%" {
		t.Errorf("unexpected patterns: %+v", patterns)
	}
}

func TestResolvePatterns_UnknownSource(t *testing.T) {
	_, err := resolvePatterns("not-a-real-source", "")
	if err == nil {
		t.Error("expected an error for an unknown source name")
	}
}

func TestResolvePatterns_RawPattern(t *testing.T) {
	patterns, err := resolvePatterns("", "%example.com%")
	if err != nil {
		t.Fatalf("resolvePatterns failed: %v", err)
	}
	if len(patterns) != 1 || patterns["custom"] != "%example.com%" {
		t.Errorf("unexpected patterns: %+v", patterns)
	}
}

func TestResolvePatterns_NeitherGiven(t *testing.T) {
	patterns, err := resolvePatterns("", "")
	if err != nil {
		t.Fatalf("resolvePatterns failed: %v", err)
	}
	if len(patterns) != len(sourcePatterns) {
		t.Errorf("expected all %d known sources when neither -source nor -pattern is given, got %d", len(sourcePatterns), len(patterns))
	}
}
