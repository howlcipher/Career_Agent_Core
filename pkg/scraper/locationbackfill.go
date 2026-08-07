package scraper

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// This file backs the one-off location backfill (bugs.md #524). Bug #516 added
// a geographic gate at discovery time, but only for postings discovered from
// that point on: every row already in the funnel kept an empty job_location, so
// nothing could screen the ~524 rows already sitting in the Assisted Apply
// queue and the operator had to open each posting and check its country by
// hand. Discovery reads board feeds *forward*; what follows reads the same
// feeds *backward*, resolving a stored posting URL to the account that can
// still answer where that posting is.
//
// Everything here is pure parsing plus URL construction. The network calls and
// the database writes live in cmd/backfill-location, so all of this is
// unit-testable against recorded fixtures with no network.

// BoardRef identifies which board account a stored posting URL belongs to, and
// the per-posting key that matches it to an entry in that account's feed.
type BoardRef struct {
	Board   string // "greenhouse", "lever", or "workable"
	Account string // the board slug / account name
	JobID   string // the per-posting key used to match a feed entry
}

// FeedJob is an exported alias for the feedJob discovery already parses, so
// cmd/backfill-location can read the same fields without this package growing
// a second, near-identical posting type.
type FeedJob = feedJob

// Account-level feeds return a company's whole current posting list; per-job
// endpoints answer for exactly one posting. The backfill needs both, and for
// different reasons — see AccountFeedURL and JobFeedURL. greenhouseBoardAPI and
// leverBoardAPI are declared in atsfeeds.go and reused here rather than
// restated, so discovery and the backfill can never drift onto different hosts.
//
// All five endpoints confirmed live 2026-08-06 against real accounts taken from
// the live queue.
const (
	workableAccountFeed = "https://apply.workable.com/api/v1/widget/accounts/%s?details=true"

	greenhouseJobAPI = "https://boards-api.greenhouse.io/v1/boards/%s/jobs/%s"
	leverJobAPI      = "https://api.lever.co/v0/postings/%s/%s"
	workableJobAPI   = "https://apply.workable.com/api/v1/accounts/%s/jobs/%s"
)

// ParseBoardRef resolves a stored posting URL to the account feed that can
// answer where the posting is located. ok is false for any URL from a board
// with no public feed (Workday, SmartRecruiters, Ashby, Jobvite, BambooHR,
// applytojob, recruitee, pinpointhq and anything else), and for a board index
// page that names no individual posting.
func ParseBoardRef(rawURL string) (BoardRef, bool) {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return BoardRef{}, false
	}

	host := strings.ToLower(u.Host)
	var parts []string
	for _, p := range strings.Split(strings.Trim(u.Path, "/"), "/") {
		if p = strings.TrimSpace(p); p != "" {
			parts = append(parts, p)
		}
	}

	switch {
	// https://job-boards.greenhouse.io/pointwild/jobs/5240015008
	// https://boards.eu.greenhouse.io/sportygroup/jobs/4725219101?gh_jid=...
	case host == "job-boards.greenhouse.io" || host == "boards.greenhouse.io" ||
		host == "job-boards.eu.greenhouse.io" || host == "boards.eu.greenhouse.io":
		if len(parts) >= 3 && parts[1] == "jobs" && isNumeric(parts[2]) {
			return BoardRef{Board: "greenhouse", Account: parts[0], JobID: parts[2]}, true
		}

	// https://jobs.lever.co/aircall/d9fc4c01-a12b-402e-94ee-9c3984a1fe79
	// https://jobs.eu.lever.co/foo/<uuid>/apply
	case host == "jobs.lever.co" || host == "jobs.eu.lever.co":
		if len(parts) >= 2 && isUUID(parts[1]) {
			return BoardRef{Board: "lever", Account: parts[0], JobID: strings.ToLower(parts[1])}, true
		}

	// https://apply.workable.com/tamnoon-dot-i-o/j/5377DBAC0B
	// https://apply.workable.com/maxana/j/9EFCA28551/
	case host == "apply.workable.com":
		if len(parts) >= 3 && parts[1] == "j" && isAlphanumeric(parts[2]) {
			return BoardRef{Board: "workable", Account: parts[0], JobID: strings.ToUpper(parts[2])}, true
		}
	}

	return BoardRef{}, false
}

