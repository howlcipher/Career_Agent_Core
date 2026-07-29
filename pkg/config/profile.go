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
	// UseMasterCoverLetter switches the pipeline from generating a tailored
	// cover letter per application to reusing one static, job-agnostic letter
	// (master_cover_letter.txt, mirroring master_resume.pdf) for every job.
	//
	// Opt in on purpose: the zero value is false, so an existing profile.yaml
	// that has never heard of this field keeps the per-job tailoring behavior
	// it had before the field existed.
	//
	// When true, cmd/agent skips the ProcessJobApplication LLM call outright
	// rather than generating and discarding a letter. That call is the single
	// most expensive step in the pipeline (measured live at 15-20+ minutes per
	// job against this machine's CPU-only Ollama), and it is one combined call
	// producing resume + cover letter + interview prep, so skipping it also
	// stops per-job resume.md and interview_prep.md. Neither is a loss for
	// auto-submitted jobs: the file uploaded to every ATS is master_resume.pdf
	// (hardcoded), and interview prep was only ever a saved reference. Jobs
	// routed to MANUAL_REQUIRED do get master documents instead of tailored
	// ones as a result, which is the real tradeoff of turning this on.
	UseMasterCoverLetter bool `yaml:"use_master_cover_letter"`
	// MasterCoverLetterPath points at the static letter UseMasterCoverLetter
	// sends. Both a PDF (uploaded as-is to file inputs, text extracted for
	// paste-style fields) and a plain-text file are supported. Optional:
	// defaults to master_cover_letter.txt when unset, so the letter can be
	// swapped for a differently-named file by editing profile.yaml alone,
	// with no rebuild.
	MasterCoverLetterPath string `yaml:"master_cover_letter_path"`
	// SendCoverLetter controls whether a cover letter is attached to
	// applications at all. Set it to false to stop sending one without
	// removing any of the machinery: the letter file, the master-letter
	// toggle, and the upload/paste logic all stay in place and resume working
	// the moment it is set back to true.
	//
	// A pointer so an absent key can be told apart from an explicit false:
	// nil means "send", preserving the behavior of every profile written
	// before this field existed. Read it through ShouldSendCoverLetter rather
	// than dereferencing it directly.
	SendCoverLetter *bool `yaml:"send_cover_letter"`
	// SkipScoring allows bypassing the expensive fit-score LLM call. When true,
	// any job that passes keyword and metadata filters (salary, remote, role)
	// will automatically be assigned a score of 100 and proceed to application.
	// This is useful for high-volume pipelines where local inference takes too long.
	SkipScoring bool `yaml:"skip_scoring"`
}

// ShouldSendCoverLetter reports whether applications should include a cover
// letter, defaulting to true when send_cover_letter is not set.
func (p *Profile) ShouldSendCoverLetter() bool {
	return p.SendCoverLetter == nil || *p.SendCoverLetter
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
