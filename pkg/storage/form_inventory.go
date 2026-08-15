package storage

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Form-inventory states: what Career Agent knows about *this employer's form*,
// as opposed to what it knows about the operator.
//
// The distinction the vocabulary exists to protect is the one bugs.md #547 was
// filed for. A packet that lists stored details and says nothing else is
// reporting one of two completely different facts -- "this form asks nothing
// beyond what you see" or "nobody has ever read this form" -- and the operator
// filling the form by hand is the person who most needs to know which. The
// schema could already tell them apart (`application_preflight`'s own comment
// says so: "we looked and found nothing" and "we could not look" are different
// facts); nothing on the packet path ever asked.
//
// FormInventoryPreparing is deliberately not derivable from the database. A
// preparation run is a live child process, so the process that spawned it is
// the only thing that knows it is still going; DeriveFormInventory reports the
// durable states and the caller overlays this one.
const (
	// FormInventoryNotPrepared: no inspection has ever been recorded for this
	// application. The packet holds stored details only and must say so.
	FormInventoryNotPrepared = "not_prepared"
	// FormInventoryPreparing: an inspection is running right now.
	FormInventoryPreparing = "preparing"
	// FormInventoryReady: a form was successfully read. QuestionCount may be
	// zero -- that is the "inspected, asks nothing extra" case, and it is a
	// different fact from not_prepared, never rendered the same way.
	FormInventoryReady = "ready"
	// FormInventoryFailed: the most recent attempt to read the form did not
	// succeed. Reason carries a bounded code, never a driver message.
	FormInventoryFailed = "failed"
)

// Where a recorded inventory came from. Both are real inspections of a live
// form; they differ only in which program did the reading, and the packet says
// which so "prepared in advance" is not confused with "seen during a session".
const (
	FormInventorySourcePreflight = "preflight"
	FormInventorySourceSession   = "assisted_session"
)

// FormInventory is what Career Agent can honestly claim about one employer's
// application form.
//
// State and QuestionCount are two axes on purpose. Reading preparation state
// off `len(questions)` is the defect this type replaces, and it is the same
// mistake bugs.md #548 makes one table over -- inferring that a fill ran from a
// row written by preparation. A count answers "how much is in the packet"; it
// has never answered "did anyone look".
type FormInventory struct {
	State         string `json:"state"`
	QuestionCount int    `json:"question_count"`
	// FieldCount is the whole form's control count as counted by preflight,
	// which is a different number from QuestionCount: `application_questions`
	// holds only what Career Agent *cannot* answer, so a form with 21 controls
	// and 19 resolvable ones is a 21-field form with 2 questions.
	FieldCount int `json:"field_count"`
	// Reason is a bounded code from pkg/submitter's closed vocabulary as
	// recorded by the preparation run, empty unless State is failed.
	Reason      string `json:"reason,omitempty"`
	InspectedAt string `json:"inspected_at,omitempty"`
	Source      string `json:"source,omitempty"`
	// Preparable reports whether asking for an inspection right now would do
	// anything. It is answered by PreflightCandidates -- the function the
	// preparation run itself uses -- so the packet cannot offer an action that
	// the run would then refuse.
	Preparable bool `json:"preparable"`
	// BlockedKind is storage's own vocabulary (PreflightSkipUnreadable /
	// PreflightSkipAlreadyApplied), empty when Preparable. Callers map it onto
	// pkg/submitter's reason codes the way cmd/preflight does; storage does not
	// import that package.
	BlockedKind string `json:"-"`
}

