package storage

import (
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"time"
)

// Dogfood feedback categories -- the fixed, closed vocabulary an operator can
// pick from after confirming one cohort application. A closed set rather than
// free text because this data feeds an automatic verdict (dogfoodVerdict),
// and a verdict fed from typo-prone free text would silently stop meaning
// anything.
const (
	DogfoodFeedbackNothing          = "nothing"
	DogfoodFeedbackBadMatch         = "bad_match"
	DogfoodFeedbackKnownNotFilled   = "known_not_filled"
	DogfoodFeedbackFilledIncorrect  = "filled_incorrect"
	DogfoodFeedbackRepeatedQuestion = "repeated_question"
	DogfoodFeedbackOneOffQuestion   = "one_off_question"
	DogfoodFeedbackBlocker          = "blocker"
	DogfoodFeedbackOther            = "other"
)

var validDogfoodFeedbackCategories = map[string]bool{
	DogfoodFeedbackNothing:          true,
	DogfoodFeedbackBadMatch:         true,
	DogfoodFeedbackKnownNotFilled:   true,
	DogfoodFeedbackFilledIncorrect:  true,
	DogfoodFeedbackRepeatedQuestion: true,
	DogfoodFeedbackOneOffQuestion:   true,
	DogfoodFeedbackBlocker:          true,
	DogfoodFeedbackOther:            true,
}

// The three verdicts a completed cohort report can reach. Deterministic and
// rule-based (dogfoodVerdict) -- never an LLM call -- because this is the one
// judgment this harness is allowed to make on its own; everything past it is
// "recommend one task and stop" (see cmd/dashboard's dogfood report handler).
const (
	DogfoodVerdictKeepUsing = "keep_using"
	DogfoodVerdictFixOne    = "fix_one_repeated_problem"
	DogfoodVerdictPause     = "pause_for_correctness"
)

// DogfoodCohortTarget is the fixed cohort size. Not configurable: a
// comparable unit of evidence across runs is the whole point of picking one
// number and always using it.
const DogfoodCohortTarget = 5

// EnsureDogfoodSchema creates the cohort and cohort-membership tables. Two
// tables, not one: everything else the report needs (fill counts, ATS,
// interaction timing) already lives durably in job_funnel,
// assisted_fill_summary, application_preflight and human_interactions. The
// only new facts here are which five applications belong to one run, in
// order, and what the operator said slowed them down -- neither exists
// anywhere else.
func EnsureDogfoodSchema(conn *sql.DB) error {
	if conn == nil {
		return errors.New("dogfood schema connection is nil")
	}
	_, err := conn.Exec(`
	CREATE TABLE IF NOT EXISTS dogfood_cohorts (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		started_at DATETIME NOT NULL,
		target_count INTEGER NOT NULL DEFAULT 5,
		completed_at DATETIME
	);
	CREATE TABLE IF NOT EXISTS dogfood_cohort_applications (
		cohort_id INTEGER NOT NULL,
		job_id TEXT NOT NULL,
		ordinal INTEGER NOT NULL,
		captured_at DATETIME NOT NULL,
		feedback_category TEXT,
		feedback_manual_count INTEGER,
		feedback_note TEXT,
		PRIMARY KEY (cohort_id, job_id)
	);
	CREATE INDEX IF NOT EXISTS idx_dogfood_cohort_applications_cohort ON dogfood_cohort_applications(cohort_id, ordinal);`)
	if err != nil {
		return fmt.Errorf("create dogfood schema: %w", err)
	}
	return nil
}

// DogfoodCohort is one five-application evidence run.
type DogfoodCohort struct {
	ID            int64      `json:"id"`
	StartedAt     time.Time  `json:"started_at"`
	TargetCount   int        `json:"target_count"`
	CompletedAt   *time.Time `json:"completed_at,omitempty"`
	CapturedCount int        `json:"captured_count"`
}

