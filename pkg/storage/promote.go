package storage

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/howlcipher/Career_Agent_Core/pkg/config"
)

var (
	ErrJobNotFound     = errors.New("job not found")
	ErrJobNotQualified = errors.New("job is not in a qualified state")
	ErrScoreTooLow     = errors.New("job score is below the current threshold")
	ErrAlreadyApplied  = errors.New("job has already been applied to")
	ErrStalePosting    = errors.New("posting is expired or stale")
	ErrAlreadyPromoted = errors.New("job is already promoted")
	ErrJobIneligible   = errors.New("job does not meet the current role/remote eligibility rules")
)

// PromoteJobToAssisted atomically promotes a qualified job to assisted apply mode.
func PromoteJobToAssisted(conn *sql.DB, jobID int64, minScore int) error {
	tx, err := conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var u, status, reason, title, location string
	var fitScore int
	var isRemote sql.NullInt64
	var discoveredAt time.Time
	var processingIntent sql.NullString

	err = tx.QueryRow(`SELECT url, status, COALESCE(status_reason, ''), fit_score, discovered_at, processing_intent,
		COALESCE(job_title, ''), COALESCE(job_location, ''), is_remote
		FROM job_funnel WHERE id = ?`, jobID).Scan(&u, &status, &reason, &fitScore, &discoveredAt, &processingIntent, &title, &location, &isRemote)
	if err != nil {
		if err == sql.ErrNoRows {
			return ErrJobNotFound
		}
		return err
	}

	if processingIntent.Valid && processingIntent.String == "assisted_promotion" {
		return ErrAlreadyPromoted // Idempotent repeat
	}

	if status != "PROCESSED_MANUAL" || reason != "find_only_threshold_met" {
		return ErrJobNotQualified
	}

	if fitScore < minScore {
		return ErrScoreTooLow
	}

	// Manual promotion must pass the same hard eligibility gate as every
	// other path into the assisted-apply queue (bugs.md/improvements.md:
	// "prevent assisted-apply from bypassing the remote filter"). An
	// attractive fit score is deliberately checked above this, not instead
	// of it: neither can substitute for the other.
	if profile, perr := resolveEligibilityProfile(); perr == nil {
		if ok, _ := config.IsEligibleJob(config.JobEligibilityInput{
			Title:         title,
			Location:      location,
			RemoteClaimed: isRemote.Valid && isRemote.Int64 != 0,
		}, profile); !ok {
			return ErrJobIneligible
		}
	}

	// Posting freshness (e.g. older than 30 days is stale)
	if time.Since(discoveredAt) > 30*24*time.Hour {
		return ErrStalePosting
	}

	// Duplicate/Applied protection
	var hasApplied bool
	err = tx.QueryRow(`SELECT 1 FROM applied_jobs WHERE url = ?`, u).Scan(&hasApplied)
	if err != nil && err != sql.ErrNoRows {
		return err
	}
	if hasApplied {
		return ErrAlreadyApplied
	}

	// Active leases - if it's locked by execution_state or something
	var hasLease bool
	err = tx.QueryRow(`SELECT 1 FROM execution_state WHERE url = ?`, u).Scan(&hasLease)
	if err != nil && err != sql.ErrNoRows {
		return err
	}
	if hasLease {
		return errors.New("job is currently locked by a worker")
	}

	_, err = tx.Exec(`UPDATE job_funnel 
		SET processing_intent = 'assisted_promotion',
		    status = 'DISCOVERED',
		    status_reason = 'promoted',
		    last_updated = ?
		WHERE id = ?`, time.Now().UTC(), jobID)
	if err != nil {
		return err
	}

	return tx.Commit()
}

func SkipQualifiedJob(conn *sql.DB, jobID int64) error {
	tx, err := conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var u, status string
	err = tx.QueryRow(`SELECT url, status FROM job_funnel WHERE id = ?`, jobID).Scan(&u, &status)
	if err != nil {
		if err == sql.ErrNoRows {
			return ErrJobNotFound
		}
		return err
	}

	if status != "PROCESSED_MANUAL" && status != "SKIPPED" {
		return ErrJobNotQualified
	}

	// idempotency
	if status == "SKIPPED" {
		return nil
	}

	_, err = tx.Exec(`UPDATE job_funnel 
		SET status = 'SKIPPED',
		    status_reason = 'manual_skip',
		    processing_intent = NULL,
		    last_updated = ?
		WHERE id = ?`, time.Now().UTC(), jobID)
	if err != nil {
		return err
	}

	return tx.Commit()
}

func MarkJobAppliedManually(conn *sql.DB, jobID int64) error {
	tx, err := conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var u, status, reason string
	err = tx.QueryRow(`SELECT url, status, status_reason FROM job_funnel WHERE id = ?`, jobID).Scan(&u, &status, &reason)
	if err != nil {
		if err == sql.ErrNoRows {
			return ErrJobNotFound
		}
		return err
	}

	if status == "APPLIED" {
		if reason == "manual_user_confirmation" {
			return tx.Commit()
		}
		return fmt.Errorf("conflicting application record exists")
	}

	if status != "PROCESSED_MANUAL" || reason != "find_only_threshold_met" {
		return ErrJobNotQualified
	}

	// Verify no conflicting application record exists
	var conflictCount int
	err = tx.QueryRow(`SELECT COUNT(*) FROM applied_jobs WHERE url = ?`, u).Scan(&conflictCount)
	if err != nil {
		return err
	}
	if conflictCount > 0 {
		return fmt.Errorf("conflicting application record exists")
	}

	now := time.Now().UTC()

	// Update funnel
	_, err = tx.Exec(`UPDATE job_funnel 
		SET status = 'APPLIED',
		    status_reason = 'manual_user_confirmation',
		    processing_intent = NULL,
		    applied_at = CASE WHEN applied_at IS NULL THEN ? ELSE applied_at END,
		    last_updated = ?
		WHERE id = ?`, now, now, jobID)
	if err != nil {
		return err
	}

	// Insert into applied_jobs (dedup record)
	_, err = tx.Exec(`INSERT INTO applied_jobs (url, applied_at, job_title, company_name) 
		SELECT url, ?, job_title, company_name FROM job_funnel WHERE id = ?`, now, jobID)
	if err != nil {
		return err
	}

	// Preserve a durable confirmation provenance field when the existing schema supports it.
	_, err = tx.Exec(`UPDATE assisted_applications SET confirmation_provenance = 'manual_user_confirmation' WHERE job_id = ?`, jobID)
	if err != nil {
		return err
	}

	return tx.Commit()
}
