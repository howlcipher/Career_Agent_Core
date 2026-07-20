package config

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

type PII struct {
	FirstName string `yaml:"first_name"`
	LastName  string `yaml:"last_name"`
	Email     string `yaml:"email"`
	Phone     string `yaml:"phone"`
	DOB       string `yaml:"dob"`
	Address   string `yaml:"address"`
	EEO       EEO    `yaml:"eeo"`
}

// EEO holds voluntary equal-employment-opportunity self-identification
// answers. All fields are optional: leave any blank to have the agent
// answer "Decline to answer" for that question instead of guessing.
type EEO struct {
	Gender          string `yaml:"gender"`
	RaceEthnicity   string `yaml:"race_ethnicity"`
	VeteranStatus   string `yaml:"veteran_status"`
	DisabilityStatus string `yaml:"disability_status"`
	SexualOrientation string `yaml:"sexual_orientation"`
}

// Summary renders the configured EEO answers as prompt context, and makes
// explicit that anything left blank must be declined rather than guessed.
func (e EEO) Summary() string {
	facts := []struct{ label, value string }{
		{"Gender", e.Gender},
		{"Race/Ethnicity", e.RaceEthnicity},
		{"Veteran status", e.VeteranStatus},
		{"Disability status", e.DisabilityStatus},
		{"Sexual orientation", e.SexualOrientation},
	}
	var known, declined []string
	for _, f := range facts {
		if strings.TrimSpace(f.value) != "" {
			known = append(known, fmt.Sprintf("%s: %s", f.label, f.value))
		} else {
			declined = append(declined, f.label)
		}
	}
	summary := "EEO / voluntary self-identification answers (use these EXACT values verbatim if a matching form field appears; never infer or guess a value for any of these categories):\n"
	if len(known) > 0 {
		summary += strings.Join(known, "; ") + ".\n"
	}
	if len(declined) > 0 {
		summary += "Not provided, so answer \"Decline to answer\" / \"Prefer not to say\" for: " + strings.Join(declined, ", ") + "."
	}
	return summary
}

func LoadPII(path string) (*PII, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read PII file: %w", err)
	}

	var p PII
	if err := yaml.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("failed to parse PII file: %w", err)
	}

	return &p, nil
}
