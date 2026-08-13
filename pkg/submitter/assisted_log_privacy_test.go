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
