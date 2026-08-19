package storage

import (
	"testing"
	"time"

	"github.com/howlcipher/Career_Agent_Core/pkg/config"
)

func withEligibilityProfile(t *testing.T, profile *config.Profile) {
	t.Helper()
	orig := resolveEligibilityProfile
	t.Cleanup(func() { resolveEligibilityProfile = orig })
	resolveEligibilityProfile = func() (*config.Profile, error) { return profile, nil }
}

func testEligibilityProfile() *config.Profile {
	return &config.Profile{
		RemoteOnly: true,
		Roles:      []string{"Software Engineer", "DevOps Engineer", "Platform Engineer", "Site Reliability Engineer"},
	}
}

// queueRow is a small helper that gets a posting all the way into the active
// assisted queue: funnel row, location/remote identity, and a legacy-migrated
// assisted_applications row.
func queueRow(t *testing.T, company, title, url, location string, remote bool) {
	t.Helper()
	if _, err := AddToFunnel(company, title, url, "AWAITING_REVIEW"); err != nil {
		t.Fatalf("AddToFunnel: %v", err)
	}
	if err := UpdateFunnelIdentity(url, location, remote); err != nil {
		t.Fatalf("UpdateFunnelIdentity: %v", err)
	}
	if _, err := MigrateLegacyAssisted(AssistedMigrationOptions{Confirm: true}); err != nil {
		t.Fatalf("MigrateLegacyAssisted: %v", err)
	}
}

func TestReconcileAssistedQueueEligibility_PrunesHybridEntry(t *testing.T) {
	setupTestDB(t)
	defer teardownTestDB()
	queueRow(t, "Hybrid Co", "DevOps Engineer", "https://boards.greenhouse.io/hybridco/jobs/1", "Hybrid - Austin, TX", false)

	report, err := ReconcileAssistedQueueEligibility(GetDB(), testEligibilityProfile())
	if err != nil {
		t.Fatal(err)
	}
	if report.RemovedRemote != 1 || report.Remaining != 0 {
		t.Fatalf("report = %+v, want one remote removal and nothing remaining", report)
	}
	jobs, err := GetAssistedQueue(GetDB())
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 0 {
		t.Fatalf("expected hybrid entry to be pruned, queue = %+v", jobs)
	}
	var status, reason string
	if err := GetDB().QueryRow("SELECT status, status_reason FROM job_funnel WHERE url = ?", "https://boards.greenhouse.io/hybridco/jobs/1").Scan(&status, &reason); err != nil {
		t.Fatal(err)
	}
	if status != "SKIPPED" {
		t.Fatalf("job_funnel status = %q, want SKIPPED (history preserved, not deleted)", status)
	}
}

func TestReconcileAssistedQueueEligibility_PrunesOnSiteEntry(t *testing.T) {
	setupTestDB(t)
	defer teardownTestDB()
	queueRow(t, "Onsite Co", "Platform Engineer", "https://boards.greenhouse.io/onsiteco/jobs/1", "On-site - Denver, CO", false)

	report, err := ReconcileAssistedQueueEligibility(GetDB(), testEligibilityProfile())
	if err != nil {
		t.Fatal(err)
	}
	if report.RemovedRemote != 1 {
		t.Fatalf("report = %+v, want an on-site entry pruned", report)
	}
}

func TestReconcileAssistedQueueEligibility_PrunesAmbiguousRemoteEntry(t *testing.T) {
	setupTestDB(t)
	defer teardownTestDB()
	// No positive remote evidence at all: is_remote is false and nothing in
	// the location text says "remote".
	queueRow(t, "Ambiguous Co", "Platform Engineer", "https://boards.greenhouse.io/ambiguousco/jobs/1", "United States", false)

	report, err := ReconcileAssistedQueueEligibility(GetDB(), testEligibilityProfile())
	if err != nil {
		t.Fatal(err)
	}
	if report.RemovedRemote != 1 {
		t.Fatalf("report = %+v, want the ambiguous-remote entry pruned", report)
	}
}

