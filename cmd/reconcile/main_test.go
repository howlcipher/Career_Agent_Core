package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestParseChecklistLine covers the exact format the three logger functions in
// pkg/storage/manager.go write, plus the lines that surround them in a real
// checklist file. The unticked case is the important negative: those rows are
// the user's working queue, and recording one as applied would mark a job the
// user has not sent.
func TestParseChecklistLine(t *testing.T) {
	for _, tc := range []struct {
		name        string
		line        string
		wantOK      bool
		wantTicked  bool
		wantCompany string
		wantTitle   string
		wantURL     string
	}{
		{
			name:        "unticked entry is parsed but not ticked",
			line:        "- [ ] **Acme Corp** - SRE: [Apply Here](https://jobs.example.com/1)",
			wantOK:      true,
			wantTicked:  false,
			wantCompany: "Acme Corp",
			wantTitle:   "SRE",
			wantURL:     "https://jobs.example.com/1",
		},
		{
			name:        "lowercase x is ticked",
			line:        "- [x] **Acme Corp** - SRE: [Apply Here](https://jobs.example.com/1)",
			wantOK:      true,
			wantTicked:  true,
			wantCompany: "Acme Corp",
			wantTitle:   "SRE",
			wantURL:     "https://jobs.example.com/1",
		},
		{
			name:        "uppercase X is ticked",
			line:        "- [X] **Acme Corp** - SRE: [Apply Here](https://jobs.example.com/1)",
			wantOK:      true,
			wantTicked:  true,
			wantCompany: "Acme Corp",
			wantTitle:   "SRE",
			wantURL:     "https://jobs.example.com/1",
		},
		{
			// LogManualRequired and LogCopilotReview append this; the URL must
			// not absorb it and the docs path must not be mistaken for a field.
			name:        "trailing docs note is ignored",
			line:        "- [x] **Acme Corp** - SRE: [Apply Here](https://jobs.example.com/1) — docs in `applications/needs_manual_apply/Acme_Corp/`",
			wantOK:      true,
			wantTicked:  true,
			wantCompany: "Acme Corp",
			wantTitle:   "SRE",
			wantURL:     "https://jobs.example.com/1",
		},
		{
			name:        "docs-not-found note is ignored",
			line:        "- [x] **Acme Corp** - SRE: [Apply Here](https://jobs.example.com/1) — docs not found",
			wantOK:      true,
			wantTicked:  true,
			wantCompany: "Acme Corp",
			wantTitle:   "SRE",
			wantURL:     "https://jobs.example.com/1",
		},
		{
			// Real company and role names are not single tokens.
			name:        "punctuation and spaces in company and title",
			line:        "- [x] **Foo, Bar & Baz, Inc.** - Sr. Site Reliability Engineer (Remote - US): [Apply Here](https://jobs.example.com/a-b_c?q=1&r=2)",
			wantOK:      true,
			wantTicked:  true,
			wantCompany: "Foo, Bar & Baz, Inc.",
			wantTitle:   "Sr. Site Reliability Engineer (Remote - US)",
			wantURL:     "https://jobs.example.com/a-b_c?q=1&r=2",
		},
		{
			name:        "hyphenated title does not truncate at the separator",
			line:        "- [x] **Acme** - Platform - Infrastructure Engineer: [Apply Here](https://jobs.example.com/2)",
			wantOK:      true,
			wantTicked:  true,
			wantCompany: "Acme",
			wantTitle:   "Platform - Infrastructure Engineer",
			wantURL:     "https://jobs.example.com/2",
		},
		{
			name:        "indented entry still parses",
			line:        "   - [x]  **Acme Corp**  -  SRE:  [Apply Here](https://jobs.example.com/1)",
			wantOK:      true,
			wantTicked:  true,
			wantCompany: "Acme Corp",
			wantTitle:   "SRE",
			wantURL:     "https://jobs.example.com/1",
		},
		{name: "markdown heading", line: "# Copilot Review Queue", wantOK: false},
		{name: "blank line", line: "", wantOK: false},
		{name: "prose line", line: "Open the apply link and submit it yourself.", wantOK: false},
		{name: "checkbox without a link", line: "- [x] **Acme Corp** - SRE: no link here", wantOK: false},
		{name: "link without a checkbox", line: "**Acme Corp** - SRE: [Apply Here](https://jobs.example.com/1)", wantOK: false},
		{name: "empty url", line: "- [x] **Acme Corp** - SRE: [Apply Here]()", wantOK: false},
		{name: "truncated entry", line: "- [x] **Acme Corp** - SRE:", wantOK: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e, ticked, ok := parseChecklistLine(tc.line)
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, want %v (entry %+v)", ok, tc.wantOK, e)
			}
			if !ok {
				return
			}
			if ticked != tc.wantTicked {
				t.Errorf("ticked = %v, want %v", ticked, tc.wantTicked)
			}
			if e.Company != tc.wantCompany {
				t.Errorf("Company = %q, want %q", e.Company, tc.wantCompany)
			}
			if e.Title != tc.wantTitle {
				t.Errorf("Title = %q, want %q", e.Title, tc.wantTitle)
			}
			if e.URL != tc.wantURL {
				t.Errorf("URL = %q, want %q", e.URL, tc.wantURL)
			}
		})
	}
}

// TestParseChecklist_ReturnsOnlyTickedEntries exercises the file-level reader
// against a document shaped like a real queue file, header and all.
func TestParseChecklist_ReturnsOnlyTickedEntries(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "copilot_queue.md")

	content := "# Copilot Review Queue\n\n" +
		"Open the apply link, fill the form using the documents saved alongside each entry, and submit it yourself.\n\n" +
		"- [x] **Sent Co** - SRE: [Apply Here](https://jobs.example.com/sent) — docs in `applications/needs_manual_apply/Sent_Co/`\n" +
		"- [ ] **Pending Co** - DevOps: [Apply Here](https://jobs.example.com/pending) — docs not found\n" +
		"- [X] **Also Sent Co** - Platform Engineer: [Apply Here](https://jobs.example.com/also)\n" +
		"not a checklist line at all\n"

	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}

	got, err := parseChecklist(path)
	if err != nil {
		t.Fatalf("parseChecklist: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d ticked entries, want 2: %+v", len(got), got)
	}
	if got[0].URL != "https://jobs.example.com/sent" || got[1].URL != "https://jobs.example.com/also" {
		t.Errorf("unexpected ticked entries: %+v", got)
	}
	for _, e := range got {
		if e.URL == "https://jobs.example.com/pending" {
			t.Error("an unticked entry was returned; the working queue must never be recorded as applied")
		}
	}
}

// TestParseChecklist_MissingFileIsNotAnError — a checklist only exists once its
// code path has fired, so absence is the normal state, not a failure.
func TestParseChecklist_MissingFileIsNotAnError(t *testing.T) {
	got, err := parseChecklist(filepath.Join(t.TempDir(), "does_not_exist.md"))
	if err != nil {
		t.Errorf("expected no error for a missing checklist, got %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected no entries, got %+v", got)
	}
}
