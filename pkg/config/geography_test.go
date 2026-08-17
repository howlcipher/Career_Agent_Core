package config

import (
	"reflect"
	"testing"
)

// usCA is the operator's configured scope and the default this whole gate
// exists to enforce (bugs.md #554).
var usCA = []string{"US", "CA"}

// The reproduction. A live assisted_applications row -- webook / Agentic AI
// Engineer, advertised in Amman -- sat in the actionable queue with
// revalidation_state 'application_ready', one click from a browser, while the
// operator's allowlist said US and CA.
func TestGeography_RejectsTheJordanReproduction(t *testing.T) {
	for _, location := range []string{
		"Amman, Amman Governorate, Jordan",
		"Jordan",
		"Remote - Jordan",
	} {
		verdict, reason := GeographyEligible(location, nil, usCA)
		if verdict != GeographyOutside {
			t.Errorf("GeographyEligible(%q) = %v (%s), want GeographyOutside", location, verdict, reason)
		}
	}
	// The same posting reaching the canonical gate must be refused there too,
	// with the stable reason code the funnel persists.
	result := ScreenJob(JobEligibilityInput{
		Title:         "Agentic AI Engineer",
		Location:      "Amman, Amman Governorate, Jordan",
		RemoteClaimed: true,
	}, &Profile{AllowedCountries: usCA, Roles: []string{"AI Engineer"}})
	if result.Eligible || result.Code != ReasonOutsideAllowedCountries {
		t.Fatalf("ScreenJob on the Jordan reproduction = %+v, want ineligible/%s", result, ReasonOutsideAllowedCountries)
	}
}

// An explicit ISO code from a feed is authority and must reject on its own,
// even when the free-text location says nothing useful.
func TestGeography_RejectsExplicitOutOfScopeCountryCode(t *testing.T) {
	verdict, _ := GeographyEligible("Remote", []string{"JO"}, usCA)
	if verdict != GeographyOutside {
		t.Fatalf("explicit JO = %v, want GeographyOutside", verdict)
	}
}

func TestGeography_AcceptsConfiguredCountries(t *testing.T) {
	for _, tc := range []struct {
		location string
		codes    []string
	}{
		{"Austin, Texas, United States", nil},
		{"United States", nil},
		{"USA", nil},
		{"U.S.", nil},
		{"Remote - US", nil},
		{"US", nil},
		{"US - Remote", nil},
		{"US (Remote)", nil},
		{"Remote US", nil},
		{"U.S.A Remote", nil},
		{"", []string{"US"}},
		{"Toronto, Ontario, Canada", nil},
		{"Canada", nil},
		{"Remote - Canada", nil},
		{"", []string{"CA"}},
	} {
		verdict, reason := GeographyEligible(tc.location, tc.codes, usCA)
		if verdict != GeographyAllowed {
			t.Errorf("GeographyEligible(%q, %v) = %v (%s), want GeographyAllowed",
				tc.location, tc.codes, verdict, reason)
		}
	}
}

func TestGeography_RejectsOtherCountries(t *testing.T) {
	for _, tc := range []struct {
		location string
		codes    []string
	}{
		{"Bengaluru, Karnataka, India", nil},
		{"India", nil},
		{"", []string{"IN"}},
		{"London, England, United Kingdom", nil},
		{"United Kingdom", nil},
		{"", []string{"GB"}},
		{"Mexico City, Mexico", nil},
		{"", []string{"MX"}},
	} {
		verdict, reason := GeographyEligible(tc.location, tc.codes, usCA)
		if verdict != GeographyOutside {
			t.Errorf("GeographyEligible(%q, %v) = %v (%s), want GeographyOutside",
				tc.location, tc.codes, verdict, reason)
		}
	}
}

