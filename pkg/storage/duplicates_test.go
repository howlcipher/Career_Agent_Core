package storage

import (
	"testing"
	"time"
)

func TestDuplicateApplicationExistsUsesStrictIdentity(t *testing.T) {
	setupTestDB(t)
	defer teardownTestDB()

	now := time.Now().UTC()
	if _, err := db.Exec(`INSERT INTO job_funnel
		(company_name, job_title, job_location, is_remote, url, status, applied_at)
		VALUES (?, ?, ?, ?, ?, 'APPLIED', ?)`,
		"Acme, Inc.", "Senior Software Engineer", "Detroit, MI", true,
		"https://jobs.example.com/acme/senior-engineer", now); err != nil {
		t.Fatalf("seed confirmed application: %v", err)
	}

	tests := []struct {
		name     string
		company  string
		title    string
		location string
		remote   bool
		want     bool
	}{
		{"same normalized company and role", "ACME LLC", "Senior Software Engineer", "Detroit MI", true, true},
		{"different seniority remains eligible", "Acme", "Software Engineer", "Detroit MI", true, false},
		{"different role family remains eligible", "Acme", "Site Reliability Engineer", "Detroit MI", true, false},
		{"different location remains eligible", "Acme", "Senior Software Engineer", "Chicago IL", true, false},
		{"different remote class remains eligible", "Acme", "Senior Software Engineer", "Detroit MI", false, false},
		{"missing location never guesses", "Acme", "Senior Software Engineer", "", true, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := DuplicateApplicationExists(tc.company, tc.title, tc.location, tc.remote, 30*24*time.Hour)
			if err != nil {
				t.Fatalf("DuplicateApplicationExists: %v", err)
			}
			if got != tc.want {
				t.Errorf("DuplicateApplicationExists() = %t, want %t", got, tc.want)
			}
		})
	}
}

func TestDuplicateApplicationExistsIgnoresExpiredAndIncompleteHistory(t *testing.T) {
	setupTestDB(t)
	defer teardownTestDB()

	if _, err := db.Exec(`INSERT INTO job_funnel
		(company_name, job_title, job_location, is_remote, url, status, applied_at)
		VALUES (?, ?, NULL, NULL, ?, 'APPLIED', ?)`,
		"Acme", "Senior Software Engineer", "https://jobs.example.com/acme/legacy", time.Now().UTC()); err != nil {
		t.Fatalf("seed incomplete historical application: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO job_funnel
		(company_name, job_title, job_location, is_remote, url, status, applied_at)
		VALUES (?, ?, ?, ?, ?, 'APPLIED', ?)`,
		"Acme", "Senior Software Engineer", "Detroit MI", true,
		"https://jobs.example.com/acme/old", time.Now().UTC().Add(-31*24*time.Hour)); err != nil {
		t.Fatalf("seed expired confirmed application: %v", err)
	}

	got, err := DuplicateApplicationExists("Acme", "Senior Software Engineer", "Detroit MI", true, 30*24*time.Hour)
	if err != nil {
		t.Fatalf("DuplicateApplicationExists: %v", err)
	}
	if got {
		t.Fatal("incomplete or expired history must not suppress a new job")
	}
}

func TestUpdateFunnelIdentity(t *testing.T) {
	setupTestDB(t)
	defer teardownTestDB()

	const jobURL = "https://jobs.example.com/acme/identity"
	if _, err := AddToFunnel("Acme", "Engineer", jobURL, "DISCOVERED"); err != nil {
		t.Fatalf("AddToFunnel: %v", err)
	}
	if err := UpdateFunnelIdentity(jobURL, " Detroit, MI ", true); err != nil {
		t.Fatalf("UpdateFunnelIdentity: %v", err)
	}

	var location string
	var remote bool
	if err := db.QueryRow("SELECT job_location, is_remote FROM job_funnel WHERE url = ?", jobURL).Scan(&location, &remote); err != nil {
		t.Fatalf("read funnel identity: %v", err)
	}
	if location != "Detroit, MI" || !remote {
		t.Errorf("stored identity = (%q, %t), want (%q, true)", location, remote, "Detroit, MI")
	}
}

func TestMigrateJobFunnelIdentityLeavesLegacyRowsUnknown(t *testing.T) {
	setupTestDB(t)
	defer teardownTestDB()

	if _, err := db.Exec("DROP TABLE job_funnel"); err != nil {
		t.Fatalf("drop job_funnel: %v", err)
	}
	if _, err := db.Exec(`CREATE TABLE job_funnel (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		company_name TEXT,
		job_title TEXT,
		url TEXT UNIQUE,
		status TEXT,
		applied_at DATETIME
	)`); err != nil {
		t.Fatalf("create legacy job_funnel: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO job_funnel (company_name, job_title, url, status, applied_at)
		VALUES ('Acme', 'Engineer', 'https://jobs.example.com/acme/legacy', 'APPLIED', ?)`, time.Now().UTC()); err != nil {
		t.Fatalf("seed legacy row: %v", err)
	}

	if err := migrateJobFunnelIdentity(); err != nil {
		t.Fatalf("migrateJobFunnelIdentity: %v", err)
	}
	if err := migrateJobFunnelIdentity(); err != nil {
		t.Fatalf("second migrateJobFunnelIdentity: %v", err)
	}

	var unknown int
	if err := db.QueryRow(`SELECT COUNT(*) FROM job_funnel
		WHERE url = ? AND job_location IS NULL AND is_remote IS NULL`, "https://jobs.example.com/acme/legacy").Scan(&unknown); err != nil {
		t.Fatalf("read legacy identity: %v", err)
	}
	if unknown != 1 {
		t.Fatalf("legacy row's identity metadata was backfilled")
	}
}
