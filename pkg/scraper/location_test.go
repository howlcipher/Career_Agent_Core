package scraper

import "testing"

func TestLocationAllowed(t *testing.T) {
	northAmerica := []string{"US", "CA"}

	tests := []struct {
		name     string
		location string
		codes    []string
		allowed  []string
		want     bool
	}{
		// The exact posting that halted the acceptance trial: Lever returned
		// country "IN" for an "AI Automation Engineer" role scored 100.
		{"explicit India code is rejected", "India", []string{"IN"}, northAmerica, false},
		{"India by name alone is rejected", "India", nil, northAmerica, false},
		{"explicit US code is allowed", "New York, NY", []string{"US"}, northAmerica, true},
		{"explicit Canada code is allowed", "Toronto", []string{"CA"}, northAmerica, true},
		{"US by name alone is allowed", "Remote, United States", nil, northAmerica, true},
		{"USA abbreviation is allowed", "Remote - USA", nil, northAmerica, true},

		// Fail-open cases: no evidence must never reject.
		{"empty location is allowed", "", nil, northAmerica, true},
		{"unrecognised location is allowed", "Remote", nil, northAmerica, true},
		{"gibberish location is allowed", "Planet Earth", nil, northAmerica, true},

		// Gate disabled entirely.
		{"no allowlist allows India", "India", []string{"IN"}, nil, true},
		{"empty allowlist entries allow India", "India", []string{"IN"}, []string{"  "}, true},

		// A posting open in several countries qualifies if any one matches.
		{"multi-country posting with US qualifies", "", []string{"IN", "US"}, northAmerica, true},
		{"multi-country posting without US is rejected", "", []string{"IN", "GB"}, northAmerica, false},

		// Explicit codes win over conflicting free text.
		{"explicit code overrides text", "India", []string{"US"}, northAmerica, true},

		// Case and padding must not matter.
		{"lowercase code is allowed", "", []string{"us"}, northAmerica, true},
		{"padded code is allowed", "", []string{" ca "}, northAmerica, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, reason := LocationAllowed(tc.location, tc.codes, tc.allowed)
			if got != tc.want {
				t.Errorf("LocationAllowed(%q, %v, %v) = %v (%s), want %v",
					tc.location, tc.codes, tc.allowed, got, reason, tc.want)
			}
		})
	}
}

// TestLocationAllowedWordBoundaries guards the specific trap in name matching:
// a substring match would read "Indiana" as "India" and silently discard every
// posting in that state, which is exactly the false rejection this gate must
// never produce.
func TestLocationAllowedWordBoundaries(t *testing.T) {
	northAmerica := []string{"US", "CA"}

	for _, location := range []string{
		"Indianapolis, Indiana",
		"Indiana",
		"Bloomington, IN, United States",
	} {
		if allowed, reason := LocationAllowed(location, nil, northAmerica); !allowed {
			t.Errorf("LocationAllowed(%q) rejected a US location: %s", location, reason)
		}
	}
}

func TestCountryCodesFor(t *testing.T) {
	tests := []struct {
		name     string
		location string
		explicit []string
		want     []string
	}{
		{"explicit codes win", "India", []string{"US"}, []string{"US"}},
		{"explicit codes are normalised", "", []string{" us ", "ca"}, []string{"US", "CA"}},
		{"explicit duplicates collapse", "", []string{"US", "us"}, []string{"US"}},
		{"malformed explicit codes are dropped", "", []string{"USA", "x", ""}, nil},
		{"name inference when no codes", "India", nil, []string{"IN"}},
		{"unknown text yields no evidence", "Remote", nil, nil},
		{"empty yields no evidence", "", nil, nil},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := CountryCodesFor(tc.location, tc.explicit)
			if len(got) != len(tc.want) {
				t.Fatalf("CountryCodesFor(%q, %v) = %v, want %v", tc.location, tc.explicit, got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("CountryCodesFor(%q, %v) = %v, want %v", tc.location, tc.explicit, got, tc.want)
				}
			}
		})
	}
}
