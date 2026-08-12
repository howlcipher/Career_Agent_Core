package tracker

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/howlcipher/Career_Agent_Core/pkg/storage"
)

func TestClassifyEmail(t *testing.T) {
	tests := []struct {
		name    string
		subject string
		body    string
		want    string
	}{
		// Bug #20's live false positives must classify as nothing
		{"google payment receipt", "google: we've received your payment for 7552-6381-4439", "thank you. next steps: no action needed.", ""},
		{"linkedin sent confirmation", "william, your application was sent to clearlyagile", "prepare for interviews with these tips", ""},
		{"automated message", "interview scheduling", "this is an automated message about your interview", ""},
		// Bug #536: live promotional/entertainment emails that triggered the old interview keyword
		{"dunhamssports weekly ad", "your weekly ad deals are here!", "shop now before sale ends", ""},
		{"google pixel pre-order", "pre-order the new google pixel 11 series", "check availability at your local store", ""},
		{"beehiiv pixel promo", "google's new pixel phone has a shiny light to arrest your attention", "limited time offer", ""},
		{"gofobo movie first look", "first look: jon hamm's new psychological thriller series, american hostage", "tickets on sale now", ""},
		{"generic retail availability", "last shot at par-fect deals", "check availability before you buy", ""},
		// Genuine signals must still classify
		{"real rejection", "glimpse - senior edge infrastructure engineer - next steps", "unfortunately we will not be moving forward", "REJECTED"},
		{"real interview", "your upcoming call with glimpse", "we would like to schedule an interview, what is your availability?", "INTERVIEW_REQUESTED"},
		{"interview availability with role", "re: rate confirmation_production ai platform engineer", "what is your availability for the position?", "INTERVIEW_REQUESTED"},
		{"phone screen availability", "follow up on your application", "are you available for a phone screen next week?", "INTERVIEW_REQUESTED"},
		{"unrelated newsletter", "weekly go digest", "generics deep dive", ""},
	}
	for _, tt := range tests {
		if got := classifyEmail(tt.subject, tt.body); got != tt.want {
			t.Errorf("%s: classifyEmail = %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestMatchTrackedCompany(t *testing.T) {
	companies := []string{"glimpse", "en-US", "en", "Unknown Company", "jobgether", "ClearlyAgile", "Remote"}
	tests := []struct {
		name         string
		senderDomain string
		subject      string
		want         string
	}{
		{"direct company domain", "glimpse.io", "your upcoming call", "glimpse"},
		{"ats relay, company in subject", "greenhouse.io", "update on your jobgether application", "jobgether"},
		{"case-insensitive stored name", "linkedin.com", "your application to clearlyagile", "ClearlyAgile"},
		// Bug #20: generic/short labels must never match
		{"google receipt never matches en", "google.com", "we've received your payment", ""},
		{"generic label ignored even if contained", "example.com", "en-us update", ""},
		{"no match at all", "randomcorp.com", "hello", ""},
		// Confirmed live 2026-07-22: "Remote" (remote.com) must not match
		// the word "remote" in a recruiter subject line
		{"common-word company not matched via subject", "theswifthire.com", "re: rtr/rc || software product engineer || contract || remote ||", ""},
		{"common-word company matched via its own domain", "remote.com", "update on your application", "Remote"},
	}
	for _, tt := range tests {
		if got := matchTrackedCompany(companies, tt.senderDomain, tt.subject); got != tt.want {
			t.Errorf("%s: matchTrackedCompany = %q, want %q", tt.name, got, tt.want)
		}
	}
}

// improvements.md #32: pulling the code out of a real Greenhouse email body.
// This is the exact wording observed live for the Surt AI application.
func TestExtractSecurityCode_RealGreenhouseWording(t *testing.T) {
	body := `Hi William,

Copy and paste this code into the security code field on your application:

uOSBQvRu

After you enter the code, resubmit your application.

© 2026 Greenhouse 18 West 18th Street, 11th Floor, New York NY`
	if got := extractSecurityCode(body); got != "uOSBQvRu" {
		t.Errorf("got %q, want uOSBQvRu", got)
	}
}

// The same message as Greenhouse actually sends it — HTML, with the code in
// its own element.
func TestExtractSecurityCode_HTMLBody(t *testing.T) {
	body := `<html><body><p>Copy and paste this code into the security code field on your application:</p><h1>uOSBQvRu</h1><p>After you enter the code, resubmit your application.</p></body></html>`
	if got := extractSecurityCode(body); got != "uOSBQvRu" {
		t.Errorf("got %q, want uOSBQvRu", got)
	}
}

// Prose must not yield a code — a false positive here types garbage into a
// real application.
func TestExtractSecurityCode_IgnoresOrdinaryProse(t *testing.T) {
	body := `Thanks for applying to Greenhouse. Your application has been received and
	our recruiting team will review it shortly. Regards, Recruiting`
	if got := extractSecurityCode(body); got != "" {
		t.Errorf("expected no code from prose, got %q", got)
	}
}

func TestIsPlausibleCode(t *testing.T) {
	for _, ok := range []string{"uOSBQvRu", "A1B2C3", "482915", "abc123XY"} {
		if !isPlausibleCode(ok) {
			t.Errorf("%q should be plausible", ok)
		}
	}
	for _, bad := range []string{"application", "greenhouse", "short", "recruiting", "the", "reviewed"} {
		if isPlausibleCode(bad) {
			t.Errorf("%q is prose and must be rejected", bad)
		}
	}
}

// Only ATS senders are ever examined — this is the user's personal mailbox.
func TestSubjectAnnouncesCode(t *testing.T) {
	if !subjectAnnouncesCode("Security code for your application to Surt AI") {
		t.Error("the real Greenhouse subject must match")
	}
	if subjectAnnouncesCode("Your application to Acme was received") {
		t.Error("an ordinary acknowledgement is not a code notification")
	}
}

func TestUpdateDBWithTrackerResultStates(t *testing.T) {
	tests := []struct {
		name       string
		company    string
		start      string
		want       trackerUpdateResult
		wantStatus string
	}{
		{
			name:       "updated",
			company:    "Updated Corp",
			start:      "APPLIED",
			want:       trackerUpdateUpdated,
			wantStatus: "REJECTED",
		},
		{
			name:       "no op after an earlier outcome write",
			company:    "Noop Corp",
			start:      "REJECTED",
			want:       trackerUpdateNoop,
			wantStatus: "REJECTED",
		},
		{
			name:       "unmatched",
			company:    "",
			start:      "",
			want:       trackerUpdateUnmatched,
			wantStatus: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := setupTrackerTestDB(t, filepath.Join(t.TempDir(), "tracker.db"))
			if tt.company != "" {
				insertTrackerTestJob(t, db, tt.company, tt.start, "https://example.com/"+tt.name)
			}

			messageID := "<" + tt.name + "@example.com>"
			got, err := updateDBWithTrackerResult(messageID, tt.company, "REJECTED", "", "", "")
			if err != nil {
				t.Fatalf("updateDBWithTrackerResult failed: %v", err)
			}
			if got != tt.want {
				t.Fatalf("update result = %q, want %q", got, tt.want)
			}
			if !storage.WasEmailProcessed(messageID) {
				t.Fatal("successfully handled email was not acknowledged")
			}
			if tt.company != "" {
				assertTrackerTestStatus(t, db, tt.company, tt.wantStatus)
			}
		})
	}
}

// TestUpdateDBWithTrackerResultHandoffStatuses covers bug #434.
//
// The candidate query used to match only APPLIED rows, which made the tracker
// a complete no-op for hand-off applications — the ones the user finishes by
// hand and therefore cares most about. MANUAL_REQUIRED was already in
// GetTrackedCompanies' match set, so those emails were fetched, recognised as
// belonging to a tracked company, and then silently dropped for want of a
// candidate row.
func TestUpdateDBWithTrackerResultHandoffStatuses(t *testing.T) {
	for _, tc := range []struct {
		name   string
		start  string
		status string
		want   string
	}{
		{"account-gated rejection", "MANUAL_REQUIRED", "REJECTED", "REJECTED"},
		{"copilot-filled rejection", "AWAITING_REVIEW", "REJECTED", "REJECTED"},
		{"copilot-filled interview", "AWAITING_REVIEW", "INTERVIEW_REQUESTED", "INTERVIEW_REQUESTED"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := setupTrackerTestDB(t, filepath.Join(t.TempDir(), "tracker.db"))
			insertTrackerTestJob(t, db, "Handoff Corp", tc.start, "https://example.com/"+tc.name)

			messageID := "<" + tc.name + "@example.com>"
			got, err := updateDBWithTrackerResult(messageID, "Handoff Corp", tc.status, "", "", "")
			if err != nil {
				t.Fatalf("updateDBWithTrackerResult failed: %v", err)
			}
			if got != trackerUpdateUpdated {
				t.Fatalf("update result = %q, want %q — a real outcome email for a hand-off application must not be dropped",
					got, trackerUpdateUpdated)
			}
			if !storage.WasEmailProcessed(messageID) {
				t.Fatal("successfully handled email was not acknowledged")
			}
			assertTrackerTestStatus(t, db, "Handoff Corp", tc.want)
		})
	}
}

// TestUpdateDBWithTrackerResultHandoffAmbiguity confirms widening the candidate
// set did not weaken the rollback-rather-than-guess behaviour from bugs
// #124/#125: two eligible rows must still resolve to ambiguous, not to a coin
// flip, even when they hold different hand-off statuses.
func TestUpdateDBWithTrackerResultHandoffAmbiguity(t *testing.T) {
	db := setupTrackerTestDB(t, filepath.Join(t.TempDir(), "tracker.db"))
	insertTrackerTestJob(t, db, "Mixed Corp", "AWAITING_REVIEW", "https://example.com/mixed-one")
	insertTrackerTestJob(t, db, "Mixed Corp", "MANUAL_REQUIRED", "https://example.com/mixed-two")

	messageID := "<mixed-handoff@example.com>"
	got, err := updateDBWithTrackerResult(messageID, "Mixed Corp", "REJECTED", "", "", "")
	if err != nil {
		t.Fatalf("ambiguous hand-off match returned an error: %v", err)
	}
	if got != trackerUpdateAmbiguous {
		t.Fatalf("got %q, want ambiguous", got)
	}

	var rejected int
	if err := db.QueryRow(
		"SELECT COUNT(*) FROM job_funnel WHERE company_name = ? AND status = 'REJECTED'",
		"Mixed Corp",
	).Scan(&rejected); err != nil {
		t.Fatalf("count REJECTED rows: %v", err)
	}
	if rejected != 0 {
		t.Errorf("%d row(s) were written on an ambiguous match; the transaction must roll back instead of guessing", rejected)
	}
}

func TestUpdateDBWithTrackerResultAmbiguousCompany(t *testing.T) {
	db := setupTrackerTestDB(t, filepath.Join(t.TempDir(), "tracker.db"))
	insertTrackerTestJob(t, db, "Double Corp", "APPLIED", "https://example.com/one")
	insertTrackerTestJob(t, db, "Double Corp", "APPLIED", "https://example.com/two")

	messageID := "<ambiguous@example.com>"
	got, err := updateDBWithTrackerResult(messageID, "Double Corp", "REJECTED", "", "", "")
	if err != nil {
		t.Fatalf("ambiguous match returned an error: %v", err)
	}
	if got != trackerUpdateAmbiguous {
		t.Fatalf("got %q, want ambiguous", got)
	}
	if !storage.WasEmailProcessed(messageID) {
		t.Fatal("ambiguous email must be acknowledged to avoid retry loops")
	}

	var applied int
	if err := db.QueryRow(
		"SELECT COUNT(*) FROM job_funnel WHERE company_name = ? AND status = 'APPLIED'",
		"Double Corp",
	).Scan(&applied); err != nil {
		t.Fatalf("count APPLIED rows: %v", err)
	}
	if applied != 2 {
		t.Fatalf("transaction changed ambiguous rows: got %d APPLIED, want 2", applied)
	}
}

func insertTrackerTestJobWithTitle(t *testing.T, db *sql.DB, company, title, status, url string) {
	t.Helper()
	if _, err := db.Exec(
		`INSERT INTO job_funnel (company_name, job_title, url, status)
		 VALUES (?, ?, ?, ?)`,
		company,
		title,
		url,
		status,
	); err != nil {
		t.Fatalf("insert tracker test job: %v", err)
	}
}

func TestUpdateDBWithTrackerResultResolvesAmbiguity(t *testing.T) {
	db := setupTrackerTestDB(t, filepath.Join(t.TempDir(), "tracker.db"))
	insertTrackerTestJobWithTitle(t, db, "Double Corp", "Backend Engineer", "APPLIED", "https://lever.co/double/12345")
	insertTrackerTestJobWithTitle(t, db, "Double Corp", "Frontend Developer", "APPLIED", "https://greenhouse.io/double/jobs/67890")

	messageID1 := "<idmatch@example.com>"
	got, err := updateDBWithTrackerResult(messageID1, "Double Corp", "REJECTED", "update on 12345", "", "")
	if err != nil || got != trackerUpdateUpdated {
		t.Fatalf("ID match failed: err=%v, got=%v", err, got)
	}
	var status1 string
	db.QueryRow("SELECT status FROM job_funnel WHERE url = 'https://lever.co/double/12345'").Scan(&status1)
	if status1 != "REJECTED" {
		t.Fatal("expected REJECTED for matched ID")
	}

	messageID2 := "<titlematch@example.com>"
	got, err = updateDBWithTrackerResult(messageID2, "Double Corp", "INTERVIEW_REQUESTED", "frontend role next steps", "", "")
	if err != nil || got != trackerUpdateUpdated {
		t.Fatalf("Title match failed: err=%v, got=%v", err, got)
	}
	var status2 string
	db.QueryRow("SELECT status FROM job_funnel WHERE url = 'https://greenhouse.io/double/jobs/67890'").Scan(&status2)
	if status2 != "INTERVIEW_REQUESTED" {
		t.Fatal("expected INTERVIEW_REQUESTED for matched Title")
	}
}

func TestUpdateDBWithTrackerResultRejectsInvalidStatus(t *testing.T) {
	db := setupTrackerTestDB(t, filepath.Join(t.TempDir(), "tracker.db"))
	insertTrackerTestJob(t, db, "Status Corp", "APPLIED", "https://example.com/status")

	messageID := "<invalid-status@example.com>"
	if _, err := updateDBWithTrackerResult(messageID, "Status Corp", "APPLIED", "", "", ""); err == nil {
		t.Fatal("unsupported tracker status must fail")
	}
	if storage.WasEmailProcessed(messageID) {
		t.Fatal("email with an invalid status must remain retryable")
	}
	assertTrackerTestStatus(t, db, "Status Corp", "APPLIED")
}

func TestUpdateDBWithTrackerResultRetriesAfterDatabaseLock(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tracker.db")
	dsn := path + "?_busy_timeout=0&_journal_mode=DELETE"
	db := setupTrackerTestDB(t, dsn)
	insertTrackerTestJob(t, db, "Locked Corp", "APPLIED", "https://example.com/locked")

	locker, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("open locking connection: %v", err)
	}
	t.Cleanup(func() {
		locker.Close()
	})
	lockTx, err := locker.Begin()
	if err != nil {
		t.Fatalf("begin locking transaction: %v", err)
	}
	if _, err := lockTx.Exec(
		"UPDATE job_funnel SET job_title = ? WHERE company_name = ?",
		"Lock held",
		"Locked Corp",
	); err != nil {
		t.Fatalf("acquire database write lock: %v", err)
	}

	messageID := "<locked@example.com>"
	if _, err := updateDBWithTrackerResult(messageID, "Locked Corp", "REJECTED", "", "", ""); err == nil {
		t.Fatal("locked database must return an error")
	}
	if storage.WasEmailProcessed(messageID) {
		t.Fatal("email was acknowledged despite the database lock")
	}
	assertTrackerTestStatus(t, db, "Locked Corp", "APPLIED")

	if err := lockTx.Rollback(); err != nil {
		t.Fatalf("release database write lock: %v", err)
	}

	got, err := updateDBWithTrackerResult(messageID, "Locked Corp", "REJECTED", "", "", "")
	if err != nil {
		t.Fatalf("retry after releasing lock failed: %v", err)
	}
	if got != trackerUpdateUpdated {
		t.Fatalf("retry result = %q, want %q", got, trackerUpdateUpdated)
	}
	if !storage.WasEmailProcessed(messageID) {
		t.Fatal("successful retry was not acknowledged")
	}
	assertTrackerTestStatus(t, db, "Locked Corp", "REJECTED")
}

