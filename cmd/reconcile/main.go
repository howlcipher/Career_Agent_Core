// Command reconcile reads the user's hand-off checklists and records the
// applications they have ticked off as genuinely applied.
//
// Bug #434: when the agent cannot submit an application itself it hands the
// job to the user — an ATS account gate (MANUAL_REQUIRED) or a deliberate
// pre-submit stop (AWAITING_REVIEW, copilot mode) — writes the tailored
// documents, and appends a checklist entry. Nothing ever moved a row back out
// of those statuses, so every application the user completed by hand stayed
// recorded as un-submitted forever, and the tracker dropped the rejection or
// interview email it eventually produced.
//
// The checklists are already the user's record of what they sent. This command
// reads the ticked boxes back into the funnel.
//
// Like cmd/requeue, it is a dry run by default and requires -confirm before it
// writes anything.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/howlcipher/Career_Agent_Core/pkg/security"
	"github.com/howlcipher/Career_Agent_Core/pkg/storage"
)

// checklistFiles are every file the agent writes hand-off entries into.
// Kept in sync with LogFailedSubmission, LogManualRequired and
// LogCopilotReview in pkg/storage/manager.go, which all share one entry
// format. A missing file is normal — it just means that path has never fired.
var checklistFiles = []string{
	filepath.Join("applications", "manual_submissions.md"),
	filepath.Join("applications", "needs_manual_apply", "manual_queue.md"),
	filepath.Join("applications", "needs_manual_apply", "copilot_queue.md"),
}

// entry is one ticked checklist line.
type entry struct {
	Company string
	Title   string
	URL     string
}

// entryPattern matches the checklist format the three logger functions write:
//
//	- [ ] **Acme Corp** - SRE: [Apply Here](https://example.com/1)
//	- [x] **Acme Corp** - SRE: [Apply Here](https://example.com/1) — docs in `path/`
//
// Company and title are non-greedy so a title containing " - " or a company
// containing "**"-adjacent punctuation cannot swallow the rest of the line,
// and the trailing docs note is ignored entirely.
var entryPattern = regexp.MustCompile(`^\s*-\s*\[([ xX])\]\s*\*\*(.+?)\*\*\s*-\s*(.+?):\s*\[[^\]]*\]\(([^)]+)\)`)

// parseChecklistLine returns the entry described by a checklist line and
// whether its box is ticked. ok is false for anything that is not a checklist
// entry at all — headers, blank lines, prose, malformed rows. Callers must
// treat a non-ticked entry as "the user has not sent this yet", not as an
// error: the unticked rows are the working queue.
func parseChecklistLine(line string) (e entry, ticked bool, ok bool) {
	m := entryPattern.FindStringSubmatch(line)
	if m == nil {
		return entry{}, false, false
	}
	e = entry{
		Company: strings.TrimSpace(m[2]),
		Title:   strings.TrimSpace(m[3]),
		URL:     strings.TrimSpace(m[4]),
	}
	if e.URL == "" {
		return entry{}, false, false
	}
	return e, m[1] != " ", true
}

// parseChecklist reads one checklist file and returns only its ticked entries.
// A missing file yields no entries and no error.
func parseChecklist(path string) ([]entry, error) {
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var ticked []entry
	scanner := bufio.NewScanner(f)
	// Checklist lines are short, but a docs path can be long; give the scanner
	// room so an oversized line is skipped rather than aborting the file.
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		e, isTicked, ok := parseChecklistLine(scanner.Text())
		if ok && isTicked {
			ticked = append(ticked, e)
		}
	}
	if err := scanner.Err(); err != nil {
		return ticked, err
	}
	return ticked, nil
}

func main() {
	if err := security.PreparePrivateWorkspace(".", os.Stderr); err != nil {
		log.Fatalf("Startup aborted because private paths could not be secured: %v", err)
	}

	confirm := flag.Bool("confirm", false, "actually record the ticked applications; without this, only a dry-run report is printed")
	flag.Parse()

	if err := storage.InitDB(); err != nil {
		log.Fatalf("Failed to initialize SQLite database: %v", err)
	}
	defer storage.CloseDB()

	var found, promoted, skipped int

	for _, path := range checklistFiles {
		entries, err := parseChecklist(path)
		if err != nil {
			log.Printf("WARNING: could not fully read %s: %v", path, err)
		}
		if len(entries) == 0 {
			continue
		}
		fmt.Printf("\n%s — %d ticked\n", path, len(entries))
		for _, e := range entries {
			found++
			if !*confirm {
				fmt.Printf("  would record: %s — %s (%s)\n", e.Company, e.Title, e.URL)
				continue
			}
			ok, err := storage.MarkHandoffApplied(e.Company, e.Title, e.URL)
			if err != nil {
				log.Printf("  ERROR recording %s (%s): %v", e.Company, e.URL, err)
				skipped++
				continue
			}
			if ok {
				promoted++
				fmt.Printf("  recorded: %s — %s (%s)\n", e.Company, e.Title, e.URL)
			} else {
				// Either no funnel row for that URL, or the row has already
				// moved past the hand-off stage — an earlier reconcile run, or
				// a real outcome the tracker has since recorded. Both are
				// normal and neither should be overwritten.
				skipped++
				fmt.Printf("  skipped (no hand-off row): %s — %s (%s)\n", e.Company, e.Title, e.URL)
			}
		}
	}

	fmt.Println()
	if !*confirm {
		fmt.Printf("Dry run: %d ticked entries found across %d checklists. Nothing was written.\n", found, len(checklistFiles))
		if found > 0 {
			fmt.Println("Re-run with -confirm to record them as applied.")
		}
		return
	}
	fmt.Printf("Recorded %d application(s); skipped %d; %d ticked entries examined.\n", promoted, skipped, found)
}
