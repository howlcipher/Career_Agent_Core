package scraper

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/howlcipher/Career_Agent_Core/pkg/storage"
)

var (
	serpAPIBaseURL = "https://serpapi.com/search.json"
	yahooBaseURL   = "https://search.yahoo.com/search"
)

type FunnelEngine struct {
	TargetATS []string
	Roles     []string
}

func NewFunnelEngine(roles []string) *FunnelEngine {
	return &FunnelEngine{
		// Common ATS providers that often host remote roles
		TargetATS: []string{"greenhouse.io", "lever.co", "workday.com", "jobs.ashbyhq.com", "breezy.hr", "bamboohr.com", "workable.com", "smartrecruiters.com", "recruitee.com", "apply.workable.com", "boards.eu.greenhouse.io", "jobs.jobvite.com", "applytojob.com", "myworkdayjobs.com", "pinpointhq.com", "homerun.co"},
		Roles:     roles,
	}
}

type SerpApiResponse struct {
	Error          string `json:"error"`
	OrganicResults []struct {
		Title string `json:"title"`
		Link  string `json:"link"`
	} `json:"organic_results"`
}

// DiscoverJobs queries Google using SerpApi to find live job pages and sends them directly to a consumer channel.
func (f *FunnelEngine) DiscoverJobs(jobChan chan<- Job) error {
	apiKey := os.Getenv("SERPAPI_API_KEY")
	if apiKey == "" {
		return fmt.Errorf("SERPAPI_API_KEY environment variable is missing. Job discovery requires this API key.")
	}

	log.Println("[FunnelEngine] Starting live job discovery via SerpApi...")
	
	f.discoverWithRemoteOK(jobChan)
	
	useFallback := false


	for _, role := range f.Roles {
		for _, ats := range f.TargetATS {
			query := fmt.Sprintf(`Remote %s site:%s`, role, ats)
			log.Printf("[FunnelEngine] Searching Google for: %s", query)

			if useFallback {
				f.discoverWithYahooHTML(query, role, jobChan)
				SleepFunc(3 * time.Second)
				continue
			}

			reqURL := fmt.Sprintf("%s?q=%s&api_key=%s&num=100", serpAPIBaseURL, url.QueryEscape(query), apiKey)
			
			client := &http.Client{Timeout: 30 * time.Second}
			resp, err := client.Get(reqURL)
			if err != nil {
				safeErr := strings.ReplaceAll(err.Error(), apiKey, "REDACTED")
				log.Printf("[FunnelEngine] API request failed: %v", safeErr)
				continue
			}
			
			body, err := io.ReadAll(resp.Body)
			resp.Body.Close()
			if err != nil {
				log.Printf("[FunnelEngine] Failed to read response body: %v", err)
				continue
			}

			var serpResult SerpApiResponse
			if err := json.Unmarshal(body, &serpResult); err != nil {
				log.Printf("[FunnelEngine] Failed to parse API response: %v", err)
				continue
			}

			if serpResult.Error != "" {
				log.Printf("[FunnelEngine] SerpApi error: %s. Switching to Yahoo Fallback...", serpResult.Error)
				useFallback = true
				f.discoverWithYahooHTML(query, role, jobChan)
				SleepFunc(3 * time.Second)
				continue
			}

			if len(serpResult.OrganicResults) == 0 {
				log.Printf("[FunnelEngine] No results found for query: %s", query)
			}

			for _, result := range serpResult.OrganicResults {
				// Some basic sanitization to extract company name from Title
				company := extractCompanyFromTitle(result.Title)
				log.Printf("[FunnelEngine] Discovered Live Job: %s at %s", result.Title, result.Link)
				
				err := storage.AddToFunnel(company, role, result.Link, "DISCOVERED")
				if err != nil {
					log.Printf("[FunnelEngine] Warning: Failed to add to funnel DB: %v", err)
				} else if jobChan != nil {
					jobChan <- Job{
						CompanyName: company,
						Title:       role,
						URL:         result.Link,
					}
				}
			}
			
			// Sleep to respect rate limits if on free tier
			SleepFunc(1 * time.Second)
		}
	}
	
	log.Println("[FunnelEngine] Job discovery complete. Backlog updated in applications.db")
	return nil
}

func extractCompanyFromTitle(title string) string {
	// Usually titles look like "Senior Backend Engineer at Stripe - Lever"
	parts := strings.Split(title, " at ")
	if len(parts) > 1 {
		subParts := strings.Split(parts[1], " - ")
		return strings.TrimSpace(subParts[0])
	}
	// Fallback
	parts = strings.Split(title, " - ")
	if len(parts) > 1 {
		return strings.TrimSpace(parts[0])
	}
	return "Unknown Company"
}

