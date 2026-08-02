package storage

import (
	"database/sql"
	"fmt"
	"net/url"
	"strings"
	"time"
)

type TerminalClass string

const (
	AttemptApplied           TerminalClass = "APPLIED"
	AttemptPostSubmitCaptcha TerminalClass = "POST_SUBMIT_CAPTCHA"
	AttemptManualAccountGate TerminalClass = "MANUAL_ACCOUNT_GATE"
	// AttemptAwaitingReview is a deliberate stop before the final submit click
	// (copilot mode, or auto_submit_click disabled). It is an operator choice
	// applied uniformly to every board, not a property of any ATS, so it is
	// neither stored as nor counted with the manual-account-gate outcomes and
	// is excluded from the source-health penalty entirely.
	AttemptAwaitingReview    TerminalClass = "AWAITING_REVIEW"
	AttemptDeadPosting       TerminalClass = "DEAD_POSTING"
	AttemptValidationFailure TerminalClass = "VALIDATION_FAILURE"
	AttemptOtherFailure      TerminalClass = "OTHER_FAILURE"
)

type ApplicationAttempt struct {
	Source         string
	URL            string
	TerminalClass  TerminalClass
	StartedAt      time.Time
	EndedAt        time.Time
	ModelCallCount int
	InferenceMs    int
}

type SourceHealthSummary struct {
	Source        string
	PeriodDays    int
	TotalAttempts int
	AppliedCount  int
	CaptchaCount  int
	ManualCount   int
	// AwaitingReviewCount is tracked separately from ManualCount and is
	// deliberately excluded from the bad-outcome penalty in ComputeSourceScores.
	// A copilot stop says nothing about the source — the operator chose it, and
	// it applies uniformly to every board — so counting it as a source defect
	// would drive a successfully-filled board below the prior for a board never
	// attempted at all, and keep doing so for the whole 30-day lookback after
	// copilot mode is switched back off.
	AwaitingReviewCount int
	DeadCount           int
	ValidationCount     int
	OtherCount          int
	AvgInferenceMs      int
	AvgModelCalls       float64
	Confidence          string // "High", "Medium", "Sparse"
}

// RecordAttempt saves a single application attempt to the database
func RecordAttempt(attempt ApplicationAttempt) error {
	if db == nil {
		return fmt.Errorf("database not initialized")
	}

	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin application attempt transaction: %w", err)
	}
	defer tx.Rollback()

	query := `
		INSERT INTO application_attempts 
		(source, url, terminal_class, started_at, ended_at, model_call_count, inference_ms)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`
	if _, err := tx.Exec(query,
		attempt.Source,
		attempt.URL,
		string(attempt.TerminalClass),
		attempt.StartedAt,
		attempt.EndedAt,
		attempt.ModelCallCount,
		attempt.InferenceMs); err != nil {
		return fmt.Errorf("failed to record application attempt: %w", err)
	}
	if err := recordSiteCapability(tx, attempt); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit application attempt transaction: %w", err)
	}
	return nil
}

func recordSiteCapability(tx *sql.Tx, attempt ApplicationAttempt) error {
	domain := attemptDomain(attempt.URL, attempt.Source)
	if domain == "" {
		return nil
	}

	observedAt := attempt.EndedAt.UTC()
	if observedAt.IsZero() {
		observedAt = time.Now().UTC()
	}
	if _, err := tx.Exec(`
		INSERT INTO career_sites (domain, ats_provider, last_scanned)
		VALUES (?, ?, ?)
		ON CONFLICT(domain) DO UPDATE SET
			ats_provider = excluded.ats_provider,
			last_scanned = excluded.last_scanned`, domain, attempt.Source, observedAt); err != nil {
		return fmt.Errorf("record career site capability: %w", err)
	}

	successfulFormReach := attempt.TerminalClass == AttemptApplied || attempt.TerminalClass == AttemptAwaitingReview
	if successfulFormReach {
		if _, err := tx.Exec(`
			UPDATE career_sites
			SET last_successful_form_reach = ?, confirmation_strategy = ?, mapping_health = 'healthy'
			WHERE domain = ?`, observedAt, confirmationStrategy(attempt.TerminalClass), domain); err != nil {
			return fmt.Errorf("record successful form reach: %w", err)
		}
	}
	if attempt.TerminalClass == AttemptManualAccountGate {
		if _, err := tx.Exec("UPDATE career_sites SET account_required = 1 WHERE domain = ?", domain); err != nil {
			return fmt.Errorf("record account requirement: %w", err)
		}
	}

	if _, err := tx.Exec(`
		UPDATE form_mappings
		SET success_count = success_count + ?,
			failure_count = failure_count + ?,
			last_validated_at = ?
		WHERE domain = ?`, boolInt(successfulFormReach), boolInt(!successfulFormReach), observedAt, domain); err != nil {
		return fmt.Errorf("record form mapping health: %w", err)
	}
	if _, err := tx.Exec(`
		UPDATE career_sites
		SET mapping_health = CASE
			WHEN EXISTS (SELECT 1 FROM form_mappings WHERE domain = career_sites.domain AND failure_count > success_count) THEN 'degraded'
			WHEN EXISTS (SELECT 1 FROM form_mappings WHERE domain = career_sites.domain) THEN 'healthy'
			ELSE mapping_health
		END
		WHERE domain = ?`, domain); err != nil {
		return fmt.Errorf("refresh mapping health: %w", err)
	}
	return nil
}