func TestReconcileAssistedQueueEligibility_PreservesEligibleRemoteEntry(t *testing.T) {
	setupTestDB(t)
	defer teardownTestDB()
	queueRow(t, "Remote Co", "DevOps Engineer", "https://boards.greenhouse.io/remoteco/jobs/1", "Remote - US", true)

	report, err := ReconcileAssistedQueueEligibility(GetDB(), testEligibilityProfile())
	if err != nil {
		t.Fatal(err)
	}
	if report.Remaining != 1 || report.RemovedRemote != 0 || report.RemovedRole != 0 {
		t.Fatalf("report = %+v, want the fully-remote eligible entry preserved", report)
	}
	jobs, err := GetAssistedQueue(GetDB())
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 1 {
		t.Fatalf("expected the eligible entry to remain queued, got %+v", jobs)
	}
}

func TestReconcileAssistedQueueEligibility_PreservesSoftwareEngineerWithPlatformDuties(t *testing.T) {
	setupTestDB(t)
	defer teardownTestDB()
	// Title alone can't show platform/DevOps duties once the description is
	// gone (job_funnel never stores it), so a generic "Software Engineer"
	// title is deliberately never pruned for role mismatch.
	queueRow(t, "Generic Co", "Software Engineer", "https://boards.greenhouse.io/genericco/jobs/1", "Remote - US", true)

	report, err := ReconcileAssistedQueueEligibility(GetDB(), testEligibilityProfile())
	if err != nil {
		t.Fatal(err)
	}
	if report.RemovedRole != 0 || report.Remaining != 1 {
		t.Fatalf("report = %+v, want Software Engineer preserved", report)
	}
}

func TestReconcileAssistedQueueEligibility_PrunesRemovedRoleTitle(t *testing.T) {
	setupTestDB(t)
	defer teardownTestDB()
	// "Data Engineer" was deliberately removed from the target role list and
	// carries no distinctive DevOps/platform/automation word of its own.
	queueRow(t, "Data Co", "Data Engineer", "https://boards.greenhouse.io/dataco/jobs/1", "Remote - US", true)

	report, err := ReconcileAssistedQueueEligibility(GetDB(), testEligibilityProfile())
	if err != nil {
		t.Fatal(err)
	}
	if report.RemovedRole != 1 {
		t.Fatalf("report = %+v, want the removed-title entry pruned for role mismatch", report)
	}
}

// Dogfood stale rows from bugs.md #556/#557 must be pruned when the current
// canonical policy rejects them, and the underlying funnel history must survive
// as a SKIPPED row rather than being deleted.
func TestReconcileAssistedQueueEligibility_PrunesDogfoodStaleRows(t *testing.T) {
	setupTestDB(t)
	defer teardownTestDB()

	queueRow(t, "Atmosera", "Cloud Support Administrator (Remote US)", "https://boards.greenhouse.io/atmosera/jobs/315616", "Remote - US", true)
	queueRow(t, "ThinkAhead", "Senior Technical Consultant - Microsoft Power Platform", "https://boards.greenhouse.io/thinkahead/jobs/304767", "United States", true)

	report, err := ReconcileAssistedQueueEligibility(GetDB(), testEligibilityProfile())
	if err != nil {
		t.Fatal(err)
	}
	if report.RemovedRole != 2 || report.Remaining != 0 {
		t.Fatalf("report = %+v, want both dogfood rows removed for role mismatch", report)
	}

	jobs, err := GetAssistedQueue(GetDB())
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 0 {
		t.Fatalf("expected no actionable rows after reconciliation, got %+v", jobs)
	}

	var status, reason string
	if err := GetDB().QueryRow("SELECT status, status_reason FROM job_funnel WHERE url = ?", "https://boards.greenhouse.io/atmosera/jobs/315616").Scan(&status, &reason); err != nil {
		t.Fatal(err)
	}
	if status != "SKIPPED" || reason != config.ReasonRoleTrackMismatch {
		t.Fatalf("funnel row = %q/%q, want SKIPPED/%s", status, reason, config.ReasonRoleTrackMismatch)
	}
}

