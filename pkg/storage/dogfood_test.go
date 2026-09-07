package storage

import (
	"strconv"
	"testing"
	"time"
)

// confirmSeededJob seeds one confirmable job (funnel row plus its
// assisted_applications handoff row) and confirms it, mirroring exactly what
// the dashboard's "Confirmed — Mark Applied" button does. It returns the
// dogfood ordinal ConfirmAssistedSubmission reported.
func confirmSeededJob(t *testing.T, id int, company string) int {
	t.Helper()
	seedSessionJob(t, id, company)
	ordinal, err := ConfirmAssistedSubmission(db, itoa(id))
	if err != nil {
		t.Fatalf("confirm job %d: %v", id, err)
	}
	return ordinal
}

func itoa(id int) string {
	return strconv.Itoa(id)
}

func TestDogfoodCohort_BeginsAtZero(t *testing.T) {
	setupTestDB(t)
	defer teardownTestDB()

	active, err := GetActiveDogfoodCohort(db)
	if err != nil {
		t.Fatal(err)
	}
	if active != nil {
		t.Fatalf("expected no active cohort before start, got %+v", active)
	}

	cohort, err := StartDogfoodCohort(db)
	if err != nil {
		t.Fatal(err)
	}
	if cohort.CapturedCount != 0 {
		t.Fatalf("expected a fresh cohort to have zero captures, got %d", cohort.CapturedCount)
	}
	if cohort.TargetCount != DogfoodCohortTarget {
		t.Fatalf("expected target count %d, got %d", DogfoodCohortTarget, cohort.TargetCount)
	}
}

func TestStartDogfoodCohort_RefusesWhenAlreadyActive(t *testing.T) {
	setupTestDB(t)
	defer teardownTestDB()

	if _, err := StartDogfoodCohort(db); err != nil {
		t.Fatal(err)
	}
	if _, err := StartDogfoodCohort(db); err == nil {
		t.Fatal("starting a second cohort while one is active must fail")
	}
}

func TestDogfoodCohort_HistoricalApplicationsAreExcluded(t *testing.T) {
	setupTestDB(t)
	defer teardownTestDB()

	// Confirmed before any cohort exists.
	if ordinal := confirmSeededJob(t, 1, "Historical"); ordinal != 0 {
		t.Fatalf("a confirmation before any cohort started must not be captured, got ordinal %d", ordinal)
	}

	if _, err := StartDogfoodCohort(db); err != nil {
		t.Fatal(err)
	}

	// Starting a cohort must never retroactively sweep in the earlier row.
	active, err := GetActiveDogfoodCohort(db)
	if err != nil {
		t.Fatal(err)
	}
	if active.CapturedCount != 0 {
		t.Fatalf("starting a cohort must not retroactively capture prior applications, got %d captured", active.CapturedCount)
	}

	// Only an application confirmed after start counts.
	if ordinal := confirmSeededJob(t, 2, "PostStart"); ordinal != 1 {
		t.Fatalf("expected the first post-start confirmation to capture at ordinal 1, got %d", ordinal)
	}
}

