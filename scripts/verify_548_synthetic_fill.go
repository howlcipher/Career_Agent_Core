//go:build ignore

// Synthetic real-fill verification for bugs.md #548.
//
// The unit tests stub the fill. This does not: it drives a real Chromium page
// through the real submitter.FillAssistedMappedPage, in the same order
// cmd/assist does it -- MarkFillAttempted, then the fill, then the summary
// write -- against a throwaway database, and reads the result back through
// GetFillSummary.
//
// Two cases, because #548 has two halves. A form Career Agent can fill must
// record an attempt *and* work; a form with nothing fillable on it must record
// an attempt and no work, which is the only state that entitles the card to
// say the attempt completed nothing.
//
// It touches nothing real: the forms are generated into a temporary directory,
// the database is a throwaway, and no employer server is contacted.
//
// Usage, from the repository root, inside the career-agent container:
//
//	go run scripts/verify_548_synthetic_fill.go
package main

import (
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"time"

	"github.com/howlcipher/Career_Agent_Core/pkg/config"
	"github.com/howlcipher/Career_Agent_Core/pkg/security"
	"github.com/howlcipher/Career_Agent_Core/pkg/storage"
	"github.com/howlcipher/Career_Agent_Core/pkg/submitter"
	"github.com/mxschmitt/playwright-go"
)

// The forms are served over loopback HTTP rather than opened as file:// URLs
// so the fill path sees an ordinary domain: ExtractDomain has to produce a key
// that the cached-mapping lookup can use, which is the route every non-
// dedicated ATS takes in production.

