package submitter

import (
	"errors"
	"log"
	"strings"

	"github.com/howlcipher/Career_Agent_Core/pkg/security"
	"github.com/mxschmitt/playwright-go"
)

// Preflight answers one question about an application the operator has not
// opened yet: what is this form going to ask me?
//
// It exists because the Application Knowledge inbox is only as useful as its
// input. Questions were previously recorded only while cmd/assist held a
// visible browser open on one job, so the inventory on a real installation was
// empty and "what do my queued applications need from me?" had no answer until
// the operator had already opened them one at a time -- which is the work the
// inbox is meant to remove.
//
// What this is NOT, deliberately:
//
//   - It is not a crawler. It loads one page per application, follows at most
//     the employer's own Apply control to reveal the form, and stops. It does
//     not follow links, paginate, or explore.
//   - It does not fill anything. No value from pii.yaml, the vault, or anywhere
//     else is typed into an employer's form here. The operator's answers reach a
//     form in exactly one place, and this is not it.
//   - It cannot submit. findSubmitControl is never reached from this file, and
//     the control inventory it uses already refuses to enumerate anything that
//     could submit (see TestControlInventory_NeverEnumeratesAControlThatCouldSubmit).
//     A test in this package pins the first half the same way.
//   - It does not work around a gate. CAPTCHA, a sign-in wall, a known
//     account-gated ATS, an ATS that rejects the assisted browser -- each one
//     ends the inspection with a reason code. "Preflight unavailable until
//     authenticated" is the honest answer and the only one offered.
//
// Everything it does before reading the form is the prologue AttemptSubmit has
// always run, reused rather than reimplemented: the same guarded session, the
// same dead-posting checks, the same bot-protection check, the same quarantine
// of employer-authored DOM before anything downstream sees it.

// Preflight outcome reasons. Closed vocabulary, the same shape and for the same
// purpose as security.BrowserFailureReason (ADR-006): a caller logging one of
// these is logging a constant declared here, never a substring of a page or of
// a driver's diagnostic.
const (
	// PreflightOK means the form was reached and inventoried.
	PreflightOK = ""
	// PreflightCaptchaBlocked: a bot-protection interstitial stands in front of
	// the form. Not something to defeat; something to report.
	PreflightCaptchaBlocked = "captcha_blocked"
	// PreflightAuthRequired: the application is behind a sign-in or account
	// gate, so no form exists to inspect until the operator authenticates.
	PreflightAuthRequired = "auth_required"
	// PreflightPostingDead: the posting has expired or redirected away.
	PreflightPostingDead = "posting_dead"
	// PreflightQuarantined: the page failed the prompt-injection quarantine
	// (ADR-002) and its content is not trusted enough to read further.
	PreflightQuarantined = "quarantined"
	// PreflightNavigationFailed: the page could not be loaded at all.
	PreflightNavigationFailed = "navigation_failed"
	// PreflightNoFormFound: the page loaded and holds no application form.
	// Distinct from every reason above: this one means Career Agent looked.
	PreflightNoFormFound = "no_form_found"
	// PreflightBrowserRejected has no producer and is kept only so a stored
	// verdict written before bugs.md #545 still renders.
	//
	// It used to be what preflight reported for an ATS that refuses submissions
	// from the assisted browser -- which is a claim about submitting, made by
	// a check that only ever asked whether the form could be read. Those are
	// different questions, and conflating them is the defect #545 closed. An
	// unreadable ATS now reports auth_required, which is what it is.
	PreflightBrowserRejected = "browser_rejected"
	// PreflightAlreadyApplied: this application has already been completed, so
	// there is nothing to prepare. Distinct from every refusal above -- it is
	// not a failure, and reporting it as one told the operator that an
	// application they had already submitted was rejected by the ATS.
	PreflightAlreadyApplied = "already_applied"
	// PreflightUnclassified is the fallback, never the underlying error's text.
	PreflightUnclassified = "unclassified"
)

// Inventory is what one application's form turned out to contain.
//
// Note what is absent: any value. Controls carry HasValue as a bool and nothing
// more, exactly as the assisted path's snapshot does, so an inventory can be
// stored and displayed without ever holding page content.
type Inventory struct {
	// Reason is PreflightOK when Controls is meaningful, and one of the codes
	// above otherwise.
	Reason   string
	ATS      string
	Controls []FormControl
}

// Inspected reports whether the form was actually read.
func (i Inventory) Inspected() bool { return i.Reason == PreflightOK }

