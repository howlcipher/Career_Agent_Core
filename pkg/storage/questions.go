package storage

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/howlcipher/Career_Agent_Core/pkg/answers"
)

// Question statuses. A question is pending until the operator deals with it,
// and then it is either answered or explicitly skipped. There is no state that
// means "answered by Career Agent": anything Career Agent could answer never
// became a question in the first place.
const (
	QuestionPending  = "pending"
	QuestionAnswered = "answered"
	QuestionSkipped  = "skipped"
)

// ApplicationQuestion is one thing an application form needs from the operator.
//
// Note what is not here: the answer. This table records that a question was
// asked and whether it has been dealt with, never what was said.
//
// An answer does have to cross a process boundary — the dashboard receives it
// and the separate cmd/assist process types it into the browser — and SQLite is
// the only channel between them, the same one every other assisted transition
// already uses. So an answer is written to pending_answers, read once by the
// assisted process, and deleted in the same transaction that reads it. It
// exists in the database for the seconds between the operator pressing Send and
// the browser receiving it, and nowhere else. This project does not keep a
// record of every answer typed onto every real application, because it does not
// need one to work. The single deliberate exception is the Approved Answer
// Vault, where an answer is kept only because the operator explicitly asked for
// it to be remembered.
type ApplicationQuestion struct {
	ID          int64    `json:"id"`
	JobID       string   `json:"job_id"`
	Key         string   `json:"key"`
	Prompt      string   `json:"prompt"`
	ControlType string   `json:"control_type"`
	Options     []string `json:"options,omitempty"`
	Required    bool     `json:"required"`
	Status      string   `json:"status"`
	Sensitivity string   `json:"sensitivity"`
	// Suggested is Career Agent's proposal, shown pre-filled so the operator
	// confirms instead of retyping. It is derived from the operator's own
	// configured facts or from an answer they previously approved, never from
	// the employer's page.
	Suggested   string `json:"suggested,omitempty"`
	Source      string `json:"source,omitempty"`
	LabelUnsafe bool   `json:"label_unsafe,omitempty"`
	// CanonicalKey groups this question with the same question asked by other
	// employers. It is derived from Prompt by the store, never supplied by a
	// caller, and it is not the same thing as Key -- see the column comment in
	// EnsureQuestionSchema.
	CanonicalKey string `json:"canonical_key,omitempty"`
	// AutoFillable is what the vault concluded about this question the last
	// time it was re-evaluated: Career Agent may type this answer without
	// asking. Advisory only; the live browser re-resolves and wins.
	AutoFillable bool   `json:"auto_fillable,omitempty"`
	CreatedAt    string `json:"created_at"`
}

// AssistedFillSummary is the "Career Agent completed" half of a card: counts
// and labels of what was filled, with no page content and no values.
type AssistedFillSummary struct {
	JobID           string   `json:"job_id"`
	FilledCount     int      `json:"filled_count"`
	ReusedAnswers   int      `json:"reused_answers"`
	Documents       []string `json:"documents"`
	FilledLabels    []string `json:"filled_labels"`
	UnresolvedCount int      `json:"unresolved_count"`
	// RecordedAt is when this row was last written, by preparation or by
	// filling. It is deliberately NOT evidence that a fill ran -- that
	// conflation is bugs.md #548, and FillAttemptedAt exists because of it.
	RecordedAt string `json:"recorded_at"`
	// FillAttemptedAt is when Career Agent last began actually filling this
	// employer's form, and empty when no fill attempt has ever been recorded.
	//
	// The two-line invariant this whole field exists to hold:
	//
	//	FormInventory = what Career Agent knows about the form.
	//	FillSummary   = what Career Agent actually did to the form.
	//
	// Three properties are load-bearing and each closes a way this could lie:
	//
	//  1. It is stamped when a fill *begins*, not when one succeeds. A fill
	//     that types nothing, one that errors halfway, and one whose closing
	//     snapshot fails are all attempts, and every one of them used to write
	//     nothing at all.
	//  2. Preparation cannot set it. Not "does not today" -- cannot:
	//     RecordPreparedQuestions has no parameter through which it could, which
	//     is the difference between an invariant and a convention that held
	//     until someone edited a call site.
	//  3. Empty means *unknown*, never "no fill ran". Every row written before
	//     this column existed is unknown, and the migration deliberately does
	//     not invent history for them.
	FillAttemptedAt string `json:"fill_attempted_at,omitempty"`
}

