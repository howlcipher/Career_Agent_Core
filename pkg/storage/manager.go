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
		last_updated DATETIME
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
	return migrateJobFunnelLastUpdated()
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

func GetDiscoveredJobs() ([]FunnelJob, error) {
	if db == nil {
		return nil, fmt.Errorf("db not initialized")
	}
	rows, err := db.Query("SELECT company_name, job_title, url FROM job_funnel WHERE status = 'DISCOVERED'")
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
