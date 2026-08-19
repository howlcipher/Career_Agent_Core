package storage

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Apply-session states.
const (
	SessionRunning  = "running"
	SessionPaused   = "paused"
	SessionFinished = "finished"
)

// Apply-session item states. Everything after ItemInProgress is terminal.
//
// The list is closed on purpose: AdvanceApplySession will only move past an
// item whose state is one of these terminal values, so a new way for an
// application to end has to be named here before the session can treat it as
// an ending. That is the mechanism behind this feature's central rule — never
// advance past an application unless Career Agent knows what happened to it.
const (
	ItemPending    = "pending"
	ItemInProgress = "in_progress"
	ItemConfirmed  = "confirmed"
	ItemSkipped    = "skipped"
	ItemNotFound   = "not_found"
	ItemBlocked    = "blocked_deferred"
	ItemStopped    = "stopped"
)

var terminalItemStates = map[string]bool{
	ItemConfirmed: true, ItemSkipped: true, ItemNotFound: true,
	ItemBlocked: true, ItemStopped: true,
}

// PauseReasonBrowserClosed is recorded when an assisted browser closes with no
// outcome. It is not a terminal item state: a closed browser says nothing
// about whether the employer received anything, so the session stops and asks
// rather than counting it either way.
const PauseReasonBrowserClosed = "browser_closed_without_outcome"

// ApplySessionItem is one application inside a session.
type ApplySessionItem struct {
	ID             int64  `json:"id"`
	Position       int    `json:"position"`
	JobID          string `json:"job_id"`
	Company        string `json:"company"`
	Role           string `json:"role"`
	State          string `json:"state"`
	TerminalReason string `json:"terminal_reason,omitempty"`
}

// ApplySession is a run through a selected set of applications.
type ApplySession struct {
	ID          int64              `json:"id"`
	State       string             `json:"state"`
	AutoAdvance bool               `json:"auto_advance"`
	StopAfter   bool               `json:"stop_after_current"`
	PauseReason string             `json:"pause_reason,omitempty"`
	CurrentJob  string             `json:"current_job_id,omitempty"`
	Position    int                `json:"position"`
	Total       int                `json:"total"`
	Completed   int                `json:"completed"`
	Confirmed   int                `json:"confirmed"`
	Items       []ApplySessionItem `json:"items"`
}

// EnsureApplySessionSchema creates the session tables.
func EnsureApplySessionSchema(conn *sql.DB) error {
	if conn == nil {
		return errors.New("apply session schema connection is nil")
	}
	_, err := conn.Exec(`CREATE TABLE IF NOT EXISTS application_sessions (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		state TEXT NOT NULL DEFAULT 'running',
		auto_advance INTEGER NOT NULL DEFAULT 1,
		stop_after_current INTEGER NOT NULL DEFAULT 0,
		pause_reason TEXT NOT NULL DEFAULT '',
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL
	);
	CREATE TABLE IF NOT EXISTS application_session_items (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		session_id INTEGER NOT NULL,
		position INTEGER NOT NULL,
		job_id INTEGER NOT NULL,
		state TEXT NOT NULL DEFAULT 'pending',
		terminal_reason TEXT NOT NULL DEFAULT '',
		started_at DATETIME,
		ended_at DATETIME,
		UNIQUE(session_id, job_id),
		FOREIGN KEY(session_id) REFERENCES application_sessions(id) ON DELETE CASCADE
	);
	-- At most one session can be open at a time. A partial unique index is the
	-- enforcement rather than a check in Go, because two dashboard tabs racing
	-- Start is exactly the case a Go-side check loses.
	CREATE UNIQUE INDEX IF NOT EXISTS idx_one_open_session
		ON application_sessions(state) WHERE state != 'finished';
	CREATE INDEX IF NOT EXISTS idx_session_items ON application_session_items(session_id, position);`)
	if err != nil {
		return fmt.Errorf("create apply session schema: %w", err)
	}
	return nil
}

