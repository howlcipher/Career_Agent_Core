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
		// breezy.hr excluded (bugs.md): 212 discovered live, 48 FAILED_SUBMIT, 0 APPLIED — worst-performing source, no fill strategy has ever worked against it.
		TargetATS: []string{"greenhouse.io", "lever.co", "workday.com", "jobs.ashbyhq.com", "bamboohr.com", "workable.com", "smartrecruiters.com", "recruitee.com", "apply.workable.com", "boards.eu.greenhouse.io", "jobs.jobvite.com", "applytojob.com", "myworkdayjobs.com", "pinpointhq.com", "homerun.co"},
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
				
				isNew, err := storage.AddToFunnel(company, role, result.Link, "DISCOVERED")
				if err != nil {
					log.Printf("[FunnelEngine] Warning: Failed to add to funnel DB: %v", err)
				} else if isNew && jobChan != nil {
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
			
			// Bug #19: derive the company from the tenant subdomain or the
			// first non-locale, non-generic path segment — the old
			// first-path-segment grab recorded locale codes ("en-US") and
			// site names ("External_Career_Site") as companies on Workday.
			company := companyFromURL(decoded)
			if company == "" {
				company = "Unknown Company"
			}
			
			log.Printf("[FunnelEngine] Yahoo Fallback Discovered Live Job at %s", decoded)
			isNew, err := storage.AddToFunnel(company, role, decoded, "DISCOVERED")
			if err == nil && isNew && jobChan != nil {
				jobChan <- Job{
					CompanyName: company,
					Title:       role,
					URL:         decoded,
				}
			}
		}
	}
}

// subdomainTenantATS lists ATS platforms where the hiring company is the
// first host label (gdit.wd5.myworkdayjobs.com, techinsights.applytojob.com)
// rather than a URL path segment.
var subdomainTenantATS = []string{
	"myworkdayjobs.com", "applytojob.com", "breezy.hr", "recruitee.com",
	"pinpointhq.com", "bamboohr.com", "homerun.co",
}

// genericPathSegments are first path segments that can never be a company
// name on path-tenant ATS platforms (section names and short locale codes).
var genericPathSegments = map[string]bool{
	"jobs": true, "careers": true, "job": true, "apply": true,
	"search": true, "o": true, "p": true, "j": true,
}

// localeSegmentRe matches locale-style path segments like "en", "en-US",
// "fr-ca". Trade-off: a genuine two-letter company slug would be skipped
// too, falling through to the next segment or the caller's fallback —
// acceptable versus recording "en-US" as a company (bugs.md #19).
var localeSegmentRe = regexp.MustCompile(`^[a-z]{2}(-[a-z]{2})?$`)

// companyFromURL extracts the hiring company's identifier from an ATS job
// URL (bugs.md #19): the tenant host label for subdomain-tenant platforms,
// otherwise the first path segment that isn't a locale ("en-US") or a
// generic section name ("careers"). Returns "" when nothing plausible is
// found so callers can keep their existing fallback.
func companyFromURL(link string) string {
	u, err := url.Parse(link)
	if err != nil {
		return ""
	}
	host := strings.ToLower(u.Hostname())
	for _, domain := range subdomainTenantATS {
		if strings.HasSuffix(host, "."+domain) {
			label := strings.Split(host, ".")[0]
			if label != "" && label != "www" && label != "jobs" && label != "apply" {
				return label
			}
		}
	}
	for _, seg := range strings.Split(u.Path, "/") {
		if seg == "" {
			continue
		}
		lower := strings.ToLower(seg)
		if genericPathSegments[lower] || localeSegmentRe.MatchString(lower) {
			continue
		}
		return seg
	}
	return ""
}

