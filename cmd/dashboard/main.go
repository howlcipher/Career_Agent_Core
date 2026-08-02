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
	"sort"
	"strconv"
	"strings"
	"syscall"
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
	ManualRequiredOnly int `json:"manual_required_only"`
	AwaitingReview     int `json:"awaiting_review"`
	BlockedCaptcha     int `json:"blocked_captcha"`
	InvalidURL         int `json:"invalid_url"`
	// InvalidURLMalformed and InvalidURLExpired split the InvalidURL total
	// (improvements.md #468) the same way bug #451 split Failed and
	// ManualRequired: a live measurement against applications.db found ~88%
	// of INVALID_URL rows are real postings checkJobAlive correctly caught as
	// expired, not the malformed/never-a-posting shape the old single caption
	// described.
	InvalidURLMalformed int `json:"invalid_url_malformed"`
	InvalidURLExpired   int `json:"invalid_url_expired"`
	// RetryExhausted counts job_funnel rows that spent MaxRetryAttempts
	// (bugs.md #466) without succeeding. It had no dashboard presence at all
	// before improvements.md #468 — the count silently dropped out of every
	// bucket's total.
	RetryExhausted            int                            `json:"retry_exhausted"`
	AssistedWaiting           int                            `json:"assisted_waiting"`
	ConfirmedToday            int                            `json:"confirmed_today"`
	ConfirmedLast7Days        int                            `json:"confirmed_last_7_days"`
	FirstAttemptMedian        string                         `json:"first_attempt_median,omitempty"`
	LastConfirmedAgo          string                         `json:"last_confirmed_ago,omitempty"`
	EligibleQueue             int                            `json:"eligible_queue"`
	EligibleNeverAttempted    int                            `json:"eligible_never_attempted"`
	WatchdogAlert             string                         `json:"watchdog_alert,omitempty"`
	WatchdogAlertAt           string                         `json:"watchdog_alert_at,omitempty"`
	DiscoveryLastFinishedAt   string                         `json:"discovery_last_finished_at,omitempty"`
	DiscoveryNewEligible      int                            `json:"discovery_new_eligible"`
	DiscoveryErrorClass       string                         `json:"discovery_error_class,omitempty"`
	DiscoverySourceCounts     []storage.DiscoverySourceCount `json:"discovery_source_counts,omitempty"`
	LastAppliedCompany        string                         `json:"last_applied_company,omitempty"`
	LastAppliedTitle          string                         `json:"last_applied_title,omitempty"`
	LastAppliedURL            string                         `json:"last_applied_url,omitempty"`
	LastAppliedAt             string                         `json:"last_applied_at,omitempty"`
	LastAppliedProcessingTime string                         `json:"last_applied_processing_time,omitempty"`

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

// conversionRows is the subset of *sql.Rows that scanSourceConversions and
// scanVariantConversions need, factored out so a hand-rolled fake can stand
// in for tests that need Next() to fail mid-stream rather than exhaust
// normally -- a shape *sql.Rows itself cannot be made to produce on demand
// against a real driver.
type conversionRows interface {
	Next() bool
	Scan(dest ...any) error
	Err() error
}

