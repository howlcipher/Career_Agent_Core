package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type Profile struct {
	SalaryFloor     int      `yaml:"salary_floor"`
	RemoteOnly      bool     `yaml:"remote_only"`
	Role            string   `yaml:"role"`
	ExperienceYears int      `yaml:"experience_years"`
	Skills          []string `yaml:"skills"`
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

func (p *Profile) ValidateJob(salary int, remote bool) bool {
	if salary < p.SalaryFloor {
		return false
	}
	if p.RemoteOnly && !remote {
		return false
	}
	return true
}
