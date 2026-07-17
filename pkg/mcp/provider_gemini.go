package mcp

import (
	"context"
	"fmt"
	"time"

	"github.com/google/generative-ai-go/genai"
	"google.golang.org/api/option"
)

// geminiProvider talks to Google AI via the generative-ai-go SDK.
// Configuration:
//
//	GEMINI_API_KEY  API key (passed through NewClient)
//	GEMINI_MODEL    model ID, default gemini-flash-latest
type geminiProvider struct {
	apiKey string
	model  string
}

func newGeminiProvider(apiKey string) *geminiProvider {
	return &geminiProvider{
		apiKey: apiKey,
		model:  envOr("GEMINI_MODEL", "gemini-flash-latest"),
	}
}

func (p *geminiProvider) Name() string { return "gemini" }

func (p *geminiProvider) Timeout() time.Duration { return 60 * time.Second }

func (p *geminiProvider) Generate(ctx context.Context, req genRequest) (string, error) {
	if p.apiKey == "" {
		return "", fmt.Errorf("GEMINI_API_KEY is not set")
	}

	client, err := genai.NewClient(ctx, option.WithAPIKey(p.apiKey))
	if err != nil {
		return "", fmt.Errorf("failed to create gemini client: %w", err)
	}
	defer client.Close()

	model := client.GenerativeModel(p.model)
	if req.temperature >= 0 {
		temp := req.temperature
		model.Temperature = &temp
	}
	if req.json {
		model.ResponseMIMEType = "application/json"
	}
	if req.system != "" {
		model.SystemInstruction = &genai.Content{
			Parts: []genai.Part{genai.Text(req.system)},
		}
	}

	parts := []genai.Part{genai.Text(req.prompt)}
	if req.imagePNG != nil {
		parts = append(parts, genai.ImageData("png", req.imagePNG))
	}

	resp, err := model.GenerateContent(ctx, parts...)
	if err != nil {
		return "", fmt.Errorf("failed to generate content from gemini: %w", err)
	}
	if len(resp.Candidates) == 0 || len(resp.Candidates[0].Content.Parts) == 0 {
		return "", fmt.Errorf("empty response from gemini")
	}

	var out string
	for _, part := range resp.Candidates[0].Content.Parts {
		if text, ok := part.(genai.Text); ok {
			out += string(text)
		}
	}
	return out, nil
}

func (p *geminiProvider) Embed(ctx context.Context, text string) ([]float32, error) {
	if p.apiKey == "" {
		return nil, fmt.Errorf("GEMINI_API_KEY is not set")
	}

	client, err := genai.NewClient(ctx, option.WithAPIKey(p.apiKey))
	if err != nil {
		return nil, fmt.Errorf("failed to create gemini client: %w", err)
	}
	defer client.Close()

	em := client.EmbeddingModel("text-embedding-004")
	res, err := em.EmbedContent(ctx, genai.Text(text))
	if err != nil {
		return nil, fmt.Errorf("failed to get embedding: %w", err)
	}
	if res == nil || res.Embedding == nil {
		return nil, fmt.Errorf("empty embedding response")
	}
	return res.Embedding.Values, nil
}
