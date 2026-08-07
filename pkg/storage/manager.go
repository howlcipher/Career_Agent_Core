package storage

import (
	"crypto/sha256"
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/danielthedm/promptsec"
	"github.com/howlcipher/Career_Agent_Core/pkg/security"
	_ "modernc.org/sqlite"
)

var db *sql.DB

// SkippedReasonExcludedSource identifies a posting from an ATS deliberately
// excluded from automated submission. It lets the dashboard distinguish this
// operator policy from an ordinary low-fit SKIPPED row.
const SkippedReasonExcludedSource = "excluded_source"

// SkippedReasonDuplicateCooldown identifies a job skipped because a recent,
// strictly equivalent application exists. The matcher intentionally requires
// complete location and remote metadata rather than guessing equivalence.
const SkippedReasonDuplicateCooldown = "duplicate_cooldown"

// SkippedReasonFirstAttemptExpired marks a job that never received a first
// attempt during its 30-day intake window. It is distinct from a posting that
// was checked and found expired: this is queue-lifecycle evidence only.
const SkippedReasonFirstAttemptExpired = "first_attempt_sla_expired"

// SkippedReasonSourceAdmissionCap records a deliberate queue-admission limit,
// rather than pretending a discovery result was low-fit or malformed.
const SkippedReasonSourceAdmissionCap = "source_pending_cap"

const (
	firstAttemptSLAPriorityDays = 7
	firstAttemptSLAPriorityAge  = firstAttemptSLAPriorityDays * 24 * time.Hour
	firstAttemptExpiryAge       = 30 * 24 * time.Hour
	maxPendingPerSource         = 25
)

func InitDB() error {
	return InitDBWithPath(DefaultDatabasePath)
}

func InitDBWithPath(path string) error {
	security.SetPrivateUmask()
	databasePath, _, _ := strings.Cut(path, "?")
	if databasePath == ":memory:" {
		databasePath = ""
	}
	if err := secureSQLiteFiles(databasePath); err != nil {
		return fmt.Errorf("secure database files before opening: %w", err)
	}

	var err error
	db, err = sql.Open("sqlite", DSN(path))
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}

	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)

	createTableQuery := `
	CREATE TABLE IF NOT EXISTS applied_jobs (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		company_name TEXT,
		job_title TEXT,
		url TEXT UNIQUE,
		applied_at DATETIME
	);
	CREATE TABLE IF NOT EXISTS execution_state (
		job_id TEXT PRIMARY KEY,
		url TEXT,
		status TEXT,
		last_updated DATETIME
	);
	CREATE TABLE IF NOT EXISTS career_sites (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		domain TEXT UNIQUE,
		ats_provider TEXT,
		last_scanned DATETIME,
		last_successful_form_reach DATETIME,
		account_required INTEGER NOT NULL DEFAULT 0,
		confirmation_strategy TEXT,
		mapping_health TEXT NOT NULL DEFAULT 'unknown'
	);
	CREATE TABLE IF NOT EXISTS job_funnel (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		company_name TEXT,
		job_title TEXT,
		url TEXT UNIQUE,
		status TEXT,
		fit_score INTEGER,
		discovered_at DATETIME,
		applied_at DATETIME,
		last_updated DATETIME,
		fit_similarity REAL,
		tone_variant TEXT,
		retry_count INTEGER DEFAULT 0,
		next_eligible_at DATETIME,
		status_reason TEXT,
		discovery_source TEXT,
		job_location TEXT,
		is_remote INTEGER,
		processing_intent TEXT
	);
	CREATE TABLE IF NOT EXISTS form_mappings (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		domain TEXT UNIQUE,
		mapping_json TEXT,
		created_at DATETIME,
		success_count INTEGER NOT NULL DEFAULT 0,
		failure_count INTEGER NOT NULL DEFAULT 0,
		last_validated_at DATETIME
	);
	CREATE TABLE IF NOT EXISTS execution_logs (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		job_id TEXT,
		url TEXT,
		tokens_used INTEGER,
		status TEXT,
		logged_at DATETIME
	);
	CREATE TABLE IF NOT EXISTS career_chunks (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		chunk_text TEXT,
		embedding_json TEXT
	);
	CREATE TABLE IF NOT EXISTS processed_emails (
		message_id TEXT PRIMARY KEY,
		processed_at DATETIME
	);
	CREATE TABLE IF NOT EXISTS unmatched_outcomes (
		message_id TEXT PRIMARY KEY,
		outcome_status TEXT NOT NULL,
		sender_domain TEXT NOT NULL,
		detected_at DATETIME NOT NULL
	);
	CREATE TABLE IF NOT EXISTS application_attempts (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		source TEXT,
		url TEXT,
		terminal_class TEXT,
		reason_code TEXT,
		started_at DATETIME,
		ended_at DATETIME,
		model_call_count INTEGER,
		inference_ms INTEGER
	);
	CREATE TABLE IF NOT EXISTS funnel_stage_events (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		url TEXT NOT NULL,
		prior_status TEXT,
		new_status TEXT NOT NULL,
		pipeline_stage TEXT NOT NULL,
		reason_code TEXT NOT NULL,
		occurred_at DATETIME NOT NULL,
		stage_duration_ms INTEGER
	);
	CREATE TABLE IF NOT EXISTS daemon_watchdog_alert (
		id INTEGER PRIMARY KEY CHECK (id = 1),
		message TEXT NOT NULL,
		updated_at DATETIME NOT NULL
	);
	CREATE TABLE IF NOT EXISTS discovery_refresh (
		id INTEGER PRIMARY KEY CHECK (id = 1),
		started_at DATETIME NOT NULL,
		finished_at DATETIME NOT NULL,
		new_eligible INTEGER NOT NULL DEFAULT 0,
		error_class TEXT NOT NULL DEFAULT ''
	);
	CREATE INDEX IF NOT EXISTS idx_job_funnel_status ON job_funnel(status);
	CREATE INDEX IF NOT EXISTS idx_application_attempts_started ON application_attempts(started_at);
	CREATE INDEX IF NOT EXISTS idx_funnel_stage_events_url_id ON funnel_stage_events(url, id);
	CREATE INDEX IF NOT EXISTS idx_funnel_stage_events_status_time ON funnel_stage_events(new_status, occurred_at);
	CREATE INDEX IF NOT EXISTS idx_job_funnel_company_name ON job_funnel(company_name);
	CREATE INDEX IF NOT EXISTS idx_career_sites_last_scanned ON career_sites(last_scanned);
	CREATE INDEX IF NOT EXISTS idx_applied_jobs_applied_at ON applied_jobs(applied_at);`
	if _, err = db.Exec(createTableQuery); err != nil {
		return err
	}

	// CREATE TABLE IF NOT EXISTS never alters an already-existing table, so
	// a job_funnel table created before last_updated was added to the
	// schema above needs an explicit migration.
	if err := migrateJobFunnelLastUpdated(); err != nil {
		return err
	}
	if err := migrateJobFunnelFitSimilarity(); err != nil {
		return err
	}
	if err := migrateJobFunnelToneVariant(); err != nil {
		return err
	}
	if err := migrateURLSchemes(); err != nil {
		return err
	}
	if err := migrateJobFunnelRetry(); err != nil {
		return err
	}
	if err := migrateJobFunnelStatusReason(); err != nil {
		return err
	}
	if err := migrateJobFunnelDiscoverySource(); err != nil {
		return err
	}
	if err := migrateJobFunnelIdentity(); err != nil {
		return err
	}
	if err := migrateCareerSiteCapabilities(); err != nil {
		return err
	}
	if err := migrateFormMappingHealth(); err != nil {
		return err
	}
	if err := migrateApplicationAttemptReasonCode(); err != nil {
		return err
	}
	if err := createFunnelStageLedgerTrigger(); err != nil {
		return err
	}
	// improvement #509: per-source discovery counts column.
	if err := migrateDiscoveryRefreshSourceCounts(); err != nil {
		return err
	}
	if err := migrateAssistedApplications(); err != nil {
		return err
	}
	if err := migrateJobFunnelProcessingIntent(); err != nil {
		return err
	}
	if err := secureSQLiteFiles(databasePath); err != nil {
		return fmt.Errorf("secure database files after initialization: %w", err)
	}
	return nil
}

func secureSQLiteFiles(databasePath string) error {
	if databasePath == "" {
		return nil
	}
	return errors.Join(
		security.SecurePrivateFile(databasePath),
		security.SecurePrivateFile(databasePath+"-wal"),
		security.SecurePrivateFile(databasePath+"-shm"),
	)
}

func migrateJobFunnelLastUpdated() error {
	rows, err := db.Query("PRAGMA table_info(job_funnel)")
	if err != nil {
		return fmt.Errorf("failed to inspect job_funnel schema: %w", err)
	}
	defer rows.Close()

	hasLastUpdated := false
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull, pk int
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			return fmt.Errorf("failed to scan job_funnel column info: %w", err)
		}
		if name == "last_updated" {
			hasLastUpdated = true
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if hasLastUpdated {
		return nil
	}

	_, err = db.Exec("ALTER TABLE job_funnel ADD COLUMN last_updated DATETIME")
	return err
}

// migrateJobFunnelFitSimilarity adds job_funnel.fit_similarity (improvements.md
// #22) to a database created before that column existed, same idempotent
// pattern as migrateJobFunnelLastUpdated above.
func migrateJobFunnelFitSimilarity() error {
	rows, err := db.Query("PRAGMA table_info(job_funnel)")
	if err != nil {
		return fmt.Errorf("failed to inspect job_funnel schema: %w", err)
	}
	defer rows.Close()

	hasFitSimilarity := false
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull, pk int
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			return fmt.Errorf("failed to scan job_funnel column info: %w", err)
		}
		if name == "fit_similarity" {
			hasFitSimilarity = true
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if hasFitSimilarity {
		return nil
	}

	_, err = db.Exec("ALTER TABLE job_funnel ADD COLUMN fit_similarity REAL")
	return err
}

// migrateJobFunnelToneVariant adds job_funnel.tone_variant (improvements.md
// #13) to a database created before that column existed, same idempotent
// pattern as the other job_funnel migrations above.
func migrateJobFunnelToneVariant() error {
	rows, err := db.Query("PRAGMA table_info(job_funnel)")
	if err != nil {
		return fmt.Errorf("failed to inspect job_funnel schema: %w", err)
	}
	defer rows.Close()

	hasToneVariant := false
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull, pk int
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			return fmt.Errorf("failed to scan job_funnel column info: %w", err)
		}
		if name == "tone_variant" {
			hasToneVariant = true
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if hasToneVariant {
		return nil
	}

	_, err = db.Exec("ALTER TABLE job_funnel ADD COLUMN tone_variant TEXT")
	return err
}