// scanSourceConversions drains rows into SourceConversionStat, computing
// each row's interview rate. Next() returning false can mean either "rows
// exhausted" or "a cursor error occurred" -- database/sql cannot tell the
// caller apart without an explicit Err() check, so a fault partway through
// the result set must not be mistaken for a complete, if short, breakdown.
func scanSourceConversions(rows conversionRows) ([]SourceConversionStat, error) {
	var bySource []SourceConversionStat
	for rows.Next() {
		var s SourceConversionStat
		if err := rows.Scan(&s.Source, &s.TotalApplied, &s.Interviews, &s.Rejections, &s.Pending); err != nil {
			log.Printf("Failed to scan conversion-by-source row: %v", err)
			continue
		}
		if s.TotalApplied > 0 {
			s.InterviewRate = fmt.Sprintf("%.1f%%", float64(s.Interviews)/float64(s.TotalApplied)*100)
		}
		bySource = append(bySource, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("scan conversion stats by source: %w", err)
	}
	return bySource, nil
}

// scanVariantConversions mirrors scanSourceConversions for the by-variant
// breakdown; see its doc comment for why the Err() check is required.
func scanVariantConversions(rows conversionRows) ([]VariantConversionStat, error) {
	var byVariant []VariantConversionStat
	for rows.Next() {
		var s VariantConversionStat
		if err := rows.Scan(&s.Variant, &s.TotalApplied, &s.Interviews, &s.Rejections, &s.Pending); err != nil {
			log.Printf("Failed to scan conversion-by-variant row: %v", err)
			continue
		}
		if s.TotalApplied > 0 {
			s.InterviewRate = fmt.Sprintf("%.1f%%", float64(s.Interviews)/float64(s.TotalApplied)*100)
		}
		byVariant = append(byVariant, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("scan conversion stats by variant: %w", err)
	}
	return byVariant, nil
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
	case "RETRY_EXHAUSTED":
		return "Failed the same retryable error 5 times in a row — requeue with cmd/requeue -status RETRY_EXHAUSTED -confirm"
	default:
		return status
	}
}

func statusReasonWithDetail(status, detail string) string {
	if status == "SKIPPED" {
		switch detail {
		case storage.SkippedReasonExcludedSource:
			return "Excluded ATS source — not eligible for automated submission"
		case storage.SkippedReasonDuplicateCooldown:
			return "Equivalent recent application is within your configured cooldown"
		}
	}
	return statusReason(status)
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
	"RETRY_EXHAUSTED",
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
//
// It uses ReaderDSN, not DSN: this connection only ever queries the database,
// it never owns schema setup (cmd/agent's storage.InitDB does, via the
// separate writer DSN), and asking to change journal_mode from a read-only
// connection is exactly what bug #450 found could fail outright against a
// genuinely fresh database with a writer's transaction already open.
var dashboardDSN = storage.ReaderDSN(storage.DefaultDatabasePath)

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
	// cmd/agent held write locks. storage.ReaderDSN is the single source of
	// truth for this connection's pragmas (bug #450).
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
	mux.HandleFunc("/api/assisted", serveAssistedQueue)
	mux.HandleFunc("/api/assisted/confirm", requireSameOrigin(serveAssistedConfirm))
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
			COALESCE(SUM(CASE WHEN applied_at >= datetime('now', 'start of day') THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN applied_at >= datetime('now', '-6 days', 'start of day') THEN 1 ELSE 0 END), 0)
			FROM job_funnel
			WHERE status IN ('APPLIED', 'PROCESSED_MANUAL') AND applied_at IS NOT NULL`).
			Scan(&m.ConfirmedToday, &m.ConfirmedLast7Days)
		if err != nil {
			return fmt.Errorf("query confirmed application cadence: %w", err)
		}
		var lastConfirmedAt sql.NullTime
		err = db.QueryRow(`SELECT applied_at FROM job_funnel
			WHERE status IN ('APPLIED', 'PROCESSED_MANUAL') AND applied_at IS NOT NULL
			ORDER BY applied_at DESC LIMIT 1`).Scan(&lastConfirmedAt)
		if err != nil && err != sql.ErrNoRows {
			return fmt.Errorf("query last confirmed application: %w", err)
		}
		if lastConfirmedAt.Valid {
			m.LastConfirmedAgo = formatDuration(time.Since(lastConfirmedAt.Time))
		}
		return nil
	})

	g.Go(func() error {
		var exists int
		if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'assisted_applications'`).Scan(&exists); err != nil {
			return fmt.Errorf("check assisted queue schema: %w", err)
		}
		if exists == 0 {
			return nil
		}
		return db.QueryRow(`SELECT COUNT(*) FROM assisted_applications WHERE assisted_state != 'completed'`).Scan(&m.AssistedWaiting)
	})

	g.Go(func() error {
		var exists int
		if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'discovery_refresh'`).Scan(&exists); err != nil {
			return fmt.Errorf("check discovery refresh schema: %w", err)
		}
		if exists == 0 {
			return nil
		}
		var finishedAt sql.NullTime
		err := db.QueryRow(`SELECT finished_at, new_eligible, error_class FROM discovery_refresh WHERE id = 1`).
			Scan(&finishedAt, &m.DiscoveryNewEligible, &m.DiscoveryErrorClass)
		if err == sql.ErrNoRows {
			return nil
		}
		if err != nil {
			return fmt.Errorf("query discovery refresh: %w", err)
		}
		if finishedAt.Valid {
			m.DiscoveryLastFinishedAt = finishedAt.Time.Local().Format("Jan 2, 3:04 PM")
		}

		// Dashboard tests and old standalone dashboard deployments may open a
		// database before the agent's additive migration has run. Keep the
		// existing refresh summary available in that state; source health is
		// simply absent until the next agent startup upgrades the table.
		var sourceCountsJSON sql.NullString
		err = db.QueryRow(`SELECT source_counts_json FROM discovery_refresh WHERE id = 1`).Scan(&sourceCountsJSON)
		if err != nil && !strings.Contains(strings.ToLower(err.Error()), "no such column") {
			return fmt.Errorf("query discovery source counts: %w", err)
		}
		if err == nil && sourceCountsJSON.Valid && sourceCountsJSON.String != "" && sourceCountsJSON.String != "null" {
			if err := json.Unmarshal([]byte(sourceCountsJSON.String), &m.DiscoverySourceCounts); err != nil {
				return fmt.Errorf("decode discovery source counts: %w", err)
			}
		}
		return nil
	})

	g.Go(func() error {
		var exists int
		if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'daemon_watchdog_alert'`).Scan(&exists); err != nil {
			return fmt.Errorf("check daemon watchdog alert schema: %w", err)
		}
		if exists == 0 {
			return nil
		}
		var updatedAt sql.NullTime
		err := db.QueryRow(`SELECT message, updated_at FROM daemon_watchdog_alert WHERE id = 1`).
			Scan(&m.WatchdogAlert, &updatedAt)
		if err == sql.ErrNoRows {
			return nil
		}
		if err != nil {
			return fmt.Errorf("query daemon watchdog alert: %w", err)
		}
		if updatedAt.Valid {
			m.WatchdogAlertAt = updatedAt.Time.Local().Format("Jan 2, 3:04 PM")
		}
		return nil
	})

	g.Go(func() error {
		rows, err := db.Query(`SELECT jf.url, jf.discovered_at, aa.started_at
			FROM job_funnel jf
			JOIN application_attempts aa ON aa.url = jf.url
			WHERE jf.discovered_at IS NOT NULL AND aa.started_at IS NOT NULL`)
		if err != nil {
			return fmt.Errorf("query first-attempt latency: %w", err)
		}
		defer rows.Close()

		firstAttempts := make(map[string]time.Time)
		discoveredTimes := make(map[string]time.Time)
		for rows.Next() {
			var url string
			var discoveredAt, startedAt time.Time
			if err := rows.Scan(&url, &discoveredAt, &startedAt); err != nil {
				return fmt.Errorf("scan first-attempt latency: %w", err)
			}
			if earliest, found := firstAttempts[url]; !found || startedAt.Before(earliest) {
				firstAttempts[url] = startedAt
				discoveredTimes[url] = discoveredAt
			}
		}
		if err := rows.Err(); err != nil {
			return fmt.Errorf("iterate first-attempt latency: %w", err)
		}
		if len(firstAttempts) == 0 {
			return nil
		}
		latencies := make([]time.Duration, 0, len(firstAttempts))
		for url, firstAttempt := range firstAttempts {
			discoveredAt := discoveredTimes[url]
			if firstAttempt.Before(discoveredAt) {
				continue
			}
			latencies = append(latencies, firstAttempt.Sub(discoveredAt))
		}
		if len(latencies) == 0 {
			return nil
		}
		sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
		middle := len(latencies) / 2
		median := latencies[middle]
		if len(latencies)%2 == 0 {
			median = (latencies[middle-1] + latencies[middle]) / 2
		}
		m.FirstAttemptMedian = formatDuration(median)
		return nil
	})

	g.Go(func() error {
		err := db.QueryRow(`SELECT
			COUNT(*),
			COALESCE(SUM(CASE WHEN NOT EXISTS (
				SELECT 1 FROM application_attempts aa WHERE aa.url = jf.url
			) THEN 1 ELSE 0 END), 0)
			FROM job_funnel jf
			WHERE jf.status = 'DISCOVERED'
				AND jf.url NOT LIKE '%breezy.hr%'
				AND (jf.next_eligible_at IS NULL OR jf.next_eligible_at <= ?)`, time.Now().UTC()).
			Scan(&m.EligibleQueue, &m.EligibleNeverAttempted)
		if err != nil {
			return fmt.Errorf("query eligible queue: %w", err)
		}
		return nil
	})

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
			COALESCE(SUM(CASE WHEN status = 'INVALID_URL' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN status = 'INVALID_URL' AND status_reason = ? THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN status = 'INVALID_URL' AND status_reason = ? THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN status = 'RETRY_EXHAUSTED' THEN 1 ELSE 0 END), 0)
		FROM job_funnel`, storage.InvalidURLReasonMalformed, storage.InvalidURLReasonExpired).Scan(
			&m.Discovered, &m.Processing, &m.Skipped, &m.Applied,
			&m.Failed, &m.FailedScore, &m.FailedSubmit,
			&m.ManualRequired, &m.ManualRequiredOnly, &m.AwaitingReview,
			&m.BlockedCaptcha, &m.InvalidURL,
			&m.InvalidURLMalformed, &m.InvalidURLExpired, &m.RetryExhausted,
		)
		if err != nil {
			return fmt.Errorf("query basic counts: %w", err)
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
			return fmt.Errorf("query last applied job: %w", err)
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
			return fmt.Errorf("query currently processing job: %w", err)
		}
		m.CurrentCompany = currentCompany.String
		m.CurrentTitle = currentTitle.String
		if currentSince.Valid {
			m.CurrentSince = currentSince.Time.Local().Format("3:04:05 PM")
		}
		return nil
	})

	g.Go(func() error {
		var skippedCompany, skippedTitle, skippedStatus, skippedDetail sql.NullString
		var skippedAt, skippedDiscoveredAt sql.NullTime
		err := db.QueryRow(`SELECT company_name, job_title, status, status_reason, last_updated, discovered_at FROM job_funnel
			WHERE status = 'SKIPPED' ORDER BY last_updated DESC LIMIT 1`).
			Scan(&skippedCompany, &skippedTitle, &skippedStatus, &skippedDetail, &skippedAt, &skippedDiscoveredAt)
		if err != nil && err != sql.ErrNoRows {
			return fmt.Errorf("query last skipped job: %w", err)
		}
		m.LastSkippedCompany = skippedCompany.String
		m.LastSkippedTitle = skippedTitle.String
		if skippedStatus.Valid {
			m.LastSkippedReason = statusReasonWithDetail(skippedStatus.String, skippedDetail.String)
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
			return fmt.Errorf("query last failed job: %w", err)
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
			return fmt.Errorf("query last manual-required job: %w", err)
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
			return fmt.Errorf("query conversion stats: %w", err)
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
			return fmt.Errorf("query conversion stats by source: %w", err)
		}
		defer sourceRows.Close()

		bySource, err := scanSourceConversions(sourceRows)
		if err != nil {
			return err
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
			return fmt.Errorf("query conversion stats by variant: %w", err)
		}
		defer variantRows.Close()

		byVariant, err := scanVariantConversions(variantRows)
		if err != nil {
			return err
		}
		m.ByVariant = byVariant
		return nil
	})

	// A real query failure must not answer 200 with whatever zero/stale
	// values the failed scans left behind (bug #452) -- that reads as a
	// confident "nothing has happened yet" to a user watching the
	// dashboard. sql.ErrNoRows is filtered out above at each call site
	// because an empty table is a legitimate state, not a failure.
	if err := g.Wait(); err != nil {
		log.Printf("serveMetrics: %v", err)
		http.Error(w, "failed to load metrics", http.StatusInternalServerError)
		return
	}

	m.StatusLegend = statusLegend()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(m)
}

// serveAssistedQueue exposes only the privacy-safe plan projection. URLs,
// document paths, browser state, and page content never leave the server.
func serveAssistedQueue(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	jobs, err := storage.GetAssistedQueue(db)
	if err != nil {
		log.Printf("serveAssistedQueue: %v", err)
		http.Error(w, "failed to load assisted applications", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"jobs": jobs})
}

func serveAssistedConfirm(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var request struct {
		JobID     string `json:"job_id"`
		Confirmed bool   `json:"confirmed"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil || !request.Confirmed || strings.TrimSpace(request.JobID) == "" {
		http.Error(w, "a confirmed assisted job identifier is required", http.StatusBadRequest)
		return
	}
	if err := storage.ConfirmAssistedSubmission(db, request.JobID); err != nil {
		log.Printf("serveAssistedConfirm: %v", err)
		http.Error(w, "unable to record application confirmation", http.StatusConflict)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"status":"confirmed"}`))
}

// Removed serveDashboard and serveFavicon as they are now handled by http.FileServer

// agentLockPath is cmd/agent's single-instance lock (bug #414), reused here
// as the source of truth for whether the agent is running.
const agentLockPath = "applications/career_agent.lock"

// agentPIDAt reports whether the agent's single-instance lock file is
// currently held, and the PID of the process holding it when the file's
// contents parse as one. This replaces identifying the agent via `pgrep -f
// career_agent_bin`, which false-positived on any process whose command line
// merely contained that substring - a `go build`, a `tail -f`, an editor with
// the file open - and whose `pkill -f` counterpart then killed the unrelated
// match (bug #449). A lock we can acquire ourselves means nothing holds it;
// we release it immediately since this call is a status check, not a claim.
func agentPIDAt(lockPath string) (pid int, running bool, err error) {
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0666)
	if err != nil {
		return 0, false, fmt.Errorf("open agent lock file: %w", err)
	}
	defer f.Close()

	if flockErr := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); flockErr != nil {
		// Held by another process: that is the agent. Its PID is whatever it
		// wrote when it acquired the lock; treat unreadable or unparsed
		// content as "running, PID unknown" rather than failing the check.
		data, readErr := os.ReadFile(lockPath)
		if readErr == nil {
			if parsed, parseErr := strconv.Atoi(strings.TrimSpace(string(data))); parseErr == nil {
				pid = parsed
			}
		}
		return pid, true, nil
	}
	syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	return 0, false, nil
}

func agentPID() (pid int, running bool, err error) {
	return agentPIDAt(agentLockPath)
}

func serveAgentStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	_, running, err := agentPID()
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to check agent status: %v", err), http.StatusInternalServerError)
		return
	}
	if running {
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

	// Signal the specific PID the lock file names, not a `pkill -f` substring
	// match: that pattern also matched a `go build`, a `tail -f`, or an editor
	// with the binary's name open, and killed whichever one it hit (bug
	// #449). If the PID is unknown, there is nothing safe to signal.
	pid, running, err := agentPID()
	if err != nil {
		log.Printf("serveAgentStop: could not determine agent status: %v", err)
	}
	if running && pid > 0 {
		if proc, findErr := os.FindProcess(pid); findErr == nil {
			if sigErr := proc.Signal(syscall.SIGTERM); sigErr != nil {
				log.Printf("serveAgentStop: failed to signal pid %d: %v", pid, sigErr)
			}
		}
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"status": "stopped"}`))
}

func serveAgentStatus(w http.ResponseWriter, r *http.Request) {
	_, running, err := agentPID()
	if err != nil {
		log.Printf("serveAgentStatus: %v", err)
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"running": running})
}
