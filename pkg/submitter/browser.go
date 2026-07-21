package submitter

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/url"
	"os"
	"strings"

	"github.com/howlcipher/Career_Agent_Core/pkg/config"
	"github.com/howlcipher/Career_Agent_Core/pkg/parser"
	"github.com/howlcipher/Career_Agent_Core/pkg/security"
	"github.com/howlcipher/Career_Agent_Core/pkg/storage"
	"github.com/mxschmitt/playwright-go"
)

// fillTarget abstracts over playwright.Page and playwright.Frame so form-fill
// logic can transparently target whichever one actually holds the form.
// Many ATS platforms (SmartRecruiters, Workday, and others) embed the real
// application form in an <iframe>; searching only the top-level page for
// input#first_name etc. finds nothing and times out no matter how long the
// timeout is, since the element genuinely isn't in that document.
type fillTarget interface {
	Loc(selector string) playwright.Locator
	WaitForSel(selector string, timeoutMs float64) (playwright.ElementHandle, error)
	HTML() (string, error)
}

type pageTarget struct{ page playwright.Page }

func (t pageTarget) Loc(selector string) playwright.Locator { return t.page.Locator(selector) }
func (t pageTarget) WaitForSel(selector string, timeoutMs float64) (playwright.ElementHandle, error) {
	return t.page.WaitForSelector(selector, playwright.PageWaitForSelectorOptions{Timeout: playwright.Float(timeoutMs)})
}
func (t pageTarget) HTML() (string, error) { return t.page.Content() }

type frameTarget struct{ frame playwright.Frame }

func (t frameTarget) Loc(selector string) playwright.Locator { return t.frame.Locator(selector) }
func (t frameTarget) WaitForSel(selector string, timeoutMs float64) (playwright.ElementHandle, error) {
	return t.frame.WaitForSelector(selector, playwright.FrameWaitForSelectorOptions{Timeout: playwright.Float(timeoutMs)})
}
func (t frameTarget) HTML() (string, error) { return t.frame.Content() }

// clickApplyIfPresent looks for a visible "Apply"-labeled clickable element on the
// top-level page and clicks it, giving click-to-reveal application forms (a
// fancybox/lightbox modal, an in-page form injected by JS, etc.) a chance to render
// before the Learner Module inspects the DOM or the fill logic looks for form
// fields. No-ops silently if no such element is found, since most ATS platforms
// already show the form directly without requiring a click.
func clickApplyIfPresent(page playwright.Page) {
	locator := page.Locator("button:has-text('Apply'), a:has-text('Apply')").First()
	count, err := locator.Count()
	if err != nil || count == 0 {
		return
	}
	if err := locator.Click(playwright.LocatorClickOptions{Timeout: playwright.Float(5000)}); err != nil {
		log.Printf("[Auto-Submit] Found an Apply-labeled element but failed to click it: %v", err)
		return
	}
	log.Printf("[Auto-Submit] Clicked an Apply-labeled element to reveal the application form")
	page.WaitForTimeout(2000)
}

// resolveFillTarget picks the top-level page if it already contains form
// inputs, otherwise scans child frames for the first one that does (the
// common case for embedded-widget ATS platforms). Falls back to the page
// itself if no frame has any inputs either, so callers get a normal
// "selector not found" error instead of a nil target.
func resolveFillTarget(page playwright.Page) fillTarget {
	if count, _ := page.Locator("input, textarea, select").Count(); count > 0 {
		return pageTarget{page}
	}
	for _, f := range page.Frames() {
		if f == page.MainFrame() {
			continue
		}
		if count, _ := f.Locator("input, textarea, select").Count(); count > 0 {
			log.Printf("[Auto-Submit] Form fields not found on main page; using embedded iframe (%s) instead", f.URL())
			return frameTarget{f}
		}
	}
	return pageTarget{page}
}

