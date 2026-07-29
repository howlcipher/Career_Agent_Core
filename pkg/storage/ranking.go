package storage

import (
	"math"
	"sort"
	"strings"
	"time"
)

// RankableJob is a generic interface for jobs that can be ranked.
type RankableJob interface {
	GetURL() string
	GetFitSimilarity() float64
	GetDiscoveredAt() time.Time
	SetRankData(score float64, reason string, isExploration bool)
}

// SourceRankScore holds computed metrics for a source.
type SourceRankScore struct {
	SmoothedSuccessRate float64
	PenaltyFactor       float64
	AvgInferenceMs      int
	Confidence          string
}

// ComputeSourceScores computes a ranking score for each source based on its 30-day health summary.
func ComputeSourceScores(summaries []SourceHealthSummary) map[string]SourceRankScore {
	scores := make(map[string]SourceRankScore)

	// Priors for Bayesian smoothing
	const priorWeight = 5.0
	const priorSuccessRate = 0.05 // Assume 5% success for unseen sources

	for _, s := range summaries {
		// Calculate smoothed success rate
		// Applied counts as success.
		successCount := float64(s.AppliedCount)
		totalCount := float64(s.TotalAttempts)

		smoothedSuccess := (successCount + priorWeight*priorSuccessRate) / (totalCount + priorWeight)

		// Calculate penalty factor (e.g. high CAPTCHA or DEAD posting rate).
		// Penalties reduce the score.
		badOutcomes := float64(s.CaptchaCount + s.DeadCount + s.ManualCount)
		badRate := badOutcomes / totalCount
		if totalCount == 0 {
			badRate = 0.5 // Default penalty for sparse
		}

		// Map badRate (0 to 1) to a penalty factor (0.2 to 1.0)
		penaltyFactor := 1.0 - (badRate * 0.8)

		// Bug #400 / Improvement: Apply speed penalty for very slow sources to make the pipeline faster
		if s.AvgInferenceMs > 20000 {
			// e.g. 120s (120,000ms) = 0.2 penalty
			speedPenalty := float64(s.AvgInferenceMs-20000) / 500000.0
			if speedPenalty > 0.4 {
				speedPenalty = 0.4
			}
			penaltyFactor -= speedPenalty
		}
		if penaltyFactor < 0.1 {
			penaltyFactor = 0.1
		}

		scores[s.Source] = SourceRankScore{
			SmoothedSuccessRate: smoothedSuccess,
			PenaltyFactor:       penaltyFactor,
			AvgInferenceMs:      s.AvgInferenceMs,
			Confidence:          s.Confidence,
		}
	}
	return scores
}

type jobWrapper struct {
	Index         int
	Score         float64
	IsExploration bool
}

// RankJobs sorts the given slice of RankableJob using Bayesian smoothed probability.
// It reserves explorationShare (0.0 to 1.0) of the top slots for jobs from sparse sources.
func RankJobs[T RankableJob](jobs []T, summaries []SourceHealthSummary, explorationShare float64) []T {
	if len(jobs) == 0 {
		return jobs
	}

	sourceScores := ComputeSourceScores(summaries)

	// Priors for unknown sources
	const priorSuccessRate = 0.05
	const priorPenalty = 0.6 // Medium penalty

	wrappers := make([]jobWrapper, len(jobs))

	for i, j := range jobs {
		source := j.GetURL()
		// fast hostname extraction
		if idx := strings.Index(source, "://"); idx != -1 {
			source = source[idx+3:]
		}
		if idx := strings.IndexByte(source, '/'); idx != -1 {
			source = source[:idx]
		}
		if idx := strings.IndexByte(source, ':'); idx != -1 {
			source = source[:idx]
		}

		scoreObj, exists := sourceScores[source]
		if !exists {
			scoreObj = SourceRankScore{
				SmoothedSuccessRate: priorSuccessRate,
				PenaltyFactor:       priorPenalty,
				AvgInferenceMs:      0,
				Confidence:          "Sparse",
			}
		}

		fit := j.GetFitSimilarity()
		if fit < 0 {
			fit = 0.0
		}

		ageDays := int(time.Since(j.GetDiscoveredAt()).Hours() / 24)
		freshnessMultiplier := math.Exp(-0.02 * float64(ageDays))

		combinedFit := 0.2 + (0.8 * fit)
		rawScore := scoreObj.SmoothedSuccessRate * combinedFit * freshnessMultiplier * scoreObj.PenaltyFactor

		isExploration := scoreObj.Confidence == "Sparse"
		reason := "Ranked by outcome"
		if isExploration {
			reason = "Exploration candidate (sparse data)"
		}

		// Adjust score to give exploration jobs a boost or segregate them.
		// For simplicity, we just set the data and use the wrappers to sort.
		j.SetRankData(rawScore, reason, isExploration)

		wrappers[i] = jobWrapper{
			Index:         i,
			Score:         rawScore,
			IsExploration: isExploration,
		}
	}

	// Separate into explore and exploit pools
	var exploit []jobWrapper
	var explore []jobWrapper
	for _, w := range wrappers {
		if w.IsExploration {
			explore = append(explore, w)
		} else {
			exploit = append(exploit, w)
		}
	}

	// Sort both pools by score DESC
	sortByScoreDesc := func(slice []jobWrapper) {
		sort.SliceStable(slice, func(a, b int) bool {
			return slice[a].Score > slice[b].Score
		})
	}
	sortByScoreDesc(exploit)
	sortByScoreDesc(explore)

	// Merge pools based on explorationShare
	result := make([]T, 0, len(jobs))
	exploitIdx := 0
	exploreIdx := 0

	// Example: if explorationShare = 0.2, 1 in 5 jobs should be from explore if available.
	exploreInterval := 0
	if explorationShare > 0 {
		exploreInterval = int(math.Round(1.0 / explorationShare))
	}

	for i := 0; i < len(jobs); i++ {
		takeExplore := false
		if exploreInterval > 0 && i%exploreInterval == (exploreInterval-1) {
			takeExplore = true
		}

		if takeExplore && exploreIdx < len(explore) {
			result = append(result, jobs[explore[exploreIdx].Index])
			exploreIdx++
		} else if exploitIdx < len(exploit) {
			result = append(result, jobs[exploit[exploitIdx].Index])
			exploitIdx++
		} else if exploreIdx < len(explore) {
			// Exploit pool is empty, drain explore pool
			result = append(result, jobs[explore[exploreIdx].Index])
			exploreIdx++
		} else {
			// Both empty, should not happen since len(result) == len(jobs)
			break
		}
	}

	return result
}
