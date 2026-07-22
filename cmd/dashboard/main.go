package main

import (
	"database/sql"
	"embed"
	"encoding/json"
	"log"
	"net/http"

	_ "github.com/mattn/go-sqlite3"
)

type Metrics struct {
	Discovered         int    `json:"discovered"`
	Processing         int    `json:"processing"`
	Skipped            int    `json:"skipped"`
	Applied            int    `json:"applied"`
	Failed             int    `json:"failed"`
	LastAppliedCompany string `json:"last_applied_company,omitempty"`
	LastAppliedTitle   string `json:"last_applied_title,omitempty"`
	LastAppliedURL     string `json:"last_applied_url,omitempty"`
	LastAppliedAt      string `json:"last_applied_at,omitempty"`

	CurrentCompany string `json:"current_company,omitempty"`
	CurrentTitle   string `json:"current_title,omitempty"`
	CurrentSince   string `json:"current_since,omitempty"`

	LastSkippedCompany string `json:"last_skipped_company,omitempty"`
	LastSkippedTitle   string `json:"last_skipped_title,omitempty"`
	LastSkippedReason  string `json:"last_skipped_reason,omitempty"`
	LastSkippedAt      string `json:"last_skipped_at,omitempty"`

	LastFailedCompany string `json:"last_failed_company,omitempty"`
	LastFailedTitle   string `json:"last_failed_title,omitempty"`
	LastFailedReason  string `json:"last_failed_reason,omitempty"`
	LastFailedAt      string `json:"last_failed_at,omitempty"`
}

// statusReason maps a raw job_funnel status code to a short human-readable
// explanation, since the DB only stores the status code itself, not a
// free-text reason (the detailed "why" - e.g. the exact fit score - only
// ever exists in the transient log file, not persisted anywhere queryable).
func statusReason(status string) string {
	switch status {
	case "SKIPPED":
		return "Fit score below the required threshold"
	case "BLOCKED_CAPTCHA":
		return "Blocked by CAPTCHA / bot protection"
	case "FAILED_SCORE":
		return "Failed to score the job against your profile"
	case "FAILED_SUBMIT":
		return "Reached the application form but failed to submit"
	default:
		return status
	}
}

//go:embed index.html
var indexHTML embed.FS

var db *sql.DB

