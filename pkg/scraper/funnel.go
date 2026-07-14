package scraper

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
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
		TargetATS: []string{"greenhouse.io", "lever.co", "workday.com", "jobs.ashbyhq.com", "breezy.hr"},
		Roles:     roles,
	}
}

type SerpApiResponse struct {
	OrganicResults []struct {
		Title string `json:"title"`
		Link  string `json:"link"`
	} `json:"organic_results"`
}

// DiscoverJobs queries Google using SerpApi to find live job pages
func (f *FunnelEngine) DiscoverJobs() error {
	apiKey := os.Getenv("SERPAPI_API_KEY")
	if apiKey == "" {
		return fmt.Errorf("SERPAPI_API_KEY environment variable is missing. Job discovery requires this API key.")
	}

	log.Println("[FunnelEngine] Starting live job discovery via SerpApi...")
	
	for _, role := range f.Roles {
		for _, ats := range f.TargetATS {
			query := fmt.Sprintf(`"Remote" "%s" site:%s`, role, ats)
			log.Printf("[FunnelEngine] Searching Google for: %s", query)

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
