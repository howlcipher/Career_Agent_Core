package answers

import (
	"strings"

	"github.com/howlcipher/Career_Agent_Core/pkg/config"
)

// pattern is one curated recognizer for a question family every ATS asks in
// its own words.
//
// Matching is token-set based rather than substring based: RequireAll holds
// groups, and the question must contain at least one token from *every* group.
// That is what lets "Are you legally authorized to work in the United States?",
// "Are you authorized to work in the US?" and "Do you currently have
// authorization to work in the United States?" all reach the same answer
// without any stemming, synonym expansion, or model call — the first group
// covers authorized/authorization, the second covers work/employment, and the
// stop-word pass has already removed everything the three phrasings disagree
// about.
//
// Deny exists because these families overlap in vocabulary. "Will you now or in
// the future require sponsorship for employment authorization?" contains both
// the authorization tokens and the sponsorship tokens, and answering it with
// the work-authorization answer would put a flatly wrong attestation on a real
// application.
type pattern struct {
	ID          string
	RequireAll  [][]string
	Deny        []string
	Sensitivity Sensitivity
	Kind        Kind
	// Value reads the answer out of the operator's own configured facts.
	// Returning "" means "the operator has not provided this", which resolves
	// to unresolved rather than to a guess.
	Value func(pii *config.PII) string
}

// patterns is ordered: the first match wins, so narrower families are listed
// before broader ones that share their vocabulary.
var patterns = []pattern{
	{
		ID:          "sponsorship",
		RequireAll:  [][]string{{"sponsorship", "sponsor", "visa"}},
		Sensitivity: Sensitive,
		Kind:        KindBoolean,
		Value: func(pii *config.PII) string {
			if v, ok := pii.AttestationValue("visa sponsorship"); ok {
				return v
			}
			return ""
		},
	},
	{
		ID:          "work_authorization",
		RequireAll:  [][]string{{"authorized", "authorised", "authorization", "authorisation", "eligible", "eligibility", "legally"}, {"work", "employment", "employed"}},
		Deny:        []string{"sponsorship", "sponsor", "visa"},
		Sensitivity: Sensitive,
		Kind:        KindBoolean,
		Value: func(pii *config.PII) string {
			v, _ := pii.AttestationValue("work authorization")
			return v
		},
	},
	{
		ID:          "security_clearance",
		RequireAll:  [][]string{{"clearance", "classified", "polygraph"}},
		Sensitivity: Sensitive,
		Kind:        KindText,
		Value: func(pii *config.PII) string {
			v, _ := pii.AttestationValue("security clearance")
			return v
		},
	},
	{
		ID:          "criminal_history",
		RequireAll:  [][]string{{"felony", "misdemeanor", "misdemeanour", "convicted", "conviction", "criminal"}},
		Sensitivity: Sensitive,
		Kind:        KindBoolean,
		Value: func(pii *config.PII) string {
			v, _ := pii.AttestationValue("criminal history")
			return v
		},
	},
	{
		ID:          "desired_salary",
		RequireAll:  [][]string{{"salary", "compensation", "pay", "wage", "hourly"}},
		Sensitivity: Sensitive,
		Kind:        KindText,
		Value:       func(pii *config.PII) string { return pii.Work.DesiredSalary },
	},
	{
		ID:          "linkedin",
		RequireAll:  [][]string{{"linkedin"}},
		Sensitivity: Routine,
		Kind:        KindURL,
		Value:       func(pii *config.PII) string { return pii.Links.LinkedIn },
	},
	{
		ID:          "github",
		RequireAll:  [][]string{{"github", "git"}},
		Sensitivity: Routine,
		Kind:        KindURL,
		Value:       func(pii *config.PII) string { return pii.Links.GitHub },
	},
	{
		ID:          "portfolio",
		RequireAll:  [][]string{{"portfolio", "website", "personal", "blog"}},
		Deny:        []string{"linkedin", "github"},
		Sensitivity: Routine,
		Kind:        KindURL,
		Value: func(pii *config.PII) string {
			if pii.Links.Portfolio != "" {
				return pii.Links.Portfolio
			}
			return pii.Links.Website
		},
	},
	{
		ID:          "years_experience",
		RequireAll:  [][]string{{"years", "year"}, {"experience"}},
		Deny:        []string{"specific", "particular"},
		Sensitivity: Routine,
		Kind:        KindNumber,
		Value:       func(pii *config.PII) string { return pii.Work.YearsExperience },
	},
	{
		ID:          "current_title",
		RequireAll:  [][]string{{"current", "present"}, {"title", "role", "position"}},
		Sensitivity: Routine,
		Kind:        KindText,
		Value:       func(pii *config.PII) string { return pii.Work.CurrentTitle },
	},
	{
		ID:          "current_employer",
		RequireAll:  [][]string{{"current", "present", "most", "recent"}, {"employer", "company", "organization", "organisation"}},
		Sensitivity: Routine,
		Kind:        KindText,
		Value:       func(pii *config.PII) string { return pii.Work.CurrentEmployer },
	},
	{
		ID:          "remote_preference",
		RequireAll:  [][]string{{"remote", "onsite", "hybrid"}},
		Sensitivity: Routine,
		Kind:        KindText,
		Value:       func(pii *config.PII) string { return pii.Work.RemotePreference },
	},
	{
		ID:          "willing_to_relocate",
		RequireAll:  [][]string{{"relocate", "relocation"}},
		Sensitivity: Routine,
		Kind:        KindBoolean,
		Value:       func(pii *config.PII) string { return pii.Work.WillingToRelocate },
	},
	{
		ID:          "notice_period",
		RequireAll:  [][]string{{"notice"}},
		Sensitivity: Routine,
		Kind:        KindText,
		Value:       func(pii *config.PII) string { return pii.Work.NoticePeriod },
	},
	{
		ID:          "earliest_start",
		RequireAll:  [][]string{{"start", "begin", "available", "availability"}, {"date", "when", "earliest", "soon"}},
		Sensitivity: Routine,
		Kind:        KindText,
		Value:       func(pii *config.PII) string { return pii.EarliestStartDate() },
	},
	{
		ID:          "certifications",
		RequireAll:  [][]string{{"certification", "certifications", "certificate", "certificates"}},
		Sensitivity: Routine,
		Kind:        KindText,
		Value:       func(pii *config.PII) string { return pii.Work.Certifications },
	},
	{
		ID:          "how_did_you_hear",
		RequireAll:  [][]string{{"hear", "heard", "find", "found", "learn", "learned", "source", "referral"}},
		Sensitivity: Routine,
		Kind:        KindText,
		Value:       func(pii *config.PII) string { return pii.Work.HowDidYouHear },
	},
	{
		ID:          "previously_employed",
		RequireAll:  [][]string{{"previously", "prior", "before", "former", "formerly"}, {"employed", "worked", "employee"}},
		Sensitivity: Routine,
		Kind:        KindBoolean,
		Value:       func(pii *config.PII) string { return pii.Work.PreviouslyEmployed },
	},
	{
		ID:          "over_18",
		RequireAll:  [][]string{{"18", "eighteen"}},
		Sensitivity: Sensitive,
		Kind:        KindBoolean,
		Value:       func(pii *config.PII) string { return pii.Work.Over18 },
	},
}