// EnsureQuestionSchema creates the per-application question log.
func EnsureQuestionSchema(conn *sql.DB) error {
	if conn == nil {
		return errors.New("question schema connection is nil")
	}
	_, err := conn.Exec(`CREATE TABLE IF NOT EXISTS application_questions (
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
		-- canonical_key is what makes the same question asked by nine employers
		-- one thing to answer instead of nine. question_key above is the DOM
		-- control's key (name, then id, then a hash of the label) and is unique
		-- to one rendering of one form; canonical_key is derived from the
		-- question's *text* and is therefore comparable across applications.
		-- The two are separate columns because they answer different questions:
		-- question_key decides which control an answer is typed into,
		-- canonical_key decides which questions are the same question.
		canonical_key TEXT NOT NULL DEFAULT '',
		-- auto_fillable records what the vault concluded the last time this row
		-- was re-evaluated. It is advisory: the live browser re-resolves from
		-- the vault when it opens the page and always wins. Keeping it here is
		-- what lets the dashboard say "9 of these are now resolved" without
		-- opening nine browsers to find out.
		auto_fillable INTEGER NOT NULL DEFAULT 0,
		created_at DATETIME NOT NULL,
		answered_at DATETIME,
		UNIQUE(job_id, question_key)
	);
	-- What Career Agent actually *did* to one employer's form, as opposed to
	-- what it knows about that form (which is application_preflight, read
	-- through DeriveFormInventory). The two are separate tables because they
	-- are separate facts, and bugs.md #548 was filed for what happens when one
	-- is read as the other.
	CREATE TABLE IF NOT EXISTS assisted_fill_summary (
		job_id INTEGER PRIMARY KEY,
		filled_count INTEGER NOT NULL DEFAULT 0,
		reused_answers INTEGER NOT NULL DEFAULT 0,
		documents TEXT NOT NULL DEFAULT '',
		filled_labels TEXT NOT NULL DEFAULT '',
		unresolved_count INTEGER NOT NULL DEFAULT 0,
		-- When this row was last touched, by either operation. Preparation
		-- writes it too, so it says nothing about filling.
		recorded_at DATETIME NOT NULL,
		-- When a fill last *began*. NULL means no fill attempt has ever been
		-- recorded -- which for rows written before this column existed is
		-- genuinely unknown, not a denial. Nullable on purpose: a NOT NULL
		-- column with a zero default would turn "we never knew" into "it never
		-- happened", which is the same class of lie as #548 itself.
		fill_attempted_at DATETIME
	);
	-- Transient. A row here lives only from the moment the operator presses
	-- Send to the moment the assisted browser reads it, and TakePendingAnswers
	-- deletes it in the same transaction that returns it.
	CREATE TABLE IF NOT EXISTS pending_answers (
		job_id INTEGER NOT NULL,
		question_key TEXT NOT NULL,
		answer_text TEXT NOT NULL,
		created_at DATETIME NOT NULL,
		PRIMARY KEY (job_id, question_key)
	);
	-- Preflight's per-application verdict. Deliberately separate from
	-- application_questions: "we looked and found nothing" and "we could not
	-- look" are different facts, and only this table can tell them apart.
	CREATE TABLE IF NOT EXISTS application_preflight (
		job_id INTEGER PRIMARY KEY,
		state TEXT NOT NULL,
		reason TEXT NOT NULL DEFAULT '',
		ats TEXT NOT NULL DEFAULT '',
		control_count INTEGER NOT NULL DEFAULT 0,
		inspected_at DATETIME NOT NULL
	);
	CREATE INDEX IF NOT EXISTS idx_application_questions_job ON application_questions(job_id, status);`)
	if err != nil {
		return fmt.Errorf("create question schema: %w", err)
	}
	return upgradeQuestionSchema(conn)
}

