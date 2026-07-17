package mcp

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// ollamaProvider talks to a local Ollama server (https://ollama.com).
// Configuration:
//
//	OLLAMA_HOST         base URL, default http://localhost:11434
//	OLLAMA_MODEL        text model, default llama3.1
//	OLLAMA_VISION_MODEL vision model, default llava
//	OLLAMA_EMBED_MODEL  embedding model, default nomic-embed-text
type ollamaProvider struct {
	host        string
	model       string
	visionModel string
	embedModel  string
	http        *http.Client
}

func newOllamaProvider() *ollamaProvider {
	return &ollamaProvider{
		host:        envOr("OLLAMA_HOST", "http://localhost:11434"),
		model:       envOr("OLLAMA_MODEL", "llama3.1"),
		visionModel: envOr("OLLAMA_VISION_MODEL", "llava"),
		embedModel:  envOr("OLLAMA_EMBED_MODEL", "nomic-embed-text"),
		http:        &http.Client{},
	}
}

func (p *ollamaProvider) Name() string { return "ollama" }

// Local inference can be slow, especially for long resume/cover-letter
// generations on CPU-bound hardware.
func (p *ollamaProvider) Timeout() time.Duration { return 10 * time.Minute }

type ollamaChatMessage struct {
	Role    string   `json:"role"`
	Content string   `json:"content"`
	Images  []string `json:"images,omitempty"`
}

type ollamaChatRequest struct {
	Model    string                 `json:"model"`
	Messages []ollamaChatMessage    `json:"messages"`
	Stream   bool                   `json:"stream"`
	Format   string                 `json:"format,omitempty"`
	Options  map[string]interface{} `json:"options,omitempty"`
}

type ollamaChatResponse struct {
	Message ollamaChatMessage `json:"message"`
	Error   string            `json:"error"`
}

func (p *ollamaProvider) Generate(ctx context.Context, req genRequest) (string, error) {
	model := p.model
	var messages []ollamaChatMessage
	if req.system != "" {
		messages = append(messages, ollamaChatMessage{Role: "system", Content: req.system})
	}
	userMsg := ollamaChatMessage{Role: "user", Content: req.prompt}
	if req.imagePNG != nil {
		model = p.visionModel
		userMsg.Images = []string{base64.StdEncoding.EncodeToString(req.imagePNG)}
	}
	messages = append(messages, userMsg)

	body := ollamaChatRequest{
		Model:    model,
		Messages: messages,
		Stream:   false,
	}
	if req.json {
		body.Format = "json"
	}
	if req.temperature >= 0 {
		body.Options = map[string]interface{}{"temperature": req.temperature}
	}

	var resp ollamaChatResponse
	if err := p.post(ctx, "/api/chat", body, &resp); err != nil {
		return "", err
	}
	if resp.Error != "" {
		return "", fmt.Errorf("ollama error: %s", resp.Error)
	}
	return resp.Message.Content, nil
}

type ollamaEmbedRequest struct {
	Model string `json:"model"`
	Input string `json:"input"`
}

type ollamaEmbedResponse struct {
	Embeddings [][]float32 `json:"embeddings"`
	Error      string      `json:"error"`
}

func (p *ollamaProvider) Embed(ctx context.Context, text string) ([]float32, error) {
	var resp ollamaEmbedResponse
	if err := p.post(ctx, "/api/embed", ollamaEmbedRequest{Model: p.embedModel, Input: text}, &resp); err != nil {
		return nil, err
	}
	if resp.Error != "" {
		return nil, fmt.Errorf("ollama embed error: %s", resp.Error)
	}
	if len(resp.Embeddings) == 0 {
		return nil, fmt.Errorf("ollama returned no embeddings")
	}
	return resp.Embeddings[0], nil
}

func (p *ollamaProvider) post(ctx context.Context, path string, payload interface{}, out interface{}) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal ollama request: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.host+path, bytes.NewReader(raw))
	if err != nil {
		return fmt.Errorf("failed to build ollama request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	httpResp, err := p.http.Do(httpReq)
	if err != nil {
		return fmt.Errorf("failed to reach ollama at %s (is it running? set OLLAMA_HOST to override): %w", p.host, err)
	}
	defer httpResp.Body.Close()

	respBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return fmt.Errorf("failed to read ollama response: %w", err)
	}
	if httpResp.StatusCode != http.StatusOK {
		return fmt.Errorf("ollama returned HTTP %d: %s", httpResp.StatusCode, string(respBody))
	}
	if err := json.Unmarshal(respBody, out); err != nil {
		return fmt.Errorf("failed to parse ollama response: %w", err)
	}
	return nil
}
