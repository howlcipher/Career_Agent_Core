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
		m.LastAppliedAt = lastAppliedAt.Time.Format("Jan 2, 2006 3:04 PM MST")
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
