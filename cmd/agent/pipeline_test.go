package main

import (
	"context"
	"database/sql"
	"errors"
	"net/netip"
	"testing"
	"time"

	"github.com/howlcipher/Career_Agent_Core/pkg/config"
	"github.com/howlcipher/Career_Agent_Core/pkg/scraper"
	"github.com/howlcipher/Career_Agent_Core/pkg/security"
	"github.com/howlcipher/Career_Agent_Core/pkg/storage"
	"github.com/howlcipher/Career_Agent_Core/pkg/submitter"
)

// classifyGenerationError draws the line bugs.md #444 is about: only a
// genuine hard quota ("Quota exceeded") may cancel the whole batch. A bare
// "429" -- Anthropic's ordinary per-minute rate limit, and also what
// Gemini's SDK returns for its per-minute limit, not just its daily one --
// must be retried like any other transient network condition instead.
func TestClassifyGenerationError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want genErrorClass
	}{
		{
			name: "gemini hard daily quota is fatal",
			err:  errors.New("failed to generate content from gemini: googleapi: Error 429: Quota exceeded for quota metric 'Generate Content API requests'"),
			want: genErrorFatalQuota,
		},
		{
			name: "bare 429 on claude is retryable, not fatal",
			err:  errors.New("claude request failed: 429 Too Many Requests"),
			want: genErrorRetryable,
		},
		{
			name: "bare 429 with no provider context is retryable, not fatal",
			err:  errors.New("unexpected status code 429"),
			want: genErrorRetryable,
		},
		{
			name: "connection refused is retryable",
			err:  errors.New("dial tcp: connect: connection refused"),
			want: genErrorRetryable,
		},
		{
			name: "no route to host is retryable",
			err:  errors.New("dial tcp: no route to host"),
			want: genErrorRetryable,
		},
		{
			name: "deadline exceeded is retryable",
			err:  errors.New("context deadline exceeded"),
			want: genErrorRetryable,
		},
		{
			name: "unrelated error is terminal",
			err:  errors.New("failed to parse json response: unexpected end of JSON input"),
			want: genErrorTerminal,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := classifyGenerationError(tc.err)
			if got != tc.want {
				t.Errorf("classifyGenerationError(%q) = %v, want %v", tc.err.Error(), got, tc.want)
			}
		})
	}
}

func TestClassifyAttemptOutcomeKeepsDistinctLedgerReasons(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantClass  storage.TerminalClass
		wantReason string
	}{
		{"prompt injection", security.ErrPromptInjectionDetected, storage.AttemptOtherFailure, "prompt_injection_quarantine"},
		{"browser recovery exhausted", errors.New("playwright: target closed"), storage.AttemptOtherFailure, "browser_crash_recovery_exhausted"},
		{"generic fill failure", errors.New("selector matched no element"), storage.AttemptOtherFailure, "generic_fill_failure"},
		{"captcha", submitter.ErrCaptchaBlocked, storage.AttemptPostSubmitCaptcha, "post_submit_captcha"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotClass, gotReason := classifyAttemptOutcome(tc.err)
			if gotClass != tc.wantClass || gotReason != tc.wantReason {
				t.Errorf("classifyAttemptOutcome(%v) = (%q, %q), want (%q, %q)", tc.err, gotClass, gotReason, tc.wantClass, tc.wantReason)
			}
		})
	}
}

// failingResolver mimics a genuine DNS lookup failure -- "no such host", not
// a rejected private/loopback target -- so NetworkGuard.ValidateURL wraps it
// as a plain resolver error rather than security.ErrUnsafeNetworkTarget.
type failingResolver struct{}

func (failingResolver) LookupNetIP(ctx context.Context, network, host string) ([]netip.Addr, error) {
	return nil, errors.New("lookup " + host + ": no such host")
}

type publicResolver struct{}

func (publicResolver) LookupNetIP(context.Context, string, string) ([]netip.Addr, error) {
	return []netip.Addr{netip.MustParseAddr("8.8.8.8")}, nil
}

