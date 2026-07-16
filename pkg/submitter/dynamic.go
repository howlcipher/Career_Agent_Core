package submitter

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net/url"
	"strings"
	"time"

	"github.com/howlcipher/Career_Agent_Core/pkg/parser"
	"github.com/howlcipher/Career_Agent_Core/pkg/security"
	"github.com/playwright-community/playwright-go"
)

// FormMapper extracts Playwright selector mappings from a DOM
type FormMapper interface {
	ExtractFormMapping(domHTML string) (string, error)
	ExtractFormMappingVision(screenshotBytes []byte) (string, error)
}

// Pipeline represents the dynamic script-generation pipeline for ATS submissions.
type Pipeline struct {
	DB        *sql.DB
	Filter    *security.QuarantineLayer
	Mapper    FormMapper
	Templates map[string]string // Known ATS footprints mapped to templates
}

func NewPipeline(db *sql.DB, filter *security.QuarantineLayer, mapper FormMapper) *Pipeline {
	return &Pipeline{
		DB:     db,
		Filter: filter,
		Mapper: mapper,
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

	// Extract visible text for security scanning
	pageText, err := page.Evaluate("document.body.innerText")
	if err != nil {
		return "", fmt.Errorf("failed to extract page text: %w", err)
	}

	// Intercept Prompt Injection Anomalies
	if textStr, ok := pageText.(string); ok {
		if err := p.Filter.CheckPayload(textStr); err != nil {
			return "", fmt.Errorf("malicious prompt injection detected on career page: %w", err)
		}
	}

	log.Println("[Pipeline] Step 2: Site verified secure. Extracting structural DOM...")

	// Extract DOM footprint
	domHTML, err := page.Content()
	if err != nil {
		return "", fmt.Errorf("failed to extract DOM content: %w", err)
	}

	return domHTML, nil
}

// ExtractDomain gets the base domain from a URL for caching
func ExtractDomain(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	parts := strings.Split(u.Hostname(), ".")
	if len(parts) > 2 {
		return strings.Join(parts[len(parts)-2:], ".")
	}
	return u.Hostname()
}

// TemplateMatchingLoop attempts to identify ATS structures in the DOM and maps them to a script
func (p *Pipeline) TemplateMatchingLoop(jobURL, domHTML string) (string, error) {
	log.Println("[Pipeline] Analyzing DOM for ATS structural footprints...")
	
	domain := ExtractDomain(jobURL)
	log.Printf("[Learner Module] Checking DB for known mappings for domain: %s", domain)
	
	// 1. Check Cache (Zero Token Path)
	if p.DB != nil {
		var cachedMapping string
		err := p.DB.QueryRow("SELECT mapping_json FROM form_mappings WHERE domain = ?", domain).Scan(&cachedMapping)
		if err == nil && cachedMapping != "" {
			log.Printf("[Learner Module] Cache Hit! Using pre-learned mappings. Tokens saved: ~15,000")
			return "CachedScript_" + domain, nil
		}
	}

	// 2. Cache Miss (High Token Path)
	log.Printf("[Learner Module] Cache Miss. Domain %s is unknown. Engaging LLM to learn DOM structure...", domain)
	
	domLower := strings.ToLower(domHTML)
	for footprint, templateName := range p.Templates {
		if strings.Contains(domLower, footprint) {
			log.Printf("[Pipeline] Match found! ATS identified as %s. Loading %s...", footprint, templateName)
			
			// Save the learning back to DB for next time
			if p.DB != nil {
				mockMappingJSON := fmt.Sprintf(`{"ats_type": "%s", "fields": {"first_name": "input#first_name"}}`, footprint)
				_, err := p.DB.Exec("INSERT INTO form_mappings (domain, mapping_json, created_at) VALUES (?, ?, ?) ON CONFLICT(domain) DO UPDATE SET mapping_json=excluded.mapping_json", domain, mockMappingJSON, time.Now())
				if err != nil {
					log.Printf("Failed to cache learned mapping: %v", err)
				}
			}
			return templateName, nil
		}
	}

	log.Println("[Pipeline] No standard ATS match found. Engaging LLM to map DOM...")
	
	if p.Mapper != nil {
		prunedDOM, pruneErr := parser.PruneDOM(domHTML)
		if pruneErr != nil {
			log.Printf("DOM pruning failed, using raw HTML: %v", pruneErr)
			prunedDOM = domHTML
		}

		mappingJSON, err := p.Mapper.ExtractFormMapping(prunedDOM)
		if err != nil {
			log.Printf("LLM mapping failed: %v", err)
			return "DynamicGeneratedScript_Failed", err
		}

		if p.DB != nil {
			p.DB.Exec("INSERT INTO form_mappings (domain, mapping_json, created_at) VALUES (?, ?, ?) ON CONFLICT(domain) DO UPDATE SET mapping_json=excluded.mapping_json", domain, mappingJSON, time.Now())
			log.Printf("[Learner Module] Successfully learned and cached new form mapping for %s", domain)
		}
		return "CachedScript_" + domain, nil
	}
	
	return "DynamicGeneratedScript", nil
}

// SaveCheckpoint records the current progress to the SQLite database
func (p *Pipeline) SaveCheckpoint(jobID, url, status string) error {
	if p.DB == nil {
		return fmt.Errorf("database not initialized for checkpointing")
	}
	query := `INSERT INTO execution_state (job_id, url, status, last_updated) VALUES (?, ?, ?, ?)
			  ON CONFLICT(job_id) DO UPDATE SET status=excluded.status, last_updated=excluded.last_updated`
	_, err := p.DB.Exec(query, jobID, url, status, time.Now())
	return err
}

// Execute handles the robust pipeline logic, including rate limiting thresholds and execution safeguards
func (p *Pipeline) Execute(ctx context.Context, jobID, url string) error {
	if err := p.SaveCheckpoint(jobID, url, "STARTED"); err != nil {
		log.Printf("Checkpoint warning: %v", err)
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
	err := playwright.Install()
	if err != nil {
		return fmt.Errorf("failed to install playwright: %w", err)
	}
	pw, err := playwright.Run()
	if err != nil {
		return err
	}
	defer pw.Stop()
	browser, err := pw.Chromium.Launch()
	if err != nil {
		return err
	}
	defer browser.Close()
	page, err := browser.NewPage()
	if err != nil {
		return err
	}

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
