package mcp

import "testing"

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