// DeriveFormInventory reports what is durably known about one application's
// form.
//
// The order of the rules is the whole point, so it is spelled out rather than
// left to be reconstructed from the code:
//
//  1. A recorded preflight verdict is the strongest evidence and wins. It is
//     the only record that distinguishes a successful read from a failed one,
//     which is exactly what `application_preflight` was added for.
//  2. With no verdict, question rows still prove a form was read -- a live
//     assisted session records them too (assisted.go's ReplaceApplicationQuestions
//     call). Treating "no preflight row" as "never prepared" would put a
//     "nobody has read this form" banner directly above questions read off that
//     very form, which is the original defect with its sign flipped.
//  3. Only with neither is the honest answer that nothing has ever looked.
//
// It deliberately does not consult `assisted_fill_summary`. That table's
// recorded_at is stamped by preparation as well as by filling (bugs.md #548),
// so it cannot tell the two apart and must not be made load-bearing for a
// third meaning.
func DeriveFormInventory(conn *sql.DB, jobID string) (FormInventory, error) {
	if conn == nil {
		return FormInventory{}, errors.New("database not initialized")
	}
	if strings.TrimSpace(jobID) == "" {
		return FormInventory{}, errors.New("a form inventory needs a job identifier")
	}

	inventory := FormInventory{State: FormInventoryNotPrepared}

	// Whether an inspection is even possible comes from the function the
	// preparation run uses to pick its own work, so the packet never offers a
	// Prepare action that the run would immediately skip.
	candidates, err := PreflightCandidates(conn, []string{jobID})
	if err != nil {
		return FormInventory{}, err
	}
	if len(candidates) > 0 {
		if candidates[0].Skip == "" {
			inventory.Preparable = true
		} else {
			inventory.BlockedKind = candidates[0].SkipKind
		}
	}

	questions, err := countPendingQuestions(conn, jobID)
	if err != nil {
		return FormInventory{}, err
	}
	inventory.QuestionCount = questions

	recorded, err := countRecordedQuestions(conn, jobID)
	if err != nil {
		return FormInventory{}, err
	}

	verdicts, err := PreflightResults(conn, []string{jobID})
	if err != nil {
		return FormInventory{}, err
	}
	if len(verdicts) > 0 {
		verdict := verdicts[0]
		inventory.InspectedAt = verdict.InspectedAt
		inventory.Source = FormInventorySourcePreflight
		switch verdict.State {
		case PreflightInspected:
			inventory.State = FormInventoryReady
			inventory.FieldCount = verdict.ControlCount
		default:
			// The latest attempt to read this form did not succeed, and that is
			// what the state reports even when an earlier attempt left rows
			// behind. Any such rows are still shown -- they are real -- under a
			// banner saying the last read failed, which is truthful in the safe
			// direction: it can understate what Career Agent has, never
			// overstate it.
			inventory.State = FormInventoryFailed
			inventory.Reason = verdict.Reason
		}
		return inventory, nil
	}

	if recorded > 0 {
		inventory.State = FormInventoryReady
		inventory.Source = FormInventorySourceSession
		return inventory, nil
	}
	return inventory, nil
}

func countPendingQuestions(conn *sql.DB, jobID string) (int, error) {
	return countQuestions(conn,
		`SELECT COUNT(*) FROM application_questions WHERE job_id = ? AND status = ?`,
		jobID, QuestionPending)
}

func countRecordedQuestions(conn *sql.DB, jobID string) (int, error) {
	return countQuestions(conn,
		`SELECT COUNT(*) FROM application_questions WHERE job_id = ?`, jobID)
}

// countQuestions treats an absent table as zero rather than as an error, the
// same way every other reader in this package does: a database that predates
// the question schema has no inventory, which is a fact this feature is built
// to report rather than a failure to report anything.
func countQuestions(conn *sql.DB, query string, args ...any) (int, error) {
	var count int
	if err := conn.QueryRow(query, args...).Scan(&count); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "no such table") {
			return 0, nil
		}
		return 0, fmt.Errorf("count application questions: %w", err)
	}
	return count, nil
}

// FormInventoryIsStale reports whether a successful inspection is old enough
// that the posting may have changed underneath it.
//
// This is a note on the packet, never a downgrade of the state: a stale
// inventory is still a real reading of a real form, and demoting it to
// "not prepared" would tell the operator that Career Agent has nothing when it
// has something. The window is deliberately generous -- postings do change, but
// an inventory a few days old is overwhelmingly still the same form, and
// crying stale on every packet would train the operator to ignore the notice.
func FormInventoryIsStale(inventory FormInventory, now time.Time) bool {
	if inventory.State != FormInventoryReady || inventory.InspectedAt == "" {
		return false
	}
	inspected, err := time.Parse(time.RFC3339, inventory.InspectedAt)
	if err != nil {
		return false
	}
	return now.UTC().Sub(inspected.UTC()) > formInventoryStaleAfter
}

const formInventoryStaleAfter = 14 * 24 * time.Hour
