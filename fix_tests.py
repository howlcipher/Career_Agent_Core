import re

with open("pkg/tracker/imap_test.go", "r") as f:
    text = f.read()

text = text.replace(
    'updateDBWithTrackerResult(messageID, tt.company, "REJECTED")',
    'updateDBWithTrackerResult(messageID, tt.company, "REJECTED", "", "")'
)
text = text.replace(
    'updateDBWithTrackerResult(messageID, "Double Corp", "REJECTED")',
    'updateDBWithTrackerResult(messageID, "Double Corp", "REJECTED", "", "")'
)
text = text.replace(
    'updateDBWithTrackerResult(messageID, "Status Corp", "APPLIED")',
    'updateDBWithTrackerResult(messageID, "Status Corp", "APPLIED", "", "")'
)
text = text.replace(
    'updateDBWithTrackerResult(messageID, "Locked Corp", "REJECTED")',
    'updateDBWithTrackerResult(messageID, "Locked Corp", "REJECTED", "", "")'
)
text = text.replace(
    'updateDBWithTrackerResult(messageID, "Retry Corp", "REJECTED")',
    'updateDBWithTrackerResult(messageID, "Retry Corp", "REJECTED", "", "")'
)

old_func = """func TestUpdateDBWithTrackerResultRejectsAmbiguousCompany(t *testing.T) {
	db := setupTrackerTestDB(t, filepath.Join(t.TempDir(), "tracker.db"))
	insertTrackerTestJob(t, db, "Double Corp", "APPLIED", "https://example.com/one")
	insertTrackerTestJob(t, db, "Double Corp", "APPLIED", "https://example.com/two")

	messageID := "<ambiguous@example.com>"
	if _, err := updateDBWithTrackerResult(messageID, "Double Corp", "REJECTED", "", ""); err == nil {
		t.Fatal("multiple matching applications must fail")
	}
	if storage.WasEmailProcessed(messageID) {
		t.Fatal("ambiguous email must remain retryable")
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
}"""

new_func = """func TestUpdateDBWithTrackerResultAmbiguousCompany(t *testing.T) {
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
}"""

text = text.replace(old_func, new_func)

with open("pkg/tracker/imap_test.go", "w") as f:
    f.write(text)
