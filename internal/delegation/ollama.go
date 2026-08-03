package delegation

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const DefaultHost = "http://localhost:11434"

type chatRequest struct {
	Model    string        `json:"model"`
	Messages []chatMessage `json:"messages"`
	Stream   bool          `json:"stream"`
	Format   string        `json:"format,omitempty"`
	Options  chatOptions   `json:"options"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatOptions struct {
	Temperature float64 `json:"temperature"`
}

type chatResponse struct {
	Message chatMessage `json:"message"`
	Error   string      `json:"error"`
}

// GenerateProposal makes the only network request supported by this package:
// a bounded request to the configured local Ollama host. It cannot run tools.
func GenerateProposal(ctx context.Context, client *http.Client, host, model, brief string) ([]byte, error) {
	return generate(ctx, client, host, model, proposalSystemPrompt, brief, "json")
}

// GeneratePatch creates a candidate diff after a separate CLI approval check.
// The result is still an artifact only; this package has no apply capability.
func GeneratePatch(ctx context.Context, client *http.Client, host, model, prompt string) ([]byte, error) {
	return generateBounded(ctx, client, host, model, patchSystemPrompt, prompt, "", maxPatchBytes)
}

func generate(ctx context.Context, client *http.Client, host, model, system, prompt, format string) ([]byte, error) {
	return generateBounded(ctx, client, host, model, system, prompt, format, maxProposalBytes)
}

func generateBounded(ctx context.Context, client *http.Client, host, model, system, prompt, format string, maxResponseBytes int) ([]byte, error) {
	if strings.TrimSpace(model) == "" {
		return nil, fmt.Errorf("model is required")
	}
	body, err := json.Marshal(chatRequest{Model: model, Stream: false, Format: format, Options: chatOptions{Temperature: 0}, Messages: []chatMessage{{Role: "system", Content: system}, {Role: "user", Content: prompt}}})
	if err != nil {
		return nil, fmt.Errorf("marshal Ollama request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(host, "/")+"/api/chat", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build Ollama request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if client == nil {
		client = http.DefaultClient
	}
	response, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("reach local Ollama host: %w", err)
	}
	defer response.Body.Close()
	payload, err := io.ReadAll(io.LimitReader(response.Body, int64(maxResponseBytes+1)))
	if err != nil {
		return nil, fmt.Errorf("read Ollama response: %w", err)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("Ollama returned HTTP %d", response.StatusCode)
	}
	if len(payload) > maxResponseBytes {
		return nil, fmt.Errorf("Ollama response exceeds %d bytes", maxResponseBytes)
	}
	var decoded chatResponse
	if err := json.Unmarshal(payload, &decoded); err != nil {
		return nil, fmt.Errorf("decode Ollama response: %w", err)
	}
	if decoded.Error != "" {
		return nil, fmt.Errorf("Ollama error: %s", decoded.Error)
	}
	if strings.TrimSpace(decoded.Message.Content) == "" {
		return nil, fmt.Errorf("Ollama returned an empty response")
	}
	return []byte(decoded.Message.Content), nil
}

const proposalSystemPrompt = `You are a read-only software investigation assistant. You have no authority to edit files, run commands, access credentials, access a browser, access a database, send email, make Git changes, or make architecture, security, or concurrency decisions. Return exactly one JSON object using schema_version "local-delegation/v1" and these fields: finding, root_cause, planned_files, implementation_summary, success_tests, failure_tests, risks, unresolved_questions, ready_to_edit. planned_files must be relative repository paths. Be concise and state uncertainty.`

const patchSystemPrompt = `You create a candidate unified text diff only. You have no authority to apply it, run commands, access credentials, access a browser, access a database, send email, make Git changes, or alter files outside the approved planned paths. Return only a conventional unified diff using matching --- a/path and +++ b/path headers. Do not include binary data.`
