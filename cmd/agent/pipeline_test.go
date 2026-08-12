package main

import (
	"context"
	"database/sql"
	"errors"
	"net"
	"net/netip"
	"path/filepath"
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
	return nil, &net.DNSError{Name: host, IsNotFound: true}
}

type temporaryFailingResolver struct{}

func (temporaryFailingResolver) LookupNetIP(ctx context.Context, network, host string) ([]netip.Addr, error) {
	return nil, &net.DNSError{Name: host, IsTemporary: true}
}

type publicResolver struct{}

func (publicResolver) LookupNetIP(context.Context, string, string) ([]netip.Addr, error) {
	return []netip.Addr{netip.MustParseAddr("8.8.8.8")}, nil
}

// The three tests below open a file database rather than `:memory:` for the
// reason documented on storage.setupTestDB (bug #527): `:memory:` is private to
// one connection, so any query issued from inside an open result set takes a
// second pooled connection, finds an empty schema, and fails `no such table` —
// silently, with no setup error to notice.
func TestStateInit_DuplicateCooldownSkipsBeforeNetworkWork(t *testing.T) {
	if err := storage.InitDBWithPath(filepath.Join(t.TempDir(), "test.db")); err != nil {
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

// The hard eligibility gate runs at the end of StateDiscovery, once the full
// description is available, and must reject a hybrid posting outright --
// before scoring ever runs, and regardless of how attractive its title is.
func TestStateDiscovery_RejectsHybridPostingBeforeScoring(t *testing.T) {
	if err := storage.InitDBWithPath(filepath.Join(t.TempDir(), "test.db")); err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}
	defer storage.CloseDB()

	const jobURL = "https://jobs.example.com/hybridco/1"
	if _, err := storage.AddToFunnel("Hybrid Co", "DevOps Engineer", jobURL, "DISCOVERED"); err != nil {
		t.Fatalf("AddToFunnel: %v", err)
	}

	g := buildJobPipeline(JobPipelineDeps{
		NetworkGuard: security.NewNetworkGuard(security.WithResolver(publicResolver{})),
		Profile: &config.Profile{
			RemoteOnly: true,
			Roles:      []string{"DevOps Engineer"},
		},
	})
	state := &JobState{Job: scraper.Job{
		CompanyName: "Hybrid Co",
		Title:       "DevOps Engineer",
		Location:    "Remote - US",
		Remote:      true,
		// A perfect title and an attractive "Remote" location must not
		// override a description that plainly requires hybrid attendance.
		Description: "This is a hybrid role requiring three days a week in office.",
		URL:         jobURL,
	}}
	// Start at StateDiscovery directly: the job's description is already
	// populated, so this exercises the hard eligibility gate without needing
	// a live network guard / job-alive check first.
	if err := g.Run(context.Background(), StateDiscovery, state); err != nil {
		t.Fatalf("pipeline run: %v", err)
	}

	var status, reason string
	if err := storage.GetDB().QueryRow("SELECT status, status_reason FROM job_funnel WHERE url = ?", jobURL).Scan(&status, &reason); err != nil {
		t.Fatalf("read rejected job: %v", err)
	}
	if status != "SKIPPED" {
		t.Errorf("status = %q, want SKIPPED", status)
	}
	if reason == "" || reason == "below_minimum_fit_score" {
		t.Errorf("status_reason = %q, want the job rejected before scoring ever ran", reason)
	}
}

// A job that fails the hard gate must never reach StateScoring, which is
// what "before scoring" actually means: no LLM call, no score, no chance for
// an attractive score to matter.
func TestStateDiscovery_RoleMismatchRejectedBeforeScoring(t *testing.T) {
	if err := storage.InitDBWithPath(filepath.Join(t.TempDir(), "test.db")); err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}
	defer storage.CloseDB()

	const jobURL = "https://jobs.example.com/dataco/1"
	if _, err := storage.AddToFunnel("Data Co", "Data Engineer", jobURL, "DISCOVERED"); err != nil {
		t.Fatalf("AddToFunnel: %v", err)
	}

	g := buildJobPipeline(JobPipelineDeps{
		NetworkGuard: security.NewNetworkGuard(security.WithResolver(publicResolver{})),
		Profile: &config.Profile{
			RemoteOnly: true,
			Roles:      []string{"Software Engineer", "DevOps Engineer"},
		},
	})
	state := &JobState{Job: scraper.Job{
		CompanyName: "Data Co",
		Title:       "Data Engineer",
		Location:    "Remote - US",
		Remote:      true,
		Description: "Fully remote data engineering role building ETL pipelines.",
		URL:         jobURL,
	}}
	if err := g.Run(context.Background(), StateDiscovery, state); err != nil {
		t.Fatalf("pipeline run: %v", err)
	}

	var status string
	if err := storage.GetDB().QueryRow("SELECT status FROM job_funnel WHERE url = ?", jobURL).Scan(&status); err != nil {
		t.Fatalf("read rejected job: %v", err)
	}
	if status != "SKIPPED" {
		t.Errorf("status = %q, want SKIPPED for a role removed from the target profile", status)
	}
}

func TestStateInit_DNSNotFoundTerminalizesWithoutRetry(t *testing.T) {
	if err := storage.InitDBWithPath(filepath.Join(t.TempDir(), "test.db")); err != nil {
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

	if status != "RETRY_EXHAUSTED" {
		t.Errorf("status = %q, want RETRY_EXHAUSTED for a permanent DNS name-not-found", status)
	}
	if retryCount != 0 {
		t.Errorf("retry_count = %d, want 0 because a permanent DNS failure must not consume retry budget", retryCount)
	}
	if nextEligible.Valid {
		t.Fatalf("next_eligible_at = %v, want NULL for a terminal permanent DNS failure", nextEligible)
	}

	jobs, err := storage.GetDiscoveredJobs()
	if err != nil {
		t.Fatalf("GetDiscoveredJobs failed: %v", err)
	}
	for _, job := range jobs {
		if job.URL == jobURL {
			t.Errorf("GetDiscoveredJobs returned %s after permanent DNS failure", jobURL)
		}
	}
}

func TestStateInit_TemporaryDNSFailureRemainsRetryable(t *testing.T) {
	if err := storage.InitDBWithPath(filepath.Join(t.TempDir(), "test.db")); err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}
	defer storage.CloseDB()

	const jobURL = "https://temporarily-unavailable.example/careers/job1"
	if _, err := storage.AddToFunnel("Example", "Engineer", jobURL, "DISCOVERED"); err != nil {
		t.Fatalf("AddToFunnel failed: %v", err)
	}

	g := buildJobPipeline(JobPipelineDeps{NetworkGuard: security.NewNetworkGuard(security.WithResolver(temporaryFailingResolver{}))})
	if err := g.Run(context.Background(), StateInit, &JobState{Job: scraper.Job{URL: jobURL, CompanyName: "Example"}}); err != nil {
		t.Fatalf("pipeline run returned an error: %v", err)
	}

	var status string
	var retryCount int
	var nextEligible sql.NullTime
	if err := storage.GetDB().QueryRow("SELECT status, retry_count, next_eligible_at FROM job_funnel WHERE url = ?", jobURL).Scan(&status, &retryCount, &nextEligible); err != nil {
		t.Fatalf("read back job_funnel row: %v", err)
	}
	if status != "DISCOVERED" || retryCount != 1 || !nextEligible.Valid {
		t.Errorf("temporary DNS failure = status %q, retry_count %d, next_eligible_at %v; want retryable DISCOVERED row", status, retryCount, nextEligible)
	}
}
