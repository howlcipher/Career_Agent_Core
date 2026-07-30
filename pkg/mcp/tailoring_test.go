package mcp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// The tailoring path is the project's headline feature and, until bug #439, it
// could not run at all: it POSTed to a hardcoded microservice URL with a
// hardcoded provider and a hardcoded model that no setup step in this repo
// installs. These tests pin the three properties that made that possible --
// configuration is threaded rather than literal, the offload is optional, and a
// missing service degrades instead of failing the job.

// fakeOllama serves Ollama's /api/chat, recording every request body.
type fakeOllama struct {
	srv      *httptest.Server
	mu       sync.Mutex
	requests []ollamaChatRequest
	reply    func(req ollamaChatRequest) map[string]any
}

func newFakeOllama(t *testing.T, reply func(req ollamaChatRequest) map[string]any) *fakeOllama {
	t.Helper()
	f := &fakeOllama{reply: reply}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/chat" {
			t.Errorf("unexpected ollama path %q", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		var body ollamaChatRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("failed to decode ollama request: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		f.mu.Lock()
		f.requests = append(f.requests, body)
		f.mu.Unlock()
		json.NewEncoder(w).Encode(f.reply(body))
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeOllama) snapshot() []ollamaChatRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]ollamaChatRequest, len(f.requests))
	copy(out, f.requests)
	return out
}

// echoByUserPrompt answers each call with text derived from its own prompt, so
// a test can tell which document a returned string belongs to.
func echoByUserPrompt(req ollamaChatRequest) map[string]any {
	last := req.Messages[len(req.Messages)-1].Content
	switch {
	case strings.Contains(last, "Missing keywords:"):
		return map[string]any{"message": map[string]string{"content": "Kubernetes, Terraform"}}
	case strings.Contains(last, "markdown resume"):
		return map[string]any{"message": map[string]string{"content": "  RESUME BODY  "}}
	case strings.Contains(last, "cover letter"):
		return map[string]any{"message": map[string]string{"content": "COVER BODY"}}
	default:
		return map[string]any{"message": map[string]string{"content": "PREP BODY"}}
	}
}

func testJobInput() (map[string]string, map[string]interface{}, string) {
	return map[string]string{
			"title": "Senior Go Engineer",
			"desc":  "Build automation in Go. Kubernetes and Terraform preferred.",
		},
		map[string]interface{}{
			"cover_letter_tone":   "direct and technical",
			"target_compensation": 165000,
		},
		"9 years of IT and software experience, Python and Go automation."
}

// fakeNLPService serves the offload contract: /health plus /generate.
type fakeNLPService struct {
	srv          *httptest.Server
	mu           sync.Mutex
	batches      []nlpGenerateRequest
	healthStatus int
	generate     func(req nlpGenerateRequest) (int, any)
}

func newFakeNLPService(t *testing.T, healthStatus int, generate func(req nlpGenerateRequest) (int, any)) *fakeNLPService {
	t.Helper()
	f := &fakeNLPService{healthStatus: healthStatus, generate: generate}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health":
			w.WriteHeader(f.healthStatus)
			json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
		case "/generate":
			var body nlpGenerateRequest
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Errorf("failed to decode offload request: %v", err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			f.mu.Lock()
			f.batches = append(f.batches, body)
			f.mu.Unlock()
			status, payload := f.generate(body)
			w.WriteHeader(status)
			json.NewEncoder(w).Encode(payload)
		default:
			t.Errorf("unexpected offload path %q", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeNLPService) snapshot() []nlpGenerateRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]nlpGenerateRequest, len(f.batches))
	copy(out, f.batches)
	return out
}

// echoOffload answers a batch the way a working service would.
func echoOffload(req nlpGenerateRequest) (int, any) {
	results := map[string]string{}
	for _, call := range req.Calls {
		switch call.Key {
		case tailoringKeyGap:
			results[call.Key] = "Kubernetes, Terraform"
		case tailoringKeyResume:
			results[call.Key] = "RESUME BODY"
		case tailoringKeyCoverLetter:
			results[call.Key] = "COVER BODY"
		default:
			results[call.Key] = "PREP BODY"
		}
	}
	return http.StatusOK, nlpGenerateResponse{Results: results, Errors: map[string]string{}}
}

