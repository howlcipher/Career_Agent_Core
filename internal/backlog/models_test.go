package backlog

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Improvement #455 found `claude-opus-4-6-thinking` sitting in seven backlog
// rows for four days. It is not a model ID — no `-thinking` suffix exists in
// any Anthropic model name, because extended thinking is a request parameter
// and not part of an ID — so the string could never have resolved.
//
// About a dozen groom passes ran in those four days, each claiming to have
// re-verified every row. They missed it because re-verification meant re-running
// a *count*, never testing the claim the count supported. #455's own report is
// the illustration: it counted 73/7/4 correctly and then got the blast radius
// (80 rows, really 7), the status of `claude-sonnet-4-6` (called obsolete,
// actually current), and the existence of a `gemini-whatever-provider-is-configured`
// cell (there is none — the string is part of bug #444's anchor link) all wrong.
//
// That last error is why this file parses the table *column* via its header
// rather than grepping the file for `claude-*`. Grepping raw markdown cannot
// tell a model column from a title, an anchor, or a prose aside, and every
// previous attempt to audit these columns by grep produced a false positive.
//
// documentation/model_allowlist.md is the authority for which IDs are real.

var (
	// Backlog item rows open with a bare item number. The groom-note score
	// tables in the same files open with `| #455 |`, so this pattern separates
	// live routing values from historical commentary without needing to know
	// where one table ends and the next begins.
	itemRowRE = regexp.MustCompile(`^\|\s*\d+\s*\|`)
	// The header of the ranked backlog table itself. bugs.md carries an extra
	// Severity column, which is exactly why columns are located by name here
	// and never by a hardcoded index.
	backlogHeaderRE = regexp.MustCompile(`^\|\s*#\s*\|.*Claude model`)
	allowlistRowRE  = regexp.MustCompile("^\\|\\s*`([^`]+)`\\s*\\|(.*)\\|\\s*$")
	allowlistSecRE  = regexp.MustCompile(`^##\s+(\S+)\s*$`)
)

// modelColumns maps a backlog table header to the allowlist section that governs
// it. Adding a provider column means adding it here and in the allowlist.
var modelColumns = map[string]string{
	"Claude model": "anthropic",
	"Gemini model": "google",
	"OpenAI model": "openai",
}

// legacyLabelBaseline is the number of model cells on *closed* rows that name
// something absent from the allowlist — almost entirely pre-2026-07-26 display
// names like "Opus 5" and "Gemini 3 Pro" rather than routable IDs.
//
// These are deliberately not fixed. A closed row's model column is a record of
// what was recommended at the time, in the same category as the dated groom
// notes, and rewriting 248 of them would churn history to no benefit: nothing
// routes a finished item. What matters is that the number never grows, which is
// what TestLegacyModelLabelsDoNotGrow enforces. Lower it freely when rows are
// cleaned up; never raise it.
//
// It is a ratchet, not a truth: a handful of these rows contain an unescaped
// pipe in their prose, which shifts the cell split, so the count is stable
// rather than semantically exact.
const legacyLabelBaseline = 237

// backlogFiles are the three ranked backlogs, relative to the repo root.
var backlogFiles = []string{"bugs.md", "improvements.md", "improvements_paywall.md"}

func repoRoot() string { return filepath.Join("..", "..") }

