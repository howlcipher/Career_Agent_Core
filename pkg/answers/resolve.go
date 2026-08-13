package answers

import (
	"database/sql"
	"errors"
	"strings"

	"github.com/howlcipher/Career_Agent_Core/pkg/config"
)

// scopeChain returns the scopes to search, most specific first. A
// company-specific approval beats an ATS-wide one, which beats a global one —
// so "How did you hear about us?" can be answered differently for one employer
// without disturbing the general answer.
func scopeChain(ctx Context) []string {
	chain := make([]string, 0, 3)
	if company := CompanyScope(ctx.Company); company != ScopeGlobal {
		chain = append(chain, company)
	}
	if ats := ATSScope(ctx.ATS); ats != ScopeGlobal {
		chain = append(chain, ats)
	}
	return append(chain, ScopeGlobal)
}

// Resolve answers one question, deterministically.
//
// The chain is: an operator-approved alias for this exact phrasing, then an
// approved answer under this question's own key, then the curated pattern table
// over the operator's configured facts, and otherwise unresolved. There is no
// model call and no similarity threshold anywhere in it, which is what makes
// the same form produce the same answers on every run and makes every step of
// it testable.
//
// An unresolved question is not a failure. It is the thing this whole feature
// is built to surface: the small set of decisions that genuinely need the
// operator, separated from the large set that does not.
func (s *Store) Resolve(question Question, ctx Context, pii *config.PII) Resolution {
	if s != nil && s.conn != nil {
		for _, scope := range scopeChain(ctx) {
			if resolution, ok := s.resolveFromAlias(question, scope); ok {
				return escalateSensitivity(resolution, question)
			}
			if resolution, ok := s.resolveFromVault(question, scope); ok {
				return escalateSensitivity(resolution, question)
			}
			if resolution, ok := s.resolveFromSkillExperience(question, scope); ok {
				return escalateSensitivity(resolution, question)
			}
		}
	}
	if resolution, ok := resolveFromPattern(question, pii); ok {
		return escalateSensitivity(resolution, question)
	}
	return Resolution{Sensitivity: Classify(question), Source: SourceUnknown}
}

// escalateSensitivity takes the stricter of what produced the answer and what
// the question itself classifies as.
//
// This closes the hole bugs.md #541 was filed for. The dashboard decides which
// acknowledgements to show from the sensitivity recorded on the *question*,
// while Store.Save enforces its rule using its own re-classification of the
// same question. When those two disagreed, the two-checkbox guarantee failed
// silently in the unsafe direction: a pattern declared Routine, so the operator
// saw one checkbox and no declaration warning, while the store classified the
// question Sensitive and accepted the resulting ReuseAllowed=true — storing a
// declaration with reuse permission nobody had granted. Observed live on a real
// Greenhouse form.
//
// Taking the union means the two can no longer disagree in the direction that
// matters. AutoFill is recomputed rather than carried over, because a
// resolution that has just become sensitive must stop being something Career
// Agent types on the operator's behalf — except for a stored answer whose reuse
// the operator explicitly granted, which is exactly what that grant is for.
func escalateSensitivity(resolution Resolution, question Question) Resolution {
	if resolution.Sensitivity == Sensitive || Classify(question) != Sensitive {
		return resolution
	}
	resolution.Sensitivity = Sensitive
	if resolution.Source == SourcePattern {
		resolution.AutoFill = false
	}
	return resolution
}

func (s *Store) resolveFromAlias(question Question, scope string) (Resolution, bool) {
	alias := Normalize(question.Prompt)
	if alias == "" {
		return Resolution{}, false
	}
	row := s.conn.QueryRow(`SELECT a.id, a.question_key, a.canonical_question, a.answer_text, a.answer_kind,
		a.sensitivity, a.scope, a.reuse_allowed, a.provenance, a.approved_at, a.updated_at, a.revoked_at, a.use_count, a.last_used_at
		FROM answer_aliases al JOIN approved_answers a ON a.id = al.answer_id
		WHERE al.alias_key = ? AND al.scope = ? AND a.revoked_at IS NULL`, alias, scope)
	answer, err := scanAnswer(row)
	if err != nil {
		return Resolution{}, false
	}
	return s.resolutionFrom(answer, SourceAlias), true
}

func (s *Store) resolveFromVault(question Question, scope string) (Resolution, bool) {
	return s.resolveFromKey(QuestionKey(question.Prompt), scope, SourceVault)
}

// resolveFromSkillExperience answers "how many years of <skill>?" from the one
// value the operator approved for that skill, whatever wording the employer
// used.
//
// This is the deterministic half of bugs.md #544. The pattern table now refuses
// a skill-scoped duration question rather than answering it with a career
// total, which on its own would mean every phrasing of every skill question
// became a separate operator interruption forever. Reducing the question to its
// subject is a closed, curated transformation over the token list in
// experience.go -- no stemming, no similarity, no model -- so "Years of
// Kubernetes experience", "How long have you worked with Kubernetes?" and
// "Kubernetes experience (years)" reach the same stored answer, and a skill the
// operator has never approved still reaches nobody.
func (s *Store) resolveFromSkillExperience(question Question, scope string) (Resolution, bool) {
	subject := SkillExperienceSubject(question)
	if subject == "" {
		return Resolution{}, false
	}
	return s.resolveFromKey(SkillExperienceKey(subject), scope, SourceSkillExperience)
}

func (s *Store) resolveFromKey(key, scope string, source Source) (Resolution, bool) {
	if key == "" {
		return Resolution{}, false
	}
	row := s.conn.QueryRow(`SELECT id, question_key, canonical_question, answer_text, answer_kind,
		sensitivity, scope, reuse_allowed, provenance, approved_at, updated_at, revoked_at, use_count, last_used_at
		FROM approved_answers WHERE question_key = ? AND scope = ? AND revoked_at IS NULL`, key, scope)
	answer, err := scanAnswer(row)
	if err != nil {
		return Resolution{}, false
	}
	return s.resolutionFrom(answer, source), true
}