func TestDogfoodCohort_SameApplicationCannotCountTwice(t *testing.T) {
	setupTestDB(t)
	defer teardownTestDB()

	cohort, err := StartDogfoodCohort(db)
	if err != nil {
		t.Fatal(err)
	}
	if ordinal := confirmSeededJob(t, 1, "First"); ordinal != 1 {
		t.Fatalf("expected ordinal 1, got %d", ordinal)
	}

	// The composite primary key must refuse a second row for the same
	// (cohort, job) pair even if something tried to insert one directly.
	if _, err := db.Exec(`INSERT OR IGNORE INTO dogfood_cohort_applications (cohort_id, job_id, ordinal, captured_at) VALUES (?, '1', 2, ?)`,
		cohort.ID, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM dogfood_cohort_applications WHERE cohort_id = ? AND job_id = '1'`, cohort.ID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("expected exactly one capture row for job 1, got %d", count)
	}
}

func TestDogfoodCohort_FifthApplicationClosesCohort(t *testing.T) {
	setupTestDB(t)
	defer teardownTestDB()

	if _, err := StartDogfoodCohort(db); err != nil {
		t.Fatal(err)
	}
	names := []string{"First", "Second", "Third", "Fourth", "Fifth"}
	for i, name := range names {
		ordinal := confirmSeededJob(t, i+1, name)
		if ordinal != i+1 {
			t.Fatalf("application %d: expected ordinal %d, got %d", i+1, i+1, ordinal)
		}
	}

	active, err := GetActiveDogfoodCohort(db)
	if err != nil {
		t.Fatal(err)
	}
	if active != nil {
		t.Fatalf("expected no active cohort once five applications are captured, got %+v", active)
	}

	cohorts, err := ListDogfoodCohorts(db)
	if err != nil {
		t.Fatal(err)
	}
	if len(cohorts) != 1 || cohorts[0].CompletedAt == nil || cohorts[0].CapturedCount != 5 {
		t.Fatalf("expected one completed cohort with five captures, got %+v", cohorts)
	}
}

func TestDogfoodCohort_SixthConfirmationIsNotCaptured(t *testing.T) {
	setupTestDB(t)
	defer teardownTestDB()

	if _, err := StartDogfoodCohort(db); err != nil {
		t.Fatal(err)
	}
	for i, name := range []string{"First", "Second", "Third", "Fourth", "Fifth"} {
		confirmSeededJob(t, i+1, name)
	}
	// No new cohort started -- the sixth confirmation has nowhere to land.
	if ordinal := confirmSeededJob(t, 6, "Sixth"); ordinal != 0 {
		t.Fatalf("a sixth confirmation with no active cohort must not be captured, got ordinal %d", ordinal)
	}
	cohorts, err := ListDogfoodCohorts(db)
	if err != nil {
		t.Fatal(err)
	}
	if len(cohorts) != 1 || cohorts[0].CapturedCount != 5 {
		t.Fatalf("the completed cohort must still show exactly five captures, got %+v", cohorts)
	}
}

func TestDogfoodCohort_AbandonedSessionIsNotCaptured(t *testing.T) {
	setupTestDB(t)
	defer teardownTestDB()

	if _, err := StartDogfoodCohort(db); err != nil {
		t.Fatal(err)
	}
	seedSessionJob(t, 1, "First")
	if _, err := StartApplySession(db, []string{"1"}, true); err != nil {
		t.Fatal(err)
	}
	if err := MarkApplySessionItemOpen(db, "1"); err != nil {
		t.Fatal(err)
	}
	// The browser closed without the operator confirming anything.
	if err := PauseApplySessionForClosedBrowser(db, "1"); err != nil {
		t.Fatal(err)
	}

	active, err := GetActiveDogfoodCohort(db)
	if err != nil {
		t.Fatal(err)
	}
	if active.CapturedCount != 0 {
		t.Fatalf("an abandoned session must never be captured, got %d captures", active.CapturedCount)
	}
}

func TestDogfoodCohort_AutomaticApplyBypassIsNotCaptured(t *testing.T) {
	setupTestDB(t)
	defer teardownTestDB()

	if _, err := StartDogfoodCohort(db); err != nil {
		t.Fatal(err)
	}
	url := "https://boards.greenhouse.io/example/jobs/automatic"
	if _, err := AddToFunnel("Automatic Co", "Engineer", url, "AWAITING_REVIEW"); err != nil {
		t.Fatal(err)
	}
	// The automatic pipeline's own path to APPLIED, bypassing
	// ConfirmAssistedSubmission entirely -- capture must be reachable only
	// through operator confirmation.
	if err := UpdateFunnelStatus(url, "APPLIED"); err != nil {
		t.Fatal(err)
	}

	active, err := GetActiveDogfoodCohort(db)
	if err != nil {
		t.Fatal(err)
	}
	if active.CapturedCount != 0 {
		t.Fatalf("an automatic APPLIED transition must never be captured, got %d captures", active.CapturedCount)
	}
}

func TestRecordDogfoodFeedback_RejectsUnknownCategory(t *testing.T) {
	setupTestDB(t)
	defer teardownTestDB()

	if _, err := StartDogfoodCohort(db); err != nil {
		t.Fatal(err)
	}
	confirmSeededJob(t, 1, "First")

	if err := RecordDogfoodFeedback(db, "1", "made_up_category", nil, ""); err == nil {
		t.Fatal("an unknown feedback category must be rejected")
	}
	negative := -1
	if err := RecordDogfoodFeedback(db, "1", DogfoodFeedbackOther, &negative, "note"); err == nil {
		t.Fatal("a negative manual field count must be rejected")
	}
	if err := RecordDogfoodFeedback(db, "1", DogfoodFeedbackNothing, nil, ""); err != nil {
		t.Fatalf("a valid category must be accepted: %v", err)
	}
}

func TestDogfoodSchema_HasNoAnswerTextColumn(t *testing.T) {
	setupTestDB(t)
	defer teardownTestDB()

	rows, err := db.Query(`PRAGMA table_info(dogfood_cohort_applications)`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	want := map[string]bool{
		"cohort_id": true, "job_id": true, "ordinal": true, "captured_at": true,
		"feedback_category": true, "feedback_manual_count": true, "feedback_note": true,
	}
	got := map[string]bool{}
	for rows.Next() {
		var cid, notNull, pk int
		var name, colType string
		var def any
		if err := rows.Scan(&cid, &name, &colType, &notNull, &def, &pk); err != nil {
			t.Fatal(err)
		}
		got[name] = true
	}
	if len(got) != len(want) {
		t.Fatalf("dogfood_cohort_applications columns = %v, want exactly %v", got, want)
	}
	for name := range want {
		if !got[name] {
			t.Fatalf("missing expected column %q", name)
		}
	}
}

func seedDogfoodMetrics(t *testing.T, jobID int, ats string, filled, reused int, interactionMs int64) {
	t.Helper()
	now := time.Now().UTC()
	if _, err := db.Exec(`INSERT INTO application_preflight (job_id, state, reason, ats, control_count, inspected_at)
		VALUES (?, 'inspected', '', ?, 5, ?)`, jobID, ats, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO assisted_fill_summary (job_id, filled_count, reused_answers, documents, filled_labels, unresolved_count, recorded_at, fill_attempted_at, fill_source)
		VALUES (?, ?, ?, '', '', 1, ?, ?, 'assisted')`, jobID, filled, reused, now, now); err != nil {
		t.Fatal(err)
	}
	if interactionMs > 0 {
		started := now.Add(-time.Duration(interactionMs) * time.Millisecond)
		if err := RecordHumanInteraction(db, itoa(jobID), InteractionReviewSubmit, started, now); err != nil {
			t.Fatal(err)
		}
	}
}

