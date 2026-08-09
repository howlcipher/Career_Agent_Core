package benchmarkjobfit

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/howlcipher/Career_Agent_Core/pkg/security"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (function roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}

func responseClient(body string) *http.Client {
	return &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(body)),
		}, nil
	})}
}

func TestFetchDescriptionBoundsBodyBeforeParsing(t *testing.T) {
	t.Parallel()

	_, reason := fetchDescription(
		context.Background(),
		responseClient(strings.Repeat("x", maxPostingBytes+1)),
		security.NewQuarantineLayer(),
		SourceJob{Title: "Engineer", URL: "https://example.com/job"},
	)
	if reason != "body_too_large" {
		t.Fatalf("reason = %q, want body_too_large", reason)
	}
}

func TestFetchDescriptionSanitizesBeforeAcceptance(t *testing.T) {
	t.Parallel()

	body := `<html><body><main>
		Example Employer seeks a Platform Engineer to build reliable distributed services.
		The role includes Go development, incident response, testing, documentation,
		and collaboration across a remote platform team. Contact jobs@example.com or
		visit https://example.com/private for more details about this opportunity.
	</main></body></html>`
	description, reason := fetchDescription(
		context.Background(),
		responseClient(body),
		security.NewQuarantineLayer(),
		SourceJob{
			CompanyName: "Example Employer",
			Title:       "Platform Engineer",
			URL:         "https://example.com/job",
		},
	)
	if reason != "" {
		t.Fatalf("reason = %q", reason)
	}
	for _, forbidden := range []string{"Example Employer", "jobs@example.com", "example.com/private"} {
		if strings.Contains(description, forbidden) {
			t.Fatalf("description contains %q: %q", forbidden, description)
		}
	}
}

func TestFetchDescriptionRejectsPostingForDifferentTitle(t *testing.T) {
	t.Parallel()

	body := `<html><body><main>
		<h1>Python Tutor</h1>
		<p>Teach online programming lessons to children using a prepared curriculum.</p>
		<p>This job description includes mentoring, progress reports, student feedback,
		class preparation, group sessions, individual sessions, and regular collaboration
		with teaching mentors throughout the remote program.</p>
	</main></body></html>`
	_, reason := fetchDescription(
		context.Background(),
		responseClient(body),
		security.NewQuarantineLayer(),
		SourceJob{Title: "Python Developer", URL: "https://example.com/job"},
	)
	if reason != "title_mismatch" {
		t.Fatalf("reason = %q, want title_mismatch", reason)
	}
}

func TestFetchDescriptionRejectsATSListingPage(t *testing.T) {
	t.Parallel()

	body := `<html><body><main>
		<h1>Current openings at Example Employer</h1>
		<p>Create a Job Alert</p>
		<ul><li>Platform Engineer</li><li>Accountant</li><li>Sales Director</li></ul>
		<p>We use AI to write clearer job descriptions for every role.</p>
		<p>Search Department Select Search Office Select and browse our open jobs.</p>
	</main></body></html>`
	_, reason := fetchDescription(
		context.Background(),
		responseClient(body),
		security.NewQuarantineLayer(),
		SourceJob{Title: "Platform Engineer", URL: "https://example.com/job"},
	)
	if reason != "listing_page" {
		t.Fatalf("reason = %q, want listing_page", reason)
	}
}

func TestPostingQualityNormalizesTitlePunctuation(t *testing.T) {
	t.Parallel()

	description := "Senior Go Developer - Platform. About the role: build reliable services."
	if reason := postingQualityReason("Senior Go Developer (Platform)", description); reason != "" {
		t.Fatalf("postingQualityReason() = %q, want acceptance", reason)
	}
}

func TestShouldSkipHostAvoidsWorkableBlock(t *testing.T) {
	t.Parallel()

	if !shouldSkipHost("https://apply.workable.com/example/j/123") {
		t.Fatal("Workable host was not excluded")
	}
	if shouldSkipHost("https://jobs.example.com/example") {
		t.Fatal("ordinary public host was excluded")
	}
}