// AccountFeedURL returns the account-level feed endpoint for ref, or "" for an
// unrecognised board. One fetch per account answers for every queued posting on
// that board, which is why the backfill groups by account before fetching.
func AccountFeedURL(ref BoardRef) string {
	switch ref.Board {
	case "greenhouse":
		return fmt.Sprintf(greenhouseBoardAPI, ref.Account)
	case "lever":
		return fmt.Sprintf(leverBoardAPI, ref.Account)
	case "workable":
		return fmt.Sprintf(workableAccountFeed, ref.Account)
	}
	return ""
}

// JobFeedURL returns the per-posting endpoint for ref, or "" for an
// unrecognised board.
//
// This exists to keep the backfill from throwing away live jobs. Absence from
// an account feed is *not* proof a posting is dead: a board can carry postings
// that are reachable by direct link but not listed publicly, and terminalizing
// those would repeat exactly the mistake bugs.md warns about — "skipping a job
// that would have submitted is strictly worse than wasting inference, because
// the goal is applications, not throughput". A per-posting 404 is proof.
// Confirmed live 2026-08-06 on both sides: greenhouse job 5240015008 is absent
// from the pointwild board *and* 404s individually (genuinely expired), while
// its sibling 5219351008 resolves on both.
func JobFeedURL(ref BoardRef) string {
	switch ref.Board {
	case "greenhouse":
		return fmt.Sprintf(greenhouseJobAPI, ref.Account, ref.JobID)
	case "lever":
		return fmt.Sprintf(leverJobAPI, ref.Account, ref.JobID)
	case "workable":
		return fmt.Sprintf(workableJobAPI, ref.Account, ref.JobID)
	}
	return ""
}

// FetchAccountFeed is a thin exported wrapper around the HTTP client discovery
// already uses, so cmd/backfill-location does not duplicate the timeout, the
// response size limit, or the User-Agent header. That header is load-bearing
// here and not merely polite: Workable's widget endpoint returns HTTP 403
// without one and HTTP 200 with one (confirmed live 2026-08-06).
func FetchAccountFeed(endpoint string) ([]byte, error) {
	return fetchATSFeed(endpoint)
}

// greenhouseFeedJob is the posting shape shared by Greenhouse's board listing
// and its per-job endpoint — the same object, returned inside "jobs" by one and
// bare by the other. atsfeeds.go's greenhouseJobsResponse deliberately decodes
// only what discovery needs and is left untouched; this named type additionally
// takes "id", which is what a stored posting URL carries and therefore the only
// key the backfill can match on.
type greenhouseFeedJob struct {
	ID          int64  `json:"id"`
	AbsoluteURL string `json:"absolute_url"`
	Title       string `json:"title"`
	Location    struct {
		Name string `json:"name"`
	} `json:"location"`
}

type greenhouseFeedResponse struct {
	Jobs []greenhouseFeedJob `json:"jobs"`
}

// workableJob is the widget listing's posting shape. Workable's per-job
// endpoint returns the same information under different names (workableJobDetail
// below), which is why the two are decoded separately and then normalized
// through one shared builder.
type workableJob struct {
	Title         string             `json:"title"`
	Shortcode     string             `json:"shortcode"`
	Telecommuting bool               `json:"telecommuting"`
	Country       string             `json:"country"`
	City          string             `json:"city"`
	State         string             `json:"state"`
	Locations     []workableLocation `json:"locations"`
}

type workableJobsResponse struct {
	Jobs []workableJob `json:"jobs"`
}

