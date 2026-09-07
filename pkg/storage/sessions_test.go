package storage

import (
	"testing"
	"time"
)

// seedSessionJob inserts the funnel and assisted rows one session item needs.
func seedSessionJob(t *testing.T, id int, company string) {
	t.Helper()
	now := time.Now().UTC()
	if _, err := db.Exec(`INSERT INTO job_funnel (id, company_name, job_title, url, status, discovered_at, last_updated)
		VALUES (?, ?, 'Engineer', ?, 'AWAITING_REVIEW', ?, ?)`,
		id, company, "https://boards.greenhouse.io/example/jobs/"+company, now, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO assisted_applications (job_id, original_status, next_action_code, interruption_reason, created_at, updated_at)
		VALUES (?, 'AWAITING_REVIEW', 'review_and_submit', '', ?, ?)`, id, now, now); err != nil {
		t.Fatal(err)
	}
}

func TestApplySession_StartsOnceAndPersistsPosition(t *testing.T) {
	setupTestDB(t)
	defer teardownTestDB()
	seedSessionJob(t, 1, "First")
	seedSessionJob(t, 2, "Second")

	session, err := StartApplySession(db, []string{"1", "2"}, true)
	if err != nil {
		t.Fatalf("start session: %v", err)
	}
	if session.Total != 2 || session.State != SessionRunning {
		t.Fatalf("unexpected session: %+v", session)
	}
	if _, err := StartApplySession(db, []string{"1"}, true); err == nil {
		t.Fatal("a second concurrent session must be refused")
	}

	// A fresh read is what a reloaded dashboard does, and it must see the same
	// session at the same position.
	reloaded, err := GetApplySession(db)
	if err != nil || reloaded == nil {
		t.Fatalf("reload session: %v", err)
	}
	if reloaded.ID != session.ID || reloaded.Total != 2 {
		t.Fatalf("session did not survive a reload: %+v", reloaded)
	}
}

func TestNextApplySessionJob_OffersOneApplicationAtATime(t *testing.T) {
	setupTestDB(t)
	defer teardownTestDB()
	seedSessionJob(t, 1, "First")
	seedSessionJob(t, 2, "Second")
	if _, err := StartApplySession(db, []string{"1", "2"}, true); err != nil {
		t.Fatal(err)
	}

	jobID, ok, err := NextApplySessionJob(db)
	if err != nil || !ok || jobID != "1" {
		t.Fatalf("expected the first job, got %q ok=%v err=%v", jobID, ok, err)
	}
	if err := MarkApplySessionItemOpen(db, "1"); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := NextApplySessionJob(db); ok {
		t.Fatal("a session with an application already open must not offer another")
	}

	if err := SetApplySessionState(db, SessionPaused); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := NextApplySessionJob(db); ok {
		t.Fatal("a paused session must not open anything")
	}
}

// The central rule of this feature. A browser closing tells Career Agent
// nothing about whether the employer received an application, so it must not be
// treated as an outcome in either direction.
func TestApplySession_ClosedBrowserPausesInsteadOfAdvancing(t *testing.T) {
	setupTestDB(t)
	defer teardownTestDB()
	seedSessionJob(t, 1, "First")
	seedSessionJob(t, 2, "Second")
	if _, err := StartApplySession(db, []string{"1", "2"}, true); err != nil {
		t.Fatal(err)
	}
	if err := MarkApplySessionItemOpen(db, "1"); err != nil {
		t.Fatal(err)
	}

	if err := PauseApplySessionForClosedBrowser(db, "1"); err != nil {
		t.Fatalf("pause on closed browser: %v", err)
	}
	session, err := GetApplySession(db)
	if err != nil || session == nil {
		t.Fatalf("load session: %v", err)
	}
	if session.State != SessionPaused || session.PauseReason != PauseReasonBrowserClosed {
		t.Fatalf("expected a paused session naming the closed browser, got %+v", session)
	}
	if session.Confirmed != 0 || session.Completed != 0 {
		t.Fatalf("a closed browser must not count as a completed application: %+v", session)
	}
	if session.Items[0].State != ItemPending {
		t.Fatalf("the application must return to pending, got %q", session.Items[0].State)
	}
	if _, ok, _ := NextApplySessionJob(db); ok {
		t.Fatal("a paused session must not silently open the next application")
	}
}

func TestAdvanceApplySession_RefusesANonTerminalState(t *testing.T) {
	setupTestDB(t)
	defer teardownTestDB()
	seedSessionJob(t, 1, "First")
	if _, err := StartApplySession(db, []string{"1"}, true); err != nil {
		t.Fatal(err)
	}
	if err := AdvanceApplySession(db, "1", ItemInProgress, "still working"); err == nil {
		t.Fatal("a session must never advance on a non-terminal item state")
	}
}

func TestConfirmAssistedSubmission_AdvancesTheSessionInTheSameTransaction(t *testing.T) {
	setupTestDB(t)
	defer teardownTestDB()
	seedSessionJob(t, 1, "First")
	seedSessionJob(t, 2, "Second")
	if _, err := StartApplySession(db, []string{"1", "2"}, true); err != nil {
		t.Fatal(err)
	}
	if started := GetApplySessionItemStartedAt(db, "1"); !started.IsZero() {
		t.Fatalf("job 1 should have zero review start before open, got %v", started)
	}
	if err := MarkApplySessionItemOpen(db, "1"); err != nil {
		t.Fatal(err)
	}
	if started := GetApplySessionItemStartedAt(db, "1"); started.IsZero() {
		t.Fatal("job 1 should report review start time when opened")
	}
	if _, err := ConfirmAssistedSubmission(db, "1"); err != nil {
		t.Fatalf("confirm: %v", err)
	}

	session, err := GetApplySession(db)
	if err != nil || session == nil {
		t.Fatalf("load session: %v", err)
	}
	if session.Confirmed != 1 {
		t.Fatalf("expected the confirmation to be counted, got %+v", session)
	}
	jobID, ok, err := NextApplySessionJob(db)
	if err != nil || !ok || jobID != "2" {
		t.Fatalf("expected the session to offer the next application, got %q ok=%v", jobID, ok)
	}
}

func TestStopAfterCurrent_RecordsTheRestAsStoppedRatherThanDroppingThem(t *testing.T) {
	setupTestDB(t)
	defer teardownTestDB()
	seedSessionJob(t, 1, "First")
	seedSessionJob(t, 2, "Second")
	if _, err := StartApplySession(db, []string{"1", "2"}, true); err != nil {
		t.Fatal(err)
	}
	if err := MarkApplySessionItemOpen(db, "1"); err != nil {
		t.Fatal(err)
	}
	if err := SetApplySessionStopAfterCurrent(db); err != nil {
		t.Fatal(err)
	}
	if _, err := ConfirmAssistedSubmission(db, "1"); err != nil {
		t.Fatal(err)
	}

	if session, _ := GetApplySession(db); session != nil {
		t.Fatalf("the session should have finished, got %+v", session)
	}
	var stopped int
	if err := db.QueryRow(`SELECT COUNT(*) FROM application_session_items WHERE state = ?`, ItemStopped).Scan(&stopped); err != nil {
		t.Fatal(err)
	}
	if stopped != 1 {
		t.Fatalf("the unworked application should be recorded as stopped, got %d", stopped)
	}
}

func TestEstimateAssistedEffort_RangesAndOrdering(t *testing.T) {
	easy := EstimateAssistedEffort(AssistedEffortInput{
		PostingURL: "https://boards.greenhouse.io/example/jobs/1", OriginalStatus: "AWAITING_REVIEW",
		ResumeReady: true, CoverLetterReady: true,
	})
	if easy.Band != EffortLow {
		t.Fatalf("a prepared Greenhouse application should be low effort, got %+v", easy)
	}

	hard := EstimateAssistedEffort(AssistedEffortInput{
		PostingURL: "https://careers.example.com/apply/1", OriginalStatus: "MANUAL_REQUIRED",
		AccountRequired: true, CaptchaEncounters: true, PendingQuestions: 6,
	})
	if hard.Band != EffortHigh {
		t.Fatalf("an unsupported ATS behind an account gate should be high effort, got %+v", hard)
	}
	if hard.HighMinute <= easy.HighMinute {
		t.Fatalf("effort ranges are not ordered: easy=%+v hard=%+v", easy, hard)
	}

	// An ATS that refuses the assisted browser is the most expensive case and
	// short-circuits everything else.
	rejected := EstimateAssistedEffort(AssistedEffortInput{
		PostingURL: "https://jobs.lever.co/example/1", ResumeReady: true, CoverLetterReady: true,
	})
	if rejected.Band != EffortHigh {
		t.Fatalf("an ATS that rejects the assisted browser should be high effort, got %+v", rejected)
	}
}

// Application ease must break ties and nothing more. A mediocre one-click job
// outranking an excellent realistic one is the failure this bound exists to
// prevent, and this test found it: the first version of EffortMultiplier used
// ±15%, which lifted a fit-74 easy job above a fit-92 hard one.
func TestEffortMultiplier_CannotOverwhelmFit(t *testing.T) {
	// A ten-point fit gap must survive the worst possible effort disagreement.
	for _, gap := range []struct{ better, worse float64 }{
		{92, 82}, {85, 75}, {70, 60},
	} {
		betterButHard := gap.better * EffortMultiplier(EffortHigh)
		worseButEasy := gap.worse * EffortMultiplier(EffortLow)
		if betterButHard <= worseButEasy {
			t.Fatalf("effort overwhelmed a %.0f-point fit gap: %.2f vs %.2f",
				gap.better-gap.worse, betterButHard, worseButEasy)
		}
	}

	// It still has to do something, or it would not be worth having: a
	// two-point difference is a tie in practice and effort should decide it.
	nearTieEasy := 80.0 * EffortMultiplier(EffortLow)
	nearTieHard := 82.0 * EffortMultiplier(EffortHigh)
	if nearTieEasy <= nearTieHard {
		t.Fatalf("effort failed to break a near tie: %.2f vs %.2f", nearTieEasy, nearTieHard)
	}
}

func TestSupportedAssistedATS(t *testing.T) {
	cases := map[string]string{
		"https://boards.greenhouse.io/example/jobs/1": "Greenhouse",
		"https://jobs.lever.co/example/1":             "Lever",
		"https://jobs.ashbyhq.com/example/1":          "Ashby",
		"https://careers.example.com/apply/1":         "",
	}
	for url, want := range cases {
		if got := SupportedAssistedATS(url); got != want {
			t.Errorf("SupportedAssistedATS(%q) = %q, want %q", url, got, want)
		}
	}
}

// --- bugs.md #542: found live — Skip did nothing visible ---

// A session must not try to open the next application while another assisted
// browser is still live. Its lease is the only one, so the launch would be
// refused, and the caller cannot tell that transient refusal from a browser
// that failed to open — which is what paused a session every time an
// application was skipped with its browser still open.
func TestNextApplySessionJob_WaitsWhileAnAssistedBrowserIsStillLive(t *testing.T) {
	setupTestDB(t)
	defer teardownTestDB()
	seedSessionJob(t, 1, "First")
	seedSessionJob(t, 2, "Second")
	if _, err := StartApplySession(db, []string{"1", "2"}, true); err != nil {
		t.Fatal(err)
	}
	if err := MarkApplySessionItemOpen(db, "1"); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	if _, err := db.Exec(`UPDATE assisted_applications SET lease_owner = 'owner', lease_expires_at = ?, updated_at = ? WHERE job_id = 1`,
		now.Add(10*time.Minute), now); err != nil {
		t.Fatal(err)
	}
	// The operator skips the open application. Its item is terminal, but its
	// browser has not closed yet.
	if err := AdvanceApplySession(db, "1", ItemSkipped, "operator skipped this application"); err != nil {
		t.Fatal(err)
	}

	if _, ok, err := NextApplySessionJob(db); err != nil || ok {
		t.Fatal("no application may be offered while the previous browser still holds the lease")
	}

	// Once that browser closes, the session continues on its own.
	if _, err := db.Exec(`UPDATE assisted_applications SET lease_owner = '', lease_expires_at = NULL WHERE job_id = 1`); err != nil {
		t.Fatal(err)
	}
	jobID, ok, err := NextApplySessionJob(db)
	if err != nil || !ok || jobID != "2" {
		t.Fatalf("expected the next application once the lease was released, got %q ok=%v err=%v", jobID, ok, err)
	}
}

// The signal cmd/assist polls so a skipped or stopped application actually
// closes its browser. Before this, only a confirmation closed one, so Skip left
// the browser open forever and the session could never advance.
func TestAssistedWorkFinished_ReportsATerminalSessionItem(t *testing.T) {
	setupTestDB(t)
	defer teardownTestDB()
	seedSessionJob(t, 1, "First")
	seedSessionJob(t, 2, "Second")
	if _, err := StartApplySession(db, []string{"1", "2"}, true); err != nil {
		t.Fatal(err)
	}
	if err := MarkApplySessionItemOpen(db, "1"); err != nil {
		t.Fatal(err)
	}

	if done, err := AssistedWorkFinished(db, "1"); err != nil || done {
		t.Fatal("an application still in progress is not finished")
	}
	if err := AdvanceApplySession(db, "1", ItemSkipped, "operator skipped this application"); err != nil {
		t.Fatal(err)
	}
	if done, err := AssistedWorkFinished(db, "1"); err != nil || !done {
		t.Fatalf("a skipped application must report finished so its browser closes: done=%v err=%v", done, err)
	}
	// A job that is not part of any open session governs itself, and must not
	// be told to close.
	if done, err := AssistedWorkFinished(db, "999"); err != nil || done {
		t.Fatalf("a job outside the session must not be reported finished: done=%v err=%v", done, err)
	}
}
