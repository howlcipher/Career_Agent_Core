package storage

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
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
	CreatedAt   string `json:"created_at"`
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
	RecordedAt      string   `json:"recorded_at"`
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
		created_at DATETIME NOT NULL,
		answered_at DATETIME,
		UNIQUE(job_id, question_key)
	);
	CREATE TABLE IF NOT EXISTS assisted_fill_summary (
		job_id INTEGER PRIMARY KEY,
		filled_count INTEGER NOT NULL DEFAULT 0,
		reused_answers INTEGER NOT NULL DEFAULT 0,
		documents TEXT NOT NULL DEFAULT '',
		filled_labels TEXT NOT NULL DEFAULT '',
		unresolved_count INTEGER NOT NULL DEFAULT 0,
		recorded_at DATETIME NOT NULL
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
	CREATE INDEX IF NOT EXISTS idx_application_questions_job ON application_questions(job_id, status);`)
	if err != nil {
		return fmt.Errorf("create question schema: %w", err)
	}
	return nil
}

// ReplaceApplicationQuestions records the outcome of one refill: the questions
// that still need the operator, and the summary of what was completed.
//
// Replacing rather than appending is deliberate. A second refill of the same
// application observes the form as it is *now*; a question the operator has
// since answered is no longer pending, and carrying stale rows forward would
// show them work they have already done. Rows the operator has already answered
// are preserved so the count of what they did survives the refresh.
func ReplaceApplicationQuestions(conn *sql.DB, jobID string, questions []ApplicationQuestion, summary AssistedFillSummary) error {
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
		if _, err := tx.Exec(`INSERT INTO application_questions
			(job_id, question_key, prompt_text, control_type, options_json, required, status, sensitivity, proposed_answer, answer_source, label_unsafe, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(job_id, question_key) DO UPDATE SET
				prompt_text = excluded.prompt_text,
				control_type = excluded.control_type,
				options_json = excluded.options_json,
				required = excluded.required,
				sensitivity = excluded.sensitivity,
				proposed_answer = excluded.proposed_answer,
				answer_source = excluded.answer_source,
				label_unsafe = excluded.label_unsafe`,
			jobID, question.Key, question.Prompt, question.ControlType, options,
			boolToInt(question.Required), QuestionPending, question.Sensitivity,
			question.Suggested, question.Source, boolToInt(question.LabelUnsafe), now); err != nil {
			return fmt.Errorf("record application question: %w", err)
		}
	}

	documents := strings.Join(summary.Documents, ",")
	labels := strings.Join(summary.FilledLabels, "\n")
	if _, err := tx.Exec(`INSERT INTO assisted_fill_summary
		(job_id, filled_count, reused_answers, documents, filled_labels, unresolved_count, recorded_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(job_id) DO UPDATE SET
			filled_count = excluded.filled_count,
			reused_answers = excluded.reused_answers,
			documents = excluded.documents,
			filled_labels = excluded.filled_labels,
			unresolved_count = excluded.unresolved_count,
			recorded_at = excluded.recorded_at`,
		jobID, summary.FilledCount, summary.ReusedAnswers, documents, labels, len(questions), now); err != nil {
		return fmt.Errorf("record assisted fill summary: %w", err)
	}
	return tx.Commit()
}

// GetPendingQuestions returns what still needs the operator for one job.
func GetPendingQuestions(conn *sql.DB, jobID string) ([]ApplicationQuestion, error) {
	if conn == nil {
		return nil, errors.New("database not initialized")
	}
	rows, err := conn.Query(`SELECT id, job_id, question_key, prompt_text, control_type, options_json,
		required, status, sensitivity, proposed_answer, answer_source, label_unsafe, created_at
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
		var required, labelUnsafe int
		var createdAt sql.NullString
		if err := rows.Scan(&question.ID, &question.JobID, &question.Key, &question.Prompt,
			&question.ControlType, &options, &required, &question.Status, &question.Sensitivity,
			&question.Suggested, &question.Source, &labelUnsafe, &createdAt); err != nil {
			return nil, err
		}
		question.Required = required != 0
		question.LabelUnsafe = labelUnsafe != 0
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

// GetFillSummary returns what Career Agent completed for one job.
func GetFillSummary(conn *sql.DB, jobID string) (AssistedFillSummary, error) {
	summary := AssistedFillSummary{JobID: jobID}
	if conn == nil {
		return summary, errors.New("database not initialized")
	}
	var documents, labels string
	var recordedAt sql.NullString
	err := conn.QueryRow(`SELECT filled_count, reused_answers, documents, filled_labels, unresolved_count, recorded_at
		FROM assisted_fill_summary WHERE job_id = ?`, jobID).
		Scan(&summary.FilledCount, &summary.ReusedAnswers, &documents, &labels, &summary.UnresolvedCount, &recordedAt)
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
