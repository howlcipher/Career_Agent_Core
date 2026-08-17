package config

import "strings"

// This file is the deterministic title policy that separates two
// independent questions TitleEligible used to collapse into one keyword
// match: what CAREER TRACK a title belongs to, and how SENIOR it is. Before
// this existed, "Director of DevOps" passed the role gate for the same
// reason "DevOps Engineer" did -- both contain the word "devops" -- because
// TitleEligible only ever asked "does a distinctive word overlap?" and never
// asked whether the title's primary noun was an individual-contributor
// engineering role at all. That let management-track postings (Director,
// VP, Head of, Chief, Manager) reach the assisted-apply queue as if they
// were engineering roles.
//
// TitleFit is reported alongside TitleEligible's plain bool so callers that
// want it (ranking, reconciliation reporting, dashboards) can tell a
// wholehearted match from one only admitted as a limited stretch.
type TitleFit string

const (
	// FitPrimary is a title that names one of the operator's core IC
	// engineering tracks (e.g. "DevOps Engineer", "Platform Engineer") at an
	// ordinary or senior level.
	FitPrimary TitleFit = "primary"
	// FitAdjacent is a title that plausibly belongs to a configured role via
	// a shared distinctive word (the pre-existing TitleEligible heuristic)
	// but did not match a configured role phrase outright.
	FitAdjacent TitleFit = "adjacent"
	// FitStretch is an otherwise-matching IC engineering title at Staff or
	// Principal seniority. Allowed by default, but meant to be a small
	// minority of the queue rather than dominate it (see ranking.go).
	FitStretch TitleFit = "stretch"
	// FitReject means the title should never reach scoring or the assisted
	// queue: it is a management/leadership track, or names no configured
	// engineering role at all.
	FitReject TitleFit = "reject"
)

// TitleClassification is ClassifyTitle's structured verdict.
type TitleClassification struct {
	Fit TitleFit
	// Reason is one of the Reason* constants in eligibility.go, populated
	// only when Fit == FitReject, so ScreenJob and the queue reconciler can
	// record precisely why (management_track_excluded vs
	// seniority_outside_target vs role_track_mismatch) rather than folding
	// every rejection into the single generic pruned_ineligible_role code.
	Reason string
}

// managementTitleWords are single, normalized tokens that indicate a
// title's primary track is people/organizational management rather than
// individual-contributor engineering. Checked as whole words (via the
// normalized word set), never as substrings, so "director" cannot
// accidentally match inside an unrelated compound word.
//
// "manager" is included deliberately and without a "people manager" carve
// out: "DevOps Manager" and "Platform Architect Manager"-style titles are
// treated the same as "Manager of DevOps" for this policy (documented
// decision, task instructions Step 10's ambiguous-case list) -- a title
// whose primary noun is "Manager" is management track even when a track
// word modifies it, exactly like "Director of DevOps" is management track
// despite naming DevOps.
var managementTitleWords = map[string]bool{
	"director": true, "vp": true, "chief": true, "manager": true,
}

// managementTitlePhrases catches multi-word management signals that are not
// single distinctive tokens.
var managementTitlePhrases = []string{
	"vice president", "head of",
}

// seniorityStretchWords mark Staff/Principal individual-contributor
// seniority. "Principal" and "Staff" are IC levels, not management --
// unlike Director, they must never be rejected outright, only ranked as a
// bounded stretch tier (task instructions Step "PRINCIPAL / STAFF
// HANDLING").
var seniorityStretchWords = map[string]bool{
	"principal": true, "staff": true,
}

// ManagementTrackTitle reports whether title's primary track reads as
// organizational/people management rather than individual-contributor
// engineering, independent of any configured role list.
func ManagementTrackTitle(title string) bool {
	t := normalizeForRemoteCheck(title)
	if t == "" {
		return false
	}
	padded := " " + t + " "
	for _, phrase := range managementTitlePhrases {
		if strings.Contains(padded, " "+phrase+" ") {
			return true
		}
	}
	for _, w := range strings.Fields(t) {
		if managementTitleWords[w] {
			return true
		}
	}
	return false
}

// stretchSeniorityTitle reports whether title carries a Staff/Principal IC
// seniority marker.
func stretchSeniorityTitle(title string) bool {
	t := normalizeForRemoteCheck(title)
	for _, w := range strings.Fields(t) {
		if seniorityStretchWords[w] {
			return true
		}
	}
	return false
}

