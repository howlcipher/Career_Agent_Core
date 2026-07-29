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

	"github.com/howlcipher/Career_Agent_Core/pkg/security"
	_ "modernc.org/sqlite"
)

var db *sql.DB

func InitDB() error {
	return InitDBWithPath("./applications.db")
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
	dsn := path
	if !strings.Contains(path, "?") {
		dsn += "?_journal_mode=WAL&_busy_timeout=5000"
	}
	db, err = sql.Open("sqlite", dsn)
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
		last_scanned DATETIME
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
		tone_variant TEXT
	);
	CREATE TABLE IF NOT EXISTS form_mappings (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		domain TEXT UNIQUE,
		mapping_json TEXT,
		created_at DATETIME
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
	CREATE TABLE IF NOT EXISTS application_attempts (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		source TEXT,
		url TEXT,
		terminal_class TEXT,
		started_at DATETIME,
		ended_at DATETIME,
		model_call_count INTEGER,
		inference_ms INTEGER
	);`
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

func mergeStatuses(s1, s2 string) string {
	if s1 == s2 {
		return s1
	}
	rank := func(s string) int {
		switch s {
		case "DISCOVERED": return 1
		case "PROCESSING": return 2
		case "SKIPPED": return 3
		case "BLOCKED_CAPTCHA": return 4
		case "FAILED_SUBMIT": return 5
		case "MANUAL_REQUIRED": return 6
		case "APPLIED": return 7
		case "REJECTED": return 8
		case "INTERVIEW_REQUESTED": return 9
		default: return 0
		}
	}

	r1, r2 := rank(s1), rank(s2)

	isSuccess1 := s1 == "APPLIED" || s1 == "REJECTED" || s1 == "INTERVIEW_REQUESTED"
	isSuccess2 := s2 == "APPLIED" || s2 == "REJECTED" || s2 == "INTERVIEW_REQUESTED"
	isFailure1 := s1 == "FAILED_SUBMIT" || s1 == "BLOCKED_CAPTCHA" || s1 == "MANUAL_REQUIRED"
	isFailure2 := s2 == "FAILED_SUBMIT" || s2 == "BLOCKED_CAPTCHA" || s2 == "MANUAL_REQUIRED"

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

	for _, r := range toProcess {
		httpsURL := "https://" + strings.TrimPrefix(r.url, "http://")
		var httpsID int
		var httpsStatus string
		err := db.QueryRow(`SELECT id, status FROM job_funnel WHERE url = ?`, httpsURL).Scan(&httpsID, &httpsStatus)
		if err == sql.ErrNoRows {
			_, err = db.Exec(`UPDATE job_funnel SET url = ? WHERE id = ?`, httpsURL, r.id)
			if err != nil {
				return fmt.Errorf("failed to update job_funnel url to https: %w", err)
			}
			continue
		} else if err != nil {
			return fmt.Errorf("failed to check https url in job_funnel: %w", err)
		}

		newStatus := mergeStatuses(r.status, httpsStatus)
		
		tx, err := db.Begin()
		if err != nil {
			return err
		}
		
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
		
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	
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
	
	for _, r := range appToProcess {
		httpsURL := "https://" + strings.TrimPrefix(r.url, "http://")
		var httpsID int
		err := db.QueryRow(`SELECT id FROM applied_jobs WHERE url = ?`, httpsURL).Scan(&httpsID)
		if err == sql.ErrNoRows {
			_, err = db.Exec(`UPDATE applied_jobs SET url = ? WHERE id = ?`, httpsURL, r.id)
			if err != nil {
				return fmt.Errorf("failed to update applied_jobs url to https: %w", err)
			}
		} else if err == nil {
			_, err = db.Exec(`DELETE FROM applied_jobs WHERE id = ?`, r.id)
			if err != nil {
				return fmt.Errorf("failed to delete duplicate http url from applied_jobs: %w", err)
			}
		} else {
			return fmt.Errorf("failed to check https url in applied_jobs: %w", err)
		}
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
	rows, err := db.Query(`SELECT DISTINCT company_name FROM job_funnel
		WHERE status IN ('APPLIED', 'INTERVIEW_REQUESTED', 'MANUAL_REQUIRED') AND company_name != ''`)
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
		VALUES (?, ?, ?, ?) ON CONFLICT(url) DO NOTHING`, companyName, jobTitle, url, time.Now())
	return err
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

