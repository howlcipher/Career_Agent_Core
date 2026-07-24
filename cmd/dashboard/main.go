package main

import (
	"database/sql"
	"embed"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

type Metrics struct {
	Discovered         int    `json:"discovered"`
	Processing         int    `json:"processing"`
	Skipped            int    `json:"skipped"`
	Applied            int    `json:"applied"`
	Failed             int    `json:"failed"`
	ManualRequired     int    `json:"manual_required"`
	BlockedCaptcha     int    `json:"blocked_captcha"`
	InvalidURL         int    `json:"invalid_url"`
	LastAppliedCompany        string `json:"last_applied_company,omitempty"`
	LastAppliedTitle          string `json:"last_applied_title,omitempty"`
	LastAppliedURL            string `json:"last_applied_url,omitempty"`
	LastAppliedAt             string `json:"last_applied_at,omitempty"`
	LastAppliedProcessingTime string `json:"last_applied_processing_time,omitempty"`

	CurrentCompany string `json:"current_company,omitempty"`
	CurrentTitle   string `json:"current_title,omitempty"`
	CurrentSince   string `json:"current_since,omitempty"`

	LastSkippedCompany        string `json:"last_skipped_company,omitempty"`
	LastSkippedTitle          string `json:"last_skipped_title,omitempty"`
	LastSkippedReason         string `json:"last_skipped_reason,omitempty"`
	LastSkippedAt             string `json:"last_skipped_at,omitempty"`
	LastSkippedProcessingTime string `json:"last_skipped_processing_time,omitempty"`

	LastFailedCompany        string `json:"last_failed_company,omitempty"`
	LastFailedTitle          string `json:"last_failed_title,omitempty"`
	LastFailedReason         string `json:"last_failed_reason,omitempty"`
	LastFailedAt             string `json:"last_failed_at,omitempty"`
	LastFailedProcessingTime string `json:"last_failed_processing_time,omitempty"`

	LastManualCompany        string `json:"last_manual_company,omitempty"`
	LastManualTitle          string `json:"last_manual_title,omitempty"`
	LastManualAt             string `json:"last_manual_at,omitempty"`
	LastManualProcessingTime string `json:"last_manual_processing_time,omitempty"`

	TotalApplied     int                    `json:"total_applied_tracked"`
	Interviews       int                    `json:"interviews"`
	Rejections       int                    `json:"rejections"`
	InterviewRatePct string                 `json:"interview_rate_pct,omitempty"`
	BySource         []SourceConversionStat `json:"by_source,omitempty"`
}

// SourceConversionStat is one ATS platform's interview-conversion slice,
// mirroring pkg/storage.SourceConversionStat's shape — not imported directly
// since cmd/dashboard queries its own local db connection rather than
// initializing pkg/storage's package-level one (see every other query in
// serveMetrics for the established pattern this follows).
type SourceConversionStat struct {
	Source        string `json:"source"`
	TotalApplied  int    `json:"total_applied"`
	Interviews    int    `json:"interviews"`
	Rejections    int    `json:"rejections"`
	Pending       int    `json:"pending"`
	InterviewRate string `json:"interview_rate_pct"`
}

// formatDuration renders how long a job sat in the pipeline (discovered_at
// to the terminal status's last_updated/applied_at) as a short human string.
// discovered_at predates last_updated by anywhere from minutes to several
// days in this single-worker, frequently-restarted system, so days must be
// called out explicitly rather than overflowing into a huge hour count.
func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return "under a minute"
	}
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	minutes := int(d.Minutes()) % 60
	switch {
	case days > 0:
		return fmt.Sprintf("%dd %dh", days, hours)
	case hours > 0:
		return fmt.Sprintf("%dh %dm", hours, minutes)
	default:
		return fmt.Sprintf("%dm", minutes)
	}
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
	case "MANUAL_REQUIRED":
		return "ATS requires an account — apply manually with the saved tailored docs"
	case "INVALID_URL":
		return "Not a real posting (board index, marketing page, or expired-redirect URL)"
	default:
		return status
	}
}

//go:embed index.html
var indexHTML embed.FS

