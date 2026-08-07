package config

import (
	"fmt"
	"os"
	"testing"
)

func TestLoadProfile(t *testing.T) {
	yamlData := `
salary_floor: 100000
target_compensation: 150000
remote_only: true
roles:
  - "Software Engineer"
  - "Backend Engineer"
experience_years: 5
skills:
  - "Go"
  - "Python"
exclude_companies:
  - "EvilCorp"
  - "BadCompany"
auto_submit: true
auto_submit_click: false
headless_browser: true
cover_letter_tone: "professional"
`
	tmpFile, err := os.CreateTemp("", "profile_*.yaml")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.Write([]byte(yamlData)); err != nil {
		t.Fatalf("Failed to write temp file: %v", err)
	}
	tmpFile.Close()

	profile, err := LoadProfile(tmpFile.Name())
	if err != nil {
		t.Fatalf("LoadProfile failed: %v", err)
	}

	if profile.SalaryFloor != 100000 {
		t.Errorf("Expected SalaryFloor 100000, got %d", profile.SalaryFloor)
	}
	if profile.TargetComp != 150000 {
		t.Errorf("Expected TargetComp 150000, got %d", profile.TargetComp)
	}
	if !profile.RemoteOnly {
		t.Errorf("Expected RemoteOnly to be true")
	}
	if len(profile.Roles) != 2 || profile.Roles[0] != "Software Engineer" || profile.Roles[1] != "Backend Engineer" {
		t.Errorf("Roles mismatch: %v", profile.Roles)
	}
	if profile.ExperienceYears != 5 {
		t.Errorf("Expected ExperienceYears 5, got %d", profile.ExperienceYears)
	}
	if len(profile.Skills) != 2 || profile.Skills[0] != "Go" || profile.Skills[1] != "Python" {
		t.Errorf("Skills mismatch: %v", profile.Skills)
	}
	if len(profile.ExcludeCompanies) != 2 || profile.ExcludeCompanies[0] != "EvilCorp" || profile.ExcludeCompanies[1] != "BadCompany" {
		t.Errorf("ExcludeCompanies mismatch: %v", profile.ExcludeCompanies)
	}
	if !profile.AutoSubmit {
		t.Errorf("Expected AutoSubmit to be true")
	}
	if profile.AutoSubmitClick {
		t.Errorf("Expected AutoSubmitClick to be false")
	}
	if !profile.HeadlessBrowser {
		t.Errorf("Expected HeadlessBrowser to be true")
	}
	if profile.CoverLetterTone != "professional" {
		t.Errorf("Expected CoverLetterTone 'professional', got '%s'", profile.CoverLetterTone)
	}
}

func TestLoadProfile_CoverLetterTones(t *testing.T) {
	yamlData := `
cover_letter_tone: "professional"
cover_letter_tones:
  - "confident and direct"
  - "warm and conversational"
`
	tmpFile, err := os.CreateTemp("", "profile_*.yaml")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	if _, err := tmpFile.Write([]byte(yamlData)); err != nil {
		t.Fatalf("Failed to write temp file: %v", err)
	}
	tmpFile.Close()

	profile, err := LoadProfile(tmpFile.Name())
	if err != nil {
		t.Fatalf("LoadProfile failed: %v", err)
	}
	if len(profile.CoverLetterTones) != 2 {
		t.Fatalf("expected 2 cover letter tone variants, got %d: %v", len(profile.CoverLetterTones), profile.CoverLetterTones)
	}

	// CoverLetterTone (singular) must remain untouched — it's still the
	// fallback for profiles that never opt into cover_letter_tones.
	if profile.CoverLetterTone != "professional" {
		t.Errorf("expected the singular CoverLetterTone to be unaffected, got %q", profile.CoverLetterTone)
	}
}

