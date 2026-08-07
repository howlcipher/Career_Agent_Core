// cmd/backfill-location fills in job_location and is_remote for funnel rows
// that were discovered before there was any geographic gate, by reading the
// same public board feeds discovery already reads.
//
// Why this exists (bugs.md #524): bug #516 added country extraction and a
// geographic gate, but only for postings discovered from that point on. Every
// row already in the funnel kept an empty job_location — measured live
// 2026-08-06, 0 of the 524 rows in the Assisted Apply queue carried one — so
// nothing could screen them and the operator had to open each posting and
// check its country by hand before launching. That is not hypothetical: an
// India-scoped role reached a live application attempt during the acceptance
// trial. The display path was already wired end to end (pkg/storage/assisted.go
// selects jf.job_location into AssistedJob.Location and the dashboard renders
// it on every queue card); the column was simply empty.
//
// This is a one-off maintenance tool, safe to re-run: a row that already has a
// location is skipped without a fetch.
//
// Usage:
//
//	go run ./cmd/backfill-location                            # dry run, assisted queue
//	go run ./cmd/backfill-location -confirm                   # write, assisted queue
//	go run ./cmd/backfill-location -scope discovered -confirm # DISCOVERED rows instead
//	go run ./cmd/backfill-location -scope all -confirm -concurrency 10
//
// Without -confirm nothing is written; the feeds are still fetched and the
// full report is still printed, so a dry run shows exactly what a real run
// would do.
package main

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"net/url"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/howlcipher/Career_Agent_Core/pkg/config"
	"github.com/howlcipher/Career_Agent_Core/pkg/scraper"
	"github.com/howlcipher/Career_Agent_Core/pkg/security"
	"github.com/howlcipher/Career_Agent_Core/pkg/storage"
)

// Retry policy, following pollBoard's in pkg/scraper/atsfeeds.go: a 4xx other
// than 429 is a settled answer and must not be retried, everything else gets
// exponential backoff.
//
// 429 gets its own, much longer schedule, which pollBoard does not need and
// this tool does. Discovery reads one feed per company; the backfill reads one
// feed per company *and then* asks individually about every posting that feed
// did not list, which on the live queue meant a burst of per-posting requests
// at a single host. The first live dry run had 76 rows come back unresolved on
// rate limiting alone — indistinguishable, in the report, from a dead board —
// which is why the summary now breaks failures down by cause rather than
// lumping them into one number.
const (
	maxFeedAttempts  = 4
	retryBackoffBase = 2 * time.Second
	rateLimitBackoff = 5 * time.Second

	// longRateLimit separates "wait a moment" from "come back tomorrow". Above
	// it, a Retry-After means the host has shut us out rather than throttled a
	// burst, and no amount of patience within one run will change that.
	longRateLimit = 2 * time.Minute
)

type funnelRow struct {
	URL      string
	Company  string
	Title    string
	Location string
}

type accountKey struct {
	Board   string
	Account string
}

type candidate struct {
	Row funnelRow
	Ref scraper.BoardRef
}

type resolved struct {
	Row  funnelRow
	Job  scraper.FeedJob
	From string // "account feed" or "per-job lookup"
}

type report struct {
	InScope         int
	AlreadyLocated  int
	Unsupported     int
	Resolved        []resolved
	Expired         []funnelRow
	Unresolved      int            // board did not answer; nothing changed
	UnresolvedWhy   map[string]int // failureClass -> row count
	WriteFailures   int
	StatusFailures  int
	WouldBeRejected []string
}

func (r *report) unresolvedRows(err error, rows int) {
	r.Unresolved += rows
	if r.UnresolvedWhy == nil {
		r.UnresolvedWhy = map[string]int{}
	}
	r.UnresolvedWhy[failureClass(err)] += rows
}

