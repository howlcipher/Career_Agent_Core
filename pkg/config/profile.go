package config

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

type Profile struct {
	SalaryFloor      int      `yaml:"salary_floor"`
	TargetComp       int      `yaml:"target_compensation"`
	RemoteOnly       bool     `yaml:"remote_only"`
	Roles            []string `yaml:"roles"`
	ExperienceYears  int      `yaml:"experience_years"`
	Skills           []string `yaml:"skills"`
	ExcludeCompanies []string `yaml:"exclude_companies"`
	AutoSubmit       bool     `yaml:"auto_submit"`
	AutoSubmitClick  bool     `yaml:"auto_submit_click"`
	HeadlessBrowser  bool     `yaml:"headless_browser"`
	CoverLetterTone  string   `yaml:"cover_letter_tone"`
}

func LoadProfile(path string) (*Profile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read profile: %w", err)
	}

	var p Profile
	if err := yaml.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("failed to parse profile: %w", err)
	}

	return &p, nil
}

func (p *Profile) ValidateJob(companyName string, salary int, remote bool) bool {
	if salary < p.SalaryFloor {
		return false
	}
	if p.RemoteOnly && !remote {
		return false
	}
	
	// Security check: Never apply to current/past employers
	nameLower := strings.ToLower(companyName)
	for _, excluded := range p.ExcludeCompanies {
		if strings.Contains(nameLower, strings.ToLower(excluded)) {
			fmt.Printf("Security Block: Skipping %s (Found in ExcludeCompanies blocklist)\n", companyName)
			return false
		}
	}
	
	return true
}
