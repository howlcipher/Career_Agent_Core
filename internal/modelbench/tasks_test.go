package modelbench

import (
	"regexp"
	"strings"
	"testing"
)

func TestClassifyErrorTask_SchemaValidAndCorrect(t *testing.T) {
	task := classifyErrorTask()
	result := task.Validate(`{"category":"network","confidence":0.85}`)
	if !result.SchemaValid || !result.Correct {
		t.Fatalf("expected schema-valid and correct, got %+v", result)
	}
}

func TestClassifyErrorTask_SchemaValidButWrongAnswer(t *testing.T) {
	task := classifyErrorTask()
	result := task.Validate(`{"category":"auth","confidence":0.5}`)
	if !result.SchemaValid {
		t.Fatalf("a well-formed but wrong answer should still be schema-valid, got %+v", result)
	}
	if result.Correct {
		t.Fatalf("expected Correct=false for the wrong category, got %+v", result)
	}
}

func TestClassifyErrorTask_InvalidJSON(t *testing.T) {
	task := classifyErrorTask()
	result := task.Validate(`not json at all`)
	if result.SchemaValid {
		t.Fatalf("expected schema-invalid for malformed JSON, got %+v", result)
	}
	if !strings.Contains(result.Reason, "invalid JSON") {
		t.Errorf("reason should say invalid JSON, got %q", result.Reason)
	}
}

func TestClassifyErrorTask_OutOfEnumCategory(t *testing.T) {
	task := classifyErrorTask()
	result := task.Validate(`{"category":"weather","confidence":0.5}`)
	if result.SchemaValid {
		t.Fatalf("expected schema-invalid for an out-of-enum category, got %+v", result)
	}
}

func TestClassifyErrorTask_ConfidenceOutOfRange(t *testing.T) {
	task := classifyErrorTask()
	result := task.Validate(`{"category":"network","confidence":1.5}`)
	if result.SchemaValid {
		t.Fatalf("expected schema-invalid for out-of-range confidence, got %+v", result)
	}
}

func TestClassifyErrorTask_OutputTooLong(t *testing.T) {
	task := classifyErrorTask()
	huge := `{"category":"network","confidence":0.9,"padding":"` + strings.Repeat("x", 1000) + `"}`
	result := task.Validate(huge)
	if result.SchemaValid {
		t.Fatalf("expected schema-invalid for output exceeding the byte cap, got %+v", result)
	}
}

func TestSummarizeTask_ValidWithRequiredKeywords(t *testing.T) {
	task := summarizeExcerptTask()
	result := task.Validate("ComputeRetryDelay implements exponential backoff with random jitter added to the delay.")
	if !result.SchemaValid {
		t.Fatalf("expected schema-valid, got %+v", result)
	}
}

func TestSummarizeTask_MissingRequiredKeyword(t *testing.T) {
	task := summarizeExcerptTask()
	result := task.Validate("This function computes a delay for retries.")
	if result.SchemaValid {
		t.Fatalf("expected schema-invalid when the function name and jitter are never mentioned, got %+v", result)
	}
	if !strings.Contains(result.Reason, "ComputeRetryDelay") {
		t.Errorf("reason should name the missing keyword, got %q", result.Reason)
	}
}

func TestSummarizeTask_OutputTooLong(t *testing.T) {
	task := summarizeExcerptTask()
	result := task.Validate(strings.Repeat("ComputeRetryDelay jitter backoff ", 200))
	if result.SchemaValid {
		t.Fatalf("expected schema-invalid for an output exceeding the byte cap, got %+v", result)
	}
}

func TestPlanTestsTask_ValidResponse(t *testing.T) {
	task := planTestsTask()
	result := task.Validate(`{
		"root_cause": "NormalizeURL never calls strings.ToLower on the host",
		"planned_files": ["pkg/scraper/normalize.go"],
		"tests": {"success": "mixed-case host dedups", "failure": "still-different host stays distinct"}
	}`)
	if !result.SchemaValid {
		t.Fatalf("expected schema-valid, got %+v", result)
	}
}

func TestPlanTestsTask_MissingRequiredFields(t *testing.T) {
	task := planTestsTask()
	result := task.Validate(`{"root_cause": "", "planned_files": [], "tests": {"success": "", "failure": ""}}`)
	if result.SchemaValid {
		t.Fatalf("expected schema-invalid for all-empty required fields, got %+v", result)
	}
	for _, want := range []string{"root_cause", "planned_files", "tests.success", "tests.failure"} {
		if !strings.Contains(result.Reason, want) {
			t.Errorf("reason %q should mention missing field %q", result.Reason, want)
		}
	}
}

func TestPlanTestsTask_InvalidJSON(t *testing.T) {
	task := planTestsTask()
	result := task.Validate(`{"root_cause": "unterminated`)
	if result.SchemaValid {
		t.Fatalf("expected schema-invalid for malformed JSON, got %+v", result)
	}
}

// sensitivePatterns are a cheap, automated proxy for "no PII in fixtures" --
// not exhaustive, but they catch the shapes AGENTS.md's constraint on
// pii.yaml/.env/applications.db/career_agent.log is actually worried about.
var sensitivePatterns = []*regexp.Regexp{
	regexp.MustCompile(`[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}`), // email address
	regexp.MustCompile(`\b\d{3}-\d{2}-\d{4}\b`),                          // SSN-shaped
	regexp.MustCompile(`\b\d{3}[-.\s]\d{3}[-.\s]\d{4}\b`),                // phone-shaped
}

func TestBuiltinTasksContainNoSensitiveContent(t *testing.T) {
	for _, task := range BuiltinTasks() {
		haystack := task.System + "\n" + task.Prompt + "\n" + task.Description
		for _, pattern := range sensitivePatterns {
			if pattern.MatchString(haystack) {
				t.Errorf("task %q fixture matches sensitive-content pattern %s", task.Name, pattern.String())
			}
		}
	}
}

func TestBuiltinTasks_ReturnsThreeRepresentativeTasks(t *testing.T) {
	tasks := BuiltinTasks()
	if len(tasks) != 3 {
		t.Fatalf("got %d built-in tasks, want 3", len(tasks))
	}
	names := map[string]bool{}
	for _, task := range tasks {
		names[task.Name] = true
		if task.Validate == nil {
			t.Errorf("task %q has no Validate function -- every task must be objectively checkable", task.Name)
		}
	}
	for _, want := range []string{"classify_error", "summarize_excerpt", "plan_tests"} {
		if !names[want] {
			t.Errorf("expected a built-in task named %q", want)
		}
	}
}
