package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/howlcipher/Career_Agent_Core/pkg/storage"
	_ "modernc.org/sqlite"
)

func TestServeAssistedRevalidate_UsesValidatedIDWithoutLaunchingBrowser(t *testing.T) {
	setupTestDB(t)
	previous := revalidateAssistedPlan
	t.Cleanup(func() { revalidateAssistedPlan = previous })
	called := false
	revalidateAssistedPlan = func(ctx context.Context, conn *sql.DB, jobID string) error {
		called = true
		if conn != db || jobID != "41" {
			t.Fatalf("revalidation inputs = conn %p, id %q", conn, jobID)
		}
		return nil
	}
	req := httptest.NewRequest(http.MethodPost, "/api/assisted/revalidate", bytes.NewBufferString(`{"job_id":"41"}`))
	rec := httptest.NewRecorder()
	serveAssistedRevalidate(rec, req)
	if rec.Code != http.StatusOK || !called {
		t.Fatalf("status=%d called=%v body=%q", rec.Code, called, rec.Body.String())
	}
}

func TestServeAssistedLaunch_RejectsUnavailablePlanBeforeStartingProcess(t *testing.T) {
	setupTestDB(t)
	previous := launchAssistedApplication
	t.Cleanup(func() { launchAssistedApplication = previous })
	started := false
	launchAssistedApplication = func(string) error {
		started = true
		return nil
	}
	req := httptest.NewRequest(http.MethodPost, "/api/assisted/launch", bytes.NewBufferString(`{"job_id":"41"}`))
	rec := httptest.NewRecorder()
	serveAssistedLaunch(rec, req)
	if rec.Code != http.StatusConflict || started {
		t.Fatalf("status=%d started=%v body=%q", rec.Code, started, rec.Body.String())
	}
}

