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
	Street      string `yaml:"street"`
	City        string `yaml:"city"`
	State       string `yaml:"state"`
	FullState   string `yaml:"full_state"`
	Zip         string `yaml:"zip"`
	Country     string `yaml:"country"`
	FullCountry string `yaml:"full_country"`

	// Application facts. Every field here exists to remove a guess: whatever
	// is set is handed to the model as fact, so it never has to infer an
	// answer a form is asking for. All optional -- anything blank is simply
	// omitted, and the model falls back to reasoning or to declining.
	Links      Links       `yaml:"links"`
	Work       WorkFacts   `yaml:"work"`
	Education  []Education `yaml:"education"`
	Experience []Job       `yaml:"experience"`

	EEO EEO `yaml:"eeo"`
}

// Links holds the profile URLs application forms ask for by name.
type Links struct {
	LinkedIn  string `yaml:"linkedin"`
	GitHub    string `yaml:"github"`
	Portfolio string `yaml:"portfolio"`
	Website   string `yaml:"website"`
	Twitter   string `yaml:"twitter"`
}

// WorkFacts holds the recurring screening answers.
//
// The legally-loaded ones (authorization, sponsorship, clearance, criminal
// history) are deliberately left for the user to fill in: they are
// attestations on a real job application, and a plausible-looking guess is
// exactly the wrong thing for an agent to supply.
type WorkFacts struct {
	AuthorizedToWorkUS  string `yaml:"authorized_to_work_us"`
	RequiresSponsorship string `yaml:"requires_sponsorship"`
	VisaStatus          string `yaml:"visa_status"`
	SecurityClearance   string `yaml:"security_clearance"`
	CriminalHistory     string `yaml:"criminal_history"`
	Over18              string `yaml:"over_18"`

	CurrentEmployer    string `yaml:"current_employer"`
	CurrentTitle       string `yaml:"current_title"`
	YearsExperience    string `yaml:"years_experience"`
	DesiredSalary      string `yaml:"desired_salary"`
	NoticePeriod       string `yaml:"notice_period"`
	EarliestStart      string `yaml:"earliest_start_date"`
	RemotePreference   string `yaml:"remote_preference"`
	WillingToRelocate  string `yaml:"willing_to_relocate"`
	WillingToTravel    string `yaml:"willing_to_travel"`
	HowDidYouHear      string `yaml:"how_did_you_hear"`
	PreviouslyEmployed string `yaml:"previously_employed_here"`
	ReferredBy         string `yaml:"referred_by"`
	Pronouns           string `yaml:"pronouns"`
	Languages          string `yaml:"languages"`
	DriversLicense     string `yaml:"drivers_license"`
	Certifications     string `yaml:"certifications"`
}

// Education is one school entry.
type Education struct {
	School       string `yaml:"school"`
	Degree       string `yaml:"degree"`
	FieldOfStudy string `yaml:"field_of_study"`
	StartYear    string `yaml:"start_year"`
	EndYear      string `yaml:"end_year"`
	Status       string `yaml:"status"`
}

// Job is one employment-history entry.
type Job struct {
	Employer  string `yaml:"employer"`
	Title     string `yaml:"title"`
	Location  string `yaml:"location"`
	StartDate string `yaml:"start_date"`
	EndDate   string `yaml:"end_date"`
}

