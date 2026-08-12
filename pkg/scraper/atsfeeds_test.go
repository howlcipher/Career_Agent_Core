package scraper

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/howlcipher/Career_Agent_Core/pkg/storage"
)

// improvements.md #26: board feeds return a company's complete current posting
// list as structured data, replacing search-dorking guesswork for these two
// platforms. Parsers are pinned against the real response shapes, captured
// live from boards-api.greenhouse.io and api.lever.co on 2026-07-25.
func TestParseGreenhouseBoard(t *testing.T) {
	body := []byte(`{"jobs":[
		{"absolute_url":"https://job-boards.greenhouse.io/reddit/jobs/8069214","title":"Senior Backend Engineer"},
		{"absolute_url":"https://job-boards.greenhouse.io/reddit/jobs/8069215","title":"  Site Reliability Engineer  "}
	]}`)
	jobs, err := parseGreenhouseBoard(body)
	if err != nil {
		t.Fatalf("parseGreenhouseBoard: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("expected 2 jobs, got %d", len(jobs))
	}
	if jobs[0].URL != "https://job-boards.greenhouse.io/reddit/jobs/8069214" {
		t.Errorf("unexpected URL: %q", jobs[0].URL)
	}
	if jobs[1].Title != "Site Reliability Engineer" {
		t.Errorf("title should be trimmed, got %q", jobs[1].Title)
	}
}

func TestParseLeverBoard(t *testing.T) {
	body := []byte(`[
		{"hostedUrl":"https://jobs.lever.co/smarsh/abc-123","text":"Staff Platform Engineer"},
		{"hostedUrl":"https://jobs.lever.co/smarsh/def-456","text":"DevOps Engineer"}
	]`)
	jobs, err := parseLeverBoard(body)
	if err != nil {
		t.Fatalf("parseLeverBoard: %v", err)
	}
	if len(jobs) != 2 || jobs[1].Title != "DevOps Engineer" {
		t.Fatalf("unexpected parse result: %+v", jobs)
	}
	if !strings.HasPrefix(jobs[0].URL, "https://jobs.lever.co/smarsh/") {
		t.Errorf("unexpected URL: %q", jobs[0].URL)
	}
}

// Both parsers discarded the location the feeds publish until bug #516, which
// is what let an India-only posting reach a live application attempt. These
// payloads are the real shapes: Lever's was captured live from
// api.lever.co/v0/postings/jobgether on 2026-08-05, including the exact
// country/categories pairing of the posting that halted the trial.
func TestParseLeverBoard_CapturesLocation(t *testing.T) {
	body := []byte(`[
		{"hostedUrl":"https://jobs.lever.co/jobgether/abc","text":"AI Automation Engineer",
		 "country":"IN","workplaceType":"remote",
		 "categories":{"location":"India","allLocations":["India"],"commitment":"Full-time"}},
		{"hostedUrl":"https://jobs.lever.co/acme/def","text":"Platform Engineer",
		 "country":"US","workplaceType":"onsite",
		 "categories":{"location":"Denver, CO","allLocations":["Denver, CO"]}},
		{"hostedUrl":"https://jobs.lever.co/acme/ghi","text":"Backend Engineer",
		 "categories":{"allLocations":["Toronto","Vancouver"]}}
	]`)
	jobs, err := parseLeverBoard(body)
	if err != nil {
		t.Fatalf("parseLeverBoard: %v", err)
	}
	if len(jobs) != 3 {
		t.Fatalf("expected 3 jobs, got %d", len(jobs))
	}
	if jobs[0].Location != "India" || len(jobs[0].CountryCodes) != 1 || jobs[0].CountryCodes[0] != "IN" {
		t.Errorf("India posting parsed as location=%q codes=%v", jobs[0].Location, jobs[0].CountryCodes)
	}
	if !jobs[0].Remote {
		t.Error("workplaceType remote should set Remote")
	}
	if jobs[1].Remote {
		t.Error("workplaceType onsite must not set Remote")
	}
	// allLocations is the fallback when categories.location is absent.
	if jobs[2].Location != "Toronto, Vancouver" {
		t.Errorf("allLocations fallback produced %q", jobs[2].Location)
	}
	if len(jobs[2].CountryCodes) != 0 {
		t.Errorf("absent country must yield no codes, got %v", jobs[2].CountryCodes)
	}

	// The gate must reject the first and admit the others.
	if allowed, _ := LocationAllowed(jobs[0].Location, jobs[0].CountryCodes, []string{"US", "CA"}); allowed {
		t.Error("India posting should be rejected by a US/CA allowlist")
	}
	if allowed, _ := LocationAllowed(jobs[1].Location, jobs[1].CountryCodes, []string{"US", "CA"}); !allowed {
		t.Error("US posting should be allowed")
	}
}

func TestParseGreenhouseBoard_CapturesLocation(t *testing.T) {
	body := []byte(`{"jobs":[
		{"absolute_url":"https://job-boards.greenhouse.io/acme/jobs/1","title":"SRE","location":{"name":"Remote - United States"}},
		{"absolute_url":"https://job-boards.greenhouse.io/acme/jobs/2","title":"Analyst","location":{"name":"Bengaluru, India"}}
	]}`)
	jobs, err := parseGreenhouseBoard(body)
	if err != nil {
		t.Fatalf("parseGreenhouseBoard: %v", err)
	}
	if jobs[0].Location != "Remote - United States" || !jobs[0].Remote {
		t.Errorf("US remote posting parsed as location=%q remote=%v", jobs[0].Location, jobs[0].Remote)
	}
	if allowed, _ := LocationAllowed(jobs[0].Location, jobs[0].CountryCodes, []string{"US", "CA"}); !allowed {
		t.Error("Remote - United States should be allowed")
	}
	if allowed, _ := LocationAllowed(jobs[1].Location, jobs[1].CountryCodes, []string{"US", "CA"}); allowed {
		t.Error("Bengaluru, India should be rejected")
	}
}

// A board that returns something unexpected must produce an error rather than
// silently contributing zero jobs, so the cause is visible in the log.
func TestParseBoards_RejectMalformedPayloads(t *testing.T) {
	if _, err := parseGreenhouseBoard([]byte(`<html>not json</html>`)); err == nil {
		t.Error("expected an error for non-JSON Greenhouse payload")
	}
	if _, err := parseLeverBoard([]byte(`{"not":"an array"}`)); err == nil {
		t.Error("expected an error for a non-array Lever payload")
	}
}

func TestPollBoard_RetriesOnTransientAndParseErrors(t *testing.T) {
	// Speed up tests
	origBackoff := retryBackoffBase
	retryBackoffBase = time.Millisecond
	defer func() { retryBackoffBase = origBackoff }()

	attempts := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		if attempts == 2 {
			w.WriteHeader(http.StatusOK)
			// Malformed JSON simulating truncation
			w.Write([]byte(`{"jobs": [{"title": "Truncated"`))
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"jobs": []}`)) // Successful empty response
	}))
	defer ts.Close()

	f := &FunnelEngine{}
	f.pollBoard("test-company", ts.URL, parseGreenhouseBoard, "atsfeed:greenhouse", nil)

	if attempts != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts)
	}
}

func TestPollBoard_NoRetryOn404(t *testing.T) {
	attempts := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	f := &FunnelEngine{}
	f.pollBoard("test-company", ts.URL, parseGreenhouseBoard, "atsfeed:greenhouse", nil)

	if attempts != 1 {
		t.Errorf("expected 1 attempt for a 404, got %d", attempts)
	}
}

func TestPollBoardPersistsDiscoverySource(t *testing.T) {
	if err := storage.InitDBWithPath(filepath.Join(t.TempDir(), "test.db")); err != nil {
		t.Fatalf("InitDBWithPath: %v", err)
	}
	defer storage.CloseDB()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"jobs":[{"absolute_url":"https://job-boards.greenhouse.io/acme/jobs/1","title":"Backend Engineer"}]}`))
	}))
	defer ts.Close()

	f := &FunnelEngine{Roles: []string{"Backend Engineer"}}
	if found := f.pollBoard("acme", ts.URL, parseGreenhouseBoard, "atsfeed:greenhouse", nil); found != 0 {
		t.Fatalf("found = %d, want 0 when no queue channel is supplied", found)
	}

	source, found, err := storage.GetDiscoverySource("https://job-boards.greenhouse.io/acme/jobs/1")
	if err != nil {
		t.Fatalf("read persisted source: %v", err)
	}
	if !found {
		t.Fatal("expected a persisted discovery source")
	}
	if source != "atsfeed:greenhouse" {
		t.Fatalf("discovery_source = %q, want atsfeed:greenhouse", source)
	}
}