// resolutionFrom converts a stored answer into a resolution.
//
// A stored answer auto-fills only when the operator allowed reuse. A stored
// answer with reuse withheld still resolves — the operator gets their own
// previous wording back as a suggestion instead of a blank box — but Career
// Agent does not commit it to the form on their behalf. That distinction is
// the difference between remembering something and deciding something.
func (s *Store) resolutionFrom(answer Answer, source Source) Resolution {
	if answer.ReuseAllowed {
		s.recordUse(answer.ID)
	}
	return Resolution{
		Resolved:          true,
		AutoFill:          answer.ReuseAllowed,
		Answer:            answer.AnswerText,
		Source:            source,
		Sensitivity:       answer.Sensitivity,
		Kind:              answer.Kind,
		AnswerID:          answer.ID,
		CanonicalQuestion: answer.CanonicalQuestion,
	}
}

// ResolveAll answers a batch of questions in one pass and splits them the way
// the operator's screen does: what Career Agent filled, and what still needs
// them.
//
// Sensitive resolutions land in NeedsOperator with their suggestion attached,
// never in Filled — the caller cannot accidentally treat a proposal as a
// completed field, because the two never arrive in the same slice.
func (s *Store) ResolveAll(questions []Question, ctx Context, pii *config.PII) Batch {
	batch := Batch{}
	for _, question := range questions {
		// The classifier needs the employer's name to discount it (bugs.md
		// #540); callers supply it once on the context rather than on every
		// question.
		if question.Company == "" {
			question.Company = ctx.Company
		}
		resolution := s.Resolve(question, ctx, pii)
		entry := Resolved{Question: question, Resolution: resolution}
		if resolution.Resolved && resolution.AutoFill {
			batch.Filled = append(batch.Filled, entry)
			continue
		}
		batch.NeedsOperator = append(batch.NeedsOperator, entry)
	}
	return batch
}

// Resolved pairs a question with what the vault concluded about it.
type Resolved struct {
	Question   Question
	Resolution Resolution
}

// Batch is one form's worth of resolutions, already split into the two groups
// the exception-only UI renders.
type Batch struct {
	Filled        []Resolved
	NeedsOperator []Resolved
}

// SeedFromPII writes suggestion rows for the stable facts the operator has
// already configured, so the vault is useful on its very first application
// instead of after a dozen.
//
// Every seeded row is written with reuse withheld, including the routine ones.
// A seeded row is Career Agent's reading of pii.yaml, not the operator's
// approval of an answer, and the whole point of this package is that those are
// different. Seeding is therefore safe to re-run: it never grants an
// auto-fill permission, and it never overwrites a row the operator has
// approved.
func (s *Store) SeedFromPII(pii *config.PII) (int, error) {
	if s == nil || s.conn == nil {
		return 0, errors.New("answer vault is not initialized")
	}
	if pii == nil {
		return 0, nil
	}
	seeded := 0
	for i := range patterns {
		candidate := &patterns[i]
		value := strings.TrimSpace(candidate.Value(pii))
		if value == "" {
			continue
		}
		question := seedQuestions[candidate.ID]
		if question == "" {
			continue
		}
		key := QuestionKey(question)
		var exists bool
		if err := s.conn.QueryRow(`SELECT EXISTS(SELECT 1 FROM approved_answers WHERE question_key = ? AND scope = ?)`, key, ScopeGlobal).Scan(&exists); err != nil {
			return seeded, err
		}
		if exists {
			continue
		}
		now := s.now()
		if _, err := s.conn.Exec(`INSERT INTO approved_answers
			(question_key, canonical_question, answer_text, answer_kind, sensitivity, scope, reuse_allowed, provenance, approved_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, 0, ?, ?, ?)`,
			key, question, value, string(candidate.Kind), string(candidate.Sensitivity), ScopeGlobal, string(SeededFromPII), now, now); err != nil {
			return seeded, err
		}
		seeded++
	}
	return seeded, nil
}

// seedQuestions is the canonical wording each seeded fact is filed under. These
// are Career Agent's own labels, not any employer's, so the operator reading
// the vault sees a question rather than a config key.
var seedQuestions = map[string]string{
	"work_authorization":  "Are you legally authorized to work in the United States?",
	"sponsorship":         "Will you now or in the future require visa sponsorship for employment?",
	"security_clearance":  "Do you hold an active security clearance?",
	"criminal_history":    "Have you been convicted of a felony?",
	"desired_salary":      "What is your desired salary?",
	"linkedin":            "LinkedIn profile URL",
	"github":              "GitHub profile URL",
	"portfolio":           "Portfolio or personal website URL",
	"years_experience":    "How many years of professional experience do you have?",
	"current_title":       "What is your current job title?",
	"current_employer":    "Who is your current employer?",
	"remote_preference":   "What is your remote work preference?",
	"willing_to_relocate": "Are you willing to relocate?",
	"notice_period":       "What is your notice period?",
	"earliest_start":      "What is your earliest available start date?",
	"certifications":      "What professional certifications do you hold?",
	"how_did_you_hear":    "How did you hear about this role?",
	"previously_employed": "Have you previously been employed by this company?",
	"over_18":             "Are you at least 18 years of age?",
}

// OpenStore is the convenience constructor for callers that have a connection
// and want the schema guaranteed in one step.
func OpenStore(conn *sql.DB) (*Store, error) {
	if err := EnsureSchema(conn); err != nil {
		return nil, err
	}
	return NewStore(conn), nil
}