// useFakeOllamaProvider points a Client at a fake Ollama server with a named
// model, the way a configured host would be set up.
func useFakeOllamaProvider(t *testing.T, f *fakeOllama, model string) *Client {
	t.Helper()
	t.Setenv("LLM_PROVIDER", "ollama")
	t.Setenv("OLLAMA_HOST", f.srv.URL)
	t.Setenv("OLLAMA_MODEL", model)
	return NewClient("")
}

// With no NLP_SERVICE_URL, nothing external is required: generation happens
// in-process against the configured provider. This is the default, and it is
// the only path that works for Claude and Gemini.
func TestProcessJobApplicationGeneratesInProcessByDefault(t *testing.T) {
	t.Setenv("NLP_SERVICE_URL", "")
	fake := newFakeOllama(t, echoByUserPrompt)
	client := useFakeOllamaProvider(t, fake, "qwen3:30b-instruct")

	scraped, constraints, doc := testJobInput()
	resume, cover, prep, err := client.ProcessJobApplication(scraped, constraints, doc)
	if err != nil {
		t.Fatalf("ProcessJobApplication failed: %v", err)
	}

	if resume != "RESUME BODY" {
		t.Errorf("resume = %q, want the trimmed resume body", resume)
	}
	if cover != "COVER BODY" {
		t.Errorf("cover letter = %q", cover)
	}
	if prep != "PREP BODY" {
		t.Errorf("interview prep = %q", prep)
	}

	reqs := fake.snapshot()
	if len(reqs) != 4 {
		t.Fatalf("got %d ollama calls, want 4 (gap, resume, cover, prep)", len(reqs))
	}
	for _, req := range reqs {
		if req.Model != "qwen3:30b-instruct" {
			t.Errorf("model = %q, want the configured OLLAMA_MODEL", req.Model)
		}
		// num_ctx is what stops Ollama from silently truncating a long
		// posting to its own small default window.
		if req.Options == nil || req.Options["num_ctx"] == nil {
			t.Errorf("call is missing num_ctx: %#v", req.Options)
		}
		if req.KeepAlive != "30m" {
			t.Errorf("keep_alive = %q, want 30m", req.KeepAlive)
		}
	}
}

// The prompts are the ones the pre-microservice implementation used. They were
// retyped in Python for #427 and four instructions were lost in the process;
// they now live in Go only, and these are the exact strings that went missing.
func TestProcessJobApplicationPromptFidelity(t *testing.T) {
	t.Setenv("NLP_SERVICE_URL", "")
	fake := newFakeOllama(t, echoByUserPrompt)
	client := useFakeOllamaProvider(t, fake, "test-model")

	scraped, constraints, doc := testJobInput()
	if _, _, _, err := client.ProcessJobApplication(scraped, constraints, doc); err != nil {
		t.Fatalf("ProcessJobApplication failed: %v", err)
	}

	var systems, prompts []string
	for _, req := range fake.snapshot() {
		for _, msg := range req.Messages {
			if msg.Role == "system" {
				systems = append(systems, msg.Content)
			} else {
				prompts = append(prompts, msg.Content)
			}
		}
	}
	allSystems := strings.Join(systems, "\n")
	allPrompts := strings.Join(prompts, "\n")

	for _, want := range []string{"anomaly detection", "CCNA foundation", "Do not hallucinate metrics", "Executive Summary"} {
		if !strings.Contains(allSystems, want) {
			t.Errorf("system prompts lost %q", want)
		}
	}
	for _, want := range []string{"without extra commentary", "talking points based on my profile"} {
		if !strings.Contains(allPrompts, want) {
			t.Errorf("user prompts lost %q", want)
		}
	}
	// Tone and compensation directives must reach the cover letter.
	if !strings.Contains(allPrompts, "direct and technical") {
		t.Error("the configured cover letter tone never reached a prompt")
	}
	if !strings.Contains(allPrompts, "$165000") {
		t.Error("target compensation never reached a prompt")
	}
}

