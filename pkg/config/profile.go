package config

import (
	"fmt"
	"math/rand"
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
	// CoverLetterTones is an optional list of tone variants to A/B test
	// (improvements.md #13). When it has 2+ entries, SelectToneVariant picks
	// one at random per application and the choice is recorded in
	// job_funnel.tone_variant so eventual outcomes (pkg/storage's
	// GetConversionStatsByVariant) can be joined back against it. Left unset
	// by default — populating it with real tone variants is a personal-
	// branding decision for the user to make deliberately, not something to
	// infer or invent on their behalf. When unset or fewer than 2 entries,
	// CoverLetterTone (singular) is used for every application, unchanged
	// from before this field existed.
	CoverLetterTones []string `yaml:"cover_letter_tones"`
}

// SelectToneVariant picks a random entry from tones for A/B testing
// (improvements.md #13), returning a stable label ("variant_0", "variant_1",
// ...) alongside the tone text so the choice can be recorded via
// storage.UpdateToneVariant and later joined against its outcome. ok is
// false when there are fewer than 2 variants to choose between — the caller
// should fall back to Profile.CoverLetterTone (singular) in that case,
// exactly as before this feature existed.
//
// Random rather than round-robin: an unbiased split doesn't correlate with
// time-of-day or batch-order effects (e.g. every Greenhouse posting
// happening to land on the same variant because of how a session's queue was
// ordered), which would confound any later comparison between variants.
func SelectToneVariant(tones []string) (label, tone string, ok bool) {
	if len(tones) < 2 {
		return "", "", false
	}
	i := rand.Intn(len(tones))
	return fmt.Sprintf("variant_%d", i), tones[i], true
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