func TestUpdateDBWithTrackerResultRetriesAfterAcknowledgementError(t *testing.T) {
	db := setupTrackerTestDB(t, filepath.Join(t.TempDir(), "tracker.db"))
	insertTrackerTestJob(t, db, "Retry Corp", "APPLIED", "https://example.com/retry")
	if _, err := db.Exec("DROP TABLE processed_emails"); err != nil {
		t.Fatalf("remove processed email table: %v", err)
	}

	messageID := "<retry@example.com>"
	if _, err := updateDBWithTrackerResult(messageID, "Retry Corp", "REJECTED", "", "", ""); err == nil {
		t.Fatal("acknowledgement failure must return an error")
	}
	assertTrackerTestStatus(t, db, "Retry Corp", "APPLIED")

	if _, err := db.Exec(`CREATE TABLE processed_emails (
		message_id TEXT PRIMARY KEY,
		processed_at DATETIME
	)`); err != nil {
		t.Fatalf("restore processed email table: %v", err)
	}

	got, err := updateDBWithTrackerResult(messageID, "Retry Corp", "REJECTED", "", "", "")
	if err != nil {
		t.Fatalf("retry after restoring acknowledgement table failed: %v", err)
	}
	if got != trackerUpdateUpdated {
		t.Fatalf("retry result = %q, want %q", got, trackerUpdateUpdated)
	}
	if !storage.WasEmailProcessed(messageID) {
		t.Fatal("successful retry was not acknowledged")
	}
	assertTrackerTestStatus(t, db, "Retry Corp", "REJECTED")
}

