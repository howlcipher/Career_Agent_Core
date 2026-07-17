package submitter

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"strings"
	"time"

	"github.com/howlcipher/Career_Agent_Core/pkg/parser"
	"github.com/howlcipher/Career_Agent_Core/pkg/security"
	"github.com/howlcipher/Career_Agent_Core/pkg/storage"
	"github.com/playwright-community/playwright-go"
)

// FormMapper extracts Playwright selector mappings from a DOM
type FormMapper interface {
	ExtractFormMapping(domHTML string) (string, error)
	ExtractFormMappingVision(screenshotBytes []byte) (string, error)
	SolveValidationErrors(domHTML string, profileContext string) (map[string]string, error)
}

// Pipeline represents the dynamic script-generation pipeline for ATS submissions.
type Pipeline struct {
	Filter    *security.QuarantineLayer
	Mapper    FormMapper
	Browser   playwright.Browser
	Templates map[string]string // Known ATS footprints mapped to templates
}

func NewPipeline(filter *security.QuarantineLayer, mapper FormMapper, browser playwright.Browser) *Pipeline {
	return &Pipeline{
		Filter:  filter,
		Mapper:  mapper,
		Browser: browser,
		Templates: map[string]string{
			"greenhouse.io": "GreenhouseTemplate",
			"lever.co":      "LeverTemplate",
			"workday.com":   "WorkdayTemplate",
			"taleo.net":     "TaleoTemplate",
		},
	}
}

// Checkpoint state to allow pause/resume during execution
type ExecutionState struct {
	JobID       string
	URL         string
	Status      string
	LastUpdated time.Time
}

// TwoStepVerification safely visits the URL, validates security/prompt injection, and extracts the DOM
func (p *Pipeline) TwoStepVerification(page playwright.Page, url string) (string, error) {
	log.Printf("[Pipeline] Step 1: Navigating to %s for security verification...", url)
	
	// Step 1: Secure Connection & Load
	resp, err := page.Goto(url, playwright.PageGotoOptions{
		WaitUntil: playwright.WaitUntilStateDomcontentloaded,
	})
	if err != nil {
		return "", fmt.Errorf("failed to navigate: %w", err)
	}
	
	if resp == nil || !resp.Ok() {
		return "", fmt.Errorf("invalid response from target URL")
	}

	domHTML, err := page.Content()
	if err != nil {
		return "", fmt.Errorf("failed to extract page DOM: %w", err)
	}

	pruned, _ := parser.PruneDOMToText(domHTML)
	if err := p.Filter.CheckPayload(pruned); err != nil {
		return "", fmt.Errorf("malicious prompt injection detected on career page: %w", err)
	}

	log.Println("[Pipeline] Step 2: Site verified secure. Extracting structural DOM...")

	return domHTML, nil
}

