package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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

// improvements.md #24: OLLAMA_FAST_MODEL routes opted-in calls (ScoreJob) to a
// smaller model while everything else stays on the strong one.
func TestOllamaProvider_FastModelRouting(t *testing.T) {
	tests := []struct {
		name      string
		fastModel string
		req       genRequest
		want      string
	}{
		{"unset falls back to the standard model", "", genRequest{fast: true}, "strong-model"},
		{"set and requested uses the fast model", "small-model", genRequest{fast: true}, "small-model"},
		{"set but not requested stays on the strong model", "small-model", genRequest{}, "strong-model"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotModel string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body ollamaChatRequest
				json.NewDecoder(r.Body).Decode(&body)
				gotModel = body.Model
				json.NewEncoder(w).Encode(map[string]any{
					"message": map[string]string{"content": "85"},
				})
			}))
			defer srv.Close()

			p := &ollamaProvider{
				host:      srv.URL,
				model:     "strong-model",
				fastModel: tt.fastModel,
				timeout:   time.Minute,
				http:      srv.Client(),
			}
			if _, err := p.Generate(context.Background(), tt.req); err != nil {
				t.Fatalf("Generate failed: %v", err)
			}
			if gotModel != tt.want {
				t.Errorf("model = %q, want %q", gotModel, tt.want)
			}
		})
	}
}

// A vision request must keep using the vision model even when it opts into
// fast: the fast slot is for text only.
func TestOllamaProvider_FastNeverOverridesVisionModel(t *testing.T) {
	var gotModel string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body ollamaChatRequest
		json.NewDecoder(r.Body).Decode(&body)
		gotModel = body.Model
		json.NewEncoder(w).Encode(map[string]any{"message": map[string]string{"content": "ok"}})
	}))
	defer srv.Close()

	p := &ollamaProvider{
		host: srv.URL, model: "strong-model", fastModel: "small-model",
		visionModel: "vision-model", timeout: time.Minute, http: srv.Client(),
	}
	if _, err := p.Generate(context.Background(), genRequest{fast: true, imagePNG: []byte{1, 2, 3}}); err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	if gotModel != "vision-model" {
		t.Errorf("model = %q, want vision-model", gotModel)
	}
}
