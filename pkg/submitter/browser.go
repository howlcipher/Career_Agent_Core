package submitter

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/url"
	"os"
	"strings"

	"github.com/danielthedm/promptsec"
	"github.com/howlcipher/Career_Agent_Core/pkg/config"
	"github.com/howlcipher/Career_Agent_Core/pkg/parser"
	"github.com/howlcipher/Career_Agent_Core/pkg/security"
	"github.com/howlcipher/Career_Agent_Core/pkg/storage"
	"github.com/mxschmitt/playwright-go"
)

// toStoredThreats converts promptsec's threat type to storage's own mirror,
// so pkg/storage doesn't need to depend on the security package's
// third-party dependency just to log what was found.
func toStoredThreats(threats []promptsec.Threat) []storage.PromptInjectionThreat {
	out := make([]storage.PromptInjectionThreat, 0, len(threats))
	for _, t := range threats {
		out = append(out, storage.PromptInjectionThreat{
			Type:     string(t.Type),
			Severity: t.Severity,
			Message:  t.Message,
			Guard:    t.Guard,
			Match:    t.Match,
			Start:    t.Start,
			End:      t.End,
		})
	}
	return out
}

// fillTarget abstracts over playwright.Page and playwright.Frame so form-fill
// logic can transparently target whichever one actually holds the form.
// Many ATS platforms (SmartRecruiters, Workday, and others) embed the real
// application form in an <iframe>; searching only the top-level page for
// input#first_name etc. finds nothing and times out no matter how long the
// timeout is, since the element genuinely isn't in that document.
type fillTarget interface {
	Loc(selector string) playwright.Locator
	// GetByLabelLoc finds an input by its visible accessible label text (a
	// <label for="...">, aria-label, or aria-labelledby association) instead
	// of a CSS selector. WCAG-compliant ATS forms - most enterprise ones -
	// reliably expose this even when their raw name/id attributes are
	// obfuscated or vary by vendor theme, making it a more robust fallback
	// than an LLM-guessed CSS selector.
	GetByLabelLoc(text string) playwright.Locator
	// GetByPlaceholderLoc finds an input by its placeholder text. Confirmed
	// live 2026-07-21 (bug #16): some minimalist ATS widgets (Jobvite,
	// ApplyToJob) style an input's placeholder to look exactly like a label
	// to a human, with no real <label>/aria-label association at all - a
	// case GetByLabelLoc structurally cannot match, since there's no
	// accessible label to find.
	GetByPlaceholderLoc(text string) playwright.Locator
	WaitForSel(selector string, timeoutMs float64) (playwright.ElementHandle, error)
	HTML() (string, error)
}

type pageTarget struct{ page playwright.Page }

func (t pageTarget) Loc(selector string) playwright.Locator { return t.page.Locator(selector) }
func (t pageTarget) GetByLabelLoc(text string) playwright.Locator {
	return t.page.GetByLabel(text)
}
func (t pageTarget) GetByPlaceholderLoc(text string) playwright.Locator {
	return t.page.GetByPlaceholder(text)
}
func (t pageTarget) WaitForSel(selector string, timeoutMs float64) (playwright.ElementHandle, error) {
	return t.page.WaitForSelector(selector, playwright.PageWaitForSelectorOptions{Timeout: playwright.Float(timeoutMs)})
}
func (t pageTarget) HTML() (string, error) { return t.page.Content() }

type frameTarget struct{ frame playwright.Frame }

func (t frameTarget) Loc(selector string) playwright.Locator { return t.frame.Locator(selector) }
func (t frameTarget) GetByLabelLoc(text string) playwright.Locator {
	return t.frame.GetByLabel(text)
}
func (t frameTarget) GetByPlaceholderLoc(text string) playwright.Locator {
	return t.frame.GetByPlaceholder(text)
}
func (t frameTarget) WaitForSel(selector string, timeoutMs float64) (playwright.ElementHandle, error) {
	return t.frame.WaitForSelector(selector, playwright.FrameWaitForSelectorOptions{Timeout: playwright.Float(timeoutMs)})
}
func (t frameTarget) HTML() (string, error) { return t.frame.Content() }

