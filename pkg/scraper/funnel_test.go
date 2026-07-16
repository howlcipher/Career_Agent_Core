package scraper

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/howlcipher/Career_Agent_Core/pkg/storage"
)

func TestDiscoverJobs(t *testing.T) {
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
