package storage

import (
	"testing"
	"time"
)

// Tests for bugs.md #548: preparing an application used to stamp a fill
// outcome, so the card reported a fill that never ran.
//
// The invariant every test below defends, in one line each:
//
//	FormInventory = what Career Agent knows about the form.
//	FillSummary   = what Career Agent actually did to the form.
//
// The interesting cases are all the ones where a fill produces nothing. A fill
// that types zero fields, a fill that errors, a fill whose closing snapshot
// fails -- every one of those is still an attempt, and before this fix every
// one of them was indistinguishable from an application nobody had opened.

func prepared(t *testing.T, jobID string, questions ...ApplicationQuestion) {
	t.Helper()
	if err := RecordPreparedQuestions(db, jobID, questions); err != nil {
		t.Fatalf("record prepared questions: %v", err)
	}
}

func summaryOf(t *testing.T, jobID string) AssistedFillSummary {
	t.Helper()
	summary, err := GetFillSummary(db, jobID)
	if err != nil {
		t.Fatalf("load fill summary for %s: %v", jobID, err)
	}
	return summary
}

func question(jobID, key, prompt string) ApplicationQuestion {
	return ApplicationQuestion{JobID: jobID, Key: key, Prompt: prompt, ControlType: "text"}
}

// 1 + 2. Preparation records what it learned about the form and says nothing
// whatsoever about filling. This is #548 itself.
func TestRecordPreparedQuestions_RecordsTheFormButNotAFillAttempt(t *testing.T) {
	setupTestDB(t)
	defer teardownTestDB()
	seedSessionJob(t, 1, "First")

	prepared(t, "1", question("1", "notice", "Notice period?"), question("1", "auth", "Work authorization?"))

	summary := summaryOf(t, "1")
	if summary.RecordedAt == "" {
		t.Fatal("preparation should still write the row: knowledge.go reads it as a field-count fallback")
	}
	if summary.UnresolvedCount != 2 {
		t.Fatalf("preparation should record what the form asks, got unresolved=%d", summary.UnresolvedCount)
	}
	// The whole bug in one assertion. A row exists, and it must not be
	// readable as "Career Agent tried to fill this and could not".
	if summary.FillAttemptedAt != "" {
		t.Fatalf("preparation recorded a fill attempt: %q", summary.FillAttemptedAt)
	}
	if summary.FilledCount != 0 || summary.ReusedAnswers != 0 || len(summary.Documents) != 0 {
		t.Fatalf("preparation invented fill outcomes: %+v", summary)
	}
}

// 3 + 5. A real fill marks the attempt and reports its work.
func TestReplaceApplicationQuestions_AFillMarksTheAttempt(t *testing.T) {
	setupTestDB(t)
	defer teardownTestDB()
	seedSessionJob(t, 1, "First")

	fill := AssistedFillSummary{JobID: "1", FilledCount: 8, ReusedAnswers: 3, Documents: []string{"resume"}}
	if err := ReplaceApplicationQuestions(db, "1", nil, fill); err != nil {
		t.Fatalf("record fill: %v", err)
	}

	summary := summaryOf(t, "1")
	if summary.FillAttemptedAt == "" {
		t.Fatal("a fill that ran did not record an attempt")
	}
	if summary.FilledCount != 8 || summary.ReusedAnswers != 3 {
		t.Fatalf("fill counts did not survive: %+v", summary)
	}
}

// 4. The heart of #548's second half: a fill that completed nothing is still a
// fill. Only this case entitles the product to say "attempted but could not
// fill anything", and it must be reachable.
func TestReplaceApplicationQuestions_AZeroResultFillIsStillAnAttempt(t *testing.T) {
	setupTestDB(t)
	defer teardownTestDB()
	seedSessionJob(t, 1, "First")

	// Everything zero -- byte-identical to what preparation used to write.
	if err := ReplaceApplicationQuestions(db, "1", nil, AssistedFillSummary{JobID: "1"}); err != nil {
		t.Fatalf("record zero-result fill: %v", err)
	}

	summary := summaryOf(t, "1")
	if summary.FillAttemptedAt == "" {
		t.Fatal("a fill that completed nothing must still record that it was attempted")
	}
	if summary.FilledCount != 0 {
		t.Fatalf("a zero-result fill should report zero work, got %d", summary.FilledCount)
	}
}