// ApplicationFacts renders every configured fact as prompt context.
//
// The point is to convert questions the model would otherwise reason about
// into lookups. Blank fields are omitted entirely rather than rendered as
// empty, so an unset field reads as "not provided" instead of as an answer.
func (p PII) ApplicationFacts() string {
	var b strings.Builder
	b.WriteString("Known applicant facts (use these EXACT values verbatim when a form asks for them; do not invent a different answer):\n")

	add := func(label, value string) {
		if v := strings.TrimSpace(value); v != "" {
			fmt.Fprintf(&b, "- %s: %s\n", label, v)
		}
	}

	add("Full name", strings.TrimSpace(p.FirstName+" "+p.LastName))
	add("Email", p.Email)
	add("Phone", p.Phone)
	add("Street address", p.Street)
	add("City", p.City)
	add("State", p.FullState)
	add("State abbreviation", p.State)
	add("ZIP/postal code", p.Zip)
	add("Country", p.FullCountry)

	add("LinkedIn", p.Links.LinkedIn)
	add("GitHub", p.Links.GitHub)
	add("Portfolio", p.Links.Portfolio)
	add("Website", p.Links.Website)
	add("Twitter/X", p.Links.Twitter)

	w := p.Work
	add("Authorized to work in the US", w.AuthorizedToWorkUS)
	add("Requires visa sponsorship", w.RequiresSponsorship)
	add("Visa status", w.VisaStatus)
	add("Security clearance", w.SecurityClearance)
	add("Criminal history", w.CriminalHistory)
	add("Over 18", w.Over18)
	add("Current employer", w.CurrentEmployer)
	add("Current job title", w.CurrentTitle)
	add("Years of experience", w.YearsExperience)
	add("Desired salary", w.DesiredSalary)
	add("Notice period", w.NoticePeriod)
	add("Earliest start date", w.EarliestStart)
	add("Remote work preference", w.RemotePreference)
	add("Willing to relocate", w.WillingToRelocate)
	add("Willing to travel", w.WillingToTravel)
	add("How did you hear about us", w.HowDidYouHear)
	add("Previously employed here", w.PreviouslyEmployed)
	add("Referred by", w.ReferredBy)
	add("Pronouns", w.Pronouns)
	add("Languages", w.Languages)
	add("Driver's license", w.DriversLicense)
	add("Certifications", w.Certifications)

	// joinRange builds "start to end" while tolerating either side being blank.
	// Deliberately not strings.Trim with a "to"/"-" cutset: that treats the
	// argument as a *character set*, so Trim("Feb 2023 to Present", "to")
	// silently produced "Feb 2023 to Presen".
	joinRange := func(start, end, sep string) string {
		start, end = strings.TrimSpace(start), strings.TrimSpace(end)
		switch {
		case start != "" && end != "":
			return start + sep + end
		case start != "":
			return start
		default:
			return end
		}
	}
	nonEmpty := func(in ...string) []string {
		var out []string
		for _, s := range in {
			if s = strings.TrimSpace(s); s != "" {
				out = append(out, s)
			}
		}
		return out
	}

	for i, e := range p.Education {
		parts := nonEmpty(e.Degree, e.FieldOfStudy, e.School, e.Status, joinRange(e.StartYear, e.EndYear, "-"))
		if len(parts) > 0 {
			fmt.Fprintf(&b, "- Education %d: %s\n", i+1, strings.Join(parts, ", "))
		}
	}
	for i, j := range p.Experience {
		parts := nonEmpty(j.Title, j.Employer, j.Location, joinRange(j.StartDate, j.EndDate, " to "))
		if len(parts) > 0 {
			fmt.Fprintf(&b, "- Experience %d: %s\n", i+1, strings.Join(parts, ", "))
		}
	}

	b.WriteString("Anything not listed above was not provided: reason from the resume context, or choose the form's decline option. Never fabricate a value for it.\n")
	return b.String()
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

// CountrySearchCandidates returns the phrasings worth trying against a country
// picker, shortest first.
//
// improvements.md #28: same reasoning as LocationSearchCandidates -- which
// string a given country list matches is not knowable in advance ("USA" vs
// "United States of America" vs "United States"), so the caller tries each
// until one commits. Only values the user actually configured are used; no
// variant is invented.
func (p PII) CountrySearchCandidates() []string {
	var out []string
	seen := map[string]bool{}
	for _, c := range []string{p.Country, p.FullCountry} {
		c = strings.TrimSpace(c)
		if c == "" || seen[c] {
			continue
		}
		seen[c] = true
		out = append(out, c)
	}
	return out
}

// LocationMustContain returns the tokens an accepted location option has to
// contain.
//
// bugs.md #79: Greenhouse's geocoder returns "Macomb, Illinois, United States"
// as the first hit for "Macomb" even though the configured address is in
// Michigan. Requiring the state token means a near-miss is rejected outright
// rather than filed on a real application.
func (p PII) LocationMustContain() []string {
	var out []string
	if city := strings.Fields(strings.TrimSpace(p.City)); len(city) > 0 {
		out = append(out, city[0])
	}
	// Prefer the spelled-out state: option labels read "Macomb, Illinois,
	// United States", not "MI".
	if fs := strings.TrimSpace(p.FullState); fs != "" {
		out = append(out, fs)
	} else if st := strings.TrimSpace(p.State); st != "" {
		out = append(out, st)
	}
	return out
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
	Gender            string `yaml:"gender"`
	RaceEthnicity     string `yaml:"race_ethnicity"`
	VeteranStatus     string `yaml:"veteran_status"`
	DisabilityStatus  string `yaml:"disability_status"`
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
