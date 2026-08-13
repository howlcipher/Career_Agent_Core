package backlog

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Improvement #456 replaced the three per-provider model columns (`Claude
// model`, `Gemini model`, `OpenAI model`, plus `OpenAI task-fit reason`) with
// a single `Tier` column — mechanical / standard / deep-reasoning — because
// concrete model IDs expire (improvement #455 found one that had sat broken
// in seven rows for four days) while a capability tier does not. This file's
// predecessor validated backlog model cells against a live-catalogue
// allowlist (documentation/model_allowlist.md); that mechanism is gone by
// construction now, since the tier vocabulary is fixed and never needs
// re-verifying against a vendor.
//
// What still needs enforcing is the same failure class in a new column:
// every Pending row must actually name one of the three tiers rather than a
// typo, a leftover model string, or an empty cell sitting unnoticed for days.
// This still parses the table column by its header name, not a fixed index
// or a grep on raw text — bugs.md carries an extra Severity column the other
// two files do not, so Tier sits at a different offset in each file.

var (
	itemRowRE       = regexp.MustCompile(`^\|\s*\d+\s*\|`)
	backlogHeaderRE = regexp.MustCompile(`^\|\s*#\s*\|.*\|\s*Tier\s*\|`)
	// independentPendingRE is deliberately not a plain substring match. A row's
	// own dated Done note can quote code or prose containing "| Pending" (e.g.
	// improvement #465's note about this exact independent-count mechanism,
	// which quotes the predecessor test's Go source verbatim) without being a
	// Pending row itself. Requiring a real cell boundary after "Pending" — a
	// closing pipe, an em dash, or the below-floor warning emoji — avoids that
	// collision while still not sharing any code with itemRowRE/splitRow.
	independentPendingRE = regexp.MustCompile(`\|\s*Pending(\s*\||\s+—|\s+⚠)`)
)

var validTiers = map[string]bool{
	"mechanical":     true,
	"standard":       true,
	"deep-reasoning": true,
}

// backlogFiles are the three ranked backlogs, relative to the repo root.
var backlogFiles = []string{"documentation/backlog/bugs.md", "documentation/backlog/improvements.md", "documentation/backlog/improvements_paywall.md"}

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

// isPlaceholder reports whether a cell means "no tier recorded" rather than
// naming one — the convention for rows that predate #456 or never needed a
// model recommendation (e.g. "shipped before this backlog restructure").
func isPlaceholder(v string) bool {
	switch v {
	case "", "—", "-", "–", "n/a", "N/A":
		return true
	}
	return false
}

// tierCell is one Tier-column value found in one backlog item row.
type tierCell struct {
	file    string
	line    int
	item    string
	value   string
	pending bool
}

// backlogTierCells walks a backlog file's ranked table and yields every
// Tier-column value, located by header name.
func backlogTierCells(t *testing.T, file string) []tierCell {
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
		// Not a soft skip: if the header is renamed again, this check would
		// otherwise start silently validating nothing, which is the same
		// class of failure it exists to prevent.
		t.Fatalf(
			"%s has no ranked-backlog header matching %q. If the Tier column was renamed, "+
				"update this test in the same change — do not delete it",
			file, backlogHeaderRE,
		)
	}

	headers := splitRow(lines[headerIdx])
	tierIdx, statusIdx := -1, -1
	for i, h := range headers {
		if h == "Tier" {
			tierIdx = i
		}
		if h == "Status" {
			statusIdx = i
		}
	}
	if tierIdx < 0 {
		t.Fatalf("%s header %v has no Tier column", file, headers)
	}
	if statusIdx < 0 {
		// Without Status the check cannot tell a live routing value from a
		// historical one, and would have to fail on every closed row or on none.
		t.Fatalf("%s header %v has no Status column; it is what separates rows that route from rows that are a record", file, headers)
	}

	var cells []tierCell
	for i := headerIdx + 2; i < len(lines); i++ {
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
		if tierIdx >= len(row) {
			continue
		}
		cells = append(cells, tierCell{
			file:    file,
			line:    i + 1,
			item:    row[0],
			value:   row[tierIdx],
			pending: statusIdx < len(row) && strings.HasPrefix(row[statusIdx], "Pending"),
		})
	}
	return cells
}

