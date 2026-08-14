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
	// Reject refuses a question this pattern's tokens would otherwise claim.
	//
	// Deny handles the case where a *token* disqualifies a match. Reject exists
	// for the case where the disqualifying fact is a property of the whole
	// question that no fixed token list can express -- currently only
	// years_experience, which must not answer a question scoped to one skill
	// with the operator's total career years (bugs.md #544). It is stated as a
	// predicate rather than left to table ordering plus an empty Value, because
	// a refusal that depends on which row happens to come first is a refusal
	// nobody can see when reading the table.
	Reject func(Question) bool
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
		ID:          "twitter",
		RequireAll:  [][]string{{"twitter", "x"}},
		Deny:        []string{"linkedin", "github"},
		Sensitivity: Routine,
		Kind:        KindURL,
		Value:       func(pii *config.PII) string { return pii.Links.Twitter },
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
		// bugs.md #544. pii.Work.YearsExperience is a career total, and it is
		// the right answer to "How many years of professional experience do you
		// have?" and the wrong answer to "How many years of Kubernetes
		// experience do you have?" -- which this pattern's tokens match just as
		// readily. Answering the second with the first states a qualification
		// the operator does not have, on a real application, under their name.
		// A skill-scoped question is therefore refused here and left for the
		// vault's own skill-experience lookup or for the operator.
		Reject: func(question Question) bool { return SkillExperienceSubject(question) != "" },
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

	// Identity and contact facts.
	//
	// These are last in the table on purpose. Every attestation family above
	// shares vocabulary with one of them -- "Are you eligible to work in your
	// country of residence?" contains "country", and matching it here would
	// offer a country name as the answer to a yes/no legal question. First match
	// wins, so the attestation patterns claim those questions before these are
	// ever consulted. Do not move them.
	//
	// They exist because the vault could not previously answer "First Name",
	// which made it look, to anything that asked, as though Career Agent did not
	// know the operator's own name. The per-ATS handlers fill these on the
	// boards they support, so on a real Greenhouse form nothing here changes;
	// what changes is that a question inventory taken *before* a handler runs no
	// longer reports the operator's own contact details back to them as work to
	// do. Observed live on 2026-08-13: 6 of 19 "questions" on a real Grafana
	// Labs form were the operator's name, email, phone and location.
	//
	// The Deny lists all guard the same mistake in different words: a form
	// asking for somebody *else's* name, email or phone -- a reference, a
	// manager, a recruiter, a previous employer -- must not be handed the
	// operator's.
	{
		ID:          "first_name",
		RequireAll:  [][]string{{"first", "given", "forename", "preferred"}, {"name"}},
		Deny:        otherPartyTokens,
		Sensitivity: Routine,
		Kind:        KindText,
		Value:       func(pii *config.PII) string { return pii.FirstName },
	},
	{
		ID:          "last_name",
		RequireAll:  [][]string{{"last", "surname", "family"}, {"name"}},
		Deny:        otherPartyTokens,
		Sensitivity: Routine,
		Kind:        KindText,
		Value:       func(pii *config.PII) string { return pii.LastName },
	},
	{
		ID:          "full_name",
		RequireAll:  [][]string{{"full", "legal", "name"}, {"name"}},
		Deny:        otherPartyTokens,
		Sensitivity: Routine,
		Kind:        KindText,
		Value: func(pii *config.PII) string {
			full := strings.TrimSpace(pii.FirstName + " " + pii.LastName)
			if full == "" {
				return ""
			}
			return full
		},
	},
	{
		ID:          "email",
		RequireAll:  [][]string{{"email", "mail"}},
		Deny:        otherPartyTokens,
		Sensitivity: Routine,
		Kind:        KindText,
		Value:       func(pii *config.PII) string { return pii.Email },
	},
	{
		ID:          "phone",
		RequireAll:  [][]string{{"phone", "mobile", "cell", "telephone"}},
		Deny:        otherPartyTokens,
		Sensitivity: Routine,
		Kind:        KindText,
		Value:       func(pii *config.PII) string { return pii.Phone },
	},
	{
		ID:          "city",
		RequireAll:  [][]string{{"city", "town", "municipality"}},
		Deny:        otherPartyTokens,
		Sensitivity: Routine,
		Kind:        KindText,
		Value:       func(pii *config.PII) string { return pii.City },
	},
	{
		ID:         "state_region",
		RequireAll: [][]string{{"state", "province", "region"}},
		// "United States" normalizes to "united states", so the plural does not
		// collide -- but a question mentioning the country by name still should
		// not be answered with a state.
		Deny:        append([]string{"united", "country"}, otherPartyTokens...),
		Sensitivity: Routine,
		Kind:        KindText,
		Value: func(pii *config.PII) string {
			if pii.FullState != "" {
				return pii.FullState
			}
			return pii.State
		},
	},
	{
		ID:          "postal_code",
		RequireAll:  [][]string{{"zip", "postal", "postcode"}},
		Deny:        otherPartyTokens,
		Sensitivity: Routine,
		Kind:        KindText,
		Value:       func(pii *config.PII) string { return pii.Zip },
	},
	{
		ID:         "country",
		RequireAll: [][]string{{"country"}},
		// Two different refusals in one list.
		//
		// A question asking for a country *and* something else wants a composite
		// answer this pattern does not have; offering only half of it would be a
		// wrong answer that looks like a right one.
		//
		// And "What is your country of citizenship?" is an immigration question,
		// not a location one. No pattern above claims it -- "citizenship" is not
		// sponsorship vocabulary -- so without this entry it fell through to here
		// and proposed the operator's residence as their citizenship. Classify
		// marks it Sensitive so it could never have auto-filled, but a suggestion
		// on a legal question is still a guess Career Agent has no business
		// making. Caught by a test written after the identity patterns were
		// added, not before.
		Deny: append([]string{
			"zone", "timezone", "code",
			"citizenship", "citizen", "national", "nationality", "passport",
		}, otherPartyTokens...),
		Sensitivity: Routine,
		Kind:        KindText,
		Value: func(pii *config.PII) string {
			if pii.FullCountry != "" {
				return pii.FullCountry
			}
			return pii.Country
		},
	},
	{
		ID:          "street_address",
		RequireAll:  [][]string{{"street", "address"}},
		Deny:        append([]string{"email", "ip", "web"}, otherPartyTokens...),
		Sensitivity: Routine,
		Kind:        KindText,
		Value: func(pii *config.PII) string {
			if pii.Street != "" {
				return pii.Street
			}
			return pii.Address
		},
	},
}