func setupTrackerTestDB(t *testing.T, dsn string) *sql.DB {
	t.Helper()
	if err := storage.InitDBWithPath(dsn); err != nil {
		t.Fatalf("initialize tracker test database: %v", err)
	}
	t.Cleanup(func() {
		if err := storage.CloseDB(); err != nil {
			t.Errorf("close tracker test database: %v", err)
		}
	})
	return storage.GetDB()
}

func insertTrackerTestJob(t *testing.T, db *sql.DB, company, status, url string) {
	t.Helper()
	if _, err := db.Exec(
		`INSERT INTO job_funnel (company_name, job_title, url, status)
		 VALUES (?, 'Engineer', ?, ?)`,
		company,
		url,
		status,
	); err != nil {
		t.Fatalf("insert tracker test job: %v", err)
	}
}

func assertTrackerTestStatus(t *testing.T, db *sql.DB, company, want string) {
	t.Helper()
	var got string
	if err := db.QueryRow(
		"SELECT status FROM job_funnel WHERE company_name = ?",
		company,
	).Scan(&got); err != nil {
		t.Fatalf("read tracker test status: %v", err)
	}
	if got != want {
		t.Fatalf("status = %q, want %q", got, want)
	}
}

// TestTrackerOutcomeIsLedgeredAtArrivalTime is bug #529's regression.
//
// The funnel stage ledger is a database trigger, not a Go call: it derives
// occurred_at from NEW.last_updated and reason_code from NEW.status_reason.
// The tracker's outcome write used to set status alone, so both fields were
// inherited from the row's previous state. A rejection arriving weeks after
// submission was ledgered as having occurred at submission time, carrying the
// submission's own reason code — so the one event the outcome feedback loop
// exists to record was written down with the wrong timestamp and a label that
// says the opposite of what happened.
//
// Against the pre-fix code this fails twice over: reason_code reads
// "submitted_ok" and occurred_at reads the submission timestamp.
func TestTrackerOutcomeIsLedgeredAtArrivalTime(t *testing.T) {
	for _, tc := range []struct {
		name       string
		status     string
		wantReason string
	}{
		{"rejection", "REJECTED", OutcomeReasonRejected},
		{"interview", "INTERVIEW_REQUESTED", OutcomeReasonInterview},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := setupTrackerTestDB(t, filepath.Join(t.TempDir(), "tracker.db"))

			submittedAt := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
			url := "https://boards.greenhouse.io/northwind/jobs/4471102"
			if _, err := db.Exec(
				`INSERT INTO job_funnel (company_name, job_title, url, status, status_reason, last_updated)
				 VALUES ('Northwind', 'Senior Platform Engineer', ?, 'DISCOVERED', '', ?)`,
				url, submittedAt,
			); err != nil {
				t.Fatalf("seed funnel row: %v", err)
			}
			// A real prior transition, so the ledger has something to measure
			// the outcome's stage_duration_ms against.
			if _, err := db.Exec(
				`UPDATE job_funnel SET status = 'APPLIED', status_reason = 'submitted_ok', last_updated = ? WHERE url = ?`,
				submittedAt, url,
			); err != nil {
				t.Fatalf("seed prior transition: %v", err)
			}

			before := time.Now().UTC()
			got, err := updateDBWithTrackerResult("<outcome-"+tc.name+"@example.com>", "Northwind", tc.status, "", "", "")
			if err != nil {
				t.Fatalf("updateDBWithTrackerResult failed: %v", err)
			}
			if got != trackerUpdateUpdated {
				t.Fatalf("result = %q, want %q", got, trackerUpdateUpdated)
			}
			assertTrackerTestStatus(t, db, "Northwind", tc.status)

			// stage_duration_ms is deliberately not asserted here: the ledger
			// trigger computes it with julianday(), which returns NULL for the
			// timestamp text the SQLite driver actually stores. That is a
			// pre-existing defect affecting every writer — all 1,385 live
			// ledger rows have a NULL duration — and is tracked separately.
			var reason, occurredAt string
			if err := db.QueryRow(
				`SELECT reason_code, occurred_at FROM funnel_stage_events
				 WHERE new_status = ? ORDER BY id DESC LIMIT 1`, tc.status,
			).Scan(&reason, &occurredAt); err != nil {
				t.Fatalf("the outcome produced no stage ledger event: %v", err)
			}

			if reason != tc.wantReason {
				t.Errorf("ledger reason_code = %q, want %q — an outcome must not inherit the previous state's reason", reason, tc.wantReason)
			}

			occurred, err := time.Parse(time.RFC3339Nano, occurredAt)
			if err != nil {
				t.Fatalf("ledger occurred_at %q is not a parseable timestamp: %v", occurredAt, err)
			}
			if !occurred.After(submittedAt) {
				t.Errorf("ledger occurred_at = %s, which is not after the submission time %s — the outcome was backdated to submission",
					occurred, submittedAt)
			}
			if occurred.Before(before.Add(-time.Minute)) {
				t.Errorf("ledger occurred_at = %s, want approximately now (%s)", occurred, before)
			}
		})
	}
}

