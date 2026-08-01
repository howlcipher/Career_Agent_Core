package scraper

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/howlcipher/Career_Agent_Core/pkg/storage"
)

func TestDiscoverWithYahooHTML_RetryTransientErrors(t *testing.T) {
	oldSleep := SleepFunc
	SleepFunc = func(time.Duration) {}
	defer func() { SleepFunc = oldSleep }()
	yahooBreaker = newSourceCircuitBreaker()

	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "applications.db")
	if err := storage.InitDBWithPath(dbPath); err != nil {
		t.Fatalf("InitDBWithPath failed: %v", err)
	}
	defer os.Remove(dbPath)

	var attempts int32
	mockYahoo := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := atomic.AddInt32(&attempts, 1)
		if count < 3 {
			// First two attempts return 502 Bad Gateway
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		// Third attempt succeeds
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`<html><a href="https://r.search.yahoo.com/_ylt=something/RV=2/RE=1690000000/RO=10/RU=https%3a%2f%2fjobs.lever.co%2fTestCorp%2f123/RK=0/RS=xyz">Link</a></html>`))
	}))
	defer mockYahoo.Close()

	origYahoo := yahooBaseURL
	yahooBaseURL = mockYahoo.URL
	defer func() { yahooBaseURL = origYahoo }()

	engine := NewFunnelEngine([]string{"backend"})
	jobChan := make(chan Job, 10)

	engine.discoverWithYahooHTML(context.Background(), "test query", "backend", jobChan)
	close(jobChan)

	var jobs []Job
	for j := range jobChan {
		jobs = append(jobs, j)
	}

	if atomic.LoadInt32(&attempts) != 3 {
		t.Fatalf("Expected 3 attempts, got %d", attempts)
	}
	if len(jobs) != 1 {
		t.Fatalf("Expected 1 job recovered from transient error, got %d", len(jobs))
	}
}

func TestDiscoverWithYahooHTML_ExhaustsRetries(t *testing.T) {
	oldSleep := SleepFunc
	SleepFunc = func(time.Duration) {}
	defer func() { SleepFunc = oldSleep }()
	yahooBreaker = newSourceCircuitBreaker()

	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "applications.db")
	if err := storage.InitDBWithPath(dbPath); err != nil {
		t.Fatalf("InitDBWithPath failed: %v", err)
	}
	defer os.Remove(dbPath)

	var attempts int32
	mockYahoo := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&attempts, 1)
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer mockYahoo.Close()

	origYahoo := yahooBaseURL
	yahooBaseURL = mockYahoo.URL
	defer func() { yahooBaseURL = origYahoo }()

	engine := NewFunnelEngine([]string{"backend"})
	jobChan := make(chan Job, 10)

	engine.discoverWithYahooHTML(context.Background(), "test query", "backend", jobChan)
	close(jobChan)

	var jobs []Job
	for j := range jobChan {
		jobs = append(jobs, j)
	}

	if atomic.LoadInt32(&attempts) != 3 {
		t.Fatalf("Expected 3 attempts before exhausting, got %d", attempts)
	}
	if len(jobs) != 0 {
		t.Fatalf("Expected 0 jobs due to exhausted retries, got %d", len(jobs))
	}
}

func TestDiscoverWithYahooHTML_NonRetryableResponse(t *testing.T) {
	oldSleep := SleepFunc
	SleepFunc = func(time.Duration) {}
	defer func() { SleepFunc = oldSleep }()
	yahooBreaker = newSourceCircuitBreaker()

	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "applications.db")
	if err := storage.InitDBWithPath(dbPath); err != nil {
		t.Fatalf("InitDBWithPath failed: %v", err)
	}
	defer os.Remove(dbPath)

	var attempts int32
	mockYahoo := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&attempts, 1)
		w.WriteHeader(http.StatusNotFound) // 404 is not retryable
	}))
	defer mockYahoo.Close()

	origYahoo := yahooBaseURL
	yahooBaseURL = mockYahoo.URL
	defer func() { yahooBaseURL = origYahoo }()

	engine := NewFunnelEngine([]string{"backend"})
	jobChan := make(chan Job, 10)

	engine.discoverWithYahooHTML(context.Background(), "test query", "backend", jobChan)
	close(jobChan)

	if atomic.LoadInt32(&attempts) != 1 {
		t.Fatalf("Expected 1 attempt for non-retryable response, got %d", attempts)
	}
}