// Bug #520: a Lever plan can be complete and fully revalidated and still must
// not spawn an assisted browser, because that ATS refuses the resulting
// submission. The operator needs to be told what actually works, not that the
// application has gone missing.
func TestServeAssistedLaunch_RefusesATSThatRejectsTheAssistedBrowser(t *testing.T) {
	setupTestDB(t)
	previous := launchAssistedApplication
	t.Cleanup(func() { launchAssistedApplication = previous })
	started := false
	launchAssistedApplication = func(string) error {
		started = true
		return nil
	}
	now := time.Now().UTC()
	if _, err := db.Exec(`INSERT INTO job_funnel (url, id, company_name, job_title, status, discovered_at)
		VALUES (?, 41, 'Veeva', 'Platform Engineer', 'AWAITING_REVIEW', ?)`,
		"https://jobs.lever.co/veeva/abc-123", now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO assisted_applications
		(job_id, original_status, next_action_code, assisted_state, revalidation_state, revalidation_version, created_at, updated_at)
		VALUES (41, 'AWAITING_REVIEW', 'review_and_submit', 'waiting_human', 'application_ready', 3, ?, ?)`,
		now, now); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/assisted/launch", bytes.NewBufferString(`{"job_id":"41"}`))
	rec := httptest.NewRecorder()
	serveAssistedLaunch(rec, req)
	if started {
		t.Fatal("no assisted browser process may start for an ATS that rejects it")
	}
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, body = %q", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "your own browser") {
		t.Fatalf("operator message must name the working next step, got %q", rec.Body.String())
	}
}

func TestAssistedApplicationCommandUsesCheckoutRoot(t *testing.T) {
	cmd, err := assistedApplicationCommand("41")
	if err != nil {
		t.Fatal(err)
	}
	if cmd.Dir == "" || !strings.Contains(cmd.Dir, "career") && !strings.Contains(cmd.Dir, "Career") {
		t.Fatalf("assisted command directory = %q", cmd.Dir)
	}
	if len(cmd.Args) < 4 || cmd.Args[0] != "go" || cmd.Args[1] != "run" || cmd.Args[2] != "./cmd/assist" {
		t.Fatalf("assisted command = %#v", cmd.Args)
	}
}

func TestCurrentPageShowsCaptcha_RequiresChallengeLanguage(t *testing.T) {
	for _, tc := range []struct {
		page string
		want bool
	}{
		{"<title>Meesho</title><div class=spinner></div>", false},
		{"<div class=g-recaptcha></div><p>Platform Engineer</p>", false},
		{"<title>Attention Required</title><p>Verify you are human</p>", true},
	} {
		if got := currentPageShowsCaptcha(tc.page); got != tc.want {
			t.Errorf("currentPageShowsCaptcha(%q) = %v, want %v", tc.page, got, tc.want)
		}
	}
}

func TestClassifyCurrentApplicationPage_RequiresRoleAndEntryPoint(t *testing.T) {
	for _, tc := range []struct {
		name, html, role, want string
	}{
		{"matching application", "<title>Example - AI Field Engineer</title><button>Apply now</button>", "AI Field Engineer", "application_ready"},
		{"wrong role", "<title>Example - Assistant Manager - Ops</title><button>Apply for this job</button>", "DevOps Engineer", "unavailable"},
		{"role only in description", "<title>Meesho - Forward Deployed Engineer II</title><p>Partner with Platform Engineering.</p><button>Apply for this job</button>", "Platform Engineer", "unavailable"},
		{"role in heading", "<h1>Platform Engineer</h1><button>Apply now</button>", "Platform Engineer", "application_ready"},
		{"blank shell", "<div class=spinner></div>", "AI Architect", "unavailable"},
		{"captcha", "<p>Verify you are human</p>", "AI Architect", "captcha_confirmed"},
	} {
		if got := classifyCurrentApplicationPage(tc.html, tc.role); got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.name, got, tc.want)
		}
	}
}

func setupTestDB(t *testing.T) {
	t.Helper()
	var err error
	db, err = sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })

	schema := `
	CREATE TABLE job_funnel (
		url TEXT PRIMARY KEY,
		id INTEGER,
		company_name TEXT,
		job_title TEXT,
		status TEXT,
		status_reason TEXT,
		last_updated DATETIME,
		discovered_at DATETIME,
		applied_at DATETIME,
		next_eligible_at DATETIME,
		tone_variant TEXT,
		processing_intent TEXT
	);
	CREATE TABLE applied_jobs (
		company_name TEXT,
		job_title TEXT,
		url TEXT,
		applied_at DATETIME
	);
	CREATE TABLE application_attempts (
		id INTEGER PRIMARY KEY,
		url TEXT,
		started_at DATETIME
	);
	CREATE TABLE discovery_refresh (
		id INTEGER PRIMARY KEY,
		started_at DATETIME,
		finished_at DATETIME,
		new_eligible INTEGER,
		error_class TEXT
	);
	CREATE TABLE assisted_applications (
		job_id INTEGER PRIMARY KEY,
		original_status TEXT,
		next_action_code TEXT,
		interruption_reason TEXT,
		assisted_state TEXT DEFAULT 'ready',
		lease_owner TEXT DEFAULT '',
		lease_expires_at DATETIME,
		is_legacy BOOLEAN DEFAULT 0,
		revalidation_state TEXT DEFAULT 'required',
		revalidation_version INTEGER DEFAULT 0,
		revalidated_at DATETIME,
		confirmation_provenance TEXT,
		created_at DATETIME,
		updated_at DATETIME
	);`
	if _, err := db.Exec(schema); err != nil {
		t.Fatalf("failed to create schema: %v", err)
	}
}

func TestServeMetrics_MissionMetrics(t *testing.T) {
	setupTestDB(t)

	now := time.Now().UTC().Truncate(time.Second)
	insertJob := func(url, status string, discoveredAt, appliedAt time.Time) {
		t.Helper()
		if _, err := db.Exec(`INSERT INTO job_funnel
			(url, status, discovered_at, applied_at) VALUES (?, ?, ?, ?)`,
			url, status, discoveredAt, appliedAt); err != nil {
			t.Fatal(err)
		}
	}
	insertAttempt := func(url string, startedAt time.Time) {
		t.Helper()
		if _, err := db.Exec("INSERT INTO application_attempts (url, started_at) VALUES (?, ?)", url, startedAt); err != nil {
			t.Fatal(err)
		}
	}

	insertJob("https://example.com/today", "APPLIED", now.Add(-2*time.Hour), now)
	insertJob("https://example.com/this-week", "APPLIED", now.Add(-6*time.Hour), now.Add(-2*24*time.Hour))
	insertJob("https://example.com/older", "APPLIED", now.Add(-10*time.Hour), now.Add(-8*24*time.Hour))
	insertJob("https://example.com/first-attempt-one", "FAILED_SUBMIT", now.Add(-2*time.Hour), time.Time{})
	insertJob("https://example.com/first-attempt-two", "FAILED_SUBMIT", now.Add(-6*time.Hour), time.Time{})
	insertAttempt("https://example.com/first-attempt-one", now)
	insertAttempt("https://example.com/first-attempt-two", now)
	insertJob("https://example.com/eligible-new", "DISCOVERED", now, time.Time{})
	insertJob("https://example.com/eligible-attempted", "DISCOVERED", now.Add(-4*time.Hour), time.Time{})
	insertAttempt("https://example.com/eligible-attempted", now)
	insertJob("https://jobs.breezy.hr/excluded", "DISCOVERED", now, time.Time{})
	if _, err := db.Exec("INSERT INTO job_funnel (url, status, next_eligible_at) VALUES (?, ?, ?)",
		"https://example.com/backing-off", "DISCOVERED", now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}

	m := fetchMetricsFromTestServer(t)
	if m.ConfirmedToday != 1 || m.ConfirmedLast7Days != 2 {
		t.Errorf("confirmed cadence = today %d, last 7 days %d; want 1 and 2", m.ConfirmedToday, m.ConfirmedLast7Days)
	}
	if m.FirstAttemptMedian != "4h 0m" {
		t.Errorf("first-attempt median = %q, want 4h 0m", m.FirstAttemptMedian)
	}
	if m.LastConfirmedAgo == "" {
		t.Error("last confirmed age should be available when an application was just confirmed")
	}
	if m.EligibleQueue != 2 || m.EligibleNeverAttempted != 1 {
		t.Errorf("eligible queue = %d, never attempted = %d; want 2 and 1", m.EligibleQueue, m.EligibleNeverAttempted)
	}
}

func fetchMetricsFromTestServer(t *testing.T) Metrics {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/metrics", nil)
	rec := httptest.NewRecorder()
	serveMetrics(rec, req)

	var m Metrics
	if err := json.NewDecoder(rec.Body).Decode(&m); err != nil {
		t.Fatalf("failed to decode metrics response: %v", err)
	}
	return m
}

func TestServeMetrics_Counts(t *testing.T) {
	setupTestDB(t)

	db.Exec("INSERT INTO job_funnel (url, status) VALUES (?, ?)", "https://a.example.com", "DISCOVERED")
	db.Exec("INSERT INTO job_funnel (url, status) VALUES (?, ?)", "https://b.example.com", "PROCESSING")
	db.Exec("INSERT INTO job_funnel (url, status) VALUES (?, ?)", "https://c.example.com", "SKIPPED")
	db.Exec("INSERT INTO job_funnel (url, status) VALUES (?, ?)", "https://d.example.com", "APPLIED")
	db.Exec("INSERT INTO job_funnel (url, status) VALUES (?, ?)", "https://e.example.com", "FAILED_SUBMIT")
	db.Exec("INSERT INTO job_funnel (url, status) VALUES (?, ?)", "https://f.example.com", "BLOCKED_CAPTCHA")
	db.Exec("INSERT INTO job_funnel (url, status) VALUES (?, ?)", "https://g.example.com", "INVALID_URL")

	m := fetchMetricsFromTestServer(t)

	if m.Discovered != 1 || m.Processing != 1 || m.Skipped != 1 || m.Applied != 1 || m.Failed != 1 {
		t.Errorf("unexpected counts: %+v", m)
	}
	// BLOCKED_CAPTCHA and INVALID_URL each get their own dedicated tile
	// (bugs.md #55) rather than being silently absent from every metric --
	// confirmed live 2026-07-24: 337 real rows (9% of the whole table) had
	// no tile at all before this fix.
	if m.BlockedCaptcha != 1 || m.InvalidURL != 1 {
		t.Errorf("expected BlockedCaptcha=1 and InvalidURL=1, got %+v", m)
	}
	// Neither of these two statuses should be double-counted into Skipped
	// or Failed.
	if m.Skipped != 1 {
		t.Errorf("expected BLOCKED_CAPTCHA to not inflate the Skipped count, got %d", m.Skipped)
	}
}

func TestServeMetricsIncludesLatestDiscoveryRefresh(t *testing.T) {
	setupTestDB(t)
	if _, err := db.Exec(`ALTER TABLE discovery_refresh ADD COLUMN source_counts_json TEXT`); err != nil {
		t.Fatal(err)
	}
	finished := time.Date(2026, 8, 2, 15, 4, 0, 0, time.UTC)
	if _, err := db.Exec(`INSERT INTO discovery_refresh (id, started_at, finished_at, new_eligible, error_class, source_counts_json)
		VALUES (1, ?, ?, 0, 'network', ?)`, finished.Add(-time.Minute), finished,
		`[{"source":"yahoo","request_attempted":3,"request_failed":1,"circuit_open_skipped":2}]`); err != nil {
		t.Fatal(err)
	}
	m := fetchMetricsFromTestServer(t)
	if m.DiscoveryNewEligible != 0 || m.DiscoveryErrorClass != "network" || m.DiscoveryLastFinishedAt == "" {
		t.Errorf("discovery metrics = %+v", m)
	}
	if len(m.DiscoverySourceCounts) != 1 || m.DiscoverySourceCounts[0].RequestFailed != 1 || m.DiscoverySourceCounts[0].CircuitOpenSkipped != 2 {
		t.Errorf("discovery source metrics = %+v", m.DiscoverySourceCounts)
	}
}

// TestServeMetrics_Counts_SplitsFailedAndManualPairs is the regression test
// for bug #451: the Failed and Manual Queue tiles each aggregate two
// statuses with genuinely different meanings ("failed to score" vs "failed
// to submit"; "ATS requires an account" vs "filled by Copilot, awaiting your
// click") into one number, and the API must expose which member(s) of each
// pair actually contributed so the UI can caption the tile correctly rather
// than hardcoding one status's reason regardless of which one dominates.
func TestServeMetrics_Counts_SplitsFailedAndManualPairs(t *testing.T) {
	cases := []struct {
		name                   string
		statuses               []string
		wantFailed             int
		wantFailedScore        int
		wantFailedSubmit       int
		wantManualRequired     int
		wantManualRequiredOnly int
		wantAwaitingReview     int
	}{
		{
			name:            "only FAILED_SCORE",
			statuses:        []string{"FAILED_SCORE", "FAILED_SCORE"},
			wantFailed:      2,
			wantFailedScore: 2,
		},
		{
			name:             "only FAILED_SUBMIT",
			statuses:         []string{"FAILED_SUBMIT"},
			wantFailed:       1,
			wantFailedSubmit: 1,
		},
		{
			name:             "both failed statuses present",
			statuses:         []string{"FAILED_SCORE", "FAILED_SUBMIT", "FAILED_SUBMIT"},
			wantFailed:       3,
			wantFailedScore:  1,
			wantFailedSubmit: 2,
		},
		{
			name:                   "only MANUAL_REQUIRED",
			statuses:               []string{"MANUAL_REQUIRED"},
			wantManualRequired:     1,
			wantManualRequiredOnly: 1,
		},
		{
			name:               "only AWAITING_REVIEW",
			statuses:           []string{"AWAITING_REVIEW", "AWAITING_REVIEW"},
			wantManualRequired: 2,
			wantAwaitingReview: 2,
		},
		{
			name:                   "both manual statuses present",
			statuses:               []string{"MANUAL_REQUIRED", "AWAITING_REVIEW"},
			wantManualRequired:     2,
			wantManualRequiredOnly: 1,
			wantAwaitingReview:     1,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			setupTestDB(t)
			for i, status := range tc.statuses {
				db.Exec("INSERT INTO job_funnel (url, status) VALUES (?, ?)",
					fmt.Sprintf("https://split-pair-%d.example.com", i), status)
			}

			m := fetchMetricsFromTestServer(t)

			if m.Failed != tc.wantFailed || m.FailedScore != tc.wantFailedScore || m.FailedSubmit != tc.wantFailedSubmit {
				t.Errorf("failed counts: got Failed=%d FailedScore=%d FailedSubmit=%d, want Failed=%d FailedScore=%d FailedSubmit=%d",
					m.Failed, m.FailedScore, m.FailedSubmit, tc.wantFailed, tc.wantFailedScore, tc.wantFailedSubmit)
			}
			if m.ManualRequired != tc.wantManualRequired || m.ManualRequiredOnly != tc.wantManualRequiredOnly || m.AwaitingReview != tc.wantAwaitingReview {
				t.Errorf("manual counts: got ManualRequired=%d ManualRequiredOnly=%d AwaitingReview=%d, want ManualRequired=%d ManualRequiredOnly=%d AwaitingReview=%d",
					m.ManualRequired, m.ManualRequiredOnly, m.AwaitingReview, tc.wantManualRequired, tc.wantManualRequiredOnly, tc.wantAwaitingReview)
			}
		})
	}
}

// TestServeMetrics_Counts_SplitsInvalidURLByReason is a regression test for
// improvements.md #468: a live measurement against applications.db found
// ~88% of INVALID_URL rows are a real posting checkJobAlive correctly caught
// as expired, not the "never a real posting" shape the old single caption
// implied for the whole bucket. InvalidURL must keep counting both, while
// InvalidURLMalformed/InvalidURLExpired split by the persisted status_reason
// -- including a row with no status_reason at all (written before this
// column existed), which must count toward the total but neither sub-bucket.
func TestServeMetrics_Counts_SplitsInvalidURLByReason(t *testing.T) {
	setupTestDB(t)

	db.Exec("INSERT INTO job_funnel (url, status, status_reason) VALUES (?, ?, ?)",
		"https://jobs.lever.co/company", "INVALID_URL", "malformed")
	db.Exec("INSERT INTO job_funnel (url, status, status_reason) VALUES (?, ?, ?)",
		"https://jobs.greenhouse.io/expired-1", "INVALID_URL", "expired")
	db.Exec("INSERT INTO job_funnel (url, status, status_reason) VALUES (?, ?, ?)",
		"https://jobs.greenhouse.io/expired-2", "INVALID_URL", "expired")
	db.Exec("INSERT INTO job_funnel (url, status) VALUES (?, ?)",
		"https://jobs.greenhouse.io/legacy-no-reason", "INVALID_URL")

	m := fetchMetricsFromTestServer(t)

	if m.InvalidURL != 4 {
		t.Errorf("InvalidURL = %d, want 4 (all rows regardless of reason)", m.InvalidURL)
	}
	if m.InvalidURLMalformed != 1 {
		t.Errorf("InvalidURLMalformed = %d, want 1", m.InvalidURLMalformed)
	}
	if m.InvalidURLExpired != 2 {
		t.Errorf("InvalidURLExpired = %d, want 2", m.InvalidURLExpired)
	}
}

// TestServeMetrics_Counts_RetryExhausted is a regression test for
// improvements.md #468: RETRY_EXHAUSTED (bugs.md #466) had zero dashboard
// presence -- absent from every count and the legend -- so a chronically
// failing row silently dropped out of every bucket's total.
func TestServeMetrics_Counts_RetryExhausted(t *testing.T) {
	setupTestDB(t)

	db.Exec("INSERT INTO job_funnel (url, status) VALUES (?, ?)", "https://exhausted-1.example.com", "RETRY_EXHAUSTED")
	db.Exec("INSERT INTO job_funnel (url, status) VALUES (?, ?)", "https://exhausted-2.example.com", "RETRY_EXHAUSTED")
	db.Exec("INSERT INTO job_funnel (url, status) VALUES (?, ?)", "https://still-discovered.example.com", "DISCOVERED")

	m := fetchMetricsFromTestServer(t)

	if m.RetryExhausted != 2 {
		t.Errorf("RetryExhausted = %d, want 2", m.RetryExhausted)
	}
	if reason, ok := m.StatusLegend["RETRY_EXHAUSTED"]; !ok || reason == "" || reason == "RETRY_EXHAUSTED" {
		t.Errorf("StatusLegend[RETRY_EXHAUSTED] = %q, ok=%v, want a human-readable explanation", reason, ok)
	}
}

// TestServeMetrics_LastApplied_OnlyCountsGenuineSuccess is a regression test
// for the bug caught live 2026-07-21: applied_jobs only records that docs
// were generated (SaveApplication runs before the actual browser
// fill/submit), not that the submission itself succeeded. "Last applied"
// must only ever surface a job whose job_funnel status genuinely reached
// APPLIED, not merely one with a row in applied_jobs.
func TestServeMetrics_LastApplied_OnlyCountsGenuineSuccess(t *testing.T) {
	setupTestDB(t)

	oldTime := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	newTime := time.Date(2026, 7, 21, 18, 0, 0, 0, time.UTC)

	// Docs were generated for both, but only the Greenhouse one actually
	// completed submission - the other failed at the fill stage, same shape
	// as bugs #4/#8/#9/#10 all session.
	db.Exec("INSERT INTO job_funnel (url, status) VALUES (?, ?)", "https://jobs.greenhouse.io/real-success", "APPLIED")
	db.Exec("INSERT INTO applied_jobs (company_name, job_title, url, applied_at) VALUES (?, ?, ?, ?)",
		"RealCorp", "Engineer", "https://jobs.greenhouse.io/real-success", oldTime)

	db.Exec("INSERT INTO job_funnel (url, status) VALUES (?, ?)", "https://jobs.example.com/search", "FAILED_SUBMIT")
	db.Exec("INSERT INTO applied_jobs (company_name, job_title, url, applied_at) VALUES (?, ?, ?, ?)",
		"FakeCorp", "Engineer", "https://jobs.example.com/search", newTime)

	m := fetchMetricsFromTestServer(t)

	if m.LastAppliedCompany != "RealCorp" {
		t.Errorf("expected last applied company to be the genuinely-completed job RealCorp (not the more recent but failed FakeCorp), got %q", m.LastAppliedCompany)
	}
	if m.LastAppliedURL != "https://jobs.greenhouse.io/real-success" {
		t.Errorf("unexpected last applied url: %q", m.LastAppliedURL)
	}
}

func TestServeMetrics_LastApplied_EmptyWhenNoneApplied(t *testing.T) {
	setupTestDB(t)

	db.Exec("INSERT INTO job_funnel (url, status) VALUES (?, ?)", "https://jobs.example.com/still-pending", "PROCESSING")
	db.Exec("INSERT INTO applied_jobs (company_name, job_title, url, applied_at) VALUES (?, ?, ?, ?)",
		"PendingCorp", "Engineer", "https://jobs.example.com/still-pending", time.Now())

	m := fetchMetricsFromTestServer(t)

	if m.LastAppliedCompany != "" {
		t.Errorf("expected no last applied company when nothing has genuinely completed, got %q", m.LastAppliedCompany)
	}
}

func TestServeMetrics_CurrentlyProcessing_PicksMostRecentlyTouched(t *testing.T) {
	setupTestDB(t)

	old := time.Date(2026, 7, 20, 9, 0, 0, 0, time.UTC)
	recent := time.Date(2026, 7, 21, 21, 40, 0, 0, time.UTC)

	// A job stuck at PROCESSING from an earlier, interrupted run - must not
	// be shown as "currently" active over the genuinely recent one.
	db.Exec("INSERT INTO job_funnel (url, company_name, job_title, status, last_updated) VALUES (?, ?, ?, ?, ?)",
		"https://jobs.example.com/stuck", "StuckCorp", "Old Role", "PROCESSING", old)
	db.Exec("INSERT INTO job_funnel (url, company_name, job_title, status, last_updated) VALUES (?, ?, ?, ?, ?)",
		"https://jobs.example.com/active", "ActiveCorp", "New Role", "PROCESSING", recent)

	m := fetchMetricsFromTestServer(t)

	if m.CurrentCompany != "ActiveCorp" {
		t.Errorf("expected the most recently touched PROCESSING job (ActiveCorp), got %q", m.CurrentCompany)
	}
	if m.CurrentSince == "" {
		t.Error("expected current_since to be populated")
	}

	expected := recent.Local().Format("3:04:05 PM")
	if m.CurrentSince != expected {
		t.Errorf("expected current_since to be converted to local time (%q), got %q - a UTC-stored timestamp must not be displayed as-is", expected, m.CurrentSince)
	}
}

func TestServeMetrics_LastSkippedAndFailed_HaveHumanReadableReasons(t *testing.T) {
	setupTestDB(t)

	now := time.Now()
	db.Exec("INSERT INTO job_funnel (url, company_name, job_title, status, last_updated) VALUES (?, ?, ?, ?, ?)",
		"https://jobs.example.com/low-fit", "LowFitCorp", "Role A", "SKIPPED", now)
	db.Exec("INSERT INTO job_funnel (url, company_name, job_title, status, last_updated) VALUES (?, ?, ?, ?, ?)",
		"https://jobs.example.com/submit-failed", "SubmitFailCorp", "Role B", "FAILED_SUBMIT", now)

	m := fetchMetricsFromTestServer(t)

	if m.LastSkippedCompany != "LowFitCorp" || m.LastSkippedReason == "" {
		t.Errorf("expected a populated skip reason for LowFitCorp, got company=%q reason=%q", m.LastSkippedCompany, m.LastSkippedReason)
	}
	if m.LastFailedCompany != "SubmitFailCorp" || m.LastFailedReason == "" {
		t.Errorf("expected a populated failure reason for SubmitFailCorp, got company=%q reason=%q", m.LastFailedCompany, m.LastFailedReason)
	}
}

func TestServeMetrics_LastSkipped_ExplainsExcludedSource(t *testing.T) {
	setupTestDB(t)

	db.Exec("INSERT INTO job_funnel (url, company_name, job_title, status, status_reason, last_updated) VALUES (?, ?, ?, ?, ?, ?)",
		"https://jobs.example.com/excluded", "ExcludedCorp", "Role A", "SKIPPED", storage.SkippedReasonExcludedSource, time.Now())

	m := fetchMetricsFromTestServer(t)
	if m.LastSkippedReason != "Excluded ATS source — not eligible for automated submission" {
		t.Errorf("last skipped reason = %q", m.LastSkippedReason)
	}
}

func TestServeMetrics_LastSkipped_ExplainsDuplicateCooldown(t *testing.T) {
	setupTestDB(t)

	db.Exec("INSERT INTO job_funnel (url, company_name, job_title, status, status_reason, last_updated) VALUES (?, ?, ?, ?, ?, ?)",
		"https://jobs.example.com/duplicate", "Acme", "Senior Engineer", "SKIPPED", storage.SkippedReasonDuplicateCooldown, time.Now())

	m := fetchMetricsFromTestServer(t)
	if m.LastSkippedReason != "Equivalent recent application is within your configured cooldown" {
		t.Errorf("last skipped reason = %q", m.LastSkippedReason)
	}
}

// TestServeMetrics_LastSkipped_ExcludesBlockedCaptcha is a regression test
// for bugs.md #55's investigation: the "last skipped" widget used to
// include BLOCKED_CAPTCHA rows, disagreeing with the Skipped tile's own
// count (which never did) -- now that BLOCKED_CAPTCHA has its own tile,
// "last skipped" must only ever reflect a genuine SKIPPED row.
func TestServeMetrics_LastSkipped_ExcludesBlockedCaptcha(t *testing.T) {
	setupTestDB(t)

	older := time.Now().Add(-time.Hour)
	newer := time.Now()
	db.Exec("INSERT INTO job_funnel (url, company_name, job_title, status, last_updated) VALUES (?, ?, ?, ?, ?)",
		"https://jobs.example.com/skipped", "SkippedCorp", "Role A", "SKIPPED", older)
	db.Exec("INSERT INTO job_funnel (url, company_name, job_title, status, last_updated) VALUES (?, ?, ?, ?, ?)",
		"https://jobs.example.com/blocked", "BlockedCorp", "Role B", "BLOCKED_CAPTCHA", newer)

	m := fetchMetricsFromTestServer(t)

	if m.LastSkippedCompany != "SkippedCorp" {
		t.Errorf("expected last-skipped to only ever reflect a genuine SKIPPED row, got company=%q (BlockedCorp is more recent but BLOCKED_CAPTCHA)", m.LastSkippedCompany)
	}
}

// TestServeMetrics_ProcessingTime_ComputedFromDiscoveredToTerminal covers
// the user's request to see how long each job actually sat in the pipeline
// (discovered_at to the terminal status), since the Manual Queue tile's raw
// count alone hid that some of these rows were discovered days before they
// were finally marked MANUAL_REQUIRED (confirmed live 2026-07-24: real rows
// spanned over 7 days from discovery to resolution).
func TestServeMetrics_ProcessingTime_ComputedFromDiscoveredToTerminal(t *testing.T) {
	setupTestDB(t)

	discovered := time.Date(2026, 7, 15, 0, 18, 32, 0, time.UTC)
	resolved := discovered.Add(7*24*time.Hour + 19*time.Hour + 22*time.Minute)

	db.Exec(`INSERT INTO job_funnel (url, company_name, job_title, status, last_updated, discovered_at)
		VALUES (?, ?, ?, ?, ?, ?)`,
		"https://jobs.example.com/manual", "SlowCorp", "DevOps Engineer", "MANUAL_REQUIRED", resolved, discovered)
	db.Exec(`INSERT INTO job_funnel (url, company_name, job_title, status, last_updated, discovered_at)
		VALUES (?, ?, ?, ?, ?, ?)`,
		"https://jobs.example.com/skipped", "QuickCorp", "Engineer", "SKIPPED", discovered.Add(90*time.Second), discovered)

	m := fetchMetricsFromTestServer(t)

	if m.LastManualProcessingTime != "7d 19h" {
		t.Errorf("expected last manual processing time %q, got %q", "7d 19h", m.LastManualProcessingTime)
	}
	if m.LastSkippedProcessingTime != "1m" {
		t.Errorf("expected last skipped processing time %q, got %q", "1m", m.LastSkippedProcessingTime)
	}
}

// TestServeMetrics_ProcessingTime_EmptyWhenDiscoveredAtMissing guards
// against a nil-pointer/garbage-duration when discovered_at was never
// backfilled for an older row (the column was added after some rows
// already existed) - processing time must be omitted, not shown as a
// bogus multi-decade duration.
func TestServeMetrics_ProcessingTime_EmptyWhenDiscoveredAtMissing(t *testing.T) {
	setupTestDB(t)

	db.Exec(`INSERT INTO job_funnel (url, company_name, job_title, status, last_updated)
		VALUES (?, ?, ?, ?, ?)`,
		"https://jobs.example.com/no-discovered-at", "LegacyCorp", "Engineer", "FAILED_SUBMIT", time.Now())

	m := fetchMetricsFromTestServer(t)

	if m.LastFailedProcessingTime != "" {
		t.Errorf("expected empty processing time when discovered_at is NULL, got %q", m.LastFailedProcessingTime)
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{30 * time.Second, "under a minute"},
		{5 * time.Minute, "5m"},
		{2*time.Hour + 15*time.Minute, "2h 15m"},
		{3*24*time.Hour + 4*time.Hour, "3d 4h"},
	}
	for _, tt := range tests {
		if got := formatDuration(tt.d); got != tt.want {
			t.Errorf("formatDuration(%v) = %q, want %q", tt.d, got, tt.want)
		}
	}
}

// TestUIDistEmbed_ContainsBuiltAssets guards bug #436. `//go:embed ui/dist`
// is a compile-time dependency on build output that .gitignore used to
// exclude, so a fresh clone could not build the project at all. Committing
// dist/ fixes that, but it introduces the opposite failure mode: a dist/
// reduced to a placeholder (or committed before `npm run build` ran) still
// compiles and still serves 200s, just with no dashboard in them. This test
// asserts the embedded tree actually holds a built bundle.
func TestUIDistEmbed_ContainsBuiltAssets(t *testing.T) {
	if _, err := uiDistFS.ReadFile("ui/dist/index.html"); err != nil {
		t.Fatalf("embedded UI is missing index.html -- run `npm run build` in cmd/dashboard/ui and commit dist/: %v", err)
	}

	entries, err := uiDistFS.ReadDir("ui/dist/assets")
	if err != nil {
		t.Fatalf("embedded UI has no assets directory -- dist/ was not built: %v", err)
	}

	var hasJS, hasCSS bool
	for _, entry := range entries {
		switch {
		case strings.HasSuffix(entry.Name(), ".js"):
			hasJS = true
		case strings.HasSuffix(entry.Name(), ".css"):
			hasCSS = true
		}
	}
	if !hasJS || !hasCSS {
		t.Errorf("embedded UI assets are incomplete (js=%v css=%v); dist/ does not hold a real Vite build", hasJS, hasCSS)
	}
}

// readBuiltUIBundle returns the embedded, built JS bundle as a string. The
// two tests below assert on rendered markup, and the only artifact that
// proves what actually reaches a browser is the bundle the binary embeds --
// asserting on src/App.tsx instead would pass while dist/ still held a
// pre-change build, which is exactly the trap TestUIDistEmbed_
// ContainsBuiltAssets already guards one layer up.
func readBuiltUIBundle(t *testing.T) string {
	t.Helper()

	entries, err := uiDistFS.ReadDir("ui/dist/assets")
	if err != nil {
		t.Fatalf("embedded UI has no assets directory -- dist/ was not built: %v", err)
	}
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".js") {
			data, err := uiDistFS.ReadFile("ui/dist/assets/" + entry.Name())
			if err != nil {
				t.Fatalf("failed to read embedded bundle %s: %v", entry.Name(), err)
			}
			return string(data)
		}
	}
	t.Fatal("embedded UI has no .js bundle -- run `npm run build` in cmd/dashboard/ui and commit dist/")
	return ""
}

