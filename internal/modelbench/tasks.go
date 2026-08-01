package modelbench

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ValidationResult is a task's objective, mechanical judgment of one model
// response. SchemaValid is the pass/fail signal a benchmark run's exit code
// depends on (required fields present, enum values in range, output bounded).
// Correct is a separate, informational axis: whether the response also
// matched the fixture's known-good answer. A schema-valid-but-incorrect
// response is not a harness failure -- it is evidence that this model is the
// wrong one to route this task to, which is the whole point of measuring it.
type ValidationResult struct {
	SchemaValid bool
	Correct     bool
	Reason      string
}

// Task is one bounded, objectively-validated unit of work a model can be
// benchmarked against. Every built-in Task's fixture is synthetic and
// committed here -- no resumes, application text, real error logs, or other
// production/personal data (see TestBuiltinTasksContainNoSensitiveContent).
type Task struct {
	Name           string
	Description    string
	System         string
	Prompt         string
	JSONFormat     bool
	MaxOutputBytes int
	Validate       func(output string) ValidationResult
}

// BuiltinTasks returns the representative task set: structured
// classification, bounded summarization, and structured implementation/test
// planning, per the harness's own scope (vision and embeddings are
// deliberately out of scope -- see cmd/modelbench's usage doc).
func BuiltinTasks() []Task {
	return []Task{classifyErrorTask(), summarizeExcerptTask(), planTestsTask()}
}

var classifyEnum = map[string]bool{
	"network": true, "timeout": true, "database": true,
	"parsing": true, "auth": true, "unknown": true,
}

func classifyErrorTask() Task {
	// Synthetic, sanitized error string shaped like a real daemon failure
	// (bugs.md #478's DNS class) but written for this fixture, not copied
	// from a live log.
	const errorMessage = `dial tcp: lookup badhost.invalid on 127.0.0.53:53: no such host`
	const maxBytes = 500
	return Task{
		Name:        "classify_error",
		Description: "Classify a sanitized daemon error into a fixed category.",
		System: "You are a log-classification assistant for a background job daemon. " +
			"Classify the given sanitized error message into exactly one category. " +
			`Reply with strict JSON only, no prose, matching this schema: ` +
			`{"category": "<network|timeout|database|parsing|auth|unknown>", "confidence": <number 0 to 1>}.`,
		Prompt:         fmt.Sprintf("Error message: %q", errorMessage),
		JSONFormat:     true,
		MaxOutputBytes: maxBytes,
		Validate: func(output string) ValidationResult {
			if len(output) > maxBytes {
				return ValidationResult{Reason: fmt.Sprintf("output %d bytes exceeds %d-byte cap", len(output), maxBytes)}
			}
			var parsed struct {
				Category   string  `json:"category"`
				Confidence float64 `json:"confidence"`
			}
			if err := json.Unmarshal([]byte(output), &parsed); err != nil {
				return ValidationResult{Reason: fmt.Sprintf("invalid JSON: %v", err)}
			}
			category := strings.ToLower(strings.TrimSpace(parsed.Category))
			if !classifyEnum[category] {
				return ValidationResult{Reason: fmt.Sprintf("category %q is not one of the allowed enum values", parsed.Category)}
			}
			if parsed.Confidence < 0 || parsed.Confidence > 1 {
				return ValidationResult{Reason: fmt.Sprintf("confidence %v out of range [0,1]", parsed.Confidence)}
			}
			return ValidationResult{SchemaValid: true, Correct: category == "network"}
		},
	}
}

