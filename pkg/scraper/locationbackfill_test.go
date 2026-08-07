package scraper

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestParseBoardRef(t *testing.T) {
	tests := []struct {
		url    string
		want   BoardRef
		wantOK bool
	}{
		// Greenhouse
		{
			url:    "https://job-boards.greenhouse.io/pointwild/jobs/5240015008",
			want:   BoardRef{Board: "greenhouse", Account: "pointwild", JobID: "5240015008"},
			wantOK: true,
		},
		{
			url:    "https://boards.greenhouse.io/acme/jobs/4123456",
			want:   BoardRef{Board: "greenhouse", Account: "acme", JobID: "4123456"},
			wantOK: true,
		},
		{
			url:    "https://job-boards.eu.greenhouse.io/sportygroup/jobs/4725219101?gh_jid=4725219101",
			want:   BoardRef{Board: "greenhouse", Account: "sportygroup", JobID: "4725219101"},
			wantOK: true,
		},
		{
			url:    "https://boards.eu.greenhouse.io/foo/jobs/123/",
			want:   BoardRef{Board: "greenhouse", Account: "foo", JobID: "123"},
			wantOK: true,
		},

		// Lever
		{
			url:    "https://jobs.lever.co/aircall/d9fc4c01-a12b-402e-94ee-9c3984a1fe79",
			want:   BoardRef{Board: "lever", Account: "aircall", JobID: "d9fc4c01-a12b-402e-94ee-9c3984a1fe79"},
			wantOK: true,
		},
		{
			url:    "https://jobs.eu.lever.co/foo/1A2B3C4D-5E6F-7A8B-9C0D-1E2F3A4B5C6D/apply",
			want:   BoardRef{Board: "lever", Account: "foo", JobID: "1a2b3c4d-5e6f-7a8b-9c0d-1e2f3a4b5c6d"},
			wantOK: true,
		},

		// Workable
		{
			url:    "https://apply.workable.com/joinbeam/j/A514643AB0",
			want:   BoardRef{Board: "workable", Account: "joinbeam", JobID: "A514643AB0"},
			wantOK: true,
		},
		{
			url:    "https://apply.workable.com/tamnoon-dot-i-o/j/5377DBAC0B",
			want:   BoardRef{Board: "workable", Account: "tamnoon-dot-i-o", JobID: "5377DBAC0B"},
			wantOK: true,
		},
		{
			url:    "https://apply.workable.com/maxana/j/9efca28551/apply/",
			want:   BoardRef{Board: "workable", Account: "maxana", JobID: "9EFCA28551"},
			wantOK: true,
		},

		// Unsupported / Malformed / Non-job URLs
		{
			url:    "https://foo.myworkdayjobs.com/en-US/careers/job/123",
			wantOK: false,
		},
		{
			url:    "https://jobs.ashbyhq.com/acme/123",
			wantOK: false,
		},
		{
			url:    "https://jobs.jobvite.com/relatient/jobs",
			wantOK: false,
		},
		{
			url:    "https://boards.greenhouse.io/pointwild",
			wantOK: false,
		},
		{
			url:    "",
			wantOK: false,
		},
		{
			url:    "not-a-url",
			wantOK: false,
		},
		{
			url:    "ftp://job-boards.greenhouse.io/pointwild/jobs/5240015008",
			wantOK: false,
		},
	}

	for _, tt := range tests {
		got, ok := ParseBoardRef(tt.url)
		if ok != tt.wantOK {
			t.Errorf("ParseBoardRef(%q) ok = %v, wantOK = %v", tt.url, ok, tt.wantOK)
			continue
		}
		if ok && got != tt.want {
			t.Errorf("ParseBoardRef(%q) = %+v, want %+v", tt.url, got, tt.want)
		}
	}
}

func TestAccountFeedURL(t *testing.T) {
	refGH := BoardRef{Board: "greenhouse", Account: "acme"}
	if got := AccountFeedURL(refGH); got != "https://boards-api.greenhouse.io/v1/boards/acme/jobs" {
		t.Errorf("AccountFeedURL(GH) = %q", got)
	}

	refLever := BoardRef{Board: "lever", Account: "acme"}
	if got := AccountFeedURL(refLever); got != "https://api.lever.co/v0/postings/acme?mode=json" {
		t.Errorf("AccountFeedURL(Lever) = %q", got)
	}

	refWorkable := BoardRef{Board: "workable", Account: "acme"}
	if got := AccountFeedURL(refWorkable); got != "https://apply.workable.com/api/v1/widget/accounts/acme?details=true" {
		t.Errorf("AccountFeedURL(Workable) = %q", got)
	}

	refUnknown := BoardRef{Board: "unknown", Account: "acme"}
	if got := AccountFeedURL(refUnknown); got != "" {
		t.Errorf("AccountFeedURL(Unknown) = %q, want empty", got)
	}
}

