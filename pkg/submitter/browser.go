package submitter

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/howlcipher/Career_Agent_Core/pkg/config"
	"github.com/howlcipher/Career_Agent_Core/pkg/storage"
	"github.com/mxschmitt/playwright-go"
)

// AttemptSubmit scaffolds the architecture for headless browser auto-submission.
// Because job boards use heavily varied Application Tracking Systems (ATS) (like Workday, Greenhouse, Lever),
// an automated submitter requires custom DOM-parsing logic per platform.
func AttemptSubmit(companyName, applyURL, resumePath, coverLetterPath string, pii *config.PII, headlessBrowser, autoSubmitClick bool) error {
	log.Printf("[Auto-Submit] Initiating submission sequence for %s at %s", companyName, applyURL)

	// Install playwright browsers if they don't exist
	err := playwright.Install()
	if err != nil {
		return fmt.Errorf("failed to install playwright browsers: %w", err)
	}

	pw, err := playwright.Run()
	if err != nil {
		return fmt.Errorf("could not start playwright: %w", err)
	}
	defer pw.Stop()

	browser, err := pw.Chromium.Launch(playwright.BrowserTypeLaunchOptions{
		Headless: playwright.Bool(headlessBrowser),
		Args: []string{
			"--disable-blink-features=AutomationControlled", // Stealth: hide automation flag
			"--disable-infobars",
		},
	})
	if err != nil {
		return fmt.Errorf("could not launch browser: %w", err)
	}
	defer browser.Close()

	page, err := browser.NewPage(playwright.BrowserNewPageOptions{
		UserAgent: playwright.String("Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"),
	})
	if err != nil {
		return fmt.Errorf("could not create page: %w", err)
	}

	// Stealth: Overwrite navigator.webdriver
	page.AddInitScript(playwright.Script{
		Content: playwright.String("Object.defineProperty(navigator, 'webdriver', {get: () => undefined})"),
	})

	log.Printf("[Auto-Submit] Navigating to %s", applyURL)
	if _, err = page.Goto(applyURL, playwright.PageGotoOptions{
		WaitUntil: playwright.WaitUntilStateNetworkidle,
	}); err != nil {
		log.Printf("[Auto-Submit] NetworkIdle wait timed out or failed. Falling back to Domcontentloaded...")
		if _, err = page.Goto(applyURL, playwright.PageGotoOptions{
			WaitUntil: playwright.WaitUntilStateDomcontentloaded,
		}); err != nil {
			return fmt.Errorf("could not navigate to apply URL after retries: %w", err)
		}
	}

	domain := ExtractDomain(applyURL)
	mappingJSON, err := storage.GetFormMapping(domain)
	if err == nil && mappingJSON != "" {
		log.Printf("[Auto-Submit] Using learned dynamic mapping for %s", domain)
		dynErr := handleDynamic(page, resumePath, pii, mappingJSON, autoSubmitClick)
		if dynErr != nil {
			log.Printf("[Auto-Submit] Dynamic Playwright mapping failed for %s. Invalidating cache. Error: %v", domain, dynErr)
			storage.DeleteFormMapping(domain)
			return fmt.Errorf("dynamic execution failed, cache cleared: %w", dynErr)
		}
		return nil
	}

	urlLower := strings.ToLower(applyURL)
	if strings.Contains(urlLower, "linkedin.com/jobs") {
		return handleLinkedIn(page, resumePath, pii, autoSubmitClick)
	} else if strings.Contains(urlLower, "greenhouse.io") || strings.Contains(urlLower, "boards.greenhouse.io") {
		return handleGreenhouse(page, resumePath, pii, autoSubmitClick)
	} else if strings.Contains(urlLower, "lever.co") || strings.Contains(urlLower, "jobs.lever.co") {
		return handleLever(page, resumePath, pii, autoSubmitClick)
	}

	return fmt.Errorf("unsupported Applicant Tracking System at %s", applyURL)
}

func handleLinkedIn(page playwright.Page, resumePath string, pii *config.PII, autoSubmitClick bool) error {
	log.Printf("[Auto-Submit] Detected LinkedIn Job. Implementing Easy Apply automation...")
	
	// Click Easy Apply button
	easyApplyBtn := page.Locator("button.jobs-apply-button")
	if count, _ := easyApplyBtn.Count(); count > 0 {
		easyApplyBtn.First().Click()
	} else {
		return fmt.Errorf("could not find Easy Apply button")
	}

	// This is a complex multi-step modal in LinkedIn. 
	// We'd upload the resume:
	// page.SetInputFiles("input[type='file']", []playwright.InputFile{{Name: "resume.md", Path: resumePath}})
	
	// A full implementation would step through the Next buttons.
	return fmt.Errorf("linkedin easy apply modal interaction not fully implemented")
}

