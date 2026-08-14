package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/howlcipher/Career_Agent_Core/pkg/config"
	"github.com/howlcipher/Career_Agent_Core/pkg/security"
	"gopkg.in/yaml.v3"
)

// This file is the one place in this repository that writes pii.yaml over HTTP,
// and the risk is stated here rather than discovered later.
//
// pii.yaml holds the operator's real personal details. It is listed in
// security.privateWorkspaceFiles, kept at 0600, and delegated agents are
// forbidden from reading it. Adding a write path to it is a deliberate
// trade: the alternative is a Knowledge Center that can tell the operator
// "eleven queued applications want a phone number you have not configured" and
// then has nothing to offer but "go and edit a YAML file". The operator asked
// for the editable version, and these are the constraints it is built under.
//
//   - Only the fields config.PII declares are accepted. DisallowUnknownFields
//     means a request cannot introduce arbitrary YAML keys.
//   - Read-modify-write, so anything in the file this struct does not model
//     survives a save.
//   - A timestamped backup at 0600 is written before every overwrite.
//   - The new file is written to a temporary path with security.PrivateFileMode
//     and renamed over the original, so a failure mid-write cannot leave a
//     truncated pii.yaml.
//   - No value is ever logged. Not on success, not on failure, not in an error
//     returned to the client.
//
// Two things this does not do, worth knowing before relying on it:
//
//   - **Comments in pii.yaml do not survive a save.** The file is re-marshalled
//     from the struct. That is what the backup is for.
//   - **requireSameOrigin is not authentication.** It admits requests carrying
//     none of Sec-Fetch-Site, Origin or Referer, so any local process can call
//     this — exactly as it can call every other mutating dashboard route. What
//     is new is that the set of things a local process can do now includes
//     rewriting pii.yaml.

// profileField is one editable fact. Sending values and their metadata together
// lets the UI render sections and explain what each fact unlocks without
// keeping its own copy of the schema.
type profileField struct {
	Key     string `json:"key"`
	Label   string `json:"label"`
	Value   string `json:"value"`
	Section string `json:"section"`
	// Sensitive marks a field the UI masks by default. It changes nothing about
	// storage — every field here is equally private — only what is shown on a
	// screen someone might be sitting next to.
	Sensitive bool `json:"sensitive,omitempty"`
	// Note explains, in the operator's terms, what this fact is for.
	Note string `json:"note,omitempty"`
}

// Profile sections, in the order the operator reads them.
const (
	sectionContact     = "Contact"
	sectionLocation    = "Location"
	sectionWorkStatus  = "Work status"
	sectionExperience  = "Experience"
	sectionPreferences = "Job preferences"
	sectionLinks       = "Links"
)

