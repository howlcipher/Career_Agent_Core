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

type FunnelEngine struct {
	TargetATS []string
	Roles     []string
}

func NewFunnelEngine(roles []string) *FunnelEngine {
	return &FunnelEngine{
		// Common ATS providers that often host remote roles
		TargetATS: []string{"greenhouse.io", "lever.co", "workday.com", "jobs.ashbyhq.com", "breezy.hr", "bamboohr.com", "workable.com", "smartrecruiters.com", "recruitee.com", "apply.workable.com", "boards.eu.greenhouse.io"},
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
	defer func() {
		if jobChan != nil {
			close(jobChan)
		}
	}()
	apiKey := os.Getenv("SERPAPI_API_KEY")
	if apiKey == "" {
		return fmt.Errorf("SERPAPI_API_KEY environment variable is missing. Job discovery requires this API key.")
	}

	log.Println("[FunnelEngine] Starting live job discovery via SerpApi...")
	
	useFallback := false


	for _, role := range f.Roles {
		for _, ats := range f.TargetATS {
			query := fmt.Sprintf(`Remote %s site:%s`, role, ats)
			log.Printf("[FunnelEngine] Searching Google for: %s", query)

			if useFallback {
				f.discoverWithYahooHTML(query, role, jobChan)
				time.Sleep(3 * time.Second)
				continue
			}

			reqURL := fmt.Sprintf("https://serpapi.com/search.json?q=%s&api_key=%s&num=10", url.QueryEscape(query), apiKey)
			
			resp, err := http.Get(reqURL)
			if err != nil {
				log.Printf("[FunnelEngine] API request failed: %v", err)
				continue
			}
			
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()

			var serpResult SerpApiResponse
			if err := json.Unmarshal(body, &serpResult); err != nil {
				log.Printf("[FunnelEngine] Failed to parse API response: %v", err)
				continue
			}

			if serpResult.Error != "" {
				log.Printf("[FunnelEngine] SerpApi error: %s. Switching to Yahoo Fallback...", serpResult.Error)
				useFallback = true
				f.discoverWithYahooHTML(query, role, jobChan)
				time.Sleep(3 * time.Second)
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
			time.Sleep(1 * time.Second)
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
	searchURL := fmt.Sprintf("https://search.yahoo.com/search?p=%s", url.QueryEscape(query))
	req, _ := http.NewRequest("GET", searchURL, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("[FunnelEngine] Yahoo fallback failed: %v", err)
		return
	}
	defer resp.Body.Close()
	
	b, _ := io.ReadAll(resp.Body)
	html := string(b)
	
	// Extract RU parameter from r.search.yahoo.com links
	re := regexp.MustCompile(`RU=(https?%3a%2f%2f[^/]+%2f[^/]+(?:%2f[^/"&<]*)?)/RK=`)
	matches := re.FindAllStringSubmatch(html, -1)
	
	found := make(map[string]bool)
	for _, m := range matches {
		decoded, _ := url.QueryUnescape(m[1])
		if !found[decoded] && (strings.Contains(decoded, "greenhouse.io") || strings.Contains(decoded, "lever.co") || strings.Contains(decoded, "workday.com") || strings.Contains(decoded, "ashbyhq.com") || strings.Contains(decoded, "breezy.hr") || strings.Contains(decoded, "bamboohr.com") || strings.Contains(decoded, "workable.com") || strings.Contains(decoded, "smartrecruiters.com") || strings.Contains(decoded, "recruitee.com")) {
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
