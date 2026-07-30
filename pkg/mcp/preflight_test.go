package mcp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// tagsServer stands in for Ollama's GET /api/tags with a fixed model list.
func tagsServer(t *testing.T, body string, status int) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/tags" {
			t.Errorf("preflight requested %q, want /api/tags", r.URL.Path)
		}
		if r.Method != http.MethodGet {
			t.Errorf("preflight used %s, want GET", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func tagsBody(names ...string) string {
	quoted := make([]string, 0, len(names))
	for _, n := range names {
		quoted = append(quoted, `{"name":"`+n+`","model":"`+n+`"}`)
	}
	return `{"models":[` + strings.Join(quoted, ",") + `]}`
}

func TestNormalizeModelTag(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"llama3.1", "llama3.1:latest"},
		{"llama3.1:latest", "llama3.1:latest"},
		{"qwen3:30b-instruct", "qwen3:30b-instruct"},
		{"  nomic-embed-text  ", "nomic-embed-text:latest"},
		{"", ""},
	}
	for _, tc := range cases {
		if got := normalizeModelTag(tc.in); got != tc.want {
			t.Errorf("normalizeModelTag(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestOllamaPreflightAcceptsInstalledModels(t *testing.T) {
	cases := []struct {
		name      string
		installed []string
		model     string
		vision    string
		embed     string
	}{
		{
			name:      "exact tagged matches",
			installed: []string{"qwen3:30b-instruct", "qwen2.5vl:7b", "nomic-embed-text:latest"},
			model:     "qwen3:30b-instruct",
			vision:    "qwen2.5vl:7b",
			embed:     "nomic-embed-text",
		},
		{
			// Configured without a tag, installed with the implicit :latest.
			name:      "config omits the tag the server reports",
			installed: []string{"llama3.1:latest", "llava:latest", "nomic-embed-text:latest"},
			model:     "llama3.1",
			vision:    "llava",
			embed:     "nomic-embed-text",
		},
		{
			// The reverse: config is explicit, the server reports the bare name.
			name:      "config states the tag the server omits",
			installed: []string{"llama3.1", "llava", "nomic-embed-text"},
			model:     "llama3.1:latest",
			vision:    "llava:latest",
			embed:     "nomic-embed-text:latest",
		},
		{
			name:      "one model serves two roles",
			installed: []string{"qwen2.5vl:7b", "nomic-embed-text:latest"},
			model:     "qwen2.5vl:7b",
			vision:    "qwen2.5vl:7b",
			embed:     "nomic-embed-text",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := tagsServer(t, tagsBody(tc.installed...), http.StatusOK)
			p := &ollamaProvider{
				host:        srv.URL,
				model:       tc.model,
				visionModel: tc.vision,
				embedModel:  tc.embed,
				http:        srv.Client(),
			}
			if err := p.preflightModels(context.Background()); err != nil {
				t.Fatalf("preflightModels() = %v, want nil", err)
			}
		})
	}
}

func TestOllamaPreflightReportsEveryMissingModel(t *testing.T) {
	srv := tagsServer(t, tagsBody("qwen3:30b-instruct", "nomic-embed-text:latest"), http.StatusOK)
	p := &ollamaProvider{
		host:        srv.URL,
		model:       "llama3.1",
		visionModel: "llava",
		embedModel:  "nomic-embed-text",
		http:        srv.Client(),
	}

	err := p.preflightModels(context.Background())
	if err == nil {
		t.Fatal("preflightModels() = nil, want an error naming both missing models")
	}
	msg := err.Error()
	// Both misses must be reported at once: fixing one and rediscovering the
	// other on the next start is the slow loop #441 was filed to remove.
	for _, want := range []string{
		"llama3.1", "OLLAMA_MODEL",
		"llava", "OLLAMA_VISION_MODEL",
		"ollama pull llama3.1", "ollama pull llava",
		"qwen3:30b-instruct", // what IS installed, so the user can choose instead
		".env",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("error message is missing %q\ngot: %s", want, msg)
		}
	}
	if strings.Contains(msg, "OLLAMA_EMBED_MODEL") {
		t.Errorf("error names OLLAMA_EMBED_MODEL, but nomic-embed-text is installed\ngot: %s", msg)
	}
}

func TestOllamaPreflightNamesBothVariablesForOneMissingModel(t *testing.T) {
	srv := tagsServer(t, tagsBody("nomic-embed-text:latest"), http.StatusOK)
	p := &ollamaProvider{
		host:        srv.URL,
		model:       "qwen3:30b-instruct",
		visionModel: "qwen3:30b-instruct",
		embedModel:  "nomic-embed-text",
		http:        srv.Client(),
	}

	err := p.preflightModels(context.Background())
	if err == nil {
		t.Fatal("preflightModels() = nil, want an error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "OLLAMA_MODEL, OLLAMA_VISION_MODEL") {
		t.Errorf("one missing model set by two variables should name both once\ngot: %s", msg)
	}
	if strings.Count(msg, "ollama pull qwen3:30b-instruct") != 1 {
		t.Errorf("expected exactly one pull suggestion for the shared model\ngot: %s", msg)
	}
	if !strings.Contains(msg, "missing 1 configured model(s)") {
		t.Errorf("expected a count of 1, not 2\ngot: %s", msg)
	}
}

func TestOllamaPreflightChecksFastModelOnlyWhenSet(t *testing.T) {
	installed := tagsBody("qwen3:30b-instruct", "qwen2.5vl:7b", "nomic-embed-text:latest")

	t.Run("unset fast model is not checked", func(t *testing.T) {
		srv := tagsServer(t, installed, http.StatusOK)
		p := &ollamaProvider{
			host:        srv.URL,
			model:       "qwen3:30b-instruct",
			visionModel: "qwen2.5vl:7b",
			embedModel:  "nomic-embed-text",
			http:        srv.Client(),
		}
		if err := p.preflightModels(context.Background()); err != nil {
			t.Fatalf("preflightModels() = %v, want nil when OLLAMA_FAST_MODEL is unset", err)
		}
	})

	t.Run("set but absent fast model fails", func(t *testing.T) {
		srv := tagsServer(t, installed, http.StatusOK)
		p := &ollamaProvider{
			host:        srv.URL,
			model:       "qwen3:30b-instruct",
			fastModel:   "qwen3:8b-instruct",
			visionModel: "qwen2.5vl:7b",
			embedModel:  "nomic-embed-text",
			http:        srv.Client(),
		}
		err := p.preflightModels(context.Background())
		if err == nil {
			t.Fatal("preflightModels() = nil, want an error for the absent fast model")
		}
		if !strings.Contains(err.Error(), "OLLAMA_FAST_MODEL") {
			t.Errorf("error should name OLLAMA_FAST_MODEL\ngot: %s", err)
		}
	})
}

func TestOllamaPreflightDistinguishesUnreachableFromMissing(t *testing.T) {
	// A closed port: the server is not there at all.
	srv := tagsServer(t, tagsBody(), http.StatusOK)
	client := srv.Client()
	url := srv.URL
	srv.Close()

	p := &ollamaProvider{
		host:        url,
		model:       "llama3.1",
		visionModel: "llava",
		embedModel:  "nomic-embed-text",
		http:        client,
	}
	err := p.preflightModels(context.Background())
	if err == nil {
		t.Fatal("preflightModels() = nil, want an unreachable-server error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "could not reach ollama") {
		t.Errorf("unreachable server should say so\ngot: %s", msg)
	}
	if !strings.Contains(msg, "install_ollama.sh") || !strings.Contains(msg, "SKIP_MODEL_PREFLIGHT") {
		t.Errorf("unreachable error should name the installer and the escape hatch\ngot: %s", msg)
	}
	// The fix differs, so the messages must not be interchangeable.
	if strings.Contains(msg, "ollama pull") {
		t.Errorf("unreachable server is not a missing-model problem\ngot: %s", msg)
	}
}

func TestOllamaPreflightEmptyLibraryIsNotUnreachable(t *testing.T) {
	srv := tagsServer(t, `{"models":[]}`, http.StatusOK)
	p := &ollamaProvider{
		host:        srv.URL,
		model:       "llama3.1",
		visionModel: "llava",
		embedModel:  "nomic-embed-text",
		http:        srv.Client(),
	}
	err := p.preflightModels(context.Background())
	if err == nil {
		t.Fatal("preflightModels() = nil, want every model reported missing")
	}
	msg := err.Error()
	if !strings.Contains(msg, "missing 3 configured model(s)") {
		t.Errorf("an empty library means all three are missing\ngot: %s", msg)
	}
	if !strings.Contains(msg, "Installed there: none") {
		t.Errorf("expected the installed list to read 'none'\ngot: %s", msg)
	}
	if strings.Contains(msg, "could not reach") {
		t.Errorf("a reachable empty server must not be reported as unreachable\ngot: %s", msg)
	}
}

func TestOllamaPreflightHandlesBadResponses(t *testing.T) {
	cases := []struct {
		name    string
		body    string
		status  int
		wantMsg string
	}{
		{"non-2xx", "upstream exploded", http.StatusInternalServerError, "returned HTTP 500"},
		{"malformed json", "{not json", http.StatusOK, "failed to parse"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := tagsServer(t, tc.body, tc.status)
			p := &ollamaProvider{
				host:        srv.URL,
				model:       "llama3.1",
				visionModel: "llava",
				embedModel:  "nomic-embed-text",
				http:        srv.Client(),
			}
			err := p.preflightModels(context.Background())
			if err == nil {
				t.Fatalf("preflightModels() = nil, want an error for %s", tc.name)
			}
			if !strings.Contains(err.Error(), tc.wantMsg) {
				t.Errorf("error should contain %q\ngot: %s", tc.wantMsg, err)
			}
		})
	}
}

func TestClaudePreflightChecksOnlyTheEmbeddingModel(t *testing.T) {
	// A Claude user's text and vision calls go to Anthropic, so their local
	// OLLAMA_MODEL / OLLAMA_VISION_MODEL values are irrelevant and must not be
	// able to fail startup. The embedding model is not optional: Anthropic has
	// no embeddings API.
	srv := tagsServer(t, tagsBody("nomic-embed-text:latest"), http.StatusOK)
	embed := &ollamaProvider{
		host:        srv.URL,
		model:       "a-model-nobody-installed",
		visionModel: "another-absent-model",
		embedModel:  "nomic-embed-text",
		http:        srv.Client(),
	}
	p := &claudeProvider{embed: embed}
	if err := p.preflightModels(context.Background()); err != nil {
		t.Fatalf("claude preflight = %v, want nil when only the embed model matters", err)
	}

	embed.embedModel = "some-absent-embedder"
	err := p.preflightModels(context.Background())
	if err == nil {
		t.Fatal("claude preflight = nil, want an error for a missing embedding model")
	}
	if !strings.Contains(err.Error(), "OLLAMA_EMBED_MODEL") {
		t.Errorf("error should name OLLAMA_EMBED_MODEL\ngot: %s", err)
	}
}

func TestClientPreflightModelsIsANoOpForGemini(t *testing.T) {
	t.Setenv("LLM_PROVIDER", "gemini")
	t.Setenv("GEMINI_API_KEY", "test-key")
	c := NewClient("test-key")
	if err := c.PreflightModels(context.Background()); err != nil {
		t.Fatalf("PreflightModels() = %v, want nil for a provider with no local models", err)
	}
}

func TestClientPreflightModelsRoutesToTheOllamaProvider(t *testing.T) {
	srv := tagsServer(t, tagsBody("qwen3:30b-instruct"), http.StatusOK)
	t.Setenv("LLM_PROVIDER", "ollama")
	t.Setenv("OLLAMA_HOST", srv.URL)
	t.Setenv("OLLAMA_MODEL", "qwen3:30b-instruct")
	t.Setenv("OLLAMA_VISION_MODEL", "qwen3:30b-instruct")
	t.Setenv("OLLAMA_EMBED_MODEL", "an-absent-embedder")
	t.Setenv("OLLAMA_FAST_MODEL", "")

	c := NewClient("")
	err := c.PreflightModels(context.Background())
	if err == nil {
		t.Fatal("PreflightModels() = nil, want the ollama provider's missing-model error")
	}
	if !strings.Contains(err.Error(), "an-absent-embedder") {
		t.Errorf("error should name the absent embedding model\ngot: %s", err)
	}
}