func (f *FunnelEngine) discoverWithYahooHTML(query, role string, jobChan chan<- Job) {
	log.Printf("[FunnelEngine] Fallback searching Yahoo HTML for: %s", query)
	
	client := &http.Client{Timeout: 10 * time.Second}
	searchURL := fmt.Sprintf("%s?p=%s", yahooBaseURL, url.QueryEscape(query))
	req, err := http.NewRequest("GET", searchURL, nil)
	if err != nil {
		log.Printf("[FunnelEngine] Failed to create request for Yahoo: %v", err)
		return
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("[FunnelEngine] Yahoo fallback failed: %v", err)
		return
	}
	defer resp.Body.Close()
	
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("[FunnelEngine] Failed to read response body for Yahoo: %v", err)
		return
	}
	html := string(b)
	
	// Extract RU parameter from r.search.yahoo.com links
	re := regexp.MustCompile(`RU=(https?%3a%2f%2f[^/]+%2f[^/]+(?:%2f[^/"&<]*)?)/RK=`)
	matches := re.FindAllStringSubmatch(html, -1)
	
	found := make(map[string]bool)
	for _, m := range matches {
		decoded, err := url.QueryUnescape(m[1])
		if err != nil {
			log.Printf("[FunnelEngine] Failed to decode URL: %v", err)
			continue
		}
		if !found[decoded] && isValidATSUrl(decoded) {
			found[decoded] = true
			
			company := "Unknown Company"
			// Try to extract company from the URL path as a simple fallback
			parts := strings.Split(decoded, "/")
			if len(parts) > 3 {
				company = parts[3]
			}
			
			log.Printf("[FunnelEngine] Yahoo Fallback Discovered Live Job at %s", decoded)
			err := storage.AddToFunnel(company, role, decoded, "DISCOVERED")
			if err == nil && jobChan != nil {
				jobChan <- Job{
					CompanyName: company,
					Title:       role,
					URL:         decoded,
				}
			}
		}
	}
}

func isValidATSUrl(link string) bool {
	u, err := url.Parse(link)
	if err != nil {
		return false
	}
	
	host := strings.ToLower(u.Hostname())
	if (host == "workable.com" || strings.HasSuffix(host, ".workable.com")) && strings.Contains(u.Path, "/search/") {
		return false
	}
	
	atsDomains := []string{
		"greenhouse.io", "lever.co", "ashbyhq.com",
		"breezy.hr", "bamboohr.com", "workable.com", "smartrecruiters.com",
		"recruitee.com", "jobvite.com", "applytojob.com", "myworkdayjobs.com",
		"pinpointhq.com", "homerun.co",
	}
	
	for _, domain := range atsDomains {
		if host == domain || strings.HasSuffix(host, "."+domain) {
			return true
		}
	}
	
	return false
}

func (f *FunnelEngine) discoverWithRemoteOK(jobChan chan<- Job) {
	log.Println("[FunnelEngine] Scraping RemoteOK API...")

	for _, role := range f.Roles {
		tag := strings.ReplaceAll(strings.ToLower(role), " ", "-")
		
		url := fmt.Sprintf("%s?tag=%s", remoteOKBaseURL, tag)
		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			log.Printf("[FunnelEngine] Failed to create request for %s: %v", tag, err)
			continue
		}
		
		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
		
		client := &http.Client{Timeout: 30 * time.Second}
		resp, err := client.Do(req)
		if err != nil || resp.StatusCode != http.StatusOK {
			if err != nil {
				log.Printf("[FunnelEngine] Failed to execute request for %s: %v", tag, err)
			} else {
				log.Printf("[FunnelEngine] API returned non-200 status for %s: %d", tag, resp.StatusCode)
			}
			if resp != nil {
				resp.Body.Close()
			}
			continue
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			log.Printf("[FunnelEngine] Failed to read response body for %s: %v", tag, err)
			continue
		}

		var rawJobs []json.RawMessage
		if err := json.Unmarshal(body, &rawJobs); err != nil {
			log.Printf("[FunnelEngine] Failed to unmarshal JSON for %s: %v", tag, err)
			continue
		}

		if len(rawJobs) <= 1 {
			continue
		}

		for i := 1; i < len(rawJobs); i++ {
			var roJob RemoteOkJob
			if err := json.Unmarshal(rawJobs[i], &roJob); err != nil {
				log.Printf("[FunnelEngine] Failed to unmarshal job %d: %v", i, err)
				continue
			}

			// RemoteOK has its own ATS, but for our pipeline, we extract the domain or let the dynamic learner handle it
			err := storage.AddToFunnel(roJob.Company, roJob.Position, roJob.URL, "DISCOVERED")
			if err != nil {
				log.Printf("[FunnelEngine] Failed to add %s to funnel: %v", roJob.URL, err)
			} else if jobChan != nil {
				log.Printf("[FunnelEngine] Discovered RemoteOK Job: %s at %s", roJob.Position, roJob.URL)
				jobChan <- Job{
					CompanyName: roJob.Company,
					Title:       roJob.Position,
					URL:         roJob.URL,
				}
			}
		}
	}
}