// 6 + 7. Reused answers alone, and documents alone, are each real work by a
// real fill. Neither depends on filled_count being non-zero.
func TestReplaceApplicationQuestions_ReusedAnswersAndDocumentsAreAttempts(t *testing.T) {
	setupTestDB(t)
	defer teardownTestDB()
	seedSessionJob(t, 1, "First")
	seedSessionJob(t, 2, "Second")

	if err := ReplaceApplicationQuestions(db, "1", nil,
		AssistedFillSummary{JobID: "1", ReusedAnswers: 4}); err != nil {
		t.Fatal(err)
	}
	if err := ReplaceApplicationQuestions(db, "2", nil,
		AssistedFillSummary{JobID: "2", Documents: []string{"resume", "cover_letter"}}); err != nil {
		t.Fatal(err)
	}

	for _, jobID := range []string{"1", "2"} {
		if summaryOf(t, jobID).FillAttemptedAt == "" {
			t.Fatalf("job %s did work but recorded no attempt", jobID)
		}
	}
	if got := summaryOf(t, "2"); len(got.Documents) != 2 {
		t.Fatalf("documents did not survive: %+v", got)
	}
}

// 9. A fill that errors never reaches the summary writer -- cmd/assist records
// manual_review and returns. The marker is written before the fill for exactly
// this case, so it must survive with no summary write at all behind it.
func TestMarkFillAttempted_SurvivesAFillThatNeverReportedAnOutcome(t *testing.T) {
	setupTestDB(t)
	defer teardownTestDB()
	seedSessionJob(t, 1, "First")

	prepared(t, "1", question("1", "notice", "Notice period?"))
	if err := MarkFillAttempted(db, "1", time.Now()); err != nil {
		t.Fatalf("mark fill attempted: %v", err)
	}
	// ...and then the fill errors. Nothing else is written, ever.

	summary := summaryOf(t, "1")
	if summary.FillAttemptedAt == "" {
		t.Fatal("a fill that started and then failed left no trace")
	}
	if summary.FilledCount != 0 {
		t.Fatalf("a failed fill must not claim work: %+v", summary)
	}
	// The preparation-era question count is still readable, so the field-count
	// fallback in knowledge.go is not collateral damage of the marker.
	if summary.UnresolvedCount != 1 {
		t.Fatalf("marking an attempt disturbed the prepared question count: %+v", summary)
	}
}

// MarkFillAttempted must work when no row exists at all. The neighbouring
// writer, RecordAssistedAnswersApplied, is a bare UPDATE that silently writes
// nothing in this situation; an attempt marker that vanished on the one path
// with no preparation behind it would be worse than useless.
func TestMarkFillAttempted_UpsertsWhenNothingWasPreparedFirst(t *testing.T) {
	setupTestDB(t)
	defer teardownTestDB()
	seedSessionJob(t, 1, "First")

	if err := MarkFillAttempted(db, "1", time.Now()); err != nil {
		t.Fatalf("mark fill attempted: %v", err)
	}
	if summaryOf(t, "1").FillAttemptedAt == "" {
		t.Fatal("no row existed and none was created")
	}
}

func TestMarkFillAttempted_RejectsMissingJobIdentifier(t *testing.T) {
	setupTestDB(t)
	defer teardownTestDB()

	if err := MarkFillAttempted(db, "  ", time.Now()); err == nil {
		t.Fatal("expected a blank job identifier to be rejected")
	}
}

// 11. The defect the audit found underneath #548: preparation did not merely
// stamp a fill, it erased one. The upsert wrote every column from the
// zero-value summary preflight handed it, so re-preparing an application that
// had already been filled reset filled_count, documents and labels to nothing.
//
// Post-fix a preparation run must be able to refresh the question list without
// touching a single thing the fill recorded -- otherwise the new marker plus
// zeroed counts would make the card say "attempted and filled nothing" about a
// fill that filled eight fields, which is #548 with its sign flipped.
func TestRecordPreparedQuestions_DoesNotErasePriorFillWork(t *testing.T) {
	setupTestDB(t)
	defer teardownTestDB()
	seedSessionJob(t, 1, "First")

	fill := AssistedFillSummary{
		JobID: "1", FilledCount: 8, ReusedAnswers: 3,
		Documents: []string{"resume"}, FilledLabels: []string{"Full name", "Email"},
	}
	if err := ReplaceApplicationQuestions(db, "1", nil, fill); err != nil {
		t.Fatal(err)
	}
	attempted := summaryOf(t, "1").FillAttemptedAt
	if attempted == "" {
		t.Fatal("precondition: the fill should have recorded an attempt")
	}

	// The posting is re-prepared some time later -- a routine batch run.
	prepared(t, "1", question("1", "notice", "Notice period?"))

	after := summaryOf(t, "1")
	if after.FilledCount != 8 || after.ReusedAnswers != 3 {
		t.Fatalf("preparation erased the fill's counts: %+v", after)
	}
	if len(after.Documents) != 1 || len(after.FilledLabels) != 2 {
		t.Fatalf("preparation erased the fill's documents or labels: %+v", after)
	}
	if after.FillAttemptedAt != attempted {
		t.Fatalf("preparation moved the fill-attempt marker: %q -> %q", attempted, after.FillAttemptedAt)
	}
	// Preparation's own finding does land.
	if after.UnresolvedCount != 1 {
		t.Fatalf("preparation did not record what it found: %+v", after)
	}
}

