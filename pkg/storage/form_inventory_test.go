package storage

import (
	"testing"
	"time"
)

// seedLeverJob mirrors seedSessionJob but on a Lever URL, because Lever is the
// ATS this feature exists for: it must be completed by hand, so the packet is
// the product there.
func seedLeverJob(t *testing.T, id int, company string) {
	t.Helper()
	now := time.Now().UTC()
	if _, err := db.Exec(`INSERT INTO job_funnel (id, company_name, job_title, url, status, discovered_at, last_updated)
		VALUES (?, ?, 'Engineer', ?, 'AWAITING_REVIEW', ?, ?)`,
		id, company, "https://jobs.lever.co/"+company+"/abc123", now, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO assisted_applications (job_id, original_status, next_action_code, interruption_reason, created_at, updated_at)
		VALUES (?, 'AWAITING_REVIEW', 'review_and_submit', '', ?, ?)`, id, now, now); err != nil {
		t.Fatal(err)
	}
}

// This is the defect itself, in one assertion: a queued application nobody has
// ever inspected must not be describable as anything but unprepared.
func TestDeriveFormInventory_NeverInspectedReportsNotPrepared(t *testing.T) {
	setupTestDB(t)
	defer teardownTestDB()
	seedLeverJob(t, 308177, "acme")

	inventory, err := DeriveFormInventory(db, "308177")
	if err != nil {
		t.Fatal(err)
	}
	if inventory.State != FormInventoryNotPrepared {
		t.Fatalf("state = %q, want %q", inventory.State, FormInventoryNotPrepared)
	}
	if inventory.QuestionCount != 0 {
		t.Fatalf("question count = %d, want 0", inventory.QuestionCount)
	}
	if inventory.Source != "" {
		t.Fatalf("an application nobody inspected has no inventory source, got %q", inventory.Source)
	}
	// Lever is readable (bugs.md #545), so the packet must be able to offer the
	// repair rather than only describing the gap.
	if !inventory.Preparable {
		t.Fatal("a queued Lever application must be preparable from the packet")
	}
}

// The distinction the whole item turns on. Both of these render zero questions;
// they must never resolve to the same state.
func TestDeriveFormInventory_InspectedWithZeroQuestionsIsNotNotPrepared(t *testing.T) {
	setupTestDB(t)
	defer teardownTestDB()
	seedLeverJob(t, 1, "quiet")
	seedLeverJob(t, 2, "unread")

	// Inspected, and the form genuinely asked nothing Career Agent could not
	// already answer: a verdict row, no question rows.
	if err := RecordPreflight(db, PreflightResult{
		JobID: "1", State: PreflightInspected, ATS: "Lever", ControlCount: 12,
	}, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}

	inspected, err := DeriveFormInventory(db, "1")
	if err != nil {
		t.Fatal(err)
	}
	never, err := DeriveFormInventory(db, "2")
	if err != nil {
		t.Fatal(err)
	}

	if inspected.QuestionCount != never.QuestionCount {
		t.Fatal("this test is meaningless unless both have the same question count")
	}
	if inspected.State != FormInventoryReady {
		t.Fatalf("inspected state = %q, want %q", inspected.State, FormInventoryReady)
	}
	if never.State != FormInventoryNotPrepared {
		t.Fatalf("never-inspected state = %q, want %q", never.State, FormInventoryNotPrepared)
	}
	if inspected.State == never.State {
		t.Fatal("a form read and found quiet must not look like a form nobody read")
	}
	// The control count is what makes "we looked" checkable rather than merely
	// asserted, and it is a different number from the question count.
	if inspected.FieldCount != 12 {
		t.Fatalf("field count = %d, want 12", inspected.FieldCount)
	}
	if inspected.Source != FormInventorySourcePreflight {
		t.Fatalf("source = %q, want %q", inspected.Source, FormInventorySourcePreflight)
	}
}

func TestDeriveFormInventory_InspectedWithQuestionsIsReadyAndCounted(t *testing.T) {
	setupTestDB(t)
	defer teardownTestDB()
	seedLeverJob(t, 1, "acme")

	if err := ReplaceApplicationQuestions(db, "1", []ApplicationQuestion{
		{JobID: "1", Key: "notice", Prompt: "What is your notice period?", ControlType: "text"},
		{JobID: "1", Key: "why", Prompt: "Why do you want to work here?", ControlType: "textarea"},
	}, AssistedFillSummary{JobID: "1"}); err != nil {
		t.Fatal(err)
	}
	if err := RecordPreflight(db, PreflightResult{
		JobID: "1", State: PreflightInspected, ATS: "Lever", ControlCount: 21,
	}, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}

	inventory, err := DeriveFormInventory(db, "1")
	if err != nil {
		t.Fatal(err)
	}
	if inventory.State != FormInventoryReady {
		t.Fatalf("state = %q, want %q", inventory.State, FormInventoryReady)
	}
	if inventory.QuestionCount != 2 {
		t.Fatalf("question count = %d, want 2", inventory.QuestionCount)
	}
	if inventory.FieldCount != 21 {
		t.Fatalf("field count = %d, want 21", inventory.FieldCount)
	}
	if inventory.InspectedAt == "" {
		t.Fatal("a successful inspection must carry when it happened")
	}
}

// A failed inspection is its own fact. Reporting it as not_prepared would lose
// the reason, and reporting it as ready would claim a reading that never
// happened.
func TestDeriveFormInventory_FailedInspectionIsDistinctAndCarriesABoundedReason(t *testing.T) {
	setupTestDB(t)
	defer teardownTestDB()
	seedLeverJob(t, 1, "gone")

	if err := RecordPreflight(db, PreflightResult{
		JobID: "1", State: PreflightUnavailable, Reason: "posting_dead", ATS: "Lever",
	}, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}

	inventory, err := DeriveFormInventory(db, "1")
	if err != nil {
		t.Fatal(err)
	}
	if inventory.State != FormInventoryFailed {
		t.Fatalf("state = %q, want %q", inventory.State, FormInventoryFailed)
	}
	if inventory.Reason != "posting_dead" {
		t.Fatalf("reason = %q, want the recorded code", inventory.Reason)
	}
	// The vocabulary is closed. Anything with spaces in it is a message that
	// escaped a driver, and messages quote page content (ADR-006).
	for _, char := range inventory.Reason {
		if char == ' ' {
			t.Fatalf("reason %q reads like a message, not a code", inventory.Reason)
		}
	}
}

// The inverse regression, and the one most likely to be shipped by accident:
// a live assisted session records questions without writing a preflight
// verdict. Calling that "not prepared" would print a "nobody has read this
// form" banner directly above questions read off that very form.
func TestDeriveFormInventory_QuestionsFromAnAssistedSessionCountAsRead(t *testing.T) {
	setupTestDB(t)
	defer teardownTestDB()
	seedLeverJob(t, 1, "session")

	if err := ReplaceApplicationQuestions(db, "1", []ApplicationQuestion{
		{JobID: "1", Key: "pronouns", Prompt: "What are your pronouns?", ControlType: "text"},
	}, AssistedFillSummary{JobID: "1"}); err != nil {
		t.Fatal(err)
	}
	// Deliberately no RecordPreflight call: this is what cmd/assist leaves
	// behind.

	inventory, err := DeriveFormInventory(db, "1")
	if err != nil {
		t.Fatal(err)
	}
	if inventory.State != FormInventoryReady {
		t.Fatalf("state = %q, want %q -- questions prove a form was read", inventory.State, FormInventoryReady)
	}
	if inventory.Source != FormInventorySourceSession {
		t.Fatalf("source = %q, want %q", inventory.Source, FormInventorySourceSession)
	}
	if inventory.QuestionCount != 1 {
		t.Fatalf("question count = %d, want 1", inventory.QuestionCount)
	}
}

// An application already submitted has nothing left to prepare, and offering to
// prepare it would be an action that the preparation run itself would skip.
func TestDeriveFormInventory_CompletedApplicationIsNotPreparable(t *testing.T) {
	setupTestDB(t)
	defer teardownTestDB()
	seedLeverJob(t, 1, "done")
	if _, err := db.Exec(`UPDATE assisted_applications SET assisted_state = 'completed' WHERE job_id = 1`); err != nil {
		t.Fatal(err)
	}

	inventory, err := DeriveFormInventory(db, "1")
	if err != nil {
		t.Fatal(err)
	}
	if inventory.Preparable {
		t.Fatal("a completed application must not be offered for preparation")
	}
	if inventory.BlockedKind != PreflightSkipAlreadyApplied {
		t.Fatalf("blocked kind = %q, want %q", inventory.BlockedKind, PreflightSkipAlreadyApplied)
	}
}

// Preparation state must never be read off assisted_fill_summary. That row's
// recorded_at is stamped by preparation as well as by filling (bugs.md #548),
// so a fix for #547 that leaned on it would deepen #548 rather than avoid it.
func TestDeriveFormInventory_IgnoresTheFillSummaryEntirely(t *testing.T) {
	setupTestDB(t)
	defer teardownTestDB()
	seedLeverJob(t, 1, "stamped")

	// Exactly what cmd/preflight leaves behind for a job it could not inspect:
	// a zero-value summary row with a timestamp, and no questions.
	if err := ReplaceApplicationQuestions(db, "1", nil, AssistedFillSummary{JobID: "1"}); err != nil {
		t.Fatal(err)
	}
	if err := RecordPreflight(db, PreflightResult{
		JobID: "1", State: PreflightUnavailable, Reason: "auth_required",
	}, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}

	inventory, err := DeriveFormInventory(db, "1")
	if err != nil {
		t.Fatal(err)
	}
	if inventory.State != FormInventoryFailed {
		t.Fatalf("state = %q: a stamped fill summary must not make a failed inspection look ready", inventory.State)
	}
}

// A database written before application_preflight existed has no verdicts at
// all. It must degrade to the truthful state rather than to an error or to a
// confident claim.
func TestDeriveFormInventory_MissingTablesDegradeToNotPrepared(t *testing.T) {
	setupTestDB(t)
	defer teardownTestDB()
	seedLeverJob(t, 1, "legacy")
	if _, err := db.Exec(`DROP TABLE application_questions`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`DROP TABLE application_preflight`); err != nil {
		t.Fatal(err)
	}

	inventory, err := DeriveFormInventory(db, "1")
	if err != nil {
		t.Fatalf("a database without the question schema must not error: %v", err)
	}
	if inventory.State != FormInventoryNotPrepared {
		t.Fatalf("state = %q, want %q", inventory.State, FormInventoryNotPrepared)
	}
}

func TestDeriveFormInventory_RejectsAnEmptyJobIdentifier(t *testing.T) {
	setupTestDB(t)
	defer teardownTestDB()
	if _, err := DeriveFormInventory(db, "  "); err == nil {
		t.Fatal("an empty job identifier must be refused rather than answered")
	}
}

// Re-inspecting the same application must replace its verdict, never stack a
// second one: RecordPreflight upserts, so a repeated Prepare converges rather
// than accumulating.
func TestDeriveFormInventory_RepeatedPreparationIsIdempotent(t *testing.T) {
	setupTestDB(t)
	defer teardownTestDB()
	seedLeverJob(t, 1, "twice")

	questions := []ApplicationQuestion{
		{JobID: "1", Key: "notice", Prompt: "What is your notice period?", ControlType: "text"},
	}
	for i := 0; i < 3; i++ {
		if err := ReplaceApplicationQuestions(db, "1", questions, AssistedFillSummary{JobID: "1"}); err != nil {
			t.Fatal(err)
		}
		if err := RecordPreflight(db, PreflightResult{
			JobID: "1", State: PreflightInspected, ATS: "Lever", ControlCount: 9,
		}, time.Now().UTC()); err != nil {
			t.Fatal(err)
		}
	}

	inventory, err := DeriveFormInventory(db, "1")
	if err != nil {
		t.Fatal(err)
	}
	if inventory.QuestionCount != 1 {
		t.Fatalf("question count = %d after three runs, want 1", inventory.QuestionCount)
	}
	var verdicts int
	if err := db.QueryRow(`SELECT COUNT(*) FROM application_preflight WHERE job_id = 1`).Scan(&verdicts); err != nil {
		t.Fatal(err)
	}
	if verdicts != 1 {
		t.Fatalf("verdict rows = %d after three runs, want 1", verdicts)
	}
}

func TestFormInventoryIsStale_OnlyForOldSuccessfulReadings(t *testing.T) {
	now := time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC)
	fresh := now.Add(-48 * time.Hour).Format(time.RFC3339)
	old := now.Add(-30 * 24 * time.Hour).Format(time.RFC3339)

	if FormInventoryIsStale(FormInventory{State: FormInventoryReady, InspectedAt: fresh}, now) {
		t.Fatal("a two-day-old reading is not stale")
	}
	if !FormInventoryIsStale(FormInventory{State: FormInventoryReady, InspectedAt: old}, now) {
		t.Fatal("a month-old reading is stale")
	}
	// Staleness is a note on a real reading, never a claim about one that does
	// not exist.
	if FormInventoryIsStale(FormInventory{State: FormInventoryNotPrepared, InspectedAt: old}, now) {
		t.Fatal("an application with no inventory cannot have a stale one")
	}
	if FormInventoryIsStale(FormInventory{State: FormInventoryReady, InspectedAt: "not a time"}, now) {
		t.Fatal("an unparseable timestamp must not be reported as stale")
	}
}
