package submitter

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/howlcipher/Career_Agent_Core/pkg/config"
	"github.com/howlcipher/Career_Agent_Core/pkg/storage"
	"github.com/playwright-community/playwright-go"
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
	})
	if err != nil {
		return fmt.Errorf("could not launch browser: %w", err)
	}
	defer browser.Close()

	page, err := browser.NewPage()
	if err != nil {
		return fmt.Errorf("could not create page: %w", err)
	}

	log.Printf("[Auto-Submit] Navigating to %s", applyURL)
	if _, err = page.Goto(applyURL); err != nil {
		return fmt.Errorf("could not navigate to apply URL: %w", err)
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