// IsKnownJunkJobURL reports whether a URL is a confirmed-junk shape that can
// never be an individual job posting. Exported because it guards two layers
// (bugs.md #22): discovery via isValidATSUrl, and worker intake in cmd/agent
// — the DISCOVERED backlog predates the discovery filters, so stale junk
// re-enters the queue on every restart and must be caught again there.
// Deliberately a blacklist, not the discovery allowlist: queued jobs from
// non-ATS sources (RemoteOK company sites) are legitimate and must pass.
func IsKnownJunkJobURL(link string) bool {
	u, err := url.Parse(link)
	if err != nil {
		return true
	}
	// Expired-posting redirects (?error=true / ?error=404) end up in search
	// indexes and get "discovered" as live jobs — confirmed live 2026-07-22
	// (job-boards.greenhouse.io/remotecom?error=true). Never a posting.
	if u.Query().Has("error") {
		return true
	}
	host := strings.ToLower(u.Hostname())
	// Any workday.com subdomain is Workday's own corporate/docs/marketing
	// site (www, developer, digital, ... — three seen live), never a job
	// posting; real postings live on *.myworkdayjobs.com.
	if host == "workday.com" || strings.HasSuffix(host, ".workday.com") {
		return true
	}
	// homerun.co's own marketing/content pages (hiring-kits, interview
	// question templates, job-description templates, ...) live on the bare
	// www.homerun.co domain and get rediscovered as postings repeatedly
	// (confirmed live 2026-07-20 through 2026-07-22, same two URLs each
	// time, occasionally crashing Playwright with "target closed" when
	// treated like an application form). Real postings are always on a
	// company subdomain, e.g. solvedex.homerun.co/golang-software-engineer.
	if host == "homerun.co" || host == "www.homerun.co" {
		return true
	}
	// Same pattern as homerun.co above: BambooHR's own marketing site
	// (www.bamboohr.com/integrations/..., /careers/, /pricing/, ...) gets
	// discovered as a posting (confirmed live 2026-07-22, "integrations"
	// listings page burned a 16-minute doc-gen cycle). Real BambooHR
	// postings are always on a company subdomain, e.g.
	// cxm.bamboohr.com/jobs/questions?id=169.
	if host == "bamboohr.com" || host == "www.bamboohr.com" {
		return true
	}
	// app.bamboohr.com is BambooHR's own shared login portal (every tenant's
	// employees log in there), not a job posting — confirmed live
	// 2026-07-22: app.bamboohr.com/login/ scored 80 and reached AttemptSubmit.
	if host == "app.bamboohr.com" {
		return true
	}
	if (host == "workable.com" || strings.HasSuffix(host, ".workable.com")) && strings.Contains(u.Path, "/search/") {
		return true
	}
	// Jobvite tenant search/listing pages are not postings (bugs.md #11,
	// observed live: jobs.jobvite.com/cloudone-digital/search scored 80 and
	// burned a full Learner cycle). Real Jobvite postings use /job/<id>.
	if (host == "jobvite.com" || strings.HasSuffix(host, ".jobvite.com")) && (strings.HasSuffix(strings.TrimSuffix(u.Path, "/"), "/search") || strings.Contains(u.Path, "/search/")) {
		return true
	}
	// Board-wide listing filters, not individual postings.
	if u.Query().Has("workplaceType") {
		return true
	}
	// On path-tenant ATS hosts, a single-segment path is the company's
	// board index (careers.smartrecruiters.com/aristanetworks — confirmed
	// live 2026-07-22 burning a full Learner+Vision cycle), never a
	// posting: real postings always carry at least one more segment
	// (/company/<uuid>, /company/jobs/<id>, /company/j/<id>).
	pathTenantATS := []string{"smartrecruiters.com", "lever.co", "greenhouse.io", "ashbyhq.com", "workable.com", "jobvite.com"}
	for _, domain := range pathTenantATS {
		if host == domain || strings.HasSuffix(host, "."+domain) {
			segments := 0
			for _, seg := range strings.Split(u.Path, "/") {
				if seg != "" {
					segments++
				}
			}
			if segments <= 1 {
				return true
			}
		}
	}
	// On applytojob.com (a subdomain-tenant platform, company.applytojob.com),
	// a bare "/apply" path with nothing after it is the company's full
	// "Current Openings" board index, never an individual posting — confirmed
	// live 2026-07-22 (holafly.applytojob.com/apply: 20 unrelated open roles
	// listed, no matching title anywhere, zero form fields). Real postings
	// carry a job ID and title slug, e.g.
	// /apply/z4xS0fd5C5/Senior-Backend-Engineer. Historically 0 APPLIED
	// across 176 attempts on this platform — this is very likely why.
	if strings.HasSuffix(host, ".applytojob.com") {
		segments := 0
		for _, seg := range strings.Split(u.Path, "/") {
			if seg != "" {
				segments++
			}
		}
		if segments <= 1 {
			return true
		}
	}
	// Same pattern again on recruitee.com (also subdomain-tenant): real
	// postings use /o/<slug> (2 segments); a bare single segment is either
	// the tenant's landing page (confirmed live 2026-07-22,
	// greatminds.recruitee.com/homepage — burned a full doc-gen + Learner
	// cycle) or, per several /o-only URLs seen earlier this session
	// (sensysgatsogroup, trafilea, primeworks), the job-board index under
	// the /o prefix itself.
	if strings.HasSuffix(host, ".recruitee.com") {
		segments := 0
		for _, seg := range strings.Split(u.Path, "/") {
			if seg != "" {
				segments++
			}
		}
		if segments <= 1 {
			return true
		}
	}
	return false
}

func isValidATSUrl(link string) bool {
	u, err := url.Parse(link)
	if err != nil {
		return false
	}
	
	if IsKnownJunkJobURL(link) {
		return false
	}
	host := strings.ToLower(u.Hostname())
	
	atsDomains := []string{
		"greenhouse.io", "lever.co", "ashbyhq.com",
		"bamboohr.com", "workable.com", "smartrecruiters.com",
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
			isNew, err := storage.AddToFunnel(roJob.Company, roJob.Position, roJob.URL, "DISCOVERED")
			if err != nil {
				log.Printf("[FunnelEngine] Failed to add %s to funnel: %v", roJob.URL, err)
			} else if isNew && jobChan != nil {
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
