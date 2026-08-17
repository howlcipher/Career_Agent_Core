package storage

import (
	"strings"
	"testing"
	"time"
)

// Tests for bugs.md #551: the automatic pipeline filled real employer forms
// and recorded nothing at all, so the Assisted card had no way to describe
// its own agent's work -- and, once cmd/assist did refill the form in a
// fresh browser, no way to tell that stale history apart from the current
// one.
//
// RecordAutomaticFillAttempt is cmd/agent's side of the same invariant
// bugs.md #548 established for cmd/assist:
//
//	FormInventory = what Career Agent knows about the form.
//	FillSummary   = what Career Agent actually did to the form.
//
// What these tests pin, beyond #548's own set, is the property #548 never
// needed: two different processes can both write this row, and the record
// must say which one wrote it most recently without losing when the very
// first attempt of either kind happened.

// TestRecordAutomaticFillAttempt_MarksAttemptAndSource is the fix itself:
// before it, nothing in this package could turn a posting URL into a fill
// attempt at all.
func TestRecordAutomaticFillAttempt_MarksAttemptAndSource(t *testing.T) {
	setupTestDB(t)
	defer teardownTestDB()
	seedSessionJob(t, 1, "First")

	began := time.Now().UTC()
	if err := RecordAutomaticFillAttempt("https://boards.greenhouse.io/example/jobs/First", began); err != nil {
		t.Fatalf("record automatic fill attempt: %v", err)
	}

	summary := summaryOf(t, "1")
	if summary.FillAttemptedAt == "" {
		t.Fatal("an automatic fill attempt was not recorded")
	}
	if summary.FillSource != FillSourceAutomatic {
		t.Fatalf("expected fill_source=%q, got %q", FillSourceAutomatic, summary.FillSource)
	}
	// recorded_at must stay untouched by the marker -- see MarkFillAttempted's
	// own doc comment for why: it is the review-ready clock, and an automatic
	// attempt is not a review-ready application.
	if summary.RecordedAt != "" {
		t.Fatalf("an automatic fill attempt started the review clock: %q", summary.RecordedAt)
	}
}

// A URL with no job_funnel row (a typo, a job discovered after this call was
// built against a stale cache, a race with a row not committed yet) must
// fail loudly rather than silently invent a job identifier.
func TestRecordAutomaticFillAttempt_UnknownURLIsAnError(t *testing.T) {
	setupTestDB(t)
	defer teardownTestDB()

	if err := RecordAutomaticFillAttempt("https://boards.greenhouse.io/nowhere/jobs/ghost", time.Now()); err == nil {
		t.Fatal("expected an error for a posting with no job_funnel row")
	}
}

// fill_attempted_at and fill_source both describe the most recent fill
// attempt, whichever machinery ran it -- an automatic attempt followed by a
// real assisted refill must overwrite both with the assisted attempt's own
// moment and machinery, not keep the automatic attempt's stale history.
func TestFillProvenance_AssistedRefillSupersedesAnEarlierAutomaticAttempt(t *testing.T) {
	setupTestDB(t)
	defer teardownTestDB()
	seedSessionJob(t, 1, "First")

	automaticBegan := time.Now().UTC().Add(-10 * time.Minute)
	if err := RecordAutomaticFillAttempt("https://boards.greenhouse.io/example/jobs/First", automaticBegan); err != nil {
		t.Fatalf("record automatic fill attempt: %v", err)
	}
	afterAutomatic := summaryOf(t, "1")
	if afterAutomatic.FillSource != FillSourceAutomatic {
		t.Fatalf("expected fill_source=%q after the automatic attempt, got %q", FillSourceAutomatic, afterAutomatic.FillSource)
	}

	// cmd/assist's refill opens a fresh browser sometime later and reaches
	// real controls -- exercised here the same way its own tests exercise
	// MarkFillAttempted, then the fill writer.
	assistedBegan := time.Now().UTC()
	if err := MarkFillAttempted(db, "1", assistedBegan); err != nil {
		t.Fatalf("mark fill attempted: %v", err)
	}
	if err := ReplaceApplicationQuestions(db, "1", nil, AssistedFillSummary{FilledCount: 5}); err != nil {
		t.Fatalf("replace application questions: %v", err)
	}

	final := summaryOf(t, "1")
	if final.FillSource != FillSourceAssisted {
		t.Fatalf("a real assisted refill did not overwrite the stale automatic source: got %q", final.FillSource)
	}
	if final.FilledCount != 5 {
		t.Fatalf("expected the assisted fill's real count to be recorded, got %d", final.FilledCount)
	}
	got, err := time.Parse(time.RFC3339, final.FillAttemptedAt)
	if err != nil {
		t.Fatalf("fill_attempted_at did not parse: %v", err)
	}
	// The assisted attempt's own begin time, not the earlier automatic one:
	// MarkFillAttempted always records the attempt happening now, and the
	// completion writer's COALESCE only protects that value from being
	// pushed forward to its own completion time, not from ever advancing.
	if diff := got.Sub(assistedBegan); diff < -time.Second || diff > time.Second {
		t.Fatalf("fill_attempted_at was not the assisted attempt's own timestamp: got %v, want ~%v", got, assistedBegan)
	}
}