// With RemoteOnly enabled, a feed posting whose location already says
// "hybrid" must never reach the funnel, let alone the assisted-apply queue.
func TestPollBoard_RejectsHybridWhenRemoteOnly(t *testing.T) {
	if err := storage.InitDBWithPath(filepath.Join(t.TempDir(), "test.db")); err != nil {
		t.Fatalf("InitDBWithPath: %v", err)
	}
	defer storage.CloseDB()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"jobs":[
			{"absolute_url":"https://job-boards.greenhouse.io/hybridco/jobs/1","title":"DevOps Engineer","location":{"name":"Hybrid - Austin, TX"}},
			{"absolute_url":"https://job-boards.greenhouse.io/hybridco/jobs/2","title":"DevOps Engineer","location":{"name":"Remote - United States"}}
		]}`))
	}))
	defer ts.Close()

	f := &FunnelEngine{Roles: []string{"DevOps Engineer"}, RemoteOnly: true}
	jobChan := make(chan Job, 10)
	found := f.pollBoard("hybridco", ts.URL, parseGreenhouseBoard, "atsfeed:greenhouse", jobChan)
	close(jobChan)
	if found != 1 {
		t.Fatalf("found = %d, want exactly the fully-remote posting", found)
	}
	for job := range jobChan {
		if job.URL == "https://job-boards.greenhouse.io/hybridco/jobs/1" {
			t.Fatalf("hybrid posting reached the funnel: %+v", job)
		}
	}
}

// Feeds must not be able to bypass the junk filter every other source passes
// through — a board can legitimately list non-posting URLs.
func TestFeedJobsStillPassThroughJunkFilter(t *testing.T) {
	if !IsKnownJunkJobURL("https://www.bamboohr.com/integrations/listings/remote") {
		t.Skip("junk filter does not classify this sample; nothing to assert")
	}
	if IsKnownJunkJobURL("https://job-boards.greenhouse.io/reddit/jobs/8069214") {
		t.Error("a real posting URL must not be filtered as junk")
	}
}

// The title gate exists because a board feed returns a company's whole
// posting list (238 for remotecom live), and each irrelevant one would
// otherwise cost ~10 minutes of local fit-scoring.
func TestTitleLooksRelevant(t *testing.T) {
	// Mirrors a representative slice of the real profile.yaml roles list --
	// the gate only matches distinctive words that actually appear in a
	// configured role, so the fixture must reflect what is configured.
	f := &FunnelEngine{Roles: []string{
		"Site Reliability Engineer", "Senior Backend Engineer",
		"DevOps Engineer", "Go Developer", "API Developer",
		"Infrastructure Engineer", "Cloud Engineer", "Platform Engineer",
	}}

	keep := []string{
		"Site Reliability Engineer",
		"Staff Backend Engineer, Payments",
		"Senior DevOps Engineer (Remote)",
		"Principal Infrastructure Engineer",
		"Cloud Security Engineer",
	}
	for _, title := range keep {
		if !f.titleLooksRelevant(title) {
			t.Errorf("expected %q to be kept for scoring", title)
		}
	}

	drop := []string{
		"Accountant",
		"Administrative Business Partner",
		"Account Executive DACH",
		"Office Manager",
		"Senior Recruiter",
	}
	for _, title := range drop {
		if f.titleLooksRelevant(title) {
			t.Errorf("expected %q to be filtered out before scoring", title)
		}
	}
}

// Regression guard: short distinctive tokens must match whole words only.
// Substring matching would let "go" hit "Cargo"/"Chicago" and "api" hit
// "capital", waving through the exact roles this filter exists to stop.
func TestTitleLooksRelevant_ShortTokensDoNotMatchSubstrings(t *testing.T) {
	f := &FunnelEngine{Roles: []string{"Go Developer", "API Developer"}}
	for _, title := range []string{"Cargo Operations Manager", "Chicago Account Manager", "Capital Markets Analyst"} {
		if f.titleLooksRelevant(title) {
			t.Errorf("%q must not match on a substring of a short token", title)
		}
	}
	if !f.titleLooksRelevant("Senior Go Developer") {
		t.Error("a genuine whole-word match must still be kept")
	}
}

// With no configured roles the gate must open, never silently discard
// everything.
func TestTitleLooksRelevant_NoRolesConfiguredKeepsEverything(t *testing.T) {
	f := &FunnelEngine{}
	if !f.titleLooksRelevant("Accountant") {
		t.Error("with no roles configured nothing should be filtered out")
	}
}