// TestUIDistEmbed_RendersEveryServedMetricsField is the direct regression
// guard for bug #437, and it is deliberately written against the *general*
// failure rather than the specific fields that went missing.
//
// #426 rewrote the dashboard as a React app that rendered six count tiles.
// The Go API was left untouched, so it kept computing and serving
// by_source, by_variant and interview_rate_pct on every poll -- improvement
// #15's and improvement #13's entire user-facing surface -- to a UI that
// referenced none of them. Everything built, vetted, tested and served
// 200s. Two separately-shipped, separately-Done improvements became
// unreachable in one commit and nothing failed.
//
// The lesson recorded in bugs.md is that a rewrite's Done note describes
// what was added and never what was dropped, and that for an API-backed
// surface the response struct is a ready-made checklist of what the old
// view consumed. This test makes that checklist executable: every field the
// API bothers to serve must appear somewhere in the bundle that is supposed
// to display it. It cannot verify the fields are rendered *well*, but it
// fails loudly the moment one stops being referenced at all.
func TestUIDistEmbed_RendersEveryServedMetricsField(t *testing.T) {
	bundle := readBuiltUIBundle(t)

	// Every JSON field serveMetrics populates that exists to be looked at.
	// Add to this list whenever the Metrics struct grows a user-facing
	// field; a field served but never listed here is a field nobody has
	// confirmed anyone can see.
	servedFields := []string{
		"discovered", "processing", "skipped", "applied", "failed",
		// The four #451 added so the Failed/Manual tiles can caption
		// whichever status(es) in their pair actually contributed.
		"failed_score", "failed_submit", "manual_required_only", "awaiting_review",
		"manual_required", "blocked_captcha", "invalid_url",
		"last_applied_company", "last_applied_title", "last_applied_at",
		"last_applied_processing_time",
		"last_skipped_company", "last_skipped_reason",
		"last_failed_company", "last_failed_reason",
		"last_manual_company", "last_manual_reason",
		"status_legend",
		"discovery_last_finished_at", "discovery_new_eligible", "discovery_error_class",
		// The three #426 deleted and #437 restored.
		"total_applied_tracked", "interviews", "rejections",
		"interview_rate_pct", "by_source", "by_variant",
	}

	for _, field := range servedFields {
		if !strings.Contains(bundle, field) {
			t.Errorf("the API serves %q but the built UI bundle never references it -- "+
				"it is computed on every poll and rendered nowhere (bug #437's exact failure mode)", field)
		}
	}
}