// AttemptSubmit scaffolds the architecture for headless browser auto-submission.
// Because job boards use heavily varied Application Tracking Systems (ATS) (like Workday, Greenhouse, Lever),
// an automated submitter requires custom DOM-parsing logic per platform.
func AttemptSubmit(browser playwright.Browser, filter *security.QuarantineLayer, mapper FormMapper, companyName, applyURL string, generateDocs func() (string, string, error), pii *config.PII, profileContext string, headlessBrowser, autoSubmitClick bool) error {
	log.Printf("[Auto-Submit] Initiating submission sequence for %s at %s", companyName, applyURL)

	bCtx, err := browser.NewContext(playwright.BrowserNewContextOptions{
		UserAgent: playwright.String("Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/120.0.0.0"),
	})
	if err != nil {
		return fmt.Errorf("could not create context: %w", err)
	}
	defer bCtx.Close()

	page, err := bCtx.NewPage()
	if err != nil {
		return fmt.Errorf("could not create page: %w", err)
	}
	defer page.Close()

	// Anti-SSRF Route Filter
	err = page.Route("**/*", func(route playwright.Route) {
		reqURL, _ := url.Parse(route.Request().URL())
		if reqURL != nil {
			ip := net.ParseIP(reqURL.Hostname())
			if ip != nil && (ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsUnspecified()) {
				route.Abort("accessdenied")
				return
			}
			if reqURL.Hostname() == "localhost" {
				route.Abort("accessdenied")
				return
			}
		}
		route.Continue()
	})
	if err != nil {
		return fmt.Errorf("failed to setup SSRF route blocking: %w", err)
	}

	// Set a strict 45-second global timeout for all page operations (navigation, clicks, fills).
	// If a captcha blocks the page, Playwright will time out instead of hanging the worker forever.
	page.SetDefaultTimeout(45000)

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

	// Wait for a brief moment to let bot-protection scripts (like Cloudflare) reveal themselves
	page.WaitForTimeout(2000)
	
	// Check for obvious dead ends
	content, _ := page.Content()
	lowerContent := strings.ToLower(content)
	if strings.Contains(lowerContent, "job is no longer available") || strings.Contains(lowerContent, "position has been filled") || strings.Contains(lowerContent, "404 not found") {
		return fmt.Errorf("job posting is dead or expired")
	}

	// At this point, the page is live. NOW we generate the costly resume and cover letter!
	log.Printf("[Auto-Submit] Verified page is live. Generating tailored documents...")
	resumePath, coverPath, err := generateDocs()
	if err != nil {
		return fmt.Errorf("failed to generate application documents: %w", err)
	}
	if !strings.Contains(resumePath, "master_resume") {
		defer os.Remove(resumePath)
	}
	if !strings.Contains(coverPath, "master_cover") {
		defer os.Remove(coverPath)
	}

	domain := ExtractDomain(applyURL)
	mappingJSON, err := storage.GetFormMapping(domain)
	if err == nil && mappingJSON != "" {
		log.Printf("[Auto-Submit] Using learned dynamic mapping for %s", domain)
		dynErr := handleDynamic(resolveFillTarget(page), resumePath, pii, mappingJSON, autoSubmitClick)
		if dynErr != nil {
			log.Printf("[Auto-Submit] Dynamic Playwright mapping failed for %s. Invalidating cache. Error: %v", domain, dynErr)
			storage.DeleteFormMapping(domain)
			return fmt.Errorf("dynamic execution failed, cache cleared: %w", dynErr)
		}
		return nil
	}
	urlLower := strings.ToLower(applyURL)
	var execErr error
	var initialAttemptComplete bool

	for attempt := 1; attempt <= 3; attempt++ {
		if !initialAttemptComplete {
			if strings.Contains(urlLower, "linkedin.com/jobs") {
				execErr = handleLinkedIn(page, resumePath, pii, autoSubmitClick)
			} else if strings.Contains(urlLower, "greenhouse.io") || strings.Contains(urlLower, "boards.greenhouse.io") {
				execErr = handleGreenhouse(resolveFillTarget(page), resumePath, pii, autoSubmitClick)
			} else if strings.Contains(urlLower, "lever.co") || strings.Contains(urlLower, "jobs.lever.co") {
				execErr = handleLever(resolveFillTarget(page), resumePath, pii, autoSubmitClick)
			} else if mapper != nil {
				log.Printf("[Auto-Submit] Unknown ATS %s. Triggering Learner Module...", domain)
				clickApplyIfPresent(page)
				target := resolveFillTarget(page)
				domHTML, _ := target.HTML()
				prunedHTML, err := parser.PruneDOMToText(domHTML)
				if err != nil {
					prunedHTML = domHTML
				}

				if filter != nil {
					if err := filter.CheckPayload(prunedHTML); err != nil {
						return fmt.Errorf("malicious prompt injection detected on career page: %w", err)
					}
				}

				newMappingJSON, err := mapper.ExtractFormMapping(prunedHTML)
				if err == nil && newMappingJSON != "" {
					log.Printf("[Learner Module] Successfully mapped %s. Saving and re-attempting...", domain)
					storage.SaveFormMapping(domain, newMappingJSON)
					execErr = handleDynamic(target, resumePath, pii, newMappingJSON, autoSubmitClick)
				} else {
					log.Printf("[Learner Module] Failed to map form: %v", err)
					log.Printf("[Auto-Submit] DOM Learner Module failed. Falling back to Vision module...")
					execErr = AttemptVisionSubmit(page, target, companyName, applyURL, resumePath, pii, mapper, autoSubmitClick)
				}
			} else {
				execErr = fmt.Errorf("unsupported Applicant Tracking System at %s", applyURL)
			}
			initialAttemptComplete = true
		} else {
			log.Printf("[Auto-Submit] Attempt %d: Solving validation errors...", attempt)
			target := resolveFillTarget(page)
			domHTML, _ := target.HTML()
			prunedHTML, err := parser.PruneDOM(domHTML)
			if err != nil { prunedHTML = domHTML }

			fixesMap, fixErr := mapper.SolveValidationErrors(prunedHTML, pii.EEO.Summary()+"\n\n"+profileContext)
			if fixErr != nil {
				return fmt.Errorf("failed to solve validation errors: %w", fixErr)
			}

			for selector, value := range fixesMap {
				safeFill(target, selector, value)
			}

			submitLocator := target.Loc("input[type='submit'], button[type='submit'], button:has-text('Submit'), button:has-text('Apply')")
			if count, _ := submitLocator.Count(); count > 0 {
				execErr = submitLocator.First().Click(playwright.LocatorClickOptions{Timeout: playwright.Float(15000)})
			} else {
				execErr = fmt.Errorf("could not find submit button to retry submission")
			}
		}

		if execErr != nil {
			return execErr
		}

		if autoSubmitClick {
			page.WaitForLoadState(playwright.PageWaitForLoadStateOptions{
				State: playwright.LoadStateNetworkidle,
				Timeout: playwright.Float(10000),
			})

			currentURL := page.URL()
			if currentURL != applyURL || strings.Contains(strings.ToLower(currentURL), "thank") || strings.Contains(strings.ToLower(currentURL), "success") || strings.Contains(strings.ToLower(currentURL), "confirmation") {
				return nil
			}
			
			log.Printf("[Auto-Submit] Submission failed validation. Retrying...")
		} else {
			return nil
		}
	}

	return fmt.Errorf("failed to submit application after 3 validation error attempts")
}

