package scraper

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/howlcipher/Career_Agent_Core/pkg/storage"
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
	} `json:"jobs"`
}

type leverPosting struct {
	HostedURL string `json:"hostedUrl"`
	Text      string `json:"text"`
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

	found := 0
	for _, slug := range gh {
		found += f.pollBoard(slug, fmt.Sprintf(greenhouseBoardAPI, slug), parseGreenhouseBoard, jobChan)
	}
	for _, slug := range lv {
		found += f.pollBoard(slug, fmt.Sprintf(leverBoardAPI, slug), parseLeverBoard, jobChan)
	}
	log.Printf("[FunnelEngine] ATS board feeds contributed %d new posting(s) across %d board(s).", found, len(gh)+len(lv))
}

type boardParser func(body []byte) ([]feedJob, error)

type feedJob struct {
	Title string
	URL   string
}

var retryBackoffBase = time.Second

func (f *FunnelEngine) pollBoard(company, endpoint string, parse boardParser, jobChan chan<- Job) int {
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
		isNew, err := storage.AddToFunnel(company, j.Title, j.URL, "DISCOVERED")
		if err != nil {
			continue
		}
		if isNew && jobChan != nil {
			found++
			jobChan <- Job{CompanyName: company, Title: j.Title, URL: j.URL}
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
		return nil, fmt.Errorf("board feed returned HTTP %d", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 8<<20))
}

func parseGreenhouseBoard(body []byte) ([]feedJob, error) {
	var parsed greenhouseJobsResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, err
	}
	out := make([]feedJob, 0, len(parsed.Jobs))
	for _, j := range parsed.Jobs {
		out = append(out, feedJob{Title: strings.TrimSpace(j.Title), URL: strings.TrimSpace(j.AbsoluteURL)})
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
		out = append(out, feedJob{Title: strings.TrimSpace(p.Text), URL: strings.TrimSpace(p.HostedURL)})
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
	if len(f.Roles) == 0 {
		return true // no configured roles: do not silently filter everything out
	}
	t := strings.ToLower(title)

	// Distinctive words are matched against whole tokens, never substrings.
	// Substring matching is actively wrong for short tokens: "go" appears
	// inside "Cargo" and "Chicago", and "api" inside "capital", so a
	// Contains-based check would wave through exactly the unrelated roles
	// this filter exists to stop.
	titleWords := map[string]bool{}
	for _, w := range strings.FieldsFunc(t, func(r rune) bool {
		return !(r >= 'a' && r <= 'z') && !(r >= '0' && r <= '9') && r != '/' && r != '+'
	}) {
		titleWords[w] = true
	}

	for _, role := range f.Roles {
		r := strings.ToLower(strings.TrimSpace(role))
		if r == "" {
			continue
		}
		// Full configured role as a phrase is safe to substring-match: it is
		// long and specific enough not to collide accidentally.
		if strings.Contains(t, r) {
			return true
		}
		for _, word := range strings.Fields(r) {
			if distinctiveRoleWords[word] && titleWords[word] {
				return true
			}
		}
	}
	return false
}

// distinctiveRoleWords are the tokens worth matching on their own. Words like
// "senior", "engineer" or "developer" appear in nearly every technical title
// and would let almost anything through, defeating the filter's purpose.
var distinctiveRoleWords = map[string]bool{
	"backend": true, "devops": true, "devsecops": true, "platform": true,
	"infrastructure": true, "reliability": true, "sre": true, "automation": true,
	"python": true, "golang": true, "go": true, "cloud": true, "security": true,
	"observability": true, "kubernetes": true, "ci/cd": true, "sdet": true,
	"integration": true, "network": true, "systems": true, "api": true,
}
