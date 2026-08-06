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

// jobgether is an aggregator whose Lever apply form is broken (see
// storage.excludedLeverBoards). Exclusion is by board slug, not host, because
// every one of its postings lives under jobs.lever.co like any other employer.
func TestIsExcludedSourceURLScraper_LeverBoards(t *testing.T) {
	excluded := []string{
		"https://jobs.lever.co/jobgether/5493c241-c7f3-44f7-915c-6bb09e0c9215/apply",
		"https://jobs.lever.co/jobgether/abc",
		"https://api.lever.co/v0/postings/jobgether?mode=json",
		"https://JOBS.LEVER.CO/JobGether/xyz",
		"https://breezy.hr/p/123",
		"https://acme.breezy.hr/p/123",
	}
	for _, raw := range excluded {
		if !isExcludedSourceURLScraper(raw) {
			t.Errorf("expected %q to be excluded", raw)
		}
	}

	allowed := []string{
		"https://jobs.lever.co/smarsh/abc-123",
		"https://jobs.lever.co/egen/def",
		// Must not match on a prefix: a different board whose slug merely
		// begins with the excluded one is a different employer.
		"https://jobs.lever.co/jobgetherx/abc",
		"https://job-boards.greenhouse.io/grafanalabs/jobs/1",
	}
	for _, raw := range allowed {
		if isExcludedSourceURLScraper(raw) {
			t.Errorf("expected %q to be allowed", raw)
		}
	}
}

func TestIsExcludedBoardSlug(t *testing.T) {
	for _, slug := range []string{"jobgether", "JobGether", "  jobgether  "} {
		if !IsExcludedBoardSlug(slug) {
			t.Errorf("expected %q to be an excluded board slug", slug)
		}
	}
	for _, slug := range []string{"grafanalabs", "smarsh", "jobgetherx", ""} {
		if IsExcludedBoardSlug(slug) {
			t.Errorf("expected %q to be pollable", slug)
		}
	}
}
