package scraper

import (
	"encoding/json"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/howlcipher/Career_Agent_Core/pkg/storage"
	"github.com/howlcipher/Career_Agent_Core/pkg/util"
)

const (
	jobicyDefaultURL   = "https://jobicy.com/api/v2/remote-jobs?count=100"
	jobicyPollInterval = time.Hour
)

var (
	jobicyBaseURL  = jobicyDefaultURL
	jobicyPollMu   sync.Mutex
	jobicyLastPoll time.Time
	jobicyNow      = time.Now
)

type jobicyResponse struct {
	Success bool        `json:"success"`
	Jobs    []jobicyJob `json:"jobs"`
}

type jobicyJob struct {
	CompanyName string `json:"companyName"`
	JobTitle    string `json:"jobTitle"`
	URL         string `json:"url"`
}

// discoverWithJobicy imports a bounded set of current remote postings from
// Jobicy's public JSON feed. The provider asks consumers not to poll more than
// once an hour, so the limit is process-wide rather than tied to one
// FunnelEngine instance (the daemon creates an engine per refresh).
func (f *FunnelEngine) discoverWithJobicy(jobChan chan<- Job) {
	if !claimJobicyPoll() {
		log.Printf("[FunnelEngine] Jobicy refresh skipped; provider polling limit is %s.", jobicyPollInterval)
		return
	}

	log.Println("[FunnelEngine] Fetching current remote jobs from Jobicy...")
	req, err := http.NewRequest(http.MethodGet, jobicyBaseURL, nil)
	if err != nil {
		log.Printf("[FunnelEngine] Failed to create Jobicy request: %v", err)
		return
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "CareerAgent/1.0 (+local job discovery)")

	resp, err := newHTTPClient(30 * time.Second).Do(req)
	if err != nil {
		log.Printf("[FunnelEngine] Jobicy request failed: %v", err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		log.Printf("[FunnelEngine] Jobicy returned HTTP %d.", resp.StatusCode)
		return
	}

	body, err := util.ReadAll(resp.Body)
	if err != nil {
		log.Printf("[FunnelEngine] Failed to read Jobicy response: %v", err)
		return
	}
	var payload jobicyResponse
	if err := json.Unmarshal(body, &payload); err != nil {
		log.Printf("[FunnelEngine] Failed to parse Jobicy response: %v", err)
		return
	}
	if !payload.Success {
		log.Println("[FunnelEngine] Jobicy reported an unsuccessful response.")
		return
	}

	found := 0
	for _, candidate := range payload.Jobs {
		company := strings.TrimSpace(candidate.CompanyName)
		title := strings.TrimSpace(candidate.JobTitle)
		jobURL := strings.TrimSpace(candidate.URL)
		if company == "" || title == "" || !isPublicJobURL(jobURL) || IsKnownJunkJobURL(jobURL) || !f.titleLooksRelevant(title) {
			continue
		}

		isNew, err := storage.AddToFunnel(company, title, jobURL, "DISCOVERED", "jobicy")
		if err != nil {
			log.Printf("[FunnelEngine] Failed to add Jobicy posting: %v", err)
			continue
		}
		if isNew {
			found++
			if jobChan != nil {
				jobChan <- Job{CompanyName: company, Title: title, URL: jobURL, Remote: true}
			}
		}
	}
	log.Printf("[FunnelEngine] Jobicy contributed %d new relevant posting(s).", found)
}

func claimJobicyPoll() bool {
	jobicyPollMu.Lock()
	defer jobicyPollMu.Unlock()
	now := jobicyNow()
	if !jobicyLastPoll.IsZero() && now.Sub(jobicyLastPoll) < jobicyPollInterval {
		return false
	}
	jobicyLastPoll = now
	return true
}

func isPublicJobURL(rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil || u.Hostname() == "" {
		return false
	}
	return u.Scheme == "http" || u.Scheme == "https"
}

func resetJobicyPollForTest() {
	jobicyPollMu.Lock()
	defer jobicyPollMu.Unlock()
	jobicyLastPoll = time.Time{}
}