// profileSchema is the single description of every editable fact: how to read
// it, how to write it, and how to explain it. One table rather than three
// parallel switch statements, so a field cannot be readable and not writable.
var profileSchema = []struct {
	Key       string
	Label     string
	Section   string
	Sensitive bool
	Note      string
	Get       func(*config.PII) string
	Set       func(*config.PII, string)
}{
	{Key: "first_name", Label: "First name", Section: sectionContact,
		Get: func(p *config.PII) string { return p.FirstName },
		Set: func(p *config.PII, v string) { p.FirstName = v }},
	{Key: "last_name", Label: "Last name", Section: sectionContact,
		Get: func(p *config.PII) string { return p.LastName },
		Set: func(p *config.PII, v string) { p.LastName = v }},
	{Key: "email", Label: "Email", Section: sectionContact, Sensitive: true,
		Get: func(p *config.PII) string { return p.Email },
		Set: func(p *config.PII, v string) { p.Email = v }},
	{Key: "phone", Label: "Phone", Section: sectionContact, Sensitive: true,
		Get: func(p *config.PII) string { return p.Phone },
		Set: func(p *config.PII, v string) { p.Phone = v }},

	{Key: "street", Label: "Street", Section: sectionLocation, Sensitive: true,
		Get: func(p *config.PII) string { return p.Street },
		Set: func(p *config.PII, v string) { p.Street = v }},
	{Key: "city", Label: "City", Section: sectionLocation,
		Get: func(p *config.PII) string { return p.City },
		Set: func(p *config.PII, v string) { p.City = v }},
	{Key: "state", Label: "State or province (abbreviation)", Section: sectionLocation,
		Get: func(p *config.PII) string { return p.State },
		Set: func(p *config.PII, v string) { p.State = v }},
	{Key: "full_state", Label: "State or province (full name)", Section: sectionLocation,
		Note: "Some forms want the abbreviation and some want the full name, so both are kept.",
		Get:  func(p *config.PII) string { return p.FullState },
		Set:  func(p *config.PII, v string) { p.FullState = v }},
	{Key: "zip", Label: "Postal code", Section: sectionLocation,
		Get: func(p *config.PII) string { return p.Zip },
		Set: func(p *config.PII, v string) { p.Zip = v }},
	{Key: "country", Label: "Country (abbreviation)", Section: sectionLocation,
		Get: func(p *config.PII) string { return p.Country },
		Set: func(p *config.PII, v string) { p.Country = v }},
	{Key: "full_country", Label: "Country (full name)", Section: sectionLocation,
		Get: func(p *config.PII) string { return p.FullCountry },
		Set: func(p *config.PII, v string) { p.FullCountry = v }},

	{Key: "work.authorized_to_work_us", Label: "Authorized to work in the US", Section: sectionWorkStatus,
		Note: "A declaration. Career Agent offers this as a suggestion and never answers it for you.",
		Get:  func(p *config.PII) string { return p.Work.AuthorizedToWorkUS },
		Set:  func(p *config.PII, v string) { p.Work.AuthorizedToWorkUS = v }},
	{Key: "work.requires_sponsorship", Label: "Requires visa sponsorship", Section: sectionWorkStatus,
		Note: "A declaration. Suggested, never auto-filled.",
		Get:  func(p *config.PII) string { return p.Work.RequiresSponsorship },
		Set:  func(p *config.PII, v string) { p.Work.RequiresSponsorship = v }},
	{Key: "work.security_clearance", Label: "Security clearance", Section: sectionWorkStatus,
		Note: "A declaration. Suggested, never auto-filled.",
		Get:  func(p *config.PII) string { return p.Work.SecurityClearance },
		Set:  func(p *config.PII, v string) { p.Work.SecurityClearance = v }},
	{Key: "work.criminal_history", Label: "Criminal history", Section: sectionWorkStatus, Sensitive: true,
		Note: "A declaration. Suggested, never auto-filled.",
		Get:  func(p *config.PII) string { return p.Work.CriminalHistory },
		Set:  func(p *config.PII, v string) { p.Work.CriminalHistory = v }},
	{Key: "work.over_18", Label: "At least 18 years old", Section: sectionWorkStatus,
		Get: func(p *config.PII) string { return p.Work.Over18 },
		Set: func(p *config.PII, v string) { p.Work.Over18 = v }},

	{Key: "work.current_title", Label: "Current job title", Section: sectionExperience,
		Get: func(p *config.PII) string { return p.Work.CurrentTitle },
		Set: func(p *config.PII, v string) { p.Work.CurrentTitle = v }},
	{Key: "work.current_employer", Label: "Current employer", Section: sectionExperience,
		Get: func(p *config.PII) string { return p.Work.CurrentEmployer },
		Set: func(p *config.PII, v string) { p.Work.CurrentEmployer = v }},
	{Key: "work.years_experience", Label: "Total years of professional experience", Section: sectionExperience,
		Note: "Answers the general question only. A question about one technology is answered from an approved skill value, never from this.",
		Get:  func(p *config.PII) string { return p.Work.YearsExperience },
		Set:  func(p *config.PII, v string) { p.Work.YearsExperience = v }},
	{Key: "work.certifications", Label: "Certifications", Section: sectionExperience,
		Get: func(p *config.PII) string { return p.Work.Certifications },
		Set: func(p *config.PII, v string) { p.Work.Certifications = v }},

	{Key: "work.desired_salary", Label: "Desired salary", Section: sectionPreferences, Sensitive: true,
		Note: "A negotiating position, not a fact. Career Agent suggests it and asks.",
		Get:  func(p *config.PII) string { return p.Work.DesiredSalary },
		Set:  func(p *config.PII, v string) { p.Work.DesiredSalary = v }},
	{Key: "work.notice_period", Label: "Notice period", Section: sectionPreferences,
		Get: func(p *config.PII) string { return p.Work.NoticePeriod },
		Set: func(p *config.PII, v string) { p.Work.NoticePeriod = v }},
	{Key: "work.earliest_start_date", Label: "Earliest start date", Section: sectionPreferences,
		Note: "Leave blank to use two weeks from today.",
		Get:  func(p *config.PII) string { return p.Work.EarliestStart },
		Set:  func(p *config.PII, v string) { p.Work.EarliestStart = v }},
	{Key: "work.remote_preference", Label: "Remote preference", Section: sectionPreferences,
		Get: func(p *config.PII) string { return p.Work.RemotePreference },
		Set: func(p *config.PII, v string) { p.Work.RemotePreference = v }},
	{Key: "work.willing_to_relocate", Label: "Willing to relocate", Section: sectionPreferences,
		Get: func(p *config.PII) string { return p.Work.WillingToRelocate },
		Set: func(p *config.PII, v string) { p.Work.WillingToRelocate = v }},
	{Key: "work.willing_to_travel", Label: "Willing to travel", Section: sectionPreferences,
		Get: func(p *config.PII) string { return p.Work.WillingToTravel },
		Set: func(p *config.PII, v string) { p.Work.WillingToTravel = v }},
	{Key: "work.how_did_you_hear", Label: "How you heard about roles", Section: sectionPreferences,
		Get: func(p *config.PII) string { return p.Work.HowDidYouHear },
		Set: func(p *config.PII, v string) { p.Work.HowDidYouHear = v }},

	{Key: "links.linkedin", Label: "LinkedIn", Section: sectionLinks,
		Get: func(p *config.PII) string { return p.Links.LinkedIn },
		Set: func(p *config.PII, v string) { p.Links.LinkedIn = v }},
	{Key: "links.github", Label: "GitHub", Section: sectionLinks,
		Get: func(p *config.PII) string { return p.Links.GitHub },
		Set: func(p *config.PII, v string) { p.Links.GitHub = v }},
	{Key: "links.portfolio", Label: "Portfolio", Section: sectionLinks,
		Get: func(p *config.PII) string { return p.Links.Portfolio },
		Set: func(p *config.PII, v string) { p.Links.Portfolio = v }},
	{Key: "links.website", Label: "Website", Section: sectionLinks,
		Get: func(p *config.PII) string { return p.Links.Website },
		Set: func(p *config.PII, v string) { p.Links.Website = v }},
}