// matchPattern returns the first curated pattern whose token requirements the
// question satisfies, or nil.
func matchPattern(question Question) *pattern {
	tokens := Tokens(question.Prompt)
	if len(tokens) == 0 {
		return nil
	}
	present := make(map[string]bool, len(tokens))
	for _, token := range tokens {
		present[token] = true
	}
	for i := range patterns {
		candidate := &patterns[i]
		if matchesPattern(present, candidate) {
			return candidate
		}
	}
	return nil
}

func matchesPattern(present map[string]bool, candidate *pattern) bool {
	for _, denied := range candidate.Deny {
		if present[denied] {
			return false
		}
	}
	for _, group := range candidate.RequireAll {
		matched := false
		for _, token := range group {
			if present[token] {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	return len(candidate.RequireAll) > 0
}

// PatternIDs lists every curated pattern, so tests and the operator
// documentation can enumerate what the vault recognizes without reading the
// table.
func PatternIDs() []string {
	out := make([]string, 0, len(patterns))
	for _, candidate := range patterns {
		out = append(out, candidate.ID)
	}
	return out
}

// SensitivePatternIDs lists the curated patterns classified Sensitive. The
// safe-live-verification step in improvements.md #497 feeds exactly this list
// through the refusal path to confirm none of them can auto-fill.
func SensitivePatternIDs() []string {
	var out []string
	for _, candidate := range patterns {
		if candidate.Sensitivity == Sensitive {
			out = append(out, candidate.ID)
		}
	}
	return out
}

// resolveFromPattern turns a pattern hit into a Resolution.
//
// A sensitive pattern never sets AutoFill. The configured value is offered to
// the operator as a pre-filled suggestion they confirm, which is the flow
// improvements.md #497 specifies: propose, show, allow edit, require approval.
// Career Agent knowing the answer is not the same as Career Agent being
// entitled to type it onto an employer's legal attestation.
func resolveFromPattern(question Question, pii *config.PII) (Resolution, bool) {
	if pii == nil {
		return Resolution{}, false
	}
	candidate := matchPattern(question)
	if candidate == nil {
		return Resolution{}, false
	}
	value := strings.TrimSpace(candidate.Value(pii))
	if value == "" {
		return Resolution{}, false
	}
	return Resolution{
		Resolved:          true,
		AutoFill:          candidate.Sensitivity == Routine,
		Answer:            value,
		Source:            SourcePattern,
		Sensitivity:       candidate.Sensitivity,
		Kind:              candidate.Kind,
		CanonicalQuestion: strings.TrimSpace(question.Prompt),
		PatternID:         candidate.ID,
	}, true
}
