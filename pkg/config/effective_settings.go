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
	// AllowedCountries is the geography allowlist actually being enforced,
	// after the operator's selector has been layered over profile.yaml. It is
	// the resolved value rather than the raw setting on purpose: the point of
	// showing it is to let the operator confirm what the queue is really
	// screened against (bugs.md #554). An empty list means no restriction.
	AllowedCountries []string `json:"allowed_countries"`
	// AvailableCountries is every country code the resolver can actually
	// detect, so the selector can only ever offer choices the gate can act on.
	AvailableCountries []string `json:"available_countries"`
	// GeographyPresets are the named scopes, each with its membership spelled
	// out, so "North America" is never left to the reader to interpret.
	GeographyPresets []GeographyPreset `json:"geography_presets"`
}

// GeographyPreset is one named scope offered by the dashboard selector.
type GeographyPreset struct {
	ID        string   `json:"id"`
	Label     string   `json:"label"`
	Countries []string `json:"countries"`
}

// GeographyPresets is the authoritative preset list shared by the API and UI.
// Membership lives here, in code, not in a label a human has to interpret.
func GeographyPresets() []GeographyPreset {
	return []GeographyPreset{
		{ID: "us", Label: "United States only", Countries: GeographyPresetUS},
		{ID: "us_ca", Label: "United States + Canada", Countries: GeographyPresetUSCA},
		{ID: "north_america", Label: "North America (US, Canada, Mexico)", Countries: GeographyPresetNorthAmerica},
		{ID: "worldwide", Label: "Worldwide (no restriction)", Countries: GeographyPresetWorldwide},
	}
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
		// Read back off the profile, after ApplyOperatorSettings has layered
		// the selector over it, so this reports what is enforced rather than
		// what was requested.
		AllowedCountries: p.AllowedCountries,
	}
	if effective.AllowedCountries == nil {
		effective.AllowedCountries = []string{}
	}
	effective.AvailableCountries = KnownCountryCodes()
	effective.GeographyPresets = GeographyPresets()

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