func main() {
	if err := security.PreparePrivateWorkspace(".", os.Stderr); err != nil {
		log.Fatalf("Startup aborted because private paths could not be secured: %v", err)
	}

	scope := flag.String("scope", "queue", "which rows to backfill: queue (open assisted applications), discovered (status=DISCOVERED), or all")
	confirm := flag.Bool("confirm", false, "write the resolved locations and expiries; without it this is a dry run")
	concurrency := flag.Int("concurrency", 6, "maximum concurrent board fetches")
	profilePath := flag.String("profile", "profile.yaml", "path to profile.yaml, read only for the screening report")
	flag.Parse()

	switch *scope {
	case "queue", "discovered", "all":
	default:
		log.Fatalf("invalid -scope %q: want queue, discovered, or all", *scope)
	}
	if *concurrency < 1 {
		*concurrency = 1
	}

	if err := storage.InitDB(); err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer storage.CloseDB()

	rows, err := loadRows(*scope)
	if err != nil {
		log.Fatalf("Failed to load %s rows: %v", *scope, err)
	}

	rep := report{InScope: len(rows)}

	// A row that already carries a location is done. Skipping it here is what
	// makes this tool re-runnable without re-fetching work already finished.
	grouped := map[accountKey][]candidate{}
	for _, row := range rows {
		if row.Location != "" {
			rep.AlreadyLocated++
			continue
		}
		ref, ok := scraper.ParseBoardRef(row.URL)
		if !ok {
			// Workday, SmartRecruiters, Ashby, Jobvite and the rest publish no
			// feed we can read. Leave those rows exactly as they are.
			rep.Unsupported++
			continue
		}
		key := accountKey{Board: ref.Board, Account: ref.Account}
		grouped[key] = append(grouped[key], candidate{Row: row, Ref: ref})
	}

	feeds := fetchAccountFeeds(grouped, *concurrency)

	for key, candidates := range grouped {
		feed, ok := feeds[key]
		if feed.err != nil || !ok {
			// The account itself did not answer. Absence proves nothing here,
			// so nothing is written and nothing is terminalized.
			rep.unresolvedRows(feed.err, len(candidates))
			continue
		}
		for _, c := range candidates {
			if job, found := feed.jobs[c.Ref.JobID]; found {
				rep.Resolved = append(rep.Resolved, resolved{Row: c.Row, Job: job, From: "account feed"})
				continue
			}
			// Absent from a feed that answered cleanly. That is a strong hint
			// the posting is gone, but not proof: a board can carry postings
			// reachable by direct link and not listed publicly, and
			// terminalizing one of those would repeat the mistake bugs.md
			// warns about under the CAPTCHA pre-skip decision — skipping a job
			// that would have submitted is strictly worse than wasting
			// inference. Ask the posting itself before writing it off.
			job, live, err := lookupOnePosting(c.Ref)
			switch {
			case err != nil:
				rep.unresolvedRows(err, 1)
			case live:
				rep.Resolved = append(rep.Resolved, resolved{Row: c.Row, Job: job, From: "per-job lookup"})
			default:
				rep.Expired = append(rep.Expired, c.Row)
			}
		}
	}

	if *confirm {
		applyWrites(&rep)
	}

	screen(&rep, *profilePath)
	printReport(&rep, *scope, *confirm)
}

// applyWrites is the only function in this tool that mutates the database.
func applyWrites(rep *report) {
	for _, r := range rep.Resolved {
		location := r.Job.Location
		if location == "" {
			// No free text, but the feed named countries: record those rather
			// than leaving the column empty, so the card still says something.
			location = strings.Join(r.Job.CountryCodes, ", ")
		}
		if err := storage.UpdateFunnelIdentity(r.Row.URL, location, r.Job.Remote); err != nil {
			log.Printf("Could not record the advertised location for %s — %s: %v", r.Row.Company, r.Row.Title, err)
			rep.WriteFailures++
		}
	}
	for _, row := range rep.Expired {
		if err := storage.UpdateFunnelStatusInvalid(row.URL, storage.InvalidURLReasonExpired); err != nil {
			log.Printf("Could not mark %s — %s as expired: %v", row.Company, row.Title, err)
			rep.StatusFailures++
		}
	}
}

// screen evaluates the resolved rows against the profile's country allowlist
// and reports which ones the #516 gate would have rejected at discovery.
//
// It never mutates a row, and must not be made to, even under -confirm. These
// rows are already in the operator's queue, and bugs.md records the governing
// lesson that skipping a job that would have submitted is strictly worse than
// wasting inference. The backfill's job is to put the location in front of the
// operator — the queue card now shows it either way — not to decide for them.
func screen(rep *report, profilePath string) {
	prof, err := config.LoadProfile(profilePath)
	if err != nil {
		log.Printf("Screening report skipped: could not read %s: %v", profilePath, err)
		return
	}
	if len(prof.AllowedCountries) == 0 {
		log.Printf("Screening report skipped: %s configures no allowed_countries, so the gate is off.", profilePath)
		return
	}
	for _, r := range rep.Resolved {
		if allowed, reason := scraper.LocationAllowed(r.Job.Location, r.Job.CountryCodes, prof.AllowedCountries); !allowed {
			rep.WouldBeRejected = append(rep.WouldBeRejected,
				fmt.Sprintf("%s — %s — %s", r.Row.Company, r.Row.Title, reason))
		}
	}
	sort.Strings(rep.WouldBeRejected)
}

