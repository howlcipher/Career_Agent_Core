package storage

import (
	"testing"
	"time"
)

func TestApplicationQuestions_RoundTripAndReplaceOnlyPendingRows(t *testing.T) {
	setupTestDB(t)
	defer teardownTestDB()
	seedSessionJob(t, 1, "First")

	summary := AssistedFillSummary{JobID: "1", FilledCount: 24, ReusedAnswers: 6, Documents: []string{"resume"}}
	questions := []ApplicationQuestion{
		{JobID: "1", Key: "comp", Prompt: "Desired compensation", ControlType: "text", Required: true, Sensitivity: "sensitive", Suggested: "$160,000"},
		{JobID: "1", Key: "backstage", Prompt: "Have you used Backstage?", ControlType: "radio", Options: []string{"Yes", "No"}, Sensitivity: "routine"},
	}
	if err := ReplaceApplicationQuestions(db, "1", questions, summary); err != nil {
		t.Fatalf("record questions: %v", err)
	}

	pending, err := GetPendingQuestions(db, "1")
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 2 {
		t.Fatalf("expected 2 pending questions, got %d", len(pending))
	}
	// Required questions come first, so the operator meets the blocking ones
	// before the optional ones.
	if !pending[0].Required {
		t.Error("required questions should sort first")
	}
	if len(pending[1].Options) != 2 {
		t.Errorf("options did not survive the round trip: %+v", pending[1])
	}

	stored, err := GetFillSummary(db, "1")
	if err != nil {
		t.Fatal(err)
	}
	if stored.FilledCount != 24 || stored.ReusedAnswers != 6 {
		t.Fatalf("summary did not round trip: %+v", stored)
	}

	// Answering one and re-running the fill must not resurrect it.
	if err := SubmitAssistedAnswersForTest(t, "1", map[string]string{"comp": "$165,000"}); err != nil {
		t.Fatal(err)
	}
	if err := ReplaceApplicationQuestions(db, "1", questions[1:], summary); err != nil {
		t.Fatal(err)
	}
	pending, err = GetPendingQuestions(db, "1")
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 1 || pending[0].Key != "backstage" {
		t.Fatalf("an answered question came back as pending: %+v", pending)
	}
}

// SubmitAssistedAnswersForTest sets up the lease state SubmitAssistedAnswers
// requires, so the answer path can be exercised without a browser.
func SubmitAssistedAnswersForTest(t *testing.T, jobID string, values map[string]string) error {
	t.Helper()
	now := time.Now().UTC()
	if _, err := db.Exec(`UPDATE assisted_applications SET assisted_state = 'needs_answers',
		lease_owner = 'test-owner', lease_expires_at = ?, updated_at = ? WHERE job_id = ?`,
		now.Add(10*time.Minute), now, jobID); err != nil {
		return err
	}
	return SubmitAssistedAnswers(db, jobID, values, now)
}

// The answers an operator types onto a real application are held only long
// enough to reach the browser. Once taken, they are gone.
func TestPendingAnswers_AreDeletedByTheReadThatDeliversThem(t *testing.T) {
	setupTestDB(t)
	defer teardownTestDB()
	seedSessionJob(t, 1, "First")
	if err := ReplaceApplicationQuestions(db, "1", []ApplicationQuestion{
		{JobID: "1", Key: "comp", Prompt: "Desired compensation", ControlType: "text"},
	}, AssistedFillSummary{JobID: "1"}); err != nil {
		t.Fatal(err)
	}
	if err := SubmitAssistedAnswersForTest(t, "1", map[string]string{"comp": "$165,000"}); err != nil {
		t.Fatalf("submit answers: %v", err)
	}

	values, err := TakePendingAnswers(db, "1")
	if err != nil {
		t.Fatal(err)
	}
	if values["comp"] != "$165,000" {
		t.Fatalf("the answer did not reach the browser: %+v", values)
	}

	var remaining int
	if err := db.QueryRow(`SELECT COUNT(*) FROM pending_answers WHERE job_id = ?`, "1").Scan(&remaining); err != nil {
		t.Fatal(err)
	}
	if remaining != 0 {
		t.Fatal("an answer that has been delivered must not still be in the database")
	}
	again, err := TakePendingAnswers(db, "1")
	if err != nil || len(again) != 0 {
		t.Fatalf("a second read should find nothing, got %+v (%v)", again, err)
	}
}