func TestReconcileAssistedQueueEligibility_PreservesAlreadyAppliedHistory(t *testing.T) {
	setupTestDB(t)
	defer teardownTestDB()
	const url = "https://boards.greenhouse.io/appliedco/jobs/1"
	// A hybrid job that was already actually submitted must remain untouched
	// history, not be retroactively pruned.
	if _, err := AddToFunnel("Applied Co", "DevOps Engineer", url, "APPLIED"); err != nil {
		t.Fatal(err)
	}
	if err := UpdateFunnelIdentity(url, "Hybrid - Austin, TX", false); err != nil {
		t.Fatal(err)
	}
	if err := RecordApplicationInDB("Applied Co", "DevOps Engineer", url); err != nil {
		t.Fatal(err)
	}

	report, err := ReconcileAssistedQueueEligibility(GetDB(), testEligibilityProfile())
	if err != nil {
		t.Fatal(err)
	}
	if report.Examined != 0 {
		t.Fatalf("report = %+v, an already-applied row must never be examined as active queue state", report)
	}
	var status string
	if err := GetDB().QueryRow("SELECT status FROM job_funnel WHERE url = ?", url).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "APPLIED" {
		t.Fatalf("status = %q, want APPLIED preserved", status)
	}
}

// GetAssistedQueue must itself re-run the gate, so a persisted row cannot
// reappear in the served queue just because it survived until the next poll
// without an explicit reconciliation call.
func TestGetAssistedQueue_ReconcilesOnEveryRead(t *testing.T) {
	setupTestDB(t)
	defer teardownTestDB()
	withEligibilityProfile(t, testEligibilityProfile())

	queueRow(t, "Hybrid Co", "DevOps Engineer", "https://boards.greenhouse.io/hybridco2/jobs/1", "Hybrid - Austin, TX", false)
	queueRow(t, "Remote Co", "DevOps Engineer", "https://boards.greenhouse.io/remoteco2/jobs/1", "Remote - US", true)

	jobs, err := GetAssistedQueue(GetDB())
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 1 || jobs[0].Company != "Remote Co" {
		t.Fatalf("queue = %+v, want only the fully-remote entry served", jobs)
	}

	// A second poll must not resurrect the pruned row from stale state.
	jobs, err = GetAssistedQueue(GetDB())
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 1 || jobs[0].Company != "Remote Co" {
		t.Fatalf("second poll queue = %+v, pruned row must not reappear", jobs)
	}
}

// A row this reconciliation already rejected and demoted to SKIPPED must not
// be able to re-enter the active queue just because something re-inserts an
// assisted_applications row for it (e.g. a stale cache restore, or a second
// migration pass) without first passing eligibility again.
func TestReconcileAssistedQueueEligibility_RejectedJobCannotReenterQueue(t *testing.T) {
	setupTestDB(t)
	defer teardownTestDB()
	profile := testEligibilityProfile()
	withEligibilityProfile(t, profile)
	const url = "https://boards.greenhouse.io/rejectedco/jobs/1"
	queueRow(t, "Rejected Co", "DevOps Engineer", url, "Hybrid - Austin, TX", false)

	if _, err := ReconcileAssistedQueueEligibility(GetDB(), profile); err != nil {
		t.Fatal(err)
	}
	jobs, err := GetAssistedQueue(GetDB())
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 0 {
		t.Fatalf("expected the hybrid row pruned on first pass, got %+v", jobs)
	}

	// Simulate a stale path re-inserting the assisted_applications row
	// without going through fresh discovery (the job_funnel row itself is
	// still SKIPPED with the hybrid location it always had).
	var id int64
	if err := GetDB().QueryRow("SELECT id FROM job_funnel WHERE url = ?", url).Scan(&id); err != nil {
		t.Fatal(err)
	}
	if _, err := GetDB().Exec(`UPDATE job_funnel SET status = 'AWAITING_REVIEW' WHERE id = ?`, id); err != nil {
		t.Fatal(err)
	}
	if _, err := GetDB().Exec(`INSERT INTO assisted_applications (job_id, original_status, next_action_code, is_legacy, created_at, updated_at)
		VALUES (?, 'AWAITING_REVIEW', 'review_and_submit', 0, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`, id); err != nil {
		t.Fatal(err)
	}

	// GetAssistedQueue must reconcile again before serving anything, so the
	// resurrected row is pruned right back out.
	jobs, err = GetAssistedQueue(GetDB())
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 0 {
		t.Fatalf("resurrected ineligible row must not bypass eligibility, got %+v", jobs)
	}
}

