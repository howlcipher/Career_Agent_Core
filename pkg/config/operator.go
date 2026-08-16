package config

import (
	"bytes"
	"fmt"
	"gopkg.in/yaml.v3"
	"os"
	"path/filepath"
	"strings"
)

type ApplicationMode string

const (
	ApplicationModeFindOnly  ApplicationMode = "find_only"
	ApplicationModeAssisted  ApplicationMode = "assisted"
	ApplicationModeAutomatic ApplicationMode = "automatic"
)

// OperatorSettings carries explicit JSON tags as well as YAML ones. Without
// them the dashboard's own POST /api/operator-settings was rejected outright:
// the UI sends snake_case (cmd/dashboard/ui/src/types.ts) while Go's decoder
// matched only "ApplicationMode"/"MinimumFitScore", and the handler runs with
// DisallowUnknownFields, so every save failed with `unknown field
// "application_mode"`. Found while routing the geography selector through this
// endpoint (bugs.md #554); the selector cannot work until saving does.
type OperatorSettings struct {
	ApplicationMode ApplicationMode `yaml:"application_mode" json:"application_mode"`
	MinimumFitScore int             `yaml:"minimum_fit_score" json:"minimum_fit_score"`
	// AllowedCountries is the operator's geography selector, as ISO-3166
	// alpha-2 codes. It lives here rather than only in profile.yaml because
	// the dashboard already owns this file, and a second writer to
	// profile.yaml would be a second source of truth for the same policy.
	//
	// Unset and empty mean different things and must stay distinguishable:
	// unset means the selector has never been touched, so profile.yaml's own
	// allowed_countries stands; an explicit empty list is the "Worldwide"
	// choice, which switches the gate off. That is why this is a pointer --
	// with a plain slice, omitempty would drop the explicit Worldwide choice
	// on save and it would silently read back as "never configured"
	// (bugs.md #554).
	AllowedCountries *[]string `yaml:"allowed_countries,omitempty" json:"allowed_countries,omitempty"`
}

// Geography presets. Each preset enumerates its countries explicitly, in
// code, because "North America" is not a term with one obvious membership --
// leaving it to be inferred is how an operator ends up with a scope they did
// not choose. The UI shows exactly these lists.
var (
	// GeographyPresetUS is the United States only.
	GeographyPresetUS = []string{"US"}
	// GeographyPresetUSCA is the United States and Canada. This is the
	// operator's configured default.
	GeographyPresetUSCA = []string{"US", "CA"}
	// GeographyPresetNorthAmerica is the United States, Canada and Mexico.
	// Spelled out on purpose: it is deliberately NOT a synonym for US + CA.
	GeographyPresetNorthAmerica = []string{"US", "CA", "MX"}
	// GeographyPresetWorldwide is the empty allowlist, which disables the
	// gate. Represented as a non-nil empty slice so it is distinguishable
	// from "never configured".
	GeographyPresetWorldwide = []string{}
)

// DefaultOperatorSettings is the sole fail-closed fallback used whenever an
// operator_settings.yaml cannot be resolved (missing file, unreadable path).
// It is the single authoritative default for the whole application: no
// call site may substitute its own inference from legacy profile.yaml
// booleans (auto_submit/auto_submit_click/copilot_mode) for this value, since
// doing so previously let a missing settings file silently reactivate
// Automatic mode's final employer submit-click (bugs.md #535).
func DefaultOperatorSettings() *OperatorSettings {
	return &OperatorSettings{
		MinimumFitScore: 50,
		ApplicationMode: ApplicationModeFindOnly,
	}
}

