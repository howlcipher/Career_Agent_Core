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

func TestRecordAttemptUpdatesCapabilityRegistryAndMappingHealth(t *testing.T) {
	setupTestDB(t)
	defer teardownTestDB()

	const domain = "jobs.example-ats.com"
	if err := SaveFormMapping(domain, `{"fields":{"submit_button":"button.submit"}}`); err != nil {
		t.Fatalf("SaveFormMapping: %v", err)
	}

	now := time.Now().UTC()
	if err := RecordAttempt(ApplicationAttempt{
		Source:        "example-ats",
		URL:           "https://" + domain + "/apply/123",
		TerminalClass: AttemptApplied,
		StartedAt:     now.Add(-time.Minute),
		EndedAt:       now,
	}); err != nil {
		t.Fatalf("RecordAttempt applied: %v", err)
	}

	var provider, strategy, health string
	var accountRequired int
	var successfulReach time.Time
	if err := db.QueryRow(`
		SELECT ats_provider, confirmation_strategy, mapping_health, account_required, last_successful_form_reach
		FROM career_sites WHERE domain = ?`, domain).Scan(&provider, &strategy, &health, &accountRequired, &successfulReach); err != nil {
		t.Fatalf("read career site: %v", err)
	}
	if provider != "example-ats" || strategy != "confirmed_submission" || health != "healthy" || accountRequired != 0 || successfulReach.IsZero() {
		t.Errorf("unexpected successful capability row: provider=%q strategy=%q health=%q account_required=%d reached=%v", provider, strategy, health, accountRequired, successfulReach)
	}

	var successes, failures int
	if err := db.QueryRow("SELECT success_count, failure_count FROM form_mappings WHERE domain = ?", domain).Scan(&successes, &failures); err != nil {
		t.Fatalf("read mapping health: %v", err)
	}
	if successes != 1 || failures != 0 {
		t.Errorf("mapping counters after success = %d/%d, want 1/0", successes, failures)
	}
	prefer, err := PreferCachedFormMapping(domain)
	if err != nil || !prefer {
		t.Errorf("PreferCachedFormMapping after success = %t, %v; want true, nil", prefer, err)
	}

	for range 2 {
		if err := RecordAttempt(ApplicationAttempt{
			Source:        "example-ats",
			URL:           "https://" + domain + "/apply/123",
			TerminalClass: AttemptValidationFailure,
			StartedAt:     now,
			EndedAt:       now.Add(time.Minute),
		}); err != nil {
			t.Fatalf("RecordAttempt failure: %v", err)
		}
	}
	if err := db.QueryRow("SELECT success_count, failure_count FROM form_mappings WHERE domain = ?", domain).Scan(&successes, &failures); err != nil {
		t.Fatalf("read degraded mapping health: %v", err)
	}
	if successes != 1 || failures != 2 {
		t.Errorf("mapping counters after failures = %d/%d, want 1/2", successes, failures)
	}
	if err := db.QueryRow("SELECT mapping_health FROM career_sites WHERE domain = ?", domain).Scan(&health); err != nil {
		t.Fatalf("read degraded site health: %v", err)
	}
	if health != "degraded" {
		t.Errorf("mapping health = %q, want degraded", health)
	}
	prefer, err = PreferCachedFormMapping(domain)
	if err != nil || prefer {
		t.Errorf("PreferCachedFormMapping after more failures = %t, %v; want false, nil", prefer, err)
	}
	if _, err := db.Exec("UPDATE form_mappings SET success_count = 2, failure_count = 0, last_validated_at = ? WHERE domain = ?", now.Add(-formMappingFreshnessWindow-time.Second), domain); err != nil {
		t.Fatalf("age mapping health: %v", err)
	}
	prefer, err = PreferCachedFormMapping(domain)
	if err != nil || prefer {
		t.Errorf("PreferCachedFormMapping with stale success = %t, %v; want false, nil", prefer, err)
	}
}

func TestRecordAttemptMarksAccountRequired(t *testing.T) {
	setupTestDB(t)
	defer teardownTestDB()

	if err := RecordAttempt(ApplicationAttempt{
		Source:        "workday",
		URL:           "https://jobs.workday.example/apply",
		TerminalClass: AttemptManualAccountGate,
		EndedAt:       time.Now().UTC(),
	}); err != nil {
		t.Fatalf("RecordAttempt: %v", err)
	}
	var accountRequired int
	if err := db.QueryRow("SELECT account_required FROM career_sites WHERE domain = ?", "jobs.workday.example").Scan(&accountRequired); err != nil {
		t.Fatalf("read account requirement: %v", err)
	}
	if accountRequired != 1 {
		t.Errorf("account_required = %d, want 1", accountRequired)
	}
}
