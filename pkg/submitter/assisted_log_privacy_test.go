package submitter

import (
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/howlcipher/Career_Agent_Core/pkg/security"
	"github.com/mxschmitt/playwright-go"
)

// logPrivacyCanary stands in for an operator's typed answer. It is synthetic:
// no real answer, resume field or pii.yaml value is used anywhere in this file.
// Its only job is to be a string that must not appear in a log.
const logPrivacyCanary = "CA_TEST_SECRET_543_d7f912a4"

// assistedCanaryForm is a local synthetic application form. The value-attribute
// mirror is not a contrivance: React-controlled inputs and the Greenhouse
// combobox bugs.md #543 was observed on both reflect the live value back into
// the attribute, which is how the value ends up inside Playwright's element
// description.
//
// The control is a plain text input on purpose. A role="combobox" control routes
// through commitComboboxSelection, which replaces the driver's error with a
// sentinel and so never leaked; the default path reaches
// safeFillWithLabelFallback, which wraps the raw Playwright error with %w and is
// the path that did.
const assistedCanaryForm = `<!doctype html>
<html><body>
<form>
<label for="salary">Desired salary</label>
<input id="salary" name="salary" type="text" value="" autocomplete="off">
</form>
<script>
var el = document.getElementById('salary');
el.addEventListener('input', function () { el.setAttribute('value', el.value); });
</script>
</body></html>`

// TestApplyApprovedAnswers_DoesNotLogTheValueItFailedToCommit is the real
// Playwright regression for bugs.md #543 on the producer side.
//
// It drives the shipped function, against a real browser and a real page, into
// the exact failure the bug is about: a commit that retries and times out
// against a control that already holds the operator's answer. Playwright's
// diagnostic for that failure quotes the control's outer HTML with the value
// attribute included -- verified directly while reproducing #543 -- so the
// assertion is simply that none of it reaches the log.
func TestApplyApprovedAnswers_DoesNotLogTheValueItFailedToCommit(t *testing.T) {
	if os.Getenv("CAREER_AGENT_PLAYWRIGHT_INTEGRATION") != "1" {
		t.Skip("set CAREER_AGENT_PLAYWRIGHT_INTEGRATION=1 to run the Chromium log-privacy regression")
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, assistedCanaryForm)
	}))
	defer server.Close()

	page, cleanup := openCanaryPage(t, server.URL)
	defer cleanup()

	target := resolveFillTarget(page)
	controls, err := SnapshotControls(target)
	if err != nil {
		t.Fatalf("inventory the synthetic form: %v", err)
	}
	var salary FormControl
	for _, control := range controls {
		if strings.Contains(control.Selector, "salary") || strings.Contains(strings.ToLower(control.Label), "salary") {
			salary = control
			break
		}
	}
	if salary.Key == "" {
		t.Fatalf("the synthetic form did not inventory a salary control: %+v", controls)
	}

	// The operator's answer is committed once, so the control genuinely holds
	// it, and the field is then locked underneath us. A second commit now
	// retries against a control whose value attribute carries the canary.
	if err := page.Locator("#salary").Fill(logPrivacyCanary); err != nil {
		t.Fatalf("seed the control with the canary: %v", err)
	}
	if _, err := page.Evaluate(`document.getElementById('salary').setAttribute('readonly','')`); err != nil {
		t.Fatalf("lock the control: %v", err)
	}

	captured := captureLog(t)
	report, err := ApplyApprovedAnswers(page, security.NewQuarantineLayer(), map[string]string{
		salary.Key: "a different answer",
	})
	if err != nil {
		t.Fatalf("ApplyApprovedAnswers returned a hard error: %v", err)
	}
	if len(report.Unresolved) != 1 {
		t.Fatalf("expected the locked control to come back unresolved, got %+v", report)
	}

	emitted := captured.String()
	if emitted == "" {
		t.Fatal("the failed commit logged nothing at all; the diagnostic was lost rather than made safe")
	}
	if strings.Contains(emitted, logPrivacyCanary) {
		t.Fatal("the canary reached the log from a real Playwright diagnostic")
	}
	if strings.Contains(emitted, "<input") || strings.Contains(emitted, "locator resolved to") {
		t.Fatal("element HTML reached the log from a real Playwright diagnostic")
	}
	// The diagnostic must still say something an operator can act on.
	if !strings.Contains(emitted, "reason=") {
		t.Fatalf("no bounded failure reason was recorded: %q", emitted)
	}
}