func printReport(rep *report, scope string, confirm bool) {
	if len(rep.WouldBeRejected) > 0 {
		fmt.Printf("\nOutside the configured region (%d) — reported only, nothing was changed for these:\n", len(rep.WouldBeRejected))
		for _, line := range rep.WouldBeRejected {
			fmt.Printf("  %s\n", line)
		}
	}

	fmt.Printf("\nLocation backfill — scope %s\n", scope)
	fmt.Printf("  rows in scope:            %d\n", rep.InScope)
	fmt.Printf("  already had a location:   %d\n", rep.AlreadyLocated)
	fmt.Printf("  location resolved:        %d\n", len(rep.Resolved))
	fmt.Printf("  expired, marked dead:     %d\n", len(rep.Expired))
	fmt.Printf("  board did not answer:     %d%s\n", rep.Unresolved, formatWhy(rep.UnresolvedWhy))
	fmt.Printf("  no readable board feed:   %d\n", rep.Unsupported)
	fmt.Printf("  outside allowed region:   %d\n", len(rep.WouldBeRejected))
	if rep.WriteFailures > 0 || rep.StatusFailures > 0 {
		fmt.Printf("  write failures:           %d location, %d status\n", rep.WriteFailures, rep.StatusFailures)
	}
	if !confirm {
		fmt.Println("\nDry run: nothing was written. Re-run with -confirm to apply.")
	}
}

// formatWhy renders the unresolved breakdown inline, e.g. " (HTTP 429 76, HTTP
// 404 10)". Rate limiting and a genuinely dead board are very different
// findings and the first dry run could not tell them apart.
func formatWhy(why map[string]int) string {
	if len(why) == 0 {
		return ""
	}
	classes := make([]string, 0, len(why))
	for class := range why {
		classes = append(classes, class)
	}
	sort.Slice(classes, func(i, j int) bool { return why[classes[i]] > why[classes[j]] })
	parts := make([]string, 0, len(classes))
	for _, class := range classes {
		parts = append(parts, fmt.Sprintf("%s %d", class, why[class]))
	}
	return " (" + strings.Join(parts, ", ") + ")"
}

type feedResult struct {
	jobs map[string]scraper.FeedJob
	err  error
}

// fetchAccountFeeds fetches each distinct board account exactly once. Grouping
// first is what keeps this cheap: the live queue's 449 feed-backed rows sit on
// 264 accounts, and Workable alone accounts for 361 rows across 210.
func fetchAccountFeeds(grouped map[accountKey][]candidate, concurrency int) map[accountKey]feedResult {
	results := map[accountKey]feedResult{}
	var mu sync.Mutex
	var eg errgroup.Group
	eg.SetLimit(concurrency)

	for key, candidates := range grouped {
		key, ref := key, candidates[0].Ref
		eg.Go(func() error {
			var res feedResult
			body, err := fetchWithRetry(scraper.AccountFeedURL(ref))
			if err != nil {
				res.err = err
			} else {
				res.jobs, res.err = scraper.ParseAccountFeed(key.Board, body)
			}
			mu.Lock()
			results[key] = res
			mu.Unlock()
			return nil
		})
	}
	_ = eg.Wait()
	return results
}

// lookupOnePosting asks a single posting whether it still exists. A 404 is the
// answer this is here for: it distinguishes a genuinely expired posting from
// one the board simply does not list.
func lookupOnePosting(ref scraper.BoardRef) (scraper.FeedJob, bool, error) {
	body, err := fetchWithRetry(scraper.JobFeedURL(ref))
	if err != nil {
		if isNotFound(err) {
			return scraper.FeedJob{}, false, nil
		}
		return scraper.FeedJob{}, false, err
	}
	return scraper.ParseJobFeed(ref.Board, body)
}

func isNotFound(err error) bool {
	return err != nil && strings.Contains(err.Error(), "HTTP 404")
}

