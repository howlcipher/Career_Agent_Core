package benchmarkjobfit

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/howlcipher/Career_Agent_Core/pkg/parser"
	"github.com/howlcipher/Career_Agent_Core/pkg/security"
)

const maxPostingBytes = 4 << 20

// FetchStats records only aggregate outcomes; it never includes private URLs
// or employer names.
type FetchStats struct {
	Attempted       int            `json:"attempted"`
	Accepted        int            `json:"accepted"`
	SkippedByReason map[string]int `json:"skipped_by_reason"`
}

// FetchCohort retrieves candidates sequentially through the repository's SSRF
// and DNS-rebinding guard. It stops once target accepted postings are present.
func FetchCohort(
	ctx context.Context,
	candidates []SourceJob,
	target int,
	timeout time.Duration,
) ([]FetchedJob, FetchStats) {
	stats := FetchStats{SkippedByReason: make(map[string]int)}
	if target <= 0 {
		return nil, stats
	}
	client := security.NewNetworkGuard().HTTPClient(timeout)
	filter := security.NewQuarantineLayer()
	cohort := make([]FetchedJob, 0, target)
	for _, candidate := range candidates {
		if len(cohort) >= target || ctx.Err() != nil {
			break
		}
		stats.Attempted++
		if shouldSkipHost(candidate.URL) {
			stats.SkippedByReason["excluded_host"]++
			continue
		}
		description, reason := fetchDescription(ctx, client, filter, candidate)
		if reason != "" {
			stats.SkippedByReason[reason]++
			continue
		}
		cohort = append(cohort, FetchedJob{Source: candidate, Description: description})
		stats.Accepted++
	}
	return cohort, stats
}

func shouldSkipHost(rawURL string) bool {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return true
	}
	host := strings.ToLower(parsed.Hostname())
	// Workable is intentionally not probed while its documented automated
	// access block is cooling down; this benchmark must not prolong it.
	return strings.Contains(host, "workable.com")
}

func fetchDescription(
	ctx context.Context,
	client *http.Client,
	filter *security.QuarantineLayer,
	candidate SourceJob,
) (string, string) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, candidate.URL, nil)
	if err != nil {
		return "", "invalid_request"
	}
	request.Header.Set("User-Agent", "Career-Agent-Core-Research-Benchmark/1.0")
	request.Header.Set("Accept", "text/html,application/xhtml+xml")
	response, err := client.Do(request)
	if err != nil {
		return "", "fetch_error"
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", fmt.Sprintf("http_%d", response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxPostingBytes+1))
	if err != nil {
		return "", "read_error"
	}
	if len(body) > maxPostingBytes {
		return "", "body_too_large"
	}
	description, err := parser.PruneDOMToText(bytes.NewReader(body))
	if err != nil {
		return "", "parse_error"
	}
	description = SanitizeDescription(description, candidate.CompanyName, 6000)
	if len([]rune(description)) < 160 {
		return "", "insufficient_text"
	}
	if err := filter.QuarantinePayload(candidate.Title + "\n" + description); err != nil {
		return "", "prompt_injection_quarantine"
	}
	return description, ""
}
