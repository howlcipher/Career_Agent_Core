package storage

import (
	"database/sql"
	"errors"
	"fmt"
	"time"
)

var (
	ErrJobNotFound     = errors.New("job not found")
	ErrJobNotQualified = errors.New("job is not in a qualified state")
	ErrScoreTooLow     = errors.New("job score is below the current threshold")
	ErrAlreadyApplied  = errors.New("job has already been applied to")
	ErrStalePosting    = errors.New("posting is expired or stale")
	ErrAlreadyPromoted = errors.New("job is already promoted")
)

// PromoteJobToAssisted atomically promotes a qualified job to assisted apply mode.
func PromoteJobToAssisted(jobID int64, minScore int) error {
	if db == nil {
		return fmt.Errorf("db not initialized")
	}

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var u, status, reason string
	var fitScore int
	var discoveredAt time.Time
	var processingIntent sql.NullString

	err = tx.QueryRow(`SELECT url, status, COALESCE(status_reason, ''), fit_score, discovered_at, processing_intent 
		FROM job_funnel WHERE rowid = ?`, jobID).Scan(&u, &status, &reason, &fitScore, &discoveredAt, &processingIntent)
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
		WHERE rowid = ?`, time.Now().UTC(), jobID)
	if err != nil {
		return err
	}

	return tx.Commit()
}

func SkipQualifiedJob(jobID int64) error {
	if db == nil {
		return fmt.Errorf("db not initialized")
	}

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var u, status string
	err = tx.QueryRow(`SELECT url, status FROM job_funnel WHERE rowid = ?`, jobID).Scan(&u, &status)
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
		WHERE rowid = ?`, time.Now().UTC(), jobID)
	if err != nil {
		return err
	}

	return tx.Commit()
}

func MarkJobAppliedManually(jobID int64) error {
	if db == nil {
		return fmt.Errorf("db not initialized")
	}

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var u, status string
	err = tx.QueryRow(`SELECT url, status FROM job_funnel WHERE rowid = ?`, jobID).Scan(&u, &status)
	if err != nil {
		if err == sql.ErrNoRows {
			return ErrJobNotFound
		}
		return err
	}

	if status != "PROCESSED_MANUAL" && status != "AWAITING_REVIEW" && status != "MANUAL_REQUIRED" && status != "DISCOVERED" && status != "APPLIED" {
		return ErrJobNotQualified
	}

	if status == "APPLIED" {
		return nil
	}

	now := time.Now().UTC()

	// Update funnel
	_, err = tx.Exec(`UPDATE job_funnel 
		SET status = 'APPLIED',
		    status_reason = 'manual_user_confirmation',
		    processing_intent = NULL,
		    applied_at = CASE WHEN applied_at IS NULL THEN ? ELSE applied_at END,
		    last_updated = ?
		WHERE rowid = ?`, now, now, jobID)
	if err != nil {
		return err
	}

	// Insert into applied_jobs (dedup record)
	_, err = tx.Exec(`INSERT OR IGNORE INTO applied_jobs (url, applied_at, role_name, company_name) 
		SELECT url, ?, job_title, company_name FROM job_funnel WHERE rowid = ?`, now, jobID)
	if err != nil {
		return err
	}

	return tx.Commit()
}