// TestPendingBacklogRowsNameValidTiers is the direct successor to the old
// TestPendingBacklogRowsNameRealModels: every Pending row must name one of
// the three fixed tiers, never a placeholder and never a typo'd or stale
// value, so a wrong cell cannot sit silently for days the way
// claude-opus-4-6-thinking did before #455 caught it.
func TestPendingBacklogRowsNameValidTiers(t *testing.T) {
	checked := 0
	for _, file := range backlogFiles {
		for _, cell := range backlogTierCells(t, file) {
			if !cell.pending {
				continue
			}
			checked++
			if !validTiers[cell.value] {
				t.Errorf(
					"%s:%d item #%s Tier column names %q, which is not one of mechanical/standard/deep-reasoning.\n"+
						"    Every Pending row must name a real tier (see improvements.md #456) — an empty or "+
						"placeholder cell here is the same silent-drift failure #455 found in the old model columns.",
					cell.file, cell.line, cell.item, cell.value,
				)
			}
		}
	}

	// Guards the guard, same as the predecessor test: a parser bug or a stray
	// edit that made itemRowRE or the Status match stop firing would
	// otherwise leave this test passing loudly while checking nothing.
	independentPendingRows := 0
	for _, file := range backlogFiles {
		independentPendingRows += len(independentPendingRE.FindAllString(readRepoFile(t, file), -1))
	}
	if checked < independentPendingRows {
		t.Errorf(
			"only %d Pending Tier cells were checked across %v, fewer than the %d Pending rows those files "+
				"currently contain (counted independently of the table parser above). Every currently Pending "+
				"row names a tier, so a shortfall here means the parser stopped matching rows it should have "+
				"found, not that the backlog emptied — fix the parser rather than this floor",
			checked, backlogFiles, independentPendingRows,
		)
	}
}

// TestClosedBacklogRowsUseValidTierOrPlaceholder guards the same drift class
// on closed rows. A Done/Resolved row may legitimately carry no tier at all —
// every row that predates #456 does, recorded as a placeholder — but must
// never carry a value that is neither a real tier nor a placeholder.
func TestClosedBacklogRowsUseValidTierOrPlaceholder(t *testing.T) {
	for _, file := range backlogFiles {
		for _, cell := range backlogTierCells(t, file) {
			if cell.pending {
				continue
			}
			if isPlaceholder(cell.value) || validTiers[cell.value] {
				continue
			}
			t.Errorf(
				"%s:%d item #%s Tier column names %q on a closed row — neither a valid tier "+
					"(mechanical/standard/deep-reasoning) nor a placeholder (%q)",
				cell.file, cell.line, cell.item, cell.value, "—",
			)
		}
	}
}

// TestBacklogTierColumnIsParsedFromHeader documents, executably, why the
// column index is never hardcoded: bugs.md carries a Severity column the
// other two do not, so the Tier column sits at a different offset in each
// file.
func TestBacklogTierColumnIsParsedFromHeader(t *testing.T) {
	offsets := make(map[string]int)
	for _, file := range backlogFiles {
		raw := readRepoFile(t, file)
		for _, line := range strings.Split(raw, "\n") {
			if !backlogHeaderRE.MatchString(line) {
				continue
			}
			for i, h := range splitRow(line) {
				if h == "Tier" {
					offsets[file] = i
				}
			}
			break
		}
	}
	if len(offsets) != len(backlogFiles) {
		t.Fatalf("expected a Tier column in all of %v, found %v", backlogFiles, offsets)
	}
	if offsets["documentation/backlog/bugs.md"] == offsets["documentation/backlog/improvements.md"] {
		t.Errorf(
			"bugs.md and improvements.md now place the Tier column at the same offset (%d). "+
				"That is not a failure in itself, but this test exists to record that they historically "+
				"differed — if the schemas were deliberately unified, delete this test rather than "+
				"weakening the header-driven parsing it justifies",
			offsets["documentation/backlog/bugs.md"],
		)
	}
}
