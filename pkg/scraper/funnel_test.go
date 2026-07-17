package scraper

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/howlcipher/Career_Agent_Core/pkg/storage"
)

func TestDiscoverJobs(t *testing.T) {
	oldSleep := SleepFunc
	SleepFunc = func(time.Duration) {}
	defer func() { SleepFunc = oldSleep }()

	// Initialize test DB
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test.db")
	err := storage.InitDBWithPath(dbPath)
	if err != nil {
		t.Fatalf("failed to init db: %v", err)
	}
	defer storage.CloseDB()

	// Mock SerpAPI
	serpTs := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := SerpApiResponse{
			OrganicResults: []struct {
				Title string `json:"title"`
				Link  string `json:"link"`
			}{
				{Title: "Backend Engineer at TestCorp - Lever", Link: "https://lever.co/testcorp/1"},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer serpTs.Close()

	// Mock RemoteOK
	roTs := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mockJobs := []interface{}{
			map[string]interface{}{"legal": "notice"},
			RemoteOkJob{
				Company:  "RemoteCorp",
				Position: "Backend",
				URL:      "https://remoteok.com/job/1",
			},
		}
		json.NewEncoder(w).Encode(mockJobs)
	}))
	defer roTs.Close()

	os.Setenv("SERPAPI_API_KEY", "test_key")

	origSerp := serpAPIBaseURL
	origRO := remoteOKBaseURL
	serpAPIBaseURL = serpTs.URL
	remoteOKBaseURL = roTs.URL
	defer func() {
		serpAPIBaseURL = origSerp
		remoteOKBaseURL = origRO
	}()

	engine := NewFunnelEngine([]string{"backend"})
	// Use small subset for faster test
	engine.TargetATS = []string{"lever.co"}

	jobChan := make(chan Job, 10)
	
	err = engine.DiscoverJobs(jobChan)
	if err != nil {
		t.Fatalf("DiscoverJobs failed: %v", err)
	}

	close(jobChan)

	var jobs []Job
	for j := range jobChan {
		jobs = append(jobs, j)
	}

	if len(jobs) != 2 {
		t.Fatalf("expected 2 jobs, got %d", len(jobs))
	}
}

func TestExtractCompanyFromTitle(t *testing.T) {
	tests := []struct{
		title    string
		expected string
	}{
		{"Senior Backend Engineer at Stripe - Lever", "Stripe"},
		{"Software Engineer - Google", "Software Engineer"}, // fallback behavior
		{"No format title", "Unknown Company"}, // fallback
	}
	
	for _, tt := range tests {
		got := extractCompanyFromTitle(tt.title)
		if got != tt.expected {
			t.Errorf("extractCompanyFromTitle(%q) = %q, want %q", tt.title, got, tt.expected)
		}
	}
}

func TestDiscoverWithYahooFallback(t *testing.T) {
	oldSleep := SleepFunc
	SleepFunc = func(time.Duration) {}
	defer func() { SleepFunc = oldSleep }()

	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test.db")
	err := storage.InitDBWithPath(dbPath)
	if err != nil {
		t.Fatalf("failed to init db: %v", err)
	}
	defer storage.CloseDB()

	// Mock SerpAPI to return rate-limit error
	serpTs := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := SerpApiResponse{
			Error: "Rate limit exceeded",
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer serpTs.Close()

	// Mock Yahoo to return some HTML
	yahooTs := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		html := `<html><body><a href="https://r.search.yahoo.com/_ylt=Awr.../RV=2/RE=.../RO=10/RU=https%3a%2f%2fjobs.lever.co%2fTestCorp%2f123/RK=2/RS=...">Test</a></body></html>`
		w.Write([]byte(html))
	}))
	defer yahooTs.Close()

	// Mock RemoteOK
	roTs := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]interface{}{}) // empty
	}))
	defer roTs.Close()

	os.Setenv("SERPAPI_API_KEY", "test_key")

	origSerp := serpAPIBaseURL
	origYahoo := yahooBaseURL
	origRO := remoteOKBaseURL
	serpAPIBaseURL = serpTs.URL
	yahooBaseURL = yahooTs.URL
	remoteOKBaseURL = roTs.URL
	defer func() {
		serpAPIBaseURL = origSerp
		yahooBaseURL = origYahoo
		remoteOKBaseURL = origRO
	}()

	engine := NewFunnelEngine([]string{"backend"})
	engine.TargetATS = []string{"lever.co"}

	jobChan := make(chan Job, 10)
	err = engine.DiscoverJobs(jobChan)
	if err != nil {
		t.Fatalf("DiscoverJobs failed: %v", err)
	}
	close(jobChan)

	var jobs []Job
	for j := range jobChan {
		jobs = append(jobs, j)
	}

	if len(jobs) != 1 {
		t.Fatalf("expected 1 job from Yahoo fallback, got %d", len(jobs))
	}
	if jobs[0].URL != "https://jobs.lever.co/TestCorp/123" {
		t.Errorf("unexpected job URL: %s", jobs[0].URL)
	}
}

func TestHybridOnsiteFiltering(t *testing.T) {
	mockJobs := []interface{}{
		map[string]interface{}{"legal": "notice"},
		RemoteOkJob{
			Company:  "Hybrid Company",
			Position: "Backend",
			Location: "Hybrid",
			URL:      "https://test.com/job/hybrid",
		},
		RemoteOkJob{
			Company:  "Onsite Company",
			Position: "Backend",
			Location: "Onsite",
			URL:      "https://test.com/job/onsite",
		},
		RemoteOkJob{
			Company:  "Remote Company",
			Position: "Backend",
			Location: "Remote",
			URL:      "https://test.com/job/remote",
		},
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(mockJobs)
	}))
	defer ts.Close()

	originalURL := remoteOKBaseURL
	remoteOKBaseURL = ts.URL
	defer func() { remoteOKBaseURL = originalURL }()

	engine := NewEngine(100000, []string{"backend"})
	jobs, err := engine.FetchJobs()
	if err != nil {
		t.Fatalf("FetchJobs failed: %v", err)
	}

	if len(jobs) != 3 {
		t.Fatalf("expected 3 jobs to be fetched, got %d", len(jobs))
	}

	for _, j := range jobs {
		if j.Location == "Remote" && !j.Remote {
			t.Errorf("expected remote to be true for Remote location")
		}
		if (j.Location == "Hybrid" || j.Location == "Onsite") && j.Remote {
			t.Errorf("expected remote to be false for %s location", j.Location)
		}
	}
}
