package config

import "strings"

// JobEligibilityInput is the minimal, source-agnostic view of a posting that
// IsEligibleJob needs. It exists so every stage of the pipeline -- a fresh
// ATS feed row with only a location, a freshly-fetched posting with a full
// description, and a persisted assisted-apply row reloaded from job_funnel
// with only its stored job_title/job_location/is_remote -- can be run through
// exactly one rule rather than each stage inventing its own.
//
// Description is optional: an empty value never manufactures evidence either
// way, it just means this call site has less to check than one made after
// the full posting was fetched.
type JobEligibilityInput struct {
	Title         string
	Location      string
	Description   string
	RemoteClaimed bool
	// CountryCodes carries ISO-3166 alpha-2 codes a feed stated explicitly,
	// kept separate from Location on purpose: an explicit field is authority,
	// free-text prose is not. "CA" published as a country code means Canada;
	// "CA" appearing in a location string may well mean California, and must
	// never be read as a country. Optional -- callers that reload a row from
	// job_funnel have only the stored location text, which is fine.
	CountryCodes []string
}

// Eligibility reason codes. These are the stable identifiers callers switch
// on and persist as job_funnel.status_reason; the human sentence that
// accompanies them is for logs only and is not load-bearing. Before bugs.md
// #554 the reconciler recovered the reason by running strings.Contains over
// that sentence, which silently mapped anything unrecognised to "role".
const (
	// ReasonIneligibleRemote -- the posting is not fully remote.
	ReasonIneligibleRemote = "pruned_ineligible_remote"
	// ReasonIneligibleRole -- the title does not match the configured roles.
	ReasonIneligibleRole = "pruned_ineligible_role"
	// ReasonOutsideAllowedCountries -- positive evidence that every country
	// the posting names sits outside the operator's allowlist.
	ReasonOutsideAllowedCountries = "outside_allowed_countries"
	// ReasonLocationUnknown -- an allowlist is configured but the posting
	// yielded no country evidence, so it is held rather than admitted.
	ReasonLocationUnknown = "location_unknown"
	// ReasonManagementTrackExcluded -- the title's primary track is
	// organizational/people management (Director, VP, Head of, Chief,
	// Manager), which is excluded by default regardless of any engineering
	// keyword it also contains. Opt in via Profile.AllowManagementRoles.
	ReasonManagementTrackExcluded = "management_track_excluded"
	// ReasonSeniorityOutsideTarget -- the title is a Staff/Principal
	// individual-contributor role, which would otherwise be an allowed
	// stretch match, but Profile.RejectStretchSeniority has turned that
	// stretch tier off.
	ReasonSeniorityOutsideTarget = "seniority_outside_target"
	// ReasonRoleTrackMismatch -- the title names neither a configured role
	// phrase nor a distinctive word shared with one. Functionally the same
	// rejection ReasonIneligibleRole has always described; ScreenJob returns
	// this more specific code so callers auditing *why* a title was
	// rejected (task-targeting work, dashboards) can tell a plain role
	// mismatch apart from a management-track or seniority exclusion instead
	// of folding all three into one generic code.
	ReasonRoleTrackMismatch = "role_track_mismatch"
)

// EligibilityResult is the structured verdict of the canonical gate: whether
// the posting may proceed, a stable Code identifying which half rejected it,
// and a log-safe sentence explaining it.
type EligibilityResult struct {
	Eligible bool
	Code     string
	Reason   string
}

// IsEligibleJob is the single hard eligibility gate every stage of the
// pipeline must apply before a job is scored, queued for assisted apply, or
// auto-submitted: appropriate role fit AND fully remote. Neither half can be
// bypassed by the other -- a perfect title never overrides a hybrid signal,
// and a high-scoring posting is never even considered here, because scoring
// happens strictly after this gate, not instead of it.
func IsEligibleJob(job JobEligibilityInput, profile *Profile) (bool, string) {
	result := ScreenJob(job, profile)
	return result.Eligible, result.Reason
}