// otherPartyTokens mark a question asking about somebody who is not the
// applicant. A reference's phone number and the applicant's phone number are
// different facts, and only one of them is in pii.yaml.
var otherPartyTokens = []string{
	"reference", "references", "referrer", "referral",
	"manager", "supervisor", "recruiter", "contact",
	"emergency", "spouse", "parent", "guardian",
	"employer", "company", "school", "university", "institution",
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
		if matchesPattern(present, question, candidate) {
			return candidate
		}
	}
	return nil
}

func matchesPattern(present map[string]bool, question Question, candidate *pattern) bool {
	if candidate.Reject != nil && candidate.Reject(question) {
		return false
	}
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

// MatchedPatternID names the curated family a question belongs to, or "".
//
// It is exported so cross-application grouping can use the same recognizers the
// resolver uses. That matters more than it sounds: if grouping had its own idea
// of which questions are the same family, the operator could answer a group and
// find the resolver disagreed about half of it. One table, one answer to
// "which question is this", and the Deny lists that keep sponsorship apart from
// work authorization apply to both.
//
// Note this reports the family a question *belongs to*, not whether it can be
// answered -- a match here says nothing about whether the operator has
// configured the corresponding fact.
func MatchedPatternID(question Question) string {
	if candidate := matchPattern(question); candidate != nil {
		return candidate.ID
	}
	return ""
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
