package modelbench

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// Config records exactly how a benchmark run was invoked, so a report is
// self-describing without needing the command line that produced it.
type Config struct {
	Host        string   `json:"host"`
	Models      []string `json:"models"`
	Tasks       []string `json:"tasks"`
	Repetitions int      `json:"repetitions"`
	Timeout     string   `json:"timeout"`
	Temperature float64  `json:"temperature"`
}

// TaskResult is one (model, task, repetition) measurement.
type TaskResult struct {
	Task               string    `json:"task"`
	Repetition         int       `json:"repetition"`
	ColdStart          bool      `json:"cold_start"`
	Timestamp          time.Time `json:"timestamp"`
	WallDurationMS     int64     `json:"wall_duration_ms"`
	TotalDurationMS    int64     `json:"total_duration_ms"`
	LoadDurationMS     int64     `json:"load_duration_ms"`
	PromptEvalCount    int       `json:"prompt_eval_count"`
	PromptTokensPerSec float64   `json:"prompt_tokens_per_sec"`
	EvalCount          int       `json:"eval_count"`
	GenTokensPerSec    float64   `json:"gen_tokens_per_sec"`
	OutputBytes        int       `json:"output_bytes"`
	SchemaValid        bool      `json:"schema_valid"`
	Correct            bool      `json:"correct"`
	ValidationReason   string    `json:"validation_reason,omitempty"`
	TimedOut           bool      `json:"timed_out"`
	Error              string    `json:"error,omitempty"`
}

// Passed is the mechanical, required-task pass/fail signal: a genuine error,
// a timeout, or a schema violation all fail it. Correct is a separate,
// informational axis that deliberately does not affect Passed -- a small
// model answering schema-valid but wrong is a routing data point, not a
// harness defect (see tasks.go's ValidationResult doc).
func (r TaskResult) Passed() bool {
	return r.Error == "" && !r.TimedOut && r.SchemaValid
}

// ModelReport aggregates every TaskResult for one model, plus host snapshots
// taken immediately before its first call and after its last.
type ModelReport struct {
	Model         string        `json:"model"`
	SizeBytes     int64         `json:"size_bytes,omitempty"`
	MemBefore     *HostSnapshot `json:"mem_before,omitempty"`
	MemAfter      *HostSnapshot `json:"mem_after,omitempty"`
	ResidentAfter bool          `json:"resident_after"`
	Results       []TaskResult  `json:"results"`
}

// Report is the full machine-readable output of one benchmark run.
type Report struct {
	GeneratedAt time.Time     `json:"generated_at"`
	Config      Config        `json:"config"`
	Models      []ModelReport `json:"models"`
}

// AllPassed reports whether every result across every model Passed(). This is
// what cmd/modelbench's exit code is based on.
func (rep Report) AllPassed() bool {
	for _, m := range rep.Models {
		for _, r := range m.Results {
			if !r.Passed() {
				return false
			}
		}
	}
	return true
}

// JSON renders the report as indented JSON, the machine-readable output
// format requirement #13 asks for.
func (rep Report) JSON() ([]byte, error) {
	return json.MarshalIndent(rep, "", "  ")
}

// Summary renders a concise, human-readable account of the run: one line per
// model/task pair plus a final pass/fail tally. It exists so a session that
// just ran a benchmark doesn't have to read raw JSON to know what happened.
func (rep Report) Summary() string {
	var b strings.Builder
	fmt.Fprintf(&b, "Model benchmark — %s (host %s)\n", rep.GeneratedAt.Format(time.RFC3339), rep.Config.Host)
	fmt.Fprintf(&b, "Tasks: %s | Repetitions: %d | Timeout: %s | Temperature: %v\n\n",
		strings.Join(rep.Config.Tasks, ", "), rep.Config.Repetitions, rep.Config.Timeout, rep.Config.Temperature)

	passed, failed := 0, 0
	for _, m := range rep.Models {
		fmt.Fprintf(&b, "== %s ==\n", m.Model)
		if m.SizeBytes > 0 {
			fmt.Fprintf(&b, "  size: %.1f GB\n", float64(m.SizeBytes)/1e9)
		}
		if avail := kbField(m.MemBefore, m.MemAfter); avail != "" {
			fmt.Fprintf(&b, "  mem available: %s\n", avail)
		}
		byTask := map[string][]TaskResult{}
		var order []string
		for _, r := range m.Results {
			if _, ok := byTask[r.Task]; !ok {
				order = append(order, r.Task)
			}
			byTask[r.Task] = append(byTask[r.Task], r)
		}
		for _, taskName := range order {
			results := byTask[taskName]
			var wallTotal time.Duration
			okCount, correctCount := 0, 0
			for _, r := range results {
				wallTotal += time.Duration(r.WallDurationMS) * time.Millisecond
				if r.Passed() {
					okCount++
					passed++
				} else {
					failed++
				}
				if r.Correct {
					correctCount++
				}
			}
			avgWall := wallTotal / time.Duration(len(results))
			label := "cold+warm"
			if len(results) > 0 && results[0].ColdStart {
				label = fmt.Sprintf("cold %dms, then warm", results[0].WallDurationMS)
			}
			fmt.Fprintf(&b, "  %-18s %d/%d passed, %d/%d correct, avg wall %s (%s)\n",
				taskName, okCount, len(results), correctCount, len(results), avgWall.Round(time.Millisecond), label)
			for _, r := range results {
				if !r.Passed() {
					reason := r.ValidationReason
					if r.Error != "" {
						reason = r.Error
					}
					if r.TimedOut {
						reason = "timed out"
					}
					fmt.Fprintf(&b, "    rep %d FAILED: %s\n", r.Repetition, reason)
				}
			}
		}
		b.WriteString("\n")
	}
	fmt.Fprintf(&b, "TOTAL: %d passed, %d failed\n", passed, failed)
	return b.String()
}

func kbField(before, after *HostSnapshot) string {
	if before == nil || before.MemAvailableKB == nil {
		return ""
	}
	beforeGB := float64(*before.MemAvailableKB) / 1e6
	if after == nil || after.MemAvailableKB == nil {
		return fmt.Sprintf("%.1f GB before", beforeGB)
	}
	afterGB := float64(*after.MemAvailableKB) / 1e6
	return fmt.Sprintf("%.1f GB before -> %.1f GB after", beforeGB, afterGB)
}