func TestParseAccountFeed_Greenhouse(t *testing.T) {
	body := []byte(`{
		"jobs": [
			{
				"absolute_url": "https://job-boards.greenhouse.io/pointwild/jobs/5219351008",
				"id": 5219351008,
				"title": "Backend Architect - Golang",
				"location": {"name": "Remote Estonia"},
				"requisition_id": "R1241"
			}
		]
	}`)

	feed, err := ParseAccountFeed("greenhouse", body)
	if err != nil {
		t.Fatalf("ParseAccountFeed(greenhouse) error: %v", err)
	}

	job, ok := feed["5219351008"]
	if !ok {
		t.Fatalf("expected job 5219351008 in parsed feed map")
	}

	if job.Title != "Backend Architect - Golang" {
		t.Errorf("Title = %q, want %q", job.Title, "Backend Architect - Golang")
	}
	if job.Location != "Remote Estonia" {
		t.Errorf("Location = %q, want %q", job.Location, "Remote Estonia")
	}
	if len(job.CountryCodes) != 0 {
		t.Errorf("CountryCodes = %v, want empty", job.CountryCodes)
	}
	if !job.Remote {
		t.Errorf("Remote = false, want true")
	}
}

func TestParseAccountFeed_Lever(t *testing.T) {
	body := []byte(`[
		{
			"hostedUrl": "https://jobs.lever.co/aircall/d9fc4c01-a12b-402e-94ee-9c3984a1fe79",
			"text": "Senior Backend Engineer",
			"country": "FR",
			"categories": {
				"location": "Paris",
				"allLocations": ["Paris", "Madrid"]
			},
			"workplaceType": "hybrid"
		}
	]`)

	feed, err := ParseAccountFeed("lever", body)
	if err != nil {
		t.Fatalf("ParseAccountFeed(lever) error: %v", err)
	}

	job, ok := feed["d9fc4c01-a12b-402e-94ee-9c3984a1fe79"]
	if !ok {
		t.Fatalf("expected job d9fc4c01-a12b-402e-94ee-9c3984a1fe79 in parsed feed map")
	}

	if job.Title != "Senior Backend Engineer" {
		t.Errorf("Title = %q", job.Title)
	}
	if job.Location != "Paris" {
		t.Errorf("Location = %q", job.Location)
	}
	if !reflect.DeepEqual(job.CountryCodes, []string{"FR"}) {
		t.Errorf("CountryCodes = %v, want [FR]", job.CountryCodes)
	}
	if job.Remote {
		t.Errorf("Remote = true, want false")
	}
}

func TestParseAccountFeed_Workable(t *testing.T) {
	body := []byte(`{
		"name": "Action1",
		"description": "...",
		"jobs": [
			{
				"title": "Account Executive - Multi Country",
				"shortcode": "9EFCA28551",
				"telecommuting": true,
				"country": "United States",
				"city": "",
				"state": "",
				"locations": [
					{"country": "United States", "countryCode": "US", "city": "San Francisco", "region": "CA", "hidden": false},
					{"country": "Brazil", "countryCode": "BR", "city": "São Paulo", "region": null, "hidden": false},
					{"country": "Argentina", "countryCode": "AR", "city": "Buenos Aires", "region": null, "hidden": true}
				]
			}
		]
	}`)

	feed, err := ParseAccountFeed("workable", body)
	if err != nil {
		t.Fatalf("ParseAccountFeed(workable) error: %v", err)
	}

	job, ok := feed["9EFCA28551"]
	if !ok {
		t.Fatalf("expected job 9EFCA28551 in parsed feed map")
	}

	wantLoc := "San Francisco, CA, United States / São Paulo, Brazil"
	if job.Location != wantLoc {
		t.Errorf("Location = %q, want %q", job.Location, wantLoc)
	}

	wantCodes := []string{"US", "BR"}
	if !reflect.DeepEqual(job.CountryCodes, wantCodes) {
		t.Errorf("CountryCodes = %v, want %v", job.CountryCodes, wantCodes)
	}

	if !job.Remote {
		t.Errorf("Remote = false, want true")
	}
}

