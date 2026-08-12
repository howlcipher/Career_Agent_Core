package config

import (
	"os"
	"path/filepath"
	"strconv"
	"syscall"
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

func withTempAgentPaths(t *testing.T) {
	t.Helper()
	oldLock := AgentLockPath
	oldActive := ActiveSettingsPath
	dir := t.TempDir()
	AgentLockPath = filepath.Join(dir, "career_agent.lock")
	ActiveSettingsPath = filepath.Join(dir, "active_operator_settings.json")
	t.Cleanup(func() {
		AgentLockPath = oldLock
		ActiveSettingsPath = oldActive
	})
}

func holdAgentLock(t *testing.T, lockPath string, pid int) *os.File {
	t.Helper()
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0666)
	if err != nil {
		t.Fatal(err)
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(0); err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteAt([]byte(strconv.Itoa(pid)), 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		_ = f.Close()
		_ = os.Remove(lockPath)
	})
	return f
}

func TestGetEffectiveSettings_DaemonActiveRequiresLiveness(t *testing.T) {
	withTempAgentPaths(t)
	d := t.TempDir()
	pPath := filepath.Join(d, "profile.yaml")
	oPath := filepath.Join(d, "operator_settings.yaml")

	os.WriteFile(pPath, []byte("auto_submit: false\nskip_scoring: true\n"), 0644)
	os.WriteFile(oPath, []byte(`{"minimum_fit_score": 75, "application_mode": "automatic"}`), 0644)

	// Acknowledge matching active settings while no agent holds the lock.
	eff, err := GetEffectiveSettings(pPath, oPath)
	if err != nil {
		t.Fatalf("GetEffectiveSettings failed: %v", err)
	}
	if err := AcknowledgeActiveSettings(eff, ActiveSettingsPath); err != nil {
		t.Fatalf("AcknowledgeActiveSettings failed: %v", err)
	}

	eff, err = GetEffectiveSettings(pPath, oPath)
	if err != nil {
		t.Fatalf("GetEffectiveSettings failed: %v", err)
	}
	if eff.DaemonActive {
		t.Errorf("DaemonActive = true with matching active settings but no agent; must require liveness")
	}
	if eff.DaemonRunning {
		t.Errorf("DaemonRunning = true with no agent running")
	}

	// Now hold the lock and confirm both liveness flags become true.
	holdAgentLock(t, AgentLockPath, 424242)

	eff, err = GetEffectiveSettings(pPath, oPath)
	if err != nil {
		t.Fatalf("GetEffectiveSettings failed: %v", err)
	}
	if !eff.DaemonActive {
		t.Errorf("DaemonActive = false with matching active settings and live agent")
	}
	if !eff.DaemonRunning {
		t.Errorf("DaemonRunning = false while lock is held")
	}
}

func TestGetEffectiveSettings_DaemonActiveFalseWhenSettingsMismatch(t *testing.T) {
	withTempAgentPaths(t)
	d := t.TempDir()
	pPath := filepath.Join(d, "profile.yaml")
	oPath := filepath.Join(d, "operator_settings.yaml")

	os.WriteFile(pPath, []byte("auto_submit: false\nskip_scoring: true\n"), 0644)
	os.WriteFile(oPath, []byte(`{"minimum_fit_score": 75, "application_mode": "automatic"}`), 0644)

	stale := &EffectiveSettings{
		ApplicationMode:            ApplicationModeFindOnly,
		MinimumFitScore:            75,
		ScoringActive:              true,
		AutomaticSubmitClickActive: false,
		DaemonActive:               true,
	}
	if err := AcknowledgeActiveSettings(stale, ActiveSettingsPath); err != nil {
		t.Fatalf("AcknowledgeActiveSettings failed: %v", err)
	}

	holdAgentLock(t, AgentLockPath, 424242)

	eff, err := GetEffectiveSettings(pPath, oPath)
	if err != nil {
		t.Fatalf("GetEffectiveSettings failed: %v", err)
	}
	if eff.DaemonActive {
		t.Errorf("DaemonActive = true when active settings do not match effective settings")
	}
	if !eff.DaemonRunning {
		t.Errorf("DaemonRunning = false while lock is held")
	}
}
