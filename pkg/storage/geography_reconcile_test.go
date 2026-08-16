package storage

import (
	"strings"
	"testing"
	"time"

	"github.com/howlcipher/Career_Agent_Core/pkg/config"
)

// geographyProfile is the operator's real configured scope.
func geographyProfile() *config.Profile {
	return &config.Profile{
		RemoteOnly:       true,
		Roles:            []string{"AI Engineer", "Platform Engineer", "Software Engineer"},
		AllowedCountries: []string{"US", "CA"},
	}
}

// The live reproduction, reconstructed: webook / Agentic AI Engineer, advertised
// in Amman, sitting in the actionable queue. Reconciliation must take it out of
// the queue and must not delete it (bugs.md #554).
func TestReconcile_RemovesOutOfScopeJobAndPreservesTheFunnelRow(t *testing.T) {
	setupTestDB(t)
	defer teardownTestDB()
	queueRow(t, "webook", "Agentic AI Engineer", "https://webook.example/jobs/b73794cb19", "Amman, Amman Governorate, Jordan", true)

	report, err := ReconcileAssistedQueueEligibility(GetDB(), geographyProfile())
	if err != nil {
		t.Fatal(err)
	}
	if report.RemovedGeography != 1 {
		t.Fatalf("report = %+v, want RemovedGeography 1", report)
	}
	if report.Remaining != 0 {
		t.Fatalf("report.Remaining = %d, want 0", report.Remaining)
	}

	// Out of the actionable queue...
	var queued int
	if err := GetDB().QueryRow(`SELECT COUNT(*) FROM assisted_applications`).Scan(&queued); err != nil {
		t.Fatal(err)
	}
	if queued != 0 {
		t.Fatalf("out-of-scope job still has %d assisted row(s)", queued)
	}

	// ...but preserved as history, with a reason that says what happened.
	var status, reason string
	if err := GetDB().QueryRow(`SELECT status, COALESCE(status_reason,'') FROM job_funnel WHERE company_name = 'webook'`).Scan(&status, &reason); err != nil {
		t.Fatalf("the funnel row must not be deleted: %v", err)
	}
	if status != "SKIPPED" || reason != config.ReasonOutsideAllowedCountries {
		t.Fatalf("funnel row = %s/%s, want SKIPPED/%s", status, reason, config.ReasonOutsideAllowedCountries)
	}
}

// Unknown geography is held rather than admitted, and is likewise preserved.
func TestReconcile_HoldsUnknownLocationWithoutDeletingIt(t *testing.T) {
	setupTestDB(t)
	defer teardownTestDB()
	queueRow(t, "Unlocatable Co", "Platform Engineer", "https://unlocatable.example/jobs/1", "Remote", true)

	report, err := ReconcileAssistedQueueEligibility(GetDB(), geographyProfile())
	if err != nil {
		t.Fatal(err)
	}
	if report.HeldUnknownLocation != 1 || report.RemovedGeography != 0 {
		t.Fatalf("report = %+v, want HeldUnknownLocation 1", report)
	}
	var status, reason string
	if err := GetDB().QueryRow(`SELECT status, COALESCE(status_reason,'') FROM job_funnel WHERE company_name = 'Unlocatable Co'`).Scan(&status, &reason); err != nil {
		t.Fatal(err)
	}
	if status != "SKIPPED" || reason != config.ReasonLocationUnknown {
		t.Fatalf("funnel row = %s/%s, want SKIPPED/%s", status, reason, config.ReasonLocationUnknown)
	}
}

// In-scope work must survive reconciliation untouched, so the gate cannot
// quietly widen into a general queue outage.
func TestReconcile_KeepsUSAndCanadianJobs(t *testing.T) {
	setupTestDB(t)
	defer teardownTestDB()
	queueRow(t, "US Co", "Platform Engineer", "https://us.example/jobs/1", "Austin, Texas, United States", true)
	queueRow(t, "Canada Co", "Platform Engineer", "https://ca.example/jobs/2", "Toronto, Ontario, Canada", true)
	queueRow(t, "Jordan Co", "Platform Engineer", "https://jo.example/jobs/3", "Amman, Amman Governorate, Jordan", true)

	report, err := ReconcileAssistedQueueEligibility(GetDB(), geographyProfile())
	if err != nil {
		t.Fatal(err)
	}
	if report.Examined != 3 || report.RemovedGeography != 1 || report.Remaining != 2 {
		t.Fatalf("report = %+v", report)
	}
	var remaining int
	if err := GetDB().QueryRow(`SELECT COUNT(*) FROM assisted_applications`).Scan(&remaining); err != nil {
		t.Fatal(err)
	}
	if remaining != 2 {
		t.Fatalf("%d in-scope rows survived, want 2", remaining)
	}
}

