package storage

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

var db *sql.DB

func InitDB() error {
	var err error
	db, err = sql.Open("sqlite3", "./applications.db")
	if err != nil {
		return err
	}

	createTableQuery := `
	CREATE TABLE IF NOT EXISTS applied_jobs (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		company_name TEXT,
		job_title TEXT,
		url TEXT UNIQUE,
		applied_at DATETIME
	);`
	_, err = db.Exec(createTableQuery)
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

func SaveApplication(companyName, jobTitle, location, url, resumeContent, coverLetterContent, interviewPrepContent string) error {
	baseDir := "applications"
	companyDir := filepath.Join(baseDir, companyName)

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

// LogFailedSubmission appends a failed auto-submission to a manual review checklist
func LogFailedSubmission(companyName, jobTitle, applyURL string) error {
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
