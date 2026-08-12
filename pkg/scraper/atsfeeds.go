package scraper

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/howlcipher/Career_Agent_Core/pkg/config"
	"github.com/howlcipher/Career_Agent_Core/pkg/storage"
	"github.com/howlcipher/Career_Agent_Core/pkg/util"
	"golang.org/x/sync/errgroup"
)

// Greenhouse and Lever both publish every posting on a company's board through
// a public, unauthenticated JSON endpoint. Confirmed live 2026-07-25 against
// real boards before any of this was written.
//
// Why this exists (improvements.md #26): every other discovery source works by
// search-engine dorking, which returns whatever a search index happens to have
// surfaced — incomplete, stale, and full of non-postings. 301 of 3,884 rows in
// the live funnel are INVALID_URL, ~8% pure waste that each still cost a fetch
// and a filter pass. A board feed instead returns that company's *complete,
// current* posting list as structured data, so there is nothing to guess at and
// nothing to filter out.
const (
	greenhouseBoardAPI = "https://boards-api.greenhouse.io/v1/boards/%s/jobs"
	leverBoardAPI      = "https://api.lever.co/v0/postings/%s?mode=json"
)

// atsFeedHTTPTimeout is per board. Deliberately short: a feed is one small
// JSON document, and hundreds of boards are polled in sequence, so a slow host
// must not be allowed to stall the whole pass.
const atsFeedHTTPTimeout = 20 * time.Second

// maxBoardsPerSource bounds one discovery pass. The known-company list grows
// without limit as the funnel fills, and an unbounded pass would spend hours
// on HTTP before the worker pool ever sees a job.
const maxBoardsPerSource = 60

type greenhouseJobsResponse struct {
	Jobs []struct {
		AbsoluteURL string `json:"absolute_url"`
		Title       string `json:"title"`
		Location    struct {
			Name string `json:"name"`
		} `json:"location"`
	} `json:"jobs"`
}

// leverPosting captures the location fields the feed has always published and
// this parser used to discard (bug #516). "country" is an ISO-3166 alpha-2 code
// and is the highest-confidence signal available; categories.location and
// allLocations are free text and are used only when the code is absent.
type leverPosting struct {
	HostedURL  string `json:"hostedUrl"`
	Text       string `json:"text"`
	Country    string `json:"country"`
	Categories struct {
		Location     string   `json:"location"`
		AllLocations []string `json:"allLocations"`
	} `json:"categories"`
	WorkplaceType string `json:"workplaceType"`
}

// discoverWithATSFeeds polls the board feeds of companies already known to use
// Greenhouse or Lever, taking the company slugs from URLs the funnel has
// already collected. This is deliberately a *widening* pass over known
// employers rather than a way to find new ones: if a company is worth applying
// to once, its other current openings are worth seeing, and the feed is the
// only way to get all of them rather than the handful a search engine indexed.
func (f *FunnelEngine) discoverWithATSFeeds(jobChan chan<- Job) {
	log.Println("[FunnelEngine] Polling Greenhouse/Lever board feeds for known companies...")

	gh, err := storage.GetKnownATSCompanies("greenhouse.io", maxBoardsPerSource)
	if err != nil {
		log.Printf("[FunnelEngine] Could not list known Greenhouse companies: %v", err)
	}
	lv, err := storage.GetKnownATSCompanies("lever.co", maxBoardsPerSource)
	if err != nil {
		log.Printf("[FunnelEngine] Could not list known Lever companies: %v", err)
	}

	var found int32
	var eg errgroup.Group
	eg.SetLimit(10)

	for _, slug := range gh {
		s := slug
		eg.Go(func() error {
			f := f.pollBoard(s, fmt.Sprintf(greenhouseBoardAPI, s), parseGreenhouseBoard, "atsfeed:greenhouse", jobChan)
			atomic.AddInt32(&found, int32(f))
			return nil
		})
	}
	for _, slug := range lv {
		s := slug
		if IsExcludedBoardSlug(s) {
			// Skip the fetch outright: an excluded aggregator board is a
			// ~2,988-posting download whose rows AddToFunnel would reject
			// anyway.
			continue
		}
		eg.Go(func() error {
			f := f.pollBoard(s, fmt.Sprintf(leverBoardAPI, s), parseLeverBoard, "atsfeed:lever", jobChan)
			atomic.AddInt32(&found, int32(f))
			return nil
		})
	}
	_ = eg.Wait()
	log.Printf("[FunnelEngine] ATS board feeds contributed %d new posting(s) across %d board(s).", found, len(gh)+len(lv))
}

