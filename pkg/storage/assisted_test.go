package storage

import (
	"testing"
	"time"
)

func TestMigrateLegacyAssisted_IsDryRunIdempotentAndPreservesStatus(t *testing.T) {
	setupTestDB(t)
	defer teardownTestDB()
	for _, tc := range []struct{ company, url, status string }{
		{"Review Co", "https://review.example/jobs/1", "AWAITING_REVIEW"},
		{"Login Co", "https://login.example/jobs/2", "MANUAL_REQUIRED"},
		{"Captcha Co", "https://captcha.example/jobs/3", "BLOCKED_CAPTCHA"},
	} {
		if _, err := AddToFunnel(tc.company, "Engineer", tc.url, tc.status); err != nil {
			t.Fatal(err)
		}
	}
	// A canonical dedup record wins over the legacy status and must not become
	// actionable again.
	if err := RecordApplicationInDB("Captcha Co", "Engineer", "https://captcha.example/jobs/3"); err != nil {
		t.Fatal(err)
	}

	dry, err := MigrateLegacyAssisted(AssistedMigrationOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if dry.Eligible != 2 || dry.Imported != 0 || dry.Excluded["confirmed_application"] != 1 {
		t.Fatalf("dry report = %+v", dry)
	}
	var count int
	if err := GetDB().QueryRow("SELECT COUNT(*) FROM assisted_applications").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("dry run inserted %d plans", count)
	}

	confirmed, err := MigrateLegacyAssisted(AssistedMigrationOptions{Confirm: true})
	if err != nil {
		t.Fatal(err)
	}
	if confirmed.Imported != 2 {
		t.Fatalf("confirmed report = %+v", confirmed)
	}
	second, err := MigrateLegacyAssisted(AssistedMigrationOptions{Confirm: true})
	if err != nil {
		t.Fatal(err)
	}
	if second.Imported != 0 || second.AlreadyIn != 2 {
		t.Fatalf("second report = %+v", second)
	}

	var status string
	if err := GetDB().QueryRow("SELECT status FROM job_funnel WHERE company_name = 'Review Co'").Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "AWAITING_REVIEW" {
		t.Fatalf("migration reset status to %q", status)
	}
}

func TestGetAssistedQueue_UsesHumanInstructionAndHidesURLs(t *testing.T) {
	setupTestDB(t)
	defer teardownTestDB()
	if _, err := AddToFunnel("Ready Co", "Platform Engineer", "https://private.example/apply/123", "AWAITING_REVIEW"); err != nil {
		t.Fatal(err)
	}
	if _, err := MigrateLegacyAssisted(AssistedMigrationOptions{Confirm: true}); err != nil {
		t.Fatal(err)
	}
	jobs, err := GetAssistedQueue(GetDB())
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 1 {
		t.Fatalf("queue length = %d", len(jobs))
	}
	job := jobs[0]
	if job.NextAction.Code != "review_and_submit" || job.NextAction.Instruction == "" || job.NextAction.PrimaryButton != "Open Prepared Application" {
		t.Fatalf("next action = %+v", job.NextAction)
	}
	if job.ID == "" || job.Company != "Ready Co" || job.LastUpdated.After(time.Now().Add(time.Minute)) {
		t.Fatalf("queue job = %+v", job)
	}
}

func TestMigrateLegacyAssisted_RejectsUnsupportedStatus(t *testing.T) {
	setupTestDB(t)
	defer teardownTestDB()
	if _, err := MigrateLegacyAssisted(AssistedMigrationOptions{Statuses: []string{"APPLIED"}}); err == nil {
		t.Fatal("expected unsupported status error")
	}
}

func TestConfirmAssistedSubmission_RequiresPlanAndPreservesManualProvenance(t *testing.T) {
	setupTestDB(t)
	defer teardownTestDB()
	url := "https://review.example/jobs/confirm"
	if _, err := AddToFunnel("Confirm Co", "Engineer", url, "AWAITING_REVIEW"); err != nil {
		t.Fatal(err)
	}
	if _, err := MigrateLegacyAssisted(AssistedMigrationOptions{Confirm: true}); err != nil {
		t.Fatal(err)
	}
	var id string
	if err := GetDB().QueryRow("SELECT id FROM job_funnel WHERE url = ?", url).Scan(&id); err != nil {
		t.Fatal(err)
	}
	if err := ConfirmAssistedSubmission(GetDB(), id); err != nil {
		t.Fatal(err)
	}
	var status, provenance string
	if err := GetDB().QueryRow(`SELECT jf.status, aa.confirmation_provenance FROM job_funnel jf JOIN assisted_applications aa ON aa.job_id = jf.id WHERE jf.id = ?`, id).Scan(&status, &provenance); err != nil {
		t.Fatal(err)
	}
	if status != "APPLIED" || provenance != "manual_user_confirmation" {
		t.Fatalf("status=%q provenance=%q", status, provenance)
	}
	if err := ConfirmAssistedSubmission(GetDB(), id); err == nil {
		t.Fatal("second confirmation must conflict")
	}
}
