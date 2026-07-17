package mcp

import (
	"context"
	"encoding/base64"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
)

// claudeProvider talks to the Anthropic API via the official Go SDK.
// Configuration:
//
//	ANTHROPIC_API_KEY  API key (read by the SDK)
//	ANTHROPIC_MODEL    model ID, default claude-opus-4-8
//
// Anthropic has no embeddings endpoint, so embeddings fall back to the local
// Ollama embedding model (see provider_ollama.go).
type claudeProvider struct {
	client anthropic.Client
	model  anthropic.Model
	embed  *ollamaProvider
}

func newClaudeProvider() *claudeProvider {
	if os.Getenv("ANTHROPIC_API_KEY") == "" {
		log.Println("[LLM] Warning: ANTHROPIC_API_KEY is not set; Claude requests will fail")
	}
	return &claudeProvider{
		client: anthropic.NewClient(), // reads ANTHROPIC_API_KEY
		model:  anthropic.Model(envOr("ANTHROPIC_MODEL", string(anthropic.ModelClaudeOpus4_8))),
		embed:  newOllamaProvider(),
	}
}

func (p *claudeProvider) Name() string { return "claude" }

func (p *claudeProvider) Timeout() time.Duration { return 5 * time.Minute }

func (p *claudeProvider) Generate(ctx context.Context, req genRequest) (string, error) {
	system := req.system
	if req.json {
		if system != "" {
			system += "\n\n"
		}
		system += "Respond with only a valid JSON object. Do not wrap it in markdown code fences and do not add any commentary."
	}

	var blocks []anthropic.ContentBlockParamUnion
	if req.imagePNG != nil {
		b64 := base64.StdEncoding.EncodeToString(req.imagePNG)
		blocks = append(blocks, anthropic.NewImageBlockBase64("image/png", b64))
	}
	blocks = append(blocks, anthropic.NewTextBlock(req.prompt))

	params := anthropic.MessageNewParams{
		Model:     p.model,
		MaxTokens: 16000,
		Messages:  []anthropic.MessageParam{anthropic.NewUserMessage(blocks...)},
	}
	if system != "" {
		params.System = []anthropic.TextBlockParam{{Text: system}}
	}
	// Note: temperature is intentionally not sent — current Claude models
	// (Opus 4.7+) reject sampling parameters.

	resp, err := p.client.Messages.New(ctx, params)
	if err != nil {
		return "", fmt.Errorf("claude request failed: %w", err)
	}
	if resp.StopReason == anthropic.StopReasonRefusal {
		return "", fmt.Errorf("claude declined the request (stop_reason: refusal)")
	}

	var out string
	for _, block := range resp.Content {
		if text, ok := block.AsAny().(anthropic.TextBlock); ok {
			out += text.Text
		}
	}
	if out == "" {
		return "", fmt.Errorf("empty response from claude")
	}
	return out, nil
}

func (p *claudeProvider) Embed(ctx context.Context, text string) ([]float32, error) {
	// Anthropic does not offer an embeddings API; use local Ollama embeddings
	// so semantic search keeps working with the Claude provider.
	vec, err := p.embed.Embed(ctx, text)
	if err != nil {
		return nil, fmt.Errorf("claude provider uses ollama for embeddings, which failed (install ollama and `ollama pull %s`, or set LLM_PROVIDER=gemini): %w", p.embed.embedModel, err)
	}
	return vec, nil
}