// Gap analysis runs first and its output is injected into the three document
// prompts, which is the whole reason it is a separate sequential call.
func TestProcessJobApplicationInjectsGapAnalysis(t *testing.T) {
	t.Setenv("NLP_SERVICE_URL", "")
	fake := newFakeOllama(t, echoByUserPrompt)
	client := useFakeOllamaProvider(t, fake, "test-model")

	scraped, constraints, doc := testJobInput()
	if _, _, _, err := client.ProcessJobApplication(scraped, constraints, doc); err != nil {
		t.Fatalf("ProcessJobApplication failed: %v", err)
	}

	var injected int
	for _, req := range fake.snapshot() {
		last := req.Messages[len(req.Messages)-1].Content
		if strings.Contains(last, "Kubernetes, Terraform") && strings.Contains(last, "de-emphasize their necessity") {
			injected++
		}
	}
	if injected != 3 {
		t.Errorf("gap keywords reached %d document prompts, want 3", injected)
	}
}

// Gap analysis is an enrichment, so losing it must cost the job nothing. The
// microservice made it fatal: it raised before the other three calls started.
func TestProcessJobApplicationGapFailureIsNonFatal(t *testing.T) {
	t.Setenv("NLP_SERVICE_URL", "")
	fake := newFakeOllama(t, func(req ollamaChatRequest) map[string]any {
		last := req.Messages[len(req.Messages)-1].Content
		if strings.Contains(last, "Missing keywords:") {
			return map[string]any{"error": "model overloaded"}
		}
		return echoByUserPrompt(req)
	})
	client := useFakeOllamaProvider(t, fake, "test-model")

	scraped, constraints, doc := testJobInput()
	resume, cover, prep, err := client.ProcessJobApplication(scraped, constraints, doc)
	if err != nil {
		t.Fatalf("a failed gap analysis must not fail the job: %v", err)
	}
	if resume == "" || cover == "" || prep == "" {
		t.Error("all three documents should still have been generated")
	}
}

// A failed document is a failed job, and the error has to say which document
// and why -- the microservice path reported a bare HTTP status code.
func TestProcessJobApplicationReportsWhichDocumentFailed(t *testing.T) {
	t.Setenv("NLP_SERVICE_URL", "")
	fake := newFakeOllama(t, func(req ollamaChatRequest) map[string]any {
		last := req.Messages[len(req.Messages)-1].Content
		if strings.Contains(last, "cover letter") {
			return map[string]any{"error": `model "llama3" not found`}
		}
		return echoByUserPrompt(req)
	})
	client := useFakeOllamaProvider(t, fake, "test-model")

	scraped, constraints, doc := testJobInput()
	_, _, _, err := client.ProcessJobApplication(scraped, constraints, doc)
	if err == nil {
		t.Fatal("expected an error when the cover letter cannot be generated")
	}
	if !strings.Contains(err.Error(), "cover letter") {
		t.Errorf("error should name the failed document, got: %v", err)
	}
	if !strings.Contains(err.Error(), "llama3") {
		t.Errorf("error should carry the underlying cause, got: %v", err)
	}
}

// Ollama can answer HTTP 200 with no content. That used to be saved as a
// finished document, so the agent applied with an empty resume.
func TestProcessJobApplicationRejectsEmptyDocuments(t *testing.T) {
	t.Setenv("NLP_SERVICE_URL", "")
	fake := newFakeOllama(t, func(req ollamaChatRequest) map[string]any {
		last := req.Messages[len(req.Messages)-1].Content
		if strings.Contains(last, "markdown resume") {
			return map[string]any{"message": map[string]string{"content": "   "}}
		}
		return echoByUserPrompt(req)
	})
	client := useFakeOllamaProvider(t, fake, "test-model")

	scraped, constraints, doc := testJobInput()
	_, _, _, err := client.ProcessJobApplication(scraped, constraints, doc)
	if err == nil {
		t.Fatal("an empty resume must not be returned as a successful document")
	}
	if !strings.Contains(err.Error(), "resume") || !strings.Contains(err.Error(), "empty") {
		t.Errorf("error should name the empty document, got: %v", err)
	}
}