// GetAssistedLaunchInfo is the actual browser-opening choke point (cmd/assist
// and the dashboard's launch handler both go through it), so it must refuse
// an ineligible job even if it is somehow still sitting in
// assisted_applications -- closing the narrow window between a queue render
// and a click.
func TestGetAssistedLaunchInfo_RefusesIneligibleJob(t *testing.T) {
	setupTestDB(t)
	defer teardownTestDB()
	withEligibilityProfile(t, testEligibilityProfile())

	const url = "https://boards.greenhouse.io/launchco/jobs/1"
	if _, err := AddToFunnel("Launch Co", "DevOps Engineer", url, "BLOCKED_CAPTCHA"); err != nil {
		t.Fatal(err)
	}
	if err := UpdateFunnelIdentity(url, "Hybrid - Austin, TX", false); err != nil {
		t.Fatal(err)
	}
	if _, err := MigrateLegacyAssisted(AssistedMigrationOptions{Confirm: true}); err != nil {
		t.Fatal(err)
	}
	var id string
	if err := GetDB().QueryRow("SELECT id FROM job_funnel WHERE url = ?", url).Scan(&id); err != nil {
		t.Fatal(err)
	}
	if err := RecordAssistedRevalidation(GetDB(), id, "application_ready", time.Now()); err != nil {
		t.Fatal(err)
	}

	if _, err := GetAssistedLaunchInfo(GetDB(), id); err == nil {
		t.Fatal("expected a hybrid job to be refused a browser launch")
	}
}

func TestPromoteJobToAssisted_RejectsIneligibleJob(t *testing.T) {
	setupTestDB(t)
	defer teardownTestDB()
	withEligibilityProfile(t, testEligibilityProfile())

	const url = "https://boards.greenhouse.io/promoteco/jobs/1"
	if _, err := AddToFunnel("Promote Co", "DevOps Engineer", url, "PROCESSED_MANUAL"); err != nil {
		t.Fatal(err)
	}
	if err := UpdateFunnelIdentity(url, "Hybrid - Austin, TX", false); err != nil {
		t.Fatal(err)
	}
	if _, err := GetDB().Exec("UPDATE job_funnel SET status_reason = 'find_only_threshold_met', fit_score = 90 WHERE url = ?", url); err != nil {
		t.Fatal(err)
	}
	var id int64
	if err := GetDB().QueryRow("SELECT id FROM job_funnel WHERE url = ?", url).Scan(&id); err != nil {
		t.Fatal(err)
	}

	if err := PromoteJobToAssisted(GetDB(), id, 50); err != ErrJobIneligible {
		t.Fatalf("PromoteJobToAssisted error = %v, want ErrJobIneligible", err)
	}
}

func TestPromoteJobToAssisted_AllowsEligibleJob(t *testing.T) {
	setupTestDB(t)
	defer teardownTestDB()
	withEligibilityProfile(t, testEligibilityProfile())

	const url = "https://boards.greenhouse.io/promoteco2/jobs/1"
	if _, err := AddToFunnel("Promote Co", "DevOps Engineer", url, "PROCESSED_MANUAL"); err != nil {
		t.Fatal(err)
	}
	if err := UpdateFunnelIdentity(url, "Remote - US", true); err != nil {
		t.Fatal(err)
	}
	if _, err := GetDB().Exec("UPDATE job_funnel SET status_reason = 'find_only_threshold_met', fit_score = 90 WHERE url = ?", url); err != nil {
		t.Fatal(err)
	}
	var id int64
	if err := GetDB().QueryRow("SELECT id FROM job_funnel WHERE url = ?", url).Scan(&id); err != nil {
		t.Fatal(err)
	}

	if err := PromoteJobToAssisted(GetDB(), id, 50); err != nil {
		t.Fatalf("PromoteJobToAssisted error = %v, want success", err)
	}
}