func main() {
	var err error
	db, err = sql.Open("sqlite3", "./applications.db?_journal_mode=WAL")
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)

	http.HandleFunc("/", serveDashboard)
	http.HandleFunc("/api/metrics", serveMetrics)

	log.Println("🚀 Career Agent Web Dashboard running at http://localhost:8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

func serveMetrics(w http.ResponseWriter, r *http.Request) {
	var m Metrics
	db.QueryRow("SELECT COUNT(*) FROM job_funnel WHERE status = 'DISCOVERED' OR status = 'NEW'").Scan(&m.Discovered)
	db.QueryRow("SELECT COUNT(*) FROM job_funnel WHERE status = 'PROCESSING'").Scan(&m.Processing)
	db.QueryRow("SELECT COUNT(*) FROM job_funnel WHERE status = 'SKIPPED'").Scan(&m.Skipped)
	db.QueryRow("SELECT COUNT(*) FROM job_funnel WHERE status IN ('APPLIED', 'PROCESSED_MANUAL')").Scan(&m.Applied)
	db.QueryRow("SELECT COUNT(*) FROM job_funnel WHERE status IN ('FAILED_SCORE', 'FAILED_SUBMIT')").Scan(&m.Failed)

	// applied_jobs only records that a tailored resume/cover letter was
	// generated and saved (SaveApplication runs early in AttemptSubmit,
	// before the actual browser fill/submit) - it does NOT mean the
	// submission itself succeeded. job_funnel.status only reaches APPLIED
	// after the full AttemptSubmit call returns without error. Join both so
	// "last applied" only ever shows a job that genuinely completed.
	var lastAppliedAt sql.NullTime
	err := db.QueryRow(`SELECT aj.company_name, aj.job_title, aj.url, aj.applied_at
		FROM applied_jobs aj
		JOIN job_funnel jf ON jf.url = aj.url
		WHERE jf.status IN ('APPLIED', 'PROCESSED_MANUAL')
		ORDER BY aj.applied_at DESC LIMIT 1`).
		Scan(&m.LastAppliedCompany, &m.LastAppliedTitle, &m.LastAppliedURL, &lastAppliedAt)
	if err != nil && err != sql.ErrNoRows {
		log.Printf("Failed to query last applied job: %v", err)
	}
	if lastAppliedAt.Valid {
		m.LastAppliedAt = lastAppliedAt.Time.Local().Format("Jan 2, 2006 3:04 PM MST")
	}

	// Currently processing: the most recently touched PROCESSING row.
	// last_updated is required here, not id/discovered_at - multiple rows
	// can be stuck at PROCESSING from an interrupted run (confirmed live
	// 2026-07-21, see bugs.md #12's data-correction notes), so only
	// "most recently touched" reliably identifies what's actually active
	// right now versus an old orphaned entry.
	var currentCompany, currentTitle sql.NullString
	var currentSince sql.NullTime
	err = db.QueryRow(`SELECT company_name, job_title, last_updated FROM job_funnel
		WHERE status = 'PROCESSING' ORDER BY last_updated DESC LIMIT 1`).
		Scan(&currentCompany, &currentTitle, &currentSince)
	if err != nil && err != sql.ErrNoRows {
		log.Printf("Failed to query currently processing job: %v", err)
	}
	m.CurrentCompany = currentCompany.String
	m.CurrentTitle = currentTitle.String
	if currentSince.Valid {
		m.CurrentSince = currentSince.Time.Local().Format("3:04:05 PM")
	}

	var skippedCompany, skippedTitle, skippedStatus sql.NullString
	var skippedAt sql.NullTime
	err = db.QueryRow(`SELECT company_name, job_title, status, last_updated FROM job_funnel
		WHERE status IN ('SKIPPED', 'BLOCKED_CAPTCHA') ORDER BY last_updated DESC LIMIT 1`).
		Scan(&skippedCompany, &skippedTitle, &skippedStatus, &skippedAt)
	if err != nil && err != sql.ErrNoRows {
		log.Printf("Failed to query last skipped job: %v", err)
	}
	m.LastSkippedCompany = skippedCompany.String
	m.LastSkippedTitle = skippedTitle.String
	if skippedStatus.Valid {
		m.LastSkippedReason = statusReason(skippedStatus.String)
	}
	if skippedAt.Valid {
		m.LastSkippedAt = skippedAt.Time.Local().Format("Jan 2, 3:04 PM")
	}

	var failedCompany, failedTitle, failedStatus sql.NullString
	var failedAt sql.NullTime
	err = db.QueryRow(`SELECT company_name, job_title, status, last_updated FROM job_funnel
		WHERE status IN ('FAILED_SCORE', 'FAILED_SUBMIT') ORDER BY last_updated DESC LIMIT 1`).
		Scan(&failedCompany, &failedTitle, &failedStatus, &failedAt)
	if err != nil && err != sql.ErrNoRows {
		log.Printf("Failed to query last failed job: %v", err)
	}
	m.LastFailedCompany = failedCompany.String
	m.LastFailedTitle = failedTitle.String
	if failedStatus.Valid {
		m.LastFailedReason = statusReason(failedStatus.String)
	}
	if failedAt.Valid {
		m.LastFailedAt = failedAt.Time.Local().Format("Jan 2, 3:04 PM")
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(m)
}

func serveDashboard(w http.ResponseWriter, r *http.Request) {
	content, err := indexHTML.ReadFile("index.html")
	if err != nil {
		http.Error(w, "Could not load dashboard", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(content)
}