func summarizeExcerptTask() Task {
	// Fully fabricated Go function, written for this fixture only -- it does
	// not exist anywhere in this repository's real source.
	const excerpt = `func ComputeRetryDelay(attempt int, base time.Duration) time.Duration {
	if attempt < 0 {
		attempt = 0
	}
	delay := base * time.Duration(1<<uint(attempt))
	jitter := time.Duration(rand.Int63n(int64(delay) / 4))
	return delay + jitter
}`
	requiredKeywords := []string{"ComputeRetryDelay", "jitter"}
	requiredAny := []string{"backoff", "exponential"}
	const maxBytes = 900
	return Task{
		Name:        "summarize_excerpt",
		Description: "Summarize a bounded, synthetic source-code excerpt.",
		System: "You are a code-review assistant. Summarize what the given function does in two sentences " +
			"or fewer, at most 50 words. Mention the function's name and the strategy it implements. " +
			"Reply with plain text only, no code, no markdown.",
		Prompt:         fmt.Sprintf("```go\n%s\n```", excerpt),
		JSONFormat:     false,
		MaxOutputBytes: maxBytes,
		Validate: func(output string) ValidationResult {
			if len(output) > maxBytes {
				return ValidationResult{Reason: fmt.Sprintf("output %d bytes exceeds %d-byte cap", len(output), maxBytes)}
			}
			lower := strings.ToLower(output)
			var missing []string
			for _, kw := range requiredKeywords {
				if !strings.Contains(lower, strings.ToLower(kw)) {
					missing = append(missing, kw)
				}
			}
			anyFound := false
			for _, kw := range requiredAny {
				if strings.Contains(lower, kw) {
					anyFound = true
					break
				}
			}
			if !anyFound {
				missing = append(missing, "backoff|exponential")
			}
			if len(missing) > 0 {
				return ValidationResult{Reason: fmt.Sprintf("missing required keyword(s): %s", strings.Join(missing, ", "))}
			}
			return ValidationResult{SchemaValid: true, Correct: true}
		},
	}
}

func planTestsTask() Task {
	// Synthetic bug description shaped like a real backlog row's Details
	// section, but invented for this fixture rather than quoting one.
	const bugDescription = "NormalizeURL does not lowercase the hostname before deduplication, " +
		"so http://Example.com/a and http://example.com/a are treated as different URLs and both get queued."
	const maxBytes = 2000
	return Task{
		Name:        "plan_tests",
		Description: "Produce a structured implementation and test plan for a synthetic bug.",
		System: "You are a senior Go engineer triaging a bug report. Reply with strict JSON only, no prose, " +
			`matching this schema: {"root_cause": "<string>", "planned_files": ["<string>", ...], ` +
			`"tests": {"success": "<string>", "failure": "<string>"}}. ` +
			`planned_files must name at least one file. Both test fields must be non-empty.`,
		Prompt:         fmt.Sprintf("Bug report: %s", bugDescription),
		JSONFormat:     true,
		MaxOutputBytes: maxBytes,
		Validate: func(output string) ValidationResult {
			if len(output) > maxBytes {
				return ValidationResult{Reason: fmt.Sprintf("output %d bytes exceeds %d-byte cap", len(output), maxBytes)}
			}
			var parsed struct {
				RootCause    string   `json:"root_cause"`
				PlannedFiles []string `json:"planned_files"`
				Tests        struct {
					Success string `json:"success"`
					Failure string `json:"failure"`
				} `json:"tests"`
			}
			if err := json.Unmarshal([]byte(output), &parsed); err != nil {
				return ValidationResult{Reason: fmt.Sprintf("invalid JSON: %v", err)}
			}
			var missing []string
			if strings.TrimSpace(parsed.RootCause) == "" {
				missing = append(missing, "root_cause")
			}
			if len(parsed.PlannedFiles) == 0 {
				missing = append(missing, "planned_files (empty)")
			}
			if strings.TrimSpace(parsed.Tests.Success) == "" {
				missing = append(missing, "tests.success")
			}
			if strings.TrimSpace(parsed.Tests.Failure) == "" {
				missing = append(missing, "tests.failure")
			}
			if len(missing) > 0 {
				return ValidationResult{Reason: fmt.Sprintf("missing/empty required field(s): %s", strings.Join(missing, ", "))}
			}
			return ValidationResult{SchemaValid: true, Correct: true}
		},
	}
}
