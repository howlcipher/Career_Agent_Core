package main

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

func setupTestDB(t *testing.T) {
	t.Helper()
	var err error
	db, err = sql.Open("sqlite", ":memory:")
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
		last_updated DATETIME,
		discovered_at DATETIME
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

func TestStatusReason_KnownAndUnknownCodes(t *testing.T) {
	tests := map[string]bool{ // status -> expect a specific (non-passthrough) reason
		"SKIPPED":         true,
		"BLOCKED_CAPTCHA": true,
		"FAILED_SCORE":    true,
		"FAILED_SUBMIT":   true,
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

func TestServeFavicon(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/favicon.png", nil)
	rec := httptest.NewRecorder()
	serveFavicon(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "image/png" {
		t.Errorf("expected Content-Type image/png, got %q", ct)
	}
	if rec.Body.Len() == 0 {
		t.Error("expected non-empty favicon body")
	}
}

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

func TestDashboardAccessibilitySemantics(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	serveDashboard(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	body := rec.Body.String()

	checks := []struct {
		name     string
		want     string
		mustHave bool
	}{
		{"Main Landmark", "<main>", true},
		{"Status Bar Aria Live", `class="status-bar" aria-live="polite"`, true},
		{"Table Caption", `<caption>Conversion by Platform</caption>`, true},
		{"Table Col Scope", `<th scope="col">Platform</th>`, true},
		{"Reduced Motion Media Query", `@media (prefers-reduced-motion: reduce)`, true},
		{"No Google Fonts", `fonts.googleapis.com`, false},
	}

	for _, tc := range checks {
		t.Run(tc.name, func(t *testing.T) {
			has := strings.Contains(body, tc.want)
			if tc.mustHave && !has {
				t.Errorf("expected HTML to contain %q, but it did not", tc.want)
			}
			if !tc.mustHave && has {
				t.Errorf("expected HTML to NOT contain %q, but it did", tc.want)
			}
		})
	}
}
