package storage

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// DiscoverySourceCount holds the privacy-safe aggregate counts for a single
// discovery source within one refresh run. No URLs, job names, company names,
// or error messages are stored here — only classified numeric outcomes.
type DiscoverySourceCount struct {
	// Source is one of: "serpapi", "yahoo", "remoteok", "hackernews",
	// "jobicy", "atsfeeds". Normalized to lowercase on write.
	Source             string `json:"source"`
	Attempted          int    `json:"attempted"`
	New                int    `json:"new"`
	Duplicate          int    `json:"duplicate"`
	Excluded           int    `json:"excluded"`
	Error              int    `json:"error"`
	RequestAttempted   int    `json:"request_attempted"`
	RequestFailed      int    `json:"request_failed"`
	CircuitOpenSkipped int    `json:"circuit_open_skipped"`
}

// DiscoveryRefresh is the latest aggregate-only discovery result. It must
// never contain a posting URL, job/company name, or raw provider error.
type DiscoveryRefresh struct {
	StartedAt    time.Time
	FinishedAt   time.Time
	NewEligible  int
	ErrorClass   string
	SourceCounts []DiscoverySourceCount
}

// SetDiscoveryRefresh atomically replaces the latest refresh diagnostic.
func SetDiscoveryRefresh(refresh DiscoveryRefresh) error {
	if db == nil {
		return fmt.Errorf("database is not initialized")
	}
	if refresh.StartedAt.IsZero() || refresh.FinishedAt.IsZero() {
		return fmt.Errorf("discovery refresh timestamps are required")
	}
	if refresh.NewEligible < 0 {
		return fmt.Errorf("new eligible count cannot be negative")
	}
	refresh.ErrorClass = sanitizeDiscoveryErrorClass(refresh.ErrorClass)

	// Normalize source names before persisting.
	for i := range refresh.SourceCounts {
		refresh.SourceCounts[i].Source = strings.ToLower(strings.TrimSpace(refresh.SourceCounts[i].Source))
	}

	sourceCountsJSON, err := json.Marshal(refresh.SourceCounts)
	if err != nil {
		return fmt.Errorf("marshal discovery source counts: %w", err)
	}

	_, err = db.Exec(`INSERT INTO discovery_refresh
		(id, started_at, finished_at, new_eligible, error_class, source_counts_json) VALUES (1, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET started_at=excluded.started_at, finished_at=excluded.finished_at,
		new_eligible=excluded.new_eligible, error_class=excluded.error_class,
		source_counts_json=excluded.source_counts_json`,
		refresh.StartedAt.UTC(), refresh.FinishedAt.UTC(), refresh.NewEligible, refresh.ErrorClass, string(sourceCountsJSON))
	return err
}

// GetDiscoveryRefresh returns the latest aggregate diagnostic, if any.
func GetDiscoveryRefresh() (DiscoveryRefresh, bool, error) {
	if db == nil {
		return DiscoveryRefresh{}, false, fmt.Errorf("database is not initialized")
	}
	var refresh DiscoveryRefresh
	var sourceCountsJSON sql.NullString
	err := db.QueryRow(`SELECT started_at, finished_at, new_eligible, error_class, source_counts_json FROM discovery_refresh WHERE id=1`).
		Scan(&refresh.StartedAt, &refresh.FinishedAt, &refresh.NewEligible, &refresh.ErrorClass, &sourceCountsJSON)
	if err == sql.ErrNoRows {
		return DiscoveryRefresh{}, false, nil
	}
	if err != nil {
		return DiscoveryRefresh{}, false, err
	}
	if sourceCountsJSON.Valid && sourceCountsJSON.String != "" && sourceCountsJSON.String != "null" {
		if jsonErr := json.Unmarshal([]byte(sourceCountsJSON.String), &refresh.SourceCounts); jsonErr != nil {
			// Non-fatal: return the row without source counts rather than failing.
			refresh.SourceCounts = nil
		}
	}
	return refresh, true, nil
}

// CountEligibleDiscoveryRows returns only a queue aggregate, suitable for
// comparing the state immediately before and after a discovery refresh.
func CountEligibleDiscoveryRows(now time.Time) (int, error) {
	if db == nil {
		return 0, fmt.Errorf("database is not initialized")
	}
	var count int
	err := db.QueryRow(`SELECT COUNT(*) FROM job_funnel
		WHERE status = 'DISCOVERED' AND url NOT LIKE '%breezy.hr%'
		AND (next_eligible_at IS NULL OR next_eligible_at <= ?)`, now.UTC()).Scan(&count)
	return count, err
}

// migrateDiscoveryRefreshSourceCounts adds the source_counts_json column to
// discovery_refresh if it was created before improvement #509. Idempotent.
func migrateDiscoveryRefreshSourceCounts() error {
	rows, err := db.Query("PRAGMA table_info(discovery_refresh)")
	if err != nil {
		return fmt.Errorf("inspect discovery_refresh schema: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull, pk int
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			return fmt.Errorf("scan discovery_refresh column info: %w", err)
		}
		if name == "source_counts_json" {
			return nil
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}

	_, err = db.Exec("ALTER TABLE discovery_refresh ADD COLUMN source_counts_json TEXT")
	return err
}

func sanitizeDiscoveryErrorClass(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	switch value {
	case "", "network", "timeout", "provider", "cancelled", "unknown":
		return value
	default:
		return "unknown"
	}
}