// migrateJobFunnelRetry adds job_funnel.retry_count and .next_eligible_at
// (bugs.md #466) to a database created before those columns existed, same
// idempotent pattern as the other job_funnel migrations above. Both columns
// back UpdateFunnelStatusRetryable and GetDiscoveredJobs's eligibility
// filter: without them a retryable failure had no way to record how many
// times it had already been retried or when it should next be picked up
// again, so the continuous scheduler reselected the same rows every cycle.
func migrateJobFunnelRetry() error {
	rows, err := db.Query("PRAGMA table_info(job_funnel)")
	if err != nil {
		return fmt.Errorf("failed to inspect job_funnel schema: %w", err)
	}
	defer rows.Close()

	hasRetryCount := false
	hasNextEligibleAt := false
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull, pk int
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			return fmt.Errorf("failed to scan job_funnel column info: %w", err)
		}
		switch name {
		case "retry_count":
			hasRetryCount = true
		case "next_eligible_at":
			hasNextEligibleAt = true
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}

	if !hasRetryCount {
		if _, err := db.Exec("ALTER TABLE job_funnel ADD COLUMN retry_count INTEGER DEFAULT 0"); err != nil {
			return err
		}
	}
	if !hasNextEligibleAt {
		if _, err := db.Exec("ALTER TABLE job_funnel ADD COLUMN next_eligible_at DATETIME"); err != nil {
			return err
		}
	}
	return nil
}

// migrateJobFunnelStatusReason adds job_funnel.status_reason (improvements.md
// #468) to a database created before that column existed, same idempotent
// pattern as the other job_funnel migrations above. It backs
// UpdateFunnelStatusInvalid: without it, every INVALID_URL row collapsed to
// one status with no persisted reason, even though the code already
// distinguishes "never a real posting" (known-junk URL, unsafe network
// target) from "was a real posting, now expired" (dead redirect, terminal
// fetch status) at the point each row is written.
func migrateJobFunnelStatusReason() error {
	rows, err := db.Query("PRAGMA table_info(job_funnel)")
	if err != nil {
		return fmt.Errorf("failed to inspect job_funnel schema: %w", err)
	}
	defer rows.Close()

	hasStatusReason := false
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull, pk int
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			return fmt.Errorf("failed to scan job_funnel column info: %w", err)
		}
		if name == "status_reason" {
			hasStatusReason = true
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if hasStatusReason {
		return nil
	}

	_, err = db.Exec("ALTER TABLE job_funnel ADD COLUMN status_reason TEXT")
	return err
}

// migrateJobFunnelDiscoverySource adds job_funnel.discovery_source
// (improvements.md #499) without inventing a source for existing rows. The
// discovery channel was previously discarded at insertion time, so NULL is
// the only truthful value for rows created before this migration.
func migrateJobFunnelDiscoverySource() error {
	rows, err := db.Query("PRAGMA table_info(job_funnel)")
	if err != nil {
		return fmt.Errorf("failed to inspect job_funnel schema: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull, pk int
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			return fmt.Errorf("failed to scan job_funnel column info: %w", err)
		}
		if name == "discovery_source" {
			return nil
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}

	_, err = db.Exec("ALTER TABLE job_funnel ADD COLUMN discovery_source TEXT")
	return err
}

// migrateJobFunnelProcessingIntent adds job_funnel.processing_intent
func migrateJobFunnelProcessingIntent() error {
	rows, err := db.Query("PRAGMA table_info(job_funnel)")
	if err != nil {
		return fmt.Errorf("failed to inspect job_funnel schema: %w", err)
	}
	defer rows.Close()

	hasCol := false
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull, pk int
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			return fmt.Errorf("failed to scan job_funnel column info: %w", err)
		}
		if name == "processing_intent" {
			hasCol = true
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if hasCol {
		return nil
	}

	_, err = db.Exec("ALTER TABLE job_funnel ADD COLUMN processing_intent TEXT")
	return err
}

// migrateJobFunnelIdentity adds the conservative duplicate-matching metadata
// used by improvement #498. Existing rows deliberately remain NULL: a past
// role's location and remote classification cannot be reconstructed safely.
func migrateJobFunnelIdentity() error {
	columns := []struct {
		name     string
		typeName string
	}{
		{"job_location", "TEXT"},
		{"is_remote", "INTEGER"},
	}

	rows, err := db.Query("PRAGMA table_info(job_funnel)")
	if err != nil {
		return fmt.Errorf("failed to inspect job_funnel schema: %w", err)
	}
	defer rows.Close()

	existing := make(map[string]bool, len(columns))
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull, pk int
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			return fmt.Errorf("failed to scan job_funnel column info: %w", err)
		}
		existing[name] = true
	}
	if err := rows.Err(); err != nil {
		return err
	}

	for _, column := range columns {
		if existing[column.name] {
			continue
		}
		if _, err := db.Exec("ALTER TABLE job_funnel ADD COLUMN " + column.name + " " + column.typeName); err != nil {
			return fmt.Errorf("add job_funnel.%s: %w", column.name, err)
		}
	}
	return nil
}

// migrateCareerSiteCapabilities upgrades the previously unused career_sites
// table so each observed site can retain only durable, outcome-derived
// capability evidence. Existing rows intentionally retain unknown values:
// inventing a past capability from an old URL would make the registry less
// trustworthy than leaving it empty.
func migrateCareerSiteCapabilities() error {
	columns := []struct {
		name       string
		definition string
	}{
		{"last_successful_form_reach", "DATETIME"},
		{"account_required", "INTEGER NOT NULL DEFAULT 0"},
		{"confirmation_strategy", "TEXT"},
		{"mapping_health", "TEXT NOT NULL DEFAULT 'unknown'"},
	}
	return addMissingColumns("career_sites", columns)
}

// migrateFormMappingHealth adds outcome-derived health fields to cached form
// mappings. A mapping's JSON remains the source of selectors; these fields
// only decide whether the cache has enough negative evidence to yield to the
// provider-specific fallback.
func migrateFormMappingHealth() error {
	columns := []struct {
		name       string
		definition string
	}{
		{"success_count", "INTEGER NOT NULL DEFAULT 0"},
		{"failure_count", "INTEGER NOT NULL DEFAULT 0"},
		{"last_validated_at", "DATETIME"},
	}
	return addMissingColumns("form_mappings", columns)
}

// migrateApplicationAttemptReasonCode preserves historical coarse outcome
// classes while letting new attempts retain a privacy-safe, normalized cause.
func migrateApplicationAttemptReasonCode() error {
	return addMissingColumns("application_attempts", []struct {
		name       string
		definition string
	}{{"reason_code", "TEXT"}})
}

// createFunnelStageLedgerTrigger records every persisted status transition in
// one place. Keeping this at the database boundary prevents a new writer from
// accidentally bypassing the ledger. The ledger stores only state metadata,
// never job content, prompts, answers, documents, or browser artifacts.
func createFunnelStageLedgerTrigger() error {
	_, err := db.Exec(`
		CREATE TRIGGER IF NOT EXISTS record_funnel_stage_transition
		AFTER UPDATE OF status ON job_funnel
		WHEN OLD.status IS NOT NEW.status
		BEGIN
			INSERT INTO funnel_stage_events
				(url, prior_status, new_status, pipeline_stage, reason_code, occurred_at, stage_duration_ms)
			VALUES (
				NEW.url,
				OLD.status,
				NEW.status,
				CASE NEW.status
					WHEN 'PROCESSING' THEN 'intake'
					WHEN 'FAILED_SCORE' THEN 'scoring'
					WHEN 'QUARANTINED_RAG_CONTEXT' THEN 'tailoring'
					WHEN 'APPLIED' THEN 'submission'
					WHEN 'AWAITING_REVIEW' THEN 'submission'
					WHEN 'MANUAL_REQUIRED' THEN 'submission'
					WHEN 'BLOCKED_CAPTCHA' THEN 'submission'
					WHEN 'FAILED_SUBMIT' THEN 'submission'
					WHEN 'QUARANTINED_PROMPT_INJECTION' THEN 'submission'
					ELSE 'funnel'
				END,
				COALESCE(NULLIF(NEW.status_reason, ''), lower(NEW.status)),
				COALESCE(NEW.last_updated, CURRENT_TIMESTAMP),
				CASE WHEN (
					SELECT occurred_at FROM funnel_stage_events
					WHERE url = NEW.url ORDER BY id DESC LIMIT 1
				) IS NULL THEN NULL ELSE CAST(ROUND((julianday(NEW.last_updated) - julianday((
					SELECT occurred_at FROM funnel_stage_events
					WHERE url = NEW.url ORDER BY id DESC LIMIT 1
				))) * 86400000) AS INTEGER) END
			);
		END`)
	return err
}

func addMissingColumns(table string, columns []struct {
	name       string
	definition string
}) error {
	rows, err := db.Query("PRAGMA table_info(" + table + ")")
	if err != nil {
		return fmt.Errorf("inspect %s schema: %w", table, err)
	}
	defer rows.Close()

	existing := make(map[string]bool, len(columns))
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull, pk int
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			return fmt.Errorf("scan %s column info: %w", table, err)
		}
		existing[name] = true
	}
	if err := rows.Err(); err != nil {
		return err
	}

	for _, column := range columns {
		if existing[column.name] {
			continue
		}
		if _, err := db.Exec("ALTER TABLE " + table + " ADD COLUMN " + column.name + " " + column.definition); err != nil {
			return fmt.Errorf("add %s.%s: %w", table, column.name, err)
		}
	}
	return nil
}

// mergeStatusRank ranks job_funnel statuses for mergeStatuses' scheme-dedup
// merges: the higher-ranked status wins and survives the merge.
//
// The ordering is a deliberate decision (bug #433), not an accident of
// insertion order — do not "tidy" it without re-reading the rationale below.
//
//   - Anything terminal must outrank DISCOVERED/PROCESSING, or a merge
//     silently requeues a closed job. That is the whole bug: five statuses
//     the codebase actually writes (FAILED_SCORE, PROCESSED_MANUAL,
//     INVALID_URL, and the two QUARANTINED_* statuses) used to fall through
//     to the default case and rank below DISCOVERED, so merging a dedup pair
//     could resurrect a job that had been deliberately closed.
//   - INVALID_URL sits above the "we tried and it failed" statuses
//     (FAILED_SUBMIT, BLOCKED_CAPTCHA, RETRY_EXHAUSTED, etc.) because the
//     URL is not a posting at all — there is nothing to retry.
//   - The two QUARANTINED_* statuses rank above every non-outcome status so
//     a merge can never reopen a security closure, but deliberately below
//     APPLIED/REJECTED/INTERVIEW_REQUESTED: those are real observed
//     outcomes and a merge must not destroy them. There is no reprocessing
//     risk in ranking them below those three, because none of those three
//     statuses is ever pulled back into the queue anyway.
func mergeStatusRank(s string) int {
	switch s {
	case "DISCOVERED":
		return 1
	case "PROCESSING":
		return 2
	case "FAILED_SCORE":
		return 3
	case "SKIPPED":
		return 4
	case "BLOCKED_CAPTCHA":
		return 5
	case "FAILED_SUBMIT":
		return 6
	case "RETRY_EXHAUSTED":
		return 7
	case "INVALID_URL":
		return 8
	case "MANUAL_REQUIRED", "AWAITING_REVIEW":
		return 9
	case "PROCESSED_MANUAL":
		return 10
	case "QUARANTINED_PROMPT_INJECTION", "QUARANTINED_RAG_CONTEXT":
		return 11
	case "APPLIED":
		return 12
	case "REJECTED":
		return 13
	case "INTERVIEW_REQUESTED":
		return 14
	default:
		// Unknown status: loses to everything. This is exactly how bug
		// #433 happened — a status the codebase writes but this switch
		// doesn't know about silently ranks below DISCOVERED. Any new
		// status added to job_funnel MUST get an explicit rank above.
		return 0
	}
}

