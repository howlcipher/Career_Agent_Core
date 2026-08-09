package benchmarkjobfit

import (
	"hash/fnv"
	"math"
	"regexp"
	"sort"
	"strings"
	"time"
)

const (
	BandPerfect       = "score_100"
	BandHigh          = "score_90_99"
	BandMedium        = "score_80_89"
	BandNearThreshold = "score_60_79"
	BandLow           = "score_below_60"
	BandUnscored      = "unscored"
)

var (
	emailPattern = regexp.MustCompile(`(?i)\b[A-Z0-9._%+\-]+@[A-Z0-9.\-]+\.[A-Z]{2,}\b`)
	urlPattern   = regexp.MustCompile(`(?i)\b(?:https?://|www\.)\S+`)
	phonePattern = regexp.MustCompile(`(?:\+?\d[\d().\-\s]{7,}\d)`)
)

// SourceJob is an in-memory production row. CompanyName and URL exist only
// long enough to fetch and redact a posting and are never serialized.
type SourceJob struct {
	DatabaseID    int64
	CompanyName   string
	Title         string
	URL           string
	Status        string
	FitScore      *float64
	FitSimilarity *float64
	DiscoveredAt  time.Time
}

// FetchedJob is a source row paired with visible posting text.
type FetchedJob struct {
	Source      SourceJob
	Description string
}

// Cohort is the private, ignored benchmark artifact consumed by the Python
// runner. It deliberately excludes employer names, URLs, and database IDs.
type Cohort struct {
	SchemaVersion  int          `json:"schema_version"`
	GeneratedAt    string       `json:"generated_at"`
	RepositoryHEAD string       `json:"repository_head"`
	Privacy        string       `json:"privacy"`
	Jobs           []CohortItem `json:"jobs"`
}

// CohortItem distinguishes current model signals from observed workflow state
// and from the separate human-label artifact.
type CohortItem struct {
	BenchmarkID              string   `json:"benchmark_id"`
	Title                    string   `json:"title"`
	Description              string   `json:"description"`
	ScoreBand                string   `json:"score_band"`
	ModelFitScore            *float64 `json:"model_fit_score"`
	ModelEmbeddingSimilarity *float64 `json:"model_embedding_similarity"`
	ObservedWorkflowStatus   string   `json:"observed_workflow_status"`
	DiscoveredDate           string   `json:"discovered_date"`
}

// ScoreBand assigns production scores to the benchmark's fixed strata.
func ScoreBand(score *float64) string {
	if score == nil {
		return BandUnscored
	}
	switch {
	case *score >= 100:
		return BandPerfect
	case *score >= 90:
		return BandHigh
	case *score >= 80:
		return BandMedium
	case *score >= 60:
		return BandNearThreshold
	default:
		return BandLow
	}
}

func deterministicOrder(job SourceJob, seed uint64) uint64 {
	hash := fnv.New64a()
	_, _ = hash.Write([]byte(job.Title))
	_, _ = hash.Write([]byte{0})
	_, _ = hash.Write([]byte(job.URL))
	value := hash.Sum64() ^ seed ^ uint64(job.DatabaseID)
	value ^= value >> 30
	value *= 0xbf58476d1ce4e5b9
	value ^= value >> 27
	value *= 0x94d049bb133111eb
	return value ^ (value >> 31)
}

