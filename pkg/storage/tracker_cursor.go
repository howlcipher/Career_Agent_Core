package storage

import (
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// TrackerCursor is the tracker's durable IMAP progress checkpoint (bug #534).
//
// The pre-fix tracker had no durable position at all: every scan re-derived
// its fetch window from the newest ~50 mailbox sequence numbers, so any
// outage longer than that window put the mail that arrived during it
// permanently out of reach. Sequence numbers are also mailbox-relative and
// shift when messages are removed, so they cannot serve as a checkpoint even
// if the window were widened — go-imap's UID is the stable identifier for a
// given mailbox generation (UIDVALIDITY).
//
// Two independent ranges are tracked so a large historical backlog cannot
// starve newly arriving mail:
//   - ForwardUID is the high-water mark for ordinary steady-state traffic.
//     Every scan fetches UIDs above it; nothing below it is ever revisited by
//     the forward path.
//   - CatchupFloorUID..CatchupCeilingUID is the bounded historical window
//     established once, at bootstrap (or after a UIDVALIDITY change).
//     CatchupNextUID walks upward through it in bounded batches, independent
//     of ForwardUID, until it reaches the ceiling and CatchupComplete is set.
type TrackerCursor struct {
	UidValidity       uint32
	ForwardUID        uint32
	CatchupFloorUID   uint32
	CatchupCeilingUID uint32
	CatchupNextUID    uint32
	CatchupComplete   bool
}

// GetTrackerCursor returns the tracker's saved checkpoint, or nil if none has
// ever been written — the pre-#534 state, and the state after a first-ever
// scan of a brand-new database.
func GetTrackerCursor() (*TrackerCursor, error) {
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	var c TrackerCursor
	var complete int
	err := db.QueryRow(
		`SELECT uid_validity, forward_uid, catchup_floor_uid, catchup_ceiling_uid, catchup_next_uid, catchup_complete
		 FROM tracker_cursor WHERE id = 1`,
	).Scan(&c.UidValidity, &c.ForwardUID, &c.CatchupFloorUID, &c.CatchupCeilingUID, &c.CatchupNextUID, &complete)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read tracker cursor: %w", err)
	}
	c.CatchupComplete = complete != 0
	return &c, nil
}

// BootstrapTrackerCursor (re)installs the checkpoint from scratch: on the
// very first scan of a database, and again whenever the mailbox's
// UIDVALIDITY has changed underneath the tracker (the IMAP-mandated signal
// that old UIDs no longer mean anything for this mailbox). It replaces
// whatever row exists — a UIDVALIDITY change makes the old ForwardUID/
// CatchupNextUID values meaningless, not merely stale, so there is nothing
// to preserve. Replaying mail already recorded in processed_emails is safe:
// that table's Message-ID dedup (not this cursor) is what protects against
// duplicate outcome writes.
func BootstrapTrackerCursor(c TrackerCursor) error {
	if db == nil {
		return fmt.Errorf("database not initialized")
	}
	complete := 0
	if c.CatchupComplete {
		complete = 1
	}
	_, err := db.Exec(
		`INSERT INTO tracker_cursor (id, uid_validity, forward_uid, catchup_floor_uid, catchup_ceiling_uid, catchup_next_uid, catchup_complete, updated_at)
		 VALUES (1, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET
		   uid_validity = excluded.uid_validity,
		   forward_uid = excluded.forward_uid,
		   catchup_floor_uid = excluded.catchup_floor_uid,
		   catchup_ceiling_uid = excluded.catchup_ceiling_uid,
		   catchup_next_uid = excluded.catchup_next_uid,
		   catchup_complete = excluded.catchup_complete,
		   updated_at = excluded.updated_at`,
		c.UidValidity, c.ForwardUID, c.CatchupFloorUID, c.CatchupCeilingUID, c.CatchupNextUID, complete, time.Now().UTC(),
	)
	if err != nil {
		return fmt.Errorf("bootstrap tracker cursor: %w", err)
	}
	return nil
}

// AdvanceForwardUID moves the steady-state checkpoint forward to uid. It is a
// no-op if uid is not strictly greater than the current value, so a stray
// out-of-order call can never move the checkpoint backwards.
//
// This is called once per scan, after every message the forward fetch
// returned has been durably handled (acknowledged, and if outcome-shaped,
// recorded) up to and including uid — never past a message whose handling
// failed. A crash before this call leaves the checkpoint at its old value;
// the next scan re-fetches the same range and processed_emails makes
// re-acknowledging already-handled messages a no-op, so nothing is skipped.
func AdvanceForwardUID(uid uint32) error {
	if db == nil {
		return fmt.Errorf("database not initialized")
	}
	_, err := db.Exec(
		`UPDATE tracker_cursor SET forward_uid = ?, updated_at = ? WHERE id = 1 AND ? > forward_uid`,
		uid, time.Now().UTC(), uid,
	)
	if err != nil {
		return fmt.Errorf("advance forward cursor: %w", err)
	}
	return nil
}

// AdvanceCatchupUID moves the historical-catchup checkpoint forward to
// nextUID (the first UID not yet handled) and marks catch-up complete once
// nextUID reaches the ceiling established at bootstrap. Same monotonic and
// crash-safety guarantees as AdvanceForwardUID.
func AdvanceCatchupUID(nextUID uint32) error {
	if db == nil {
		return fmt.Errorf("database not initialized")
	}
	_, err := db.Exec(
		`UPDATE tracker_cursor
		 SET catchup_next_uid = ?,
		     catchup_complete = CASE WHEN ? >= catchup_ceiling_uid THEN 1 ELSE catchup_complete END,
		     updated_at = ?
		 WHERE id = 1 AND ? > catchup_next_uid`,
		nextUID, nextUID, time.Now().UTC(), nextUID,
	)
	if err != nil {
		return fmt.Errorf("advance catchup cursor: %w", err)
	}
	return nil
}

// EarliestTrackableApplicationTime returns the earliest discovery/submission
// time among job_funnel rows an inbound outcome email could still legally
// update — the same status set GetTrackedCompanies and the tracker's
// candidate query use (APPLIED, MANUAL_REQUIRED, AWAITING_REVIEW). It is the
// evidence bug #534's historical catch-up floor is derived from: an outcome
// for an application cannot meaningfully predate the earliest still-open
// application, so mail older than this has no application left to match. The
// second return value is false when there is no open application at all, in
// which case there is nothing for a historical catch-up to recover.
func EarliestTrackableApplicationTime() (time.Time, bool, error) {
	if db == nil {
		return time.Time{}, false, fmt.Errorf("database not initialized")
	}
	var earliest any
	err := db.QueryRow(
		`SELECT MIN(COALESCE(applied_at, discovered_at)) FROM job_funnel
		 WHERE status IN ('APPLIED', 'MANUAL_REQUIRED', 'AWAITING_REVIEW')`,
	).Scan(&earliest)
	if err != nil {
		return time.Time{}, false, fmt.Errorf("query earliest trackable application: %w", err)
	}
	if earliest == nil {
		return time.Time{}, false, nil
	}
	switch v := earliest.(type) {
	case time.Time:
		return v, true, nil
	case string:
		for _, layout := range assistedTimeLayouts {
			if t, err := time.Parse(layout, v); err == nil {
				return t, true, nil
			}
		}
	}
	return time.Time{}, false, fmt.Errorf("earliest trackable application time %v is not a parseable timestamp", earliest)
}