// TestTrackerOutcomeReasonCarriesNoEmailContent pins the privacy boundary the
// bounded reason codes exist to hold. status_reason is copied verbatim into
// the stage ledger, which is documented to store state metadata only.
func TestTrackerOutcomeReasonCarriesNoEmailContent(t *testing.T) {
	db := setupTrackerTestDB(t, filepath.Join(t.TempDir(), "tracker.db"))
	insertTrackerTestJob(t, db, "Northwind", "APPLIED", "https://example.com/privacy")

	subject := "northwind — your application to the senior platform role"
	body := "unfortunately we are not moving forward. contact hiring@northwind.example for details."

	if _, err := updateDBWithTrackerResult("<privacy@example.com>", "Northwind", "REJECTED", subject, body, ""); err != nil {
		t.Fatalf("updateDBWithTrackerResult failed: %v", err)
	}

	var reason, storedReason string
	if err := db.QueryRow(`SELECT reason_code FROM funnel_stage_events ORDER BY id DESC LIMIT 1`).Scan(&reason); err != nil {
		t.Fatalf("read ledger reason: %v", err)
	}
	if err := db.QueryRow(`SELECT status_reason FROM job_funnel WHERE company_name = 'Northwind'`).Scan(&storedReason); err != nil {
		t.Fatalf("read status_reason: %v", err)
	}

	for _, field := range []string{reason, storedReason} {
		if field != OutcomeReasonRejected {
			t.Errorf("persisted reason = %q, want the bounded code %q", field, OutcomeReasonRejected)
		}
		for _, leak := range []string{"unfortunately", "northwind.example", "hiring@", "senior platform"} {
			if strings.Contains(strings.ToLower(field), leak) {
				t.Errorf("persisted reason %q leaks email content %q", field, leak)
			}
		}
	}
}