func TestLoadProfile_DuplicateCooldownDays(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		want    int
		wantErr bool
	}{
		{"absent remains disabled", "salary_floor: 1\n", 0, false},
		{"positive duration is accepted", "duplicate_cooldown_days: 30\n", 30, false},
		{"negative duration is rejected", "duplicate_cooldown_days: -1\n", 0, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			file, err := os.CreateTemp(t.TempDir(), "profile_*.yaml")
			if err != nil {
				t.Fatalf("create profile: %v", err)
			}
			if _, err := file.WriteString(tc.yaml); err != nil {
				t.Fatalf("write profile: %v", err)
			}
			if err := file.Close(); err != nil {
				t.Fatalf("close profile: %v", err)
			}
			profile, err := LoadProfile(file.Name())
			if tc.wantErr {
				if err == nil {
					t.Fatal("LoadProfile succeeded, want error")
				}
				return
			}
			if err != nil {
				t.Fatalf("LoadProfile: %v", err)
			}
			if profile.DuplicateCooldownDays != tc.want {
				t.Errorf("DuplicateCooldownDays = %d, want %d", profile.DuplicateCooldownDays, tc.want)
			}
		})
	}
}

func TestSelectToneVariant(t *testing.T) {
	if _, _, ok := SelectToneVariant(nil); ok {
		t.Error("expected ok=false for zero tones")
	}
	if _, _, ok := SelectToneVariant([]string{"only one"}); ok {
		t.Error("expected ok=false for a single tone — nothing to A/B test against")
	}

	tones := []string{"confident and direct", "warm and conversational", "formal"}
	seen := map[string]bool{}
	for i := 0; i < 50; i++ {
		label, tone, ok := SelectToneVariant(tones)
		if !ok {
			t.Fatal("expected ok=true for 3 tones")
		}
		found := false
		for j, want := range tones {
			if tone == want {
				found = true
				if label != fmt.Sprintf("variant_%d", j) {
					t.Errorf("expected label %q for tone %q, got %q", fmt.Sprintf("variant_%d", j), tone, label)
				}
			}
		}
		if !found {
			t.Errorf("returned tone %q is not one of the configured variants %v", tone, tones)
		}
		seen[label] = true
	}
	if len(seen) < 2 {
		t.Errorf("expected random selection to produce more than one distinct variant across 50 calls, got %v", seen)
	}
}

func TestValidateJob(t *testing.T) {
	profile := &Profile{
		SalaryFloor:      100000,
		RemoteOnly:       true,
		ExcludeCompanies: []string{"EvilCorp", "BadCompany"},
	}

	tests := []struct {
		name        string
		company     string
		salary      int
		remote      bool
		expectedRes bool
	}{
		{"Valid Job", "GoodCorp", 120000, true, true},
		{"Low Salary", "GoodCorp", 90000, true, false},
		{"Not Remote", "GoodCorp", 120000, false, false},
		{"Excluded Company Match", "EvilCorp", 120000, true, false},
		{"Excluded Company Case Insensitive", "evilcorp inc", 120000, true, false},
		{"Valid Job Exact Floor", "OkayCorp", 100000, true, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := profile.ValidateJob(tt.company, tt.salary, tt.remote)
			if res != tt.expectedRes {
				t.Errorf("ValidateJob(%s, %d, %v) = %v; expected %v", tt.company, tt.salary, tt.remote, res, tt.expectedRes)
			}
		})
	}
}

func TestLoadProfile_InvalidFile(t *testing.T) {
	_, err := LoadProfile("non_existent_profile.yaml")
	if err == nil {
		t.Errorf("Expected error for non-existent file, got nil")
	}
}

func TestValidateJob_NotRemoteOnly(t *testing.T) {
	profile := &Profile{
		SalaryFloor:      100000,
		RemoteOnly:       false,
		ExcludeCompanies: []string{"EvilCorp", "BadCompany"},
	}

	res := profile.ValidateJob("GoodCorp", 120000, false)
	if !res {
		t.Errorf("ValidateJob failed for non-remote job when RemoteOnly is false")
	}
}

func TestLoadProfile_MalformedYaml(t *testing.T) {
	yamlData := `salary_floor: 100000
	malformed_yaml_here
	`
	tmpFile, err := os.CreateTemp("", "profile_invalid_*.yaml")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Write([]byte(yamlData))
	tmpFile.Close()

	_, err = LoadProfile(tmpFile.Name())
	if err == nil {
		t.Errorf("Expected error for invalid yaml, got nil")
	}
}

