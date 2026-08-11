package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGetEffectiveSettings(t *testing.T) {
	d := t.TempDir()
	pPath := filepath.Join(d, "profile.yaml")
	oPath := filepath.Join(d, "operator_settings.yaml")

	os.WriteFile(pPath, []byte("auto_submit: false\nskip_scoring: true\n"), 0644)
	os.WriteFile(oPath, []byte(`{"minimum_fit_score": 75, "application_mode": "automatic"}`), 0644)

	eff, err := GetEffectiveSettings(pPath, oPath)
	if err != nil {
		t.Fatalf("GetEffectiveSettings failed: %v", err)
	}

	if !eff.ScoringActive {
		t.Errorf("ScoringActive should be true when forced by mode/score, got false")
	}
	if !eff.AutomaticSubmitClickActive {
		t.Errorf("AutomaticSubmitClickActive should be true for automatic mode")
	}
	if eff.DaemonActive {
		t.Errorf("DaemonActive should be false when active settings missing")
	}
}

// TestGetEffectiveSettings_MissingOperatorSettingsNeverInfersAutomatic is bug #535's
// regression test. A missing operator_settings.yaml must never let legacy
// profile.yaml booleans (auto_submit/auto_submit_click/copilot_mode) resolve to
// Automatic mode -- that would let a missing tiny settings file silently
// reactivate final employer submit-click.
func TestGetEffectiveSettings_MissingOperatorSettingsNeverInfersAutomatic(t *testing.T) {
	d := t.TempDir()
	pPath := filepath.Join(d, "profile.yaml")
	oPath := filepath.Join(d, "operator_settings.yaml") // deliberately never created

	os.WriteFile(pPath, []byte("auto_submit: true\nauto_submit_click: true\ncopilot_mode: false\n"), 0644)

	eff, err := GetEffectiveSettings(pPath, oPath)
	if err != nil {
		t.Fatalf("GetEffectiveSettings failed: %v", err)
	}

	if eff.ApplicationMode == ApplicationModeAutomatic {
		t.Errorf("ApplicationMode = automatic with no operator settings file; a missing settings file must never infer Automatic from legacy profile flags")
	}
	if eff.AutomaticSubmitClickActive {
		t.Errorf("AutomaticSubmitClickActive = true with no operator settings file; final employer submit-click must never activate without an explicit operator setting")
	}
}

func TestGetEffectiveSettings_MissingProfile(t *testing.T) {
	d := t.TempDir()
	pPath := filepath.Join(d, "profile.yaml")
	oPath := filepath.Join(d, "operator_settings.yaml")

	os.WriteFile(oPath, []byte(`{"minimum_fit_score": 60, "application_mode": "find_only"}`), 0644)

	eff, err := GetEffectiveSettings(pPath, oPath)
	if err != nil {
		t.Fatalf("GetEffectiveSettings should tolerate a missing profile.yaml, got: %v", err)
	}
	if eff.MinimumFitScore != 60 {
		t.Errorf("MinimumFitScore = %d, want 60", eff.MinimumFitScore)
	}
}