// TestTrackerOutcomeChainEndToEnd walks the real path an inbound message
// takes — classifyEmail, then matchTrackedCompany, then persistence — rather
// than calling the writer with a company already resolved. Every other test
// in this file starts after the two decisions that actually determine whether
// an outcome reaches the database, which is precisely the gap bug #529 was
// filed against.
func TestTrackerOutcomeChainEndToEnd(t *testing.T) {
	db := setupTrackerTestDB(t, filepath.Join(t.TempDir(), "tracker.db"))

	insertTrackerTestJob(t, db, "Northwind", "APPLIED", "https://boards.greenhouse.io/northwind/jobs/4471102")
	insertTrackerTestJob(t, db, "Calder", "AWAITING_REVIEW", "https://jobs.lever.co/calder/aa11bb22")
	insertTrackerTestJob(t, db, "Tessera", "APPLIED", "https://boards.greenhouse.io/tessera/jobs/900001")
	insertTrackerTestJob(t, db, "Tessera", "APPLIED", "https://boards.greenhouse.io/tessera/jobs/900002")

	companies, err := storage.GetTrackedCompanies()
	if err != nil {
		t.Fatalf("GetTrackedCompanies: %v", err)
	}

	// deliver mirrors StartTracker's per-message branch exactly.
	deliver := func(t *testing.T, messageID, senderDomain, subject, body string) (string, trackerUpdateResult) {
		t.Helper()
		subjectLower, bodyLower := strings.ToLower(subject), strings.ToLower(body)
		status := classifyEmail(subjectLower, bodyLower)
		if status == "" {
			if err := storage.MarkEmailProcessed(messageID); err != nil {
				t.Fatalf("mark processed: %v", err)
			}
			return "", ""
		}
		company := matchTrackedCompany(companies, senderDomain, subjectLower)
		res, err := updateDBWithTrackerResult(messageID, company, status, subjectLower, bodyLower, "")
		if err != nil {
			t.Fatalf("persist outcome: %v", err)
		}
		return status, res
	}

	statusOf := func(t *testing.T, url string) string {
		t.Helper()
		var s string
		if err := db.QueryRow(`SELECT status FROM job_funnel WHERE url = ?`, url).Scan(&s); err != nil {
			t.Fatalf("read status for %s: %v", url, err)
		}
		return s
	}

	t.Run("rejection for a uniquely identifiable application", func(t *testing.T) {
		status, res := deliver(t, "<e2e-reject@example.com>", "northwind.com",
			"Northwind - Senior Platform Engineer",
			"Thank you for applying. Unfortunately we are not moving forward with your application.")
		if status != "REJECTED" || res != trackerUpdateUpdated {
			t.Fatalf("status=%q result=%q, want REJECTED/updated", status, res)
		}
		if got := statusOf(t, "https://boards.greenhouse.io/northwind/jobs/4471102"); got != "REJECTED" {
			t.Errorf("funnel status = %q, want REJECTED", got)
		}
	})

	t.Run("recruiter response for a hand-off application", func(t *testing.T) {
		status, res := deliver(t, "<e2e-interview@example.com>", "calder.com",
			"Calder - Backend Engineer",
			"We would love to schedule an interview. What is your availability next week?")
		if status != "INTERVIEW_REQUESTED" || res != trackerUpdateUpdated {
			t.Fatalf("status=%q result=%q, want INTERVIEW_REQUESTED/updated", status, res)
		}
		if got := statusOf(t, "https://jobs.lever.co/calder/aa11bb22"); got != "INTERVIEW_REQUESTED" {
			t.Errorf("funnel status = %q, want INTERVIEW_REQUESTED", got)
		}
	})

	t.Run("unrelated email touches nothing", func(t *testing.T) {
		status, _ := deliver(t, "<e2e-unrelated@example.com>", "golangweekly.com",
			"Go Weekly Digest #412", "A generics deep dive and a new release.")
		if status != "" {
			t.Fatalf("unrelated newsletter classified as %q", status)
		}
		if !storage.WasEmailProcessed("<e2e-unrelated@example.com>") {
			t.Error("an unrelated email must still be acknowledged so it is not rescanned forever")
		}
	})

	t.Run("ambiguous match fails safe", func(t *testing.T) {
		_, res := deliver(t, "<e2e-ambiguous@example.com>", "tessera.com",
			"Tessera - an update on your application",
			"Unfortunately we are not moving forward at this time.")
		if res != trackerUpdateAmbiguous {
			t.Fatalf("result = %q, want %q — two open roles at one company must not resolve to an arbitrary pick",
				res, trackerUpdateAmbiguous)
		}
		for _, u := range []string{
			"https://boards.greenhouse.io/tessera/jobs/900001",
			"https://boards.greenhouse.io/tessera/jobs/900002",
		} {
			if got := statusOf(t, u); got != "APPLIED" {
				t.Errorf("%s moved to %q on an ambiguous email; it must stay APPLIED", u, got)
			}
		}
	})

	t.Run("outcome for a job that is not in the database", func(t *testing.T) {
		_, res := deliver(t, "<e2e-unknown@example.com>", "acmeunrelated.com",
			"Acmeunrelated - your application",
			"Unfortunately we are not moving forward.")
		if res != trackerUpdateUnmatched {
			t.Fatalf("result = %q, want %q", res, trackerUpdateUnmatched)
		}
		var outcomes int
		if err := db.QueryRow(
			`SELECT COUNT(*) FROM job_funnel WHERE status IN ('REJECTED','INTERVIEW_REQUESTED')`,
		).Scan(&outcomes); err != nil {
			t.Fatalf("count outcomes: %v", err)
		}
		if outcomes != 2 {
			t.Errorf("outcome rows = %d, want the 2 written by earlier subtests — an unknown company must write nothing", outcomes)
		}
	})

	t.Run("redelivery of an already-processed message is idempotent", func(t *testing.T) {
		var ledgerBefore int
		if err := db.QueryRow(`SELECT COUNT(*) FROM funnel_stage_events`).Scan(&ledgerBefore); err != nil {
			t.Fatalf("count ledger: %v", err)
		}

		// StartTracker skips any Message-ID it has already acknowledged.
		if !storage.WasEmailProcessed("<e2e-reject@example.com>") {
			t.Fatal("the first delivery was never acknowledged, so redelivery would be reprocessed")
		}

		// Even if the dedup gate were bypassed, reprocessing must not write again.
		_, res := deliver(t, "<e2e-reject@example.com>", "northwind.com",
			"Northwind - Senior Platform Engineer",
			"Thank you for applying. Unfortunately we are not moving forward with your application.")
		if res != trackerUpdateNoop {
			t.Errorf("reprocessing result = %q, want %q — the row already holds the outcome", res, trackerUpdateNoop)
		}

		var ledgerAfter int
		if err := db.QueryRow(`SELECT COUNT(*) FROM funnel_stage_events`).Scan(&ledgerAfter); err != nil {
			t.Fatalf("count ledger: %v", err)
		}
		if ledgerAfter != ledgerBefore {
			t.Errorf("ledger grew from %d to %d on a duplicate message; outcome events must not be double-counted",
				ledgerBefore, ledgerAfter)
		}
	})
}