// serveKnowledgeProfile reads and writes the operator's configured details.
func serveKnowledgeProfile(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		pii, err := config.LoadPII(piiPath)
		if err != nil {
			// A first-run installation has no pii.yaml. Rendering an empty form
			// is the right answer; refusing to render is not.
			log.Printf("serveKnowledgeProfile: configured details unavailable: %v", err)
			pii = &config.PII{}
		}
		fields := make([]profileField, 0, len(profileSchema))
		for _, entry := range profileSchema {
			fields = append(fields, profileField{
				Key: entry.Key, Label: entry.Label, Section: entry.Section,
				Sensitive: entry.Sensitive, Note: entry.Note, Value: entry.Get(pii),
			})
		}
		writeJSON(w, map[string]any{
			"sections": []string{sectionContact, sectionLocation, sectionWorkStatus,
				sectionExperience, sectionPreferences, sectionLinks},
			"fields": fields,
			"path":   piiPath,
		})

	case http.MethodPost:
		var request struct {
			Fields map[string]string `json:"fields"`
		}
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxProfileRequestBytes))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&request); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		if len(request.Fields) == 0 {
			http.Error(w, "no details were provided", http.StatusBadRequest)
			return
		}
		changed, err := saveProfile(request.Fields)
		if err != nil {
			// The error is this package's own wording. A filesystem error's text
			// can contain a path, and a YAML error can quote the line it failed
			// on -- which here is somebody's home address.
			log.Printf("serveKnowledgeProfile: could not save configured details: %v", err)
			http.Error(w, "your details could not be saved", http.StatusConflict)
			return
		}
		// A count, never the details themselves.
		log.Printf("Configured details updated: %d field(s) set.", changed)

		// What the operator just supplied may answer questions the queue was
		// waiting on, so say so rather than making them go and look.
		resolved := 0
		if service, err := openKnowledgeService(); err == nil {
			if report, err := service.ReEvaluate(time.Now().UTC()); err == nil {
				resolved = report.Resolved
			}
		}
		writeJSON(w, map[string]any{"status": "saved", "fields_set": changed, "questions_resolved": resolved})

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// unknownProfileFieldError names a key the schema does not have. The key is
// echoed because the client supplied it and it is a field name, not a value.
type unknownProfileFieldError struct{ key string }

