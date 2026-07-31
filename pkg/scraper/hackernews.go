package scraper

import (
	"encoding/json"
	"fmt"
	"github.com/howlcipher/Career_Agent_Core/pkg/storage"
	"github.com/howlcipher/Career_Agent_Core/pkg/util"
	"golang.org/x/sync/errgroup"
	"html"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// hnAlgoliaBaseURL is the public, unauthenticated Hacker News search API
// (https://hn.algolia.com/api), used here instead of scraping
// news.ycombinator.com's HTML directly.
var hnAlgoliaBaseURL = "https://hn.algolia.com/api/v1/search_by_date"

type hnStorySearchResponse struct {
	Hits []struct {
		ObjectID string `json:"objectID"`
		Title    string `json:"title"`
	} `json:"hits"`
}

type hnComment struct {
	ObjectID    string `json:"objectID"`
	ParentID    int64  `json:"parent_id"`
	StoryID     int64  `json:"story_id"`
	CommentText string `json:"comment_text"`
}

type hnCommentSearchResponse struct {
	Hits    []hnComment `json:"hits"`
	NbPages int         `json:"nbPages"`
}

// discoverWithHackerNews finds the current month's "Ask HN: Who is hiring?"
// thread and treats each top-level comment as a candidate job posting — a
// distinct source class from FunnelEngine's ATS-domain Google-dorking
// (improvements.md #12): postings here are freeform text, not a structured
// form. Only the first http(s) link in each comment is taken as the
// posting's URL; a comment with no link at all (e.g. "email me at ...") is
// skipped, since this pipeline has no apply-by-email pathway.
func (f *FunnelEngine) discoverWithHackerNews(jobChan chan<- Job) {
	log.Println("[FunnelEngine] Scraping Hacker News \"Who is hiring\" thread...")

	storyID, err := latestWhoIsHiringStoryID()
	if err != nil {
		log.Printf("[FunnelEngine] Failed to find latest Who is Hiring thread: %v", err)
		return
	}

	comments, err := fetchHNThreadComments(storyID)
	if err != nil {
		log.Printf("[FunnelEngine] Failed to fetch Who is Hiring comments: %v", err)
		return
	}

	found := 0
	for _, c := range comments {
		if c.ParentID != storyID {
			continue // only top-level postings, not replies/meta-discussion
		}
		company, title, jobURL, ok := parseHNJobPosting(c.CommentText)
		if !ok {
			continue
		}
		isNew, err := storage.AddToFunnel(company, title, jobURL, "DISCOVERED")
		if err != nil {
			log.Printf("[FunnelEngine] Failed to add HN posting to funnel: %v", err)
			continue
		}
		if isNew && jobChan != nil {
			found++
			jobChan <- Job{CompanyName: company, Title: title, URL: jobURL, Remote: true}
		}
	}
	log.Printf("[FunnelEngine] Discovered %d new Hacker News \"Who is hiring\" postings.", found)
}

// latestWhoIsHiringStoryID finds the most recent "Ask HN: Who is hiring?"
// story, excluding the sibling "Who wants to be hired?"/"Freelancer? Seeking
// freelancer?" threads the same author (whoishiring) posts monthly.
func latestWhoIsHiringStoryID() (int64, error) {
	reqURL := hnAlgoliaBaseURL + "?tags=story,author_whoishiring&hitsPerPage=20"
	var resp hnStorySearchResponse
	if err := fetchHNJSON(reqURL, &resp); err != nil {
		return 0, err
	}
	for _, hit := range resp.Hits {
		if strings.Contains(strings.ToLower(hit.Title), "who is hiring") {
			id, err := strconv.ParseInt(hit.ObjectID, 10, 64)
			if err != nil {
				continue
			}
			return id, nil
		}
	}
	return 0, fmt.Errorf("no \"Who is hiring\" thread found in the most recent 20 whoishiring posts")
}

// fetchHNThreadComments fetches every comment (all nesting levels) for the
// given story, paging through Algolia's search results.
func fetchHNThreadComments(storyID int64) ([]hnComment, error) {
	reqURL := fmt.Sprintf("%s?tags=comment,story_%d&hitsPerPage=1000&page=0", hnAlgoliaBaseURL, storyID)
	var resp hnCommentSearchResponse
	if err := fetchHNJSON(reqURL, &resp); err != nil {
		return nil, err
	}

	if resp.NbPages <= 1 {
		return resp.Hits, nil
	}

	allHits := make([][]hnComment, resp.NbPages)
	allHits[0] = resp.Hits

	var eg errgroup.Group
	eg.SetLimit(5)

	for page := 1; page < resp.NbPages; page++ {
		p := page
		eg.Go(func() error {
			pageURL := fmt.Sprintf("%s?tags=comment,story_%d&hitsPerPage=1000&page=%d", hnAlgoliaBaseURL, storyID, p)
			var pageResp hnCommentSearchResponse
			if err := fetchHNJSON(pageURL, &pageResp); err != nil {
				return err
			}
			allHits[p] = pageResp.Hits
			return nil
		})
	}

	if err := eg.Wait(); err != nil {
		return nil, err
	}

	var all []hnComment
	for _, hits := range allHits {
		all = append(all, hits...)
	}
	return all, nil
}

var (
	hnLinkPattern  = regexp.MustCompile(`href="([^"]+)"`)
	hnHTMLTagRegex = regexp.MustCompile(`<[^>]*>`)
)

// parseHNJobPosting extracts a candidate (company, title, url) from one "Who
// is hiring" comment's HTML text, or ok=false if no usable http(s) link is
// present. HN "who is hiring" postings loosely follow a "Company | Title |
// Location | ..." convention but it's never enforced, so company/title are
// best-effort only — the real job description gets fetched from the URL
// itself once this reaches cmd/agent's worker loop, same as every other
// source.
func parseHNJobPosting(commentHTML string) (company, title, jobURL string, ok bool) {
	match := hnLinkPattern.FindStringSubmatch(commentHTML)
	if match == nil {
		return "", "", "", false
	}
	rawURL := html.UnescapeString(match[1])
	u, err := url.Parse(rawURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return "", "", "", false
	}
	if u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1" || u.Hostname() == "169.254.169.254" {
		return "", "", "", false
	}
	// A link to HN itself (e.g. a "reply" link or a user profile) is not a
	// job posting.
	if strings.HasSuffix(u.Hostname(), "ycombinator.com") {
		return "", "", "", false
	}

	plainText := html.UnescapeString(hnHTMLTagRegex.ReplaceAllString(commentHTML, " "))
	parts := strings.SplitN(plainText, "|", 3)
	company = strings.TrimSpace(parts[0])
	if company == "" || len(company) > 80 {
		company = "Unknown Company (HN Who is Hiring)"
	}
	title = "Who is Hiring posting"
	if len(parts) > 1 {
		if t := strings.TrimSpace(parts[1]); t != "" && len(t) <= 120 {
			title = t
		}
	}
	return company, title, rawURL, true
}

func fetchHNJSON(reqURL string, out interface{}) error {
	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")

	client := newHTTPClient(30 * time.Second)
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d from %s", resp.StatusCode, reqURL)
	}
	body, err := util.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	return json.Unmarshal(body, out)
}
