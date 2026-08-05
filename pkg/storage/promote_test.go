package storage

import (
	"testing"
)

func TestMarkJobAppliedManually(t *testing.T) {
	setupTestDB(t)
	defer teardownTestDB()

	// Setup a job in PROCESSED_MANUAL state
	_, err := db.Exec(`INSERT INTO job_funnel (id, company_name, job_title, url, status, status_reason) 
		VALUES (1, 'TestCorp', 'Eng', 'http://test.com', 'PROCESSED_MANUAL', 'find_only_threshold_met')`)
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	// Also setup assisted_applications row to test provenance preservation
	_, err = db.Exec(`INSERT INTO assisted_applications (job_id, original_status, next_action_code, created_at, updated_at) 
		VALUES (1, 'PROCESSED_MANUAL', 'wait', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`)
	if err != nil {
		t.Fatalf("setup assisted failed: %v", err)
	}

	err = MarkJobAppliedManually(db, 1)
	if err != nil {
		t.Fatalf("MarkJobAppliedManually failed: %v", err)
	}

	// Verify funnel status
	var status, reason string
	db.QueryRow(`SELECT status, status_reason FROM job_funnel WHERE id = 1`).Scan(&status, &reason)
	if status != "APPLIED" || reason != "manual_user_confirmation" {
		t.Errorf("funnel not updated correctly, got %s, %s", status, reason)
	}

	// Verify applied_jobs dedup
	var count int
	db.QueryRow(`SELECT COUNT(*) FROM applied_jobs WHERE url = 'http://test.com'`).Scan(&count)
	if count != 1 {
		t.Errorf("applied_jobs dedup not inserted")
	}

	// Verify provenance
	var prov string
	db.QueryRow(`SELECT confirmation_provenance FROM assisted_applications WHERE job_id = 1`).Scan(&prov)
	if prov != "manual_user_confirmation" {
		t.Errorf("provenance not updated, got %s", prov)
	}

	// Test idempotency
	err = MarkJobAppliedManually(db, 1)
	if err != nil {
		t.Errorf("idempotent call failed: %v", err)
	}
}

func TestMarkJobAppliedManually_WrongState(t *testing.T) {
	setupTestDB(t)
	defer teardownTestDB()

	_, err := db.Exec(`INSERT INTO job_funnel (id, company_name, job_title, url, status, status_reason) 
		VALUES (2, 'TestCorp', 'Eng', 'http://test2.com', 'PROCESSED_MANUAL', 'some_other_reason')`)
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	err = MarkJobAppliedManually(db, 2)
	if err != ErrJobNotQualified {
		t.Errorf("expected ErrJobNotQualified, got %v", err)
	}
}
