package config

import (
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