func fetchWithRetry(endpoint string) ([]byte, error) {
	if endpoint == "" {
		return nil, fmt.Errorf("no feed endpoint for this board")
	}
	host := hostOf(endpoint)
	if until, blocked := blockedUntil(host); blocked {
		return nil, fmt.Errorf("board feed returned HTTP 429 (host blocked for another %s)", until.Round(time.Second))
	}

	var err error
	for attempt := 1; attempt <= maxFeedAttempts; attempt++ {
		var body []byte
		body, err = scraper.FetchAccountFeed(endpoint)
		if err == nil {
			return body, nil
		}

		var httpErr *scraper.FeedHTTPError
		if errors.As(err, &httpErr) {
			// A host that names a long Retry-After is not throttling
			// individual requests, it has shut us out. Retrying cannot
			// succeed and only keeps hitting a host that said stop, so give
			// the whole host up for this run and let every remaining row on it
			// report as rate-limited. Measured live: Workable answered every
			// path on apply.workable.com with Retry-After 84643 (23.5 hours).
			if httpErr.RetryAfter > longRateLimit {
				blockHost(host, httpErr.RetryAfter)
				return nil, err
			}
			// Any other 4xx is a settled answer; retrying only wastes time.
			if httpErr.StatusCode >= 400 && httpErr.StatusCode < 500 && httpErr.StatusCode != 429 {
				return nil, err
			}
		}

		if attempt == maxFeedAttempts {
			break
		}
		base := retryBackoffBase
		if strings.Contains(err.Error(), "HTTP 429") {
			base = rateLimitBackoff
		}
		time.Sleep(time.Duration(1<<(attempt-1)) * base)
	}
	return nil, err
}

// Hosts given up for the rest of the run, with when they said they would be
// ready again. Shared across the concurrent feed fetches.
var (
	blockedHostsMu sync.Mutex
	blockedHosts   = map[string]time.Time{}
)

func blockHost(host string, retryAfter time.Duration) {
	if host == "" {
		return
	}
	blockedHostsMu.Lock()
	defer blockedHostsMu.Unlock()
	if _, already := blockedHosts[host]; !already {
		readyAt := time.Now().Add(retryAfter)
		blockedHosts[host] = readyAt
		log.Printf("%s is rate-limiting every request and asked for %s; skipping it for the rest of this run. Re-run after %s.",
			host, retryAfter.Round(time.Minute), readyAt.Format(time.RFC1123))
	}
}

func blockedUntil(host string) (time.Duration, bool) {
	blockedHostsMu.Lock()
	defer blockedHostsMu.Unlock()
	readyAt, ok := blockedHosts[host]
	if !ok {
		return 0, false
	}
	return time.Until(readyAt), true
}

func hostOf(endpoint string) string {
	u, err := url.Parse(endpoint)
	if err != nil {
		return ""
	}
	return strings.ToLower(u.Host)
}

// failureClass reduces a fetch error to something countable, so the report can
// say *why* rows went unresolved. It deliberately keeps only the status code or
// error kind: an endpoint URL identifies a posting, and this output is meant to
// be safe to paste into a backlog entry.
func failureClass(err error) string {
	if err == nil {
		return "none"
	}
	msg := err.Error()
	for _, code := range []string{"429", "403", "404", "410", "500", "502", "503", "504"} {
		if strings.Contains(msg, "HTTP "+code) {
			return "HTTP " + code
		}
	}
	switch {
	case strings.Contains(msg, "context deadline exceeded"), strings.Contains(msg, "Timeout"), strings.Contains(msg, "timeout"):
		return "timeout"
	case strings.Contains(msg, "no such host"):
		return "DNS failure"
	case strings.Contains(msg, "unexpected end of JSON"), strings.Contains(msg, "parse "):
		return "unparseable body"
	}
	return "other network error"
}

func loadRows(scope string) ([]funnelRow, error) {
	db := storage.GetDB()
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var queries []string
	if scope == "queue" || scope == "all" {
		queries = append(queries, `
			SELECT COALESCE(jf.url, ''), COALESCE(jf.company_name, ''), COALESCE(jf.job_title, ''), COALESCE(jf.job_location, '')
			FROM assisted_applications aa JOIN job_funnel jf ON jf.id = aa.job_id
			WHERE aa.assisted_state != 'completed'`)
	}
	if scope == "discovered" || scope == "all" {
		queries = append(queries, `
			SELECT COALESCE(url, ''), COALESCE(company_name, ''), COALESCE(job_title, ''), COALESCE(job_location, '')
			FROM job_funnel WHERE status = 'DISCOVERED'`)
	}

	var out []funnelRow
	seen := map[string]bool{}
	for _, query := range queries {
		rows, err := db.Query(query)
		if err != nil {
			return nil, fmt.Errorf("query rows: %w", err)
		}
		for rows.Next() {
			var row funnelRow
			if err := rows.Scan(&row.URL, &row.Company, &row.Title, &row.Location); err != nil {
				rows.Close()
				return nil, fmt.Errorf("scan row: %w", err)
			}
			row.URL = strings.TrimSpace(row.URL)
			row.Location = strings.TrimSpace(row.Location)
			if row.URL == "" || seen[row.URL] {
				continue
			}
			seen[row.URL] = true
			out = append(out, row)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return nil, fmt.Errorf("iterate rows: %w", err)
		}
	}
	return out, nil
}
