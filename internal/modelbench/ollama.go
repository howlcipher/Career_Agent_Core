package modelbench

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// DefaultHost matches pkg/mcp's OLLAMA_HOST default so a benchmark run talks
// to the same server the production agent would, absent an override.
const DefaultHost = "http://localhost:11434"

// OllamaModel is the subset of /api/tags' per-model fields this package uses.
type OllamaModel struct {
	Name          string `json:"name"`
	Size          int64  `json:"size"`
	ParameterSize string `json:"-"`
	Family        string `json:"-"`
}

type tagsResponse struct {
	Models []struct {
		Name    string `json:"name"`
		Size    int64  `json:"size"`
		Details struct {
			ParameterSize string `json:"parameter_size"`
			Family        string `json:"family"`
		} `json:"details"`
	} `json:"models"`
}

// ListModels calls Ollama's /api/tags and returns the currently installed
// models. It performs no mutation and never pulls a model.
func ListModels(ctx context.Context, host string) ([]OllamaModel, error) {
	var parsed tagsResponse
	if err := getJSON(ctx, host, "/api/tags", &parsed); err != nil {
		return nil, fmt.Errorf("list models from %s: %w", host, err)
	}
	models := make([]OllamaModel, 0, len(parsed.Models))
	for _, m := range parsed.Models {
		models = append(models, OllamaModel{
			Name:          m.Name,
			Size:          m.Size,
			ParameterSize: m.Details.ParameterSize,
			Family:        m.Details.Family,
		})
	}
	return models, nil
}

// RunningModel is one entry from /api/ps: a model Ollama currently holds
// resident in memory.
type RunningModel struct {
	Name     string `json:"name"`
	SizeVRAM int64  `json:"size_vram"`
}

type psResponse struct {
	Models []RunningModel `json:"models"`
}

// ListRunning calls Ollama's /api/ps to report which models are currently
// resident, used to confirm cold/warm labeling and to warn against
// benchmarking while something heavyweight is already loaded.
func ListRunning(ctx context.Context, host string) ([]RunningModel, error) {
	var parsed psResponse
	if err := getJSON(ctx, host, "/api/ps", &parsed); err != nil {
		return nil, fmt.Errorf("list running models from %s: %w", host, err)
	}
	return parsed.Models, nil
}

func getJSON(ctx context.Context, host, path string, out interface{}) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, host+path, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("reach ollama (is it running? OLLAMA_HOST=%s): %w", host, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("ollama returned HTTP %d: %s", resp.StatusCode, string(body))
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("parse response: %w (body: %s)", err, truncate(body, 200))
	}
	return nil
}

func truncate(b []byte, n int) string {
	if len(b) <= n {
		return string(b)
	}
	return string(b[:n]) + "..."
}

// chatMessage mirrors pkg/mcp's ollamaChatMessage; duplicated rather than
// imported because pkg/mcp is the production inference path and this package
// must never share mutable state or behavior changes with it (see doc.go).
type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatRequest struct {
	Model     string                 `json:"model"`
	Messages  []chatMessage          `json:"messages"`
	Stream    bool                   `json:"stream"`
	Format    string                 `json:"format,omitempty"`
	Options   map[string]interface{} `json:"options,omitempty"`
	KeepAlive interface{}            `json:"keep_alive,omitempty"`
}

// chatResponse captures every timing/token field Ollama's /api/chat returns.
// pkg/mcp's ollamaChatResponse deliberately ignores these; this is the whole
// reason this package talks to the API directly instead of reusing it.
type chatResponse struct {
	Message struct {
		Content string `json:"content"`
	} `json:"message"`
	Done               bool   `json:"done"`
	DoneReason         string `json:"done_reason"`
	Error              string `json:"error"`
	TotalDurationNS    int64  `json:"total_duration"`
	LoadDurationNS     int64  `json:"load_duration"`
	PromptEvalCount    int    `json:"prompt_eval_count"`
	PromptEvalDuration int64  `json:"prompt_eval_duration"`
	EvalCount          int    `json:"eval_count"`
	EvalDuration       int64  `json:"eval_duration"`
}

// GenerateOptions configures one /api/chat call.
type GenerateOptions struct {
	System      string
	Prompt      string
	JSONFormat  bool
	Temperature float64
	NumCtx      int
	// KeepAlive is passed through verbatim, e.g. "5m" or "0" to unload
	// immediately after the call. Empty means Ollama's own default (5m).
	KeepAlive string
}

