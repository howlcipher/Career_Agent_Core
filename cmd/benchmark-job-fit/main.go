// Command benchmark-job-fit creates privacy-safe aggregate measurements and
// ignored local benchmark cohorts. It never opens the production database for
// writing and is not imported by normal Career Agent operation.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/howlcipher/Career_Agent_Core/internal/benchmarkjobfit"
	"github.com/howlcipher/Career_Agent_Core/pkg/storage"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "benchmark-job-fit: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	mode := flag.String("mode", "distribution", "distribution or extract")
	databasePath := flag.String("db", storage.DefaultDatabasePath, "SQLite database path")
	since := flag.String("since", "", "optional YYYY-MM-DD distribution start")
	target := flag.Int("target", 100, "number of accepted cohort jobs")
	candidateLimit := flag.Int("candidate-limit", 360, "maximum stratified URLs to attempt")
	labelCount := flag.Int("label-count", 50, "number of human-review items")
	cohortPath := flag.String("cohort", "benchmark_results/private/job_fit_cohort.json", "ignored private cohort path")
	reviewPath := flag.String("review", "benchmark_results/private/human_review.md", "ignored private review path")
	labelsPath := flag.String("labels", "benchmark_results/private/human_labels.csv", "ignored private labels path")
	requestTimeout := flag.Duration("request-timeout", 12*time.Second, "per-posting HTTP timeout")
	flag.Parse()

	database, err := benchmarkjobfit.OpenReadOnly(*databasePath)
	if err != nil {
		return err
	}
	defer database.Close()

	switch *mode {
	case "distribution":
		scores, err := benchmarkjobfit.LoadFitScores(database, *since)
		if err != nil {
			return err
		}
		return writeJSON(os.Stdout, benchmarkjobfit.SummarizeDistribution(scores))
	case "extract":
		if *target < 1 || *candidateLimit < *target {
			return fmt.Errorf("target must be positive and candidate-limit must be at least target")
		}
		jobs, err := benchmarkjobfit.LoadSourceJobs(database)
		if err != nil {
			return err
		}
		candidates := benchmarkjobfit.SelectCandidates(jobs, *candidateLimit, 20260808)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		fetched, stats := benchmarkjobfit.FetchCohort(ctx, candidates, *target, *requestTimeout)
		head := repositoryHEAD()
		cohort := benchmarkjobfit.BuildCohort(fetched, time.Now().UTC(), head)
		if err := benchmarkjobfit.WritePrivateArtifacts(
			*cohortPath,
			*reviewPath,
			*labelsPath,
			cohort,
			*labelCount,
		); err != nil {
			return err
		}
		return writeJSON(os.Stdout, map[string]any{
			"cohort_size":        len(cohort.Jobs),
			"human_review_items": min(*labelCount, len(cohort.Jobs)),
			"fetch":              stats,
			"privacy":            "output paths contain private ignored artifacts; no source identifiers emitted",
		})
	default:
		return fmt.Errorf("unknown mode %q", *mode)
	}
}

func repositoryHEAD() string {
	output, err := exec.Command("git", "rev-parse", "--short=12", "HEAD").Output()
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(output))
}

func writeJSON(destination *os.File, value any) error {
	encoder := json.NewEncoder(destination)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}
