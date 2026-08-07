package storage

import (
	"os"
	"strings"
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
	if job.NextAction.Code != "revalidate_current_page" || job.NextAction.RequiresBrowser || job.NextAction.Instruction == "" || job.NextAction.PrimaryButton != "Check Current Page" {
		t.Fatalf("next action = %+v", job.NextAction)
	}
	if job.ID == "" || job.Company != "Ready Co" || job.LastUpdated.After(time.Now().Add(time.Minute)) {
		t.Fatalf("queue job = %+v", job)
	}
}

func TestAssistedRevalidationGuidesBrowserLaunchAndPersistsSafeOutcome(t *testing.T) {
	setupTestDB(t)
	defer teardownTestDB()
	if _, err := AddToFunnel("Captcha Co", "Engineer", "https://captcha.example/jobs/1", "BLOCKED_CAPTCHA"); err != nil {
		t.Fatal(err)
	}
	if _, err := MigrateLegacyAssisted(AssistedMigrationOptions{Confirm: true}); err != nil {
		t.Fatal(err)
	}
	var id string
	if err := GetDB().QueryRow("SELECT id FROM job_funnel WHERE company_name = 'Captcha Co'").Scan(&id); err != nil {
		t.Fatal(err)
	}
	if _, err := GetAssistedLaunchInfo(GetDB(), id); err == nil {
		t.Fatal("unrevalidated job must not be launchable")
	}
	if err := RecordAssistedRevalidation(GetDB(), id, "application_ready", time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err := GetAssistedLaunchInfo(GetDB(), id); err != nil {
		t.Fatalf("reviewed current page should be launchable: %v", err)
	}
	jobs, err := GetAssistedQueue(GetDB())
	if err != nil || len(jobs) != 1 {
		t.Fatalf("queue = %#v, %v", jobs, err)
	}
	if jobs[0].NextAction.Code != "open_verified_application" || jobs[0].NextAction.PrimaryButton != "Open Verified Application" {
		t.Fatalf("reviewed next action = %+v", jobs[0].NextAction)
	}
}

func TestAssistedQueueExposesContinueAndReviewAfterRefill(t *testing.T) {
	setupTestDB(t)
	defer teardownTestDB()
	if _, err := AddToFunnel("Live Handoff Co", "Engineer", "https://live.example/jobs/1", "MANUAL_REQUIRED"); err != nil {
		t.Fatal(err)
	}
	if _, err := MigrateLegacyAssisted(AssistedMigrationOptions{Confirm: true}); err != nil {
		t.Fatal(err)
	}
	var id string
	if err := GetDB().QueryRow("SELECT id FROM job_funnel WHERE company_name = 'Live Handoff Co'").Scan(&id); err != nil {
		t.Fatal(err)
	}
	if err := RecordAssistedRevalidation(GetDB(), id, "application_ready", time.Now()); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	if claimed, err := AcquireAssistedLease(GetDB(), id, "owner", now); err != nil || !claimed {
		t.Fatalf("claim live handoff: claimed=%v err=%v", claimed, err)
	}
	jobs, err := GetAssistedQueue(GetDB())
	if err != nil || len(jobs) != 1 || !jobs[0].NextAction.CanContinue || !jobs[0].LiveBrowser {
		t.Fatalf("live handoff action = %#v, err=%v", jobs, err)
	}
	if err := RequestAssistedContinue(GetDB(), id, now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := RecordAssistedRefill(GetDB(), id, "owner", now.Add(2*time.Second)); err != nil {
		t.Fatal(err)
	}
	jobs, err = GetAssistedQueue(GetDB())
	if err != nil || len(jobs) != 1 || jobs[0].NextAction.Code != "review_and_submit" || !jobs[0].NextAction.RequiresExplicitSubmit {
		t.Fatalf("post-refill action = %#v, err=%v", jobs, err)
	}
}

func TestEnsureAssistedSchema_ResetsObsoleteCurrentPageReview(t *testing.T) {
	setupTestDB(t)
	defer teardownTestDB()
	if _, err := AddToFunnel("Legacy Review Co", "Engineer", "https://review.example/jobs/legacy", "AWAITING_REVIEW"); err != nil {
		t.Fatal(err)
	}
	if _, err := MigrateLegacyAssisted(AssistedMigrationOptions{Confirm: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := GetDB().Exec("UPDATE assisted_applications SET revalidation_state = 'current_page_review', revalidated_at = CURRENT_TIMESTAMP"); err != nil {
		t.Fatal(err)
	}
	if err := EnsureAssistedSchema(GetDB()); err != nil {
		t.Fatal(err)
	}
	var state string
	var revalidatedAt any
	if err := GetDB().QueryRow("SELECT revalidation_state, revalidated_at FROM assisted_applications").Scan(&state, &revalidatedAt); err != nil {
		t.Fatal(err)
	}
	if state != "required" || revalidatedAt != nil {
		t.Fatalf("obsolete review state = %q, revalidated_at = %#v", state, revalidatedAt)
	}
}

func TestMigrateLegacyAssisted_RejectsUnsupportedStatus(t *testing.T) {
	setupTestDB(t)
	defer teardownTestDB()
	if _, err := MigrateLegacyAssisted(AssistedMigrationOptions{Statuses: []string{"APPLIED"}}); err == nil {
		t.Fatal("expected unsupported status error")
	}
}

func TestMigrateLegacyAssisted_ExcludesExpiredAndLowFitRows(t *testing.T) {
	setupTestDB(t)
	defer teardownTestDB()
	for _, tc := range []struct {
		url, reason string
		fit         int
	}{
		{"https://expired.example/jobs/1", "expired", 90},
		{"https://lowfit.example/jobs/2", "", 49},
	} {
		if _, err := AddToFunnel("Excluded Co", "Engineer", tc.url, "AWAITING_REVIEW"); err != nil {
			t.Fatal(err)
		}
		if _, err := GetDB().Exec("UPDATE job_funnel SET status_reason = ?, fit_score = ? WHERE url = ?", tc.reason, tc.fit, tc.url); err != nil {
			t.Fatal(err)
		}
	}
	report, err := MigrateLegacyAssisted(AssistedMigrationOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if report.Eligible != 0 || report.AlreadyIn != 0 || report.Excluded["posting_expired"] != 1 || report.Excluded["below_fit_threshold"] != 1 {
		t.Fatalf("unsafe rows entered migration report: %+v", report)
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

func TestAssistedLease_AllowsOneOwnerAndContinuationOnlyWhileLive(t *testing.T) {
	setupTestDB(t)
	defer teardownTestDB()
	if _, err := AddToFunnel("Lease Co", "Engineer", "https://lease.example/jobs/1", "MANUAL_REQUIRED"); err != nil {
		t.Fatal(err)
	}
	if _, err := MigrateLegacyAssisted(AssistedMigrationOptions{Confirm: true}); err != nil {
		t.Fatal(err)
	}
	var id string
	if err := GetDB().QueryRow("SELECT id FROM job_funnel WHERE company_name = 'Lease Co'").Scan(&id); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	claimed, err := AcquireAssistedLease(GetDB(), id, "first", now)
	if err != nil || !claimed {
		t.Fatalf("first claim: claimed=%v err=%v", claimed, err)
	}
	if claimed, err := AcquireAssistedLease(GetDB(), id, "second", now.Add(20*time.Second)); err != nil || claimed {
		t.Fatalf("second claim: claimed=%v err=%v", claimed, err)
	}
	if err := RequestAssistedContinue(GetDB(), id, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := ReleaseAssistedLease(GetDB(), id, "second", now.Add(time.Minute)); err == nil {
		t.Fatal("different owner released lease")
	}
	if err := ReleaseAssistedLease(GetDB(), id, "first", now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := RequestAssistedContinue(GetDB(), id, now.Add(2*time.Minute)); err == nil {
		t.Fatal("continuation without live browser must fail")
	}
}

func TestRecordAssistedManualReview_KeepsLiveBrowserAvailableForConfirmation(t *testing.T) {
	setupTestDB(t)
	defer teardownTestDB()
	if _, err := AddToFunnel("Manual Review Co", "Engineer", "https://manual-review.example/jobs/1", "BLOCKED_CAPTCHA"); err != nil {
		t.Fatal(err)
	}
	if _, err := MigrateLegacyAssisted(AssistedMigrationOptions{Confirm: true}); err != nil {
		t.Fatal(err)
	}
	var id string
	if err := GetDB().QueryRow("SELECT id FROM job_funnel WHERE company_name = 'Manual Review Co'").Scan(&id); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	if claimed, err := AcquireAssistedLease(GetDB(), id, "owner", now); err != nil || !claimed {
		t.Fatalf("claim: claimed=%v err=%v", claimed, err)
	}
	if err := RequestAssistedContinue(GetDB(), id, now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := RecordAssistedManualReview(GetDB(), id, "other-owner", now.Add(2*time.Second)); err == nil {
		t.Fatal("different owner advanced assisted plan")
	}
	if err := RecordAssistedManualReview(GetDB(), id, "owner", now.Add(2*time.Second)); err != nil {
		t.Fatal(err)
	}
	jobs, err := GetAssistedQueue(GetDB())
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 1 {
		t.Fatalf("queue length = %d, want 1", len(jobs))
	}
	job := jobs[0]
	if !job.LiveBrowser || job.NextAction.Code != "manual_review" || !job.NextAction.RequiresExplicitSubmit || job.NextAction.CanContinue {
		t.Fatalf("manual review projection = %+v, live=%v", job.NextAction, job.LiveBrowser)
	}
	if job.NextAction.DocumentsReady {
		t.Fatal("manual review must not claim unavailable documents are ready")
	}
	if err := ReleaseAssistedLease(GetDB(), id, "owner", now.Add(3*time.Second)); err != nil {
		t.Fatal(err)
	}
	jobs, err = GetAssistedQueue(GetDB())
	if err != nil {
		t.Fatal(err)
	}
	job = jobs[0]
	if job.LiveBrowser || job.NextAction.PrimaryButton != "Reopen Verified Application" || strings.Contains(job.NextAction.Instruction, "remains open") {
		t.Fatalf("closed manual review projection = %+v, live=%v", job.NextAction, job.LiveBrowser)
	}
}

func TestAssistedLease_AllowsOnlyOneVisibleBrowserAcrossPlans(t *testing.T) {
	setupTestDB(t)
	defer teardownTestDB()
	for _, tc := range []struct{ company, url string }{
		{"First Lease Co", "https://lease.example/jobs/first"},
		{"Second Lease Co", "https://lease.example/jobs/second"},
	} {
		if _, err := AddToFunnel(tc.company, "Engineer", tc.url, "MANUAL_REQUIRED"); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := MigrateLegacyAssisted(AssistedMigrationOptions{Confirm: true}); err != nil {
		t.Fatal(err)
	}
	var firstID, secondID string
	if err := GetDB().QueryRow("SELECT id FROM job_funnel WHERE company_name = 'First Lease Co'").Scan(&firstID); err != nil {
		t.Fatal(err)
	}
	if err := GetDB().QueryRow("SELECT id FROM job_funnel WHERE company_name = 'Second Lease Co'").Scan(&secondID); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	if claimed, err := AcquireAssistedLease(GetDB(), firstID, "first", now); err != nil || !claimed {
		t.Fatalf("first claim: claimed=%v err=%v", claimed, err)
	}
	if claimed, err := AcquireAssistedLease(GetDB(), secondID, "second", now.Add(20*time.Second)); err != nil || claimed {
		t.Fatalf("second plan claim while another browser is active: claimed=%v err=%v", claimed, err)
	}
}

func TestAssistedLease_HeartbeatRenewsAndStaleOwnerCanBeReclaimed(t *testing.T) {
	setupTestDB(t)
	defer teardownTestDB()
	if _, err := AddToFunnel("Heartbeat Co", "Engineer", "https://heartbeat.example/jobs/1", "BLOCKED_CAPTCHA"); err != nil {
		t.Fatal(err)
	}
	if _, err := MigrateLegacyAssisted(AssistedMigrationOptions{Confirm: true}); err != nil {
		t.Fatal(err)
	}
	var id string
	if err := GetDB().QueryRow("SELECT id FROM job_funnel WHERE company_name = 'Heartbeat Co'").Scan(&id); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	claimed, err := AcquireAssistedLease(GetDB(), id, "first", now)
	if err != nil || !claimed {
		t.Fatalf("initial claim: claimed=%v err=%v", claimed, err)
	}
	if err := RenewAssistedLease(GetDB(), id, "first", now.Add(10*time.Second)); err != nil {
		t.Fatal(err)
	}
	if claimed, err := AcquireAssistedLease(GetDB(), id, "second", now.Add(20*time.Second)); err != nil || claimed {
		t.Fatalf("fresh heartbeat should block second owner: claimed=%v err=%v", claimed, err)
	}
	if claimed, err := AcquireAssistedLease(GetDB(), id, "second", now.Add(2*time.Minute)); err != nil || !claimed {
		t.Fatalf("stale heartbeat should be reclaimable: claimed=%v err=%v", claimed, err)
	}
}

func TestEnsureAssistedPlanForURL_CreatesNewInterruptionPlanOnce(t *testing.T) {
	setupTestDB(t)
	defer teardownTestDB()
	url := "https://new.example/jobs/1"
	if _, err := AddToFunnel("New Co", "Engineer", url, "DISCOVERED"); err != nil {
		t.Fatal(err)
	}
	if err := UpdateFunnelStatus(url, "BLOCKED_CAPTCHA"); err != nil {
		t.Fatal(err)
	}
	if err := EnsureAssistedPlanForURL(url, "BLOCKED_CAPTCHA"); err != nil {
		t.Fatal(err)
	}
	if err := EnsureAssistedPlanForURL(url, "BLOCKED_CAPTCHA"); err != nil {
		t.Fatal(err)
	}
	var count int
	var legacy bool
	var action string
	if err := GetDB().QueryRow("SELECT COUNT(*), is_legacy, next_action_code FROM assisted_applications").Scan(&count, &legacy, &action); err != nil {
		t.Fatal(err)
	}
	if count != 1 || legacy || action != "solve_captcha" {
		t.Fatalf("new plan count=%d legacy=%v action=%q", count, legacy, action)
	}
}

// The résumé employers receive is master_resume.pdf for every job: cmd/agent
// returns that path from both its tailored and its untailored document branch.
// The per-job resume.md is a saved reference document, and when tailoring is
// skipped it holds nothing but a short "master documents were used" note.
// Assisted Apply resolved its upload payload from that artifact, so a real
// application uploaded the note in place of the résumé (bugs.md #515).
func TestGetAssistedDocument_ResumeIsMasterResumeNotSavedArtifact(t *testing.T) {
	t.Chdir(t.TempDir())
	setupTestDB(t)
	defer teardownTestDB()

	const postingURL = "https://captcha.example/jobs/1"
	if _, err := AddToFunnel("Captcha Co", "Engineer", postingURL, "BLOCKED_CAPTCHA"); err != nil {
		t.Fatal(err)
	}
	if err := EnsureAssistedPlanForURL(postingURL, "BLOCKED_CAPTCHA"); err != nil {
		t.Fatal(err)
	}
	const untailoredNote = "Master documents used for this application (use_master_cover_letter is enabled); no per-job tailoring was generated."
	if _, err := SaveApplication("Captcha Co", "Engineer", "", postingURL, untailoredNote, "Dear team,", untailoredNote); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(MasterResumePath, []byte("%PDF-1.7 genuine resume"), 0o600); err != nil {
		t.Fatal(err)
	}

	var id string
	if err := GetDB().QueryRow("SELECT id FROM job_funnel WHERE url = ?", NormalizeURL(postingURL)).Scan(&id); err != nil {
		t.Fatal(err)
	}

	document, err := GetAssistedDocument(GetDB(), id, "resume")
	if err != nil {
		t.Fatalf("resolve assisted résumé: %v", err)
	}
	content, err := os.ReadFile(document.Path)
	if err != nil {
		t.Fatalf("read resolved résumé: %v", err)
	}
	if strings.Contains(string(content), "no per-job tailoring") {
		t.Fatalf("assisted résumé resolved to the saved artifact %q, so the upload payload is the placeholder note rather than the master résumé", document.Path)
	}
	if document.Path != MasterResumePath {
		t.Fatalf("assisted résumé path = %q, want the master résumé %q", document.Path, MasterResumePath)
	}
}

// The cover letter genuinely is the per-job artifact: cmd/agent hands the
// submitter storage.CoverLetterPath. Pinning it here keeps the résumé fix from
// being over-applied to a document that was already correct.
func TestGetAssistedDocument_CoverLetterStaysPerJobArtifact(t *testing.T) {
	t.Chdir(t.TempDir())
	setupTestDB(t)
	defer teardownTestDB()

	const postingURL = "https://captcha.example/jobs/2"
	if _, err := AddToFunnel("Captcha Co", "Engineer", postingURL, "BLOCKED_CAPTCHA"); err != nil {
		t.Fatal(err)
	}
	if err := EnsureAssistedPlanForURL(postingURL, "BLOCKED_CAPTCHA"); err != nil {
		t.Fatal(err)
	}
	if _, err := SaveApplication("Captcha Co", "Engineer", "", postingURL, "resume note", "role specific letter", "prep"); err != nil {
		t.Fatal(err)
	}

	var id string
	if err := GetDB().QueryRow("SELECT id FROM job_funnel WHERE url = ?", NormalizeURL(postingURL)).Scan(&id); err != nil {
		t.Fatal(err)
	}

	document, err := GetAssistedDocument(GetDB(), id, "cover_letter")
	if err != nil {
		t.Fatalf("resolve assisted cover letter: %v", err)
	}
	if document.Path != CoverLetterPath("Captcha Co", postingURL) {
		t.Fatalf("cover letter path = %q, want the per-job artifact", document.Path)
	}
	content, err := os.ReadFile(document.Path)
	if err != nil {
		t.Fatalf("read resolved cover letter: %v", err)
	}
	if !strings.Contains(string(content), "role specific letter") {
		t.Fatal("cover letter no longer resolves to the role-specific artifact")
	}
}

// With use_master_cover_letter enabled, cmd/agent uploads the static master
// letter itself, while the per-job coverletter.txt holds only the extracted
// *text* of that same file. The content matches, so a textarea field is
// unaffected — but a file-upload field received an unformatted .txt where the
// automatic path sent the designed PDF (bugs.md #525).
func TestGetAssistedDocument_MasterCoverLetterServedWhenEnabled(t *testing.T) {
	t.Chdir(t.TempDir())
	setupTestDB(t)
	defer teardownTestDB()

	const masterPath = "Omni_CoverLetter.pdf"
	const masterContent = "%PDF-1.7 master cover letter content"
	if err := os.WriteFile(masterPath, []byte(masterContent), 0o600); err != nil {
		t.Fatal(err)
	}

	origResolver := resolveMasterCoverLetter
	defer func() { resolveMasterCoverLetter = origResolver }()
	resolveMasterCoverLetter = func() string { return masterPath }

	const postingURL = "https://captcha.example/jobs/master-cl"
	if _, err := AddToFunnel("Master CL Co", "Engineer", postingURL, "BLOCKED_CAPTCHA"); err != nil {
		t.Fatal(err)
	}
	if err := EnsureAssistedPlanForURL(postingURL, "BLOCKED_CAPTCHA"); err != nil {
		t.Fatal(err)
	}
	if _, err := SaveApplication("Master CL Co", "Engineer", "", postingURL, "resume note", "distinguishable per-job text letter", "prep"); err != nil {
		t.Fatal(err)
	}

	var id string
	if err := GetDB().QueryRow("SELECT id FROM job_funnel WHERE url = ?", NormalizeURL(postingURL)).Scan(&id); err != nil {
		t.Fatal(err)
	}

	document, err := GetAssistedDocument(GetDB(), id, "cover_letter")
	if err != nil {
		t.Fatalf("resolve assisted cover letter: %v", err)
	}
	if document.Path != masterPath {
		t.Fatalf("cover letter path = %q, want master cover letter path %q", document.Path, masterPath)
	}
	if document.Name != masterPath {
		t.Fatalf("cover letter name = %q, want %q", document.Name, masterPath)
	}

	perJobPath := CoverLetterPath("Master CL Co", postingURL)
	if document.Path == perJobPath {
		t.Fatalf("cover letter path = %q, must not be per-job artifact path", document.Path)
	}

	content, err := os.ReadFile(document.Path)
	if err != nil {
		t.Fatalf("read resolved cover letter: %v", err)
	}
	if string(content) != masterContent {
		t.Fatalf("served content = %q, want master content %q", string(content), masterContent)
	}
	if strings.Contains(string(content), "distinguishable per-job text letter") {
		t.Fatal("served cover letter contained per-job text artifact content instead of master letter")
	}
}

// A configured master letter that cannot be validated must fail closed. The
// tempting fallback — serve the per-job .txt instead — is precisely the defect
// bugs.md #525 describes, so it would turn a visible error into the silent
// wrong-format upload. An error degrades safely: the dashboard reports the
// document as not ready and cmd/assist preserves the application for manual
// completion.
func TestGetAssistedDocument_InvalidMasterCoverLetterReturnsError(t *testing.T) {
	t.Chdir(t.TempDir())
	setupTestDB(t)
	defer teardownTestDB()

	const nonExistentPath = "missing_master_cover_letter.pdf"

	origResolver := resolveMasterCoverLetter
	defer func() { resolveMasterCoverLetter = origResolver }()
	resolveMasterCoverLetter = func() string { return nonExistentPath }

	const postingURL = "https://captcha.example/jobs/invalid-master-cl"
	if _, err := AddToFunnel("Invalid Master CL Co", "Engineer", postingURL, "BLOCKED_CAPTCHA"); err != nil {
		t.Fatal(err)
	}
	if err := EnsureAssistedPlanForURL(postingURL, "BLOCKED_CAPTCHA"); err != nil {
		t.Fatal(err)
	}
	if _, err := SaveApplication("Invalid Master CL Co", "Engineer", "", postingURL, "resume note", "per-job letter", "prep"); err != nil {
		t.Fatal(err)
	}

	var id string
	if err := GetDB().QueryRow("SELECT id FROM job_funnel WHERE url = ?", NormalizeURL(postingURL)).Scan(&id); err != nil {
		t.Fatal(err)
	}

	document, err := GetAssistedDocument(GetDB(), id, "cover_letter")
	if err == nil {
		t.Fatalf("expected error for missing master cover letter, got path %q", document.Path)
	}
	perJobPath := CoverLetterPath("Invalid Master CL Co", postingURL)
	if document.Path == perJobPath {
		t.Fatalf("document path fell back to per-job text artifact %q on master letter validation failure", perJobPath)
	}
}

// cover_letter_ready is what the operator reads before opening an assisted
// application, so the queue's readiness signal has to track the same document
// GetAssistedDocument would actually serve — a queue still probing the per-job
// .txt would report ready even with no master letter on disk.
//
// This drives assistedDocumentExists directly rather than GetAssistedQueue,
// which cannot be asserted on here: GetAssistedQueue calls this helper from
// inside an open rows iteration, and that nested query takes a *second* pooled
// connection. Against the `:memory:` database setupTestDB opens, every
// connection gets its own private, empty schema, so the nested lookup fails
// with "no such table" and both readiness fields come back false no matter
// what the resolver returns. That is an artifact of the in-memory harness, not
// of production, where the pool's connections all open the same file — but it
// does mean these two fields cannot be covered end to end through the queue
// (filed as bugs.md #527).
func TestAssistedDocumentExists_CoverLetterReadinessFollowsTheMasterLetter(t *testing.T) {
	t.Chdir(t.TempDir())
	setupTestDB(t)
	defer teardownTestDB()

	const masterPath = "Omni_CoverLetter.pdf"
	const postingURL = "https://captcha.example/jobs/queue-master-cl"
	if _, err := AddToFunnel("Queue CL Co", "Engineer", postingURL, "BLOCKED_CAPTCHA"); err != nil {
		t.Fatal(err)
	}
	if err := EnsureAssistedPlanForURL(postingURL, "BLOCKED_CAPTCHA"); err != nil {
		t.Fatal(err)
	}
	// The per-job artifact is written and stays written throughout: a
	// readiness check still pointed at it would report ready in both phases.
	if _, err := SaveApplication("Queue CL Co", "Engineer", "", postingURL, "resume note", "per-job letter", "prep"); err != nil {
		t.Fatal(err)
	}

	var id string
	if err := GetDB().QueryRow("SELECT id FROM job_funnel WHERE url = ?", NormalizeURL(postingURL)).Scan(&id); err != nil {
		t.Fatal(err)
	}

	if assistedDocumentExists(GetDB(), id, "cover_letter", masterPath) {
		t.Fatal("cover letter reported ready while the configured master letter did not exist")
	}
	if err := os.WriteFile(masterPath, []byte("%PDF-1.7 master cover letter content"), 0o600); err != nil {
		t.Fatal(err)
	}
	if !assistedDocumentExists(GetDB(), id, "cover_letter", masterPath) {
		t.Fatal("cover letter reported unavailable after the master letter was written")
	}
}

// A revalidated AWAITING_REVIEW handoff must still expose the explicit-submit
// affordance. Without it the dashboard never renders "I saw a confirmation —
// Mark Applied", so an application the operator genuinely submitted cannot be
// confirmed from the UI once its browser closes (bugs.md #518). Revalidation
// is routine before every launch, so this affected every assisted application.
func TestActionForRevalidation_AwaitingReviewAllowsConfirmation(t *testing.T) {
	action := actionForRevalidation("AWAITING_REVIEW", "", "application_ready")
	if action.Code != "open_verified_application" {
		t.Errorf("Code = %q, want open_verified_application", action.Code)
	}
	if !action.RequiresExplicitSubmit {
		t.Error("RequiresExplicitSubmit = false; the Mark Applied control would not render")
	}
	if !action.RequiresBrowser {
		t.Error("RequiresBrowser = false, want true")
	}
}

// Other statuses must not gain a confirmation affordance they never had: a
// CAPTCHA-blocked page has no prepared form to have been submitted.
func TestActionForRevalidation_OtherStatusesUnchanged(t *testing.T) {
	for _, status := range []string{"BLOCKED_CAPTCHA", "MANUAL_REQUIRED", ""} {
		action := actionForRevalidation(status, "", "application_ready")
		if action.RequiresExplicitSubmit {
			t.Errorf("status %q: RequiresExplicitSubmit = true, want false", status)
		}
	}
}

// Bug #521: four Veeva postings of one role in four cities rendered as
// identical cards, and confirming the wrong one wrote a false APPLIED record.
// The projection must carry something the operator can check, and must say how
// many rows it could be confused with.
func TestGetAssistedQueue_DistinguishesPostingsSharingCompanyAndRole(t *testing.T) {
	setupTestDB(t)
	defer teardownTestDB()
	for _, tc := range []struct{ company, title, url string }{
		{"Veeva", "Senior Software Engineer - Python", "https://boards.greenhouse.io/veeva/jobs/293750"},
		{"Veeva", "Senior Software Engineer - Python", "https://boards.greenhouse.io/veeva/jobs/293752"},
		{"Solo Co", "Platform Engineer", "https://boards.greenhouse.io/solo/jobs/11"},
	} {
		if _, err := AddToFunnel(tc.company, tc.title, tc.url, "AWAITING_REVIEW"); err != nil {
			t.Fatal(err)
		}
	}
	// Only one of the two duplicates has been backfilled with a location
	// (bug #524 backfills the rest); the other must still be distinguishable.
	if err := UpdateFunnelIdentity("https://boards.greenhouse.io/veeva/jobs/293750", "Raleigh, NC", true); err != nil {
		t.Fatal(err)
	}
	if _, err := MigrateLegacyAssisted(AssistedMigrationOptions{Confirm: true}); err != nil {
		t.Fatal(err)
	}
	jobs, err := GetAssistedQueue(GetDB())
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 3 {
		t.Fatalf("queue length = %d", len(jobs))
	}
	byRequisition := map[string]AssistedJob{}
	for _, job := range jobs {
		if job.RequisitionID == "" {
			t.Fatalf("job %+v carries no requisition id", job)
		}
		byRequisition[job.RequisitionID] = job
	}
	if len(byRequisition) != 3 {
		t.Fatalf("requisition ids are not unique: %+v", byRequisition)
	}
	if got := byRequisition["293750"]; got.Location != "Raleigh, NC" || got.DuplicateSiblings != 1 {
		t.Fatalf("located duplicate = %+v", got)
	}
	if got := byRequisition["293752"]; got.Location != "" || got.DuplicateSiblings != 1 {
		t.Fatalf("unlocated duplicate = %+v", got)
	}
	if got := byRequisition["11"]; got.DuplicateSiblings != 0 {
		t.Fatalf("unique row reported %d siblings", got.DuplicateSiblings)
	}
	// The posting URL itself stays server-side for a row Career Agent can open
	// on the operator's behalf; only the derived identifier is projected.
	for _, job := range jobs {
		if job.ApplyURL != "" {
			t.Fatalf("job %s leaked a posting URL", job.ID)
		}
	}
}

// A posting the world has moved on from must not keep a clickable card
// (bug #530). #524's backfill asked each board directly and found 18 queued
// postings had been taken down; every one of them stayed in the queue, because
// this projection filtered on assisted_state alone and read jf.status only to
// discard it.
func TestGetAssistedQueue_ExcludesPostingsNoLongerWorkable(t *testing.T) {
	setupTestDB(t)
	defer teardownTestDB()

	const liveURL = "https://boards.greenhouse.io/live/jobs/1"
	const deadURL = "https://boards.greenhouse.io/dead/jobs/2"
	for _, tc := range []struct{ company, title, url string }{
		{"Live Co", "Platform Engineer", liveURL},
		{"Dead Co", "Platform Engineer", deadURL},
	} {
		if _, err := AddToFunnel(tc.company, tc.title, tc.url, "AWAITING_REVIEW"); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := MigrateLegacyAssisted(AssistedMigrationOptions{Confirm: true}); err != nil {
		t.Fatal(err)
	}

	// Both are workable to begin with, so the exclusion below cannot be an
	// artefact of one of them never having been queued.
	jobs, err := GetAssistedQueue(GetDB())
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 2 {
		t.Fatalf("queue length before the posting died = %d, want 2", len(jobs))
	}

	// The posting is taken down. Its assisted_state is untouched — only the
	// funnel status changes, exactly as cmd/backfill-location leaves it.
	if err := UpdateFunnelStatusInvalid(deadURL, InvalidURLReasonExpired); err != nil {
		t.Fatal(err)
	}

	jobs, err = GetAssistedQueue(GetDB())
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 1 {
		t.Fatalf("queue length after the posting died = %d, want 1", len(jobs))
	}
	if jobs[0].Company != "Live Co" {
		t.Fatalf("surviving card = %q, want the live posting", jobs[0].Company)
	}

	// The row itself must survive untouched: it is still a real historical
	// record, and nothing here may imply an application happened.
	var assistedState, provenance string
	if err := GetDB().QueryRow(`SELECT aa.assisted_state, COALESCE(aa.confirmation_provenance, '')
		FROM assisted_applications aa JOIN job_funnel jf ON jf.id = aa.job_id
		WHERE jf.url = ?`, deadURL).Scan(&assistedState, &provenance); err != nil {
		t.Fatal(err)
	}
	if assistedState == "completed" || provenance != "" {
		t.Fatalf("dead posting was marked completed/confirmed (state=%q provenance=%q); that would fabricate an application",
			assistedState, provenance)
	}
}

// The queue projection's scan is positional and was realigned when jf.status
// and jf.status_reason left the SELECT list (bug #530). A silent off-by-one
// there would put a timestamp in a status field and still compile, so pin the
// fields either side of the removal.
func TestGetAssistedQueue_ScanStaysAlignedAfterStatusColumnsRemoved(t *testing.T) {
	setupTestDB(t)
	defer teardownTestDB()
	const alignURL = "https://boards.greenhouse.io/align/jobs/7"
	if _, err := AddToFunnel("Align Co", "Site Reliability Engineer", alignURL, "BLOCKED_CAPTCHA"); err != nil {
		t.Fatal(err)
	}
	if err := UpdateFunnelStatusWithReasonAndScore(alignURL, "BLOCKED_CAPTCHA", "captcha at plan time", 80); err != nil {
		t.Fatal(err)
	}
	if _, err := MigrateLegacyAssisted(AssistedMigrationOptions{Confirm: true}); err != nil {
		t.Fatal(err)
	}
	// Drive the two reasons apart, so Interruption pins the assisted plan's own
	// reason rather than whatever the funnel currently says. (Removing the old
	// duplicate jf.status_reason scan changed no value — the later
	// aa.interruption_reason scan always overwrote it — so this does not test
	// that removal; it tests that the surviving column is the right one.)
	if err := UpdateFunnelStatusWithReasonAndScore(alignURL, "BLOCKED_CAPTCHA", "funnel reason changed later", 80); err != nil {
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
	if job.Company != "Align Co" {
		t.Errorf("Company = %q", job.Company)
	}
	if job.Role != "Site Reliability Engineer" {
		t.Errorf("Role = %q", job.Role)
	}
	if job.Provider != "Greenhouse" {
		t.Errorf("Provider = %q, want Greenhouse", job.Provider)
	}
	// The column immediately after the removal, and the one whose scan target
	// was previously written twice.
	if job.OriginalStatus != "BLOCKED_CAPTCHA" {
		t.Errorf("OriginalStatus = %q, want BLOCKED_CAPTCHA", job.OriginalStatus)
	}
	if job.Interruption != "captcha at plan time" {
		t.Errorf("Interruption = %q, want the assisted plan's own reason, not the funnel's current one", job.Interruption)
	}
	if job.RequisitionID != "7" {
		t.Errorf("RequisitionID = %q, want 7 (posting URL still scanned last-but-one)", job.RequisitionID)
	}
	// LastUpdated is deliberately not asserted here. It is genuinely zero for
	// rows whose last_updated was written by Go — parseAssistedTime cannot read
	// time.Time's default string form, filed as bug #531 — so it would fail for
	// a reason unrelated to alignment. The timestamp column's position is
	// already bracketed by Provider before it and OriginalStatus after it, both
	// asserted above, so a shift there could not pass unnoticed.
}
