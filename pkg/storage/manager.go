package storage

import (
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

var db *sql.DB

func InitDB() error {
	return InitDBWithPath("./applications.db")
}

func InitDBWithPath(path string) error {
	var err error
	dsn := path
	if !strings.Contains(path, "?") {
		dsn += "?_journal_mode=WAL&_busy_timeout=5000"
	}
	db, err = sql.Open("sqlite3", dsn)
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
	return migrateJobFunnelToneVariant()
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

func HasApplied(url string) bool {
	if db == nil {
		return false
	}
	var id int
	err := db.QueryRow("SELECT id FROM applied_jobs WHERE url = ?", url).Scan(&id)
	return err == nil
}

func RecordApplicationInDB(companyName, jobTitle, url string) error {
	if db == nil {
		return fmt.Errorf("db not initialized")
	}
	_, err := db.Exec("INSERT INTO applied_jobs (company_name, job_title, url, applied_at) VALUES (?, ?, ?, ?)", companyName, jobTitle, url, time.Now())
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
func CoverLetterPath(companyName string) string {
	return filepath.Join("applications", safeCompanyDirName(companyName), "coverletter.txt")
}

func SaveApplication(companyName, jobTitle, location, url, resumeContent, coverLetterContent, interviewPrepContent string) error {
	companyDir := filepath.Join("applications", safeCompanyDirName(companyName))
	if err := os.MkdirAll(companyDir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	resumePath := filepath.Join(companyDir, "resume.md")
	if err := os.WriteFile(resumePath, []byte(resumeContent), 0644); err != nil {
		return fmt.Errorf("failed to write resume: %w", err)
	}

	coverLetterPath := filepath.Join(companyDir, "coverletter.txt")
	if err := os.WriteFile(coverLetterPath, []byte(coverLetterContent), 0644); err != nil {
		return fmt.Errorf("failed to write cover letter: %w", err)
	}

	interviewPrepPath := filepath.Join(companyDir, "interview_prep.md")
	if err := os.WriteFile(interviewPrepPath, []byte(interviewPrepContent), 0644); err != nil {
		return fmt.Errorf("failed to write interview prep: %w", err)
	}

	metadata := Metadata{
		CompanyName:        companyName,
		JobTitle:           jobTitle,
		Location:           location,
		OriginalPostingURL: url,
		ApplicationDate:    time.Now(),
	}

	metadataBytes, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	metadataPath := filepath.Join(companyDir, "metadata.json")
	if err := os.WriteFile(metadataPath, metadataBytes, 0644); err != nil {
		return fmt.Errorf("failed to write metadata: %w", err)
	}

	return RecordApplicationInDB(companyName, jobTitle, url)
}

var logMutex sync.Mutex

// LogFailedSubmission appends a failed auto-submission to a manual review checklist
func LogFailedSubmission(companyName, jobTitle, applyURL string) error {
	logMutex.Lock()
	defer logMutex.Unlock()

	reportPath := filepath.Join("applications", "manual_submissions.md")

	if _, err := os.Stat(reportPath); os.IsNotExist(err) {
		header := "# Manual Submission Backlog\n\nThe auto-submitter failed to process the following applications. Please submit them manually:\n\n"
		os.WriteFile(reportPath, []byte(header), 0644)
	}

	entry := fmt.Sprintf("- [ ] **%s** - %s: [Apply Here](%s)\n", companyName, jobTitle, applyURL)

	f, err := os.OpenFile(reportPath, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0644)
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

// MoveToManualApply relocates a company's saved docs folder from
// applications/<company>/ into applications/needs_manual_apply/<company>/
// so account-gated jobs live in one clearly-labeled place. Returns the
// destination path, or "" if the source folder doesn't exist (docs may
// have failed to save). A pre-existing destination gets a numeric suffix
// rather than being overwritten — company-name collisions are real
// (pre-#19 rows share labels like "en_US").
func MoveToManualApply(companyName string) (string, error) {
	safeCompany := safeCompanyDirName(companyName)
	src := filepath.Join("applications", safeCompany)
	if _, err := os.Stat(src); os.IsNotExist(err) {
		return "", nil
	}
	if err := os.MkdirAll(manualApplyBase, 0755); err != nil {
		return "", fmt.Errorf("failed to create manual-apply dir: %w", err)
	}
	dst := filepath.Join(manualApplyBase, safeCompany)
	for i := 2; ; i++ {
		if _, err := os.Stat(dst); os.IsNotExist(err) {
			break
		}
		dst = filepath.Join(manualApplyBase, fmt.Sprintf("%s-%d", safeCompany, i))
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

	if err := os.MkdirAll(manualApplyBase, 0755); err != nil {
		return fmt.Errorf("failed to create manual-apply dir: %w", err)
	}
	reportPath := filepath.Join(manualApplyBase, "manual_queue.md")

	if _, err := os.Stat(reportPath); os.IsNotExist(err) {
		header := "# Manual Apply Queue\n\nThese jobs sit behind an ATS account sign-in, so automation hands them off by design. Tailored documents are already saved in each company's folder alongside this file — create the account, upload, submit, check the box.\n\n"
		os.WriteFile(reportPath, []byte(header), 0644)
	}

	docsNote := "docs not found"
	if docsDir != "" {
		docsNote = fmt.Sprintf("docs in `%s/`", docsDir)
	}
	entry := fmt.Sprintf("- [ ] **%s** - %s: [Apply Here](%s) — %s\n", companyName, jobTitle, applyURL, docsNote)

	f, err := os.OpenFile(reportPath, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0644)
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

	if err := os.MkdirAll("applications", 0755); err != nil {
		return fmt.Errorf("failed to create applications directory: %w", err)
	}

	reportPath := filepath.Join("applications", "prompt_injection_detections.csv")
	writeHeader := false
	if _, err := os.Stat(reportPath); os.IsNotExist(err) {
		writeHeader = true
	}

	f, err := os.OpenFile(reportPath, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0644)
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
	_, err := db.Exec("INSERT INTO execution_logs (job_id, url, tokens_used, status, logged_at) VALUES (?, ?, ?, ?, ?)", jobID, url, tokens, status, time.Now())
	return err
}

type FunnelJob struct {
	CompanyName string
	JobTitle    string
	URL         string
}

// sourcePriorityCASE ranks jobs by how likely their platform is to actually
// reach APPLIED, based on live outcome data rather than guesswork (bugs.md
// #45-#50, 2026-07-23 session). Tier 0: Greenhouse/Lever, dedicated handlers
// confirmed working end-to-end (a real Lever posting reached APPLIED the
// same session #47 shipped). Tier 1: other platforms with dedicated or
// Learner-Module handling and a real on-page form once #45/#46's CAPTCHA
// false-positives stopped killing them early (Ashby, Pinpoint, Homerun) —
// not yet individually proven to reach APPLIED, but not known to have a
// structural blocker either. Tier 2 (default): everything else, including
// platforms with a known extra friction point that isn't fully solved
// (SmartRecruiters' persistent DataDome CAPTCHA even post-click, Jobvite's
// consent gate, applytojob.com's repeated resume-selector failures). Tier 3:
// myworkdayjobs.com and workable.com — both confirmed account-gated (bug
// #18, bug #50), can only ever reach MANUAL_REQUIRED regardless of fill
// logic. This ordering will go stale as more bugs are fixed or new ones
// found; re-derive it from fresh cmd/requeue -stats output rather than
// trusting this comment indefinitely.
const sourcePriorityCASE = `CASE
		WHEN url LIKE '%myworkdayjobs.com%' OR url LIKE '%workable%' THEN 3
		WHEN url LIKE '%greenhouse%' OR url LIKE '%lever.co%' THEN 0
		WHEN url LIKE '%ashbyhq%' OR url LIKE '%pinpointhq%' OR url LIKE '%homerun.co%' THEN 1
		ELSE 2
	END`

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

// GetDiscoveredJobs orders the queue by sourcePriorityCASE first (platform
// reachability — a topically perfect job is still worthless if its ATS can
// only ever reach MANUAL_REQUIRED or worse), then by fit_similarity DESC as
// a tie-break within each tier (improvements.md #22: an embedding-similarity
// score between the job's title/company and the resume, backfilled
// out-of-band by cmd/rankjobs since computing it inline here would mean an
// embedding call per query). COALESCE(fit_similarity, -1) means a
// not-yet-backfilled row (NULL) sorts after every scored row in its tier,
// falling back to the pre-#22 id-only order — so this change is additive,
// never a regression, for rows cmd/rankjobs hasn't reached yet.
func GetDiscoveredJobs() ([]FunnelJob, error) {
	if db == nil {
		return nil, fmt.Errorf("db not initialized")
	}
	// breezy.hr excluded entirely (0 APPLIED / 48 FAILED_SUBMIT, worst-performing source).
	rows, err := db.Query(`SELECT company_name, job_title, url FROM job_funnel
		WHERE status = 'DISCOVERED' AND url NOT LIKE '%breezy.hr%'
		ORDER BY ` + sourcePriorityCASE + `, COALESCE(fit_similarity, -1) DESC, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var jobs []FunnelJob
	for rows.Next() {
		var j FunnelJob
		if err := rows.Scan(&j.CompanyName, &j.JobTitle, &j.URL); err != nil {
			log.Printf("[Storage] Error scanning discovered job row: %v", err)
			continue
		}
		jobs = append(jobs, j)
	}
	return jobs, nil
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
		`+atsSourceCASE+` AS source,
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
