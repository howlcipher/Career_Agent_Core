package storage

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// This file is the read side of Application Knowledge: the queries that let the
// dashboard ask "what do the applications in my queue actually need from me?"
// rather than "what does this one application need from me?".
//
// Nothing here groups or resolves anything. Grouping lives in pkg/knowledge,
// deliberately, because it has to take the strictest sensitivity across a
// group, union option sets that employers word differently, and notice when
// two employers offer genuinely incompatible choices -- three decisions that
// SQL's bare-column grouping would make silently and arbitrarily.

// Preflight verdicts. The state says whether Career Agent managed to look at
// the application at all; the reason, when it could not, comes from the closed
// vocabulary in pkg/submitter. "We looked and found no form" and "we were not
// allowed to look" are different facts and are never collapsed.
const (
	PreflightInspected   = "inspected"
	PreflightUnavailable = "unavailable"
)

// QueuedQuestion is one pending question, carried together with enough about
// the application that asked it to resolve it correctly.
//
// Company and ATS travel with the question because the vault's answer depends
// on them: a company-scoped approval beats an ATS-wide one, which beats a
// global one. Grouping without them would let one employer's answer be offered
// for another's question.
type QueuedQuestion struct {
	ApplicationQuestion
	Company string `json:"company"`
	Role    string `json:"role"`
	ATS     string `json:"ats"`
}

// QueuedQuestions returns every pending question across the queue, for
// applications no assisted browser currently holds.
//
// The lease exclusion is the important part. A job with a live browser is being
// worked right now, by a process that owns its question rows and its state
// transitions; re-evaluating underneath it would change what the operator is
// looking at while they look at it. The inventory is advisory and the live
// browser is authoritative, and this WHERE clause is where that rule is
// actually enforced.
func QueuedQuestions(conn *sql.DB, now time.Time) ([]QueuedQuestion, error) {
	if conn == nil {
		return nil, errors.New("database not initialized")
	}
	rows, err := conn.Query(`
		SELECT q.id, q.job_id, q.question_key, q.prompt_text, q.control_type, q.options_json,
		       q.required, q.status, q.sensitivity, q.proposed_answer, q.answer_source,
		       q.label_unsafe, q.canonical_key, q.auto_fillable, q.created_at,
		       COALESCE(f.company_name, ''), COALESCE(f.job_title, ''), COALESCE(f.url, '')
		FROM application_questions q
		LEFT JOIN job_funnel f ON f.id = q.job_id
		LEFT JOIN assisted_applications a ON a.job_id = q.job_id
		WHERE q.status = ?
		  AND NOT (a.lease_owner IS NOT NULL AND a.lease_owner != '' AND a.lease_expires_at > ?)
		ORDER BY q.job_id ASC, q.required DESC, q.id ASC`, QuestionPending, now.UTC())
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "no such table") {
			return []QueuedQuestion{}, nil
		}
		return nil, fmt.Errorf("load queued questions: %w", err)
	}
	defer rows.Close()

	out := []QueuedQuestion{}
	for rows.Next() {
		var question QueuedQuestion
		var options, applyURL string
		var required, labelUnsafe, autoFillable int
		var createdAt sql.NullString
		if err := rows.Scan(&question.ID, &question.JobID, &question.Key, &question.Prompt,
			&question.ControlType, &options, &required, &question.Status, &question.Sensitivity,
			&question.Suggested, &question.Source, &labelUnsafe, &question.CanonicalKey,
			&autoFillable, &createdAt, &question.Company, &question.Role, &applyURL); err != nil {
			return nil, fmt.Errorf("scan queued question: %w", err)
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
		// The posting URL is read only to name the ATS and is never carried any
		// further: assisted.go's invariant is that a posting URL leaves the
		// server on exactly one path, and this is not it.
		question.ATS = SupportedAssistedATS(applyURL)
		out = append(out, question)
	}
	return out, rows.Err()
}

// SetQuestionResolution records what the vault concluded about one question.
//
// It writes only the advisory columns -- never `status`, never `answered_at`.
// A re-evaluation is Career Agent learning something, not the operator doing
// something, and only the operator's own act closes a question.
func SetQuestionResolution(conn *sql.DB, id int64, suggested, source string, autoFillable bool) error {
	if conn == nil {
		return errors.New("database not initialized")
	}
	_, err := conn.Exec(`UPDATE application_questions
		SET proposed_answer = ?, answer_source = ?, auto_fillable = ?
		WHERE id = ? AND status = ?`, suggested, source, boolToInt(autoFillable), id, QuestionPending)
	if err != nil {
		return fmt.Errorf("record question resolution: %w", err)
	}
	return nil
}

// PreflightResult is one application's preflight verdict.
type PreflightResult struct {
	JobID        string `json:"job_id"`
	Company      string `json:"company"`
	Role         string `json:"role"`
	State        string `json:"state"`
	Reason       string `json:"reason,omitempty"`
	ATS          string `json:"ats,omitempty"`
	ControlCount int    `json:"control_count"`
	InspectedAt  string `json:"inspected_at"`
}

