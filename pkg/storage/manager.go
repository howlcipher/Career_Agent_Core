package storage

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

var db *sql.DB

func InitDB() error {
	var err error
	db, err = sql.Open("sqlite3", "./applications.db?_journal_mode=WAL&_busy_timeout=5000")
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
		applied_at DATETIME
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
	);`
	_, err = db.Exec(createTableQuery)
	return err
}

func GetDB() *sql.DB {
	return db
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

func SaveApplication(companyName, jobTitle, location, url, resumeContent, coverLetterContent, interviewPrepContent string) error {
	safeCompany := strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			return r
		}
		return '_'
	}, companyName)

	companyDir := filepath.Join("applications", safeCompany)
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

func CloseDB() error {
	if db != nil {
		return db.Close()
	}
	return nil
}

func AddToFunnel(company, title, url, status string) error {
	if db == nil {
		return fmt.Errorf("db not initialized")
	}
	_, err := db.Exec(`INSERT INTO job_funnel (company_name, job_title, url, status, discovered_at) 
		VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(url) DO UPDATE SET status=excluded.status`, company, title, url, status)
	return err
}

func UpdateFunnelStatus(url, status string) error {
	if db == nil {
		return fmt.Errorf("db not initialized")
	}
	_, err := db.Exec("UPDATE job_funnel SET status = ? WHERE url = ?", status, url)
	return err
}

func UpdateFunnelStatusWithScore(url, status string, fitScore int) error {
	if db == nil {
		return fmt.Errorf("db not initialized")
	}
	_, err := db.Exec("UPDATE job_funnel SET status = ?, fit_score = ? WHERE url = ?", status, fitScore, url)
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
