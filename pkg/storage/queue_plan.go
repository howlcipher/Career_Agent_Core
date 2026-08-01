package storage

import (
	"database/sql"
	"fmt"
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

	// Ranking fields
	DiscoveredAt  time.Time
	RankingScore  float64
	RankingReason string
	IsExploration bool
}

func (c *QueuePlanCandidate) GetURL() string             { return c.OriginalURL }
func (c *QueuePlanCandidate) GetFitSimilarity() float64  { return c.FitSimilarity }
func (c *QueuePlanCandidate) GetDiscoveredAt() time.Time { return c.DiscoveredAt }
func (c *QueuePlanCandidate) SetRankData(score float64, reason string, isExploration bool) {
	c.RankingScore = score
	c.RankingReason = reason
	c.IsExploration = isExploration
}

type QueuePlan struct {
	Candidates         []QueuePlanCandidate
	TotalCandidates    int
	TotalWithDedup     int
	TotalWithSchemeDup int
}

// queuePlanRows is the subset of *sql.Rows that scanQueuePlanCandidates
// needs, factored out so a hand-rolled fake can stand in for a test that
// needs Next() to fail mid-stream rather than exhaust normally -- a real
// driver cannot be made to produce a cursor fault on demand.
type queuePlanRows interface {
	Next() bool
	Scan(dest ...any) error
	Err() error
}

// scanQueuePlanCandidates drains rows into QueuePlanCandidates. Next()
// returning false can mean either "rows exhausted" or "a cursor error
// occurred" -- database/sql cannot tell the caller apart without an
// explicit Err() check, so a fault partway through the result set must not
// be mistaken for a complete, if short, plan.
func scanQueuePlanCandidates(rows queuePlanRows, willClearDedup bool) ([]QueuePlanCandidate, int, int, error) {
	var candidates []QueuePlanCandidate
	var totalWithDedup, totalWithSchemeDup int

	for rows.Next() {
		var cand QueuePlanCandidate
		var discoveredAt sql.NullTime
		var fitSim sql.NullFloat64
		var dedupCount, dupCount int
		if err := rows.Scan(&cand.OriginalURL, &cand.CurrentStatus, &discoveredAt, &fitSim, &dedupCount, &dupCount); err != nil {
			continue
		}
		if fitSim.Valid {
			cand.FitSimilarity = fitSim.Float64
		}

		var discoveredAtTime time.Time
		if discoveredAt.Valid {
			discoveredAtTime = discoveredAt.Time
		} else {
			discoveredAtTime = time.Now()
		}

		cand.NormalizedURL = NormalizeURL(cand.OriginalURL)
		cand.AgeDays = int(time.Since(discoveredAtTime).Hours() / 24)
		cand.PriorOutcome = cand.CurrentStatus
		cand.DiscoveredAt = discoveredAtTime

		// Determine source from domain
		source := cand.OriginalURL
		if idx := strings.Index(source, "://"); idx != -1 {
			source = source[idx+3:]
		}
		if idx := strings.IndexByte(source, '/'); idx != -1 {
			source = source[:idx]
		}
		if idx := strings.IndexByte(source, ':'); idx != -1 {
			source = source[:idx]
		}
		cand.Source = source

		cand.HasDedupRow = dedupCount > 0
		cand.HasSchemeDup = dupCount > 0

		cand.ProposedAction = "Requeue to DISCOVERED"
		if cand.HasDedupRow {
			if willClearDedup {
				cand.ProposedAction = "Clear applied_jobs dedup and requeue to DISCOVERED"
			} else {
				cand.ProposedAction = "Requeue to DISCOVERED (WARNING: Has dedup row, will be skipped by HasApplied unless -clear-dedup is used)"
			}
		}

		if cand.HasDedupRow {
			totalWithDedup++
		}
		if cand.HasSchemeDup {
			totalWithSchemeDup++
		}

		candidates = append(candidates, cand)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, 0, fmt.Errorf("scan queue plan candidates: %w", err)
	}

	return candidates, totalWithDedup, totalWithSchemeDup, nil
}

func GetQueuePlan(urlPattern, fromStatus string, willClearDedup bool) (*QueuePlan, error) {
	if db == nil {
		return nil, fmt.Errorf("db not initialized")
	}

	query := `SELECT f.url, f.status, f.discovered_at, f.fit_similarity,
		(SELECT COUNT(*) FROM applied_jobs a WHERE a.url = f.url OR a.url = CASE WHEN f.url LIKE 'http://%' THEN REPLACE(f.url, 'http://', 'https://') ELSE f.url END) as dedup_count,
		(SELECT COUNT(*) FROM job_funnel dup WHERE dup.url = CASE WHEN f.url LIKE 'http://%' THEN REPLACE(f.url, 'http://', 'https://') WHEN f.url LIKE 'https://%' THEN REPLACE(f.url, 'https://', 'http://') ELSE f.url END AND dup.url != f.url) as scheme_dup_count
		FROM job_funnel f WHERE f.status = ? AND f.url LIKE ?`
	rows, err := db.Query(query, fromStatus, urlPattern)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var plan QueuePlan
	candidates, totalWithDedup, totalWithSchemeDup, err := scanQueuePlanCandidates(rows, willClearDedup)
	if err != nil {
		return nil, err
	}
	plan.Candidates = candidates
	plan.TotalWithDedup = totalWithDedup
	plan.TotalWithSchemeDup = totalWithSchemeDup

	summaries, err := GetSourceHealthSummaries(30)
	if err == nil {
		var ptrs []*QueuePlanCandidate
		for i := range plan.Candidates {
			ptrs = append(ptrs, &plan.Candidates[i])
		}
		ptrs = RankJobs(ptrs, summaries, 0.20)
		var ranked []QueuePlanCandidate
		for _, p := range ptrs {
			ranked = append(ranked, *p)
		}
		plan.Candidates = ranked
	}

	plan.TotalCandidates = len(plan.Candidates)
	return &plan, nil
}