// GenerateResult is one call's raw Ollama-reported metrics plus the wall time
// this package itself measured around the HTTP round trip.
type GenerateResult struct {
	Content            string
	WallDuration       time.Duration
	TotalDurationNS    int64
	LoadDurationNS     int64
	PromptEvalCount    int
	PromptEvalDuration int64
	EvalCount          int
	EvalDuration       int64
}

// PromptTokensPerSec returns 0 if the duration is zero or missing rather than
// dividing by zero — Ollama can legitimately report 0 for a cached/instant
// prompt evaluation.
func (r GenerateResult) PromptTokensPerSec() float64 {
	return tokensPerSec(r.PromptEvalCount, r.PromptEvalDuration)
}

// GenTokensPerSec is the generation-phase equivalent of PromptTokensPerSec.
func (r GenerateResult) GenTokensPerSec() float64 {
	return tokensPerSec(r.EvalCount, r.EvalDuration)
}

func tokensPerSec(count int, durationNS int64) float64 {
	if count <= 0 || durationNS <= 0 {
		return 0
	}
	seconds := float64(durationNS) / float64(time.Second)
	return float64(count) / seconds
}

// Generate runs one chat completion against model and returns its content and
// timing. The caller controls the timeout via ctx; a context deadline
// exceeded surfaces as an error whose text names the model, so a benchmark
// report can tell a genuine timeout from a schema failure.
func Generate(ctx context.Context, host, model string, opts GenerateOptions) (GenerateResult, error) {
	var messages []chatMessage
	if opts.System != "" {
		messages = append(messages, chatMessage{Role: "system", Content: opts.System})
	}
	messages = append(messages, chatMessage{Role: "user", Content: opts.Prompt})

	body := chatRequest{
		Model:    model,
		Messages: messages,
		Stream:   false,
	}
	if opts.JSONFormat {
		body.Format = "json"
	}
	if opts.KeepAlive != "" {
		body.KeepAlive = opts.KeepAlive
	}
	options := map[string]interface{}{"temperature": opts.Temperature}
	if opts.NumCtx > 0 {
		options["num_ctx"] = opts.NumCtx
	}
	body.Options = options

	raw, err := json.Marshal(body)
	if err != nil {
		return GenerateResult{}, fmt.Errorf("marshal request for %s: %w", model, err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, host+"/api/chat", bytes.NewReader(raw))
	if err != nil {
		return GenerateResult{}, fmt.Errorf("build request for %s: %w", model, err)
	}
	req.Header.Set("Content-Type", "application/json")

	start := time.Now()
	resp, err := http.DefaultClient.Do(req)
	wall := time.Since(start)
	if err != nil {
		if ctx.Err() != nil {
			return GenerateResult{WallDuration: wall}, fmt.Errorf("model %s timed out after %s: %w", model, wall.Round(time.Millisecond), ctx.Err())
		}
		return GenerateResult{WallDuration: wall}, fmt.Errorf("reach ollama for model %s (OLLAMA_HOST=%s): %w", model, host, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return GenerateResult{WallDuration: wall}, fmt.Errorf("read response for %s: %w", model, err)
	}
	if resp.StatusCode != http.StatusOK {
		return GenerateResult{WallDuration: wall}, fmt.Errorf("ollama returned HTTP %d for model %s: %s", resp.StatusCode, model, truncate(respBody, 300))
	}

	var parsed chatResponse
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return GenerateResult{WallDuration: wall}, fmt.Errorf("parse response for %s: %w", model, err)
	}
	if parsed.Error != "" {
		return GenerateResult{WallDuration: wall}, fmt.Errorf("ollama error for model %s: %s", model, parsed.Error)
	}

	return GenerateResult{
		Content:            parsed.Message.Content,
		WallDuration:       wall,
		TotalDurationNS:    parsed.TotalDurationNS,
		LoadDurationNS:     parsed.LoadDurationNS,
		PromptEvalCount:    parsed.PromptEvalCount,
		PromptEvalDuration: parsed.PromptEvalDuration,
		EvalCount:          parsed.EvalCount,
		EvalDuration:       parsed.EvalDuration,
	}, nil
}

// Unload asks Ollama to drop model from memory immediately (keep_alive: 0)
// so a subsequent call can be measured as a genuine cold start. It is
// deliberately best-effort: a failure here is not fatal to a benchmark run,
// only to that one cold-start measurement, since the run can still proceed
// and note the model may already have been resident.
func Unload(ctx context.Context, host, model string) error {
	body := map[string]interface{}{
		"model":      model,
		"prompt":     "",
		"keep_alive": 0,
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, host+"/api/generate", bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("unload %s: %w", model, err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unload %s: ollama returned HTTP %d", model, resp.StatusCode)
	}
	return nil
}