// An already-applied job is history. Reconciliation must never rewrite it,
// whatever its location says -- the operator really did apply.
func TestReconcile_NeverTouchesAnAppliedRow(t *testing.T) {
	setupTestDB(t)
	defer teardownTestDB()
	queueRow(t, "Applied Abroad Co", "Platform Engineer", "https://applied.example/jobs/1", "Amman, Amman Governorate, Jordan", true)
	if _, err := GetDB().Exec(`UPDATE job_funnel SET status = 'APPLIED', status_reason = 'submitted' WHERE company_name = 'Applied Abroad Co'`); err != nil {
		t.Fatal(err)
	}

	if _, err := ReconcileAssistedQueueEligibility(GetDB(), geographyProfile()); err != nil {
		t.Fatal(err)
	}
	var status, reason string
	if err := GetDB().QueryRow(`SELECT status, COALESCE(status_reason,'') FROM job_funnel WHERE company_name = 'Applied Abroad Co'`).Scan(&status, &reason); err != nil {
		t.Fatal(err)
	}
	if status != "APPLIED" || reason != "submitted" {
		t.Fatalf("an APPLIED row was rewritten to %s/%s", status, reason)
	}
}

// The launch gate is the last thing between the operator and a browser opened
// on an out-of-scope posting, so it re-checks rather than trusting the queue.
func TestGetAssistedLaunchInfo_RefusesAnOutOfScopeJob(t *testing.T) {
	setupTestDB(t)
	defer teardownTestDB()
	withEligibilityProfile(t, geographyProfile())
	queueRow(t, "webook", "Agentic AI Engineer", "https://webook.example/jobs/b73794cb19", "Amman, Amman Governorate, Jordan", true)
	var id string
	if err := GetDB().QueryRow(`SELECT id FROM job_funnel WHERE company_name = 'webook'`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	if err := RecordAssistedRevalidation(GetDB(), id, "application_ready", time.Now()); err != nil {
		t.Fatal(err)
	}
	_, err := GetAssistedLaunchInfo(GetDB(), id)
	if err == nil {
		t.Fatal("a fully revalidated out-of-scope job must not be launchable")
	}
	if !strings.Contains(err.Error(), config.ReasonOutsideAllowedCountries) {
		t.Fatalf("refusal should name the geography reason, got %q", err)
	}
}

// A missing policy is a refusal, not a pass. This is the exact condition the
// live box was in between 2026-08-12 and this fix.
func TestGetAssistedLaunchInfo_RefusesWhenNoPolicyLoads(t *testing.T) {
	setupTestDB(t)
	defer teardownTestDB()
	orig := resolveEligibilityProfile
	t.Cleanup(func() { resolveEligibilityProfile = orig })
	resolveEligibilityProfile = func() (*config.Profile, error) {
		return nil, errNoTestProfile{}
	}
	queueRow(t, "Any Co", "Platform Engineer", "https://any.example/jobs/1", "Austin, Texas, United States", true)
	var id string
	if err := GetDB().QueryRow(`SELECT id FROM job_funnel WHERE company_name = 'Any Co'`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	if err := RecordAssistedRevalidation(GetDB(), id, "application_ready", time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err := GetAssistedLaunchInfo(GetDB(), id); err == nil {
		t.Fatal("no job may launch while the eligibility policy is unavailable")
	}
}

// Manual promotion is a route into the actionable queue too, and must apply
// the same geography rule as every other one.
func TestPromoteJobToAssisted_RefusesAnOutOfScopeJob(t *testing.T) {
	setupTestDB(t)
	defer teardownTestDB()
	withEligibilityProfile(t, geographyProfile())

	const url = "https://webook.example/jobs/promote"
	if _, err := AddToFunnel("webook", "AI Engineer", url, "PROCESSED_MANUAL"); err != nil {
		t.Fatal(err)
	}
	if err := UpdateFunnelIdentity(url, "Amman, Amman Governorate, Jordan", true); err != nil {
		t.Fatal(err)
	}
	// A perfect fit score is deliberately present: it must not rescue the job.
	if _, err := GetDB().Exec("UPDATE job_funnel SET status_reason = 'find_only_threshold_met', fit_score = 100 WHERE url = ?", url); err != nil {
		t.Fatal(err)
	}
	var id int64
	if err := GetDB().QueryRow("SELECT id FROM job_funnel WHERE url = ?", url).Scan(&id); err != nil {
		t.Fatal(err)
	}
	if err := PromoteJobToAssisted(GetDB(), id, 50); err != ErrJobIneligible {
		t.Fatalf("PromoteJobToAssisted on an out-of-scope job = %v, want ErrJobIneligible", err)
	}
}

// A newly-interrupted automatic pipeline job must pass the current geography
// rule before it is allowed into the assisted queue, not rely solely on the
// earlier discovery-time check or on the next reconciliation pass.
func TestEnsureAssistedPlanForURL_RefusesOutOfScopeInterruption(t *testing.T) {
	setupTestDB(t)
	defer teardownTestDB()
	withEligibilityProfile(t, geographyProfile())

	const url = "https://webook.example/jobs/b73794cb19"
	if _, err := AddToFunnel("webook", "Agentic AI Engineer", url, "BLOCKED_CAPTCHA"); err != nil {
		t.Fatal(err)
	}
	if err := UpdateFunnelIdentity(url, "Amman, Amman Governorate, Jordan", true); err != nil {
		t.Fatal(err)
	}
	if err := EnsureAssistedPlanForURL(url, "BLOCKED_CAPTCHA"); err == nil {
		t.Fatal("expected refusal for out-of-scope interruption")
	}
	var count int
	if err := GetDB().QueryRow("SELECT COUNT(*) FROM assisted_applications").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("assisted row created for out-of-scope job: %d", count)
	}
}

type errNoTestProfile struct{}

func (errNoTestProfile) Error() string { return "no profile configured in this test" }
