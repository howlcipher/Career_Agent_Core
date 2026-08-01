package modelbench

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// RunOptions configures one benchmark run across one or more models.
type RunOptions struct {
	Host        string
	Tasks       []Task
	Repetitions int
	Timeout     time.Duration
	Temperature float64
}

// CheckModelsAvailable refuses to proceed against any model Ollama does not
// currently report installed, returning an actionable error naming exactly
// which requested model(s) are missing and which models are actually
// available -- this harness never pulls, installs, or otherwise mutates a
// model (see doc.go).
func CheckModelsAvailable(ctx context.Context, host string, requested []string) error {
	installed, err := ListModels(ctx, host)
	if err != nil {
		return fmt.Errorf("could not list installed models: %w", err)
	}
	have := make(map[string]bool, len(installed))
	names := make([]string, 0, len(installed))
	for _, m := range installed {
		have[m.Name] = true
		names = append(names, m.Name)
	}
	var missing []string
	for _, r := range requested {
		if !have[r] {
			missing = append(missing, r)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	return fmt.Errorf(
		"model(s) not installed on this Ollama host: %s. Available: %s. "+
			"Pull a missing model yourself first (e.g. `ollama pull %s`) -- this harness never installs models",
		strings.Join(missing, ", "), strings.Join(names, ", "), missing[0],
	)
}

func taskNames(tasks []Task) []string {
	names := make([]string, len(tasks))
	for i, t := range tasks {
		names[i] = t.Name
	}
	return names
}

// Run benchmarks each of models sequentially against opts.Tasks, never
// running two models concurrently and never starting a task for the next
// model until the previous one's results are recorded (requirement: avoid
// loading multiple heavyweight models at once). Callers must already have
// validated availability via CheckModelsAvailable.
func Run(ctx context.Context, opts RunOptions, models []string) Report {
	rep := Report{
		GeneratedAt: time.Now(),
		Config: Config{
			Host:        opts.Host,
			Models:      models,
			Tasks:       taskNames(opts.Tasks),
			Repetitions: opts.Repetitions,
			Timeout:     opts.Timeout.String(),
			Temperature: opts.Temperature,
		},
	}

	installedSizes := map[string]int64{}
	if installed, err := ListModels(ctx, opts.Host); err == nil {
		for _, m := range installed {
			installedSizes[m.Name] = m.Size
		}
	}

	for _, model := range models {
		mr := ModelReport{Model: model, SizeBytes: installedSizes[model]}

		memBefore := TakeHostSnapshot()
		mr.MemBefore = &memBefore

		// Best-effort: force an unload so the first real call below is a
		// genuine cold start. A failure here (model already unloaded, or the
		// unload call itself erroring) does not abort the run -- it just
		// means the first call might already be warm, which the recorded
		// LoadDurationMS lets a reader confirm independently.
		_ = Unload(ctx, opts.Host, model)

		first := true
		for _, task := range opts.Tasks {
			for repetition := 1; repetition <= opts.Repetitions; repetition++ {
				result := runOne(ctx, opts, model, task, repetition, first)
				first = false
				mr.Results = append(mr.Results, result)
			}
		}

		memAfter := TakeHostSnapshot()
		mr.MemAfter = &memAfter
		if running, err := ListRunning(ctx, opts.Host); err == nil {
			for _, r := range running {
				if r.Name == model {
					mr.ResidentAfter = true
				}
			}
		}

		rep.Models = append(rep.Models, mr)
	}

	return rep
}

func runOne(ctx context.Context, opts RunOptions, model string, task Task, repetition int, cold bool) TaskResult {
	callCtx, cancel := context.WithTimeout(ctx, opts.Timeout)
	defer cancel()

	result := TaskResult{
		Task:       task.Name,
		Repetition: repetition,
		ColdStart:  cold,
		Timestamp:  time.Now(),
	}

	genResult, err := Generate(callCtx, opts.Host, model, GenerateOptions{
		System:      task.System,
		Prompt:      task.Prompt,
		JSONFormat:  task.JSONFormat,
		Temperature: opts.Temperature,
	})
	result.WallDurationMS = genResult.WallDuration.Milliseconds()
	if err != nil {
		if callCtx.Err() == context.DeadlineExceeded {
			result.TimedOut = true
		}
		result.Error = err.Error()
		return result
	}

	result.TotalDurationMS = genResult.TotalDurationNS / int64(time.Millisecond)
	result.LoadDurationMS = genResult.LoadDurationNS / int64(time.Millisecond)
	result.PromptEvalCount = genResult.PromptEvalCount
	result.PromptTokensPerSec = genResult.PromptTokensPerSec()
	result.EvalCount = genResult.EvalCount
	result.GenTokensPerSec = genResult.GenTokensPerSec()
	result.OutputBytes = len(genResult.Content)

	validation := task.Validate(genResult.Content)
	result.SchemaValid = validation.SchemaValid
	result.Correct = validation.Correct
	result.ValidationReason = validation.Reason
	return result
}
