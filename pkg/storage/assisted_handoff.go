package storage

import (
	"errors"
	"net/url"
	"strings"
)

// ErrAssistedBrowserRejected reports that a posting's ATS refuses applications
// completed inside the assisted browser, so no assisted browser may be opened
// for it. Callers match it with errors.Is to tell this apart from a plan that
// is merely unavailable or stale.
var ErrAssistedBrowserRejected = errors.New("this ATS rejects applications submitted from the assisted browser")

// assistedBrowserRejection records one ATS whose own submission verification
// refuses an application completed inside the guarded assisted browser, even
// though the identical application succeeds when the operator opens the same
// posting in their ordinary browser.
//
// This registry exists because of bug #520: during the 2026-08-06 acceptance
// trial, Greenhouse succeeded 4 of 4 inside the assisted browser while every
// Lever attempt failed with "There was an error verifying your application.
// Please try again." The same Veeva (Lever) posting then submitted
// successfully in the operator's own Chrome, so the variable is the assisted
// browser combined with that ATS -- not the network guard, which relays HTTPS
// as an opaque CONNECT tunnel and cannot alter a request body.
//
// The deliberate response is to route the operator around the assisted browser
// rather than to disguise it. Career Agent does not try to defeat a site's
// anti-automation verification; when a site legitimately refuses an automated
// browser, the honest answer is to hand the operator the link and let them
// finish in a browser that site accepts. Everything Career Agent prepared --
// the tailored resume, the cover letter, the confirmation flow -- stays
// available for that hand-off.
type assistedBrowserRejection struct {
	// domainSuffix matches the posting host exactly or as a parent domain,
	// so "lever.co" covers "jobs.lever.co" but never "notlever.co".
	domainSuffix string
	reason       string
	// blocksPreflight says the ATS cannot even be *read* without the operator,
	// which is a separate claim from refusing a submission and needs its own
	// evidence.
	//
	// The two were the same field until bugs.md #545, and conflating them cost
	// the operator the one thing that still works on a submit-rejected ATS. A
	// posting Career Agent may not submit is precisely the posting whose
	// questions the operator most needs in advance, because they are going to
	// answer every one of them by hand. Lever proves the distinction: it
	// refuses the assisted browser's submissions, and serves its /apply form to
	// an anonymous request with no sign-in at all.
	//
	// Set this only for an ATS observed to be unreadable — a sign-in wall in
	// front of the form, not a rejected submission. Everything reachable
	// through it is read-only either way: preflight fills nothing and cannot
	// submit.
	blocksPreflight bool
}

// assistedBrowserRejections is intentionally a short, evidence-backed list.
// Add an entry only after a posting has been observed failing inside the
// assisted browser and succeeding in an ordinary one; a guess here silently
// costs the operator the automatic prefill for an entire ATS.
var assistedBrowserRejections = []assistedBrowserRejection{
	{
		domainSuffix: "lever.co",
		reason:       "Lever rejects applications submitted from the assisted browser with \"There was an error verifying your application\", while the same application succeeds in an ordinary browser (bug #520, observed 2026-08-06).",
		// Readable. Verified 2026-08-13 against four real postings: the /apply
		// form returns 200 to an anonymous request and inspects cleanly.
		blocksPreflight: false,
	},
}

// AssistedBrowserRejectionReason reports why a posting cannot be completed
// inside the guarded assisted browser and must be finished by the operator in
// their own browser. An empty string means the assisted browser is usable for
// that posting, which is the case for every ATS not in the registry.
func AssistedBrowserRejectionReason(rawURL string) string {
	if rejection, matched := rejectionFor(rawURL); matched {
		return rejection.reason
	}
	return ""
}

// PreflightRefusalReason reports why a posting cannot be *inspected* at all.
//
// This is a different question from AssistedBrowserRejectionReason, and asking
// the wrong one is what bugs.md #545 closed. Preflight neither fills nor
// submits: it loads a page and reads the form's structure. An ATS that refuses
// an automated submission has said nothing about whether its form can be read,
// and treating the two as one answer left the operator's Copy Application
// Packet empty for the applications that depend on it most -- 20 of the 26 in
// the live queue.
func PreflightRefusalReason(rawURL string) string {
	if rejection, matched := rejectionFor(rawURL); matched && rejection.blocksPreflight {
		return rejection.reason
	}
	return ""
}

// rejectionFor matches a posting URL against the registry.
func rejectionFor(rawURL string) (assistedBrowserRejection, bool) {
	host := assistedPostingHost(rawURL)
	if host == "" {
		return assistedBrowserRejection{}, false
	}
	for _, rejection := range assistedBrowserRejections {
		if host == rejection.domainSuffix ||
			strings.HasSuffix(host, "."+rejection.domainSuffix) {
			return rejection, true
		}
	}
	return assistedBrowserRejection{}, false
}

// assistedPostingHost extracts a lowercase hostname from a posting URL.
// A URL that does not parse, carries no host, or is not HTTP(S) yields an
// empty host so it can never accidentally match a registry entry.
func assistedPostingHost(rawURL string) string {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return ""
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return ""
	}
	return strings.ToLower(strings.TrimSuffix(parsed.Hostname(), "."))
}

// actionForOperatorBrowser is the next action for a posting whose ATS refuses
// the assisted browser. RequiresBrowser is false so the dashboard never spawns
// a browser that is already known to fail, and RequiresExplicitSubmit is true
// so the operator can still confirm the application once the employer accepts
// it -- the confirmation path is what turns the row into an APPLIED record.
func actionForOperatorBrowser(reason string) AssistedNextAction {
	return AssistedNextAction{
		Code:  "open_in_own_browser",
		Title: "Finish in your own browser",
		Instruction: reason +
			" Open the posting in your own browser, use the prepared resume and cover letter below, submit it yourself, then mark it applied only after the employer confirms it was received.",
		PrimaryButton:          "Open in Your Own Browser",
		RequiresBrowser:        false,
		DocumentsReady:         true,
		RequiresExplicitSubmit: true,
		CanContinue:            false,
	}
}
