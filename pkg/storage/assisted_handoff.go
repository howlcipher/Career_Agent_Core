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
}

// assistedBrowserRejections is intentionally a short, evidence-backed list.
// Add an entry only after a posting has been observed failing inside the
// assisted browser and succeeding in an ordinary one; a guess here silently
// costs the operator the automatic prefill for an entire ATS.
var assistedBrowserRejections = []assistedBrowserRejection{
	{
		domainSuffix: "lever.co",
		reason:       "Lever rejects applications submitted from the assisted browser with \"There was an error verifying your application\", while the same application succeeds in an ordinary browser (bug #520, observed 2026-08-06).",
	},
}

// AssistedBrowserRejectionReason reports why a posting cannot be completed
// inside the guarded assisted browser and must be finished by the operator in
// their own browser. An empty string means the assisted browser is usable for
// that posting, which is the case for every ATS not in the registry.
func AssistedBrowserRejectionReason(rawURL string) string {
	host := assistedPostingHost(rawURL)
	if host == "" {
		return ""
	}
	for _, rejection := range assistedBrowserRejections {
		if host == rejection.domainSuffix ||
			strings.HasSuffix(host, "."+rejection.domainSuffix) {
			return rejection.reason
		}
	}
	return ""
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