func TestGetDogfoodReport_ComputesCountsCorrectly(t *testing.T) {
	setupTestDB(t)
	defer teardownTestDB()

	cohort, err := StartDogfoodCohort(db)
	if err != nil {
		t.Fatal(err)
	}
	names := []string{"First", "Second", "Third", "Fourth", "Fifth"}
	filled := []int{3, 4, 5, 2, 1}
	reused := []int{1, 2, 1, 0, 0}
	ats := []string{"greenhouse", "greenhouse", "greenhouse", "lever", "lever"}
	interactionMs := []int64{60000, 90000, 0, 0, 0}
	feedback := []string{
		DogfoodFeedbackNothing,
		DogfoodFeedbackOneOffQuestion,
		DogfoodFeedbackKnownNotFilled,
		DogfoodFeedbackKnownNotFilled,
		DogfoodFeedbackBadMatch,
	}
	for i, name := range names {
		id := i + 1
		confirmSeededJob(t, id, name)
		seedDogfoodMetrics(t, id, ats[i], filled[i], reused[i], interactionMs[i])
		if err := RecordDogfoodFeedback(db, itoa(id), feedback[i], nil, ""); err != nil {
			t.Fatal(err)
		}
	}

	report, err := GetDogfoodReport(db, cohort.ID)
	if err != nil {
		t.Fatal(err)
	}
	if report.PlausibleTargets != 4 || report.BadMatches != 1 {
		t.Fatalf("targeting: plausible=%d bad=%d, want 4/1", report.PlausibleTargets, report.BadMatches)
	}
	if report.TotalFieldsFilled != 15 {
		t.Fatalf("total fields filled = %d, want 15", report.TotalFieldsFilled)
	}
	if report.AverageFieldsFilled != 3 {
		t.Fatalf("average fields filled = %v, want 3", report.AverageFieldsFilled)
	}
	if report.TotalAnswersReused != 4 {
		t.Fatalf("total answers reused = %d, want 4", report.TotalAnswersReused)
	}
	if report.KnownFactsNotFilled != 2 {
		t.Fatalf("known facts not filled = %d, want 2", report.KnownFactsNotFilled)
	}
	if report.ATSDistribution["greenhouse"] != 3 || report.ATSDistribution["lever"] != 2 {
		t.Fatalf("ats distribution = %+v, want greenhouse:3 lever:2", report.ATSDistribution)
	}
	if report.ApplicationsWithTiming != 2 {
		t.Fatalf("applications with timing = %d, want 2", report.ApplicationsWithTiming)
	}
	// bad_match legitimately appears too (only "nothing" and "one_off_question"
	// are excluded from friction) -- known_not_filled must rank first because
	// it occurred twice against bad_match's one.
	if len(report.RepeatedFriction) != 2 ||
		report.RepeatedFriction[0].Category != DogfoodFeedbackKnownNotFilled || report.RepeatedFriction[0].Count != 2 ||
		report.RepeatedFriction[1].Category != DogfoodFeedbackBadMatch || report.RepeatedFriction[1].Count != 1 {
		t.Fatalf("repeated friction = %+v, want known_not_filled:2 then bad_match:1", report.RepeatedFriction)
	}
	if report.Verdict != DogfoodVerdictFixOne {
		t.Fatalf("verdict = %q, want %q (reason: %s)", report.Verdict, DogfoodVerdictFixOne, report.VerdictReason)
	}
}

