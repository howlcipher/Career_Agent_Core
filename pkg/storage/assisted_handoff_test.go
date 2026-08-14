package storage

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestAssistedBrowserRejectionReason_MatchesRegisteredATSOnly(t *testing.T) {
	for _, tc := range []struct {
		name     string
		url      string
		rejected bool
	}{
		{"lever posting host", "https://jobs.lever.co/veeva/abc-123/apply", true},
		{"lever apex domain", "https://lever.co/veeva/abc-123", true},
		{"lever host is case-insensitive", "https://JOBS.LEVER.CO/veeva/abc-123", true},
		{"fully qualified lever host", "https://jobs.lever.co./veeva/abc-123", true},
		{"greenhouse is unaffected", "https://boards.greenhouse.io/grafanalabs/jobs/1", false},
		{"ashby is unaffected", "https://jobs.ashbyhq.com/example/1", false},
		{"a lookalike domain must not match", "https://notlever.co/jobs/1", false},
		{"lever as a path segment must not match", "https://jobs.example.com/lever.co/1", false},
		{"empty URL", "", false},
		{"non-HTTP scheme", "file:///jobs.lever.co/apply", false},
		// A URL no parser accepts is also a URL no browser can open, so it
		// fails open rather than being special-cased into the registry.
		{"unparseable URL", "https://jobs.lever.co/%zz", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reason := AssistedBrowserRejectionReason(tc.url)
			if tc.rejected && reason == "" {
				t.Fatalf("%q should be rejected for the assisted browser", tc.url)
			}
			if !tc.rejected && reason != "" {
				t.Fatalf("%q should be allowed, got reason %q", tc.url, reason)
			}
		})
	}
}

// "This ATS refuses a submission from the assisted browser" and "this ATS
// cannot be read at all" were one field until bugs.md #545, and collapsing them
// left the Copy Application Packet empty for the applications that depend on it
// most: an ATS Career Agent may not submit to is exactly the one whose questions
// the operator has to answer by hand.
//
// Both directions matter. Lever must stay submit-rejected -- that is bug #520,
// and weakening it would put Career Agent back in front of a Submit the
// employer rejects. Lever must also be readable, because preflight fills
// nothing and submits nothing.
func TestPreflightRefusalReason_IsSeparateFromTheSubmitRejection(t *testing.T) {
	const lever = "https://jobs.lever.co/veeva/abc-123"

	if AssistedBrowserRejectionReason(lever) == "" {
		t.Error("Lever must still be refused the assisted browser (bug #520)")
	}
	if reason := PreflightRefusalReason(lever); reason != "" {
		t.Errorf("Lever's form is public and reads cleanly; preflight must not refuse it, got %q", reason)
	}

	// An ATS in neither category answers no to both questions.
	const greenhouse = "https://boards.greenhouse.io/grafanalabs/jobs/1"
	if AssistedBrowserRejectionReason(greenhouse) != "" || PreflightRefusalReason(greenhouse) != "" {
		t.Error("an unregistered ATS must be refused by neither gate")
	}

	// The read refusal still exists as a mechanism; it is simply unclaimed. A
	// registry entry that sets it must be refused inspection.
	original := assistedBrowserRejections
	t.Cleanup(func() { assistedBrowserRejections = original })
	assistedBrowserRejections = []assistedBrowserRejection{
		{domainSuffix: "example.com", reason: "sign-in wall in front of the form", blocksPreflight: true},
	}
	if PreflightRefusalReason("https://jobs.example.com/1") == "" {
		t.Error("an ATS marked unreadable must be refused inspection")
	}
}

// A rejected ATS must be refused at the single gate both the dashboard handler
// and cmd/assist pass through, so no route can open a browser whose submission
// the employer will refuse (bug #520).
func TestGetAssistedLaunchInfo_RefusesATSThatRejectsTheAssistedBrowser(t *testing.T) {
	setupTestDB(t)
	defer teardownTestDB()
	if _, err := AddToFunnel("Veeva", "Platform Engineer", "https://jobs.lever.co/veeva/abc-123", "AWAITING_REVIEW"); err != nil {
		t.Fatal(err)
	}
	if _, err := MigrateLegacyAssisted(AssistedMigrationOptions{Confirm: true}); err != nil {
		t.Fatal(err)
	}
	var id string
	if err := GetDB().QueryRow("SELECT id FROM job_funnel WHERE company_name = 'Veeva'").Scan(&id); err != nil {
		t.Fatal(err)
	}
	if err := RecordAssistedRevalidation(GetDB(), id, "application_ready", time.Now()); err != nil {
		t.Fatal(err)
	}
	_, err := GetAssistedLaunchInfo(GetDB(), id)
	if !errors.Is(err, ErrAssistedBrowserRejected) {
		t.Fatalf("a fully revalidated Lever plan must still refuse a browser, got %v", err)
	}
	if !strings.Contains(err.Error(), "#520") {
		t.Fatalf("refusal should carry the observed evidence, got %q", err)
	}
}

