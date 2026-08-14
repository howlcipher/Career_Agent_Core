package main

import (
	"bytes"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/howlcipher/Career_Agent_Core/pkg/security"
	"github.com/mxschmitt/playwright-go"
)

// logPrivacyCanary stands in for an operator's typed answer. Every use of it in
// this file is synthetic; no real answer, resume field or pii.yaml value is
// read anywhere here.
const logPrivacyCanary = "CA_TEST_SECRET_543_d7f912a4"

// captureAssistedLog runs readAssistedStderr over input with the standard
// logger pointed at a buffer, and returns everything that was persisted.
func captureAssistedLog(t *testing.T, input string) (string, bool) {
	t.Helper()
	var persisted strings.Builder
	previousWriter := log.Writer()
	previousFlags := log.Flags()
	log.SetOutput(&persisted)
	log.SetFlags(log.LstdFlags)
	t.Cleanup(func() {
		log.SetOutput(previousWriter)
		log.SetFlags(previousFlags)
	})
	readySeen := false
	readAssistedStderr(assistedLogLabel, strings.NewReader(input), func() { readySeen = true })
	return persisted.String(), readySeen
}

func TestReadAssistedStderr_KeepsCareerAgentRecords(t *testing.T) {
	persisted, _ := captureAssistedLog(t, strings.Join([]string{
		"2026/08/13 10:09:12 Assisted browser lease acquired.",
		"2026/08/13 10:09:41 Filled 12 field(s) and reused 3 approved answer(s).",
		"2026/08/13 10:11:02 Assisted application was confirmed; closing the browser.",
		"2026/08/13 10:11:03 Assisted browser closed; releasing its lease.",
	}, "\n")+"\n")

	for _, expected := range []string{
		"Assisted browser lease acquired.",
		"Filled 12 field(s) and reused 3 approved answer(s).",
		"Assisted application was confirmed; closing the browser.",
		"releasing its lease.",
	} {
		if !strings.Contains(persisted, expected) {
			t.Fatalf("an operational record was lost: %q\ngot:\n%s", expected, persisted)
		}
	}
}

// The readiness contract cmd/assist and this process share must survive the
// filter, and must be decided from the raw stream rather than from whatever
// happens to be persisted.
func TestReadAssistedStderr_PreservesTheReadinessContract(t *testing.T) {
	persisted, ready := captureAssistedLog(t,
		"2026/08/13 10:09:41 "+assistedReadySentinel+" Verified destination: application. "+
			"Complete the stated human step, then return to the dashboard and click Continue.\n")
	if !ready {
		t.Fatal("the readiness sentinel was not detected")
	}
	if !strings.Contains(persisted, assistedReadySentinel) {
		t.Fatalf("the readiness record was not persisted:\n%s", persisted)
	}
}

// Readiness is read from the raw line, so it still fires even when the record
// carrying it is one the filter refuses to persist. Control signalling and
// diagnostic persistence are separate concerns over the same bytes.
func TestReadAssistedStderr_DetectsReadinessWithoutPersistingAnUnownedLine(t *testing.T) {
	persisted, ready := captureAssistedLog(t, "  - "+assistedReadySentinel+" <div>"+logPrivacyCanary+"</div>\n")
	if !ready {
		t.Fatal("readiness must be decided from the raw stream")
	}
	if strings.Contains(persisted, logPrivacyCanary) {
		t.Fatalf("an unowned line was persisted because it carried the sentinel:\n%s", persisted)
	}
}

// The defect itself: a multiline Playwright diagnostic whose continuation lines
// quote the element, including the value the operator typed.
func TestReadAssistedStderr_DoesNotPersistPlaywrightRetryDiagnostics(t *testing.T) {
	stream := "2026/08/13 10:10:02 Your answers could not be entered automatically; " +
		"the application remains open so you can complete it yourself: reason=\"browser_timeout\"\n" +
		"playwright: timeout: Timeout 2500ms exceeded.\n" +
		"Call log:\n" +
		"  - waiting for locator('#salary')\n" +
		"    - locator resolved to <input readonly id=\"salary\" type=\"text\" value=\"" + logPrivacyCanary + "\"/>\n" +
		"    - fill(\"second attempt\")\n" +
		"  - attempting fill action\n" +
		"    2 × waiting for element to be visible, enabled and editable\n" +
		"      - element is not editable\n"

	persisted, _ := captureAssistedLog(t, stream)

	if strings.Contains(persisted, logPrivacyCanary) {
		t.Fatalf("the canary was persisted:\n%s", persisted)
	}
	if strings.Contains(persisted, "<input") {
		t.Fatalf("element HTML was persisted:\n%s", persisted)
	}
	if strings.Contains(persisted, "locator resolved to") {
		t.Fatalf("a Playwright continuation line was persisted:\n%s", persisted)
	}
	// The failure still has to be diagnosable.
	if !strings.Contains(persisted, "Your answers could not be entered automatically") {
		t.Fatalf("the safe failure record was lost:\n%s", persisted)
	}
	if !strings.Contains(persisted, "browser_timeout") {
		t.Fatalf("the bounded failure reason was lost:\n%s", persisted)
	}
	if !strings.Contains(persisted, "withheld") {
		t.Fatalf("the withheld-line count was not reported:\n%s", persisted)
	}
}