func mergeStatuses(s1, s2 string) string {
	if s1 == s2 {
		return s1
	}

	r1, r2 := mergeStatusRank(s1), mergeStatusRank(s2)

	isSuccess1 := s1 == "APPLIED" || s1 == "REJECTED" || s1 == "INTERVIEW_REQUESTED"
	isSuccess2 := s2 == "APPLIED" || s2 == "REJECTED" || s2 == "INTERVIEW_REQUESTED"
	isFailure1 := s1 == "FAILED_SUBMIT" || s1 == "BLOCKED_CAPTCHA" || s1 == "MANUAL_REQUIRED" || s1 == "AWAITING_REVIEW"
	isFailure2 := s2 == "FAILED_SUBMIT" || s2 == "BLOCKED_CAPTCHA" || s2 == "MANUAL_REQUIRED" || s2 == "AWAITING_REVIEW"

	if (isSuccess1 && isFailure2) || (isSuccess2 && isFailure1) {
		return "MANUAL_REQUIRED"
	}
	if r1 > r2 {
		return s1
	}
	return s2
}

func migrateURLSchemes() error {
	rows, err := db.Query(`SELECT id, url, status FROM job_funnel WHERE url LIKE 'http://%'`)
	if err != nil {
		return fmt.Errorf("failed to query http urls in job_funnel: %w", err)
	}
	defer rows.Close()

	type httpRow struct {
		id     int
		url    string
		status string
	}
	var toProcess []httpRow
	for rows.Next() {
		var r httpRow
		if err := rows.Scan(&r.id, &r.url, &r.status); err != nil {
			return err
		}
		toProcess = append(toProcess, r)
	}
	rows.Close()

	rowsApp, err := db.Query(`SELECT id, url FROM applied_jobs WHERE url LIKE 'http://%'`)
	if err != nil {
		return fmt.Errorf("failed to query http urls in applied_jobs: %w", err)
	}
	defer rowsApp.Close()

	type appRow struct {
		id  int
		url string
	}
	var appToProcess []appRow
	for rowsApp.Next() {
		var r appRow
		if err := rowsApp.Scan(&r.id, &r.url); err != nil {
			return err
		}
		appToProcess = append(appToProcess, r)
	}
	rowsApp.Close()

	if len(toProcess) == 0 && len(appToProcess) == 0 {
		return nil
	}

	tx, err := db.Begin()
	if err != nil {
		return err
	}

	for _, r := range toProcess {
		httpsURL := "https://" + strings.TrimPrefix(r.url, "http://")
		var httpsID int
		var httpsStatus string
		err := tx.QueryRow(`SELECT id, status FROM job_funnel WHERE url = ?`, httpsURL).Scan(&httpsID, &httpsStatus)
		if err == sql.ErrNoRows {
			_, err = tx.Exec(`UPDATE job_funnel SET url = ? WHERE id = ?`, httpsURL, r.id)
			if err != nil {
				tx.Rollback()
				return fmt.Errorf("failed to update job_funnel url to https: %w", err)
			}
			continue
		} else if err != nil {
			tx.Rollback()
			return fmt.Errorf("failed to check https url in job_funnel: %w", err)
		}

		newStatus := mergeStatuses(r.status, httpsStatus)

		_, err = tx.Exec(`UPDATE job_funnel SET status = ? WHERE id = ?`, newStatus, httpsID)
		if err != nil {
			tx.Rollback()
			return err
		}

		_, err = tx.Exec(`DELETE FROM job_funnel WHERE id = ?`, r.id)
		if err != nil {
			tx.Rollback()
			return err
		}
	}

	for _, r := range appToProcess {
		httpsURL := "https://" + strings.TrimPrefix(r.url, "http://")
		var httpsID int
		err := tx.QueryRow(`SELECT id FROM applied_jobs WHERE url = ?`, httpsURL).Scan(&httpsID)
		if err == sql.ErrNoRows {
			_, err = tx.Exec(`UPDATE applied_jobs SET url = ? WHERE id = ?`, httpsURL, r.id)
			if err != nil {
				tx.Rollback()
				return fmt.Errorf("failed to update applied_jobs url to https: %w", err)
			}
		} else if err == nil {
			_, err = tx.Exec(`DELETE FROM applied_jobs WHERE id = ?`, r.id)
			if err != nil {
				tx.Rollback()
				return fmt.Errorf("failed to delete duplicate http url from applied_jobs: %w", err)
			}
		} else {
			tx.Rollback()
			return fmt.Errorf("failed to check https url in applied_jobs: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return err
	}
	return nil
}

func GetDB() *sql.DB {
	return db
}

// GetTrackedCompanies returns the distinct company names of jobs whose
// status could legitimately change from an inbound email — the tracker
// must never write a status for a company we never applied to (bug #20).
func GetTrackedCompanies() ([]string, error) {
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	// Bug #434: AWAITING_REVIEW belongs here for the same reason
	// MANUAL_REQUIRED does — the user finishes those applications by hand, so
	// an inbound outcome email for one is legitimate. Leaving it out meant a
	// copilot-filled company was not even a candidate for correlation.
	rows, err := db.Query(`SELECT DISTINCT company_name FROM job_funnel
		WHERE status IN ('APPLIED', 'INTERVIEW_REQUESTED', 'MANUAL_REQUIRED', 'AWAITING_REVIEW') AND company_name != ''`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var companies []string
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err != nil {
			return nil, err
		}
		companies = append(companies, c)
	}
	return companies, rows.Err()
}

// WasEmailProcessed reports whether the tracker has already handled this
// IMAP Message-ID, so re-fetching the same recent messages every cycle
// doesn't re-detect (and re-log) the same threads (bug #20).
func WasEmailProcessed(messageID string) bool {
	if db == nil || messageID == "" {
		return false
	}
	var one int
	err := db.QueryRow("SELECT 1 FROM processed_emails WHERE message_id = ?", messageID).Scan(&one)
	return err == nil
}

// MarkEmailProcessed records an IMAP Message-ID as handled.
func MarkEmailProcessed(messageID string) error {
	if db == nil || messageID == "" {
		return nil
	}
	_, err := db.Exec("INSERT INTO processed_emails (message_id, processed_at) VALUES (?, ?) ON CONFLICT(message_id) DO NOTHING",
		messageID, time.Now().UTC())
	return err
}

// canonicalURLKey normalises a posting URL for dedup comparisons.
//
// bugs.md #112: job_funnel and applied_jobs are both keyed on the raw URL, and
// the same posting reaches this code under both schemes -- discovery yields
// https, some earlier records and verification lists hold http. Measured
// 2026-07-26: 20 scheme-duplicate pairs in job_funnel, 11 of them holding
// DIFFERENT statuses. For the dedup path that split is outward-facing: a job
// recorded as applied under one scheme is not deduped under the other, so it
// can be applied to twice.
//
// NormalizeURL normalises the scheme of a URL to https:// if it starts with http://.
// This prevents the same job from being tracked twice under different schemes.
// Only the scheme is normalised. Nothing else about the URL is touched, because
// query strings and trailing paths genuinely distinguish postings on Lever.
func NormalizeURL(rawURL string) string {
	if strings.HasPrefix(rawURL, "http://") {
		return "https://" + strings.TrimPrefix(rawURL, "http://")
	}
	return rawURL
}

func HasApplied(url string) bool {
	if db == nil {
		return false
	}
	url = NormalizeURL(url)
	var id int
	err := db.QueryRow(`SELECT id FROM applied_jobs WHERE url = ?`, url).Scan(&id)
	return err == nil
}

// RecordApplicationInDB writes the dedup row that HasApplied gates on.
//
// bugs.md #94: call this ONLY after a submission is genuinely confirmed. It
// used to run from SaveApplication, i.e. at document-generation time, so any
// job that generated documents and then failed to submit was recorded as
// applied. Combined with the funnel row returning to DISCOVERED (the startup
// reaper, #85's duplicate-path reset, or cmd/requeue without -clear-dedup),
// that job was then re-queued every run and skipped instantly, forever --
// silently unreachable rather than visibly failed.
//
// ON CONFLICT DO NOTHING because the row is no longer written on a path that
// is guaranteed to run once: a confirmation re-check (#89) can legitimately
// observe success twice for one URL, and the UNIQUE constraint on url would
// otherwise turn that into a spurious error on an application that worked.
func RecordApplicationInDB(companyName, jobTitle, url string) error {
	if db == nil {
		return fmt.Errorf("db not initialized")
	}
	url = NormalizeURL(url)
	_, err := db.Exec(`INSERT INTO applied_jobs (company_name, job_title, url, applied_at)
		VALUES (?, ?, ?, ?) ON CONFLICT(url) DO NOTHING`, companyName, jobTitle, url, time.Now().Format(time.RFC3339))
	return err
}

// HandoffStatuses are the funnel statuses that mean "the agent stopped and a
// human has to finish this": an ATS account gate (MANUAL_REQUIRED) or a
// deliberate pre-submit stop (AWAITING_REVIEW, copilot mode). Kept in one
// place because bug #434 was precisely the two halves of the codebase
// disagreeing about which statuses these were — GetTrackedCompanies listed one
// of them, the tracker's candidate query listed neither.
var HandoffStatuses = []string{"MANUAL_REQUIRED", "AWAITING_REVIEW"}

func isHandoffStatus(status string) bool {
	for _, s := range HandoffStatuses {
		if status == s {
			return true
		}
	}
	return false
}

// MarkHandoffApplied promotes a job the user submitted by hand out of its
// hand-off status, and is the only writer that moves a row out of one.
//
// Bug #434: nothing did this before. A job handed to the user stayed recorded
// as un-submitted forever, so the funnel under-reported every application the
// user personally sent, and the tracker — which correlates outcome emails
// against applied rows — dropped the resulting rejection or interview notice
// for exactly those applications.
//
// It is deliberately narrow. It refuses any row that is not currently in a
// hand-off status, so a stale ticked checkbox can never overwrite a REJECTED
// or INTERVIEW_REQUESTED outcome the tracker has since recorded, nor re-apply
// to something already APPLIED. A missing row is not an error: checklists
// outlive the database rows they describe.
//
// The status update and the dedup record commit together, because a promotion
// visible in the funnel but not in applied_jobs would let the agent re-apply
// to a job the user already submitted.
func MarkHandoffApplied(companyName, jobTitle, applyURL string) (bool, error) {
	if db == nil {
		return false, fmt.Errorf("db not initialized")
	}
	applyURL = NormalizeURL(applyURL)

	tx, err := db.Begin()
	if err != nil {
		return false, fmt.Errorf("begin handoff promotion: %w", err)
	}
	defer tx.Rollback()

	var current string
	err = tx.QueryRow(`SELECT status FROM job_funnel WHERE url = ?`, applyURL).Scan(&current)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("look up handoff status: %w", err)
	}
	if !isHandoffStatus(current) {
		return false, nil
	}

	now := time.Now().UTC()
	if _, err := tx.Exec(`UPDATE job_funnel
		SET status = ?, last_updated = ?, applied_at = ?
		WHERE url = ?`, "APPLIED", now, now, applyURL); err != nil {
		return false, fmt.Errorf("promote handoff row: %w", err)
	}

	// Same ON CONFLICT DO NOTHING reasoning as RecordApplicationInDB: the user
	// may tick a box that a previous reconcile run already promoted.
	if _, err := tx.Exec(`INSERT INTO applied_jobs (company_name, job_title, url, applied_at)
		VALUES (?, ?, ?, ?) ON CONFLICT(url) DO NOTHING`,
		companyName, jobTitle, applyURL, now.Format(time.RFC3339)); err != nil {
		return false, fmt.Errorf("record handoff application: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("commit handoff promotion: %w", err)
	}
	return true, nil
}

type Metadata struct {
	CompanyName        string    `json:"company_name"`
	JobTitle           string    `json:"job_title"`
	Location           string    `json:"location"`
	OriginalPostingURL string    `json:"original_posting_url"`
	ApplicationDate    time.Time `json:"application_date"`
}

// safeCompanyDirName maps a company name to the filesystem-safe directory
// name SaveApplication uses — shared so every path that references a
// company's docs folder (the manual-apply move, queue links) agrees with
// where the docs were actually written.
func safeCompanyDirName(companyName string) string {
	return strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			return r
		}
		return '_'
	}, companyName)
}

