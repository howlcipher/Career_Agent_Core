package storage

import (
	"database/sql"
	"fmt"
	"time"
)

// UnmatchedOutcome is an outcome-shaped email that classified as a real
// rejection or interview request but matched no tracked application.
//
// It deliberately holds no email content. The sender's domain is the only
// identifying field, and it is the minimum needed to correlate the message
// with an application later; subject, body, address and Message-ID content
// are never stored beyond the opaque ID already kept in processed_emails.
type UnmatchedOutcome struct {
	MessageID    string
	Status       string
	SenderDomain string
	DetectedAt   time.Time
}

// recordUnmatchedOutcome persists an outcome-shaped email that matched
// nothing, using the caller's transaction so the record and the message's
// acknowledgement commit together or not at all.
//
// Bug #533: the tracker acknowledges an unmatched outcome email inside the
// same transaction that gives up on it, and StartTracker skips any
// acknowledged Message-ID forever. That is correct for avoiding the rescan
// churn bug #20 fixed — re-reading the same 50 messages every 15 minutes
// re-logs them and re-runs an LLM call per cycle — but it meant the evidence
// itself was discarded, not merely deferred. A rejection arriving before the
// user confirms the hand-off application it belongs to was unrecoverable.
//
// Keeping the acknowledgement and recording the fact separately gets both:
// the message is never reprocessed, and the outcome remains visible.
func recordUnmatchedOutcome(tx *sql.Tx, messageID, status, senderDomain string) error {
	if messageID == "" {
		return nil
	}
	if _, err := tx.Exec(
		`INSERT INTO unmatched_outcomes (message_id, outcome_status, sender_domain, detected_at)
		 VALUES (?, ?, ?, ?)
		 ON CONFLICT(message_id) DO NOTHING`,
		messageID, status, senderDomain, time.Now().UTC(),
	); err != nil {
		return fmt.Errorf("record unmatched outcome: %w", err)
	}
	return nil
}

// RecordUnmatchedOutcomeTx is the package-external entry point for the
// tracker, which owns the transaction that also writes processed_emails.
func RecordUnmatchedOutcomeTx(tx *sql.Tx, messageID, status, senderDomain string) error {
	return recordUnmatchedOutcome(tx, messageID, status, senderDomain)
}

// UnmatchedOutcomeCounts returns how many unmatched outcome emails have been
// seen per sender domain and status, most recent first. It is the recovery
// surface: an operator seeing a domain here knows a real outcome arrived for
// an application the funnel could not identify.
func UnmatchedOutcomeCounts() ([]UnmatchedOutcome, error) {
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	rows, err := db.Query(
		`SELECT message_id, outcome_status, sender_domain, detected_at
		 FROM unmatched_outcomes ORDER BY detected_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("query unmatched outcomes: %w", err)
	}
	defer rows.Close()

	var out []UnmatchedOutcome
	for rows.Next() {
		var u UnmatchedOutcome
		var detected any
		if err := rows.Scan(&u.MessageID, &u.Status, &u.SenderDomain, &detected); err != nil {
			return nil, fmt.Errorf("scan unmatched outcome: %w", err)
		}
		// ADR-003 decision 6: the driver's stored timestamp shape is not
		// guaranteed, so parse defensively rather than assuming one layout.
		switch v := detected.(type) {
		case time.Time:
			u.DetectedAt = v
		case string:
			for _, layout := range assistedTimeLayouts {
				if t, err := time.Parse(layout, v); err == nil {
					u.DetectedAt = t
					break
				}
			}
		}
		out = append(out, u)
	}
	return out, rows.Err()
}
