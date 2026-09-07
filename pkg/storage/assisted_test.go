package storage

import (
	"github.com/howlcipher/Career_Agent_Core/pkg/config"
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
	// This test asserts the revalidation gate, so the fail-closed eligibility
	// policy (bugs.md #554) must not be what refuses the launch.
	withEligibilityProfile(t, &config.Profile{Roles: []string{"Engineer"}})
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
	if _, err := ConfirmAssistedSubmission(GetDB(), id); err != nil {
		t.Fatal(err)
	}
	var status, provenance string
	if err := GetDB().QueryRow(`SELECT jf.status, aa.confirmation_provenance FROM job_funnel jf JOIN assisted_applications aa ON aa.job_id = jf.id WHERE jf.id = ?`, id).Scan(&status, &provenance); err != nil {
		t.Fatal(err)
	}
	if status != "APPLIED" || provenance != "manual_user_confirmation" {
		t.Fatalf("status=%q provenance=%q", status, provenance)
	}
	if _, err := ConfirmAssistedSubmission(GetDB(), id); err == nil {
		t.Fatal("second confirmation must conflict")
	}
}

func TestMarkAssistedNotFound_CompletesPlanAndMarksInvalidURL(t *testing.T) {
	setupTestDB(t)
	defer teardownTestDB()
	url := "https://expired.example/jobs/notfound"
	if _, err := AddToFunnel("Expired Co", "Engineer", url, "MANUAL_REQUIRED"); err != nil {
		t.Fatal(err)
	}
	if _, err := MigrateLegacyAssisted(AssistedMigrationOptions{Confirm: true}); err != nil {
		t.Fatal(err)
	}
	var id string
	if err := GetDB().QueryRow("SELECT id FROM job_funnel WHERE url = ?", url).Scan(&id); err != nil {
		t.Fatal(err)
	}
	if err := MarkAssistedNotFound(GetDB(), id); err != nil {
		t.Fatal(err)
	}
	var status, provenance, reason string
	if err := GetDB().QueryRow(`SELECT jf.status, jf.status_reason, aa.confirmation_provenance
		FROM job_funnel jf JOIN assisted_applications aa ON aa.job_id = jf.id WHERE jf.id = ?`, id).
		Scan(&status, &reason, &provenance); err != nil {
		t.Fatal(err)
	}
	if status != "INVALID_URL" || reason != InvalidURLReasonExpired || provenance != "manual_posting_not_found" {
		t.Fatalf("status=%q reason=%q provenance=%q", status, reason, provenance)
	}
	if err := MarkAssistedNotFound(GetDB(), id); err == nil {
		t.Fatal("second not-found must conflict")
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

// permissiveAssistedProfile is used by tests whose subject is the assisted
// plan mechanics, not the eligibility gate. It accepts the generic "Engineer"
// title used by those fixtures and disables geography/remote restrictions.
func permissiveAssistedProfile(t *testing.T) {
	t.Helper()
	withEligibilityProfile(t, &config.Profile{RemoteOnly: false, Roles: []string{"Engineer"}})
}

func TestEnsureAssistedPlanForURL_CreatesNewInterruptionPlanOnce(t *testing.T) {
	setupTestDB(t)
	defer teardownTestDB()
	permissiveAssistedProfile(t)
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
	permissiveAssistedProfile(t)

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
	permissiveAssistedProfile(t)

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
	permissiveAssistedProfile(t)

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
	permissiveAssistedProfile(t)

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
	permissiveAssistedProfile(t)

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

// A revalidated handoff of any status ConfirmAssistedSubmission itself
// accepts must still expose the explicit-submit affordance. Without it the
// dashboard never renders "I saw a confirmation — Mark Applied", so an
// application the operator genuinely submitted cannot be confirmed from the
// UI once its browser closes (bugs.md #518, then #557). #518 fixed this for
// AWAITING_REVIEW only; #557 found a BLOCKED_CAPTCHA-origin job (Wurl, job
// 1240, live dogfood run 2026-08-20) reach and submit a real form the exact
// same way and lose the exact same control the exact same way, which
// falsified the assumption the old version of this test asserted — that a
// CAPTCHA-blocked page never has a prepared form to have been submitted. It
// can, once the CAPTCHA is solved and the browser is later closed without the
// guided Continue step.
func TestActionForRevalidation_EligibleStatusesAllowConfirmation(t *testing.T) {
	for _, status := range []string{"AWAITING_REVIEW", "BLOCKED_CAPTCHA", "MANUAL_REQUIRED"} {
		action := actionForRevalidation(status, "", "application_ready")
		if action.Code != "open_verified_application" {
			t.Errorf("status %q: Code = %q, want open_verified_application", status, action.Code)
		}
		if !action.RequiresExplicitSubmit {
			t.Errorf("status %q: RequiresExplicitSubmit = false; the Mark Applied control would not render", status)
		}
		if !action.RequiresBrowser {
			t.Errorf("status %q: RequiresBrowser = false, want true", status)
		}
	}
}

// A status ConfirmAssistedSubmission itself would refuse must not gain a
// confirmation affordance either — there is no eligible funnel row for it to
// commit against.
func TestActionForRevalidation_IneligibleStatusUnchanged(t *testing.T) {
	action := actionForRevalidation("", "", "application_ready")
	if action.RequiresExplicitSubmit {
		t.Error("RequiresExplicitSubmit = true for an ineligible status, want false")
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
	// LastUpdated could not be asserted here until bug #531 was fixed: the row
	// above is written by Go, and parseAssistedTime had no layout for
	// time.Time's default string form, so the field was genuinely zero for a
	// reason unrelated to alignment. Now that it reads, assert it — a real
	// value here pins the timestamp column's own position rather than relying
	// on Provider before it and OriginalStatus after it to bracket a shift.
	if job.LastUpdated.IsZero() {
		t.Error("LastUpdated is the zero time; the scanned column parsed to nothing (bug #531)")
	}
}

// Every shape job_funnel.last_updated and assisted_applications.updated_at were
// observed to hold in the live database on 2026-08-07, plus the failure path.
// The cases are written as literal stored strings rather than as formatted
// times, because the defect bug #531 recorded was precisely that the layout
// list and the writers had drifted apart — deriving the input from the layouts
// under test would assume away the thing being tested.
func TestParseAssistedTime_ReadsEveryStoredTimestampShape(t *testing.T) {
	for _, tc := range []struct {
		name   string
		stored string
		want   string
		wantOK bool
	}{
		{
			// The form 12046 of 12980 job_funnel rows held, and the one that
			// had no layout: Go writers pass a time.Time to db.Exec and the
			// driver stores its default String() form.
			name:   "go time.Time String form",
			stored: "2026-08-01 12:18:28.408800856 +0000 UTC",
			want:   "2026-08-01T12:18:28.408800856Z",
			wantOK: true,
		},
		{
			// Same writer, non-UTC zone. The numeric offset drives the
			// conversion, not the abbreviation, so this must land at 18:57Z.
			name:   "go time.Time String form outside UTC",
			stored: "2026-08-06 14:57:49.676108047 -0400 EDT",
			want:   "2026-08-06T18:57:49.676108047Z",
			wantOK: true,
		},
		{
			name:   "go time.Time String form with no fractional seconds",
			stored: "2026-08-06 14:57:49 +0000 UTC",
			want:   "2026-08-06T14:57:49Z",
			wantOK: true,
		},
		{
			name:   "colon-separated offset",
			stored: "2026-07-26 03:19:43.453588535+00:00",
			want:   "2026-07-26T03:19:43.453588535Z",
			wantOK: true,
		},
		{
			// SQLite's own CURRENT_TIMESTAMP, which is UTC and zone-less.
			name:   "sqlite CURRENT_TIMESTAMP",
			stored: "2026-08-06 19:06:02",
			want:   "2026-08-06T19:06:02Z",
			wantOK: true,
		},
		{
			name:   "rfc3339",
			stored: "2026-08-06T14:57:49Z",
			want:   "2026-08-06T14:57:49Z",
			wantOK: true,
		},
		{
			// 152 job_funnel rows hold this. It is an absent timestamp, not a
			// malformed one, so the zero time is the right answer — but it is
			// still reported as unread, which is what keeps the caller from
			// logging it as a layout defect.
			name:   "empty",
			stored: "",
			wantOK: false,
		},
		{
			name:   "unreadable",
			stored: "last Tuesday",
			wantOK: false,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := parseAssistedTime(tc.stored)
			if ok != tc.wantOK {
				t.Fatalf("parseAssistedTime(%q) ok = %v, want %v", tc.stored, ok, tc.wantOK)
			}
			if !tc.wantOK {
				if !got.IsZero() {
					t.Fatalf("parseAssistedTime(%q) = %v on a miss, want the zero time", tc.stored, got)
				}
				return
			}
			if formatted := got.Format(time.RFC3339Nano); formatted != tc.want {
				t.Fatalf("parseAssistedTime(%q) = %s, want %s", tc.stored, formatted, tc.want)
			}
			if got.Location() != time.UTC {
				t.Errorf("parseAssistedTime(%q) location = %v, want UTC", tc.stored, got.Location())
			}
		})
	}
}

// The end-to-end shape of bug #531: a row whose timestamp a real Go writer put
// in the column must reach the queue card as a real time. The unit test above
// pins the layouts in isolation; this pins the whole path that broke — a real
// writer's stored bytes, read back through the projection's COALESCE, which
// erases the decltype the driver's own time conversion depends on.
func TestGetAssistedQueue_ServesRealTimestampsForRowsWrittenByGo(t *testing.T) {
	setupTestDB(t)
	defer teardownTestDB()

	const url = "https://boards.greenhouse.io/timestamps/jobs/9"
	if _, err := AddToFunnel("Timestamp Co", "Platform Engineer", url, "AWAITING_REVIEW"); err != nil {
		t.Fatal(err)
	}
	if _, err := MigrateLegacyAssisted(AssistedMigrationOptions{Confirm: true}); err != nil {
		t.Fatal(err)
	}
	before := time.Now().UTC().Add(-time.Minute)
	// A Go writer, not a hand-written literal: this is the path that produced
	// every one of the 12046 unreadable rows.
	if err := UpdateFunnelStatusWithReasonAndScore(url, "AWAITING_REVIEW", "ready for review", 80); err != nil {
		t.Fatal(err)
	}

	// Confirm the premise rather than assuming it — if the driver ever starts
	// storing RFC3339 instead, this test would otherwise keep passing while
	// covering nothing. The CAST is load-bearing: reading last_updated bare
	// would let the driver parse the DATETIME column and hand back a time.Time,
	// which is exactly the conversion the queue's COALESCE defeats, so the
	// unconverted text is what has to be asserted here.
	var stored string
	if err := GetDB().QueryRow(`SELECT CAST(last_updated AS TEXT) FROM job_funnel WHERE url = ?`, NormalizeURL(url)).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(stored, " UTC") {
		t.Fatalf("stored last_updated = %q, want time.Time's default String form; this test no longer covers the writer it was written for", stored)
	}

	jobs, err := GetAssistedQueue(GetDB())
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 1 {
		t.Fatalf("queue length = %d, want 1", len(jobs))
	}
	if jobs[0].LastUpdated.IsZero() {
		t.Fatal("LastUpdated is the zero time; the card would serve 0001-01-01T00:00:00Z (bug #531)")
	}
	if jobs[0].LastUpdated.Before(before) {
		t.Errorf("LastUpdated = %v, want a time at or after the write at %v", jobs[0].LastUpdated, before)
	}
}

// The projection-level counterpart to
// TestAssistedDocumentExists_CoverLetterReadinessFollowsTheMasterLetter, which
// had to drive the helper directly because this test could not be written at
// all before bug #527: GetAssistedQueue resolves both readiness fields from
// inside its own open rows iteration, and under the old `:memory:` harness
// every such nested lookup failed, so both fields were unconditionally false
// and any assertion here would have been an assertion about the failure path.
func TestGetAssistedQueue_ReadinessFieldsFollowTheDocumentsOnDisk(t *testing.T) {
	t.Chdir(t.TempDir())
	setupTestDB(t)
	defer teardownTestDB()
	permissiveAssistedProfile(t)

	const masterCoverLetter = "Omni_CoverLetter.pdf"
	const postingURL = "https://captcha.example/jobs/queue-readiness"
	origResolver := resolveMasterCoverLetter
	defer func() { resolveMasterCoverLetter = origResolver }()
	resolveMasterCoverLetter = func() string { return masterCoverLetter }

	if _, err := AddToFunnel("Readiness Co", "Engineer", postingURL, "BLOCKED_CAPTCHA"); err != nil {
		t.Fatal(err)
	}
	if err := EnsureAssistedPlanForURL(postingURL, "BLOCKED_CAPTCHA"); err != nil {
		t.Fatal(err)
	}

	// Neither master document exists yet, so both fields must be false for a
	// reason the queue can act on rather than because the lookup errored.
	jobs, err := GetAssistedQueue(GetDB())
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 1 {
		t.Fatalf("queue length = %d, want 1", len(jobs))
	}
	if jobs[0].ResumeReady {
		t.Error("ResumeReady = true with no master résumé on disk")
	}
	if jobs[0].CoverLetterReady {
		t.Error("CoverLetterReady = true with no master cover letter on disk")
	}

	if err := os.WriteFile(MasterResumePath, []byte("%PDF-1.7 master resume content"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(masterCoverLetter, []byte("%PDF-1.7 master cover letter content"), 0o600); err != nil {
		t.Fatal(err)
	}

	jobs, err = GetAssistedQueue(GetDB())
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 1 {
		t.Fatalf("queue length = %d, want 1", len(jobs))
	}
	if !jobs[0].ResumeReady {
		t.Error("ResumeReady = false after the master résumé was written; the card would refuse a document it has")
	}
	if !jobs[0].CoverLetterReady {
		t.Error("CoverLetterReady = false after the master cover letter was written; the card would refuse a document it has")
	}
}

// Canary for bug #527's harness invariant. setupTestDB must hand every
// connection in the pool the same schema, which a `:memory:` database cannot
// do — a second connection opens its own empty database instead. The failure
// mode is silent: nothing errors at setup, and a test whose assertions happen
// to expect "not found" keeps passing while covering nothing. This asserts the
// invariant directly so a future return to `:memory:` fails here, loudly and
// in one place, rather than quietly weakening every test that reads from
// inside an open iteration.
func TestSetupTestDB_ServesQueriesNestedInsideAnOpenIteration(t *testing.T) {
	setupTestDB(t)
	defer teardownTestDB()
	if _, err := AddToFunnel("Nested Co", "Engineer", "https://nested.example/jobs/1", "DISCOVERED"); err != nil {
		t.Fatal(err)
	}

	rows, err := GetDB().Query(`SELECT url FROM job_funnel`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var url string
		if err := rows.Scan(&url); err != nil {
			t.Fatal(err)
		}
		// Issued while rows is still open, so it must take a second pooled
		// connection — the exact shape GetAssistedQueue uses for its readiness
		// lookups.
		var count int
		if err := GetDB().QueryRow(`SELECT COUNT(*) FROM job_funnel WHERE url = ?`, url).Scan(&count); err != nil {
			t.Fatalf("nested query failed, so setupTestDB is not sharing one schema across the pool (bug #527): %v", err)
		}
		if count != 1 {
			t.Errorf("nested count for %s = %d, want 1", url, count)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
}