// Arbitrary third-party stderr, including a Chromium build writing to the
// descriptor cmd/assist inherits, must not be persisted on any line.
func TestReadAssistedStderr_DoesNotPersistArbitraryThirdPartyOutput(t *testing.T) {
	persisted, _ := captureAssistedLog(t, strings.Join([]string{
		"[0813/101002.482913:ERROR:gpu_process_host.cc(993)] GPU process exited",
		"ALSA lib pcm_dmix.c:1032:(snd_pcm_dmix_open) unable to open slave",
		"Fontconfig warning: ignoring UTF-8: not a valid region tag",
		`{"level":"error","msg":"<input value=\"` + logPrivacyCanary + `\">"}`,
		"2026/08/13 10:10:05 Assisted browser closed; releasing its lease.",
	}, "\n")+"\n")

	if strings.Contains(persisted, logPrivacyCanary) {
		t.Fatalf("the canary was persisted:\n%s", persisted)
	}
	for _, unwanted := range []string{"gpu_process_host", "ALSA lib", "Fontconfig"} {
		if strings.Contains(persisted, unwanted) {
			t.Fatalf("third-party output %q was persisted:\n%s", unwanted, persisted)
		}
	}
	if !strings.Contains(persisted, "releasing its lease") {
		t.Fatalf("the surrounding operational record was lost:\n%s", persisted)
	}
}

// A producer-side regression -- an application record that renders a raw
// Playwright error with %v -- puts markup on a line the prefix rule accepts.
// The second pass is what stops that becoming a leak.
func TestReadAssistedStderr_RedactsMarkupOnAnOwnedRecord(t *testing.T) {
	persisted, _ := captureAssistedLog(t,
		"2026/08/13 10:10:02 Assisted refill stopped safely: locator resolved to "+
			"<input value=\""+logPrivacyCanary+"\"/> and could not be filled\n")

	if strings.Contains(persisted, logPrivacyCanary) {
		t.Fatalf("the canary survived on an owned record:\n%s", persisted)
	}
	if !strings.Contains(persisted, "Assisted refill stopped safely") {
		t.Fatalf("the operational message was lost:\n%s", persisted)
	}
	if !strings.Contains(persisted, security.RedactedMarkupMarker) {
		t.Fatalf("redaction was not marked:\n%s", persisted)
	}
}

// An employer's page controls how long a diagnostic is. bufio.Scanner fails the
// whole stream on an over-long token, which would stop the read that keeps the
// child's pipe draining; this asserts the reader consumes the stream instead.
func TestReadAssistedStderr_BoundsAnOverlongLineAndKeepsReading(t *testing.T) {
	huge := "  - locator resolved to <input value=\"" + logPrivacyCanary + "\" data-x=\"" +
		strings.Repeat("A", 4*1024*1024) + "\"/>"
	stream := "2026/08/13 10:09:41 " + assistedReadySentinel + "\n" +
		huge + "\n" +
		"2026/08/13 10:10:05 Assisted browser closed; releasing its lease.\n"

	done := make(chan struct{})
	var persisted string
	var ready bool
	go func() {
		defer close(done)
		persisted, ready = captureAssistedLog(t, stream)
	}()
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("readAssistedStderr did not finish; an over-long line stalled the reader")
	}

	if !ready {
		t.Fatal("readiness was lost")
	}
	if strings.Contains(persisted, logPrivacyCanary) {
		t.Fatalf("the canary was persisted from an over-long line:\n%s", persisted)
	}
	if !strings.Contains(persisted, "releasing its lease") {
		t.Fatal("the reader stopped after the over-long line instead of continuing")
	}
}

