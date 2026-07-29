package mcp

import (
	"context"
	"log"
	"os"
	"strings"
	"time"
)

// genRequest is a provider-agnostic generation request.
type genRequest struct {
	system      string
	prompt      string
	json        bool    // request strict JSON output
	temperature float32 // < 0 means provider default
	imagePNG    []byte  // non-nil switches to a vision request
	// fast opts this call into a smaller/faster model when the provider has
	// one configured, falling back to the standard model otherwise. Set it
	// only for calls whose output is simple enough to survive a weaker model
	// (improvements.md #24).
	fast      bool
	numCtx    int    // optional override for Ollama num_ctx (e.g. 32000 for large inputs)
	keepAlive string // optional keep_alive duration (e.g. "30m")
}

// provider abstracts an LLM backend (Ollama, Claude, Gemini).
type provider interface {
	Name() string
	Generate(ctx context.Context, req genRequest) (string, error)
	Embed(ctx context.Context, text string) ([]float32, error)
	Timeout() time.Duration
}

// newProviderFromEnv selects the backend via LLM_PROVIDER.
// Supported values: "ollama" (default), "claude"/"anthropic", "gemini"/"google".
// geminiAPIKey is the key passed to NewClient (kept for backward compatibility
// with the original Gemini-only client).
func newProviderFromEnv(geminiAPIKey string) provider {
	name := strings.ToLower(strings.TrimSpace(os.Getenv("LLM_PROVIDER")))
	switch name {
	case "gemini", "google":
		return newGeminiProvider(geminiAPIKey)
	case "claude", "anthropic":
		return newClaudeProvider()
	case "", "ollama":
		return newOllamaProvider()
	default:
		log.Printf("[LLM] Unknown LLM_PROVIDER %q, defaulting to ollama", name)
		return newOllamaProvider()
	}
}

func envOr(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

// stripJSONFences removes markdown code fences that some models wrap around
// JSON output despite instructions.
func stripJSONFences(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	return strings.TrimSpace(s)
}