// The offload carries the caller's real configuration. This is the bug: the
// old payload said "ollama"/"llama3" no matter what the user had configured,
// and llama3 is not installed by anything in this repo.
func TestProcessJobApplicationOffloadCarriesRealConfiguration(t *testing.T) {
	svc := newFakeNLPService(t, http.StatusOK, echoOffload)
	fake := newFakeOllama(t, echoByUserPrompt)
	client := useFakeOllamaProvider(t, fake, "qwen3:30b-instruct")
	t.Setenv("NLP_SERVICE_URL", svc.srv.URL+"/")

	scraped, constraints, doc := testJobInput()
	resume, cover, prep, err := client.ProcessJobApplication(scraped, constraints, doc)
	if err != nil {
		t.Fatalf("ProcessJobApplication failed: %v", err)
	}
	if resume != "RESUME BODY" || cover != "COVER BODY" || prep != "PREP BODY" {
		t.Errorf("unexpected documents: %q / %q / %q", resume, cover, prep)
	}

	if calls := fake.snapshot(); len(calls) != 0 {
		t.Errorf("the offload was configured and healthy, so nothing should have been generated in-process; got %d calls", len(calls))
	}

	batches := svc.snapshot()
	if len(batches) != 2 {
		t.Fatalf("got %d offload batches, want 2 (gap, then the three documents)", len(batches))
	}
	for i, batch := range batches {
		if batch.Model != "qwen3:30b-instruct" {
			t.Errorf("batch %d model = %q, want the configured OLLAMA_MODEL", i, batch.Model)
		}
		if batch.Host != fake.srv.URL {
			t.Errorf("batch %d host = %q, want the configured OLLAMA_HOST %q", i, batch.Host, fake.srv.URL)
		}
		if batch.NumCtx <= 0 {
			t.Errorf("batch %d num_ctx = %d, want a prompt-sized window", i, batch.NumCtx)
		}
		if batch.KeepAlive != "30m" {
			t.Errorf("batch %d keep_alive = %q", i, batch.KeepAlive)
		}
		// The provider's own timeout is what the service must honour; a
		// hardcoded 5-minute deadline is what made this path unable to finish
		// a real generation on CPU-only hardware.
		if batch.TimeoutSeconds < 3600 {
			t.Errorf("batch %d timeout_seconds = %d, want the provider's timeout (>= 1h by default)", i, batch.TimeoutSeconds)
		}
	}
	if len(batches[0].Calls) != 1 || batches[0].Calls[0].Key != tailoringKeyGap {
		t.Errorf("first batch should be gap analysis alone, got %#v", batches[0].Calls)
	}
	if len(batches[1].Calls) != 3 {
		t.Errorf("second batch should hold all three documents, got %d", len(batches[1].Calls))
	}
	// The service owns no prompts, so every call must arrive with its text.
	for _, call := range batches[1].Calls {
		if call.System == "" || call.Prompt == "" {
			t.Errorf("call %q was sent without prompt text", call.Key)
		}
	}
}