// PromptInjectionThreat is a storage-local mirror of promptsec.Threat, kept
// separate so this package doesn't need to import the security package's
// third-party dependency just to log what was found.
type PromptInjectionThreat struct {
	Type     string
	Severity float64
	Message  string
	Guard    string
	Match    string
	Start    int
	End      int
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

// AddToFunnel inserts a newly discovered job. Callers only ever pass
// "DISCOVERED" as status, so on a conflict (a URL already known from an
// earlier discovery pass, possibly from a previous session) this is a no-op:
// it must NOT reset an in-progress or already-resolved job's status back to
// "DISCOVERED", which would make it eligible for reprocessing while a worker
// is already handling it (or already finished it) - confirmed live 2026-07-21
// as the root cause of the same URL being queued and processed multiple
// times, eventually hitting the UNIQUE constraint on applied_jobs.url.
// The returned bool reports whether a genuinely new row was inserted, so
// callers can avoid re-queuing a URL they already know about.
func AddToFunnel(company, title, url, status string) (bool, error) {
	if db == nil {
		return false, fmt.Errorf("db not initialized")
	}
	url = NormalizeURL(url)
	result, err := db.Exec(`INSERT INTO job_funnel (company_name, job_title, url, status, discovered_at)
		VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(url) DO NOTHING`, company, title, url, status)
	if err != nil {
		return false, err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return rowsAffected > 0, nil
}

func UpdateFunnelStatus(url, status string) error {
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
	_, err := db.Exec("UPDATE job_funnel SET status = ?, last_updated = ? WHERE url = ?", status, time.Now().UTC(), url)
	return err
}

func UpdateFunnelStatusWithScore(url, status string, fitScore int) error {
	if db == nil {
		return fmt.Errorf("db not initialized")
	}
	url = NormalizeURL(url)
	_, err := db.Exec("UPDATE job_funnel SET status = ?, fit_score = ?, last_updated = ? WHERE url = ?", status, fitScore, time.Now().UTC(), url)
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
	CompanyName   string
	JobTitle      string
	URL           string
	FitSimilarity float64
	DiscoveredAt  time.Time
	RankingScore  float64
	RankingReason string
	IsExploration bool
}

func (j *FunnelJob) GetURL() string { return j.URL }
func (j *FunnelJob) GetFitSimilarity() float64 { return j.FitSimilarity }
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
// (improvements.md #35).
func GetDiscoveredJobs() ([]FunnelJob, error) {
	if db == nil {
		return nil, fmt.Errorf("db not initialized")
	}
	// breezy.hr excluded entirely (0 APPLIED / 48 FAILED_SUBMIT, worst-performing source).
	rows, err := db.Query(`SELECT company_name, job_title, url, COALESCE(fit_similarity, -1), discovered_at FROM job_funnel
		WHERE status = 'DISCOVERED' AND url NOT LIKE '%breezy.hr%'`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var jobs []FunnelJob
	for rows.Next() {
		var j FunnelJob
		var discoveredAt sql.NullTime
		if err := rows.Scan(&j.CompanyName, &j.JobTitle, &j.URL, &j.FitSimilarity, &discoveredAt); err != nil {
			log.Printf("[Storage] Error scanning discovered job row: %v", err)
			continue
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
	Total   int
	Applied int
	Captcha int
	Failed  int
	Manual  int
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
		COALESCE(SUM(CASE WHEN status = 'MANUAL_REQUIRED' THEN 1 ELSE 0 END), 0)
		FROM job_funnel
		WHERE url LIKE ? AND status IN ('APPLIED','BLOCKED_CAPTCHA','FAILED_SUBMIT','MANUAL_REQUIRED','PROCESSED_MANUAL')`,
		urlPattern).Scan(&s.Total, &s.Applied, &s.Captcha, &s.Failed, &s.Manual)
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
func RequeueByURLPattern(urlPattern, fromStatus string) (int64, error) {
	if db == nil {
		return 0, fmt.Errorf("db not initialized")
	}
	result, err := db.Exec(`UPDATE job_funnel SET status = 'DISCOVERED' WHERE status = ? AND url LIKE ?`, fromStatus, urlPattern)
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