func attemptDomain(rawURL, fallback string) string {
	parsed, err := url.Parse(rawURL)
	if err == nil {
		if host := strings.TrimPrefix(strings.ToLower(parsed.Hostname()), "www."); host != "" {
			return host
		}
	}
	return strings.TrimPrefix(strings.ToLower(strings.TrimSpace(fallback)), "www.")
}

func confirmationStrategy(terminalClass TerminalClass) string {
	if terminalClass == AttemptApplied {
		return "confirmed_submission"
	}
	return "human_review"
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

// GetSourceHealthSummaries returns aggregated outcome data by source for the given lookback period
func GetSourceHealthSummaries(periodDays int) ([]SourceHealthSummary, error) {
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	cutoff := time.Now().AddDate(0, 0, -periodDays)

	query := `
		SELECT 
			source,
			COUNT(id) as total_attempts,
			SUM(CASE WHEN terminal_class = 'APPLIED' THEN 1 ELSE 0 END) as applied_count,
			SUM(CASE WHEN terminal_class = 'POST_SUBMIT_CAPTCHA' THEN 1 ELSE 0 END) as captcha_count,
			SUM(CASE WHEN terminal_class = 'MANUAL_ACCOUNT_GATE' THEN 1 ELSE 0 END) as manual_count,
			SUM(CASE WHEN terminal_class = 'AWAITING_REVIEW' THEN 1 ELSE 0 END) as awaiting_review_count,
			SUM(CASE WHEN terminal_class = 'DEAD_POSTING' THEN 1 ELSE 0 END) as dead_count,
			SUM(CASE WHEN terminal_class = 'VALIDATION_FAILURE' THEN 1 ELSE 0 END) as validation_count,
			SUM(CASE WHEN terminal_class = 'OTHER_FAILURE' THEN 1 ELSE 0 END) as other_count,
			COALESCE(AVG(inference_ms), 0) as avg_inference_ms,
			COALESCE(AVG(model_call_count), 0) as avg_model_calls
		FROM application_attempts
		WHERE started_at >= ?
		GROUP BY source
		ORDER BY source ASC
	`

	rows, err := db.Query(query, cutoff)
	if err != nil {
		return nil, fmt.Errorf("failed to query source health summaries: %w", err)
	}
	defer rows.Close()

	var summaries []SourceHealthSummary
	for rows.Next() {
		var s SourceHealthSummary
		s.PeriodDays = periodDays

		var avgInfMs, avgModCalls sql.NullFloat64
		err := rows.Scan(
			&s.Source,
			&s.TotalAttempts,
			&s.AppliedCount,
			&s.CaptchaCount,
			&s.ManualCount,
			&s.AwaitingReviewCount,
			&s.DeadCount,
			&s.ValidationCount,
			&s.OtherCount,
			&avgInfMs,
			&avgModCalls,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan source health summary: %w", err)
		}

		if avgInfMs.Valid {
			s.AvgInferenceMs = int(avgInfMs.Float64)
		}
		if avgModCalls.Valid {
			s.AvgModelCalls = avgModCalls.Float64
		}

		if s.TotalAttempts < 5 {
			s.Confidence = "Sparse"
		} else if s.TotalAttempts < 20 {
			s.Confidence = "Medium"
		} else {
			s.Confidence = "High"
		}

		summaries = append(summaries, s)
	}

	return summaries, nil
}