// StartApplySession opens a session over the given jobs, in the given order.
//
// It refuses while another session is open. Finishing or stopping the previous
// one is an explicit operator act, because an abandoned session still holds
// items whose outcome nobody recorded.
func StartApplySession(conn *sql.DB, jobIDs []string, autoAdvance bool) (*ApplySession, error) {
	if conn == nil {
		return nil, errors.New("database not initialized")
	}
	if len(jobIDs) == 0 {
		return nil, errors.New("an apply session needs at least one application")
	}
	tx, err := conn.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var open int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM application_sessions WHERE state != ?`, SessionFinished).Scan(&open); err != nil {
		return nil, fmt.Errorf("check for an open apply session: %w", err)
	}
	if open > 0 {
		return nil, errors.New("an apply session is already open; stop it before starting another")
	}
	now := time.Now().UTC()
	result, err := tx.Exec(`INSERT INTO application_sessions (state, auto_advance, stop_after_current, created_at, updated_at)
		VALUES (?, ?, 0, ?, ?)`, SessionRunning, boolToInt(autoAdvance), now, now)
	if err != nil {
		return nil, fmt.Errorf("open apply session: %w", err)
	}
	sessionID, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}
	position := 0
	seen := map[string]bool{}
	for _, jobID := range jobIDs {
		jobID = strings.TrimSpace(jobID)
		if jobID == "" || seen[jobID] {
			continue
		}
		seen[jobID] = true
		if _, err := tx.Exec(`INSERT INTO application_session_items (session_id, position, job_id, state)
			VALUES (?, ?, ?, ?)`, sessionID, position, jobID, ItemPending); err != nil {
			return nil, fmt.Errorf("add apply session item: %w", err)
		}
		position++
	}
	if position == 0 {
		return nil, errors.New("an apply session needs at least one application")
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return GetApplySession(conn)
}

// GetApplySession returns the open session, or nil when none is open.
func GetApplySession(conn *sql.DB) (*ApplySession, error) {
	if conn == nil {
		return nil, errors.New("database not initialized")
	}
	var session ApplySession
	var autoAdvance, stopAfter int
	err := conn.QueryRow(`SELECT id, state, auto_advance, stop_after_current, pause_reason
		FROM application_sessions WHERE state != ? ORDER BY id DESC LIMIT 1`, SessionFinished).
		Scan(&session.ID, &session.State, &autoAdvance, &stopAfter, &session.PauseReason)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "no such table") {
			return nil, nil
		}
		return nil, fmt.Errorf("load apply session: %w", err)
	}
	session.AutoAdvance = autoAdvance != 0
	session.StopAfter = stopAfter != 0

	rows, err := conn.Query(`SELECT i.id, i.position, CAST(i.job_id AS TEXT), i.state, i.terminal_reason,
		COALESCE(jf.company_name, ''), COALESCE(jf.job_title, '')
		FROM application_session_items i LEFT JOIN job_funnel jf ON jf.id = i.job_id
		WHERE i.session_id = ? ORDER BY i.position ASC`, session.ID)
	if err != nil {
		return nil, fmt.Errorf("load apply session items: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var item ApplySessionItem
		if err := rows.Scan(&item.ID, &item.Position, &item.JobID, &item.State, &item.TerminalReason, &item.Company, &item.Role); err != nil {
			return nil, err
		}
		session.Items = append(session.Items, item)
		if terminalItemStates[item.State] {
			session.Completed++
			if item.State == ItemConfirmed {
				session.Confirmed++
			}
		}
		if item.State == ItemInProgress {
			session.CurrentJob = item.JobID
			session.Position = item.Position + 1
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	session.Total = len(session.Items)
	if session.CurrentJob == "" {
		// No item is open. The position the operator is "at" is the first one
		// still waiting, so the header reads 3/12 rather than jumping to 12/12
		// between applications.
		for _, item := range session.Items {
			if item.State == ItemPending {
				session.Position = item.Position + 1
				break
			}
		}
		if session.Position == 0 {
			session.Position = session.Total
		}
	}
	return &session, nil
}

// NextApplySessionJob reports the job the session should open next, and
// whether there is one.
//
// It returns nothing while an item is already in progress, while the session
// is paused, and while stop-after-current is set. Those are three different
// reasons not to advance and they all have the same correct answer: do not
// open another employer's application yet.
func NextApplySessionJob(conn *sql.DB) (string, bool, error) {
	session, err := GetApplySession(conn)
	if err != nil || session == nil {
		return "", false, err
	}
	if session.State != SessionRunning || !session.AutoAdvance || session.StopAfter {
		return "", false, nil
	}
	for _, item := range session.Items {
		if item.State == ItemInProgress {
			return "", false, nil
		}
	}
	// An assisted browser that is still live holds the only visible-browser
	// lease, so a launch now would be refused by AcquireAssistedLease.
	//
	// Checking here rather than letting the launch fail is bugs.md #542's
	// second half: the caller cannot tell a refused lease (transient — the
	// previous browser is closing) from a browser that failed to open (an
	// unknown outcome), and treating the first as the second paused the session
	// every time an application was skipped while its browser was still open.
	live, err := assistedBrowserIsLive(conn)
	if err != nil || live {
		return "", false, err
	}
	for _, item := range session.Items {
		if item.State == ItemPending {
			return item.JobID, true, nil
		}
	}
	return "", false, nil
}

// assistedBrowserIsLive reports whether any assisted application currently
// holds a live lease, using the same heartbeat window AcquireAssistedLease
// applies so the two cannot disagree about what "live" means.
func assistedBrowserIsLive(conn *sql.DB) (bool, error) {
	var live int
	err := conn.QueryRow(`SELECT COUNT(*) FROM assisted_applications
		WHERE lease_owner != '' AND lease_expires_at > ? AND updated_at > ?`,
		time.Now().UTC(), time.Now().UTC().Add(-assistedLeaseHeartbeatTimeout)).Scan(&live)
	if err != nil {
		return false, fmt.Errorf("check for a live assisted browser: %w", err)
	}
	return live > 0, nil
}

// AssistedBrowserIsLive is the exported form, for callers outside this package
// that must not start work while the operator has an application open --
// currently the preflight launcher, which would otherwise put a second Chromium
// on the machine while the first one is mid-application.
func AssistedBrowserIsLive(conn *sql.DB) (bool, error) {
	if conn == nil {
		return false, errors.New("database not initialized")
	}
	return assistedBrowserIsLive(conn)
}

// AssistedWorkFinished reports whether this job's place in the open apply
// session has reached an outcome, so the process holding its browser knows its
// work is done.
//
// cmd/assist polls this. Without it, skipping or stopping a session left the
// browser open indefinitely holding the only visible-browser lease, so the
// session could never advance — the operator pressed Skip and nothing happened
// (bugs.md #542). Confirming already closed the browser, because that path sets
// assisted_state to 'completed', which the same loop watches for; skip and stop
// had no equivalent signal until this.
func AssistedWorkFinished(conn *sql.DB, jobID string) (bool, error) {
	if conn == nil {
		return false, errors.New("database not initialized")
	}
	var state string
	err := conn.QueryRow(`SELECT i.state FROM application_session_items i
		JOIN application_sessions s ON s.id = i.session_id
		WHERE i.job_id = ? AND s.state != ? ORDER BY i.id DESC LIMIT 1`, jobID, SessionFinished).Scan(&state)
	if errors.Is(err, sql.ErrNoRows) {
		// Not part of an open session: this browser was opened on its own and
		// only its own state governs it.
		return false, nil
	}
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "no such table") {
			return false, nil
		}
		return false, fmt.Errorf("check apply session item state: %w", err)
	}
	return terminalItemStates[state], nil
}

// MarkApplySessionItemOpen records that this job's browser is now open.
func MarkApplySessionItemOpen(conn *sql.DB, jobID string) error {
	if conn == nil {
		return errors.New("database not initialized")
	}
	_, err := conn.Exec(`UPDATE application_session_items
		SET state = ?, started_at = ?
		WHERE job_id = ? AND state = ? AND session_id IN (SELECT id FROM application_sessions WHERE state != ?)`,
		ItemInProgress, time.Now().UTC(), jobID, ItemPending, SessionFinished)
	if err != nil {
		return fmt.Errorf("open apply session item: %w", err)
	}
	return nil
}

// GetApplySessionItemStartedAt reports when this application began review/interaction
// in a session, or the zero time when unknown or not part of a session.
func GetApplySessionItemStartedAt(conn *sql.DB, jobID string) time.Time {
	if conn == nil || strings.TrimSpace(jobID) == "" {
		return time.Time{}
	}
	var startedAt sql.NullTime
	err := conn.QueryRow(`SELECT started_at FROM application_session_items
		WHERE job_id = ? AND started_at IS NOT NULL
		ORDER BY id DESC LIMIT 1`, jobID).Scan(&startedAt)
	if err != nil || !startedAt.Valid {
		return time.Time{}
	}
	return startedAt.Time
}

// advanceApplySessionItemTx records a terminal outcome for one item inside a
// caller's transaction, and finishes the session when nothing is left.
//
// This is deliberately transaction-scoped and unexported. Its only callers are
// ConfirmAssistedSubmission and MarkAssistedNotFound, which record the outcome
// of an application and the outcome of a session item in the same commit — so
// there is no window in which an application is confirmed but the session
// still thinks it is open, or the reverse.
func advanceApplySessionItemTx(tx *sql.Tx, jobID, state, reason string, now time.Time) error {
	if !terminalItemStates[state] {
		return fmt.Errorf("refusing to advance an apply session on non-terminal state %q", state)
	}
	result, err := tx.Exec(`UPDATE application_session_items
		SET state = ?, terminal_reason = ?, ended_at = ?
		WHERE job_id = ? AND state IN (?, ?)
			AND session_id IN (SELECT id FROM application_sessions WHERE state != ?)`,
		state, reason, now, jobID, ItemPending, ItemInProgress, SessionFinished)
	if err != nil {
		return fmt.Errorf("advance apply session item: %w", err)
	}
	if changed, _ := result.RowsAffected(); changed == 0 {
		// This job is not part of an open session. That is the ordinary case
		// for a one-off application and is not an error.
		return nil
	}
	return finishApplySessionIfCompleteTx(tx, now)
}

// finishApplySessionIfCompleteTx closes a session once no item can still be
// worked, and honours stop-after-current.
func finishApplySessionIfCompleteTx(tx *sql.Tx, now time.Time) error {
	var sessionID int64
	var stopAfter int
	err := tx.QueryRow(`SELECT id, stop_after_current FROM application_sessions WHERE state != ? ORDER BY id DESC LIMIT 1`, SessionFinished).Scan(&sessionID, &stopAfter)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	var remaining int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM application_session_items WHERE session_id = ? AND state IN (?, ?)`,
		sessionID, ItemPending, ItemInProgress).Scan(&remaining); err != nil {
		return err
	}
	if remaining == 0 || stopAfter != 0 {
		if remaining > 0 {
			// Stop-after-current: everything still waiting is recorded as
			// stopped rather than left dangling, so the session's item states
			// still add up to what happened.
			if _, err := tx.Exec(`UPDATE application_session_items SET state = ?, terminal_reason = 'operator stopped the session', ended_at = ?
				WHERE session_id = ? AND state = ?`, ItemStopped, now, sessionID, ItemPending); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(`UPDATE application_sessions SET state = ?, updated_at = ? WHERE id = ?`, SessionFinished, now, sessionID); err != nil {
			return err
		}
	}
	return nil
}

