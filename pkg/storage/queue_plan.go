package storage

import (
	"database/sql"
	"fmt"
	"net/url"
	"strings"
	"time"
)

type QueuePlanCandidate struct {
	OriginalURL    string
	NormalizedURL  string
	Source         string
	CurrentStatus  string
	AgeDays        int
	FitSimilarity  float64
	PriorOutcome   string
	HasDedupRow    bool
	HasSchemeDup   bool
	ProposedAction string
}

type QueuePlan struct {
	Candidates         []QueuePlanCandidate
	TotalCandidates    int
	TotalWithDedup     int
	TotalWithSchemeDup int
}

func GetQueuePlan(urlPattern, fromStatus string, willClearDedup bool) (*QueuePlan, error) {
	if db == nil {
		return nil, fmt.Errorf("db not initialized")
	}

	query := `SELECT url, status, discovered_at, fit_similarity FROM job_funnel WHERE status = ? AND url LIKE ?`
	rows, err := db.Query(query, fromStatus, urlPattern)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var plan QueuePlan

	for rows.Next() {
		var cand QueuePlanCandidate
		var discoveredAt time.Time
		var fitSim sql.NullFloat64
		if err := rows.Scan(&cand.OriginalURL, &cand.CurrentStatus, &discoveredAt, &fitSim); err != nil {
			return nil, err
		}
		if fitSim.Valid {
			cand.FitSimilarity = fitSim.Float64
		}

		cand.NormalizedURL = NormalizeURL(cand.OriginalURL)
		cand.AgeDays = int(time.Since(discoveredAt).Hours() / 24)
		cand.PriorOutcome = cand.CurrentStatus
		
		// Determine source from domain
		if u, err := url.Parse(cand.OriginalURL); err == nil {
			cand.Source = u.Hostname()
		} else {
			cand.Source = cand.OriginalURL
		}

		// Check for dedup row
		var dedupCount int
		err = db.QueryRow(`SELECT COUNT(*) FROM applied_jobs WHERE url = ? OR url = ?`, cand.OriginalURL, cand.NormalizedURL).Scan(&dedupCount)
		if err != nil {
			return nil, err
		}
		cand.HasDedupRow = dedupCount > 0

		// Check for scheme duplicate
		var dupCount int
		// A scheme duplicate is another row with the same normalized URL but a different original URL, or maybe just different scheme
		otherScheme := cand.OriginalURL
		if strings.HasPrefix(cand.OriginalURL, "http://") {
			otherScheme = strings.Replace(cand.OriginalURL, "http://", "https://", 1)
		} else if strings.HasPrefix(cand.OriginalURL, "https://") {
			otherScheme = strings.Replace(cand.OriginalURL, "https://", "http://", 1)
		}
		
		if otherScheme != cand.OriginalURL {
			err = db.QueryRow(`SELECT COUNT(*) FROM job_funnel WHERE url = ?`, otherScheme).Scan(&dupCount)
			if err != nil {
				return nil, err
			}
			cand.HasSchemeDup = dupCount > 0
		}

		cand.ProposedAction = "Requeue to DISCOVERED"
		if cand.HasDedupRow {
			if willClearDedup {
				cand.ProposedAction = "Clear applied_jobs dedup and requeue to DISCOVERED"
			} else {
				cand.ProposedAction = "Requeue to DISCOVERED (WARNING: Has dedup row, will be skipped by HasApplied unless -clear-dedup is used)"
			}
		}

		if cand.HasDedupRow {
			plan.TotalWithDedup++
		}
		if cand.HasSchemeDup {
			plan.TotalWithSchemeDup++
		}
		
		plan.Candidates = append(plan.Candidates, cand)
	}
	plan.TotalCandidates = len(plan.Candidates)
	return &plan, nil
}
