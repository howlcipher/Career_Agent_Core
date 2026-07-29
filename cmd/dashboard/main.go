package main

import (
	"database/sql"
	"embed"
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/howlcipher/Career_Agent_Core/pkg/security"
	_ "modernc.org/sqlite"
)

type Metrics struct {
	Discovered                int    `json:"discovered"`
	Processing                int    `json:"processing"`
	Skipped                   int    `json:"skipped"`
	Applied                   int    `json:"applied"`
	Failed                    int    `json:"failed"`
	ManualRequired            int    `json:"manual_required"`
	BlockedCaptcha            int    `json:"blocked_captcha"`
	InvalidURL                int    `json:"invalid_url"`
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

	TotalApplied     int                     `json:"total_applied_tracked"`
	Interviews       int                     `json:"interviews"`
	Rejections       int                     `json:"rejections"`
	InterviewRatePct string                  `json:"interview_rate_pct,omitempty"`
	BySource         []SourceConversionStat  `json:"by_source,omitempty"`
	ByVariant        []VariantConversionStat `json:"by_variant,omitempty"`
}

// VariantConversionStat is one cover-letter tone variant's interview-
// conversion slice (improvements.md #13), mirroring
// pkg/storage.VariantConversionStat's shape — same "query its own local db
// connection" reasoning as SourceConversionStat above.
type VariantConversionStat struct {
	Variant       string `json:"variant"`
	TotalApplied  int    `json:"total_applied"`
	Interviews    int    `json:"interviews"`
	Rejections    int    `json:"rejections"`
	Pending       int    `json:"pending"`
	InterviewRate string `json:"interview_rate_pct"`
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
	case "AWAITING_REVIEW":
		return "Filled by Copilot — awaiting your review and submit"
	case "INVALID_URL":
		return "Not a real posting (board index, marketing page, or expired-redirect URL)"
	default:
		return status
	}
}

//go:embed ui/dist
var uiDistFS embed.FS

var db *sql.DB

const defaultDashboardAddress = "127.0.0.1:8080"

func normalizeDashboardAddress(raw string) (string, error) {
	address := strings.TrimSpace(raw)
	if address == "" {
		address = defaultDashboardAddress
	}

	_, portText, err := net.SplitHostPort(address)
	if err != nil {
		return "", fmt.Errorf("dashboard address must use host:port form: %w", err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port < 1 || port > 65535 {
		return "", fmt.Errorf("dashboard address has invalid port %q", portText)
	}
	return address, nil
}

func dashboardExposureWarning(address string) string {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Sprintf("WARNING: dashboard address %q is invalid", address)
	}

	host = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(host)), ".")
	if host == "localhost" {
		return ""
	}
	if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
		return ""
	}
	return fmt.Sprintf(
		"WARNING: dashboard address %q is not loopback; this unauthenticated server exposes private application data",
		address,
	)
}

func newDashboardServer(address string, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:              address,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
}

func main() {
	if err := security.PreparePrivateWorkspace(".", os.Stderr); err != nil {
		log.Fatalf("Startup aborted because private paths could not be secured: %v", err)
	}

	requestedAddress := flag.String(
		"addr",
		defaultDashboardAddress,
		"dashboard listen address in host:port form",
	)
	flag.Parse()
	address, err := normalizeDashboardAddress(*requestedAddress)
	if err != nil {
		log.Fatalf("Invalid dashboard address: %v", err)
	}

	db, err = sql.Open("sqlite", "./applications.db?_journal_mode=WAL")
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()
	db.SetMaxOpenConns(10)

	mux := http.NewServeMux()
	
	// Serve static files from the embedded React/Vite app
	subFS, err := fs.Sub(uiDistFS, "ui/dist")
	if err != nil {
		log.Fatalf("Failed to sub ui/dist: %v", err)
	}
	fileServer := http.FileServer(http.FS(subFS))
	mux.Handle("/", fileServer)

	mux.HandleFunc("/api/metrics", serveMetrics)
	mux.HandleFunc("/api/agent/start", serveAgentStart)
	mux.HandleFunc("/api/agent/stop", serveAgentStop)
	mux.HandleFunc("/api/agent/status", serveAgentStatus)

	if warning := dashboardExposureWarning(address); warning != "" {
		log.Print(warning)
	}
	log.Printf("🚀 Career Agent Web Dashboard running at http://%s", address)
	log.Fatal(newDashboardServer(address, mux).ListenAndServe())
}