// AdvanceApplySession records a terminal outcome for a job outside any other
// transaction. Used by the skip and defer paths, which have no funnel write of
// their own to join.
func AdvanceApplySession(conn *sql.DB, jobID, state, reason string) error {
	if conn == nil {
		return errors.New("database not initialized")
	}
	tx, err := conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := advanceApplySessionItemTx(tx, jobID, state, reason, time.Now().UTC()); err != nil {
		return err
	}
	return tx.Commit()
}

// PauseApplySessionForClosedBrowser is what happens when an assisted browser
// closes without the operator confirming anything.
//
// The item goes back to pending and the session pauses. It does not advance,
// because a closed browser is not evidence of a submitted application — the
// employer may have received it, or the operator may have given up halfway,
// and Career Agent cannot tell those apart. Guessing either way would either
// lose a real application or record one that never happened.
func PauseApplySessionForClosedBrowser(conn *sql.DB, jobID string) error {
	if conn == nil {
		return errors.New("database not initialized")
	}
	tx, err := conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := time.Now().UTC()
	result, err := tx.Exec(`UPDATE application_session_items SET state = ?, started_at = NULL
		WHERE job_id = ? AND state = ? AND session_id IN (SELECT id FROM application_sessions WHERE state != ?)`,
		ItemPending, jobID, ItemInProgress, SessionFinished)
	if err != nil {
		return fmt.Errorf("return apply session item to pending: %w", err)
	}
	if changed, _ := result.RowsAffected(); changed == 0 {
		return nil
	}
	if _, err := tx.Exec(`UPDATE application_sessions SET state = ?, pause_reason = ?, updated_at = ?
		WHERE state = ?`, SessionPaused, PauseReasonBrowserClosed, now, SessionRunning); err != nil {
		return fmt.Errorf("pause apply session: %w", err)
	}
	return tx.Commit()
}

