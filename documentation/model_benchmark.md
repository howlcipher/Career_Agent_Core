# Local-model benchmark and routing-evidence harness

`cmd/modelbench` measures how the Ollama models installed on this laptop actually perform on a
small set of bounded, objectively-validated task classes, so a model-routing decision (which
model handles which kind of work) can be made from evidence instead of assuming the largest
model is automatically the best one. It never installs, pulls, deletes, or otherwise mutates a
model — it only calls Ollama's existing HTTP API and reads `/proc/meminfo`.

## Why this exists

This laptop (Ryzen 5 5600U, 32 GB RAM, integrated Vega graphics, CPU-only Ollama) runs Career
Agent Core, Chromium/Playwright, SQLite, and local inference all on one machine, alongside
whatever the user is doing with it directly. "Bigger model" and "better for this task" are not
the same claim on hardware this constrained: `qwen3:30b-instruct` (the largest installed text
model) costs far more wall time and memory per call than `qwen3:4b-instruct`, and several of the
task classes this repo's own automation might eventually delegate to a local model (classifying
a sanitized error, drafting a small structured plan) may not need the larger model's quality at
all. Nothing in the repository had ever measured that trade-off; this harness is the measurement
infrastructure, not a routing policy by itself — see "Routing hypothesis" below for the
distinction.

## Listing installed models

```bash
go run ./cmd/modelbench -list
```

Prints every model Ollama currently reports (name, size, parameter count) and exits. This is the
same data `curl localhost:11434/api/tags` returns; the flag exists so a session doesn't need to
parse raw JSON by hand.

## Running a benchmark

```bash
go run ./cmd/modelbench -models qwen3:4b-instruct -reps 2
```

- `-models` (required unless `-list`): comma-separated model names. The harness refuses to
  benchmark a model Ollama doesn't report installed, with an actionable error naming what *is*
  available — it never tries to pull one.
- `-tasks`: comma-separated task names, or `all` (the default). See "Task set" below.
- `-reps`: repetitions per (model, task) pair. The first call to a given model in the run is
  always the cold-start measurement (see below); every repetition after that is warm. 2-3 is
  usually enough to see whether a model's timing is stable.
- `-timeout`: per-call timeout (default 5m). A call that exceeds it is recorded as `TimedOut`,
  not silently dropped.
- `-temperature`: sampling temperature (default 0, the nearest deterministic setting Ollama
  supports, and the recommended value — this is a routing measurement, not a creativity test).
- `-out`: write the JSON report to this path. Omit it and the report only prints to stdout —
  **nothing is written to disk by default**, and `benchmark_results/` (a natural place to redirect
  it) is gitignored, so a benchmark run never accidentally becomes a commit.
- `-force`: bypass the idle-window safety check (see below). Only pass this if you have confirmed
  it is actually safe to compete with whatever else is using Ollama right now.

Comparing models:

```bash
go run ./cmd/modelbench -models qwen3:4b-instruct,qwen3:30b-instruct -reps 3 \
    -out benchmark_results/$(date +%Y%m%d-%H%M)-compare.json
```

Models are always benchmarked **sequentially, one at a time** — the harness never loads two
models concurrently, so it can't itself create the memory-pressure problem it exists partly to
help reason about (see Candidate B in `improvements.md`, not yet built).

## Avoiding disruption to Career Agent

Before doing anything else, the harness checks whether `cmd/agent`'s single-instance lock
(`applications/career_agent.lock`) is currently held — the same lock `cmd/dashboard` reads to
report agent status. If the production agent is running, `modelbench` refuses to start:
benchmarking unloads and reloads models on the same Ollama instance the agent depends on, and
racing it for the CPU and the one model Ollama keeps resident (`OLLAMA_MAX_LOADED_MODELS=1`)
would slow down or destabilize a real, in-flight application attempt. Wait for an idle window, or
pass `-force` only once you have confirmed nothing important is running.

**Run benchmarks during a controlled idle window** — not while the daemon is mid-cycle, not
while you're relying on the dashboard/tracker for something time-sensitive.

## Task set

Three representative, objectively-validated task classes ship built in (`-tasks all`, the
default). Every fixture is synthetic and committed in `internal/modelbench/tasks.go` — none of
them are real error logs, real code, or real backlog content.

| Task | What it asks | How it's validated |
| --- | --- | --- |
| `classify_error` | Classify a sanitized, synthetic daemon error string into one of a fixed six-value enum, as JSON with a confidence field | Valid JSON, category in the allowed enum, confidence in [0,1], output ≤ 500 bytes. `Correct` (informational, does not fail the run) checks the fixture's known right answer |
| `summarize_excerpt` | Summarize a small, fabricated Go function in ≤ 50 words, plain text | Required keywords present (function name, "jitter", "backoff" or "exponential"), output ≤ 900 bytes |
| `plan_tests` | Produce a structured root-cause/file/test plan for a synthetic bug description, as JSON | Required fields present and non-empty (`root_cause`, `planned_files`, `tests.success`, `tests.failure`), output ≤ 2000 bytes |