// TestUnmatchedOutcomeIsRecoverable covers bug #533.
//
// An outcome-shaped email that reaches no application is acknowledged inside
// the same transaction that gives up on it, and StartTracker skips an
// acknowledged Message-ID forever. Acknowledging is right — bug #20 exists
// because rescanning the same 50 messages every cycle re-logs them and re-runs
// an LLM call each time — but discarding the evidence with it is not. A
// rejection that arrives before the user confirms the hand-off application it
// belongs to used to be unrecoverable.
//
// Against the pre-fix code this fails: unmatched_outcomes stays empty.
func TestUnmatchedOutcomeIsRecoverable(t *testing.T) {
	for _, tc := range []struct {
		name    string
		company string
		seed    string
		want    trackerUpdateResult
	}{
		{"no application at all", "", "", trackerUpdateUnmatched},
		{"company known but no open row", "Known Corp", "REJECTED", trackerUpdateNoop},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := setupTrackerTestDB(t, filepath.Join(t.TempDir(), "tracker.db"))
			if tc.company != "" {
				insertTrackerTestJob(t, db, tc.company, tc.seed, "https://example.com/"+tc.name)
			}

			messageID := "<lost-" + tc.name + "@example.com>"
			got, err := updateDBWithTrackerResult(messageID, tc.company, "REJECTED", "", "", "careers.examplecorp.com")
			if err != nil {
				t.Fatalf("updateDBWithTrackerResult failed: %v", err)
			}
			if got != tc.want {
				t.Fatalf("result = %q, want %q", got, tc.want)
			}

			// The message must still be acknowledged: not reprocessing it is
			// the whole reason recording it separately is necessary.
			if !storage.WasEmailProcessed(messageID) {
				t.Error("the email was not acknowledged; it would be rescanned every cycle")
			}

			var status, domain string
			if err := db.QueryRow(
				`SELECT outcome_status, sender_domain FROM unmatched_outcomes WHERE message_id = ?`,
				messageID,
			).Scan(&status, &domain); err != nil {
				t.Fatalf("the outcome was acknowledged but not recorded — it is unrecoverable: %v", err)
			}
			if status != "REJECTED" {
				t.Errorf("recorded status = %q, want REJECTED", status)
			}
			if domain != "careers.examplecorp.com" {
				t.Errorf("recorded sender_domain = %q, want the sender's domain", domain)
			}
		})
	}
}

