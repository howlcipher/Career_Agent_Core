package security

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
)

// canary is the synthetic value these tests stand in for an operator's typed
// answer. It is not real data and never was; it exists so an assertion can name
// the thing that must be absent.
const canary = "CA_TEST_SECRET_543_d7f912a4"

// playwrightRetryDiagnostic is a verbatim capture of what Playwright produced
// against a local synthetic form when a fill retried against a control that
// already held the canary. It is kept as a fixture because the shape -- summary
// on the first line, the element's outer HTML on an indented continuation line
// -- is the whole reason bugs.md #543 exists.
const playwrightRetryDiagnostic = `playwright: timeout: Timeout 2500ms exceeded.
Call log:
  - waiting for locator('#salary')
    - locator resolved to <input readonly id="salary" type="text" name="salary" role="combobox" autocomplete="off" value="` + canary + `"/>
    - fill("second attempt")
  - attempting fill action
    2 × waiting for element to be visible, enabled and editable
      - element is not editable
    - retrying fill action`

func TestBrowserFailureReason_ClassifiesRealPlaywrightDiagnosticsWithoutQuotingThem(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want string
	}{
		{"retry timeout", errors.New(playwrightRetryDiagnostic), BrowserFailureTimeout},
		{
			"strict mode violation",
			fmt.Errorf("playwright: Error: strict mode violation: locator('.dup') resolved to 2 elements:\n    1) <input name=\"a\" value=\"%s\"/>", canary),
			BrowserFailureAmbiguousTarget,
		},
		{
			"closed under the action",
			errors.New("playwright: Target page, context or browser has been closed"),
			BrowserFailureClosed,
		},
		{
			"navigation",
			errors.New("playwright: Error: net::ERR_CONNECTION_REFUSED at https://example.test/apply"),
			BrowserFailureNavigation,
		},
		{"deadline", fmt.Errorf("commit answer: %w", context.DeadlineExceeded), BrowserFailureTimeout},
		{"nil", nil, BrowserFailureNone},
		{"unknown wording", errors.New("something nobody has seen before"), BrowserFailureUnclassified},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			got := BrowserFailureReason(testCase.err)
			if got != testCase.want {
				t.Fatalf("BrowserFailureReason = %q, want %q", got, testCase.want)
			}
			if strings.Contains(got, canary) {
				t.Fatal("the reason code carried the canary")
			}
		})
	}
}

// TestBrowserFailureReason_ReturnsOnlyTheClosedVocabulary is the property that
// makes this function safe: whatever it is handed, what comes back is one of a
// fixed set of strings chosen in logsafe.go. A wording change in Playwright can
// therefore cost accuracy, but it cannot cost confidentiality.
func TestBrowserFailureReason_ReturnsOnlyTheClosedVocabulary(t *testing.T) {
	allowed := map[string]bool{
		BrowserFailureNone: true, BrowserFailureTimeout: true,
		BrowserFailureAmbiguousTarget: true, BrowserFailureNotInteractable: true,
		BrowserFailureTargetMissing: true, BrowserFailureNavigation: true,
		BrowserFailureClosed: true, BrowserFailureDriverUnavailable: true,
		BrowserFailureUnclassified: true,
		RejectionUnclassified:      true,
	}
	inputs := []error{
		nil,
		errors.New(playwrightRetryDiagnostic),
		errors.New("<input value=\"" + canary + "\"> is not editable"),
		fmt.Errorf("wrapped: %w", errors.New(playwrightRetryDiagnostic)),
		errors.New(strings.Repeat(canary+" ", 500)),
		errors.New(""),
	}
	for _, err := range inputs {
		reason := BrowserFailureReason(err)
		if strings.Contains(reason, canary) {
			t.Fatalf("reason %q carried the canary", reason)
		}
		if !allowed[reason] && !strings.HasPrefix(reason, "network_") && !strings.HasPrefix(reason, "dns_") {
			t.Fatalf("reason %q is outside the closed vocabulary", reason)
		}
	}
}

func TestSanitizeChildLogLine_KeepsCareerAgentRecords(t *testing.T) {
	line := "2026/08/13 10:09:41 Filled 12 field(s) and reused 3 approved answer(s)."
	body, ok := SanitizeChildLogLine(line)
	if !ok {
		t.Fatal("an application-owned record was dropped")
	}
	if body != "Filled 12 field(s) and reused 3 approved answer(s)." {
		t.Fatalf("body = %q", body)
	}
}