// TestUIDistEmbed_KeepsAccessibleTableSemantics guards the other half of bug
// #437. Improvement #34 ("Make the local dashboard accessible and
// self-contained") shipped two <caption> elements and twelve scope="col"
// headers; #426's rewrite deleted the template holding them, so that work
// was silently reverted while still reading Done in improvements.md.
//
// Accessibility markup is uniquely prone to this: nothing renders
// differently without it, no test that checks values notices, and the only
// people who find out are the ones who cannot use the page.
func TestUIDistEmbed_KeepsAccessibleTableSemantics(t *testing.T) {
	bundle := readBuiltUIBundle(t)

	if !strings.Contains(bundle, "caption") {
		t.Error("built UI bundle has no caption element -- improvement #34's table semantics were dropped again")
	}

	// Minified JSX emits props as scope:`col` (or scope:"col"/scope:'col'
	// depending on the minifier), so match the prop and its value together
	// with the quoting left open. Checking for a bare "col" or "row"
	// substring is worthless here: React's own bundled source contains
	// both, so such an assertion passes even against a bundle with no
	// tables in it at all -- verified against the pre-#437 bundle, where
	// only the caption check failed.
	scopeValue := regexp.MustCompile(`scope:\s*["'` + "`" + `](col|row)["'` + "`" + `]`)
	found := map[string]bool{}
	for _, match := range scopeValue.FindAllStringSubmatch(bundle, -1) {
		found[match[1]] = true
	}
	if !found["col"] {
		t.Error(`built UI bundle has no scope="col" header -- improvement #34's column semantics were dropped again`)
	}
	if !found["row"] {
		t.Error(`built UI bundle has no scope="row" header -- conversion rows announce as bare numbers without it`)
	}

	// Both conversion tables must actually exist. Their captions are the
	// least ambiguous proof, and they double as the check that the tables
	// themselves survived rather than just their styling.
	for _, caption := range []string{
		"Conversion by Platform",
		"Conversion by Cover-Letter Tone Variant",
	} {
		if !strings.Contains(bundle, caption) {
			t.Errorf("built UI bundle is missing the %q table -- improvement #15/#13's surface was deleted again (bug #437)", caption)
		}
	}

	if !strings.Contains(bundle, "Interview Rate") {
		t.Error("built UI bundle is missing the Interview Rate tile -- interview_rate_pct is served and shown nowhere")
	}
}

