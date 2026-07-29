package scraper

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/howlcipher/Career_Agent_Core/pkg/storage"
)

func TestDiscoverWithYahooHTML_RetryTransientErrors(t *testing.T) {
	oldSleep := SleepFunc
	SleepFunc = func(time.Duration) {}
	defer func() { SleepFunc = oldSleep }()

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
}
