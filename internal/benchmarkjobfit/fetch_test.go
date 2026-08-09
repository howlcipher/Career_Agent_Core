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
		Example Employer seeks an engineer to build reliable distributed services.
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

func TestShouldSkipHostAvoidsWorkableBlock(t *testing.T) {
	t.Parallel()

	if !shouldSkipHost("https://apply.workable.com/example/j/123") {
		t.Fatal("Workable host was not excluded")
	}
	if shouldSkipHost("https://jobs.example.com/example") {
		t.Fatal("ordinary public host was excluded")
	}
}
