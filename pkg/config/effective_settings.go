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
	DaemonActive               bool            `json:"daemon_active"`
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

	op, err := LoadOperatorSettings(operatorPath)
	if err != nil {
		return nil, err
	}

	if op == nil {
		op = &OperatorSettings{
			MinimumFitScore: 50,
			ApplicationMode: ApplicationModeFindOnly,
		}
		if p.AutoSubmit && !p.AutoSubmitClick && p.CopilotMode {
			op.ApplicationMode = ApplicationModeAssisted
		} else if p.AutoSubmit && p.AutoSubmitClick && !p.CopilotMode {
			op.ApplicationMode = ApplicationModeAutomatic
		}
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

	active, err := LoadActiveSettings("applications/active_operator_settings.json")
	if err == nil && active != nil {
		effective.DaemonActive = (active.ApplicationMode == effective.ApplicationMode && active.MinimumFitScore == effective.MinimumFitScore && active.ScoringActive == effective.ScoringActive && active.AutomaticSubmitClickActive == effective.AutomaticSubmitClickActive)
	}

	return effective, nil
}