// deadJobPhrases are lowercase substrings different ATS platforms use to say a
// posting is gone. Confirmed live 2026-07-21: a Jobvite listing that had
// expired between discovery and AttemptSubmit said "the job listing no
// longer [exists]" — a phrasing the original single hardcoded check for
// "job is no longer available" did not match, so the dead-job guard silently
// let an expired posting through a full generation + Learner Module cycle.
var deadJobPhrases = []string{
	"job is no longer available",
	"position has been filled",
	"404 not found",
	"no longer exists",
	"no longer accepting applications",
	"job listing no longer",
	"posting is no longer active",
	"job has been filled",
	// Lever renders expired postings as a 404 shell titled
	// "Not found – 404 error" with an HTTP 200 status (bugs.md #15).
	"404 error",
}

// registrableDomain approximates eTLD+1 with the host's last two labels —
// sufficient for the ATS domains this agent targets (greenhouse.io,
// lever.co, myworkdayjobs.com, ...), which all sit directly under
// two-label public suffixes.
func registrableDomain(host string) string {
	parts := strings.Split(strings.ToLower(host), ".")
	if len(parts) <= 2 {
		return strings.ToLower(host)
	}
	return strings.Join(parts[len(parts)-2:], ".")
}

// deadRedirectReason reports why the post-navigation URL indicates the
// posting is gone (bugs.md #15): ATS expired-posting redirects append an
// error query parameter (Greenhouse `?error=true`, Jobvite `?error=404`),
// and companies that migrate their board off the ATS redirect to their own
// domain (boards.eu.greenhouse.io -> careers.nebius.com), landing on a
// generic careers page that will never contain this posting's form. Returns
// "" when the final URL still plausibly hosts the posting — same-domain
// redirects (boards.greenhouse.io -> job-boards.greenhouse.io) are allowed
// through, since board migrations within the ATS can preserve the posting.
func deadRedirectReason(applyURL, finalURL string) string {
	from, errFrom := url.Parse(applyURL)
	to, errTo := url.Parse(finalURL)
	if errFrom != nil || errTo != nil || to.Host == "" {
		return ""
	}
	if to.Query().Has("error") {
		return fmt.Sprintf("redirected to an error page (%s)", finalURL)
	}
	if registrableDomain(from.Hostname()) != registrableDomain(to.Hostname()) {
		return fmt.Sprintf("redirected off the posting's ATS domain to %s", to.Hostname())
	}
	return ""
}

// ErrAuthWall marks an application flow gated behind account creation or
// sign-in, where no fillable application form exists pre-auth (bug #18:
// Workday). Callers should route these to the manual-submission backlog
// (the tailored documents are already generated and saved) instead of
// treating them as an automation failure.
var ErrAuthWall = errors.New("application form is gated behind account sign-in")

// authGatedATSHosts lists ATS platforms whose application flow always requires
// creating an account or signing in before any form field is reachable, so
// attempting the Learner Module / fill / Vision chain against them is a
// guaranteed waste (confirmed live on two Workday tenants, bugs.md #18).
// Matched as host suffixes.
var authGatedATSHosts = []string{
	"myworkdayjobs.com",
}

// isKnownAuthGatedHost reports whether the apply URL's host belongs to an ATS
// platform known to gate its entire application flow behind an account.
func isKnownAuthGatedHost(applyURL string) bool {
	u, err := url.Parse(applyURL)
	if err != nil {
		return false
	}
	host := strings.ToLower(u.Hostname())
	for _, gated := range authGatedATSHosts {
		if host == gated || strings.HasSuffix(host, "."+gated) {
			return true
		}
	}
	return false
}

var authWallPhrases = []string{
	"sign in to apply",
	"log in to apply",
	"create an account",
	"create account",
	"already have an account",
	"sign in with your account",
	"returning candidate",
}

// looksLikeAuthWallContent reports whether page content matches a known
// sign-in/account-creation gate phrasing. Only meaningful combined with a
// structural signal (a password input present) — plenty of legitimate
// application pages mention accounts in passing.
func looksLikeAuthWallContent(content string) bool {
	lowerContent := strings.ToLower(content)
	for _, phrase := range authWallPhrases {
		if strings.Contains(lowerContent, phrase) {
			return true
		}
	}
	return false
}

