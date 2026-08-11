package config

import (
	"os"
	"path/filepath"
	"testing"
)

// This file is bugs.md #535's required test matrix: Automatic final employer
// submission must never be reachable without an explicit, successfully
// loaded operator_settings.yaml selecting application_mode: automatic. See
// ApplyOperatorSettings and DefaultOperatorSettings in operator.go, and
// GetEffectiveSettings in effective_settings.go, for the fix these tests
// cover.

// 1. Missing settings + legacy Automatic flags: must not resolve to Automatic.
func TestGetEffectiveSettings_MissingSettingsLegacyAutomaticFlags(t *testing.T) {
	d := t.TempDir()
	pPath := filepath.Join(d, "profile.yaml")
	oPath := filepath.Join(d, "operator_settings.yaml") // never created

	os.WriteFile(pPath, []byte("auto_submit: true\nauto_submit_click: true\ncopilot_mode: false\n"), 0644)

	eff, err := GetEffectiveSettings(pPath, oPath)
	if err != nil {
		t.Fatalf("GetEffectiveSettings failed: %v", err)
	}
	if eff.ApplicationMode == ApplicationModeAutomatic {
		t.Errorf("ApplicationMode = automatic; missing operator settings must not infer Automatic from legacy flags")
	}
	if eff.ApplicationMode != ApplicationModeFindOnly {
		t.Errorf("ApplicationMode = %q, want find_only (the documented fail-closed default)", eff.ApplicationMode)
	}
	if eff.AutomaticSubmitClickActive {
		t.Errorf("AutomaticSubmitClickActive = true; final submit-click must never be active with no operator settings")
	}
}

// 2. Missing settings + ordinary (all-false) profile: must resolve to the safe default.
func TestGetEffectiveSettings_MissingSettingsOrdinaryProfile(t *testing.T) {
	d := t.TempDir()
	pPath := filepath.Join(d, "profile.yaml")
	oPath := filepath.Join(d, "operator_settings.yaml")

	os.WriteFile(pPath, []byte("auto_submit: false\nauto_submit_click: false\ncopilot_mode: false\n"), 0644)

	eff, err := GetEffectiveSettings(pPath, oPath)
	if err != nil {
		t.Fatalf("GetEffectiveSettings failed: %v", err)
	}
	if eff.ApplicationMode != ApplicationModeFindOnly {
		t.Errorf("ApplicationMode = %q, want find_only", eff.ApplicationMode)
	}
	if eff.AutomaticSubmitClickActive {
		t.Errorf("AutomaticSubmitClickActive = true, want false")
	}
	if eff.MinimumFitScore != 50 {
		t.Errorf("MinimumFitScore = %d, want the documented default of 50", eff.MinimumFitScore)
	}
}

// 3. Missing settings + legacy Assisted/Copilot flags: chosen compatibility
// behavior (fail-closed default, not an inferred Assisted mode) must never
// enable automatic final submit.
func TestGetEffectiveSettings_MissingSettingsLegacyAssistedFlags(t *testing.T) {
	d := t.TempDir()
	pPath := filepath.Join(d, "profile.yaml")
	oPath := filepath.Join(d, "operator_settings.yaml")

	os.WriteFile(pPath, []byte("auto_submit: true\nauto_submit_click: false\ncopilot_mode: true\n"), 0644)

	eff, err := GetEffectiveSettings(pPath, oPath)
	if err != nil {
		t.Fatalf("GetEffectiveSettings failed: %v", err)
	}
	if eff.ApplicationMode == ApplicationModeAutomatic {
		t.Errorf("ApplicationMode = automatic; must never be inferred from legacy flags")
	}
	if eff.AutomaticSubmitClickActive {
		t.Errorf("AutomaticSubmitClickActive = true; final submit-click must never be active with no operator settings")
	}
}

// 4. Explicit find_only remains find_only.
func TestGetEffectiveSettings_ExplicitFindOnly(t *testing.T) {
	d := t.TempDir()
	pPath := filepath.Join(d, "profile.yaml")
	oPath := filepath.Join(d, "operator_settings.yaml")

	os.WriteFile(oPath, []byte("application_mode: find_only\nminimum_fit_score: 55\n"), 0644)

	eff, err := GetEffectiveSettings(pPath, oPath)
	if err != nil {
		t.Fatalf("GetEffectiveSettings failed: %v", err)
	}
	if eff.ApplicationMode != ApplicationModeFindOnly {
		t.Errorf("ApplicationMode = %q, want find_only", eff.ApplicationMode)
	}
	if eff.AutomaticSubmitClickActive {
		t.Errorf("AutomaticSubmitClickActive = true, want false")
	}
}

// 5. Explicit assisted remains assisted, and final submit-click stays false.
func TestGetEffectiveSettings_ExplicitAssisted(t *testing.T) {
	d := t.TempDir()
	pPath := filepath.Join(d, "profile.yaml")
	oPath := filepath.Join(d, "operator_settings.yaml")

	os.WriteFile(oPath, []byte("application_mode: assisted\nminimum_fit_score: 60\n"), 0644)

	eff, err := GetEffectiveSettings(pPath, oPath)
	if err != nil {
		t.Fatalf("GetEffectiveSettings failed: %v", err)
	}
	if eff.ApplicationMode != ApplicationModeAssisted {
		t.Errorf("ApplicationMode = %q, want assisted", eff.ApplicationMode)
	}
	if eff.AutomaticSubmitClickActive {
		t.Errorf("AutomaticSubmitClickActive = true, want false for assisted mode")
	}
}

