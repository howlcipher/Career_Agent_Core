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

// fillActionTimeoutMs bounds each individual fill/click/upload Playwright
// action. Was a flat 15000 (bugs.md): confirmed live 2026-07-22 that a single
// clean agent instance, no duplicate processes, still hit this on 3
// consecutive fill tiers (label/placeholder/CSS selector) in the same ~45s
// window immediately after the local Ollama model finished a heavy
// generation burst (200%+ CPU) — the same "hardcoded timeout too tight for
// genuinely slow-but-honest work under real load" shape as bug #6's Ollama
// client timeout fix, not the iframe/selector-strategy problems bugs #4/#14
// already addressed. Doubled for headroom against that CPU contention.
const fillActionTimeoutMs = 30000

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
	// .First() avoids a Playwright strict-mode violation when more than one
	// element matches the same label text (confirmed live 2026-07-22,
	// Workable/Dispel: getByLabel('Phone') resolved to 2 elements — a
	// visible field plus a hidden/duplicate one sharing the label). Filling
	// isn't order-sensitive here: any element with the right label is an
	// acceptable fill target, so narrowing beats failing outright.
	return t.page.GetByLabel(text).First()
}
func (t pageTarget) GetByPlaceholderLoc(text string) playwright.Locator {
	return t.page.GetByPlaceholder(text).First()
}
func (t pageTarget) WaitForSel(selector string, timeoutMs float64) (playwright.ElementHandle, error) {
	return t.page.WaitForSelector(selector, playwright.PageWaitForSelectorOptions{Timeout: playwright.Float(timeoutMs)})
}
func (t pageTarget) HTML() (string, error) { return t.page.Content() }

type frameTarget struct{ frame playwright.Frame }