type boardParser func(body []byte) ([]feedJob, error)

type feedJob struct {
	Title string
	URL   string

	// Location is the posting's advertised location as free text, and
	// CountryCodes holds any ISO-3166 alpha-2 codes the feed stated outright.
	// Either may be empty: not every board publishes them, and an empty pair
	// means "no evidence", never "not allowed" (bug #516).
	Location     string
	CountryCodes []string
	Remote       bool
}

var retryBackoffBase = time.Second

func (f *FunnelEngine) pollBoard(company, endpoint string, parse boardParser, discoverySource string, jobChan chan<- Job) int {
	var jobs []feedJob
	var err error

	maxRetries := 3
	for attempt := 1; attempt <= maxRetries; attempt++ {
		var body []byte
		body, err = fetchATSFeed(endpoint)
		if err != nil {
			// A single dead or renamed board is entirely expected (HTTP 4xx).
			// Do not retry 4xx errors except 429 Too Many Requests.
			if strings.Contains(err.Error(), "HTTP 4") && !strings.Contains(err.Error(), "HTTP 429") {
				return 0
			}
			// Other fetch errors (network, 5xx, 429) -> retry
		} else {
			jobs, err = parse(body)
			if err == nil {
				break
			}
			// Parse error (e.g. unexpected end of JSON input) -> retry
		}

		if attempt < maxRetries {
			// Exponential backoff
			time.Sleep(time.Duration(1<<attempt) * retryBackoffBase)
		}
	}

	if err != nil {
		log.Printf("[FunnelEngine] Could not process board feed for %s after %d attempts: %v", company, maxRetries, err)
		return 0
	}

	found := 0
	for _, j := range jobs {
		if j.URL == "" || j.Title == "" {
			continue
		}
		// Reuse the same junk filter every other source goes through, so a
		// feed cannot bypass filters the rest of the pipeline relies on.
		if IsKnownJunkJobURL(j.URL) {
			continue
		}
		// A board feed returns a company's ENTIRE posting list -- 238 for
		// remotecom, 287 for palantir when measured live -- including
		// accountants and office managers. Every one of those would otherwise
		// reach the worker pool and cost a full fit-scoring call, measured at
		// ~10 minutes each on this hardware. A free title check is what makes
		// this source a net win rather than a denial-of-service on the queue.
		if !f.titleLooksRelevant(j.Title) {
			continue
		}
		// Geographic gate (bug #516). The same free-filter reasoning as the
		// title check applies: an India-only posting costs a full fit-scoring
		// call and can reach a live Assisted Apply attempt, and the feed
		// already told us the country. Rejects only on positive evidence, so a
		// board that publishes no location is unaffected.
		if allowed, reason := LocationAllowed(j.Location, j.CountryCodes, f.AllowedCountries); !allowed {
			log.Printf("[FunnelEngine] Skipping %s posting outside the configured region: %s", company, reason)
			continue
		}
		// Fully-remote hard gate. Only the location text is available at
		// intake (the description is fetched later, and re-checked there by
		// cmd/agent's pipeline with the full text) -- but a feed that already
		// says "hybrid" or a location that names an office requirement is
		// rejected here rather than costing a full fit-scoring call only to
		// be rejected later anyway.
		if f.RemoteOnly {
			if ok, reason := config.RemoteEligible(j.Remote, j.Location, ""); !ok {
				log.Printf("[FunnelEngine] Skipping %s posting that is not confirmed fully remote: %s", company, reason)
				continue
			}
		}
		isNew, err := f.addToFunnelCounted(discoverySource, company, j.Title, j.URL, "DISCOVERED")
		if err != nil {
			continue
		}
		if isNew {
			// Retain what the feed said about location so the queue, the
			// dashboard and the duplicate matcher can all screen on it. Before
			// #516 this was only ever written when a duplicate cooldown was
			// configured, which it is not, leaving the column empty for all
			// 12,902 rows.
			if j.Location != "" || len(j.CountryCodes) > 0 {
				identity := j.Location
				if identity == "" {
					identity = strings.Join(j.CountryCodes, ", ")
				}
				if err := storage.UpdateFunnelIdentity(j.URL, identity, j.Remote); err != nil {
					log.Printf("[FunnelEngine] Could not record advertised location for a %s posting: %v", company, err)
				}
			}
			if jobChan != nil {
				found++
				jobChan <- Job{
					CompanyName: company,
					Title:       j.Title,
					URL:         j.URL,
					Location:    j.Location,
					Remote:      j.Remote,
				}
			}
		}
	}
	return found
}

