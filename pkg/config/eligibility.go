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
}

// IsEligibleJob is the single hard eligibility gate every stage of the
// pipeline must apply before a job is scored, queued for assisted apply, or
// auto-submitted: appropriate role fit AND fully remote. Neither half can be
// bypassed by the other -- a perfect title never overrides a hybrid signal,
// and a high-scoring posting is never even considered here, because scoring
// happens strictly after this gate, not instead of it.
func IsEligibleJob(job JobEligibilityInput, profile *Profile) (bool, string) {
	if profile == nil {
		return true, ""
	}
	if profile.RemoteOnly {
		if ok, reason := RemoteEligible(job.RemoteClaimed, job.Location, job.Description); !ok {
			return false, reason
		}
	}
	if !TitleEligible(job.Title, profile.Roles) {
		return false, "title does not match the configured role list"
	}
	return true, ""
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
var distinctiveRoleWords = map[string]bool{
	"backend": true, "devops": true, "devsecops": true, "platform": true,
	"infrastructure": true, "reliability": true, "sre": true, "automation": true,
	"python": true, "golang": true, "go": true, "cloud": true, "security": true,
	"observability": true, "kubernetes": true, "ci": true, "cd": true, "sdet": true,
	"integration": true, "network": true, "systems": true, "api": true,
	"production": true, "release": true, "support": true, "operations": true,
	"agent": true, "agentic": true, "llm": true, "rag": true,
}

// TitleEligible reports whether title plausibly matches one of the
// configured roles. It matches a full configured role as a phrase, or any
// distinctive single word shared between the title and a configured role, so
// a genuinely plausible title survives to real fit-scoring rather than being
// discarded here on a keyword technicality -- scoring remains the authority
// on fit; this only prevents obviously-unrelated roles from consuming a
// scoring slot or occupying the assisted-apply queue.
//
// This is the single canonical title-matching implementation: it backs both
// fresh discovery (pkg/scraper's ATS feed intake) and re-evaluation of
// already-persisted assisted-apply rows, so the two paths cannot drift onto
// different definitions of "matches the role list".
func TitleEligible(title string, roles []string) bool {
	if len(roles) == 0 {
		return true // no configured roles: do not silently filter everything out
	}
	t := normalizeForRemoteCheck(title)
	if t == "" {
		return false
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
		// Full configured role as a phrase is safe to substring-match: it is
		// long and specific enough not to collide accidentally.
		if strings.Contains(" "+t+" ", " "+r+" ") || t == r {
			return true
		}
		for _, word := range strings.Fields(r) {
			if distinctiveRoleWords[word] && titleWords[word] {
				return true
			}
		}
	}
	return false
}