// workableJobDetail is the per-job shape. Note "remote" rather than
// "telecommuting", a nested "location" object rather than flat city/country
// fields, and a "state" that reports publication status rather than a region.
type workableJobDetail struct {
	Title     string             `json:"title"`
	Shortcode string             `json:"shortcode"`
	Remote    bool               `json:"remote"`
	State     string             `json:"state"`
	Location  workableLocation   `json:"location"`
	Locations []workableLocation `json:"locations"`
}

type workableLocation struct {
	Country     string  `json:"country"`
	CountryCode string  `json:"countryCode"`
	City        string  `json:"city"`
	Region      *string `json:"region"`
	Hidden      bool    `json:"hidden"`
}

// ParseAccountFeed turns one account feed body into its postings, keyed by the
// same JobID that ParseBoardRef derives from a posting URL. An unknown board or
// an undecodable body is an error, never an empty map — an empty map means
// "this account has no postings", which the caller reads as "every queued
// posting on it has expired".
func ParseAccountFeed(board string, body []byte) (map[string]FeedJob, error) {
	switch board {
	case "greenhouse":
		var parsed greenhouseFeedResponse
		if err := json.Unmarshal(body, &parsed); err != nil {
			return nil, fmt.Errorf("parse greenhouse account feed: %w", err)
		}
		out := make(map[string]FeedJob, len(parsed.Jobs))
		for _, j := range parsed.Jobs {
			if j.ID == 0 {
				continue
			}
			out[strconv.FormatInt(j.ID, 10)] = greenhouseFeedJobToFeedJob(j)
		}
		return out, nil

	case "lever":
		// parseLeverBoard already decodes every location field this needs
		// (bug #516 added them); the backfill only has to re-key its result by
		// the UUID a stored URL carries.
		jobs, err := parseLeverBoard(body)
		if err != nil {
			return nil, fmt.Errorf("parse lever account feed: %w", err)
		}
		out := make(map[string]FeedJob, len(jobs))
		for _, j := range jobs {
			ref, ok := ParseBoardRef(j.URL)
			if !ok || ref.Board != "lever" {
				continue
			}
			out[ref.JobID] = j
		}
		return out, nil

	case "workable":
		var parsed workableJobsResponse
		if err := json.Unmarshal(body, &parsed); err != nil {
			return nil, fmt.Errorf("parse workable account feed: %w", err)
		}
		out := make(map[string]FeedJob, len(parsed.Jobs))
		for _, j := range parsed.Jobs {
			shortcode := strings.ToUpper(strings.TrimSpace(j.Shortcode))
			if shortcode == "" {
				continue
			}
			job := workableFeedJob(j.Title, j.Telecommuting, j.Locations)
			if job.Location == "" && len(job.CountryCodes) == 0 {
				// No locations array: fall back to the flat fields, which are
				// names rather than codes, so they can only feed the free-text
				// side of CountryCodesFor.
				job.Location = joinLocationParts(j.City, j.State, j.Country)
			}
			out[shortcode] = job
		}
		return out, nil
	}

	return nil, fmt.Errorf("unsupported board: %s", board)
}