// TestServeMetrics_ConversionStats_CountsOutcomesAcrossTrackedStatuses
// covers the aggregate conversion numbers, which had no test at all on the
// dashboard side despite pkg/storage's mirror implementation carrying three
// (TestGetConversionStats and friends). Found while restoring the UI that
// consumes them for bug #437.
func TestServeMetrics_ConversionStats_CountsOutcomesAcrossTrackedStatuses(t *testing.T) {
	setupTestDB(t)

	// Four tracked applications: one interview, one rejection, two still
	// awaiting an outcome. DISCOVERED must not be counted -- a job that was
	// never applied to is not a denominator.
	db.Exec("INSERT INTO job_funnel (url, status) VALUES (?, ?)", "https://x.greenhouse.io/1", "INTERVIEW_REQUESTED")
	db.Exec("INSERT INTO job_funnel (url, status) VALUES (?, ?)", "https://x.greenhouse.io/2", "REJECTED")
	db.Exec("INSERT INTO job_funnel (url, status) VALUES (?, ?)", "https://x.greenhouse.io/3", "APPLIED")
	db.Exec("INSERT INTO job_funnel (url, status) VALUES (?, ?)", "https://x.greenhouse.io/4", "APPLIED")
	db.Exec("INSERT INTO job_funnel (url, status) VALUES (?, ?)", "https://x.greenhouse.io/5", "DISCOVERED")

	m := fetchMetricsFromTestServer(t)

	if m.TotalApplied != 4 {
		t.Errorf("expected 4 tracked applications, got %d (DISCOVERED must not count)", m.TotalApplied)
	}
	if m.Interviews != 1 || m.Rejections != 1 {
		t.Errorf("expected 1 interview and 1 rejection, got %d and %d", m.Interviews, m.Rejections)
	}
	if m.InterviewRatePct != "25.0%" {
		t.Errorf("expected interview rate 25.0%%, got %q", m.InterviewRatePct)
	}
}

// TestServeMetrics_ConversionStats_EmptyWhenNothingTracked pins the
// omitempty behaviour the restored UI depends on: with no tracked
// applications there is no rate to show, and the tile must fall back to an
// em dash rather than claim a 0% interview rate the data does not support.
func TestServeMetrics_ConversionStats_EmptyWhenNothingTracked(t *testing.T) {
	setupTestDB(t)

	db.Exec("INSERT INTO job_funnel (url, status) VALUES (?, ?)", "https://x.greenhouse.io/1", "DISCOVERED")

	m := fetchMetricsFromTestServer(t)

	if m.TotalApplied != 0 {
		t.Errorf("expected 0 tracked applications, got %d", m.TotalApplied)
	}
	if m.InterviewRatePct != "" {
		t.Errorf("expected no interview rate when nothing is tracked, got %q", m.InterviewRatePct)
	}
	if len(m.BySource) != 0 || len(m.ByVariant) != 0 {
		t.Errorf("expected empty breakdowns, got %d source rows and %d variant rows", len(m.BySource), len(m.ByVariant))
	}
}