// SetApplySessionState pauses or resumes the open session.
func SetApplySessionState(conn *sql.DB, state string) error {
	if conn == nil {
		return errors.New("database not initialized")
	}
	if state != SessionRunning && state != SessionPaused {
		return fmt.Errorf("unsupported apply session state %q", state)
	}
	result, err := conn.Exec(`UPDATE application_sessions SET state = ?, pause_reason = '', updated_at = ?
		WHERE state != ?`, state, time.Now().UTC(), SessionFinished)
	if err != nil {
		return fmt.Errorf("set apply session state: %w", err)
	}
	if changed, _ := result.RowsAffected(); changed == 0 {
		return errors.New("no apply session is open")
	}
	return nil
}

// SetApplySessionStopAfterCurrent asks the session to end once the application
// in progress reaches an outcome.
func SetApplySessionStopAfterCurrent(conn *sql.DB) error {
	if conn == nil {
		return errors.New("database not initialized")
	}
	result, err := conn.Exec(`UPDATE application_sessions SET stop_after_current = 1, updated_at = ?
		WHERE state != ?`, time.Now().UTC(), SessionFinished)
	if err != nil {
		return fmt.Errorf("stop apply session after current: %w", err)
	}
	if changed, _ := result.RowsAffected(); changed == 0 {
		return errors.New("no apply session is open")
	}
	return nil
}

// StopApplySession ends the session now. Items still waiting are recorded as
// stopped: an application nobody worked is a real outcome and is written down
// as one, rather than being deleted as though it had never been selected.
func StopApplySession(conn *sql.DB) error {
	if conn == nil {
		return errors.New("database not initialized")
	}
	tx, err := conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := time.Now().UTC()
	var sessionID int64
	err = tx.QueryRow(`SELECT id FROM application_sessions WHERE state != ? ORDER BY id DESC LIMIT 1`, SessionFinished).Scan(&sessionID)
	if errors.Is(err, sql.ErrNoRows) {
		return errors.New("no apply session is open")
	}
	if err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE application_session_items SET state = ?, terminal_reason = 'operator stopped the session', ended_at = ?
		WHERE session_id = ? AND state IN (?, ?)`, ItemStopped, now, sessionID, ItemPending, ItemInProgress); err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE application_sessions SET state = ?, updated_at = ? WHERE id = ?`, SessionFinished, now, sessionID); err != nil {
		return err
	}
	return tx.Commit()
}
