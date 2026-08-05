package scraper

import (
	"encoding/json"
	"fmt"
	"github.com/howlcipher/Career_Agent_Core/pkg/security"
	"github.com/howlcipher/Career_Agent_Core/pkg/util"
	"golang.org/x/sync/errgroup"
	"log"
	"math/rand"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

var remoteOKBaseURL = "https://remoteok.com/api"

var SleepFunc = time.Sleep

var newHTTPClient = security.NewSafeHTTPClient

type Job struct {
	CompanyName string
	Title       string
	Location    string
	URL         string
	Salary      int
	Remote      bool
	Description string
	Intent      string
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
	var mu sync.Mutex
	seenURLs := make(map[string]bool)

	rolesToSearch := e.Roles
	if len(rolesToSearch) == 0 {
		rolesToSearch = []string{"backend"}
	}

	var eg errgroup.Group
	eg.SetLimit(3)

	for _, role := range rolesToSearch {
		role := role
		eg.Go(func() error {
			tag := url.QueryEscape(strings.ToLower(strings.ReplaceAll(role, " ", "-")))

			SleepFunc(time.Duration(rand.Intn(2000)+1000) * time.Millisecond)

			reqURL := fmt.Sprintf("%s?tag=%s", remoteOKBaseURL, tag)
			req, err := http.NewRequest("GET", reqURL, nil)
			if err != nil {
				log.Printf("[Scraper] Failed to create request for %s: %v", role, err)
				return nil
			}

			req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
			req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8")
			req.Header.Set("Accept-Language", "en-US,en;q=0.5")
			req.Header.Set("Connection", "keep-alive")
			req.Header.Set("Upgrade-Insecure-Requests", "1")

			client := newHTTPClient(30 * time.Second)
			body, err := func() ([]byte, error) {
				resp, err := client.Do(req)
				if err != nil {
					return nil, fmt.Errorf("failed to execute request: %w", err)
				}
				defer resp.Body.Close()

				if resp.StatusCode != http.StatusOK {
					log.Printf("[Scraper] API returned non-200 status for %s: %d", role, resp.StatusCode)
					SleepFunc(5 * time.Second)
					return nil, nil
				}

				b, err := util.ReadAll(resp.Body)
				if err != nil {
					return nil, fmt.Errorf("failed to read response body: %w", err)
				}
				return b, nil
			}()

			if err != nil {
				log.Printf("[Scraper] Error fetching data for %s: %v", role, err)
				return nil
			}
			if body == nil {
				return nil
			}

			var rawJobs []json.RawMessage
			if err := json.Unmarshal(body, &rawJobs); err != nil {
				log.Printf("[Scraper] Failed to unmarshal JSON for %s: %v", role, err)
				return nil
			}

			if len(rawJobs) <= 1 {
				return nil
			}

			var localJobs []Job
			for i := 1; i < len(rawJobs); i++ {
				var roJob RemoteOkJob
				if err := json.Unmarshal(rawJobs[i], &roJob); err != nil {
					log.Printf("[Scraper] Failed to unmarshal job %d: %v", i, err)
					continue
				}

				mu.Lock()
				if seenURLs[roJob.URL] {
					mu.Unlock()
					continue
				}
				seenURLs[roJob.URL] = true
				mu.Unlock()

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

				localJobs = append(localJobs, Job{
					CompanyName: roJob.Company,
					Title:       roJob.Position,
					Location:    roJob.Location,
					URL:         roJob.URL,
					Salary:      estimatedSalary,
					Remote:      isRemote,
					Description: roJob.Description,
				})
			}

			mu.Lock()
			allJobs = append(allJobs, localJobs...)
			mu.Unlock()
			return nil
		})
	}
	_ = eg.Wait()

	// Architectural Stubs for Data collection engine targeting fully remote listings only
	log.Println("[Scraper] Scraping We Work Remotely (Implementation pending)")
	log.Println("[Scraper] Scraping Wellfound (Implementation pending)")
	log.Println("[Scraper] Scraping Built In (Remote) (Implementation pending)")

	log.Printf("[Scraper] Successfully fetched and parsed %d jobs.", len(allJobs))
	return allJobs, nil
}