Every task's schema check is mechanical — there is no "looks good" step. A run's exit code is
nonzero if any invoked task, on any repetition, fails schema validation, times out, or errors.

**Deliberately out of scope for this harness**, recorded here rather than silently dropped:

- **Vision benchmarking** — the installed vision model (`qwen2.5vl:7b`) answers a structurally
  different kind of task (image + prompt → text) than the three text tasks above, and folding it
  in would have doubled this item's scope. Left as a documented follow-up rather than built now.
- **Embeddings** — `nomic-embed-text` doesn't generate text at all, so none of this harness's
  validation model (schema/enum/keyword checks against a chat completion) applies. A useful
  embedding benchmark would measure retrieval quality (e.g. nearest-neighbor accuracy on a known
  set), which is a different measurement design, not a fourth task on this harness.
- **A concrete 12B-16B "medium" model** — the installed inventory jumps from 4B to 30B with
  nothing between. This harness can benchmark a mid-size model the moment the user installs one
  and names it on the command line (`-models <name>`); it does not itself install, recommend, or
  hardcode one, since any specific ID named today would likely be stale by the time someone reads
  this.

## Interpreting cold vs. warm timing

The harness explicitly unloads a model (`keep_alive: 0`) before its first call in a run, so that
first call is a genuine cold start — Ollama has to read the model off disk into memory before it
can answer. Every repetition after that, for the same model, is warm: the model is already
resident (subject to Ollama's own `keep_alive` eviction, default 5 minutes, which a short
benchmark run won't hit). The JSON report's `load_duration_ms` field is Ollama's own reported load
time per call, so you can cross-check the cold/warm label against it directly rather than trusting
call order alone — a call labeled "warm" with a large `load_duration_ms` would mean something
else evicted the model in between (e.g. `OLLAMA_MAX_LOADED_MODELS=1` swapping in a different
model), which is itself useful information about how routing.

Cold-start cost matters most for a task that runs in isolation (a one-off classification call);
warm-run cost matters most for a task that runs repeatedly in a batch (this repo's own document
generation, which already keeps a request stream going).

## Interpreting a schema failure

A `schema_valid: false` result means the model's raw output didn't parse into the task's required
structure — invalid JSON, a missing field, an out-of-enum value, or an output that blew past the
task's byte cap. This is evidence the model (at temperature 0, on this exact prompt) is not
currently a safe choice to route this task class to unsupervised. It is not evidence the harness
is broken; a model that reliably fails a task's schema is exactly the routing signal this exists
to surface. `correct: false` on an otherwise schema-valid result is a *softer* signal — the model
answered in the right shape but got the substantive answer wrong (e.g. classified a network error
as `auth`) — worth weighing but not, by itself, disqualifying the way a schema failure is.

## Where results go

Nothing is committed by default. `-out` writes to whatever path you give it; `benchmark_results/`
is gitignored so redirecting output there (the convention this doc uses in its own examples)
never risks landing in a commit by accident.

## Routing hypothesis (not yet final policy)

This is a **hypothesis awaiting representative benchmark results**, not a conclusion one short run
establishes:

- The smallest installed text model (`qwen3:4b-instruct`) for frequent, low-stakes classification,
  extraction, routing, and summarization.
- The largest installed text model (`qwen3:30b-instruct`) only for bounded tasks where a benchmark
  run actually shows a quality improvement worth its wall-time and memory cost — not by default.
- The vision model (`qwen2.5vl:7b`) only for visual input; the embedding model
  (`nomic-embed-text`) only for retrieval — neither is a general-purpose text worker.
- Claude stays responsible for architecture, security, concurrency, ambiguous reasoning, final
  patch review, verification, commit, and push — this harness does not change that, and nothing
  in `improvements.md`'s Candidate C (safe local-model delegation harness, not yet built) proposes
  otherwise.

Run a real comparison (`-models qwen3:4b-instruct,qwen3:30b-instruct -reps 3+`) before treating
any of the above as settled for a specific task.

## Related backlog items

- `improvements.md` **#484** — this harness (Done).
- `improvements.md` **#442** — measures a different question (in-process vs. offloaded HTTP call
  to the optional `nlp_service`, both against the same Ollama). This harness's timing-capture
  approach is reusable if someone extends `scripts/verify_tailoring.go` with the same JSON-report
  shape, but #484 does not itself answer #442's question.
- `improvements.md` **#485-#488** — resource-aware admission control, a safe delegation contract,
  lightweight log triage, and an OpenClaw sidecar evaluation, all Pending and none built this
  session; see their own Details sections.
