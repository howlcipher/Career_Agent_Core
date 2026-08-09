package benchmarkjobfit

import (
	"database/sql"
	"fmt"
	"math"
	"net/url"
	"path/filepath"
	"sort"
	"strconv"
	"time"

	_ "modernc.org/sqlite"
)

// Distribution summarizes stored production fit scores without exposing rows.
type Distribution struct {
	NumberScored    int     `json:"number_scored"`
	Minimum         float64 `json:"minimum"`
	Maximum         float64 `json:"maximum"`
	Mean            float64 `json:"mean"`
	Median          float64 `json:"median"`
	P10             float64 `json:"p10"`
	P25             float64 `json:"p25"`
	P75             float64 `json:"p75"`
	P90             float64 `json:"p90"`
	P95             float64 `json:"p95"`
	CountEqual100   int     `json:"count_equal_100"`
	PercentEqual100 float64 `json:"percent_equal_100"`
	CountAtLeast95  int     `json:"count_at_least_95"`
	CountAtLeast90  int     `json:"count_at_least_90"`
	Count80To89     int     `json:"count_80_to_89"`
	Count70To79     int     `json:"count_70_to_79"`
	CountBelow70    int     `json:"count_below_70"`
	NumberDistinct  int     `json:"number_distinct"`
}

// OpenReadOnly opens an existing SQLite database with both URI-level read-only
// enforcement and SQLite query_only. It never runs migrations or pragmas that
// mutate database-wide state.
func OpenReadOnly(path string) (*sql.DB, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolve database path: %w", err)
	}
	dsnURL := &url.URL{Scheme: "file", Path: filepath.ToSlash(absolute)}
	query := dsnURL.Query()
	query.Set("mode", "ro")
	query.Add("_pragma", "query_only(1)")
	query.Add("_pragma", "busy_timeout(5000)")
	dsnURL.RawQuery = query.Encode()
	database, err := sql.Open("sqlite", dsnURL.String())
	if err != nil {
		return nil, fmt.Errorf("open database read-only: %w", err)
	}
	database.SetMaxOpenConns(1)
	if err := database.Ping(); err != nil {
		database.Close()
		return nil, fmt.Errorf("ping read-only database: %w", err)
	}
	return database, nil
}

// LoadSourceJobs reads only fields required for cohort selection and fetching.
func LoadSourceJobs(database *sql.DB) ([]SourceJob, error) {
	rows, err := database.Query(`SELECT
		id,
		COALESCE(company_name, ''),
		COALESCE(job_title, ''),
		COALESCE(url, ''),
		COALESCE(status, ''),
		fit_score,
		fit_similarity,
		COALESCE(CAST(discovered_at AS TEXT), '')
		FROM job_funnel
		ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("query source jobs: %w", err)
	}
	defer rows.Close()

	var jobs []SourceJob
	for rows.Next() {
		var (
			job           SourceJob
			fitScore      sql.NullFloat64
			fitSimilarity sql.NullFloat64
			discovered    string
		)
		if err := rows.Scan(
			&job.DatabaseID,
			&job.CompanyName,
			&job.Title,
			&job.URL,
			&job.Status,
			&fitScore,
			&fitSimilarity,
			&discovered,
		); err != nil {
			return nil, fmt.Errorf("scan source job: %w", err)
		}
		if fitScore.Valid {
			job.FitScore = floatPointer(fitScore.Float64)
		}
		if fitSimilarity.Valid {
			job.FitSimilarity = floatPointer(fitSimilarity.Float64)
		}
		job.DiscoveredAt = parseSQLiteTime(discovered)
		jobs = append(jobs, job)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate source jobs: %w", err)
	}
	return jobs, nil
}

// LoadFitScores returns aggregate inputs only; callers never receive row data.
func LoadFitScores(database *sql.DB, discoveredOnOrAfter string) ([]float64, error) {
	query := `SELECT fit_score FROM job_funnel WHERE fit_score IS NOT NULL`
	arguments := []any{}
	if discoveredOnOrAfter != "" {
		if _, err := time.Parse(time.DateOnly, discoveredOnOrAfter); err != nil {
			return nil, fmt.Errorf("parse distribution start date: %w", err)
		}
		query += ` AND date(discovered_at) >= date(?)`
		arguments = append(arguments, discoveredOnOrAfter)
	}
	rows, err := database.Query(query, arguments...)
	if err != nil {
		return nil, fmt.Errorf("query fit scores: %w", err)
	}
	defer rows.Close()
	var scores []float64
	for rows.Next() {
		var score float64
		if err := rows.Scan(&score); err != nil {
			return nil, fmt.Errorf("scan fit score: %w", err)
		}
		scores = append(scores, score)
	}
	return scores, rows.Err()
}

// SummarizeDistribution computes the requested privacy-safe score aggregates.
func SummarizeDistribution(scores []float64) Distribution {
	if len(scores) == 0 {
		return Distribution{}
	}
	values := append([]float64(nil), scores...)
	sort.Float64s(values)
	distinct := make(map[string]struct{}, len(values))
	var summary Distribution
	summary.NumberScored = len(values)
	summary.Minimum = values[0]
	summary.Maximum = values[len(values)-1]
	var total float64
	for _, score := range values {
		total += score
		distinct[strconv.FormatFloat(score, 'g', -1, 64)] = struct{}{}
		switch {
		case score == 100:
			summary.CountEqual100++
		}
		if score >= 95 {
			summary.CountAtLeast95++
		}
		if score >= 90 {
			summary.CountAtLeast90++
		}
		if score >= 80 && score < 90 {
			summary.Count80To89++
		}
		if score >= 70 && score < 80 {
			summary.Count70To79++
		}
		if score < 70 {
			summary.CountBelow70++
		}
	}
	summary.Mean = total / float64(len(values))
	summary.Median = percentile(values, 0.50)
	summary.P10 = percentile(values, 0.10)
	summary.P25 = percentile(values, 0.25)
	summary.P75 = percentile(values, 0.75)
	summary.P90 = percentile(values, 0.90)
	summary.P95 = percentile(values, 0.95)
	summary.PercentEqual100 = 100 * float64(summary.CountEqual100) / float64(len(values))
	summary.NumberDistinct = len(distinct)
	return summary
}

func percentile(sorted []float64, quantile float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	position := quantile * float64(len(sorted)-1)
	lower := int(math.Floor(position))
	upper := int(math.Ceil(position))
	if lower == upper {
		return sorted[lower]
	}
	weight := position - float64(lower)
	return sorted[lower]*(1-weight) + sorted[upper]*weight
}

func parseSQLiteTime(value string) time.Time {
	for _, layout := range []string{
		time.RFC3339Nano,
		"2006-01-02 15:04:05.999999999-07:00",
		"2006-01-02 15:04:05-07:00",
		"2006-01-02 15:04:05.999999999",
		"2006-01-02 15:04:05",
		time.DateOnly,
	} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed
		}
	}
	return time.Time{}
}

func floatPointer(value float64) *float64 {
	return &value
}