// upgradeQuestionSchema adds the columns a database created before them is
// missing, and backfills the one that can be derived from what is already
// stored.
//
// The backfill matters more than it looks. canonical_key is how every
// cross-application view finds its rows, so a database that upgraded without it
// would not be missing a column -- it would silently report that the operator
// has no repeated questions, which is indistinguishable from good news.
func upgradeQuestionSchema(conn *sql.DB) error {
	if err := addQuestionSchemaColumns(conn, "application_questions", []questionSchemaColumn{
		{"canonical_key", "TEXT NOT NULL DEFAULT ''"},
		{"auto_fillable", "INTEGER NOT NULL DEFAULT 0"},
	}); err != nil {
		return err
	}
	// fill_attempted_at is added here, in EnsureQuestionSchema's own upgrade,
	// rather than in InitDBWithPath's migration chain. cmd/dashboard
	// deliberately never runs that chain -- it calls the Ensure*Schema
	// functions directly -- so a column added there would be missing on a
	// dashboard-first startup, and the dashboard is the one process that reads
	// this column to decide what to tell the operator.
	//
	// Nullable with no default, so existing rows become NULL rather than a
	// timestamp nobody earned.
	if err := addQuestionSchemaColumns(conn, "assisted_fill_summary", []questionSchemaColumn{
		{"fill_attempted_at", "DATETIME"},
	}); err != nil {
		return err
	}
	if err := backfillFillAttempts(conn); err != nil {
		return err
	}
	// After the ALTERs, never alongside the CREATE TABLE: on a database that
	// already had the table, the column this index covers does not exist until
	// the loop above has run, and SQLite rejects the whole batch.
	if _, err := conn.Exec(`CREATE INDEX IF NOT EXISTS idx_application_questions_canonical
		ON application_questions(canonical_key, status)`); err != nil {
		return fmt.Errorf("index questions by canonical key: %w", err)
	}
	return backfillCanonicalKeys(conn)
}

// backfillFillAttempts recovers the one class of historical row whose
// provenance is not in doubt.
//
// The rule for bugs.md #548 is that an absent marker means *unknown* and must
// never be manufactured into a yes or a no. That rule protects rows that carry
// no evidence. It was never meant to discard evidence that does exist, and a
// row with a non-zero filled_count, reused_answers or documents carries the
// strongest kind there is: those columns are only ever written from a fill
// report, preparation has never written them non-zero, and since #548's writer
// split it cannot write them at all. DeriveFormInventory already relies on
// exactly this inference to conclude a form was read.
//
// So refusing to backfill these would not have been caution, it would have
// been converting known work into unknown work -- and the card renders unknown
// as "no fill has been recorded", which about a row holding eight filled fields
// is #548 with its sign flipped. Found by two of the three independent
// reviewers, who each noticed the contradiction with form_inventory.go.
//
// recorded_at is the best available timestamp for when that fill happened; on
// such a row it was written by the fill itself. Rows with no evidence are left
// NULL, which is the whole point.
func backfillFillAttempts(conn *sql.DB) error {
	if _, err := conn.Exec(`UPDATE assisted_fill_summary
		SET fill_attempted_at = recorded_at
		WHERE fill_attempted_at IS NULL
		  AND recorded_at != ''
		  AND (filled_count > 0 OR reused_answers > 0 OR documents != '')`); err != nil {
		return fmt.Errorf("backfill fill attempts: %w", err)
	}
	return nil
}

type questionSchemaColumn struct{ name, definition string }

// addQuestionSchemaColumns adds whichever of the named columns the table does
// not already have. Re-entrant by construction: every startup runs it, and a
// database that already has the column takes no action.
func addQuestionSchemaColumns(conn *sql.DB, table string, columns []questionSchemaColumn) error {
	rows, err := conn.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		return fmt.Errorf("inspect %s schema: %w", table, err)
	}
	existing := map[string]bool{}
	// PRAGMA table_info on a table that does not exist returns zero rows and
	// no error, so an absent table is indistinguishable here from a table with
	// no columns -- and every ALTER below would then fail with "no such table".
	// Callers create the table first, but this helper is generic now, so it
	// says so rather than relying on that.
	for rows.Next() {
		var cid, notNull, pk int
		var name, columnType string
		var defaultValue sql.NullString
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &pk); err != nil {
			rows.Close()
			return fmt.Errorf("scan %s schema: %w", table, err)
		}
		existing[name] = true
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	if len(existing) == 0 {
		return nil
	}

	for _, column := range columns {
		if existing[column.name] {
			continue
		}
		if _, err := conn.Exec("ALTER TABLE " + table + " ADD COLUMN " + column.name + " " + column.definition); err != nil {
			// Every process runs this on startup, and the PRAGMA and the ALTER
			// are two statements with no lock between them: cmd/dashboard,
			// cmd/agent and cmd/assist starting together can all observe the
			// column missing and all try to add it. The losers get "duplicate
			// column name", which is the migration having already succeeded,
			// not a failure -- and cmd/dashboard turns an error here into
			// log.Fatalf, so treating it as one would take the dashboard down
			// on exactly the one startup where an upgrade is happening.
			if strings.Contains(strings.ToLower(err.Error()), "duplicate column name") {
				continue
			}
			return fmt.Errorf("add %s schema column %s: %w", table, column.name, err)
		}
	}
	return nil
}

