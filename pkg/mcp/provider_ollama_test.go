package mcp

import (
	"testing"
	"time"
)

func TestOllamaTimeoutFromEnv(t *testing.T) {
	cases := []struct {
		name string
		env  string
		want time.Duration
	}{
		{"unset uses default", "", defaultOllamaTimeoutMinutes * time.Minute},
		{"valid override", "60", 60 * time.Minute},
		{"non-numeric falls back to default", "not-a-number", defaultOllamaTimeoutMinutes * time.Minute},
		{"zero falls back to default", "0", defaultOllamaTimeoutMinutes * time.Minute},
		{"negative falls back to default", "-5", defaultOllamaTimeoutMinutes * time.Minute},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("OLLAMA_TIMEOUT_MINUTES", tc.env)
			if got := ollamaTimeoutFromEnv(); got != tc.want {
				t.Errorf("ollamaTimeoutFromEnv() with OLLAMA_TIMEOUT_MINUTES=%q = %v, want %v", tc.env, got, tc.want)
			}
		})
	}
}

func TestNewOllamaProviderUsesConfiguredTimeout(t *testing.T) {
	t.Setenv("OLLAMA_TIMEOUT_MINUTES", "20")
	p := newOllamaProvider()
	if got, want := p.Timeout(), 20*time.Minute; got != want {
		t.Errorf("Timeout() = %v, want %v", got, want)
	}
}
