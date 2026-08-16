package config

import (
	"sort"
	"strings"
)

// Career Agent had no geographic filter of any kind until bug #516: every one
// of the 12,902 rows in job_funnel carried an empty job_location, profile.yaml
// offered only remote_only (which says nothing about *where* remote work is
// permitted from), and an India-scoped Lever posting consequently reached a
// live Assisted Apply attempt with a fit score of 100. The board feeds we
// already poll publish the answer — Lever returns an ISO-3166 country code per
// posting — so the gate below costs one map lookup per posting and no network.

// countryNamesToCodes maps location wording that appears in real board feeds to
// its ISO-3166 alpha-2 code. It is deliberately partial: it covers the markets
// that actually show up in this funnel, and anything unrecognised is treated as
// "no evidence" rather than guessed at.
var countryNamesToCodes = map[string]string{
	"united states":            "US",
	"united states of america": "US",
	"usa":                      "US",
	"u s a":                    "US",
	"u s":                      "US",
	"america":                  "US",
	// Bare "US". Safe only because matching is whole-word on normalized text:
	// "Columbus, Ohio" normalizes to "columbus ohio", which contains no
	// free-standing "us" token. Added with bugs.md #554 -- until then "US",
	// "Remote US", "US - Remote" and "US (Remote)" all resolved to no country
	// at all, which mattered the moment unresolvable geography stopped being
	// treated as permission: 14 genuinely-US rows in the live queue would
	// have been held as unlocatable. ("uk" is already covered below.)
	"us":                     "US",
	"canada":                 "CA",
	"mexico":                 "MX",
	"india":                  "IN",
	"united kingdom":         "GB",
	"uk":                     "GB",
	"england":                "GB",
	"scotland":               "GB",
	"wales":                  "GB",
	"northern ireland":       "GB",
	"ireland":                "IE",
	"germany":                "DE",
	"france":                 "FR",
	"spain":                  "ES",
	"portugal":               "PT",
	"poland":                 "PL",
	"netherlands":            "NL",
	"belgium":                "BE",
	"switzerland":            "CH",
	"austria":                "AT",
	"italy":                  "IT",
	"sweden":                 "SE",
	"norway":                 "NO",
	"denmark":                "DK",
	"finland":                "FI",
	"romania":                "RO",
	"ukraine":                "UA",
	"czechia":                "CZ",
	"czech republic":         "CZ",
	"turkey":                 "TR",
	"israel":                 "IL",
	"united arab emirates":   "AE",
	"egypt":                  "EG",
	"nigeria":                "NG",
	"kenya":                  "KE",
	"south africa":           "ZA",
	"brazil":                 "BR",
	"argentina":              "AR",
	"colombia":               "CO",
	"chile":                  "CL",
	"peru":                   "PE",
	"australia":              "AU",
	"new zealand":            "NZ",
	"singapore":              "SG",
	"japan":                  "JP",
	"china":                  "CN",
	"hong kong":              "HK",
	"taiwan":                 "TW",
	"south korea":            "KR",
	"korea":                  "KR",
	"philippines":            "PH",
	"pakistan":               "PK",
	"bangladesh":             "BD",
	"indonesia":              "ID",
	"vietnam":                "VN",
	"thailand":               "TH",
	"malaysia":               "MY",
	"estonia":                "EE",
	"latvia":                 "LV",
	"lithuania":              "LT",
	"hungary":                "HU",
	"bulgaria":               "BG",
	"croatia":                "HR",
	"slovenia":               "SI",
	"slovakia":               "SK",
	"serbia":                 "RS",
	"greece":                 "GR",
	"cyprus":                 "CY",
	"malta":                  "MT",
	"iceland":                "IS",
	"luxembourg":             "LU",
	"montenegro":             "ME",
	"north macedonia":        "MK",
	"bosnia and herzegovina": "BA",
	"albania":                "AL",
	"moldova":                "MD",
	// Note: bare "georgia" is omitted deliberately (bug #524) because it collides
	// with the US state of Georgia. The normalizer matches on whole words, so
	// "Atlanta, Georgia" would otherwise be read as the country GE.
	"armenia":            "AM",
	"morocco":            "MA",
	"tunisia":            "TN",
	"ghana":              "GH",
	"uganda":             "UG",
	"tanzania":           "TZ",
	"ethiopia":           "ET",
	"saudi arabia":       "SA",
	"qatar":              "QA",
	"kuwait":             "KW",
	"jordan":             "JO",
	"lebanon":            "LB",
	"sri lanka":          "LK",
	"nepal":              "NP",
	"cambodia":           "KH",
	"uruguay":            "UY",
	"ecuador":            "EC",
	"bolivia":            "BO",
	"paraguay":           "PY",
	"venezuela":          "VE",
	"costa rica":         "CR",
	"panama":             "PA",
	"guatemala":          "GT",
	"dominican republic": "DO",
	"puerto rico":        "PR",
	"jamaica":            "JM",
}