// InspectApplication loads one application and reports what it asks, without
// filling or submitting anything.
//
// The browser is the caller's, so a batch reuses one headless instance across
// applications; the per-page context, its network guard and its proxy are set
// up and torn down here for each one.
func InspectApplication(browser playwright.Browser, filter *security.QuarantineLayer, companyName, applyURL string) Inventory {
	inventory := Inventory{ATS: ATSName(applyURL)}

	// An ATS whose form cannot be read without the operator signed in is not
	// inspected: there would be nothing there to read.
	//
	// This is deliberately *not* the same question as whether the ATS accepts a
	// submission from Career Agent's browser. It was, until bugs.md #545, and
	// the two answers differ on the ATS it matters most for. An application the
	// operator must finish by hand is the one whose questions they most need in
	// advance -- refusing to read it is the opposite of helping.
	if reason := assistedBrowserRejectionForPreflight(applyURL); reason != "" {
		// auth_required, not browser_rejected: this check is about whether the
		// form can be read without the operator signed in. Reporting a submit
		// rejection here would tell them a fact nothing established.
		inventory.Reason = PreflightAuthRequired
		return inventory
	}
	// Workday and friends have no pre-auth form at all. Bug #18 established
	// this costs 4-10 minutes per job to discover the hard way; there is no
	// reason for preflight to rediscover it.
	if isKnownAuthGatedHost(applyURL) {
		inventory.Reason = PreflightAuthRequired
		return inventory
	}

	session, page, err := newSubmitPageWithRecovery(browser, applyURL)
	if err != nil {
		// The error is classified rather than rendered: a navigation failure's
		// text can quote the page (ADR-006).
		log.Printf("[Preflight] Could not open the application: reason=%q", security.BrowserFailureReason(err))
		inventory.Reason = PreflightNavigationFailed
		return inventory
	}
	defer session.Close()
	defer page.Close()

	if reason := DeadRedirectReason(applyURL, page.URL()); reason != "" {
		inventory.Reason = PreflightPostingDead
		return inventory
	}
	content, _ := page.Content()
	if isDeadJobPage(content) {
		inventory.Reason = PreflightPostingDead
		return inventory
	}
	if isCaptchaBlocked(page, content) {
		inventory.Reason = PreflightCaptchaBlocked
		return inventory
	}
	if err := quarantineCareerPageDOM(filter, applyURL, companyName, content); err != nil {
		inventory.Reason = PreflightQuarantined
		return inventory
	}

	// Greenhouse, Lever and several others render the form only after their own
	// Apply control is pressed. This is the one interaction preflight performs,
	// and it is the employer's own affordance for reaching their form.
	clickApplyIfPresent(page, "[Preflight]")

	target := resolveFillTarget(page)
	controls, err := SnapshotControls(target)
	if err != nil {
		log.Printf("[Preflight] Could not read the form: reason=%q", security.BrowserFailureReason(err))
		inventory.Reason = PreflightUnclassified
		return inventory
	}
	controls = SanitizeControls(filter, controls)

	if len(controls) == 0 {
		// Distinguish "nothing here" from "we were kept out". A sign-in gate
		// looks like an empty form, and reporting it as one would tell the
		// operator this application needs nothing from them.
		if afterContent, contentErr := page.Content(); contentErr == nil && looksLikeAuthWallContent(afterContent) {
			inventory.Reason = PreflightAuthRequired
			return inventory
		}
		inventory.Reason = PreflightNoFormFound
		return inventory
	}
	if looksLikePasswordGate(controls) {
		inventory.Reason = PreflightAuthRequired
		return inventory
	}

	inventory.Controls = controls
	return inventory
}

// looksLikePasswordGate reports whether the controls found are a sign-in form
// rather than an application form.
//
// The structural signal is the reliable one, which is why it is used here and
// the phrase list is used only as a fallback: plenty of real application pages
// mention accounts in passing, and none of them ask for a password.
func looksLikePasswordGate(controls []FormControl) bool {
	for _, control := range controls {
		if strings.EqualFold(control.ControlType, "password") {
			return true
		}
	}
	return false
}

// assistedBrowserRejectionForPreflight is a seam over the storage-layer
// registry, kept as a variable so this package's tests can exercise the refusal
// without a database. It is set by cmd/preflight, which has storage available,
// to storage.PreflightRefusalReason -- the "can this be read" question, not the
// "will a submission be accepted" one. Unset, preflight simply does not apply
// the refusal, which is safe: the inspection is read-only either way.
var assistedBrowserRejectionForPreflight = func(string) string { return "" }

// SetAssistedBrowserRejectionCheck installs the read-refusal check.
func SetAssistedBrowserRejectionCheck(check func(string) string) {
	if check != nil {
		assistedBrowserRejectionForPreflight = check
	}
}

// ErrPreflightUnavailable is returned to a caller that wants an error rather
// than a reason code.
var ErrPreflightUnavailable = errors.New("this application could not be inspected in advance")