// CoverLetterPath returns where SaveApplication writes companyName's cover
// letter. Exported because callers outside this package need to hand that
// exact path to the submitter, and building it by hand is a live bug source:
// cmd/agent previously concatenated the raw company name while
// SaveApplication writes under safeCompanyDirName's sanitized one, so the two
// silently disagreed for any company whose name contains a space or
// punctuation (bugs.md #62).
func applicationDir(companyName, postingURL string) string {
	parsed, err := url.Parse(postingURL)
	if err == nil {
		parsed.Scheme = strings.ToLower(parsed.Scheme)
		parsed.Host = strings.ToLower(parsed.Host)
		parsed.Fragment = ""
		postingURL = parsed.String()
	}
	digest := sha256.Sum256([]byte(postingURL))
	return filepath.Join("applications", safeCompanyDirName(companyName), fmt.Sprintf("%x", digest[:8]))
}

// ResolveApplicationDir returns the directory that actually holds a job's saved
// documents. SaveApplication writes applications/<company>/<digest>, but
// MoveToManualApply *renames* that directory into
// applications/needs_manual_apply/<digest> as soon as a job needs a human —
// which in assisted/copilot mode is every job — leaving an empty company
// folder behind. Readers that only consult applicationDir therefore miss the
// documents entirely: the assisted dashboard served HTTP 404 for every
// copilot-path cover letter and reported cover_letter_ready false (bugs.md
// #517). Falls back to the original path when neither home exists, so callers
// still get a meaningful path in their error.
func ResolveApplicationDir(companyName, postingURL string) string {
	primary := applicationDir(companyName, postingURL)
	if isExistingDir(primary) {
		return primary
	}
	digest := filepath.Base(primary)
	if moved := filepath.Join(manualApplyBase, digest); isExistingDir(moved) {
		return moved
	}
	// MoveToManualApply appends a numeric suffix rather than overwriting when
	// the destination already exists. That only happens when the same posting
	// URL is moved more than once, so the scan is deliberately shallow.
	for i := 2; i < 10; i++ {
		candidate := filepath.Join(manualApplyBase, fmt.Sprintf("%s-%d", digest, i))
		if isExistingDir(candidate) {
			return candidate
		}
	}
	return primary
}

func isExistingDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// CoverLetterPath returns the exact role-specific cover letter saved for a
// posting. Company names alone are not a stable artifact key (bugs.md #128).
func CoverLetterPath(companyName, postingURL string) string {
	return filepath.Join(applicationDir(companyName, postingURL), "coverletter.txt")
}

var applicationSaveMutex sync.Mutex

// SaveApplication writes a role's documents into its own stable URL-keyed
// directory and returns that exact directory for later manual handoff.
func SaveApplication(companyName, jobTitle, location, postingURL, resumeContent, coverLetterContent, interviewPrepContent string) (string, error) {
	applicationSaveMutex.Lock()
	defer applicationSaveMutex.Unlock()

	docsDir := applicationDir(companyName, postingURL)
	if err := os.MkdirAll(docsDir, security.PrivateDirMode); err != nil {
		return "", fmt.Errorf("failed to create application directory: %w", err)
	}

	resumePath := filepath.Join(docsDir, "resume.md")
	if err := atomicWritePrivateFile(resumePath, []byte(resumeContent)); err != nil {
		return "", fmt.Errorf("failed to write resume: %w", err)
	}

	coverLetterPath := filepath.Join(docsDir, "coverletter.txt")
	if err := atomicWritePrivateFile(coverLetterPath, []byte(coverLetterContent)); err != nil {
		return "", fmt.Errorf("failed to write cover letter: %w", err)
	}

	interviewPrepPath := filepath.Join(docsDir, "interview_prep.md")
	if err := atomicWritePrivateFile(interviewPrepPath, []byte(interviewPrepContent)); err != nil {
		return "", fmt.Errorf("failed to write interview prep: %w", err)
	}

	metadata := Metadata{
		CompanyName:        companyName,
		JobTitle:           jobTitle,
		Location:           location,
		OriginalPostingURL: postingURL,
		ApplicationDate:    time.Now(),
	}

	metadataBytes, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal metadata: %w", err)
	}

	metadataPath := filepath.Join(docsDir, "metadata.json")
	if err := atomicWritePrivateFile(metadataPath, metadataBytes); err != nil {
		return "", fmt.Errorf("failed to write metadata: %w", err)
	}

	// bugs.md #94: deliberately does NOT write the applied_jobs dedup row.
	// Generating documents is not applying -- the submit can still bounce,
	// need manual review, or be interrupted. cmd/agent records the row on the
	// confirmed-submission branch instead. This function stays responsible for
	// the folder MoveToManualApply archives and the record the dashboard reads.
	return docsDir, nil
}

var logMutex sync.Mutex