func handleLinkedIn(page playwright.Page, resumePath string, pii *config.PII, autoSubmitClick bool) error {
	log.Printf("[Auto-Submit] Detected LinkedIn Job. Implementing Easy Apply automation...")
	
	// Click Easy Apply button
	easyApplyBtn := page.Locator("button.jobs-apply-button")
	if count, _ := easyApplyBtn.Count(); count > 0 {
		if err := easyApplyBtn.First().Click(); err != nil {
			log.Printf("[Playwright] Failed to click Easy Apply button: %v", err)
			return fmt.Errorf("failed to click easy apply: %w", err)
		}
	} else {
		return fmt.Errorf("could not find Easy Apply button")
	}

	// This is a complex multi-step modal in LinkedIn. 
	// We'd upload the resume:
	// page.SetInputFiles("input[type='file']", []playwright.InputFile{{Name: "resume.md", Path: resumePath}})
	
	// A full implementation would step through the Next buttons.
	return fmt.Errorf("linkedin easy apply modal interaction not fully implemented")
}

func handleGreenhouse(target fillTarget, resumePath string, pii *config.PII, autoSubmitClick bool) error {
	log.Printf("[Auto-Submit] Detected Greenhouse ATS. Filling out fields...")

	if _, err := target.WaitForSel("input#first_name", 30000); err != nil {
		return fmt.Errorf("form failed to render in time: %w", err)
	}

	// Basic fields
	if pii != nil {
		if pii.FirstName != "" {
			if err := target.Loc("input#first_name").Fill(pii.FirstName); err != nil {
				return fmt.Errorf("failed to fill first_name: %w", err)
			}
		}
		if pii.LastName != "" {
			if err := target.Loc("input#last_name").Fill(pii.LastName); err != nil {
				return fmt.Errorf("failed to fill last_name: %w", err)
			}
		}
		if pii.Email != "" {
			if err := target.Loc("input#email").Fill(pii.Email); err != nil {
				return fmt.Errorf("failed to fill email: %w", err)
			}
		}
		if pii.Phone != "" {
			if err := target.Loc("input#phone").Fill(pii.Phone); err != nil {
				return fmt.Errorf("failed to fill phone: %w", err)
			}
		}
	}

	// Upload resume
	fileInput := target.Loc("input[type='file'][name='resume']")
	if count, _ := fileInput.Count(); count > 0 {
		fileBytes, err := os.ReadFile(resumePath)
		if err == nil {
			if err := fileInput.First().SetInputFiles([]playwright.InputFile{{
				Name:   "resume.pdf",
				Buffer: fileBytes,
			}}); err != nil {
				return fmt.Errorf("failed to set resume file: %w", err)
			}
		} else {
			log.Printf("[Auto-Submit] Failed to read resume for upload: %v", err)
		}
	}

	if autoSubmitClick {
		if err := target.Loc("input#submit_app").Click(); err != nil {
			return fmt.Errorf("failed to click submit: %w", err)
		}
	}

	return nil
}