// backfillCanonicalKeys fills in any row whose canonical key was never
// computed, whether because it predates the column or because it was written
// with an empty prompt.
func backfillCanonicalKeys(conn *sql.DB) error {
	rows, err := conn.Query(`SELECT id, prompt_text FROM application_questions WHERE canonical_key = ''`)
	if err != nil {
		return fmt.Errorf("read questions needing a canonical key: %w", err)
	}
	type pending struct {
		id     int64
		prompt string
	}
	var todo []pending
	for rows.Next() {
		var row pending
		if err := rows.Scan(&row.id, &row.prompt); err != nil {
			rows.Close()
			return fmt.Errorf("scan question needing a canonical key: %w", err)
		}
		todo = append(todo, row)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	for _, row := range todo {
		key := CanonicalQuestionKey(row.prompt)
		if key == "" {
			continue
		}
		if _, err := conn.Exec(`UPDATE application_questions SET canonical_key = ? WHERE id = ?`, key, row.id); err != nil {
			return fmt.Errorf("backfill canonical key: %w", err)
		}
	}
	return nil
}

// CanonicalQuestionKey reduces an employer's question text to the key every
// cross-application view groups on.
//
// It is a thin wrapper over answers.QuestionKey rather than its own
// normalization, because two normalizations of the same text is exactly how a
// question ends up in one group here and a different one in the vault -- and
// the vault's is the one that decides whether an approved answer applies.
func CanonicalQuestionKey(prompt string) string {
	return answers.QuestionKey(prompt)
}

// ReplaceApplicationQuestions records the outcome of one refill: the questions
// that still need the operator, and the summary of what was completed.
//
// Replacing rather than appending is deliberate. A second refill of the same
// application observes the form as it is *now*; a question the operator has
// since answered is no longer pending, and carrying stale rows forward would
// show them work they have already done. Rows the operator has already answered
// are preserved so the count of what they did survives the refresh.
//
// This is the *fill* writer. It is the only path that may write the fill
// columns, and it stamps fill_attempted_at because reaching it means a fill
// ran. Preparation uses RecordPreparedQuestions, which cannot reach them --
// see that function for why the split exists at all.
func ReplaceApplicationQuestions(conn *sql.DB, jobID string, questions []ApplicationQuestion, summary AssistedFillSummary) error {
	return replaceQuestions(conn, jobID, questions, &summary)
}

// RecordPreparedQuestions records what an inspection found: the questions this
// form asks and how many are outstanding. Nothing else.
//
// It takes no summary, and that absence is the entire point. Preparation and
// filling used to share one function with a summary parameter, and preparation
// passed a zero value through it -- which meant preparation both *stamped*
// recorded_at (bugs.md #548: the card then reported a fill that never ran) and
// *erased* any real fill's filled_count, documents and labels on its way past,
// because the upsert set every column from the zero value it had been handed.
//
// The fix is not to remember not to do that. It is to remove the parameter, so
// a preparation run has no argument through which it could say anything about
// filling even if a future call site tried. What survives a preparation run
// untouched: filled_count, reused_answers, documents, filled_labels and
// fill_attempted_at.
//
// recorded_at is still written, and still means only "this row was last
// touched". unresolved_count is still written because it is preparation's own
// finding and pkg/storage/knowledge.go reads it as a field-count fallback.
func RecordPreparedQuestions(conn *sql.DB, jobID string, questions []ApplicationQuestion) error {
	return replaceQuestions(conn, jobID, questions, nil)
}

// replaceQuestions is the shared body. A nil summary means preparation: write
// the questions and the preparation-owned columns, and leave every fill column
// exactly as it was.
func replaceQuestions(conn *sql.DB, jobID string, questions []ApplicationQuestion, summary *AssistedFillSummary) error {
	if conn == nil {
		return errors.New("database not initialized")
	}
	if strings.TrimSpace(jobID) == "" {
		return errors.New("an application question needs a job identifier")
	}
	tx, err := conn.Begin()
	if err != nil {
		return fmt.Errorf("begin question update: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`DELETE FROM application_questions WHERE job_id = ? AND status = ?`, jobID, QuestionPending); err != nil {
		return fmt.Errorf("clear stale questions: %w", err)
	}
	now := time.Now().UTC()
	for _, question := range questions {
		options := ""
		if len(question.Options) > 0 {
			encoded, err := json.Marshal(question.Options)
			if err != nil {
				return fmt.Errorf("encode question options: %w", err)
			}
			options = string(encoded)
		}
		// The canonical key is computed here rather than taken from the caller,
		// for the reason answers.Store.Save enforces its own rule rather than
		// trusting a handler: a check at the single write point protects every
		// caller, including the ones written next year. A caller that forgot it
		// would not produce an error, it would produce a question that silently
		// belongs to no group.
		canonical := CanonicalQuestionKey(question.Prompt)
		if _, err := tx.Exec(`INSERT INTO application_questions
			(job_id, question_key, prompt_text, control_type, options_json, required, status, sensitivity, proposed_answer, answer_source, label_unsafe, canonical_key, auto_fillable, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(job_id, question_key) DO UPDATE SET
				prompt_text = excluded.prompt_text,
				control_type = excluded.control_type,
				options_json = excluded.options_json,
				required = excluded.required,
				sensitivity = excluded.sensitivity,
				proposed_answer = excluded.proposed_answer,
				answer_source = excluded.answer_source,
				label_unsafe = excluded.label_unsafe,
				canonical_key = excluded.canonical_key,
				auto_fillable = excluded.auto_fillable`,
			jobID, question.Key, question.Prompt, question.ControlType, options,
			boolToInt(question.Required), QuestionPending, question.Sensitivity,
			question.Suggested, question.Source, boolToInt(question.LabelUnsafe),
			canonical, boolToInt(question.AutoFillable), now); err != nil {
			return fmt.Errorf("record application question: %w", err)
		}
	}

	if summary == nil {
		// Preparation. Touch only what an inspection actually learned. The
		// fill columns are absent from the UPDATE clause rather than being
		// written back with their current values, so there is no version of
		// this statement that could carry a stale or zeroed fill outcome.
		if _, err := tx.Exec(`INSERT INTO assisted_fill_summary
			(job_id, unresolved_count, recorded_at)
			VALUES (?, ?, ?)
			ON CONFLICT(job_id) DO UPDATE SET
				unresolved_count = excluded.unresolved_count,
				recorded_at = excluded.recorded_at`,
			jobID, len(questions), now); err != nil {
			return fmt.Errorf("record prepared question summary: %w", err)
		}
		return tx.Commit()
	}

	documents := strings.Join(summary.Documents, ",")
	labels := strings.Join(summary.FilledLabels, "\n")
	if _, err := tx.Exec(`INSERT INTO assisted_fill_summary
		(job_id, filled_count, reused_answers, documents, filled_labels, unresolved_count, recorded_at, fill_attempted_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(job_id) DO UPDATE SET
			filled_count = excluded.filled_count,
			reused_answers = excluded.reused_answers,
			documents = excluded.documents,
			filled_labels = excluded.filled_labels,
			unresolved_count = excluded.unresolved_count,
			recorded_at = excluded.recorded_at,
			-- COALESCE, not excluded: MarkFillAttempted already stamped the
			-- moment this fill *began*, and that is the time the column is
			-- defined to hold. Overwriting it here would quietly redefine it as
			-- the moment the fill ended, and would erase the distinction on the
			-- paths that matter most -- the ones where a fill started and never
			-- reached this statement at all.
			--
			-- The excluded value is still supplied so that a fill which somehow
			-- reaches this writer without a marker still records one. This
			-- statement can only be reached by a fill.
			fill_attempted_at = COALESCE(assisted_fill_summary.fill_attempted_at, excluded.fill_attempted_at)`,
		jobID, summary.FilledCount, summary.ReusedAnswers, documents, labels, len(questions), now, now); err != nil {
		return fmt.Errorf("record assisted fill summary: %w", err)
	}
	return tx.Commit()
}

// MarkFillAttempted records that Career Agent has begun filling this form.
//
// It is called before the fill runs, not after, which is what makes it honest
// about the outcomes that produce no summary at all: a fill that errors, a
// browser the operator closes mid-fill, a fill whose closing snapshot fails.
// Every one of those used to leave no trace whatsoever, so the card had no way
// to distinguish them from an application nobody had touched.
//
// An attempt is not a claim of success. A fill that runs and completes zero
// fields is still an attempt, and the card is allowed to say so only because
// this marker is set independently of what the fill achieved.
//
// It upserts rather than updates: RecordAssistedAnswersApplied's bare UPDATE
// silently writes nothing when no row exists yet, and an attempt marker that
// disappears on the one path where no preparation preceded it would be worse
// than no marker.
func MarkFillAttempted(conn *sql.DB, jobID string, now time.Time) error {
	if conn == nil {
		return errors.New("database not initialized")
	}
	if strings.TrimSpace(jobID) == "" {
		return errors.New("a fill attempt needs a job identifier")
	}
	stamp := now.UTC()
	// recorded_at is NOT NULL, so the insert branch must supply something, and
	// it deliberately supplies the empty string rather than a timestamp.
	//
	// This is not squeamishness about a column default. cmd/dashboard's
	// assistedReviewStartedAt reads recorded_at as "when this application
	// became ready for the operator to review", and writing a real timestamp
	// here would start that clock on a job that had never been prepared --
	// turning a fill that failed at 10:00 and forty minutes of the operator's
	// own typing into forty minutes of recorded "review time". This function
	// exists to report that a fill began; it must not also decide when the
	// operator's review began. An unparseable value round-trips through
	// GetFillSummary as an absent RecordedAt, which is the honest answer:
	// nothing has written a review-ready time for this application.
	//
	// The conflict branch leaves recorded_at alone for the same reason.
	if _, err := conn.Exec(`INSERT INTO assisted_fill_summary
		(job_id, unresolved_count, recorded_at, fill_attempted_at)
		VALUES (?, 0, '', ?)
		ON CONFLICT(job_id) DO UPDATE SET
			fill_attempted_at = excluded.fill_attempted_at`,
		jobID, stamp); err != nil {
		return fmt.Errorf("mark fill attempted: %w", err)
	}
	return nil
}

// GetPendingQuestions returns what still needs the operator for one job.
func GetPendingQuestions(conn *sql.DB, jobID string) ([]ApplicationQuestion, error) {
	if conn == nil {
		return nil, errors.New("database not initialized")
	}
	rows, err := conn.Query(`SELECT id, job_id, question_key, prompt_text, control_type, options_json,
		required, status, sensitivity, proposed_answer, answer_source, label_unsafe,
		canonical_key, auto_fillable, created_at
		FROM application_questions WHERE job_id = ? AND status = ?
		ORDER BY required DESC, id ASC`, jobID, QuestionPending)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "no such table") {
			return []ApplicationQuestion{}, nil
		}
		return nil, fmt.Errorf("load application questions: %w", err)
	}
	defer rows.Close()
	var out []ApplicationQuestion
	for rows.Next() {
		var question ApplicationQuestion
		var options string
		var required, labelUnsafe, autoFillable int
		var createdAt sql.NullString
		if err := rows.Scan(&question.ID, &question.JobID, &question.Key, &question.Prompt,
			&question.ControlType, &options, &required, &question.Status, &question.Sensitivity,
			&question.Suggested, &question.Source, &labelUnsafe,
			&question.CanonicalKey, &autoFillable, &createdAt); err != nil {
			return nil, err
		}
		question.Required = required != 0
		question.LabelUnsafe = labelUnsafe != 0
		question.AutoFillable = autoFillable != 0
		if options != "" {
			_ = json.Unmarshal([]byte(options), &question.Options)
		}
		if parsed, ok := parseAssistedTime(createdAt.String); ok {
			question.CreatedAt = parsed.Format(time.RFC3339)
		}
		out = append(out, question)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if out == nil {
		out = []ApplicationQuestion{}
	}
	return out, nil
}

// GetFillSummary returns what Career Agent completed for one job, and -- via
// FillAttemptedAt -- whether it ever tried. Those are different questions, and
// answering the second with the first is bugs.md #548.
func GetFillSummary(conn *sql.DB, jobID string) (AssistedFillSummary, error) {
	summary := AssistedFillSummary{JobID: jobID}
	if conn == nil {
		return summary, errors.New("database not initialized")
	}
	var documents, labels string
	var recordedAt, fillAttemptedAt sql.NullString
	err := conn.QueryRow(`SELECT filled_count, reused_answers, documents, filled_labels, unresolved_count, recorded_at, fill_attempted_at
		FROM assisted_fill_summary WHERE job_id = ?`, jobID).
		Scan(&summary.FilledCount, &summary.ReusedAnswers, &documents, &labels, &summary.UnresolvedCount, &recordedAt, &fillAttemptedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return summary, nil
	}
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "no such table") {
			return summary, nil
		}
		return summary, fmt.Errorf("load assisted fill summary: %w", err)
	}
	if documents != "" {
		summary.Documents = strings.Split(documents, ",")
	}
	if labels != "" {
		summary.FilledLabels = strings.Split(labels, "\n")
	}
	if parsed, ok := parseAssistedTime(recordedAt.String); ok {
		summary.RecordedAt = parsed.Format(time.RFC3339)
	}
	// A NULL here stays empty, and empty means "no fill attempt is recorded",
	// which for a row written before this column existed is the truth: nobody
	// knows. It must never be rendered as "a fill ran and achieved nothing".
	if parsed, ok := parseAssistedTime(fillAttemptedAt.String); ok {
		summary.FillAttemptedAt = parsed.Format(time.RFC3339)
	}
	return summary, nil
}