// An unhealthy or absent service degrades to in-process generation. The point
// of the preflight check is that "service not started" is diagnosable and
// survivable rather than a failed job.
func TestProcessJobApplicationFallsBackWhenServiceIsUnhealthy(t *testing.T) {
	svc := newFakeNLPService(t, http.StatusServiceUnavailable, func(nlpGenerateRequest) (int, any) {
		t.Error("/generate must not be called after a failed health check")
		return http.StatusInternalServerError, nil
	})
	fake := newFakeOllama(t, echoByUserPrompt)
	client := useFakeOllamaProvider(t, fake, "test-model")
	t.Setenv("NLP_SERVICE_URL", svc.srv.URL)

	scraped, constraints, doc := testJobInput()
	resume, _, _, err := client.ProcessJobApplication(scraped, constraints, doc)
	if err != nil {
		t.Fatalf("an unhealthy offload service must not fail the job: %v", err)
	}
	if resume != "RESUME BODY" {
		t.Errorf("resume = %q, want the in-process result", resume)
	}
	if len(fake.snapshot()) != 4 {
		t.Errorf("expected all 4 calls to run in-process, got %d", len(fake.snapshot()))
	}
}

// Passing the health check is not a promise that the service works. A
// transport-level failure mid-run falls back for the rest of the job.
func TestProcessJobApplicationFallsBackWhenServiceFailsMidRun(t *testing.T) {
	svc := newFakeNLPService(t, http.StatusOK, func(nlpGenerateRequest) (int, any) {
		return http.StatusInternalServerError, map[string]string{"detail": "worker crashed"}
	})
	fake := newFakeOllama(t, echoByUserPrompt)
	client := useFakeOllamaProvider(t, fake, "test-model")
	t.Setenv("NLP_SERVICE_URL", svc.srv.URL)

	scraped, constraints, doc := testJobInput()
	resume, cover, prep, err := client.ProcessJobApplication(scraped, constraints, doc)
	if err != nil {
		t.Fatalf("a broken offload service must not fail the job: %v", err)
	}
	if resume == "" || cover == "" || prep == "" {
		t.Error("all three documents should have been generated in-process")
	}
	// One failed batch is enough to stop trying: the second batch must not
	// have been sent to the service again.
	if got := len(svc.snapshot()); got != 1 {
		t.Errorf("offload was attempted %d times, want 1 before falling back", got)
	}
}

// A per-call error from a working service is a real generation failure, not a
// transport problem, so it propagates with its detail instead of being retried
// in-process -- the same model would fail the same way.
func TestProcessJobApplicationPropagatesOffloadCallErrors(t *testing.T) {
	svc := newFakeNLPService(t, http.StatusOK, func(req nlpGenerateRequest) (int, any) {
		if req.Calls[0].Key == tailoringKeyGap {
			return echoOffload(req)
		}
		return http.StatusOK, nlpGenerateResponse{
			Results: map[string]string{tailoringKeyCoverLetter: "COVER BODY", tailoringKeyInterviewPrep: "PREP BODY"},
			Errors:  map[string]string{tailoringKeyResume: `ollama returned HTTP 404: model "llama3" not found`},
		}
	})
	fake := newFakeOllama(t, echoByUserPrompt)
	client := useFakeOllamaProvider(t, fake, "test-model")
	t.Setenv("NLP_SERVICE_URL", svc.srv.URL)

	scraped, constraints, doc := testJobInput()
	_, _, _, err := client.ProcessJobApplication(scraped, constraints, doc)
	if err == nil {
		t.Fatal("expected the resume failure to surface")
	}
	if !strings.Contains(err.Error(), "resume") || !strings.Contains(err.Error(), "llama3") {
		t.Errorf("error should name the document and carry the service's detail, got: %v", err)
	}
}

// A key the service acknowledges neither way is a contract violation. Treating
// it as an empty success is how an empty document reaches an employer.
func TestProcessJobApplicationRejectsUnacknowledgedOffloadKeys(t *testing.T) {
	svc := newFakeNLPService(t, http.StatusOK, func(req nlpGenerateRequest) (int, any) {
		if req.Calls[0].Key == tailoringKeyGap {
			return echoOffload(req)
		}
		return http.StatusOK, nlpGenerateResponse{
			Results: map[string]string{tailoringKeyResume: "RESUME BODY", tailoringKeyCoverLetter: "COVER BODY"},
			Errors:  map[string]string{},
		}
	})
	fake := newFakeOllama(t, echoByUserPrompt)
	client := useFakeOllamaProvider(t, fake, "test-model")
	t.Setenv("NLP_SERVICE_URL", svc.srv.URL)

	scraped, constraints, doc := testJobInput()
	_, _, _, err := client.ProcessJobApplication(scraped, constraints, doc)
	if err == nil {
		t.Fatal("expected a missing interview_prep key to be an error")
	}
	if !strings.Contains(err.Error(), "interview prep") {
		t.Errorf("error should name the unacknowledged document, got: %v", err)
	}
}

