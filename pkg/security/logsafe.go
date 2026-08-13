package security

import (
	"context"
	"errors"
	"regexp"
	"strings"
)

// This file holds the confidentiality boundary for logging, in two halves that
// are meant to be read together (bugs.md #543).
//
// The producer half is BrowserFailureReason: browser automation errors carry
// the page with them. Playwright's timeout and strict-mode diagnostics quote
// the target element's outer HTML, attributes included, so an error raised
// while committing an answer to a control embeds the value that control holds
// -- which on an application form is whatever the operator typed. Rendering
// such an error with %v puts that value in the process's stderr.
//
// The consumer half is SanitizeChildLogLine: a parent process that persists a
// child's stderr is persisting whatever the child's dependencies decided to
// write, which is not a set this project controls. Career Agent's own records
// are recognised and kept; everything else is dropped unread.
//
// Neither half is a blocklist of third-party phrasing. BrowserFailureReason
// only ever returns a constant from the closed vocabulary below, so its output
// cannot contain page text no matter how the input is worded, and
// SanitizeChildLogLine decides by record shape rather than by content.

// Reason codes returned by BrowserFailureReason. This vocabulary is closed on
// purpose: a caller logging one of these constants is logging a value chosen
// from this file, never a substring of the error it classified.
const (
	// BrowserFailureNone is reported for a nil error, which no caller should pass.
	BrowserFailureNone = "none"
	// BrowserFailureTimeout covers every action that ran out of time, including
	// the retry loops Playwright ends with a timeout when an element never
	// became visible, enabled or editable.
	BrowserFailureTimeout = "browser_timeout"
	// BrowserFailureAmbiguousTarget is a strict-mode violation: the locator
	// matched more than one control and Playwright refused to guess.
	BrowserFailureAmbiguousTarget = "ambiguous_target"
	// BrowserFailureNotInteractable is an element the page presented but would
	// not accept input on.
	BrowserFailureNotInteractable = "element_not_interactable"
	// BrowserFailureTargetMissing is a locator that resolved to nothing.
	BrowserFailureTargetMissing = "target_missing"
	// BrowserFailureNavigation is a failed or aborted page load.
	BrowserFailureNavigation = "navigation_failed"
	// BrowserFailureClosed is the page, context or browser going away underneath
	// the action -- normally the operator closing the assisted window.
	BrowserFailureClosed = "browser_closed"
	// BrowserFailureDriverUnavailable is the Playwright driver or a browser
	// build failing to install or start, before any page exists.
	BrowserFailureDriverUnavailable = "browser_driver_unavailable"
	// BrowserFailureUnclassified is anything this file did not recognise. It is
	// deliberately the fallback rather than the error's own text.
	BrowserFailureUnclassified = "unclassified"
)

// browserFailureSignatures maps a lowercased substring of an automation error's
// first line to a reason code. Matching is a routing decision only -- the value
// returned is always the constant, never the matched text -- so a wording
// change in Playwright degrades a reason to "unclassified" instead of leaking
// anything.
var browserFailureSignatures = []struct {
	needle string
	reason string
}{
	{"strict mode violation", BrowserFailureAmbiguousTarget},
	{"has been closed", BrowserFailureClosed},
	{"target closed", BrowserFailureClosed},
	{"browser has been closed", BrowserFailureClosed},
	{"page closed", BrowserFailureClosed},
	{"not editable", BrowserFailureNotInteractable},
	{"not enabled", BrowserFailureNotInteractable},
	{"not visible", BrowserFailureNotInteractable},
	{"intercepts pointer events", BrowserFailureNotInteractable},
	{"element is not an <input>", BrowserFailureNotInteractable},
	{"resolved to 0 elements", BrowserFailureTargetMissing},
	{"no element matching", BrowserFailureTargetMissing},
	{"waiting for selector", BrowserFailureTargetMissing},
	{"err_", BrowserFailureNavigation},
	{"navigating to", BrowserFailureNavigation},
	{"navigation failed", BrowserFailureNavigation},
	{"could not start playwright", BrowserFailureDriverUnavailable},
	{"please install", BrowserFailureDriverUnavailable},
	{"executable doesn't exist", BrowserFailureDriverUnavailable},
	{"driver", BrowserFailureDriverUnavailable},
	{"timeout", BrowserFailureTimeout},
	{"deadline exceeded", BrowserFailureTimeout},
}

