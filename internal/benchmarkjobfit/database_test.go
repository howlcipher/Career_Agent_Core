package benchmarkjobfit

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func TestOpenReadOnlyRejectsWrites(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "cohort.db")
	writer, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open writer: %v", err)
	}
	if _, err := writer.Exec(`CREATE TABLE job_funnel (
		id INTEGER PRIMARY KEY,
		company_name TEXT,
		job_title TEXT,
		url TEXT,
		status TEXT,
		fit_score REAL,
		fit_similarity REAL,
		discovered_at DATETIME
	)`); err != nil {
		t.Fatalf("create fixture: %v", err)
	}
	if _, err := writer.Exec(`INSERT INTO job_funnel VALUES (
		1, 'Private Employer', 'Engineer', 'https://example.com/job',
		'AWAITING_REVIEW', 85, 0.7, ?
	)`, time.Date(2026, 8, 8, 0, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("insert fixture: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}

	reader, err := OpenReadOnly(path)
	if err != nil {
		t.Fatalf("OpenReadOnly() error: %v", err)
	}
	defer reader.Close()

	jobs, err := LoadSourceJobs(reader)
	if err != nil {
		t.Fatalf("LoadSourceJobs() error: %v", err)
	}
	if len(jobs) != 1 || jobs[0].Title != "Engineer" {
		t.Fatalf("LoadSourceJobs() = %#v", jobs)
	}
	if _, err := reader.Exec(`DELETE FROM job_funnel`); err == nil {
		t.Fatal("read-only database unexpectedly accepted a write")
	}
}

func TestSummarizeDistribution(t *testing.T) {
	t.Parallel()

	scores := []float64{0, 65, 80, 85, 90, 100, 100, 100}
	summary := SummarizeDistribution(scores)
	if summary.NumberScored != 8 || summary.CountEqual100 != 3 {
		t.Fatalf("unexpected counts: %#v", summary)
	}
	if summary.Median != 87.5 {
		t.Fatalf("Median = %v, want 87.5", summary.Median)
	}
	if summary.NumberDistinct != 6 {
		t.Fatalf("NumberDistinct = %d, want 6", summary.NumberDistinct)
	}
	if summary.Count80To89 != 2 || summary.Count70To79 != 0 || summary.CountBelow70 != 2 {
		t.Fatalf("unexpected bands: %#v", summary)
	}
}