// TestServeMetrics_ConversionBySource_GroupsByATSPlatform covers the
// per-platform breakdown (improvement #15) that bug #437 restored to the UI.
func TestServeMetrics_ConversionBySource_GroupsByATSPlatform(t *testing.T) {
	setupTestDB(t)

	db.Exec("INSERT INTO job_funnel (url, status) VALUES (?, ?)", "https://boards.greenhouse.io/a", "INTERVIEW_REQUESTED")
	db.Exec("INSERT INTO job_funnel (url, status) VALUES (?, ?)", "https://boards.greenhouse.io/b", "REJECTED")
	db.Exec("INSERT INTO job_funnel (url, status) VALUES (?, ?)", "https://jobs.lever.co/c", "APPLIED")
	db.Exec("INSERT INTO job_funnel (url, status) VALUES (?, ?)", "https://example.com/d", "APPLIED")

	m := fetchMetricsFromTestServer(t)

	bySource := map[string]SourceConversionStat{}
	for _, s := range m.BySource {
		bySource[s.Source] = s
	}

	gh, ok := bySource["Greenhouse"]
	if !ok {
		t.Fatalf("expected a Greenhouse row, got %+v", m.BySource)
	}
	if gh.TotalApplied != 2 || gh.Interviews != 1 || gh.Rejections != 1 {
		t.Errorf("unexpected Greenhouse row: %+v", gh)
	}
	// Pending is "applied, no outcome yet" -- both Greenhouse rows already
	// have an outcome, so none is pending.
	if gh.Pending != 0 {
		t.Errorf("expected 0 pending Greenhouse applications, got %d", gh.Pending)
	}
	if gh.InterviewRate != "50.0%" {
		t.Errorf("expected Greenhouse interview rate 50.0%%, got %q", gh.InterviewRate)
	}

	lever, ok := bySource["Lever"]
	if !ok {
		t.Fatalf("expected a Lever row, got %+v", m.BySource)
	}
	if lever.TotalApplied != 1 || lever.Pending != 1 {
		t.Errorf("expected Lever to show 1 applied and 1 pending, got %+v", lever)
	}
	// A URL matching no known ATS still has to appear somewhere, or its
	// applications vanish from the breakdown entirely.
	if _, ok := bySource["Other"]; !ok {
		t.Errorf("expected unrecognised ATS URLs to fall into an Other row, got %+v", m.BySource)
	}
}

// TestServeMetrics_ConversionByVariant_GroupsByToneAndLabelsUnset covers the
// per-cover-letter-tone breakdown (improvement #13) restored by bug #437.
// The empty-tone case matters: the agent writes tone_variant only for jobs
// that went through variant selection, so real databases hold a mix, and an
// unlabelled group would render as a blank row header.
func TestServeMetrics_ConversionByVariant_GroupsByToneAndLabelsUnset(t *testing.T) {
	setupTestDB(t)

	db.Exec("INSERT INTO job_funnel (url, status, tone_variant) VALUES (?, ?, ?)", "https://a.example.com", "INTERVIEW_REQUESTED", "formal")
	db.Exec("INSERT INTO job_funnel (url, status, tone_variant) VALUES (?, ?, ?)", "https://b.example.com", "APPLIED", "formal")
	db.Exec("INSERT INTO job_funnel (url, status, tone_variant) VALUES (?, ?, ?)", "https://c.example.com", "REJECTED", "")
	db.Exec("INSERT INTO job_funnel (url, status) VALUES (?, ?)", "https://d.example.com", "APPLIED")

	m := fetchMetricsFromTestServer(t)

	byVariant := map[string]VariantConversionStat{}
	for _, v := range m.ByVariant {
		byVariant[v.Variant] = v
	}

	formal, ok := byVariant["formal"]
	if !ok {
		t.Fatalf("expected a formal variant row, got %+v", m.ByVariant)
	}
	if formal.TotalApplied != 2 || formal.Interviews != 1 || formal.Pending != 1 {
		t.Errorf("unexpected formal row: %+v", formal)
	}
	if formal.InterviewRate != "50.0%" {
		t.Errorf("expected formal interview rate 50.0%%, got %q", formal.InterviewRate)
	}

	// Both the empty string and SQL NULL must land in one labelled bucket.
	unspecified, ok := byVariant["unspecified"]
	if !ok {
		t.Fatalf("expected empty and NULL tone_variant to group as 'unspecified', got %+v", m.ByVariant)
	}
	if unspecified.TotalApplied != 2 {
		t.Errorf("expected 2 unspecified applications (one empty string, one NULL), got %d", unspecified.TotalApplied)
	}
}

func TestStatusReason_KnownAndUnknownCodes(t *testing.T) {
	tests := map[string]bool{ // status -> expect a specific (non-passthrough) reason
		"SKIPPED":         true,
		"BLOCKED_CAPTCHA": true,
		"FAILED_SCORE":    true,
		"FAILED_SUBMIT":   true,
		"MANUAL_REQUIRED": true,
		"AWAITING_REVIEW": true,
		"INVALID_URL":     true,
	}
	for status, expectMapped := range tests {
		reason := statusReason(status)
		if expectMapped && reason == status {
			t.Errorf("expected statusReason(%q) to return a human-readable reason, got the raw status back", status)
		}
	}
	// Unknown codes fall back to the raw status rather than an empty string.
	if statusReason("SOME_FUTURE_STATUS") != "SOME_FUTURE_STATUS" {
		t.Error("expected statusReason to fall back to the raw status for unknown codes")
	}
}

// The three tests below exist because TestStatusReason_KnownAndUnknownCodes
// above cannot catch bug #435: it tests statusReason in isolation and asserts
// nothing about whether anything calls it, so it passed the whole time four of
// the seven arms were unreachable in production. Same shape as bug #76 -- a
// shipped fix is not a working fix until something observes it firing -- so
// these assert on the served payload instead of on the function.

func TestServeMetrics_LastManual_DistinguishesCopilotFromAccountGated(t *testing.T) {
	awaiting := time.Now()
	gated := awaiting.Add(-time.Hour)

	// Both statuses share the one Manual Queue card, and they ask completely
	// different things of the user: AWAITING_REVIEW needs a click on an
	// already-filled form, MANUAL_REQUIRED needs the whole application done by
	// hand. Before #435 the card rendered them identically.
	for _, tc := range []struct {
		name        string
		recentURL   string
		recentCo    string
		recentAt    time.Time
		recentState string
		wantReason  string
	}{
		{"copilot", "https://jobs.example.com/copilot", "CopilotCorp", awaiting, "AWAITING_REVIEW", statusReason("AWAITING_REVIEW")},
		{"account gated", "https://jobs.example.com/gated", "GatedCorp", awaiting, "MANUAL_REQUIRED", statusReason("MANUAL_REQUIRED")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			setupTestDB(t)
			// An older row in the *other* manual status, to prove the reason
			// tracks the row actually selected rather than a fixed string.
			other := "MANUAL_REQUIRED"
			if tc.recentState == "MANUAL_REQUIRED" {
				other = "AWAITING_REVIEW"
			}
			db.Exec("INSERT INTO job_funnel (url, company_name, job_title, status, last_updated) VALUES (?, ?, ?, ?, ?)",
				"https://jobs.example.com/older", "OlderCorp", "Role Z", other, gated)
			db.Exec("INSERT INTO job_funnel (url, company_name, job_title, status, last_updated) VALUES (?, ?, ?, ?, ?)",
				tc.recentURL, tc.recentCo, "Role A", tc.recentState, tc.recentAt)

			m := fetchMetricsFromTestServer(t)

			if m.LastManualCompany != tc.recentCo {
				t.Fatalf("expected the most recent manual row (%s), got company=%q", tc.recentCo, m.LastManualCompany)
			}
			if m.LastManualReason != tc.wantReason {
				t.Errorf("expected last-manual reason %q for a %s row, got %q", tc.wantReason, tc.recentState, m.LastManualReason)
			}
			if m.LastManualReason == tc.recentState {
				t.Errorf("last-manual reason is the raw status %q, so statusReason has no arm for it", tc.recentState)
			}
		})
	}
}

// TestServeMetrics_StatusLegend_ExplainsEveryCountedOnlyStatus covers the other
// half of #435: BLOCKED_CAPTCHA and INVALID_URL have count tiles but no "last
// job" card, so the legend is the only thing that can explain them.
func TestServeMetrics_StatusLegend_ExplainsEveryCountedOnlyStatus(t *testing.T) {
	setupTestDB(t)

	m := fetchMetricsFromTestServer(t)

	if len(m.StatusLegend) == 0 {
		t.Fatal("expected the metrics payload to carry a status legend")
	}
	for _, status := range explainedStatuses {
		reason, ok := m.StatusLegend[status]
		if !ok {
			t.Errorf("status %q is surfaced to the user but absent from the served legend", status)
			continue
		}
		if reason == "" || reason == status {
			t.Errorf("legend entry for %q is not a human-readable reason (got %q), so statusReason has no arm for it", status, reason)
		}
	}
}

// TestExplainedStatuses_CoverEveryStatusReasonArm is the guard against #435
// recurring in the opposite direction: an arm added to statusReason but left
// out of explainedStatuses is an arm nothing renders. Keep the two in step.
func TestExplainedStatuses_CoverEveryStatusReasonArm(t *testing.T) {
	arms := []string{
		"SKIPPED",
		"BLOCKED_CAPTCHA",
		"FAILED_SCORE",
		"FAILED_SUBMIT",
		"MANUAL_REQUIRED",
		"AWAITING_REVIEW",
		"INVALID_URL",
		"RETRY_EXHAUSTED",
	}
	legend := statusLegend()
	for _, status := range arms {
		if _, ok := legend[status]; !ok {
			t.Errorf("statusReason has an arm for %q but explainedStatuses omits it, so nothing renders it", status)
		}
	}
	if len(legend) != len(arms) {
		t.Errorf("explainedStatuses has %d entries but statusReason has %d mapped arms; a status is surfaced with no explanation or explained with nothing to render it", len(legend), len(arms))
	}
}