// SelectCandidates returns a deterministic, stratified, round-robin list.
// Quotas are proportional to a 120-job target and unused quota is backfilled
// from every remaining stratum rather than fabricating a missing score band.
func SelectCandidates(jobs []SourceJob, limit int, seed uint64) []SourceJob {
	if limit <= 0 {
		return nil
	}
	bandOrder := []string{
		BandPerfect,
		BandHigh,
		BandMedium,
		BandNearThreshold,
		BandLow,
		BandUnscored,
	}
	weights := map[string]int{
		BandPerfect:       20,
		BandHigh:          15,
		BandMedium:        20,
		BandNearThreshold: 15,
		BandLow:           20,
		BandUnscored:      30,
	}
	byBand := make(map[string][]SourceJob, len(bandOrder))
	for _, job := range jobs {
		if strings.TrimSpace(job.Title) == "" || strings.TrimSpace(job.URL) == "" {
			continue
		}
		band := ScoreBand(job.FitScore)
		byBand[band] = append(byBand[band], job)
	}
	for _, band := range bandOrder {
		sort.SliceStable(byBand[band], func(left, right int) bool {
			leftOrder := deterministicOrder(byBand[band][left], seed)
			rightOrder := deterministicOrder(byBand[band][right], seed)
			if leftOrder == rightOrder {
				return byBand[band][left].DatabaseID < byBand[band][right].DatabaseID
			}
			return leftOrder < rightOrder
		})
	}

	selected := make(map[string][]SourceJob, len(bandOrder))
	remaining := make(map[string][]SourceJob, len(bandOrder))
	selectedCount := 0
	for _, band := range bandOrder {
		quota := int(math.Round(float64(limit) * float64(weights[band]) / 120.0))
		if quota < 1 {
			quota = 1
		}
		if quota > len(byBand[band]) {
			quota = len(byBand[band])
		}
		selected[band] = append(selected[band], byBand[band][:quota]...)
		remaining[band] = byBand[band][quota:]
		selectedCount += quota
	}

	for selectedCount < limit {
		added := false
		for _, band := range bandOrder {
			if len(remaining[band]) == 0 || selectedCount >= limit {
				continue
			}
			selected[band] = append(selected[band], remaining[band][0])
			remaining[band] = remaining[band][1:]
			selectedCount++
			added = true
		}
		if !added {
			break
		}
	}

	result := make([]SourceJob, 0, selectedCount)
	for index := 0; len(result) < selectedCount; index++ {
		for _, band := range bandOrder {
			if index < len(selected[band]) {
				result = append(result, selected[band][index])
			}
		}
	}
	if len(result) > limit {
		result = result[:limit]
	}
	return result
}

// SanitizeDescription removes source identifiers and bounds private text.
func SanitizeDescription(text string, employer string, maxRunes int) string {
	text = strings.Join(strings.Fields(text), " ")
	if len([]rune(strings.TrimSpace(employer))) >= 4 {
		employerPattern := regexp.MustCompile(`(?i)` + regexp.QuoteMeta(strings.TrimSpace(employer)))
		text = employerPattern.ReplaceAllString(text, "[employer]")
	}
	text = emailPattern.ReplaceAllString(text, "[email]")
	text = urlPattern.ReplaceAllString(text, "[link]")
	text = phonePattern.ReplaceAllString(text, "[phone]")
	text = strings.Join(strings.Fields(text), " ")
	if maxRunes > 0 {
		runes := []rune(text)
		if len(runes) > maxRunes {
			text = strings.TrimSpace(string(runes[:maxRunes])) + "…"
		}
	}
	return text
}

// BuildCohort drops all transient source identifiers and assigns opaque IDs.
func BuildCohort(jobs []FetchedJob, generatedAt time.Time, head string) Cohort {
	items := make([]CohortItem, 0, len(jobs))
	for index, job := range jobs {
		discoveredDate := ""
		if !job.Source.DiscoveredAt.IsZero() {
			discoveredDate = job.Source.DiscoveredAt.UTC().Format(time.DateOnly)
		}
		items = append(items, CohortItem{
			BenchmarkID:              benchmarkID(index + 1),
			Title:                    SanitizeDescription(job.Source.Title, job.Source.CompanyName, 240),
			Description:              SanitizeDescription(job.Description, job.Source.CompanyName, 6000),
			ScoreBand:                ScoreBand(job.Source.FitScore),
			ModelFitScore:            job.Source.FitScore,
			ModelEmbeddingSimilarity: job.Source.FitSimilarity,
			ObservedWorkflowStatus:   job.Source.Status,
			DiscoveredDate:           discoveredDate,
		})
	}
	return Cohort{
		SchemaVersion:  1,
		GeneratedAt:    generatedAt.UTC().Format(time.RFC3339),
		RepositoryHEAD: head,
		Privacy:        "private local artifact; employer names, URLs, and database IDs removed",
		Jobs:           items,
	}
}

func benchmarkID(index int) string {
	if index < 10 {
		return "job-00" + string(rune('0'+index))
	}
	if index < 100 {
		return "job-0" + intString(index)
	}
	return "job-" + intString(index)
}

func intString(value int) string {
	const digits = "0123456789"
	if value == 0 {
		return "0"
	}
	buffer := [20]byte{}
	position := len(buffer)
	for value > 0 {
		position--
		buffer[position] = digits[value%10]
		value /= 10
	}
	return string(buffer[position:])
}
