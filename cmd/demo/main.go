package main

import (
	"fmt"
	"log"
	"time"

	"github.com/howlcipher/Career_Agent_Core/pkg/storage"
	_ "modernc.org/sqlite"
)

func main() {
	fmt.Println("Career Agent Core - Demo Mode Seeder")
	fmt.Println("Initializing local database with synthetic data...")

	// Initialize DB schema
	err := storage.InitDB()
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer storage.CloseDB()

	db := storage.GetDB()
	if db == nil {
		log.Fatalf("Failed to get database connection")
	}

	// Clear existing data to ensure a clean demo state
	fmt.Println("Clearing existing job_funnel data...")
	_, err = db.Exec("DELETE FROM job_funnel")
	if err != nil {
		log.Fatalf("Failed to clear job_funnel: %v", err)
	}

	// Insert synthetic discovered jobs
	fmt.Println("Populating mock jobs...")
	now := time.Now().UTC()
	yesterday := now.Add(-24 * time.Hour)
	twoDaysAgo := now.Add(-48 * time.Hour)

	mockJobs := []struct {
		Company string
		Title   string
		Status  string
		Score   int
		Time    time.Time
		Source  string
		Tone    string
	}{
		{"Globex Corporation", "Staff Platform Engineer", "APPLIED", 85, twoDaysAgo, "greenhouse", "Direct"},
		{"Initech", "Site Reliability Engineer", "INTERVIEW_REQUESTED", 92, twoDaysAgo, "lever", "Professional"},
		{"Stark Industries", "AI Infrastructure Engineer", "REJECTED", 78, twoDaysAgo, "workday", "Concise"},
		{"Soylent Corp", "DevOps Engineer", "APPLIED", 81, yesterday, "greenhouse", "Professional"},
		{"Cyberdyne Systems", "Automation Engineer", "AWAITING_REVIEW", 89, now, "lever", "Direct"},
		{"Wayne Enterprises", "Platform Engineer", "FAILED_SUBMIT", 82, now, "workday", "Professional"},
		{"Acme Corp", "Backend Engineer", "BLOCKED_CAPTCHA", 90, now, "ashby", "Conversational"},
		{"Massive Dynamic", "Junior DevOps", "SKIPPED_LOW_FIT", 40, yesterday, "greenhouse", ""},
		{"Oscorp", "AI Platform Engineer", "DISCOVERED", 0, now, "lever", ""},
	}

	for i, job := range mockJobs {
		url := fmt.Sprintf("https://jobs.example.com/demo/%d", i)
		_, err := db.Exec(`
			INSERT INTO job_funnel 
			(company_name, job_title, url, status, fit_score, discovered_at, applied_at, last_updated, discovery_source, tone_variant)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, job.Company, job.Title, url, job.Status, job.Score, job.Time, job.Time, job.Time, job.Source, job.Tone)
		if err != nil {
			log.Fatalf("Failed to insert mock job %s: %v", job.Company, err)
		}
	}

	fmt.Println("Demo data successfully seeded!")
	fmt.Println("Run 'go run ./cmd/dashboard' and visit http://localhost:8080 to view.")
}