// TakePendingAnswers returns the operator's answers for one job and deletes
// them in the same transaction.
//
// Read-and-delete rather than read-then-delete is the whole design: an answer
// that has been handed to the assisted browser must not still be sitting in the
// database, and a crash between two separate statements is exactly how it would
// end up there.
func TakePendingAnswers(conn *sql.DB, jobID string) (map[string]string, error) {
	values := map[string]string{}
	if conn == nil {
		return values, errors.New("database not initialized")
	}
	tx, err := conn.Begin()
	if err != nil {
		return values, err
	}
	defer tx.Rollback()
	rows, err := tx.Query(`SELECT question_key, answer_text FROM pending_answers WHERE job_id = ?`, jobID)
	if err != nil {
		return values, fmt.Errorf("read pending answers: %w", err)
	}
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			rows.Close()
			return values, err
		}
		values[key] = value
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return values, err
	}
	rows.Close()
	if _, err := tx.Exec(`DELETE FROM pending_answers WHERE job_id = ?`, jobID); err != nil {
		return values, fmt.Errorf("clear pending answers: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return map[string]string{}, err
	}
	return values, nil
}

// DiscardPendingAnswers removes any answers still waiting for a job. Called
// when a browser closes or a lease is lost, so an answer never outlives the
// application it was typed for.
func DiscardPendingAnswers(conn *sql.DB, jobID string) {
	if conn == nil {
		return
	}
	_, _ = conn.Exec(`DELETE FROM pending_answers WHERE job_id = ?`, jobID)
}

