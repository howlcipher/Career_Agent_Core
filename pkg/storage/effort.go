package storage

import (
	"database/sql"
	"strings"
)

// assistedATSProviders is the single source of truth for which ATS platforms
// this project has hand-written support for.
//
// It lives here rather than in pkg/submitter because two things need the same
// answer and only one of them can own it: the fill path needs the handler, and
// the effort model needs to know whether a handler exists. pkg/submitter
// already imports pkg/storage, so this direction is the one that does not
// create a cycle, and submitter.dedicatedATSHandler now routes off this table
// rather than keeping a second copy of the domain list that could drift from
// it.
var assistedATSProviders = []struct{ domain, name string }{
	{"greenhouse.io", "Greenhouse"},
	{"lever.co", "Lever"},
	{"ashbyhq.com", "Ashby"},
}

// SupportedAssistedATS names the ATS behind rawURL when this project has a
// dedicated handler for it, and "" otherwise.
func SupportedAssistedATS(rawURL string) string {
	lowered := strings.ToLower(rawURL)
	for _, provider := range assistedATSProviders {
		if strings.Contains(lowered, provider.domain) {
			return provider.name
		}
	}
	return ""
}

// Effort bands. Three is deliberate: this model has no basis for finer
// resolution, and a fourth band would imply a precision it does not have.
const (
	EffortLow    = "LOW"
	EffortMedium = "MEDIUM"
	EffortHigh   = "HIGH"
)

// AssistedEffortInput is everything the estimate is derived from. Every field
// is a fact this project already records; none of it is inferred from the
// employer's page.
type AssistedEffortInput struct {
	PostingURL        string
	OriginalStatus    string
	ResumeReady       bool
	CoverLetterReady  bool
	PendingQuestions  int
	MappingHealthy    bool
	AccountRequired   bool
	CaptchaEncounters bool
}

// EstimateAssistedEffort reports roughly how much of the operator's own time
// one application will cost.
//
// It is deliberately a band and a range, not a number. The honest state of
// this project's knowledge is "a supported ATS with everything prepared takes
// a minute or two, and an account gate takes several" — and a model that
// printed "73 seconds" would be claiming a precision nothing here supports.
// The signals are returned alongside so the operator can see *why* an
// application is expensive rather than being asked to trust the band.
func EstimateAssistedEffort(input AssistedEffortInput) AssistedEffort {
	effort := AssistedEffort{Band: EffortLow, LowMinutes: 1, HighMinute: 2}
	add := func(low, high int, signal string) {
		effort.LowMinutes += low
		effort.HighMinute += high
		effort.Signals = append(effort.Signals, signal)
	}

	// An ATS that refuses the assisted browser costs the most: the operator
	// does the whole application themselves in their own browser.
	if reason := AssistedBrowserRejectionReason(input.PostingURL); reason != "" {
		return AssistedEffort{
			Band: EffortHigh, LowMinutes: 5, HighMinute: 10,
			Signals: []string{"this ATS rejects the assisted browser, so the whole application happens in your own"},
		}
	}

	if ats := SupportedAssistedATS(input.PostingURL); ats != "" {
		effort.Signals = append(effort.Signals, "dedicated "+ats+" handler")
	} else if input.MappingHealthy {
		effort.Signals = append(effort.Signals, "reusable form mapping is healthy")
	} else {
		add(2, 4, "no dedicated handler and no healthy form mapping for this ATS")
	}

	if !input.ResumeReady {
		add(1, 2, "résumé not prepared yet")
	}
	if !input.CoverLetterReady {
		add(0, 1, "cover letter not prepared yet")
	}
	if input.AccountRequired {
		add(2, 4, "an account or login is expected before the form")
	}
	if input.CaptchaEncounters {
		add(1, 2, "a human verification step blocked this application before")
	}
	switch {
	case input.PendingQuestions >= 5:
		add(2, 4, "several questions need your judgement")
	case input.PendingQuestions > 0:
		add(1, 2, "a few questions need your judgement")
	}

	switch {
	case effort.HighMinute >= 8:
		effort.Band = EffortHigh
	case effort.HighMinute >= 4:
		effort.Band = EffortMedium
	default:
		effort.Band = EffortLow
	}
	return effort
}

// effortAdjustment is how far the effort model may move a job's rank, as a
// fraction either side of 1.0.
//
// The size was chosen against the failure it exists to prevent, not picked for
// roundness. At ±15% — the first value this shipped with — the swing between
// the best and worst band is 1.35×, enough to lift a fit-74 one-click job above
// a fit-92 realistic one; the ordering test caught exactly that. At ±4% the
// full swing is about 8%, which can reorder jobs within roughly seven fit
// points of each other and cannot touch a gap wider than that.
//
// That is the intended power. Application ease is a real signal — an excellent
// job on an unsupported ATS behind an account gate genuinely does cost more
// than the same job on Greenhouse — but it is a much weaker signal than whether
// the job suits the applicant, and it must only ever break ties.
const effortAdjustment = 0.04

// EffortMultiplier converts an effort band into a bounded ranking adjustment.
func EffortMultiplier(band string) float64 {
	switch band {
	case EffortLow:
		return 1 + effortAdjustment
	case EffortHigh:
		return 1 - effortAdjustment
	default:
		return 1.0
	}
}

// mappingHealthy reports whether a cached form mapping for this posting's
// domain is good enough to reuse. A missing mapping and an unhealthy one are
// the same answer here: neither will save the operator any time.
func mappingHealthy(postingURL string) bool {
	if db == nil {
		return false
	}
	domain := postingDomain(postingURL)
	if domain == "" {
		return false
	}
	healthy, err := PreferCachedFormMapping(domain)
	return err == nil && healthy
}

// mappingHealthyOn is mappingHealthy against an explicit connection, for the
// queue projection, which never assumes the package singleton is initialized.
func mappingHealthyOn(conn *sql.DB, postingURL string) bool {
	if conn == nil {
		return false
	}
	domain := postingDomain(postingURL)
	if domain == "" {
		return false
	}
	var successes, failures int
	var lastValidated sql.NullTime
	err := conn.QueryRow("SELECT success_count, failure_count, last_validated_at FROM form_mappings WHERE domain = ?", domain).
		Scan(&successes, &failures, &lastValidated)
	if err != nil {
		return false
	}
	return successes >= failures
}

// postingDomain reduces a posting URL to the host key form_mappings is keyed
// by. It mirrors submitter.ExtractDomain rather than importing it, because
// that package imports this one.
func postingDomain(rawURL string) string {
	lowered := strings.ToLower(strings.TrimSpace(rawURL))
	lowered = strings.TrimPrefix(strings.TrimPrefix(lowered, "https://"), "http://")
	if index := strings.IndexAny(lowered, "/?#"); index >= 0 {
		lowered = lowered[:index]
	}
	return strings.TrimPrefix(lowered, "www.")
}
