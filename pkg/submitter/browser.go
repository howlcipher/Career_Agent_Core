package submitter

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

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

func quarantineCareerPageDOM(
	filter *security.QuarantineLayer,
	applyURL string,
	companyName string,
	domHTML string,
) error {
	if err := filter.QuarantinePayload(domHTML); err != nil {
		var detection *security.PromptInjectionError
		if !errors.As(err, &detection) {
			return fmt.Errorf("quarantine career page DOM: %w", err)
		}
		auditErr := storage.LogPromptInjectionDetections(
			applyURL,
			companyName,
			storage.ThreatsToStored(detection.Threats),
		)
		if auditErr != nil {
			log.Printf(
				"[Auto-Submit] Failed to log prompt injection detection: %v",
				auditErr,
			)
			return errors.Join(
				err,
				fmt.Errorf("log prompt-injection detection: %w", auditErr),
			)
		}
		return err
	}
	return nil
}

func quarantineFillTargetDOM(
	filter *security.QuarantineLayer,
	applyURL string,
	companyName string,
	target fillTarget,
) error {
	if target == nil {
		return errors.New("cannot quarantine a nil form target")
	}
	domHTML, err := target.HTML()
	if err != nil {
		return fmt.Errorf("read form DOM for quarantine: %w", err)
	}
	return quarantineCareerPageDOM(
		filter,
		applyURL,
		companyName,
		domHTML,
	)
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
	// Eval runs an expression in the target's own document. It exists for the
	// form-control inventory in questions.go, which has to read structure
	// (labels, control shapes, option lists, and whether a control is empty)
	// that no combination of Locator calls can gather in one pass without
	// dozens of round trips per form.
	Eval(expression string, arg ...any) (any, error)
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
func (t pageTarget) Eval(expression string, arg ...any) (any, error) {
	return t.page.Evaluate(expression, arg...)
}

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
func (t frameTarget) Eval(expression string, arg ...any) (any, error) {
	return t.frame.Evaluate(expression, arg...)
}

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

// DeadRedirectReason reports why the post-navigation URL indicates the
// posting is gone (bugs.md #15): ATS expired-posting redirects append an
// error query parameter (Greenhouse `?error=true`, Jobvite `?error=404`),
// and companies that migrate their board off the ATS redirect to their own
// domain (boards.eu.greenhouse.io -> careers.nebius.com), landing on a
// generic careers page that will never contain this posting's form. Returns
// "" when the final URL still plausibly hosts the posting — same-domain
// redirects (boards.greenhouse.io -> job-boards.greenhouse.io) are allowed
// through, since board migrations within the ATS can preserve the posting.
func DeadRedirectReason(applyURL, finalURL string) string {
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

// submissionConfirmationPhrases are common ATS post-submit confirmation
// wordings, checked directly against page content rather than relying
// solely on a URL change. Some platforms show a success message via AJAX
// without ever changing the URL (a bare "URL changed" check would wrongly
// treat that as a failure and retry a submission that already succeeded).
var submissionConfirmationPhrases = []string{
	"thank you for applying",
	"application received",
	"application has been submitted",
	"successfully submitted",
	"we've received your application",
	"we have received your application",
	"your application has been sent",
	"application submitted",
}

// submissionErrorPhrases are common validation-error wordings. Bug #51:
// "the URL changed after clicking submit" alone is not proof of success --
// a validation-error page or a redirect back to the listing also changes
// the URL, and would previously have been misclassified as APPLIED.
var submissionErrorPhrases = []string{
	"required field",
	"please fill",
	"please complete",
	"please correct",
	"this field is required",
	"cannot be blank",
	"is required",
}

// submissionConfirmationReason names which tier of isSubmissionConfirmed's
// evidence ladder decided the outcome, so callers can log how strong the
// evidence for a given APPLIED status actually is instead of a bare bool
// (bug #52 follow-up: the user asked whether APPLIED could be a false
// positive, and there was no way to answer that from the log alone).
type submissionConfirmationReason string

const (
	reasonConfirmationPhrase submissionConfirmationReason = "confirmation phrase found on page"
	reasonConfirmationURL    submissionConfirmationReason = "URL looks like a confirmation page"
	reasonURLUnchanged       submissionConfirmationReason = "URL did not change after submit"
	reasonErrorPhrase        submissionConfirmationReason = "validation-error wording found on page"
	reasonURLChangedNoError  submissionConfirmationReason = "URL changed, no confirmation or error wording found (weakest signal)"
	reasonFieldsFlagged      submissionConfirmationReason = "page re-rendered with fields flagged invalid"
	reasonNoOutcomeInBudget  submissionConfirmationReason = "no confirmation and no rejection evidence within the settle budget"
	reasonSecurityCodeGate   submissionConfirmationReason = "submission accepted; an emailed security code is required"
)

// bugs.md #95. A submit click used to be judged from a single page read taken
// immediately after WaitForLoadState(networkidle). That wait can return at
// once: Playwright's Click returns when the event is dispatched, not when the
// application reacts, so at the moment it is called the page frequently has no
// request in flight yet and already counts as idle. The verdict was then read
// off a DOM that had not changed because the submission had not happened yet.
//
// The costs are asymmetric, which is why this waits rather than guesses. A
// premature "failed" sends the whole form back through a ~12-minute model call
// and then re-clicks submit -- #89's duplicate-application risk, filed against
// a real employer. A few extra seconds costs nothing by comparison.
// Vars rather than consts so tests can shorten them. The no-evidence-either-way
// case legitimately waits the whole budget, which is correct behaviour but not
// something the suite should sit through.
var (
	// Minimum time after the click before flagged fields are allowed to count
	// as rejection. Below this, "nothing changed" means "too early to tell".
	//
	// bugs.md #102 raised this from 2s. Greenhouse accepts a submission and
	// issues an emailed security-code challenge within ~1s, but re-renders the
	// challenge later while the OLD aria-invalid markers are still on the page.
	// At 2s the verdict was reading those stale flags and calling an accepted
	// submission a validation failure -- measured on Akuity and ClickHouse,
	// where the security-code email is timestamped BETWEEN the submit click and
	// the verdict.
	submitOutcomeSettleFloor = 8 * time.Second
	// Upper bound on waiting for any outcome at all.
	submitOutcomeBudget = 15 * time.Second
	// Gap between polls inside the budget.
	submitOutcomePollInterval = 500 * time.Millisecond
)

// submissionOutcomeVerdict is one poll's decision inside awaitSubmissionOutcome.
// Split out from the Playwright loop so the timing rules are unit-testable --
// branching inside the driver loop is what let bugs.md #76 ship inert.
type submissionOutcomeVerdict struct {
	Done      bool
	Confirmed bool
	Reason    submissionConfirmationReason
}

// decideSubmissionOutcome applies one poll's evidence.
//
// Ordering matters and is deliberate:
//  1. Confirmation always wins, at any elapsed time -- a thank-you page that
//     renders in 200ms is just as real as one that takes 8s.
//  2. Past the settle floor, either flagged fields OR explicit validation-error
//     wording counts as rejection. Both are positive evidence that the server
//     answered and the form came back, which is why this may exit early
//     instead of burning the whole budget -- but before the floor they are
//     simply the pre-submit DOM, unchanged. Both forms are needed: a Greenhouse
//     theme that sets no aria-invalid still renders error text (the gap #83
//     hit), and a form flagged purely via aria-invalid may carry no wording.
//  3. Otherwise keep waiting until the budget runs out.
func decideSubmissionOutcome(urlBeforeClick, currentURL, pageContent string, elapsed time.Duration, invalidFieldCount int, securityCodeGate bool) submissionOutcomeVerdict {
	confirmed, reason := isSubmissionConfirmed(urlBeforeClick, currentURL, pageContent)
	if confirmed {
		return submissionOutcomeVerdict{Done: true, Confirmed: true, Reason: reason}
	}
	// bugs.md #102: a security-code challenge means the server ACCEPTED the
	// submission. It must be tested before the flagged-field branch, because
	// Greenhouse leaves the previous attempt's aria-invalid markers in place
	// while the challenge renders -- so both signals are true at once and the
	// stale one must not win.
	if securityCodeGate {
		return submissionOutcomeVerdict{Done: true, Confirmed: false, Reason: reasonSecurityCodeGate}
	}
	if elapsed >= submitOutcomeSettleFloor {
		if invalidFieldCount > 0 {
			return submissionOutcomeVerdict{Done: true, Confirmed: false, Reason: reasonFieldsFlagged}
		}
		if reason == reasonErrorPhrase {
			return submissionOutcomeVerdict{Done: true, Confirmed: false, Reason: reasonErrorPhrase}
		}
	}
	if elapsed >= submitOutcomeBudget {
		return submissionOutcomeVerdict{Done: true, Confirmed: false, Reason: reasonNoOutcomeInBudget}
	}
	return submissionOutcomeVerdict{Done: false}
}

// awaitSubmissionOutcome drives decideSubmissionOutcome against the live page
// after a submit click, replacing the previous single instantaneous read.
func awaitSubmissionOutcome(page playwright.Page, urlBeforeClick string) (bool, submissionConfirmationReason, string) {
	page.WaitForLoadState(playwright.PageWaitForLoadStateOptions{
		State:   playwright.LoadStateNetworkidle,
		Timeout: playwright.Float(10000),
	})

	start := time.Now()
	currentURL := page.URL()
	for {
		currentURL = page.URL()
		pageContent, _ := page.Content()
		invalid := parser.InvalidFieldIdentifiers(pageContent)
		elapsed := time.Since(start)
		v := decideSubmissionOutcome(urlBeforeClick, currentURL, pageContent, elapsed,
			len(invalid), parser.DetectSecurityCodeChallenge(pageContent))
		if v.Done {
			// bugs.md #96: one line per submit click recording what the verdict
			// was actually made on. #95 took a full day to find because the only
			// evidence a submit had been judged was the word "failed" -- nothing
			// said how long it waited, whether the page moved, or what came
			// back. Same reasoning as #80: the diagnostic is written the moment
			// its absence costs a cycle, not after the next one.
			log.Printf("[Auto-Submit] Submit verdict after %s: %s (url moved: %t, invalid fields: %d, page %d chars)",
				elapsed.Round(100*time.Millisecond), v.Reason, currentURL != urlBeforeClick, len(invalid), len(pageContent))
			return v.Confirmed, v.Reason, currentURL
		}
		page.WaitForTimeout(float64(submitOutcomePollInterval.Milliseconds()))
	}
}

// isSubmissionConfirmed decides whether a post-submit-click page state
// represents a genuine success, given the URL before/after the click and
// the resulting page's content. Preferred signal: explicit confirmation
// wording anywhere on the page (works even when the URL never changed).
// Fallback: the URL itself reads like a confirmation page. Last resort:
// the URL changed at all AND the page doesn't show validation-error
// wording -- weaker, but still requires the absence of an error signal
// rather than trusting navigation alone (bug #51).
func isSubmissionConfirmed(applyURL, currentURL, pageContent string) (bool, submissionConfirmationReason) {
	lowerContent := strings.ToLower(pageContent)
	for _, phrase := range submissionConfirmationPhrases {
		if strings.Contains(lowerContent, phrase) {
			return true, reasonConfirmationPhrase
		}
	}
	lowerURL := strings.ToLower(currentURL)
	if strings.Contains(lowerURL, "thank") || strings.Contains(lowerURL, "success") || strings.Contains(lowerURL, "confirmation") {
		return true, reasonConfirmationURL
	}
	if currentURL == applyURL {
		return false, reasonURLUnchanged
	}
	for _, phrase := range submissionErrorPhrases {
		if strings.Contains(lowerContent, phrase) {
			return false, reasonErrorPhrase
		}
	}
	return true, reasonURLChangedNoError
}

// confirmOrError waits for the page to settle after a submit click and
// applies isSubmissionConfirmed, returning a descriptive error when the
// evidence doesn't support a genuine success. urlBeforeClick must be the
// page's URL immediately before the submit click being verified, not the
// original job-posting URL — bug #47's click-to-reveal step means those
// two can differ by the time any handler even attempts a submit, which
// would otherwise make "the URL changed" trivially true regardless of
// whether the click itself did anything (bug #52 follow-up).
//
// Bug #52 follow-up, second gap: this used to only run for the Lever/
// Greenhouse/LinkedIn dispatch path in AttemptSubmit's loop. The cached-
// mapping fast path and both AttemptVisionSubmit call sites returned
// success straight from handleDynamic's bare error value, with no
// confirmation evidence at all — the same unverified-success pattern bug
// #51 fixed, just never extended past three of the many ATS paths. Every
// path that can result in APPLIED now goes through this same check.
func confirmOrError(page playwright.Page, companyName, urlBeforeClick string, copilotMode, autoSubmitClick bool) error {
	if err := submitGate(copilotMode, autoSubmitClick); err != nil {
		return err
	}
	// bugs.md #95: wait for evidence of an outcome instead of reading the DOM
	// the instant the click returns.
	confirmed, reason, currentURL := awaitSubmissionOutcome(page, urlBeforeClick)
	if confirmed {
		log.Printf("[Auto-Submit] Submission confirmed for %s (%s) at %s", companyName, reason, currentURL)
		return nil
	}
	return fmt.Errorf("submission not confirmed for %s: %s (at %s)", companyName, reason, currentURL)
}

// ErrAwaitingHumanReview marks a form filled in copilot mode, where the agent
// stops before the final submit click so a human can review and finish it.
var ErrAwaitingHumanReview = errors.New("copilot mode: form filled, awaiting human review before submit")

// ErrSubmitClickDisabled marks a form filled when auto_submit_click is false,
// where auto-clicking the final submit button is disabled.
//
// Before improvement #423 this case returned a bare nil, which pipeline.go
// read as success and recorded as APPLIED with a permanent dedup row, for a
// form that was never submitted (bug #432).
var ErrSubmitClickDisabled = errors.New("auto submit click disabled: form filled but not submitted")

// submitGate reports the sentinel error that must be returned instead of
// clicking a form's final submit control, or nil when clicking is allowed.
// Every submit path must consult this rather than testing autoSubmitClick
// directly, so a new gate can never be wired into one ATS handler and
// missed in another — the single most repeated structural defect in this
// project's history (#65/#66 → #67, #74 → #75, #28 → #31).
func submitGate(copilotMode, autoSubmitClick bool) error {
	if copilotMode {
		return ErrAwaitingHumanReview
	}
	if !autoSubmitClick {
		return ErrSubmitClickDisabled
	}
	return nil
}

// isSubmitGated reports whether err is one of submitGate's sentinels, meaning
// the form was filled successfully and the run stopped deliberately at the
// final click.
//
// Every recovery path must short-circuit on this before doing anything else.
// A gated return is *not* a fill failure, but it travels through the same
// error values that fill failures do, so without this check the recovery
// machinery reacts to an outcome that never happened: the cached-mapping and
// Learner Module paths delete the learned form mapping and fall back to an
// expensive Vision call, and the retry loop re-reads the page for bot
// protection and can rewrap the sentinel as ErrCaptchaBlocked — losing the
// sentinel's identity and filing the job as BLOCKED_CAPTCHA. Copilot mode
// would otherwise wipe every learned ATS blueprint it touched.
func isSubmitGated(err error) bool {
	return errors.Is(err, ErrAwaitingHumanReview) || errors.Is(err, ErrSubmitClickDisabled)
}

// ErrAuthWall marks an application flow gated behind account creation or
// sign-in, where no fillable application form exists pre-auth (bug #18:
// Workday). Callers should route these to the manual-submission backlog
// (the tailored documents are already generated and saved) instead of
// treating them as an automation failure.
var ErrAuthWall = errors.New("application form is gated behind account sign-in")

// ErrFormTooLargeForModel marks a form whose mapping/fill prompt would
// exceed the local model's context window. Confirmed live 2026-07-24
// (bugs.md #52's later recurrences, Reddit and Akuity): a form can pass
// the character-based circuit breaker (pkg/mcp's payloadSafetyLimits) and
// still get rejected by Ollama itself ("exceeds the available context
// size"), since HTML content runs roughly 3 characters per token, well
// short of the 1:1 assumption a pure character limit implies. Callers
// should route these to manual submission the same way ErrAuthWall does,
// rather than burning a doomed LLM call (and the ~20-40 min of doc-gen
// that already happened before it) only to fail with an ugly HTTP 400.
var ErrFormTooLargeForModel = errors.New("form content exceeds the local model's context window")

// ErrSubmitProducedNoOutcome marks a form that is fully satisfied -- nothing
// flagged invalid -- whose submit produced neither confirmation nor rejection.
//
// bugs.md #108. Ethos reached `invalid fields: 0`, exhausted the settle budget
// with no bot-protection frame present to explain it, and was then reported as
// "form content exceeds the local model's context window" -- because narrowing
// found nothing to narrow, fell back to the whole document, and the size check
// caught it incidentally. The outcome (manual review, documents preserved) was
// right; the reason named the wrong cause entirely, which is the same
// diagnostic failure #83 suffered before #93 reframed it.
var ErrSubmitProducedNoOutcome = errors.New("form is fully filled but the submit produced no confirmation and no rejection")

// ErrNeedsUnprovidedAttestation marks a form that cannot be completed without
// the applicant declaring something they have not configured.
//
// bugs.md #82: Greenhouse's "Are you authorized to work in the U.S.?" and
// "Do you require sponsorship?" are required and offer **only Yes and No** --
// verified against the live form. There is no decline option, so a model with
// no configured answer does not abstain; it picks one, and that answer is a
// legal declaration submitted under the applicant's name. Routing the job to
// manual review is the only honest outcome.
var ErrNeedsUnprovidedAttestation = errors.New("form requires a legal attestation the applicant has not provided")

// ErrUncommittableField marks a form with a required widget whose value cannot
// be committed at all -- not because the automation is broken, but because the
// widget will not accept the configured value.
//
// bugs.md #88: measured on Lever, whose geocoder returns **zero** results for
// "Macomb", "Macomb Township" and "Macomb, MI" while Greenhouse's resolves the
// same address happily. With no option to select, the required hidden
// selectedLocation can never be populated and the form can never validate.
// Substituting a nearby city the geocoder does know would misrepresent the
// applicant's location on a real application, so the honest outcome is to hand
// the job to a human with its documents intact rather than burn three attempts
// reaching the same wall.
var ErrUncommittableField = errors.New("a required field could not be committed with the configured value")

// ErrNeedsEmailVerification marks a form waiting on a one-time code that was
// delivered out of band.
//
// bugs.md #93: confirmed live. A Greenhouse submit to Surt AI produced an
// email at the exact second of the click -- "Copy and paste this code into the
// security code field on your application ... After you enter the code,
// resubmit your application." The agent has no access to the mailbox, so no
// number of retries can satisfy it. Worse, the code field reads as an ordinary
// unsatisfied required field, so the previous behaviour was to send the entire
// form to the model and burn the full 45-minute timeout on it.
var ErrNeedsEmailVerification = errors.New("form is waiting on a one-time code sent by email")

// SecurityCodeFetcher, when set, retrieves a one-time code an ATS emailed
// during an application. cmd/agent wires this to the IMAP credentials the
// email tracker already uses; left nil, the submitter behaves exactly as
// bugs.md #93 described and hands the job to manual review.
//
// improvements.md #32. Indirected rather than imported so pkg/submitter keeps
// no dependency on the mail layer, and so tests can supply a stub.
var SecurityCodeFetcher func(notBefore time.Time) (string, error)

// securityCodeSelectors locate the field the code is typed into.
var securityCodeSelectors = []string{
	"input[name='security_code']", "#security_code",
	"input[name='securityCode']", "#securityCode",
	"input[name='verification_code']", "#verification_code",
	"input[id*='security-code']", "input[id*='verification-code']",
}

// waitForSecurityCode polls for the emailed code. Delivery takes seconds to a
// minute, so this is deliberately patient but bounded -- and it only ever
// accepts a message that arrived *after* the submit that triggered it, so a
// stale code from an earlier application can never be reused.
func waitForSecurityCode(notBefore time.Time) (string, error) {
	const (
		budget = 90 * time.Second
		tick   = 10 * time.Second
	)
	deadline := time.Now().Add(budget)
	var lastErr error
	for {
		code, err := SecurityCodeFetcher(notBefore)
		if err != nil {
			lastErr = err
		} else if code != "" {
			return code, nil
		}
		if time.Now().After(deadline) {
			return "", lastErr
		}
		time.Sleep(tick)
	}
}

// securityCodeFieldBudget bounds how long fillSecurityCode waits for the code
// input to become visible.
//
// bugs.md #113: the gate is *detected* by substring-matching the HTML, but
// filling needs a real visible locator, and the input renders later than the
// markers do. Measured on Akuity: gate detected and code retrieved at 06:30:51,
// ~11s after the accepted submit, and the field was not yet fillable -- so a
// genuinely accepted application with its code in hand was sent to manual review
// one step from completion. Same DOM-lag that produced #95, #102 and #111, one
// layer further in.
var securityCodeFieldBudget = 20 * time.Second

// splitSecurityCodeSelectors match one-character-per-box one-time-code widgets.
//
// bugs.md #115: Greenhouse renders the code as EIGHT separate inputs --
// security-input-0 .. security-input-7, each maxlength=1, each with an empty
// name attribute. Discovered from #114's diagnostic dump; every previous
// selector looked for a single `security_code`/`security-code` field, so none
// could match, and filling one box with an 8-character code would have failed
// anyway.
var splitSecurityCodeSelectors = []string{
	"input[id^='security-input-']",
	"input[id^='verification-input-']",
	"input[data-testid^='security-input']",
}

// fillSplitSecurityCode distributes one character per box across a
// one-time-code widget, and reports whether it filled the whole code.
//
// Requires at least as many boxes as characters, and fills exactly len(code) of
// them. Fewer boxes than characters means this is not the widget for this code,
// and a partial fill would submit a truncated answer.
func fillSplitSecurityCode(target fillTarget, code string) bool {
	for _, sel := range splitSecurityCodeSelectors {
		loc := target.Loc(sel)
		count, err := loc.Count()
		if err != nil || count < len(code) {
			continue
		}
		filled := 0
		for i, ch := range code {
			box := loc.Nth(i)
			if vis, vErr := box.IsVisible(); vErr != nil || !vis {
				break
			}
			if fErr := box.Fill(string(ch), playwright.LocatorFillOptions{
				Timeout: playwright.Float(fillActionTimeoutMs),
			}); fErr != nil {
				break
			}
			filled++
		}
		if filled == len(code) {
			return true
		}
	}
	return false
}

func fillSecurityCode(target fillTarget, code string) error {
	deadline := time.Now().Add(securityCodeFieldBudget)
	for {
		// bugs.md #115: the split-box layout first -- it is what Greenhouse
		// actually uses, and a single-field selector can never match it.
		if fillSplitSecurityCode(target, code) {
			return nil
		}
		for _, sel := range securityCodeSelectors {
			loc := target.Loc(sel)
			count, err := loc.Count()
			if err != nil || count == 0 {
				continue
			}
			el, ok := firstVisibleSubmit(loc, count)
			if !ok {
				continue
			}
			if err := el.Fill(code, playwright.LocatorFillOptions{Timeout: playwright.Float(fillActionTimeoutMs)}); err == nil {
				return nil
			}
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("could not find a visible security-code field to fill within %s", securityCodeFieldBudget)
		}
		time.Sleep(500 * time.Millisecond)
	}
}

// manualReviewErrors are the outcomes that are not automation failures: the
// job is sound, something outside the agent's authority is simply required to
// finish it. Every one of them must be preserved for manual completion rather
// than written off as FAILED_SUBMIT.
//
// bugs.md #84: this list exists so a newly-added sentinel cannot silently fall
// through to the generic failure path. #82 added
// ErrNeedsUnprovidedAttestation and the cmd/agent branch meant to route it was
// never actually applied -- the build still passed, and ClickHouse was written
// off as FAILED_SUBMIT with its tailored documents stranded.
var manualReviewErrors = []error{
	ErrAuthWall,
	ErrFormTooLargeForModel,
	ErrNeedsUnprovidedAttestation,
	ErrUncommittableField,
	ErrNeedsEmailVerification,
	// bugs.md #108: fully filled, submit went nowhere, no bot protection to
	// explain it. Not an automation failure -- a human can finish it in
	// seconds, and the tailored documents must survive.
	ErrSubmitProducedNoOutcome,
}

// IsManualReviewError reports whether err means "queue this for a human"
// rather than "this attempt failed".
func IsManualReviewError(err error) bool {
	for _, sentinel := range manualReviewErrors {
		if errors.Is(err, sentinel) {
			return true
		}
	}
	return false
}

// maxPromptCharsForModelContext is a conservative character budget for any
// single prompt sent to the local model, derived from two real failures:
// Reddit's 54,917-char prompt needed 18,572 tokens (~2.96 chars/token) and
// Akuity's needed 16,604 tokens at a similar ratio. Assumes 2.5 chars/token
// (more conservative than either real sample) and reserves ~400 tokens for
// the system prompt and EEO profile context that don't appear in this count.
//
// Raised 2026-07-24 (found live during the 82-job re-verification: every
// single MANUAL_REQUIRED outcome in that run was this exact circuit breaker
// firing — 13/13, the dominant blocker to reaching a real APPLIED). Root
// cause was an unnecessarily conservative Ollama server config, not a real
// model limitation: the running model (qwen3:30b-instruct, qwen3moe
// architecture) uses GQA with only 4 KV heads, making its context memory
// cost unusually cheap (~96KB/token at f16). The server was pinned to
// OLLAMA_CONTEXT_LENGTH=6144 via a systemd override
// (~/.config/systemd/user/ollama.service.d/override.conf) despite the model
// architecturally supporting up to 262,144 tokens. Raised the server to
// 32768 alongside OLLAMA_KV_CACHE_TYPE=q8_0 (quantizes the KV cache,
// roughly halving its already-cheap per-token cost, near-lossless in
// practice) — confirmed live: system memory headroom actually *improved*
// after the restart (15GB available vs. 4.8GB before) despite the ~5x
// larger context, since the KV cache was never the dominant memory cost
// here (the 30.5B model's weights are, at a fixed ~17GB regardless of
// context length). This constant must stay in sync with that server config
// — (32768-400)*2.5 ≈ 80,000 characters, rounded down for margin.
//
// Checked before either ExtractFormMapping or SolveValidationErrors is
// called, on the exact content that will make up their prompt (DOM plus
// profile context).
const maxPromptCharsForModelContext = 80000

// maxPromptCharsForTimeBudget is the *time* ceiling, which on this hardware
// binds long before the context ceiling above does.
//
// bugs.md #83: measured live 2026-07-25. A Greenhouse theme that sets no
// aria-invalid attributes defeats #64's narrowing, so the retry falls back to
// the whole form — 50,501 characters. That fits the 80,000-char context
// window comfortably and passed the existing check, then ran from 16:58:03 to
// 17:43:03 and died on the 45-minute Ollama timeout. Exactly 45 minutes of
// the single serialised LLM resource, spent on a request that could never
// have finished.
//
// Derivation: prompt processing measured at ~7 tok/s on this host's 30B model
// (bugs.md #64, improvements #25) at ~2.5 chars/token is ~17.5 chars/s.
// Against defaultOllamaTimeoutMinutes (45) that is ~47,000 characters before
// the request is mathematically doomed — which matches the 50,501-char
// observation precisely. Set to 40,000 to leave headroom for token generation
// and for CPU contention with the browser.
const maxPromptCharsForTimeBudget = 28000

// maxInvalidFieldsForTimeBudget caps how many fields one retry may ask the
// model to answer.
//
// bugs.md #105. #83 derived its character ceiling from input size alone, and
// that model is wrong: the run has to *generate* a value for every rejected
// field, and output dominates once the field count grows. Measured on this
// hardware:
//
//	ClickHouse   11,140 chars,  3 fields -> ~7 min   ok
//	Reddit       18,639 chars, 13 fields -> ~15 min  ok
//	Remote       30,477 chars, 34 fields -> 45 min   TIMED OUT
//
// Remote passed the old 40,000-char ceiling comfortably and still burned the
// entire Ollama timeout. Field count, not payload size, is what separates it
// from the two that finished -- so the guard needs both, and the character
// ceiling drops below the observed failure as well.
const maxInvalidFieldsForTimeBudget = 20

func likelyExceedsModelContext(domContent, profileContext string) bool {
	total := len(domContent) + len(profileContext)
	return total > maxPromptCharsForModelContext || total > maxPromptCharsForTimeBudget
}

// exceedsRetryTimeBudget reports whether one validation retry is too expensive
// for the local model, by size OR by the number of answers it must generate.
//
// bugs.md #105: the second half is the part #83 missed. A 34-field form sat
// well inside the character ceiling and still consumed the full 45-minute
// timeout, because the cost is dominated by generating 34 values rather than
// by reading the form.
func exceedsRetryTimeBudget(domContent, profileContext string, invalidFieldCount int) bool {
	if likelyExceedsModelContext(domContent, profileContext) {
		return true
	}
	return invalidFieldCount > maxInvalidFieldsForTimeBudget
}

// authGatedATSHosts lists ATS platforms whose application flow always requires
// creating an account or signing in before any form field is reachable, so
// attempting the Learner Module / fill / Vision chain against them is a
// guaranteed waste (confirmed live on two Workday tenants, bugs.md #18).
// Matched as host suffixes.
var authGatedATSHosts = []string{
	"myworkdayjobs.com",
	// Confirmed live 2026-07-23 (bug #50): probed multiple real
	// jobs.workable.com postings and found a "log in"/"sign in" gate before
	// any form field on every one — 0 APPLIED across 12 attempts this
	// session, same structural shape as Workday, not a fill-selector
	// problem worth chasing.
	"workable.com",
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

// AssistedDestination names the verified human-action surface reached by an
// assisted browser. A public job description is deliberately not a
// destination: the browser must expose a form, an account gate, or a CAPTCHA
// before the dashboard reports that it opened successfully.
type AssistedDestination string

const (
	AssistedDestinationApplication AssistedDestination = "application"
	AssistedDestinationAccount     AssistedDestination = "account"
	AssistedDestinationCaptcha     AssistedDestination = "captcha"
)

const assistedApplicationEntrySelector = "button:has-text('Apply now'), a:has-text('Apply now'), button:has-text('Apply for this job'), a:has-text('Apply for this job'), button:has-text(\"I'm interested\"), a:has-text(\"I'm interested\"), button:has-text('Apply'), a:has-text('Apply')"

// ReachAssistedDestination advances a verified posting to the first actual
// human-action surface. It never fills a field or clicks a submit control.
// A successful Apply click is insufficient by itself: the resulting page
// must structurally prove that a form, account gate, or CAPTCHA is present.
func ReachAssistedDestination(page playwright.Page) (playwright.Page, AssistedDestination, error) {
	postingPage := page
	if destination := assistedDestination(page); destination != "" {
		return page, destination, nil
	}

	browserContext := page.Context()
	dismissCookieBanner(page)
	entries := page.Locator(assistedApplicationEntrySelector)
	count, err := entries.Count()
	if err != nil {
		return nil, "", fmt.Errorf("inspect application entry: %w", err)
	}
	if count == 0 {
		return nil, "", errors.New("verified posting has no reachable application entry")
	}
	entry := firstVisibleLocator(entries, count)
	href := assistedEntryHref(entry)
	if href == "" {
		href = workableAssistedEntryURL(page.URL())
	}
	if href != "" {
		target, err := resolveAssistedEntryURL(page.URL(), href)
		if err != nil {
			return nil, "", err
		}
		page, err = navigateAssistedEntry(page, browserContext, target)
		if err != nil {
			return nil, "", err
		}
		log.Printf("[Assisted] Navigated a guarded tab to the application entry")
	} else if err := entry.Click(playwright.LocatorClickOptions{Timeout: playwright.Float(10000)}); err != nil {
		return nil, "", fmt.Errorf("open application entry: %w", err)
	} else {
		log.Printf("[Assisted] Clicked an in-page application entry")
	}

	for range 20 {
		if page.IsClosed() {
			page = newestOpenAssistedPage(browserContext)
			if page == nil {
				return nil, "", errors.New("application entry closed its posting without opening a usable page")
			}
		} else {
			page.WaitForTimeout(500)
		}
		if destination := assistedDestination(page); destination != "" {
			if postingPage != page && !postingPage.IsClosed() {
				_ = postingPage.Close()
			}
			return page, destination, nil
		}
	}
	return nil, "", errors.New("application entry did not reveal a form, account gate, or CAPTCHA")
}

func navigateAssistedEntry(page playwright.Page, browserContext playwright.BrowserContext, target string) (playwright.Page, error) {
	destinationPage := page
	if browserContext != nil {
		created, err := browserContext.NewPage()
		if err != nil {
			return nil, fmt.Errorf("create application destination page: %w", err)
		}
		destinationPage = created
	}
	if _, err := destinationPage.Goto(target, playwright.PageGotoOptions{WaitUntil: playwright.WaitUntilStateDomcontentloaded}); err != nil {
		if destinationPage != page {
			_ = destinationPage.Close()
		}
		return nil, fmt.Errorf("navigate to application entry: %w", err)
	}
	return destinationPage, nil
}

// AssistedApplicationEntryURL returns a deterministic application route when
// the ATS publishes one in its posting URL. An empty result means the posting
// must be inspected interactively to discover its application entry.
func AssistedApplicationEntryURL(currentURL string) string {
	return workableAssistedEntryURL(currentURL)
}

func workableAssistedEntryURL(currentURL string) string {
	parsed, err := url.Parse(currentURL)
	if err != nil || !strings.EqualFold(parsed.Hostname(), "apply.workable.com") {
		return ""
	}
	segments := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	if len(segments) >= 4 && segments[len(segments)-1] == "apply" && segments[len(segments)-3] == "j" {
		parsed.Path = "/" + strings.Join(segments, "/") + "/"
		parsed.RawQuery = ""
		parsed.Fragment = ""
		return parsed.String()
	}
	if len(segments) < 3 || segments[len(segments)-2] != "j" || segments[len(segments)-1] == "" {
		return ""
	}
	parsed.Path = "/" + strings.Join(segments, "/") + "/apply/"
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String()
}

func assistedEntryHref(entry playwright.Locator) string {
	if href, err := entry.GetAttribute("href"); err == nil && strings.TrimSpace(href) != "" {
		return strings.TrimSpace(href)
	}
	value, err := entry.Evaluate(`element => {
		const anchor = element.closest('a[href]');
		return anchor ? anchor.href : '';
	}`, nil)
	if err != nil {
		return ""
	}
	href, ok := value.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(href)
}

func resolveAssistedEntryURL(currentURL, href string) (string, error) {
	base, err := url.Parse(currentURL)
	if err != nil {
		return "", errors.New("current assisted page URL is invalid")
	}
	reference, err := url.Parse(strings.TrimSpace(href))
	if err != nil {
		return "", errors.New("application entry URL is invalid")
	}
	target := base.ResolveReference(reference)
	if target.Scheme != "http" && target.Scheme != "https" {
		return "", errors.New("application entry must use HTTP or HTTPS")
	}
	if target.Hostname() == "" {
		return "", errors.New("application entry has no host")
	}
	return target.String(), nil
}

func newestOpenAssistedPage(browserContext playwright.BrowserContext) playwright.Page {
	if browserContext == nil {
		return nil
	}
	pages := browserContext.Pages()
	for i := len(pages) - 1; i >= 0; i-- {
		if pages[i] != nil && !pages[i].IsClosed() {
			return pages[i]
		}
	}
	created, err := browserContext.WaitForEvent("page", playwright.BrowserContextWaitForEventOptions{Timeout: playwright.Float(5000)})
	if err != nil {
		return nil
	}
	page, ok := created.(playwright.Page)
	if ok && page != nil && !page.IsClosed() {
		return page
	}
	return nil
}

func assistedDestination(page playwright.Page) AssistedDestination {
	content, err := page.Content()
	if err != nil {
		return ""
	}
	if isCaptchaBlocked(page, content) {
		return AssistedDestinationCaptcha
	}
	passwords := page.Locator("input[type='password']")
	if visibleAssistedLocatorCount(passwords, 2) > 0 && looksLikeAuthWallContent(content) {
		return AssistedDestinationAccount
	}
	strongFields := page.Locator("input[type='file'], input[type='email'], textarea")
	if visibleAssistedLocatorCount(strongFields, 4) > 0 {
		return AssistedDestinationApplication
	}
	fields := page.Locator("input:not([type='hidden']):not([type='search']), textarea, select")
	if visibleAssistedLocatorCount(fields, 3) >= 2 {
		return AssistedDestinationApplication
	}
	for _, frame := range page.Frames() {
		if frame == page.MainFrame() {
			continue
		}
		frameFields := frame.Locator("input:not([type='hidden']):not([type='search']), textarea, select")
		if visibleAssistedLocatorCount(frameFields, 3) >= 2 {
			return AssistedDestinationApplication
		}
	}
	return ""
}

func visibleAssistedLocatorCount(locator playwright.Locator, limit int) int {
	count, err := locator.Count()
	if err != nil || count == 0 {
		return 0
	}
	if count > limit {
		count = limit
	}
	visible := 0
	for i := 0; i < count; i++ {
		if ok, err := locator.Nth(i).IsVisible(); err == nil && ok {
			visible++
		}
	}
	return visible
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

// targetClosedPhrases match Playwright's wording when the tab/context an
// action was issued against no longer exists -- either a genuine renderer
// crash (bugs.md #467: the same live session that reported this also logged
// a Chromium headless crash) or a target closed mid-action. Every later
// Playwright call against the same page/frame fails the same way until a new
// page exists, so this must be checked before treating the failure as an
// ordinary fill/click error.
var targetClosedPhrases = []string{
	"target closed",
	"target page, context or browser has been closed",
	"browser has been closed",
}

// isTargetClosedErr reports whether err is Playwright's target-closed or
// browser-crashed error rather than an ordinary fill or validation failure.
func isTargetClosedErr(err error) bool {
	if err == nil {
		return false
	}
	lower := strings.ToLower(err.Error())
	for _, phrase := range targetClosedPhrases {
		if strings.Contains(lower, phrase) {
			return true
		}
	}
	return false
}

// maxTargetRecoveryAttempts bounds how many times AttemptSubmit will
// recreate a crashed browser context for a single job (bugs.md #467). A
// posting that crashes the browser deterministically must still fail and
// free the worker for the next job rather than retry forever.
const maxTargetRecoveryAttempts = 1

// newSubmitPage creates a fresh proxy-guarded browser context/page and
// navigates it to applyURL, replaying the setup AttemptSubmit needs both for
// its initial page and, bounded by maxTargetRecoveryAttempts, to recover
// from a crashed target (bugs.md #467) -- without re-running the document
// generation that happens later and is the expensive part of a job attempt.
func newSubmitPage(browser playwright.Browser, applyURL string) (*secureBrowserSession, playwright.Page, error) {
	session, err := newSecureBrowserSession(
		browser,
		playwright.BrowserNewContextOptions{
			UserAgent: playwright.String("Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/120.0.0.0"),
		},
	)
	if err != nil {
		return nil, nil, fmt.Errorf("could not create context: %w", err)
	}

	page, err := session.context.NewPage()
	if err != nil {
		session.Close()
		return nil, nil, fmt.Errorf("could not create page: %w", err)
	}

	if err := installSafeBrowserRoutes(page, session.guard); err != nil {
		page.Close()
		session.Close()
		return nil, nil, err
	}

	// Set a strict 45-second global timeout for all page operations (navigation, clicks, fills).
	// If a captcha blocks the page, Playwright will time out instead of hanging the worker forever.
	page.SetDefaultTimeout(45000)

	// Stealth: Overwrite properties to bypass naive bot protection
	page.AddInitScript(playwright.Script{
		Content: playwright.String(`
			Object.defineProperty(navigator, 'webdriver', {get: () => undefined});
			window.chrome = { runtime: {} };
			Object.defineProperty(navigator, 'plugins', { get: () => [1, 2, 3, 4, 5] });
			Object.defineProperty(navigator, 'languages', { get: () => ['en-US', 'en'] });
		`),
	})

	log.Printf("[Auto-Submit] Navigating to %s", applyURL)
	if _, err = page.Goto(applyURL, playwright.PageGotoOptions{
		WaitUntil: playwright.WaitUntilStateNetworkidle,
	}); err != nil {
		log.Printf("[Auto-Submit] NetworkIdle wait timed out or failed. Falling back to Domcontentloaded...")
		if _, err = page.Goto(applyURL, playwright.PageGotoOptions{
			WaitUntil: playwright.WaitUntilStateDomcontentloaded,
		}); err != nil {
			page.Close()
			session.Close()
			return nil, nil, fmt.Errorf("could not navigate to apply URL after retries: %w", err)
		}
	}

	// Wait for a brief moment to let bot-protection scripts (like Cloudflare) reveal themselves
	page.WaitForTimeout(2000)
	dismissCookieBanner(page)

	return session, page, nil
}

// newSubmitPageWithRecovery retries only the initial browser setup when
// Playwright reports that its target closed. The later fill paths already
// recover this condition, but a renderer crash can happen during context
// creation or navigation, before document generation and those paths exist.
// Keep this bounded: a browser that crashes again must release the worker for
// the next job instead of retrying indefinitely.
func newSubmitPageWithRecovery(browser playwright.Browser, applyURL string) (*secureBrowserSession, playwright.Page, error) {
	session, page, err := newSubmitPage(browser, applyURL)
	if err == nil || !isTargetClosedErr(err) {
		return session, page, err
	}

	log.Printf("[Auto-Submit] Browser target closed during initial page setup; retrying once")
	return newSubmitPage(browser, applyURL)
}

// recordAutomaticFillAttempt is storage.RecordAutomaticFillAttempt, indirected
// so tests can substitute it without a real database. A write failure here is
// deliberately non-fatal and only logged: bugs.md #551 exists because the
// automatic pipeline recorded nothing about its own fills, and refusing to
// fill an employer's form because the record *of* filling it failed to write
// would be a strictly worse trade.
var recordAutomaticFillAttempt = storage.RecordAutomaticFillAttempt

// markAutomaticFillAttempted is AttemptSubmit's only honest place to say a
// fill happened, mirroring FillAssistedMappedPage's OnFormReached (bugs.md
// #548) on the automatic side. Call it only once every guard capable of
// ending AttemptSubmit before a control is touched -- dead posting, bot
// protection, DOM quarantine, account-gated ATS, and (on the branches that
// re-check after a reveal click) the post-click captcha and auth-wall
// checks -- has already run for the branch about to dispatch to a fill
// handler. Calling it any earlier would claim Career Agent engaged a form
// that a guard proved was never reachable; calling it later, inside a
// per-ATS handler, would need six call sites instead of one per branch and
// would still miss the cached-mapping and Learner Module dispatch points,
// which share no handler code at all.
func markAutomaticFillAttempted(applyURL string) {
	if err := recordAutomaticFillAttempt(applyURL, time.Now()); err != nil {
		log.Printf("[Auto-Submit] Could not record that an automatic fill was attempted; the fill itself proceeds: %v", err)
	}
}

// AttemptSubmit scaffolds the architecture for headless browser auto-submission.
// Because job boards use heavily varied Application Tracking Systems (ATS) (like Workday, Greenhouse, Lever),
// an automated submitter requires custom DOM-parsing logic per platform.
func AttemptSubmit(browser playwright.Browser, filter *security.QuarantineLayer, mapper FormMapper, judge LLMJudge, companyName, applyURL string, generateDocs func() (string, string, error), pii *config.PII, profileContext string, headlessBrowser, copilotMode, autoSubmitClick bool) error {
	log.Printf("[Auto-Submit] Initiating submission sequence for %s at %s", companyName, applyURL)

	session, page, err := newSubmitPageWithRecovery(browser, applyURL)
	if err != nil {
		return err
	}
	defer session.Close()
	defer page.Close()

	// Check for obvious dead ends. Expired postings frequently redirect
	// to a company's main careers page or an error URL.
	if reason := DeadRedirectReason(applyURL, page.URL()); reason != "" {
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
	if err := quarantineCareerPageDOM(
		filter,
		applyURL,
		companyName,
		content,
	); err != nil {
		return fmt.Errorf("career page rejected before model use: %w", err)
	}

	// At this point, the page is live. NOW we generate the costly resume and cover letter!
	log.Printf("[Auto-Submit] Verified page is live. Generating tailored documents...")
	resumePath, coverPath, err := generateDocs()
	if err != nil {
		return fmt.Errorf("failed to generate application documents: %w", err)
	}
	// bugs.md #62: these files are not scratch temporaries. SaveApplication
	// writes them into applications/<company>/ deliberately as the saved
	// record of what was sent, and MoveToManualApply archives that whole
	// folder for jobs routed to MANUAL_REQUIRED — a queue whose entire value
	// is handing the user ready-made documents. Deleting them here fired on
	// every exit path, including the ErrAuthWall early return below, so the
	// manual queue was being stripped of its cover letter before it was ever
	// archived. Confirmed live 2026-07-24 across applications/
	// needs_manual_apply/: five sampled folders held resume.md and
	// interview_prep.md but no coverletter.txt.
	//
	// Nothing here needs cleanup: resumePath is the persistent
	// master_resume.pdf, and coverPath is either the persistent
	// master_cover_letter.txt or an application folder file that is meant to
	// outlive this call.

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
	preferCachedMapping, preferenceErr := storage.PreferCachedFormMapping(domain)
	if preferenceErr != nil {
		log.Printf("[Auto-Submit] Could not read mapping health for %s: %v", domain, preferenceErr)
		preferCachedMapping = true
	}
	if err == nil && mappingJSON != "" && preferCachedMapping {
		log.Printf("[Auto-Submit] Using learned dynamic mapping for %s", domain)
		cachedTarget := resolveFillTarget(page)
		if err := quarantineFillTargetDOM(
			filter,
			applyURL,
			companyName,
			cachedTarget,
		); err != nil {
			return fmt.Errorf("cached form rejected before model use: %w", err)
		}
		urlBeforeClick := page.URL()
		markAutomaticFillAttempted(applyURL)
		dynErr := handleDynamic(cachedTarget, resumePath, coverPath, pii, mappingJSON, copilotMode, autoSubmitClick)

		// improvements.md #471: a crashed browser target here would otherwise
		// fall straight into the cache-invalidation/Vision path below,
		// discarding a good learned mapping over a transient browser crash
		// rather than the form actually being wrong. Recover the same way
		// bug #467 does for the generic retry loop, before treating this as
		// a fill failure.
		cachedMappingRecoveries := 0
		for dynErr != nil && isTargetClosedErr(dynErr) && cachedMappingRecoveries < maxTargetRecoveryAttempts {
			cachedMappingRecoveries++
			log.Printf("[Auto-Submit] Browser target closed for %s during cached-mapping fill (%v); recreating the browser context (recovery %d/%d)",
				companyName, dynErr, cachedMappingRecoveries, maxTargetRecoveryAttempts)
			page.Close()
			session.Close()
			newSession, newPage, recErr := newSubmitPage(browser, applyURL)
			if recErr != nil {
				log.Printf("[Auto-Submit] Could not recover browser target for %s: %v", companyName, recErr)
				break
			}
			session, page = newSession, newPage
			defer session.Close()
			defer page.Close()

			if reason := DeadRedirectReason(applyURL, page.URL()); reason != "" {
				return fmt.Errorf("job posting is dead or expired: %s", reason)
			}
			if recoveredContent, cErr := page.Content(); cErr == nil {
				if isDeadJobPage(recoveredContent) {
					return fmt.Errorf("job posting is dead or expired")
				}
				if isCaptchaBlocked(page, recoveredContent) {
					return fmt.Errorf("%w at %s", ErrCaptchaBlocked, ExtractDomain(applyURL))
				}
			}
			cachedTarget = resolveFillTarget(page)
			urlBeforeClick = page.URL()
			dynErr = handleDynamic(cachedTarget, resumePath, coverPath, pii, mappingJSON, copilotMode, autoSubmitClick)
		}

		if isSubmitGated(dynErr) {
			// The cached mapping filled the form correctly; only the click was
			// withheld. Invalidating it here would discard a working blueprint.
			return dynErr
		}
		if dynErr != nil {
			log.Printf("[Auto-Submit] Dynamic Playwright mapping failed for %s. Invalidating cache. Error: %v", domain, dynErr)
			storage.DeleteFormMapping(domain)
			if mapper != nil {
				log.Printf("[Auto-Submit] Falling back to Vision module after cached-mapping fill failure...")
				return attemptQuarantinedVisionSubmit(
					page,
					cachedTarget,
					filter,
					companyName,
					applyURL,
					resumePath,
					coverPath,
					pii,
					mapper,
					copilotMode,
					autoSubmitClick,
				)
			}
			return fmt.Errorf("dynamic execution failed, cache cleared: %w", dynErr)
		}
		return confirmOrError(page, companyName, urlBeforeClick, copilotMode, autoSubmitClick)
	}
	urlLower := strings.ToLower(applyURL)
	var execErr error
	var initialAttemptComplete bool
	// urlBeforeSubmitClick tracks the page's URL immediately before each
	// attempt's own submit click, so confirmation compares against the
	// right baseline instead of the original applyURL -- see confirmOrError.
	urlBeforeSubmitClick := applyURL
	// improvements.md #32: when the submit that triggers a code email happened.
	// Only messages that arrived after this are eligible, so a code from an
	// earlier application can never be reused.
	submitClickedAt := time.Now()

	// bugs.md #88: remembers the fields the final attempt could not commit, so
	// an uncommittable required widget can be reported as needing a human
	// rather than as a generic automation failure.
	var lastNotLanded []string
	// bugs.md #108: the previous attempt's verdict, so an attempt that finds
	// nothing left to fix can tell "the form is satisfied" from "the submit
	// went nowhere".
	var lastVerdict submissionConfirmationReason
	// bugs.md #100: what the previous attempt actually wrote, so a field that
	// lands and is still rejected can be diagnosed. Keyed by the model's selector.
	lastApplied := map[string]string{}
	// bugs.md #467: how many times the browser context has been recreated
	// after a crashed target for this job, capped at maxTargetRecoveryAttempts.
	targetRecoveries := 0
	for attempt := 1; attempt <= 3; attempt++ {
		if !initialAttemptComplete {
			if strings.Contains(urlLower, "linkedin.com/jobs") {
				urlBeforeSubmitClick = page.URL()
				markAutomaticFillAttempted(applyURL)
				execErr = handleLinkedIn(page, resumePath, pii, copilotMode, autoSubmitClick)
			} else if handler, atsName := dedicatedATSHandler(applyURL); handler != nil {
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
				urlBeforeSubmitClick = page.URL()
				target := resolveFillTarget(page)
				if err := quarantineFillTargetDOM(
					filter,
					applyURL,
					companyName,
					target,
				); err != nil {
					return fmt.Errorf("%s form rejected before use: %w", atsName, err)
				}
				markAutomaticFillAttempted(applyURL)
				execErr = handler(target, resumePath, coverPath, pii, copilotMode, autoSubmitClick)
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
				if err := quarantineCareerPageDOM(
					filter,
					applyURL,
					companyName,
					domHTML,
				); err != nil {
					return fmt.Errorf("generic form rejected before model use: %w", err)
				}
				prunedHTML, err := parser.PruneDOMToText(strings.NewReader(domHTML))
				if err != nil {
					prunedHTML = domHTML
				}

				fullProfileContext := pii.ApplicationFacts() + "\n" + pii.EEO.Summary() + "\n\n" + profileContext
				// bugs.md #98: the model cannot see a react-select's permitted
				// values -- they exist only once the widget is opened, and these
				// forms carry no native <select>. Put the real options in front
				// of it rather than leaving it to guess the wording.
				if optionBlock := enumerateComboboxOptions(target, parser.InvalidFieldIdentifiers(prunedHTML)); optionBlock != "" {
					fullProfileContext += "\n\n" + optionBlock
				}
				if likelyExceedsModelContext(prunedHTML, fullProfileContext) {
					return fmt.Errorf("%w: %s", ErrFormTooLargeForModel, domain)
				}
				markAutomaticFillAttempted(applyURL)
				newMappingJSON, err := mapper.ExtractFormMapping(prunedHTML, fullProfileContext)
				if err == nil && newMappingJSON != "" {
					log.Printf("[Learner Module] Successfully mapped %s. Saving and re-attempting...", domain)
					storage.SaveFormMapping(domain, newMappingJSON)
					urlBeforeSubmitClick = page.URL()
					execErr = handleDynamic(target, resumePath, coverPath, pii, newMappingJSON, copilotMode, autoSubmitClick)
					if isSubmitGated(execErr) {
						// The mapping the Learner Module just produced worked;
						// keep it rather than discarding a fresh, good blueprint.
						return execErr
					}
					if execErr != nil {
						log.Printf("[Auto-Submit] Dynamic fill failed for %s after Learner Module mapping. Invalidating cache. Falling back to Vision module. Error: %v", domain, execErr)
						storage.DeleteFormMapping(domain)
						// AttemptVisionSubmit confirms its own submission
						// internally (bug #52 follow-up) -- return its result
						// directly rather than letting it re-enter this loop's
						// own confirmation check against a now-stale baseline.
						return attemptQuarantinedVisionSubmit(
							page,
							target,
							filter,
							companyName,
							applyURL,
							resumePath,
							coverPath,
							pii,
							mapper,
							copilotMode,
							autoSubmitClick,
						)
					}
				} else {
					log.Printf("[Learner Module] Failed to map form: %v", err)
					log.Printf("[Auto-Submit] DOM Learner Module failed. Falling back to Vision module...")
					return attemptQuarantinedVisionSubmit(
						page,
						target,
						filter,
						companyName,
						applyURL,
						resumePath,
						coverPath,
						pii,
						mapper,
						copilotMode,
						autoSubmitClick,
					)
				}
			} else {
				execErr = fmt.Errorf("unsupported Applicant Tracking System at %s", applyURL)
			}
			initialAttemptComplete = true
		} else {
			// bugs.md #89: re-check confirmation before retrying anything.
			// Greenhouse replaces the form in place, so a successful submit
			// leaves the URL unchanged and only a confirmation *phrase* can
			// prove it -- and if that page renders after the 10s networkidle
			// wait, the check right after the click sees the old DOM and
			// reports failure. Retrying from there re-submits an application
			// that already went through. Cheap to re-test, and the only
			// protection against filing duplicates with a real employer.
			if content, cErr := page.Content(); cErr == nil {
				if confirmed, reason := isSubmissionConfirmed(urlBeforeSubmitClick, page.URL(), content); confirmed {
					log.Printf("[Auto-Submit] Submission confirmed for %s on re-check before attempt %d (%s) — the previous click had succeeded", companyName, attempt, reason)
					return nil
				}
			}

			log.Printf("[Auto-Submit] Attempt %d: Solving validation errors...", attempt)
			target := resolveFillTarget(page)
			domHTML, _ := target.HTML()
			if err := quarantineCareerPageDOM(
				filter,
				applyURL,
				companyName,
				domHTML,
			); err != nil {
				return fmt.Errorf("validation DOM rejected before model use: %w", err)
			}
			prunedHTML, err := parser.PruneDOMToForm(domHTML)
			if err != nil {
				prunedHTML = domHTML
			}

			// bugs.md #111: ask the mailbox whether the previous submit was in fact
			// accepted. The DOM lags the acceptance by longer than the settle
			// budget, so this is the only signal available at this point that can
			// distinguish "blocked" from "accepted, code pending".
			emailedCode := pendingSecurityCodeAfter(submitClickedAt)
			if emailedCode != "" {
				log.Printf("[Auto-Submit] %s issued a security code after the last submit — that submission was ACCEPTED", companyName)
			}

			// bugs.md #64: send only the fields the page actually rejected.
			// The whole form is typically ~55k chars, which at this machine's
			// measured prompt-processing rate is over half an hour of
			// inference against a 45-minute timeout — large forms were failing
			// on time rather than on logic. Falls back to the full form when
			// no invalid control can be identified, since an unreadable theme
			// is a reason to send more, never less.
			// bugs.md #89: when nothing is flagged invalid, say whether a form
			// is even still on the page. "Narrowing found nothing" is
			// ambiguous between "the form is fine now" and "the form is gone
			// because it submitted", and that ambiguity is exactly what made
			// this case unreadable from the logs.
			if _, ok, _ := parser.PruneDOMToInvalidFields(prunedHTML); !ok {
				log.Printf("[Auto-Submit] Attempt %d: no field is flagged invalid (form present: %v, payload %d chars) — sending the whole form",
					attempt, strings.Contains(strings.ToLower(prunedHTML), "<form"), len(prunedHTML))
				// bugs.md #108: nothing is flagged invalid AND the previous
				// submit produced no outcome at all. There is nothing for the
				// model to fix, so sending the whole form can only waste a
				// cycle and then be refused for its size -- naming a cause
				// that has nothing to do with what happened. Report the real
				// state instead.
				if lastVerdict == reasonNoOutcomeInBudget {
					return fmt.Errorf("%w: %s", ErrSubmitProducedNoOutcome, ExtractDomain(applyURL))
				}
			}
			if narrowed, ok, nErr := parser.PruneDOMToInvalidFields(prunedHTML); nErr == nil && ok {
				// bugs.md #80: name the fields, not just the byte count. The
				// size alone cannot distinguish "the same fields are still
				// failing" from "different fields now are".
				if ids := parser.InvalidFieldIdentifiers(narrowed); len(ids) > 0 {
					sort.Strings(ids)
					log.Printf("[Auto-Submit] Narrowed validation retry to the rejected fields only (%d -> %d chars); still invalid: %s",
						len(prunedHTML), len(narrowed), strings.Join(ids, ", "))
					// bugs.md #100: a field the last attempt reported as successfully
					// set, and which came back rejected anyway, is the one case #97
					// does not cover -- it names values only for fields that failed to
					// land. Without this the rejected value is invisible and the loop
					// can only re-guess it.
					if rej := rejectedDespiteLanding(ids, lastApplied); rej != "" {
						log.Printf("[Auto-Submit] Rejected despite being set last attempt: %s", rej)
						// bugs.md #104: every field still flagged was successfully set
						// last attempt, so there is nothing left for the model to fix.
						// On a page carrying bot protection that means the submission is
						// never reaching the server and the flags are the previous
						// attempt's leftovers -- the #102 shape, with a captcha in place
						// of the code gate. #99 cannot catch it because the verdict
						// settles on flagged fields and never reaches budget exhaustion.
						// bugs.md #104 follow-up: acceptance outranks the captcha verdict,
						// exactly as #102 established. Measured the same night: Akuity
						// carries a reCAPTCHA frame AND its submit was accepted (code email
						// at 23:40:07), so "widget present + nothing left to fix" would
						// otherwise label an accepted application captcha-blocked. This
						// check sits above #93's gate handling below, so it has to test the
						// gate itself rather than rely on ordering.
						if allFieldsWereSet(ids, lastApplied) && !parser.DetectSecurityCodeChallenge(prunedHTML) && emailedCode == "" {
							if hits := detectBotProtectionOnPage(page); len(hits) > 0 {
								return fmt.Errorf("%w at %s (every rejected field was already set; %s present)",
									ErrCaptchaBlocked, ExtractDomain(applyURL), strings.Join(hits, ", "))
							}
						}
					}
				} else {
					log.Printf("[Auto-Submit] Narrowed validation retry to the rejected fields only (%d -> %d chars)", len(prunedHTML), len(narrowed))
				}
				prunedHTML = narrowed
			}

			// bugs.md #82: refuse before asking the model, not after. Once the
			// combobox commit works (#81), whatever the model picks here is
			// really submitted -- so an unanswerable attestation has to stop
			// the job, not produce a guess.
			// bugs.md #93: check before the attestation guard and before any
			// model call -- a code the agent cannot obtain makes everything
			// downstream pointless, and this is what silently cost 45 minutes.
			if parser.DetectSecurityCodeChallenge(prunedHTML) || emailedCode != "" {
				// improvements.md #32: the code is obtainable if a fetcher is
				// wired up. Without one this is a dead end, exactly as #93
				// described.
				if SecurityCodeFetcher == nil {
					return fmt.Errorf("%w: %s", ErrNeedsEmailVerification, ExtractDomain(applyURL))
				}
				log.Printf("[Auto-Submit] Security-code gate detected for %s — waiting for the emailed code...", companyName)
				// bugs.md #111: if the code is already in hand from the check above,
				// do not spend another 90 seconds waiting for it.
				code, cErr := emailedCode, error(nil)
				if code == "" {
					code, cErr = waitForSecurityCode(submitClickedAt)
				}
				if cErr != nil || code == "" {
					log.Printf("[Auto-Submit] No security code arrived for %s (%v) — queued for manual submission", companyName, cErr)
					return fmt.Errorf("%w: %s", ErrNeedsEmailVerification, ExtractDomain(applyURL))
				}
				if fErr := fillSecurityCode(target, code); fErr != nil {
					log.Printf("[Auto-Submit] Retrieved a security code for %s but could not enter it: %v", companyName, fErr)
					// bugs.md #114: name what IS on the page, since the error can
					// only name selectors that did not match. Without this the
					// real field cannot be identified without reproducing an
					// accepted submit, which means filing a real application.
					if cands := describeCodeFieldCandidates(page); cands != "" {
						log.Printf("[Auto-Submit] Fillable inputs present when the code could not be entered: %s", cands)
					}
					return fmt.Errorf("%w: %s", ErrNeedsEmailVerification, ExtractDomain(applyURL))
				}
				log.Printf("[Auto-Submit] Entered the emailed security code for %s; resubmitting", companyName)
				if sLoc, sCount := findSubmitControl(target); sCount > 0 {
					if visible, ok := firstVisibleSubmit(sLoc, sCount); ok {
						if err := submitGate(copilotMode, autoSubmitClick); err != nil {
							return err
						}
						urlBeforeSubmitClick = page.URL()
						submitClickedAt = time.Now()
						if clickErr := visible.Click(playwright.LocatorClickOptions{Timeout: playwright.Float(fillActionTimeoutMs)}); clickErr == nil {
							// bugs.md #116: wait for evidence, exactly as #95 does
							// on the other two submit paths. This third site kept
							// the original single instantaneous read after
							// WaitForLoadState, so the verdict on the
							// post-code resubmit was taken before the page could
							// answer -- and this is the last step before a
							// confirmed application, so it is the most expensive
							// place to get it wrong.
							confirmed, reason, currentURL := awaitSubmissionOutcome(page, urlBeforeSubmitClick)
							if confirmed {
								log.Printf("[Auto-Submit] Submission confirmed for %s (%s) after entering the emailed code at %s", companyName, reason, currentURL)
								return nil
							}
							log.Printf("[Auto-Submit] Resubmit after the security code did not confirm for %s (%s)", companyName, reason)
						} else if isTargetClosedErr(clickErr) {
							// Improvement #472: this click happens outside the ordinary
							// validation-resubmit path, but a renderer crash has the same
							// remedy.  Hand it to the common, one-recreation recovery block
							// below instead of misclassifying an already-entered code as an
							// email-verification failure.
							execErr = clickErr
							goto recoverTarget
						}
					}
				}
				// Entered but not confirmed: hand it over rather than loop.
				return fmt.Errorf("%w: %s (code entered, no confirmation)", ErrNeedsEmailVerification, ExtractDomain(applyURL))
			}

			if missing := pii.MissingAttestations(parser.DetectAttestationQuestions(prunedHTML)); len(missing) > 0 {
				return fmt.Errorf("%w: %s", ErrNeedsUnprovidedAttestation, strings.Join(missing, ", "))
			}

			fullProfileContext := pii.ApplicationFacts() + "\n" + pii.EEO.Summary() + "\n\n" + profileContext
			// bugs.md #98: the model cannot see a react-select's permitted
			// values -- they exist only once the widget is opened, and these
			// forms carry no native <select>. Put the real options in front
			// of it rather than leaving it to guess the wording.
			if optionBlock := enumerateComboboxOptions(target, parser.InvalidFieldIdentifiers(prunedHTML)); optionBlock != "" {
				fullProfileContext += "\n\n" + optionBlock
			}
			// bugs.md #105: count the answers this retry must generate, not just
			// the bytes it must read.
			if exceedsRetryTimeBudget(prunedHTML, fullProfileContext, len(parser.InvalidFieldIdentifiers(prunedHTML))) {
				return fmt.Errorf("%w: %s", ErrFormTooLargeForModel, ExtractDomain(applyURL))
			}
			fixesMap, fixErr := mapper.SolveValidationErrors(prunedHTML, fullProfileContext)
			if fixErr != nil {
				return fmt.Errorf("failed to solve validation errors: %w", fixErr)
			}

			// bugs.md #65: dispatch on the control's real type, and never
			// discard the outcome. A silently-failed fix leaves the field
			// exactly as invalid as it was, so the next attempt re-sends an
			// identical payload and the loop is guaranteed to exhaust itself.
			// bugs.md #70: an empty fix map is a dead end, not a no-op. The
			// previous guard only fired when the model proposed fixes that all
			// failed to apply; proposing *nothing* fell straight through to the
			// submit click below, re-submitting a byte-identical form, failing
			// validation identically, and burning another ~6-12 minutes of
			// inference. Bail out immediately instead.
			if len(fixesMap) == 0 {
				return fmt.Errorf("model proposed no fixes for the rejected fields")
			}

			// Selectors only, never values: the values are drawn from the PII
			// profile and this log is not a place for them. The selector list
			// is what makes a non-converging retry loop diagnosable -- the same
			// selectors recurring across attempts means the fix is not landing.
			// bugs.md #72: an empty value must not be counted as an applied
			// fix. applyValidationFix returns nil for one by design -- the
			// *initial* fill path relies on that to mean "the profile has no
			// data for this field, skip it". In the retry path the same return
			// is a lie: the field was just rejected, proposing "" for it
			// changes nothing, and counting it inflated the applied tally into
			// reporting progress the loop was not making.
			appliedAny := false
			applied := make([]string, 0, len(fixesMap))
			appliedValues := make(map[string]string, len(fixesMap))
			empty := make([]string, 0)
			notLanded := make([]string, 0)
			comboCommitted := make([]string, 0)
			for selector, value := range fixesMap {
				if strings.TrimSpace(value) == "" {
					empty = append(empty, selector)
					continue
				}
				if err := applyValidationFix(target, selector, value); err != nil {
					log.Printf("[Auto-Submit] Validation fix for %q failed: %v", selector, err)
					continue
				}
				applied = append(applied, selector)
				appliedValues[selector] = value
				appliedAny = true
				// bugs.md #81: a verification *error* previously fell through
				// this condition entirely -- the field was recorded as neither
				// landed nor failed, so it vanished from the logs and no commit
				// was attempted. Treat an unverifiable field as not landed.
				landed, vErr := verifyFixLanded(target, selector, value)
				if vErr != nil {
					log.Printf("[Auto-Submit] Could not verify whether %q was set (%v); treating as not set", selector, vErr)
				}
				if !landed {
					// bugs.md #74: the value may not have stuck because this is
					// an autocomplete that needs its selection committed.
					if committed, cErr := commitComboboxSelection(target, selector, value); cErr == nil && committed {
						comboCommitted = append(comboCommitted, selector)
					} else {
						// bugs.md #97 wanted the attempted value here, to tell a
						// broken commit mechanism apart from a value the widget
						// simply does not offer. bugs.md #549: that put the
						// operator's value in the log the comment above forbids
						// it from. The selector alone still answers #97's actual
						// question -- whether the same field keeps failing across
						// attempts -- without the value.
						notLanded = append(notLanded, notLandedRecord(selector, value))
					}
				}
			}
			if len(comboCommitted) > 0 {
				sort.Strings(comboCommitted)
				log.Printf("[Auto-Submit] Attempt %d committed %d autocomplete selection(s) that Fill() alone had left empty: %s", attempt, len(comboCommitted), strings.Join(comboCommitted, ", "))
			}
			lastNotLanded = notLanded
			lastApplied = appliedValues
			if len(notLanded) > 0 {
				sort.Strings(notLanded)
				log.Printf("[Auto-Submit] Attempt %d: %d fix(es) reported success but left the control empty (autocomplete/combobox suspected): %s", attempt, len(notLanded), strings.Join(notLanded, ", "))
			}
			if len(empty) > 0 {
				sort.Strings(empty)
				log.Printf("[Auto-Submit] Attempt %d: model proposed an empty value for %d rejected field(s), which cannot satisfy them: %s", attempt, len(empty), strings.Join(empty, ", "))
			}
			if !appliedAny {
				return fmt.Errorf("none of the %d proposed validation fixes could be applied", len(fixesMap))
			}
			sort.Strings(applied)
			log.Printf("[Auto-Submit] Attempt %d applied %d/%d validation fix(es) to: %s", attempt, len(applied), len(fixesMap), strings.Join(applied, ", "))

			submitLocator, count := findSubmitControl(target)
			if count > 0 {
				// bugs.md #71: when none of the matches is visible, do not fall
				// back to .First(). That match is known-hidden (typically
				// Lever's <button type="submit" class="hidden"
				// id="hcaptchaSubmitBtn">), so clicking it can only burn the
				// full action timeout before failing -- which is the exact hang
				// firstVisibleLocator was written to prevent, reintroduced by
				// its own fallback. Report it accurately and immediately.
				visible, ok := firstVisibleSubmit(submitLocator, count)
				if !ok {
					execErr = fmt.Errorf("found %d submit control(s) but none visible", count)
				} else {
					if err := submitGate(copilotMode, autoSubmitClick); err != nil {
						return err
					}
					urlBeforeSubmitClick = page.URL()
					submitClickedAt = time.Now()
					execErr = visible.Click(playwright.LocatorClickOptions{Timeout: playwright.Float(fillActionTimeoutMs)})
				}
			} else {
				execErr = fmt.Errorf("could not find submit button to retry submission")
			}
		}

	recoverTarget:
		// bugs.md #467: a crashed browser target (Playwright reports "target
		// closed"; the same live session also logged a Chromium headless
		// crash) leaves every later call against page/target failing the same
		// way. Recreate the context/page once rather than writing the whole
		// job off -- the expensive part, document generation, already
		// happened above and is untouched; only navigation and the fill
		// attempt are redone, and the recreation does not consume one of the
		// three validation-retry attempts.
		if execErr != nil && isTargetClosedErr(execErr) && targetRecoveries < maxTargetRecoveryAttempts {
			targetRecoveries++
			log.Printf("[Auto-Submit] Browser target closed for %s (%v); recreating the browser context (recovery %d/%d)",
				companyName, execErr, targetRecoveries, maxTargetRecoveryAttempts)
			page.Close()
			session.Close()
			newSession, newPage, recErr := newSubmitPage(browser, applyURL)
			if recErr != nil {
				log.Printf("[Auto-Submit] Could not recover browser target for %s: %v", companyName, recErr)
				return execErr
			}
			session, page = newSession, newPage
			defer session.Close()
			defer page.Close()

			if reason := DeadRedirectReason(applyURL, page.URL()); reason != "" {
				return fmt.Errorf("job posting is dead or expired: %s", reason)
			}
			if recoveredContent, cErr := page.Content(); cErr == nil {
				if isDeadJobPage(recoveredContent) {
					return fmt.Errorf("job posting is dead or expired")
				}
				if isCaptchaBlocked(page, recoveredContent) {
					return fmt.Errorf("%w at %s", ErrCaptchaBlocked, ExtractDomain(applyURL))
				}
			}
			execErr = nil
			initialAttemptComplete = false
			attempt--
			continue
		}

		if isSubmitGated(execErr) {
			// Nothing was clicked, so there is no failed click to explain and
			// no bot-protection verdict to reach. Returning here also keeps the
			// sentinel's identity intact — the ErrCaptchaBlocked rewrap below
			// formats execErr with %v and would file this as BLOCKED_CAPTCHA.
			return execErr
		}
		if execErr != nil {
			// bugs.md #101: a bare `Timeout 30000ms exceeded` from the submit
			// click says the click never landed and nothing about what stopped
			// it. Three jobs ended the day exactly that way (Akuity, Nova,
			// Zimperium), each written off as a generic failure. Read the
			// control's actionability directly instead of inferring it.
			if obstruction := describeSubmitObstruction(page); obstruction != "" {
				log.Printf("[Auto-Submit] Submit click failed for %s (%s); submit control: %s", companyName, execErr, obstruction)
			}
			// Only claim a bot-protection block when a provider frame is
			// actually embedded -- #45/#46 were false positives that killed most
			// jobs on this platform, so this stays evidence-led.
			if hits := detectBotProtectionOnPage(page); len(hits) > 0 {
				return fmt.Errorf("%w at %s (submit click did not land; %s present): %v",
					ErrCaptchaBlocked, ExtractDomain(applyURL), strings.Join(hits, ", "), execErr)
			}
			return execErr
		}

		if autoSubmitClick {
			// bugs.md #95: same race as confirmOrError, and more costly here --
			// a premature verdict spends another ~12-minute model call and then
			// re-clicks submit on a form that may already have gone through.
			confirmed, reason, currentURL := awaitSubmissionOutcome(page, urlBeforeSubmitClick)
			lastVerdict = reason
			if confirmed {
				log.Printf("[Auto-Submit] Submission confirmed for %s (%s) at %s", companyName, reason, currentURL)
				return nil
			}

			// bugs.md #102: the server accepted this submission and is asking
			// for an emailed code. It is not a validation failure and must not
			// be attributed to bot protection by #99 below. Fall through to the
			// next attempt, whose #93/#32 block retrieves and enters the code.
			if reason == reasonSecurityCodeGate {
				log.Printf("[Auto-Submit] %s accepted the submission and issued a security-code challenge", companyName)
				continue
			}
			// bugs.md #99: a submit that produced neither confirmation nor
			// rejection, on a page carrying a live bot-protection widget, is a
			// silently swallowed submission -- not a validation bounce. Reddit
			// reached invalid fields: 0 and still went nowhere, and no Greenhouse
			// email arrived, so the request never reached the server. Report the
			// real cause instead of spending another ~15-minute model call on a
			// form that has nothing left to fix.
			if reason == reasonNoOutcomeInBudget {
				if hits := detectBotProtectionOnPage(page); len(hits) > 0 {
					return fmt.Errorf("%w at %s (submit produced no outcome; %s present)",
						ErrCaptchaBlocked, ExtractDomain(applyURL), strings.Join(hits, ", "))
				}
			}
			log.Printf("[Auto-Submit] Submission failed validation (%s). Retrying...", reason)
		} else {
			return nil
		}
	}

	// bugs.md #88: if the last attempt left a required widget uncommitted, this
	// is not an automation failure -- the value simply is not selectable there.
	// Preserve the job for manual completion instead of writing it off.
	if len(lastNotLanded) > 0 {
		sort.Strings(lastNotLanded)
		return fmt.Errorf("%w: %s", ErrUncommittableField, strings.Join(lastNotLanded, ", "))
	}
	return fmt.Errorf("failed to submit application after 3 validation error attempts")
}

func handleLinkedIn(page playwright.Page, resumePath string, pii *config.PII, copilotMode, autoSubmitClick bool) error {
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

func fillPreMappedATSSelectors(target fillTarget, pii *config.PII, ats string) {
	if pii == nil {
		return
	}

	type selectorMapping struct {
		value    string
		selector string
	}

	var mappings []selectorMapping

	switch ats {
	case "greenhouse":
		mappings = []selectorMapping{
			{pii.Links.LinkedIn, "input[autocomplete='custom-network-linkedin'], label:has-text('LinkedIn') input, label:has-text('LinkedIn') textarea"},
			{pii.Links.GitHub, "input[autocomplete='custom-network-github'], label:has-text('GitHub') input, label:has-text('GitHub') textarea"},
			{pii.Links.Portfolio, "input[autocomplete='custom-network-portfolio'], label:has-text('Portfolio') input, label:has-text('Website') input, label:has-text('Portfolio') textarea, label:has-text('Website') textarea"},
			{pii.Links.Twitter, "input[autocomplete='custom-network-twitter'], label:has-text('Twitter') input, label:has-text('Twitter') textarea"},
			{pii.Work.CurrentEmployer, "label:has-text('Current Company') input, label:has-text('Current Employer') input"},
		}
	case "lever":
		mappings = []selectorMapping{
			{pii.Links.LinkedIn, "input[name='urls[LinkedIn]']"},
			{pii.Links.GitHub, "input[name='urls[GitHub]']"},
			{pii.Links.Portfolio, "input[name='urls[Portfolio]'], input[name='urls[Website]']"},
			{pii.Links.Twitter, "input[name='urls[Twitter]']"},
			{pii.Work.CurrentEmployer, "input[name='org']"},
		}
	case "ashby":
		mappings = []selectorMapping{
			{pii.Links.LinkedIn, "input[name='linkedin'], input[name='linkedInUrl'], label:has-text('LinkedIn') input"},
			{pii.Links.GitHub, "input[name='github'], input[name='githubUrl'], label:has-text('GitHub') input"},
			{pii.Links.Portfolio, "input[name='portfolio'], input[name='portfolioUrl'], input[name='website'], input[name='websiteUrl'], label:has-text('Website') input, label:has-text('Portfolio') input"},
			{pii.Work.CurrentEmployer, "input[name='currentEmployer'], label:has-text('Current Employer') input"},
		}
	}

	for _, m := range mappings {
		if m.value == "" {
			continue
		}
		loc := target.Loc(m.selector)
		count, err := loc.Count()
		if err != nil || count == 0 {
			continue
		}
		if err := loc.First().Fill(m.value); err != nil {
			// A matched selector that refuses the value is not a silent miss:
			// the field may be disabled, hidden, or fail validation. Log the
			// selector only (never the value) so the report stays honest and the
			// operator can see that a known value did not commit.
			log.Printf("[Assisted] Pre-mapped selector %q matched but the control did not accept the value: reason=%q", m.selector, security.BrowserFailureReason(err))
		}
	}
}

// atsFillHandler fills an application form on an ATS this project has a
// hand-written handler for. Every implementation consults submitGate before
// touching the form's final submit control, so a handler invoked with
// copilotMode set fills and stops.
type atsFillHandler func(target fillTarget, resumePath, coverPath string, pii *config.PII, copilotMode, autoSubmitClick bool) error

// dedicatedATSHandler returns the hand-written handler for applyURL's ATS
// and a display name for it, or (nil, "") when no dedicated handler covers
// the domain and the caller should fall back to a learned form mapping.
//
// This exists as one shared decision rather than a branch per caller
// (bugs.md #519): the automatic path routed Greenhouse/Lever/Ashby here
// while Assisted Apply knew only about cached mappings, which these three
// platforms never produce — so the feature could not prefill on precisely
// the two ATSes it is used with. A single router means a newly supported
// ATS reaches both paths at once.
// Which ATS a URL belongs to is decided by storage.SupportedAssistedATS rather
// than by a second copy of the domain list here, so the effort model and this
// router cannot disagree about whether an application is supported.
func dedicatedATSHandler(applyURL string) (atsFillHandler, string) {
	switch storage.SupportedAssistedATS(applyURL) {
	case "Greenhouse":
		return handleGreenhouse, "Greenhouse"
	case "Lever":
		return handleLever, "Lever"
	case "Ashby":
		return handleAshby, "Ashby"
	default:
		return nil, ""
	}
}

func handleAshby(target fillTarget, resumePath, coverPath string, pii *config.PII, copilotMode, autoSubmitClick bool) error {
	log.Printf("[Auto-Submit] Detected Ashby ATS. Filling out fields...")

	if _, err := target.WaitForSel("input[name='name'], input[name='email']", 30000); err != nil {
		return fmt.Errorf("form failed to render in time: %w", err)
	}

	if pii != nil {
		if pii.FirstName != "" || pii.LastName != "" {
			nameLoc := target.Loc("input[name='name']")
			if count, _ := nameLoc.Count(); count > 0 {
				if err := nameLoc.First().Fill(pii.FirstName + " " + pii.LastName); err != nil {
					return fmt.Errorf("failed to fill name: %w", err)
				}
			}
		}
		if pii.Email != "" {
			emailLoc := target.Loc("input[name='email']")
			if count, _ := emailLoc.Count(); count > 0 {
				if err := emailLoc.First().Fill(pii.Email); err != nil {
					return fmt.Errorf("failed to fill email: %w", err)
				}
			}
		}
		if pii.Phone != "" {
			phoneLoc := target.Loc("input[name='phone']")
			if count, _ := phoneLoc.Count(); count > 0 {
				if err := phoneLoc.First().Fill(pii.Phone); err != nil {
					return fmt.Errorf("failed to fill phone: %w", err)
				}
			}
		}

		fillPreMappedATSSelectors(target, pii, "ashby")
	}

	// Resume upload. Ashby exposes no stable resume-specific id, so this used
	// to grab the form's first file input outright — which on a form that
	// also takes a cover letter can be the wrong control entirely.
	// attachResume's fallback chain prefers resume-named inputs and only
	// falls back to a lone file input that is provably not the cover-letter
	// one (bugs.md #519).
	if resumePath != "" {
		if err := attachResume(target, "", resumePath, false); err != nil {
			return err
		}
	}

	if err := submitGate(copilotMode, autoSubmitClick); err != nil {
		return err
	}
	submitLoc := target.Loc("button[type='submit']")
	if count, _ := submitLoc.Count(); count > 0 {
		if err := submitLoc.First().Click(); err != nil {
			return fmt.Errorf("failed to click submit: %w", err)
		}
	}

	return nil
}

func handleGreenhouse(target fillTarget, resumePath, coverPath string, pii *config.PII, copilotMode, autoSubmitClick bool) error {
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

		// improvements.md #28: Location and Country are required on every
		// Greenhouse form, and this handler used to skip both -- so the first
		// submit was guaranteed to bounce and only the validation-retry loop
		// (~12 min of inference on this host) could ever satisfy them. Both are
		// react-select autocompletes, so they need the commit step from bugs.md
		// #74/#76, not a bare Fill().
		//
		// Best-effort throughout: a miss here costs nothing beyond the retry
		// cycle that used to happen unconditionally.
		// mustContain guards against the geocoder's first hit being the wrong
		// place entirely -- "Macomb" surfaces Macomb, Illinois ahead of the
		// configured Michigan address (bugs.md #79).
		fillComboboxFromCandidates(target, "input#candidate-location", "Location",
			pii.LocationSearchCandidates(), pii.LocationMustContain())
		fillComboboxFromCandidates(target, "input#country", "Country",
			pii.CountrySearchCandidates(), nil)

		fillPreMappedATSSelectors(target, pii, "greenhouse")
	}

	// Upload resume. bugs.md #519 (found live 2026-08-06 on
	// job-boards.greenhouse.io/smartsheet): the modern board renders the
	// control as <input type="file" id="resume"> with no name attribute at
	// all, so this handler's own name='resume' selector matched nothing and
	// the applicant's resume was silently never attached. attachResume tries
	// that selector first, then the resume-named fallbacks (#118), then the
	// sole-non-cover-file-input rule, and stays best-effort so a form with no
	// upload control is unaffected.
	if resumePath != "" {
		if err := attachResume(target, "input[type='file'][name='resume']", resumePath, false); err != nil {
			return err
		}
	}

	// bugs.md #61. Greenhouse exposes the cover letter as a file input under
	// the same naming convention as the resume one above; the label fallback
	// inside the helper covers boards that render a paste textarea instead.
	fillCoverLetterIfPresent(target, coverPath, "input[type='file'][name='cover_letter']", "Cover Letter")

	if err := submitGate(copilotMode, autoSubmitClick); err != nil {
		return err
	}
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

	return nil
}

func handleLever(target fillTarget, resumePath, coverPath string, pii *config.PII, copilotMode, autoSubmitClick bool) error {
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

		// improvements.md #31: Lever marks location required and this handler
		// used to skip it, so the first submit always bounced and only the
		// validation-retry loop -- a SolveValidationErrors call, ~12 min of
		// inference here -- could ever satisfy it. Same gap #28 closed for
		// Greenhouse. Best-effort: a miss costs nothing beyond the retry that
		// used to happen unconditionally, and bugs.md #88 now routes a
		// genuinely uncommittable location to manual review.
		fillComboboxFromCandidates(target, "input[data-qa='location-input']", "Location",
			pii.LocationSearchCandidates(), pii.LocationMustContain())

		fillPreMappedATSSelectors(target, pii, "lever")
	}

	// Same fallback chain as the Greenhouse handler above (bugs.md #519):
	// Lever's own id is tried first, then the resume-named selectors, then
	// the sole-non-cover-file-input rule, so a template that renames the
	// control still gets the resume attached.
	if resumePath != "" {
		if err := attachResume(target, "input[type='file'][id='resume-upload-input']", resumePath, false); err != nil {
			return err
		}
	}

	// bugs.md #61. Lever renders the cover letter as a plain textarea rather
	// than an upload control on most templates, which the helper's text path
	// handles; the label fallback covers templates that differ.
	fillCoverLetterIfPresent(target, coverPath, "textarea[name='comments']", "Cover Letter")

	if err := submitGate(copilotMode, autoSubmitClick); err != nil {
		return err
	}
	if err := target.Loc("button.postings-btn.template-btn-submit").Click(); err != nil {
		return fmt.Errorf("failed to click submit: %w", err)
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
	// Answers holds a generated response for each custom screening question
	// the mapper found beyond the standard fields (keyed custom_q_1,
	// custom_q_2, ... matching the same key in Fields/Labels). Added
	// 2026-07-24 (improvements.md #16) so custom questions get answered
	// proactively on the first fill pass instead of only reactively via
	// SolveValidationErrors after a validation failure.
	Answers map[string]string `json:"answers"`
}

// firstVisibleLocator returns the first of loc's count matches that is
// actually visible, falling back to the first match if none report visible
// (or IsVisible itself errors) -- better to attempt a click and get a clear
// timeout than to silently give up when visibility detection is unreliable.
//
// Root cause this guards against, confirmed live 2026-07-24 (a real Lever
// posting, "Nova"): a broad selector like "button[type='submit']" can match
// a hidden auxiliary button belonging to an anti-spam widget (observed:
// <button type="submit" class="hidden" id="hcaptchaSubmitBtn">, part of
// Lever's hCaptcha embed) before it ever reaches the real, visible submit
// button later in the DOM. blindly clicking .First() then hangs for the
// full click timeout on an element Playwright will never consider
// clickable, misreporting a real, fixable failure as a generic timeout.
func firstVisibleLocator(loc playwright.Locator, count int) playwright.Locator {
	if candidate, ok := firstVisibleSubmit(loc, count); ok {
		return candidate
	}
	return loc.First()
}

// submitControlSelectors are tried in order, most specific first, and the
// first group with a visible match wins.
//
// bugs.md #87: the previous single selector put `button:has-text('Apply')` in
// the same alternation as the real submit controls, and CSS alternations have
// no precedence -- matches come back in DOM order. Measured on a live
// Greenhouse form:
//
//	[0] visible BUTTON type=button  "Apply"                      <- clicked
//	[1] visible BUTTON type=button  "Quick Apply with MyGreenhouse"
//	[2] visible BUTTON type=submit  "Submit application"          <- the real one
//
// So every retry "submitted" by clicking the click-to-reveal Apply button,
// which does nothing at that point. The page never changed, the same fields
// stayed flagged, and all three attempts failed identically -- in the same
// second, because no navigation ever happened.
//
// "Apply" is deliberately absent here. It reveals a form; it never submits
// one. Keeping it as a last resort would just restore the bug on any form
// where the reveal button remains in the DOM.
var submitControlSelectors = []string{
	"button[type='submit'], input[type='submit']",
	"button:has-text('Submit application'), button:has-text('Submit Application')",
	"button:has-text('Submit'), input[value='Submit']",
}

// findSubmitControl returns the highest-precedence locator that has at least
// one visible match, with its match count.
func findSubmitControl(target fillTarget) (playwright.Locator, int) {
	var firstNonEmpty playwright.Locator
	firstCount := 0
	for _, sel := range submitControlSelectors {
		loc := target.Loc(sel)
		count, err := loc.Count()
		if err != nil || count == 0 {
			continue
		}
		if firstNonEmpty == nil {
			firstNonEmpty, firstCount = loc, count
		}
		if _, ok := firstVisibleSubmit(loc, count); ok {
			return loc, count
		}
	}
	// Nothing visible anywhere: hand back the first group that matched at all
	// so the caller reports "found N but none visible" rather than "not found".
	if firstNonEmpty != nil {
		return firstNonEmpty, firstCount
	}
	return target.Loc(submitControlSelectors[0]), 0
}

// firstVisibleSubmit is firstVisibleLocator without the .First() fallback: it
// reports whether a visible match was found at all.
//
// bugs.md #71: the fallback is right for the fill paths, where a hidden
// element is still worth attempting, but wrong for the submit click. Clicking
// a match already known to be invisible cannot succeed -- it can only hang for
// the full action timeout and then surface as a bare "Timeout 30000ms
// exceeded", which is what made this failure look like a network/CPU problem
// rather than a "there is no visible submit button here" problem.
func firstVisibleSubmit(loc playwright.Locator, count int) (playwright.Locator, bool) {
	for i := 0; i < count; i++ {
		candidate := loc.Nth(i)
		if visible, err := candidate.IsVisible(); err == nil && visible {
			return candidate, true
		}
	}
	return loc.First(), false
}

var ErrEmptySelector = fmt.Errorf("empty selector provided for form filling")

// ErrNoMatchingCheckboxOption reports that none of a checkbox group's options
// matched the proposed value. bugs.md #549: named as a sentinel rather than
// formatted with the value, which the group's options -- not what was
// proposed -- is the safe thing to name, and the caller already knows which
// selector this came from.
var ErrNoMatchingCheckboxOption = fmt.Errorf("no option in the checkbox group matches the proposed value")

func safeFill(target fillTarget, selector, text string) error {
	if selector == "" {
		return ErrEmptySelector
	}
	if text == "" {
		return nil
	}
	return target.Loc(selector).Fill(text, playwright.LocatorFillOptions{Timeout: playwright.Float(fillActionTimeoutMs)})
}

// splitTagID decomposes a simple "tag#id" or "#id" selector into its parts,
// reporting false for anything more complex (descendants, attribute filters,
// class chains, selector lists) where a naive rewrite would change meaning.
//
// bugs.md #73: this exists so an id that is not a valid CSS identifier can be
// retried in attribute form. It is the leading-digit case that bites in
// practice -- Greenhouse's custom-question ids are bare integers.
func splitTagID(selector string) (tag, id string, ok bool) {
	hash := strings.IndexByte(selector, '#')
	if hash < 0 || strings.Count(selector, "#") != 1 {
		return "", "", false
	}
	tag, id = selector[:hash], selector[hash+1:]
	// bugs.md #92: brackets are deliberately allowed here. Greenhouse names
	// checkbox-group controls id="question_8242451101[]_54236360101", and
	// "#question_...[]_..." is not a valid CSS id selector -- the brackets read
	// as attribute syntax -- so the verbatim match fails and the attribute
	// form [id="..."] is the only thing that resolves it. Combinators and
	// separators still disqualify, since those mean a compound selector where
	// rewriting the whole tail as one id would change the meaning.
	if id == "" || strings.ContainsAny(id, "#.>:, \t") {
		return "", "", false
	}
	for _, r := range tag {
		if !(r >= 'a' && r <= 'z') && !(r >= 'A' && r <= 'Z') && !(r >= '0' && r <= '9') {
			return "", "", false
		}
	}
	return tag, id, true
}

// looksLikeCSSSelector reports whether s already carries CSS syntax. A bare
// word does not: Playwright reads it as a *tag name*, so "country" searches
// for a <country> element and matches nothing.
func looksLikeCSSSelector(s string) bool {
	return strings.ContainsAny(s, "#.[]>:,= \t")
}

// resolveFieldLocator finds the element a fix refers to, tolerating the model
// returning a bare id/name value instead of a real CSS selector.
//
// bugs.md #66: SolveValidationErrors routinely returns identifiers verbatim —
// confirmed live, 12 of 12 fixes on one Greenhouse form came back as
// "question_9558065008", "country", "candidate-location" — because that is
// what the id/name attributes literally say in the DOM it was shown. Passed
// straight to Loc() every one matched nothing, so the form could never be
// corrected and the job burned all three attempts. Tries the string as given
// first (so genuinely-correct selectors are unaffected), then the id and name
// interpretations.
func resolveFieldLocator(target fillTarget, selector string) (playwright.Locator, error) {
	candidates := []string{selector}
	if !looksLikeCSSSelector(selector) {
		candidates = append(candidates,
			"#"+selector,
			fmt.Sprintf("[name=%q]", selector),
			fmt.Sprintf("[id=%q]", selector),
			fmt.Sprintf("[data-qa=%q]", selector),
		)
	} else if tag, id, ok := splitTagID(selector); ok {
		// bugs.md #73: a CSS id selector cannot start with a digit, so "#430"
		// is a syntax error and matches nothing -- but Greenhouse numbers its
		// custom-question controls exactly that way (id="430"). The attribute
		// form has no such restriction. Confirmed live on Reddit: the model
		// sent bare "430" on one attempt (resolved via the fallbacks below)
		// and "input#430" on the next (used verbatim, matched nothing), so the
		// same field alternated between fillable and unfillable.
		if tag != "" {
			candidates = append(candidates, fmt.Sprintf("%s[id=%q]", tag, id))
		}
		candidates = append(candidates,
			fmt.Sprintf("[id=%q]", id),
			fmt.Sprintf("[name=%q]", id),
		)
	} else {
		// bugs.md #106: the selector looked like CSS but has no tag#id to
		// split, which is what a BARE bracketed identifier does --
		// Greenhouse names checkbox-group controls
		// "question_8242451101[]_54236360101", and the brackets alone make
		// looksLikeCSSSelector true. It is not valid CSS for an id either,
		// so it matched nothing and got no fallbacks at all: "tried 1
		// form(s)". #73 fixed the "input#430" shape and #92 the
		// "#question_...[]_..." shape; this is the third, with no prefix.
		//
		// Safe for genuine CSS: these are appended AFTER the verbatim
		// selector, and an attribute form built from a real selector simply
		// matches nothing.
		candidates = append(candidates,
			fmt.Sprintf("[id=%q]", selector),
			fmt.Sprintf("[name=%q]", selector),
		)
	}
	for _, c := range candidates {
		loc := target.Loc(c)
		count, err := loc.Count()
		if err != nil || count == 0 {
			continue
		}
		return firstVisibleLocator(loc, count), nil
	}
	return nil, fmt.Errorf("selector matched no element (tried %d form(s) of %q)", len(candidates), selector)
}

// applyValidationFix sets a value on whatever kind of control the selector
// actually resolves to, instead of assuming everything is a text input.
//
// bugs.md #65: the validation-retry loop previously called safeFill (which is
// Fill()-only) and discarded its error. Playwright refuses Fill() on a
// <select>, and Greenhouse-style forms routinely make dropdowns required
// (work authorization, EEO self-identification, "how did you hear about
// us"). So a required dropdown could never be satisfied, every attempt
// re-sent an identical payload -- confirmed live, the narrowed payload was
// byte-identical across attempts 2 and 3 -- and the job burned all three
// retries and failed, with nothing logged to say why.
func applyValidationFix(target fillTarget, selector, value string) error {
	if selector == "" {
		return ErrEmptySelector
	}
	if value == "" {
		return nil
	}
	el, err := resolveFieldLocator(target, selector)
	if err != nil {
		return err
	}

	kind, evalErr := el.Evaluate(controlShapeJS, nil)
	shape, _ := kind.(string)
	if evalErr != nil {
		shape = ""
	}

	switch {
	case strings.HasPrefix(shape, "select"):
		// Try matching the visible option text first, since the model is
		// answering the question as a human would ("Yes", "Decline to
		// answer"), then fall back to the underlying value attribute.
		if _, err := el.SelectOption(playwright.SelectOptionValues{Labels: &[]string{value}},
			playwright.LocatorSelectOptionOptions{Timeout: playwright.Float(fillActionTimeoutMs)}); err == nil {
			return nil
		}
		if _, err := el.SelectOption(playwright.SelectOptionValues{Values: &[]string{value}},
			playwright.LocatorSelectOptionOptions{Timeout: playwright.Float(fillActionTimeoutMs)}); err != nil {
			// bugs.md #549 (found by post-fix review): this used to be
			// fmt.Errorf("select option %q not available: %w", value, err),
			// which put the model's proposed value into the error string --
			// logged verbatim with %v at the call site a few lines below the
			// "Selectors only, never values" comment, and again wherever this
			// error propagates (the initial-fill path reuses this function).
			// Also drop the raw Playwright error text: it can itself carry
			// whatever the widget echoes back. security.BrowserFailureReason
			// is the existing bounded-vocabulary classifier for exactly this
			// (see its use a few hundred lines below at the refill-inventory
			// log sites).
			return fmt.Errorf("select option not available (%s)", security.BrowserFailureReason(err))
		}
		return nil

	case shape == "input|checkbox", shape == "input|radio":
		// A checkbox's "value" is its checked state; anything affirmative
		// means tick it, and an explicit negative must not silently tick it.
		// bugs.md #109: when this checkbox is one option of a group sharing a
		// name, the value names WHICH option to tick, not whether to tick this
		// one. "No" against a Yes/No/Prefer-not-to-say group means the box
		// labelled "No" -- the opposite of the standalone reading below.
		if group := checkboxGroupOptions(el); len(group) > 0 {
			if optID, _, ok := pickComboboxOption(group, value, nil); ok {
				return target.Loc(fmt.Sprintf("[id=%q]", optID)).
					Check(playwright.LocatorCheckOptions{Timeout: playwright.Float(fillActionTimeoutMs)})
			}
			// No option matches: tick nothing rather than guess, exactly as
			// #79 requires for comboboxes. A wrong choice here is a wrong
			// answer on a real application.
			//
			// bugs.md #549: this used to be fmt.Errorf("no option in the
			// checkbox group matches %q", value), putting the model's
			// proposed value into an error later logged verbatim.
			return ErrNoMatchingCheckboxOption
		}
		if isNegativeCheckboxValue(value) {
			return el.Uncheck(playwright.LocatorUncheckOptions{Timeout: playwright.Float(fillActionTimeoutMs)})
		}
		return el.Check(playwright.LocatorCheckOptions{Timeout: playwright.Float(fillActionTimeoutMs)})

	default:
		return el.Fill(value, playwright.LocatorFillOptions{Timeout: playwright.Float(fillActionTimeoutMs)})
	}
}

// verifyFixLanded reports whether a control actually holds a value after a fix
// was applied to it.
//
// bugs.md #72: applyValidationFix returning nil only means Playwright accepted
// the call, not that the control ended up set. The gap is real on ATS
// autocomplete widgets (Greenhouse's candidate-location and country are the
// known cases), where the visible text box is backed by a separate hidden
// field: typing into it without choosing a suggestion leaves the value the
// form actually validates completely unset. That failure is invisible -- the
// fix reports success, the field stays invalid, and the retry loop burns
// another ~12-minute inference cycle re-proposing the same value.
//
// The test is deliberately "is it non-empty now" rather than "does it equal
// exactly what we sent": forms legitimately reformat input (phone numbers,
// dates) and a <select> set by visible label reports its underlying value, so
// strict equality would cry wolf on fixes that did land.
// bugs.md #74: a combobox's committed value does not live on the input.
// Confirmed against Reddit's real Greenhouse markup, which is react-select:
// the <input id="candidate-location"> is a *search* box, and the chosen value
// is held in widget state and rendered into a sibling .select__single-value.
// So reading el.value reports "" both when nothing was selected and when a
// selection succeeded -- this script has to look at the widget, not the input.
// readInputValueJS reads an ordinary control's value. Never used on a
// combobox -- see readComboboxValueJS for why.
const readInputValueJS = `el => (el.type === 'checkbox' || el.type === 'radio') ? String(el.checked) : String(el.value || '')`

// readComboboxValueJS deliberately never looks at el.value.
//
// bugs.md #76: a react-select search input genuinely holds the typed text
// after Fill(), so reading el.value on a combobox reports a committed
// selection where there is none. That silently suppressed #74's commit step
// entirely -- every combobox looked satisfied, nothing was ever committed, and
// the form kept bouncing on the same required fields. Only the widget's own
// rendering of the selection counts.
// bugs.md #78: the shell lookup must not be able to match the input itself.
// react-select sets role="combobox" *on the input*, and Element.closest()
// tests the element before its ancestors -- so `closest('[role="combobox"]')`
// returned the input, which has no children, so the committed value was never
// found. Proven against Reddit's live form: with this corrected the same DOM
// reads back "Macomb, Illinois, United States" where it previously read "".
// bugs.md #81: only the widget's rendered selection counts. The earlier
// [data-value] fallback was actively wrong -- react-select puts data-value on
// .select__input-container to mirror the *typed search text* for input sizing,
// so it is non-empty the instant anything is typed, committed or not. Probed
// directly: after a bare Fill() with nothing selected, this returned
// "I don't wish to answer". That false "landed" suppressed the commit step for
// every custom question on every Greenhouse form, which is why 13/13 fixes
// "applied" and the invalid-field list came back byte-identical.
//
// This is the same mistake as #76 one layer deeper: reading the artifact of
// typing and calling it a committed value.
const readComboboxValueJS = `el => {
  const shell = el.closest('.select__control, .select-shell') ||
                (el.parentElement && el.parentElement.closest('[role="combobox"]'));
  if (!shell) return '';
  const sv = shell.querySelector('.select__single-value, .select__multi-value__label');
  return sv && sv.textContent.trim() ? sv.textContent.trim() : '';
}`

// readHiddenCommitValueJS reads the hidden field a typeahead writes its
// committed selection into (bugs.md #86: Lever's selectedLocation). Never
// reads el.value, for the same reason as #81 -- that is the typed text.
const readHiddenCommitValueJS = `el => {
  const p = el.parentElement;
  if (!p) return '';
  const h = p.querySelector('input[type="hidden"][name^="selected"]');
  return h && h.value ? h.value : '';
}`

// isComboboxInputJS reports whether the control is an autocomplete widget
// whose selection must be committed with a keypress. Deliberately narrow:
// pressing Enter in an ordinary text input can submit the form early, so this
// must not guess.
// bugs.md #86: Lever's location typeahead is a combobox with none of
// react-select's markers -- no role, no aria-*, no select__ classes. It is an
// ordinary <input name="location"> beside a hidden <input
// name="selectedLocation"> that holds the committed value, with results in a
// sibling .dropdown-results. Detecting only react-select made it read as a
// plain text input, so it was filled with text and never committed.
const isComboboxInputJS = `el => {
  if (el.closest('.select__control, .select-shell') ||
      el.getAttribute('role') === 'combobox' || el.closest('[role="combobox"]')) return true;
  const p = el.parentElement;
  return !!(p && (p.querySelector('.dropdown-results, .dropdown-container') ||
                  p.querySelector('input[type="hidden"][name^="selected"]')));
}`

// verifyFixLanded reports whether a fix produced the state the model asked for.
//
// bugs.md #107: the intended value matters. For a checkbox the model
// deliberately declined ("No"), *unchecked is the requested state*, and the
// generic "does it hold a value" read reports false -- so a correct answer was
// recorded as an uncommittable field and routed the whole job to manual
// review. Live on Sporty Group: `left the control empty ...
// input[id='question_8242451101[]_54236360101'] (tried "No")`.
func verifyFixLanded(target fillTarget, selector, value string) (bool, error) {
	el, err := resolveFieldLocator(target, selector)
	if err != nil {
		return false, err
	}
	// bugs.md #109: for a grouped checkbox, "landed" means the option the
	// value names is ticked -- which is usually a DIFFERENT element than the
	// one the selector resolved to.
	if group := checkboxGroupOptions(el); len(group) > 0 {
		optID, _, ok := pickComboboxOption(group, value, nil)
		if !ok {
			return false, nil
		}
		checked, cErr := target.Loc(fmt.Sprintf("[id=%q]", optID)).IsChecked()
		if cErr != nil {
			return false, cErr
		}
		return checked, nil
	}
	if isNegativeCheckboxValue(value) && isTickableControl(el) {
		return true, nil
	}
	return locatorHasValue(el)
}

// locatorHasValue is verifyFixLanded against an already-resolved control, so
// the label/placeholder fill tiers -- which never have a selector string to
// re-resolve -- can use the same check (bugs.md #75).
// bugs.md #76: which script is used is decided here, in Go, rather than by
// branching inside one JS blob -- so the choice is unit-testable. Getting the
// order wrong is not a hypothetical: it shipped, and it silently disabled
// #74's combobox commit on every field.
func locatorHasValue(el playwright.Locator) (bool, error) {
	combo, cErr := isComboboxLocator(el)
	script := readInputValueJS
	if cErr == nil && combo {
		script = readComboboxValueJS
	}
	got, err := el.Evaluate(script, nil)
	if err != nil {
		return false, err
	}
	s, _ := got.(string)
	if v := strings.TrimSpace(s); v != "" && v != "false" {
		return true, nil
	}
	// bugs.md #86: a typeahead may keep its committed value in a hidden
	// sibling instead of rendering it (Lever's selectedLocation).
	if cErr == nil && combo {
		if hv, hErr := el.Evaluate(readHiddenCommitValueJS, nil); hErr == nil {
			if h, _ := hv.(string); strings.TrimSpace(h) != "" {
				return true, nil
			}
		}
	}
	return false, nil
}

// bugs.md #98. react-select renders its options only into a listbox created
// when the widget opens, and Greenhouse's forms contain zero native <select>
// elements -- so no option text is ever present in the served HTML that the
// model's prompt is built from. The model was being asked to supply a value
// for a control whose permitted values it is never shown, so it guessed the
// wording. Confirmed live on Reddit: it proposed "I am not a protected
// veteran" on two consecutive attempts for a widget offering "No military
// service" and "I don't wish to answer", filtering the list to zero both
// times. Yes/No fields committed fine precisely because they are guessable.
const (
	// Bounds on how much this adds to the prompt. The payload already has a
	// hard time budget (#83), so enumerating must not push a form past it.
	maxEnumeratedComboboxes      = 25
	maxEnumeratedOptionsPerField = 40
)

// enumerateComboboxOptions opens each named control that is genuinely a
// combobox, reads its real option list, and renders a block naming the exact
// permitted values.
//
// Only comboboxes are opened, gated on isComboboxLocator. That gate is a
// correctness requirement, not an optimisation: the invalid-field list
// routinely includes checkboxes (Greenhouse's GDPR consent among them), and
// clicking one would toggle it -- silently changing the answer this function
// exists to help the model get right.
func enumerateComboboxOptions(target fillTarget, selectors []string) string {
	if len(selectors) == 0 {
		return ""
	}
	var b strings.Builder
	shown := 0
	for _, sel := range selectors {
		if shown >= maxEnumeratedComboboxes {
			break
		}
		el, err := resolveFieldLocator(target, sel)
		if err != nil {
			continue
		}
		if combo, cErr := isComboboxLocator(el); cErr != nil || !combo {
			continue
		}
		opts := openAndReadComboboxOptions(el)
		if len(opts) == 0 {
			continue
		}
		if len(opts) > maxEnumeratedOptionsPerField {
			opts = opts[:maxEnumeratedOptionsPerField]
		}
		quoted := make([]string, 0, len(opts))
		for _, o := range opts {
			// bugs.md #103: readComboboxOptions returns "id|label" so
			// pickComboboxOption can click by id. Showing that raw to the model
			// made it answer with "react-select-question_67179376-option-0|Yes"
			// -- an internal DOM id the widget does not offer. Only the label
			// is a value a human could pick, so only the label goes in.
			quoted = append(quoted, fmt.Sprintf("%q", optionLabel(o)))
		}
		fmt.Fprintf(&b, "- %s: %s\n", sel, strings.Join(quoted, " | "))
		shown++
	}
	if shown == 0 {
		return ""
	}
	return "AVAILABLE OPTIONS FOR DROPDOWN FIELDS.\n" +
		"These controls accept ONLY the values listed. Reply with one of these " +
		"strings copied exactly, character for character. Any other wording " +
		"selects nothing and leaves the field empty.\n" + b.String()
}

// optionLabel strips the "id|" prefix readComboboxOptions attaches to every
// entry, leaving the human-readable text.
//
// bugs.md #103. The pairing exists so pickComboboxOption can click an option by
// id, which is internal machinery; a value the model proposes has to be a
// string a person could actually choose. Entries with no separator are already
// bare labels and pass through unchanged.
func optionLabel(entry string) string {
	if i := strings.Index(entry, "|"); i >= 0 {
		return strings.TrimSpace(entry[i+1:])
	}
	return strings.TrimSpace(entry)
}

// openAndReadComboboxOptions opens a combobox, reads its options, and closes
// it again, leaving the control as it was found.
//
// Reads with no query typed. bugs.md #91 is the reason that matters: typing
// filters the list, and a query the widget does not recognise filters it to
// nothing -- which is exactly the state this function exists to reveal rather
// than reproduce.
func openAndReadComboboxOptions(el playwright.Locator) []string {
	if err := el.Click(playwright.LocatorClickOptions{
		Timeout: playwright.Float(fillActionTimeoutMs),
	}); err != nil {
		return nil
	}
	waitForComboboxReady(el)
	opts := readComboboxOptions(el)
	// Leave the widget closed; an open menu can swallow a later click.
	_ = el.Press("Escape", playwright.LocatorPressOptions{
		Timeout: playwright.Float(fillActionTimeoutMs),
	})
	return opts
}

// botProtectionAnchorJS reports whether the page embeds a live bot-protection
// widget. Deliberately narrow: it matches the *iframe src* of the known
// providers, not page wording.
//
// bugs.md #45/#46 are the reason for that narrowness. Both were CAPTCHA
// false positives from phrase matching, and between them they killed the large
// majority of Greenhouse/Lever/Ashby/Workable jobs before they ever reached
// fit-scoring. A false positive here is far more expensive than a false
// negative, so this only reports what it can point at.
// botProtectionSrcPattern is the single source of truth for which iframe
// sources count as a bot-protection widget. It is interpolated into the
// browser-side check below AND compiled in Go for the tests, so the test
// exercises the real pattern rather than a copy of it that can drift.
const botProtectionSrcPattern = `(recaptcha\.net|google\.com)/recaptcha/|hcaptcha\.com/captcha|challenges\.cloudflare\.com|client-api\.arkoselabs|captcha\.datadome`

var botProtectionAnchorJS = fmt.Sprintf(`() => {
  const re = new RegExp(%q, 'i');
  const hits = [];
  document.querySelectorAll('iframe').forEach(f => {
    const src = f.src || '';
    if (re.test(src)) hits.push(src.split('?')[0]);
  });
  return hits;
}`, botProtectionSrcPattern)

// detectBotProtectionOnPage returns the provider endpoints of any live
// bot-protection widget embedded in the page.
func detectBotProtectionOnPage(page playwright.Page) []string {
	got, err := page.Evaluate(botProtectionAnchorJS, nil)
	if err != nil {
		return nil
	}
	raw, ok := got.([]interface{})
	if !ok {
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, v := range raw {
		if s, ok := v.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

// allFieldsWereSet reports whether every still-rejected field was successfully
// written by the previous attempt.
//
// bugs.md #104. When that holds there is nothing left for the model to fix:
// the form is rejecting values it already has. Combined with a bot-protection
// widget it means the submission is not reaching the server at all, and the
// aria-invalid flags are the previous attempt's leftovers rather than a fresh
// verdict -- the #102 shape with a captcha in place of the code gate.
//
// Requires ALL of them, not merely some: a single genuinely-bad answer among
// several is an ordinary validation failure that deserves its remaining retry.
func allFieldsWereSet(rejectedIDs []string, applied map[string]string) bool {
	if len(rejectedIDs) == 0 || len(applied) == 0 {
		return false
	}
	for _, id := range rejectedIDs {
		found := false
		for selector := range applied {
			if selectorTargetsID(selector, id) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

// controlShapeJS reports "tagname|type" for a control, e.g. "input|checkbox".
const controlShapeJS = `el => el.tagName.toLowerCase() + '|' + ((el.type || '').toLowerCase())`

// isNegativeCheckboxValue reports whether a model-supplied value means "leave
// this box unticked". Single source of truth for applyValidationFix (which
// unchecks) and verifyFixLanded (which must then accept unchecked as correct).
//
// bugs.md #107.
func isNegativeCheckboxValue(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "false", "no", "0", "unchecked":
		return true
	}
	return false
}

// isTickableControl reports whether the control is a checkbox or radio.
func isTickableControl(el playwright.Locator) bool {
	kind, err := el.Evaluate(controlShapeJS, nil)
	if err != nil {
		return false
	}
	shape, _ := kind.(string)
	return shape == "input|checkbox" || shape == "input|radio"
}

// checkboxGroupJS reports the sibling checkboxes sharing this control's name,
// with their labels, as "id|label" entries -- the same shape
// readComboboxOptions uses, so pickComboboxOption can select among them.
// Returns an empty list for a standalone checkbox.
const checkboxGroupJS = `el => {
  const name = el.name || '';
  if (!name) return [];
  const members = Array.from(document.querySelectorAll(
    'input[type="checkbox"][name="' + name.replace(/"/g, '\\"') + '"]'));
  if (members.length < 2) return [];
  return members.map(m => {
    let text = '';
    if (m.id) {
      const l = document.querySelector('label[for="' + CSS.escape(m.id) + '"]');
      if (l) text = l.textContent.trim();
    }
    return m.id + '|' + text;
  });
}`

// checkboxGroupOptions returns the "id|label" entries of the checkbox group
// this control belongs to, or nil when it stands alone.
//
// bugs.md #109: Greenhouse renders some single-choice questions as a set of
// checkboxes sharing one name -- Sporty Group asks a Yes / No / Prefer not to
// say question exactly that way. For those, a model value of "No" means "tick
// the box labelled No", not "leave this box unticked", and the two readings
// produce opposite results.
func checkboxGroupOptions(el playwright.Locator) []string {
	got, err := el.Evaluate(checkboxGroupJS, nil)
	if err != nil {
		return nil
	}
	raw, ok := got.([]interface{})
	if !ok {
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, v := range raw {
		if s, ok := v.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

// pendingSecurityCodeAfter does ONE mailbox check for a security code issued
// after the given submit click, and reports it if present.
//
// bugs.md #111. Greenhouse ACCEPTS a submission and emails the code within ~1s,
// but the code input does not appear in the DOM for far longer -- measured on
// Akuity, where the email is timestamped 05:59:19 and the verdict at 05:59:27
// still saw no gate. So DOM detection alone cannot tell an accepted submission
// apart from a blocked one, and #104's captcha verdict fired on an application
// that had in fact gone through.
//
// The mailbox is the ground truth: SecurityCodeFetcher only returns codes that
// arrived AFTER the click that triggered them, so a hit here means "the server
// accepted this submission", independently of what the page is showing.
//
// bugs.md #117: a SINGLE fetch is not enough. Measured on ClickHouse: Greenhouse
// sent the code at 08:48:11 and a fetch at 08:48:21 -- ten seconds later --
// still returned nothing, because IMAP had not indexed it yet. The agent
// concluded the submit was not accepted, proceeded to another attempt, and
// clicked a submit button that was by then disabled (`disabled=true`), burning
// the 30s action timeout on an application that had in fact gone through.
//
// So this polls briefly. It stays far below waitForSecurityCode's 90s budget
// because it runs on every retry attempt, but long enough to outlast the
// indexing lag actually observed.
var (
	pendingCodeBudget = 25 * time.Second
	pendingCodeTick   = 5 * time.Second
)

func pendingSecurityCodeAfter(clickedAt time.Time) string {
	if SecurityCodeFetcher == nil || clickedAt.IsZero() {
		return ""
	}
	deadline := time.Now().Add(pendingCodeBudget)
	for {
		if code, err := SecurityCodeFetcher(clickedAt); err == nil && code != "" {
			return code
		}
		if time.Now().After(deadline) {
			return ""
		}
		time.Sleep(pendingCodeTick)
	}
}

// codeFieldCandidatesJS lists every text-ish input on the page with the
// attributes needed to recognise a one-time-code field, plus whether it is
// visible. Deliberately unfiltered: the point is to discover what the field
// actually looks like, not to confirm a guess about it.
const codeFieldCandidatesJS = `() => {
  const out = [];
  document.querySelectorAll('input:not([type=hidden]), textarea').forEach(el => {
    const t = (el.type || '').toLowerCase();
    if (['checkbox','radio','file','submit','button'].includes(t)) return;
    const lab = el.id ? document.querySelector('label[for="' + CSS.escape(el.id) + '"]') : null;
    out.push({
      id: el.id || '', name: el.name || '', type: t,
      placeholder: el.placeholder || '',
      autocomplete: el.getAttribute('autocomplete') || '',
      maxlength: el.maxLength > 0 ? el.maxLength : null,
      label: lab ? lab.textContent.trim().slice(0, 60) : '',
      visible: !!(el.offsetParent || el.getClientRects().length),
      inIframe: window !== window.top,
    });
  });
  return out.slice(0, 40);
}`

// describeCodeFieldCandidates renders the page's fillable inputs for the log.
//
// bugs.md #114: when the emailed code cannot be entered, the failure names the
// selectors that did not match and nothing about what IS on the page -- so the
// real field cannot be identified without reproducing an accepted submit, which
// requires filing a real application. Measured on Akuity: the gate was detected
// in the HTML, #32 retrieved the code, and a full 20-second wait found no
// visible field, so the marker that satisfied detection was probably not an
// input at all. This is the same "log the evidence before guessing the cause"
// move as #80, #96, #97 and #100, each of which paid off within one cycle.
func describeCodeFieldCandidates(page playwright.Page) string {
	got, err := page.Evaluate(codeFieldCandidatesJS, nil)
	if err != nil {
		return ""
	}
	raw, ok := got.([]interface{})
	if !ok || len(raw) == 0 {
		return "no fillable inputs on the page at all"
	}
	parts := make([]string, 0, len(raw))
	for _, v := range raw {
		parts = append(parts, fmt.Sprintf("%v", v))
	}
	return strings.Join(parts, " | ")
}

// notLandedRecord names the field that failed to land without repeating what
// was typed into it.
//
// bugs.md #549: the previous shape (`fmt.Sprintf("%s (tried %q)", selector,
// value)`) put the operator's value into the log directly beneath the
// "Selectors only, never values" comment above its call site, reversing what
// bugs.md #97 actually needed -- whether the same field keeps failing across
// attempts, which the selector alone answers. value is still a parameter
// (it's what the caller has in hand) so this stays honestly testable: a
// future change that starts formatting it back in has something concrete to
// break. Split out so the rule is testable without a browser, mirroring
// comboboxCommitRecord (bugs.md #546).
func notLandedRecord(selector, value string) string {
	return selector
}

// rejectedDespiteLanding names each still-rejected field that the previous
// attempt successfully wrote a value into, without repeating that value.
//
// bugs.md #100. #97 (now notLandedRecord) names a field only when a fix fails
// to land. The opposite case -- verifyFixLanded reports the control as set,
// and the form rejects it anyway -- had no diagnostic at all. Akuity produced
// exactly that: "applied 7/7", no not-landed line, and the identical 7 fields
// flagged again, so nothing in the log said which fields had been written and
// then refused.
//
// bugs.md #549: the original shape paired each id with the value that was
// written (`id = "value"`), which is the same operator-data-in-the-log defect
// as #97's. The field-id list alone still answers what #100 needed -- which
// already-applied fields the form keeps rejecting -- without the value.
//
// Matching is by suffix because the identifiers come from different places:
// the model's selector is `input#question_6039579009` or `#question_...`,
// while parser.InvalidFieldIdentifiers reports the bare id.
func rejectedDespiteLanding(rejectedIDs []string, applied map[string]string) string {
	if len(applied) == 0 || len(rejectedIDs) == 0 {
		return ""
	}
	ids := make([]string, 0, len(rejectedIDs))
	for _, id := range rejectedIDs {
		for selector := range applied {
			if !selectorTargetsID(selector, id) {
				continue
			}
			ids = append(ids, id)
			break
		}
	}
	if len(ids) == 0 {
		return ""
	}
	sort.Strings(ids)
	return strings.Join(ids, ", ")
}

// selectorTargetsID reports whether a model-supplied selector addresses the
// bare control id that parser.InvalidFieldIdentifiers reported.
func selectorTargetsID(selector, id string) bool {
	if id == "" {
		return false
	}
	if selector == id {
		return true
	}
	// input#question_1, #question_1, input[id='question_1'], [id="question_1"]
	for _, suffix := range []string{"#" + id, "[id='" + id + "']", `[id="` + id + `"]`} {
		if strings.HasSuffix(selector, suffix) {
			return true
		}
	}
	return false
}

// truncateForLog keeps a free-text answer readable in the log without
// dumping an entire essay response into it.
func truncateForLog(s string, max int) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

// submitObstructionJS reports what, if anything, sits on top of the submit
// control -- direct evidence of interception rather than inference from a
// timeout. Uses elementFromPoint at the control's centre, the same check a
// read-only probe used to clear Reddit's button in bugs.md #99.
const submitObstructionJS = `() => {
  const btn = document.querySelector("button[type='submit'], input[type='submit']");
  if (!btn) return {found: false};
  const r = btn.getBoundingClientRect();
  const cx = r.left + r.width/2, cy = r.top + r.height/2;
  const top = document.elementFromPoint(cx, cy);
  let coveredBy = null;
  if (top && top !== btn && !btn.contains(top)) {
    coveredBy = top.tagName +
      (top.id ? '#' + top.id : '') +
      (top.className && typeof top.className === 'string'
        ? '.' + top.className.trim().split(/\s+/).slice(0,2).join('.') : '');
    if (top.tagName === 'IFRAME' && top.src) coveredBy += ' src=' + top.src.split('?')[0];
  }
  return {found: true, disabled: !!btn.disabled, w: Math.round(r.width), h: Math.round(r.height),
          inViewport: r.top < innerHeight && r.bottom > 0, coveredBy};
}`

// describeSubmitObstruction renders the submit control's actionability for the
// log. Best-effort: any failure yields "" and the caller reports what it has.
//
// bugs.md #101: three jobs (Akuity, Nova, Zimperium) ended the day with a bare
// `playwright: timeout: Timeout 30000ms exceeded` from the submit click and no
// indication of why the control was unactionable. A timeout says the click
// never landed; it does not say what stopped it.
func describeSubmitObstruction(page playwright.Page) string {
	got, err := page.Evaluate(submitObstructionJS, nil)
	if err != nil {
		return ""
	}
	m, ok := got.(map[string]interface{})
	if !ok {
		return ""
	}
	if found, _ := m["found"].(bool); !found {
		return "no submit control present at all"
	}
	parts := []string{fmt.Sprintf("disabled=%v inViewport=%v", m["disabled"], m["inViewport"])}
	if cb, ok := m["coveredBy"].(string); ok && cb != "" {
		parts = append(parts, "covered by "+cb)
	} else {
		parts = append(parts, "nothing covering it")
	}
	return strings.Join(parts, ", ")
}

func isComboboxLocator(el playwright.Locator) (bool, error) {
	got, err := el.Evaluate(isComboboxInputJS, nil)
	if err != nil {
		return false, err
	}
	ok, _ := got.(bool)
	return ok, nil
}

// commitComboboxOnLocator is commitComboboxSelection against an
// already-resolved control (bugs.md #75).
func commitComboboxOnLocator(target fillTarget, el playwright.Locator, value string, mustContain []string) (bool, error) {
	combo, err := isComboboxLocator(el)
	if err != nil {
		return false, err
	}
	if !combo || strings.TrimSpace(value) == "" {
		return false, nil
	}
	return setComboboxValue(target, el, value, mustContain)
}

// activeDescendantJS reports which option the widget currently has focused.
//
// bugs.md #78: this replaces a document-wide count of [role="option"], which
// was worthless -- measured live, an unrelated select's open menu kept 244
// options in the document at all times, so the count was non-zero before the
// target widget had opened at all and the wait returned instantly. Every
// commit then fired into an empty menu. aria-activedescendant is set on the
// input itself, so it cannot be confused with another widget, and a non-empty
// value is the precise condition under which Enter selects something.
const activeDescendantJS = `el => el.getAttribute('aria-activedescendant') || ''`

// waitForComboboxReady blocks until the widget has focused an option, or the
// budget runs out. Best-effort: on timeout the caller still tries Enter, and
// the read-back afterwards is what actually decides whether anything
// committed.
func waitForComboboxReady(el playwright.Locator) {
	const (
		budget = 5 * time.Second
		tick   = 200 * time.Millisecond
	)
	deadline := time.Now().Add(budget)
	for time.Now().Before(deadline) {
		got, err := el.Evaluate(activeDescendantJS, nil)
		if err != nil {
			return
		}
		if s, _ := got.(string); strings.TrimSpace(s) != "" {
			return
		}
		time.Sleep(tick)
	}
}

// setComboboxValue drives an autocomplete the way a person does: click it,
// type, wait for it to focus a match, then commit.
//
// bugs.md #78: Fill() alone can never work here. Measured live against
// Reddit's form, Fill() sets input.value without react-select ever opening its
// menu -- the widget's own option count stayed 0 and aria-activedescendant
// stayed empty for a full 3 seconds, so the Enter that followed had nothing to
// select. Real keystrokes open and filter the menu within ~600ms.
func setComboboxValue(target fillTarget, el playwright.Locator, want string, mustContain []string) (bool, error) {
	for _, prefix := range searchPrefixes(want) {
		if err := el.Click(playwright.LocatorClickOptions{Timeout: playwright.Float(fillActionTimeoutMs)}); err != nil {
			return false, err
		}
		// Clear whatever a previous Fill() or attempt left behind, or typing
		// would append to it.
		_ = el.Fill("", playwright.LocatorFillOptions{Timeout: playwright.Float(fillActionTimeoutMs)})
		if err := el.Type(prefix, playwright.LocatorTypeOptions{
			Timeout: playwright.Float(fillActionTimeoutMs),
			Delay:   playwright.Float(40),
		}); err != nil {
			continue
		}
		waitForComboboxReady(el)
		opts := readComboboxOptions(el)
		if len(opts) == 0 {
			// bugs.md #91: typing a value the widget does not recognise filters
			// its list to nothing, so #90's "exactly one option is
			// unambiguous" rule could never fire for the case it was written
			// for -- a lone "Acknowledge/Confirm" is filtered out by a typed
			// "Yes". Clearing the query restores the unfiltered list.
			_ = el.Fill("", playwright.LocatorFillOptions{Timeout: playwright.Float(fillActionTimeoutMs)})
			waitForComboboxReady(el)
			opts = readComboboxOptions(el)
		}
		optID, optIndex, ok := pickComboboxOption(opts, want, mustContain)
		if !ok {
			continue
		}
		if err := target.Loc("#" + optID).Click(playwright.LocatorClickOptions{Timeout: playwright.Float(fillActionTimeoutMs)}); err == nil {
			if landed, lErr := locatorHasValue(el); lErr == nil && landed {
				return true, nil
			}
		}
		// bugs.md #86: clicking the option loses a blur race on Lever -- the
		// input blurs, the dropdown closes, and nothing is committed (measured:
		// both the visible input and selectedLocation ended up empty).
		// Keyboard selection is blur-safe. Arrow to the option that was chosen
		// rather than taking whichever is focused, so #79's guarantee -- never
		// commit the wrong entry -- still holds: "Detroit, ME" sits directly
		// beneath "Detroit, MI" in the same list.
		steps := optIndex
		if ad, adErr := el.Evaluate(activeDescendantJS, nil); adErr == nil {
			if s, _ := ad.(string); strings.TrimSpace(s) == "" {
				// Nothing focused yet, so the first ArrowDown lands on index 0.
				steps = optIndex + 1
			}
		}
		for k := 0; k < steps; k++ {
			if err := el.Press("ArrowDown", playwright.LocatorPressOptions{Timeout: playwright.Float(fillActionTimeoutMs)}); err != nil {
				break
			}
		}
		if err := el.Press("Enter", playwright.LocatorPressOptions{Timeout: playwright.Float(fillActionTimeoutMs)}); err != nil {
			continue
		}
		if landed, err := locatorHasValue(el); err == nil && landed {
			return true, nil
		}
	}
	return false, nil
}

// comboboxOptionsJS returns the options belonging to THIS widget, resolved via
// the input's aria-controls (falling back to the listbox implied by
// aria-activedescendant).
//
// bugs.md #79: scoping matters enormously. Greenhouse pages carry an
// always-present intl-tel-input phone-country widget holding ~244 options at
// all times, so any document-wide option query is permanently non-empty and
// says nothing about the widget being driven.
const comboboxOptionsJS = `el => {
  const id = el.getAttribute('aria-controls') || '';
  let box = id ? document.getElementById(id) : null;
  if (!box) {
    const ad = el.getAttribute('aria-activedescendant') || '';
    const m = ad.match(/^(.*)-option-\d+$/);
    if (m) box = document.getElementById(m[1] + '-listbox');
  }
  if (!box) {
    // bugs.md #86: Lever renders results in a sibling .dropdown-results with
    // no aria wiring at all.
    const p = el.parentElement;
    const alt = p && p.querySelector('.dropdown-results');
    if (!alt) return [];
    return Array.from(alt.querySelectorAll('.dropdown-location, [role="option"]'))
      .map(o => o.id + '|' + o.textContent.trim());
  }
  return Array.from(box.querySelectorAll('[role="option"], .select__option'))
    .map(o => o.id + '|' + o.textContent.trim());
}`

// normalizeOptionText lowercases and strips punctuation and dial-code noise,
// so "United States +1" compares as "united states".
func normalizeOptionText(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || r == ' ' {
			b.WriteRune(r)
		} else {
			b.WriteRune(' ')
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

// optionTextMatches reports whether an option's label and the wanted value
// refer to the same choice, comparing WHOLE WORDS in sequence rather than raw
// substrings.
//
// bugs.md #110. The previous rule was bidirectional strings.Contains, and a
// short label is a substring of longer prose far too easily: "No" sits inside
// "prefer NOt to say", so asking for "Prefer not to say" selected the box
// labelled "No". That is the precise failure #79 exists to prevent -- and on
// an EEO question it turns a declined answer into a substantive one on a real
// application.
//
// Word-sequence containment keeps every match the old rule was written for:
// "United States of America" still matches the option "United States +1", and
// "Macomb, MI" still matches "Macomb, MI, USA", because in both cases one
// side's words appear contiguously in the other's.
func optionTextMatches(optionText, want string) bool {
	if optionText == "" || want == "" {
		return false
	}
	if optionText == want {
		return true
	}
	o, w := strings.Fields(optionText), strings.Fields(want)
	return containsWordRun(w, o) || containsWordRun(o, w)
}

// containsWordRun reports whether needle appears as a contiguous run of whole
// words inside haystack.
func containsWordRun(haystack, needle []string) bool {
	if len(needle) == 0 || len(needle) > len(haystack) {
		return false
	}
	for i := 0; i+len(needle) <= len(haystack); i++ {
		matched := true
		for j := range needle {
			if haystack[i+j] != needle[j] {
				matched = false
				break
			}
		}
		if matched {
			return true
		}
	}
	return false
}

// searchPrefixes yields progressively shorter leading word-groups of a value.
//
// bugs.md #79: measured live, these widgets filter by substring against their
// own labels, so the configured value often cannot be typed in full --
// "United States of America" matches nothing against a list whose entry is
// "United States", and "Macomb Township, MI" matches nothing against a
// geocoder that wants "Macomb".
func searchPrefixes(value string) []string {
	words := strings.Fields(strings.ReplaceAll(value, ",", " "))
	var out []string
	seen := map[string]bool{}
	for n := len(words); n >= 1; n-- {
		p := strings.Join(words[:n], " ")
		if !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	return out
}

// pickComboboxOption chooses the option that genuinely corresponds to want.
//
// bugs.md #79: taking the *first* option is wrong and actively dangerous.
// Typing "Macomb" into Greenhouse's location field puts "Macomb, Illinois,
// United States" at option-0 while the configured address is in Michigan --
// committing that files real applications with the wrong location. Every token
// in mustContain has to appear; failing that, option and wanted value must
// contain one another. Otherwise nothing is selected at all, and the field is
// left to the validation-retry loop rather than filled with something wrong.
func pickComboboxOption(options []string, want string, mustContain []string) (id string, index int, ok bool) {
	wantN := normalizeOptionText(want)
	for i, entry := range options {
		parts := strings.SplitN(entry, "|", 2)
		if len(parts) != 2 {
			continue
		}
		optID, text := parts[0], normalizeOptionText(parts[1])
		if optID == "" {
			continue
		}
		// improvements.md #33: a token may list alternatives separated by "|",
		// any one of which satisfies it. Geocoders disagree on state spelling
		// ("Macomb, Illinois, United States" vs "Macomb, MI, USA"), so
		// demanding one exact form rejected correct options.
		matched := true
		for _, tok := range mustContain {
			anyAlt := false
			for _, alt := range strings.Split(tok, "|") {
				if t := normalizeOptionText(alt); t != "" && strings.Contains(text, t) {
					anyAlt = true
					break
				}
			}
			if !anyAlt {
				matched = false
				break
			}
		}
		if !matched {
			continue
		}
		if len(mustContain) > 0 || optionTextMatches(text, wantN) {
			return optID, i, true
		}
	}

	// bugs.md #90: a required control offering exactly one option is
	// unambiguous -- there is no wrong choice to protect against, so refusing
	// it is over-conservative and costs a real application. Measured live:
	// Sporty Group's "GDPR Acknowledgement*" offers only "Acknowledge/Confirm",
	// and the model proposed a differently-worded affirmative, so nothing
	// matched and the job went to manual review one click from completion.
	//
	// Deliberately NOT applied when mustContain is set. Those tokens exist
	// because the *identity* of the option matters (#79: "Detroit, ME" sits
	// beneath "Detroit, MI"), and a lone option that fails them is a wrong
	// answer, not an obvious one.
	if len(mustContain) == 0 && len(options) == 1 {
		parts := strings.SplitN(options[0], "|", 2)
		if len(parts) == 2 && parts[0] != "" {
			return parts[0], 0, true
		}
	}
	return "", 0, false
}

func readComboboxOptions(el playwright.Locator) []string {
	got, err := el.Evaluate(comboboxOptionsJS, nil)
	if err != nil {
		return nil
	}
	raw, ok := got.([]interface{})
	if !ok {
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, v := range raw {
		if s, ok := v.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

// fillComboboxFromCandidates types each candidate into an autocomplete until
// one actually commits a selection. Not ATS-specific: used for Greenhouse's
// react-select widgets and Lever's own typeahead alike (improvements #31).
//
// improvements.md #28: which phrasing a geocoder accepts is not knowable in
// advance, so this tries them in order and stops at the first that sticks --
// checkable only because bugs.md #74/#76 made "did the selection commit"
// observable. Entirely best-effort: every failure path leaves the field for
// the validation-retry loop, which is exactly where it used to go anyway.
func fillComboboxFromCandidates(target fillTarget, selector, what string, candidates []string, mustContain []string) {
	if len(candidates) == 0 {
		return
	}
	el, err := resolveFieldLocator(target, selector)
	if err != nil {
		return
	}
	for index, want := range candidates {
		if committed, err := setComboboxValue(target, el, want, mustContain); err == nil && committed {
			log.Printf("[Auto-Submit] %s", comboboxCommitRecord(what, index+1, len(candidates)))
			return
		}
	}
	log.Printf("[Auto-Submit] Could not commit a %s selection from %d candidate(s); leaving it to the validation retry", what, len(candidates))
}

// comboboxCommitRecord names which candidate committed without repeating what
// was typed into the control.
//
// The candidates here are the operator's own address values -- Location and
// Country are required react-select fields on every Greenhouse form -- so the
// obvious record ("Location set to %q") writes a home city and state into
// career_agent.log, and into dashboard.log too once cmd/assist runs a fill:
// security.SanitizeChildLogLine strips markup and truncates, and a well-formed
// prose line passes through it untouched. That is the boundary ADR-006 draws.
//
// The position carries the comparative diagnostic the value was there for: what
// this loop exists to answer is that different boards accept different phrasings
// (improvements #28), and a run that commits at 1 where another commits at 4
// says so, because the caller fixes the candidate order.
//
// Two honest limits. The record is no longer self-contained -- decoding "4 of 6"
// needs pii.yaml as it was at run time, and the list length itself changes with
// which address keys are set, so an old line's meaning shifts. And it names the
// candidate, not the query: setComboboxValue types searchPrefixes(want), so the
// commit may have landed on a prefix of candidate 4 rather than candidate 4
// itself. The record it replaced could not distinguish those either -- it logged
// `want`, not what was typed -- so this loses the absolute value, not a precision
// that ever existed.
//
// Note that bugs.md #97 made the opposite trade in the validation-retry path
// above (formerly browser.go:2019): it logged the attempted value. bugs.md
// #549 resolved that contradiction by bringing notLandedRecord and
// rejectedDespiteLanding in line with this function's rule -- name the
// position or the field, never the value. Split out as a function so the rule
// is testable without a browser.
func comboboxCommitRecord(what string, position, total int) string {
	return fmt.Sprintf("%s committed on the initial fill from candidate %d of %d (saved a validation-retry cycle)",
		what, position, total)
}

// commitFilledCombobox finishes an autocomplete the initial fill only typed
// into. Best-effort: a failure here must never fail an otherwise-good fill,
// since the validation-retry path is still there as a backstop.
//
// bugs.md #75: without this, every Greenhouse form carrying a react-select
// field (Location and Country are required on all of them) was guaranteed to
// bounce on the first pass and force a full validation-retry cycle -- roughly
// 12 minutes of inference on this machine -- to fix something the first pass
// could have committed with a keypress.
func commitFilledCombobox(target fillTarget, el playwright.Locator, what, value string) {
	if landed, err := locatorHasValue(el); err != nil || landed {
		return
	}
	if committed, err := commitComboboxOnLocator(target, el, value, nil); err == nil && committed {
		log.Printf("[Auto-Submit] Committed autocomplete selection for %s on the initial fill", what)
	}
}

// commitComboboxSelection finishes an autocomplete interaction that Fill()
// only half-performed: filling types the search text, and react-select focuses
// the first matching option but commits nothing until Enter is pressed.
//
// bugs.md #74. Returns whether the control ended up holding a value. It is a
// no-op on anything that is not a combobox, so an ordinary text input is never
// sent a stray Enter that could submit the form early.
func commitComboboxSelection(target fillTarget, selector, value string) (bool, error) {
	el, err := resolveFieldLocator(target, selector)
	if err != nil {
		return false, err
	}
	return commitComboboxOnLocator(target, el, value, nil)
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
		labelLoc := target.GetByLabelLoc(labelText)
		if err := labelLoc.Fill(text, playwright.LocatorFillOptions{Timeout: playwright.Float(fillActionTimeoutMs)}); err == nil {
			commitFilledCombobox(target, labelLoc, labelText, text)
			return nil
		} else {
			lastErr = fmt.Errorf("label fill for %q failed: %w", labelText, err)
		}

		phLoc := target.GetByPlaceholderLoc(labelText)
		if err := phLoc.Fill(text, playwright.LocatorFillOptions{Timeout: playwright.Float(fillActionTimeoutMs)}); err == nil {
			log.Printf("[Auto-Submit] Label fill for %q failed; placeholder fallback succeeded", labelText)
			commitFilledCombobox(target, phLoc, labelText, text)
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
	// bugs.md #67: this tier goes through applyValidationFix rather than
	// safeFill so the *initial* fill gets the same two capabilities the
	// validation-retry path already had — dispatch by control type (#65, so a
	// required <select> is settable at all) and bare-identifier resolution
	// (#66, so an id/name value returned in place of a CSS selector still
	// finds its element). Without this, a required dropdown always failed the
	// first pass and forced an avoidable, expensive validation-retry cycle.
	if err := applyValidationFix(target, selector, text); err != nil {
		if lastErr != nil {
			return fmt.Errorf("%v; CSS selector fill also failed: %w", lastErr, err)
		}
		return err
	}
	if el, rErr := resolveFieldLocator(target, selector); rErr == nil {
		commitFilledCombobox(target, el, selector, text)
	}
	if lastErr != nil {
		log.Printf("[Auto-Submit] Label and placeholder fill both failed; CSS selector fallback succeeded")
	}
	return nil
}

// coverLetterFileInputSelectors are searched, in order, for a cover letter
// upload control when the mapped selector isn't one. Every entry is scoped to
// an attribute naming the field a cover letter, deliberately: a bare
// input[type='file'] would match the *resume* input on most forms, and
// SetInputFiles replaces a file input's contents outright, so a loose selector
// here would silently overwrite the resume with the cover letter and send the
// employer no resume at all.
var coverLetterFileInputSelectors = []string{
	// Greenhouse's own naming, and the most common convention generally.
	"input[type='file'][name='cover_letter']",
	// Case-insensitive substring matches cover the rest: coverLetterFile,
	// cover-letter-upload, candidate.coverLetter, id="cover_letter_upload"...
	"input[type='file'][name*='cover' i]",
	"input[type='file'][id*='cover' i]",
	"input[type='file'][name*='letter' i]",
	"input[type='file'][id*='letter' i]",
	"input[type='file'][aria-label*='cover letter' i]",
	"input[type='file'][data-qa*='cover' i]",
}

// uploadCoverLetter attaches the letter to selector when it resolves to a file
// input, reporting whether it actually did. A non-file match (or no match) is
// not an error here — it just means this candidate wasn't the upload control,
// and the caller should keep looking.
func uploadCoverLetter(target fillTarget, selector, uploadName string, content []byte) bool {
	loc := target.Loc(selector)
	count, cErr := loc.Count()
	if cErr != nil || count == 0 {
		return false
	}
	candidate := firstVisibleLocator(loc, count)
	isFile, evalErr := candidate.Evaluate("el => el.tagName === 'INPUT' && el.type === 'file'", nil)
	if evalErr != nil {
		return false
	}
	isFileInput, ok := isFile.(bool)
	if !ok || !isFileInput {
		return false
	}
	if err := candidate.SetInputFiles([]playwright.InputFile{{
		Name:   uploadName,
		Buffer: content,
	}}, playwright.LocatorSetInputFilesOptions{Timeout: playwright.Float(fillActionTimeoutMs)}); err != nil {
		log.Printf("[Auto-Submit] Failed to upload cover letter via %q: %v", selector, err)
		return false
	}
	log.Printf("[Auto-Submit] Cover letter uploaded as %s via %q", uploadName, selector)
	return true
}

// fillCoverLetterIfPresent supplies the cover letter to whichever control the
// form exposes for it, tolerating both shapes ATS platforms use: a file input
// (upload, preferred) or a textarea/text input (paste). Best effort by design, matching
// the custom-screening-question pass below it — a cover letter is optional on
// the large majority of real postings, so failing to place one must never
// abort an otherwise complete, submittable application.
//
// bugs.md #61: nothing in this package ever did this. ExtractFormMapping has
// always been prompted to map a "cover_letter" selector and FormMapping has
// always carried it, but no handler read it, so every application this project
// sent went out resume only while still paying full LLM cost to write a letter
// that was then discarded.
func fillCoverLetterIfPresent(target fillTarget, coverPath, selector, labelText string) {
	if coverPath == "" {
		return
	}
	content, err := os.ReadFile(coverPath)
	if err != nil {
		log.Printf("[Auto-Submit] Cover letter unreadable at %s, skipping: %v", coverPath, err)
		return
	}
	if len(content) == 0 {
		return
	}

	// An upload keeps the file exactly as-is (a PDF stays a formatted PDF),
	// but a paste-into-textarea needs real text — writing a PDF's raw bytes
	// into a form field would send the employer binary garbage.
	uploadName := "cover_letter" + filepath.Ext(coverPath)
	if uploadName == "cover_letter" {
		uploadName = "cover_letter.txt"
	}

	// Uploading is strictly preferred over pasting whenever the form offers
	// it: the employer then receives the real, formatted document (the same
	// treatment the resume already gets) instead of flattened plain text that
	// loses every bit of layout. A file input also cannot be Fill()ed at all —
	// Playwright rejects it — so the control's shape has to be resolved up
	// front either way.
	//
	// The mapped selector is tried first since it is form-specific, then a
	// direct search, because the mapper frequently points cover_letter at a
	// paste textarea on forms that also expose an upload control. Without the
	// search, that mapping alone would silently downgrade every such
	// application to plain text.
	if selector != "" && uploadCoverLetter(target, selector, uploadName, content) {
		return
	}
	for _, fallbackSel := range coverLetterFileInputSelectors {
		if uploadCoverLetter(target, fallbackSel, uploadName, content) {
			return
		}
	}

	text, err := parser.ExtractDocumentText(coverPath)
	if err != nil || strings.TrimSpace(text) == "" {
		log.Printf("[Auto-Submit] Cover letter at %s has no extractable text to paste, skipping: %v", coverPath, err)
		return
	}
	if err := safeFillWithLabelFallback(target, selector, labelText, text); err != nil {
		log.Printf("[Auto-Submit] Failed to fill cover letter (optional, continuing): %v", err)
		return
	}
	log.Printf("[Auto-Submit] Cover letter filled")
}

// resumeFileInputSelectors are searched, in order, for a resume upload control
// when the mapped selector isn't one. The hazard is the mirror image of the one
// documented on coverLetterFileInputSelectors: every entry is scoped to an
// attribute naming the field a resume, deliberately, because SetInputFiles
// replaces a file input's contents outright — a loose selector that happened to
// match the *cover letter* input would overwrite the letter with the resume and
// send the employer no cover letter at all.
var resumeFileInputSelectors = []string{
	// Greenhouse's own naming, and the most common convention generally.
	"input[type='file'][name='resume']",
	// Case-insensitive substring matches cover the rest: resumeFile,
	// resume-upload, candidate.resume, id="resume_upload"...
	"input[type='file'][name*='resume' i]",
	"input[type='file'][id*='resume' i]",
	"input[type='file'][aria-label*='resume' i]",
	"input[type='file'][data-qa*='resume' i]",
	// "CV" is the common non-US naming for the same document.
	"input[type='file'][name*='cv' i]",
	"input[type='file'][id*='cv' i]",
}

// soleFileInputSelector matches file inputs that are not the cover-letter
// control. When exactly one such input exists, it is the resume input by
// elimination — the last resort after every resume-named selector has missed,
// for forms whose upload control carries no resume-ish attribute at all (React
// ATSs commonly render input[type='file'] with nothing but a generated id).
const soleFileInputSelector = "input[type='file']" +
	":not([name*='cover' i]):not([id*='cover' i])" +
	":not([name*='letter' i]):not([id*='letter' i])" +
	":not([aria-label*='cover' i]):not([aria-label*='letter' i])" +
	":not([data-qa*='cover' i]):not([data-qa*='letter' i])"

// resumeUploadLocator resolves selector only when it names a real file input.
// A non-file match (or no match) is not an error here — it just means this
// candidate wasn't the upload control, and the caller should keep looking.
func resumeUploadLocator(target fillTarget, selector string) (playwright.Locator, bool) {
	loc := target.Loc(selector)
	if loc == nil {
		return nil, false
	}
	count, cErr := loc.Count()
	if cErr != nil || count == 0 {
		return nil, false
	}
	candidate := firstVisibleLocator(loc, count)
	if candidate == nil {
		return nil, false
	}
	isFile, evalErr := candidate.Evaluate("el => el.tagName === 'INPUT' && el.type === 'file'", nil)
	if evalErr != nil {
		return nil, false
	}
	isFileInput, ok := isFile.(bool)
	if !ok || !isFileInput {
		return nil, false
	}
	return candidate, true
}

// findResumeUpload searches without reading the resume first. That ordering is
// important for optional-resume forms: if no upload control exists, a missing
// resume path is irrelevant and must not abort an otherwise valid submission.
func findResumeUpload(target fillTarget, mappedSelector string) (playwright.Locator, string, bool) {
	if mappedSelector != "" {
		if loc, ok := resumeUploadLocator(target, mappedSelector); ok {
			return loc, mappedSelector, true
		}
	}
	for _, fallbackSel := range resumeFileInputSelectors {
		if loc, ok := resumeUploadLocator(target, fallbackSel); ok {
			return loc, fallbackSel, true
		}
	}
	if soleInput := target.Loc(soleFileInputSelector); soleInput != nil {
		if count, cErr := soleInput.Count(); cErr == nil && count == 1 {
			if loc, ok := resumeUploadLocator(target, soleFileInputSelector); ok {
				return loc, soleFileInputSelector, true
			}
		}
	}
	return nil, "", false
}

// attachResume places the resume on the form, trying the mapped selector first,
// then resume-named file inputs, then — only when the form exposes exactly one
// file input that is not the cover-letter control — that input.
//
// required distinguishes the two callers' contracts. When the mapper produced a
// resume selector the form is known to want a resume, so exhausting every path
// is a genuine failure and aborts the submission. When it produced none, the
// form may legitimately have no upload control, so the search is best effort and
// a miss must not throw away an otherwise complete application.
//
// bugs.md #118: the mapped selector used to be the only thing tried, and a miss
// aborted outright. Both mapping paths emit selectors the page need not actually
// contain — the Learner Module infers them from pruned DOM, the Vision module
// from a screenshot where the real input is usually invisible behind a styled
// button — so one wrong guess discarded a fillable application. Measured live
// 2026-07-26 on sunking.pinpointhq.com: Learner and Vision each mapped the form,
// each emitted a resume selector matching nothing, and the job burned an hour
// before failing.
func attachResume(target fillTarget, mappedSelector, resumePath string, required bool) error {
	fileInput, resolvedSelector, found := findResumeUpload(target, mappedSelector)
	if !found {
		if !required {
			log.Printf("[Auto-Submit] No resume upload control found on this form, continuing without one")
			return nil
		}
		return fmt.Errorf("resume input selector not found: mapped %q, %d resume-named fallbacks, and the sole-file-input rule all missed",
			mappedSelector, len(resumeFileInputSelectors))
	}

	// Once the form exposes an upload control, an unreadable resume is a hard
	// failure. The old code silently skipped the upload when os.ReadFile
	// failed, then reported success even though no resume was attached.
	content, err := os.ReadFile(resumePath)
	if err != nil {
		return fmt.Errorf("resume unreadable at %s: %w", resumePath, err)
	}
	if len(content) == 0 {
		return fmt.Errorf("resume at %s is empty", resumePath)
	}

	if err := fileInput.SetInputFiles([]playwright.InputFile{{
		Name:   "resume.pdf",
		Buffer: content,
	}}, playwright.LocatorSetInputFilesOptions{Timeout: playwright.Float(fillActionTimeoutMs)}); err != nil {
		return fmt.Errorf("failed to upload resume via %q: %w", resolvedSelector, err)
	}
	log.Printf("[Auto-Submit] Resume uploaded via %q", resolvedSelector)
	return nil
}

func handleDynamic(target fillTarget, resumePath, coverPath string, pii *config.PII, mappingJSON string, copilotMode, autoSubmitClick bool) error {
	log.Printf("[Auto-Submit] Executing dynamic Playwright mapping...")
	var mapping FormMapping
	if err := json.Unmarshal([]byte(mappingJSON), &mapping); err != nil {
		return fmt.Errorf("failed to parse mapping json: %w", err)
	}

	if pii != nil {
		if pii.FirstName != "" {
			if err := safeFillWithLabelFallback(target, mapping.Fields["first_name"], mapping.Labels["first_name"], pii.FirstName); err != nil && !errors.Is(err, ErrEmptySelector) {
				return fmt.Errorf("failed to fill first_name: %w", err)
			}
		}
		if pii.LastName != "" {
			if err := safeFillWithLabelFallback(target, mapping.Fields["last_name"], mapping.Labels["last_name"], pii.LastName); err != nil && !errors.Is(err, ErrEmptySelector) {
				return fmt.Errorf("failed to fill last_name: %w", err)
			}
		}
		if err := safeFillWithLabelFallback(target, mapping.Fields["email"], mapping.Labels["email"], pii.Email); err != nil && !errors.Is(err, ErrEmptySelector) {
			return fmt.Errorf("failed to fill email: %w", err)
		}
		if err := safeFillWithLabelFallback(target, mapping.Fields["phone"], mapping.Labels["phone"], pii.Phone); err != nil && !errors.Is(err, ErrEmptySelector) {
			return fmt.Errorf("failed to fill phone: %w", err)
		}
	}

	mappedResume := mapping.Fields["resume"]
	if err := attachResume(target, mappedResume, resumePath, mappedResume != ""); err != nil {
		return err
	}

	fillCoverLetterIfPresent(target, coverPath, mapping.Fields["cover_letter"], mapping.Labels["cover_letter"])

	// Custom screening questions (improvements.md #16, 2026-07-24): best-
	// effort, unlike the required fields above -- if one fails to fill, the
	// ATS's own validation (or SolveValidationErrors' retry pass) is the
	// backstop, same as it already is for a question this proactive pass
	// never found at all. One bad selector shouldn't abort an otherwise
	// fillable submission.
	for key, answer := range mapping.Answers {
		if err := safeFillWithLabelFallback(target, mapping.Fields[key], mapping.Labels[key], answer); err != nil {
			log.Printf("[Auto-Submit] Failed to fill custom question %q: %v", mapping.Labels[key], err)
		}
	}

	if err := submitGate(copilotMode, autoSubmitClick); err != nil {
		return err
	}
	if sel, ok := mapping.Fields["submit_button"]; ok && sel != "" {
		err := target.Loc(sel).Click(playwright.LocatorClickOptions{Timeout: playwright.Float(fillActionTimeoutMs)})
		if err != nil {
			return fmt.Errorf("failed to click submit: %w", err)
		}
	}

	return nil
}

// FillAssistedMappedPage refills the fields this project already knows how to
// fill on an already visible assisted page. It never learns from raw DOM,
// answers unknown/legal questions, solves CAPTCHA, or clicks Submit.
//
// bugs.md #519: this used to resolve a cached dynamic mapping and nothing
// else. Cached mappings only ever exist for ATSes the Learner Module has
// mapped, and Greenhouse/Lever/Ashby are routed to hand-written handlers
// that never call SaveFormMapping — so on precisely the two platforms
// Assisted Apply is used with, every refill failed with "no reusable form
// mapping" and the operator retyped a form the pipeline had already filled.
// Route through the same dedicated handlers the automatic path uses, and
// keep the cached mapping as the fallback for everything else.
//
// It now also reports what it did. The fill used to return nothing but an
// error, which meant that after a successful refill neither the operator nor
// the dashboard knew which fields were filled or which were still empty — so
// the operator re-read the whole employer form to find out. AssistedFillPlan
// gathers a before/after inventory around the unchanged handler call, resolves
// what is left against the Approved Answer Vault, and returns the remainder as
// the short list of things that genuinely need a human.
func FillAssistedMappedPage(plan AssistedFillPlan) (FillReport, error) {
	page := plan.Page
	report := FillReport{}
	content, err := page.Content()
	if err != nil {
		return report, err
	}
	if isDeadJobPage(content) || DeadRedirectReason(plan.ApplyURL, page.URL()) != "" {
		return report, errors.New("job posting is expired")
	}
	if isCaptchaBlocked(page, content) {
		return report, ErrCaptchaBlocked
	}
	if err := quarantineCareerPageDOM(plan.Filter, plan.ApplyURL, plan.CompanyName, content); err != nil {
		return report, err
	}
	// Deliberately no clickApplyIfPresent here, unlike the automatic path:
	// ReachAssistedDestination has already advanced this browser to a
	// verified form surface, so a second Apply-labeled click inside the
	// operator's visible window would risk navigating away from the very
	// form they are about to review and submit by hand.
	target := resolveFillTarget(page)

	// Past every guard above, on a real form surface, about to touch the
	// employer's controls. This is the only honest place to call a fill
	// "attempted": an expired posting, a bot check and a quarantined DOM all
	// returned above without reading a single control, and reporting an
	// attempt for those would claim Career Agent engaged with a form that was
	// never there.
	if plan.OnFormReached != nil {
		plan.OnFormReached()
	}

	// Best effort: a page whose inventory cannot be read still gets filled by
	// the handler exactly as before. Reporting is an improvement on top of the
	// fill, never a precondition for it.
	before, inventoryErr := SnapshotControls(target)
	if inventoryErr != nil {
		log.Printf("[Assisted] Could not inventory the form before filling; the refill still runs, but its report will be incomplete: reason=%q", security.BrowserFailureReason(inventoryErr))
	}

	fillErr := runAssistedHandler(target, plan)
	if fillErr != nil {
		return report, fillErr
	}
	report.ATS = plan.ATSName
	report.Documents = plan.PreparedDocuments()

	after, err := SnapshotControls(target)
	if err != nil {
		// The handler succeeded and stopped at the submit gate; only the
		// report is unavailable. Saying so is better than discarding a good
		// fill, and better than claiming an empty question list.
		log.Printf("[Assisted] Refill completed but the form could not be re-inventoried; no question list is available: reason=%q", security.BrowserFailureReason(err))
		return report, nil
	}
	after = SanitizeControls(plan.Filter, after)
	filled, stillEmpty := DiffSnapshots(SanitizeControls(plan.Filter, before), after)
	for _, control := range filled {
		report.Filled = append(report.Filled, FilledField{Key: control.Key, Label: control.Label, How: FilledByHandler})
	}

	report.absorb(resolveAndApply(target, plan, stillEmpty))
	return report, nil
}

// runAssistedHandler performs the fill itself, unchanged from what this
// function has always done: a dedicated ATS handler when one covers the
// domain, and an already-healthy cached mapping otherwise. Both run in copilot
// mode, so both stop at the submit gate.
func runAssistedHandler(target fillTarget, plan AssistedFillPlan) error {
	if handler, atsName := dedicatedATSHandler(plan.ApplyURL); handler != nil {
		if err := quarantineFillTargetDOM(plan.Filter, plan.ApplyURL, plan.CompanyName, target); err != nil {
			return fmt.Errorf("%s form rejected before use: %w", atsName, err)
		}
		return assistedFillOutcome(handler(target, plan.ResumePath, plan.CoverPath, plan.PII, true, false))
	}
	domain := ExtractDomain(plan.ApplyURL)
	mapping, err := storage.GetFormMapping(domain)
	if err != nil {
		return fmt.Errorf("no reusable form mapping: %w", err)
	}
	healthy, err := storage.PreferCachedFormMapping(domain)
	if err != nil || !healthy {
		return errors.New("form mapping is not healthy enough to reuse")
	}
	return assistedFillOutcome(handleDynamic(target, plan.ResumePath, plan.CoverPath, plan.PII, mapping, true, false))
}

// assistedFillOutcome translates a copilot-mode fill result into the assisted
// caller's contract: a form that filled cleanly and stopped at the submit
// gate is a success, and anything else is reported so cmd/assist preserves
// the page for manual completion.
//
// A bare nil is deliberately treated as a failure. Under copilotMode every
// fill handler must return submitGate's ErrAwaitingHumanReview, so a nil
// means one of them ran past the gate and clicked the employer's Submit
// control on the operator's behalf — the one thing assisted mode must never
// do. Nothing can undo that click, but reporting it truthfully is the only
// correct remaining action, and it keeps a future handler that forgets the
// gate from silently reading as a healthy refill.
func assistedFillOutcome(err error) error {
	if errors.Is(err, ErrAwaitingHumanReview) {
		return nil
	}
	if err == nil {
		return errors.New("assisted fill did not stop at the copilot submit gate")
	}
	return err
}