// RecordPreflight stores one application's verdict, replacing any earlier one.
func RecordPreflight(conn *sql.DB, result PreflightResult, now time.Time) error {
	if conn == nil {
		return errors.New("database not initialized")
	}
	if strings.TrimSpace(result.JobID) == "" {
		return errors.New("a preflight result needs a job identifier")
	}
	if result.State != PreflightInspected && result.State != PreflightUnavailable {
		return fmt.Errorf("unknown preflight state %q", result.State)
	}
	_, err := conn.Exec(`INSERT INTO application_preflight
		(job_id, state, reason, ats, control_count, inspected_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(job_id) DO UPDATE SET
			state = excluded.state,
			reason = excluded.reason,
			ats = excluded.ats,
			control_count = excluded.control_count,
			inspected_at = excluded.inspected_at`,
		result.JobID, result.State, result.Reason, result.ATS, result.ControlCount, now.UTC())
	if err != nil {
		return fmt.Errorf("record preflight result: %w", err)
	}
	return nil
}

// PreflightResults returns the stored verdicts for the given applications, or
// for all of them when jobIDs is empty.
func PreflightResults(conn *sql.DB, jobIDs []string) ([]PreflightResult, error) {
	if conn == nil {
		return nil, errors.New("database not initialized")
	}
	query := `SELECT CAST(p.job_id AS TEXT), COALESCE(f.company_name, ''), COALESCE(f.job_title, ''),
		p.state, p.reason, p.ats, p.control_count, p.inspected_at
		FROM application_preflight p
		LEFT JOIN job_funnel f ON f.id = p.job_id`
	args := []any{}
	if len(jobIDs) > 0 {
		query += ` WHERE p.job_id IN (?` + strings.Repeat(`, ?`, len(jobIDs)-1) + `)`
		for _, id := range jobIDs {
			args = append(args, id)
		}
	}
	query += ` ORDER BY p.inspected_at DESC, p.job_id ASC`

	rows, err := conn.Query(query, args...)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "no such table") {
			return []PreflightResult{}, nil
		}
		return nil, fmt.Errorf("load preflight results: %w", err)
	}
	defer rows.Close()

	out := []PreflightResult{}
	for rows.Next() {
		var result PreflightResult
		var inspectedAt sql.NullString
		if err := rows.Scan(&result.JobID, &result.Company, &result.Role, &result.State,
			&result.Reason, &result.ATS, &result.ControlCount, &inspectedAt); err != nil {
			return nil, fmt.Errorf("scan preflight result: %w", err)
		}
		if parsed, ok := parseAssistedTime(inspectedAt.String); ok {
			result.InspectedAt = parsed.Format(time.RFC3339)
		}
		out = append(out, result)
	}
	return out, rows.Err()
}

// PreflightCandidates lists the queued applications a preflight run may
// inspect, in queue order.
//
// It refuses the same things GetAssistedLaunchInfo refuses, for the same
// reasons, plus one more: an ATS that rejects the assisted browser is not
// inspected either. Career Agent cannot see that application without an
// operator signed in, and reporting "preflight unavailable, needs
// authentication" is the honest answer rather than something to work around.
func PreflightCandidates(conn *sql.DB, jobIDs []string) ([]PreflightCandidate, error) {
	if conn == nil {
		return nil, errors.New("database not initialized")
	}
	if len(jobIDs) == 0 {
		return []PreflightCandidate{}, nil
	}
	query := `SELECT CAST(f.id AS TEXT), COALESCE(f.company_name, ''), COALESCE(f.job_title, ''),
		COALESCE(f.url, ''), COALESCE(a.assisted_state, '')
		FROM job_funnel f
		LEFT JOIN assisted_applications a ON a.job_id = f.id
		WHERE f.id IN (?` + strings.Repeat(`, ?`, len(jobIDs)-1) + `)`
	args := make([]any, 0, len(jobIDs))
	for _, id := range jobIDs {
		args = append(args, id)
	}
	rows, err := conn.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("load preflight candidates: %w", err)
	}
	defer rows.Close()

	out := []PreflightCandidate{}
	for rows.Next() {
		var candidate PreflightCandidate
		var assistedState string
		if err := rows.Scan(&candidate.JobID, &candidate.Company, &candidate.Role,
			&candidate.URL, &assistedState); err != nil {
			return nil, fmt.Errorf("scan preflight candidate: %w", err)
		}
		candidate.ATS = SupportedAssistedATS(candidate.URL)
		candidate.Skip = AssistedBrowserRejectionReason(candidate.URL)
		if assistedState == "completed" {
			candidate.Skip = "already completed"
		}
		out = append(out, candidate)
	}
	return out, rows.Err()
}

// PreflightCandidate is one application a preflight run was asked to inspect.
// Skip is non-empty when it must not be, and says why in words meant for the
// operator.
type PreflightCandidate struct {
	JobID   string
	Company string
	Role    string
	URL     string
	ATS     string
	Skip    string
}