func (e *unknownProfileFieldError) Error() string {
	return fmt.Sprintf("unknown detail %q", e.key)
}

// saveProfile applies a patch to pii.yaml and returns how many fields ended up
// with a value.
//
// Read-modify-write rather than write-what-was-sent: a client that knows about
// four fields must not silently erase the twenty it does not.
func saveProfile(patch map[string]string) (int, error) {
	byKey := map[string]int{}
	for index, entry := range profileSchema {
		byKey[entry.Key] = index
	}
	for key := range patch {
		if _, known := byKey[key]; !known {
			return 0, &unknownProfileFieldError{key: key}
		}
	}

	pii, err := config.LoadPII(piiPath)
	if err != nil {
		// Missing is fine — that is a first run. Malformed is not: overwriting a
		// file we could not parse would destroy details we cannot see.
		if !errors.Is(err, os.ErrNotExist) && !strings.Contains(err.Error(), "no such file") {
			return 0, fmt.Errorf("existing details could not be read")
		}
		pii = &config.PII{}
	}
	for key, value := range patch {
		profileSchema[byKey[key]].Set(pii, strings.TrimSpace(value))
	}

	encoded, err := yaml.Marshal(pii)
	if err != nil {
		return 0, fmt.Errorf("details could not be encoded")
	}
	if err := writePrivateFileAtomically(piiPath, encoded); err != nil {
		return 0, err
	}
	// Re-assert the workspace's permissions after the rename, so a fresh file
	// cannot end up looser than the one it replaced.
	if err := security.RepairPrivatePaths("."); err != nil {
		log.Printf("serveKnowledgeProfile: could not re-assert private permissions: %v", err)
	}

	set := 0
	for _, entry := range profileSchema {
		if strings.TrimSpace(entry.Get(pii)) != "" {
			set++
		}
	}
	return set, nil
}

// writePrivateFileAtomically backs up the existing file, writes the new content
// to a temporary file in the same directory at 0600, and renames it into place.
//
// Same directory matters: rename is only atomic within a filesystem, and a
// temporary file elsewhere would degrade to a copy, which is the truncation
// this function exists to prevent.
func writePrivateFileAtomically(path string, content []byte) error {
	directory := filepath.Dir(path)
	if existing, err := os.ReadFile(path); err == nil {
		backup := fmt.Sprintf("%s.%s.bak", path, time.Now().UTC().Format("20060102-150405"))
		if err := os.WriteFile(backup, existing, security.PrivateFileMode); err != nil {
			return fmt.Errorf("previous details could not be backed up")
		}
		// Every backup holds the same real personal data as the file itself, so
		// they are bounded rather than left to accumulate: an unbounded pile of
		// copies of somebody's address and phone number is a worse outcome than
		// losing the tenth-oldest one. Failing to prune is not worth failing the
		// save over -- the new file is already written correctly.
		if err := pruneProfileBackups(path); err != nil {
			log.Printf("serveKnowledgeProfile: could not prune old backups: %v", err)
		}
	}
	temporary, err := os.CreateTemp(directory, filepath.Base(path)+".*.tmp")
	if err != nil {
		return fmt.Errorf("details could not be staged")
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)

	if err := temporary.Chmod(security.PrivateFileMode); err != nil {
		temporary.Close()
		return fmt.Errorf("details could not be protected")
	}
	if _, err := temporary.Write(content); err != nil {
		temporary.Close()
		return fmt.Errorf("details could not be written")
	}
	// Flush to disk before the rename, so a crash between the two cannot leave
	// the new name pointing at an empty file.
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return fmt.Errorf("details could not be flushed")
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("details could not be written")
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("details could not be saved")
	}
	return nil
}

// maxProfileBackups is how many previous versions are kept beside pii.yaml.
// Enough to recover from a mistaken save or two, few enough that copies of the
// operator's personal data do not pile up unbounded.
const maxProfileBackups = 5

// pruneProfileBackups deletes all but the newest maxProfileBackups backups.
//
// Names are timestamped in a sortable format, so lexical order is chronological
// order and no file has to be stat-ed to rank it.
func pruneProfileBackups(path string) error {
	pattern := path + ".*.bak"
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return err
	}
	if len(matches) <= maxProfileBackups {
		return nil
	}
	sort.Strings(matches)
	for _, stale := range matches[:len(matches)-maxProfileBackups] {
		if err := os.Remove(stale); err != nil {
			return err
		}
	}
	return nil
}
