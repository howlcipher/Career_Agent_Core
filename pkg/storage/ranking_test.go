package storage

import (
	"strings"
	"testing"
	"time"
)

// TestComputeSourceScores_AwaitingReviewIsNotASourceDefect pins the fix for the
// ranking inversion Copilot Mode would otherwise cause.
//
// AWAITING_REVIEW attempts are the operator saying "stop before the click". They
// apply uniformly to every board and reveal nothing about whether that board
// would have accepted a submission. If they are counted either as bad outcomes
// or in the success-rate denominator, then during a copilot run every source's
// score collapses while a source that has never been attempted keeps the 0.05
// prior — so the queue actively prefers boards it knows nothing about over
// boards it has just filled successfully 20 times. Because the health summary
// is a 30-day lookback, that distortion outlives the copilot run by a month.
func TestComputeSourceScores_AwaitingReviewIsNotASourceDefect(t *testing.T) {
	const attempts = 20

	summaries := []SourceHealthSummary{
		{
			// A board filled 20 times under copilot mode: no submit was ever
			// attempted, so there is no evidence for or against it.
			Source:              "copilot-filled",
			PeriodDays:          30,
			TotalAttempts:       attempts,
			AwaitingReviewCount: attempts,
		},
		{
			// A board that genuinely refused 20 times. This one should be
			// penalised.
			Source:        "really-blocked",
			PeriodDays:    30,
			TotalAttempts: attempts,
			CaptchaCount:  attempts,
		},
	}

	scores := ComputeSourceScores(summaries)

	copilot, ok := scores["copilot-filled"]
	if !ok {
		t.Fatal("expected a score for the copilot-filled source")
	}
	blocked, ok := scores["really-blocked"]
	if !ok {
		t.Fatal("expected a score for the really-blocked source")
	}

	// An all-copilot source must be indistinguishable from one with no history:
	// the prior, and no penalty.
	unattempted := ComputeSourceScores([]SourceHealthSummary{{
		Source:     "never-tried",
		PeriodDays: 30,
	}})["never-tried"]

	if copilot.SmoothedSuccessRate != unattempted.SmoothedSuccessRate {
		t.Errorf("copilot-only attempts changed the smoothed success rate: got %v, want the unattempted prior %v",
			copilot.SmoothedSuccessRate, unattempted.SmoothedSuccessRate)
	}
	// Not 1.0: with no submitting attempts left in the denominator this source
	// is correctly treated as sparse, which carries the same default penalty an
	// unattempted source gets. Equality with that baseline is the invariant —
	// copilot activity must leave a source exactly where it started.
	if copilot.PenaltyFactor != unattempted.PenaltyFactor {
		t.Errorf("copilot-only attempts changed the penalty factor: got %v, want the unattempted baseline %v",
			copilot.PenaltyFactor, unattempted.PenaltyFactor)
	}

	// And it must rank strictly better than a source that really is blocked,
	// which is the comparison the inversion got backwards.
	copilotScore := copilot.SmoothedSuccessRate * copilot.PenaltyFactor
	blockedScore := blocked.SmoothedSuccessRate * blocked.PenaltyFactor
	if copilotScore <= blockedScore {
		t.Errorf("a copilot-filled source scored %v, no better than a genuinely blocked source at %v", copilotScore, blockedScore)
	}
}

// TestComputeSourceScores_RealFailuresStillPenalised guards the other direction:
// excluding AWAITING_REVIEW must not weaken the penalties that do matter.
func TestComputeSourceScores_RealFailuresStillPenalised(t *testing.T) {
	scores := ComputeSourceScores([]SourceHealthSummary{
		{Source: "healthy", PeriodDays: 30, TotalAttempts: 10, AppliedCount: 5},
		{Source: "captcha", PeriodDays: 30, TotalAttempts: 10, CaptchaCount: 10},
		{Source: "gated", PeriodDays: 30, TotalAttempts: 10, ManualCount: 10},
		{Source: "dead", PeriodDays: 30, TotalAttempts: 10, DeadCount: 10},
	})

	healthy := scores["healthy"].SmoothedSuccessRate * scores["healthy"].PenaltyFactor
	for _, bad := range []string{"captcha", "gated", "dead"} {
		got := scores[bad].SmoothedSuccessRate * scores[bad].PenaltyFactor
		if got >= healthy {
			t.Errorf("source %q scored %v, not below the healthy source's %v", bad, got, healthy)
		}
		if scores[bad].PenaltyFactor >= 1.0 {
			t.Errorf("source %q was not penalised: PenaltyFactor = %v", bad, scores[bad].PenaltyFactor)
		}
	}
}