func (t frameTarget) Loc(selector string) playwright.Locator { return t.frame.Locator(selector) }
func (t frameTarget) GetByLabelLoc(text string) playwright.Locator {
	return t.frame.GetByLabel(text).First()
}
func (t frameTarget) GetByPlaceholderLoc(text string) playwright.Locator {
	return t.frame.GetByPlaceholder(text).First()
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
	// SmartRecruiters renders a "Sorry, this job has expired" banner on the
	// otherwise-normal posting page (confirmed live 2026-07-22, Arista).
	"job has expired",
	// Workable's own expiry wording (confirmed live 2026-07-22, GoMining
	// SecOps Engineer posting) — didn't match any existing phrase, so the
	// pipeline burned a full generation + Learner Module cycle trying to
	// fill a form on a page with no form at all.
	"this job is not available anymore",
	// ApplyToJob/JazzHR's expiry wording (root cause of bugs.md #39,
	// confirmed live 2026-07-23 via a standalone script against a fresh
	// brightvisiontechnologies.applytojob.com posting): a plain "position"
	// wording that "job is no longer available" doesn't match. Previously
	// this dead page sailed all the way to the Vision fallback, which then
	// had no real form to map and either hallucinated a fully plausible but
	// fake selector set or returned empty fields/labels (ErrEmptySelector) -
	// same root-cause class as the Jobvite/Workable/SmartRecruiters gaps
	// above, just a different exact phrase.
	"position is no longer available",
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

// ErrCaptchaBlocked marks a page serving a bot-protection challenge instead
// of the posting (bug #23: DataDome on SmartRecruiters). The worker records
// BLOCKED_CAPTCHA; solving is improvements.md #17's (paid, user-gated) scope.
var ErrCaptchaBlocked = errors.New("page is behind a bot-protection challenge")

// captchaFrameHosts identify challenge iframes by URL substring.
var captchaFrameHosts = []string{
	"captcha-delivery.com", // DataDome
	"hcaptcha.com",
	"recaptcha",
	"challenges.cloudflare.com", // Turnstile
	"arkoselabs.com",
}

// captchaContentPhrases identify challenge pages by their visible copy.
var captchaContentPhrases = []string{
	"access is temporarily restricted",
	"verify you are human",
	"unusual activity from your device",
	"attention required",
	"cf-turnstile",
}

// isCaptchaContent reports whether page content reads like a bot-protection
// interstitial rather than a job posting.
func isCaptchaContent(content string) bool {
	lowerContent := strings.ToLower(content)
	for _, phrase := range captchaContentPhrases {
		if strings.Contains(lowerContent, phrase) {
			return true
		}
	}
	return false
}

// captchaWidgetFieldThreshold: a genuine bot-protection interstitial (e.g.
// DataDome) replaces the real page content, leaving little or nothing real
// behind — the original repro (bug #23, AbbVie/SmartRecruiters) was a page
// with zero real form fields. A standard invisible reCAPTCHA/hCaptcha
// anti-spam checkbox, by contrast, is a normal part of many legitimate ATS
// application forms and sits alongside a real, large, fillable form.
// Confirmed live 2026-07-23: real Greenhouse/Lever forms with 21-40 fields
// all still embed one of these widget iframes, and were being misdetected
// as blocked before this fix, most likely the dominant reason live batches
// rarely reached a fresh APPLIED. A form with more than this many real
// fields on the main page is treated as evidence the page is not blocked,
// regardless of which frames are present.
const captchaWidgetFieldThreshold = 5

// isCaptchaBlocked combines the content check with a frame scan: DataDome's
// interstitial has almost no text of its own — the giveaway is the challenge
// iframe's source host (confirmed live 2026-07-22 on AbbVie/SmartRecruiters:
// a 12-element page whose only real frame was geo.captcha-delivery.com). The
// frame-host signal alone is not trusted when the main page already has a
// real form (see captchaWidgetFieldThreshold) — only the explicit
// block-wording check still applies in that case.
func isCaptchaBlocked(page playwright.Page, content string) bool {
	if isCaptchaContent(content) {
		return true
	}
	if mainFieldCount, err := page.Locator("input, textarea, select").Count(); err == nil && mainFieldCount > captchaWidgetFieldThreshold {
		return false
	}
	for _, f := range page.Frames() {
		frameURL := strings.ToLower(f.URL())
		for _, host := range captchaFrameHosts {
			if strings.Contains(frameURL, host) {
				return true
			}
		}
	}
	return false
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

// dismissCookieBanner clicks past a cookie-consent banner if one is present.
// Confirmed live 2026-07-22 (Workable/European Dynamics posting): the
// banner's own backdrop div intercepts pointer events across the click
// target area, so the real "Apply now" button reports visible/enabled/stable
// yet every click retries and times out — a genuine interaction blocker, not
// a timing issue, and no amount of increasing fillActionTimeoutMs helps.
// Consent-detection in isCaptchaBlocked-style content checks doesn't catch
// this: the banner's markup uses obfuscated CSS-module classes
// (styles__backdrop--1TOnJ), not literal "cookie"/"consent" substrings.
// Prefers a decline/reject option when offered, falling back to accept —
// this is a one-shot headless session with no persistent identity to
// protect, so the choice only matters for unblocking the click; declining
// is preferred only because it's not always offered, and this project is
// otherwise privacy-conscious (pii.yaml, scripts/sanitize_jobs.go).
func dismissCookieBanner(page playwright.Page) {
	declineLocator := page.Locator("button:has-text('Decline'), button:has-text('Reject'), a:has-text('Decline'), a:has-text('Reject')").First()
	if count, err := declineLocator.Count(); err == nil && count > 0 {
		if err := declineLocator.Click(playwright.LocatorClickOptions{Timeout: playwright.Float(5000)}); err == nil {
			log.Printf("[Auto-Submit] Dismissed a cookie-consent banner (declined)")
			page.WaitForTimeout(500)
			return
		}
	}
	acceptLocator := page.Locator("button:has-text('Accept all'), button:has-text('Accept All'), button:has-text('Accept'), a:has-text('Accept all'), a:has-text('Accept')").First()
	if count, err := acceptLocator.Count(); err == nil && count > 0 {
		if err := acceptLocator.Click(playwright.LocatorClickOptions{Timeout: playwright.Float(5000)}); err == nil {
			log.Printf("[Auto-Submit] Dismissed a cookie-consent banner (accepted)")
			page.WaitForTimeout(500)
		}
	}
}

// resolveConsentGateIfPresent handles Jobvite's "Data Consent" step (confirmed
// live 2026-07-22, CMG Financial posting): after the Apply click, the page
// shows only a single <select> ("Location of Residence and Language") with
// zero real form fields anywhere on the page or in any frame — the
// application form doesn't exist in the DOM at all until an option is
// chosen, which is why the Learner Module and every fill tier found nothing
// to map or fill no matter how long the timeout. Confirmed live that
// selecting an option alone (no extra click needed) immediately reveals the
// real form (24 fields). Prefers an option matching the candidate's actual
// state/country from pii so the (often California-privacy-specific)
// disclosure the applicant is shown stays honest, falling back to the first
// non-placeholder option if no match is found. No-ops if the page already
// has real fields, so single-page ATS forms are never affected.
func resolveConsentGateIfPresent(page playwright.Page, pii *config.PII) {
	inputCount, err := page.Locator("input").Count()
	if err != nil || inputCount > 0 {
		return
	}
	selectLocator := page.Locator("select").First()
	count, err := selectLocator.Count()
	if err != nil || count == 0 {
		return
	}

	preferred := ""
	if pii != nil && pii.Address != "" && !strings.Contains(strings.ToLower(pii.Address), ", ca ") && !strings.Contains(strings.ToLower(pii.Address), "california") {
		preferred = "non-california"
	}

	options, err := selectLocator.Locator("option").All()
	if err != nil || len(options) == 0 {
		return
	}
	selectedIndex := -1
	if preferred != "" {
		for i, opt := range options {
			text, _ := opt.TextContent()
			if strings.Contains(strings.ToLower(text), preferred) {
				selectedIndex = i
				break
			}
		}
	}
	if selectedIndex == -1 {
		// Skip index 0 - it's almost always a "Select..." placeholder.
		if len(options) < 2 {
			return
		}
		selectedIndex = 1
	}

	if _, err := selectLocator.SelectOption(playwright.SelectOptionValues{Indexes: &[]int{selectedIndex}}); err != nil {
		log.Printf("[Auto-Submit] Found a likely consent-gate <select> but failed to choose an option: %v", err)
		return
	}
	log.Printf("[Auto-Submit] Selected an option on a location/consent gate to reveal the application form")
	page.WaitForTimeout(2000)
}

// clickApplyIfPresent looks for a visible "Apply"-labeled clickable element on the
// top-level page and clicks it, giving click-to-reveal application forms (a
// fancybox/lightbox modal, an in-page form injected by JS, etc.) a chance to render
// before the Learner Module inspects the DOM or the fill logic looks for form
// fields. No-ops silently if no such element is found, since most ATS platforms
// already show the form directly without requiring a click.
func clickApplyIfPresent(page playwright.Page) {
	// SmartRecruiters uses "I'm interested" instead of any "Apply" wording
	// (confirmed live 2026-07-22, Oteemo posting) — the original selector
	// silently found nothing on that platform, so the fill logic always
	// targeted the public job-description page, which has no form at all.
	locator := page.Locator("button:has-text('Apply'), a:has-text('Apply'), button:has-text(\"I'm interested\"), a:has-text(\"I'm interested\")").First()
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
func AttemptSubmit(browser playwright.Browser, filter *security.QuarantineLayer, mapper FormMapper, judge LLMJudge, companyName, applyURL string, generateDocs func() (string, string, error), pii *config.PII, profileContext string, headlessBrowser, autoSubmitClick bool) error {
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
	dismissCookieBanner(page)

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
	// Bug #23: a bot-protection interstitial means nothing downstream can
	// work — bail before the costly doc generation and mapping attempts.
	if isCaptchaBlocked(page, content) {
		return fmt.Errorf("%w at %s", ErrCaptchaBlocked, ExtractDomain(applyURL))
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
				// Bug #47: the dedicated handlers were never wired to
				// bug #8's click-to-reveal step, unlike the Learner Module
				// path — invisible until bug #45's CAPTCHA-detection fix
				// stopped killing these jobs before they ever reached here.
				// Confirmed live 2026-07-23: a real Lever posting has zero
				// form fields until "Apply for this job" is clicked.
				// clickApplyIfPresent no-ops when no such element exists, so
				// postings whose form is already on the page are unaffected.
				clickApplyIfPresent(page)
				if postClickContent, cErr := page.Content(); cErr == nil && isCaptchaBlocked(page, postClickContent) {
					return fmt.Errorf("%w at %s", ErrCaptchaBlocked, ExtractDomain(applyURL))
				}
				execErr = handleGreenhouse(resolveFillTarget(page), resumePath, pii, autoSubmitClick)
			} else if strings.Contains(urlLower, "lever.co") || strings.Contains(urlLower, "jobs.lever.co") {
				// Bug #47, same reasoning as the Greenhouse branch above.
				clickApplyIfPresent(page)
				if postClickContent, cErr := page.Content(); cErr == nil && isCaptchaBlocked(page, postClickContent) {
					return fmt.Errorf("%w at %s", ErrCaptchaBlocked, ExtractDomain(applyURL))
				}
				execErr = handleLever(resolveFillTarget(page), resumePath, pii, autoSubmitClick)
			} else if mapper != nil {
				log.Printf("[Auto-Submit] Unknown ATS %s. Triggering Learner Module...", domain)
				clickApplyIfPresent(page)

				// Bug #23 follow-up: the reveal click above can navigate to a
				// new URL that's gated by a bot-protection challenge the
				// earlier isCaptchaBlocked check (run before this click,
				// against the public job-description page) never saw —
				// confirmed live 2026-07-22: SmartRecruiters' "I'm interested"
				// button navigates to a oneclick-ui apply URL behind
				// geo.captcha-delivery.com. Bail now instead of burning a
				// Learner Module + Vision cycle on an unfillable page.
				if postClickContent, cErr := page.Content(); cErr == nil && isCaptchaBlocked(page, postClickContent) {
					return fmt.Errorf("%w at %s", ErrCaptchaBlocked, ExtractDomain(applyURL))
				}

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

				resolveConsentGateIfPresent(page, pii)

				target := resolveFillTarget(page)
				domHTML, _ := target.HTML()
				prunedHTML, err := parser.PruneDOMToText(domHTML)
				if err != nil {
					prunedHTML = domHTML
				}

				if filter != nil {
					if safe, threats, err := filter.CheckPayloadDetailed(prunedHTML); !safe {
						if judge != nil {
							if isSafe, verifyErr := judge.VerifySafeJobDescription(prunedHTML); verifyErr == nil && isSafe {
								log.Printf("[Auto-Submit] Security filter flagged job description, but LLM verified it as SAFE.")
							} else {
								if logErr := storage.LogPromptInjectionDetections(applyURL, companyName, toStoredThreats(threats)); logErr != nil {
									log.Printf("[Auto-Submit] Failed to log prompt injection detection: %v", logErr)
								}
								return fmt.Errorf("malicious prompt injection detected on career page: %w", err)
							}
						} else {
							if logErr := storage.LogPromptInjectionDetections(applyURL, companyName, toStoredThreats(threats)); logErr != nil {
								log.Printf("[Auto-Submit] Failed to log prompt injection detection: %v", logErr)
							}
							return fmt.Errorf("malicious prompt injection detected on career page: %w", err)
						}
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
				execErr = submitLocator.First().Click(playwright.LocatorClickOptions{Timeout: playwright.Float(fillActionTimeoutMs)})
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
		// Bug #49: input#submit_app only exists on Greenhouse's legacy embed
		// theme. Confirmed live 2026-07-23 (job-boards.greenhouse.io/
		// alphasense): a modern-board posting has zero elements matching
		// that ID at all — the real control is a plain, unidentified
		// <button type="submit">Submit application</button>. Try the legacy
		// selector first so postings that do use it are unaffected, then
		// fall back to the type-submit button.
		submitLoc := target.Loc("input#submit_app")
		if count, _ := submitLoc.Count(); count == 0 {
			submitLoc = target.Loc("button[type='submit']")
		}
		if err := submitLoc.Click(); err != nil {
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
	return target.Loc(selector).Fill(text, playwright.LocatorFillOptions{Timeout: playwright.Float(fillActionTimeoutMs)})
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
		if err := target.GetByLabelLoc(labelText).Fill(text, playwright.LocatorFillOptions{Timeout: playwright.Float(fillActionTimeoutMs)}); err == nil {
			return nil
		} else {
			lastErr = fmt.Errorf("label fill for %q failed: %w", labelText, err)
		}

		if err := target.GetByPlaceholderLoc(labelText).Fill(text, playwright.LocatorFillOptions{Timeout: playwright.Float(fillActionTimeoutMs)}); err == nil {
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
				}}, playwright.LocatorSetInputFilesOptions{Timeout: playwright.Float(fillActionTimeoutMs)})
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
			err := target.Loc(sel).Click(playwright.LocatorClickOptions{Timeout: playwright.Float(fillActionTimeoutMs)})
			if err != nil {
				return fmt.Errorf("failed to click submit: %w", err)
			}
		}
	}

	return nil
}


