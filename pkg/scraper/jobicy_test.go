package scraper

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/howlcipher/Career_Agent_Core/pkg/storage"
)

func TestDiscoverWithJobicyPersistsRelevantJobs(t *testing.T) {
	if err := storage.InitDBWithPath(filepath.Join(t.TempDir(), "test.db")); err != nil {
		t.Fatalf("InitDBWithPath: %v", err)
	}
	defer storage.CloseDB()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Accept"); got != "application/json" {
			t.Errorf("Accept = %q, want application/json", got)
		}
		_, _ = w.Write([]byte(`{"success":true,"jobs":[
			{"companyName":"Acme","jobTitle":"Senior Backend Engineer","url":"https://jobs.example.com/backend"},
			{"companyName":"Acme","jobTitle":"Account Executive","url":"https://jobs.example.com/sales"},
			{"companyName":"Acme","jobTitle":"Platform Engineer","url":"not a URL"},
			{"companyName":"","jobTitle":"Platform Engineer","url":"https://jobs.example.com/missing-company"}
		]}`))
	}))
	defer server.Close()

	originalURL := jobicyBaseURL
	jobicyBaseURL = server.URL
	resetJobicyPollForTest()
	defer func() {
		jobicyBaseURL = originalURL
		resetJobicyPollForTest()
	}()

	jobs := make(chan Job, 2)
	(&FunnelEngine{Roles: []string{"Backend Engineer", "Platform Engineer"}}).discoverWithJobicy(jobs)
	close(jobs)

	if got := len(jobs); got != 1 {
		t.Fatalf("queued jobs = %d, want 1", got)
	}
	source, found, err := storage.GetDiscoverySource("https://jobs.example.com/backend")
	if err != nil {
		t.Fatalf("GetDiscoverySource: %v", err)
	}
	if !found || source != "jobicy" {
		t.Fatalf("source = %q, found = %t; want jobicy, true", source, found)
	}

	// A repeat feed must rely on URL deduplication rather than creating a
	// second candidate.
	resetJobicyPollForTest()
	(&FunnelEngine{Roles: []string{"Backend Engineer"}}).discoverWithJobicy(nil)
	discovered, err := storage.GetDiscoveredJobs()
	if err != nil {
		t.Fatalf("GetDiscoveredJobs: %v", err)
	}
	if len(discovered) != 1 {
		t.Fatalf("discovered rows = %d, want 1 after duplicate feed", len(discovered))
	}
}

func TestDiscoverWithJobicyRejectsFailures(t *testing.T) {
	if err := storage.InitDBWithPath(filepath.Join(t.TempDir(), "test.db")); err != nil {
		t.Fatalf("InitDBWithPath: %v", err)
	}
	defer storage.CloseDB()

	for _, response := range []struct {
		name   string
		status int
		body   string
	}{
		{name: "non-200", status: http.StatusServiceUnavailable, body: "unavailable"},
		{name: "malformed JSON", status: http.StatusOK, body: "{"},
		{name: "unsuccessful payload", status: http.StatusOK, body: `{"success":false,"jobs":[{"companyName":"Acme","jobTitle":"Backend Engineer","url":"https://jobs.example.com/backend"}]}`},
	} {
		t.Run(response.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(response.status)
				_, _ = w.Write([]byte(response.body))
			}))
			defer server.Close()

			originalURL := jobicyBaseURL
			jobicyBaseURL = server.URL
			resetJobicyPollForTest()
			defer func() {
				jobicyBaseURL = originalURL
				resetJobicyPollForTest()
			}()

			(&FunnelEngine{Roles: []string{"Backend Engineer"}}).discoverWithJobicy(nil)
			discovered, err := storage.GetDiscoveredJobs()
			if err != nil {
				t.Fatalf("GetDiscoveredJobs: %v", err)
			}
			if len(discovered) != 0 {
				t.Fatalf("discovered rows = %d, want 0", len(discovered))
			}
		})
	}
}

func TestJobicyPollLimit(t *testing.T) {
	originalNow := jobicyNow
	jobicyNow = func() time.Time { return time.Date(2026, time.August, 1, 12, 0, 0, 0, time.UTC) }
	resetJobicyPollForTest()
	defer func() {
		jobicyNow = originalNow
		resetJobicyPollForTest()
	}()

	if !claimJobicyPoll() {
		t.Fatal("first poll should be accepted")
	}
	if claimJobicyPoll() {
		t.Fatal("second poll inside one hour should be rejected")
	}
}
