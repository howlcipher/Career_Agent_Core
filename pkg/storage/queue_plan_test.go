package storage

import (
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