// LogFailedSubmission appends a failed auto-submission to a manual review checklist
func LogFailedSubmission(companyName, jobTitle, applyURL string) error {
	logMutex.Lock()
	defer logMutex.Unlock()

	if err := os.MkdirAll("applications", security.PrivateDirMode); err != nil {
		return fmt.Errorf("failed to create applications directory: %w", err)
	}
	reportPath := filepath.Join("applications", "manual_submissions.md")

	if _, err := os.Stat(reportPath); os.IsNotExist(err) {
		header := "# Manual Submission Backlog\n\nThe auto-submitter failed to process the following applications. Please submit them manually:\n\n"
		if err := writePrivateFile(reportPath, []byte(header)); err != nil {
			return fmt.Errorf("failed to initialize manual submission report: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("failed to inspect manual submission report: %w", err)
	}

	applyURL = NormalizeURL(applyURL)
	entry := fmt.Sprintf("- [ ] **%s** - %s: [Apply Here](%s)\n", companyName, jobTitle, applyURL)

	f, err := openPrivateAppendFile(reportPath)
	if err != nil {
		return fmt.Errorf("failed to open manual submission report: %w", err)
	}
	defer f.Close()

	if _, err = f.WriteString(entry); err != nil {
		return fmt.Errorf("failed to write to manual submission report: %w", err)
	}

	return nil
}

// manualApplyBase is the single home for everything a human needs to act
// on: the queue file plus each account-gated job's tailored-docs folder.
var manualApplyBase = filepath.Join("applications", "needs_manual_apply")

// MoveToManualApply relocates one exact saved docs directory into
// applications/needs_manual_apply/.
// so account-gated jobs live in one clearly-labeled place. Returns the
// destination path, or "" if the source folder doesn't exist (docs may
// have failed to save). A pre-existing destination gets a numeric suffix
// rather than being overwritten — company-name collisions are real
// (pre-#19 rows share labels like "en_US").
func MoveToManualApply(src string) (string, error) {
	if _, err := os.Stat(src); os.IsNotExist(err) {
		return "", nil
	}
	if err := os.MkdirAll(manualApplyBase, security.PrivateDirMode); err != nil {
		return "", fmt.Errorf("failed to create manual-apply dir: %w", err)
	}
	baseName := filepath.Base(src)
	dst := filepath.Join(manualApplyBase, baseName)
	for i := 2; ; i++ {
		if _, err := os.Stat(dst); os.IsNotExist(err) {
			break
		}
		dst = filepath.Join(manualApplyBase, fmt.Sprintf("%s-%d", baseName, i))
	}
	if err := os.Rename(src, dst); err != nil {
		return "", fmt.Errorf("failed to move docs to manual-apply dir: %w", err)
	}
	return dst, nil
}

// LogManualRequired appends an account-gated job to the actionable
// manual-apply queue — deliberately separate from LogFailedSubmission's
// failure log (improvements.md #21): these are not failures, the tailored
// documents are already saved (docsDir, from MoveToManualApply) and the
// only missing step is a human creating the ATS account and submitting.
func LogManualRequired(companyName, jobTitle, applyURL, docsDir string) error {
	logMutex.Lock()
	defer logMutex.Unlock()

	if err := os.MkdirAll(manualApplyBase, security.PrivateDirMode); err != nil {
		return fmt.Errorf("failed to create manual-apply dir: %w", err)
	}
	reportPath := filepath.Join(manualApplyBase, "manual_queue.md")

	if _, err := os.Stat(reportPath); os.IsNotExist(err) {
		header := "# Manual Apply Queue\n\nThese jobs sit behind an ATS account sign-in, so automation hands them off by design. Tailored documents are already saved in each company's folder alongside this file — create the account, upload, submit, check the box.\n\n"
		if err := writePrivateFile(reportPath, []byte(header)); err != nil {
			return fmt.Errorf("failed to initialize manual queue: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("failed to inspect manual queue: %w", err)
	}

	applyURL = NormalizeURL(applyURL)
	docsNote := "docs not found"
	if docsDir != "" {
		docsNote = fmt.Sprintf("docs in `%s/`", docsDir)
	}
	entry := fmt.Sprintf("- [ ] **%s** - %s: [Apply Here](%s) — %s\n", companyName, jobTitle, applyURL, docsNote)

	f, err := openPrivateAppendFile(reportPath)
	if err != nil {
		return fmt.Errorf("failed to open manual queue: %w", err)
	}
	defer f.Close()

	if _, err = f.WriteString(entry); err != nil {
		return fmt.Errorf("failed to write to manual queue: %w", err)
	}

	return nil
}

// LogCopilotReview appends a Copilot-mode job to the copilot review queue
// (applications/needs_manual_apply/copilot_queue.md). The form was filled
// completely by the agent and stopped before final submit.
func LogCopilotReview(companyName, jobTitle, applyURL, docsDir string) error {
	logMutex.Lock()
	defer logMutex.Unlock()

	if err := os.MkdirAll(manualApplyBase, security.PrivateDirMode); err != nil {
		return fmt.Errorf("failed to create manual-apply dir: %w", err)
	}
	reportPath := filepath.Join(manualApplyBase, "copilot_queue.md")

	if _, err := os.Stat(reportPath); os.IsNotExist(err) {
		// The header must not promise a pre-filled form. The agent fills the
		// form inside its own ephemeral Playwright context, which is closed the
		// moment it stops at the gate — nothing about that fill survives into
		// the user's browser. What genuinely carries over is the tailored
		// resume and cover letter, which are the expensive part. Saying
		// otherwise would send the user to a blank form expecting their work.
		header := "# Copilot Review Queue\n\n" +
			"The agent scored each job below, wrote tailored documents for it, and confirmed the application form was reachable and fillable — then stopped without submitting.\n\n" +
			"Open the apply link, fill the form using the documents saved alongside each entry, and submit it yourself. The agent's own fill happened in a separate automated browser session and is not carried over, so expect a blank form.\n\n" +
			"Tick a checkbox once you've submitted that application.\n\n"
		if err := writePrivateFile(reportPath, []byte(header)); err != nil {
			return fmt.Errorf("failed to initialize copilot queue: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("failed to inspect copilot queue: %w", err)
	}

	applyURL = NormalizeURL(applyURL)
	docsNote := "docs not found"
	if docsDir != "" {
		docsNote = fmt.Sprintf("docs in `%s/`", docsDir)
	}
	entry := fmt.Sprintf("- [ ] **%s** - %s: [Apply Here](%s) — %s\n", companyName, jobTitle, applyURL, docsNote)

	f, err := openPrivateAppendFile(reportPath)
	if err != nil {
		return fmt.Errorf("failed to open copilot queue: %w", err)
	}
	defer f.Close()

	if _, err = f.WriteString(entry); err != nil {
		return fmt.Errorf("failed to write to copilot queue: %w", err)
	}

	return nil
}

// PromptInjectionThreat is a storage-local mirror of promptsec.Threat, kept
// separate so this package doesn't need to import the security package
// itself (which would cycle back to pkg/storage) just to log what was found.
type PromptInjectionThreat struct {
	Type     string
	Severity float64
	Message  string
	Guard    string
	Match    string
	Start    int
	End      int
}

// ThreatsToStored converts promptsec's threat type into PromptInjectionThreat,
// the single field mapping shared by every quarantine call site that logs to
// the prompt-injection audit trail (improvements.md #505).
func ThreatsToStored(threats []promptsec.Threat) []PromptInjectionThreat {
	out := make([]PromptInjectionThreat, 0, len(threats))
	for _, t := range threats {
		out = append(out, PromptInjectionThreat{
			Type:     string(t.Type),
			Severity: t.Severity,
			Message:  t.Message,
			Guard:    t.Guard,
			Match:    t.Match,
			Start:    t.Start,
			End:      t.End,
		})
	}
	return out
}

var injectionLogMutex sync.Mutex

// LogPromptInjectionDetections appends one CSV row per detected threat to
// applications/prompt_injection_detections.csv, so a real prompt-injection
// or hidden-content attempt on a scraped career page is kept as a
// reviewable record instead of only appearing transiently in the log file.
func LogPromptInjectionDetections(url, companyName string, threats []PromptInjectionThreat) error {
	if len(threats) == 0 {
		return nil
	}

	injectionLogMutex.Lock()
	defer injectionLogMutex.Unlock()

	if err := os.MkdirAll("applications", security.PrivateDirMode); err != nil {
		return fmt.Errorf("failed to create applications directory: %w", err)
	}

	reportPath := filepath.Join("applications", "prompt_injection_detections.csv")
	writeHeader := false
	if _, err := os.Stat(reportPath); os.IsNotExist(err) {
		writeHeader = true
	}

	f, err := openPrivateAppendFile(reportPath)
	if err != nil {
		return fmt.Errorf("failed to open prompt injection report: %w", err)
	}
	defer f.Close()

	w := csv.NewWriter(f)
	if writeHeader {
		if err := w.Write([]string{"detected_at", "url", "company_name", "threat_type", "severity", "guard", "message", "matched_text", "match_start", "match_end"}); err != nil {
			return fmt.Errorf("failed to write CSV header: %w", err)
		}
	}

	now := time.Now().Format(time.RFC3339)
	for _, t := range threats {
		row := []string{
			now,
			url,
			companyName,
			t.Type,
			strconv.FormatFloat(t.Severity, 'f', 2, 64),
			t.Guard,
			t.Message,
			t.Match,
			strconv.Itoa(t.Start),
			strconv.Itoa(t.End),
		}
		if err := w.Write(row); err != nil {
			return fmt.Errorf("failed to write CSV row: %w", err)
		}
	}
	w.Flush()
	return w.Error()
}

func writePrivateFile(path string, data []byte) error {
	f, err := os.OpenFile(
		path,
		os.O_WRONLY|os.O_CREATE|os.O_TRUNC,
		security.PrivateFileMode,
	)
	if err != nil {
		return err
	}
	if err := f.Chmod(security.PrivateFileMode); err != nil {
		f.Close()
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

func atomicWritePrivateFile(path string, data []byte) error {
	temp, err := os.CreateTemp(filepath.Dir(path), ".partial-*")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	if err := temp.Chmod(security.PrivateFileMode); err != nil {
		temp.Close()
		return err
	}
	if _, err := temp.Write(data); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Sync(); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	return os.Rename(tempPath, path)
}

func openPrivateAppendFile(path string) (*os.File, error) {
	f, err := os.OpenFile(
		path,
		os.O_APPEND|os.O_WRONLY|os.O_CREATE,
		security.PrivateFileMode,
	)
	if err != nil {
		return nil, err
	}
	if err := f.Chmod(security.PrivateFileMode); err != nil {
		f.Close()
		return nil, err
	}
	return f, nil
}

func CloseDB() error {
	if db != nil {
		return db.Close()
	}
	return nil
}

// AddToFunnel inserts a newly discovered job. Callers normally pass
// "DISCOVERED" as status, so on a conflict (a URL already known from an
// earlier discovery pass, possibly from a previous session) this is a no-op:
// it must NOT reset an in-progress or already-resolved job's status back to
// "DISCOVERED", which would make it eligible for reprocessing while a worker
// is already handling it (or already finished it) - confirmed live 2026-07-21
// as the root cause of the same URL being queued and processed multiple
// times, eventually hitting the UNIQUE constraint on applied_jobs.url.
// The returned bool reports whether a genuinely new row was inserted into the
// automatic-processing queue. Excluded sources are retained as SKIPPED audit
// rows but return false, so discovery callers never send them straight to a
// one-shot batch worker.
func AddToFunnel(company, title, url, status string, discoverySources ...string) (bool, error) {
	if db == nil {
		return false, fmt.Errorf("db not initialized")
	}
	if len(discoverySources) > 1 {
		return false, fmt.Errorf("expected at most one discovery source, got %d", len(discoverySources))
	}
	discoverySource := ""
	if len(discoverySources) == 1 {
		discoverySource = strings.TrimSpace(discoverySources[0])
	}
	url = NormalizeURL(url)
	statusReason := ""
	excluded := status == "DISCOVERED" && isExcludedSourceURL(url)
	if excluded {
		status = "SKIPPED"
		statusReason = SkippedReasonExcludedSource
	}
	tx, err := db.Begin()
	if err != nil {
		return false, fmt.Errorf("begin funnel insert transaction: %w", err)
	}
	defer tx.Rollback()
	if status == "DISCOVERED" && discoverySource != "" {
		var pending int
		if err := tx.QueryRow(`SELECT COUNT(*) FROM job_funnel WHERE status = 'DISCOVERED' AND discovery_source = ?`, discoverySource).Scan(&pending); err != nil {
			return false, fmt.Errorf("count pending source rows: %w", err)
		}
		if pending >= maxPendingPerSource {
			status = "SKIPPED"
			statusReason = SkippedReasonSourceAdmissionCap
		}
	}
	result, err := tx.Exec(`INSERT INTO job_funnel (company_name, job_title, url, status, status_reason, discovery_source, discovered_at, last_updated)
		VALUES (?, ?, ?, ?, NULLIF(?, ''), NULLIF(?, ''), CURRENT_TIMESTAMP, CASE WHEN ? = '' THEN NULL ELSE ? END)
		ON CONFLICT(url) DO NOTHING`, company, title, url, status, statusReason, discoverySource, statusReason, time.Now().UTC())
	if err != nil {
		return false, err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	if rowsAffected > 0 {
		domain := attemptDomain(url, "")
		if domain != "" {
			if _, err := tx.Exec(`
				INSERT INTO career_sites (domain, ats_provider, last_scanned)
				VALUES (?, ?, ?)
				ON CONFLICT(domain) DO UPDATE SET last_scanned = excluded.last_scanned`, domain, domain, time.Now().UTC()); err != nil {
				return false, fmt.Errorf("record discovered career site: %w", err)
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("commit funnel insert transaction: %w", err)
	}
	return rowsAffected > 0 && status == "DISCOVERED", nil
}

// GetDiscoverySource returns the channel that inserted a funnel row. The
// boolean is false for legacy rows, whose channel was not stored and must not
// be inferred from their destination hostname.
func GetDiscoverySource(rawURL string) (string, bool, error) {
	if db == nil {
		return "", false, fmt.Errorf("db not initialized")
	}
	var source sql.NullString
	err := db.QueryRow("SELECT discovery_source FROM job_funnel WHERE url = ?", NormalizeURL(rawURL)).Scan(&source)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return source.String, source.Valid, nil
}

// excludedLeverBoards names Lever board slugs whose postings are never worth
// admitting. jobgether is an aggregator, not an employer: it republishes the
// same role across many countries (the identical "AI Automation Engineer"
// posting exists for India, the US, Portugal, and Brazil) and carries ~2,988
// live postings. Across 1,642 rows it produced 792 prompt-injection
// quarantines, 589 invalid URLs, 54 failed submits, and zero applications,
// and its Lever apply form fails with "There was an error verifying your
// application" for two different reqs — reproduced in an ordinary browser
// outside this agent entirely, so the flow is broken on their side. Same
// treatment breezy.hr got, on stronger evidence.
var excludedLeverBoards = map[string]struct{}{
	"jobgether": {},
}

func isExcludedSourceURL(rawURL string) bool {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	host := strings.TrimSuffix(strings.ToLower(parsed.Hostname()), ".")
	if host == "breezy.hr" || strings.HasSuffix(host, ".breezy.hr") {
		return true
	}
	if host == "lever.co" || strings.HasSuffix(host, ".lever.co") {
		for _, segment := range strings.Split(strings.ToLower(parsed.Path), "/") {
			if _, excluded := excludedLeverBoards[segment]; excluded {
				return true
			}
		}
	}
	return false
}

// SkipExcludedSourceDiscoveredJobs terminalizes legacy rows that were written
// before AddToFunnel applied the excluded-source policy. It is idempotent and
// only touches still-DISCOVERED Breezy postings, never a row that has entered
// another outcome path.
func SkipExcludedSourceDiscoveredJobs() (int64, error) {
	if db == nil {
		return 0, fmt.Errorf("db not initialized")
	}
	result, err := db.Exec(`UPDATE job_funnel
		SET status = 'SKIPPED', status_reason = ?, last_updated = ?
		WHERE status = 'DISCOVERED'
		  AND (lower(url) LIKE '%breezy.hr%' OR lower(url) LIKE '%lever.co/jobgether/%')`,
		SkippedReasonExcludedSource, time.Now().UTC())
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// SweepStaleDiscoveredJobs performs the periodic first-attempt lifecycle
// maintenance. It first terminalizes legacy excluded-source rows, then marks
// only rows that have remained unattempted for 30 days as SKIPPED. Recent and
// SLA-priority rows remain eligible; this function never changes fit ranking.
func SweepStaleDiscoveredJobs(now time.Time) (int64, error) {
	if db == nil {
		return 0, fmt.Errorf("db not initialized")
	}
	excluded, err := SkipExcludedSourceDiscoveredJobs()
	if err != nil {
		return 0, err
	}
	result, err := db.Exec(`UPDATE job_funnel
		SET status = 'SKIPPED', status_reason = ?, last_updated = ?
		WHERE status = 'DISCOVERED' AND discovered_at IS NOT NULL AND discovered_at <= ?`,
		SkippedReasonFirstAttemptExpired, now.UTC(), now.UTC().Add(-firstAttemptExpiryAge))
	if err != nil {
		return 0, fmt.Errorf("expire stale discovered rows: %w", err)
	}
	expired, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}
	return excluded + expired, nil
}

func UpdateFunnelStatus(url, status string) error {
	return UpdateFunnelStatusWithReason(url, status, "")
}

// UpdateFunnelStatusWithReason changes the current funnel state and records a
// normalized, non-sensitive reason for the stage ledger. Callers must pass a
// stable code rather than a raw error because errors can contain page content.
func UpdateFunnelStatusWithReason(url, status, reasonCode string) error {
	if db == nil {
		return fmt.Errorf("db not initialized")
	}
	url = NormalizeURL(url)
	// Store as canonical UTC (.UTC()), not a local-offset time.Time. This
	// column is compared with a plain SQL ORDER BY, which does a TEXT
	// comparison, not a real chronological one - confirmed live 2026-07-21:
	// an earlier build briefly wrote this column via SQLite's
	// CURRENT_TIMESTAMP (always UTC, e.g. "2026-07-22T01:48:26Z" after a
	// UTC date rollover past midnight), then a later build wrote it as a
	// local-offset string (e.g. "2026-07-21T21:50:47-04:00"). Mixing the two
	// formats broke ORDER BY last_updated DESC: the UTC string's rolled-over
	// date sorted as "later" even though it was actually the older row,
	// making the dashboard's "currently processing" card show a stuck job
	// from ~20 minutes earlier as if it were the current one. Storing
	// everything as UTC keeps every row's string directly comparable;
	// convert to local time only when formatting for display.
	now := time.Now().UTC()
	_, err := db.Exec(`UPDATE job_funnel
		SET status = ?, status_reason = NULLIF(?, ''), last_updated = ?,
			applied_at = CASE WHEN ? = 'APPLIED' AND applied_at IS NULL THEN ? ELSE applied_at END
		WHERE url = ?`, status, reasonCode, now, status, now, url)
	return err
}

// InvalidURLReasonMalformed and InvalidURLReasonExpired classify why a row
// was marked INVALID_URL (improvements.md #468). The row's own live-database
// measurement found the two causes are not a small edge case: of a 1,214-row
// INVALID_URL sample, ~88% were InvalidURLReasonExpired (a real posting
// checkJobAlive/fetchJobPage correctly caught as dead or gone by the time the
// agent fetched it -- ordinary job-market churn) and ~12% were
// InvalidURLReasonMalformed (a known-junk URL pattern or unsafe network
// target that was never a real posting). Collapsing both into one status
// with one caption misdescribed the majority of the bucket.
const (
	InvalidURLReasonMalformed = "malformed"
	InvalidURLReasonExpired   = "expired"
)

// UpdateFunnelStatusInvalid marks url INVALID_URL and records why, using the
// InvalidURLReason* constants above. See their doc comment for the split
// this backs.
func UpdateFunnelStatusInvalid(url, reason string) error {
	if db == nil {
		return fmt.Errorf("db not initialized")
	}
	url = NormalizeURL(url)
	_, err := db.Exec(
		"UPDATE job_funnel SET status = 'INVALID_URL', status_reason = ?, last_updated = ? WHERE url = ?",
		reason, time.Now().UTC(), url)
	return err
}

func UpdateFunnelStatusWithScore(url, status string, fitScore int) error {
	if db == nil {
		return fmt.Errorf("db not initialized")
	}
	url = NormalizeURL(url)
	_, err := db.Exec("UPDATE job_funnel SET status = ?, status_reason = NULL, fit_score = ?, last_updated = ? WHERE url = ?", status, fitScore, time.Now().UTC(), url)
	return err
}

func UpdateFunnelStatusWithReasonAndScore(url, status, reason string, fitScore int) error {
	if db == nil {
		return fmt.Errorf("db not initialized")
	}
	url = NormalizeURL(url)
	_, err := db.Exec("UPDATE job_funnel SET status = ?, status_reason = ?, fit_score = ?, last_updated = ? WHERE url = ?", status, reason, fitScore, time.Now().UTC(), url)
	return err
}

// MaxRetryAttempts caps how many times a retryable failure may return a
// job_funnel row to DISCOVERED before UpdateFunnelStatusRetryable moves it
// to the terminal RETRY_EXHAUSTED status (bugs.md #466).
const MaxRetryAttempts = 5

// RetryBackoffBase is the first retry's delay; each subsequent retry doubles
// it (2, 4, 8, 16 minutes for retry_count 1-4) so a chronically failing row
// competes less and less for worker time before it is exhausted.
const RetryBackoffBase = 2 * time.Minute

// UpdateFunnelStatusRetryable records a retryable failure and its reason for
// url. Bugs.md
// #466: the live daemon's continuous scheduler re-selects every DISCOVERED
// row with no attempt count or cooldown, so a transient failure that simply
// reset status back to DISCOVERED was retried immediately on the very next
// cycle and could monopolize the worker while newer rows waited indefinitely
// behind it in a 10k-row backlog.
//
// This increments the row's retry_count. While under MaxRetryAttempts, the
// row goes back to DISCOVERED with next_eligible_at pushed out by an
// exponential backoff, so GetDiscoveredJobs will not reselect it until the
// delay elapses. Once retry_count reaches MaxRetryAttempts, the row moves to
// RETRY_EXHAUSTED instead — a terminal status distinct from DISCOVERED so a
// chronically failing job stops competing with the rest of the queue and
// (per mergeStatusRank's own warning comment) has an explicit rank so
// URL-scheme dedup cannot silently resurrect it. The row remains queryable
// by status for manual investigation (e.g. `cmd/requeue`, which resets
// retry_count/next_eligible_at on a deliberate requeue); the dashboard UI
// surfaces RETRY_EXHAUSTED on its own card as of improvements.md #468. The
// terminal row retains the final retryable reason so failures can be grouped
// from the database without reconstructing them from logs (bugs.md #480).
func UpdateFunnelStatusRetryable(url, reason string) error {
	if db == nil {
		return fmt.Errorf("db not initialized")
	}
	// A focused legacy migration can invoke this writer after adding retry
	// fields but before the later status-reason migration. Make the writer
	// safe in that order too; normal initialization makes this a no-op.
	if err := migrateJobFunnelStatusReason(); err != nil {
		return fmt.Errorf("ensure status_reason migration: %w", err)
	}
	url = NormalizeURL(url)

	var retryCount int
	if err := db.QueryRow("SELECT retry_count FROM job_funnel WHERE url = ?", url).Scan(&retryCount); err != nil {
		return fmt.Errorf("failed to read retry_count for %s: %w", url, err)
	}
	retryCount++
	now := time.Now().UTC()

	if retryCount >= MaxRetryAttempts {
		_, err := db.Exec(
			"UPDATE job_funnel SET status = 'RETRY_EXHAUSTED', status_reason = ?, retry_count = ?, last_updated = ? WHERE url = ?",
			reason, retryCount, now, url)
		return err
	}

	backoff := RetryBackoffBase * time.Duration(1<<uint(retryCount-1))
	nextEligible := now.Add(backoff)
	_, err := db.Exec(
		"UPDATE job_funnel SET status = 'DISCOVERED', status_reason = ?, retry_count = ?, next_eligible_at = ?, last_updated = ? WHERE url = ?",
		reason, retryCount, nextEligible, now, url)
	return err
}

// DeferFunnelStatus returns url to DISCOVERED with next_eligible_at pushed
// out by cooldown, without touching retry_count. This is deliberately
// distinct from UpdateFunnelStatusRetryable: a caller that skipped an
// attempt entirely (e.g. improvements.md #469's per-domain circuit breaker
// declining to even try a domain it already knows is failing) has not
// observed a new failure for this specific job, so charging it against
// MaxRetryAttempts would let a busy domain exhaust jobs that were never
// actually attempted -- the same starvation shape bugs.md #466 fixed, one
// layer up. Rows deferred this way keep whatever retry_count they already
// had and can still reach RETRY_EXHAUSTED, but only from genuine fetch
// failures.
func DeferFunnelStatus(url string, cooldown time.Duration) error {
	if db == nil {
		return fmt.Errorf("db not initialized")
	}
	url = NormalizeURL(url)
	now := time.Now().UTC()
	_, err := db.Exec(
		"UPDATE job_funnel SET status = 'DISCOVERED', status_reason = NULL, next_eligible_at = ?, last_updated = ? WHERE url = ?",
		now.Add(cooldown), now, url)
	return err
}

func SaveFormMapping(domain, mappingJson string) error {
	if db == nil {
		return fmt.Errorf("db not initialized")
	}
	// LLM mappers sometimes return prose instead of JSON; caching that
	// guarantees a parse failure on every future visit to the domain
	// (confirmed live 2026-07-22: cached mapping for www.workday.com/en-us
	// began with "T", failing every reuse until invalidated).
	if !json.Valid([]byte(mappingJson)) {
		return fmt.Errorf("refusing to cache non-JSON form mapping for %s", domain)
	}
	// bugs.md #68: valid JSON is not the same as a usable mapping. A response
	// with every selector null parses cleanly and was cached happily, so each
	// later visit to the domain loaded it, failed every fill, invalidated the
	// cache, and burned a fresh Learner Module call to produce the same
	// nulls again. Found live: 7 of 60 cached mappings were in this state,
	// including smartrecruiters.com, pinpointhq.com and applytojob.com.
	if !hasUsableSelector(mappingJson) {
		return fmt.Errorf("refusing to cache form mapping for %s: no usable selectors", domain)
	}
	_, err := db.Exec("INSERT INTO form_mappings (domain, mapping_json, created_at) VALUES (?, ?, ?) ON CONFLICT(domain) DO UPDATE SET mapping_json=excluded.mapping_json", domain, mappingJson, time.Now())
	return err
}

func GetFormMapping(domain string) (string, error) {
	if db == nil {
		return "", fmt.Errorf("db not initialized")
	}
	var mappingJson string
	err := db.QueryRow("SELECT mapping_json FROM form_mappings WHERE domain = ?", domain).Scan(&mappingJson)
	return mappingJson, err
}

const formMappingFreshnessWindow = 30 * 24 * time.Hour

// PreferCachedFormMapping returns false only when the registry has stronger
// evidence that a cached mapping is failing than that it succeeds, or its
// last success is stale. Unknown mappings remain eligible so a newly learned
// mapping gets its first chance; this is a fallback decision, never a
// permanent site blacklist.
func PreferCachedFormMapping(domain string) (bool, error) {
	if db == nil {
		return false, fmt.Errorf("db not initialized")
	}
	var successes, failures int
	var lastValidated sql.NullTime
	err := db.QueryRow("SELECT success_count, failure_count, last_validated_at FROM form_mappings WHERE domain = ?", domain).Scan(&successes, &failures, &lastValidated)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if successes == 0 {
		return successes >= failures, nil
	}
	return successes >= failures && lastValidated.Valid && lastValidated.Time.After(time.Now().Add(-formMappingFreshnessWindow)), nil
}

func DeleteFormMapping(domain string) error {
	if db == nil {
		return fmt.Errorf("db not initialized")
	}
	_, err := db.Exec("DELETE FROM form_mappings WHERE domain = ?", domain)
	return err
}

func LogExecution(jobID, url, status string, tokens int) error {
	if db == nil {
		return fmt.Errorf("db not initialized")
	}
	url = NormalizeURL(url)
	_, err := db.Exec("INSERT INTO execution_logs (job_id, url, tokens_used, status, logged_at) VALUES (?, ?, ?, ?, ?)", jobID, url, tokens, status, time.Now())
	return err
}

type FunnelJob struct {
	CompanyName      string
	JobTitle         string
	URL              string
	FitSimilarity    float64
	DiscoveredAt     time.Time
	RankingScore     float64
	RankingReason    string
	IsExploration    bool
	StatusReason     string
	ProcessingIntent string
}

func (j *FunnelJob) GetURL() string             { return j.URL }
func (j *FunnelJob) GetFitSimilarity() float64  { return j.FitSimilarity }
func (j *FunnelJob) GetDiscoveredAt() time.Time { return j.DiscoveredAt }
func (j *FunnelJob) SetRankData(score float64, reason string, isExploration bool) {
	j.RankingScore = score
	j.RankingReason = reason
	j.IsExploration = isExploration
}

// The fixed sourcePriorityCASE ordering has been replaced by Bayesian smoothed
// outcome ranking (improvements.md #35). See GetDiscoveredJobs and ranking.go.

// ReapStaleProcessingJobs resets every job_funnel row stuck in PROCESSING
// back to DISCOVERED. Confirmed live 2026-07-24: 235 rows accumulated
// since 2026-07-21, one for every time a run got killed (kill -9, the only
// reliable way documented in bugs.md's Operational Trap notes) while a job
// was mid-flight. GetDiscoveredJobs only ever pulls status='DISCOVERED', so
// none of these could ever be retried again -- and the dashboard's raw
// PROCESSING count made it look like hundreds of jobs were actively being
// worked, when a single-worker run can only ever have one truly in flight.
// Callers should invoke this once, right after InitDB and before any job
// gets marked PROCESSING by the caller's own run -- a freshly-started
// process cannot have produced any of the rows it would reset, regardless
// of worker count, since it hasn't processed anything yet.
func ReapStaleProcessingJobs() (int64, error) {
	if db == nil {
		return 0, fmt.Errorf("db not initialized")
	}
	result, err := db.Exec(`UPDATE job_funnel SET status = 'DISCOVERED' WHERE status = 'PROCESSING'`)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// GetDiscoveredJobs fetches all discovered jobs and orders the queue using
// Bayesian smoothed application outcomes, fit similarity, and freshness
// (improvements.md #35), with jobs at or past urgentAgeDays forced to the
// front regardless of score so a growing backlog cannot starve them past
// expiry (bugs.md #481).
func GetDiscoveredJobs() ([]FunnelJob, error) {
	if db == nil {
		return nil, fmt.Errorf("db not initialized")
	}
	// breezy.hr excluded entirely (0 APPLIED / 48 FAILED_SUBMIT, worst-performing source).
	// next_eligible_at IS NULL covers every row that has never been retried
	// (the overwhelming majority); rows UpdateFunnelStatusRetryable has
	// backed off are excluded until their delay elapses (bugs.md #466).
	rows, err := db.Query(`SELECT company_name, job_title, url, COALESCE(fit_similarity, -1), discovered_at, status_reason FROM job_funnel
		WHERE status = 'DISCOVERED' AND url NOT LIKE '%breezy.hr%'
		AND (next_eligible_at IS NULL OR next_eligible_at <= ?)`, time.Now().UTC())
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var jobs []FunnelJob
	for rows.Next() {
		var j FunnelJob
		var discoveredAt sql.NullTime
		var statusReason sql.NullString
		if err := rows.Scan(&j.CompanyName, &j.JobTitle, &j.URL, &j.FitSimilarity, &discoveredAt, &statusReason); err != nil {
			log.Printf("[Storage] Error scanning discovered job row: %v", err)
			continue
		}
		if statusReason.Valid {
			j.StatusReason = statusReason.String
		}
		if discoveredAt.Valid {
			j.DiscoveredAt = discoveredAt.Time
		} else {
			j.DiscoveredAt = time.Now()
		}
		jobs = append(jobs, j)
	}

	summaries, err := GetSourceHealthSummaries(30)
	if err != nil {
		log.Printf("[Storage] Error getting source health: %v", err)
	}

	var ptrs []*FunnelJob
	for i := range jobs {
		ptrs = append(ptrs, &jobs[i])
	}

	// Apply ranking with 20% exploration share
	ptrs = RankJobs(ptrs, summaries, 0.20)

	var result []FunnelJob
	for _, p := range ptrs {
		result = append(result, *p)
	}

	return result, nil
}

// GetJobsMissingFitSimilarity returns DISCOVERED job_funnel rows whose
// fit_similarity has not yet been backfilled (improvements.md #22), oldest
// first, capped at limit (0 = unlimited). Used by cmd/rankjobs so repeated
// runs make forward progress through the backlog instead of re-scoring the
// same rows.
func GetJobsMissingFitSimilarity(limit int) ([]FunnelJob, error) {
	if db == nil {
		return nil, fmt.Errorf("db not initialized")
	}
	query := `SELECT company_name, job_title, url FROM job_funnel
		WHERE status = 'DISCOVERED' AND fit_similarity IS NULL AND url NOT LIKE '%breezy.hr%'
		ORDER BY id`
	if limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", limit)
	}
	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var jobs []FunnelJob
	for rows.Next() {
		var j FunnelJob
		if err := rows.Scan(&j.CompanyName, &j.JobTitle, &j.URL); err != nil {
			log.Printf("[Storage] Error scanning fit-similarity-missing job row: %v", err)
			continue
		}
		jobs = append(jobs, j)
	}
	return jobs, nil
}

// UpdateFitSimilarity stores a job's resume-similarity score (improvements.md
// #22), computed by cmd/rankjobs from an embedding of the job's title/company
// against the resume's career_chunks. Deliberately does not touch
// last_updated — this is a background ranking signal, not a funnel status
// transition, and mixing it into the same timestamp would make the
// dashboard's "time to process" cards misread a re-ranking as new activity.
func UpdateFitSimilarity(url string, score float32) error {
	if db == nil {
		return fmt.Errorf("db not initialized")
	}
	url = NormalizeURL(url)
	_, err := db.Exec(`UPDATE job_funnel SET fit_similarity = ? WHERE url = ?`, score, url)
	return err
}

// SourceOutcomeStat is one row of the per-URL-pattern outcome breakdown
// cmd/requeue reports, mirroring the ad hoc query used to find bugs #45/#46
// (2026-07-23): grouping job_funnel by outcome status per platform is what
// actually revealed those CAPTCHA false positives, rather than guesswork.
type SourceOutcomeStat struct {
	Total          int
	Applied        int
	Captcha        int
	Failed         int
	Manual         int
	RetryExhausted int
}

// SourceOutcomeBreakdown reports outcome counts for job_funnel rows whose
// URL matches urlPattern (a SQL LIKE pattern, e.g. "%lever.co%").
func SourceOutcomeBreakdown(urlPattern string) (SourceOutcomeStat, error) {
	var s SourceOutcomeStat
	if db == nil {
		return s, fmt.Errorf("db not initialized")
	}
	err := db.QueryRow(`SELECT
		COUNT(*),
		COALESCE(SUM(CASE WHEN status = 'APPLIED' THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN status = 'BLOCKED_CAPTCHA' THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN status = 'FAILED_SUBMIT' THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN status IN ('MANUAL_REQUIRED', 'AWAITING_REVIEW') THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN status = 'RETRY_EXHAUSTED' THEN 1 ELSE 0 END), 0)
		FROM job_funnel
		WHERE url LIKE ? AND status IN ('APPLIED','BLOCKED_CAPTCHA','FAILED_SUBMIT','MANUAL_REQUIRED','AWAITING_REVIEW','PROCESSED_MANUAL','RETRY_EXHAUSTED')`,
		urlPattern).Scan(&s.Total, &s.Applied, &s.Captcha, &s.Failed, &s.Manual, &s.RetryExhausted)
	return s, err
}

// atsSourceCASE labels a job_funnel row by ATS platform for conversion
// reporting. Deliberately a separate expression from sourcePriorityCASE
// above (that one ranks by success-likelihood tier; this one just names the
// platform) — reusing it here would conflate two unrelated concerns.
const atsSourceCASE = `CASE
		WHEN url LIKE '%greenhouse%' THEN 'Greenhouse'
		WHEN url LIKE '%lever.co%' THEN 'Lever'
		WHEN url LIKE '%myworkdayjobs.com%' THEN 'Workday'
		WHEN url LIKE '%smartrecruiters%' THEN 'SmartRecruiters'
		WHEN url LIKE '%ashbyhq%' THEN 'Ashby'
		ELSE 'Other'
	END`

// ConversionStats is the interview-conversion breakdown for job_funnel rows
// that were ever actually applied to. pkg/tracker/imap.go only ever moves a
// row from APPLIED to REJECTED or INTERVIEW_REQUESTED (never a distinct
// OFFER status), so "ever applied" = status IN ('APPLIED','REJECTED',
// 'INTERVIEW_REQUESTED') and Pending means still APPLIED with no email
// response detected yet.
type ConversionStats struct {
	TotalApplied  int
	Interviews    int
	Rejections    int
	Pending       int
	InterviewRate float64 // Interviews / TotalApplied, 0-1 range; 0 if TotalApplied == 0
}

// GetConversionStats reports the overall interview-conversion rate across
// every tracked application (improvements.md #15).
func GetConversionStats() (ConversionStats, error) {
	var s ConversionStats
	if db == nil {
		return s, fmt.Errorf("db not initialized")
	}
	err := db.QueryRow(`SELECT
		COUNT(*),
		COALESCE(SUM(CASE WHEN status = 'INTERVIEW_REQUESTED' THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN status = 'REJECTED' THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN status = 'APPLIED' THEN 1 ELSE 0 END), 0)
		FROM job_funnel
		WHERE status IN ('APPLIED','REJECTED','INTERVIEW_REQUESTED')`).
		Scan(&s.TotalApplied, &s.Interviews, &s.Rejections, &s.Pending)
	if err != nil {
		return s, err
	}
	if s.TotalApplied > 0 {
		s.InterviewRate = float64(s.Interviews) / float64(s.TotalApplied)
	}
	return s, nil
}

// SourceConversionStat is one ATS platform's slice of GetConversionStats.
type SourceConversionStat struct {
	Source string
	ConversionStats
}

// GetConversionStatsBySource reports GetConversionStats grouped by ATS
// platform (atsSourceCASE), ordered by TotalApplied descending. Platforms
// with zero tracked applications are omitted — most sources never appear
// here since job_funnel.company_name only gets tracked by pkg/tracker for
// rows that actually reached APPLIED.
func GetConversionStatsBySource() ([]SourceConversionStat, error) {
	if db == nil {
		return nil, fmt.Errorf("db not initialized")
	}
	rows, err := db.Query(`SELECT
		` + atsSourceCASE + ` AS source,
		COUNT(*),
		COALESCE(SUM(CASE WHEN status = 'INTERVIEW_REQUESTED' THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN status = 'REJECTED' THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN status = 'APPLIED' THEN 1 ELSE 0 END), 0)
		FROM job_funnel
		WHERE status IN ('APPLIED','REJECTED','INTERVIEW_REQUESTED')
		GROUP BY source
		HAVING COUNT(*) > 0
		ORDER BY COUNT(*) DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stats []SourceConversionStat
	for rows.Next() {
		var s SourceConversionStat
		if err := rows.Scan(&s.Source, &s.TotalApplied, &s.Interviews, &s.Rejections, &s.Pending); err != nil {
			return nil, err
		}
		if s.TotalApplied > 0 {
			s.InterviewRate = float64(s.Interviews) / float64(s.TotalApplied)
		}
		stats = append(stats, s)
	}
	return stats, rows.Err()
}

// UpdateToneVariant records which cover-letter tone variant (improvements.md
// #13) was actually used for a job, so its eventual outcome can be joined
// back against the variant that produced it via GetConversionStatsByVariant.
// Deliberately does not touch last_updated, same reasoning as
// UpdateFitSimilarity — this is metadata about how the application was
// generated, not a funnel status transition.
func UpdateToneVariant(url, variant string) error {
	if db == nil {
		return fmt.Errorf("db not initialized")
	}
	url = NormalizeURL(url)
	_, err := db.Exec(`UPDATE job_funnel SET tone_variant = ? WHERE url = ?`, variant, url)
	return err
}

// VariantConversionStat is one cover-letter tone variant's slice of
// GetConversionStats (improvements.md #13).
type VariantConversionStat struct {
	Variant string
	ConversionStats
}

// GetConversionStatsByVariant reports GetConversionStats grouped by
// tone_variant, ordered by TotalApplied descending. Rows with no recorded
// variant (tone A/B testing not configured, or applied before this feature
// existed) are grouped under "unspecified" rather than silently dropped, so
// this total always reconciles with GetConversionStats's TotalApplied.
func GetConversionStatsByVariant() ([]VariantConversionStat, error) {
	if db == nil {
		return nil, fmt.Errorf("db not initialized")
	}
	rows, err := db.Query(`SELECT
		COALESCE(NULLIF(tone_variant, ''), 'unspecified') AS variant,
		COUNT(*),
		COALESCE(SUM(CASE WHEN status = 'INTERVIEW_REQUESTED' THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN status = 'REJECTED' THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN status = 'APPLIED' THEN 1 ELSE 0 END), 0)
		FROM job_funnel
		WHERE status IN ('APPLIED','REJECTED','INTERVIEW_REQUESTED')
		GROUP BY variant
		HAVING COUNT(*) > 0
		ORDER BY COUNT(*) DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stats []VariantConversionStat
	for rows.Next() {
		var s VariantConversionStat
		if err := rows.Scan(&s.Variant, &s.TotalApplied, &s.Interviews, &s.Rejections, &s.Pending); err != nil {
			return nil, err
		}
		if s.TotalApplied > 0 {
			s.InterviewRate = float64(s.Interviews) / float64(s.TotalApplied)
		}
		stats = append(stats, s)
	}
	return stats, rows.Err()
}

// RequeueByURLPattern resets job_funnel rows matching urlPattern and
// currently in fromStatus back to DISCOVERED, so a fix that makes them
// newly fillable actually gets a retry — GetDiscoveredJobs only ever pulls
// status='DISCOVERED', so without this a fixed bug's backlog sits idle
// forever (confirmed live 2026-07-23: this exact gap kept bugs #45/#46's
// fix from producing a fresh APPLIED until 830 stale BLOCKED_CAPTCHA rows
// were manually reset). Returns the number of rows changed.
//
// Also resets retry_count and next_eligible_at (bugs.md #466). Without
// this, requeuing a row that had already accumulated retries under its old
// status (including RETRY_EXHAUSTED itself, which an operator can requeue
// like any other terminal status) would inherit a stale, already-high
// retry_count: its very next transient failure could push it straight back
// past MaxRetryAttempts and re-exhaust it after a single attempt, instead
// of giving it the fresh retry budget a deliberate manual requeue implies.
func RequeueByURLPattern(urlPattern, fromStatus string) (int64, error) {
	if db == nil {
		return 0, fmt.Errorf("db not initialized")
	}
	result, err := db.Exec(`UPDATE job_funnel SET status = 'DISCOVERED', retry_count = 0, next_eligible_at = NULL WHERE status = ? AND url LIKE ?`, fromStatus, urlPattern)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// ClearApplicationRecordsByURLPattern deletes applied_jobs rows matching
// urlPattern. HasApplied checks applied_jobs, which gets a row as soon as
// tailored documents are saved — before the actual submit attempt runs —
// so a job that generated real documents but then failed to submit (e.g.
// FAILED_SUBMIT, not BLOCKED_CAPTCHA) will be skipped as "already applied"
// on a requeue unless its dedup record is cleared too (confirmed live
// 2026-07-23 re-testing the Lever "smarsh" posting). Not needed when
// requeuing BLOCKED_CAPTCHA rows, since both known CAPTCHA checks run
// before document generation. Returns the number of rows deleted.
func ClearApplicationRecordsByURLPattern(urlPattern string) (int64, error) {
	if db == nil {
		return 0, fmt.Errorf("db not initialized")
	}
	result, err := db.Exec(`DELETE FROM applied_jobs WHERE url LIKE ?`, urlPattern)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

type CareerChunk struct {
	ID        int
	Text      string
	Embedding []float32
}

func SaveCareerChunk(chunkText string, embedding []float32) error {
	if db == nil {
		return fmt.Errorf("db not initialized")
	}
	embeddingJSON, err := json.Marshal(embedding)
	if err != nil {
		return err
	}
	// Upsert based on text matching is hard without unique constraint, so we just insert
	// In reality we should clear table on re-ingest or use a hash. We will just insert for now.
	_, err = db.Exec("INSERT INTO career_chunks (chunk_text, embedding_json) VALUES (?, ?)", chunkText, string(embeddingJSON))
	return err
}

func ClearCareerChunks() error {
	if db == nil {
		return fmt.Errorf("db not initialized")
	}
	_, err := db.Exec("DELETE FROM career_chunks")
	return err
}

func GetAllCareerChunks() ([]CareerChunk, error) {
	if db == nil {
		return nil, fmt.Errorf("db not initialized")
	}
	rows, err := db.Query("SELECT id, chunk_text, embedding_json FROM career_chunks")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var chunks []CareerChunk
	for rows.Next() {
		var c CareerChunk
		var embStr string
		if err := rows.Scan(&c.ID, &c.Text, &embStr); err != nil {
			log.Printf("[Storage] Error scanning career chunk row: %v", err)
			continue
		}
		if err := json.Unmarshal([]byte(embStr), &c.Embedding); err != nil {
			log.Printf("[Storage] Error unmarshaling career chunk embedding: %v", err)
			continue
		}
		chunks = append(chunks, c)
	}
	return chunks, nil
}

// hasUsableSelector reports whether a form mapping contains at least one
// non-empty selector. The LLM mapper sometimes returns the right shape with
// every value null -- syntactically valid, semantically worthless -- and
// caching that costs a full Learner Module call on every subsequent visit to
// the domain for no possible benefit (bugs.md #68).
// Accepts both shapes seen in practice: the current nested form
// ({"fields": {...}}) that ExtractFormMapping produces, and a flat top-level
// map of field->selector. Being tolerant here matters because the guard's job
// is to reject *worthless* mappings, and mistaking an unfamiliar-but-usable
// shape for a worthless one would throw away good work.
func hasUsableSelector(mappingJson string) bool {
	var raw map[string]interface{}
	if err := json.Unmarshal([]byte(mappingJson), &raw); err != nil {
		return false
	}
	if fields, ok := raw["fields"].(map[string]interface{}); ok {
		return anyNonEmptyString(fields)
	}
	return anyNonEmptyString(raw)
}

func anyNonEmptyString(m map[string]interface{}) bool {
	for _, v := range m {
		if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
			return true
		}
	}
	return false
}

// GetKnownATSCompanies returns distinct company slugs for boards on the given
// ATS host, extracted from URLs the funnel has already collected. Ordered by
// how many postings each company has contributed, so a bounded pass spends its
// budget on employers this profile actually matches rather than one-offs.
//
// improvements.md #26: the slug is the path segment immediately after the ATS
// host, which is how both Greenhouse (job-boards.greenhouse.io/<slug>/jobs/id)
// and Lever (jobs.lever.co/<slug>/uuid) address a company board.
func GetKnownATSCompanies(atsHost string, limit int) ([]string, error) {
	if db == nil {
		return nil, fmt.Errorf("db not initialized")
	}
	rows, err := db.Query(`
		SELECT slug, COUNT(*) AS n FROM (
			SELECT substr(
				substr(url, instr(url, ?) + length(?)),
				1,
				instr(substr(url, instr(url, ?) + length(?)) || '/', '/') - 1
			) AS slug
			FROM job_funnel
			WHERE url LIKE '%' || ? || '%'
		)
		WHERE slug != '' AND slug NOT LIKE '%.%' AND slug NOT LIKE '%?%'
		GROUP BY slug ORDER BY n DESC LIMIT ?`,
		atsHost+"/", atsHost+"/", atsHost+"/", atsHost+"/", atsHost+"/", limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []string
	for rows.Next() {
		var slug string
		var n int
		if err := rows.Scan(&slug, &n); err != nil {
			return nil, err
		}
		out = append(out, slug)
	}
	return out, rows.Err()
}