func TestDiscoverWithYahooHTML_Cancellation(t *testing.T) {
	oldSleep := SleepFunc
	SleepFunc = func(time.Duration) {}
	defer func() { SleepFunc = oldSleep }()
	yahooBreaker = newSourceCircuitBreaker()

	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "applications.db")
	if err := storage.InitDBWithPath(dbPath); err != nil {
		t.Fatalf("InitDBWithPath failed: %v", err)
	}
	defer os.Remove(dbPath)

	var attempts int32
	mockYahoo := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&attempts, 1)
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer mockYahoo.Close()

	origYahoo := yahooBaseURL
	yahooBaseURL = mockYahoo.URL
	defer func() { yahooBaseURL = origYahoo }()

	engine := NewFunnelEngine([]string{"backend"})
	jobChan := make(chan Job, 10)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	engine.discoverWithYahooHTML(ctx, "test query", "backend", jobChan)
	close(jobChan)

	// Since context is cancelled immediately, it should only make at most 1 attempt
	// or no attempts if http client detects cancellation before sending.
	// Actually, http.NewRequestWithContext with a cancelled context will cause client.Do to fail immediately.
	// We want to ensure it doesn't loop 3 times.
	if atomic.LoadInt32(&attempts) > 1 {
		t.Fatalf("Expected at most 1 attempt due to cancellation, got %d", attempts)
	}
	if yahooBreaker.consecutiveFailures != 0 {
		t.Fatalf("a caller-side context cancellation must not count as a source failure, got %d consecutive failures", yahooBreaker.consecutiveFailures)
	}
}

// TestDiscoverWithYahooHTML_TransportErrorTripsBreaker proves a genuine
// transport error (connection refused, not a caller-side context
// cancellation) is actually wired into the breaker -- distinct from
// TestDiscoverWithYahooHTML_Cancellation, which proves the opposite case
// (a cancellation must NOT count).
func TestDiscoverWithYahooHTML_TransportErrorTripsBreaker(t *testing.T) {
	oldSleep := SleepFunc
	SleepFunc = func(time.Duration) {}
	defer func() { SleepFunc = oldSleep }()
	yahooBreaker = newSourceCircuitBreaker()

	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "applications.db")
	if err := storage.InitDBWithPath(dbPath); err != nil {
		t.Fatalf("InitDBWithPath failed: %v", err)
	}
	defer os.Remove(dbPath)

	mockYahoo := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	closedURL := mockYahoo.URL
	mockYahoo.Close() // further connections to closedURL are refused: a real transport error

	origYahoo := yahooBaseURL
	yahooBaseURL = closedURL
	defer func() { yahooBaseURL = origYahoo }()

	engine := NewFunnelEngine([]string{"backend"})
	jobChan := make(chan Job, 10)

	engine.discoverWithYahooHTML(context.Background(), "test query", "backend", jobChan)
	close(jobChan)

	if yahooBreaker.consecutiveFailures != 1 {
		t.Fatalf("a real transport error must count as a source failure, got %d consecutive failures", yahooBreaker.consecutiveFailures)
	}
}

