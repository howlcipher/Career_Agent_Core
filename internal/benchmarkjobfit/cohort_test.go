package benchmarkjobfit

import (
	"strings"
	"testing"
	"time"
)

func float64Pointer(value float64) *float64 {
	return &value
}

func TestScoreBand(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		score *float64
		want  string
	}{
		{name: "unscored", score: nil, want: BandUnscored},
		{name: "perfect", score: float64Pointer(100), want: BandPerfect},
		{name: "high", score: float64Pointer(90), want: BandHigh},
		{name: "medium", score: float64Pointer(80), want: BandMedium},
		{name: "near threshold", score: float64Pointer(65), want: BandNearThreshold},
		{name: "low", score: float64Pointer(15), want: BandLow},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := ScoreBand(test.score); got != test.want {
				t.Fatalf("ScoreBand() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestSelectCandidatesIsDeterministicAndStratified(t *testing.T) {
	t.Parallel()

	var jobs []SourceJob
	bands := []struct {
		score *float64
		count int
	}{
		{score: float64Pointer(100), count: 30},
		{score: float64Pointer(90), count: 20},
		{score: float64Pointer(85), count: 25},
		{score: float64Pointer(65), count: 15},
		{score: float64Pointer(15), count: 20},
		{score: nil, count: 40},
	}
	id := int64(1)
	for _, band := range bands {
		for range band.count {
			jobs = append(jobs, SourceJob{
				DatabaseID:  id,
				CompanyName: "Example Employer",
				Title:       "Role",
				URL:         "https://example.com/job",
				FitScore:    band.score,
			})
			id++
		}
	}

	first := SelectCandidates(jobs, 120, 20260808)
	second := SelectCandidates(jobs, 120, 20260808)
	if len(first) != 120 {
		t.Fatalf("len(SelectCandidates()) = %d, want 120", len(first))
	}
	for index := range first {
		if first[index].DatabaseID != second[index].DatabaseID {
			t.Fatalf("selection changed at index %d", index)
		}
	}

	counts := make(map[string]int)
	for _, job := range first {
		counts[ScoreBand(job.FitScore)]++
	}
	for _, band := range []string{
		BandPerfect,
		BandHigh,
		BandMedium,
		BandNearThreshold,
		BandLow,
		BandUnscored,
	} {
		if counts[band] == 0 {
			t.Fatalf("selection omitted band %q", band)
		}
	}
}

func TestSanitizeDescriptionRemovesSensitiveIdentifiers(t *testing.T) {
	t.Parallel()

	input := strings.Join([]string{
		"Acme Robotics is hiring.",
		"Email recruiter@example.com or call +1 (415) 555-0123.",
		"Apply at https://jobs.example.com/secret?id=42.",
	}, " ")
	got := SanitizeDescription(input, "Acme Robotics", 500)
	for _, forbidden := range []string{
		"Acme Robotics",
		"recruiter@example.com",
		"415",
		"jobs.example.com",
		"secret?id=42",
	} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("sanitized text still contains %q: %q", forbidden, got)
		}
	}
	for _, marker := range []string{"[employer]", "[email]", "[phone]", "[link]"} {
		if !strings.Contains(got, marker) {
			t.Fatalf("sanitized text does not contain %q: %q", marker, got)
		}
	}
}

func TestBuildCohortUsesOpaqueIDsAndNoSourceURL(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC)
	jobs := []FetchedJob{{
		Source: SourceJob{
			DatabaseID:    42,
			CompanyName:   "Example Employer",
			Title:         "Platform Engineer",
			URL:           "https://example.com/private-job",
			Status:        "AWAITING_REVIEW",
			FitScore:      float64Pointer(85),
			FitSimilarity: float64Pointer(0.72),
			DiscoveredAt:  now,
		},
		Description: "Build reliable Go services for Example Employer.",
	}}

	cohort := BuildCohort(jobs, now, "a27ffde")
	if len(cohort.Jobs) != 1 {
		t.Fatalf("len(cohort.Jobs) = %d, want 1", len(cohort.Jobs))
	}
	item := cohort.Jobs[0]
	if item.BenchmarkID != "job-001" {
		t.Fatalf("BenchmarkID = %q, want job-001", item.BenchmarkID)
	}
	if strings.Contains(item.Description, "Example Employer") {
		t.Fatalf("description contains employer name: %q", item.Description)
	}
}