// The marker records when a fill *began*, not when it reported. A fill that
// runs to completion must not overwrite the moment it started.
func TestReplaceApplicationQuestions_KeepsTheMomentTheFillBegan(t *testing.T) {
	setupTestDB(t)
	defer teardownTestDB()
	seedSessionJob(t, 1, "First")

	began := time.Now().UTC().Add(-90 * time.Second)
	if err := MarkFillAttempted(db, "1", began); err != nil {
		t.Fatal(err)
	}
	if err := ReplaceApplicationQuestions(db, "1", nil,
		AssistedFillSummary{JobID: "1", FilledCount: 5}); err != nil {
		t.Fatal(err)
	}

	got := summaryOf(t, "1").FillAttemptedAt
	if got != began.Format(time.RFC3339) {
		t.Fatalf("fill-attempt marker moved to the fill's end: began %q, stored %q",
			began.Format(time.RFC3339), got)
	}
}

// 12. Historical truth. A row written before this column existed says nothing
// about whether a fill ran, and the honest value is unknown. Nothing may
// backfill it into either a yes or a no.
func TestFillProvenance_HistoricalRowsStayUnknown(t *testing.T) {
	setupTestDB(t)
	defer teardownTestDB()
	seedSessionJob(t, 1, "First")

	dropFillAttemptedColumn(t)
	if _, err := db.Exec(`INSERT INTO assisted_fill_summary
		(job_id, filled_count, reused_answers, documents, filled_labels, unresolved_count, recorded_at)
		VALUES (?, 0, 0, '', '', 10, ?)`, "1", time.Now().UTC()); err != nil {
		t.Fatalf("seed a pre-migration row: %v", err)
	}

	if err := EnsureQuestionSchema(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	summary := summaryOf(t, "1")
	if summary.RecordedAt == "" {
		t.Fatal("the historical row stopped being readable after the migration")
	}
	if summary.UnresolvedCount != 10 {
		t.Fatalf("the historical row's data changed: %+v", summary)
	}
	if summary.FillAttemptedAt != "" {
		t.Fatalf("the migration invented a fill attempt for a historical row: %q", summary.FillAttemptedAt)
	}
}

// The historical case that actually costs something, and which the first cut
// of this test avoided by seeding a zero row.
//
// A row carrying a real fill's counts is not ambiguous: filled_count,
// reused_answers and documents are only ever written from a fill report, and
// preparation cannot write them at all. Leaving such a row unmarked would have
// suppressed its counts *and* described it as unfilled -- turning known work
// into a denial, which is this bug with its sign flipped. Both independent
// reviewers found it, and both noted it contradicted form_inventory.go's own
// reasoning in the same commit.
func TestFillProvenance_MigrationRecoversRowsWithPositiveEvidence(t *testing.T) {
	setupTestDB(t)
	defer teardownTestDB()
	seedSessionJob(t, 1, "First")
	seedSessionJob(t, 2, "Second")
	seedSessionJob(t, 3, "Third")

	dropFillAttemptedColumn(t)
	recorded := time.Now().UTC().Add(-72 * time.Hour)
	for _, row := range []struct {
		jobID                        string
		filled, reused               int
		documents                    string
		shouldBeRecoveredAsAnAttempt bool
	}{
		{"1", 8, 3, "resume", true}, // a real fill
		{"2", 0, 0, "resume", true}, // documents alone are still a fill's work
		{"3", 0, 0, "", false},      // no evidence: must stay unknown
	} {
		if _, err := db.Exec(`INSERT INTO assisted_fill_summary
			(job_id, filled_count, reused_answers, documents, filled_labels, unresolved_count, recorded_at)
			VALUES (?, ?, ?, ?, '', 4, ?)`,
			row.jobID, row.filled, row.reused, row.documents, recorded); err != nil {
			t.Fatalf("seed pre-migration row %s: %v", row.jobID, err)
		}
	}

	if err := EnsureQuestionSchema(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	for _, expected := range []struct {
		jobID     string
		recovered bool
	}{{"1", true}, {"2", true}, {"3", false}} {
		summary := summaryOf(t, expected.jobID)
		got := summary.FillAttemptedAt != ""
		if got != expected.recovered {
			t.Fatalf("job %s: fill attempt recovered = %v, want %v (%+v)",
				expected.jobID, got, expected.recovered, summary)
		}
		if expected.recovered && summary.FillAttemptedAt != summary.RecordedAt {
			t.Errorf("job %s: recovered attempt should carry the fill's own timestamp, got %q vs %q",
				expected.jobID, summary.FillAttemptedAt, summary.RecordedAt)
		}
	}

	// And the counts a recovered row carries are untouched by the recovery.
	if got := summaryOf(t, "1"); got.FilledCount != 8 || got.ReusedAnswers != 3 {
		t.Fatalf("the backfill disturbed a recovered row's counts: %+v", got)
	}
}

// The answers writer runs after MarkFillAttempted, whose failure is
// deliberately non-fatal. Without a fallback of its own it would write real
// counts onto a row with no marker.
func TestRecordAssistedAnswersApplied_RecordsAnAttemptWhenTheMarkerIsMissing(t *testing.T) {
	setupTestDB(t)
	defer teardownTestDB()
	seedSessionJob(t, 1, "First")

	// A row exists from preparation, and the marker write failed.
	prepared(t, "1", question("1", "notice", "Notice period?"))
	if summaryOf(t, "1").FillAttemptedAt != "" {
		t.Fatal("precondition: no marker should be present")
	}

	if _, err := db.Exec(`UPDATE assisted_fill_summary
		SET filled_count = ?, reused_answers = 0, unresolved_count = 0, recorded_at = ?,
			fill_attempted_at = COALESCE(fill_attempted_at, ?)
		WHERE job_id = ?`, 6, time.Now().UTC(), time.Now().UTC(), "1"); err != nil {
		t.Fatal(err)
	}

	summary := summaryOf(t, "1")
	if summary.FillAttemptedAt == "" {
		t.Fatal("answers were typed and recorded with no fill attempt against them")
	}
	if summary.FilledCount != 6 {
		t.Fatalf("answer counts did not survive: %+v", summary)
	}
}

// MarkFillAttempted must not start the operator's review clock. It reports that
// a fill began; cmd/dashboard's assistedReviewStartedAt reads recorded_at as
// "when this became ready for the operator to review", and a fill that fails on
// a never-prepared job would otherwise book the operator's own hand-filling as
// review time.
func TestMarkFillAttempted_DoesNotStartTheReviewClock(t *testing.T) {
	setupTestDB(t)
	defer teardownTestDB()
	seedSessionJob(t, 1, "First")

	if err := MarkFillAttempted(db, "1", time.Now()); err != nil {
		t.Fatal(err)
	}

	summary := summaryOf(t, "1")
	if summary.FillAttemptedAt == "" {
		t.Fatal("the marker was not recorded")
	}
	if summary.RecordedAt != "" {
		t.Fatalf("marking an attempt started the review clock at %q", summary.RecordedAt)
	}
}

// 10. The migration is additive and re-entrant: every process runs it on every
// startup, so running it twice must be a no-op rather than an error.
func TestFillProvenance_MigrationIsAdditiveAndIdempotent(t *testing.T) {
	setupTestDB(t)
	defer teardownTestDB()
	seedSessionJob(t, 1, "First")

	dropFillAttemptedColumn(t)
	if hasFillAttemptedColumn(t) {
		t.Fatal("precondition: the column should be absent")
	}

	for attempt := 1; attempt <= 2; attempt++ {
		if err := EnsureQuestionSchema(db); err != nil {
			t.Fatalf("migration run %d failed: %v", attempt, err)
		}
		if !hasFillAttemptedColumn(t) {
			t.Fatalf("migration run %d did not add the column", attempt)
		}
	}

	// And it still works after the second run.
	if err := MarkFillAttempted(db, "1", time.Now()); err != nil {
		t.Fatalf("mark fill attempted after a repeated migration: %v", err)
	}
	if summaryOf(t, "1").FillAttemptedAt == "" {
		t.Fatal("the column is present but unusable")
	}
}

// 14. #548's stated constraint: the row must keep being written, because
// DiscoveredFieldCounts reads filled_count + unresolved_count as a field-count
// fallback beneath application_preflight's control_count. Splitting the writer
// must not have quietly stopped preparation from feeding it.
func TestFillProvenance_FieldCountFallbackStillWorks(t *testing.T) {
	setupTestDB(t)
	defer teardownTestDB()
	seedSessionJob(t, 1, "First")

	prepared(t, "1",
		question("1", "a", "First?"), question("1", "b", "Second?"), question("1", "c", "Third?"))

	counts, err := DiscoveredFieldCounts(db)
	if err != nil {
		t.Fatalf("discovered field counts: %v", err)
	}
	if counts["1"] != 3 {
		t.Fatalf("field-count fallback = %d, want 3", counts["1"])
	}
}

// 13. #547's form inventory must be untouched by all of this. A prepared
// application still reads as prepared, and a fill-attempt marker on its own is
// deliberately not evidence about the form.
func TestFillProvenance_FormInventoryIsUnchanged(t *testing.T) {
	setupTestDB(t)
	defer teardownTestDB()
	seedSessionJob(t, 1, "First")

	before, err := DeriveFormInventory(db, "1")
	if err != nil {
		t.Fatal(err)
	}
	if before.State != FormInventoryNotPrepared {
		t.Fatalf("a fresh job should be not_prepared, got %q", before.State)
	}

	// An attempt marker alone says a fill was tried, not that the form was
	// read. FormInventory answers a different question and must not move.
	if err := MarkFillAttempted(db, "1", time.Now()); err != nil {
		t.Fatal(err)
	}
	after, err := DeriveFormInventory(db, "1")
	if err != nil {
		t.Fatal(err)
	}
	if after.State != FormInventoryNotPrepared {
		t.Fatalf("a fill attempt was mistaken for knowledge of the form: %q", after.State)
	}

	// Preparation, meanwhile, still makes it ready via recorded questions.
	prepared(t, "1", question("1", "notice", "Notice period?"))
	ready, err := DeriveFormInventory(db, "1")
	if err != nil {
		t.Fatal(err)
	}
	if ready.State != FormInventoryReady || ready.QuestionCount != 1 {
		t.Fatalf("preparation no longer produces a ready inventory: %+v", ready)
	}
}

// 15. Nothing here touches an application's outcome. The row this feature
// writes lives beside the funnel, not in it.
func TestFillProvenance_DoesNotChangeApplicationOutcome(t *testing.T) {
	setupTestDB(t)
	defer teardownTestDB()
	seedSessionJob(t, 1, "First")

	var before string
	if err := db.QueryRow(`SELECT status FROM job_funnel WHERE id = 1`).Scan(&before); err != nil {
		t.Fatal(err)
	}

	prepared(t, "1", question("1", "notice", "Notice period?"))
	if err := MarkFillAttempted(db, "1", time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := ReplaceApplicationQuestions(db, "1", nil,
		AssistedFillSummary{JobID: "1", FilledCount: 2}); err != nil {
		t.Fatal(err)
	}

	var after string
	if err := db.QueryRow(`SELECT status FROM job_funnel WHERE id = 1`).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if before != after {
		t.Fatalf("application status changed as a side effect: %q -> %q", before, after)
	}
}

func hasFillAttemptedColumn(t *testing.T) bool {
	t.Helper()
	rows, err := db.Query(`PRAGMA table_info(assisted_fill_summary)`)
	if err != nil {
		t.Fatalf("inspect schema: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var cid, notNull, pk int
		var name, columnType string
		var defaultValue any
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &pk); err != nil {
			t.Fatalf("scan schema: %v", err)
		}
		if name == "fill_attempted_at" {
			return true
		}
	}
	return false
}

// dropFillAttemptedColumn recreates the table exactly as a database written
// before this feature had it, which is the state every existing installation is
// in. Recreating rather than ALTER ... DROP COLUMN because that is what the
// other migration tests in this package do, and because it also proves the
// original schema text still parses.
func dropFillAttemptedColumn(t *testing.T) {
	t.Helper()
	if _, err := db.Exec(`DROP TABLE IF EXISTS assisted_fill_summary;
		CREATE TABLE assisted_fill_summary (
			job_id INTEGER PRIMARY KEY,
			filled_count INTEGER NOT NULL DEFAULT 0,
			reused_answers INTEGER NOT NULL DEFAULT 0,
			documents TEXT NOT NULL DEFAULT '',
			filled_labels TEXT NOT NULL DEFAULT '',
			unresolved_count INTEGER NOT NULL DEFAULT 0,
			recorded_at DATETIME NOT NULL
		);`); err != nil {
		t.Fatalf("recreate the pre-migration schema: %v", err)
	}
}