func serveMetrics(w http.ResponseWriter, r *http.Request) {
	var m Metrics
	var g errgroup.Group

	g.Go(func() error {
		err := db.QueryRow(`SELECT 
			COALESCE(SUM(CASE WHEN status IN ('DISCOVERED', 'NEW') THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN status = 'PROCESSING' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN status = 'SKIPPED' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN status IN ('APPLIED', 'PROCESSED_MANUAL') THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN status IN ('FAILED_SCORE', 'FAILED_SUBMIT') THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN status IN ('MANUAL_REQUIRED', 'AWAITING_REVIEW') THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN status = 'BLOCKED_CAPTCHA' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN status = 'INVALID_URL' THEN 1 ELSE 0 END), 0)
		FROM job_funnel`).Scan(
			&m.Discovered, &m.Processing, &m.Skipped, &m.Applied,
			&m.Failed, &m.ManualRequired, &m.BlockedCaptcha, &m.InvalidURL,
		)
		if err != nil {
			log.Printf("Failed to query basic counts: %v", err)
		}
		return nil
	})

	g.Go(func() error {
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
		return nil
	})

	g.Go(func() error {
		var currentCompany, currentTitle sql.NullString
		var currentSince sql.NullTime
		err := db.QueryRow(`SELECT company_name, job_title, last_updated FROM job_funnel
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
		return nil
	})

	g.Go(func() error {
		var skippedCompany, skippedTitle, skippedStatus sql.NullString
		var skippedAt, skippedDiscoveredAt sql.NullTime
		err := db.QueryRow(`SELECT company_name, job_title, status, last_updated, discovered_at FROM job_funnel
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
		return nil
	})

	g.Go(func() error {
		var failedCompany, failedTitle, failedStatus sql.NullString
		var failedAt, failedDiscoveredAt sql.NullTime
		err := db.QueryRow(`SELECT company_name, job_title, status, last_updated, discovered_at FROM job_funnel
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
		return nil
	})

	g.Go(func() error {
		var manualCompany, manualTitle sql.NullString
		var manualAt, manualDiscoveredAt sql.NullTime
		err := db.QueryRow(`SELECT company_name, job_title, last_updated, discovered_at FROM job_funnel
			WHERE status IN ('MANUAL_REQUIRED', 'AWAITING_REVIEW') ORDER BY last_updated DESC LIMIT 1`).
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
		return nil
	})

	g.Go(func() error {
		var interviews, rejections int
		err := db.QueryRow(`SELECT
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
		return nil
	})

	g.Go(func() error {
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
			return nil
		}
		defer sourceRows.Close()

		var bySource []SourceConversionStat
		for sourceRows.Next() {
			var s SourceConversionStat
			if err := sourceRows.Scan(&s.Source, &s.TotalApplied, &s.Interviews, &s.Rejections, &s.Pending); err != nil {
				log.Printf("Failed to scan conversion-by-source row: %v", err)
				continue
			}
			if s.TotalApplied > 0 {
				s.InterviewRate = fmt.Sprintf("%.1f%%", float64(s.Interviews)/float64(s.TotalApplied)*100)
			}
			bySource = append(bySource, s)
		}
		m.BySource = bySource
		return nil
	})

	g.Go(func() error {
		variantRows, err := db.Query(`SELECT
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
			log.Printf("Failed to query conversion stats by variant: %v", err)
			return nil
		}
		defer variantRows.Close()

		var byVariant []VariantConversionStat
		for variantRows.Next() {
			var s VariantConversionStat
			if err := variantRows.Scan(&s.Variant, &s.TotalApplied, &s.Interviews, &s.Rejections, &s.Pending); err != nil {
				log.Printf("Failed to scan conversion-by-variant row: %v", err)
				continue
			}
			if s.TotalApplied > 0 {
				s.InterviewRate = fmt.Sprintf("%.1f%%", float64(s.Interviews)/float64(s.TotalApplied)*100)
			}
			byVariant = append(byVariant, s)
		}
		m.ByVariant = byVariant
		return nil
	})

	_ = g.Wait()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(m)
}

// Removed serveDashboard and serveFavicon as they are now handled by http.FileServer

func serveAgentStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	
	err := exec.Command("pgrep", "-f", "career_agent_bin").Run()
	if err == nil {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status": "already_running"}`))
		return
	}

	cmd := exec.Command("./career_agent_bin", "-daemon", "-cycle-limit", "5")
	if err := cmd.Start(); err != nil {
		http.Error(w, fmt.Sprintf("Failed to start agent: %v", err), http.StatusInternalServerError)
		return
	}
	
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"status": "started"}`))
}

func serveAgentStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	_ = exec.Command("pkill", "-f", "career_agent_bin").Run()
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"status": "stopped"}`))
}

func serveAgentStatus(w http.ResponseWriter, r *http.Request) {
	err := exec.Command("pgrep", "-f", "career_agent_bin").Run()
	running := err == nil
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"running": running})
}