// isDeadJobPage reports whether page content matches a known "this posting is
// gone" phrasing, so AttemptSubmit can bail out before spending a costly
// generation cycle and a fill attempt on a job that can never succeed.
func isDeadJobPage(content string) bool {
	lowerContent := strings.ToLower(content)
	for _, phrase := range deadJobPhrases {
		if strings.Contains(lowerContent, phrase) {
			return true
		}
	}
	return false
}

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
	
	// Check for obvious dead ends. Expired postings frequently redirect
	// instead of rendering a "job closed" message (bugs.md #15), so check
	// where navigation actually landed before checking page phrasing —
	// both run before the costly document generation below.
	if reason := deadRedirectReason(applyURL, page.URL()); reason != "" {
		return fmt.Errorf("job posting is dead or expired: %s", reason)
	}
	content, _ := page.Content()
	if isDeadJobPage(content) {
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

	// Bug #18: known account-gated ATS platforms (Workday) have no fillable
	// form anywhere pre-auth — every mapping/fill/Vision attempt is a
	// guaranteed 4-10 minute waste. The tailored documents above are still
	// generated on purpose: they are the payload for the manual application
	// this job gets routed to.
	if isKnownAuthGatedHost(applyURL) {
		log.Printf("[Auto-Submit] %s is a known account-gated ATS — no pre-auth application form exists. Routing to manual submissions with tailored docs ready.", domain)
		return fmt.Errorf("%w: known account-gated ATS %s", ErrAuthWall, domain)
	}

	mappingJSON, err := storage.GetFormMapping(domain)
	if err == nil && mappingJSON != "" {
		log.Printf("[Auto-Submit] Using learned dynamic mapping for %s", domain)
		cachedTarget := resolveFillTarget(page)
		dynErr := handleDynamic(cachedTarget, resumePath, pii, mappingJSON, autoSubmitClick)
		if dynErr != nil {
			log.Printf("[Auto-Submit] Dynamic Playwright mapping failed for %s. Invalidating cache. Error: %v", domain, dynErr)
			storage.DeleteFormMapping(domain)
			if mapper != nil {
				log.Printf("[Auto-Submit] Falling back to Vision module after cached-mapping fill failure...")
				return AttemptVisionSubmit(page, cachedTarget, companyName, applyURL, resumePath, pii, mapper, autoSubmitClick)
			}
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

				// Bug #18, generic tier: after the Apply click, a sign-in
				// wall (password input plus account-gate phrasing) means the
				// real form is behind auth — skip the Learner/fill/Vision
				// chain and route to manual submission instead.
				if pwCount, pwErr := page.Locator("input[type='password']").Count(); pwErr == nil && pwCount > 0 {
					if pageContent, cErr := page.Content(); cErr == nil && looksLikeAuthWallContent(pageContent) {
						log.Printf("[Auto-Submit] Sign-in wall detected at %s (password field + account-gate phrasing). Routing to manual submissions.", domain)
						return fmt.Errorf("%w: sign-in wall detected at %s", ErrAuthWall, domain)
					}
				}

				target := resolveFillTarget(page)
				domHTML, _ := target.HTML()
				prunedHTML, err := parser.PruneDOMToText(domHTML)
				if err != nil {
					prunedHTML = domHTML
				}

				if filter != nil {
					if safe, threats, err := filter.CheckPayloadDetailed(prunedHTML); !safe {
						if logErr := storage.LogPromptInjectionDetections(applyURL, companyName, toStoredThreats(threats)); logErr != nil {
							log.Printf("[Auto-Submit] Failed to log prompt injection detection: %v", logErr)
						}
						return fmt.Errorf("malicious prompt injection detected on career page: %w", err)
					}
				}

				newMappingJSON, err := mapper.ExtractFormMapping(prunedHTML)
				if err == nil && newMappingJSON != "" {
					log.Printf("[Learner Module] Successfully mapped %s. Saving and re-attempting...", domain)
					storage.SaveFormMapping(domain, newMappingJSON)
					execErr = handleDynamic(target, resumePath, pii, newMappingJSON, autoSubmitClick)
					if execErr != nil {
						log.Printf("[Auto-Submit] Dynamic fill failed for %s after Learner Module mapping. Invalidating cache. Falling back to Vision module. Error: %v", domain, execErr)
						storage.DeleteFormMapping(domain)
						execErr = AttemptVisionSubmit(page, target, companyName, applyURL, resumePath, pii, mapper, autoSubmitClick)
					}
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
	// Labels holds the field's visible accessible label text (e.g. "First
	// Name"), keyed the same as Fields. Optional: populated when the mapper
	// can identify it, used as a fallback when the CSS selector guess in
	// Fields turns out to be wrong - WCAG-compliant ATS forms reliably
	// expose a stable label even when raw name/id attributes don't.
	Labels map[string]string `json:"labels"`
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

// safeFillWithLabelFallback tries the field's accessible label first when one
// is available, falling back to the LLM-guessed CSS selector otherwise (or
// if the label attempt itself fails). Label-based locators are the
// established best practice for resilient form automation against unknown
// markup - Playwright's own "user-first locator" guidance recommends
// GetByLabel ahead of raw CSS selectors precisely because it's tied to what
// a human actually sees, not implementation details that vary by ATS vendor
// theme, whereas a CSS selector can also silently match the wrong element
// of the same tag/name without erroring at all. CSS selector remains the
// fallback since not every mapping call successfully identifies a label.
// safeFillWithLabelFallback tries, in order: the field's accessible label
// (most robust - tied to what a human sees, not implementation details),
// then its placeholder text (covers minimalist ATS widgets that style a
// placeholder to look like a label with no real <label>/aria-label
// association at all, confirmed live 2026-07-21 as bug #16 on Jobvite and
// ApplyToJob), then finally the LLM-guessed CSS selector. Each tier is only
// attempted if the previous one failed or wasn't available.
func safeFillWithLabelFallback(target fillTarget, selector, labelText, text string) error {
	if text == "" {
		return nil
	}

	var lastErr error
	if labelText != "" {
		if err := target.GetByLabelLoc(labelText).Fill(text, playwright.LocatorFillOptions{Timeout: playwright.Float(15000)}); err == nil {
			return nil
		} else {
			lastErr = fmt.Errorf("label fill for %q failed: %w", labelText, err)
		}

		if err := target.GetByPlaceholderLoc(labelText).Fill(text, playwright.LocatorFillOptions{Timeout: playwright.Float(15000)}); err == nil {
			log.Printf("[Auto-Submit] Label fill for %q failed; placeholder fallback succeeded", labelText)
			return nil
		} else {
			lastErr = fmt.Errorf("%v; placeholder fill for %q also failed: %w", lastErr, labelText, err)
		}
	}

	if selector == "" {
		if lastErr != nil {
			return fmt.Errorf("%v; no CSS selector available either", lastErr)
		}
		return ErrEmptySelector
	}
	if err := safeFill(target, selector, text); err != nil {
		if lastErr != nil {
			return fmt.Errorf("%v; CSS selector fill also failed: %w", lastErr, err)
		}
		return err
	}
	if lastErr != nil {
		log.Printf("[Auto-Submit] Label and placeholder fill both failed; CSS selector fallback succeeded")
	}
	return nil
}

func handleDynamic(target fillTarget, resumePath string, pii *config.PII, mappingJSON string, autoSubmitClick bool) error {
	log.Printf("[Auto-Submit] Executing dynamic Playwright mapping...")
	var mapping FormMapping
	if err := json.Unmarshal([]byte(mappingJSON), &mapping); err != nil {
		return fmt.Errorf("failed to parse mapping json: %w", err)
	}

	if pii != nil {
		if pii.FirstName != "" {
			if err := safeFillWithLabelFallback(target, mapping.Fields["first_name"], mapping.Labels["first_name"], pii.FirstName); err != nil {
				return fmt.Errorf("failed to fill first_name: %w", err)
			}
		}
		if pii.LastName != "" {
			if err := safeFillWithLabelFallback(target, mapping.Fields["last_name"], mapping.Labels["last_name"], pii.LastName); err != nil {
				return fmt.Errorf("failed to fill last_name: %w", err)
			}
		}
		if err := safeFillWithLabelFallback(target, mapping.Fields["email"], mapping.Labels["email"], pii.Email); err != nil {
			return fmt.Errorf("failed to fill email: %w", err)
		}
		if err := safeFillWithLabelFallback(target, mapping.Fields["phone"], mapping.Labels["phone"], pii.Phone); err != nil {
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


