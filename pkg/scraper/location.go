package scraper

import "github.com/howlcipher/Career_Agent_Core/pkg/config"

// The country resolver itself now lives in pkg/config, beside the
// Profile.AllowedCountries it reads and the config.IsEligibleJob gate that
// must consult it (bugs.md #554). It had to move down rather than be imported
// up: pkg/scraper already imports pkg/config, so a country check inside
// IsEligibleJob would have closed an import cycle.
//
// These two wrappers exist so the discovery-time call sites that predate that
// move -- pollBoard in atsfeeds.go and the cmd/backfill-location screening
// report -- keep reading the same names they always did. Discovery keeps the
// fail-open contract #516 shipped with; the fail-closed decision is made in
// pkg/config by GeographyEligible, on the eligibility path only.

// CountryCodesFor resolves the country evidence for one posting. See
// config.CountryCodesFor for the matching rules.
func CountryCodesFor(location string, explicitCodes []string) []string {
	return config.CountryCodesFor(location, explicitCodes)
}

// LocationAllowed reports whether a posting may proceed at discovery time,
// given an ISO-3166 alpha-2 allowlist. Unknown location is allowed here, which
// is the deliberate #516 intake policy; see config.GeographyEligible for the
// stricter rule the actionable queue applies.
func LocationAllowed(location string, explicitCodes []string, allowed []string) (bool, string) {
	return config.LocationAllowed(location, explicitCodes, allowed)
}
