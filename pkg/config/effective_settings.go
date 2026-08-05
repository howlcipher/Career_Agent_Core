package config

type EffectiveSettings struct {
	ApplicationMode            ApplicationMode `json:"application_mode"`
	MinimumFitScore            int             `json:"minimum_fit_score"`
	ScoringActive              bool            `json:"scoring_active"`
	AutomaticSubmitClickActive bool            `json:"automatic_submit_click_active"`
	DaemonActive               bool            `json:"daemon_active"`
}

func GetEffectiveSettings(profilePath string, operatorPath string) (*EffectiveSettings, error) {
	p, err := LoadProfile(profilePath)
	if err != nil && err.Error() != "profile not found" {
		// If profile not found, we can proceed with defaults if we want, but usually it exists.
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
		if p != nil {
			if p.AutoSubmit && !p.AutoSubmitClick && p.CopilotMode {
				op.ApplicationMode = ApplicationModeAssisted
			} else if p.AutoSubmit && p.AutoSubmitClick && !p.CopilotMode {
				op.ApplicationMode = ApplicationModeAutomatic
			}
		}
	}

	effective := &EffectiveSettings{
		ApplicationMode:            op.ApplicationMode,
		MinimumFitScore:            op.MinimumFitScore,
		ScoringActive:              true,
		AutomaticSubmitClickActive: false,
	}

	if p != nil {
		effective.ScoringActive = !p.SkipScoring
	}

	if effective.ApplicationMode == ApplicationModeAutomatic {
		effective.AutomaticSubmitClickActive = true
	}

	active, err := LoadActiveSettings("applications/active_operator_settings.json")
	if err == nil && active != nil {
		effective.DaemonActive = (active.ApplicationMode == effective.ApplicationMode && active.MinimumFitScore == effective.MinimumFitScore)
	}

	return effective, nil
}
