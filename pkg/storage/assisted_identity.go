package storage

import (
	"net/url"
	"strings"
)

// This file exists because of bug #521. During the 2026-08-06 acceptance trial
// Veeva listed the same "Senior Software Engineer - Python" requisition in
// Raleigh, Boston, Kansas City and Toronto. The assisted queue rendered them as
// four byte-identical cards -- same company, same role, nothing else -- and the
// operator, having submitted exactly one of them, confirmed two 8 seconds
// apart. The second confirmation wrote a false APPLIED record for a posting
// that was never submitted, which is strictly worse than a missed one: the row
// leaves the actionable queue permanently and DuplicateApplicationExists then
// suppresses any future attempt at an opportunity nobody ever pursued.
//
// The fix is to make a card that shares a company and role with another queued
// card physically impossible to confuse with it, using something the operator
// can check against the posting open in front of them.

// assistedRequisitionSeparators are path segments that decorate a posting URL
// rather than identify the requisition. Lever appends "apply" to its
// application form and Greenhouse nests requisitions under "jobs", so the
// identifier is the last segment that is none of these.
var assistedRequisitionSeparators = map[string]bool{
	"apply":       true,
	"application": true,
	"job":         true,
	"jobs":        true,
	"posting":     true,
	"postings":    true,
}

// assistedRequisitionParams are query parameters ATSes use to carry the
// requisition when the posting is served from an employer's own careers page,
// where the path identifies the site rather than the job.
var assistedRequisitionParams = []string{"gh_jid", "jobId", "job_id", "requisitionId", "id"}

// assistedRequisitionDepth bounds how far left of the URL the search may walk.
// Greenhouse and Lever put the requisition last, but JazzHR
// (`/apply/<id>/<role-slug>`), Workday (`/job/<location>/<slug>_<req>`) and
// several employer career sites (`/job/<req>/<role-slug>`) put a human-readable
// slug after it. Three segments reaches all of those and still stops short of
// the employer and board names, which are identical across duplicate postings
// and would distinguish nothing while looking like they had.
const assistedRequisitionDepth = 3

// AssistedRequisitionID extracts the ATS requisition identifier from a posting
// URL so two postings of the same role at the same company can be told apart.
//
// It is deliberately conservative: a value it cannot recognise as an
// identifier yields an empty string, because a wrong identifier on a card is
// worse than none at all -- an operator who checks a distinguisher and finds it
// matching stops looking, so it must never appear to match by accident. That is
// why a candidate must contain a digit; every ATS requisition observed here
// (Greenhouse "293750", Lever UUIDs, Workday "JR-12345", Ashby UUIDs) does,
// while the decorative slugs a URL ends with ("apply", "senior-engineer") do
// not.
func AssistedRequisitionID(rawURL string) string {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return ""
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return ""
	}
	query := parsed.Query()
	for _, param := range assistedRequisitionParams {
		if candidate := strings.TrimSpace(query.Get(param)); isAssistedRequisitionID(candidate) {
			return candidate
		}
	}
	segments := strings.Split(parsed.EscapedPath(), "/")
	examined := 0
	for i := len(segments) - 1; i >= 0 && examined < assistedRequisitionDepth; i-- {
		segment, err := url.PathUnescape(segments[i])
		if err != nil {
			continue
		}
		segment = strings.TrimSpace(segment)
		if segment == "" || assistedRequisitionSeparators[strings.ToLower(segment)] {
			continue
		}
		examined++
		if isAssistedRequisitionID(segment) {
			return segment
		}
		// Workday builds its last segment as `<role-slug>_<requisition>`, which
		// is far too long to be an identifier on its own but ends in one.
		if tail := segment[strings.LastIndex(segment, "_")+1:]; tail != segment && isAssistedRequisitionID(tail) {
			return tail
		}
	}
	return ""
}

// isAssistedRequisitionID accepts a candidate only if it is plausibly a machine
// identifier rather than a human-readable slug. Two shapes qualify, both taken
// from postings actually sitting in the live queue:
//
//   - it carries a digit (Greenhouse "293750", Lever UUIDs, Workday "req-4917",
//     JazzHR "fqdsbx66if"). A purely numeric candidate is accepted at any
//     length, since a short number can only be an identifier, while a short
//     mixed token ("en", "US", "v2") is more likely a locale or path decoration.
//   - it is an unseparated token carrying an interior capital (Jobvite
//     "orrtAfwF"). The capital must not be the leading one, or ordinary
//     capitalised path words ("Workday", "Careers") would qualify.
//
// Everything else is refused, because a wrong identifier on a card is worse
// than none: an operator who checks a distinguisher and finds it matching stops
// looking, so it must never appear to match by accident.
func isAssistedRequisitionID(candidate string) bool {
	if candidate == "" || len(candidate) > 64 {
		return false
	}
	digits, letters, separators, interiorCapital := false, false, false, false
	for i, r := range candidate {
		switch {
		case r >= '0' && r <= '9':
			digits = true
		case r >= 'A' && r <= 'Z':
			letters = true
			if i > 0 {
				interiorCapital = true
			}
		case r >= 'a' && r <= 'z':
			letters = true
		case r == '-', r == '_':
			separators = true
		default:
			return false
		}
	}
	if digits {
		return !letters || len(candidate) >= 3
	}
	return interiorCapital && !separators && len(candidate) >= 6
}

// assistedDuplicateKey groups queue rows that an operator cannot tell apart
// from the card heading alone. It reuses the normalization the confirmed
// application duplicate matcher already applies to company and title, and
// deliberately excludes location: location is one of the things that makes two
// otherwise identical postings distinct, so folding it into the key would hide
// exactly the rows the operator most needs warning about.
func assistedDuplicateKey(company, role string) string {
	normalizedCompanyName := normalizedCompany(company)
	roleWords, seniority := normalizedRole(role)
	if normalizedCompanyName == "" || roleWords == "" {
		return ""
	}
	return normalizedCompanyName + "\x00" + roleWords + "\x00" + seniority
}

// markAssistedDuplicates records, for each row, how many other rows in the same
// queue share its company and role, and whether this row's distinguishers
// actually tell it apart from those siblings. The dashboard uses both to warn
// before a confirmation, which is the click that writes an irreversible APPLIED
// record.
//
// Ambiguity is reported rather than papered over. A row whose location and
// requisition are both absent -- or, worse, identical to a sibling's -- must
// say so, because a distinguisher an operator checks and finds matching is the
// exact failure this whole file exists to prevent.
func markAssistedDuplicates(jobs []AssistedJob) {
	counts := make(map[string]int, len(jobs))
	distinguishers := make(map[string]int, len(jobs))
	for _, job := range jobs {
		key := assistedDuplicateKey(job.Company, job.Role)
		if key == "" {
			continue
		}
		counts[key]++
		distinguishers[key+"\x00"+job.Location+"\x00"+job.RequisitionID]++
	}
	for i := range jobs {
		key := assistedDuplicateKey(jobs[i].Company, jobs[i].Role)
		if key == "" {
			continue
		}
		siblings := counts[key] - 1
		if siblings <= 0 {
			continue
		}
		jobs[i].DuplicateSiblings = siblings
		hasDistinguisher := jobs[i].Location != "" || jobs[i].RequisitionID != ""
		unique := distinguishers[key+"\x00"+jobs[i].Location+"\x00"+jobs[i].RequisitionID] == 1
		jobs[i].Ambiguous = !hasDistinguisher || !unique
	}
}
