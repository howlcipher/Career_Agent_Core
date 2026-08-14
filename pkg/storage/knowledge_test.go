package storage

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/howlcipher/Career_Agent_Core/pkg/answers"
)

func TestQueuedQuestions_CarryACanonicalKeyThatIsNotTheDOMKey(t *testing.T) {
	setupTestDB(t)
	defer teardownTestDB()
	seedSessionJob(t, 1, "Acme")
	seedSessionJob(t, 2, "Globex")

	// The same question, asked by two employers whose forms name the control
	// differently and whose copy differs only in presentation. The DOM keys
	// disagree; the canonical key must not.
	if err := ReplaceApplicationQuestions(db, "1", []ApplicationQuestion{
		{JobID: "1", Key: "question_917244", Prompt: "How many years of Terraform experience do you have?", ControlType: "text"},
	}, AssistedFillSummary{JobID: "1"}); err != nil {
		t.Fatal(err)
	}
	if err := ReplaceApplicationQuestions(db, "2", []ApplicationQuestion{
		{JobID: "2", Key: "cards[abc123][field0]", Prompt: "HOW MANY YEARS OF TERRAFORM EXPERIENCE DO YOU HAVE? *", ControlType: "text"},
	}, AssistedFillSummary{JobID: "2"}); err != nil {
		t.Fatal(err)
	}

	queued, err := QueuedQuestions(db, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if len(queued) != 2 {
		t.Fatalf("expected both queued questions, got %d", len(queued))
	}
	if queued[0].Key == queued[1].Key {
		t.Fatal("this test is meaningless unless the DOM keys differ")
	}
	if queued[0].CanonicalKey == "" || queued[0].CanonicalKey != queued[1].CanonicalKey {
		t.Fatalf("canonical keys did not agree: %q vs %q", queued[0].CanonicalKey, queued[1].CanonicalKey)
	}
	// The key must be the vault's own, or a question could group one way here
	// and resolve another way there.
	if queued[0].CanonicalKey != answers.QuestionKey(queued[0].Prompt) {
		t.Fatal("the canonical key must be answers.QuestionKey, not a second normalization")
	}
	if queued[0].Company != "Acme" || queued[1].Company != "Globex" {
		t.Fatalf("the asking employer must travel with the question: %q, %q", queued[0].Company, queued[1].Company)
	}
	if queued[0].ATS != "Greenhouse" {
		t.Fatalf("ATS = %q, want Greenhouse", queued[0].ATS)
	}
}

func TestQueuedQuestions_ExcludeApplicationsAnAssistedBrowserHolds(t *testing.T) {
	setupTestDB(t)
	defer teardownTestDB()
	seedSessionJob(t, 1, "Leased")
	seedSessionJob(t, 2, "Free")

	for _, id := range []string{"1", "2"} {
		if err := ReplaceApplicationQuestions(db, id, []ApplicationQuestion{
			{JobID: id, Key: "notice", Prompt: "What is your notice period?", ControlType: "text"},
		}, AssistedFillSummary{JobID: id}); err != nil {
			t.Fatal(err)
		}
	}

	now := time.Now().UTC()
	if claimed, err := AcquireAssistedLease(db, "1", "owner-1", now); err != nil || !claimed {
		t.Fatalf("acquire lease: claimed=%v err=%v", claimed, err)
	}

	queued, err := QueuedQuestions(db, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(queued) != 1 || queued[0].JobID != "2" {
		t.Fatalf("a leased application's questions must be left alone, got %+v", queued)
	}

	// Once the lease is gone the question is back in scope: the exclusion is
	// about who is working on it right now, not about the job itself.
	if err := ReleaseAssistedLease(db, "1", "owner-1", now); err != nil {
		t.Fatal(err)
	}
	queued, err = QueuedQuestions(db, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(queued) != 2 {
		t.Fatalf("expected both applications after the lease was released, got %d", len(queued))
	}
}

func TestSetQuestionResolution_AnnotatesWithoutClosingTheQuestion(t *testing.T) {
	setupTestDB(t)
	defer teardownTestDB()
	seedSessionJob(t, 1, "Acme")
	if err := ReplaceApplicationQuestions(db, "1", []ApplicationQuestion{
		{JobID: "1", Key: "notice", Prompt: "What is your notice period?", ControlType: "text"},
	}, AssistedFillSummary{JobID: "1"}); err != nil {
		t.Fatal(err)
	}
	pending, err := GetPendingQuestions(db, "1")
	if err != nil || len(pending) != 1 {
		t.Fatalf("setup: %d questions, err %v", len(pending), err)
	}
	if pending[0].AutoFillable {
		t.Fatal("a freshly recorded question is not auto-fillable until something says so")
	}

	if err := SetQuestionResolution(db, pending[0].ID, "Two weeks", string(answers.SourceVault), true); err != nil {
		t.Fatal(err)
	}

	after, err := GetPendingQuestions(db, "1")
	if err != nil || len(after) != 1 {
		t.Fatalf("the question must still be pending: %d questions, err %v", len(after), err)
	}
	if !after[0].AutoFillable || after[0].Suggested != "Two weeks" || after[0].Source != string(answers.SourceVault) {
		t.Fatalf("resolution was not recorded: %+v", after[0])
	}
	// The operator has not done anything. Career Agent learning the answer must
	// not look like the operator having answered it.
	if after[0].Status != QuestionPending {
		t.Fatalf("status = %q, want it left pending", after[0].Status)
	}
}

func TestPreflight_RecordsAVerdictAndRefusesAnUnknownState(t *testing.T) {
	setupTestDB(t)
	defer teardownTestDB()
	seedSessionJob(t, 1, "Acme")
	seedSessionJob(t, 2, "Globex")
	now := time.Now().UTC()

	if err := RecordPreflight(db, PreflightResult{
		JobID: "1", State: PreflightInspected, ATS: "Greenhouse", ControlCount: 19,
	}, now); err != nil {
		t.Fatal(err)
	}
	if err := RecordPreflight(db, PreflightResult{
		JobID: "2", State: PreflightUnavailable, Reason: "captcha_blocked",
	}, now); err != nil {
		t.Fatal(err)
	}
	if err := RecordPreflight(db, PreflightResult{JobID: "1", State: "probably fine"}, now); err == nil {
		t.Fatal("an unknown preflight state must be refused, not stored")
	}

	results, err := PreflightResults(db, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 verdicts, got %d", len(results))
	}
	byJob := map[string]PreflightResult{}
	for _, result := range results {
		byJob[result.JobID] = result
	}
	if byJob["1"].State != PreflightInspected || byJob["1"].ControlCount != 19 {
		t.Fatalf("inspected verdict wrong: %+v", byJob["1"])
	}
	// "Could not look" must stay distinguishable from "looked and found none".
	if byJob["2"].State != PreflightUnavailable || byJob["2"].Reason != "captcha_blocked" {
		t.Fatalf("unavailable verdict wrong: %+v", byJob["2"])
	}
	if byJob["2"].ControlCount != 0 {
		t.Fatal("an application that could not be inspected must not report a field count")
	}
}

// This test used to assert the opposite: that an ATS refusing the assisted
// browser was refused inspection too, on the reasoning that "preflight is not a
// way around bug #520". That reasoning was wrong, and bugs.md #545 is what it
// cost. Preflight is not a way around anything -- it fills nothing and cannot
// submit -- so refusing a *submission* says nothing about whether the form can
// be *read*. Lever answers no to the first and yes to the second, and the old
// rule therefore withheld preparation from 20 of the 26 applications in the live
// queue: precisely the ones the operator has to complete by hand, and so
// precisely the ones worth preparing.
//
// What is unchanged is the submit boundary. Lever still never gets an assisted
// browser and still cannot be submitted by Career Agent; the assertion for that
// lives in TestPreflightRefusalReason_IsSeparateFromTheSubmitRejection and in
// TestGetAssistedLaunchInfo_RefusesATSThatRejectsTheAssistedBrowser.
func TestPreflightCandidates_RefuseOnlyWhatCannotBeRead(t *testing.T) {
	setupTestDB(t)
	defer teardownTestDB()
	now := time.Now().UTC()
	rows := []struct {
		id      int
		company string
		url     string
	}{
		{1, "Greenhouse Co", "https://boards.greenhouse.io/example/jobs/1"},
		{2, "Lever Co", "https://jobs.lever.co/example/abc"},
	}
	for _, row := range rows {
		if _, err := db.Exec(`INSERT INTO job_funnel (id, company_name, job_title, url, status, discovered_at, last_updated)
			VALUES (?, ?, 'Engineer', ?, 'AWAITING_REVIEW', ?, ?)`, row.id, row.company, row.url, now, now); err != nil {
			t.Fatal(err)
		}
	}

	candidates, err := PreflightCandidates(db, []string{"1", "2"})
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 2 {
		t.Fatalf("expected 2 candidates, got %d", len(candidates))
	}
	bySkip := map[string]string{}
	for _, candidate := range candidates {
		bySkip[candidate.JobID] = candidate.Skip
	}
	if bySkip["1"] != "" {
		t.Fatalf("a Greenhouse posting should be inspectable, got skip %q", bySkip["1"])
	}
	// Lever rejects the assisted browser (bug #520) and serves its form to
	// anyone. Reading it is exactly what the operator needs.
	if bySkip["2"] != "" {
		t.Fatalf("a Lever posting is readable and must be inspectable, got skip %q", bySkip["2"])
	}
	// The submit boundary is untouched by that: no assisted browser may open.
	if AssistedBrowserRejectionReason("https://jobs.lever.co/example/abc") == "" {
		t.Fatal("Lever must still be refused the assisted browser")
	}
}

func TestEnsureQuestionSchema_UpgradesAndBackfillsAPreCanonicalDatabase(t *testing.T) {
	// A database that predates the canonical key does not merely lack a column.
	// Every cross-application view finds its rows through that column, so an
	// upgrade that skipped the backfill would report "you have no repeated
	// questions" -- which is indistinguishable from good news and wrong.
	conn, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "legacy.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	// The exact shape shipped before this change.
	if _, err := conn.Exec(`CREATE TABLE application_questions (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		job_id INTEGER NOT NULL,
		question_key TEXT NOT NULL,
		prompt_text TEXT NOT NULL,
		control_type TEXT NOT NULL DEFAULT 'text',
		options_json TEXT NOT NULL DEFAULT '',
		required INTEGER NOT NULL DEFAULT 0,
		status TEXT NOT NULL DEFAULT 'pending',
		sensitivity TEXT NOT NULL DEFAULT 'routine',
		proposed_answer TEXT NOT NULL DEFAULT '',
		answer_source TEXT NOT NULL DEFAULT '',
		label_unsafe INTEGER NOT NULL DEFAULT 0,
		created_at DATETIME NOT NULL,
		answered_at DATETIME,
		UNIQUE(job_id, question_key)
	)`); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(`INSERT INTO application_questions
		(job_id, question_key, prompt_text, created_at) VALUES
		(1, 'notice', 'What is your notice period?', ?),
		(2, 'notice_period', 'What is your notice period? *', ?)`,
		time.Now().UTC(), time.Now().UTC()); err != nil {
		t.Fatal(err)
	}

	if err := EnsureQuestionSchema(conn); err != nil {
		t.Fatalf("upgrade a pre-canonical database: %v", err)
	}
	// Idempotent: the dashboard and the agent both run this on every start.
	if err := EnsureQuestionSchema(conn); err != nil {
		t.Fatalf("re-running the upgrade must be safe: %v", err)
	}

	rows, err := conn.Query(`SELECT job_id, canonical_key, auto_fillable FROM application_questions ORDER BY job_id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	keys := map[int64]string{}
	for rows.Next() {
		var jobID, autoFillable int64
		var key string
		if err := rows.Scan(&jobID, &key, &autoFillable); err != nil {
			t.Fatal(err)
		}
		if autoFillable != 0 {
			t.Fatal("a backfilled row must not claim to be auto-fillable")
		}
		keys[jobID] = key
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if keys[1] == "" {
		t.Fatal("existing rows were not backfilled with a canonical key")
	}
	if keys[1] != keys[2] {
		t.Fatalf("backfilled keys must agree across employers: %q vs %q", keys[1], keys[2])
	}
	if keys[1] != answers.QuestionKey("What is your notice period?") {
		t.Fatalf("backfill used the wrong key: %q", keys[1])
	}
}