// MarkQuestionsAnswered flips the named questions to answered. It records only
// that they were dealt with; the values travel separately through
// pending_answers and are deleted as soon as the browser has them.
func MarkQuestionsAnswered(conn *sql.DB, jobID string, keys []string) error {
	if conn == nil {
		return errors.New("database not initialized")
	}
	if len(keys) == 0 {
		return nil
	}
	tx, err := conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := time.Now().UTC()
	for _, key := range keys {
		if _, err := tx.Exec(`UPDATE application_questions SET status = ?, answered_at = ?
			WHERE job_id = ? AND question_key = ? AND status = ?`, QuestionAnswered, now, jobID, key, QuestionPending); err != nil {
			return fmt.Errorf("mark question answered: %w", err)
		}
	}
	return tx.Commit()
}

// PendingQuestionCounts returns the number of unanswered questions per job, for
// the queue projection. One query rather than one per card: the assisted queue
// endpoint is polled every two seconds.
func PendingQuestionCounts(conn *sql.DB) (map[string]int, error) {
	counts := map[string]int{}
	if conn == nil {
		return counts, nil
	}
	rows, err := conn.Query(`SELECT CAST(job_id AS TEXT), COUNT(*) FROM application_questions WHERE status = ? GROUP BY job_id`, QuestionPending)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "no such table") {
			return counts, nil
		}
		return counts, fmt.Errorf("count pending questions: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var jobID string
		var count int
		if err := rows.Scan(&jobID, &count); err != nil {
			return counts, err
		}
		counts[jobID] = count
	}
	return counts, rows.Err()
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