// A successful match must not leave a phantom "lost outcome" behind.
func TestMatchedOutcomeIsNotRecordedAsUnmatched(t *testing.T) {
	db := setupTrackerTestDB(t, filepath.Join(t.TempDir(), "tracker.db"))
	insertTrackerTestJob(t, db, "Matched Corp", "APPLIED", "https://example.com/matched")

	got, err := updateDBWithTrackerResult("<matched@example.com>", "Matched Corp", "REJECTED", "", "", "matchedcorp.com")
	if err != nil {
		t.Fatalf("updateDBWithTrackerResult failed: %v", err)
	}
	if got != trackerUpdateUpdated {
		t.Fatalf("result = %q, want updated", got)
	}

	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM unmatched_outcomes`).Scan(&n); err != nil {
		t.Fatalf("count unmatched: %v", err)
	}
	if n != 0 {
		t.Errorf("%d unmatched record(s) written for an outcome that matched", n)
	}
}

// The record must carry no email content — same boundary the ledger holds.
func TestUnmatchedOutcomeRecordCarriesNoEmailContent(t *testing.T) {
	db := setupTrackerTestDB(t, filepath.Join(t.TempDir(), "tracker.db"))

	subject := "acme — unfortunately we are not moving forward"
	body := "please contact hiring@acme.example if you have questions."

	if _, err := updateDBWithTrackerResult("<privacy-unmatched@example.com>", "", "REJECTED", subject, body, "acme.example"); err != nil {
		t.Fatalf("updateDBWithTrackerResult failed: %v", err)
	}

	rows, err := db.Query(`SELECT message_id, outcome_status, sender_domain FROM unmatched_outcomes`)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var a, b, c string
		if err := rows.Scan(&a, &b, &c); err != nil {
			t.Fatal(err)
		}
		joined := strings.ToLower(a + " " + b + " " + c)
		for _, leak := range []string{"unfortunately", "hiring@", "not moving forward", "questions"} {
			if strings.Contains(joined, leak) {
				t.Errorf("unmatched record leaks email content %q", leak)
			}
		}
	}
}

// Reprocessing the same lost outcome must not duplicate the record.
func TestUnmatchedOutcomeRecordIsIdempotent(t *testing.T) {
	db := setupTrackerTestDB(t, filepath.Join(t.TempDir(), "tracker.db"))

	for i := 0; i < 2; i++ {
		if _, err := updateDBWithTrackerResult("<dupe-lost@example.com>", "", "REJECTED", "", "", "acme.example"); err != nil {
			t.Fatalf("pass %d: %v", i, err)
		}
	}

	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM unmatched_outcomes WHERE message_id = ?`, "<dupe-lost@example.com>").Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 1 {
		t.Errorf("record count = %d, want 1", n)
	}
}