func TestGetDogfoodReport_HandlesMissingFeedback(t *testing.T) {
	setupTestDB(t)
	defer teardownTestDB()

	cohort, err := StartDogfoodCohort(db)
	if err != nil {
		t.Fatal(err)
	}
	for i, name := range []string{"First", "Second", "Third", "Fourth", "Fifth"} {
		id := i + 1
		confirmSeededJob(t, id, name)
		seedDogfoodMetrics(t, id, "greenhouse", 2, 1, 0)
		// Deliberately no RecordDogfoodFeedback call -- feedback is optional.
	}

	report, err := GetDogfoodReport(db, cohort.ID)
	if err != nil {
		t.Fatalf("a cohort with no feedback at all must not error: %v", err)
	}
	if report.PlausibleTargets != 5 || report.BadMatches != 0 {
		t.Fatalf("with no feedback every application should default to plausible, got plausible=%d bad=%d", report.PlausibleTargets, report.BadMatches)
	}
	if report.Verdict != DogfoodVerdictKeepUsing {
		t.Fatalf("with no reported friction the verdict must be keep-using, got %q", report.Verdict)
	}
}

func TestGetDogfoodReport_HandlesMissingInteractionTiming(t *testing.T) {
	setupTestDB(t)
	defer teardownTestDB()

	cohort, err := StartDogfoodCohort(db)
	if err != nil {
		t.Fatal(err)
	}
	for i, name := range []string{"First", "Second", "Third", "Fourth", "Fifth"} {
		id := i + 1
		confirmSeededJob(t, id, name)
		seedDogfoodMetrics(t, id, "greenhouse", 2, 1, 0) // no interaction timing recorded
	}

	report, err := GetDogfoodReport(db, cohort.ID)
	if err != nil {
		t.Fatalf("a cohort with zero interaction timing must not error: %v", err)
	}
	if report.ApplicationsWithTiming != 0 || report.MedianInteractionSeconds != 0 || report.AverageInteractionSeconds != 0 {
		t.Fatalf("expected zero-valued timing, got %+v", report)
	}
}

func TestDogfoodVerdict_ExpectedHumanOnlyQuestionsAreNotFlaggedAsDefects(t *testing.T) {
	setupTestDB(t)
	defer teardownTestDB()

	cohort, err := StartDogfoodCohort(db)
	if err != nil {
		t.Fatal(err)
	}
	feedback := []string{
		DogfoodFeedbackOneOffQuestion,
		DogfoodFeedbackOneOffQuestion,
		DogfoodFeedbackNothing,
		DogfoodFeedbackNothing,
		DogfoodFeedbackOneOffQuestion,
	}
	for i, name := range []string{"First", "Second", "Third", "Fourth", "Fifth"} {
		id := i + 1
		confirmSeededJob(t, id, name)
		seedDogfoodMetrics(t, id, "greenhouse", 4, 2, 0)
		if err := RecordDogfoodFeedback(db, itoa(id), feedback[i], nil, ""); err != nil {
			t.Fatal(err)
		}
	}

	report, err := GetDogfoodReport(db, cohort.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.RepeatedFriction) != 0 {
		t.Fatalf("repeated one-off questions and 'nothing' feedback must never count as repeated friction, got %+v", report.RepeatedFriction)
	}
	if report.Verdict != DogfoodVerdictKeepUsing {
		t.Fatalf("expected keep-using verdict, got %q (%s)", report.Verdict, report.VerdictReason)
	}
}
