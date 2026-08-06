//go:build ignore

// verify_assisted_prefill drives FillAssistedMappedPage against a real, live
// ATS posting and reports what actually landed in the form, without ever
// submitting it.
//
// It exists for bugs.md #519's shipping condition: the dedicated Greenhouse /
// Lever / Ashby handlers were written for the automatic path, where they also
// click Submit, and #519 routes Assisted Apply through those same handlers.
// A regression there would press Submit on the operator's behalf inside the
// visible browser, which is the one thing assisted mode must never do. Unit
// tests prove the copilot gate stops each handler against mocks; this proves
// it against a real employer's DOM.
//
// Synthetic identity only — it never reads pii.yaml, so a stray submission
// could not send the operator's real details. The posting's URL before and
// after the fill is printed: an unchanged URL with no confirmation text is
// the evidence that nothing was submitted.
//
//	go run scripts/verify_assisted_prefill.go <apply-url> [<apply-url>...]
package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/howlcipher/Career_Agent_Core/pkg/config"
	"github.com/howlcipher/Career_Agent_Core/pkg/security"
	"github.com/howlcipher/Career_Agent_Core/pkg/submitter"
	"github.com/mxschmitt/playwright-go"
)

// readbackScript reports every filled value and the page's submit controls,
// so the caller can see both that the prefill worked and that no submit
// control was activated.
const readbackScript = `() => {
  const filled = [];
  for (const el of document.querySelectorAll('input, textarea, select')) {
    if (el.type === 'hidden') continue;
    if (el.type === 'file') {
      const attached = el.files && el.files.length > 0 ? el.files[0].name : 'EMPTY';
      filled.push('file[name=' + (el.name || '') + ',id=' + (el.id || '') + '] = ' + attached);
      continue;
    }
    if (el.value) filled.push((el.name || el.id || el.type) + ' = ' + el.value);
  }
  const submits = document.querySelectorAll("input#submit_app, button[type='submit'], button.postings-btn.template-btn-submit");
  return JSON.stringify({filled: filled, submitControls: submits.length, body: document.body.innerText.slice(0, 400)});
}`

// confirmationPhrases are what an ATS shows once an application really was
// submitted. Seeing one here would mean the copilot gate failed.
var confirmationPhrases = []string{
	"thank you for applying",
	"application submitted",
	"we have received your application",
	"thanks for applying",
}

func main() {
	if len(os.Args) < 2 {
		log.Fatal("usage: go run scripts/verify_assisted_prefill.go <apply-url> [<apply-url>...]")
	}

	workspace, err := os.MkdirTemp("", "assisted-prefill-*")
	if err != nil {
		log.Fatalf("temp workspace: %v", err)
	}
	defer os.RemoveAll(workspace)
	resumePath := filepath.Join(workspace, "resume.pdf")
	coverPath := filepath.Join(workspace, "coverletter.txt")
	if err := os.WriteFile(resumePath, []byte("%PDF-1.4 synthetic verification resume"), 0o600); err != nil {
		log.Fatalf("write resume: %v", err)
	}
	if err := os.WriteFile(coverPath, []byte("Dear hiring team, this is a synthetic verification letter."), 0o600); err != nil {
		log.Fatalf("write cover letter: %v", err)
	}

	pw, err := playwright.Run()
	if err != nil {
		log.Fatalf("start playwright: %v", err)
	}
	defer pw.Stop()
	browser, err := pw.Chromium.Launch(playwright.BrowserTypeLaunchOptions{
		Headless: playwright.Bool(true),
		Args:     []string{"--no-sandbox"},
	})
	if err != nil {
		log.Fatalf("launch chromium: %v", err)
	}
	defer browser.Close()

	failures := 0
	for _, applyURL := range os.Args[1:] {
		if !verifyOne(browser, applyURL, resumePath, coverPath) {
			failures++
		}
	}
	fmt.Printf("\n%d of %d posting(s) verified clean.\n", len(os.Args[1:])-failures, len(os.Args[1:]))
	if failures > 0 {
		os.Exit(1)
	}
}

func verifyOne(browser playwright.Browser, applyURL, resumePath, coverPath string) bool {
	fmt.Printf("\n=== %s ===\n", applyURL)
	browserContext, err := browser.NewContext()
	if err != nil {
		fmt.Printf("  SKIP: new context: %v\n", err)
		return false
	}
	defer browserContext.Close()
	page, err := browserContext.NewPage()
	if err != nil {
		fmt.Printf("  SKIP: new page: %v\n", err)
		return false
	}
	if _, err := page.Goto(applyURL, playwright.PageGotoOptions{
		WaitUntil: playwright.WaitUntilStateDomcontentloaded,
		Timeout:   playwright.Float(45000),
	}); err != nil {
		fmt.Printf("  SKIP: navigation failed: %v\n", err)
		return false
	}
	page.WaitForTimeout(2000)

	destinationPage, destination, err := submitter.ReachAssistedDestination(page)
	if err != nil {
		fmt.Printf("  SKIP: no assisted destination (posting likely expired): %v\n", err)
		return false
	}
	page = destinationPage
	fmt.Printf("  destination: %s\n", destination)

	urlBefore := page.URL()
	fillErr := submitter.FillAssistedMappedPage(
		page,
		security.NewQuarantineLayer(),
		"Verification Run",
		applyURL,
		resumePath,
		coverPath,
		verificationPII(),
	)
	urlAfter := page.URL()

	raw, evalErr := page.Evaluate(readbackScript)
	readback := ""
	if evalErr == nil {
		readback, _ = raw.(string)
	}

	fmt.Printf("  fill result: %v\n", fillErr)
	fmt.Printf("  url before:  %s\n  url after:   %s\n", urlBefore, urlAfter)
	fmt.Printf("  readback:    %s\n", readback)

	clean := true
	if urlBefore != urlAfter {
		fmt.Printf("  FAIL: the page navigated during the fill — a submit click is the likeliest cause\n")
		clean = false
	}
	lowered := strings.ToLower(readback)
	for _, phrase := range confirmationPhrases {
		if strings.Contains(lowered, phrase) {
			fmt.Printf("  FAIL: page shows submission confirmation %q — the copilot gate did not hold\n", phrase)
			clean = false
		}
	}
	if fillErr == nil && clean {
		fmt.Printf("  OK: form prefilled and left unsubmitted for the operator\n")
	}
	return clean
}

// verificationPII is deliberately synthetic. This script fills real employer
// forms, so nothing it types may be the operator's actual details.
func verificationPII() *config.PII {
	return &config.PII{
		FirstName:   "Verification",
		LastName:    "Testrun",
		Email:       "verification.testrun@example.invalid",
		Phone:       "555-0100",
		City:        "Detroit",
		State:       "MI",
		FullState:   "Michigan",
		Country:     "US",
		FullCountry: "United States",
	}
}