// The readiness contract in bugs.md #543 and cmd/dashboard must survive the
// filter, because the filter does not know it is special.
func TestSanitizeChildLogLine_KeepsTheReadinessSentinel(t *testing.T) {
	line := "2026/08/13 10:09:41 Assisted application is open. Verified destination: application. " +
		"Complete the stated human step, then return to the dashboard and click Continue."
	body, ok := SanitizeChildLogLine(line)
	if !ok {
		t.Fatal("the readiness sentinel record was dropped")
	}
	if !strings.Contains(body, "Assisted application is open.") {
		t.Fatalf("the sentinel did not survive: %q", body)
	}
}

func TestSanitizeChildLogLine_DropsThirdPartyContinuationLines(t *testing.T) {
	for _, line := range strings.Split(playwrightRetryDiagnostic, "\n") {
		body, ok := SanitizeChildLogLine(line)
		if ok {
			t.Fatalf("a third-party diagnostic line was persisted: %q -> %q", line, body)
		}
	}
}

// A Career Agent record that carries markup is the producer-side regression
// this second pass exists for: the record is kept, the page content is not.
func TestSanitizeChildLogLine_RedactsMarkupOnAnOwnedRecord(t *testing.T) {
	line := `2026/08/13 10:09:41 Assisted refill stopped: <input value="` + canary + `">`
	body, ok := SanitizeChildLogLine(line)
	if !ok {
		t.Fatal("the record should be kept with its markup removed, not dropped")
	}
	if strings.Contains(body, canary) {
		t.Fatalf("the canary survived redaction: %q", body)
	}
	if strings.Contains(body, "<input") {
		t.Fatalf("markup survived redaction: %q", body)
	}
	if !strings.Contains(body, "Assisted refill stopped") {
		t.Fatalf("the operational message was lost: %q", body)
	}
	if !strings.Contains(body, RedactedMarkupMarker) {
		t.Fatalf("redaction was not marked: %q", body)
	}
}

func TestSanitizeChildLogLine_BoundsAnUnusuallyLongRecord(t *testing.T) {
	line := "2026/08/13 10:09:41 " + strings.Repeat("x", 50_000)
	body, ok := SanitizeChildLogLine(line)
	if !ok {
		t.Fatal("a long owned record should be bounded, not dropped")
	}
	if len(body) > MaxPersistedChildLogLine+len(TruncatedMarker)+1 {
		t.Fatalf("record was not bounded: %d bytes", len(body))
	}
	if !strings.HasSuffix(body, TruncatedMarker) {
		t.Fatalf("truncation was not marked: %q", body[len(body)-40:])
	}
}

func TestSanitizeChildLogLine_HandlesCarriageReturnsAndEmptyBodies(t *testing.T) {
	if body, ok := SanitizeChildLogLine("2026/08/13 10:09:41 Assisted browser closed.\r"); !ok || strings.HasSuffix(body, "\r") {
		t.Fatalf("CRLF was not handled: ok=%v body=%q", ok, body)
	}
	if _, ok := SanitizeChildLogLine("2026/08/13 10:09:41 "); ok {
		t.Fatal("an empty body was persisted")
	}
	if _, ok := SanitizeChildLogLine(""); ok {
		t.Fatal("an empty line was persisted")
	}
	if _, ok := SanitizeChildLogLine("2026/08/13 10:09:41 <div>" + canary + "</div>"); ok {
		t.Fatal("a record that is nothing but markup was persisted")
	}
}

// Shapes that are close to an owned record but are not one. The prefix is the
// only evidence available, so it has to be read strictly.
func TestSanitizeChildLogLine_RejectsNearMissPrefixes(t *testing.T) {
	for _, line := range []string{
		"2026/8/13 10:09:41 short date fields",
		"2026/08/13 10:09 missing seconds",
		" 2026/08/13 10:09:41 leading space",
		"[0813/100941.123:ERROR:chrome] a Chromium record",
		"prefixed 2026/08/13 10:09:41 not at the start",
	} {
		if _, ok := SanitizeChildLogLine(line); ok {
			t.Fatalf("a non-owned line was persisted: %q", line)
		}
	}
}
