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
		status TEXT
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