// An owned record that is itself over-long is bounded rather than dropped.
func TestReadAssistedStderr_BoundsAnOverlongOwnedRecord(t *testing.T) {
	persisted, _ := captureAssistedLog(t,
		"2026/08/13 10:09:41 Assisted refill stopped safely: "+strings.Repeat("z", 200_000)+"\n")
	if !strings.Contains(persisted, "Assisted refill stopped safely") {
		t.Fatalf("an over-long owned record was dropped entirely:\n%s", persisted[:min(400, len(persisted))])
	}
	if len(persisted) > 4*security.MaxPersistedChildLogLine {
		t.Fatalf("an over-long owned record was not bounded: %d bytes persisted", len(persisted))
	}
}

func TestReadAssistedStderr_HandlesCRLFAndAMissingFinalNewline(t *testing.T) {
	persisted, ready := captureAssistedLog(t,
		"2026/08/13 10:09:41 "+assistedReadySentinel+"\r\n"+
			"  - locator resolved to <input value=\""+logPrivacyCanary+"\"/>\r\n"+
			"2026/08/13 10:10:05 Assisted browser closed; releasing its lease.")

	if !ready {
		t.Fatal("readiness was lost across CRLF")
	}
	if strings.Contains(persisted, logPrivacyCanary) {
		t.Fatalf("the canary was persisted across CRLF:\n%s", persisted)
	}
	if !strings.Contains(persisted, "releasing its lease") {
		t.Fatalf("a final line without a trailing newline was dropped:\n%s", persisted)
	}
	if strings.Contains(persisted, "\r") {
		t.Fatalf("a carriage return was persisted:\n%q", persisted)
	}
}

// --- Real Playwright, real child process, real logging boundary -------------

// assistChildEnv makes this test binary re-execute itself as a stand-in for
// cmd/assist. The parent then reads its stderr through the shipped boundary, so
// the assertion covers a genuine cross-process stream rather than a string.
const assistChildEnv = "CAREER_AGENT_ASSIST_LOG_CHILD"

const assistedCanaryForm = `<!doctype html>
<html><body><form>
<label for="salary">Desired salary</label>
<input id="salary" name="salary" type="text" role="combobox" value="" autocomplete="off">
</form>
<script>
var el = document.getElementById('salary');
el.addEventListener('input', function () { el.setAttribute('value', el.value); });
</script></body></html>`

// TestAssistedBrowserLogging_RealPlaywrightDiagnosticNeverReachesTheLogFile is
// the proof that closes bugs.md #543.
//
// A child process opens a localhost synthetic form in a real Chromium, types
// the canary into a control, provokes a real Playwright retry failure against
// that filled control, and writes to stderr the way cmd/assist does. The parent
// reads that stderr through readAssistedStderr into a real log file on disk,
// then reads the file back.
//
// The child deliberately emits three things: the record the shipped code now
// produces, a record rendered the old way (%v) to stand for a producer-side
// regression, and raw untimestamped browser output. All three carry the same
// real diagnostic, and none of them may put it in the file.
func TestAssistedBrowserLogging_RealPlaywrightDiagnosticNeverReachesTheLogFile(t *testing.T) {
	if os.Getenv(assistChildEnv) == "1" {
		runAssistLogChild(t)
		return
	}
	if os.Getenv("CAREER_AGENT_PLAYWRIGHT_INTEGRATION") != "1" {
		t.Skip("set CAREER_AGENT_PLAYWRIGHT_INTEGRATION=1 to run the Chromium logging-boundary regression")
	}

	logPath := filepath.Join(t.TempDir(), "dashboard-test.log")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		t.Fatalf("open the test log sink: %v", err)
	}

	child := exec.Command(os.Args[0], "-test.run="+t.Name(), "-test.v")
	child.Env = append(os.Environ(), assistChildEnv+"=1")
	child.Stdout = nil
	stderr, err := child.StderrPipe()
	if err != nil {
		t.Fatalf("child stderr: %v", err)
	}
	if err := child.Start(); err != nil {
		t.Fatalf("start the child: %v", err)
	}

	previousWriter := log.Writer()
	previousFlags := log.Flags()
	log.SetOutput(logFile)
	log.SetFlags(log.LstdFlags)

	ready := false
	var once sync.Once
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		readAssistedStderr(assistedLogLabel, stderr, func() { once.Do(func() { ready = true }) })
	}()

	select {
	case <-finished:
	case <-time.After(5 * time.Minute):
		_ = child.Process.Kill()
		t.Fatal("the child's stderr never reached end of stream")
	}
	waitErr := child.Wait()

	log.SetOutput(previousWriter)
	log.SetFlags(previousFlags)
	_ = logFile.Close()

	if waitErr != nil {
		contents, _ := os.ReadFile(logPath)
		t.Fatalf("the child failed: %v\npersisted log:\n%s", waitErr, contents)
	}

	raw, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read the test log sink: %v", err)
	}
	persisted := string(raw)

	if strings.Contains(persisted, logPrivacyCanary) {
		t.Fatal("a real Playwright diagnostic put the canary in the persisted log")
	}
	if strings.Contains(persisted, "<input") || strings.Contains(persisted, "locator resolved to") {
		t.Fatalf("element HTML reached the persisted log:\n%s", persisted)
	}
	if !ready {
		t.Fatal("the readiness sentinel was not detected across the real process boundary")
	}
	if !strings.Contains(persisted, assistedReadySentinel) {
		t.Fatalf("the readiness record was not persisted:\n%s", persisted)
	}
	for _, expected := range []string{
		"Your answers could not be entered automatically",
		"reason=",
		"Assisted browser closed",
	} {
		if !strings.Contains(persisted, expected) {
			t.Fatalf("a safe operational record was lost: %q\n%s", expected, persisted)
		}
	}
	if !strings.Contains(persisted, "withheld") {
		t.Fatalf("third-party output was not accounted for:\n%s", persisted)
	}
	// Nothing beside the sink may hold the raw stream.
	entries, _ := os.ReadDir(filepath.Dir(logPath))
	if len(entries) != 1 {
		t.Fatalf("the boundary created files beyond the log sink: %v", entries)
	}
}

