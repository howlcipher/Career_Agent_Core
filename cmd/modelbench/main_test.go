package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func flockExclusive(f *os.File) error {
	return syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
}

func newMockOllama(t *testing.T, chatContent string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/tags", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"models":[{"name":"test-model","size":123,"details":{"parameter_size":"1B"}}]}`))
	})
	mux.HandleFunc("/api/chat", func(w http.ResponseWriter, r *http.Request) {
		quoted, _ := json.Marshal(chatContent)
		w.Write([]byte(`{"message":{"content":` + string(quoted) + `},"done":true,` +
			`"total_duration":1000000,"load_duration":100,"prompt_eval_count":5,` +
			`"prompt_eval_duration":1000,"eval_count":10,"eval_duration":2000}`))
	})
	mux.HandleFunc("/api/generate", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"done":true}`))
	})
	mux.HandleFunc("/api/ps", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"models":[]}`))
	})
	return httptest.NewServer(mux)
}

// runCapture invokes run() with temp-file stdout/stderr and returns their
// contents plus the exit code, so main's flag/exit-code wiring can be tested
// without touching the process's real stdio.
func runCapture(t *testing.T, args []string) (stdout, stderr string, code int) {
	t.Helper()
	outFile, err := os.CreateTemp(t.TempDir(), "stdout")
	if err != nil {
		t.Fatalf("create stdout temp file: %v", err)
	}
	defer outFile.Close()
	errFile, err := os.CreateTemp(t.TempDir(), "stderr")
	if err != nil {
		t.Fatalf("create stderr temp file: %v", err)
	}
	defer errFile.Close()

	code = run(args, outFile, errFile)

	outFile.Seek(0, 0)
	errFile.Seek(0, 0)
	outBytes, _ := os.ReadFile(outFile.Name())
	errBytes, _ := os.ReadFile(errFile.Name())
	return string(outBytes), string(errBytes), code
}

func TestRun_ListModels(t *testing.T) {
	srv := newMockOllama(t, "{}")
	defer srv.Close()

	stdout, _, code := runCapture(t, []string{"-host", srv.URL, "-list"})
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if !strings.Contains(stdout, "test-model") {
		t.Errorf("stdout should list the installed model, got: %s", stdout)
	}
}

func TestRun_MissingModelsFlag(t *testing.T) {
	_, stderr, code := runCapture(t, []string{})
	if code == 0 {
		t.Fatalf("expected a nonzero exit when -models is missing")
	}
	if !strings.Contains(stderr, "-models is required") {
		t.Errorf("stderr should explain -models is required, got: %s", stderr)
	}
}

func TestRun_UnavailableModelRejectedWithActionableError(t *testing.T) {
	srv := newMockOllama(t, "{}")
	defer srv.Close()
	lockPath := filepath.Join(t.TempDir(), "career_agent.lock")

	_, stderr, code := runCapture(t, []string{
		"-host", srv.URL, "-models", "does-not-exist", "-lock-path", lockPath,
	})
	if code == 0 {
		t.Fatalf("expected a nonzero exit for an unavailable model")
	}
	if !strings.Contains(stderr, "does-not-exist") || !strings.Contains(stderr, "test-model") {
		t.Errorf("stderr should name both the missing and available models, got: %s", stderr)
	}
}

func TestRun_RefusesWhileAgentLockHeld(t *testing.T) {
	srv := newMockOllama(t, `{"category":"network","confidence":0.9}`)
	defer srv.Close()

	lockPath := filepath.Join(t.TempDir(), "career_agent.lock")
	if err := os.WriteFile(lockPath, []byte("1234"), 0644); err != nil {
		t.Fatalf("seed lock file: %v", err)
	}
	// Hold the lock for the duration of the check, the same way the real
	// agent would.
	f, err := os.OpenFile(lockPath, os.O_RDWR, 0666)
	if err != nil {
		t.Fatalf("open lock: %v", err)
	}
	defer f.Close()
	if err := flockExclusive(f); err != nil {
		t.Fatalf("flock: %v", err)
	}

	_, stderr, code := runCapture(t, []string{
		"-host", srv.URL, "-models", "test-model", "-lock-path", lockPath,
	})
	if code == 0 {
		t.Fatalf("expected a nonzero exit while the agent lock is held")
	}
	if !strings.Contains(stderr, "agent appears to be running") {
		t.Errorf("stderr should explain the refusal, got: %s", stderr)
	}
}

func TestRun_ForceOverridesAgentLockCheck(t *testing.T) {
	srv := newMockOllama(t, `{"category":"network","confidence":0.9}`)
	defer srv.Close()

	lockPath := filepath.Join(t.TempDir(), "career_agent.lock")
	if err := os.WriteFile(lockPath, []byte("1234"), 0644); err != nil {
		t.Fatalf("seed lock file: %v", err)
	}
	f, err := os.OpenFile(lockPath, os.O_RDWR, 0666)
	if err != nil {
		t.Fatalf("open lock: %v", err)
	}
	defer f.Close()
	if err := flockExclusive(f); err != nil {
		t.Fatalf("flock: %v", err)
	}

	_, _, code := runCapture(t, []string{
		"-host", srv.URL, "-models", "test-model", "-lock-path", lockPath,
		"-tasks", "classify_error", "-reps", "1", "-force",
	})
	if code != 0 {
		t.Fatalf("expected -force to bypass the lock check and succeed, got exit %d", code)
	}
}

func TestRun_SchemaFailureExitsNonzero(t *testing.T) {
	srv := newMockOllama(t, `not valid json`)
	defer srv.Close()
	lockPath := filepath.Join(t.TempDir(), "career_agent.lock")

	_, stderr, code := runCapture(t, []string{
		"-host", srv.URL, "-models", "test-model", "-lock-path", lockPath,
		"-tasks", "classify_error", "-reps", "1",
	})
	if code == 0 {
		t.Fatalf("expected a nonzero exit when a required task fails schema validation")
	}
	if !strings.Contains(stderr, "failed, timed out, or violated") {
		t.Errorf("stderr should explain the failure, got: %s", stderr)
	}
}

func TestRun_SuccessfulRunWritesJSONReportToFile(t *testing.T) {
	srv := newMockOllama(t, `{"category":"network","confidence":0.9}`)
	defer srv.Close()
	lockPath := filepath.Join(t.TempDir(), "career_agent.lock")
	outPath := filepath.Join(t.TempDir(), "report.json")

	stdout, _, code := runCapture(t, []string{
		"-host", srv.URL, "-models", "test-model", "-lock-path", lockPath,
		"-tasks", "classify_error", "-reps", "1", "-out", outPath,
	})
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stdout: %s", code, stdout)
	}
	raw, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("expected a report file at %s: %v", outPath, err)
	}
	if !strings.Contains(string(raw), "test-model") {
		t.Errorf("report file should mention the model, got: %s", raw)
	}
}

func TestRun_UnknownTaskNameRejected(t *testing.T) {
	srv := newMockOllama(t, "{}")
	defer srv.Close()
	lockPath := filepath.Join(t.TempDir(), "career_agent.lock")

	_, stderr, code := runCapture(t, []string{
		"-host", srv.URL, "-models", "test-model", "-lock-path", lockPath,
		"-tasks", "not_a_real_task",
	})
	if code == 0 {
		t.Fatalf("expected a nonzero exit for an unknown task name")
	}
	if !strings.Contains(stderr, "unknown task") {
		t.Errorf("stderr should say the task is unknown, got: %s", stderr)
	}
}
