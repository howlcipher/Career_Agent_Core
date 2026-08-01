package modelbench

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestListModels_ParsesValidResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/tags" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"models":[{"name":"qwen3:4b-instruct","size":2497293803,"details":{"parameter_size":"4.0B","family":"qwen3"}}]}`))
	}))
	defer srv.Close()

	models, err := ListModels(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if len(models) != 1 {
		t.Fatalf("got %d models, want 1", len(models))
	}
	if models[0].Name != "qwen3:4b-instruct" || models[0].Size != 2497293803 || models[0].ParameterSize != "4.0B" {
		t.Errorf("got %+v", models[0])
	}
}

func TestListModels_MalformedResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`not json`))
	}))
	defer srv.Close()

	_, err := ListModels(context.Background(), srv.URL)
	if err == nil {
		t.Fatal("expected an error parsing malformed JSON, got nil")
	}
}

func TestListModels_UnreachableHost(t *testing.T) {
	_, err := ListModels(context.Background(), "http://127.0.0.1:1")
	if err == nil {
		t.Fatal("expected an error reaching an unreachable host, got nil")
	}
	if !strings.Contains(err.Error(), "list models from") {
		t.Errorf("error should be actionable and name the host, got: %v", err)
	}
}

func TestGenerate_ParsesTimingFields(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req chatRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req.Model != "qwen3:4b-instruct" {
			t.Errorf("got model %q", req.Model)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"message": {"role":"assistant","content":"{\"category\":\"network\",\"confidence\":0.9}"},
			"done": true,
			"total_duration": 1000000000,
			"load_duration": 200000000,
			"prompt_eval_count": 10,
			"prompt_eval_duration": 100000000,
			"eval_count": 20,
			"eval_duration": 400000000
		}`))
	}))
	defer srv.Close()

	result, err := Generate(context.Background(), srv.URL, "qwen3:4b-instruct", GenerateOptions{
		System: "sys", Prompt: "prompt", JSONFormat: true, Temperature: 0,
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if result.PromptEvalCount != 10 || result.EvalCount != 20 {
		t.Errorf("got %+v", result)
	}
	if result.PromptTokensPerSec() != 100 { // 10 tokens / 0.1s
		t.Errorf("PromptTokensPerSec = %v, want 100", result.PromptTokensPerSec())
	}
	if result.GenTokensPerSec() != 50 { // 20 tokens / 0.4s
		t.Errorf("GenTokensPerSec = %v, want 50", result.GenTokensPerSec())
	}
	if !strings.Contains(result.Content, "network") {
		t.Errorf("Content = %q", result.Content)
	}
}

func TestGenerate_ZeroTokenOrMissingDuration(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"message":{"content":""},"done":true}`))
	}))
	defer srv.Close()

	result, err := Generate(context.Background(), srv.URL, "m", GenerateOptions{Prompt: "p"})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if result.PromptTokensPerSec() != 0 || result.GenTokensPerSec() != 0 {
		t.Errorf("expected zero throughput on missing counters, got %+v", result)
	}
}

func TestGenerate_Timeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.Write([]byte(`{"message":{"content":"late"},"done":true}`))
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	_, err := Generate(ctx, srv.URL, "m", GenerateOptions{Prompt: "p"})
	if err == nil {
		t.Fatal("expected a timeout error, got nil")
	}
	if !strings.Contains(err.Error(), "timed out") {
		t.Errorf("error should say it timed out, got: %v", err)
	}
}

func TestGenerate_OllamaErrorField(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"error":"model requires more system memory"}`))
	}))
	defer srv.Close()

	_, err := Generate(context.Background(), srv.URL, "m", GenerateOptions{Prompt: "p"})
	if err == nil || !strings.Contains(err.Error(), "model requires more system memory") {
		t.Fatalf("expected the ollama error to surface, got: %v", err)
	}
}

func TestGenerate_HTTPErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("internal error"))
	}))
	defer srv.Close()

	_, err := Generate(context.Background(), srv.URL, "m", GenerateOptions{Prompt: "p"})
	if err == nil || !strings.Contains(err.Error(), "500") {
		t.Fatalf("expected an error naming the HTTP status, got: %v", err)
	}
}

func TestListRunning_ParsesResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/ps" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		w.Write([]byte(`{"models":[{"name":"qwen3:30b-instruct","size_vram":123}]}`))
	}))
	defer srv.Close()

	running, err := ListRunning(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("ListRunning: %v", err)
	}
	if len(running) != 1 || running[0].Name != "qwen3:30b-instruct" {
		t.Errorf("got %+v", running)
	}
}
