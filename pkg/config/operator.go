package config

import (
	"bytes"
	"fmt"
	"gopkg.in/yaml.v3"
	"os"
	"path/filepath"
)

type ApplicationMode string

const (
	ApplicationModeFindOnly  ApplicationMode = "find_only"
	ApplicationModeAssisted  ApplicationMode = "assisted"
	ApplicationModeAutomatic ApplicationMode = "automatic"
)

type OperatorSettings struct {
	ApplicationMode ApplicationMode `yaml:"application_mode"`
	MinimumFitScore int             `yaml:"minimum_fit_score"`
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
func ApplyOperatorSettings(p *Profile, op *OperatorSettings) error {
	if op == nil {
		// Infer legacy mode
		// Preserve existing behavior, no rewrites
		return nil
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

	return nil
}
