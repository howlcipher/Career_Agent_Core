package storage

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestGetQueuePlan(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "queue-plan-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	dbPath := filepath.Join(tempDir, "applications.db")
	t.Setenv("CAREER_AGENT_DB_PATH", dbPath)
	t.Setenv("CAREER_AGENT_PRIVATE_WORKSPACE_PATH", tempDir)

	if err := InitDB(); err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}
	defer CloseDB()

	// Insert test data
	now := time.Now()
	yesterday := now.Add(-24 * time.Hour)
	lastWeek := now.Add(-7 * 24 * time.Hour)

	db.Exec(`INSERT INTO job_funnel (url, company_name, job_title, status, fit_score, fit_similarity, discovered_at)
		VALUES ('https://lever.co/a', 'A', 'Role A', 'BLOCKED_CAPTCHA', 85, 0.85, ?)`, yesterday)

	// Has a dedup row
	db.Exec(`INSERT INTO job_funnel (url, company_name, job_title, status, fit_score, fit_similarity, discovered_at)
		VALUES ('https://lever.co/b', 'B', 'Role B', 'FAILED_SUBMIT', 80, 0.80, ?)`, lastWeek)
	db.Exec(`INSERT INTO applied_jobs (url, company_name, job_title, applied_at)
		VALUES ('https://lever.co/b', 'B', 'Role B', ?)`, now)

	// Has a scheme duplicate
	db.Exec(`INSERT INTO job_funnel (url, company_name, job_title, status, fit_score, fit_similarity, discovered_at)
		VALUES ('http://lever.co/c', 'C', 'Role C', 'BLOCKED_CAPTCHA', 90, 0.90, ?)`, yesterday)
	db.Exec(`INSERT INTO job_funnel (url, company_name, job_title, status, fit_score, fit_similarity, discovered_at)
		VALUES ('https://lever.co/c', 'C', 'Role C', 'APPLIED', 90, 0.90, ?)`, yesterday)

	// Unrelated status
	db.Exec(`INSERT INTO job_funnel (url, company_name, job_title, status, fit_score, fit_similarity, discovered_at)
		VALUES ('https://lever.co/d', 'D', 'Role D', 'DISCOVERED', 70, 0.70, ?)`, yesterday)

	// Null discovered_at (bug #453): should not abort the whole plan, and
	// should fall back to time.Now() instead of the zero value.
	db.Exec(`INSERT INTO job_funnel (url, company_name, job_title, status, fit_score, fit_similarity, discovered_at)
		VALUES ('https://indeed.com/e', 'E', 'Role E', 'BLOCKED_CAPTCHA', 75, 0.75, NULL)`)
	db.Exec(`INSERT INTO job_funnel (url, company_name, job_title, status, fit_score, fit_similarity, discovered_at)
		VALUES ('https://indeed.com/f', 'F', 'Role F', 'BLOCKED_CAPTCHA', 78, 0.78, ?)`, yesterday)

	t.Run("Empty cohort", func(t *testing.T) {
		plan, err := GetQueuePlan("%greenhouse%", "BLOCKED_CAPTCHA", false)
		if err != nil {
			t.Fatalf("GetQueuePlan failed: %v", err)
		}
		if plan.TotalCandidates != 0 {
			t.Errorf("expected 0 candidates, got %d", plan.TotalCandidates)
		}
	})

	t.Run("BLOCKED_CAPTCHA cohort", func(t *testing.T) {
		plan, err := GetQueuePlan("%lever.co%", "BLOCKED_CAPTCHA", false)
		if err != nil {
			t.Fatalf("GetQueuePlan failed: %v", err)
		}
		if plan.TotalCandidates != 2 {
			t.Fatalf("expected 2 candidates, got %d", plan.TotalCandidates)
		}

		var hasSchemeDup bool
		for _, c := range plan.Candidates {
			if c.OriginalURL == "http://lever.co/c" {
				hasSchemeDup = c.HasSchemeDup
			}
		}
		if !hasSchemeDup {
			t.Errorf("expected http://lever.co/c to have a scheme duplicate")
		}
		if plan.TotalWithSchemeDup != 1 {
			t.Errorf("expected 1 scheme duplicate total, got %d", plan.TotalWithSchemeDup)
		}
		if plan.TotalWithDedup != 0 {
			t.Errorf("expected 0 dedup total, got %d", plan.TotalWithDedup)
		}
	})

	t.Run("FAILED_SUBMIT cohort with dedup", func(t *testing.T) {
		plan, err := GetQueuePlan("%lever.co%", "FAILED_SUBMIT", false)
		if err != nil {
			t.Fatalf("GetQueuePlan failed: %v", err)
		}
		if plan.TotalCandidates != 1 {
			t.Fatalf("expected 1 candidate, got %d", plan.TotalCandidates)
		}
		if !plan.Candidates[0].HasDedupRow {
			t.Errorf("expected HasDedupRow to be true")
		}
		if plan.TotalWithDedup != 1 {
			t.Errorf("expected 1 dedup total, got %d", plan.TotalWithDedup)
		}
	})

	t.Run("BLOCKED_CAPTCHA cohort with null discovered_at", func(t *testing.T) {
		plan, err := GetQueuePlan("%indeed.com%", "BLOCKED_CAPTCHA", false)
		if err != nil {
			t.Fatalf("GetQueuePlan failed: %v", err)
		}
		if plan.TotalCandidates != 2 {
			t.Fatalf("expected 2 candidates (null row must not abort the plan), got %d", plan.TotalCandidates)
		}

		var found bool
		for _, c := range plan.Candidates {
			if c.OriginalURL == "https://indeed.com/e" {
				found = true
				if time.Since(c.DiscoveredAt) > time.Minute {
					t.Errorf("expected null discovered_at to fall back to time.Now(), got %v", c.DiscoveredAt)
				}
			}
		}
		if !found {
			t.Errorf("expected https://indeed.com/e (null discovered_at) to still be included in the plan")
		}
	})

	t.Run("Clear dedup true", func(t *testing.T) {
		plan, err := GetQueuePlan("%lever.co%", "FAILED_SUBMIT", true)
		if err != nil {
			t.Fatalf("GetQueuePlan failed: %v", err)
		}
		if len(plan.Candidates) == 1 && plan.Candidates[0].ProposedAction != "Clear applied_jobs dedup and requeue to DISCOVERED" {
			t.Errorf("unexpected proposed action: %s", plan.Candidates[0].ProposedAction)
		}
	})
}

