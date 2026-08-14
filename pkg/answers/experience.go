package answers

import "strings"

// This file exists because of one live defect, and the reasoning behind it is
// worth keeping next to the code.
//
// The `years_experience` curated pattern requires only a duration token and an
// experience token, is classified Routine, and resolveFromPattern grants
// AutoFill to every Routine pattern. So "How many years of Kubernetes
// experience do you have?" matched it and typed pii.Work.YearsExperience --
// the operator's *total career* years -- onto a real employer's screening
// question about one technology. That is a fabricated qualification, submitted
// under the operator's name, and it is the exact failure this package exists to
// prevent.
//
// The fix is not to delete the pattern: "How many years of professional
// experience do you have?" is a real question with a real configured answer.
// The fix is to tell the two apart deterministically, and to give the
// skill-scoped half somewhere honest to get its answer from -- an approved
// value the operator set, or nothing at all.

// experienceGenericTokens are the words a duration question is made of once its
// subject is removed. Whatever survives this list is what the employer is
// actually asking about.
//
// The list is closed and deliberately generous, because the two failure modes
// are not symmetric. Keeping a word that should have been dropped invents a
// skill subject where there is none, which costs the operator one approval that
// then becomes an alias forever. Dropping a word that was really the subject
// lets a skill question fall through to the total-years answer, which is the
// fabrication this file exists to stop. So when in doubt, the word goes in.
//
// Note that Tokens() has already removed the stop-word list in normalize.go
// ("do", "have", "your", "with", "of", "in", "on", ...), so this list only
// needs the words that survive it.
var experienceGenericTokens = map[string]bool{
	// Interrogatives and quantifiers.
	"how": true, "what": true, "many": true, "much": true, "long": true,
	"approximately": true, "roughly": true, "about": true, "number": true,
	// Duration vocabulary.
	"years": true, "year": true, "yrs": true, "duration": true,
	// Experience vocabulary.
	"experience": true, "experienced": true, "expertise": true,
	"work": true, "working": true, "worked": true,
	"use": true, "using": true, "used": true, "usage": true,
	"hands": true,
	// Qualifiers that describe the *measure* rather than its subject. These are
	// what keep "How many years of professional experience do you have?" a
	// general question rather than a question about a skill called
	// "professional".
	"professional": true, "total": true, "overall": true, "relevant": true,
	"industry": true, "full": true, "part": true, "time": true,
	"minimum": true, "maximum": true, "least": true, "most": true,
	"required": true, "requirement": true, "combined": true, "cumulative": true,
}

// experienceDurationTokens and experienceSubjectTokens are the two things a
// question must contain before it counts as an experience-duration question at
// all. Requiring both is what stops "Describe your Kubernetes work" (an essay
// prompt) and "What is your current title?" from being read as duration
// questions.
var experienceDurationTokens = map[string]bool{
	"years": true, "year": true, "yrs": true, "long": true, "duration": true,
}

var experienceSubjectTokens = map[string]bool{
	"experience": true, "experienced": true, "expertise": true,
	"work": true, "working": true, "worked": true,
	"use": true, "using": true, "used": true, "usage": true,
}

// SkillExperienceSubject returns the skill or technology a duration question is
// asking about, or "" when the question is a general one.
//
// "How many years of Kubernetes experience do you have?" -> "kubernetes"
// "How long have you worked with Terraform?"             -> "terraform"
// "Years of experience with Go"                          -> "go"
// "How many years of professional experience?"           -> ""
// "What is your current job title?"                      -> ""
//
// The employer's own name is subtracted before the subject is read, for the
// same reason Classify subtracts it (bugs.md #540): "How many years have you
// worked at Affirm?" is a question about tenure at one company, not about a
// skill called Affirm, and a company name is the one token guaranteed to appear
// in an employer's questions.
func SkillExperienceSubject(question Question) string {
	tokens := Tokens(question.Prompt)
	if len(tokens) == 0 {
		return ""
	}
	company := make(map[string]bool)
	for _, token := range Tokens(question.Company) {
		company[token] = true
	}

	hasDuration := false
	hasSubject := false
	remainder := make([]string, 0, len(tokens))
	for _, token := range tokens {
		if experienceDurationTokens[token] {
			hasDuration = true
		}
		if experienceSubjectTokens[token] {
			hasSubject = true
		}
		if experienceGenericTokens[token] || company[token] {
			continue
		}
		remainder = append(remainder, token)
	}
	if !hasDuration || !hasSubject {
		return ""
	}
	return strings.Join(remainder, " ")
}

// SkillExperienceQuestion is the canonical wording a skill's approved
// experience value is filed under. It is Career Agent's own label, not any
// employer's, so the operator reading the vault sees a question rather than a
// token.
func SkillExperienceQuestion(subject string) string {
	subject = strings.TrimSpace(subject)
	if subject == "" {
		return ""
	}
	return "How many years of " + subject + " experience do you have?"
}

// SkillExperienceKey is the vault key an approved skill-experience answer is
// stored under. Every phrasing of the same skill's duration question resolves
// through this key, which is what makes one approval cover all of them without
// any similarity matching.
func SkillExperienceKey(subject string) string {
	return QuestionKey(SkillExperienceQuestion(subject))
}