func handleGreenhouse(page playwright.Page, resumePath string, pii *config.PII, autoSubmitClick bool) error {
	log.Printf("[Auto-Submit] Detected Greenhouse ATS. Filling out fields...")

	if _, err := page.WaitForSelector("input#first_name", playwright.PageWaitForSelectorOptions{
		Timeout: playwright.Float(15000),
	}); err != nil {
		return fmt.Errorf("form failed to render in time: %w", err)
	}

	// Basic fields
	if pii != nil {
		page.Locator("input#first_name").Fill("William")
		page.Locator("input#last_name").Fill("Elias")
		if pii.Email != "" {
			page.Locator("input#email").Fill(pii.Email)
		}
		if pii.Phone != "" {
			page.Locator("input#phone").Fill(pii.Phone)
		}
	}

	// Upload resume
	fileInput := page.Locator("input[type='file'][name='resume']")
	if count, _ := fileInput.Count(); count > 0 {
		fileBytes, err := os.ReadFile(resumePath)
		if err == nil {
			fileInput.First().SetInputFiles([]playwright.InputFile{{
				Name:   "resume.pdf", 
				Buffer: fileBytes,
			}})
		} else {
			log.Printf("Failed to read resume for upload: %v", err)
		}
	}

	if autoSubmitClick {
		page.Locator("input#submit_app").Click()
	}
	
	return nil
}

func handleLever(page playwright.Page, resumePath string, pii *config.PII, autoSubmitClick bool) error {
	log.Printf("[Auto-Submit] Detected Lever ATS. Filling out fields...")

	if _, err := page.WaitForSelector("input[name='name']", playwright.PageWaitForSelectorOptions{
		Timeout: playwright.Float(15000),
	}); err != nil {
		return fmt.Errorf("form failed to render in time: %w", err)
	}

	if pii != nil {
		page.Locator("input[name='name']").Fill("William Elias")
		if pii.Email != "" {
			page.Locator("input[name='email']").Fill(pii.Email)
		}
		if pii.Phone != "" {
			page.Locator("input[name='phone']").Fill(pii.Phone)
		}
	}

	fileInput := page.Locator("input[type='file'][id='resume-upload-input']")
	if count, _ := fileInput.Count(); count > 0 {
		fileBytes, err := os.ReadFile(resumePath)
		if err == nil {
			fileInput.First().SetInputFiles([]playwright.InputFile{{
				Name:   "resume.pdf",
				Buffer: fileBytes,
			}})
		} else {
			log.Printf("Failed to read resume for upload: %v", err)
		}
	}

	if autoSubmitClick {
		page.Locator("button.postings-btn.template-btn-submit").Click()
	}

	return nil
}

type FormMapping struct {
	Fields map[string]string `json:"fields"`
}

func safeFill(page playwright.Page, selector, text string) error {
	if selector == "" || text == "" {
		return nil
	}
	return page.Locator(selector).Fill(text, playwright.LocatorFillOptions{Timeout: playwright.Float(5000)})
}

func handleDynamic(page playwright.Page, resumePath string, pii *config.PII, mappingJSON string, autoSubmitClick bool) error {
	log.Printf("[Auto-Submit] Executing dynamic Playwright mapping...")
	var mapping FormMapping
	if err := json.Unmarshal([]byte(mappingJSON), &mapping); err != nil {
		return fmt.Errorf("failed to parse mapping json: %w", err)
	}

	if pii != nil {
		if err := safeFill(page, mapping.Fields["first_name"], "William"); err != nil {
			return fmt.Errorf("failed to fill first_name: %w", err)
		}
		if err := safeFill(page, mapping.Fields["last_name"], "Elias"); err != nil {
			return fmt.Errorf("failed to fill last_name: %w", err)
		}
		if err := safeFill(page, mapping.Fields["email"], pii.Email); err != nil {
			return fmt.Errorf("failed to fill email: %w", err)
		}
		if err := safeFill(page, mapping.Fields["phone"], pii.Phone); err != nil {
			return fmt.Errorf("failed to fill phone: %w", err)
		}
	}

	if sel, ok := mapping.Fields["resume"]; ok && sel != "" {
		fileInput := page.Locator(sel)
		if count, _ := fileInput.Count(); count > 0 {
			fileBytes, err := os.ReadFile(resumePath)
			if err == nil {
				err = fileInput.First().SetInputFiles([]playwright.InputFile{{
					Name:   "resume.pdf",
					Buffer: fileBytes,
				}}, playwright.LocatorSetInputFilesOptions{Timeout: playwright.Float(5000)})
				if err != nil {
					return fmt.Errorf("failed to upload resume: %w", err)
				}
			}
		} else {
			return fmt.Errorf("resume input selector not found")
		}
	}

	if autoSubmitClick {
		if sel, ok := mapping.Fields["submit_button"]; ok && sel != "" {
			err := page.Locator(sel).Click(playwright.LocatorClickOptions{Timeout: playwright.Float(5000)})
			if err != nil {
				return fmt.Errorf("failed to click submit: %w", err)
			}
		}
	}

	return nil
}

// PruneDOM removes <script>, <style>, and <svg> tags to drastically reduce token counts for Gemini LLM.
func PruneDOM(html string) string {
	// A full implementation would use x/net/html, but basic string manipulation works for simple minification
	rules := []struct {
		open  string
		close string
	}{
		{"<script", "</script>"},
		{"<style", "</style>"},
		{"<svg", "</svg>"},
	}

	res := html
	for _, rule := range rules {
		for {
			start := strings.Index(res, rule.open)
			if start == -1 {
				break
			}
			end := strings.Index(res[start:], rule.close)
			if end == -1 {
				// Malformed, just remove the open tag
				res = res[:start] + res[start+len(rule.open):]
				break
			}
			res = res[:start] + res[start+end+len(rule.close):]
		}
	}
	return res
}