func readRepoFile(t *testing.T, name string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(repoRoot(), name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return string(raw)
}

// splitRow splits a markdown table row into trimmed cells, dropping the empty
// strings the leading and trailing pipes produce.
func splitRow(line string) []string {
	parts := strings.Split(strings.TrimSpace(line), "|")
	if len(parts) > 0 && strings.TrimSpace(parts[0]) == "" {
		parts = parts[1:]
	}
	if len(parts) > 0 && strings.TrimSpace(parts[len(parts)-1]) == "" {
		parts = parts[:len(parts)-1]
	}
	cells := make([]string, 0, len(parts))
	for _, p := range parts {
		cells = append(cells, strings.TrimSpace(p))
	}
	return cells
}

// allowedModels parses documentation/model_allowlist.md into
// provider -> model ID -> provenance.
func allowedModels(t *testing.T) map[string]map[string]string {
	t.Helper()
	allowed := make(map[string]map[string]string)
	section := ""
	for _, line := range strings.Split(readRepoFile(t, filepath.Join("documentation", "model_allowlist.md")), "\n") {
		if m := allowlistSecRE.FindStringSubmatch(line); m != nil {
			section = strings.ToLower(m[1])
			if _, ok := allowed[section]; !ok {
				allowed[section] = make(map[string]string)
			}
			continue
		}
		if section == "" {
			continue
		}
		if m := allowlistRowRE.FindStringSubmatch(line); m != nil {
			allowed[section][m[1]] = strings.TrimSpace(m[2])
		}
	}
	return allowed
}

// modelCell is one model-column value found in one backlog item row.
type modelCell struct {
	file     string
	line     int
	item     string
	column   string
	provider string
	value    string
	pending  bool
}

// normalizeModel strips the code-fence backticks some rows wrap their model
// value in, so `gpt-5.6-terra` and gpt-5.6-terra are the same value. Formatting
// is not identity.
func normalizeModel(v string) string {
	return strings.TrimSpace(strings.Trim(strings.TrimSpace(v), "`"))
}

// backlogModelCells walks a backlog file's ranked table and yields every
// model-column value, located by header name.
func backlogModelCells(t *testing.T, file string) []modelCell {
	t.Helper()
	lines := strings.Split(readRepoFile(t, file), "\n")

	headerIdx := -1
	for i, line := range lines {
		if backlogHeaderRE.MatchString(line) {
			headerIdx = i
			break
		}
	}
	if headerIdx < 0 {
		// Not a soft skip: if the header is renamed (improvements.md #456
		// proposes replacing these columns with capability tiers) this check
		// would otherwise start silently validating nothing, which is the same
		// class of failure it exists to prevent.
		t.Fatalf(
			"%s has no ranked-backlog header matching %q. If the model columns were "+
				"renamed or replaced (see improvements.md #456), update modelColumns and "+
				"documentation/model_allowlist.md in the same change — do not delete this test",
			file, backlogHeaderRE,
		)
	}

	headers := splitRow(lines[headerIdx])
	colProvider := make(map[int]string)
	colName := make(map[int]string)
	statusIdx := -1
	for i, h := range headers {
		if provider, ok := modelColumns[h]; ok {
			colProvider[i] = provider
			colName[i] = h
		}
		if h == "Status" {
			statusIdx = i
		}
	}
	if len(colProvider) != len(modelColumns) {
		t.Fatalf("%s header %v does not contain all of %v", file, headers, modelColumns)
	}
	if statusIdx < 0 {
		// Without Status the check cannot tell a live routing value from a
		// historical one, and would have to fail on 248 closed rows or on none.
		t.Fatalf("%s header %v has no Status column; it is what separates rows that route from rows that are a record", file, headers)
	}

	var cells []modelCell
	for i := headerIdx + 1; i < len(lines); i++ {
		line := lines[i]
		if !strings.HasPrefix(strings.TrimSpace(line), "|") {
			if strings.TrimSpace(line) == "" {
				continue // blank lines inside a doc, keep scanning
			}
			break // prose resumed; the table is over
		}
		if !itemRowRE.MatchString(line) {
			continue // separator row, or a groom-note row like `| #455 |`
		}
		row := splitRow(line)
		pending := statusIdx < len(row) && strings.HasPrefix(row[statusIdx], "Pending")
		for idx, provider := range colProvider {
			if idx >= len(row) {
				continue
			}
			cells = append(cells, modelCell{
				file:     file,
				line:     i + 1,
				item:     row[0],
				column:   colName[idx],
				provider: provider,
				value:    normalizeModel(row[idx]),
				pending:  pending,
			})
		}
	}
	return cells
}

// isPlaceholder reports whether a cell means "no recommendation" rather than
// naming a model. Em dash is the convention used throughout these tables.
func isPlaceholder(v string) bool {
	switch v {
	case "", "—", "-", "–", "n/a", "N/A":
		return true
	}
	return false
}

// TestPendingBacklogRowsNameRealModels is the check that would have caught
// `claude-opus-4-6-thinking` on the day it was written instead of four days and
// a dozen groom passes later.
//
// It is scoped to Pending rows because those are the ones anything would ever
// route on. A closed row's model column is a record of what was recommended
// then; TestLegacyModelLabelsDoNotGrow holds that historical debt flat.
func TestPendingBacklogRowsNameRealModels(t *testing.T) {
	allowed := allowedModels(t)
	for _, provider := range modelColumns {
		if len(allowed[provider]) == 0 {
			t.Fatalf("documentation/model_allowlist.md has no ## %s section, or it is empty", provider)
		}
	}

	checked := 0
	for _, file := range backlogFiles {
		for _, cell := range backlogModelCells(t, file) {
			if !cell.pending || isPlaceholder(cell.value) {
				continue
			}
			checked++
			if _, ok := allowed[cell.provider][cell.value]; !ok {
				t.Errorf(
					"%s:%d item #%s %s column names %q, which is not in the ## %s section of "+
						"documentation/model_allowlist.md.\n"+
						"    Either the value is wrong (this is how `claude-opus-4-6-thinking` got in — "+
						"a plausible-looking ID nobody resolved against a catalogue), or the model is real "+
						"and the allowlist needs an entry with real provenance.\n"+
						"    Do not add it from memory. Check the vendor catalogue first.",
					cell.file, cell.line, cell.item, cell.column, cell.value, cell.provider,
				)
			}
		}
	}

	// Guards the guard. A parser bug, a schema rewrite, or a stray edit that
	// made itemRowRE or the Status match stop firing would otherwise leave this
	// test passing loudly while checking nothing — which is the exact shape of
	// the failure it exists to prevent.
	//
	// The floor used to be a hardcoded historical snapshot (44 cells, then a
	// bound of 20) taken the day this test was written. That is exactly the
	// kind of unchecked number improvement #455 warned about: it stopped being
	// evidence for "the parser still works" and quietly became evidence for
	// "the backlog hasn't shrunk since 2026-07-30" instead — closing improvement
	// #460 dropped the real count from 20 to 18 and tripped it for a reason that
	// had nothing to do with parsing. The floor is now derived independently,
	// via a plain substring scan that does not share any code path with
	// itemRowRE/splitRow/backlogHeaderRE above, so a regression in those still
	// gets caught (checked would fall to 0 while this count would not) without
	// the test being sensitive to ordinary backlog progress.
	independentPendingRows := 0
	for _, file := range backlogFiles {
		independentPendingRows += strings.Count(readRepoFile(t, file), "| Pending")
	}
	if checked < independentPendingRows {
		t.Errorf(
			"only %d Pending model cells were checked across %v, fewer than the %d Pending rows "+
				"those files currently contain (counted independently of the table parser above). "+
				"Every currently Pending row names at least one real model, so a shortfall here means "+
				"the parser stopped matching cells it should have found, not that the backlog emptied — "+
				"fix the parser rather than this floor",
			checked, backlogFiles, independentPendingRows,
		)
	}
}

// TestLegacyModelLabelsDoNotGrow ratchets the pre-2026-07-26 display-name debt
// on closed rows ("Opus 5", "Gemini 3 Pro") so it can shrink but never grow.
//
// Without this, scoping the real check to Pending rows would silently license
// writing junk into any row that happens to be closed.
func TestLegacyModelLabelsDoNotGrow(t *testing.T) {
	allowed := allowedModels(t)
	legacy := 0
	for _, file := range backlogFiles {
		for _, cell := range backlogModelCells(t, file) {
			if cell.pending || isPlaceholder(cell.value) {
				continue
			}
			if _, ok := allowed[cell.provider][cell.value]; !ok {
				legacy++
			}
		}
	}
	switch {
	case legacy > legacyLabelBaseline:
		t.Errorf(
			"closed backlog rows now hold %d model cells naming something outside the allowlist, up from "+
				"the recorded baseline of %d. A closed row may keep a historical label, but the count must "+
				"not grow — a new one means junk was written into a row that merely happens to be closed",
			legacy, legacyLabelBaseline,
		)
	case legacy < legacyLabelBaseline:
		t.Errorf(
			"legacy model labels on closed rows dropped from %d to %d — nice. Lower legacyLabelBaseline "+
				"to %d so the ratchet holds the new ground",
			legacyLabelBaseline, legacy, legacy,
		)
	}
}

// TestModelAllowlistEntriesCarryProvenance stops the allowlist from becoming a
// second place where unsourced model IDs accumulate. An allowlist that accepts
// anything reproduces #455 one level up.
func TestModelAllowlistEntriesCarryProvenance(t *testing.T) {
	allowed := allowedModels(t)
	total := 0
	for provider, models := range allowed {
		for id, provenance := range models {
			total++
			if strings.TrimSpace(provenance) == "" {
				t.Errorf(
					"documentation/model_allowlist.md: ## %s entry %q has an empty provenance cell. "+
						"Record where the ID came from and when it was checked",
					provider, id,
				)
			}
		}
	}
	if total == 0 {
		t.Fatal("documentation/model_allowlist.md parsed to zero entries; the table format likely changed")
	}
}

// TestBacklogModelColumnsAreParsedFromHeaders documents, executably, why the
// column index is never hardcoded: bugs.md carries a Severity column the other
// two do not, so the Claude column sits at a different offset in each file.
func TestBacklogModelColumnsAreParsedFromHeaders(t *testing.T) {
	offsets := make(map[string]int)
	for _, file := range backlogFiles {
		raw := readRepoFile(t, file)
		for _, line := range strings.Split(raw, "\n") {
			if !backlogHeaderRE.MatchString(line) {
				continue
			}
			for i, h := range splitRow(line) {
				if h == "Claude model" {
					offsets[file] = i
				}
			}
			break
		}
	}
	if len(offsets) != len(backlogFiles) {
		t.Fatalf("expected a Claude model column in all of %v, found %v", backlogFiles, offsets)
	}
	if offsets["bugs.md"] == offsets["improvements.md"] {
		t.Errorf(
			"bugs.md and improvements.md now place the Claude column at the same offset (%d). "+
				"That is not a failure in itself, but this test exists to record that they historically "+
				"differed — if the schemas were deliberately unified, delete this test rather than "+
				"weakening the header-driven parsing it justifies",
			offsets["bugs.md"],
		)
	}
	fmt.Fprintf(os.Stderr, "Claude column offsets by file: %v\n", offsets)
}