func handleLever(target fillTarget, resumePath string, pii *config.PII, autoSubmitClick bool) error {
	log.Printf("[Auto-Submit] Detected Lever ATS. Filling out fields...")

	if _, err := target.WaitForSel("input[name='name']", 30000); err != nil {
		return fmt.Errorf("form failed to render in time: %w", err)
	}

	if pii != nil {
		if pii.FirstName != "" || pii.LastName != "" {
			if err := target.Loc("input[name='name']").Fill(pii.FirstName + " " + pii.LastName); err != nil {
				return fmt.Errorf("failed to fill name: %w", err)
			}
		}
		if pii.Email != "" {
			if err := target.Loc("input[name='email']").Fill(pii.Email); err != nil {
				return fmt.Errorf("failed to fill email: %w", err)
			}
		}
		if pii.Phone != "" {
			if err := target.Loc("input[name='phone']").Fill(pii.Phone); err != nil {
				return fmt.Errorf("failed to fill phone: %w", err)
			}
		}
	}

	fileInput := target.Loc("input[type='file'][id='resume-upload-input']")
	if count, _ := fileInput.Count(); count > 0 {
		fileBytes, err := os.ReadFile(resumePath)
		if err == nil {
			if err := fileInput.First().SetInputFiles([]playwright.InputFile{{
				Name:   "resume.pdf",
				Buffer: fileBytes,
			}}); err != nil {
				return fmt.Errorf("failed to set resume file: %w", err)
			}
		} else {
			log.Printf("[Auto-Submit] Failed to read resume for upload: %v", err)
		}
	}

	if autoSubmitClick {
		if err := target.Loc("button.postings-btn.template-btn-submit").Click(); err != nil {
			return fmt.Errorf("failed to click submit: %w", err)
		}
	}

	return nil
}

type FormMapping struct {
	Fields map[string]string `json:"fields"`
}

var ErrEmptySelector = fmt.Errorf("empty selector provided for form filling")

func safeFill(target fillTarget, selector, text string) error {
	if selector == "" {
		return ErrEmptySelector
	}
	if text == "" {
		return nil
	}
	return target.Loc(selector).Fill(text, playwright.LocatorFillOptions{Timeout: playwright.Float(15000)})
}

func handleDynamic(target fillTarget, resumePath string, pii *config.PII, mappingJSON string, autoSubmitClick bool) error {
	log.Printf("[Auto-Submit] Executing dynamic Playwright mapping...")
	var mapping FormMapping
	if err := json.Unmarshal([]byte(mappingJSON), &mapping); err != nil {
		return fmt.Errorf("failed to parse mapping json: %w", err)
	}

	if pii != nil {
		if pii.FirstName != "" {
			if err := safeFill(target, mapping.Fields["first_name"], pii.FirstName); err != nil {
				return fmt.Errorf("failed to fill first_name: %w", err)
			}
		}
		if pii.LastName != "" {
			if err := safeFill(target, mapping.Fields["last_name"], pii.LastName); err != nil {
				return fmt.Errorf("failed to fill last_name: %w", err)
			}
		}
		if err := safeFill(target, mapping.Fields["email"], pii.Email); err != nil {
			return fmt.Errorf("failed to fill email: %w", err)
		}
		if err := safeFill(target, mapping.Fields["phone"], pii.Phone); err != nil {
			return fmt.Errorf("failed to fill phone: %w", err)
		}
	}

	if sel, ok := mapping.Fields["resume"]; ok && sel != "" {
		fileInput := target.Loc(sel)
		if count, _ := fileInput.Count(); count > 0 {
			fileBytes, err := os.ReadFile(resumePath)
			if err == nil {
				err = fileInput.First().SetInputFiles([]playwright.InputFile{{
					Name:   "resume.pdf",
					Buffer: fileBytes,
				}}, playwright.LocatorSetInputFilesOptions{Timeout: playwright.Float(15000)})
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
			err := target.Loc(sel).Click(playwright.LocatorClickOptions{Timeout: playwright.Float(15000)})
			if err != nil {
				return fmt.Errorf("failed to click submit: %w", err)
			}
		}
	}

	return nil
}


