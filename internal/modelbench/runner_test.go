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

// newMockOllama serves a minimal /api/tags, /api/chat, /api/generate (used
// for Unload) and /api/ps, enough for Run's full loop to execute without a
// real Ollama server.
func newMockOllama(t *testing.T, chatContent string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/tags", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"models":[{"name":"test-model","size":123,"details":{"parameter_size":"1B"}}]}`))
	})
	mux.HandleFunc("/api/chat", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"message":{"content":` + quoteJSON(chatContent) + `},"done":true,` +
			`"total_duration":1000000,"load_duration":100,"prompt_eval_count":5,` +
			`"prompt_eval_duration":1000,"eval_count":10,"eval_duration":2000}`))
	})
	mux.HandleFunc("/api/generate", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"done":true}`))
	})
	mux.HandleFunc("/api/ps", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"models":[{"name":"test-model","size_vram":1}]}`))
	})
	return httptest.NewServer(mux)
}

func quoteJSON(s string) string {
	raw, _ := json.Marshal(s)
	return string(raw)
}

func TestCheckModelsAvailable_RejectsUnavailableModel(t *testing.T) {
	srv := newMockOllama(t, "{}")
	defer srv.Close()

	err := CheckModelsAvailable(context.Background(), srv.URL, []string{"not-installed-model"})
	if err == nil {
		t.Fatal("expected an error for an unavailable model")
	}
	if !strings.Contains(err.Error(), "not-installed-model") || !strings.Contains(err.Error(), "test-model") {
		t.Errorf("error should name both the missing and the available models, got: %v", err)
	}
	if !strings.Contains(err.Error(), "ollama pull") {
		t.Errorf("error should suggest how to fix it, got: %v", err)
	}
}

func TestCheckModelsAvailable_AcceptsInstalledModel(t *testing.T) {
	srv := newMockOllama(t, "{}")
	defer srv.Close()

	if err := CheckModelsAvailable(context.Background(), srv.URL, []string{"test-model"}); err != nil {
		t.Fatalf("expected no error for an installed model, got: %v", err)
	}
}

func TestRun_LabelsFirstCallColdSubsequentWarm(t *testing.T) {
	srv := newMockOllama(t, `{"category":"network","confidence":0.9}`)
	defer srv.Close()

	opts := RunOptions{
		Host:        srv.URL,
		Tasks:       []Task{classifyErrorTask()},
		Repetitions: 3,
		Timeout:     5 * time.Second,
		Temperature: 0,
	}
	report := Run(context.Background(), opts, []string{"test-model"})

	if len(report.Models) != 1 || len(report.Models[0].Results) != 3 {
		t.Fatalf("got %+v", report.Models)
	}
	results := report.Models[0].Results
	if !results[0].ColdStart {
		t.Errorf("first result should be labeled ColdStart, got %+v", results[0])
	}
	if results[1].ColdStart || results[2].ColdStart {
		t.Errorf("subsequent results should not be labeled ColdStart, got %+v / %+v", results[1], results[2])
	}
}

func TestRun_SchemaValidResultsPass(t *testing.T) {
	srv := newMockOllama(t, `{"category":"network","confidence":0.9}`)
	defer srv.Close()

	opts := RunOptions{
		Host:        srv.URL,
		Tasks:       []Task{classifyErrorTask()},
		Repetitions: 1,
		Timeout:     5 * time.Second,
	}
	report := Run(context.Background(), opts, []string{"test-model"})
	if !report.AllPassed() {
		t.Fatalf("expected all results to pass, got %+v", report.Models[0].Results)
	}
}

func TestRun_SchemaInvalidResultsFail(t *testing.T) {
	srv := newMockOllama(t, `not valid json`)
	defer srv.Close()

	opts := RunOptions{
		Host:        srv.URL,
		Tasks:       []Task{classifyErrorTask()},
		Repetitions: 1,
		Timeout:     5 * time.Second,
	}
	report := Run(context.Background(), opts, []string{"test-model"})
	if report.AllPassed() {
		t.Fatalf("expected schema-invalid output to fail the run, got %+v", report.Models[0].Results)
	}
	if report.Models[0].Results[0].SchemaValid {
		t.Errorf("expected SchemaValid=false")
	}
}

func TestRun_RecordsResidencyAndSize(t *testing.T) {
	srv := newMockOllama(t, `{"category":"network","confidence":0.9}`)
	defer srv.Close()

	opts := RunOptions{
		Host:        srv.URL,
		Tasks:       []Task{classifyErrorTask()},
		Repetitions: 1,
		Timeout:     5 * time.Second,
	}
	report := Run(context.Background(), opts, []string{"test-model"})
	mr := report.Models[0]
	if mr.SizeBytes != 123 {
		t.Errorf("SizeBytes = %d, want 123", mr.SizeBytes)
	}
	if !mr.ResidentAfter {
		t.Errorf("expected ResidentAfter=true from the mock /api/ps response")
	}
}
