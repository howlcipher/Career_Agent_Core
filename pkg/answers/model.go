// Package answers is the Approved Answer Vault: a durable store of
// application answers the operator has explicitly approved, plus the
// deterministic machinery that decides whether a question the agent just found
// on an employer form can be answered without interrupting them.
//
// The package deliberately does not replace pii.yaml (pkg/config). pii.yaml
// remains the source of *facts about the applicant*; this package stores
// *approved answers to questions*, which is a different thing: the same fact
// can be the right answer to one phrasing and the wrong answer to another, and
// an answer carries an approval decision and a reuse decision that a fact does
// not.
//
// Resolution is deterministic by design (no LLM in the path): an approved
// alias, then an approved answer, then a curated pattern over known facts, and
// otherwise "unresolved" — which is a question for the operator, never a guess.
package answers

import (
	"errors"
	"strings"
	"time"
)

// Sensitivity is how carefully an answer has to be handled. It is decided from
// the question text, never from the answer, so a question is classified the
// same way whether or not the vault happens to know an answer for it.
type Sensitivity string

const (
	// Routine answers may be filled automatically once approved, and may be
	// learned from an operator's answer when they opt in.
	Routine Sensitivity = "routine"
	// Sensitive covers legal attestations, protected-class questions, and
	// anything whose wrong answer misrepresents the applicant to an employer.
	// These are never learned implicitly and never auto-filled from a pattern;
	// see Store.Save and Resolution.AutoFill.
	Sensitive Sensitivity = "sensitive"
	// GeneratePerJob covers questions whose honest answer is specific to the
	// employer ("Why this company?"). These are never stored as reusable
	// answers at all — reusing one verbatim across employers is exactly the
	// kind of content this project refuses to produce.
	GeneratePerJob Sensitivity = "generate_per_job"
)

// Kind describes the shape of an answer so the filler knows how to commit it.
type Kind string

const (
	KindText    Kind = "text"
	KindBoolean Kind = "boolean"
	KindChoice  Kind = "choice"
	KindNumber  Kind = "number"
	KindURL     Kind = "url"
)

// Provenance records how an answer came to be in the vault. It is stored
// rather than inferred because "the operator approved this" is the fact that
// makes a sensitive answer legitimate, and it has to survive restarts,
// exports, and audits.
type Provenance string

const (
	// OperatorApproved: the operator accepted a proposed answer as-is.
	OperatorApproved Provenance = "operator_approved"
	// OperatorEdited: the operator typed or changed the answer themselves.
	OperatorEdited Provenance = "operator_edited"
	// SeededFromPII: derived from pii.yaml as a *suggestion*. A seeded row is
	// never reusable on its own; it exists so the operator has something to
	// approve rather than something to retype.
	SeededFromPII Provenance = "seeded_from_pii"
)

// Scope narrows an answer to where it is true. "Current employer" is global;
// "How did you hear about us?" may legitimately differ per company.
const (
	ScopeGlobal = "global"
	scopeATS    = "ats:"
	scopeCompan = "company:"
)

// ATSScope and CompanyScope build the two narrowed scopes. Both normalize
// their input so "Greenhouse" and "greenhouse" are one scope, not two.
func ATSScope(provider string) string {
	provider = strings.ToLower(strings.TrimSpace(provider))
	if provider == "" {
		return ScopeGlobal
	}
	return scopeATS + provider
}

func CompanyScope(company string) string {
	company = normalizeCompany(company)
	if company == "" {
		return ScopeGlobal
	}
	return scopeCompan + company
}

// Answer is one approved, reusable application answer.
type Answer struct {
	ID                int64
	QuestionKey       string
	CanonicalQuestion string
	AnswerText        string
	Kind              Kind
	Sensitivity       Sensitivity
	Scope             string
	ReuseAllowed      bool
	Provenance        Provenance
	ApprovedAt        time.Time
	UpdatedAt         time.Time
	RevokedAt         *time.Time
	UseCount          int
	LastUsedAt        *time.Time
}

// Revoked reports whether this answer has been withdrawn. A revoked answer is
// kept rather than deleted so its provenance stays auditable, but it never
// resolves again.
func (a Answer) Revoked() bool { return a.RevokedAt != nil }

// Question is one control found on an employer form, reduced to the parts that
// are safe to keep: the visible prompt, the control's shape, and the choices it
// offers. Never the value already in it, and never surrounding page text.
type Question struct {
	// Key is stable within a page render (name, then id, then a hash of the
	// label). It is how an answer gets applied back to the right control.
	Key string
	// Prompt is the accessible label. It is employer-authored, so every caller
	// that stores or displays it must have passed it through the quarantine
	// layer first (ADR-002).
	Prompt      string
	ControlType string
	Options     []string
	Required    bool
}

// Context is what the vault knows about where a question was asked, used to
// pick the most specific applicable scope.
type Context struct {
	ATS     string
	Company string
}

// Source records which step of the resolution chain produced an answer.
type Source string

const (
	SourceAlias   Source = "alias"
	SourceVault   Source = "vault"
	SourcePattern Source = "pattern"
	SourceUnknown Source = ""
)

// Resolution is the outcome of asking the vault about one question.
//
// Resolved and AutoFill are deliberately separate. A sensitive question can be
// resolved to a *suggestion* the operator should see pre-filled, while still
// being something Career Agent must not commit to an employer's form on its
// own. Collapsing the two would mean the only way to offer a suggestion was to
// also fill it, which is the failure this split exists to prevent.
type Resolution struct {
	Resolved          bool
	AutoFill          bool
	Answer            string
	Source            Source
	Sensitivity       Sensitivity
	Kind              Kind
	AnswerID          int64
	CanonicalQuestion string
	// PatternID names the curated pattern that matched, for diagnostics. Empty
	// for alias and vault hits.
	PatternID string
}

// ErrSensitiveNeedsApproval is returned by Save when a sensitive answer is
// written without an explicit operator approval and an explicit reuse
// decision. It is an error rather than a silent downgrade so a caller that
// forgets cannot quietly persist an attestation.
var ErrSensitiveNeedsApproval = errors.New("a sensitive answer needs an explicit operator approval and an explicit reuse decision")

// ErrNotReusable is returned when a caller tries to store a per-job generated
// answer as a reusable one.
var ErrNotReusable = errors.New("a per-job generated answer is never stored as a reusable answer")
