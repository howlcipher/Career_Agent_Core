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

func TestIsValidATSUrl(t *testing.T) {
	tests := []struct {
		url  string
		want bool
	}{
		{"https://developer.workday.com/welcome", false},
		{"https://developer.workday.com/documentation", false},
		{"https://acme.myworkdayjobs.com/en-US/External/job/12345", true},
		{"https://jobs.workable.com/search/global/remote-software-engineer-jobs", false},
		{"https://apply.workable.com/protera/j/5E238E8E8A/", true},
		{"https://jobs.lever.co/TestCorp/123", true},
		{"https://example.com/random-page", false},
		// Jobvite listing/search pages are not postings (bugs.md #11)
		{"https://jobs.jobvite.com/cloudone-digital/search", false},
		{"https://jobs.jobvite.com/acme/search/", false},
		{"https://jobs.jobvite.com/acme/search/all", false},
		// Real Jobvite postings must still pass
		{"https://jobs.jobvite.com/dwt/job/o79Qzfwp/apply", true},
		// Expired-posting error redirects must never be discovered as jobs
		{"https://job-boards.greenhouse.io/remotecom?error=true", false},
		{"https://jobs.jobvite.com/careers/dwt/jobs?error=404", false},
		// Workday corporate subdomains are never postings
		{"https://digital.workday.com/en-us/whatever", false},
	}

	for _, tt := range tests {
		got := isValidATSUrl(tt.url)
		if got != tt.want {
			t.Errorf("isValidATSUrl(%q) = %v, want %v", tt.url, got, tt.want)
		}
	}
}

func TestIsKnownJunkJobURL(t *testing.T) {
	tests := []struct {
		url  string
		want bool
	}{
		// All three workday.com corporate subdomains seen live, plus any other
		{"https://www.workday.com/en-us/company/careers/hiring-programs.html", true},
		{"https://developer.workday.com/welcome", true},
		{"https://digital.workday.com/en-us/x", true},
		{"https://workday.com/anything", true},
		{"https://job-boards.greenhouse.io/remotecom?error=true", true},
		{"https://jobs.lever.co/gohighlevel/?workplaceType=remote", true},
		{"https://jobs.jobvite.com/acme/search", true},
		{"https://jobs.workable.com/search/global/remote-jobs", true},
		// Real postings must never be junk — including Workday tenants
		{"https://gdit.wd5.myworkdayjobs.com/External_Career_Site/job/X/SRE_RQ1", false},
		{"https://jobs.lever.co/acme/abc-123", false},
		{"https://job-boards.greenhouse.io/mixpanel/jobs/7941929", false},
		// Non-ATS sources (RemoteOK company sites) must pass the blacklist
		{"https://somestartup.com/careers/backend-engineer", false},
		// Company board-index pages on path-tenant ATS hosts are never postings
		{"https://careers.smartrecruiters.com/aristanetworks", true},
		{"https://jobs.lever.co/mistral", true},
		{"https://job-boards.greenhouse.io/remotecom", true},
		{"https://jobs.jobvite.com/cloudone-digital/", true},
		// Real postings carry more segments and must pass
		{"https://jobs.smartrecruiters.com/sosi1/3743990013881284-cloud-web-developer", false},
		{"https://apply.workable.com/azumo/j/DC928C07B2/", false},
	}
	for _, tt := range tests {
		if got := IsKnownJunkJobURL(tt.url); got != tt.want {
			t.Errorf("IsKnownJunkJobURL(%q) = %v, want %v", tt.url, got, tt.want)
		}
	}
}

func TestCompanyFromURL(t *testing.T) {
	tests := []struct {
		url  string
		want string
	}{
		// Subdomain-tenant platforms: company is the first host label
		{"https://gdit.wd5.myworkdayjobs.com/External_Career_Site/job/Any-Location--Remote/SRE_RQ219922-1", "gdit"},
		{"https://redhat.wd5.myworkdayjobs.com/en-US/jobs/123", "redhat"},
		{"https://techinsights.applytojob.com/apply/xyz", "techinsights"},
		{"https://jway-group.breezy.hr/p/419b44576d64-backend-developer", "jway-group"},
		// Path-tenant platforms: first non-generic, non-locale path segment
		{"https://boards.eu.greenhouse.io/nebius/jobs/4558243101", "nebius"},
		{"https://jobs.lever.co/mistral/f76907fd-428a", "mistral"},
		{"https://jobs.jobvite.com/careers/dwt/jobs", "dwt"},
		{"https://apply.workable.com/azumo/j/DC928C07B2/", "azumo"},
		// Locale and generic segments are never a company
		{"https://uhaul.wd1.myworkdayjobs.com/en-US/Uhauljobs/job/123", "uhaul"},
		{"https://example.pinpointhq.com/en/postings/abc", "example"},
		// Nothing plausible: empty result, caller falls back
		{"https://jobs.example.com/en-US/", ""},
		{"not a url ://", ""},
	}

	for _, tt := range tests {
		if got := companyFromURL(tt.url); got != tt.want {
			t.Errorf("companyFromURL(%q) = %q, want %q", tt.url, got, tt.want)
		}
	}
}