// TestDiscoverWithYahooHTML_BodyReadErrorTripsBreaker proves the body-read
// failure path (util.ReadAll returning an error after a 200 status) is also
// wired into the breaker, by hijacking the connection and closing it before
// the advertised Content-Length is satisfied.
func TestDiscoverWithYahooHTML_BodyReadErrorTripsBreaker(t *testing.T) {
	oldSleep := SleepFunc
	SleepFunc = func(time.Duration) {}
	defer func() { SleepFunc = oldSleep }()
	yahooBreaker = newSourceCircuitBreaker()

	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "applications.db")
	if err := storage.InitDBWithPath(dbPath); err != nil {
		t.Fatalf("InitDBWithPath failed: %v", err)
	}
	defer os.Remove(dbPath)

	mockYahoo := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hj, ok := w.(http.Hijacker)
		if !ok {
			return
		}
		conn, buf, err := hj.Hijack()
		if err != nil {
			return
		}
		defer conn.Close()
		// Advertise more body than is actually sent, then close mid-body so
		// the client's read fails with io.ErrUnexpectedEOF.
		buf.WriteString("HTTP/1.1 200 OK\r\nContent-Length: 100\r\n\r\nshort")
		buf.Flush()
	}))
	defer mockYahoo.Close()

	origYahoo := yahooBaseURL
	yahooBaseURL = mockYahoo.URL
	defer func() { yahooBaseURL = origYahoo }()

	engine := NewFunnelEngine([]string{"backend"})
	jobChan := make(chan Job, 10)

	engine.discoverWithYahooHTML(context.Background(), "test query", "backend", jobChan)
	close(jobChan)

	if yahooBreaker.consecutiveFailures != 1 {
		t.Fatalf("a body-read error must count as a source failure, got %d consecutive failures", yahooBreaker.consecutiveFailures)
	}
}

// TestDiscoverWithYahooHTML_CircuitOpensAfterSustainedFailures is bug #475's
// acceptance criterion: a sustained failure streak, not a single transient
// blip, must eventually stop spending requests on Yahoo at all.
func TestDiscoverWithYahooHTML_CircuitOpensAfterSustainedFailures(t *testing.T) {
	oldSleep := SleepFunc
	SleepFunc = func(time.Duration) {}
	defer func() { SleepFunc = oldSleep }()
	yahooBreaker = newSourceCircuitBreaker()

	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "applications.db")
	if err := storage.InitDBWithPath(dbPath); err != nil {
		t.Fatalf("InitDBWithPath failed: %v", err)
	}
	defer os.Remove(dbPath)

	var requests int32
	mockYahoo := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requests, 1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer mockYahoo.Close()

	origYahoo := yahooBaseURL
	yahooBaseURL = mockYahoo.URL
	defer func() { yahooBaseURL = origYahoo }()

	engine := NewFunnelEngine([]string{"backend"})
	jobChan := make(chan Job, 10)

	// Each exhausted query is one consecutive failure toward the breaker's
	// threshold; drive it past the threshold with distinct queries.
	for i := 0; i < yahooCircuitFailureThreshold; i++ {
		engine.discoverWithYahooHTML(context.Background(), "query", "backend", jobChan)
	}
	if yahooBreaker.allow() {
		t.Fatal("circuit should be open after a sustained failure streak")
	}

	requestsBeforeOpenQuery := atomic.LoadInt32(&requests)
	engine.discoverWithYahooHTML(context.Background(), "query", "backend", jobChan)
	close(jobChan)

	if got := atomic.LoadInt32(&requests); got != requestsBeforeOpenQuery {
		t.Fatalf("a query while the circuit is open must not spend any HTTP requests, got %d more", got-requestsBeforeOpenQuery)
	}
}

func TestDiscoverWithYahooHTML_SetsBrowserHeaders(t *testing.T) {
	oldSleep := SleepFunc
	SleepFunc = func(time.Duration) {}
	defer func() { SleepFunc = oldSleep }()
	yahooBreaker = newSourceCircuitBreaker()

	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "applications.db")
	if err := storage.InitDBWithPath(dbPath); err != nil {
		t.Fatalf("InitDBWithPath failed: %v", err)
	}
	defer os.Remove(dbPath)

	var gotAccept, gotAcceptLanguage string
	mockYahoo := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAccept = r.Header.Get("Accept")
		gotAcceptLanguage = r.Header.Get("Accept-Language")
		w.WriteHeader(http.StatusOK)
	}))
	defer mockYahoo.Close()

	origYahoo := yahooBaseURL
	yahooBaseURL = mockYahoo.URL
	defer func() { yahooBaseURL = origYahoo }()

	engine := NewFunnelEngine([]string{"backend"})
	jobChan := make(chan Job, 10)
	engine.discoverWithYahooHTML(context.Background(), "test query", "backend", jobChan)
	close(jobChan)

	if gotAccept == "" {
		t.Error("expected a non-empty Accept header on the Yahoo fallback request, improvement #477 requires one")
	}
	if gotAcceptLanguage == "" {
		t.Error("expected a non-empty Accept-Language header on the Yahoo fallback request, improvement #477 requires one")
	}
}