// ScreenJob is IsEligibleJob with the reason code preserved. New call sites
// should prefer it; IsEligibleJob remains for the ones that only need a bool.
//
// Order is deliberate. Positive out-of-scope geographic evidence is checked
// first and can be overridden by nothing -- not a perfect title, not a
// "Remote" label, not the provider. Unresolvable geography is checked last,
// so a posting that fails the role or remote gate is recorded under that
// concrete reason rather than being filed as merely unlocatable; only a
// posting that would otherwise be actionable gets held for location.
func ScreenJob(job JobEligibilityInput, profile *Profile) EligibilityResult {
	if profile == nil {
		return EligibilityResult{Eligible: true}
	}
	geography, geoReason := GeographyEligible(job.Location, job.CountryCodes, profile.AllowedCountries)
	if geography == GeographyOutside {
		return EligibilityResult{Code: ReasonOutsideAllowedCountries, Reason: geoReason}
	}
	if profile.RemoteOnly {
		if ok, reason := RemoteEligible(job.RemoteClaimed, job.Location, job.Description); !ok {
			return EligibilityResult{Code: ReasonIneligibleRemote, Reason: reason}
		}
	}
	cls := ClassifyTitle(job.Title, profile.Roles, profile.AllowManagementRoles, !profile.RejectStretchSeniority)
	if cls.Fit == FitReject {
		reason := "title does not match the configured role list"
		switch cls.Reason {
		case ReasonManagementTrackExcluded:
			reason = "title's primary track is organizational/people management, not individual-contributor engineering"
		case ReasonSeniorityOutsideTarget:
			reason = "title is a Staff/Principal stretch match, but stretch seniority is disabled"
		}
		return EligibilityResult{Code: cls.Reason, Reason: reason}
	}
	if geography == GeographyUnknown {
		return EligibilityResult{Code: ReasonLocationUnknown, Reason: geoReason}
	}
	return EligibilityResult{Eligible: true}
}

// nonRemoteSignalPhrases are wording that contradicts a "fully remote" claim,
// or otherwise leaves the arrangement ambiguous. Every entry is already
// normalized (lowercase, punctuation reduced to single spaces) so it can be
// matched against normalizeForRemoteCheck's output directly. Presence of any
// one of these is treated as disqualifying regardless of how the posting
// otherwise advertises itself -- "remote" in the title or a feed's own
// workplaceType field never outweighs the posting's own description (that is
// the entire point of this list: catching the conflicting-signal case).
var nonRemoteSignalPhrases = []string{
	"hybrid",
	"on site", "onsite", "in office", "in the office", "in our office",
	"in person",
	"relocation required", "must relocate", "requires relocation", "relocate to",
	"willing to relocate",
	"commuting distance", "commutable distance", "within commuting distance",
	"office attendance",
	"days a week in office", "days per week in office", "day a week in office",
	"day per week in office", "office days", "days in the office",
	"remote hybrid",
	"flexible workplace",
	"mostly remote", "partially remote", "primarily remote",
	"depending on location",
	"work from home several days",
	"periodic office visit", "monthly office visit", "quarterly office visit",
	"occasional office visit", "occasional office attendance",
	"return to office",
	"onsite requirement", "on site requirement",
}

