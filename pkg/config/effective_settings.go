package config

import (
	"errors"
	"fmt"
	"os"
)

type EffectiveSettings struct {
	ApplicationMode            ApplicationMode `json:"application_mode"`
	MinimumFitScore            int             `json:"minimum_fit_score"`
	ScoringActive              bool            `json:"scoring_active"`
	AutomaticSubmitClickActive bool            `json:"automatic_submit_click_active"`
	// DaemonActive means the currently running daemon (if any) has acknowledged
	// settings that match the effective settings. It is false both when no daemon
	// is running and when a running daemon is still operating under different
	// settings.
	DaemonActive bool `json:"daemon_active"`
	// DaemonRunning is true exactly when an agent process currently holds the
	// single-instance lock, independent of whether its acknowledged settings
	// match the current effective settings.
	DaemonRunning bool `json:"daemon_running"`
}

func GetEffectiveSettings(profilePath string, operatorPath string) (*EffectiveSettings, error) {
	p, err := LoadProfile(profilePath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("failed to load profile: %w", err)
	}

	if p == nil {
		p = &Profile{}
	} else {
		// Clone profile to safely apply operator settings
		cloned := *p
		p = &cloned
	}

	// A missing or unreadable operator_settings.yaml resolves to
	// DefaultOperatorSettings() (find_only) via ApplyOperatorSettings below --
	// never inferred from legacy profile.yaml booleans (bugs.md #535). A
	// corrupt or invalid settings file still fails closed via the error
	// return from LoadOperatorSettings.
	op, err := LoadOperatorSettings(operatorPath)
	if err != nil {
		return nil, err
	}
	if op == nil {
		op = DefaultOperatorSettings()
	}

	if err := ApplyOperatorSettings(p, op); err != nil {
		return nil, err
	}

	effective := &EffectiveSettings{
		ApplicationMode:            op.ApplicationMode,
		MinimumFitScore:            op.MinimumFitScore,
		ScoringActive:              !p.SkipScoring,
		AutomaticSubmitClickActive: p.AutoSubmitClick,
	}

	acknowledged := false
	active, err := LoadActiveSettings(ActiveSettingsPath)
	if err == nil && active != nil {
		acknowledged = (active.ApplicationMode == effective.ApplicationMode && active.MinimumFitScore == effective.MinimumFitScore && active.ScoringActive == effective.ScoringActive && active.AutomaticSubmitClickActive == effective.AutomaticSubmitClickActive)
	}

	_, alive, _ := IsAgentAlive(AgentLockPath)
	effective.DaemonRunning = alive
	effective.DaemonActive = acknowledged && alive

	return effective, nil
}
