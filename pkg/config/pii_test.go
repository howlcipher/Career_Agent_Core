package config

import (
	"os"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestLoadPII(t *testing.T) {
	yamlData := `
first_name: "John"
last_name: "Doe"
email: "john.doe@example.com"
phone: "+1234567890"
dob: "1990-01-01"
address: "123 Main St"
`
	tmpFile, err := os.CreateTemp("", "pii_*.yaml")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.Write([]byte(yamlData)); err != nil {
		t.Fatalf("Failed to write temp file: %v", err)
	}
	tmpFile.Close()

	pii, err := LoadPII(tmpFile.Name())
	if err != nil {
		t.Fatalf("LoadPII failed: %v", err)
	}

	if pii.FirstName != "John" {
		t.Errorf("Expected FirstName 'John', got '%s'", pii.FirstName)
	}
	if pii.LastName != "Doe" {
		t.Errorf("Expected LastName 'Doe', got '%s'", pii.LastName)
	}
	if pii.Email != "john.doe@example.com" {
		t.Errorf("Expected Email 'john.doe@example.com', got '%s'", pii.Email)
	}
	if pii.Phone != "+1234567890" {
		t.Errorf("Expected Phone '+1234567890', got '%s'", pii.Phone)
	}
	if pii.DOB != "1990-01-01" {
		t.Errorf("Expected DOB '1990-01-01', got '%s'", pii.DOB)
	}
	if pii.Address != "123 Main St" {
		t.Errorf("Expected Address '123 Main St', got '%s'", pii.Address)
	}
}

func TestLoadPII_InvalidFile(t *testing.T) {
	_, err := LoadPII("non_existent_file.yaml")
	if err == nil {
		t.Errorf("Expected error for non-existent file, got nil")
	}
}

func TestLoadPII_InvalidYAML(t *testing.T) {
	yamlData := `first_name: "John"
	invalid_yaml_here
	`
	tmpFile, err := os.CreateTemp("", "pii_invalid_*.yaml")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Write([]byte(yamlData))
	tmpFile.Close()

	_, err = LoadPII(tmpFile.Name())
	if err == nil {
		t.Errorf("Expected error for invalid yaml, got nil")
	}
}

// improvements.md #28: gopkg.in/yaml.v3 matches keys case-SENSITIVELY --
// verified empirically that `City:` silently leaves a `yaml:"city"` field
// empty, with no error at all. pii.yaml is hand-maintained real data, so a
// casing slip quietly dropping an address field is a bad failure mode: it
// loads fine and the only symptom is a form field left blank much later.
func TestPII_LoadsMixedCaseAddressKeys(t *testing.T) {
	var p PII
	// Deliberately mixed casing, matching how the keys were actually written.
	in := "first_name: \"A\"\nCity: \"Macomb Township\"\nState: \"Mi\"\nFull_state: \"Michigan\"\nzip: \"48042\"\nstreet : \"1 Main\"\n"
	if err := yaml.Unmarshal([]byte(in), &p); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if p.City != "Macomb Township" {
		t.Errorf("City = %q, want %q", p.City, "Macomb Township")
	}
	if p.State != "Mi" {
		t.Errorf("State = %q", p.State)
	}
	if p.FullState != "Michigan" {
		t.Errorf("FullState = %q", p.FullState)
	}
	if p.Zip != "48042" {
		t.Errorf("Zip = %q", p.Zip)
	}
	if p.Street != "1 Main" {
		t.Errorf("Street = %q (note the space before the colon in the source)", p.Street)
	}
	if p.FirstName != "A" {
		t.Errorf("FirstName = %q — already-lowercase keys must keep working", p.FirstName)
	}
}

func TestPII_LocationSearchCandidates(t *testing.T) {
	p := PII{City: "Macomb Township", State: "Mi", FullState: "Michigan"}
	got := p.LocationSearchCandidates()
	want := []string{"Macomb Township, MI", "Macomb Township, Michigan", "Macomb Township"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("candidate %d = %q, want %q", i, got[i], want[i])
		}
	}
}

// No city configured must mean "leave it to the retry loop", never a guess.
func TestPII_LocationSearchCandidates_EmptyWithoutCity(t *testing.T) {
	if got := (PII{State: "MI", FullState: "Michigan"}).LocationSearchCandidates(); got != nil {
		t.Errorf("expected no candidates without a city, got %v", got)
	}
}

// improvements.md #29: every fact configured here removes a guess the model
// would otherwise make on a real application form.
func TestApplicationFacts_RendersConfiguredFactsAndOmitsBlanks(t *testing.T) {
	p := PII{
		FirstName: "A", LastName: "B", City: "Macomb Township", FullState: "Michigan",
		Links: Links{GitHub: "https://github.com/x"},
		Work: WorkFacts{
			CurrentEmployer: "Acme",
			// AuthorizedToWorkUS deliberately blank.
		},
		Experience: []Job{{Title: "Engineer", Employer: "Acme", StartDate: "Feb 2023", EndDate: "Present"}},
		Education:  []Education{{Degree: "B.S.", School: "CSU", Status: "Completed"}},
	}
	got := p.ApplicationFacts()

	for _, want := range []string{
		"Full name: A B",
		"GitHub: https://github.com/x",
		"Current employer: Acme",
		"Experience 1: Engineer, Acme, Feb 2023 to Present",
		"Education 1: B.S., CSU, Completed",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	// A blank legal attestation must not render as an answer at all.
	if strings.Contains(got, "Authorized to work") {
		t.Error("a blank field must be omitted entirely, not rendered as empty")
	}
}

// A date range must survive verbatim. strings.Trim(s, "to") treats "to" as a
// character SET, which silently turned "Feb 2023 to Present" into
// "Feb 2023 to Presen" -- caught by eyeballing the real rendered output.
func TestApplicationFacts_DoesNotChewTheEndOfPresent(t *testing.T) {
	p := PII{Experience: []Job{{Employer: "Acme", StartDate: "Feb 2023", EndDate: "Present"}}}
	got := p.ApplicationFacts()
	if !strings.Contains(got, "Feb 2023 to Present") || strings.Contains(got, "Presen,") || strings.Contains(got, "Presen\n") {
		t.Errorf("date range mangled:\n%s", got)
	}
}

// Only one side of a range configured must not leave a dangling separator.
func TestApplicationFacts_HandlesAOneSidedDateRange(t *testing.T) {
	p := PII{Experience: []Job{{Employer: "Acme", StartDate: "Feb 2023"}}}
	if got := p.ApplicationFacts(); strings.Contains(got, "Feb 2023 to") {
		t.Errorf("dangling separator:\n%s", got)
	}
}

// bugs.md #82: a configured answer unblocks the form; a blank one must hold it
// back rather than let the model guess a legal declaration.
func TestMissingAttestations(t *testing.T) {
	all := []string{"work authorization", "visa sponsorship"}

	blank := PII{}
	if got := blank.MissingAttestations(all); len(got) != 2 {
		t.Errorf("expected both categories missing, got %v", got)
	}

	set := PII{Work: WorkFacts{AuthorizedToWorkUS: "Yes", RequiresSponsorship: "No"}}
	if got := set.MissingAttestations(all); len(got) != 0 {
		t.Errorf("expected none missing once configured, got %v", got)
	}

	// visa_status is an acceptable stand-in for the sponsorship question.
	partial := PII{Work: WorkFacts{AuthorizedToWorkUS: "Yes", VisaStatus: "U.S. Citizen"}}
	if got := partial.MissingAttestations(all); len(got) != 0 {
		t.Errorf("visa_status should satisfy the sponsorship question, got %v", got)
	}
}