func TestStateInit_DuplicateCooldownSkipsBeforeNetworkWork(t *testing.T) {
	if err := storage.InitDBWithPath(":memory:"); err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}
	defer storage.CloseDB()

	now := time.Now().UTC()
	if _, err := storage.GetDB().Exec(`INSERT INTO job_funnel
		(company_name, job_title, job_location, is_remote, url, status, applied_at)
		VALUES (?, ?, ?, ?, ?, 'APPLIED', ?)`,
		"Acme Inc.", "Senior Software Engineer", "Detroit MI", true,
		"https://jobs.example.com/acme/previous", now); err != nil {
		t.Fatalf("seed confirmed application: %v", err)
	}
	const jobURL = "https://jobs.example.com/acme/new"
	if _, err := storage.AddToFunnel("Acme", "Senior Software Engineer", jobURL, "DISCOVERED"); err != nil {
		t.Fatalf("AddToFunnel: %v", err)
	}

	g := buildJobPipeline(JobPipelineDeps{
		NetworkGuard: security.NewNetworkGuard(security.WithResolver(publicResolver{})),
		Profile:      &config.Profile{DuplicateCooldownDays: 30},
	})
	state := &JobState{Job: scraper.Job{
		CompanyName: "Acme LLC",
		Title:       "Senior Software Engineer",
		Location:    "Detroit, MI",
		Remote:      true,
		URL:         jobURL,
	}}
	if err := g.Run(context.Background(), StateInit, state); err != nil {
		t.Fatalf("pipeline run: %v", err)
	}

	var status, reason string
	if err := storage.GetDB().QueryRow("SELECT status, status_reason FROM job_funnel WHERE url = ?", jobURL).Scan(&status, &reason); err != nil {
		t.Fatalf("read skipped job: %v", err)
	}
	if status != "SKIPPED" || reason != storage.SkippedReasonDuplicateCooldown {
		t.Errorf("duplicate row = (%q, %q), want (SKIPPED, %q)", status, reason, storage.SkippedReasonDuplicateCooldown)
	}
}

// TestStateInit_DNSFailureLeavesJobRetryable covers bugs.md #478: a DNS
// resolution failure in StateInit's else branch must route through
// storage.UpdateFunnelStatusRetryable like every other retryable failure in
// this file, instead of only logging and leaving the row at DISCOVERED
// forever -- which is what let one bad hostname spin the live daemon at
// ~1 cycle/sec instead of the documented ~1/minute cadence.
func TestStateInit_DNSFailureLeavesJobRetryable(t *testing.T) {
	if err := storage.InitDBWithPath(":memory:"); err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}
	defer func() {
		storage.GetDB().Close()
	}()

	const jobURL = "https://wwww.raileurope.com/careers/job1"
	if _, err := storage.AddToFunnel("Rail Europe", "Engineer", jobURL, "DISCOVERED"); err != nil {
		t.Fatalf("AddToFunnel failed: %v", err)
	}

	guard := security.NewNetworkGuard(security.WithResolver(failingResolver{}))
	deps := JobPipelineDeps{NetworkGuard: guard}
	g := buildJobPipeline(deps)
	state := &JobState{
		Job:      scraper.Job{URL: jobURL, CompanyName: "Rail Europe"},
		WorkerID: 1,
	}

	before := time.Now().UTC()
	if err := g.Run(context.Background(), StateInit, state); err != nil {
		t.Fatalf("pipeline run returned an error: %v", err)
	}

	var status string
	var retryCount int
	var nextEligible sql.NullTime
	if err := storage.GetDB().QueryRow(
		"SELECT status, retry_count, next_eligible_at FROM job_funnel WHERE url = ?", jobURL,
	).Scan(&status, &retryCount, &nextEligible); err != nil {
		t.Fatalf("failed to read back job_funnel row: %v", err)
	}

	if status != "DISCOVERED" {
		t.Errorf("status = %q, want still DISCOVERED (one failure, under MaxRetryAttempts)", status)
	}
	if retryCount != 1 {
		t.Errorf("retry_count = %d, want 1 -- a DNS failure must count as a retryable attempt", retryCount)
	}
	if !nextEligible.Valid || !nextEligible.Time.After(before) {
		t.Fatalf("next_eligible_at = %v, want a time after %v -- without this the row is immediately reselectable and the daemon spins on it every cycle", nextEligible, before)
	}

	jobs, err := storage.GetDiscoveredJobs()
	if err != nil {
		t.Fatalf("GetDiscoveredJobs failed: %v", err)
	}
	for _, job := range jobs {
		if job.URL == jobURL {
			t.Errorf("GetDiscoveredJobs still returned %s right after its DNS failure -- it must not reappear in the very next queue cycle", jobURL)
		}
	}
}
