package main

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

func setupTestDB(t *testing.T) {
	t.Helper()
	var err error
	db, err = sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	schema := `
	CREATE TABLE job_funnel (
		url TEXT PRIMARY KEY,
		company_name TEXT,
		job_title TEXT,
		status TEXT,
		last_updated DATETIME
	);
	CREATE TABLE applied_jobs (
		company_name TEXT,
		job_title TEXT,
		url TEXT,
		applied_at DATETIME
	);`
	if _, err := db.Exec(schema); err != nil {
		t.Fatalf("failed to create schema: %v", err)
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

	m := fetchMetricsFromTestServer(t)

	if m.Discovered != 1 || m.Processing != 1 || m.Skipped != 1 || m.Applied != 1 || m.Failed != 1 {
		t.Errorf("unexpected counts: %+v", m)
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

func TestStatusReason_KnownAndUnknownCodes(t *testing.T) {
	tests := map[string]bool{ // status -> expect a specific (non-passthrough) reason
		"SKIPPED":         true,
		"BLOCKED_CAPTCHA": true,
		"FAILED_SCORE":    true,
		"FAILED_SUBMIT":   true,
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