// ClassifyTitle is the canonical deterministic title policy. It answers
// career track and seniority as two independent questions before falling
// back to TitleEligible's existing phrase/distinctive-word matching for
// track fit:
//
//  1. Management/leadership track (Director, VP, Head of, Chief, Manager)
//     is rejected unless allowManagement is true, regardless of how well
//     any track word in the title matches a configured role -- a perfect
//     "DevOps" keyword hit never rescues a Director title.
//  2. Otherwise, the title must match a configured role the same way
//     TitleEligible always has (full role phrase, or a shared distinctive
//     word). No match is FitReject with ReasonRoleTrackMismatch.
//  3. A match at Staff/Principal seniority is FitStretch rather than
//     FitPrimary/FitAdjacent, and is rejected instead (with
//     ReasonSeniorityOutsideTarget) when allowStretch is false.
func ClassifyTitle(title string, roles []string, allowManagement, allowStretch bool) TitleClassification {
	if ManagementTrackTitle(title) && !allowManagement {
		return TitleClassification{Fit: FitReject, Reason: ReasonManagementTrackExcluded}
	}

	phraseMatch, wordMatch := matchesConfiguredRole(title, roles)
	if !phraseMatch && !wordMatch {
		return TitleClassification{Fit: FitReject, Reason: ReasonRoleTrackMismatch}
	}

	if stretchSeniorityTitle(title) {
		if !allowStretch {
			return TitleClassification{Fit: FitReject, Reason: ReasonSeniorityOutsideTarget}
		}
		return TitleClassification{Fit: FitStretch}
	}

	if phraseMatch {
		return TitleClassification{Fit: FitPrimary}
	}
	return TitleClassification{Fit: FitAdjacent}
}

// matchesConfiguredRole is TitleEligible's original matching logic, factored
// out so ClassifyTitle can distinguish a full role-phrase match (Primary)
// from a shared-distinctive-word match (Adjacent) instead of collapsing both
// into a single bool.
func matchesConfiguredRole(title string, roles []string) (phraseMatch, wordMatch bool) {
	if len(roles) == 0 {
		// No configured roles: do not silently filter everything out. Mirrors
		// TitleEligible's own no-roles-configured behavior. Reported as a
		// phrase match so callers that only care about admission (not the
		// Primary/Adjacent distinction) see the same "eligible" outcome as
		// before.
		return true, false
	}
	t := normalizeForRemoteCheck(title)
	if t == "" {
		return false, false
	}

	titleWords := map[string]bool{}
	for _, w := range strings.Fields(t) {
		titleWords[w] = true
	}

	for _, role := range roles {
		r := normalizeForRemoteCheck(role)
		if r == "" {
			continue
		}
		if strings.Contains(" "+t+" ", " "+r+" ") || t == r {
			phraseMatch = true
			continue
		}
		for _, word := range strings.Fields(r) {
			if distinctiveRoleWords[word] && titleWords[word] {
				wordMatch = true
			}
		}
	}
	return phraseMatch, wordMatch
}

// IsStretchSeniorityTitle reports whether title carries a Staff/Principal
// individual-contributor seniority marker, independent of any role list.
// Exported so callers that already know a title passed the role gate (e.g.
// pkg/storage's ranking, which ranks already-DISCOVERED rows) can bias a
// stretch match's rank down without re-running the full role match --
// Staff/Principal roles are meant to remain a bounded minority of the queue,
// not to dominate it (task instructions, "Desired queue mix").
func IsStretchSeniorityTitle(title string) bool {
	return stretchSeniorityTitle(title)
}

// TitleEligibleForRoles is TitleEligible with explicit control over the two
// opt-in policy switches (Profile.AllowManagementRoles and
// Profile.RejectStretchSeniority's inverse). TitleEligible itself always
// calls this with allowManagement=false, allowStretch=true -- the product
// default of excluding management-track titles while keeping Staff/Principal
// as an allowed stretch tier.
func TitleEligibleForRoles(title string, roles []string, allowManagement, allowStretch bool) bool {
	return ClassifyTitle(title, roles, allowManagement, allowStretch).Fit != FitReject
}
