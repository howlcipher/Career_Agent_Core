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

	// Structured address parts (improvements.md #28). Address is free text and
	// deliberately not parsed: Greenhouse's Location field is a geocoded
	// autocomplete, and a wrong guess there is worse than leaving it blank for
	// the validation-retry loop to handle. All optional -- blank fields are
	// skipped exactly like the EEO ones.
	Street    string `yaml:"street"`
	City      string `yaml:"city"`
	State     string `yaml:"state"`
	FullState string `yaml:"full_state"`
	Zip       string `yaml:"zip"`
	Country   string `yaml:"country"`

	EEO EEO `yaml:"eeo"`
}

// UnmarshalYAML lower-cases every mapping key before decoding, so `City`,
// `city` and `CITY` all bind to the same field.
//
// improvements.md #28: gopkg.in/yaml.v3 matches keys case-*sensitively*
// (verified empirically -- `City:` silently leaves a `yaml:"city"` field
// empty, with no error). pii.yaml is hand-maintained real personal data, so a
// casing slip silently dropping an address field is a genuinely bad failure
// mode: everything still loads, and the only symptom is a form field left
// blank much later.
func (p *PII) UnmarshalYAML(value *yaml.Node) error {
	lowerMappingKeys(value)
	type plain PII
	var tmp plain
	if err := value.Decode(&tmp); err != nil {
		return err
	}
	*p = PII(tmp)
	return nil
}

func lowerMappingKeys(n *yaml.Node) {
	if n == nil {
		return
	}
	if n.Kind == yaml.MappingNode {
		for i := 0; i+1 < len(n.Content); i += 2 {
			n.Content[i].Value = strings.ToLower(n.Content[i].Value)
		}
	}
	for _, c := range n.Content {
		lowerMappingKeys(c)
	}
}

// LocationSearchCandidates returns the strings worth trying against a geocoded
// location autocomplete, best guess first.
//
// improvements.md #28: which phrasing a geocoder accepts is not knowable in
// advance ("Macomb Township, MI" vs "Macomb, MI" vs the full state name), so
// the caller tries each until one actually commits -- verifiable since bugs.md
// #74/#76 made "did the selection commit" observable. Returns nil when no city
// is configured, so the field is left to the validation-retry loop.
func (p PII) LocationSearchCandidates() []string {
	city := strings.TrimSpace(p.City)
	if city == "" {
		return nil
	}
	var out []string
	seen := map[string]bool{}
	add := func(s string) {
		s = strings.TrimSpace(s)
		if s == "" || seen[s] {
			return
		}
		seen[s] = true
		out = append(out, s)
	}
	if st := strings.TrimSpace(p.State); st != "" {
		add(city + ", " + strings.ToUpper(st))
	}
	if fs := strings.TrimSpace(p.FullState); fs != "" {
		add(city + ", " + fs)
	}
	add(city)
	return out
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
