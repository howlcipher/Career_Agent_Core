package storage

import (
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

// Human-interaction kinds. These are the three places an operator's own time
// goes in an assisted application, and they are measured separately because
// they have different fixes: answering is reduced by the Answer Vault,
// reviewing by better prefill, confirming by auto-advance.
const (
	InteractionAnswering    = "answering"
	InteractionReviewSubmit = "review_submit"
	InteractionConfirm      = "confirm"
)

// HumanEffortMetrics is the local measurement of whether this feature is
// actually saving the operator time.
//
// Every number is derived from this machine's own SQLite database. Nothing is
// sent anywhere, and nothing is recorded about *what* the operator answered —
// only that an interaction of a given kind happened and how long it took.
type HumanEffortMetrics struct {
	ApplicationsConfirmed  int     `json:"applications_confirmed"`
	MedianHumanSeconds     int     `json:"median_human_seconds"`
	MedianAnsweringSeconds int     `json:"median_answering_seconds"`
	FieldsAutoFilled       int     `json:"fields_auto_filled"`
	FieldsNeedingHuman     int     `json:"fields_needing_human"`
	ApprovedAnswersReused  int     `json:"approved_answers_reused"`
	AutoFillRatePct        string  `json:"auto_fill_rate_pct"`
	MeanUnresolved         float64 `json:"mean_unresolved_per_application"`
	SessionsCompleted      int     `json:"sessions_completed"`
	SessionsAbandoned      int     `json:"sessions_abandoned"`
	AppsPerSession         float64 `json:"applications_per_session"`
	BrowserHandoffs        int     `json:"browser_handoffs"`
	MappingCacheHitPct     string  `json:"mapping_cache_hit_pct"`
}

// EnsureHumanInteractionSchema creates the interaction ledger.
func EnsureHumanInteractionSchema(conn *sql.DB) error {
	if conn == nil {
		return errors.New("human interaction schema connection is nil")
	}
	_, err := conn.Exec(`CREATE TABLE IF NOT EXISTS human_interactions (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		job_id INTEGER NOT NULL,
		session_id INTEGER,
		kind TEXT NOT NULL,
		started_at DATETIME NOT NULL,
		ended_at DATETIME NOT NULL,
		duration_ms INTEGER NOT NULL
	);
	CREATE INDEX IF NOT EXISTS idx_human_interactions_kind ON human_interactions(kind, ended_at);`)
	if err != nil {
		return fmt.Errorf("create human interaction schema: %w", err)
	}
	return nil
}

// RecordHumanInteraction writes one measured stretch of operator time.
//
// A negative or absurd duration is dropped rather than stored: the start time
// comes from a browser tab that may have been left open overnight, and a
// fourteen-hour "interaction" would poison a median that exists to tell the
// operator whether this is getting faster.
func RecordHumanInteraction(conn *sql.DB, jobID, kind string, startedAt, endedAt time.Time) error {
	if conn == nil {
		return errors.New("database not initialized")
	}
	switch kind {
	case InteractionAnswering, InteractionReviewSubmit, InteractionConfirm:
	default:
		return fmt.Errorf("unsupported human interaction kind %q", kind)
	}
	duration := endedAt.Sub(startedAt)
	const maxCredibleInteraction = 30 * time.Minute
	if duration <= 0 || duration > maxCredibleInteraction {
		return nil
	}
	_, err := conn.Exec(`INSERT INTO human_interactions (job_id, session_id, kind, started_at, ended_at, duration_ms)
		VALUES (?, (SELECT id FROM application_sessions WHERE state != ? ORDER BY id DESC LIMIT 1), ?, ?, ?, ?)`,
		jobID, SessionFinished, kind, startedAt.UTC(), endedAt.UTC(), duration.Milliseconds())
	if err != nil {
		return fmt.Errorf("record human interaction: %w", err)
	}
	return nil
}

// GetHumanEffortMetrics computes the local effort picture.
//
// Missing tables and empty tables both produce a zero-valued report rather than
// an error: these numbers are a panel on a dashboard, and a database that has
// simply not recorded any yet is the normal state on day one, not a failure.
func GetHumanEffortMetrics(conn *sql.DB) (HumanEffortMetrics, error) {
	metrics := HumanEffortMetrics{}
	if conn == nil {
		return metrics, nil
	}
	_ = conn.QueryRow(`SELECT COUNT(*) FROM assisted_applications WHERE confirmation_provenance = 'manual_user_confirmation'`).Scan(&metrics.ApplicationsConfirmed)

	metrics.MedianHumanSeconds = medianInteractionSeconds(conn, "")
	metrics.MedianAnsweringSeconds = medianInteractionSeconds(conn, InteractionAnswering)

	_ = conn.QueryRow(`SELECT COALESCE(SUM(filled_count), 0), COALESCE(SUM(reused_answers), 0) FROM assisted_fill_summary`).
		Scan(&metrics.FieldsAutoFilled, &metrics.ApprovedAnswersReused)
	_ = conn.QueryRow(`SELECT COUNT(*) FROM application_questions WHERE status = ?`, QuestionAnswered).Scan(&metrics.FieldsNeedingHuman)

	total := metrics.FieldsAutoFilled + metrics.FieldsNeedingHuman
	if total > 0 {
		metrics.AutoFillRatePct = fmt.Sprintf("%.0f%%", float64(metrics.FieldsAutoFilled)*100/float64(total))
	} else {
		metrics.AutoFillRatePct = "—"
	}

	var summaries, unresolved int
	_ = conn.QueryRow(`SELECT COUNT(*), COALESCE(SUM(unresolved_count), 0) FROM assisted_fill_summary`).Scan(&summaries, &unresolved)
	if summaries > 0 {
		metrics.MeanUnresolved = float64(unresolved) / float64(summaries)
	}

	_ = conn.QueryRow(`SELECT COUNT(*) FROM application_sessions WHERE state = ?`, SessionFinished).Scan(&metrics.SessionsCompleted)
	_ = conn.QueryRow(`SELECT COUNT(*) FROM application_session_items WHERE state = ?`, ItemStopped).Scan(&metrics.SessionsAbandoned)
	var sessionItems int
	_ = conn.QueryRow(`SELECT COUNT(*) FROM application_session_items WHERE state = ?`, ItemConfirmed).Scan(&sessionItems)
	if metrics.SessionsCompleted > 0 {
		metrics.AppsPerSession = float64(sessionItems) / float64(metrics.SessionsCompleted)
	}

	// A browser handoff is an application this project deliberately sends to
	// the operator's own browser rather than the assisted one. Which rows those
	// are is a URL-pattern decision (AssistedBrowserRejectionReason), not
	// something SQL can express, so it is counted in Go.
	metrics.BrowserHandoffs, _ = CountBrowserHandoffs(conn)

	var healthy, mappings int
	_ = conn.QueryRow(`SELECT COUNT(*), COALESCE(SUM(CASE WHEN success_count >= failure_count THEN 1 ELSE 0 END), 0) FROM form_mappings`).Scan(&mappings, &healthy)
	if mappings > 0 {
		metrics.MappingCacheHitPct = fmt.Sprintf("%.0f%%", float64(healthy)*100/float64(mappings))
	} else {
		metrics.MappingCacheHitPct = "—"
	}
	return metrics, nil
}

// medianInteractionSeconds reads the median rather than the mean deliberately:
// one application that went badly wrong should not move the number the
// operator uses to judge whether this is working.
func medianInteractionSeconds(conn *sql.DB, kind string) int {
	query := `SELECT duration_ms FROM human_interactions`
	var args []any
	if kind != "" {
		query += ` WHERE kind = ?`
		args = append(args, kind)
	}
	rows, err := conn.Query(query, args...)
	if err != nil {
		if !strings.Contains(strings.ToLower(err.Error()), "no such table") {
			return 0
		}
		return 0
	}
	defer rows.Close()
	var durations []int
	for rows.Next() {
		var ms int
		if err := rows.Scan(&ms); err != nil {
			return 0
		}
		durations = append(durations, ms)
	}
	if len(durations) == 0 {
		return 0
	}
	sort.Ints(durations)
	middle := durations[len(durations)/2]
	return middle / 1000
}

// CountBrowserHandoffs reports how many queued applications this project has
// deliberately routed to the operator's own browser, which is a real cost the
// effort metrics should show rather than hide.
func CountBrowserHandoffs(conn *sql.DB) (int, error) {
	if conn == nil {
		return 0, nil
	}
	rows, err := conn.Query(`SELECT COALESCE(jf.url, '') FROM assisted_applications aa
		JOIN job_funnel jf ON jf.id = aa.job_id WHERE aa.assisted_state != 'completed'`)
	if err != nil {
		return 0, nil
	}
	defer rows.Close()
	handoffs := 0
	for rows.Next() {
		var postingURL string
		if err := rows.Scan(&postingURL); err != nil {
			return handoffs, err
		}
		if AssistedBrowserRejectionReason(postingURL) != "" {
			handoffs++
		}
	}
	return handoffs, rows.Err()
}