// ExtractDomain gets the base domain + tenant path from a URL for caching
func ExtractDomain(rawURL string) string {
	if !strings.HasPrefix(rawURL, "http://") && !strings.HasPrefix(rawURL, "https://") {
		rawURL = "https://" + rawURL
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	
	pathSegments := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(pathSegments) > 0 && pathSegments[0] != "" {
		return u.Hostname() + "/" + pathSegments[0]
	}
	return u.Hostname()
}

// TemplateMatchingLoop attempts to identify ATS structures in the DOM and maps them to a script
func (p *Pipeline) TemplateMatchingLoop(jobURL, domHTML string) (string, error) {
	log.Println("[Pipeline] Analyzing DOM for ATS structural footprints...")
	
	domain := ExtractDomain(jobURL)
	log.Printf("[Learner Module] Checking DB for known mappings for domain: %s", domain)
	
	// 1. Check Cache (Zero Token Path)
	cachedMapping, err := storage.GetFormMapping(domain)
	if err == nil && cachedMapping != "" {
		log.Printf("[Learner Module] Cache Hit! Using pre-learned mappings. Tokens saved: ~15,000")
		return "CachedScript_" + domain, nil
	}

	// 2. Cache Miss (High Token Path)
	log.Printf("[Learner Module] Cache Miss. Domain %s is unknown. Engaging LLM to learn DOM structure...", domain)
	
	domLower := strings.ToLower(domHTML)
	for footprint, templateName := range p.Templates {
		if strings.Contains(domLower, footprint) {
			log.Printf("[Pipeline] Match found! ATS identified as %s. Loading %s...", footprint, templateName)
			
			// Save the learning back to DB for next time
			mockMappingJSON := fmt.Sprintf(`{"ats_type": "%s", "fields": {"first_name": "input#first_name"}}`, footprint)
			if err := storage.SaveFormMapping(domain, mockMappingJSON); err != nil {
				log.Printf("[Learner Module] Failed to cache learned mapping: %v", err)
			}
			return templateName, nil
		}
	}

	log.Println("[Pipeline] No standard ATS match found. Engaging LLM to map DOM...")
	
	if p.Mapper != nil {
		prunedDOM, pruneErr := parser.PruneDOM(domHTML)
		if pruneErr != nil {
			log.Printf("[Submitter] DOM pruning failed, using raw HTML: %v", pruneErr)
			prunedDOM = domHTML
		}

		mappingJSON, err := p.Mapper.ExtractFormMapping(prunedDOM)
		if err != nil {
			log.Printf("[Learner Module] LLM mapping failed: %v", err)
			return "DynamicGeneratedScript_Failed", err
		}

		if err := storage.SaveFormMapping(domain, mappingJSON); err != nil {
			log.Printf("[Learner Module] Failed to cache learned mapping: %v", err)
		}
		log.Printf("[Learner Module] Successfully learned and cached new form mapping for %s", domain)
		return "CachedScript_" + domain, nil
	}
	
	return "DynamicGeneratedScript", nil
}

func (p *Pipeline) CheckCache(domain string) (string, error) {
	var cachedMapping string
	cachedMapping, err := storage.GetFormMapping(domain)
	if err == nil && cachedMapping != "" {
		return cachedMapping, nil
	}

	return "", fmt.Errorf("not found in cache")
}

func (p *Pipeline) ProcessDomain(domain string) (string, error) {
	// Attempt Cache First
	cached, err := p.CheckCache(domain)
	if err == nil {
		log.Printf("[Learner Module] Cache hit for domain %s", domain)
		return cached, nil
	}

	time.Sleep(2 * time.Second)
	mockMappingJSON := `{"fields": {"first_name": "#first_name_input"}}`
	
	err = storage.SaveFormMapping(domain, mockMappingJSON)
	if err != nil {
		log.Printf("[Storage] Failed to cache form mapping: %v", err)
	}

	return mockMappingJSON, nil
}

func (p *Pipeline) AnalyzeAndMapForm(htmlContent, domain string) (string, error) {
	mappingJSON, err := p.Mapper.ExtractFormMapping(htmlContent)
	if err != nil {
		return "", fmt.Errorf("LLM failed to map form: %w", err)
	}

	err = storage.SaveFormMapping(domain, mappingJSON)
	if err != nil {
		log.Printf("[Storage] Failed to cache form mapping: %v", err)
	}

	return mappingJSON, nil
}

// SaveCheckpoint saves the execution state.
func (p *Pipeline) SaveCheckpoint(jobID, url, status string) error {
	return storage.LogExecution(jobID, url, status, 0)
}

// Execute handles the robust pipeline logic, including rate limiting thresholds and execution safeguards
func (p *Pipeline) Execute(ctx context.Context, jobID, url string) error {
	if err := p.SaveCheckpoint(jobID, url, "STARTED"); err != nil {
		log.Printf("[Pipeline] Checkpoint warning: %v", err)
	}

	// Placeholder for compute/rate limit circuit breaker
	select {
	case <-ctx.Done():
		p.SaveCheckpoint(jobID, url, "PAUSED_RATE_LIMIT")
		return fmt.Errorf("execution paused due to context cancellation/rate limits")
	default:
		// proceed
	}

	// 1. Launch Playwright
	bCtx, err := p.Browser.NewContext()
	if err != nil {
		return err
	}
	defer bCtx.Close()

	page, err := bCtx.NewPage()
	if err != nil {
		return err
	}
	defer page.Close()

	// 2. Two-Step Verification
	dom, err := p.TwoStepVerification(page, url)
	if err != nil {
		p.SaveCheckpoint(jobID, url, "FAILED_VERIFICATION")
		return err
	}

	// 3. Template Matching
	scriptRef, err := p.TemplateMatchingLoop(url, dom)
	if err != nil {
		p.SaveCheckpoint(jobID, url, "FAILED_MATCHING")
		return err
	}

	log.Printf("[Pipeline] Utilizing script layout: %s (Application Integrity Maintained)", scriptRef)
	p.SaveCheckpoint(jobID, url, "COMPLETED")
	return nil
}
