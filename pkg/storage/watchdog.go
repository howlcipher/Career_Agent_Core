package storage

import (
	"database/sql"
	"fmt"
	"time"
)

// DaemonWatchdogSnapshot is deliberately aggregate-only. It provides enough
// evidence for a daemon health alert without copying job content into logs.
type DaemonWatchdogSnapshot struct {
	EligibleQueue int
	Confirmed     int
	Terminal      map[string]int
}

func GetDaemonWatchdogSnapshot(now time.Time) (DaemonWatchdogSnapshot, error) {
	if db == nil {
		return DaemonWatchdogSnapshot{}, fmt.Errorf("database is not initialized")
	}
	snapshot := DaemonWatchdogSnapshot{Terminal: make(map[string]int)}
	if err := db.QueryRow(`SELECT COUNT(*) FROM job_funnel
		WHERE status = 'DISCOVERED' AND url NOT LIKE '%breezy.hr%'
		AND (next_eligible_at IS NULL OR next_eligible_at <= ?)`, now.UTC()).Scan(&snapshot.EligibleQueue); err != nil {
		return snapshot, fmt.Errorf("count eligible watchdog queue: %w", err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM applied_jobs`).Scan(&snapshot.Confirmed); err != nil {
		return snapshot, fmt.Errorf("count confirmed applications: %w", err)
	}
	rows, err := db.Query(`SELECT status, COUNT(*) FROM job_funnel
		WHERE last_updated >= ? AND status IN (
			'QUARANTINED_PROMPT_INJECTION', 'FAILED_SCORE', 'FAILED_SUBMIT',
			'BLOCKED_CAPTCHA', 'INVALID_URL', 'RETRY_EXHAUSTED', 'SKIPPED',
			'MANUAL_REQUIRED', 'AWAITING_REVIEW') GROUP BY status`, now.UTC().Add(-24*time.Hour))
	if err != nil {
		return snapshot, fmt.Errorf("query recent terminal outcomes: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			return snapshot, fmt.Errorf("scan terminal outcome: %w", err)
		}
		snapshot.Terminal[status] = count
	}
	if err := rows.Err(); err != nil {
		return snapshot, fmt.Errorf("iterate terminal outcomes: %w", err)
	}
	return snapshot, nil
}

func SetDaemonWatchdogAlert(message string) error {
	if db == nil {
		return fmt.Errorf("database is not initialized")
	}
	_, err := db.Exec(`INSERT INTO daemon_watchdog_alert (id, message, updated_at) VALUES (1, ?, ?)
		ON CONFLICT(id) DO UPDATE SET message = excluded.message, updated_at = excluded.updated_at`, message, time.Now().UTC())
	return err
}

func GetDaemonWatchdogAlert() (message string, updatedAt time.Time, err error) {
	if db == nil {
		return "", time.Time{}, fmt.Errorf("database is not initialized")
	}
	err = db.QueryRow(`SELECT message, updated_at FROM daemon_watchdog_alert WHERE id = 1`).Scan(&message, &updatedAt)
	if err == sql.ErrNoRows {
		return "", time.Time{}, nil
	}
	return message, updatedAt, err
}
