package scraper

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"
	"math/rand"
)

type Job struct {
	CompanyName string
	Title       string
	Location    string
	URL         string
	Salary      int
	Remote      bool
	Description string
}

type Engine struct {
	SalaryFloor int
	Roles       []string
}

func NewEngine(salaryFloor int, roles []string) *Engine {
	return &Engine{
		SalaryFloor: salaryFloor,
		Roles:       roles,
	}
}

type RemoteOkJob struct {
	Company     string   `json:"company"`
	Position    string   `json:"position"`
	Location    string   `json:"location"`
	URL         string   `json:"url"`
	SalaryMin   int      `json:"salary_min"`
	SalaryMax   int      `json:"salary_max"`
	Tags        []string `json:"tags"`
	Description string   `json:"description"`
}

// DiscoverSources utilizes an LLM pipeline to actively parse new sources like Himalayas and Remotive
// This is an architectural placeholder for the 'Dynamic Source Discovery' goal.
func (e *Engine) DiscoverSources() {
	log.Println("[Scraper] Dynamic Source Discovery: Analyzing Himalayas, Remotive, and seed lists for new career endpoints...")
	// AI source discovery implementation would parse HTML and append to a SQLite database.
}

func (e *Engine) FetchJobs() ([]Job, error) {
	// e.DiscoverSources()
	log.Printf("[Scraper] Scraping RemoteOK API for roles: %v...", e.Roles)

	var allJobs []Job
	seenURLs := make(map[string]bool)

	rolesToSearch := e.Roles
	if len(rolesToSearch) == 0 {
		rolesToSearch = []string{"backend"}
	}

	for _, role := range rolesToSearch {
		// Convert "DevOps Engineer" to "devops-engineer"
		tag := url.QueryEscape(strings.ToLower(strings.ReplaceAll(role, " ", "-")))

		// Sleep for a random jitter (1-3 seconds) to seem human
		time.Sleep(time.Duration(rand.Intn(2000)+1000) * time.Millisecond)

		reqURL := fmt.Sprintf("https://remoteok.com/api?tag=%s", tag)
		req, err := http.NewRequest("GET", reqURL, nil)
		if err != nil {
			log.Printf("[Scraper] Failed to create request for %s: %v", role, err)
			continue
		}
	
	// Humanize the headers to bypass basic bot protection
		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
		req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8")
		req.Header.Set("Accept-Language", "en-US,en;q=0.5")
		req.Header.Set("Connection", "keep-alive")
		req.Header.Set("Upgrade-Insecure-Requests", "1")

		client := &http.Client{Timeout: 30 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			log.Printf("[Scraper] Failed to execute request for %s: %v", role, err)
			continue
		}
		
		if resp.StatusCode != http.StatusOK {
			log.Printf("[Scraper] API returned non-200 status for %s: %d", role, resp.StatusCode)
			time.Sleep(5 * time.Second)
			resp.Body.Close()
			continue
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			log.Printf("[Scraper] Failed to read response body for %s: %v", role, err)
			continue
		}

		var rawJobs []json.RawMessage
		if err := json.Unmarshal(body, &rawJobs); err != nil {
			log.Printf("[Scraper] Failed to unmarshal JSON for %s: %v", role, err)
			continue
		}

		if len(rawJobs) <= 1 {
			continue
		}

		for i := 1; i < len(rawJobs); i++ {
			var roJob RemoteOkJob
			if err := json.Unmarshal(rawJobs[i], &roJob); err != nil {
				log.Printf("[Scraper] Failed to unmarshal job %d: %v", i, err)
				continue
			}

			if seenURLs[roJob.URL] {
				continue
			}
			seenURLs[roJob.URL] = true

			isRemote := true
			if strings.Contains(strings.ToLower(roJob.Location), "hybrid") || strings.Contains(strings.ToLower(roJob.Location), "onsite") {
				isRemote = false
			}

			estimatedSalary := roJob.SalaryMax
			if roJob.SalaryMin > 0 && roJob.SalaryMax == 0 {
				estimatedSalary = roJob.SalaryMin
			}
			u, err := url.Parse(roJob.URL)
			if err != nil || u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1" || u.Hostname() == "169.254.169.254" {
				continue
			}

			allJobs = append(allJobs, Job{
				CompanyName: roJob.Company,
				Title:       roJob.Position,
				Location:    roJob.Location,
				URL:         roJob.URL,
				Salary:      estimatedSalary,
				Remote:      isRemote,
				Description: roJob.Description,
			})
		}
	}
	
	// Architectural Stubs for Data collection engine targeting fully remote listings only
	log.Println("[Scraper] Scraping We Work Remotely (Implementation pending)")
	log.Println("[Scraper] Scraping Wellfound (Implementation pending)")
	log.Println("[Scraper] Scraping Built In (Remote) (Implementation pending)")

	log.Printf("[Scraper] Successfully fetched and parsed %d jobs.", len(allJobs))
	return allJobs, nil
}