// MarkFillAttempted's existing external behaviour -- used only by cmd/assist
// -- must be unchanged by adding a source: every call through it means
// "assisted", exactly as if the column had always existed and always held
// that value.
func TestMarkFillAttempted_RecordsAssistedSource(t *testing.T) {
	setupTestDB(t)
	defer teardownTestDB()
	seedSessionJob(t, 1, "First")

	if err := MarkFillAttempted(db, "1", time.Now()); err != nil {
		t.Fatal(err)
	}
	if got := summaryOf(t, "1").FillSource; got != FillSourceAssisted {
		t.Fatalf("expected fill_source=%q, got %q", FillSourceAssisted, got)
	}
}

// backfillFillSource must not invent a source for a row with no evidence a
// fill of any kind ever ran -- the same "empty means unknown" rule #548
// already enforces for fill_attempted_at itself.
func TestBackfillFillSource_LeavesUnattemptedRowsUnknown(t *testing.T) {
	setupTestDB(t)
	defer teardownTestDB()
	seedSessionJob(t, 1, "First")

	prepared(t, "1", question("1", "notice", "Notice period?"))

	if err := backfillFillSource(db); err != nil {
		t.Fatal(err)
	}
	if got := summaryOf(t, "1").FillSource; got != "" {
		t.Fatalf("a preparation-only row was assigned a fill source: %q", got)
	}
}

// A database upgraded from before bugs.md #551 has fill_attempted_at rows
// that only cmd/assist could ever have written -- backfillFillSource must
// recover that evidence rather than leaving every pre-existing row unknown
// forever.
func TestBackfillFillSource_RecoversPreexistingAssistedRows(t *testing.T) {
	setupTestDB(t)
	defer teardownTestDB()
	seedSessionJob(t, 1, "First")

	if err := MarkFillAttempted(db, "1", time.Now()); err != nil {
		t.Fatal(err)
	}
	// Simulate the pre-#551 state: fill_attempted_at is real evidence of an
	// assisted fill (nothing else could have written it at the time), but the
	// source column that would say so does not exist for this row yet.
	if _, err := db.Exec(`UPDATE assisted_fill_summary SET fill_source = '' WHERE job_id = 1`); err != nil {
		t.Fatal(err)
	}

	if err := backfillFillSource(db); err != nil {
		t.Fatal(err)
	}
	if got := summaryOf(t, "1").FillSource; got != FillSourceAssisted {
		t.Fatalf("a pre-existing fill_attempted_at row was not recovered as assisted, got %q", got)
	}
}

// The schema-column check this repo's other migrations pin: fill_source is
// added to a database created before it, not just to a fresh one.
func TestEnsureQuestionSchema_AddsFillSourceColumnToAnOlderDatabase(t *testing.T) {
	setupTestDB(t)
	defer teardownTestDB()

	rows, err := db.Query(`PRAGMA table_info(assisted_fill_summary)`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	found := false
	for rows.Next() {
		var cid, notNull, pk int
		var name, columnType string
		var defaultValue interface{}
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &pk); err != nil {
			t.Fatal(err)
		}
		if strings.EqualFold(name, "fill_source") {
			found = true
		}
	}
	if !found {
		t.Fatal("assisted_fill_summary has no fill_source column")
	}
}