func TestDiscoverWithYahooHTML_SharesCookieJarWithinOneEngine(t *testing.T) {
	oldSleep := SleepFunc
	SleepFunc = func(time.Duration) {}
	defer func() { SleepFunc = oldSleep }()
	yahooBreaker = newSourceCircuitBreaker()

	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "applications.db")
	if err := storage.InitDBWithPath(dbPath); err != nil {
		t.Fatalf("InitDBWithPath failed: %v", err)
	}
	defer os.Remove(dbPath)

	var requestCount int32
	var secondRequestCookie string
	mockYahoo := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := atomic.AddInt32(&requestCount, 1)
		if count == 1 {
			http.SetCookie(w, &http.Cookie{Name: "session", Value: "abc123"})
		} else {
			secondRequestCookie = r.Header.Get("Cookie")
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer mockYahoo.Close()

	origYahoo := yahooBaseURL
	yahooBaseURL = mockYahoo.URL
	defer func() { yahooBaseURL = origYahoo }()

	engine := NewFunnelEngine([]string{"backend"})
	jobChan := make(chan Job, 10)

	engine.discoverWithYahooHTML(context.Background(), "first query", "backend", jobChan)
	engine.discoverWithYahooHTML(context.Background(), "second query", "backend", jobChan)
	close(jobChan)

	if secondRequestCookie == "" {
		t.Fatal("expected the second query on the same FunnelEngine to carry the cookie the first query's response set, improvement #477's shared jar is not working")
	}
	if !strings.Contains(secondRequestCookie, "session=abc123") {
		t.Fatalf("expected the second request's Cookie header to contain the session cookie, got %q", secondRequestCookie)
	}

	// A fresh engine instance (a new DiscoverJobs run) must not inherit the
	// previous run's cookies — the jar is scoped per engine, not global.
	freshEngine := NewFunnelEngine([]string{"backend"})
	var thirdRequestCookie string
	requestCount = 0
	mockYahoo2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		thirdRequestCookie = r.Header.Get("Cookie")
		w.WriteHeader(http.StatusOK)
	}))
	defer mockYahoo2.Close()
	yahooBaseURL = mockYahoo2.URL

	freshJobChan := make(chan Job, 10)
	freshEngine.discoverWithYahooHTML(context.Background(), "third query", "backend", freshJobChan)
	close(freshJobChan)

	if thirdRequestCookie != "" {
		t.Fatalf("expected a fresh FunnelEngine to start with no cookies, got Cookie header %q", thirdRequestCookie)
	}
}

// TestDiscoverWithYahooHTML_ConcurrentQueriesShareOneClient mirrors
// DiscoverJobs's real access pattern (eg.SetLimit(5) firing many concurrent
// discoverWithYahooHTML calls against one FunnelEngine) to prove the
// yahooClientOnce lazy-init race described in improvement #477's fix is
// actually race-free — run with `go test -race` to make this test meaningful.
func TestDiscoverWithYahooHTML_ConcurrentQueriesShareOneClient(t *testing.T) {
	oldSleep := SleepFunc
	SleepFunc = func(time.Duration) {}
	defer func() { SleepFunc = oldSleep }()
	yahooBreaker = newSourceCircuitBreaker()

	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "applications.db")
	if err := storage.InitDBWithPath(dbPath); err != nil {
		t.Fatalf("InitDBWithPath failed: %v", err)
	}
	defer os.Remove(dbPath)

	mockYahoo := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer mockYahoo.Close()

	origYahoo := yahooBaseURL
	yahooBaseURL = mockYahoo.URL
	defer func() { yahooBaseURL = origYahoo }()

	engine := NewFunnelEngine([]string{"backend"})
	jobChan := make(chan Job, 10)

	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			engine.discoverWithYahooHTML(context.Background(), fmt.Sprintf("query %d", n), "backend", jobChan)
		}(i)
	}
	wg.Wait()
	close(jobChan)

	if engine.yahooClient == nil {
		t.Fatal("expected discoverWithYahooHTML to have initialized the shared client")
	}
}