// Mexico is in scope under the North America preset and out of scope under
// US+CA. The preset the operator picked is the only thing that decides it --
// "North America" is never inferred to mean one or the other.
func TestGeography_MexicoFollowsTheSelectedPreset(t *testing.T) {
	if verdict, _ := GeographyEligible("Mexico City, Mexico", nil, GeographyPresetUSCA); verdict != GeographyOutside {
		t.Errorf("Mexico under US+CA = %v, want GeographyOutside", verdict)
	}
	if verdict, _ := GeographyEligible("Mexico City, Mexico", nil, GeographyPresetNorthAmerica); verdict != GeographyAllowed {
		t.Errorf("Mexico under North America = %v, want GeographyAllowed", verdict)
	}
	// The preset memberships themselves are the contract; a silent change to
	// what "North America" includes is exactly what this asserts against.
	if !reflect.DeepEqual(GeographyPresetUSCA, []string{"US", "CA"}) {
		t.Errorf("US+CA preset = %v", GeographyPresetUSCA)
	}
	if !reflect.DeepEqual(GeographyPresetNorthAmerica, []string{"US", "CA", "MX"}) {
		t.Errorf("North America preset = %v", GeographyPresetNorthAmerica)
	}
	if len(GeographyPresetWorldwide) != 0 || GeographyPresetWorldwide == nil {
		t.Errorf("Worldwide must be a non-nil empty allowlist, got %#v", GeographyPresetWorldwide)
	}
}

// The single most important false positive: a US state whose name contains a
// country name. Indiana must never be read as India.
func TestGeography_IndianaIsNeverIndia(t *testing.T) {
	for _, location := range []string{
		"Indiana",
		"Indianapolis, Indiana, United States",
		"Indiana, United States",
		// The bare-"us" entry added for #554 must not start reading a city
		// whose name merely ends in those letters as the country.
		"Columbus, Ohio, United States",
	} {
		if codes := CountryCodesFor(location, nil); contains(codes, "IN") {
			t.Errorf("CountryCodesFor(%q) = %v, must not resolve India", location, codes)
		}
		if verdict, reason := GeographyEligible(location, nil, usCA); verdict == GeographyOutside {
			t.Errorf("GeographyEligible(%q) rejected a US state: %s", location, reason)
		}
	}
}

// The mirror-image false positive: "CA" in prose is California far more often
// than it is Canada, so free text may never resolve it. An explicit country
// code field is a different kind of evidence and may.
func TestGeography_BareCAInProseIsNeverCanada(t *testing.T) {
	for _, location := range []string{
		"San Francisco, CA",
		"Los Angeles, CA, United States",
		"Remote - CA",
	} {
		if codes := CountryCodesFor(location, nil); contains(codes, "CA") {
			t.Errorf("CountryCodesFor(%q) = %v, must not read CA as Canada", location, codes)
		}
	}
	if codes := CountryCodesFor("", []string{"CA"}); !contains(codes, "CA") {
		t.Error("an explicit CA country code must resolve to Canada")
	}
}

// Unknown geography is held, not admitted. The operator asked for a
// restriction; silence is not permission.
func TestGeography_UnknownIsHeldNotAllowed(t *testing.T) {
	for _, location := range []string{"", "Remote", "Anywhere", "Worldwide", "Remote - EMEA"} {
		verdict, _ := GeographyEligible(location, nil, usCA)
		if verdict != GeographyUnknown {
			t.Errorf("GeographyEligible(%q) = %v, want GeographyUnknown", location, verdict)
		}
	}
	// ...and it reaches the canonical gate as its own reason code, so the
	// funnel can record why the row is not actionable without pretending it
	// was a role or remote failure.
	result := ScreenJob(JobEligibilityInput{
		Title:         "Platform Engineer",
		Location:      "Remote",
		RemoteClaimed: true,
	}, &Profile{AllowedCountries: usCA, Roles: []string{"Platform Engineer"}})
	if result.Eligible || result.Code != ReasonLocationUnknown {
		t.Fatalf("unknown-location screen = %+v, want ineligible/%s", result, ReasonLocationUnknown)
	}
}

// An empty allowlist means the operator has not asked for a restriction, so
// nothing is held and nothing is rejected.
func TestGeography_EmptyAllowlistDisablesTheGate(t *testing.T) {
	for _, location := range []string{"Amman, Amman Governorate, Jordan", "Remote", "India"} {
		if verdict, _ := GeographyEligible(location, nil, nil); verdict != GeographyAllowed {
			t.Errorf("GeographyEligible(%q) with no allowlist = %v, want GeographyAllowed", location, verdict)
		}
	}
}

