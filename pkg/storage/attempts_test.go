package storage

import (
	"testing"
	"time"
)

func TestApplicationAttempts(t *testing.T) {
	// Initialize test database
	setupTestDB(t)
	defer teardownTestDB()

	// Clear table in case of shared test db state
	db.Exec("DELETE FROM application_attempts")

	now := time.Now()
	
	// Create some attempts
	attempts := []ApplicationAttempt{
		{
			Source:         "greenhouse.io",
			URL:            "https://greenhouse.io/company1/jobs/1",
			TerminalClass:  AttemptApplied,
			StartedAt:      now.Add(-2 * time.Hour),
			EndedAt:        now.Add(-1 * time.Hour),
			ModelCallCount: 5,
			InferenceMs:    15000,
		},
		{
			Source:         "greenhouse.io",
			URL:            "https://greenhouse.io/company1/jobs/2",
			TerminalClass:  AttemptPostSubmitCaptcha,
			StartedAt:      now.Add(-2 * time.Hour),
			EndedAt:        now.Add(-1 * time.Hour),
			ModelCallCount: 4,
			InferenceMs:    12000,
		},
		{
			Source:         "lever.co",
			URL:            "https://jobs.lever.co/company2/1",
			TerminalClass:  AttemptManualAccountGate,
			StartedAt:      now.Add(-48 * time.Hour),
			EndedAt:        now.Add(-47 * time.Hour),
			ModelCallCount: 1,
			InferenceMs:    2000,
		},
		{
			Source:         "lever.co",
			URL:            "https://jobs.lever.co/company2/2",
			TerminalClass:  AttemptDeadPosting,
			StartedAt:      now.Add(-5 * time.Hour),
			EndedAt:        now.Add(-4 * time.Hour),
			ModelCallCount: 0,
			InferenceMs:    0,
		},
	}

	for _, a := range attempts {
		if err := RecordAttempt(a); err != nil {
			t.Fatalf("failed to record attempt: %v", err)
		}
	}

	// Test querying 7-day summary
	summaries, err := GetSourceHealthSummaries(7)
	if err != nil {
		t.Fatalf("failed to get summaries: %v", err)
	}

	if len(summaries) != 2 {
		t.Fatalf("expected 2 source summaries, got %d", len(summaries))
	}

	// Summaries are ordered by source ascending
	// greenhouse.io
	if summaries[0].Source != "greenhouse.io" {
		t.Errorf("expected greenhouse.io, got %s", summaries[0].Source)
	}
	if summaries[0].TotalAttempts != 2 {
		t.Errorf("expected 2 attempts for greenhouse, got %d", summaries[0].TotalAttempts)
	}
	if summaries[0].AppliedCount != 1 {
		t.Errorf("expected 1 applied for greenhouse, got %d", summaries[0].AppliedCount)
	}
	if summaries[0].CaptchaCount != 1 {
		t.Errorf("expected 1 captcha for greenhouse, got %d", summaries[0].CaptchaCount)
	}
	if summaries[0].AvgInferenceMs != 13500 {
		t.Errorf("expected 13500 avg inference ms for greenhouse, got %d", summaries[0].AvgInferenceMs)
	}
	if summaries[0].AvgModelCalls != 4.5 {
		t.Errorf("expected 4.5 avg model calls for greenhouse, got %f", summaries[0].AvgModelCalls)
	}
	if summaries[0].Confidence != "Sparse" {
		t.Errorf("expected Sparse confidence for greenhouse, got %s", summaries[0].Confidence)
	}

	// lever.co
	if summaries[1].Source != "lever.co" {
		t.Errorf("expected lever.co, got %s", summaries[1].Source)
	}
	if summaries[1].TotalAttempts != 2 {
		t.Errorf("expected 2 attempts for lever, got %d", summaries[1].TotalAttempts)
	}
	if summaries[1].ManualCount != 1 {
		t.Errorf("expected 1 manual for lever, got %d", summaries[1].ManualCount)
	}
	if summaries[1].DeadCount != 1 {
		t.Errorf("expected 1 dead posting for lever, got %d", summaries[1].DeadCount)
	}
	if summaries[1].AvgInferenceMs != 1000 {
		t.Errorf("expected 1000 avg inference ms for lever, got %d", summaries[1].AvgInferenceMs)
	}

	// Test 1-day summary (should exclude the 48-hour old lever attempt)
	summaries1Day, err := GetSourceHealthSummaries(1)
	if err != nil {
		t.Fatalf("failed to get 1-day summaries: %v", err)
	}

	if len(summaries1Day) != 2 {
		t.Fatalf("expected 2 source summaries, got %d", len(summaries1Day))
	}

	// lever.co in 1-day summary
	if summaries1Day[1].TotalAttempts != 1 {
		t.Errorf("expected 1 attempt for lever in 1-day summary, got %d", summaries1Day[1].TotalAttempts)
	}
	if summaries1Day[1].DeadCount != 1 {
		t.Errorf("expected 1 dead posting for lever in 1-day summary, got %d", summaries1Day[1].DeadCount)
	}
	if summaries1Day[1].ManualCount != 0 {
		t.Errorf("expected 0 manual for lever in 1-day summary, got %d", summaries1Day[1].ManualCount)
	}
}