//go:embed favicon.png
var faviconPNG embed.FS

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
	http.HandleFunc("/favicon.png", serveFavicon)

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
	db.QueryRow("SELECT COUNT(*) FROM job_funnel WHERE status = 'MANUAL_REQUIRED'").Scan(&m.ManualRequired)
	db.QueryRow("SELECT COUNT(*) FROM job_funnel WHERE status = 'BLOCKED_CAPTCHA'").Scan(&m.BlockedCaptcha)
	db.QueryRow("SELECT COUNT(*) FROM job_funnel WHERE status = 'INVALID_URL'").Scan(&m.InvalidURL)

	// applied_jobs only records that a tailored resume/cover letter was
	// generated and saved (SaveApplication runs early in AttemptSubmit,
	// before the actual browser fill/submit) - it does NOT mean the
	// submission itself succeeded. job_funnel.status only reaches APPLIED
	// after the full AttemptSubmit call returns without error. Join both so
	// "last applied" only ever shows a job that genuinely completed.
	var lastAppliedAt, lastAppliedDiscoveredAt sql.NullTime
	err := db.QueryRow(`SELECT aj.company_name, aj.job_title, aj.url, aj.applied_at, jf.discovered_at
		FROM applied_jobs aj
		JOIN job_funnel jf ON jf.url = aj.url
		WHERE jf.status IN ('APPLIED', 'PROCESSED_MANUAL')
		ORDER BY aj.applied_at DESC LIMIT 1`).
		Scan(&m.LastAppliedCompany, &m.LastAppliedTitle, &m.LastAppliedURL, &lastAppliedAt, &lastAppliedDiscoveredAt)
	if err != nil && err != sql.ErrNoRows {
		log.Printf("Failed to query last applied job: %v", err)
	}
	if lastAppliedAt.Valid {
		m.LastAppliedAt = lastAppliedAt.Time.Local().Format("Jan 2, 2006 3:04 PM MST")
	}
	if lastAppliedAt.Valid && lastAppliedDiscoveredAt.Valid {
		m.LastAppliedProcessingTime = formatDuration(lastAppliedAt.Time.Sub(lastAppliedDiscoveredAt.Time))
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
	var skippedAt, skippedDiscoveredAt sql.NullTime
	// Narrowed to just SKIPPED (was SKIPPED + BLOCKED_CAPTCHA) now that
	// BLOCKED_CAPTCHA has its own dedicated tile -- this widget's status
	// used to disagree with the Skipped tile's own count, which never
	// included BLOCKED_CAPTCHA (confirmed live 2026-07-24, bugs.md #55's
	// investigation).
	err = db.QueryRow(`SELECT company_name, job_title, status, last_updated, discovered_at FROM job_funnel
		WHERE status = 'SKIPPED' ORDER BY last_updated DESC LIMIT 1`).
		Scan(&skippedCompany, &skippedTitle, &skippedStatus, &skippedAt, &skippedDiscoveredAt)
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
	if skippedAt.Valid && skippedDiscoveredAt.Valid {
		m.LastSkippedProcessingTime = formatDuration(skippedAt.Time.Sub(skippedDiscoveredAt.Time))
	}

	var failedCompany, failedTitle, failedStatus sql.NullString
	var failedAt, failedDiscoveredAt sql.NullTime
	err = db.QueryRow(`SELECT company_name, job_title, status, last_updated, discovered_at FROM job_funnel
		WHERE status IN ('FAILED_SCORE', 'FAILED_SUBMIT') ORDER BY last_updated DESC LIMIT 1`).
		Scan(&failedCompany, &failedTitle, &failedStatus, &failedAt, &failedDiscoveredAt)
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
	if failedAt.Valid && failedDiscoveredAt.Valid {
		m.LastFailedProcessingTime = formatDuration(failedAt.Time.Sub(failedDiscoveredAt.Time))
	}

	var manualCompany, manualTitle sql.NullString
	var manualAt, manualDiscoveredAt sql.NullTime
	err = db.QueryRow(`SELECT company_name, job_title, last_updated, discovered_at FROM job_funnel
		WHERE status = 'MANUAL_REQUIRED' ORDER BY last_updated DESC LIMIT 1`).
		Scan(&manualCompany, &manualTitle, &manualAt, &manualDiscoveredAt)
	if err != nil && err != sql.ErrNoRows {
		log.Printf("Failed to query last manual-required job: %v", err)
	}
	m.LastManualCompany = manualCompany.String
	m.LastManualTitle = manualTitle.String
	if manualAt.Valid {
		m.LastManualAt = manualAt.Time.Local().Format("Jan 2, 3:04 PM")
	}
	if manualAt.Valid && manualDiscoveredAt.Valid {
		m.LastManualProcessingTime = formatDuration(manualAt.Time.Sub(manualDiscoveredAt.Time))
	}

	// Conversion-rate analytics (improvements.md #15): pkg/tracker only ever
	// moves a job_funnel row from APPLIED to REJECTED or INTERVIEW_REQUESTED
	// (never a distinct OFFER status), so "ever applied" = status IN
	// ('APPLIED','REJECTED','INTERVIEW_REQUESTED').
	var interviews, rejections int
	err = db.QueryRow(`SELECT
		COUNT(*),
		COALESCE(SUM(CASE WHEN status = 'INTERVIEW_REQUESTED' THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN status = 'REJECTED' THEN 1 ELSE 0 END), 0)
		FROM job_funnel WHERE status IN ('APPLIED','REJECTED','INTERVIEW_REQUESTED')`).
		Scan(&m.TotalApplied, &interviews, &rejections)
	if err != nil {
		log.Printf("Failed to query conversion stats: %v", err)
	}
	m.Interviews = interviews
	m.Rejections = rejections
	if m.TotalApplied > 0 {
		m.InterviewRatePct = fmt.Sprintf("%.1f%%", float64(interviews)/float64(m.TotalApplied)*100)
	}

	sourceRows, err := db.Query(`SELECT
		CASE
			WHEN url LIKE '%greenhouse%' THEN 'Greenhouse'
			WHEN url LIKE '%lever.co%' THEN 'Lever'
			WHEN url LIKE '%myworkdayjobs.com%' THEN 'Workday'
			WHEN url LIKE '%smartrecruiters%' THEN 'SmartRecruiters'
			WHEN url LIKE '%ashbyhq%' THEN 'Ashby'
			ELSE 'Other'
		END AS source,
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
		log.Printf("Failed to query conversion stats by source: %v", err)
	} else {
		defer sourceRows.Close()
		for sourceRows.Next() {
			var s SourceConversionStat
			if err := sourceRows.Scan(&s.Source, &s.TotalApplied, &s.Interviews, &s.Rejections, &s.Pending); err != nil {
				log.Printf("Failed to scan conversion-by-source row: %v", err)
				continue
			}
			if s.TotalApplied > 0 {
				s.InterviewRate = fmt.Sprintf("%.1f%%", float64(s.Interviews)/float64(s.TotalApplied)*100)
			}
			m.BySource = append(m.BySource, s)
		}
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

func serveFavicon(w http.ResponseWriter, r *http.Request) {
	content, err := faviconPNG.ReadFile("favicon.png")
	if err != nil {
		http.Error(w, "Could not load favicon", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Write(content)
}