// ParseJobFeed decodes one per-posting response. live is false when the board
// answered but reports the posting as no longer published, which Workable does
// explicitly through its "state" field; the other two boards say so by
// returning HTTP 404, which the caller sees as a fetch error instead.
func ParseJobFeed(board string, body []byte) (job FeedJob, live bool, err error) {
	switch board {
	case "greenhouse":
		var parsed greenhouseFeedJob
		if err := json.Unmarshal(body, &parsed); err != nil {
			return FeedJob{}, false, fmt.Errorf("parse greenhouse job: %w", err)
		}
		if parsed.ID == 0 {
			return FeedJob{}, false, nil
		}
		return greenhouseFeedJobToFeedJob(parsed), true, nil

	case "lever":
		var parsed leverPosting
		if err := json.Unmarshal(body, &parsed); err != nil {
			return FeedJob{}, false, fmt.Errorf("parse lever job: %w", err)
		}
		if strings.TrimSpace(parsed.HostedURL) == "" {
			return FeedJob{}, false, nil
		}
		location := strings.TrimSpace(parsed.Categories.Location)
		if location == "" && len(parsed.Categories.AllLocations) > 0 {
			location = strings.TrimSpace(strings.Join(parsed.Categories.AllLocations, ", "))
		}
		var codes []string
		if code := strings.TrimSpace(parsed.Country); code != "" {
			codes = append(codes, code)
		}
		return FeedJob{
			Title:        strings.TrimSpace(parsed.Text),
			URL:          strings.TrimSpace(parsed.HostedURL),
			Location:     location,
			CountryCodes: codes,
			Remote:       strings.EqualFold(strings.TrimSpace(parsed.WorkplaceType), "remote"),
		}, true, nil

	case "workable":
		var parsed workableJobDetail
		if err := json.Unmarshal(body, &parsed); err != nil {
			return FeedJob{}, false, fmt.Errorf("parse workable job: %w", err)
		}
		if strings.TrimSpace(parsed.Shortcode) == "" {
			return FeedJob{}, false, nil
		}
		if state := strings.ToLower(strings.TrimSpace(parsed.State)); state != "" && state != "published" {
			return FeedJob{}, false, nil
		}
		locations := parsed.Locations
		if len(locations) == 0 {
			locations = []workableLocation{parsed.Location}
		}
		return workableFeedJob(parsed.Title, parsed.Remote, locations), true, nil
	}

	return FeedJob{}, false, fmt.Errorf("unsupported board: %s", board)
}

func greenhouseFeedJobToFeedJob(j greenhouseFeedJob) FeedJob {
	location := strings.TrimSpace(j.Location.Name)
	// Greenhouse publishes free text only — no country code — so the location
	// string is the sole evidence CountryCodesFor gets to work with, and Remote
	// is derived the same way parseGreenhouseBoard derives it.
	return FeedJob{
		Title:    strings.TrimSpace(j.Title),
		URL:      strings.TrimSpace(j.AbsoluteURL),
		Location: location,
		Remote:   strings.Contains(strings.ToLower(location), "remote"),
	}
}

// workableFeedJob renders Workable's locations array into one posting.
//
// Every visible country code is kept, not just the first. Multi-country
// postings are common on this board and are precisely the #516 hazard: queued
// job action1/950A91C0A2 is Cyprus/Serbia/Montenegro and maxana/9EFCA28551 is
// US/Brazil/Argentina (both confirmed live 2026-08-06). Keeping every code does
// not change LocationAllowed's verdict, which already allows a posting if *any*
// of its countries is permitted — it changes what the operator reads on the
// queue card, which is the whole point of the backfill.
func workableFeedJob(title string, remote bool, locations []workableLocation) FeedJob {
	var rendered []string
	var codes []string
	seen := map[string]struct{}{}

	for _, l := range locations {
		if l.Hidden {
			continue
		}
		region := ""
		if l.Region != nil {
			region = *l.Region
		}
		if part := joinLocationParts(l.City, region, l.Country); part != "" {
			rendered = append(rendered, part)
		}
		code := strings.ToUpper(strings.TrimSpace(l.CountryCode))
		if len(code) != 2 {
			continue
		}
		if _, dup := seen[code]; dup {
			continue
		}
		seen[code] = struct{}{}
		codes = append(codes, code)
	}

	return FeedJob{
		Title:        strings.TrimSpace(title),
		Location:     strings.Join(rendered, " / "),
		CountryCodes: codes,
		Remote:       remote,
	}
}

func joinLocationParts(parts ...string) string {
	var kept []string
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			kept = append(kept, p)
		}
	}
	return strings.Join(kept, ", ")
}

func isNumeric(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func isAlphanumeric(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')) {
			return false
		}
	}
	return true
}

func isUUID(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i, r := range s {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			if r != '-' {
				return false
			}
			continue
		}
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')) {
			return false
		}
	}
	return true
}
