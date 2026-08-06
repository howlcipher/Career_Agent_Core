package scraper

import "strings"

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
	"canada":                   "CA",
	"mexico":                   "MX",
	"india":                    "IN",
	"united kingdom":           "GB",
	"uk":                       "GB",
	"england":                  "GB",
	"scotland":                 "GB",
	"wales":                    "GB",
	"northern ireland":         "GB",
	"ireland":                  "IE",
	"germany":                  "DE",
	"france":                   "FR",
	"spain":                    "ES",
	"portugal":                 "PT",
	"poland":                   "PL",
	"netherlands":              "NL",
	"belgium":                  "BE",
	"switzerland":              "CH",
	"austria":                  "AT",
	"italy":                    "IT",
	"sweden":                   "SE",
	"norway":                   "NO",
	"denmark":                  "DK",
	"finland":                  "FI",
	"romania":                  "RO",
	"ukraine":                  "UA",
	"czechia":                  "CZ",
	"czech republic":           "CZ",
	"turkey":                   "TR",
	"israel":                   "IL",
	"united arab emirates":     "AE",
	"egypt":                    "EG",
	"nigeria":                  "NG",
	"kenya":                    "KE",
	"south africa":             "ZA",
	"brazil":                   "BR",
	"argentina":                "AR",
	"colombia":                 "CO",
	"chile":                    "CL",
	"peru":                     "PE",
	"australia":                "AU",
	"new zealand":              "NZ",
	"singapore":                "SG",
	"japan":                    "JP",
	"china":                    "CN",
	"hong kong":                "HK",
	"taiwan":                   "TW",
	"south korea":              "KR",
	"korea":                    "KR",
	"philippines":              "PH",
	"pakistan":                 "PK",
	"bangladesh":               "BD",
	"indonesia":                "ID",
	"vietnam":                  "VN",
	"thailand":                 "TH",
	"malaysia":                 "MY",
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