// Nothing may override positive out-of-scope evidence: not a "Remote" claim,
// not a perfect title. Fit score never even reaches this gate -- scoring runs
// strictly after it -- which is asserted by its absence from the input type.
func TestGeography_NothingOverridesPositiveOutOfScopeEvidence(t *testing.T) {
	profile := &Profile{
		RemoteOnly:       true,
		Roles:            []string{"Platform Engineer"},
		AllowedCountries: usCA,
	}
	result := ScreenJob(JobEligibilityInput{
		Title:         "Platform Engineer", // a perfect title match
		Location:      "Amman, Amman Governorate, Jordan",
		Description:   "Fully remote, work from anywhere.",
		RemoteClaimed: true, // and an explicit remote claim
	}, profile)
	if result.Eligible {
		t.Fatal("a remote-labelled, perfectly-titled posting outside the allowlist must still be refused")
	}
	if result.Code != ReasonOutsideAllowedCountries {
		t.Fatalf("geography must be the reason, got %s (%s)", result.Code, result.Reason)
	}
}

// Geography is additive to the existing gates, never a replacement: the role
// and remote rules must behave exactly as they did before #554.
func TestGeography_LeavesRoleAndRemoteGatesIntact(t *testing.T) {
	profile := &Profile{
		RemoteOnly:       true,
		Roles:            []string{"Platform Engineer"},
		AllowedCountries: usCA,
	}
	inScope := "Austin, Texas, United States"

	hybrid := ScreenJob(JobEligibilityInput{
		Title: "Platform Engineer", Location: inScope,
		Description: "This is a hybrid role, 3 days a week in office.", RemoteClaimed: true,
	}, profile)
	if hybrid.Eligible || hybrid.Code != ReasonIneligibleRemote {
		t.Fatalf("hybrid in-scope job = %+v, want ineligible/%s", hybrid, ReasonIneligibleRemote)
	}

	wrongRole := ScreenJob(JobEligibilityInput{
		Title: "Dental Hygienist", Location: inScope, RemoteClaimed: true,
	}, profile)
	if wrongRole.Eligible || wrongRole.Code != ReasonRoleTrackMismatch {
		t.Fatalf("off-role in-scope job = %+v, want ineligible/%s", wrongRole, ReasonRoleTrackMismatch)
	}

	good := ScreenJob(JobEligibilityInput{
		Title: "Platform Engineer", Location: inScope, RemoteClaimed: true,
	}, profile)
	if !good.Eligible {
		t.Fatalf("an in-scope, remote, on-role job must stay eligible, got %+v", good)
	}
}

// A posting that fails role or remote is recorded under that concrete reason
// rather than being filed as merely unlocatable, so "held for location" only
// ever describes a row that would otherwise be actionable.
func TestGeography_ConcreteFailuresOutrankUnknownLocation(t *testing.T) {
	profile := &Profile{RemoteOnly: true, Roles: []string{"Platform Engineer"}, AllowedCountries: usCA}
	result := ScreenJob(JobEligibilityInput{
		Title: "Dental Hygienist", Location: "Remote", RemoteClaimed: true,
	}, profile)
	if result.Code != ReasonRoleTrackMismatch {
		t.Fatalf("off-role, unlocatable job = %s, want %s", result.Code, ReasonRoleTrackMismatch)
	}
}

// IsEligibleJob is retained for callers that only need a bool; it must never
// disagree with the structured verdict it now wraps.
func TestGeography_IsEligibleJobAgreesWithScreenJob(t *testing.T) {
	profile := &Profile{RemoteOnly: true, Roles: []string{"Platform Engineer"}, AllowedCountries: usCA}
	for _, location := range []string{
		"Austin, Texas, United States", "Amman, Amman Governorate, Jordan", "Remote", "Toronto, Ontario, Canada",
	} {
		job := JobEligibilityInput{Title: "Platform Engineer", Location: location, RemoteClaimed: true}
		ok, _ := IsEligibleJob(job, profile)
		if ok != ScreenJob(job, profile).Eligible {
			t.Errorf("IsEligibleJob and ScreenJob disagree for %q", location)
		}
	}
	// A nil profile stays permissive, exactly as before.
	if ok, _ := IsEligibleJob(JobEligibilityInput{Location: "Jordan"}, nil); !ok {
		t.Error("a nil profile must remain permissive")
	}
}

func contains(haystack []string, needle string) bool {
	for _, v := range haystack {
		if v == needle {
			return true
		}
	}
	return false
}