func TestJobFeedURL(t *testing.T) {
	cases := []struct {
		ref  BoardRef
		want string
	}{
		{BoardRef{Board: "greenhouse", Account: "pointwild", JobID: "5240015008"},
			"https://boards-api.greenhouse.io/v1/boards/pointwild/jobs/5240015008"},
		{BoardRef{Board: "lever", Account: "aircall", JobID: "d9fc4c01-a12b-402e-94ee-9c3984a1fe79"},
			"https://api.lever.co/v0/postings/aircall/d9fc4c01-a12b-402e-94ee-9c3984a1fe79"},
		{BoardRef{Board: "workable", Account: "action1", JobID: "950A91C0A2"},
			"https://apply.workable.com/api/v1/accounts/action1/jobs/950A91C0A2"},
		{BoardRef{Board: "ashby", Account: "acme", JobID: "x"}, ""},
	}
	for _, tt := range cases {
		if got := JobFeedURL(tt.ref); got != tt.want {
			t.Errorf("JobFeedURL(%+v) = %q, want %q", tt.ref, got, tt.want)
		}
	}
}

// The per-job endpoints are what let the backfill tell "this posting expired"
// apart from "this board does not list it publicly", so their shapes — which
// differ from the account feeds' — need their own coverage.
func TestParseJobFeed(t *testing.T) {
	t.Run("greenhouse", func(t *testing.T) {
		body := []byte(`{"id":5219351008,"absolute_url":"https://job-boards.greenhouse.io/pointwild/jobs/5219351008",
			"title":"Backend Architect - Golang","location":{"name":"Remote Estonia"}}`)
		job, live, err := ParseJobFeed("greenhouse", body)
		if err != nil || !live {
			t.Fatalf("ParseJobFeed(greenhouse) = live %v, err %v", live, err)
		}
		if job.Location != "Remote Estonia" || !job.Remote {
			t.Errorf("job = %+v", job)
		}
	})

	t.Run("lever", func(t *testing.T) {
		body := []byte(`{"hostedUrl":"https://jobs.lever.co/aircall/d9fc4c01-a12b-402e-94ee-9c3984a1fe79",
			"text":"Senior Data Engineer","country":"FR",
			"categories":{"location":"France Remote","allLocations":["France Remote"]},"workplaceType":"remote"}`)
		job, live, err := ParseJobFeed("lever", body)
		if err != nil || !live {
			t.Fatalf("ParseJobFeed(lever) = live %v, err %v", live, err)
		}
		if job.Location != "France Remote" || !job.Remote {
			t.Errorf("job = %+v", job)
		}
		if !reflect.DeepEqual(job.CountryCodes, []string{"FR"}) {
			t.Errorf("CountryCodes = %v, want [FR]", job.CountryCodes)
		}
	})

	// The per-job shape uses "remote" and a nested "location" object where the
	// widget listing uses "telecommuting" and flat fields.
	t.Run("workable multi-country", func(t *testing.T) {
		body := []byte(`{"shortcode":"950A91C0A2","title":"Applied AI Engineer","remote":true,"state":"published",
			"location":{"country":"Cyprus","countryCode":"CY","city":"","region":null},
			"locations":[{"country":"Cyprus","countryCode":"CY","city":"","region":null,"hidden":false},
				{"country":"Serbia","countryCode":"RS","city":"","region":null,"hidden":false},
				{"country":"Montenegro","countryCode":"ME","city":"Budva","region":"Budva Municipality","hidden":false}]}`)
		job, live, err := ParseJobFeed("workable", body)
		if err != nil || !live {
			t.Fatalf("ParseJobFeed(workable) = live %v, err %v", live, err)
		}
		if want := "Cyprus / Serbia / Budva, Budva Municipality, Montenegro"; job.Location != want {
			t.Errorf("Location = %q, want %q", job.Location, want)
		}
		if want := []string{"CY", "RS", "ME"}; !reflect.DeepEqual(job.CountryCodes, want) {
			t.Errorf("CountryCodes = %v, want %v", job.CountryCodes, want)
		}
	})

	// Workable says a posting is gone in the body rather than with a 404.
	t.Run("workable unpublished is not live", func(t *testing.T) {
		body := []byte(`{"shortcode":"950A91C0A2","title":"Applied AI Engineer","remote":true,"state":"closed",
			"locations":[{"country":"Cyprus","countryCode":"CY","hidden":false}]}`)
		if _, live, err := ParseJobFeed("workable", body); err != nil || live {
			t.Errorf("ParseJobFeed(closed workable job) = live %v, err %v; want live false", live, err)
		}
	})

	t.Run("errors", func(t *testing.T) {
		if _, _, err := ParseJobFeed("ashby", []byte(`{}`)); err == nil {
			t.Errorf("expected an error for an unsupported board")
		}
		if _, _, err := ParseJobFeed("lever", []byte(`not json`)); err == nil {
			t.Errorf("expected an error for a malformed body")
		}
	})
}

