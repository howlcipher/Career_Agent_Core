// Command modelbench measures how the local Ollama models installed on this
// host perform on a small set of bounded, objectively-validated task classes
// (structured classification, bounded summarization, structured
// implementation/test planning), so a model-routing decision can be made
// from evidence instead of assuming the largest model is always the right
// one.
//
// It is read-only against Ollama and the host: it never pulls, deletes, or
// otherwise mutates a model. See internal/modelbench for the measurement
// logic and its own tests, which run against a mocked Ollama server and need
// no live model. This command's own execution against a real model is a
// separate, manual verification step -- see documentation/model_benchmark.md.
//
// Usage:
//
//	# see what's installed
//	go run ./cmd/modelbench -list
//
//	# benchmark one model on the built-in task set, 2 repetitions each
//	go run ./cmd/modelbench -models qwen3:4b-instruct
//
//	# compare two models, more repetitions, write the JSON report to a file
//	go run ./cmd/modelbench -models qwen3:4b-instruct,qwen3:30b-instruct \
//	    -reps 3 -out benchmark_results/run.json
//
// Run this during a controlled idle window: it deliberately refuses to start
// while the production agent (cmd/agent) holds its single-instance lock,
// since benchmarking unloads and reloads models on the same Ollama instance
// the agent depends on. Pass -force to override that check if you understand
// the risk.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/howlcipher/Career_Agent_Core/internal/modelbench"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr *os.File) int {
	fs := flag.NewFlagSet("modelbench", flag.ContinueOnError)
	fs.SetOutput(stderr)

	host := fs.String("host", modelbench.DefaultHost, "Ollama base URL")
	modelsFlag := fs.String("models", "", "comma-separated model names to benchmark (required unless -list)")
	tasksFlag := fs.String("tasks", "all", "comma-separated task names, or \"all\"")
	reps := fs.Int("reps", 2, "repetitions per (model, task) pair")
	timeout := fs.Duration("timeout", 5*time.Minute, "per-call timeout")
	temperature := fs.Float64("temperature", 0, "sampling temperature (0 = deterministic, the default and recommended value)")
	out := fs.String("out", "", "write the JSON report to this path (default: print to stdout only, nothing written to disk)")
	list := fs.Bool("list", false, "list installed Ollama models and exit")
	force := fs.Bool("force", false, "run even if the production agent's single-instance lock is currently held")
	lockPath := fs.String("lock-path", modelbench.AgentLockPath, "path to the agent's single-instance lock file, for the idle check")

	if err := fs.Parse(args); err != nil {
		return 2
	}

	ctx := context.Background()

	if *list {
		installed, err := modelbench.ListModels(ctx, *host)
		if err != nil {
			fmt.Fprintf(stderr, "modelbench: %v\n", err)
			return 1
		}
		if len(installed) == 0 {
			fmt.Fprintln(stdout, "No models installed on this Ollama host.")
			return 0
		}
		for _, m := range installed {
			fmt.Fprintf(stdout, "%-30s %6.1f GB  %s\n", m.Name, float64(m.Size)/1e9, m.ParameterSize)
		}
		return 0
	}

	if strings.TrimSpace(*modelsFlag) == "" {
		fmt.Fprintln(stderr, "modelbench: -models is required (or pass -list to see what's installed)")
		return 2
	}
	models := splitAndTrim(*modelsFlag)

	if !*force {
		running, pid, err := modelbench.IsAgentRunning(*lockPath)
		if err != nil {
			fmt.Fprintf(stderr, "modelbench: could not check whether the agent is running: %v\n", err)
			return 1
		}
		if running {
			fmt.Fprintf(stderr,
				"modelbench: the production agent appears to be running (lock held, pid %d). "+
					"Benchmarking unloads and reloads models on the same Ollama instance it depends on, "+
					"so this refuses to run during a live attempt. Wait for an idle window, or pass -force "+
					"if you have confirmed it is safe.\n", pid)
			return 1
		}
	}

	if err := modelbench.CheckModelsAvailable(ctx, *host, models); err != nil {
		fmt.Fprintf(stderr, "modelbench: %v\n", err)
		return 1
	}

	tasks, err := resolveTasks(*tasksFlag)
	if err != nil {
		fmt.Fprintf(stderr, "modelbench: %v\n", err)
		return 2
	}

	if *reps < 1 {
		fmt.Fprintln(stderr, "modelbench: -reps must be at least 1")
		return 2
	}

	opts := modelbench.RunOptions{
		Host:        *host,
		Tasks:       tasks,
		Repetitions: *reps,
		Timeout:     *timeout,
		Temperature: *temperature,
	}

	report := modelbench.Run(ctx, opts, models)

	fmt.Fprint(stdout, report.Summary())

	raw, err := report.JSON()
	if err != nil {
		fmt.Fprintf(stderr, "modelbench: could not render JSON report: %v\n", err)
		return 1
	}

	if *out != "" {
		if err := os.WriteFile(*out, raw, 0644); err != nil {
			fmt.Fprintf(stderr, "modelbench: could not write report to %s: %v\n", *out, err)
			return 1
		}
		fmt.Fprintf(stdout, "\nJSON report written to %s\n", *out)
	} else {
		fmt.Fprintln(stdout, "\n--- JSON report ---")
		fmt.Fprintln(stdout, string(raw))
	}

	if !report.AllPassed() {
		fmt.Fprintln(stderr, "\nmodelbench: one or more required task results failed, timed out, or violated their expected schema")
		return 1
	}
	return 0
}

func splitAndTrim(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func resolveTasks(spec string) ([]modelbench.Task, error) {
	all := modelbench.BuiltinTasks()
	if strings.TrimSpace(spec) == "" || strings.EqualFold(strings.TrimSpace(spec), "all") {
		return all, nil
	}
	byName := make(map[string]modelbench.Task, len(all))
	var names []string
	for _, t := range all {
		byName[t.Name] = t
		names = append(names, t.Name)
	}
	sort.Strings(names)

	var selected []modelbench.Task
	var unknown []string
	for _, want := range splitAndTrim(spec) {
		t, ok := byName[want]
		if !ok {
			unknown = append(unknown, want)
			continue
		}
		selected = append(selected, t)
	}
	if len(unknown) > 0 {
		return nil, fmt.Errorf("unknown task(s): %s. Known tasks: %s", strings.Join(unknown, ", "), strings.Join(names, ", "))
	}
	return selected, nil
}