func fetchATSFeed(endpoint string) ([]byte, error) {
	client := newHTTPClient(atsFeedHTTPTimeout)
	req, err := http.NewRequest("GET", endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; CareerAgent/1.0)")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, &FeedHTTPError{
			StatusCode: resp.StatusCode,
			RetryAfter: parseRetryAfter(resp.Header.Get("Retry-After")),
		}
	}
	return util.ReadAll(io.LimitReader(resp.Body, 128<<20))
}

// FeedHTTPError carries the status a board feed refused with, and how long it
// asked us to wait before asking again.
//
// The Retry-After half exists because a 429 is not always a brief throttle.
// Measured live 2026-08-06: after a backfill pass polled ~210 Workable accounts,
// apply.workable.com returned 429 for *every* path on the host with
// `Retry-After: 84643` — 23.5 hours, a host-wide block rather than a per-request
// rate limit. Retrying into that is worse than useless: it cannot succeed, and
// it keeps hitting a host that has already said stop. Callers should read
// RetryAfter and give the host up for the run rather than spending attempts.
//
// Error() keeps the original wording so the existing `strings.Contains(err, "HTTP 4")`
// checks in pollBoard and cmd/backfill-location keep behaving as before.
type FeedHTTPError struct {
	StatusCode int
	RetryAfter time.Duration // zero when the response named none
}

func (e *FeedHTTPError) Error() string {
	return fmt.Sprintf("board feed returned HTTP %d", e.StatusCode)
}

// parseRetryAfter reads the delay-seconds form of the header. The HTTP-date
// form is deliberately not handled: no board feed observed here uses it, and a
// missing value already means "no guidance", which callers treat as a normal
// retryable error.
func parseRetryAfter(value string) time.Duration {
	seconds, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || seconds <= 0 {
		return 0
	}
	return time.Duration(seconds) * time.Second
}

func parseGreenhouseBoard(body []byte) ([]feedJob, error) {
	var parsed greenhouseJobsResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, err
	}
	out := make([]feedJob, 0, len(parsed.Jobs))
	for _, j := range parsed.Jobs {
		location := strings.TrimSpace(j.Location.Name)
		out = append(out, feedJob{
			Title:    strings.TrimSpace(j.Title),
			URL:      strings.TrimSpace(j.AbsoluteURL),
			Location: location,
			Remote:   strings.Contains(strings.ToLower(location), "remote"),
		})
	}
	return out, nil
}

func parseLeverBoard(body []byte) ([]feedJob, error) {
	var parsed []leverPosting
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, err
	}
	out := make([]feedJob, 0, len(parsed))
	for _, p := range parsed {
		location := strings.TrimSpace(p.Categories.Location)
		if location == "" && len(p.Categories.AllLocations) > 0 {
			location = strings.TrimSpace(strings.Join(p.Categories.AllLocations, ", "))
		}
		var codes []string
		if code := strings.TrimSpace(p.Country); code != "" {
			codes = append(codes, code)
		}
		out = append(out, feedJob{
			Title:        strings.TrimSpace(p.Text),
			URL:          strings.TrimSpace(p.HostedURL),
			Location:     location,
			CountryCodes: codes,
			Remote:       strings.EqualFold(strings.TrimSpace(p.WorkplaceType), "remote"),
		})
	}
	return out, nil
}

// titleLooksRelevant is a cheap keyword gate applied only to feed-sourced
// jobs, where the alternative is spending ~10 minutes of local inference to
// reject an accountant. It is deliberately generous: it matches a full
// configured role, or any distinctive single word from one, so a genuinely
// plausible title survives to real fit-scoring rather than being discarded
// here on a keyword technicality. Scoring remains the authority on fit; this
// only prevents obviously-unrelated roles from consuming a scoring slot.
func (f *FunnelEngine) titleLooksRelevant(title string) bool {
	return config.TitleEligible(title, f.Roles)
}
