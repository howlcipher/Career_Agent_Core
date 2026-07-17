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
