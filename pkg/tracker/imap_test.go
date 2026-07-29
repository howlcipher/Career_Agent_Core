package tracker

import (
	"database/sql"
	"path/filepath"
	"testing"

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
		// Genuine signals must still classify
		{"real rejection", "glimpse - senior edge infrastructure engineer - next steps", "unfortunately we will not be moving forward", "REJECTED"},
		{"real interview", "your upcoming call with glimpse", "we would like to schedule an interview, what is your availability?", "INTERVIEW_REQUESTED"},
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
			got, err := updateDBWithTrackerResult(messageID, tt.company, "REJECTED", "", "")
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

func TestUpdateDBWithTrackerResultAmbiguousCompany(t *testing.T) {
	db := setupTrackerTestDB(t, filepath.Join(t.TempDir(), "tracker.db"))
	insertTrackerTestJob(t, db, "Double Corp", "APPLIED", "https://example.com/one")
	insertTrackerTestJob(t, db, "Double Corp", "APPLIED", "https://example.com/two")

	messageID := "<ambiguous@example.com>"
	got, err := updateDBWithTrackerResult(messageID, "Double Corp", "REJECTED", "", "")
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
	got, err := updateDBWithTrackerResult(messageID1, "Double Corp", "REJECTED", "update on 12345", "")
	if err != nil || got != trackerUpdateUpdated {
		t.Fatalf("ID match failed: err=%v, got=%v", err, got)
	}
	var status1 string
	db.QueryRow("SELECT status FROM job_funnel WHERE url = 'https://lever.co/double/12345'").Scan(&status1)
	if status1 != "REJECTED" {
		t.Fatal("expected REJECTED for matched ID")
	}

	messageID2 := "<titlematch@example.com>"
	got, err = updateDBWithTrackerResult(messageID2, "Double Corp", "INTERVIEW_REQUESTED", "frontend role next steps", "")
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
	if _, err := updateDBWithTrackerResult(messageID, "Status Corp", "APPLIED", "", ""); err == nil {
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
	if _, err := updateDBWithTrackerResult(messageID, "Locked Corp", "REJECTED", "", ""); err == nil {
		t.Fatal("locked database must return an error")
	}
	if storage.WasEmailProcessed(messageID) {
		t.Fatal("email was acknowledged despite the database lock")
	}
	assertTrackerTestStatus(t, db, "Locked Corp", "APPLIED")

	if err := lockTx.Rollback(); err != nil {
		t.Fatalf("release database write lock: %v", err)
	}

	got, err := updateDBWithTrackerResult(messageID, "Locked Corp", "REJECTED", "", "")
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
	if _, err := updateDBWithTrackerResult(messageID, "Retry Corp", "REJECTED", "", ""); err == nil {
		t.Fatal("acknowledgement failure must return an error")
	}
	assertTrackerTestStatus(t, db, "Retry Corp", "APPLIED")

	if _, err := db.Exec(`CREATE TABLE processed_emails (
		message_id TEXT PRIMARY KEY,
		processed_at DATETIME
	)`); err != nil {
		t.Fatalf("restore processed email table: %v", err)
	}

	got, err := updateDBWithTrackerResult(messageID, "Retry Corp", "REJECTED", "", "")
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