// DogfoodCohortSummary is the read-only listing shape for past cohorts.
type DogfoodCohortSummary struct {
	ID            int64      `json:"id"`
	StartedAt     time.Time  `json:"started_at"`
	CompletedAt   *time.Time `json:"completed_at,omitempty"`
	TargetCount   int        `json:"target_count"`
	CapturedCount int        `json:"captured_count"`
}

// StartDogfoodCohort opens a new five-application evidence run. It refuses
// when one is already open: a cohort is a fixed, comparable unit, and two
// open at once would make "the applications confirmed after cohort start"
// ambiguous for whichever one confirms next.
func StartDogfoodCohort(conn *sql.DB) (*DogfoodCohort, error) {
	if conn == nil {
		return nil, errors.New("database not initialized")
	}
	var activeID int64
	err := conn.QueryRow(`SELECT id FROM dogfood_cohorts WHERE completed_at IS NULL LIMIT 1`).Scan(&activeID)
	if err == nil {
		return nil, fmt.Errorf("a dogfood run is already active (cohort %d)", activeID)
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("check for an active dogfood run: %w", err)
	}
	now := time.Now().UTC()
	result, err := conn.Exec(`INSERT INTO dogfood_cohorts (started_at, target_count) VALUES (?, ?)`, now, DogfoodCohortTarget)
	if err != nil {
		return nil, fmt.Errorf("start dogfood cohort: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("read new dogfood cohort id: %w", err)
	}
	return &DogfoodCohort{ID: id, StartedAt: now, TargetCount: DogfoodCohortTarget}, nil
}

// GetActiveDogfoodCohort returns the open cohort, or nil when none is active.
func GetActiveDogfoodCohort(conn *sql.DB) (*DogfoodCohort, error) {
	if conn == nil {
		return nil, nil
	}
	var cohort DogfoodCohort
	var startedAt sql.NullTime
	err := conn.QueryRow(`SELECT id, started_at, target_count FROM dogfood_cohorts WHERE completed_at IS NULL LIMIT 1`).
		Scan(&cohort.ID, &startedAt, &cohort.TargetCount)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load active dogfood cohort: %w", err)
	}
	cohort.StartedAt = startedAt.Time
	if err := conn.QueryRow(`SELECT COUNT(*) FROM dogfood_cohort_applications WHERE cohort_id = ?`, cohort.ID).
		Scan(&cohort.CapturedCount); err != nil {
		return nil, fmt.Errorf("count dogfood cohort captures: %w", err)
	}
	return &cohort, nil
}