// runAssistLogChild is the child half of the test above. It is a stand-in for
// cmd/assist: same standard logger, same default flags, same readiness line.
func runAssistLogChild(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, assistedCanaryForm)
	}))
	defer server.Close()

	pw, err := playwright.Run()
	if err != nil {
		t.Fatalf("start Playwright: %v", err)
	}
	defer pw.Stop()
	browser, err := pw.Chromium.Launch(playwright.BrowserTypeLaunchOptions{Headless: playwright.Bool(true)})
	if err != nil {
		t.Fatalf("launch Chromium: %v", err)
	}
	defer browser.Close()
	page, err := browser.NewPage()
	if err != nil {
		t.Fatalf("new page: %v", err)
	}
	if _, err := page.Goto(server.URL); err != nil {
		t.Fatalf("open the synthetic form: %v", err)
	}

	log.Print(assistedReadySentinel + " Verified destination: application. Complete the stated human step, then return to the dashboard and click Continue.")

	salary := page.Locator("#salary")
	if err := salary.Fill(logPrivacyCanary); err != nil {
		t.Fatalf("type the canary: %v", err)
	}
	if _, err := page.Evaluate(`document.getElementById('salary').setAttribute('readonly','')`); err != nil {
		t.Fatalf("lock the control: %v", err)
	}
	fillErr := salary.Fill("a different answer", playwright.LocatorFillOptions{Timeout: playwright.Float(2500)})
	if fillErr == nil {
		t.Fatal("the retry did not fail; the reproduction is no longer provoking one")
	}

	// 1. What the shipped producer now writes.
	log.Printf("Your answers could not be entered automatically; the application remains open so you can complete it yourself: reason=%q",
		security.BrowserFailureReason(fillErr))
	// 2. A producer-side regression, rendering the same real error the old way.
	log.Printf("regression stand-in for an unfixed producer: %v", fillErr)
	// 3. Raw third-party output on the same descriptor, as Chromium writes it.
	fmt.Fprintf(os.Stderr, "[0813/101002.482913:ERROR:renderer.cc(77)] <input value=\"%s\">\n", logPrivacyCanary)

	log.Print("Assisted browser closed; releasing its lease.")
}

func TestReadAssistedStderr_NamesWhichChildProducedALine(t *testing.T) {
	// Two children now share this reader. Reporting a preparation run's output
	// as "assisted browser:" would tell an operator reading dashboard.log that a
	// visible browser was open on an application when none was.
	var persisted bytes.Buffer
	previousWriter := log.Writer()
	previousFlags := log.Flags()
	log.SetOutput(&persisted)
	log.SetFlags(0)
	t.Cleanup(func() {
		log.SetOutput(previousWriter)
		log.SetFlags(previousFlags)
	})

	record := "2026/08/13 21:29:16 Preflight inspected one application: fields=28 questions=12\n"
	readAssistedStderr(preflightLogLabel, strings.NewReader(record), func() {})

	got := persisted.String()
	if !strings.Contains(got, "preparation: Preflight inspected") {
		t.Fatalf("expected the preparation label, got %q", got)
	}
	if strings.Contains(got, "assisted browser") {
		t.Fatalf("a preparation run was labelled as an assisted browser: %q", got)
	}
}