// queuePlanRow is one row of fakeQueuePlanRows' canned result set.
type queuePlanRow struct {
	url        string
	status     string
	discovered sql.NullTime
	fitSim     sql.NullFloat64
	dedup      int
	schemeDup  int
	scanErr    error
}

// fakeQueuePlanRows is a hand-rolled queuePlanRows for exercising the
// mid-stream cursor-fault path directly, mirroring cmd/dashboard's
// fakeConversionRows (bug #452/#459's precedent). A real driver cannot be
// made to return a cursor error from Next() on demand against an in-memory
// sqlite database, so this fake stands in for "the row(s) before the fault
// scanned fine, then the cursor broke" -- the exact ambiguity Next()
// returning false cannot resolve on its own per database/sql's contract.
type fakeQueuePlanRows struct {
	rows    []queuePlanRow
	i       int
	failErr error
}

func (f *fakeQueuePlanRows) Next() bool {
	if f.i >= len(f.rows) {
		return false
	}
	f.i++
	return true
}

func (f *fakeQueuePlanRows) Scan(dest ...any) error {
	row := f.rows[f.i-1]
	if row.scanErr != nil {
		return row.scanErr
	}
	*dest[0].(*string) = row.url
	*dest[1].(*string) = row.status
	*dest[2].(*sql.NullTime) = row.discovered
	*dest[3].(*sql.NullFloat64) = row.fitSim
	*dest[4].(*int) = row.dedup
	*dest[5].(*int) = row.schemeDup
	return nil
}

func (f *fakeQueuePlanRows) Err() error {
	if f.i >= len(f.rows) {
		return f.failErr
	}
	return nil
}

// TestScanQueuePlanCandidates_PropagatesRowsErr pins bug #476: a cursor
// fault partway through the result stream must surface as an error, not as
// a silently truncated plan that renders as if it were complete. Mutation
// check: deleting scanQueuePlanCandidates's rows.Err() check makes this
// fail, since the loop above it already appended the one row that scanned
// fine before Next() returned false.
func TestScanQueuePlanCandidates_PropagatesRowsErr(t *testing.T) {
	rows := &fakeQueuePlanRows{
		rows: []queuePlanRow{
			{url: "https://lever.co/a", status: "BLOCKED_CAPTCHA", discovered: sql.NullTime{Time: time.Now(), Valid: true}},
		},
		failErr: errors.New("cursor error: connection reset"),
	}

	got, _, _, err := scanQueuePlanCandidates(rows, false)
	if err == nil {
		t.Fatalf("expected an error from a mid-stream cursor fault, got nil with candidates %+v", got)
	}
	if got != nil {
		t.Errorf("expected no partial candidate list on a cursor fault, got %+v", got)
	}
}

func TestScanQueuePlanCandidates_NoErrorOnCleanExhaustion(t *testing.T) {
	rows := &fakeQueuePlanRows{
		rows: []queuePlanRow{
			{url: "https://lever.co/a", status: "BLOCKED_CAPTCHA", discovered: sql.NullTime{Time: time.Now(), Valid: true}},
		},
	}

	got, dedup, schemeDup, err := scanQueuePlanCandidates(rows, false)
	if err != nil {
		t.Fatalf("unexpected error on clean exhaustion: %v", err)
	}
	if len(got) != 1 || got[0].OriginalURL != "https://lever.co/a" {
		t.Errorf("unexpected result: %+v", got)
	}
	if dedup != 0 || schemeDup != 0 {
		t.Errorf("expected zero dedup/scheme-dup totals, got %d/%d", dedup, schemeDup)
	}
}

// TestScanQueuePlanCandidates_SkipsRowScanErrorWithoutAborting is a
// regression guard for bug #453: a single row's Scan error must still
// `continue` past that row rather than discarding the whole plan, and this
// must keep holding now that a *cursor* error (bug #476, a different
// failure layer) also surfaces. The two must not be conflated back
// together.
func TestScanQueuePlanCandidates_SkipsRowScanErrorWithoutAborting(t *testing.T) {
	rows := &fakeQueuePlanRows{
		rows: []queuePlanRow{
			{scanErr: errors.New("scan error: converting NULL to time.Time")},
			{url: "https://lever.co/b", status: "DISCOVERED", discovered: sql.NullTime{Time: time.Now(), Valid: true}},
		},
	}

	got, _, _, err := scanQueuePlanCandidates(rows, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 || got[0].OriginalURL != "https://lever.co/b" {
		t.Errorf("expected the scan-error row skipped and the good row kept, got %+v", got)
	}
}
