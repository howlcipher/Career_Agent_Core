package mcp

import (
	"strings"
	"testing"
)

func TestIncrementAndLogAPICall_DefaultLimit(t *testing.T) {
	if err := incrementAndLogAPICall("ScoreJob", 50000); err != nil {
		t.Errorf("expected 50000 chars to pass the default limit, got: %v", err)
	}
	if err := incrementAndLogAPICall("ScoreJob", 50001); err == nil {
		t.Error("expected 50001 chars to trip the default 50k limit, got nil")
	}
}

// TestIncrementAndLogAPICall_SolveValidationErrorsHigherLimit is the
// live-confirmed shape from bugs.md #52's Reddit recurrence: a real, large
// screening form (35 fields) still lands around 52-55k chars even after
// PruneDOMToForm + StripPresentationalAttrs, genuinely proportional to
// field count rather than bloat.
func TestIncrementAndLogAPICall_SolveValidationErrorsHigherLimit(t *testing.T) {
	if err := incrementAndLogAPICall("SolveValidationErrors", 55000); err != nil {
		t.Errorf("expected 55000 chars to pass SolveValidationErrors' 75k limit, got: %v", err)
	}
	if err := incrementAndLogAPICall("SolveValidationErrors", 75001); err == nil {
		t.Error("expected 75001 chars to still trip SolveValidationErrors' 75k limit, got nil")
	}
}

func TestIncrementAndLogAPICall_OtherCallTypesUnaffected(t *testing.T) {
	if err := incrementAndLogAPICall("ExtractFormMapping", 60000); err == nil {
		t.Error("expected an unrelated call type to still use the default 50k limit, got nil")
	}
}

// improvements.md #25: scoring cost is set by prompt length on this CPU-only
// host, so over-long descriptions are trimmed -- but from the middle, because
// rubric rules 2/3/7 (remote, salary floor, geographic restriction) depend on
// trailing details that a plain head truncation would discard.
func TestTrimForScoring(t *testing.T) {
	t.Run("short descriptions pass through untouched", func(t *testing.T) {
		in := "Senior Go Engineer, fully remote, $150k."
		if got := trimForScoring(in); got != in {
			t.Errorf("expected passthrough, got %q", got)
		}
	})

	t.Run("keeps both ends of an over-long description", func(t *testing.T) {
		head := "REQUIREMENTS: Senior Go engineer, fully remote."
		tail := "Salary: $95,000. Must reside in Romania."
		in := head + strings.Repeat("x", maxScoringDescChars*2) + tail

		got := trimForScoring(in)

		if len(got) >= len(in) {
			t.Errorf("expected a shorter prompt, got %d vs %d", len(got), len(in))
		}
		if !strings.Contains(got, "REQUIREMENTS: Senior Go engineer") {
			t.Error("the head must survive — it carries the role and remote wording")
		}
		if !strings.Contains(got, "Must reside in Romania") {
			t.Error("the tail must survive — rubric rule 7 depends on it")
		}
		if !strings.Contains(got, "Salary: $95,000") {
			t.Error("the tail must survive — rubric rule 3 depends on it")
		}
		if !strings.Contains(got, "omitted for scoring") {
			t.Error("the elision should be marked so the model knows text was removed")
		}
	})

	t.Run("result stays within the intended budget", func(t *testing.T) {
		in := strings.Repeat("y", 100_000)
		got := trimForScoring(in)
		budget := scoringDescHeadChars + len(scoringElision) + scoringDescTailChars
		if len(got) != budget {
			t.Errorf("trimmed length = %d, want %d", len(got), budget)
		}
	})
}