// The question log records that a question was asked and dealt with. It must
// never hold what was said.
func TestApplicationQuestions_NeverStoreTheOperatorsAnswer(t *testing.T) {
	setupTestDB(t)
	defer teardownTestDB()
	seedSessionJob(t, 1, "First")
	if err := ReplaceApplicationQuestions(db, "1", []ApplicationQuestion{
		{JobID: "1", Key: "comp", Prompt: "Desired compensation", ControlType: "text"},
	}, AssistedFillSummary{JobID: "1"}); err != nil {
		t.Fatal(err)
	}
	const secret = "$165,432-unique-token"
	if err := SubmitAssistedAnswersForTest(t, "1", map[string]string{"comp": secret}); err != nil {
		t.Fatal(err)
	}
	if _, err := TakePendingAnswers(db, "1"); err != nil {
		t.Fatal(err)
	}

	var hits int
	if err := db.QueryRow(`SELECT COUNT(*) FROM application_questions
		WHERE prompt_text LIKE ? OR proposed_answer LIKE ?`, "%"+secret+"%", "%"+secret+"%").Scan(&hits); err != nil {
		t.Fatal(err)
	}
	if hits != 0 {
		t.Fatal("the operator's answer reached the question log")
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM pending_answers WHERE answer_text LIKE ?`, "%"+secret+"%").Scan(&hits); err != nil {
		t.Fatal(err)
	}
	if hits != 0 {
		t.Fatal("the operator's answer outlived its delivery")
	}
}

func TestSubmitAssistedAnswers_RefusesWhenNoBrowserIsWaiting(t *testing.T) {
	setupTestDB(t)
	defer teardownTestDB()
	seedSessionJob(t, 1, "First")
	// assisted_state is the default, not needs_answers, and there is no lease.
	if err := SubmitAssistedAnswers(db, "1", map[string]string{"comp": "x"}, time.Now()); err == nil {
		t.Fatal("answers must not be queued for an application with no live browser")
	}
	var queued int
	if err := db.QueryRow(`SELECT COUNT(*) FROM pending_answers`).Scan(&queued); err != nil {
		t.Fatal(err)
	}
	if queued != 0 {
		t.Fatal("a refused submission must not leave answers in the database")
	}
}

func TestPendingQuestionCounts_GroupsByJob(t *testing.T) {
	setupTestDB(t)
	defer teardownTestDB()
	seedSessionJob(t, 1, "First")
	seedSessionJob(t, 2, "Second")
	if err := ReplaceApplicationQuestions(db, "1", []ApplicationQuestion{
		{JobID: "1", Key: "a", Prompt: "A", ControlType: "text"},
		{JobID: "1", Key: "b", Prompt: "B", ControlType: "text"},
	}, AssistedFillSummary{JobID: "1"}); err != nil {
		t.Fatal(err)
	}
	if err := ReplaceApplicationQuestions(db, "2", []ApplicationQuestion{
		{JobID: "2", Key: "c", Prompt: "C", ControlType: "text"},
	}, AssistedFillSummary{JobID: "2"}); err != nil {
		t.Fatal(err)
	}
	counts, err := PendingQuestionCounts(db)
	if err != nil {
		t.Fatal(err)
	}
	if counts["1"] != 2 || counts["2"] != 1 {
		t.Fatalf("unexpected counts: %+v", counts)
	}
}

func TestRecordAssistedQuestions_RequiresALiveLeaseAndTheRightPriorState(t *testing.T) {
	setupTestDB(t)
	defer teardownTestDB()
	seedSessionJob(t, 1, "First")
	now := time.Now().UTC()
	questions := []ApplicationQuestion{{JobID: "1", Key: "a", Prompt: "A", ControlType: "text"}}

	// Wrong prior state: the application is not mid-continuation.
	if err := RecordAssistedQuestions(db, "1", "owner", questions, AssistedFillSummary{JobID: "1"}, now); err == nil {
		t.Fatal("questions must not be recorded against an application that is not continuing")
	}

	if _, err := db.Exec(`UPDATE assisted_applications SET assisted_state = 'continue_requested',
		lease_owner = 'owner', lease_expires_at = ? WHERE job_id = 1`, now.Add(10*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := RecordAssistedQuestions(db, "1", "someone-else", questions, AssistedFillSummary{JobID: "1"}, now); err == nil {
		t.Fatal("a process that does not own the lease must not record questions")
	}
	if err := RecordAssistedQuestions(db, "1", "owner", questions, AssistedFillSummary{JobID: "1"}, now); err != nil {
		t.Fatalf("the lease owner should be able to record questions: %v", err)
	}
	var state string
	if err := db.QueryRow(`SELECT assisted_state FROM assisted_applications WHERE job_id = 1`).Scan(&state); err != nil {
		t.Fatal(err)
	}
	if state != "needs_answers" {
		t.Fatalf("expected needs_answers, got %q", state)
	}
}
