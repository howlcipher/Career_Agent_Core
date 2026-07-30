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
	"net/url"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/howlcipher/Career_Agent_Core/pkg/security"
	"github.com/howlcipher/Career_Agent_Core/pkg/storage"
	_ "modernc.org/sqlite"
)

type Metrics struct {
	Discovered int `json:"discovered"`
	Processing int `json:"processing"`
	Skipped    int `json:"skipped"`
	Applied    int `json:"applied"`
	Failed     int `json:"failed"`
	// FailedScore and FailedSubmit split the Failed total by which of the two
	// unrelated statuses it counts (bug #451): the tile's number was already
	// correct, but its caption was hardcoded to one member of the pair.
	FailedScore    int `json:"failed_score"`
	FailedSubmit   int `json:"failed_submit"`
	ManualRequired int `json:"manual_required"`
	// ManualRequiredOnly and AwaitingReview split the ManualRequired total the
	// same way (bug #451) — the two statuses are "create an ATS account" and
	// "click submit on a form already filled", not interchangeable asks.
	ManualRequiredOnly        int    `json:"manual_required_only"`
	AwaitingReview            int    `json:"awaiting_review"`
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
	LastManualReason         string `json:"last_manual_reason,omitempty"`
	LastManualAt             string `json:"last_manual_at,omitempty"`
	LastManualProcessingTime string `json:"last_manual_processing_time,omitempty"`

	// StatusLegend explains every status the dashboard puts a tile or card on
	// screen for, keyed by the raw status code. The counted-only statuses
	// (BLOCKED_CAPTCHA, INVALID_URL) have no "last job" card to carry a reason
	// of their own, so without this they were numbers with no explanation -
	// see bug #435.
	StatusLegend map[string]string `json:"status_legend,omitempty"`

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

// explainedStatuses is every status the dashboard surfaces to the user, and so
// every status statusReason must have a real arm for. Bug #435: statusReason
// grew arms for MANUAL_REQUIRED, AWAITING_REVIEW, BLOCKED_CAPTCHA and
// INVALID_URL that nothing ever called, because the only two call sites were
// the SKIPPED and FAILED_* queries. Driving the legend off this list is what
// keeps the two counted-only statuses reachable; TestStatusLegend_CoversEvery
// ExplainedStatus fails if a status here has no arm.
var explainedStatuses = []string{
	"SKIPPED",
	"BLOCKED_CAPTCHA",
	"INVALID_URL",
	"FAILED_SCORE",
	"FAILED_SUBMIT",
	"MANUAL_REQUIRED",
	"AWAITING_REVIEW",
}

// statusLegend renders explainedStatuses into the map the UI reads.
func statusLegend() map[string]string {
	legend := make(map[string]string, len(explainedStatuses))
	for _, status := range explainedStatuses {
		legend[status] = statusReason(status)
	}
	return legend
}

//go:embed ui/dist
var uiDistFS embed.FS

var db *sql.DB

// dashboardDSN is the connection string this command opens. It is derived from
// pkg/storage rather than written out here, and TestDashboardDSNMatchesStorage
// pins it to that derivation: bug #446 was this command quietly keeping a DSN
// literal of its own, which then failed to follow pkg/storage when bug #416
// corrected the pragma syntax.
var dashboardDSN = storage.DSN(storage.DefaultDatabasePath)

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

// sameOriginViolation reports why a request must be rejected as
// cross-origin, or "" when it is acceptable. Bug #445: the agent
// start/stop endpoints validated nothing but the HTTP method, and a
// cross-origin `fetch(..., {method:'POST', mode:'no-cors'})` is a CORS
// *simple request* - no preflight is sent, the browser issues it, and the
// server acts on it. CORS only stops the attacking page from reading the
// response; by then the agent has already been launched and is submitting
// real applications with real PII. Binding the listener to loopback (bug
// #126) is no defence at all here, because the request originates from the
// user's own browser on the same machine.
//
// The returned string is a human-readable description of the offending
// header, so the caller can log it and the user can tell an attack from a
// misconfiguration.
func sameOriginViolation(r *http.Request) string {
	// Primary check: the fetch-metadata header. Every current browser sends
	// it on every request and JavaScript cannot set it, which is exactly
	// what makes it trustworthy. "same-origin" is our own dashboard page;
	// "none" is a user-initiated navigation (typing the URL, a bookmark).
	// "cross-site" and "same-site" both mean another page drove this.
	if site := r.Header.Get("Sec-Fetch-Site"); site != "" {
		if site == "same-origin" || site == "none" {
			return ""
		}
		return fmt.Sprintf("Sec-Fetch-Site: %q", site)
	}

	// Fallback for clients that do not send Sec-Fetch-Site (curl, older
	// browsers, this project's own scripts): the Origin header, then
	// Referer. Either one's host must match the host the request was
	// addressed to.
	if origin := r.Header.Get("Origin"); origin != "" {
		if hostMatchesRequest(origin, r.Host) {
			return ""
		}
		return fmt.Sprintf("Origin: %q (request Host: %q)", origin, r.Host)
	}
	if referer := r.Header.Get("Referer"); referer != "" {
		if hostMatchesRequest(referer, r.Host) {
			return ""
		}
		return fmt.Sprintf("Referer: %q (request Host: %q)", referer, r.Host)
	}

	// No Sec-Fetch-Site, no Origin, no Referer at all. Such a request did
	// not come from a browser page: every browser attaches at least one of
	// the three when a page issues a request, so this shape is curl, a
	// script, or the project's own tooling. Blocking it would break that
	// usage without closing the browser-driven attack this guard exists for,
	// so it is allowed through.
	return ""
}

// hostMatchesRequest reports whether an Origin/Referer value points at the
// same host the request was addressed to. It fails closed: anything that
// does not parse, or that parses with no host at all (a bare "null" Origin
// from a sandboxed iframe, or outright garbage), does not match.
func hostMatchesRequest(headerValue, requestHost string) bool {
	parsed, err := url.Parse(headerValue)
	if err != nil || parsed.Host == "" {
		return false
	}
	return strings.EqualFold(parsed.Host, requestHost)
}

// requireSameOrigin wraps a state-changing handler so a cross-origin caller
// is rejected before the handler runs at all.
func requireSameOrigin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if violation := sameOriginViolation(r); violation != "" {
			log.Printf(
				"Rejected cross-origin request to %s - %s",
				r.URL.Path, violation,
			)
			http.Error(
				w,
				"Forbidden: cross-origin requests are not allowed on this endpoint",
				http.StatusForbidden,
			)
			return
		}
		next(w, r)
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

	// The dashboard keeps its own connection rather than going through
	// pkg/storage, but it must not keep its own DSN: bug #446 was this line
	// carrying the mattn/go-sqlite3 pragma spelling that modernc.org/sqlite
	// ignores, which left this read-only connection with no busy timeout while
	// cmd/agent held write locks. storage.DSN is the single source of truth.
	db, err = sql.Open("sqlite", dashboardDSN)
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

	// /api/metrics and /api/agent/status are read-only GETs: they change no
	// state and launch no process, so a forged cross-origin request to them
	// achieves nothing the attacker could not already do, and gating them
	// would break scripted polling for no security gain. They are
	// deliberately left ungated.
	mux.HandleFunc("/api/metrics", serveMetrics)
	mux.HandleFunc("/api/agent/status", serveAgentStatus)

	// These two are state-changing: start launches the agent (which submits
	// real applications with real PII) and stop pkills it. Bug #445 - they
	// must never be drivable by another site's page in the user's browser.
	mux.HandleFunc("/api/agent/start", requireSameOrigin(serveAgentStart))
	mux.HandleFunc("/api/agent/stop", requireSameOrigin(serveAgentStop))

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
			COALESCE(SUM(CASE WHEN status = 'FAILED_SCORE' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN status = 'FAILED_SUBMIT' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN status IN ('MANUAL_REQUIRED', 'AWAITING_REVIEW') THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN status = 'MANUAL_REQUIRED' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN status = 'AWAITING_REVIEW' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN status = 'BLOCKED_CAPTCHA' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN status = 'INVALID_URL' THEN 1 ELSE 0 END), 0)
		FROM job_funnel`).Scan(
			&m.Discovered, &m.Processing, &m.Skipped, &m.Applied,
			&m.Failed, &m.FailedScore, &m.FailedSubmit,
			&m.ManualRequired, &m.ManualRequiredOnly, &m.AwaitingReview,
			&m.BlockedCaptcha, &m.InvalidURL,
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
		var manualCompany, manualTitle, manualStatus sql.NullString
		var manualAt, manualDiscoveredAt sql.NullTime
		// status is selected purely so the reason can be rendered: a
		// Copilot-filled job awaiting a click and an account-gated job the
		// agent can never submit both land in this queue and are entirely
		// different asks of the user (bug #435).
		err := db.QueryRow(`SELECT company_name, job_title, status, last_updated, discovered_at FROM job_funnel
			WHERE status IN ('MANUAL_REQUIRED', 'AWAITING_REVIEW') ORDER BY last_updated DESC LIMIT 1`).
			Scan(&manualCompany, &manualTitle, &manualStatus, &manualAt, &manualDiscoveredAt)
		if err != nil && err != sql.ErrNoRows {
			log.Printf("Failed to query last manual-required job: %v", err)
		}
		m.LastManualCompany = manualCompany.String
		m.LastManualTitle = manualTitle.String
		if manualStatus.Valid {
			m.LastManualReason = statusReason(manualStatus.String)
		}
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

	m.StatusLegend = statusLegend()

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

	// Keep the dashboard-launched agent actively draining its backlog while
	// retaining a short pause between source-refresh cycles to avoid a tight
	// retry loop when an upstream job board is unavailable.
	cmd := exec.Command(
		"./career_agent_bin",
		"-daemon",
		"-cycle-limit", "15",
		"-cycle-interval", "1m",
	)
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
