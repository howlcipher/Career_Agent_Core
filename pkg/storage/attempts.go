package storage

import (
	"database/sql"
	"fmt"
	"time"
)

type TerminalClass string

const (
	AttemptApplied           TerminalClass = "APPLIED"
	AttemptPostSubmitCaptcha TerminalClass = "POST_SUBMIT_CAPTCHA"
	AttemptManualAccountGate TerminalClass = "MANUAL_ACCOUNT_GATE"
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
	Source          string
	PeriodDays      int
	TotalAttempts   int
	AppliedCount    int
	CaptchaCount    int
	ManualCount     int
	DeadCount       int
	ValidationCount int
	OtherCount      int
	AvgInferenceMs  int
	AvgModelCalls   float64
	Confidence      string // "High", "Medium", "Sparse"
}

// RecordAttempt saves a single application attempt to the database
func RecordAttempt(attempt ApplicationAttempt) error {
	if db == nil {
		return fmt.Errorf("database not initialized")
	}

	query := `
		INSERT INTO application_attempts 
		(source, url, terminal_class, started_at, ended_at, model_call_count, inference_ms)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`
	_, err := db.Exec(query,
		attempt.Source,
		attempt.URL,
		string(attempt.TerminalClass),
		attempt.StartedAt,
		attempt.EndedAt,
		attempt.ModelCallCount,
		attempt.InferenceMs)

	if err != nil {
		return fmt.Errorf("failed to record application attempt: %w", err)
	}
	return nil
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