// normalizeLocationText lowercases value and reduces every run of non
// alphanumeric characters to a single space, so phrase matching can be done on
// whole words. Word boundaries are the point: a naive strings.Contains would
// read "Indiana" as "India" and silently discard every posting in that state.
func normalizeLocationText(value string) string {
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

// CountryCodesFor resolves the country evidence for one posting. Explicit codes
// from the feed win outright; otherwise the free-text location is matched
// against countryNamesToCodes on whole-word boundaries. An empty result means
// "no evidence", which callers must not treat as a rejection.
func CountryCodesFor(location string, explicitCodes []string) []string {
	var out []string
	seen := map[string]struct{}{}
	add := func(code string) {
		code = strings.ToUpper(strings.TrimSpace(code))
		if len(code) != 2 {
			return
		}
		if _, dup := seen[code]; dup {
			return
		}
		seen[code] = struct{}{}
		out = append(out, code)
	}

	for _, code := range explicitCodes {
		add(code)
	}
	if len(out) > 0 {
		return out
	}

	padded := " " + normalizeLocationText(location) + " "
	for name, code := range countryNamesToCodes {
		if strings.Contains(padded, " "+name+" ") {
			add(code)
		}
	}
	return out
}

// LocationAllowed reports whether a posting may proceed, given an ISO-3166
// alpha-2 allowlist. It rejects only on positive evidence that every country
// the posting names is outside the allowlist.
//
// Unknown location is deliberately allowed. bugs.md records the governing
// lesson from the CAPTCHA pre-skip decision — "skipping a job that would have
// submitted is strictly worse than wasting inference, because the goal is
// applications, not throughput" — and a fail-closed gate would additionally
// discard all 12,902 existing rows, none of which carry a location.
//
// The second return value is a short reason suitable for logging; it never
// contains the posting URL or any personal data.
func LocationAllowed(location string, explicitCodes []string, allowed []string) (bool, string) {
	if len(allowed) == 0 {
		return true, "no country allowlist configured"
	}

	permitted := map[string]struct{}{}
	for _, code := range allowed {
		code = strings.ToUpper(strings.TrimSpace(code))
		if code != "" {
			permitted[code] = struct{}{}
		}
	}
	if len(permitted) == 0 {
		return true, "no country allowlist configured"
	}

	codes := CountryCodesFor(location, explicitCodes)
	if len(codes) == 0 {
		return true, "no location evidence"
	}
	for _, code := range codes {
		if _, ok := permitted[code]; ok {
			return true, "allowed country " + code
		}
	}
	return false, "country " + strings.Join(codes, "/") + " is outside the configured allowlist"
}

// GeographyVerdict is the three-way outcome of screening one posting's
// advertised location against the operator's country allowlist.
//
// Three states rather than a boolean because "we know this is out of scope"
// and "we could not tell where this is" call for different handling, and
// collapsing them is exactly how a Jordan posting reached a launchable
// Assisted Apply row (bugs.md #554). LocationAllowed above keeps treating
// both as "allowed" for discovery intake, preserving the #516 contract that
// admitted 2,993 postings; the actionable queue uses this instead.
type GeographyVerdict int

const (
	// GeographyAllowed means the posting names at least one allowed country,
	// or no allowlist is configured at all.
	GeographyAllowed GeographyVerdict = iota
	// GeographyOutside means every country the posting names is outside the
	// allowlist. This is positive evidence and is never overridable -- not by
	// a fit score, not by a "Remote" label, not by the provider.
	GeographyOutside
	// GeographyUnknown means the posting yielded no country evidence at all.
	// The operator asked for a geographic restriction, so this is not treated
	// as permission; it is held for resolution instead.
	GeographyUnknown
)

// GeographyEligible screens one posting's location against an ISO-3166
// alpha-2 allowlist, distinguishing "outside" from "unresolvable".
//
// The second return value is a short reason suitable for logging; like
// LocationAllowed's it never contains the posting URL or any personal data.
func GeographyEligible(location string, explicitCodes []string, allowed []string) (GeographyVerdict, string) {
	permitted := map[string]struct{}{}
	for _, code := range allowed {
		code = strings.ToUpper(strings.TrimSpace(code))
		if code != "" {
			permitted[code] = struct{}{}
		}
	}
	// An empty allowlist disables the gate entirely, exactly as it does for
	// LocationAllowed. The operator has not asked for a restriction, so there
	// is nothing to hold anything against.
	if len(permitted) == 0 {
		return GeographyAllowed, "no country allowlist configured"
	}

	codes := CountryCodesFor(location, explicitCodes)
	if len(codes) == 0 {
		return GeographyUnknown, "no country evidence in the advertised location"
	}
	for _, code := range codes {
		if _, ok := permitted[code]; ok {
			return GeographyAllowed, "allowed country " + code
		}
	}
	return GeographyOutside, "country " + strings.Join(codes, "/") + " is outside the configured allowlist"
}

// KnownCountryCodes returns every ISO-3166 alpha-2 code this resolver can
// recognise from a posting's advertised location, sorted.
//
// It exists so the dashboard's geography selector validates against exactly
// the codes the gate can actually act on, rather than against a second list
// maintained by hand. Offering the operator a country the resolver cannot
// detect would produce an allowlist entry that silently never matches.
func KnownCountryCodes() []string {
	seen := map[string]struct{}{}
	for _, code := range countryNamesToCodes {
		seen[code] = struct{}{}
	}
	out := make([]string, 0, len(seen))
	for code := range seen {
		out = append(out, code)
	}
	sort.Strings(out)
	return out
}

// IsKnownCountryCode reports whether code is one this resolver can detect.
func IsKnownCountryCode(code string) bool {
	code = strings.ToUpper(strings.TrimSpace(code))
	for _, known := range countryNamesToCodes {
		if known == code {
			return true
		}
	}
	return false
}