// normalizeForRemoteCheck lowercases value and collapses every run of
// non-alphanumeric characters to a single space, mirroring
// scraper.normalizeLocationText's word-boundary approach so a naive substring
// scan cannot mismatch "hybrid" inside an unrelated word, and so punctuation
// variants ("on-site", "on/site", "remote/hybrid") all normalize identically.
func normalizeForRemoteCheck(value string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(value) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		} else {
			b.WriteByte(' ')
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

// RemoteEligible is the hard "fully remote" gate. It rejects on either of two
// independent grounds:
//
//  1. No positive claim of remote work at all (remoteClaimed is false). This
//     covers on-site jobs, hybrid jobs the upstream source already flagged,
//     and -- deliberately -- any posting whose remote status could not be
//     confidently established upstream: the task requires defaulting to
//     rejection on ambiguity, unlike the geographic allowlist in location.go,
//     which fails open for a different, already-documented reason (bug #516).
//  2. A conflicting or ambiguous phrase appears in the location or
//     description text, even when remoteClaimed is true. A posting that
//     calls itself "Remote" while requiring hybrid office attendance is
//     exactly the case this half exists to catch.
func RemoteEligible(remoteClaimed bool, location, description string) (bool, string) {
	combined := " " + normalizeForRemoteCheck(location+" "+description) + " "
	for _, phrase := range nonRemoteSignalPhrases {
		p := " " + strings.TrimSpace(phrase) + " "
		if strings.Contains(combined, p) {
			return false, "conflicting or ambiguous remote signal: " + strings.TrimSpace(phrase)
		}
	}
	// remoteClaimed is the caller's best positive evidence so far (e.g. a
	// feed's explicit workplaceType=="remote", or a persisted job_funnel.
	// is_remote flag). It is not the only way to establish that: a
	// description that plainly says "remote" and named no disqualifying
	// phrase above is just as good, and is needed because some discovery
	// sources (RemoteOK, Jobicy, Hacker News) publish no structured remote
	// flag at intake at all. Absent either kind of positive evidence, the
	// posting's remote status has not been confidently established, and the
	// rule is to default to rejection rather than guess.
	if remoteClaimed || strings.Contains(combined, " remote ") {
		return true, "fully remote"
	}
	return false, "not confirmed fully remote"
}

// distinctiveRoleWords are single tokens worth matching on their own when a
// title does not contain a full configured role phrase. Words like "senior",
// "engineer" or "developer" appear in nearly every technical title and would
// let almost anything through, defeating the point of the filter.
//
// bugs.md #557: this list used to also include generic tokens --
// "systems", "operations", "support", "network", "security", "api",
// "production", "cloud" -- that appear routinely in ordinary
// non-engineering titles too ("Senior Business Systems Analyst", "GTM
// Operations Lead", "Cloud Support Administrator", ...). They were removed
// entirely rather than narrowed: every configured role in profile.yaml that
// legitimately needs one of them ("Cloud Platform Engineer", "Production
// Support Engineer", "Network Automation Engineer", "Cloud Systems
// Administrator") is itself a full phrase, so it already matches via the
// phrase check in matchesConfiguredRole -- this single-word fallback was
// never actually load-bearing for those words, only for the *other*
// distinctive word in the same phrase (reliability, automation,
// infrastructure, systems->dropped, ...). "platform" is kept, but
// matchesConfiguredRole gives it an extra structural requirement (see
// platformFollowedByOccupationNoun) because it is generic enough to also
// appear as a trailing product/business-unit tag ("Product Specialist:
// Platform") rather than the role's actual track word.
var distinctiveRoleWords = map[string]bool{
	"backend": true, "devops": true, "devsecops": true, "platform": true,
	"infrastructure": true, "reliability": true, "sre": true, "automation": true,
	"python": true, "golang": true, "go": true,
	"observability": true, "kubernetes": true, "ci": true, "cd": true, "sdet": true,
	"integration": true, "release": true,
	"agent": true, "agentic": true, "llm": true, "rag": true,
}

// TitleEligible reports whether title plausibly matches one of the
// configured roles AND is not a management/leadership-track title. It
// matches a full configured role as a phrase, or any distinctive single word
// shared between the title and a configured role, so a genuinely plausible
// title survives to real fit-scoring rather than being discarded here on a
// keyword technicality -- scoring remains the authority on fit; this only
// prevents obviously-unrelated roles, and management-track roles regardless
// of keyword overlap, from consuming a scoring slot or occupying the
// assisted-apply queue.
//
// This is the single canonical title-matching implementation: it backs both
// fresh discovery (pkg/scraper's ATS feed intake) and re-evaluation of
// already-persisted assisted-apply rows, so the two paths cannot drift onto
// different definitions of "matches the role list". It always applies the
// product default (management excluded, Staff/Principal allowed as
// stretch); callers that need the operator's configured opt-ins should use
// TitleEligibleForRoles or ScreenJob/IsEligibleJob directly.
func TitleEligible(title string, roles []string) bool {
	return TitleEligibleForRoles(title, roles, false, true)
}