// The three tests below were deleted by improvement #426's React rewrite
// (commit 0028b2f) along with the Go template they shared a file with, and are
// restored verbatim from 0028b2f^ here -- see bug #438. They cover
// normalizeDashboardAddress, dashboardExposureWarning and newDashboardServer,
// none of which the rewrite touched, and all of which implement the security
// boundary bug #126 established: this is an unauthenticated server over real
// application data, so it binds loopback-only by default and must warn loudly
// when configured otherwise. That they pass unmodified against today's code is
// the evidence the deletion was collateral damage, not an intentional change.
//
// TestServeFavicon was deliberately not restored: it asserted a
// cmd/dashboard/favicon.png that the rewrite legitimately removed, and the
// Vite bundle's favicon is covered by TestUIDistEmbed_ContainsBuiltAssets.
func TestNormalizeDashboardAddress(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		want    string
		wantErr bool
	}{
		{
			name: "empty uses loopback default",
			raw:  "",
			want: defaultDashboardAddress,
		},
		{
			name: "configured loopback",
			raw:  "127.0.0.1:9090",
			want: "127.0.0.1:9090",
		},
		{
			name: "configured IPv6 loopback",
			raw:  "[::1]:9090",
			want: "[::1]:9090",
		},
		{
			name: "configured wildcard",
			raw:  ":9090",
			want: ":9090",
		},
		{
			name:    "missing port",
			raw:     "127.0.0.1",
			wantErr: true,
		},
		{
			name:    "non-numeric port",
			raw:     "127.0.0.1:http",
			wantErr: true,
		},
		{
			name:    "port out of range",
			raw:     "127.0.0.1:70000",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizeDashboardAddress(tt.raw)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("normalizeDashboardAddress(%q) returned no error", tt.raw)
				}
				return
			}
			if err != nil {
				t.Fatalf("normalizeDashboardAddress(%q): %v", tt.raw, err)
			}
			if got != tt.want {
				t.Fatalf("normalizeDashboardAddress(%q) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
}

func TestDashboardExposureWarning(t *testing.T) {
	tests := []struct {
		address  string
		wantWarn bool
	}{
		{address: "127.0.0.1:8080"},
		{address: "localhost:8080"},
		{address: "[::1]:8080"},
		{address: ":8080", wantWarn: true},
		{address: "0.0.0.0:8080", wantWarn: true},
		{address: "192.168.1.20:8080", wantWarn: true},
	}

	for _, tt := range tests {
		t.Run(tt.address, func(t *testing.T) {
			got := dashboardExposureWarning(tt.address)
			if tt.wantWarn && got == "" {
				t.Fatalf("dashboardExposureWarning(%q) returned no warning", tt.address)
			}
			if !tt.wantWarn && got != "" {
				t.Fatalf("dashboardExposureWarning(%q) = %q, want no warning", tt.address, got)
			}
		})
	}
}

// The tests below cover bug #445. The agent start/stop endpoints validated
// nothing but the HTTP method, so a cross-origin
// `fetch('http://127.0.0.1:8080/api/agent/start', {method:'POST',
// mode:'no-cors'})` from *any* page open in *any* tab -- including an ad
// iframe -- was a CORS simple request the browser sent and the server acted
// on. CORS blocks reading the response, not the side effect: by then the
// agent is running and submitting real applications with the user's real PII.
// The loopback bind from bug #126 is worth nothing against this, because the
// request comes from the user's own browser on the same machine.
//
// These exercise requireSameOrigin against a sentinel handler rather than
// serveAgentStart itself, deliberately: a test that reached the real handler
// on a POST would launch ./career_agent_bin or run pkill. The sentinel
// records whether it was reached, so every rejection case below genuinely
// fails if the guard is deleted -- with no guard, reached becomes true and
// the status stays 200 instead of 403.
func TestRequireSameOrigin(t *testing.T) {
	tests := []struct {
		name    string
		host    string
		headers map[string]string
		want    int
	}{
		{
			name:    "Sec-Fetch-Site same-origin is our own dashboard page",
			host:    "127.0.0.1:8080",
			headers: map[string]string{"Sec-Fetch-Site": "same-origin"},
			want:    http.StatusOK,
		},
		{
			name:    "Sec-Fetch-Site none is a user-initiated navigation",
			host:    "127.0.0.1:8080",
			headers: map[string]string{"Sec-Fetch-Site": "none"},
			want:    http.StatusOK,
		},
		{
			name:    "Sec-Fetch-Site cross-site is the attack in bug #445",
			host:    "127.0.0.1:8080",
			headers: map[string]string{"Sec-Fetch-Site": "cross-site"},
			want:    http.StatusForbidden,
		},
		{
			name:    "Sec-Fetch-Site same-site still means another page drove it",
			host:    "127.0.0.1:8080",
			headers: map[string]string{"Sec-Fetch-Site": "same-site"},
			want:    http.StatusForbidden,
		},
		{
			// A browser too old to send fetch metadata still sends Origin
			// on a cross-origin POST, so Origin is the fallback.
			name:    "no Sec-Fetch-Site, Origin matching Host",
			host:    "127.0.0.1:8080",
			headers: map[string]string{"Origin": "http://127.0.0.1:8080"},
			want:    http.StatusOK,
		},
		{
			name:    "no Sec-Fetch-Site, Origin on a different host",
			host:    "127.0.0.1:8080",
			headers: map[string]string{"Origin": "https://evil.example.com"},
			want:    http.StatusForbidden,
		},
		{
			// Same host, different port is a different origin.
			name:    "no Sec-Fetch-Site, Origin on a different port",
			host:    "127.0.0.1:8080",
			headers: map[string]string{"Origin": "http://127.0.0.1:9090"},
			want:    http.StatusForbidden,
		},
		{
			name:    "no Sec-Fetch-Site, no Origin, Referer matching Host",
			host:    "127.0.0.1:8080",
			headers: map[string]string{"Referer": "http://127.0.0.1:8080/index.html"},
			want:    http.StatusOK,
		},
		{
			name:    "no Sec-Fetch-Site, no Origin, Referer on a different host",
			host:    "127.0.0.1:8080",
			headers: map[string]string{"Referer": "https://evil.example.com/attack.html"},
			want:    http.StatusForbidden,
		},
		{
			// An Origin that does not parse as a URL must fail closed
			// rather than fall through to the no-headers allowance.
			name:    "malformed Origin that does not parse",
			host:    "127.0.0.1:8080",
			headers: map[string]string{"Origin": "://not a url"},
			want:    http.StatusForbidden,
		},
		{
			// A sandboxed iframe sends a literal "null" Origin; it parses
			// cleanly but carries no host, so it must not match either.
			name:    "null Origin from a sandboxed iframe",
			host:    "127.0.0.1:8080",
			headers: map[string]string{"Origin": "null"},
			want:    http.StatusForbidden,
		},
		{
			// curl, the project's own scripts, and anything else that is
			// not a browser page send none of the three headers. Blocking
			// that shape breaks scripted use without closing the
			// browser-driven attack, so it is allowed through.
			name: "no origin-ish headers at all is the curl case",
			host: "127.0.0.1:8080",
			want: http.StatusOK,
		},
		{
			// Sec-Fetch-Site wins outright when present: it cannot be set
			// by JavaScript, so a forged Origin must not rescue a
			// cross-site request.
			name: "cross-site Sec-Fetch-Site beats a spoofed matching Origin",
			host: "127.0.0.1:8080",
			headers: map[string]string{
				"Sec-Fetch-Site": "cross-site",
				"Origin":         "http://127.0.0.1:8080",
			},
			want: http.StatusForbidden,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reached := false
			guarded := requireSameOrigin(func(w http.ResponseWriter, r *http.Request) {
				reached = true
				w.WriteHeader(http.StatusOK)
			})

			req := httptest.NewRequest(http.MethodPost, "/api/agent/start", nil)
			req.Host = tt.host
			for name, value := range tt.headers {
				req.Header.Set(name, value)
			}
			rec := httptest.NewRecorder()
			guarded(rec, req)

			if rec.Code != tt.want {
				t.Errorf("status = %d, want %d", rec.Code, tt.want)
			}
			wantReached := tt.want == http.StatusOK
			if reached != wantReached {
				t.Errorf("wrapped handler reached = %v, want %v -- a rejected request must never reach the handler that launches the agent", reached, wantReached)
			}
		})
	}
}

