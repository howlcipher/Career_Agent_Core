package answers

import "strings"

// sensitiveMarkers are the token groups that force a question to Sensitive.
//
// Each entry is a group of alternatives; a question is sensitive if it contains
// any token from any group. The taxonomy is broad on purpose. A false positive
// costs the operator one confirmation click on a question Career Agent could
// arguably have filled; a false negative means an agent silently answered a
// legal attestation or a protected-class question on a real application, which
// is the single failure this package exists to prevent. Those costs are not
// comparable, so the classifier is not tuned to balance them.
var sensitiveMarkers = [][]string{
	// Work authorization, sponsorship, immigration status.
	{"authorized", "authorised", "authorization", "authorisation", "eligibility", "eligible"},
	{"sponsorship", "sponsor", "visa", "h1b", "h1", "ead", "immigration", "citizenship", "citizen"},
	// Criminal history, background and drug screening.
	{"felony", "misdemeanor", "misdemeanour", "convicted", "conviction", "criminal", "offense", "offence"},
	{"background", "screening", "drug", "substance"},
	// Security clearance.
	{"clearance", "classified", "polygraph"},
	// Protected characteristics and voluntary self-identification.
	{"disability", "disabled", "impairment", "accommodation", "accommodations"},
	{"veteran", "military", "protected"},
	{"gender", "sex", "race", "ethnicity", "ethnic", "hispanic", "latino", "orientation", "transgender", "pronoun", "pronouns"},
	{"age", "birth", "birthdate", "dob"},
	// Compensation, which is a negotiating position rather than a fact.
	{"salary", "compensation", "pay", "wage", "hourly", "expectations"},
	// Explicit legal declarations and consents.
	{"certify", "attest", "affirm", "acknowledge", "consent", "agree", "declaration", "terms", "policy", "truthful", "accurate"},
}

// generatedMarkers are the token groups that force GeneratePerJob. A question
// containing any of these has an honest answer only in the context of one
// employer, so caching one and replaying it is precisely the behavior this
// project refuses.
var generatedMarkers = [][]string{
	{"why"},
	{"interest", "interested", "excites", "excited", "attracts", "attracted", "motivates", "motivated", "drew"},
	{"describe", "tell", "explain", "elaborate", "summarize", "summarise"},
	{"cover"},
}

// generatedRequiresFreeText lists the control types where a GeneratePerJob
// marker is believable. "Why" appears inside plenty of short yes/no questions
// ("Why not?" follow-ups, "Do you know why..."), and treating a radio button as
// an essay prompt would send the operator a text box for a question that wants
// a click.
var generatedRequiresFreeText = map[string]bool{
	"textarea": true,
	"text":     true,
	"":         true,
}

// longFreeTextPrompt is the length past which a free-text question is treated
// as an essay regardless of wording. Measured in normalized characters.
const longFreeTextPrompt = 120

// Classify decides how carefully a question has to be handled, from the
// question alone.
//
// Order matters: Sensitive wins over GeneratePerJob. "Describe any criminal
// convictions" is an essay prompt *and* an attestation, and the attestation
// rules are the stricter ones.
func Classify(question Question) Sensitivity {
	tokens := Tokens(question.Prompt)
	if len(tokens) == 0 {
		// A control whose label could not be read is the least safe thing to
		// answer automatically, not the most.
		return Sensitive
	}
	present := make(map[string]bool, len(tokens))
	for _, token := range tokens {
		present[token] = true
	}
	// Discount the employer's own name (bugs.md #540). A question is not a
	// legal declaration because the company is called Affirm, Consent or
	// Certify. Only the exact words of the company's name are removed, and the
	// marker groups are redundant enough that a real attestation from such a
	// company still matches on its other vocabulary — "Do you consent to a
	// background check?" at Consent Systems still trips "background".
	for _, token := range Tokens(question.Company) {
		delete(present, token)
	}
	if len(present) == 0 {
		return Sensitive
	}
	if matchesAnyGroup(present, sensitiveMarkers) {
		return Sensitive
	}
	controlType := strings.ToLower(strings.TrimSpace(question.ControlType))
	if generatedRequiresFreeText[controlType] || controlType == "textarea" {
		if matchesAnyGroup(present, generatedMarkers) {
			return GeneratePerJob
		}
		if controlType == "textarea" && len(Normalize(question.Prompt)) >= longFreeTextPrompt {
			return GeneratePerJob
		}
	}
	return Routine
}

func matchesAnyGroup(present map[string]bool, groups [][]string) bool {
	for _, group := range groups {
		for _, token := range group {
			if present[token] {
				return true
			}
		}
	}
	return false
}