// The same fully revalidated plan on an unaffected ATS must still launch, so
// the refusal cannot silently widen into a general assisted-apply outage.
func TestGetAssistedLaunchInfo_StillLaunchesUnaffectedATS(t *testing.T) {
	setupTestDB(t)
	defer teardownTestDB()
	if _, err := AddToFunnel("Grafana Labs", "Platform Engineer", "https://boards.greenhouse.io/grafanalabs/jobs/1", "AWAITING_REVIEW"); err != nil {
		t.Fatal(err)
	}
	if _, err := MigrateLegacyAssisted(AssistedMigrationOptions{Confirm: true}); err != nil {
		t.Fatal(err)
	}
	var id string
	if err := GetDB().QueryRow("SELECT id FROM job_funnel WHERE company_name = 'Grafana Labs'").Scan(&id); err != nil {
		t.Fatal(err)
	}
	if err := RecordAssistedRevalidation(GetDB(), id, "application_ready", time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err := GetAssistedLaunchInfo(GetDB(), id); err != nil {
		t.Fatalf("Greenhouse must remain launchable in the assisted browser: %v", err)
	}
}

func TestGetAssistedQueue_HandsRejectedATSToTheOperatorsOwnBrowser(t *testing.T) {
	setupTestDB(t)
	defer teardownTestDB()
	const leverURL = "https://jobs.lever.co/veeva/abc-123"
	if _, err := AddToFunnel("Veeva", "Platform Engineer", leverURL, "AWAITING_REVIEW"); err != nil {
		t.Fatal(err)
	}
	if _, err := AddToFunnel("Grafana Labs", "Platform Engineer", "https://boards.greenhouse.io/grafanalabs/jobs/1", "AWAITING_REVIEW"); err != nil {
		t.Fatal(err)
	}
	if _, err := MigrateLegacyAssisted(AssistedMigrationOptions{Confirm: true}); err != nil {
		t.Fatal(err)
	}
	jobs, err := GetAssistedQueue(GetDB())
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 2 {
		t.Fatalf("queue length = %d", len(jobs))
	}
	var lever, greenhouse *AssistedJob
	for i := range jobs {
		switch jobs[i].Company {
		case "Veeva":
			lever = &jobs[i]
		case "Grafana Labs":
			greenhouse = &jobs[i]
		}
	}
	if lever == nil || greenhouse == nil {
		t.Fatalf("queue = %+v", jobs)
	}

	if lever.NextAction.Code != "open_in_own_browser" {
		t.Fatalf("Lever next action = %+v", lever.NextAction)
	}
	if lever.NextAction.RequiresBrowser {
		t.Fatal("the hand-off must never ask the dashboard to open the assisted browser")
	}
	if !lever.NextAction.RequiresExplicitSubmit {
		t.Fatal("the operator must still be able to confirm the application was received")
	}
	if lever.NextAction.CanContinue {
		t.Fatal("there is no assisted browser to continue in")
	}
	if lever.ApplyURL != leverURL {
		t.Fatalf("hand-off must carry the posting URL, got %q", lever.ApplyURL)
	}
	if !strings.Contains(lever.PriorityReason, "own browser") {
		t.Fatalf("priority reason = %q", lever.PriorityReason)
	}

	// Every other row keeps the guarded browser and keeps its URL server-side.
	if greenhouse.NextAction.Code == "open_in_own_browser" {
		t.Fatalf("Greenhouse next action = %+v", greenhouse.NextAction)
	}
	if greenhouse.ApplyURL != "" {
		t.Fatalf("an unaffected row must not leak its URL, got %q", greenhouse.ApplyURL)
	}
}