// TestAgentHandlers_RejectNonPost keeps the pre-existing method check honest
// now that the origin guard sits in front of it. GET is safe to run against
// the real handlers: the method check is the first statement in both, so
// neither exec.Command is ever reached.
func TestAgentHandlers_RejectNonPost(t *testing.T) {
	handlers := map[string]http.HandlerFunc{
		"/api/agent/start": serveAgentStart,
		"/api/agent/stop":  serveAgentStop,
	}
	for path, handler := range handlers {
		t.Run(path, func(t *testing.T) {
			// Wrapped exactly as main() registers it, so this also proves
			// the guard lets a legitimate same-origin request through to
			// the handler rather than swallowing it.
			guarded := requireSameOrigin(handler)

			req := httptest.NewRequest(http.MethodGet, path, nil)
			req.Host = "127.0.0.1:8080"
			req.Header.Set("Sec-Fetch-Site", "same-origin")
			rec := httptest.NewRecorder()
			guarded(rec, req)

			if rec.Code != http.StatusMethodNotAllowed {
				t.Errorf("GET %s = %d, want %d", path, rec.Code, http.StatusMethodNotAllowed)
			}
		})
	}
}

// TestAgentHandlers_CrossSiteGetIsForbiddenBeforeTheMethodCheck proves the
// ordering: the guard runs first, so a cross-origin caller cannot even
// distinguish a wrong method from a blocked origin.
func TestAgentHandlers_CrossSiteGetIsForbiddenBeforeTheMethodCheck(t *testing.T) {
	guarded := requireSameOrigin(serveAgentStart)

	req := httptest.NewRequest(http.MethodGet, "/api/agent/start", nil)
	req.Host = "127.0.0.1:8080"
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	rec := httptest.NewRecorder()
	guarded(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("cross-site GET = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestNewDashboardServerUsesAddressHandlerAndTimeouts(t *testing.T) {
	handler := http.NewServeMux()
	server := newDashboardServer("127.0.0.1:9090", handler)

	if server.Addr != "127.0.0.1:9090" {
		t.Fatalf("server address = %q, want 127.0.0.1:9090", server.Addr)
	}
	if server.Handler != handler {
		t.Fatal("server does not use the configured handler")
	}
	if server.ReadHeaderTimeout != 5*time.Second {
		t.Fatalf("ReadHeaderTimeout = %v, want 5s", server.ReadHeaderTimeout)
	}
	if server.ReadTimeout != 15*time.Second {
		t.Fatalf("ReadTimeout = %v, want 15s", server.ReadTimeout)
	}
	if server.WriteTimeout != 30*time.Second {
		t.Fatalf("WriteTimeout = %v, want 30s", server.WriteTimeout)
	}
	if server.IdleTimeout != 60*time.Second {
		t.Fatalf("IdleTimeout = %v, want 60s", server.IdleTimeout)
	}
}

// TestServeMetrics_QueryFailure_Returns500NotZeroedJSON is the regression
// test for bug #452: every g.Go closure in serveMetrics used to log its
// error and unconditionally return nil, and the errgroup's result was
// discarded (`_ = g.Wait()`), so a genuine query failure -- not an empty
// table, a real failure -- always answered 200 OK carrying whatever
// zero/stale values the failed scans left behind. A user watching the
// dashboard during an outage saw a confident "nothing has happened yet"
// instead of an error.
func TestServeMetrics_QueryFailure_Returns500NotZeroedJSON(t *testing.T) {
	setupTestDB(t)

	// Drop the table the basic-counts query depends on so the query
	// genuinely fails (e.g. "no such table"), which is a different failure
	// shape than sql.ErrNoRows on an empty-but-present table.
	if _, err := db.Exec("DROP TABLE job_funnel"); err != nil {
		t.Fatalf("failed to drop table: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/metrics", nil)
	rec := httptest.NewRecorder()
	serveMetrics(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 on a genuine query failure, got %d with body %q", rec.Code, rec.Body.String())
	}

	// The bug's exact failure shape was a 200 response carrying a fully
	// decodable, all-zero Metrics -- confirm the response is not that shape.
	var m Metrics
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err == nil {
		t.Errorf("expected a non-JSON error body, got a decodable Metrics: %+v", m)
	}
}

// TestServeMetrics_EmptyTable_StillReturns200 pins the legitimate-empty-state
// side of bug #452's fix: an empty (but present) table must still answer 200
// with zero values, not be mistaken for the query-failure case above.
func TestServeMetrics_EmptyTable_StillReturns200(t *testing.T) {
	setupTestDB(t)

	req := httptest.NewRequest(http.MethodGet, "/api/metrics", nil)
	rec := httptest.NewRecorder()
	serveMetrics(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 on an empty-but-present table, got %d with body %q", rec.Code, rec.Body.String())
	}

	var m Metrics
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
		t.Fatalf("expected a decodable Metrics body, got error: %v (body %q)", err, rec.Body.String())
	}
	if m.Discovered != 0 || m.Applied != 0 {
		t.Errorf("expected all-zero counts on an empty table, got %+v", m)
	}
}

// fakeConversionRows is a hand-rolled conversionRows for exercising the
// mid-stream cursor-fault path directly. A real driver cannot be made to
// return a cursor error from Next() on demand against an in-memory sqlite
// database (bug #452's precedent -- dropping the table -- only reaches the
// top-level query failure, never a fault after rows have already started
// streaming), so this fake stands in for "the row(s) before the fault
// scanned fine, then the cursor broke" -- the exact ambiguity Next()
// returning false cannot resolve on its own per database/sql's contract.
type fakeConversionRows struct {
	values  [][]any
	i       int
	failErr error
}

func (f *fakeConversionRows) Next() bool {
	if f.i >= len(f.values) {
		return false
	}
	f.i++
	return true
}

func (f *fakeConversionRows) Scan(dest ...any) error {
	row := f.values[f.i-1]
	for i, d := range dest {
		switch v := d.(type) {
		case *string:
			*v = row[i].(string)
		case *int:
			*v = row[i].(int)
		default:
			return fmt.Errorf("fakeConversionRows: unsupported scan dest type %T", d)
		}
	}
	return nil
}

func (f *fakeConversionRows) Err() error {
	if f.i >= len(f.values) {
		return f.failErr
	}
	return nil
}

// TestScanSourceConversions_PropagatesRowsErr and its variant counterpart
// below pin bug #459: a cursor fault partway through the by-source or
// by-variant result stream must surface as an error, not as a silently
// truncated breakdown that renders as if it were complete. Mutation check:
// deleting scanSourceConversions'/scanVariantConversions's rows.Err() check
// makes these fail, since the loop above it already appended the one row
// that scanned fine before Next() returned false.
func TestScanSourceConversions_PropagatesRowsErr(t *testing.T) {
	rows := &fakeConversionRows{
		values:  [][]any{{"Greenhouse", 3, 1, 1, 1}},
		failErr: errors.New("cursor error: connection reset"),
	}

	got, err := scanSourceConversions(rows)
	if err == nil {
		t.Fatalf("expected an error from a mid-stream cursor fault, got nil with breakdown %+v", got)
	}
	if got != nil {
		t.Errorf("expected no partial breakdown on a cursor fault, got %+v", got)
	}
}

func TestScanSourceConversions_NoErrorOnCleanExhaustion(t *testing.T) {
	rows := &fakeConversionRows{
		values: [][]any{{"Greenhouse", 4, 2, 1, 1}},
	}

	got, err := scanSourceConversions(rows)
	if err != nil {
		t.Fatalf("unexpected error on clean exhaustion: %v", err)
	}
	if len(got) != 1 || got[0].Source != "Greenhouse" || got[0].InterviewRate != "50.0%" {
		t.Errorf("unexpected result: %+v", got)
	}
}

func TestScanVariantConversions_PropagatesRowsErr(t *testing.T) {
	rows := &fakeConversionRows{
		values:  [][]any{{"formal", 2, 1, 0, 1}},
		failErr: errors.New("cursor error: connection reset"),
	}

	got, err := scanVariantConversions(rows)
	if err == nil {
		t.Fatalf("expected an error from a mid-stream cursor fault, got nil with breakdown %+v", got)
	}
	if got != nil {
		t.Errorf("expected no partial breakdown on a cursor fault, got %+v", got)
	}
}

func TestScanVariantConversions_NoErrorOnCleanExhaustion(t *testing.T) {
	rows := &fakeConversionRows{
		values: [][]any{{"formal", 4, 1, 1, 2}},
	}

	got, err := scanVariantConversions(rows)
	if err != nil {
		t.Fatalf("unexpected error on clean exhaustion: %v", err)
	}
	if len(got) != 1 || got[0].Variant != "formal" || got[0].InterviewRate != "25.0%" {
		t.Errorf("unexpected result: %+v", got)
	}
}