// A Workable posting with no locations array must still yield its flat fields
// rather than an empty column.
func TestParseAccountFeed_WorkableFlatFallback(t *testing.T) {
	body := []byte(`{"jobs":[{"title":"Support Engineer","shortcode":"ABC123","telecommuting":false,
		"country":"Canada","city":"Toronto","state":"Ontario","locations":[]}]}`)
	feed, err := ParseAccountFeed("workable", body)
	if err != nil {
		t.Fatalf("ParseAccountFeed: %v", err)
	}
	job := feed["ABC123"]
	if want := "Toronto, Ontario, Canada"; job.Location != want {
		t.Errorf("Location = %q, want %q", job.Location, want)
	}
	// The flat fields are country *names*, never codes, so they must reach
	// CountryCodesFor as free text rather than being passed off as evidence.
	if len(job.CountryCodes) != 0 {
		t.Errorf("CountryCodes = %v, want empty", job.CountryCodes)
	}
	if codes := CountryCodesFor(job.Location, job.CountryCodes); !reflect.DeepEqual(codes, []string{"CA"}) {
		t.Errorf("CountryCodesFor(%q) = %v, want [CA]", job.Location, codes)
	}
}

func TestParseAccountFeed_Errors(t *testing.T) {
	if _, err := ParseAccountFeed("unknown_board", []byte(`{}`)); err == nil {
		t.Errorf("expected error for unknown board")
	}

	if _, err := ParseAccountFeed("greenhouse", []byte(`invalid json`)); err == nil {
		t.Errorf("expected error for malformed json body")
	}
}

func TestCountryCodesFor_BackfillAdditions(t *testing.T) {
	// "Remote Estonia" -> EE
	codesEstonia := CountryCodesFor("Remote Estonia", nil)
	if !reflect.DeepEqual(codesEstonia, []string{"EE"}) {
		t.Errorf("CountryCodesFor('Remote Estonia') = %v, want [EE]", codesEstonia)
	}

	// "Atlanta, Georgia" -> NO country code (georgia is omitted to prevent collision with US state)
	codesGeorgia := CountryCodesFor("Atlanta, Georgia", nil)
	if len(codesGeorgia) > 0 {
		t.Errorf("CountryCodesFor('Atlanta, Georgia') = %v, want empty slice", codesGeorgia)
	}
}

// A 429 that names a long Retry-After is a host-wide shutout, not a throttle,
// and the caller has to be able to tell the difference — retrying into it
// cannot succeed. Measured live: apply.workable.com answered every path with
// Retry-After 84643 after a backfill pass polled ~210 of its accounts.
func TestParseRetryAfter(t *testing.T) {
	cases := []struct {
		header string
		want   time.Duration
	}{
		{"84643", 84643 * time.Second},
		{" 120 ", 2 * time.Minute},
		{"", 0},
		{"0", 0},
		{"-5", 0},
		{"Wed, 21 Oct 2026 07:28:00 GMT", 0}, // HTTP-date form: treated as no guidance
		{"not-a-number", 0},
	}
	for _, tt := range cases {
		if got := parseRetryAfter(tt.header); got != tt.want {
			t.Errorf("parseRetryAfter(%q) = %v, want %v", tt.header, got, tt.want)
		}
	}
}

func TestFeedHTTPErrorMessage(t *testing.T) {
	err := &FeedHTTPError{StatusCode: 429, RetryAfter: 84643 * time.Second}
	// pollBoard and cmd/backfill-location both branch on this wording, so it
	// must not drift.
	if got := err.Error(); got != "board feed returned HTTP 429" {
		t.Errorf("Error() = %q", got)
	}
	if !strings.Contains(err.Error(), "HTTP 4") {
		t.Errorf("Error() no longer satisfies the HTTP 4xx check callers make")
	}
}