func (s *OperatorSettings) Validate() error {
	if s.ApplicationMode != ApplicationModeFindOnly &&
		s.ApplicationMode != ApplicationModeAssisted &&
		s.ApplicationMode != ApplicationModeAutomatic {
		return fmt.Errorf("invalid application mode: %s", s.ApplicationMode)
	}
	if s.MinimumFitScore < 0 || s.MinimumFitScore > 100 {
		return fmt.Errorf("minimum_fit_score must be between 0 and 100")
	}
	if s.AllowedCountries != nil {
		for _, code := range *s.AllowedCountries {
			if len(code) != 2 || strings.ToUpper(code) != code {
				return fmt.Errorf("allowed_countries entries must be uppercase ISO-3166 alpha-2 codes, got %q", code)
			}
			// Reject a code the resolver cannot detect. Accepting one would
			// store an allowlist entry that silently never matches any
			// posting, which reads as a configured scope but enforces
			// nothing.
			if !IsKnownCountryCode(code) {
				return fmt.Errorf("allowed_countries entry %q is not a country this resolver can detect", code)
			}
		}
	}
	return nil
}

func LoadOperatorSettings(path string) (*OperatorSettings, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // Return nil if it doesn't exist
		}
		return nil, fmt.Errorf("failed to read operator settings: %w", err)
	}

	var settings OperatorSettings
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&settings); err != nil {
		return nil, fmt.Errorf("failed to parse operator settings: %w", err)
	}

	// reject trailing document
	var dummy interface{}
	if err := dec.Decode(&dummy); err == nil {
		return nil, fmt.Errorf("failed to parse operator settings: trailing document found")
	}

	if err := settings.Validate(); err != nil {
		return nil, err
	}

	return &settings, nil
}

func SaveOperatorSettings(path string, settings *OperatorSettings) error {
	if err := settings.Validate(); err != nil {
		return err
	}

	data, err := yaml.Marshal(settings)
	if err != nil {
		return fmt.Errorf("failed to marshal operator settings: %w", err)
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	tmpFile := path + ".tmp"
	if err := os.WriteFile(tmpFile, data, 0600); err != nil {
		return fmt.Errorf("failed to write temp file: %w", err)
	}

	if err := os.Rename(tmpFile, path); err != nil {
		os.Remove(tmpFile)
		return fmt.Errorf("failed to rename temp file: %w", err)
	}

	return nil
}

// ApplyOperatorSettings applies the canonical operator settings to the legacy Profile booleans.
//
// This is the single authoritative site for the operator-settings-missing
// fallback: op == nil (no operator_settings.yaml resolved) always falls
// back to DefaultOperatorSettings(), never to whatever auto_submit/
// auto_submit_click/copilot_mode happen to be sitting in profile.yaml. Prior
// to bugs.md #535, this branch was a no-op that left the legacy booleans
// exactly as loaded from profile.yaml, which meant a missing settings file
// could silently reactivate Automatic mode's final employer submit-click.
func ApplyOperatorSettings(p *Profile, op *OperatorSettings) error {
	if op == nil {
		op = DefaultOperatorSettings()
	}

	// Canonical mode dictates the pipeline behavior
	switch op.ApplicationMode {
	case ApplicationModeFindOnly:
		p.AutoSubmit = false
		p.AutoSubmitClick = false
		p.CopilotMode = false
		p.SkipScoring = false
	case ApplicationModeAssisted:
		p.AutoSubmit = true
		p.AutoSubmitClick = false
		p.CopilotMode = true
		p.SkipScoring = false
	case ApplicationModeAutomatic:
		p.AutoSubmit = true
		p.AutoSubmitClick = true
		p.CopilotMode = false
		p.SkipScoring = false
	default:
		return fmt.Errorf("invalid application mode: %s", op.ApplicationMode)
	}

	// Minimum fit score is now authoritative, though the threshold was hardcoded before.
	// We will inject it dynamically in the pipeline instead of hardcoding 50.
	p.MinimumFitScore = op.MinimumFitScore

	// The operator's geography selector wins over profile.yaml when it has
	// been set, so the dashboard is authoritative for this policy everywhere
	// it is enforced -- discovery, queue reconciliation and launch alike --
	// and no path can be screening against a different allowlist than the one
	// the operator is looking at (bugs.md #554). Untouched, profile.yaml
	// stands.
	if op.AllowedCountries != nil {
		p.AllowedCountries = append([]string(nil), *op.AllowedCountries...)
	}

	return nil
}