// BrowserFailureReason classifies a browser automation error into a bounded
// code that is safe to write to a log, in the same spirit as
// NetworkRejectionReason and for the same reason: the error's own text is not
// this project's to disclose.
//
// Callers should log this instead of the error whenever the error could have
// come from a page. It is not a substitute for handling the error; it is a
// substitute for printing it.
func BrowserFailureReason(err error) string {
	if err == nil {
		return BrowserFailureNone
	}
	// A guard rejection is already classified, and its vocabulary is the more
	// specific one. Prefer it over re-deriving a weaker answer here.
	if reason := NetworkRejectionReason(err); reason != RejectionUnclassified {
		return reason
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return BrowserFailureTimeout
	}
	if errors.Is(err, context.Canceled) {
		return BrowserFailureClosed
	}
	// Only the first line is consulted. Everything after it is Playwright's
	// call log, which is exactly the part that quotes the page.
	first := err.Error()
	if index := strings.IndexByte(first, '\n'); index >= 0 {
		first = first[:index]
	}
	first = strings.ToLower(first)
	for _, signature := range browserFailureSignatures {
		if strings.Contains(first, signature.needle) {
			return signature.reason
		}
	}
	return BrowserFailureUnclassified
}

// MaxPersistedChildLogLine bounds how much of any single child-process log
// record this project will persist. An employer's page controls how long a
// diagnostic is; a log file should not.
const MaxPersistedChildLogLine = 1000

// applicationLogRecord matches the prefix Go's standard log package writes with
// the default flags, which is what every Career Agent command emits. It is the
// only evidence available on a raw byte stream that a line is one this project
// wrote rather than one of its dependencies.
var applicationLogRecord = regexp.MustCompile(`^\d{4}/\d{2}/\d{2} \d{2}:\d{2}:\d{2}(\.\d+)? `)

// markupOpening finds the start of an HTML tag: '<' followed by a tag name or a
// closing slash. Career Agent's own log messages never contain markup, so this
// is a structural test rather than a guess at what a third party might print.
var markupOpening = regexp.MustCompile(`</?[A-Za-z]`)

// RedactedMarkupMarker replaces the tail of a record from the first markup
// opening onward. Seeing it in a log means a producer that should have been
// reporting a bounded reason reported page content instead.
const RedactedMarkupMarker = "[markup withheld]"

// TruncatedMarker replaces the tail of an over-long record.
const TruncatedMarker = "[truncated]"

// SanitizeChildLogLine decides whether one line read from a child process's
// stderr may be persisted, and returns the text to persist.
//
// A line qualifies only by being shaped like one of this project's own log
// records. That single rule is what keeps arbitrary third-party stderr -- the
// Playwright driver, a Chromium build writing to the same descriptor, anything
// a future dependency decides to print -- out of the log entirely, without
// depending on how any of them happen to phrase things today.
//
// A qualifying record is still stripped of markup and bounded in length. That
// second pass is not redundant: it is what stops a producer-side regression, a
// wrapped error rendered with %v, from turning an application-owned record into
// a carrier for the page.
//
// The returned bool is false when nothing should be written at all.
func SanitizeChildLogLine(line string) (string, bool) {
	line = strings.TrimRight(line, "\r\n")
	location := applicationLogRecord.FindStringIndex(line)
	if location == nil {
		return "", false
	}
	// The parent stamps its own record when it writes this, so the child's
	// timestamp is dropped rather than repeated.
	body := strings.TrimSpace(line[location[1]:])
	if body == "" {
		return "", false
	}
	if opening := markupOpening.FindStringIndex(body); opening != nil {
		kept := strings.TrimSpace(body[:opening[0]])
		if kept == "" {
			// Nothing but markup. There is no operational message to preserve,
			// so the record is dropped rather than reduced to a marker.
			return "", false
		}
		body = kept + " " + RedactedMarkupMarker
	}
	if len(body) > MaxPersistedChildLogLine {
		body = strings.TrimSpace(body[:MaxPersistedChildLogLine]) + " " + TruncatedMarker
	}
	return body, true
}