// TestComboboxCommitRecord_NamesThePositionNotTheValue pins the producer-side
// rule for the record fillComboboxFromCandidates writes when a candidate sticks.
//
// The candidates on that path are the operator's own address values, so the
// record this replaced ("Location set to %q") wrote a home city and state into
// career_agent.log. Nothing downstream caught it: security.SanitizeChildLogLine
// only strips markup and truncates, so a well-formed prose line reaches
// dashboard.log intact once cmd/assist runs a fill. The existing coverage in
// this package and in pkg/security all guards markup and driver diagnostics --
// there was no test asserting that an ordinary sentence carries no operator
// value, which is exactly the shape that got through.
//
// It asserts the record's exact text rather than searching it for a value.
// Searching would be worthless here and it is worth saying why: the helper is
// not given the candidates at all, so "the record does not contain a candidate"
// is true of every possible implementation and would pass forever. Pinning the
// whole string is what actually bites -- widening the signature to thread the
// typed value back in cannot leave this assertion standing, and whoever does it
// has to edit a test whose name says not to.
//
// The call site itself -- that this helper is what the log line is built from --
// is guarded by the Chromium test below, which is gated and does not run in a
// default `go test ./...`.
func TestComboboxCommitRecord_NamesThePositionNotTheValue(t *testing.T) {
	for _, testCase := range []struct {
		field    string
		position int
		total    int
		want     string
	}{
		{"Location", 1, 3, "Location committed on the initial fill from candidate 1 of 3 (saved a validation-retry cycle)"},
		{"Location", 2, 3, "Location committed on the initial fill from candidate 2 of 3 (saved a validation-retry cycle)"},
		{"Country", 2, 2, "Country committed on the initial fill from candidate 2 of 2 (saved a validation-retry cycle)"},
		{"Location", 1, 1, "Location committed on the initial fill from candidate 1 of 1 (saved a validation-retry cycle)"},
	} {
		if got := comboboxCommitRecord(testCase.field, testCase.position, testCase.total); got != testCase.want {
			t.Errorf("comboboxCommitRecord(%q, %d, %d)\n got: %q\nwant: %q",
				testCase.field, testCase.position, testCase.total, got, testCase.want)
		}
	}
}

// comboboxCanaryForm is a local synthetic Location autocomplete shaped like the
// react-select widget Greenhouse serves: a role="combobox" input inside a
// .select__control shell, an aria-controls listbox of [role="option"] entries,
// and a .select__single-value that the shell renders once a selection commits.
// That is what readComboboxValueJS reads, so the commit is recognised by the
// production rule.
//
// It is not a full stand-in for a live react-select and should not be read as
// one: it never sets aria-activedescendant, so waitForComboboxReady times out on
// every attempt (which is where this test's ~14s goes) and the keyboard-selection
// fork is never exercised. What is being pinned here is what reaches the log,
// not the commit mechanism -- browser_test.go covers that.
//
// Only the second option matches the second candidate below, so the first
// candidate genuinely fails and the run commits at position 2 -- which is the
// case where the position record has to carry real information.
const comboboxCanaryForm = `<!doctype html>
<html><body>
<form>
<label for="loc">Location</label>
<div class="select__control">
  <span class="select__single-value"></span>
  <input id="loc" name="loc" type="text" role="combobox" aria-controls="loc-listbox" autocomplete="off">
</div>
<div id="loc-listbox">
  <div id="loc-option-0" role="option">CA_TEST_OTHERPLACE</div>
  <div id="loc-option-1" role="option">CA_TEST_CITY, CA_TEST_STATE</div>
</div>
</form>
<script>
document.querySelectorAll('#loc-listbox [role="option"]').forEach(function (opt) {
  opt.addEventListener('click', function () {
    document.querySelector('.select__single-value').textContent = opt.textContent;
  });
});
</script>
</body></html>`