// ListDogfoodCohorts returns every cohort, most recent first, for the
// read-only history view. Completed cohorts never change once listed here.
func ListDogfoodCohorts(conn *sql.DB) ([]DogfoodCohortSummary, error) {
	if conn == nil {
		return nil, nil
	}
	rows, err := conn.Query(`SELECT c.id, c.started_at, c.completed_at, c.target_count,
		(SELECT COUNT(*) FROM dogfood_cohort_applications a WHERE a.cohort_id = c.id)
		FROM dogfood_cohorts c ORDER BY c.id DESC`)
	if err != nil {
		return nil, fmt.Errorf("list dogfood cohorts: %w", err)
	}
	defer rows.Close()
	var out []DogfoodCohortSummary
	for rows.Next() {
		var s DogfoodCohortSummary
		var started, completed sql.NullTime
		if err := rows.Scan(&s.ID, &started, &completed, &s.TargetCount, &s.CapturedCount); err != nil {
			return nil, fmt.Errorf("scan dogfood cohort: %w", err)
		}
		s.StartedAt = started.Time
		if completed.Valid {
			t := completed.Time
			s.CompletedAt = &t
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// captureDogfoodApplicationTx records one confirmed application against the
// active cohort, if any, as part of ConfirmAssistedSubmission's own
// transaction. It returns the 1-based ordinal it was captured at, or 0 when
// nothing was captured (no active cohort, or the cohort is already full).
//
// This is the only place a row is ever written to dogfood_cohort_applications
// from a real application. Because it runs inside the confirmation's own
// transaction, capture is atomic with the APPLIED write: a confirmation that
// commits always captures (if a cohort is open and has room), and a
// confirmation that rolls back never does. The composite primary key on
// dogfood_cohort_applications makes capturing the same job twice for the same
// cohort a no-op rather than an error.
func captureDogfoodApplicationTx(tx *sql.Tx, jobID string, now time.Time) (int, error) {
	var cohortID int64
	var target int
	err := tx.QueryRow(`SELECT id, target_count FROM dogfood_cohorts WHERE completed_at IS NULL LIMIT 1`).Scan(&cohortID, &target)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("check active dogfood cohort: %w", err)
	}
	var captured int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM dogfood_cohort_applications WHERE cohort_id = ?`, cohortID).Scan(&captured); err != nil {
		return 0, fmt.Errorf("count dogfood cohort captures: %w", err)
	}
	if captured >= target {
		return 0, nil
	}
	ordinal := captured + 1
	if _, err := tx.Exec(`INSERT OR IGNORE INTO dogfood_cohort_applications (cohort_id, job_id, ordinal, captured_at) VALUES (?, ?, ?, ?)`,
		cohortID, jobID, ordinal, now); err != nil {
		return 0, fmt.Errorf("capture dogfood application: %w", err)
	}
	if ordinal >= target {
		if _, err := tx.Exec(`UPDATE dogfood_cohorts SET completed_at = ? WHERE id = ? AND completed_at IS NULL`, now, cohortID); err != nil {
			return 0, fmt.Errorf("complete dogfood cohort: %w", err)
		}
	}
	return ordinal, nil
}

// RecordDogfoodFeedback stores the one operator-authored fact this feature
// needs that nothing else durably records: what slowed the operator down on
// one cohort application, in their own judgment. It targets the most
// recently started cohort's row for this job, which is correct because
// feedback is only ever solicited immediately after that same job's
// confirmation.
//
// category must be one of the fixed DogfoodFeedback* values -- never free
// text -- because the cohort report's verdict is computed from these counts,
// and a typo-prone free-form category would make that computation silently
// wrong. note is only ever kept when category is DogfoodFeedbackOther; it is
// operator-authored, about their own experience, and is never extracted from
// employer form content or an approved answer.
func RecordDogfoodFeedback(conn *sql.DB, jobID, category string, manualCount *int, note string) error {
	if conn == nil {
		return errors.New("database not initialized")
	}
	if !validDogfoodFeedbackCategories[category] {
		return fmt.Errorf("unsupported dogfood feedback category %q", category)
	}
	if manualCount != nil && *manualCount < 0 {
		return errors.New("manual field count cannot be negative")
	}
	if category != DogfoodFeedbackOther {
		note = ""
	}
	result, err := conn.Exec(`UPDATE dogfood_cohort_applications
		SET feedback_category = ?, feedback_manual_count = ?, feedback_note = ?
		WHERE job_id = ? AND cohort_id = (SELECT id FROM dogfood_cohorts ORDER BY id DESC LIMIT 1)`,
		category, manualCount, note, jobID)
	if err != nil {
		return fmt.Errorf("record dogfood feedback: %w", err)
	}
	if n, _ := result.RowsAffected(); n != 1 {
		return errors.New("no dogfood cohort application found for this job")
	}
	return nil
}

// DogfoodApplicationRecord is one cohort application's row in the report,
// joining its own captured membership against the durable data every other
// part of this project already keeps.
type DogfoodApplicationRecord struct {
	Ordinal              int    `json:"ordinal"`
	JobID                string `json:"job_id"`
	Company              string `json:"company,omitempty"`
	Role                 string `json:"role,omitempty"`
	ATS                  string `json:"ats,omitempty"`
	FitScore             *int   `json:"fit_score,omitempty"`
	FilledCount          int    `json:"filled_count"`
	ReusedAnswers        int    `json:"reused_answers"`
	UnresolvedCount      int    `json:"unresolved_count"`
	DocumentCount        int    `json:"document_count"`
	FillSource           string `json:"fill_source,omitempty"`
	InteractionSeconds   int    `json:"interaction_seconds"`
	HasInteractionTiming bool   `json:"has_interaction_timing"`
	FeedbackCategory     string `json:"feedback_category,omitempty"`
	FeedbackManualCount  *int   `json:"feedback_manual_count,omitempty"`
	FeedbackNote         string `json:"feedback_note,omitempty"`
}

// DogfoodFrictionEntry is one repeated-friction category ranked in the
// report, frequency first.
type DogfoodFrictionEntry struct {
	Category string `json:"category"`
	Count    int    `json:"count"`
}

// DogfoodReport is the automatic, sanitized report generated once a cohort's
// fifth application is captured (or on demand for a completed cohort).
// Nothing here is computed by an LLM; every count is either a durable
// existing measurement or a tally of the operator's own closed-vocabulary
// feedback.
type DogfoodReport struct {
	CohortID     int64                      `json:"cohort_id"`
	StartedAt    time.Time                  `json:"started_at"`
	CompletedAt  *time.Time                 `json:"completed_at,omitempty"`
	TargetCount  int                        `json:"target_count"`
	Applications []DogfoodApplicationRecord `json:"applications"`

	// Targeting -- the one judgment only the operator can make.
	PlausibleTargets int `json:"plausible_targets"`
	BadMatches       int `json:"bad_matches"`

	// Automation.
	TotalFieldsFilled   int     `json:"total_fields_filled"`
	AverageFieldsFilled float64 `json:"average_fields_filled"`
	TotalAnswersReused  int     `json:"total_answers_reused"`
	KnownFactsNotFilled int     `json:"known_facts_not_filled"`

	// Human effort.
	MedianInteractionSeconds    int     `json:"median_interaction_seconds"`
	AverageInteractionSeconds   float64 `json:"average_interaction_seconds"`
	ApplicationsWithTiming      int     `json:"applications_with_timing"`
	TotalManualFieldsHandled    int     `json:"total_manual_fields_handled"`
	ApplicationsWithManualCount int     `json:"applications_with_manual_count"`
	AverageManualFieldsHandled  float64 `json:"average_manual_fields_handled"`
	OneOffQuestions             int     `json:"one_off_questions"`
	RepeatedQuestions           int     `json:"repeated_questions"`

	// Correctness.
	WrongFills int `json:"wrong_fills"`
	Blocked    int `json:"blocked"`

	// ATS.
	ATSDistribution map[string]int `json:"ats_distribution"`

	// Friction categories other than expected one-off human work, ranked by
	// frequency descending.
	RepeatedFriction []DogfoodFrictionEntry `json:"repeated_friction"`

	// One of the DogfoodVerdict* constants, and the one concrete reason for
	// it. This harness recommends; it never starts coding the recommendation
	// itself.
	Verdict       string `json:"verdict"`
	VerdictReason string `json:"verdict_reason"`
}

// GetDogfoodReport computes the sanitized report for one cohort, joining its
// membership against job_funnel, application_preflight, assisted_fill_summary
// (via GetFillSummary, the same reader every other view of a fill uses) and
// human_interactions. Missing data in any of those -- no preflight row, no
// interaction timing -- produces a zero value here, never an error: a
// dogfood run against a database that has not recorded everything yet is the
// normal case, not a failure.
func GetDogfoodReport(conn *sql.DB, cohortID int64) (*DogfoodReport, error) {
	if conn == nil {
		return nil, errors.New("database not initialized")
	}
	report := &DogfoodReport{CohortID: cohortID, ATSDistribution: map[string]int{}}
	var started, completed sql.NullTime
	err := conn.QueryRow(`SELECT started_at, completed_at, target_count FROM dogfood_cohorts WHERE id = ?`, cohortID).
		Scan(&started, &completed, &report.TargetCount)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("no dogfood cohort %d", cohortID)
	}
	if err != nil {
		return nil, fmt.Errorf("load dogfood cohort: %w", err)
	}
	report.StartedAt = started.Time
	if completed.Valid {
		t := completed.Time
		report.CompletedAt = &t
	}

	rows, err := conn.Query(`SELECT job_id, ordinal, feedback_category, feedback_manual_count, feedback_note
		FROM dogfood_cohort_applications WHERE cohort_id = ? ORDER BY ordinal`, cohortID)
	if err != nil {
		return nil, fmt.Errorf("load dogfood cohort applications: %w", err)
	}
	type rawRow struct {
		jobID       string
		ordinal     int
		feedbackCat sql.NullString
		manualCount sql.NullInt64
		note        sql.NullString
	}
	var raws []rawRow
	for rows.Next() {
		var r rawRow
		if err := rows.Scan(&r.jobID, &r.ordinal, &r.feedbackCat, &r.manualCount, &r.note); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan dogfood cohort application: %w", err)
		}
		raws = append(raws, r)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()

	frictionCounts := map[string]int{}
	var interactionSeconds []int

	for _, r := range raws {
		rec := DogfoodApplicationRecord{Ordinal: r.ordinal, JobID: r.jobID}

		var company, title sql.NullString
		var fitScore sql.NullInt64
		_ = conn.QueryRow(`SELECT company_name, job_title, fit_score FROM job_funnel WHERE id = ?`, r.jobID).
			Scan(&company, &title, &fitScore)
		rec.Company = company.String
		rec.Role = title.String
		if fitScore.Valid {
			v := int(fitScore.Int64)
			rec.FitScore = &v
		}

		var ats sql.NullString
		_ = conn.QueryRow(`SELECT ats FROM application_preflight WHERE job_id = ?`, r.jobID).Scan(&ats)
		rec.ATS = ats.String
		if rec.ATS != "" {
			report.ATSDistribution[rec.ATS]++
		}

		summary, err := GetFillSummary(conn, r.jobID)
		if err != nil {
			return nil, fmt.Errorf("load fill summary for dogfood application %s: %w", r.jobID, err)
		}
		rec.FilledCount = summary.FilledCount
		rec.ReusedAnswers = summary.ReusedAnswers
		rec.UnresolvedCount = summary.UnresolvedCount
		rec.DocumentCount = len(summary.Documents)
		rec.FillSource = summary.FillSource
		report.TotalFieldsFilled += rec.FilledCount
		report.TotalAnswersReused += rec.ReusedAnswers

		var totalMs sql.NullInt64
		_ = conn.QueryRow(`SELECT SUM(duration_ms) FROM human_interactions WHERE job_id = ?`, r.jobID).Scan(&totalMs)
		if totalMs.Valid && totalMs.Int64 > 0 {
			rec.InteractionSeconds = int(totalMs.Int64 / 1000)
			rec.HasInteractionTiming = true
			interactionSeconds = append(interactionSeconds, rec.InteractionSeconds)
		}

		if r.feedbackCat.Valid {
			rec.FeedbackCategory = r.feedbackCat.String
			switch r.feedbackCat.String {
			case DogfoodFeedbackBadMatch:
				report.BadMatches++
			case DogfoodFeedbackKnownNotFilled:
				report.KnownFactsNotFilled++
			case DogfoodFeedbackFilledIncorrect:
				report.WrongFills++
			case DogfoodFeedbackOneOffQuestion:
				report.OneOffQuestions++
			case DogfoodFeedbackRepeatedQuestion:
				report.RepeatedQuestions++
			case DogfoodFeedbackBlocker:
				report.Blocked++
			}
			// Expected human work is never counted as friction, however often
			// it happens: a per-job essay or a genuinely unique question is
			// the product working as intended, not a defect repeating.
			if r.feedbackCat.String != DogfoodFeedbackNothing && r.feedbackCat.String != DogfoodFeedbackOneOffQuestion {
				frictionCounts[r.feedbackCat.String]++
			}
		}
		if r.manualCount.Valid {
			v := int(r.manualCount.Int64)
			rec.FeedbackManualCount = &v
			report.TotalManualFieldsHandled += v
			report.ApplicationsWithManualCount++
		}
		rec.FeedbackNote = r.note.String

		report.Applications = append(report.Applications, rec)
	}

	report.PlausibleTargets = len(raws) - report.BadMatches

	if len(report.Applications) > 0 {
		report.AverageFieldsFilled = float64(report.TotalFieldsFilled) / float64(len(report.Applications))
	}
	if len(interactionSeconds) > 0 {
		sort.Ints(interactionSeconds)
		report.MedianInteractionSeconds = interactionSeconds[len(interactionSeconds)/2]
		sum := 0
		for _, s := range interactionSeconds {
			sum += s
		}
		report.AverageInteractionSeconds = float64(sum) / float64(len(interactionSeconds))
		report.ApplicationsWithTiming = len(interactionSeconds)
	}
	if report.ApplicationsWithManualCount > 0 {
		report.AverageManualFieldsHandled = float64(report.TotalManualFieldsHandled) / float64(report.ApplicationsWithManualCount)
	}

	for category, count := range frictionCounts {
		report.RepeatedFriction = append(report.RepeatedFriction, DogfoodFrictionEntry{Category: category, Count: count})
	}
	sort.Slice(report.RepeatedFriction, func(i, j int) bool {
		if report.RepeatedFriction[i].Count != report.RepeatedFriction[j].Count {
			return report.RepeatedFriction[i].Count > report.RepeatedFriction[j].Count
		}
		return report.RepeatedFriction[i].Category < report.RepeatedFriction[j].Category
	})

	report.Verdict, report.VerdictReason = dogfoodVerdict(report.WrongFills, report.RepeatedFriction)

	return report, nil
}

// dogfoodVerdict is the whole go/no-go decision, and deliberately the
// smallest thing that could implement it: a wrong fill anywhere is a
// correctness problem regardless of frequency, and short of that the most
// frequent repeated (non-expected) friction category is worth naming only
// once it happened on at least two of the five applications. Anything less
// is normal one-off employer variation, not evidence of a defect.
func dogfoodVerdict(wrongFills int, friction []DogfoodFrictionEntry) (string, string) {
	if wrongFills > 0 {
		return DogfoodVerdictPause, fmt.Sprintf("Career Agent filled a field incorrectly on %d of the five applications.", wrongFills)
	}
	if len(friction) > 0 && friction[0].Count >= 2 {
		return DogfoodVerdictFixOne, fmt.Sprintf("%s occurred on %d of the five applications.", dogfoodFrictionLabel(friction[0].Category), friction[0].Count)
	}
	return DogfoodVerdictKeepUsing, "No repeated automation or correctness problem was reported across the five applications."
}

func dogfoodFrictionLabel(category string) string {
	switch category {
	case DogfoodFeedbackBadMatch:
		return "An irrelevant job match"
	case DogfoodFeedbackKnownNotFilled:
		return "A known fact not being filled"
	case DogfoodFeedbackFilledIncorrect:
		return "An incorrect fill"
	case DogfoodFeedbackRepeatedQuestion:
		return "A repeated question Career Agent should already know"
	case DogfoodFeedbackBlocker:
		return "A browser/application blocker"
	case DogfoodFeedbackOther:
		return "An unclassified issue"
	default:
		return category
	}
}