// nlp_service speaks Ollama's API and nothing else, so a Claude or Gemini user
// must never be silently redirected to a local model -- that redirect is the
// original bug.
func TestResolveOffloadRouteRefusesNonOllamaProviders(t *testing.T) {
	t.Setenv("NLP_SERVICE_URL", "http://127.0.0.1:1")
	t.Setenv("ANTHROPIC_API_KEY", "test-key-not-used")

	for _, providerName := range []string{"claude", "gemini"} {
		t.Run(providerName, func(t *testing.T) {
			t.Setenv("LLM_PROVIDER", providerName)
			client := NewClient("test-key-not-used")
			if route := client.resolveOffloadRoute(); route != nil {
				t.Errorf("provider %s must not be offloaded, got route %+v", providerName, route)
			}
		})
	}
}

func TestResolveOffloadRouteRequiresTheEnvVar(t *testing.T) {
	t.Setenv("NLP_SERVICE_URL", "")
	t.Setenv("LLM_PROVIDER", "ollama")
	if route := NewClient("").resolveOffloadRoute(); route != nil {
		t.Errorf("offload must be opt-in, got route %+v", route)
	}
}

func TestProbeNLPService(t *testing.T) {
	t.Run("healthy", func(t *testing.T) {
		svc := newFakeNLPService(t, http.StatusOK, echoOffload)
		if err := probeNLPService(svc.srv.URL); err != nil {
			t.Errorf("expected a healthy probe, got %v", err)
		}
	})

	t.Run("wrong status", func(t *testing.T) {
		svc := newFakeNLPService(t, http.StatusServiceUnavailable, echoOffload)
		err := probeNLPService(svc.srv.URL)
		if err == nil || !strings.Contains(err.Error(), "503") {
			t.Errorf("expected the status in the error, got %v", err)
		}
	})

	t.Run("not running", func(t *testing.T) {
		err := probeNLPService("http://127.0.0.1:1")
		if err == nil || !strings.Contains(err.Error(), "unreachable") {
			t.Errorf("expected an unreachable error, got %v", err)
		}
	})
}

func TestTailoringNumCtx(t *testing.T) {
	tests := []struct {
		name       string
		totalChars int
		want       int
	}{
		{"short input still gets a usable floor", 100, 8192},
		{"scales with the prompt", 60000, 22000},
		{"capped so a huge posting cannot exhaust memory", 10_000_000, 64000},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tailoringNumCtx(tt.totalChars); got != tt.want {
				t.Errorf("tailoringNumCtx(%d) = %d, want %d", tt.totalChars, got, tt.want)
			}
		})
	}
}

// The payload circuit breaker was called four times on this path before the
// microservice replaced it, and zero times afterwards, so the tailoring path
// had no size ceiling at all.
func TestProcessJobApplicationEnforcesThePayloadCircuitBreaker(t *testing.T) {
	t.Setenv("NLP_SERVICE_URL", "")
	fake := newFakeOllama(t, echoByUserPrompt)
	client := useFakeOllamaProvider(t, fake, "test-model")

	scraped, constraints, _ := testJobInput()
	_, _, _, err := client.ProcessJobApplication(scraped, constraints, strings.Repeat("x", defaultPayloadSafetyLimit+1))
	if err == nil {
		t.Fatal("an oversized prompt must trip the circuit breaker")
	}
	if !strings.Contains(err.Error(), "CIRCUIT BREAKER") {
		t.Errorf("expected the breaker's error, got: %v", err)
	}
}