// TestFillComboboxFromCandidates_DoesNotLogTheValueItCommitted is the live
// re-verification for the address-value leak, on the producer side.
//
// The unit test above pins the record's shape; this drives the shipped function
// through a real Chromium against a real page and asserts on what actually
// reached the log. That distinction is the whole reason this file exists: the
// defect it replaces passed every unit test in the repository, because the
// value only ever appeared at the moment a selection committed against a
// browser.
func TestFillComboboxFromCandidates_DoesNotLogTheValueItCommitted(t *testing.T) {
	if os.Getenv("CAREER_AGENT_PLAYWRIGHT_INTEGRATION") != "1" {
		t.Skip("set CAREER_AGENT_PLAYWRIGHT_INTEGRATION=1 to run the Chromium log-privacy regression")
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, comboboxCanaryForm)
	}))
	defer server.Close()

	page, cleanup := openCanaryPage(t, server.URL)
	defer cleanup()

	// Stand-ins for the operator's address values, in the order the caller
	// fixes them. Synthetic: no pii.yaml value appears in this file.
	candidates := []string{"CA_TEST_UNMATCHED_CITY", "CA_TEST_CITY, CA_TEST_STATE"}

	captured := captureLog(t)
	fillComboboxFromCandidates(resolveFillTarget(page), "#loc", "Location", candidates, nil)

	emitted := captured.String()
	if emitted == "" {
		t.Fatal("the commit logged nothing at all; the diagnostic was lost rather than made safe")
	}
	for _, candidate := range candidates {
		if strings.Contains(emitted, candidate) {
			t.Fatalf("a typed address value reached the log: %q", emitted)
		}
	}
	if !strings.Contains(emitted, "candidate 2 of 2") {
		t.Fatalf("the log does not say which candidate committed: %q", emitted)
	}
}

// openCanaryPage launches Chromium and returns a page on url.
func openCanaryPage(t *testing.T, url string) (playwright.Page, func()) {
	t.Helper()
	pw, err := playwright.Run()
	if err != nil {
		t.Skipf("Playwright driver unavailable: %v", err)
	}
	browser, err := pw.Chromium.Launch(playwright.BrowserTypeLaunchOptions{Headless: playwright.Bool(true)})
	if err != nil {
		_ = pw.Stop()
		t.Skipf("Chromium unavailable: %v", err)
	}
	page, err := browser.NewPage()
	if err != nil {
		_ = browser.Close()
		_ = pw.Stop()
		t.Fatalf("new page: %v", err)
	}
	if _, err := page.Goto(url); err != nil {
		_ = browser.Close()
		_ = pw.Stop()
		t.Fatalf("open the synthetic form: %v", err)
	}
	return page, func() {
		_ = browser.Close()
		_ = pw.Stop()
	}
}

// safeBuffer collects log output written from whichever goroutine the
// automation happens to be on.
type safeBuffer struct {
	mutex   sync.Mutex
	builder strings.Builder
}

func (buffer *safeBuffer) Write(p []byte) (int, error) {
	buffer.mutex.Lock()
	defer buffer.mutex.Unlock()
	return buffer.builder.Write(p)
}

func (buffer *safeBuffer) String() string {
	buffer.mutex.Lock()
	defer buffer.mutex.Unlock()
	return buffer.builder.String()
}

func captureLog(t *testing.T) *safeBuffer {
	t.Helper()
	buffer := &safeBuffer{}
	previousWriter := log.Writer()
	previousFlags := log.Flags()
	log.SetOutput(buffer)
	t.Cleanup(func() {
		log.SetOutput(previousWriter)
		log.SetFlags(previousFlags)
	})
	return buffer
}
