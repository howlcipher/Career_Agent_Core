package scraper

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
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
	f.pollBoard("test-company", ts.URL, parseGreenhouseBoard, nil)

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
	f.pollBoard("test-company", ts.URL, parseGreenhouseBoard, nil)

	if attempts != 1 {
		t.Errorf("expected 1 attempt for a 404, got %d", attempts)
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
