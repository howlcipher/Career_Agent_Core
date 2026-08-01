package modelbench

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func sampleReport() Report {
	return Report{
		GeneratedAt: time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC),
		Config: Config{
			Host:        DefaultHost,
			Models:      []string{"test-model"},
			Tasks:       []string{"classify_error"},
			Repetitions: 2,
			Timeout:     "5m0s",
			Temperature: 0,
		},
		Models: []ModelReport{
			{
				Model:     "test-model",
				SizeBytes: 123,
				Results: []TaskResult{
					{Task: "classify_error", Repetition: 1, ColdStart: true, WallDurationMS: 500, SchemaValid: true, Correct: true},
					{Task: "classify_error", Repetition: 2, ColdStart: false, WallDurationMS: 100, SchemaValid: true, Correct: false},
				},
			},
		},
	}
}

func TestReport_JSONRoundTrip(t *testing.T) {
	rep := sampleReport()
	raw, err := rep.JSON()
	if err != nil {
		t.Fatalf("JSON: %v", err)
	}
	var decoded Report
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("round trip unmarshal: %v", err)
	}
	if len(decoded.Models) != 1 || decoded.Models[0].Model != "test-model" {
		t.Fatalf("decoded report mismatch: %+v", decoded)
	}
	if len(decoded.Models[0].Results) != 2 {
		t.Fatalf("decoded %d results, want 2", len(decoded.Models[0].Results))
	}
}

func TestReport_AllPassed(t *testing.T) {
	rep := sampleReport()
	if !rep.AllPassed() {
		t.Fatalf("expected all-schema-valid results to pass, got %+v", rep.Models[0].Results)
	}

	rep.Models[0].Results[1].SchemaValid = false
	if rep.AllPassed() {
		t.Fatalf("expected AllPassed=false once a result is schema-invalid")
	}
}

func TestTaskResult_TimeoutFailsRegardlessOfSchemaValid(t *testing.T) {
	r := TaskResult{SchemaValid: true, TimedOut: true}
	if r.Passed() {
		t.Fatalf("a timed-out result must not count as passed even if SchemaValid is true")
	}
}

func TestTaskResult_ErrorFailsRegardlessOfSchemaValid(t *testing.T) {
	r := TaskResult{SchemaValid: true, Error: "boom"}
	if r.Passed() {
		t.Fatalf("an errored result must not count as passed even if SchemaValid is true")
	}
}

func TestReport_SummaryContainsModelAndTotals(t *testing.T) {
	summary := sampleReport().Summary()
	if !strings.Contains(summary, "test-model") {
		t.Errorf("summary should name the model, got:\n%s", summary)
	}
	if !strings.Contains(summary, "TOTAL:") {
		t.Errorf("summary should include a total tally, got:\n%s", summary)
	}
}