func main() {
	dir, err := os.MkdirTemp("", "verify548")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(dir)

	forms := filepath.Join(dir, "forms")
	if err := os.MkdirAll(forms, 0o755); err != nil {
		log.Fatal(err)
	}
	for name, body := range map[string]string{
		// A perfectly ordinary application form.
		"fillable.html": `<!doctype html><html><head><title>Synthetic Application</title></head><body>
<h1>Apply for Platform Engineer</h1>
<form id="application_form">
  <label for="first_name">First Name</label><input type="text" id="first_name" name="first_name">
  <label for="last_name">Last Name</label><input type="text" id="last_name" name="last_name">
  <label for="email">Email</label><input type="email" id="email" name="email">
  <label for="phone">Phone</label><input type="tel" id="phone" name="phone">
  <button type="submit" id="submit_app">Submit Application</button>
</form></body></html>`,
		// A form with none of the controls a fill knows how to complete. This
		// is the case that produces a genuine zero-result attempt.
		"empty.html": `<!doctype html><html><head><title>Synthetic Application</title></head><body>
<h1>Apply for Platform Engineer</h1>
<p>This posting collects nothing beyond your consent.</p>
<form id="application_form">
  <button type="submit" id="submit_app">Submit Application</button>
</form></body></html>`,
	} {
		if err := os.WriteFile(filepath.Join(forms, name), []byte(body), 0o644); err != nil {
			log.Fatal(err)
		}
	}

	if err := storage.InitDBWithPath(filepath.Join(dir, "synthetic.db")); err != nil {
		log.Fatalf("init database: %v", err)
	}
	db := storage.GetDB()

	pw, err := playwright.Run()
	if err != nil {
		log.Fatalf("start playwright: %v", err)
	}
	defer pw.Stop()
	headless := true
	browser, err := pw.Chromium.Launch(playwright.BrowserTypeLaunchOptions{Headless: &headless})
	if err != nil {
		log.Fatalf("launch chromium: %v", err)
	}
	defer browser.Close()

	// Synthetic values only. Nothing here is the operator's, and nothing is
	// written anywhere but a throwaway database in a temporary directory.
	pii := &config.PII{
		FirstName: "Ada",
		LastName:  "Lovelace",
		Email:     "ada@example.com",
		Phone:     "555-0100",
	}

	server := httptest.NewServer(http.FileServer(http.Dir(forms)))
	defer server.Close()

	failures := 0
	for _, testCase := range []struct {
		name      string
		file      string
		jobID     string
		wantFills bool
	}{
		{"a form Career Agent can fill", "fillable.html", "9001", true},
		{"a form with nothing fillable on it", "empty.html", "9002", false},
	} {
		url := server.URL + "/" + testCase.file

		// Seed the cached mapping the generic handler reuses. A dedicated ATS
		// handler would supply this itself; a synthetic domain has no learned
		// mapping, and learning one would put a model call in the middle of a
		// verification about database provenance.
		mapping := `{"fields":{"first_name":"#first_name","last_name":"#last_name","email":"#email","phone":"#phone"},` +
			`"labels":{"first_name":"First Name","last_name":"Last Name","email":"Email","phone":"Phone"},"answers":{}}`
		if err := storage.SaveFormMapping(submitter.ExtractDomain(url), mapping); err != nil {
			log.Fatalf("seed form mapping: %v", err)
		}

		page, err := browser.NewPage()
		if err != nil {
			log.Fatalf("new page: %v", err)
		}
		if _, err := page.Goto(url); err != nil {
			log.Fatalf("open %s: %v", url, err)
		}

		// Exactly cmd/assist's order. The marker goes down before the fill so
		// that an outcome which never reports still leaves a true record.
		if err := storage.MarkFillAttempted(db, testCase.jobID, time.Now()); err != nil {
			log.Fatalf("mark fill attempted: %v", err)
		}

		report, fillErr := submitter.FillAssistedMappedPage(submitter.AssistedFillPlan{
			Page:        page,
			Filter:      security.NewQuarantineLayer(),
			CompanyName: "Synthetic Co",
			ApplyURL:    url,
			PII:         pii,
		})
		if fillErr == nil {
			summary := storage.AssistedFillSummary{
				JobID:         testCase.jobID,
				FilledCount:   report.FilledCount(),
				ReusedAnswers: report.ReusedAnswers,
				Documents:     report.Documents,
			}
			if err := storage.ReplaceApplicationQuestions(db, testCase.jobID, nil, summary); err != nil {
				log.Fatalf("record summary: %v", err)
			}
		}
		page.Close()

		stored, err := storage.GetFillSummary(db, testCase.jobID)
		if err != nil {
			log.Fatalf("read summary: %v", err)
		}

		fmt.Printf("\n== %s ==\n", testCase.name)
		fmt.Printf("   fill error       : %v\n", fillErr)
		fmt.Printf("   fields filled    : %d\n", stored.FilledCount)
		fmt.Printf("   fill_attempted_at: %q\n", stored.FillAttemptedAt)
		fmt.Printf("   recorded_at      : %q\n", stored.RecordedAt)

		if stored.FillAttemptedAt == "" {
			fmt.Println("   FAIL: a real fill ran and recorded no attempt")
			failures++
			continue
		}
		if testCase.wantFills && stored.FilledCount == 0 {
			fmt.Println("   FAIL: expected this form to be filled")
			failures++
			continue
		}
		if !testCase.wantFills && stored.FilledCount != 0 {
			fmt.Printf("   FAIL: expected no work, got %d fields\n", stored.FilledCount)
			failures++
			continue
		}
		if testCase.wantFills {
			fmt.Println("   OK: attempt recorded, and the card reports the work it did")
		} else {
			fmt.Println("   OK: attempt recorded with zero work -- the card may truthfully say the attempt completed nothing")
		}
	}

	// And the control: preparation over the same table must not produce a
	// marker, which is the defect itself.
	if err := storage.RecordPreparedQuestions(db, "9003", []storage.ApplicationQuestion{
		{JobID: "9003", Key: "notice", Prompt: "Notice period?", ControlType: "text"},
	}); err != nil {
		log.Fatalf("record prepared questions: %v", err)
	}
	control, err := storage.GetFillSummary(db, "9003")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("\n== control: preparation only ==\n")
	fmt.Printf("   recorded_at      : %q\n", control.RecordedAt)
	fmt.Printf("   fill_attempted_at: %q\n", control.FillAttemptedAt)
	if control.FillAttemptedAt != "" {
		fmt.Println("   FAIL: preparation recorded a fill attempt")
		failures++
	} else if control.RecordedAt == "" {
		fmt.Println("   FAIL: preparation wrote no row at all; the field-count fallback would break")
		failures++
	} else {
		fmt.Println("   OK: the row exists and claims no fill")
	}

	if failures > 0 {
		fmt.Printf("\n%d check(s) failed\n", failures)
		os.Exit(1)
	}
	fmt.Print("\nAll synthetic fill-provenance checks passed\n")
}