func TestLoadProfile_UseMasterCoverLetter(t *testing.T) {
	// Opt-in semantics matter here: a profile that predates this field must
	// keep per-job tailoring, so an absent key has to load as false rather
	// than silently switching a live pipeline onto the static letter.
	tests := []struct {
		name string
		yaml string
		want bool
	}{
		{"absent key defaults to tailoring", "cover_letter_tone: \"professional\"\n", false},
		{"explicitly enabled", "use_master_cover_letter: true\n", true},
		{"explicitly disabled", "use_master_cover_letter: false\n", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpFile, err := os.CreateTemp("", "profile_*.yaml")
			if err != nil {
				t.Fatalf("Failed to create temp file: %v", err)
			}
			defer os.Remove(tmpFile.Name())
			if _, err := tmpFile.Write([]byte(tt.yaml)); err != nil {
				t.Fatalf("Failed to write temp file: %v", err)
			}
			tmpFile.Close()

			profile, err := LoadProfile(tmpFile.Name())
			if err != nil {
				t.Fatalf("LoadProfile failed: %v", err)
			}
			if profile.UseMasterCoverLetter != tt.want {
				t.Errorf("UseMasterCoverLetter = %v, want %v", profile.UseMasterCoverLetter, tt.want)
			}
		})
	}
}

// send_cover_letter must default to sending when the key is absent: a plain
// bool would have made the Go zero value mean "off", silently disabling cover
// letters for every profile written before the field existed.
func TestLoadProfile_SendCoverLetter(t *testing.T) {
	tests := []struct {
		name string
		yaml string
		want bool
	}{
		{"absent key still sends", "cover_letter_tone: \"professional\"\n", true},
		{"explicitly disabled", "send_cover_letter: false\n", false},
		{"explicitly enabled", "send_cover_letter: true\n", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpFile, err := os.CreateTemp("", "profile_*.yaml")
			if err != nil {
				t.Fatalf("Failed to create temp file: %v", err)
			}
			defer os.Remove(tmpFile.Name())
			if _, err := tmpFile.Write([]byte(tt.yaml)); err != nil {
				t.Fatalf("Failed to write temp file: %v", err)
			}
			tmpFile.Close()

			profile, err := LoadProfile(tmpFile.Name())
			if err != nil {
				t.Fatalf("LoadProfile failed: %v", err)
			}
			if got := profile.ShouldSendCoverLetter(); got != tt.want {
				t.Errorf("ShouldSendCoverLetter() = %v, want %v", got, tt.want)
			}
		})
	}
}

// A zero-value Profile (no YAML at all) must also default to sending.
func TestShouldSendCoverLetter_ZeroValueProfileSends(t *testing.T) {
	var p Profile
	if !p.ShouldSendCoverLetter() {
		t.Error("a zero-value Profile must default to sending a cover letter")
	}
}

func TestResolvedMasterCoverLetterPath(t *testing.T) {
	falseVal := false
	trueVal := true

	tests := []struct {
		name    string
		profile Profile
		want    string
	}{
		{
			name: "toggle off",
			profile: Profile{
				UseMasterCoverLetter: false,
			},
			want: "",
		},
		{
			name: "toggle on with explicit path",
			profile: Profile{
				UseMasterCoverLetter:  true,
				MasterCoverLetterPath: "custom_letter.pdf",
			},
			want: "custom_letter.pdf",
		},
		{
			name: "toggle on with empty path uses default",
			profile: Profile{
				UseMasterCoverLetter:  true,
				MasterCoverLetterPath: "",
			},
			want: DefaultMasterCoverLetterPath,
		},
		{
			name: "send_cover_letter false returns empty",
			profile: Profile{
				UseMasterCoverLetter:  true,
				MasterCoverLetterPath: "custom_letter.pdf",
				SendCoverLetter:       &falseVal,
			},
			want: "",
		},
		{
			name: "send_cover_letter true with toggle on",
			profile: Profile{
				UseMasterCoverLetter:  true,
				MasterCoverLetterPath: "custom_letter.pdf",
				SendCoverLetter:       &trueVal,
			},
			want: "custom_letter.pdf",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.profile.ResolvedMasterCoverLetterPath(); got != tt.want {
				t.Errorf("ResolvedMasterCoverLetterPath() = %q, want %q", got, tt.want)
			}
		})
	}
}