// TestRankJobs_UrgentAgeBypassesStarvation pins the fix for bugs.md #481: a
// job with a weak source/fit score used to only ever fall further behind
// fresh high-scoring jobs as it aged (freshnessMultiplier decays score, it
// never boosts it), so under a backlog that outpaces processing capacity, a
// low-scoring job could sit in DISCOVERED indefinitely until the real posting
// expired. A job at or past urgentAgeDays must now be surfaced ahead of every
// non-urgent job regardless of score.
func TestRankJobs_UrgentAgeBypassesStarvation(t *testing.T) {
	now := time.Now()

	stale := &FunnelJob{
		URL:           "https://weak-source.example.com/jobs/1",
		FitSimilarity: 0.1, // weak fit
		DiscoveredAt:  now.Add(-time.Duration(urgentAgeDays+5) * 24 * time.Hour),
	}

	var fresh []*FunnelJob
	for i := 0; i < 20; i++ {
		fresh = append(fresh, &FunnelJob{
			URL:           "https://strong-source.example.com/jobs/x",
			FitSimilarity: 0.95, // strong fit
			DiscoveredAt:  now,
		})
	}

	summaries := []SourceHealthSummary{
		{Source: "strong-source.example.com", PeriodDays: 30, TotalAttempts: 100, AppliedCount: 60},
		{Source: "weak-source.example.com", PeriodDays: 30, TotalAttempts: 100, CaptchaCount: 90},
	}

	jobs := append([]*FunnelJob{stale}, fresh...)
	ranked := RankJobs(jobs, summaries, 0.20)

	if len(ranked) != len(jobs) {
		t.Fatalf("RankJobs dropped jobs: got %d, want %d", len(ranked), len(jobs))
	}
	if ranked[0] != stale {
		t.Fatalf("expected the urgent stale job to lead the ranked queue despite its weak score, got URL %q first", ranked[0].URL)
	}
	if !strings.Contains(stale.RankingReason, "Urgent") {
		t.Errorf("expected the urgent job's ranking reason to explain the bypass, got %q", stale.RankingReason)
	}

	// Sanity check on the score alone, to prove this is a real starvation
	// scenario and not a tautology: absent the urgent override, the stale
	// job's raw score must indeed lose to every fresh job's.
	for _, f := range fresh {
		if stale.RankingScore >= f.RankingScore {
			t.Fatalf("test setup invalid: stale job's score %v was not below a fresh job's %v — urgency wasn't actually needed to win", stale.RankingScore, f.RankingScore)
		}
	}
}

// TestRankJobs_NonUrgentJobsStillRankedByScore guards the other direction:
// jobs under urgentAgeDays must keep the pre-existing score-based ordering,
// not be swept into the urgent bypass path.
func TestRankJobs_NonUrgentJobsStillRankedByScore(t *testing.T) {
	now := time.Now()

	strong := &FunnelJob{
		URL:           "https://strong-source.example.com/jobs/1",
		FitSimilarity: 0.95,
		DiscoveredAt:  now,
	}
	weak := &FunnelJob{
		URL:           "https://weak-source.example.com/jobs/2",
		FitSimilarity: 0.1,
		DiscoveredAt:  now,
	}

	summaries := []SourceHealthSummary{
		{Source: "strong-source.example.com", PeriodDays: 30, TotalAttempts: 100, AppliedCount: 60},
		{Source: "weak-source.example.com", PeriodDays: 30, TotalAttempts: 100, CaptchaCount: 90},
	}

	ranked := RankJobs([]*FunnelJob{weak, strong}, summaries, 0.20)
	if ranked[0] != strong {
		t.Fatalf("expected the stronger-scoring non-urgent job first, got URL %q first", ranked[0].URL)
	}
}