// 6. Explicit automatic remains automatic and retains the requested
// automatic-submit behavior -- this change must not remove Automatic mode.
func TestGetEffectiveSettings_ExplicitAutomatic(t *testing.T) {
	d := t.TempDir()
	pPath := filepath.Join(d, "profile.yaml")
	oPath := filepath.Join(d, "operator_settings.yaml")

	os.WriteFile(oPath, []byte("application_mode: automatic\nminimum_fit_score: 70\n"), 0644)

	eff, err := GetEffectiveSettings(pPath, oPath)
	if err != nil {
		t.Fatalf("GetEffectiveSettings failed: %v", err)
	}
	if eff.ApplicationMode != ApplicationModeAutomatic {
		t.Errorf("ApplicationMode = %q, want automatic", eff.ApplicationMode)
	}
	if !eff.AutomaticSubmitClickActive {
		t.Errorf("AutomaticSubmitClickActive = false, want true for an explicit automatic setting")
	}
	if eff.MinimumFitScore != 70 {
		t.Errorf("MinimumFitScore = %d, want 70", eff.MinimumFitScore)
	}
}

// 7. Corrupt operator settings fails closed (returns an error, never silently
// interpreted as Automatic).
func TestGetEffectiveSettings_CorruptSettingsFailsClosed(t *testing.T) {
	d := t.TempDir()
	pPath := filepath.Join(d, "profile.yaml")
	oPath := filepath.Join(d, "operator_settings.yaml")

	os.WriteFile(oPath, []byte("{ this is not: valid yaml : : :"), 0644)

	eff, err := GetEffectiveSettings(pPath, oPath)
	if err == nil {
		t.Fatalf("expected an error for corrupt operator settings, got eff=%+v", eff)
	}
	if eff != nil {
		t.Errorf("expected nil EffectiveSettings on error, got %+v", eff)
	}
}

// 8. Invalid application_mode value fails closed.
func TestGetEffectiveSettings_InvalidModeFailsClosed(t *testing.T) {
	d := t.TempDir()
	pPath := filepath.Join(d, "profile.yaml")
	oPath := filepath.Join(d, "operator_settings.yaml")

	os.WriteFile(oPath, []byte("application_mode: yolo_mode\nminimum_fit_score: 50\n"), 0644)

	eff, err := GetEffectiveSettings(pPath, oPath)
	if err == nil {
		t.Fatalf("expected an error for invalid application_mode, got eff=%+v", eff)
	}
	if eff != nil {
		t.Errorf("expected nil EffectiveSettings on error, got %+v", eff)
	}
}

// 9. A stale active_operator_settings.json (daemon acknowledgement) must not
// itself grant Automatic mode -- it may only contribute to DaemonActive
// liveness/status, never override ApplicationMode/AutomaticSubmitClickActive.
func TestGetEffectiveSettings_StaleActiveSettingsDoesNotGrantAutomatic(t *testing.T) {
	d := t.TempDir()
	pPath := filepath.Join(d, "profile.yaml")
	oPath := filepath.Join(d, "operator_settings.yaml")
	activePath := filepath.Join(d, "active_operator_settings.json")

	os.WriteFile(oPath, []byte("application_mode: find_only\nminimum_fit_score: 50\n"), 0644)

	stale := &EffectiveSettings{
		ApplicationMode:            ApplicationModeAutomatic,
		MinimumFitScore:            50,
		ScoringActive:              true,
		AutomaticSubmitClickActive: true,
	}
	if err := AcknowledgeActiveSettings(stale, activePath); err != nil {
		t.Fatalf("failed to write stale active settings fixture: %v", err)
	}

	eff, err := GetEffectiveSettings(pPath, oPath)
	if err != nil {
		t.Fatalf("GetEffectiveSettings failed: %v", err)
	}
	if eff.ApplicationMode == ApplicationModeAutomatic {
		t.Errorf("ApplicationMode = automatic; a stale active-settings heartbeat must never override the current operator settings")
	}
	if eff.AutomaticSubmitClickActive {
		t.Errorf("AutomaticSubmitClickActive = true; a stale active-settings heartbeat must never grant automatic submit-click")
	}
}

// Fresh-start regression: a legacy profile exists (as it would after an
// upgrade) and operator_settings.yaml has never been created. This is
// exactly the environment bug #535 describes, exercised directly against
// ApplyOperatorSettings -- the function cmd/agent's startup path calls with
// whatever LoadOperatorSettings returns, which is nil in this scenario.
func TestApplyOperatorSettings_FreshStartAfterUpgradeFailsClosed(t *testing.T) {
	p := &Profile{
		AutoSubmit:      true,
		AutoSubmitClick: true,
		CopilotMode:     false,
	}

	if err := ApplyOperatorSettings(p, nil); err != nil {
		t.Fatalf("ApplyOperatorSettings failed: %v", err)
	}

	if p.AutoSubmit || p.AutoSubmitClick || p.CopilotMode {
		t.Errorf("legacy flags after nil operator settings = (AutoSubmit=%v, AutoSubmitClick=%v, CopilotMode=%v), want all false (find_only)",
			p.AutoSubmit, p.AutoSubmitClick, p.CopilotMode)
	}
}
