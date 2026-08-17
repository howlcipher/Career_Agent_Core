# improvements.md — Archived Full Accounts for Closed (Done/Resolved/Closed) Items

Full accounts for closed improvement rows, moved out of `improvements.md`'s ranked-table rationale cells and `### N.` Details sections during the 2026-08-01 backlog-size restructure. `improvements.md` keeps only a one-line pointer for each closed item; this file has the full account for audit purposes.

## 472. Extend bug #467's target-closed browser recovery to the security-code resubmit click

**Completed 2026-08-02.** The emailed security-code resubmit click now classifies Playwright target-closed errors and passes them to the existing shared recovery block. That block releases the crashed page/context, recreates the browser context once, and repeats the normal form flow without repeating document generation. Ordinary click failures still retain the established `ErrNeedsEmailVerification` handoff, while a second crash remains bounded and surfaces as an error instead of retrying indefinitely.

Focused regression coverage guards the resubmit-to-recovery wiring and the existing post-click confirmation invariant. `go build ./...`, `go vet ./...`, `go test ./...`, `gofmt -l ./cmd ./pkg ./internal`, and `git diff --check` passed in the closing verification. No production browser, mailbox, or application data was accessed.

---

## 487. Lightweight 4B log triage and context compression

**Completed 2026-08-02.** `cmd/logtriage` is an explicit opt-in, stdin-to-stdout, read-only utility backed by `internal/logtriage`. It applies deterministic email, phone, credential-shaped-value, and URL-query redaction before retaining any line or making an optional local-model request. It bounds input to 100 events and 500 bytes per event, caps model context/output and the call deadline, groups repeated deterministic failure classes, and emits a compact JSON packet with confidence and model/fallback provenance. Invalid, oversized, timed-out, or unavailable model responses return the deterministic packet rather than untrusted partial output.

The command has no database, application, browser, email, Git, filesystem-write, or submission integration. It therefore yields completely to application-critical work and cannot mutate user state. Existing individual deterministic classifiers remain unchanged; this fills the distinct cross-event context-compression gap rather than replacing established safety paths.

Live prerequisite benchmark: `qwen3:4b-instruct` passed `classify_error` 2/2 schema-valid and correct at temperature zero (20.9s cold, 2.4s warm; no swap consumed). A synthetic two-event optional-model smoke test returned validated summary JSON with confidence 0.98, and a sensitive-pattern deterministic smoke test showed email, phone, token query, and source text redacted before output. Focused tests cover redaction, grouping, and the no-model fallback; the full Go verification loop passed in the closing commit.

---

## 492. Explicit first-attempt SLA and bounded fresh-queue admission

**Completed 2026-08-02.** The fresh queue now uses an explicit, bounded policy without changing the fit-ranking formula: jobs receive the existing urgency treatment at seven days, each agent queue cycle terminalizes `DISCOVERED` rows that have not received a first attempt by 30 days, and each identified discovery source is limited to 25 pending rows. Over-cap rows remain auditable as `SKIPPED` with `source_pending_cap`; expired rows are `SKIPPED` with `first_attempt_sla_expired`. The sweep also retains excluded-source terminalization, so excluded rows cannot accumulate indefinitely.

Storage and agent-cycle tests cover the 0/1/7/14/30-day synthetic age set, source admission at and above the cap, and the scheduled sweep. Full Go and dashboard verification passed. The prior real-data baseline (4.8-day p50 and 11.7-day p90 from discovery to first attempt) is not claimed to have improved yet: it needs a future read-only measurement after normal, user-controlled operation creates new attempts.

---

## 510. Record discovery-source request failures and circuit-open skips in refresh health

**Completed 2026-08-02.** Yahoo discovery now records its outbound request attempts, final request failures after retries, and queries skipped while its circuit breaker is open. These privacy-safe numeric counters persist with the existing source insertion outcomes in `discovery_refresh.source_counts_json`; no posting URL, title, company, request text, or raw provider error is retained. The dashboard API and React health message surface the failure and skip totals so an empty queue can be distinguished from a healthy no-results refresh.

Regression tests cover a retry-exhausted request, a circuit-open skip with no request, aggregate persistence, and dashboard API decoding. Dashboard reads remain backward compatible with a pre-migration refresh table. Focused Go tests and the dashboard Vitest suite passed before release.

---

# 511. Assisted Apply Queue with resumable human handoff and legacy-job backfill

**Completed 2026-08-02.** Added private, resumable assisted plans for existing and new `AWAITING_REVIEW`, `MANUAL_REQUIRED`, and `BLOCKED_CAPTCHA` applications. The normal loopback dashboard now exposes a visible Assisted Apply queue with plain-language next actions, a five-step progress panel, validated job documents, lease-aware Continue, explicit employer-confirmation dialog, and sequential selected-job workflow. The browser command uses an exclusive SQLite lease, dedicated private persistent profile, guarded proxy/network boundary, and stable job ID only. After a human gate, it re-inspects the same visible page and reuses only healthy mappings and validated documents to refill deterministic fields, never submitting or inferring restricted answers.

Legacy migration is dry-run by default, requires `-confirm`, reports aggregate exclusions, is transaction-backed and idempotent, and leaves statuses/history/dedup intact. Production migration was performed under a paused agent after an owner-only verified ignored backup: 493 eligible plans imported, then a second run reported 493 already queued and zero imports. The original agent process was restored. Expired and below-threshold candidates are excluded; every dashboard mutation is same-origin, method checked, stable-ID based, and argument-safe. Document serving rejects traversal and symlink escape and is private/no-store. Manual confirmation records `manual_user_confirmation` in the same transaction as canonical APPLIED/dedup state.

Verification passed: UI tests/build; storage, dashboard, agent, submitter, and full `go build ./...`, `go vet ./...`, `go test ./...`, `gofmt -l ./cmd ./pkg ./internal`. The task journal is removed in the closing commit.

---

## 479. A permanent DNS failure spends the full retry/backoff budget instead of failing fast to a terminal status

**Completed 2026-08-02.** `StateInit` now recognizes a wrapped `*net.DNSError` whose `IsNotFound` flag is true and terminalizes that URL as `RETRY_EXHAUSTED` with the normalized `dns_not_found` reason. This deliberately reuses the existing terminal status and dashboard/requeue treatment rather than adding a narrow new status. It bypasses retry accounting and backoff because the resolver has authoritatively reported that the hostname does not exist. A temporary DNS resolver error remains on the existing retryable path.

Pipeline tests prove both branches: authoritative name-not-found is terminal after its first attempt, while temporary DNS failure records one retry with a future eligibility time. Focused Go tests and the dashboard Vitest suite passed before release.

---

## 442. Measure whether the NLP offload is worth keeping

**Completed 2026-08-02.** Matched live synthetic tailoring runs used the same local `qwen3:4b-instruct` model and Ollama endpoint. In-process completed in 7m6s with 422,864 KB maximum verifier memory; healthy localhost `nlp_service` completed in 5m18s with 42,732 KB. Both succeeded without fallback. That 25% time reduction and about 90% lower waiting-process memory corroborate the earlier 2026-07-29 observation, so the opt-in service is retained and its tradeoff is documented in README and CHANGELOG.

---

## 448. `npm run lint` lints the dashboard's own committed build output

**Completed 2026-08-02.** The committed dashboard bundle must remain present because `cmd/dashboard/main.go` embeds `ui/dist`, but it is generated output and should not be linted as project source. `cmd/dashboard/ui/.oxlintrc.json` now uses `ignorePatterns: ["dist/**"]`, so the documented `npm run lint` command checks the dashboard source without emitting warnings from minified React internals.

The UI lint and Vitest suite pass, as do `go build ./...`, `go vet ./...`, `go test ./...`, and `gofmt -l ./cmd ./pkg ./internal`. This is an internal developer-experience change; no runtime behavior or generated asset changed.

---

## 494. Append-only funnel/attempt stage ledger

**Completed 2026-08-01.** `funnel_stage_events` is an append-only, indexed SQLite ledger of every actual `job_funnel.status` transition. Each event contains only the canonical URL, prior and next state, derived pipeline stage, normalized reason code, UTC timestamp, and time since the preceding state. A database trigger writes the event as part of the status update, chosen over a separate Go call at every writer because it prevents new or overlooked writers from silently bypassing history. The marginal write is one compact indexed insert per state change, appropriate for the local agent's volume; no retention policy was added because the full durable transition history is the explicit purpose of the item and the ledger deliberately excludes all job content and applicant data.

`application_attempts.reason_code` receives an idempotent migration without fabricating historical values. Pipeline classification preserves the existing coarse terminal classes for compatibility while distinguishing `prompt_injection_quarantine`, `browser_crash_recovery_exhausted`, and `generic_fill_failure`; no raw error text is stored. Existing `status_reason` values are cleared when a reasonless state replaces them, preventing a prior terminal cause from leaking into a later ledger event.

Focused tests prove state-history reconstruction, normalized attempt-code persistence, and the three required failure classifications. Legacy-schema tests exposed two compatibility gaps during implementation: old rows may have no `last_updated`, so the trigger safely timestamps those events at write time; a focused retry migration may predate `status_reason`, so its writer ensures that idempotent migration before use. Full `go build ./...`, `go vet ./...`, `go test ./...`, and `gofmt -l ./cmd ./pkg ./internal` all passed. No production database was queried or modified; the normal next pipeline transition will begin accumulating live ledger evidence.

---

## 496. ATS capability and automation-success registry

**Completed 2026-08-01.** The existing `career_sites` table is now populated whenever a new funnel row is discovered and whenever an application attempt is recorded. It retains the observed domain/provider, most recent observation, successful form reach, account-gate evidence, confirmation strategy, and mapping-health state. `form_mappings` now stores success and failure counts plus its latest validation time. Both upgrades are idempotent: old databases gain the fields without fabricated historical evidence.

`RecordAttempt` writes the original attempt and registry evidence in one transaction. A confirmed application or intentional human-review handoff counts as a successful form reach; a manual account gate sets durable evidence without treating it as a permanent source ban. Cached mappings remain selectable when new or successful, but `AttemptSubmit` now yields to the existing provider-specific flow if a mapping becomes failure-dominated or its successful evidence is older than 30 days. This preserves a safe fallback without blacklisting an ATS from one error.

Storage tests cover old-table migrations, discovery population, successful and failed outcome updates, account-gate evidence, fresh-success preference, failure-dominated fallback, and stale-success fallback. Focused storage and submitter tests passed. Full `go build ./...`, `go vet ./...`, `go test ./...`, and `gofmt -l ./cmd ./pkg ./internal` verification is recorded in the final task commit. No live production database was queried or changed during this task; the next normal discovery or attempt will populate the registry automatically.

---

## 495. No-progress / dominant-failure-reason watchdog

**Filing brief, moved out of the live backlog by the 2026-08-06 groom pass:**

**Filed 2026-08-01**, mission-alignment audit (seeded candidate E).

Confirmed no mechanism detects "the daemon is alive and processing but producing no confirmed applications" or "one failure reason has dominated several cycles." `runAgentSchedule` (`cmd/agent/main.go:518-593`) only distinguishes "cycle had work" from "no eligible jobs" for scheduling purposes; `runDaemonDiscoveryLoop` (`:598-631`) just logs per-refresh errors. The existing poll-failure banner (#447/#460) and per-domain circuit breakers (#469/#475) are both narrower, failure-triggered mechanisms — neither tracks time-since-last-confirmed-application or a dominant status/reason across cycles.

**This audit is itself the evidence for this item's value:** bugs.md #489 (51% of the entire funnel quarantined, mission-critical) was only found because a human ran a manual multi-hour database audit — exactly the shape of condition ("a high percentage of jobs terminate at one stage," per the seeded brief) a watchdog should have surfaced automatically, and much sooner than a week after #394's incomplete fix.

**Proposed direction, per the brief:** track eligible-fresh-queue-nonempty + cycles-continuing + no-confirmed-application-for-N-hours; a dominant terminal status or (post-bugs.md #480 broadening / #494) dominant `status_reason` across recent cycles. Emit one deduplicated actionable alert (log line + dashboard status, no email/SMS infrastructure exists to page through); create a sanitized diagnostic snapshot; never auto-relax user constraints; never auto-requeue at volume. A coarser first version can ship using only existing status counts (no dependency on #480/#494), with reason-level detail added once those land.

**Acceptance criteria:** a seeded test fixture with N cycles of a dominant failure status triggers exactly one alert, not one per cycle; a fixture with healthy variety triggers none; a fixture with an empty eligible queue (nothing to attempt) triggers none.

**Automated tests:** table-driven tests over the trigger conditions above.

**Safe live verification:** run the watchdog against a read-only copy of the real `applications.db`'s history and confirm it would have flagged the #489 condition (QUARANTINED_PROMPT_INJECTION dominant on 2026-08-01) had it existed.

**Boundaries:** detection and alerting only — no automatic recovery action (source suppression, requeue, constraint relaxation) is in this item's scope; evaluate those separately per the brief's own caution against unlimited watchdog authority.

**Completion account follows.**

**Completed 2026-08-01.** The daemon now observes a sanitized aggregate snapshot after each queue cycle. After three consecutive cycles with eligible work but no new confirmation, it alerts only when one terminal status is at least 75% of recent outcomes. Alerts are deduplicated, logged without job content or URLs, persisted as the current dashboard alert, and rendered by the React dashboard. The watchdog never requeues jobs, suppresses sources, relaxes constraints, or changes submission behavior.

Table-driven coverage includes the #489-shaped `QUARANTINED_PROMPT_INJECTION` condition, healthy variety, and an empty eligible queue. The full Go build, vet, test, formatting loop, dashboard UI tests, and production UI build passed. The dashboard and daemon were rebuilt and restarted; immediately after restart the dashboard reported 128 eligible jobs and no watchdog alert, as expected before three completed cycles.

---

## 499. Persist `discovery_source` at `AddToFunnel` time

**Completed 2026-08-01.** `job_funnel` now has a nullable `discovery_source` column. The idempotent migration leaves every pre-existing row `NULL`: the original discovery channel was discarded, so reconstructing it from a destination hostname would fabricate data. New inserts use the actual channel at the five live paths: `remoteok`, `hackernews`, `atsfeed:greenhouse`, `atsfeed:lever`, `serpapi`, and `yahoo`.

Storage coverage verifies both the migration and a source-aware insert. Discovery coverage verifies SerpApi, RemoteOK, Yahoo, Hacker News, and an ATS feed persist their respective labels. `go build ./...`, `go vet ./...`, `go test ./...`, and `gofmt -l ./cmd ./pkg ./internal` all passed.

Live verification rebuilt the dashboard-managed daemon and restarted it through its local lifecycle endpoint. The daemon preflight passed and began a fresh discovery cycle. A read-only grouped aggregate at verification time contained only `legacy_unknown` rows, and the dashboard reported zero eligible jobs; this is an honest no-new-posting observation, not a migration failure. The next newly inserted job will carry its channel label automatically, allowing #493 and #496 to use the data without pretending historical rows have it.

---

## 491. Define authoritative mission metrics and surface them on the dashboard

**Completed 2026-08-01.** `/api/metrics` now returns a deliberately small set of outcome and queue-health measures: confirmed applications today and over the last seven days, median discovery-to-first-attempt latency, time since the last confirmed application, and both the eligible queue count and its never-attempted subset. Confirmations use `job_funnel.applied_at`, the canonical timestamp bug #490 added; first-attempt latency uses the earliest `application_attempts.started_at` per URL; eligibility exactly mirrors `storage.GetDiscoveredJobs`'s status, breezy.hr exclusion, and retry-backoff condition. The React dashboard renders the metrics together in a semantic definition list and uses an em dash or explicit no-confirmation message where no value exists, rather than manufacturing a zero duration.

The seeded SQLite test covers confirmations across three days, the median of multiple first attempts, an eligible unattempted row, an eligible attempted row, a breezy.hr exclusion, and a deferred row. The Vitest coverage confirms the dashboard renders both populated aggregates and the unavailable confirmation state. `go build ./...`, `go vet ./...`, `go test ./...`, `gofmt -l ./cmd ./pkg ./internal`, focused UI tests/build, and `oxlint src` all passed.

Live verification restarted the dashboard only (the agent daemon was already healthy) and read the new endpoint against the active database: it reported 0 confirmed today/week, a 4d 18h median first-attempt latency, 185 raw `DISCOVERED` rows, and 0 eligible/never-attempted rows. This confirms the user's observation: the daemon is idle because all visible discovered rows are excluded by bug #482's breezy.hr policy, not because the running process is stuck. Search discovery is also degraded separately by an exhausted SerpAPI quota and Yahoo fallback EOFs; no failed jobs were requeued and no source policy changed in this task.

---

## 506. `/work_next_item`'s selection rule never returns to `bugs.md` once the gate is MET, starving Minor Pending bugs indefinitely

**Fixed 2026-08-01.** The canonical local workflow at `.agents/prompts/work_next_item.md` previously said that a `MET` Usability Gate should select from `improvements.md`; that made the gate a one-way ratchet and left every remaining Minor bug permanently unreachable by normal selection. The revised rule preserves the hard priority while the gate is unmet, then compares all open, above-floor rows in `bugs.md` and `improvements.md` by their common ROI formula after it is met. It explicitly states that a met gate means no Blocker or Major bug remains, not that Minor defects become ineligible.

The acceptance criterion is demonstrated by the live backlog state at close: bug #501 scores 2.0 while the highest Pending improvement, #491, scores 1.5, so a new `/work_next_item` run now selects #501. No production behavior changed; this is a canonical prompt and backlog-record correction only. Antigravity model discovery and local Ollama discovery were both unavailable from the sandbox, so no delegate was used for this bounded documentation task. The required Go build, vet, test, and formatting checks were run before close.

---

## 500. [Add a missing index on `job_funnel(discovered_at)`](#500-add-a-missing-index-on-job_funneldiscovered_at)

**Table rationale cell (original):** **Closed 2026-08-01 — investigated, not implemented.** Live `EXPLAIN QUERY PLAN` evidence showed the premise didn't hold: no current query filters or sorts on `discovered_at`, so the index would never be selected. See detail section

### 500. Add a missing index on `job_funnel(discovered_at)`

**Re-evaluated 2026-08-01** while working `/work_next_item` (this was the top-scoring Pending row, 3.0). The row's premise — "`discovered_at` is read on every row by `RankJobs`'s age computation and `GetDiscoveredJobs`'s `ORDER BY`" — conflates two different things: `discovered_at` genuinely is read on every row, but only in **Go**, after the SQL query has already returned. `RankJobs` (`pkg/storage/ranking.go:151`) computes `ageDays` from an already-fetched `time.Time` field; there is no SQL `ORDER BY discovered_at` anywhere in `GetDiscoveredJobs`, `GetQueuePlan`, or any dashboard query — every one of them either has no `ORDER BY` at all (ranking happens in Go) or orders by `last_updated`/`applied_at` instead.

**Live-verified before implementing anything:** ran `EXPLAIN QUERY PLAN` (via a throwaway `modernc.org/sqlite` script against a read-only copy of the real `applications.db`, 11,731 rows) on both hot queries the row named:

```
GetDiscoveredJobs: SEARCH job_funnel USING INDEX idx_job_funnel_status (status=?)
GetQueuePlan:      SEARCH f USING INDEX idx_job_funnel_status (status=?)
```

Both already use the existing `idx_job_funnel_status` index — neither is a full table scan today, which is what the row's own acceptance criteria ("`EXPLAIN QUERY PLAN` on `GetDiscoveredJobs`'s query shows the new index in use instead of a full table scan") assumed was the starting condition. Adding a plain or composite `(status, discovered_at)` index would not change either plan, because `discovered_at` never appears in a `WHERE` or `ORDER BY` clause for these queries — SQLite has no reason to touch it.

**Closed rather than implemented.** `job_funnel` receives a write (insert or status update) on essentially every discovered/processed job — thousands per day per the row's own growth note (3,000 → 11,731 rows in one day). An index that no live query plan would ever select is pure write-amplification with zero read benefit, which is the opposite of what this item was trying to buy. If a future item (e.g. #491's mission-metrics queries, or #493's ranking-by-yield rewrite) actually adds a SQL `WHERE`/`ORDER BY` on `discovered_at`, add the index at that point and verify the plan change the same way this investigation did — don't add it speculatively ahead of the query that would use it.

`go build ./...`, `go vet ./...`, `go test ./...`, `gofmt -l ./cmd ./pkg ./internal` all clean (no production code touched).

---

## 505. [`storedPromptInjectionThreats` and `toStoredThreats` are the same field-for-field conversion, written twice](#505-storedpromptinjectionthreats-and-tostoredthreats-are-the-same-field-for-field-conversion-written-twice)

**Table rationale cell (original):** **Fixed 2026-08-01.** See detail section for the shared `storage.ThreatsToStored` helper and the two call sites now using it

### 505. `storedPromptInjectionThreats` and `toStoredThreats` are the same field-for-field conversion, written twice

**Boundaries (from the live row, moved here 2026-08-06):** pure refactor — no behavior change, no new fields.

**Fixed 2026-08-01.** Added `storage.ThreatsToStored([]promptsec.Threat) []PromptInjectionThreat` (`pkg/storage/manager.go`, next to the `PromptInjectionThreat` type it builds) as the single field mapping (`Type`, `Severity`, `Message`, `Guard`, `Match`, `Start`, `End`). Chosen location over `pkg/security`: `pkg/storage` already imports `pkg/security` (for `SetPrivateUmask`/`SecurePrivateFile`/`PrivateDirMode`), so a function in `pkg/security` returning `[]storage.PromptInjectionThreat` would have created an import cycle. `pkg/storage` importing `github.com/danielthedm/promptsec` directly (already a transitive dependency via `pkg/security`) has no such problem — updated the type's doc comment to describe the real constraint (avoiding a `pkg/security` import specifically, not "any third-party dependency") rather than leave a now-inaccurate comment in place.

- `cmd/agent/main.go`'s `storedPromptInjectionThreats` is now a 3-line nil-check wrapper around `storage.ThreatsToStored(detection.Threats)` (kept, rather than deleted, because its caller passes a `*security.PromptInjectionError` that can be nil, and dereferencing `.Threats` on a nil pointer would panic).
- `pkg/submitter/browser.go`'s `toStoredThreats` was deleted outright (its only caller already had a non-nil `*security.PromptInjectionError` in scope) and its call site now calls `storage.ThreatsToStored(detection.Threats)` directly; the now-unused `github.com/danielthedm/promptsec` import was removed from that file.

No behavior change at either call site — `go test ./...` passes unchanged (neither function had direct unit tests; both are exercised indirectly via the packages' existing quarantine-path tests). `go build ./...`, `go vet ./...`, `gofmt -l ./cmd ./pkg ./internal` all clean.

---

## 484. [Local-model benchmark and routing-evidence harness](#484-local-model-benchmark-and-routing-evidence-harness)

**Table rationale cell (original):** **Fixed 2026-08-01.** See detail section for `cmd/modelbench`/`internal/modelbench`, its mocked-Ollama test suite, and the live idle-guard verification against the real running daemon. Foundational infrastructure for every other local-model routing decision in this file

### 484. Local-model benchmark and routing-evidence harness

**User-directed 2026-08-01**, as the foundational item of a resource-aware local-model utilization plan for this laptop (Ryzen 5 5600U, 32 GB RAM, integrated Vega, one shared Ollama instance — see the groom note above the ranked table for the full session account and the candidates deliberately not built this session, #485-#488).

**The gap:** nothing in this repository had ever measured which locally-installed model is actually the right one for a given bounded task. `pkg/mcp`'s per-call-type model selection (improvement #24, `OLLAMA_FAST_MODEL`) exists as plumbing but is unset by default and was never validated against measured task-level quality or timing — the assumption "bigger model, better answer" was never tested against this host's own constraint (CPU-only inference, one model resident at a time via `OLLAMA_MAX_LOADED_MODELS=1`).

**Fixed 2026-08-01.** Added:

- `internal/modelbench` — the measurement logic, independent of `pkg/mcp` on purpose: `pkg/mcp`'s `ollamaProvider` is the production inference path and its `ollamaChatResponse` struct deliberately ignores Ollama's per-call timing fields (`total_duration`, `load_duration`, `prompt_eval_count`/`_duration`, `eval_count`/`_duration`) since nothing in the product needs them. This package talks to the same `/api/chat`, `/api/tags`, and `/api/ps` endpoints directly so it can capture that data without touching or risking the production path.
  - `ollama.go` — `ListModels`, `ListRunning`, `Generate` (captures every timing field plus derived prompt/generation tokens-per-second), `Unload` (best-effort `keep_alive: 0` to force a genuine cold start before the first call).
  - `tasks.go` — three representative, synthetic, objectively-validated task classes: `classify_error` (structured JSON classification into a fixed enum + confidence), `summarize_excerpt` (bounded plain-text summary of a fabricated Go function, required-keyword-checked), `plan_tests` (structured JSON root-cause/planned-files/success-and-failure-test plan for a synthetic bug). Every fixture is invented for this file — no real logs, code, or backlog content.
  - `metrics.go` — best-effort `/proc/meminfo` snapshot (`MemAvailable`, `MemTotal`, `SwapTotal`, `SwapFree`), every field a pointer so "unavailable" (non-Linux, sandboxed) is distinguishable from "measured zero" and never fails a run.
  - `guard.go` — `IsAgentRunning`, a read-only re-implementation of `cmd/dashboard`'s `agentPIDAt` (bug #449) against the same `applications/career_agent.lock`, duplicated rather than imported since `cmd/dashboard` is `package main`.
  - `runner.go` — sequential-only orchestration (never two models loaded concurrently), `CheckModelsAvailable` (refuses an uninstalled model with an actionable error naming what is available, never pulls one), cold/warm labeling (first call per model is cold, after an explicit unload; every repetition after is warm).
  - `report.go` — `Report`/`ModelReport`/`TaskResult` JSON schema plus a human-readable `Summary()`. A result's `Passed()` is timeout/error/schema-valid only; `Correct` (matching the fixture's known answer) is a separate, informational field that does not fail a run — a schema-valid-but-wrong answer is a routing data point, not a harness defect.
- `cmd/modelbench/main.go` — the CLI: `-list`, `-models` (required, comma-separated), `-tasks` (default `all`), `-reps`, `-timeout`, `-temperature` (default 0), `-out` (optional; nothing is written to disk by default), `-force` (bypass the idle check). **Refuses to start while `cmd/agent`'s single-instance lock is held**, unless `-force` is passed — benchmarking unloads/reloads models on the same Ollama instance the production agent depends on, and this is a live-verified safety property (see below), not just a documentation note. Exits nonzero if any invoked task fails, times out, or violates its schema.
- `documentation/model_benchmark.md` — why this exists, how to list/run/interpret results, why vision and embeddings are deliberately out of scope for this first pass (recorded, not silently dropped), and the routing hypothesis this measurement infrastructure is meant to test (explicitly labeled a hypothesis, not a settled policy).
- `.gitignore` gained `/benchmark_results/`, the documented convention for `-out`, so a benchmark run can never accidentally become a commit.

**Tests** (`internal/modelbench/*_test.go`, `cmd/modelbench/main_test.go`), all against a mocked `httptest` Ollama server — the normal `go test ./...` run needs no live Ollama: `/api/tags` discovery (valid, malformed, unreachable-host), unavailable-model rejection with an actionable error, `/api/chat` timing-field parsing, zero-token/missing-duration handled without a divide-by-zero, request timeout, Ollama error-field and non-200 propagation, `/api/ps` residency parsing, every task's schema-valid/schema-invalid/too-long/wrong-answer-but-schema-valid cases, a synthetic-fixture-contains-no-PII-shaped-content check, `/proc/meminfo` parsing including the case where a field (e.g. swap) is simply absent, cold-vs-warm call labeling, JSON round-trip, `Passed()` semantics (timeout/error fail regardless of `SchemaValid`), the agent-lock guard (free/held/PID-parsed/second-check-not-left-locked), and the CLI's exit codes for every one of the above plus `-force` bypass and successful-run-writes-report-to-file.

**Live verification performed, and deliberately not a full run.** `ps aux` showed the production daemon genuinely running (`career_agent_bin -daemon -cycle-limit 15 -cycle-interval 1m`, pid 1379357) at the time this item was implemented — not an idle window. Per this item's own safety requirement, `go run ./cmd/modelbench -models qwen3:4b-instruct -tasks classify_error -reps 1` was run against the **real** `applications/career_agent.lock` (not a mock) and correctly refused with `the production agent appears to be running (lock held, pid 1379357)`, exit code 1 — a genuine, live-verified exercise of the safety mechanism against real production state, not a synthetic one. A full benchmark run against `qwen3:4b-instruct` was deliberately deferred rather than forced through with `-force`, per the instruction not to disrupt a live application attempt to satisfy this check. **Manual verification command for the next idle window:**

```bash
go run ./cmd/modelbench -models qwen3:4b-instruct -reps 2
```

`go build ./...`, `go vet ./...`, `go test ./...` and `gofmt -l ./cmd ./pkg ./internal` all clean.

**Boundaries and exclusions, recorded rather than silently dropped:** vision benchmarking (structurally different task shape, would have doubled this item's scope — documented as a follow-up); embeddings (not generative text, a useful benchmark needs a retrieval-quality design, not a fourth chat task); a concrete 12B-16B "medium" model (none installed — the harness can benchmark one the moment the user installs and names it, but this item does not install, recommend, or hardcode a specific ID that would likely be stale later). See `documentation/model_benchmark.md` for the full account of each.

**Cross-reference:** #442 (measuring the NLP-microservice offload) asks a different question — in-process vs. offloaded HTTP routing of the *same* model call — and is not merged into or subsumed by this item; see #442's own row for the distinction.


---

## 477. [Yahoo fallback requests carry no realistic browser headers or cookie jar](#477-yahoo-fallback-requests-carry-no-realistic-browser-headers-or-cookie-jar)

**Table rationale cell (original):** **Fixed 2026-08-01.** See detail section for the fix, the mutation-checked tests, the independent Claude review, and the live before/after sample (25.2% → 19.7% failure rate, directionally positive but not statistically conclusive — the row's own text always allowed closing either way)

### 477. Yahoo fallback requests carry no realistic browser headers or cookie jar

**Found 2026-07-31** while fixing bug #475 (a source-level circuit breaker for the Yahoo HTML discovery fallback). #475's root-cause analysis identified two contributing factors behind the sustained ~78% failure rate: the dominant cause (a block that outlasts any per-query retry, fixed by the breaker) and a secondary, unverified one — `discoverWithYahooHTML` (`pkg/scraper/funnel.go`) sets only a `User-Agent` header on every request, no `Accept`, `Accept-Language`, `Referer`, or persistent cookie jar, and up to 5 of these requests run concurrently against the same host. That is a recognizable non-browser fingerprint, and #475's own text flagged it as "cheap to test" but explicitly declined to fix it as part of that bug, since the breaker was the load-bearing fix and this half's benefit is unverified — Yahoo's blocking behavior is opaque from here, and it might be purely volume/rate-based rather than fingerprint-based, in which case this would change nothing.

**Fix direction:** add a realistic `Accept`/`Accept-Language` header pair to the existing request, and share one `http.CookieJar` (via `http.Client.Jar`) across every Yahoo query within a single `DiscoverJobs` run rather than each query using an independent client. Verify live against the real daemon log whether the sustained failure rate actually drops — if it does not, that is useful information for #475's own note that a per-domain block can be purely volume-based, and this row can close with that finding either way.

**Fixed 2026-08-01.** `discoverWithYahooHTML` (`pkg/scraper/funnel.go`) now sets `Accept: text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,*/*;q=0.8` and `Accept-Language: en-US,en;q=0.9` alongside the existing `User-Agent`. The per-query client was replaced with a client shared across every query made by one `FunnelEngine` instance: a new `newYahooHTTPClient()` wraps the existing `security.NewSafeHTTPClient` (preserving its transport/redirect guard) and adds `Jar: cookiejar.New(nil)`; `FunnelEngine` gained `yahooClientOnce sync.Once` + `yahooClient *http.Client` fields so the client (and its jar) is built exactly once per engine and reused by every concurrent caller. Scoping is correct because `cmd/agent/main.go:1108` constructs a fresh `scraper.NewFunnelEngine(...)` on every `DiscoverJobs` call — one engine per run, never shared across runs, and never touching the separate SerpAPI/RemoteOK clients which still call the package-level `newHTTPClient` with no jar.

Three new tests in `pkg/scraper/yahoo_test.go` cover this: headers actually arrive at a mock server (`TestDiscoverWithYahooHTML_SetsBrowserHeaders`); a second query on the same engine carries the `Set-Cookie` the first query's response set, and a fresh engine does not inherit it (`TestDiscoverWithYahooHTML_SharesCookieJarWithinOneEngine`); and 5 concurrent calls on one engine (mirroring `DiscoverJobs`'s real `eg.SetLimit(5)` access pattern) are race-clean under `go test -race` and still land on one shared client (`TestDiscoverWithYahooHTML_ConcurrentQueriesShareOneClient`). All three were mutation-checked by reverting the corresponding piece of the fix and confirming the test fails, then restoring — done independently twice, once by the implementing session and once by a separate background Claude review pass, which found no bugs (concurrency safety, scoping, and test quality all confirmed).

**Live verification:** the real daemon (`career_agent_bin -daemon`, `SERPAPI_API_KEY` present but exhausted for the billing period — "Your account has run out of searches", so nearly every discovery query hits the Yahoo fallback) was rebuilt with the fix, gracefully stopped (SIGTERM, clean shutdown) and restarted with identical flags so `career_agent.log` continues one file for a fair before/after comparison. Both windows already had bug #475's circuit breaker active, so the comparison isolates this fix's effect: pre-fix, 2026-07-31 20:24 to 2026-08-01 09:04 (~12.6h), 678 final failures out of 2,690 breaker-allowed attempts = **25.2%**. Post-fix, the first ~5 minutes after restart, 15 final failures out of 76 breaker-allowed attempts = **19.7%**, before the breaker itself opened on the observed failures and started throttling further attempts (working as #475 designed). Directionally lower, consistent with the header/fingerprint theory, but the post-fix sample is two orders of magnitude smaller than the pre-fix one and Yahoo's blocking behavior is still opaque from here — not a statistically confident causal claim. Per this row's own text, that is an acceptable close either way; the fix is real, tested, and live-deployed regardless of whether the rate drop holds up under a longer observation window.


---

## 474. [ADRs have no process ensuring they get updated when the decision they describe changes](#474-adrs-have-no-process-ensuring-they-get-updated-when-the-decision-they-describe-changes)

**Table rationale cell (original):** **Fixed 2026-08-01.** See detail section for the fix

### 474. ADRs have no process ensuring they get updated when the decision they describe changes

**Fixed 2026-08-01.** Working Protocol step 7 (`improvements.md`) now requires checking `docs/adrs/*.md` for any ADR the change touches (grep the touched package/file names against the existing ADR set, which is small and fixed) and correcting it in the same commit if it describes behavior the change alters — the same commit-scoped treatment #454 gave `CHANGELOG.md`, applied to the ADR audience/failure-cost pair this row identified. `docs/adrs/ADR-003-SQLite-Concurrency.md`'s own stale note (about bug #446) was already corrected in a prior session (2026-07-31), so this fix is the general process, not a specific-file repair — confirmed by re-reading the file before starting. Not a `CHANGELOG.md`-worthy change per the Working Protocol's own scoping (a backlog/protocol-only edit, no code or user-visible behavior change). Original report follows.

**Found 2026-07-31** while updating `docs/adrs/ADR-003-SQLite-Concurrency.md`'s consequences and decisions to describe #450's reader/writer DSN split.

The ADR's "Known gap, not a decision" paragraph under Consequences still read, verbatim: *"`cmd/dashboard/main.go` opens the same file with the older `?_journal_mode=WAL` query-parameter form... This is tracked as bug #446 and is a defect to fix, not part of this decision."* Bug #446 shipped long before this session — `cmd/dashboard` has derived its DSN from `pkg/storage` since then, and two tests (`TestDashboardDSNMatchesStorage`, `TestDashboardDSNCarriesABusyTimeout`) have pinned that fact ever since. The ADR simply never got told.

**This is the same shape as #454** (`CHANGELOG.md` drifting a full day behind five shipped bug fixes): a document that describes executable behavior, with no step in the Working Protocol that touches it when the behavior it describes changes. #454's fix added a `CHANGELOG.md` requirement to the close-the-loop step; it did not extend to `docs/adrs/`, which is a different artifact with a different audience (future engineers deciding whether to revisit an architecture decision, not users reading release notes) and a different failure cost (a stale ADR reads as "known issue, not yet fixed" long after it *was* fixed, which could send a future session chasing a defect that no longer exists).

**Fix direction:** extend the Working Protocol's close-the-loop step (or add a sibling step next to the `CHANGELOG.md` one) to require checking whether the change touches any ADR's described decision, and correcting it in the same commit if so — scoped the same way #454 scoped `CHANGELOG.md`, to changes that actually affect what an ADR describes, not every commit. `docs/adrs/` currently holds a small, fixed set of files, so a grep for the touched package/file names against existing ADRs is enough to catch the common case.


---

## 470. [`cmd/requeue`'s `countForStatus` is fully written and tested but never called](#470-cmdrequeues-countforstatus-is-fully-written-and-tested-but-never-called)

**Table rationale cell (original):** **Fixed 2026-07-31.** See detail section for the fix, the RETRY_EXHAUSTED gap it surfaced and closed, and its live verification

### 470. `cmd/requeue`'s `countForStatus` is fully written and tested but never called

**Found 2026-07-31** while fixing bug #466, whose fix touched `cmd/requeue/main.go`'s `-status` flag help text. `countForStatus` (line ~169) maps a `-status` value to the matching field on a `SourceOutcomeStat`, has a doc comment explaining the distinction between recovery statuses and `APPLIED`, and has its own table-driven test (`TestCountForStatus` in `main_test.go`, covering all three valid values plus an unknown-status error case) — but nothing in `main()` calls it. The actual requeue path calls `storage.RequeueByURLPattern(p, *fromStatus)` directly with no validation at all, so `-status TYPO_STATUS -confirm` runs cleanly and reports "requeued 0 row(s) from TYPO_STATUS to DISCOVERED" instead of failing loudly on an operator's typo.

**Fix direction:** either wire `countForStatus` (or an equivalent switch) into `main()` as real pre-flight validation of `-status` before the requeue loop runs, printing the current count for context; or, if `printStats`'s per-source breakdown already makes it redundant, delete the function and its test. Low value either way — the current behavior is silently unhelpful, not unsafe (an invalid status matches zero rows, never the wrong ones) — but it is a small, cheap fix. `mechanical` tier: mostly wiring an already-correct, already-tested function into its evident call site, or removing it outright.

**Fixed 2026-07-31.** Re-verifying against current code before wiring anything in found the row's own fix direction was incomplete: `countForStatus`'s switch only covered `BLOCKED_CAPTCHA`/`FAILED_SUBMIT`/`APPLIED`, but bug #466 had already made `RETRY_EXHAUSTED` a valid, documented `-status` value (the flag's help text and `RequeueByURLPattern`'s own doc comment both already treat it as legitimate). Wiring the switch in unmodified would have made `-status RETRY_EXHAUSTED` — a real, already-shipped use case — fail as "not a real status", trading one silent-success bug for a new hard-failure regression on a status that had genuinely been requeueable since #466 shipped.

Scope widened accordingly: `storage.SourceOutcomeStat` gained a `RetryExhausted` field, `SourceOutcomeBreakdown`'s query now counts `RETRY_EXHAUSTED` rows and includes the status in its `WHERE ... IN (...)` clause, and `countForStatus` gained a `RETRY_EXHAUSTED` case plus an updated error message naming all four valid values. `main()` now validates `*fromStatus` once via `countForStatus(storage.SourceOutcomeStat{}, *fromStatus)` — a zero-value stat is sufficient since the switch only inspects the status string — before the per-pattern loop runs, `log.Fatal`-ing on an invalid value before any DB write is attempted regardless of `-plan`/`-confirm`/`-stats` mode. Each per-source iteration also now prints `[name] N row(s) currently <status>` before acting, closing the specific visibility gap in the `-confirm`-without-`-plan` path, which previously skipped straight to `RequeueByURLPattern` with no count printed at all.

Tests: `pkg/storage/manager_test.go`'s `TestSourceOutcomeBreakdown` extended with a `RETRY_EXHAUSTED` row; `cmd/requeue/main_test.go`'s `TestCountForStatus` extended with a `RETRY_EXHAUSTED` case. **Live-verified** in a scratch directory (a throwaway `applications.db`, not the production one): `./requeue_bin -source lever -status NOT_A_REAL_STATUS -confirm` now exits 1 with `-status must be BLOCKED_CAPTCHA, FAILED_SUBMIT, APPLIED, or RETRY_EXHAUSTED, got "NOT_A_REAL_STATUS"` and performs no DB write, where it previously would have printed "requeued 0 row(s)" and exited 0; a valid `-status BLOCKED_CAPTCHA` dry run correctly prints `[lever] 0 row(s) currently BLOCKED_CAPTCHA` ahead of the existing queue-plan output. `go build ./...`, `go vet ./...`, `go test ./...`, and `gofmt -l ./cmd ./pkg ./internal` all clean.


---

## 469. [Add per-domain circuit breakers for repeated fetch and pre-flight timeouts](#469-add-per-domain-circuit-breakers-for-repeated-fetch-and-pre-flight-timeouts)

**Table rationale cell (original):** **Fixed 2026-07-31.** New `domainCircuitBreaker` (`cmd/agent/circuit_breaker.go`), a closed/open/half-open state machine keyed by `getATSProvider`'s host string, shared across all workers and daemon cycles via a new `CircuitBreaker` field on `JobPipelineDeps`. Opens after 5 consecutive retryable failures for a domain, defers further jobs from that domain for a 2-minute cooldown, then allows exactly one half-open probe through before closing again on success or re-opening on failure. Wired into all three `checkJobAlive`/`fetchJobPage` call sites in `cmd/agent/pipeline.go` (StateInit preflight, StateDiscovery fetch, StateTailoring post-score revalidation). **A same-session independent Claude review pass caught three real defects before anyone else could hit them:** (1) the "circuit open, skip" branches originally called the existing `storage.UpdateFunnelStatusRetryable`, which increments `retry_count` and can terminate a row to `RETRY_EXHAUSTED` — charging a job's retry budget for an attempt the breaker never actually made, the same starvation shape bug #466 fixed, one layer up; fixed by adding a new `storage.DeferFunnelStatus(url, cooldown)` that returns the row to `DISCOVERED` with `next_eligible_at` pushed out but `retry_count` untouched. (2) `recordFailure`'s open-circuit branch re-stamped `openedAt` on every subsequent failure, so a straggler request already in flight when the circuit tripped could keep re-extending the cooldown past its intended 2 minutes; fixed by only resetting `openedAt` on the actual closed→open transition. (3) the `fetchJobPage` classification originally used `disposition`/error-sentinel checks that miscategorized a same-domain non-timeout HTTP response (e.g. a stray 401/403) as a domain failure; fixed by keying off `fetchResult.statusCode != 0` instead, which is only ever set once a real response came back from the server. **Mutation-checked all three fixes** by reverting each alone and confirming the exact test written for it fails with the exact symptom described. 7 new state-machine tests plus a `-race`-clean 50-goroutine concurrency test in `cmd/agent/circuit_breaker_test.go`, and a new `pkg/storage` test confirming `DeferFunnelStatus` leaves `retry_count` unchanged. `go build ./...`, `go vet ./...`, `go test ./...` (including `-race` on `cmd/agent` and `pkg/storage`), and `gofmt -l ./cmd ./pkg ./internal` all clean. README's re-queueing section gained a paragraph documenting the new circuit-breaker log line and its no-retry-budget-spent behavior. Original report follows. Found 2026-07-30 during a live-log audit: the same domains repeatedly logged `context deadline exceeded` while awaiting headers, consuming cycles that could have gone to healthy sources; retry was purely request-local with no shared domain health, cooldown, or circuit state

### 469. Add per-domain circuit breakers for repeated fetch and pre-flight timeouts

**Found 2026-07-30** during a live-log audit. The same domains repeatedly logged `context deadline exceeded` while awaiting headers, consuming cycles that could have gone to healthy sources. The current retry behavior is request-local; it has no shared domain health, cooldown, or circuit state.

**Fix direction:** track consecutive timeout/retryable failures by registrable domain, open a short cooldown after a threshold, probe during half-open recovery, and reset on success. Keep the state bounded and test that one failing domain does not delay unrelated queue entries.


---

## 468. [Filter weak discovery URLs and expose deferred queue state in the dashboard](#468-filter-weak-discovery-urls-and-expose-deferred-queue-state-in-the-dashboard)

**Table rationale cell (original):** **Fixed 2026-07-31.** New `job_funnel.status_reason` column + `storage.UpdateFunnelStatusInvalid` split `INVALID_URL` into malformed/expired at all 5 write sites in `cmd/agent/pipeline.go`; the dashboard's "Not A Posting" tile now captions whichever reason(s) contributed (same pattern as bug #451's Failed/Manual split), and a new tile surfaces `RETRY_EXHAUSTED`, which had zero dashboard presence before. See detail section for what was re-scoped out and why

### 468. Filter weak discovery URLs and expose deferred queue state in the dashboard

**Found 2026-07-30** from the live queue and logs. Records with query-only or generic page paths and pages whose fetched content has fewer than 200 visible runes are admitted as `DISCOVERED`, then repeatedly revisited after retryable failure. Existing URL filters cover known junk patterns but do not make this weak-page class visible to operators.

**Fix direction:** reject malformed and low-content records before they enter the normal queue, terminalize them after a bounded number of attempts, and add dashboard counts for deferred, invalid, weak-content, and retry-exhausted rows. This is separate from bug #466 because it improves admission quality and operator visibility after the retry scheduler is corrected.

**2026-07-31 note:** bug #466 (the retry scheduler correction this row was waiting on) is now Done — the "retry-exhausted" status this row should surface exists as the literal `RETRY_EXHAUSTED` job_funnel status, with a fresh retry budget available via `cmd/requeue -status RETRY_EXHAUSTED -confirm`. `#466`'s own review pass confirmed the dashboard (`cmd/dashboard/main.go`'s `serveMetrics`/`statusReason`/`explainedStatuses`) has no tile, card, or legend entry for it today — the count silently drops out of every bucket's total. This row is now fully unblocked and its dashboard half is a real, live gap, not just anticipated.

**2026-07-31 note (measured against the live `applications.db`, prompted by the user asking about a 1,214-row `INVALID_URL` count):** queried directly rather than estimated. Of the 1,214 `INVALID_URL` rows, only **~144 (12%) are the genuinely malformed, non-posting shape this row's admission-filtering half targets** — 19 Yahoo search-result/privacy-consent pages picked up as if they were postings, 44 generic Workday marketing `.html` pages, 53 bare Lever company boards with no job ID, and 28 bare Ashby company boards, same shape. The remaining **~1,070 (88%) are well-formed job-posting URLs** (Greenhouse `/jobs/<id>`: 450, Lever `company/<uuid>`: 246, Jobvite `/job/`: 116, Workday `/job/`: 4) that `checkJobAlive`'s dead-redirect detection correctly caught as expired or closed by the time the agent fetched them — real job-market churn, not a discovery-quality defect, and outside this row's admission-filtering scope. `pkg/storage`'s `statusReason`/dashboard caption ("Not a real posting…") does not distinguish the two, so an operator reading the dashboard would reasonably read the whole bucket as junk when 88% of it is "was real, went stale." Folding this into this row's scope rather than filing separately, since whoever adds the dashboard counts this row already calls for should split `INVALID_URL`'s caption/count into these two cases at the same time, not just surface one combined number.

**Fixed 2026-07-31.** Re-verified this row's own claims against current code before implementing, and one of them no longer held: `TestFetchJobPageRejectsWeak2xxContent` (`cmd/agent/main_test.go`) proves the "repeatedly revisited after retryable failure" complaint for weak-content pages is a deliberate, already-tested design decision (a 200 OK with too little text could be a slow-rendering page, so it stays retryable) that #466 already bounded to `MaxRetryAttempts = 5` before terminalizing to `RETRY_EXHAUSTED` — not a live defect, so the "reject before the queue" and "terminalize after N attempts" halves of the original fix direction were **not** re-implemented; forcing new machinery there would have duplicated #466's retry budget for no measured benefit. Likewise no new "DEFERRED" status was added — `DeferFunnelStatus` already resets rows to `DISCOVERED` with `next_eligible_at` pushed out, and the row's "deferred" wording maps to that existing mechanism, not a gap.

What *did* ship, targeting the two live, evidence-backed gaps the notes above actually found: a new `job_funnel.status_reason` column (idempotent migration `migrateJobFunnelStatusReason`, same pattern as the sibling `migrateJobFunnel*` migrations) and `storage.UpdateFunnelStatusInvalid(url, reason)` with `InvalidURLReasonMalformed`/`InvalidURLReasonExpired` constants, wired into all 5 `INVALID_URL`-writing call sites in `cmd/agent/pipeline.go` (known-junk URL and unsafe-network-target → malformed; dead-redirect preflight, terminal fetch status, and post-score dead-redirect revalidation → expired). `cmd/dashboard/main.go` splits `InvalidURL` into `InvalidURLMalformed`/`InvalidURLExpired` the same way bug #451 split Failed/ManualRequired, and adds `RETRY_EXHAUSTED` to `statusReason`/`explainedStatuses`/`serveMetrics` for the first time. `cmd/dashboard/ui/src/App.tsx` gets a new `explainInvalidURL` caption helper (a persisted-reason split can't reuse the legend-keyed `explainPair`) and a new Retry Exhausted card.

Tests: `pkg/storage/manager_test.go` (`TestMigrateJobFunnelStatusReason`, `TestUpdateFunnelStatusInvalid_RecordsReason`), `cmd/dashboard/main_test.go` (`TestServeMetrics_Counts_SplitsInvalidURLByReason`, `TestServeMetrics_Counts_RetryExhausted`, plus updating `TestExplainedStatuses_CoverEveryStatusReasonArm`'s hardcoded arms list and the inline test schema for the new column). **Independently reviewed by a second Claude session** (fresh context, own `git diff` read): confirmed all 5 call-site classifications, SQL placeholder/scan-order correctness, migration idempotency against both a fresh `CREATE TABLE` and a pre-existing DB, and that each new test would fail if its underlying fix were reverted. One minor cosmetic gap noted and accepted rather than fixed: a bucket mixing reasoned and legacy-NULL rows undercounts in the caption relative to the tile's total (e.g. 1 malformed + 2 expired + 1 legacy shows "1; 2" against a headline of 4) — self-resolving, since every write from this point forward sets a reason and only pre-existing historical rows lack one.

**Live-verified** against a scratch copy of `applications.db` on a second dashboard instance (`127.0.0.1:8099`, production `:8080` instance and its live agent left untouched throughout): seeded rows with `status_reason` values `malformed`/`expired`/NULL(legacy) plus a `RETRY_EXHAUSTED` row, confirmed `/api/metrics` returned the correct split counts (`invalid_url=1218` total against the real data, `invalid_url_malformed=1`, `invalid_url_expired=2` for the seeded rows, `retry_exhausted=1`) and correct legend text, and confirmed the built JS bundle contains the new "Retry Exhausted" card text. `go build ./...`, `go vet ./...`, `go test ./...`, `gofmt -l ./cmd ./pkg ./internal`, `npx tsc -b`, `npx oxlint src`, `npm run test` (vitest), and `npm run build` all clean; `cmd/dashboard/ui/dist/` rebuilt and committed alongside the source change per the project's `//go:embed` requirement. README's dashboard feature bullet updated to describe the new split and tile; `CHANGELOG.md` got a dated entry.


---

## 464. [`scripts/server.go`'s transpiled body is not `gofmt`-clean, and the documented `gofmt -l` loop can't catch it](#464-scriptsservergos-transpiled-body-is-not-gofmt-clean-and-the-documented-gofmt--l-loop-cant-catch-it)

**Table rationale cell (original):** Ran `gofmt -w scripts/server.go`; `gofmt -l scripts/server.go` now prints nothing. No behavior change (the file is `//go:build ignore`, excluded from `go build ./...` since #440, no caller) — whitespace/alignment only, so no `CHANGELOG.md` entry per the Working Protocol's own carve-out for `//go:build ignore` scripts. No ADR references `server.go`. Full loop re-run clean: `go build ./...`, `go vet ./...`, `go test ./...`, `gofmt -l ./cmd ./pkg ./internal`

### 464. `scripts/server.go`'s transpiled body is not `gofmt`-clean, and the documented `gofmt -l` loop can't catch it

**Found 2026-07-30** while fixing bug #440 (`scripts/server.go` was the only file in `scripts/` without `//go:build ignore`, so it was the one script `go build ./...` and `go vet ./...` compiled).

Adding the tag closed #440, but `gofmt -l scripts/server.go` still names the file afterward — the two added lines (`//go:build ignore` plus a blank line) are clean; the pre-existing transpiled Zero body (struct-field alignment, brace indentation carried over from the `//line metrics_summary.zero:NN` markers) is not, and was never touched by #440's fix.

`AGENTS.md`'s documented verification loop runs `gofmt -l ./cmd ./pkg ./internal` on purpose — `scripts/` is deliberately excluded because every file there (now including `server.go`) is `//go:build ignore`, meant to be run standalone with `go run scripts/<name>.go` rather than checked as part of the module build. That means formatting drift in `scripts/` can accumulate indefinitely with nothing in the documented loop ever flagging it.

Value is low: `server.go` compiles into nothing after #440 (it is excluded from `go build ./...`) and has no caller, so the only real cost is a confusing diff if anyone ever re-enables or edits the file by hand. `gofmt -w scripts/server.go` is the whole fix.

**Done 2026-08-01.** `gofmt -w scripts/server.go` applied; `gofmt -l scripts/server.go` now prints nothing. Whitespace/struct-alignment only, no behavior change and no caller, so no `CHANGELOG.md` entry (Working Protocol's own `//go:build ignore` carve-out). `go build ./...`, `go vet ./...`, `go test ./...`, `gofmt -l ./cmd ./pkg ./internal` all re-run clean.


---

## 463. [`cmd/dashboard/ui` has no test framework, so frontend logic bugs are only ever caught by manual live verification](#463-cmddashboardui-has-no-test-framework-so-frontend-logic-bugs-are-only-ever-caught-by-manual-live-verification)

**Table rationale cell (original):** Found 2026-07-30 while fixing bug #447. `package.json`'s only scripts are `dev`/`build`/`lint`/`preview` — no `vitest`, `@testing-library/react`, or any test runner is a dependency, and bug #451's detail section already flagged this in passing ("There are no frontend tests under `cmd/dashboard/ui` at all") without filing it as its own item. Verifying #447's fix (a `try/catch` plus a poll sequence-number guard, both pure state-machine logic) required running a second live dashboard instance and grepping the built bundle for the expected strings, because `tsc -b` and `oxlint` only catch type and lint errors, not behavior. This is the second dashboard UI bug in two days (#447, and #451/#452's frontend half) whose correctness had to be argued from a live instance rather than asserted by a fast, repeatable check. Value 3: every future `App.tsx` change pays this same live-verification tax. Effort 2: `vitest` plus `@testing-library/react` is a small, well-trodden addition, but writing real coverage for the existing polling/error-state logic (not just scaffolding) is the actual work

### 463. `cmd/dashboard/ui` has no test framework, so frontend logic bugs are only ever caught by manual live verification

**Found 2026-07-30** while fixing bug #447 (the dashboard's start/stop buttons silently swallowed failures, and a slow poll could overwrite fresher metrics with stale data).

`cmd/dashboard/ui/package.json`'s only scripts are `dev`, `build` (`tsc -b && vite build`), `lint` (`oxlint`), and `preview`. No test runner — `vitest`, `@testing-library/react`, `jsdom`, or anything else — appears in `dependencies` or `devDependencies`. Bug #451's detail section noticed this in passing while explaining why a caption bug went unnoticed ("There are no frontend tests under `cmd/dashboard/ui` at all") but did not file it as its own item.

The cost showed up directly while verifying #447's fix: the fix was pure state-machine logic (a `try`/`catch` around two `fetch` calls, and a request-sequence-number guard on a poll loop), exactly the kind of thing a unit test checks in milliseconds. Instead, verification meant building the UI, starting a second live dashboard instance on a scratch port, hitting the real `/api/agent/start` endpoint to provoke a genuine `500`, and grepping the minified production bundle for the expected error string — because `tsc -b` only checks types and `oxlint` only checks lint rules, neither of which reads runtime behavior. This is the second dashboard-UI bug in two days (#447, and the frontend half of #451/#452) whose correctness had to be argued from a live instance rather than a fast, repeatable check.

**Fix direction:** add `vitest` and `@testing-library/react` as dev dependencies, a `test` script, and real coverage for the existing logic — at minimum the poll sequence-number guard (a slow first response must not overwrite a fast second one) and the start/stop error states (a non-2xx or rejected `fetch` must set `actionError`). Scaffolding alone (an empty test file) would not pay for the effort; the value is in covering the logic that has twice now required a live instance to trust.

**Done 2026-07-30.** Added `vitest` `^4.1.10`, `@testing-library/react` `^16.3.2`, `@testing-library/jest-dom` `^7.0.0`, and `jsdom` `^30.0.1` as devDependencies; vitest config merged into `vite.config.ts` (`environment: 'jsdom'`, `setupFiles: ['./src/setupTests.ts']`) rather than a separate config file, so dev/build/test share one `react()` plugin instance. Six tests in the new `src/App.test.tsx`, all exercising real state-machine logic rather than smoke-testing render: two cover the poll sequence guard (a stale, slower `/api/metrics` or `/api/agent/status` response resolving after a fresher one must not overwrite it — mocked with a queue of controllable deferred promises resolved out of order), four cover the start/stop `actionError` states (non-2xx response, rejected `fetch`, and a prior error clearing on the next successful click). **Mutation-checked**: temporarily reverting the `/api/metrics` sequence guard (`if (seq === pollSeq.current) setMetrics(data)` → unconditional `setMetrics(data)`) makes the corresponding test fail with the exact stale-data symptom the guard exists to prevent; reverted immediately after confirming, `git diff` on `App.tsx` came back empty. `npm run test` (6/6), `npx tsc -b`, `npx oxlint src`, and `npm run build` (identical output bundle hash to the already-committed `dist/`, confirming zero behavior change) all clean; `go build ./...`, `go vet ./...`, `go test ./...`, and `gofmt -l ./cmd ./pkg ./internal` from the repo root all clean too. Implementation delegated to a Claude Sonnet 5 subagent per the item's own model column, reviewed and independently verified (including the mutation check) in the orchestrating session before commit.


---

## 462. [The RAG embedding retry loop duplicates `classifyGenerationError`'s logic instead of reusing it](#462-the-rag-embedding-retry-loop-duplicates-classifygenerationerrors-logic-instead-of-reusing-it)

**Table rationale cell (original):** **Fixed 2026-07-31.** `cmd/agent/pipeline.go:274`'s inline four-condition `strings.Contains` chain replaced with `classifyGenerationError(embErr) == genErrorRetryable`, the exact call the row asked for. No behavior change: `TestClassifyGenerationError`'s seven cases already cover all four retryable substrings this loop depended on, re-run with `-count=1` (not cache) and confirmed passing against the new code. `go build ./...`, `go vet ./...`, `go test ./...` and `gofmt -l ./cmd ./pkg ./internal` all clean. Not a `CHANGELOG.md`-worthy change per the Working Protocol (internal refactor, no user-visible behavior change)

### 462. The RAG embedding retry loop duplicates `classifyGenerationError`'s logic instead of reusing it

**Found 2026-07-30** while fixing bug #444 (a bare HTTP 429 was wrongly treated as a fatal daily-quota condition, shutting down the whole batch).

That fix introduced `classifyGenerationError` in `cmd/agent/pipeline.go`, a small helper that decides whether a generation error is `genErrorFatalQuota`, `genErrorRetryable`, or `genErrorTerminal`, and wired it into the two retry loops that could previously call `deps.Cancel()` (job scoring and document tailoring).

A third retry loop sits just above those two, at `pipeline.go:200`, retrying `GetEmbedding` for RAG context:

```go
if strings.Contains(embErr.Error(), "connect:") || strings.Contains(embErr.Error(), "no route to host") || strings.Contains(embErr.Error(), "429") || strings.Contains(embErr.Error(), "deadline exceeded") {
    log.Printf("[Worker-%d] Network or Rate Limit error getting embedding (attempt %d/3). Sleeping with backoff...", workerID, attempt)
    time.Sleep(mcp.ExponentialBackoff(attempt))
} else {
    break
}
```

This is the same four-condition match `classifyGenerationError`'s `genErrorRetryable` case now encodes, written out by hand instead of calling the helper. It is not a live bug — this loop never had a fatal branch, so it cannot regress into #444's shape — but it is the same classification logic living in two places, which is exactly the kind of drift risk this backlog has filed and fixed before under other names (e.g. #439's prompt duplication between Go and the Python microservice).

**Fix direction:** replace the four `strings.Contains` calls with `classifyGenerationError(embErr) == genErrorRetryable`, keeping the existing `else { break }` terminal-error behavior unchanged. One-line change, no test needed beyond confirming the existing embedding-retry tests (if any) still pass, since the observable behavior does not change — only the source of truth for what counts as retryable does.


---

## 460. [The dashboard UI shows no visible sign that a metrics poll failed](#460-the-dashboard-ui-shows-no-visible-sign-that-a-metrics-poll-failed)

**Table rationale cell (original):** **Fixed 2026-07-30.** `App.tsx` gained a `pollFailures` counter, reset to 0 on any successful `/api/metrics` poll and incremented (guarded by the existing `pollSeq` sequence check) on a non-ok response or a thrown fetch error. A single miss stays silent — the counter only renders once it reaches 2 consecutive failures, as a `role="status"` (not `role="alert"`) message reading "Metrics may be out of date — the last N polls failed", styled distinctly from the existing `.action-error` red banner so it reads as informational rather than urgent. Two new vitest cases cover the transient-miss (one failure then recovery: no indicator) and persistent-failure (two failures: indicator shown; a subsequent success clears it, not just stops growing it) paths; **mutation-checked** by stashing the fix and confirming exactly the new persistent-failure test fails while all others (including #447's existing poll-guard tests) still pass. Verified live: a second dashboard instance built from the fixed tree served HTTP 200 on both `/` and `/api/metrics` on `127.0.0.1:8099`, and the embedded JS bundle contains the new indicator text, confirming the `go:embed`'d `dist/` (rebuilt in this commit) matches the new `src/`. The production dashboard on `:8080` was untouched throughout. `go build ./...`, `go vet ./...`, `go test ./...`, `gofmt -l ./cmd ./pkg ./internal`, `npm run build`, `npm test`, and `npx oxlint src` are all clean

### 460. The dashboard UI shows no visible sign that a metrics poll failed

**Found 2026-07-30** by the independent Claude review pass on bug #452's fix.

`App.tsx`'s `fetchMetrics`:

```ts
const res = await fetch('/api/metrics');
if (res.ok) {
  const data = await res.json();
  setMetrics(data);
}
```

only updates state on a 2xx response. Before #452, `/api/metrics` always answered 200, so this branch was effectively unreachable for a query failure — the endpoint could not fail in a way this code would notice. #452 makes a genuine query failure a real, observable `500` for the first time. The fallback behavior on that failure (keep showing the last-good numbers rather than overwrite them with zeros or an error page) is the right instinct, but there is no companion signal anywhere in the UI: no banner, no "last updated" timestamp, no retry indicator. A single missed poll self-corrects silently on the next 2-second tick, which is fine — but a *persistent* failure (the database locked for an extended window, the dashboard's connection actually dead) looks identical to a healthy dashboard that simply has not changed, and a user has no way to tell the two apart from the page alone.

**Fix direction:** track consecutive failures or a last-successful-fetch timestamp in `App.tsx`'s state, and render a small, non-alarming indicator (e.g. "last updated 3m ago" or a subdued banner) once a poll has been failing for more than one or two cycles. Needs tests for both the transient-miss case (no visible change) and the persistent-failure case (indicator appears and clears correctly on recovery).


---

## 459. [`serveMetrics`'s by-source/by-variant breakdowns never check `rows.Err()` after their scan loops](#459-servemetricss-by-sourceby-variant-breakdowns-never-check-rowserr-after-their-scan-loops)

**Table rationale cell (original):** Found 2026-07-30 by the independent review pass on bug #452's fix. `cmd/dashboard/main.go`'s by-source and by-variant closures loop `for sourceRows.Next()`/`for variantRows.Next()` and never call `rows.Err()` afterward, so a cursor error partway through the result set (a dropped connection, a corrupted page) is silently swallowed — the response renders whatever rows were read before the failure as a complete breakdown, with no signal that it is truncated. Deliberately left alone by #452, which was scoped to the top-level query failures, not per-row iteration. **Fixed:** the two scan loops are now `scanSourceConversions`/`scanVariantConversions`, each returning an error on `rows.Err()` that flows into #452's 500 path. A real driver cannot be made to fail `Next()` mid-stream on demand, so both are tested against a hand-rolled `conversionRows` fake; mutation-checked by temporarily deleting each `rows.Err()` check and confirming the new tests fail with the exact truncated-breakdown behavior the bug described

### 459. `serveMetrics`'s by-source/by-variant breakdowns never check `rows.Err()` after their scan loops

**Found 2026-07-30** by the independent Claude review pass on bug #452's fix.

`cmd/dashboard/main.go`'s by-source and by-variant closures each loop over `db.Query`'s result set:

```go
for sourceRows.Next() {
    var s SourceConversionStat
    if err := sourceRows.Scan(...); err != nil {
        log.Printf("Failed to scan conversion-by-source row: %v", err)
        continue
    }
    bySource = append(bySource, s)
}
m.BySource = bySource
return nil
```

Neither loop calls `rows.Err()` after `Next()` returns false. Per `database/sql`'s own contract, `Next()` returning false can mean either "the result set is exhausted" or "an error occurred while advancing the cursor" — the two are indistinguishable without checking `Err()` explicitly. A fault partway through the by-source or by-variant result stream (a dropped connection, a corrupted page, any driver-level error mid-scan) is silently treated as "that's all the rows," and the response renders a truncated breakdown as if it were complete, with no signal that data is missing. This is distinct from bug #452, which was scoped to the top-level `.Query()`/`.QueryRow()` call failing outright before any row is read; #452 deliberately left per-row iteration alone.

**Fix:** add `if err := sourceRows.Err(); err != nil { return fmt.Errorf(...) }` (and the equivalent for `variantRows`) immediately after each loop, before `return nil`.


---

## 458. [The Gemini and OpenAI model columns have never been checked against a vendor catalogue](#458-the-gemini-and-openai-model-columns-have-never-been-checked-against-a-vendor-catalogue)

**Table rationale cell (original):** **Fixed 2026-07-30.** `agy models` confirmed `gemini-3.6-flash-high`, `gemini-3.1-pro-high` and `gpt-oss-120b-medium` live; `documentation/model_allowlist.md`'s provenance for all three now reads "Confirmed live via `agy models`, 2026-07-30". `gpt-5.6-terra`/`gpt-5.6-sol`/`gpt-5.6-luna` could not be closed the same way — this machine has no `OPENAI_API_KEY` (per `AGENTS.md`'s no-paid-keys-assumed constraint) and none of the three appear in `agy models`' output, so there is no catalogue reachable here to check them against. Their provenance now says so explicitly instead of repeating the old blanket "not vendor-verified" note. No Pending row in any of the three backlogs currently names an OpenAI-column model, so this gap has zero live blast radius today. Closing the remaining three fully requires an OpenAI API key, which is a user decision under this project's paid-key constraint

### 458. The Gemini and OpenAI model columns have never been checked against a vendor catalogue

**Filed 2026-07-30**, alongside #457. **Fixed 2026-07-30.**

#455 corrected the Claude column against an authoritative model catalogue. The Gemini and OpenAI columns were deliberately left untouched in that pass, because no equivalent catalogue was available to that session and correcting them from memory would have been the exact mistake being fixed — inventing a plausible ID and then reporting the column verified.

**What closing it actually found.** `agy models` (live, 2026-07-30) lists: `gemini-3.6-flash-high`, `gemini-3.6-flash-medium`, `gemini-3.6-flash-low`, `gemini-3.5-flash-high`, `gemini-3.5-flash-medium`, `gemini-3.5-flash-low`, `gemini-3.1-pro-high`, `gemini-3.1-pro-low`, `claude-sonnet-4-6`, `claude-opus-4-6-thinking`, `gpt-oss-120b-medium`. Both allowlisted Gemini values (`gemini-3.6-flash-high`, `gemini-3.1-pro-high`) and one of the four OpenAI-column values (`gpt-oss-120b-medium`, routed through Antigravity rather than a direct key) are confirmed present. `documentation/model_allowlist.md`'s provenance for all three now reads "Confirmed live via `agy models`, 2026-07-30".

**The other three OpenAI values could not be closed the same way, and not for lack of trying.** This machine has no `OPENAI_API_KEY` (`AGENTS.md`: paid keys are never assumed present), so there is no direct OpenAI catalogue to check against, and `gpt-5.6-terra`/`gpt-5.6-sol`/`gpt-5.6-luna` do not appear in `agy models`' output either — Antigravity does not proxy them. `bugs.md`'s own groom history (line 150) records a session that ran inline on `gpt-5.6-sol` directly against OpenAI, which is some evidence the name is real, but a prose note from a past session is exactly the kind of unverified claim this item exists to stop trusting. Their provenance in the allowlist now says precisely that — absent from every catalogue reachable on this machine, not merely "not vendor-verified" — rather than repeating the old blanket caveat.

**Blast radius today: zero.** `go test ./internal/backlog/` (`TestPendingBacklogRowsNameRealModels`) only enforces Pending rows, and no currently Pending row in any of the three backlog files names an OpenAI-column model — every citation of `gpt-5.6-*` is on a Done, Closed, or historical/prose row. So the three still-unverified values pose no live risk; they only need a real check once a Pending row actually names one, or once an OpenAI key exists to check them directly. #457's test would catch either a fabricated value in a new Pending row or an OpenAI key holder attempting to verify them from memory instead of a real catalogue.


---

## 457. [Enforce the model columns with a test instead of an instruction](#457-enforce-the-model-columns-with-a-test-instead-of-an-instruction)

**Table rationale cell (original):** #455 fixed the wrong model IDs; this fixes the reason nobody caught them. A fabricated ID survived four days and ~12 groom passes that each reported "every row re-verified", because re-verification meant re-running a count. Another prose instruction would have been the thirteenth. Added `internal/backlog` (four tests, run by `go test ./...`) validating every Pending row's model columns against `documentation/model_allowlist.md`, plus the allowlist itself with per-entry provenance. It parses the table **column by header name**, not by grep — grep is what invented `gemini-whatever-provider-is-configured`, a "value" that is really part of bug #444's anchor link, and what made #455 report 80 bad rows when there were 7. Verified by reintroducing the original defect and a date-suffixed variant; both fail with an actionable message

### 457. Enforce the model columns with a test instead of an instruction

**Found and fixed 2026-07-30**, immediately after #455.

#455 fixed the wrong values. It did not touch the reason nobody noticed them, which is the more expensive defect: **`claude-opus-4-6-thinking` sat in seven rows for four days across roughly a dozen groom passes, every one of which reported that it had re-verified every row.**

They were not lying. They re-ran the counts, and the counts were right. What none of them did was test the *claim* the counts supported. Writing a thirteenth instruction saying "check more carefully" would have been the same intervention that had already failed twelve times.

**What shipped.** `internal/backlog`, four tests that `go test ./...` already runs:

| test | what it holds |
| --- | --- |
| `TestPendingBacklogRowsNameRealModels` | every model named in a **Pending** row exists in `documentation/model_allowlist.md` |
| `TestLegacyModelLabelsDoNotGrow` | the 237 pre-2026-07-26 display-name cells on **closed** rows may shrink, never grow |
| `TestModelAllowlistEntriesCarryProvenance` | no allowlist entry without a sourced provenance cell |
| `TestBacklogModelColumnsAreParsedFromHeaders` | records executably why the column index is never hardcoded |

**Two design decisions worth keeping.**

*It parses the table column by header name, never by grep.* Grep on raw markdown cannot tell a model column from a title, an anchor, or a prose aside — and that is not hypothetical. #455 reported that a row's Gemini column "reads literally `gemini-whatever-provider-is-configured`". No such cell exists: the string is part of bug #444's *anchor link*. Grep invented it, and the same technique is why #455 put the blast radius at 80 rows when it was 7. Header-driven parsing also means bugs.md's extra Severity column is handled for free, and that a future column rename (#456) fails loudly instead of silently validating zero rows.

*It is scoped to Pending rows, with a ratchet behind it.* All 237 legacy labels are on closed rows, where the model column is a record of what was recommended then — the same category as the dated groom notes, and rewriting them would churn history for no benefit, since nothing routes a finished item. Scoping alone would have licensed writing junk into any closed row, so the ratchet holds the count flat and asks to be lowered whenever it drops.

**Verified by reintroducing the bug**, not by reasoning that it would work: putting `claude-opus-4-6-thinking` back into a Pending row fails with a message naming the file, line, item, column and provider, and so does the subtler `claude-opus-5-20260730` — a date-suffixed variant, which is a real hazard because Anthropic model IDs take no date suffix. Both restored to green afterwards.

**What this does and does not fix.** It makes one class of unverified claim mechanical. It does not make the general lesson mechanical, and no test can — the durable version lives in `groom_backlogs.md` now: *a number is evidence for the thing you counted, not the thing you concluded.* The useful corollary is narrower and actionable: **when a claim can be checked mechanically, a prose instruction to check it is a bug report, not a fix.**


---

## 456. [Replace the concrete model columns with capability tiers that cannot expire](#456-replace-the-concrete-model-columns-with-capability-tiers-that-cannot-expire)

**Table rationale cell (original):** **Fixed 2026-07-31**, after the user resolved both open questions: drop all three provider columns entirely (no hybrid free-text constraint column), and keep Tier and Effort as separate fields with their relationship documented rather than derived or merged. All three backlog tables (`bugs.md`, `improvements.md`, `improvements_paywall.md`) now carry a single `Tier` column (`mechanical`/`standard`/`deep-reasoning`) in place of `Claude model`/`Gemini model`/`OpenAI model`/`OpenAI task-fit reason`, derived per row from its prior Claude-model value (opus→deep-reasoning, sonnet→standard, haiku/fable→mechanical, no model→`—`). The now-obsolete "OpenAI task-fit model assignments" tables in `bugs.md` and `improvements.md` were removed. `internal/backlog/models_test.go` was rewritten to validate the closed tier vocabulary directly instead of checking cells against `documentation/model_allowlist.md`'s live catalogue — that file is retained as an unenforced reference for the Working Protocol's model-selection step, not a machine-checked allowlist, since tiers don't expire the way model IDs did (#455). Working Protocol step 3 (`improvements.md`) and the Ranked Backlog section now describe tier-based model selection and the Tier/Effort relationship (orthogonal axes, not meant to agree); `.agents/prompts/groom_backlogs.md`'s "Model fallback" paragraph and `.agents/prompts/work_next_item.md`'s delegate-selection step were updated to match. Found and fixed two unrelated pre-existing document defects while rewriting the exact table region: `improvements.md` row #23's line had been truncated mid-link and merged with the OpenAI task-fit section's intro sentence (restored from `git log -p`), and `bugs.md` row #44 had lost its ROI rationale cell entirely (also restored from history). `go build ./...`, `go vet ./...`, `go test ./...`, and `gofmt -l ./cmd ./pkg ./internal` all clean. **Follow-up same day:** the user asked whether a fresh session would actually know which model to invoke for a given tier — it would not have, cleanly. Added an explicit "Tier → model starting point" table to `documentation/model_allowlist.md` (concrete model per tier per provider, still unenforced/re-check-live), and fixed `groom_backlogs.md`'s fallback-ladder wording, which had bolted each tier onto a single rung as if the ladder were tier-partitioned rather than a real escalate-on-unavailability sequence within a chosen tier

### 456. Replace the concrete model columns with capability tiers that cannot expire

**Filed 2026-07-30**, out of #455 — which fixed the symptom and deliberately left the cause.

Every backlog row carries three columns naming a concrete model (`claude-*`, `gemini-*`, and often `—` for OpenAI). Concrete IDs expire. This backlog has now had them go stale at least twice, and #455 demonstrated that the failure is *silent*: an ID that does not exist sat in seven rows for days, because nothing reads the column mechanically and every human session probes availability live and ignores it. A column that is only correct because nobody relies on it will keep drifting, and each groom pass pays to re-check it.

**The proposal.** Replace the per-provider model columns with a single capability tier describing what the item actually needs:

| tier | meaning |
| --- | --- |
| *mechanical* | one-line or one-file change, verification is obvious |
| *standard* | ordinary implementation work with a clear spec |
| *deep-reasoning* | the difficulty is deciding what correct looks like, not typing it |

Tiers do not expire, survive a provider switch, and encode the judgment the model column was reaching for anyway.

**Two things to settle before doing it**, which is why this is a user decision and not autonomous work:

1. **Whether to keep any provider column at all.** Dropping all three loses the ability to say "this one specifically wants a vision model" or "this one needs a long context window". A hybrid — a tier column plus an optional free-text constraint — may be better than either extreme.
2. **The overlap with Effort.** The Effort column (1–8, log-scale) already carries part of the "how hard is this" signal. A tier column that disagrees with Effort is worse than no tier column. Reconcile the two, or derive the tier from Effort and drop it as a stored field.

The edit spans the table schema in all three backlog files plus the Working Protocol's model-selection step in `improvements.md`, and `groom_backlogs.md`'s model-fallback ladder refers to the columns by name.

**Resolved 2026-07-31** — see this item's row in the Ranked Backlog table above for what shipped and both user decisions it was blocked on.


---

## 455. [The Claude model column named one ID that does not exist and one that is a generation behind](#455-the-claude-model-column-named-one-id-that-does-not-exist-and-one-that-is-a-generation-behind)

**Table rationale cell (original):** Found 2026-07-30 while answering a question about routing each backlog item to the model its row names. **The original report was half wrong.** It claimed `claude-sonnet-4-6` "is not in the current lineup" and counted 80 stale rows; checked against the authoritative model catalogue, `claude-sonnet-4-6` is current and active — previous-generation Sonnet, still served. The real defect was narrower: **`claude-opus-4-6-thinking` (7 rows) is not a model ID at all** — no `-thinking` suffix exists in the lineup, and thinking is now a request parameter (`thinking: {type: "adaptive"}`, on by default for Opus 5), not part of an ID. So 7 rows would have failed, not 80. Fixed by rewriting 79 item rows: `claude-opus-4-6-thinking` → `claude-opus-5`, `claude-sonnet-4-6` → `claude-sonnet-5`, scoped to `^ | [0-9]+ | ` so dated groom notes were left as historical record. **Gemini/OpenAI columns deliberately untouched** — no authoritative catalogue for them this session, and the grounding protocol says ask rather than guess. The recurring-cost question is now #456

### 455. The Claude model column named one ID that does not exist and one that is a generation behind

**Found 2026-07-30**, while the user asked whether a session should run a cheap orchestrator and then invoke, per item, the Claude model each backlog row names. **Resolved 2026-07-30** in the same pass.

**The original report was half wrong, and the half it got wrong matters.** It claimed that `claude-sonnet-4-6` "is not a model in the current lineup" and counted it among 80 stale rows. Checked against the authoritative model catalogue rather than against memory, `claude-sonnet-4-6` **is** current and active — it is the previous-generation Sonnet, still served, with `claude-sonnet-5` as its documented successor. Naming it would have routed fine.

The genuine defect was narrower and worse:

| value | item rows | verdict |
| --- | --- | --- |
| `claude-sonnet-4-6` | 72 | **Valid**, but a generation behind — superseded by `claude-sonnet-5` |
| `claude-opus-4-6-thinking` | 7 | **Not a model ID at all.** No `-thinking` suffix exists anywhere in the lineup; this string would 404 |
| `claude-opus-5` | 7 | Current, left alone |

So the count of rows that would actually fail was **7, not 80** — and those 7 failed for a reason the original report never identified. `claude-opus-4-6-thinking` looks plausible because extended thinking was once a request-level toggle, but it was never part of a model ID. On the current lineup thinking is a `thinking: {type: "adaptive"}` request parameter and is on by default for Opus 5, so the capability that suffix was reaching for is now simply what `claude-opus-5` does.

**What shipped.** 79 item rows across all three backlogs were rewritten: `claude-opus-4-6-thinking` → `claude-opus-5` (the deep-reasoning tier, which is what the suffix meant), and `claude-sonnet-4-6` → `claude-sonnet-5`. The edit was scoped to lines matching `^| [0-9]+ |` so that dated historical groom notes and this item's own evidence tables were not rewritten — a dated note is a record of what was true then, not a claim about now.

**Deliberately not changed: the Gemini and OpenAI columns.** The original report asserted they are "in the same state". That may well be true, but this session had an authoritative catalogue for Anthropic models and none for the other two providers, and the grounding protocol says to ask rather than guess. Those columns are left as they are; **confirming or correcting them is open work and needs a live probe** (`agy models`, and whatever the OpenAI equivalent is on this machine).

**The recurring-cost question is still open, and is the more interesting half.** Concrete model IDs expire; this backlog has now had them go stale at least twice. The alternative is to replace the names with a capability tier the row actually means — *mechanical* (one-line change, obvious verification), *standard*, *deep-reasoning* (the difficulty is in deciding what correct looks like, not in typing it). Tiers do not expire and survive a provider switch. That was not done here because it changes the table schema across three files and is a user decision, not a mechanical one. Whoever takes it should note that the Effort column already carries part of this signal, so the two should be reconciled rather than left to disagree. **Filed as #456.**

**The lesson, which is this backlog's recurring one in a new costume.** Every prior instance was "do not trust a Done note / a bug report / a row's arithmetic." This one is: *a report that counts something is not thereby verified.* #455 counted precisely — 73, 7, 4 — and the precision made the conclusion feel checked, when the actual claim being made ("not in the current lineup") had never been tested against a catalogue at all. **A number is evidence for the thing you counted, not for the thing you concluded.**


---

## 454. [Nothing in the Working Protocol updates `CHANGELOG.md`, and it drifted a full day](#454-nothing-in-the-working-protocol-updates-changelogmd-and-it-drifted-a-full-day)

**Table rationale cell (original):** Found 2026-07-30 while auditing the docs after #446. `grep -c "2026-07-30" CHANGELOG.md` returned **0**, though five bug fixes had shipped that day (#436, #437, #441, #445, #446) across nine commits. The file's most recent entry was 2026-07-29. The cause is structural rather than anyone's oversight: the Working Protocol's close-the-loop step names the backlog row, the journal, the commit and the push, and never names `CHANGELOG.md`, so a session that follows the protocol exactly still leaves it stale. **Fixed:** step 7 of the Working Protocol (`improvements.md`) now requires a dated `CHANGELOG.md` entry in the same commit for any user-visible change, scoped to exclude internal refactors, backlog-only edits, and ignored scripts — the distinction the row's own Fix note asked for. `bugs.md` shares this protocol by reference, so no duplicate edit was needed there

### 454. Nothing in the Working Protocol updates `CHANGELOG.md`, and it drifted a full day

**Found 2026-07-30** while auditing the documentation after bug #446.

`grep -c "2026-07-30" CHANGELOG.md` returned **0**. Five bug fixes had shipped that day across nine commits — #436 (fresh clones could not build), #437 (the dashboard's deleted conversion analytics), #441 (installer and `.env.example` naming different models), #445 (cross-origin start/stop), #446 (the shared DSN) — and the changelog's most recent entry was still 2026-07-29.

**The cause is structural, not anybody's oversight.** The Working Protocol's close-the-loop step enumerates what a finished item must leave behind: the backlog row set to `Done`, the journal deleted, the verification run, the commit, the push. It does not name `CHANGELOG.md`. A session can follow the protocol perfectly and still leave the changelog a day stale, and five consecutive sessions did.

This is a close relative of what #443 found about `gofmt`: a check nobody runs is a check that does not exist, and here the artifact nobody updates is the one aimed squarely at people who were not in the session. A changelog silently one day behind is arguably worse than no changelog, because its top entry presents itself as current.

The 2026-07-30 section was written by hand this session from `git log`, which repairs the symptom and leaves the cause in place.

**Fix:** add the changelog to the close-the-loop list in the Working Protocol, so an item is not Done until its user-visible change is recorded there. Worth deciding at the same time whether every item earns an entry or only user-visible ones — a row like #440 (`//go:build ignore` on an unused script) plainly does not.

**Fixed 2026-07-30.** Step 7 of the Working Protocol (`improvements.md`) now requires a dated `CHANGELOG.md` entry in the same commit whenever the change is user-visible — explicitly excluding internal refactors, backlog-only edits, and ignored/unused scripts, which answers the open question the row left about #440-shaped items. `bugs.md`'s protocol intro already states it shares this file's Working Protocol by reference, so the fix is a single edit rather than two. This entry in `CHANGELOG.md` is itself the first one written under the new rule.


---

## 450. [The shared DSN's `journal_mode(WAL)` can fail a connection outright, and `busy_timeout` does not cover it](#450-the-shared-dsns-journal_modewal-can-fail-a-connection-outright-and-busy_timeout-does-not-cover-it)

**Table rationale cell (original):** **Fixed 2026-07-31.** New `storage.ReaderDSN` drops `journal_mode` and is now what `cmd/dashboard` opens with; `storage.DSN` (unchanged) stays the writer DSN used by `storage.InitDBWithPath`. See detail section for the fix, its mutation-checked reproduction test, and its live verification against the real database with the production agent running concurrently

### 450. The shared DSN's `journal_mode(WAL)` can fail a connection outright, and `busy_timeout` does not cover it

**Found 2026-07-30** during bug #446's live verification — specifically, by running the experiment that was meant to *confirm* the fix and getting the opposite result.

The setup: a database in `delete` journal mode, another process holding an open write transaction, and two readers opened concurrently.

```
=== database journal_mode=delete ===
post-fix (storage.DSN)       FAILED after 1ms   err=database is locked (5) (SQLITE_BUSY)
pre-fix  (go-sqlite3 DSN)    ok     after 1ms   count=3

=== database journal_mode=wal ===
post-fix (storage.DSN)       ok     after 1ms   count=3
pre-fix  (go-sqlite3 DSN)    ok     after 1ms   count=3
```

The corrected DSN loses where the broken one wins. The reason is that the shared DSN asks to *change* the journal mode on connect, and **SQLite refuses a journal-mode change while another connection is active — `busy_timeout` does not apply to it.** The pre-fix DSN "won" only because it asked for nothing, which is not a virtue; it is the defect #446 was filed about.

Pragma ordering was tested as the obvious hypothesis and ruled out. Both `_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)` and `_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)` fail identically, twice each.

**Scope and severity.** This is pre-existing `pkg/storage` behaviour that #446 propagated to `cmd/dashboard`; it was not introduced by that fix, and `cmd/agent` has been carrying it since #416. It is unreachable in the steady state, because the database is always WAL once either command has opened it — and after #446 both of them set it. It becomes reachable only on a genuinely fresh database being opened by two processes at the same moment.

**Fix direction:** stop asking readers to set `journal_mode` at all and let the writer own it — the reader keeps `busy_timeout` and the cache/temp settings, which are per-connection anyway. That is a small change with a real question attached: which connection is the designated writer, and what should happen when a database does not exist yet and the reader is first? Answering it is most of the effort.

**Fixed 2026-07-31.** The designated writer is whichever connection calls `storage.InitDBWithPath` (every command in this repo except `cmd/dashboard`, which deliberately keeps its own handle) — it is the one that runs `CREATE TABLE IF NOT EXISTS` and owns schema setup, so it keeps `storage.DSN` unchanged, `journal_mode` and all. New `storage.ReaderDSN` carries every other pragma (`synchronous`, `busy_timeout`, `cache_size`, `temp_store`) and drops `journal_mode`; `cmd/dashboard/main.go`'s `dashboardDSN` now calls it instead of `DSN`. On the "reader is first" question: an empty database opened by `ReaderDSN` alone just stays in SQLite's default journal mode until a writer connects and sets WAL, which is harmless — the reader was never going to find any tables yet either way, since only the writer runs `CREATE TABLE`. **Reproduced and mutation-checked**: a new `pkg/storage` test (`TestReaderDSNOpensAgainstAFreshDatabaseWithAnActiveWriterTransaction`) recreates the row's own experiment — a fresh, default-journal-mode database with a separate connection holding an open `BEGIN IMMEDIATE` transaction — and confirms `DSN` still fails with `SQLITE_BUSY` there (the negative control) while `ReaderDSN` opens cleanly; temporarily reintroducing `journal_mode` into `readerPragmas` makes both this test and a plain pragma-content test fail with the exact symptom described, confirming they'd catch a regression. `cmd/dashboard`'s own `dsn_test.go` gained `TestDashboardDSNNeverSetsJournalMode`. Live-verified: a second dashboard instance built from the fixed tree served real, correctly-typed data from `/api/metrics` on `127.0.0.1:8099` against the actual `applications.db` **while the production `cmd/agent` daemon was running concurrently** — a genuine live instance of the reader/writer contention this row is about. The production dashboard on `:8080` was untouched throughout. `go build ./...`, `go vet ./...`, `go test ./...` (including `-race` on `pkg/storage`), and `gofmt -l ./cmd ./pkg ./internal` all clean.


---

## 443. [Eight Go files are not `gofmt`-clean, and nothing in the verification loop notices](#443-eight-go-files-are-not-gofmt-clean-and-nothing-in-the-verification-loop-notices)

**Table rationale cell (original):** **Fixed 2026-07-30.** `gofmt -w` applied to all 8 files; `gofmt -l ./cmd ./pkg ./internal` is now empty. `AGENTS.md`'s documented verification loop gained a fourth `gofmt -l` step, with a one-line note on what it catches that the other three don't. **Re-verification found the row's own diff description had drifted**: it claimed all 8 files were "trailing whitespace on otherwise-blank lines, 16 lines"; re-running `gofmt -d` today showed 5 files matched that (`backoff.go`, `dom_test.go`, `funnel.go`, `browser.go`, `bufferpool.go`), but `cmd/reconcile/main.go` had a doc-comment list-style reformat and `pkg/scraper/hackernews.go`/`scraper.go` had import-block reordering — a gofmt/toolchain version change since the row was filed, not a new defect. The fix and its risk were unaffected; only this description needed correcting. `go build ./...`, `go vet ./...`, `go test ./...` all clean, `gofmt -l` empty. Original report follows. Found 2026-07-30 while formatting the files touched for bug #441. `gofmt -l ./cmd ./pkg` names eight files; every diff is trailing whitespace on otherwise-blank lines, 16 lines across the eight files. Nothing is broken and `go vet` is silent, because formatting is not what vet checks — the project's stated loop is `go build` / `go vet` / `go test`, none of which reads formatting. The cost is diff noise: any future edit near one of those lines carries an unrelated whitespace change, which is exactly the kind of collateral that hid what #437 and #439 dropped. One `gofmt -w` plus adding the check to the documented loop

### 443. Eight Go files are not `gofmt`-clean, and nothing in the verification loop notices

**Filed 2026-07-30** while fixing bug #441. `cmd/agent/main.go` turned out to be unformatted before any of that session's edits — a single trailing blank line at EOF — which prompted running `gofmt -l` across the tree for the first time in a while.

`gofmt -l .` names **16** files. Eight are under `cmd/` and `pkg/`:

| file | unformatted lines |
| --- | --- |
| `cmd/reconcile/main.go` | 2 |
| `pkg/mcp/backoff.go` | 2 |
| `pkg/parser/dom_test.go` | 1 |
| `pkg/scraper/funnel.go` | 1 |
| `pkg/scraper/hackernews.go` | 3 |
| `pkg/scraper/scraper.go` | 4 |
| `pkg/submitter/browser.go` | 1 |
| `pkg/util/bufferpool.go` | 2 |

The other eight are the `//go:build ignore` scripts (`assess.go`, `audit_jobs.go`, `explore_data.go`, `queue_analysis.go`, `requeue_jobs.go`, `sanitize_jobs.go`, `server.go`, `source_health.go`), which matter less — they are excluded from the module build, except `server.go`, which is bug #440.

Every one is the same thing: whitespace on a line that is otherwise blank. 16 lines across the eight compiled files. Nothing is broken, the build is green, and `go vet` is silent — **because formatting is not what vet checks.** The project's documented loop in `AGENTS.md` is `go build ./...`, `go vet ./...`, `go test ./...`, and none of the three reads formatting, so this drift is invisible to the only gate anyone runs.

Why it is worth 10 minutes rather than zero: an edit anywhere near one of those lines silently carries an unrelated whitespace change into the diff. This repository has now been bitten three times (#437, #438, #439) by *a diff nobody read closely enough*, and its own conclusion was that the only safe review of a rewrite is a line-by-line diff against what it replaced. Noise in that diff is not free.

The work: `gofmt -w` all sixteen as one whitespace-only commit — reviewable at a glance precisely because it touches nothing else — and add `gofmt -l ./cmd ./pkg` (empty output required) to the verification loop in `AGENTS.md` so the next drift is caught by the gate rather than by accident. Value 2, Effort 1: trivially small, but it makes a check exist where there was none.


---

## 429. [Rewrite data ingestion CLI tools in Zero](#429-rewrite-data-ingestion-cli-tools-in-zero)

**Table rationale cell (original):** Utilize the Zero transpiler's new direct string `parse_json` support (added 2026-07-28) to build robust API-fetching data ingestion scripts.

### 429. Rewrite data ingestion CLI tools in Zero
**Background:** The upstream Zero transpiler recently added support for using `parse_json` on strings and byte arrays (commit `f7848d5`), expanding its capability beyond just HTTP request bodies. This unlocks the ability to use Zero for API clients, config parsers, or log aggregators without needing heavy boilerplate Go structs for every layer of nesting.
**Acceptance criteria:** Identify a Go-based data ingestion or API-fetching script within the project that parses JSON strings. Rewrite it into a `.zero` script that leverages `parse_json`, transpile it to Go, and verify it maintains parity while reducing boilerplate.


---

## 428. [Expand usage of Zero transpiler for analytics and tooling](#428-expand-usage-of-zero-transpiler-for-analytics-and-tooling)

**Table rationale cell (original):** The Lisp-like AI-first Zero transpiler (already used in `metrics_summary.zero`) is perfect for rapidly generating robust Go utility scripts for queue analysis and data exploration without writing boilerplate Go.

---

## 427. [Python NLP Microservice for Resume Tailoring](#427-python-nlp-microservice-for-resume-tailoring)

**Table rationale cell (original):** Offload complex ML pipelines and resume tailoring text-structuring to a specialized Python microservice. Implemented a FastAPI microservice in `nlp_service` and refactored `ProcessJobApplication` in Go to offload LLM calls. **2026-07-29 groom correction:** this row described what was added and nothing about what it removed. The refactor deleted a working provider-agnostic in-process implementation and replaced it with a hard dependency on a manually-started service that hardcoded `ollama`/`llama3`, dropping the payload circuit breaker, `[API Metrics]` logging, dynamic `num_ctx`, the provider abstraction, the 120-minute Ollama timeout (reintroducing bug #6) and four prompt instructions. The path could not succeed at all on this host and plausibly never ran. Bug #439 restored all of it and reduced this feature to an opt-in offload behind `NLP_SERVICE_URL`; whether it is worth keeping at all is now #442

---

## 426. [TypeScript React/Vue Dashboard Rewrite](#426-typescript-reactvue-dashboard-rewrite)

**Table rationale cell (original):** Replace the Go template dashboard with a modern TypeScript frontend (React/Vite) for better live-updating metrics and visualization. **2026-07-29 groom correction:** the rewrite replaced an 831-line template with a 137-line app rendering six count tiles, silently dropping #15's conversion analytics and #34's accessibility markup (bug #437), four tests including both loopback-exposure guards (bug #438, fixed), and leaving `go:embed ui/dist` pointing at a gitignored path so no fresh clone could build (bug #436, fixed). **2026-07-30:** bug #437 is now fixed too, so every regression this rewrite introduced has been repaired and improvements #13, #15 and #34 are genuinely Done again. The rewrite itself stands; it took three separate bugs across two days to make its Done note true

---

## 425. [Memory Profiling & sync.Pool Implementation](#425-memory-profiling--syncpool-implementation)

**Table rationale cell (original):** Leverage `sync.Pool` for byte buffers and pre-allocate slices across all API responses (e.g., in `mcp` logic) to reduce GC overhead.

---

## 423. [Human-in-the-Loop "Copilot" Mode](#423-human-in-the-loop-copilot-mode)

**Table rationale cell (original):** Shipped `copilot_mode` in `profile.yaml`: fills the form completely, stops before the final click, and routes the job to `AWAITING_REVIEW` with its documents and a `copilot_queue.md` entry. One shared `submitGate` serves all six submit sites. Also closed bug #432 (`auto_submit_click: false` was recording false `APPLIED` rows) with the same mechanism

### 423. Human-in-the-Loop "Copilot" Mode

**Filed with a table row but no Details section.** Written up 2026-07-29 when the item was picked up, from a fresh read of the submit path rather than from the row's one-line summary.

The agent had exactly two submission postures, and neither was "let me look first":

- `auto_submit: false` (`cmd/agent/pipeline.go:318`) stops *before tailoring* — status `PROCESSED_MANUAL`, no documents generated, nothing for a human to finish.
- `auto_submit_click: false` fills the whole form and skips the click, which is the right shape — but it returned `nil`, and the pipeline read `nil` as success. That was bug #432, fixed here.

Copilot Mode is the missing third posture: **do all the expensive work, stop at the one irreversible step.** The agent discovers, scores, tailors, opens the form, fills every field it can and resolves validation errors — then, instead of clicking Submit, records the job as `AWAITING_REVIEW`, moves the generated documents into the manual-apply folder, and appends a row to `applications/needs_manual_apply/copilot_queue.md` with the apply URL and document paths so the user can open it and finish by hand.

**Why it is worth building.** Every automated submit is a bet against bot protection: the 2026-07-26 monitoring cohort measured 6 of 7 fully-filled forms blocked *after* submit, and the four-board CAPTCHA ceiling in `bugs.md` is still the dominant live constraint. A human clicking the final button in their own browser session is the one path bot protection is not designed to stop. Copilot Mode converts a class of guaranteed-blocked applications into ones the user can actually land, without discarding the ~10 minutes of local inference already spent on scoring and tailoring.

**Design as shipped.** A single `submitGate(copilotMode, autoSubmitClick) error` helper in `pkg/submitter/browser.go` is consulted at all six submit-click sites plus `confirmOrError`, rather than each handler testing the flags itself. This is deliberate: the single most repeated structural defect in this project's history is a capability wired into one fill path and missed in the others (#65/#66→#67, #74→#75, #28→#31 — three separate instances). A shared gate makes that class of drift impossible by construction. The gate returns sentinel errors (`ErrAwaitingHumanReview`, `ErrSubmitClickDisabled`) matching the existing `ErrAuthWall`/`ErrCaptchaBlocked` pattern, which `cmd/agent/pipeline.go` branches on before its generic-error arm.

**The non-obvious half, found in review of the delegate's implementation and worth recording.** Threading the gate through the handlers is the easy part; the sentinels then travel through the *same error values that fill failures travel through*, and every recovery path in `AttemptSubmit` reacts to them. Left alone, that made Copilot Mode actively destructive rather than merely incomplete:

- the cached-mapping path (`browser.go:1207`) and the Learner Module path read any non-nil return as "this mapping is stale", called `storage.DeleteFormMapping(domain)`, and fell back to a Vision call — so **every board the agent had already learned would be un-learned and re-inferred, on every job**;
- the retry loop's `ErrCaptchaBlocked` rewrap formats the underlying error with `%v`, discarding the sentinel's identity, so a perfectly healthy board would be filed as `BLOCKED_CAPTCHA`.

`isSubmitGated(err)` short-circuits all three sites ahead of any recovery logic. `TestSubmitGateResultsAreGated` pins the two predicates together so a future gate reason added to one and not the other cannot silently reintroduce either behaviour. This is the same lesson as #76: the delegate's own tests passed and the build was clean while both defects were live.

**What Copilot Mode actually delivers, and the claim that had to be retracted before shipping.** The first version of this feature's documentation — and the header written into `copilot_queue.md` — told the user to "open each URL to review the pre-filled form and click submit." **That was false, and a review pass caught it before push.** `AttemptSubmit` fills the form inside an ephemeral Playwright context created at `browser.go:1106` and closed by `defer session.Close()` on return. When the gate fires, that context is destroyed: no cookies, no storage state, no user-data-dir, and headless in the default configuration. The user opens the link and gets a blank form. Every field, combobox and upload the agent committed is gone.

What genuinely carries over is still worth the run, and is what the docs now say: the job was **scored as a real fit**, a **tailored resume and cover letter** were written for it, and the form was **proven reachable and fillable** rather than dead, auth-gated, or bot-blocked. That is the expensive part on this CPU-only host. The typing is not.

Preserving the fill itself would mean either launching a non-headless browser the user drives (headless is a process-wide launch decision at `cmd/agent/main.go:818`, and the worker pool runs many jobs concurrently) or exporting and re-importing storage state into the user's own browser. Both are real features, neither is a documentation fix, and guessing at one here would have been worse than being accurate about what ships. **The lesson is the one this backlog keeps re-learning from the other direction:** the feature worked, the tests passed, the build was clean — and the thing being promised to the user still wasn't happening. It took reading the object lifetime, not the control flow, to see it.

**Configuration trap, documented rather than redesigned.** `copilot_mode: true` does nothing unless `auto_submit: true` is also set. `auto_submit` is checked at `cmd/agent/pipeline.go:318`, *before* tailoring: with it off, a job stops at `PROCESSED_MANUAL` immediately after scoring, with no documents generated and no browser ever opened. So the two settings a cautious user would naturally combine — "don't auto-submit, and let me review" — silently produce nothing to review. The correct pairing is `auto_submit: true` + `copilot_mode: true`, i.e. "do all the work, submit nothing." This is documented in `README.md` and inline in `profile.yaml` rather than fixed by renaming, because `auto_submit`'s existing meaning is load-bearing for anyone already relying on it; renaming it is a separate, breaking change that should be its own item if the confusion recurs.

**Not built, deliberately:** a blocking interactive prompt, or a dashboard approve-button that resumes a live browser session. The agent and dashboard share only `applications.db` with no IPC, `headless_browser` is a process-wide launch decision (`cmd/agent/main.go:818`), and `ReapStaleProcessingJobs` would reclaim any job parked mid-flight. A queue-and-hand-off design needs none of that and degrades safely if the agent is killed. If live use shows the user wants to drive the same browser session, that is a follow-up with real evidence behind it, not a guess now.


---

## 422. [Vector-Based Job Matchmaking](#422-vector-based-job-matchmaking)

**Table rationale cell (original):** Implemented continuous matching pipeline via `runContinuousJobMatching` goroutine inside `cmd/agent/main.go` that loops and vectors missing fit_similarity against user profile embeddings.

---

## 421. [Pre-Submission Keyword Gap Analysis](#421-pre-submission-keyword-gap-analysis)

**Table rationale cell (original):** Identify and flag missing keywords between USER_PROFILE.md and job description before generating final docs.

---

## 420. [Pre-Mapped ATS Selectors (Accuracy & Speed)](#420-pre-mapped-ats-selectors)

**Table rationale cell (original):** Implemented fillPreMappedATSSelectors for Greenhouse, Lever, and Ashby forms to automatically inject common links and basic inputs before fallback to LLM overhead.

---

## 419. [Use defer for resp.Body.Close() in scraper.go](#419-use-defer-for-respbodyclose-in-scrapergo)

**Table rationale cell (original):** In `pkg/scraper/scraper.go`, `resp.Body.Close()` is called explicitly. While it doesn't leak on the happy path, using `defer` provides panic and early-return safety.

### 419. Use defer for resp.Body.Close() in scraper.go
In `pkg/scraper/scraper.go`, `resp.Body.Close()` is called explicitly instead of using `defer`. While it doesn't currently leak in the happy path, it lacks panic/early-return safety and is non-idiomatic Go. Wrapping it in a `defer` immediately after successful response checking will ensure proper resource management.


---

## 418. [Parallelize DiscoverJobs queries in funnel.go](#418-parallelize-discoverjobs-queries-in-funnelgo)

**Table rationale cell (original):** `DiscoverJobs` in `pkg/scraper/funnel.go` runs SerpApi and Yahoo fallback queries in deeply nested sequential loops over `Roles` and `TargetATS`. Parallelizing this would significantly speed up discovery.

### 418. Parallelize DiscoverJobs queries in funnel.go
`DiscoverJobs` in `pkg/scraper/funnel.go` runs SerpApi and Yahoo fallback queries in deeply nested sequential loops over `Roles` and `TargetATS`. Parallelizing this execution with bounded worker pools or `errgroup` would significantly speed up discovery performance.


---

## 415. [UI Overhaul & Agent Start/Stop Controls](#415-ui-overhaul--agent-startstop-controls)

**Table rationale cell (original):** Essential for user control over the autonomous agent. Requires new API endpoints to manage agent processes, and a complete aesthetic revamp of the dashboard.

---

## 412. [Implement Exponential Backoff with Jitter for LLM API Retries](#412-implement-exponential-backoff-with-jitter-for-llm-api-retries)

**Table rationale cell (original):** API retries currently use fixed 60-second loops which can cause thundering herd contention.

### 412. Implement Exponential Backoff with Jitter for LLM API Retries

**Shipped 2026-07-29:** Replaced the flat `time.Sleep(60 * time.Second)` loops in `cmd/agent/pipeline.go` and `cmd/rankjobs/main.go` with an exponential backoff strategy (`mcp.ExponentialBackoff`). The new function uses a base of 10s up to a maximum of 120s with a +/- 20% jitter.

	API retries currently use fixed `time.Sleep(60 * time.Second)` loops on network/429 errors. Replace flat 60-second sleeps with exponential backoff and jitter to prevent thundering herd contention across workers.


---

## 411. [Implement Stateful Graph-Based Pipeline Architecture](#411-implement-stateful-graph-based-pipeline-architecture)

**Table rationale cell (original):** Move from sequential loops to a state machine (like LangGraph) for robust error handling on multi-step forms.

### 411. Implement Stateful Graph-Based Pipeline Architecture
**Background:** Current logic relies on simple query-response loops for navigating complex forms. Adopting a stateful graph-based state machine (similar to LangGraph) combined with a multi-agent orchestration approach would significantly increase resiliency against UI changes and complex branching paths on job boards.
**Acceptance criteria:** Refactor the core pipeline to handle application state using a directed graph, enabling branching, loops, and targeted retries. Design clear boundaries between Discovery, Scoring, Tailoring, and Submission sub-agents for parallel processing and modular updates.
**Done 2026-07-28:** Created a `graph` package that implements a stateful graph-based pipeline execution engine. Refactored the core loop in `cmd/agent/main.go` and `cmd/agent/pipeline.go` to use this new directed graph state machine. Replaced the monolithic sequential job processing loop with discrete, encapsulated states (`StateInit`, `StateDiscovery`, `StateScoring`, `StateTailoring`, `StateSubmission`), establishing clear boundaries for parallel processing and modular updates.



---

## 410. [AI Processing Optimizations (Concurrency & Context Limits)](#410-ai-processing-optimizations-concurrency--context-limits)

**Table rationale cell (original):** Split monolithic generation into concurrent goroutines, inject dynamic num_ctx, and add keep_alive to prevent cold starts.

### 410. AI Processing Optimizations (Concurrency & Context Limits)
**Background:** The AI integration (`pkg/mcp/`) features a monolithic generation sequence that packs resume, cover letter, and interview prep all into one LLM call, severely degrading output speed due to quadratic attention scaling. Furthermore, Ollama model unloading and truncated `num_ctx` on large DOMs lead to performance hits and hallucinated selectors.
**Acceptance criteria:** Split `ProcessJobApplication` into concurrent goroutines with targeted prompts. Inject dynamic `num_ctx` limits for large inputs and add `"keep_alive": "30m"` to prevent cold-start penalties when models unload between navigation gaps.
**Done 2026-07-28:** Split `ProcessJobApplication` into 3 concurrent goroutines with targeted system prompts. Injected dynamic `num_ctx` limit calculations (from 8192 to 64000/128000 depending on the size of the input) for generation calls, and added `keepAlive: "30m"` to all LLM calls to prevent cold-start penalties during navigation gaps.


---

## 409. [Frontend Rendering Speed Optimizations](#409-frontend-rendering-speed-optimizations)

**Table rationale cell (original):** Remove expensive CSS blurs, SVG noise, infinite animation loops, and scroll thrashing.

### 409. Frontend Rendering Speed Optimizations
**Background:** The dashboard frontend (`docs/style.css` and `docs/script.js`) has significant rendering bottlenecks, including a 120px CSS blur filter over large elements, full-screen SVG noise leading to high CPU/GPU overhead, an infinite `requestAnimationFrame` loop that runs even when idle, and scroll event thrashing.
**Acceptance criteria:** Replace the CSS blur with a base64-encoded SVG or PNG background. Replace the SVG turbulence filter with a tiled static noise PNG. Add a delta threshold check in the animation loop to break early when idle. Switch scroll event listeners to an `IntersectionObserver`.

**Done 2026-07-28:** Replaced CSS blur with base64 SVG radial gradients, replaced SVG turbulence with base64 noise PNG, added delta threshold check to `requestAnimationFrame` loop in `script.js` to break when idle, and switched scroll event listener to an `IntersectionObserver`.


---

## 407. [Utilize goroutines and bounded worker pools for dashboard queries and embedding inference](#407-utilize-goroutines-and-bounded-worker-pools-for-dashboard-queries-and-embedding-inference)

**Table rationale cell (original):** Dashboard queries now use errgroup; rankjobs uses a bounded worker pool for concurrent embeddings.

### 407. Utilize goroutines and bounded worker pools for dashboard queries and embedding inference
**Background:** Both `cmd/dashboard` and `cmd/rankjobs` suffer from missed concurrency opportunities. The `/api/metrics` dashboard sequentially fires eight `COUNT(*)` queries along with other independent UI stat fetches. `rankjobs` retrieves embeddings one by one from Ollama (`client.GetEmbedding`), which could be parallelized against robust APIs or local LLMs.
**Acceptance criteria:** Collapse sequential `COUNT(*)` queries into a single SQLite conditional aggregation query where applicable. Execute independent dashboard metrics queries concurrently via `errgroup`. Introduce a configurable bounded worker pool in `rankjobs` (e.g., 2-4 concurrent workers) to fetch `fit_similarity` embeddings and handle updates concurrently without overwhelming local hardware.


---

## 406. [Implement concurrent HTTP execution for ATS and job-board discovery scrapers](#406-implement-concurrent-http-execution-for-ats-and-job-board-discovery-scrapers)

**Table rationale cell (original):** Discovery sequentially blocking on every ATS board or page takes vastly longer. Unlocking IO bound scraping shrinks pipeline duration from hours to minutes.

### 406. Implement concurrent HTTP execution for ATS and job-board discovery scrapers
**Background:** Discovery pipelines (like `discoverWithATSFeeds`, `discoverWithRemoteOK`, and `fetchHNThreadComments`) process up to hundreds of HTTP pages, roles, or endpoints sequentially. Given a 20-second timeout per ATS board, sequential polling can block execution for extended periods, slowing down the discovery phase artificially.
**Acceptance criteria:** Wrap HTTP scraper requests in concurrent execution primitives (such as `golang.org/x/sync/errgroup` or a `sync.WaitGroup` pool). For Hacker News, fetch the initial page to extract `NbPages`, then fetch the remaining pages concurrently. For ATS feeds and RemoteOK, dispatch polling tasks in parallel and safely aggregate the responses using mutexes or atomics. Validate with testing that data parity remains exact but discovery wall-time is slashed.


---

## 405. [Refactor DOM parsing pipeline to use single AST pass and IO streams](#405-refactor-dom-parsing-pipeline-to-use-single-ast-pass-and-io-streams)

**Table rationale cell (original):** Refactored PruneDOMToText to accept io.Reader, added strings.Builder to minimize allocations. Consolidated PruneDOMToForm and StripPresentationalAttrs into a single AST pass. Streamed up to 10MB HTTP payload into HTML parser via io.TeeReader to eliminate 10MB string allocation.

### 405. Refactor DOM parsing pipeline to use single AST pass and IO streams
**Background:** `pkg/parser/dom.go` and `cmd/agent/main.go` currently suffer from excessive memory allocations and redundant HTML parsing. HTTP responses up to 10MB are buffered fully into strings before being processed. Moreover, `PruneDOMToForm` and `StripPresentationalAttrs` run sequential, redundant `html.Parse` and `html.Render` cycles on the same HTML block. Small tight loops (like text node concat) cause thousands of intermediate heap allocations.
**Acceptance criteria:** Refactor `parser.PruneDOMToText` to accept an `io.Reader` directly from the HTTP response body to eliminate the 10MB string buffer. Combine the multiple pruning and stripping steps in `dom.go` into a single AST traversal pass that renders exactly once. Refactor text node aggregations to use `buf.WriteString(text)` and `buf.WriteByte(' ')` instead of string concatenation. Test that DOM parsing outputs are identical but heap profile footprint drops significantly.


---

## 404. [Batch SQLite writes into explicit transactions for performance](#404-batch-sqlite-writes-into-explicit-transactions-for-performance)

**Table rationale cell (original):** SQLite single unbatched writes create massive disk fsync overhead. Wrapping mass state-updates into explicit transactions will dramatically cut DB-bound latency.

### 404. Batch SQLite writes into explicit transactions for performance
**Background:** The current SQLite implementation in `pkg/storage/manager.go` frequently runs unbatched `db.Exec()` insertions and updates. In SQLite, each individual unbatched statement creates an implicit transaction that triggers a full disk sync (`fsync`), capping write throughput and causing bottlenecks under load.
**Acceptance criteria:** Audit write-heavy operations (like processing email outcomes or iterative job migrations) in `manager.go`. Refactor loops that update or insert multiple records to run within explicit transactions (`tx, err := db.Begin()`, `tx.Exec()`, `tx.Commit()`). Ensure atomicity across multi-step updates. Verify tests pass and document any observed wall-time improvements for mass data ops.
**Done 2026-07-28:** Audited `manager.go` and refactored the iterative URL scheme migration in `migrateURLSchemes()` to use a single explicit transaction. This batches what was previously hundreds of unbatched `db.Exec` queries into one disk sync, drastically cutting DB-bound latency.


---

## 403. [Evaluate and prototype go-rod as a lightweight replacement for playwright-go](#403-evaluate-and-prototype-go-rod-as-a-lightweight-replacement-for-playwright-go)

**Table rationale cell (original):** Playwright requires heavy Node.js browser binaries. go-rod is pure Go, uses local Chrome, and has strong stealth plugins which fits better for a standalone Go CLI.

### 403. Evaluate and prototype go-rod as a lightweight replacement for playwright-go
**Background:** The project currently leverages `github.com/mxschmitt/playwright-go` for ATS form submissions and scraping. Playwright downloads large browser binaries (Chromium, Firefox, WebKit) and relies on Node.js dependencies in the background, which balloons the footprint of the agent. `go-rod` is a pure Go browser automation library that can hook directly into the user's existing Chrome/Edge installation. It's lighter, faster, and maintains excellent stealth plugins (`go-rod/stealth`) to bypass basic ATS bot detection.
**Acceptance criteria:** Create a prototype submitter branch or internal implementation using `go-rod`. Replicate the basic form filling for at least one ATS (e.g., Greenhouse). Measure the difference in startup time, memory footprint, and binary weight compared to Playwright. If the prototype succeeds, draft a plan to migrate the remaining ATS handlers.

**Status:** Done (2026-07-28). Created prototype in `cmd/prototype_go_rod`. Replicated Greenhouse form filling via `stealth.MustPage`. The binary size reduced drastically (from ~48MB for the playwright agent to ~15MB for the rod agent) and startup was virtually instantaneous (~1s loading time utilizing local Chrome) compared to managing Node.js binary downloads. 
**Migration Plan**:
1. Introduce `go-rod` into `pkg/submitter` as a separate internal package/file.
2. Abstract the Playwright `Page` logic behind a `FormFiller` interface.
3. Iteratively rewrite `handleGreenhouse`, `handleLever`, and dynamic mapping functions to use `rod.Page`.
4. Run integration testing and manual verification of live job submits.
5. Fully excise `playwright-go` dependency.

---

## 402. [Migrate from go-sqlite3 CGO driver to pure Go modernc.org/sqlite](#402-migrate-from-go-sqlite3-cgo-driver-to-pure-go-moderncorgsqlite)

**Table rationale cell (original):** Removing CGO drastically simplifies cross-compilation and deployments for the agent CLI across different architectures and environments.

### 402. Migrate from go-sqlite3 CGO driver to pure Go modernc.org/sqlite
**Background:** The project currently uses `github.com/mattn/go-sqlite3`. Because this relies on CGO, building cross-platform binaries requires a C compiler and toolchain for every target architecture. For a CLI/agent tool that users want to drop into any environment, this is a major friction point.
**Acceptance criteria:** Swap the SQLite driver in `pkg/storage/manager.go` to a CGo-free pure Go alternative (such as `modernc.org/sqlite` or `github.com/ncruces/go-sqlite3`). Ensure all database initialization, WAL settings, and schema migrations continue to work correctly. Ensure tests in `pkg/storage` pass without CGO enabled (`CGO_ENABLED=0 go test ./pkg/storage/...`).

**Status:** Done (2026-07-28). Swapped the `github.com/mattn/go-sqlite3` driver with `modernc.org/sqlite` in all 16 files where it was imported. Ran `go mod tidy` and verified that tests pass with `CGO_ENABLED=0`.


---

## 398. [Enhance Playwright stealth capabilities to bypass basic ATS bot detection](#398-enhance-playwright-stealth-capabilities-to-bypass-basic-ats-bot-detection)

**Table rationale cell (original):** Added `window.chrome`, `navigator.plugins`, and `navigator.languages` mocks to `pkg/submitter/browser.go` to evade simple bot challenges on Lever and Greenhouse.

---

## 397. [Add SkipScoring configuration to bypass local LLM fit evaluation bottlenecks](#397-add-skipscoring-configuration-to-bypass-local-llm-fit-evaluation-bottlenecks)

**Table rationale cell (original):** Local LLM inference takes 10+ minutes per job, bottlenecking volume application tools. Added `SkipScoring` bool to `profile.yaml` which overrides score to 100 to instantly process jobs that pass keyword filters.

---

## 96. [Filter out dead or expired job postings early](#96-filter-out-dead-or-expired-job-postings-early)

**Table rationale cell (original):** Over 230 jobs redirected to an error page during submission. Checking URL validity and caching dead links early will save inference and bandwidth.

---

## 39. [Track recent source health and application-attempt cost](#39-track-recent-source-health-and-application-attempt-cost)

**Table rationale cell (original):** Added application_attempts table and tracked terminal outcomes with model inference ms and elapsed time.

### 39. Track recent source health and application-attempt cost

`SourceOutcomeBreakdown` reports lifetime counts, but lifetime totals combine stale postings, old bugs, CAPTCHA outcomes, account gates and confirmed submissions. They are insufficient for deciding whether a source is improving. The queue also lacks a consistent record of the expensive work spent before each terminal result, even though local inference time is the binding resource.

**Acceptance criteria:** record a privacy-safe attempt event with source, normalized posting key, start/end timestamps, terminal class (`APPLIED`, post-submit CAPTCHA, manual/account gate, dead posting, validation failure, other failure), and model-call count or elapsed inference time; expose rolling 7/30-day source summaries with sample counts and confidence/sparsity indicators; distinguish current behavior from historical rows; never record résumé text, PII, one-time codes or full page content; and add deterministic aggregation tests. The output feeds #35 but does not itself reorder or mutate the queue.


---

## 38. [Produce a dry-run queue plan before requeueing or reprioritizing jobs](#38-produce-a-dry-run-queue-plan-before-requeueing-or-reprioritizing-jobs)

**Table rationale cell (original):** Implemented queue plan mode in cmd/requeue showing normalized URLs, status, fit score, and duplicates.

### 38. Produce a dry-run queue plan before requeueing or reprioritizing jobs

The existing `cmd/requeue` tool can safely dry-run counts, but it does not explain which individual rows would be affected, why they are candidates, whether their URL has a scheme duplicate, or whether a matching `applied_jobs` record makes clearing dedup dangerous. Direct SQL edits are especially risky while the agent is running because its queue is an in-memory startup snapshot.

**Acceptance criteria:** add a read-only queue-plan command or mode that reports, without personal profile data, each candidate's normalized posting key, source, current status, age, fit similarity, prior terminal outcome and proposed action; flag HTTP/HTTPS duplicate pairs and any dedup row; show aggregate counts before individual rows; require an explicit confirmation flag for any future mutating mode; keep the default side-effect free; and add tests for empty, duplicate, stale, applied and mixed-status cohorts. The report must state that a running agent needs a restart before a changed queue can be observed.

**Completed 2026-07-28:** Implemented `QueuePlan` in `pkg/storage` to gather candidates, calculate age, duplicates, scheme equivalents, and format proposed actions. Added the `-plan` flag to `cmd/requeue`, showing a tabular layout of all candidates. Included a warning that the agent needs to restart before seeing any queue changes. Added tests in `pkg/storage/queue_plan_test.go` covering empty, duplicate, and stale/mixed cohort scenarios.


---

## 37. [Revalidate posting freshness before expensive document generation](#37-revalidate-posting-freshness-before-expensive-document-generation)

**Table rationale cell (original):** The live log recorded five jobs that scored 80–90 and then redirected to an expired/error page when submission began several minutes later. A bounded post-score freshness check can avoid document generation and failed-submit noise without changing the pre-score fetch validation or security boundaries

### 37. Revalidate posting freshness before expensive document generation

The live 2026-07-27 run scored five postings at 80 or 90 and then found them dead or redirected when `AttemptSubmit` navigated several minutes later: Ping Identity, Typeform, Dropbox, and two Avride postings. The current pipeline validates the page before scoring, then spends minutes in local inference before the submission navigation; a posting can expire in that gap and is recorded as `FAILED_SUBMIT` after the expensive work has already happened.

**Acceptance criteria:** after a passing fit score and immediately before document generation, perform one bounded, security-policy-controlled freshness check against the same posting URL; classify a confirmed dead/error redirect as `INVALID_URL` rather than `FAILED_SUBMIT`; leave transient network failures retryable; preserve the existing SSRF, quarantine and pre-score fetch gates; add tests for live, dead, redirect, transient and cancellation outcomes; record the check timing and disposition without logging private form data.

**Done 2026-07-28:** Added a post-score freshness check right before document generation in the `cmd/agent` worker loop, leveraging the existing `checkJobAlive` function (exported in #96). Jobs that hit an error-page redirect (like 404, 410, or known dead reasons from `submitter.DeadRedirectReason`) are caught and marked `INVALID_URL`, safely avoiding document generation. Transient network errors safely return the job to `DISCOVERED` without throwing a permanent failure. Tests added for `checkJobAlive` covering live, dead, gone, transient, redirect loops, and cancellation outcomes.


---

## 36. [Reconcile setup and feature documentation with executable behavior](#36-reconcile-setup-and-feature-documentation-with-executable-behavior)

**Table rationale cell (original):** Added a safe fake-data-only `pii.yaml.template`, corrected README payload, timeout, and CPU-performance claims, documented the timeout setting, corrected stale historical changelog wording, and added tests that parse the template and check required README entrypoints

### 36. Reconcile setup and feature documentation with executable behavior

At filing time, the README could not be followed from a clean checkout: it instructed `cp pii.yaml.template pii.yaml`, but no template existed. Several feature claims had also drifted from the executable:

- it promises ~1,500-character model payloads and strict 60-second LLM timeouts, while current scoring/form limits and the local-provider timeout are deliberately much larger;
- it describes `qwen3:30b-instruct` scoring in about five seconds on CPU, while this project's own measurements record minutes.
- the changelog attributes daemon looping to an earlier change that did not actually ship it and historically labels a `net.ParseIP` literal check as “true IP resolution.” Bugs #120, #121, and #122 corrected the current daemon, pre-model-quarantine, and resolver-bound network behavior on 2026-07-27, but did not repair those historical claims.

**Acceptance criteria:** add a safe, fake-data-only PII template that stays schema-synchronized by test; validate every command from a clean checkout; derive configuration/model guidance from `.env.example` and current code; clearly separate measured performance from estimates; remove or correct stale security/stealth claims; add a lightweight docs-link/config-name check so obvious drift fails CI without reading real PII.

**Re-scored 2026-07-26 after bug #119:** the SerpApi/Yahoo fallback documentation now matches executable behavior and was removed from this item's scope. The missing PII template plus five remaining operational/security claim groups still make this a high-value, one-pass documentation correction, so Value 5, Decay 1.0, Effort 1, and score 5.0 remain unchanged.

**Re-verified after bug #120 on 2026-07-27:** current daemon behavior and launch guidance agreed, so that README defect was removed from scope. The remaining setup, payload-size, timeout, measured-performance, and historical/security wording defects were corrected below.

**Completed 2026-07-27:** `pii.yaml.template` now provides fake placeholders without personal data, and `TestPIITemplateParsesWithoutPersonalData` prevents schema drift. README now points to the template, documents the 50k/75k payload breakers and provider-specific timeouts, and labels local CPU timing as workload-dependent rather than promising five-second scoring. `TestREADMENamesCurrentSetupEntrypoints` checks the template link, profile path, timeout setting, dashboard URL, and daemon command. Historical changelog notes now identify superseded daemon and `net.ParseIP` wording instead of presenting it as current behavior.


---

## 35. [Rank the queue from observed outcomes while preserving exploration](#35-rank-the-queue-from-observed-outcomes-while-preserving-exploration)

**Table rationale cell (original):** Replaced static source tiers with Bayesian smoothed application outcome tracking, incorporating recent source health penalties, freshness decay, fit score, and a 20% exploration share for sparse sources.

### 35. Rank the queue from observed outcomes while preserving exploration

`sourcePriorityCASE` permanently puts Greenhouse and Lever in tier 0 based on older reachability evidence. The current journal measured a new constraint: six of seven forms that reached a complete fill were blocked after submit, including all four Lever attempts. Hard-coding the opposite conclusion indefinitely spends the slow local scoring call first on the cohort most likely to stop at bot protection.

This must **not** become a pre-skip based on CAPTCHA-widget presence. Akuity carried reCAPTCHA and still produced the first genuinely confirmed application; the journal and bugs #45/#46 show why presence is not failure.

**Acceptance criteria:** derive a rolling, minimum-sample outcome/cost score by source or host; distinguish confirmed submission, post-fill CAPTCHA, manual-required and structural failure; retain a nonzero exploration quota and recency decay so recovery is observable; never exclude solely on widget presence; expose the ranking evidence in a report; test cold-start, sparse samples, changing outcomes and the exploration floor. Reassess after a larger clean post-#118 cohort because the current sample is informative but small.

**Re-scored after bug #120 on 2026-07-27:** daemon mode now admits only 15 jobs per six-hour cycle by default. With thousands of rows waiting, `sourcePriorityCASE` no longer controls only within-batch latency; it strongly influences which backlog jobs receive the day's scarce scoring capacity. Value rises from 5 to 6. Decay 1.0 and Effort 5 remain, producing **1.2**.

**Expanded after the 2026-07-27 live queue audit:** the current source tiers are not merely old; they treat Greenhouse and Lever identically even though the live report shows one confirmed Greenhouse application versus zero Lever applications and 34 Lever CAPTCHA blocks. The score must use a minimum-sample prior or Bayesian smoothing, not raw percentages, and must weight recent outcomes more heavily than stale historical rows. Recommended ranking inputs are smoothed confirmed-application probability, `fit_similarity`, posting freshness, estimated inference cost and a source-health penalty. Reserve a configurable exploration share (for example, 10–20% of each capped cycle) for under-sampled or recovering sources. Never pre-skip a job because a CAPTCHA widget is present: a widget was present on a confirmed Greenhouse application.

**Acceptance additions:** prove that duplicate-normalized postings are counted once (bug #112 prerequisite); cover cold-start and sparse-source priors, recent decay, known manual/account-gated sources, fit-score ties, changing outcomes and exploration-floor guarantees; expose the reason and component values for every ranked job in the report from #38; and verify that queue ordering changes do not mutate funnel statuses or `applied_jobs`.


---

## 34. [Make the local dashboard accessible and self-contained](#34-make-the-local-dashboard-accessible-and-self-contained)

**Table rationale cell (original):** Added `<main>` landmark, `aria-live` regions for live polling data, table `<caption>` and scoped `<th scope="col">` headers, and `prefers-reduced-motion` override. Swapped Google Fonts for system-ui fallback stack to remove external dependency. Added DOM-level automated tests. **2026-07-29 groom correction:** #426's rewrite deleted the `aria-live` region, both `<caption>`s, all twelve `scope="col"` headers, and the DOM-level tests. `<main>`, `prefers-reduced-motion` and the system-ui stack survived; `aria-live` was restored while fixing bug #435. The table semantics are still missing because the tables themselves are (bug #437)

### 34. Make the local dashboard accessible and self-contained

`cmd/dashboard/index.html` has no `<main>` landmark, no `aria-live` status for polling updates, no table captions or scoped headers, and no `prefers-reduced-motion` override despite `pulse-dot`, `sweep`, and `spin` keyframes. It also loads fonts from `fonts.googleapis.com`, causing an external request whenever the private local dashboard opens.

**Acceptance criteria:** meet a focused WCAG 2.2 AA pass for landmarks, keyboard/focus behavior, names, table semantics, contrast and live updates; disable nonessential motion under `prefers-reduced-motion`; bundle or use system fonts; preserve the compact visual design; add DOM-level tests for durable semantics and perform a manual keyboard/screen-reader smoke check.


---

## 33. [Make the configured location resolvable on every geocoder, and compute the start date](#33-make-the-configured-location-resolvable-on-every-geocoder-and-compute-the-start-date)

**Table rationale cell (original):** **Filed and shipped 2026-07-25 (user direction: "Macomb works, sometimes township doesn't show up ... try to get this right").** Verified live: Lever returns **zero** for `Macomb Township`, `Macomb Township, MI` and bare `Macomb`, but resolves **`Macomb, MI`** — an earlier zero for that exact term was the geocoder rate-limiting, not an absence. Candidates now include the bare place name. Also fixed a self-inflicted trap: `LocationMustContain` demanded the spelled-out `Michigan`, which **rejects Lever's own correct option `Macomb, MI, USA`** — tokens now accept alternatives. Plus `earliest_start_date` is computed as **today + 14 days** at render time, per the user's standing rule

### 33. Make the configured location resolvable on every geocoder, and compute the start date

**Filed and shipped 2026-07-25** on the user's direction: *"Macomb works, sometimes township doesn't show up, with goal of applying try to get this right."*

**Measured against Lever's live geocoder** rather than assumed:

```
typed "Macomb"       -> []
typed "Macomb, MI"   -> [Macomb, MI, USA | Macomb, Township of Macomb, MI, USA]
```

Note this **corrects an earlier finding**. bugs.md #88 recorded that Lever returns nothing for `Macomb, MI`; re-testing on a fresh page shows it resolves fine. The original zero was the geocoder **rate-limiting after repeated probe queries**, not an absence — a reminder that a flaky external service needs a clean re-test before its answer is written down as fact.

**Two changes:**

1. **Bare place name added to the candidate list.** `stripCivilDivisionSuffix` drops `Township`/`Twp`/`Village`/`Borough`/`Town` and the inverted `Township of`/`City of` forms, so `Macomb Township` also offers `Macomb, MI`. Greenhouse keeps resolving the full civil division; Lever now has something it recognises.

2. **A self-inflicted trap fixed at the same time.** `LocationMustContain` required the spelled-out `Michigan`. Lever's own correct option reads `Macomb, MI, USA` — which does not contain "Michigan" — so the safety check from #79 would have **rejected the right answer**. Tokens now accept alternatives separated by `|`, and any one satisfies. The guarantee is unchanged: `Macomb, IL, USA` still fails, because it carries neither `Michigan` nor `MI`.

**Also shipped here:** `earliest_start_date` is computed as **today + 14 days** at render time when unset, per the user's standing rule ("always put two weeks from the date applying"). A fixed date in a config file goes stale silently; `nowFunc` is indirected so it stays testable.

**Tests:** `TestPII_LocationSearchCandidates` (updated), `TestPII_LocationMustContain_AcceptsEitherStateSpelling`, `TestStripCivilDivisionSuffix`, `TestApplicationFacts_ComputesEarliestStartTwoWeeksOut`, `TestApplicationFacts_ConfiguredStartDateWins`.


---

## 32. [Retrieve emailed one-time codes so verification gates can be completed](#32-retrieve-emailed-one-time-codes-so-verification-gates-can-be-completed)

**Table rationale cell (original):** **Filed and shipped 2026-07-25 (user approved).** The agent now polls narrowly-scoped ATS mailbox messages for one-time application codes and completes supported verification gates without logging the code itself

### 32. Retrieve emailed one-time codes so verification gates can be completed

**Filed 2026-07-25** after the user asked directly: *"will you be able to check my gmail and retrieve these codes?"*

bugs.md #93 established the need. Greenhouse issued a one-time security code by email at the exact second of a submit, and the application cannot complete until that code is typed back into the form. The agent has no mailbox access, so #93 routes such jobs to `MANUAL_REQUIRED` — correct, but it caps what the pipeline can finish on its own.

**In the current Claude Code session this is already possible** — the Gmail tooling is available and was used to retrieve the Surt AI code (`uOSBQvRu`) on request. **`cmd/agent` cannot**: it has no Gmail credentials and no code path for it.

**What it would take:** Gmail API OAuth (free, no paid tier), a token stored alongside the other secrets, and a narrow polling step that runs only when `ErrNeedsEmailVerification` fires — querying a tight filter (`from:` known ATS senders, `subject:` code wording, `newer_than:10m`), extracting the code, filling the field and resubmitting.

**Shipped 2026-07-25 on the user's explicit approval.** It turned out to need far less than expected: `pkg/tracker/imap.go` already speaks IMAP via `emersion/go-imap` for the email tracker, and `IMAP_USER` / `IMAP_APP_PASSWORD` / `IMAP_SERVER` were **already present in `.env`**. So no OAuth, no Google Cloud project, no new dependency, no additional setup — the capability was one small file away the whole time.

**How the access is bounded.** This reads the user's personal mailbox, so the search is as narrow as the task allows, at three independent levels:
1. **Sender** — only known ATS domains (`greenhouse-mail.io`, `lever.co`, `ashbyhq.com`, `workable.com`, `smartrecruiters.com`, …). Anything else is never examined.
2. **Subject** — only messages announcing a code ("security code", "verification code", …).
3. **Time** — only messages that arrived **after the submit click that triggered them**, so a stale code from an earlier application can never be reused. The mailbox is opened **read-only**.

The code itself is **never logged** — it is a live credential for an application in flight. Only the subject is.

**Extraction is anchored, not greedy.** It keys off the sentence introducing the code ("copy and paste this code", "your code is", …) rather than scanning the whole message, because a marketing footer is full of 6-12 character tokens. `isPlausibleCode` then rejects prose: a real code mixes cases or digits, "application" and "greenhouse" do not. Verified against the exact Greenhouse wording and its HTML form.

**Failure is always safe.** No fetcher configured, no code within 90 seconds, no field to type it into, or a resubmit that is not confirmed — every path falls back to `ErrNeedsEmailVerification` and manual review, which is where bugs.md #93 already put it.

**Tests:** `TestExtractSecurityCode_RealGreenhouseWording`, `TestExtractSecurityCode_HTMLBody`, `TestExtractSecurityCode_IgnoresOrdinaryProse`, `TestIsPlausibleCode`, `TestSubjectAnnouncesCode`, `TestWaitForSecurityCode_ReturnsTheCodeAndPassesTheCutoff`, `TestFillSecurityCode_FillsAVisibleField`, `TestFillSecurityCode_ReportsWhenNoFieldIsPresent`.

**Note on the existing code:** `uOSBQvRu` was issued against a browser session that no longer exists, so completing Surt AI by hand will need a fresh code rather than that one.


---

## 31. [Fill Lever's required location on the initial pass, and generalise the combobox helper](#31-fill-levers-required-location-on-the-initial-pass-and-generalise-the-combobox-helper)

**Table rationale cell (original):** **Filed and shipped 2026-07-25.** The identical gap #28 closed for Greenhouse, found by reading `handleLever` after bugs.md #86: it filled only name/email/phone while Lever marks **location required**, so the first submit always bounced and only the validation-retry loop (~12 min of inference) could satisfy it. `fillGreenhouseCombobox` renamed to `fillComboboxFromCandidates` — the logic was never ATS-specific and now serves Greenhouse's react-select and Lever's typeahead alike. **This is the third instance of the same structural pattern** (#65/#66→#67, #74→#75, #28→#31): a capability wired into one fill path and not the others

### 31. Fill Lever's required location on the initial pass, and generalise the combobox helper

**Filed and shipped 2026-07-25**, found by reading `handleLever` immediately after bugs.md #86 — the same reflex that produced #28 for Greenhouse.

`handleLever` filled `name`, `email` and `phone` and nothing else. Lever marks **location required** (confirmed by probe: exactly three required fields, and the resume upload is *optional*), so the first submit was structurally guaranteed to bounce and the only path that could ever satisfy it was the validation-retry loop — a `SolveValidationErrors` call, ~12 minutes of inference on this host, per Lever posting.

Also renamed `fillGreenhouseCombobox` → `fillComboboxFromCandidates`. The logic was never Greenhouse-specific; after #86 it drives Lever's typeahead too, and the old name would have discouraged exactly the reuse that was needed.

**This is the third instance of one structural pattern**, and it is worth naming as such:

| | capability | added to | missing from |
| --- | --- | --- | --- |
| #67 | dispatch-by-control-type, bare-id resolution (#65/#66) | validation retry | initial fill |
| #75 | combobox commit (#74) | validation retry | initial fill |
| #31 | required-location fill (#28) | Greenhouse handler | Lever handler |

Every fill capability should be checked against **all** fill paths — the retry loop, `safeFillWithLabelFallback`, and each dedicated ATS handler — before it is called done.

**Best-effort by design:** a miss costs nothing beyond the retry cycle that used to happen unconditionally, and bugs.md #88 now routes a genuinely uncommittable location to manual review rather than burning three attempts.


---

## 30. [Detect unanswerable attestations before fit-scoring, not after](#30-detect-unanswerable-attestations-before-fit-scoring-not-after)

**Table rationale cell (original):** Closed by user request. Motivating authorization and sponsorship answers are already configured.

### 30. Detect unanswerable attestations before fit-scoring, not after

**Filed 2026-07-25 while monitoring, with the risk stated, and deliberately not built.**

The observation is real: #82 refuses an unanswerable form in 0 seconds, but only *after* ~10 minutes of fit-scoring, because a form's questions are unknowable until it is loaded. On a host where every LLM call serialises, that is the dominant cost of a blocked job. And the check could plausibly run earlier — verified live that a plain `curl` of `job-boards.greenhouse.io/reddit/jobs/8044767` returns the form markup, and `DetectAttestationQuestions` finds both categories in it without a browser.

**Why it is not built: the false-positive cost is an entire lost application.**

Job *descriptions* very commonly carry the sentence "must be legally authorized to work in the United States" as a stated requirement. Run the detector over the fetched description and it trips on close to every US posting — mass-refusing jobs whose forms never asked the question, and routing perfectly applicable work to the manual queue. That is strictly worse than the ~10 minutes it saves.

Scoping to `parser.PruneDOMToForm` first would exclude prose that sits outside `<form>`, and does work where the form is server-rendered. But `PruneDOMToForm` falls back to the whole document when no `<form>` element is present — which is exactly the client-rendered case — so the prose comes straight back in, silently. Making this safe needs a positive check that the attestation text is inside a real form *control's* label, not merely present on the page.

**Scored below the floor on purpose.** The value evaporates the moment `work.authorized_to_work_us` and `work.requires_sponsorship` are set in `pii.yaml`, which is the actual fix and costs two lines. Reconsider only if a category the user genuinely cannot answer (security clearance, say) turns out to be common in the backlog.

**Re-scored 2026-07-26:** both motivating work-attestation fields are now configured; only their configured/blank state was checked, never their personal values. No newly observed unanswered category replaces them. Value therefore drops from 2 to 1, with Decay 1.0 and Effort 5 unchanged: **0.20**, further below the ROI floor. Recommendation: close this item, and reopen a narrowly scoped detector only if live evidence finds a different unanswered category.

**Closed 2026-07-28:** Closed by user request since the motivating fields are now configured and no new category is observed.

**Re-scored 2026-07-27 after bug #121:** live boolean-only inspection now shows both motivating fields blank, conflicting with the historical configured-state note above. Live evidence supersedes the backlog assumption, so Value returns from 1 to 2; Decay remains 1.0 and Effort remains 5, producing **0.40**. The item is still below the 0.5 floor because a safe early detector must positively associate the question with a real form control across server-rendered and client-rendered ATSes; page-text matching would silently discard legitimate applications. Recommendation: configure the two answers if appropriate, or re-scope this to a smaller server-rendered control-label detector before requesting implementation.

**Corrected 2026-07-27 after bug #120:** the post-#121 blank-state note conflicts with the current file when checked against the actual `WorkFacts` schema keys. Boolean-only live inspection shows `authorized_to_work_us`, `requires_sponsorship`, and the accepted `visa_status` fallback configured; no personal values were printed. Live evidence supersedes the note above. With no newly observed unanswered category, Value returns to 1, Decay remains 1.0, Effort remains 5, and the score is **0.20**. Recommend closing the item and reopening a narrower detector only when a real unanswered category appears.


---

## 29. [Hard-code every repeatable application fact so the model stops guessing](#29-hard-code-every-repeatable-application-fact-so-the-model-stops-guessing)

**Table rationale cell (original):** **Filed and shipped 2026-07-25 (user request).** Extends `pii.yaml` with links, screening answers, education and employment history, rendered into prompt context by `PII.ApplicationFacts()` so recurring questions become lookups rather than inferences. Grounded from `ai_knowledge_library/USER_PROFILE.md`; blanks are omitted entirely so an unset field reads as "not provided" rather than as an answer. **Legal attestations (work authorization, sponsorship, visa, clearance, criminal history, over-18) are deliberately left blank for the user** — they are declarations on a real application and a plausible guess is precisely the wrong thing for an agent to supply

### 29. Hard-code every repeatable application fact so the model stops guessing

**Filed and shipped 2026-07-25 at the user's request**, while monitoring the live run: *"the more hardcoded information like that, the less the ai has to think or guess at it."* That is exactly right — on this host every avoided inference is real wall-clock time, and every avoided *guess* is a wrong answer that never reaches a real employer.

`pii.yaml` gains `links`, `work`, `education` and `experience` sections; `PII.ApplicationFacts()` renders them into the same prompt context that already carried the EEO answers, for both `ExtractFormMapping` and `SolveValidationErrors`.

**Filled from `ai_knowledge_library/USER_PROFILE.md`** (the library's designated career profile, per the Domain Routing rule): LinkedIn, GitHub, current employer and title, 10+ years experience, $105k target, remote preference, CCNA, 3 education entries and 6 employment entries.

**Deliberately left blank, and reported to the user rather than filled:**
- **Legal attestations** — work authorization, visa sponsorship, visa status, security clearance, criminal history, over-18. These are declarations on a real job application. A confident-looking guess here is worse than a blank, and it is not the agent's to make.
- **Preferences and timing** — notice period, earliest start date, willing to relocate/travel, how-did-you-hear, previously-employed-here, referred-by, pronouns, languages, driver's license. Not derivable from the profile; the user's call.

Blank fields are **omitted from the rendered context entirely**, so an unset field reads as "not provided" instead of as an empty answer, and the closing instruction tells the model to decline rather than fabricate.

**A bug caught while eyeballing the real rendered output:** employment ranges came out as `Feb 2023 to Presen`. `strings.Trim(s, "to")` treats its argument as a **character set**, so it stripped the trailing `t` from "Present". Replaced with explicit range joining. Pinned by `TestApplicationFacts_DoesNotChewTheEndOfPresent` — worth keeping because the symptom is a single missing character in text that goes to real employers.

**Tests:** `TestApplicationFacts_RendersConfiguredFactsAndOmitsBlanks`, `TestApplicationFacts_DoesNotChewTheEndOfPresent`, `TestApplicationFacts_HandlesAOneSidedDateRange`.


---

## 28. [Fill Greenhouse's required Location/Country comboboxes on the first pass](#28-fill-greenhouses-required-locationcountry-comboboxes-on-the-first-pass)

**Table rationale cell (original):** **Filed 2026-07-25 while monitoring the live run.** `handleGreenhouse` fills only `first_name`/`last_name`/`email`/`phone` and never attempts `Location (City)` or `Country` — both **required** on every Greenhouse form. So the first submit always bounces and the *only* path that can satisfy them is the validation-retry loop, i.e. a `SolveValidationErrors` call at **~12 min of inference** on this host, on every Greenhouse posting. Throughput is the binding constraint (~3,100 jobs, all LLM calls serialised), and Greenhouse is the single most common ATS in this backlog, so this is one of the largest avoidable time sinks left. Blocked on a `pii.yaml` schema change — `config.PII` has `Address` but no discrete city/country — which needs the user to populate the new fields, so it is **not** autonomous work

### 28. Fill Greenhouse's required Location/Country comboboxes on the first pass

**Filed 2026-07-25 while monitoring the live 82-job run**, from the same investigation that produced bugs #74/#75/#76.

`handleGreenhouse` (`pkg/submitter/browser.go`) fills exactly four fields — `input#first_name`, `input#last_name`, `input#email`, `input#phone` — plus the resume upload, then submits. It never attempts `Location (City)` or `Country`. Both carry the required marker on every Greenhouse form.

**Consequence:** the first submit is structurally guaranteed to bounce, and the only code path that can ever satisfy those two fields is the validation-retry loop — which means a `SolveValidationErrors` call, measured at **~12 minutes of inference** on this CPU-only host. Every Greenhouse posting pays that toll before it can succeed, to set two values the handler already had the information to set.

This matters because throughput is the binding constraint (~3,100 jobs waiting, `llama-server` at `-np 1` so every LLM call serialises — see the 2026-07-25 throughput audit), and Greenhouse is the most common ATS in this backlog. Removing a guaranteed ~12-minute cycle per posting is worth more than most new capability.

**Why this is not autonomous work:** `config.PII` has `Address` but no discrete city/country field, so this needs a `pii.yaml` schema addition that only the user can populate with real data. Parsing the existing free-text `Address` was considered and rejected — it would guess at the exact fields where a wrong value is worse than no value, and `pii.yaml` holds real personal data that must not be inferred.

**Shipped 2026-07-25**, same day, once the user supplied the address fields.

`config.PII` gained `Street`, `City`, `State`, `FullState`, `Zip`, `Country`, all optional and skipped when blank exactly like the EEO fields. `handleGreenhouse` now sets Location and Country through the combobox-commit path from bugs.md #74/#76 rather than a bare `Fill()`.

**Two things worth recording, both found while building it:**

1. **`gopkg.in/yaml.v3` matches keys case-sensitively.** Verified empirically rather than assumed: `City:` binds to nothing on a `yaml:"city"` field, returns **no error**, and leaves the field empty. Since `pii.yaml` is hand-maintained real personal data, a casing slip silently dropping an address field is a nasty failure mode — everything loads fine and the only symptom is a form field left blank much later, inside a 12-minute retry cycle. `PII.UnmarshalYAML` now lower-cases every mapping key before decoding, so `City`/`city`/`CITY` all bind. Pinned by `TestPII_LoadsMixedCaseAddressKeys`, which uses the exact mixed casing the file was written with (including a space before one key's colon).

2. **The geocoder's accepted phrasing is not knowable in advance.** `LocationSearchCandidates` returns `"City, ST"`, then `"City, Full State"`, then bare `"City"`, and `fillGreenhouseCombobox` tries each until one actually commits — checkable only because #74/#76 made "did the selection commit" observable. Every failure path leaves the field to the validation-retry loop, which is precisely where it went unconditionally before, so the change cannot make anything worse.

**Free text `Address` is still deliberately not parsed.** Location is a geocoded autocomplete where a wrong value is worse than a blank one, and inferring real personal data is not something to do silently.


---

## 27. [Local MCP server for career context](#27-local-mcp-server-for-career-context)

**Table rationale cell (original):** Closed by user request. Grounded screening answers and career-context retrieval already ship in-process.

### 27. Local MCP server for career context
**Proposed by the user 2026-07-25:** a Go server holding résumé facts, production-support experience and cyber-defense coursework, exposed over Model Context Protocol so automation can query it for accurate screening-question answers.

**Assessment — the capability already exists; what is proposed is a protocol layer over it.** Verified against the code before scoring:
- Grounded screening-question answering **shipped** as improvements.md #16: `ExtractFormMapping` detects custom questions and generates answers constrained to `profile.yaml` / `USER_PROFILE.md` facts, with an explicit never-invent rule for EEO categories.
- The "hold your résumé facts and retrieve the relevant ones" part **shipped** as the RAG layer: `career_chunks` + `parser.RetrieveTopK`, live in every application (and the subject of bug #58's dimension fix).
- **`pkg/mcp` is not Model Context Protocol.** It is this repo's LLM-provider abstraction (Ollama/Claude/Gemini) and predates the name clash. There is no JSON-RPC, no server, no MCP wire format anywhere in the repo. Worth stating plainly because the directory name makes it look otherwise.

**So the honest value is interoperability, not capability**: an MCP server would let *other* tools (Claude Desktop, another agent, a future non-Go component) reach the same career context. For a single-process Go pipeline that already queries it in-memory, that is speculative. Scored Value 2 × Decay 0.5 (same "better screening answers" curve #16 already advanced) ÷ Effort 3 = **0.33**, below the floor. Flagged ⚠️ — do not work without explicit confirmation.

**Reconsider if** the user starts driving this data from Claude Desktop or another MCP client, or splits the pipeline into multiple processes. At that point interop stops being speculative and Value rises accordingly.

**Re-scored 2026-07-26:** repository-wide checks still find no Model Context Protocol server, wire implementation, or named external consumer; `pkg/mcp` remains only the internal LLM-provider abstraction. Value 2 × Decay 0.5 ÷ Effort 3 remains **0.33**. Recommendation: close unless the user can name a real client that needs this interoperability layer.

**Closed 2026-07-28:** Closed by user request as the current in-process integration is sufficient.


---

## 26. [Discover via Greenhouse/Lever board feeds instead of search dorking](#26-discover-via-greenhouselever-board-feeds-instead-of-search-dorking)

**Table rationale cell (original):** **Filed and shipped 2026-07-25 (user proposal).** Both ATSes publish complete board contents via public unauthenticated JSON; verified live at **238 postings for remotecom and 287 for palantir from one call each**. Replaces search-dorking guesswork for the two platforms that matter most — 605 company slugs were already recoverable from URLs the funnel had collected

### 26. Discover via Greenhouse/Lever board feeds instead of search dorking (Done 2026-07-25)
**Proposed by the user 2026-07-25**, framed as "job boards maintain hidden XML feeds; a Go service could monitor them and hand off only the final submission to Playwright."

**One premise corrected before building.** Discovery in this repo does **not** use browser automation — `FunnelEngine.DiscoverJobs` runs `discoverWithYahooHTML` (search dorking), `discoverWithRemoteOK` and `discoverWithHackerNews`, all plain HTTP. Playwright is used only for *submission*. So the stated saving (browser compute during discovery) does not apply here, and no Python component is needed — this is already a Go project using `playwright-go`.

**The idea is still strongly worth doing, for a different and better reason: feed quality and coverage.** Search dorking returns whatever an index happened to surface — incomplete, stale, and polluted: **301 of 3,884 funnel rows are `INVALID_URL`**, roughly 8% pure waste that each still cost a fetch and a filter pass. A board feed returns the company's complete, current posting list as structured data.

**Live-verified before writing code**, per this repo's standing rule:
- `boards-api.greenhouse.io/v1/boards/<slug>/jobs` → **238 postings** for `remotecom`
- `api.lever.co/v0/postings/<slug>?mode=json` → **287 postings** for `palantir`

Both public, unauthenticated, free. And the company slugs were already sitting in the database: **373 distinct Greenhouse companies and 232 Lever companies** recoverable from URLs the funnel had already collected.

**Shipped:** `pkg/scraper/atsfeeds.go` — `discoverWithATSFeeds` polls known company boards, wired into `DiscoverJobs` alongside the existing sources. New `storage.GetKnownATSCompanies` extracts slugs from collected URLs, ordered by posting count so a bounded pass spends its budget on employers this profile actually matches. Deliberately a **widening pass over known employers** rather than a way to find new ones: if a company was worth applying to once, its other current openings are worth seeing, and the feed is the only way to get all of them.

**The safeguard that makes this a net win rather than a queue denial-of-service:** a feed returns a company's *entire* posting list, accountants and office managers included. At the ~10 minutes per fit-score measured on this hardware, admitting all 238 remotecom postings would have been catastrophic. `titleLooksRelevant` is a free keyword gate applied only to feed-sourced jobs, matching a full configured role as a phrase or a *distinctive* whole word from one. Verified against the real `profile.yaml`: kept "Senior Backend Engineer", "Site Reliability Engineer", "Staff Platform Engineer", "Cloud Security Engineer"; dropped "Accountant", "Administrative Business Partner", "Account Executive DACH", "Office Manager", "Cargo Operations Manager".

**A bug caught in this code before it shipped:** the distinctive-word tier originally used substring matching, which would have matched `"go"` inside "Car**go** Operations Manager" and "Chica**go**", and `"api"` inside "c**api**tal" — waving through precisely the roles the gate exists to stop. Now matched against whole tokens; `TestTitleLooksRelevant_ShortTokensDoNotMatchSubstrings` is a permanent regression guard. Feed jobs also still pass through `IsKnownJunkJobURL`, so a feed cannot bypass filters the rest of the pipeline depends on.

Tests: `TestParseGreenhouseBoard`, `TestParseLeverBoard` (both pinned to response shapes captured live), `TestParseBoards_RejectMalformedPayloads`, `TestTitleLooksRelevant`, `TestTitleLooksRelevant_ShortTokensDoNotMatchSubstrings`, `TestTitleLooksRelevant_NoRolesConfiguredKeepsEverything`, `TestFeedJobsStillPassThroughJunkFilter`. `go build/vet/test ./...` all pass, 10 packages, 0 failures.

**Not extended to other platforms yet:** Ashby, Workable, SmartRecruiters and Workday also expose feeds of varying quality, but Greenhouse and Lever are where this profile's real volume is. Add others only after confirming each endpoint live, the same way these two were.


---

## 25. [Trim over-long job descriptions before scoring](#25-trim-over-long-job-descriptions-before-scoring)

**Table rationale cell (original):** **Filed and shipped 2026-07-25.** Inference cost on this CPU-only host is dominated by *prompt processing* (measured ~6.8 tok/s on the 30B, ~17 tok/s on the 4B), so scoring cost is set by how many characters are sent, not by which model runs. Descriptions ran 5.5k-14k chars. Trimmed from the middle, keeping both ends, because rubric rules 2/3/7 depend on trailing salary/location details a head truncation would discard

### 25. Trim over-long job descriptions before scoring (Done 2026-07-25)
**Filed from measurement taken while validating #24.** Two live data points established that this pipeline's LLM cost is a *prompt-length* problem, not a model-size problem: the 30B processed ~3,900 prompt tokens in 9m38s (**~6.8 tok/s**) and the 4B handled comparable prompts at roughly **17 tok/s**. Generation is negligible by comparison — `ScoreJob` emits a single integer, confirmed by a direct probe returning `eval_count: 3`. So what scoring costs is decided almost entirely by how many characters go in.

`ScoreJob`'s prompt carries the full job description (**5,594-13,990 chars across the sampled set**) plus the RAG-retrieved résumé context, on top of a fixed ~1.2k-char rubric.

**Shipped:** `trimForScoring` caps the description at 9,000 chars, keeping the **first 6,000 and last 3,000** with a marked elision between them.

**Why middle-out rather than a simple head truncation** — this is the whole design decision: a posting's role summary and remote/onsite wording sit at the top, but the **salary range, "must reside in X" restrictions, and EEO boilerplate sit at the very bottom**. Rubric rules 2 (remote mismatch, −80), 3 (below salary floor, −30) and 7 (geographic restriction, −80) are the three largest deductions available, and rules 3 and 7 depend on precisely those trailing details. Truncating the tail would have silently disabled the rubric's biggest penalties and inflated scores on exactly the jobs that should be skipped. The elision is marked so the model is told text was removed rather than being handed a description that appears to end mid-sentence.

Tests: `TestTrimForScoring` — passthrough for short descriptions, both ends preserved for long ones (asserting the salary line and the "Must reside in Romania" line specifically survive), and the trimmed output lands exactly on the intended budget. `go build/vet/test ./...` all pass, 10 packages, 0 failures.

**Not yet validated against live scores.** The saving is arithmetic and certain; what is *not* yet proven is that trimming never changes a score across the `<50` skip threshold. Worth a follow-up comparison on a posting long enough to actually trip the cap — none of the three jobs in the #24 benchmark exceeded 9,000 chars, so this code path was not exercised by that run.


---

## 24. [Per-call-type model selection — run ScoreJob on a smaller, faster model](#24-per-call-type-model-selection--run-scorejob-on-a-smaller-faster-model)

**Table rationale cell (original):** **Filed 2026-07-25 (groom).** With #23 removing the tailoring call, `ScoreJob` is now the *sole* remaining bottleneck — measured live at ~9m49s of a ~10m job cycle. One `OLLAMA_MODEL` serves every text call, so scoring cannot be made cheap without also downgrading form mapping. Per-role model slots already exist for vision and embeddings, so the pattern is established

### 24. Per-call-type model selection — run ScoreJob on a smaller, faster model
**Filed 2026-07-25 (`/groom_backlogs`), from live measurement rather than theory.** Item #23 removed `ProcessJobApplication` (the 15-20+ min/job tailoring call) from the pipeline. That promoted `ScoreJob` to the sole remaining bottleneck, and the first job after the restart measured it precisely:

```
00:20:25 [Worker-1] Fetching job description for Reddit...
00:30:14 [Worker-1] Fit Score Pipeline: Reddit scored 90! Proceeding with application.
00:30:19 [Worker-1] Using master resume for Reddit (no per-job tailoring, cover letter disabled)
00:30:21 [Auto-Submit] Detected Greenhouse ATS. Filling out fields...
```

Document generation is now **0 seconds** (was 15-20 minutes); scoring is **9m49s** of a ~10m cycle. Against the ~3000-job backlog, scoring is effectively the entire cost of the system now.

**Why it can't just be changed today:** `pkg/mcp/provider_ollama.go` resolves one `OLLAMA_MODEL` (`envOr("OLLAMA_MODEL", "llama3.1")`) and uses it for every text call — `ScoreJob`, `ExtractFormMapping`, and `SolveValidationErrors` alike. Pointing that at a small model to make scoring cheap would simultaneously downgrade form mapping and validation-error solving, which are the correctness-critical paths (they choose selectors and answer real screening questions, including EEO-sensitive ones). **The pattern to follow already exists in the same struct:** `visionModel` and `embedModel` are separate per-role slots (`OLLAMA_VISION_MODEL`, `OLLAMA_EMBED_MODEL`). This item adds the same idea for text, e.g. an `OLLAMA_FAST_MODEL` used by `ScoreJob` only, defaulting to `OLLAMA_MODEL` so the change is inert until configured.

**Scope:** add the model slot and route `ScoreJob` through it; keep every other text call on the strong model. **The real work is validation, not plumbing** — scoring decides whether to apply at all, so a smaller model that scores badly means applying to unsuitable jobs or skipping good ones. Ship it with a real comparison: run both models over the same sample of already-scored jobs and check the scores agree closely enough (especially around the `< 50` skip threshold) before switching the default. Effort 4 reflects that validation requirement, not the code.

**Scoring:** Value 7 (throughput on the dominant cost, against a ~3000-job backlog), Decay 0.5 (second item on the "reduce per-job LLM time" curve, after #23 shipped), Effort 4 → **0.88**, above the 0.5 floor and currently the highest-scoring Pending row in this backlog.

**No new install or spend required:** `qwen3:30b-instruct`, `qwen2.5vl:7b`, and `nomic-embed-text` are already pulled locally; a smaller text model would need one `ollama pull`, which is free and local. Confirm the specific model with the user before pulling.

**Shipped 2026-07-25 (same groom pass that filed it), partial — the routing, not the model choice.** `genRequest.fast` opts a call into a smaller model; `ollamaProvider.fastModel` (`OLLAMA_FAST_MODEL`) serves those calls and falls back to `model` when unset, so **the change is completely inert until the env var is deliberately set** — no existing behavior changes on any machine. `ScoreJob` is the only caller that opts in; `ExtractFormMapping` and `SolveValidationErrors` stay on the strong model by construction, since they choose real selectors and answer real screening questions including EEO-sensitive ones. A vision request keeps using `visionModel` even when it opts in, so the fast slot can never hijack the vision path. Documented in `.env.example` with the validation requirement stated inline.

Tests: `TestOllamaProvider_FastModelRouting` (3 cases: unset falls back, set+requested routes to fast, set-but-not-requested stays strong) and `TestOllamaProvider_FastNeverOverridesVisionModel`. `go build/vet/test ./...` all pass, 10 packages, 0 failures.

**Deliberately not done, and why this row says "partial":** no model was pulled and `OLLAMA_FAST_MODEL` is left unset. Picking one means an `ollama pull` — a new install, which `AGENTS.md` requires discussing with the user first — and, more importantly, the item's real risk was never the plumbing but whether a smaller model scores jobs *equivalently*. Turning this on without first comparing both models' scores over the same jobs (especially around the `< 50` skip threshold) would risk silently applying to unsuitable roles or skipping good ones. **Next step for whoever picks this up: agree a model with the user, pull it, run both over a sample of already-scored jobs, and only then set the env var.**


---

## 23. [Static master cover letter, reused across every application](#23-static-master-cover-letter-reused-across-every-application)

**Table rationale cell (original):** User-requested: one generic, job-agnostic letter for every application instead of a per-job LLM-generated one. Skips the single most expensive step in the pipeline (~15-20+ min/job). Surfaced and fixed bugs #61/#62 along the way — cover letters had never actually been reaching employers at all

### 23. Static master cover letter, reused across every application (Done 2026-07-24)
**Request 2026-07-24:** user asked for one generic cover letter written from the master resume, reused across all jobs, avoiding specifics like the job title, and for the per-job custom cover letter to be toggled off.

**Investigation changed the scope.** Tracing where `ProcessJobApplication`'s cover letter actually goes turned up two genuine, previously-unfiled bugs, both fixed as part of this item because the requested feature would otherwise have been completely inert: **bugs.md #61** (no handler ever filled the cover letter into any form, so every application in this project's history went out resume only) and **bugs.md #62** (the saved letter was then deleted from the application folder, stripping the manual-apply queue). See those Details sections for the full diagnostic chain and live evidence.

**Shipped:**
- `master_cover_letter.txt` at the repo root, gitignored alongside `master_resume.pdf` since it carries real contact details. Three paragraphs, written strictly from `master_resume.pdf`'s real content, no job title / company / per-posting specifics, matching `profile.yaml`'s existing `cover_letter_tone` and the project's no-hyphens convention. **Superseded same day** by the user's own `Omni_CoverLetter.pdf` (see the amendment below); the generated `.txt` remains in place as the default fallback when `master_cover_letter_path` is unset.
- `config.Profile.UseMasterCoverLetter` (`use_master_cover_letter` in `profile.yaml`). **Opt in on purpose:** the Go zero value is false, so a profile that predates the field keeps per-job tailoring rather than silently switching a live pipeline onto the static letter.
- When on, `cmd/agent`'s `generateDocsFunc` **skips the `ProcessJobApplication` LLM call outright** rather than generating documents and discarding them. `storage.SaveApplication` still runs, because it is not just a file write: `RecordApplicationInDB` behind it is what `HasApplied` dedups against, and the folder it creates is what `MoveToManualApply` archives for `MANUAL_REQUIRED` jobs.
- Hardcoded `masterResumePath`/`masterCoverLetterPath` constants, replacing the inline string literal.

**The real tradeoff, stated plainly:** `ProcessJobApplication` is one combined call producing resume + cover letter + interview prep, so skipping it also stops per-job `resume.md` and `interview_prep.md`. Neither is a loss for auto-submitted jobs — the file uploaded to every ATS is the static `master_resume.pdf` (hardcoded, and always has been), and interview prep was only ever a saved reference document. Jobs routed to `MANUAL_REQUIRED` do now get master documents instead of tailored ones, which is the one genuine downside of turning this on. Re-scope into a separate item if tailored docs are wanted for the manual queue specifically.

**Payoff:** removes the single most expensive step in the pipeline, measured live at 15-20+ minutes per job against this machine's CPU-only Ollama. `ScoreJob`, `ExtractFormMapping`, and `SolveValidationErrors` all still run, so the local model is still needed — this cuts roughly one of four LLM call types, but by far the largest one.

Tests: `TestLoadProfile_UseMasterCoverLetter` (3 cases, including that an absent key defaults to tailoring), plus bugs #61/#62's six tests. `go build/vet/test ./...` all pass. **Verified live:** loaded the real `profile.yaml` through `config.LoadProfile` and confirmed `UseMasterCoverLetter = true`, `master_cover_letter.txt` reads (2143 bytes), and `storage.CoverLetterPath("Acme Corp")` correctly sanitizes to `applications/Acme_Corp/coverletter.txt`.

**Amended 2026-07-24 (user supplied their own letter):** user asked to use their existing `Omni_CoverLetter.pdf` instead of the generated text file. A PDF broke an assumption in the first pass — `fillCoverLetterIfPresent` pasted the file's raw bytes into textarea-style fields, which for a PDF would have sent employers binary garbage. Now format-aware:
- **Path is configurable** via `master_cover_letter_path` in `profile.yaml` (`config.Profile.MasterCoverLetterPath`), defaulting to `master_cover_letter.txt` when unset. The letter can be swapped for a differently-named file by editing `profile.yaml` alone, no rebuild.
- **Upload fields** get the file byte-for-byte under its real extension (`cover_letter.pdf`), so employers receive the properly formatted document rather than a flattened text rename.
- **Paste fields** go through new `parser.ExtractDocumentText`, which extracts text from a PDF and reads a text file as-is. If extraction yields nothing usable the letter is skipped entirely rather than pasted raw — the same best-effort contract as the rest of this path.
- `SaveApplication`'s stored record also uses the extracted text, so `coverletter.txt` in the application folder stays human-readable instead of holding PDF bytes.

Tests added: `TestFillCoverLetter_UploadsPDFUnderPDFName` (asserts both the `.pdf` name and byte-for-byte content), `TestFillCoverLetter_NeverPastesRawPDFBytes`, `TestExtractDocumentText_PlainText`, `TestExtractDocumentText_MissingFile`. **Verified live against the real file:** `Omni_CoverLetter.pdf` extracts to 2151 chars of clean text with no `%PDF` markers, and `profile.yaml` loads `master_cover_letter_path = "Omni_CoverLetter.pdf"`.

**Amended again 2026-07-24 (upload now actively preferred):** user asked whether the letter could be *uploaded* wherever the form offers it, the way the resume already is, instead of being typed into a text section. It could not, reliably — the first pass only uploaded when the mapper's `cover_letter` selector happened to land on a file input, and the mapper frequently points that key at a paste textarea on forms that also expose an upload control, silently downgrading those applications to flattened plain text. Now: the mapped selector is tried first (form-specific), then `coverLetterFileInputSelectors` is searched directly for an upload control, and only with no file input anywhere does it fall back to pasting.

**Safety constraint worth preserving if this list is ever edited:** every entry in `coverLetterFileInputSelectors` is scoped to an attribute naming the field a cover letter. A bare `input[type='file']` would match the **resume** input on most forms, and `SetInputFiles` replaces a file input's contents outright — a loose selector would overwrite the resume with the cover letter and send the employer no resume at all. `TestCoverLetterFileInputSelectorsNeverMatchBareResumeInput` enforces this.

Tests added: `TestFillCoverLetter_PrefersUploadOverPasteWhenMappingPointsAtTextarea`, `TestCoverLetterFileInputSelectorsNeverMatchBareResumeInput`, `TestFillCoverLetter_FallsBackToPasteWhenNoFileInputExists` (9 cover-letter tests total).

**Amended again 2026-07-24 (cover letters switched off entirely, feature retained):** user decided cover letters aren't worth the complexity for now and asked to toggle them off without removing the feature. New `send_cover_letter` in `profile.yaml` (`config.Profile.SendCoverLetter`, read via `ShouldSendCoverLetter()`); set to `false` in the live profile. When off, `generateDocsFunc` returns an empty cover path, which `fillCoverLetterIfPresent` already treats as "do nothing" — no control lookup, no upload search, no paste. Everything else stays exactly in place (the PDF, `master_cover_letter_path`, the upload-preference logic, all 10 tests), so flipping it back to `true` restores full behavior with no code change.

**Typed as `*bool`, deliberately:** a plain `bool` would make Go's zero value mean "off", silently disabling cover letters for any profile written before this field existed. `nil` means send. `TestLoadProfile_SendCoverLetter` and `TestShouldSendCoverLetter_ZeroValueProfileSends` both pin that.

**Current live configuration** (`use_master_cover_letter: true` + `send_cover_letter: false`) is the fastest available: `ProcessJobApplication` is skipped entirely, and no cover letter is attached. Applications go out with `master_resume.pdf` alone.

**Verified live end to end 2026-07-25 (`/groom_backlogs`), on the restarted 82-job run (PID `3486446`, HEAD `375fcdb`):**
```
00:30:19 [Auto-Submit] Verified page is live. Generating tailored documents...
00:30:19 [Worker-1] Using master resume for Reddit (no per-job tailoring, cover letter disabled)
00:30:21 [Auto-Submit] Detected Greenhouse ATS. Filling out fields...
```
Document generation completed in **0 seconds** against a 15-20 minute baseline, the config resolved exactly as intended (master resume, no tailoring, no cover letter), and the run proceeded straight into form filling with no cover-letter interaction attempted anywhere. This also promoted `ScoreJob` to the sole remaining bottleneck at ~9m49s of the same cycle, filed as item #24.

**Two known caveats, neither blocking:** (1) the PDF has a fixed date ("July 20, 2026") baked into it, which will read as stale over time — regenerate the PDF periodically. (2) `ledongthuc/pdf`'s text extraction drops some line breaks ("July 20, 2026Hiring Manager"), so *pasted* letters read slightly run-together; uploads are unaffected since the file goes as-is, and upload is the more common shape on real ATS forms (Greenhouse and Lever's own cover letter controls among them).


---

## 21. [Separate the actionable manual-apply queue from historical failure noise](#21-separate-the-actionable-manual-apply-queue-from-historical-failure-noise)

**Table rationale cell (original):** Small: new `storage.LogManualRequired` → `applications/manual_queue.md` with docs-dir links; 431-entry legacy file archived; failures still go to `manual_submissions.md`

### 21. Separate the actionable manual-apply queue from historical failure noise
`applications/manual_submissions.md` is appended to by `LogFailedSubmission` for *every* failed auto-submit and now also for every `MANUAL_REQUIRED` job — 431 unchecked entries as of 2026-07-21, the vast majority being old failures (many from since-fixed bugs) that nobody will ever act on. The genuinely actionable entries (account-gated jobs with tailored docs saved) are indistinguishable from the noise. Options to evaluate: a separate `manual_queue.md` for `MANUAL_REQUIRED` entries with a link to the saved docs directory, or annotating entries with their funnel status and date so the file can be filtered; either way, consider a one-time archival of the pre-fix backlog (the DB remains the source of truth via status fields).

**Done 2026-07-22:** went with the separate-file option — new `storage.LogManualRequired` writes the queue, and the agent's `ErrAuthWall` branch calls it instead of `LogFailedSubmission`, which remains failure-only. The 431-entry legacy file was archived to `applications/manual_submissions_archive_2026-07-21.md` (filesystem-only — `applications/` is gitignored); `LogFailedSubmission` recreates its file fresh on the next genuine failure. Deliberately not done: backfilling the pre-fix `MANUAL_REQUIRED`/failure history into the new queue — the DB status fields remain the source of truth for anything older than tonight.

**Amended same night (user request):** everything now lives in one self-contained folder, `applications/needs_manual_apply/` — the queue file (`manual_queue.md`) plus each job's tailored-docs folder, moved there automatically by new `storage.MoveToManualApply` when the auth wall fires (collision-safe numeric suffixes; missing source folders are tolerated and marked "docs not found" in the entry). The sanitizer `SaveApplication` uses for folder names was extracted as `safeCompanyDirName` and shared, fixing a link bug in the first pass where queue entries used the raw company name ("en-US") while docs actually lived in the sanitized folder ("en_US"). The existing HealthCatalyst entry and its docs were migrated in. Tests: `TestMoveToManualApply` (move, collision suffix, missing source), updated `TestLogManualRequired` (new path, docs-less entry).


---

## 20. [Dashboard tile for the MANUAL_REQUIRED queue](#20-dashboard-tile-for-the-manual_required-queue)

**Table rationale cell (original):** Minutes: the metrics API already served `manual_required` (bug #18's fix); added the card, a "Needs Manual Apply" mini-status, and the backend last-manual query

### 20. Dashboard tile for the MANUAL_REQUIRED queue
Bug #18's fix (2026-07-21) routes account-gated ATS jobs (Workday) to a new `MANUAL_REQUIRED` funnel status with tailored docs already saved under `applications/<company>/` — these are the highest-value action items the pipeline produces, since a human applying manually with ready-made docs converts far better than a failed automation. The `/api/metrics` endpoint already returns the count (`manual_required`) and `statusReason` explains the status; only `cmd/dashboard/index.html` needs a card (mirror the existing stat-card pattern) plus, ideally, a "most recent manual-required" mini-status like the last-skipped/last-failed ones.

**Done 2026-07-22:** orange-accented "Manual Queue" stat card (new `--neon-orange` var, mirrors every hover/glow rule of the existing cards), a third "Needs Manual Apply" mini-status in the secondary row (which now uses `auto-fit` columns to hold three cards responsively) showing the most recent `MANUAL_REQUIRED` job's company/title/time, and the matching `last_manual_*` fields + query in `serveMetrics`. Verified live against the running DB: `manual_required:1` with the HealthCatalyst SRE entry rendered. Dashboard rebuilt and the running instance restarted on the new binary.


---

## 19. [Prompt-injection/hidden-content detection CSV log](#19-prompt-injection-hidden-content-detection-csv-log)

**Table rationale cell (original):** User-requested 2026-07-21: turn the QuarantineLayer's transient block-and-log into a reviewable record of what was actually found on scraped career pages

### 19. Prompt-injection/hidden-content detection CSV log (Done 2026-07-21)
**Request:** the security guardrail (`pkg/security.QuarantineLayer`, wrapping `promptsec`) had already blocked several real detections live during this session's testing (`system_prompt_leak`, `encoding_attack`, `role_manipulation`, etc. — visible many times in `career_agent.log`), but the only record was a transient log line with a Go-formatted `%v` dump of the threats, no reviewable structured record of what was actually found or on which page.

**Shipped:** `QuarantineLayer.CheckPayloadDetailed` exposes the underlying `[]promptsec.Threat` (type, severity, message, matched text, guard, match position) instead of only an error string. Wired into `AttemptSubmit`'s live Learner Module branch — the only one of the three `CheckPayload` call sites that actually inspects scraped site content in the live pipeline (the other two check internal RAG output or dead code, see bugs.md's orphaned-process note for how thoroughly this session verified what's actually exercised). `storage.LogPromptInjectionDetections` appends one CSV row per detected threat to `applications/prompt_injection_detections.csv` (`detected_at, url, company_name, threat_type, severity, guard, message, matched_text, match_start, match_end`), using `encoding/csv` for correct quoting of arbitrary matched text. Already covered by the existing `applications/` gitignore rule.


---

## 18. [Configurable worker concurrency](#18-configurable-worker-concurrency)

**Table rationale cell (original):** Shipped 2026-07-20 alongside `bugs.md` #3

### 18. Configurable worker concurrency (Done 2026-07-20)
`cmd/agent/main.go` hardcoded `numWorkers := 10` with the comment "Increased to 10 workers for massive concurrency on Paid Tier" — tuned for a paid API backend, not the local-Ollama-by-default setup `.env.example` recommends. Added `defaultWorkerCount()`: returns `10` when `LLM_PROVIDER` is a paid backend, `1` when unset/`ollama`. Initially tried `2` as a "sensible lower default," but a live test showed even 2 workers against Ollama's single request slot (`-np 1`) causes the second worker's call to queue and blow past its own 10-minute client timeout before ever being served — `1` is the value that actually matches the server's real capacity and eliminates the queuing/timeout churn entirely (confirmed live: a real job completed tailoring in ~5 min with zero errors under 1 worker). Also added a `WORKER_COUNT` env var to override either default explicitly.

---

## 16. [Automated assessment/screening solver](#16-automated-assessmentscreening-solver)

**Table rationale cell (original):** Shipped for single-page custom questions reached via the generic Learner Module (SmartRecruiters, Ashby, Homerun, Pinpoint, BambooHR, etc.); Workday's specific multi-step pre-screening flow this item originally described is still unreachable — Workday is auth-gated pre-form (bugs.md #18/#50) and item 11's multi-step state machine still doesn't exist

### 16. Automated assessment/screening solver (Done 2026-07-24, partial)
Some ATS platforms (Workday in particular, see item 11) insert pre-screening multiple-choice or numeric questions before the standard form fields. Use the existing LLM provider abstraction (`pkg/mcp`) to read the question text extracted from the pruned DOM and answer strictly from `USER_PROFILE.md`/`profile.yaml` facts — never invent an answer not grounded in those files, to avoid the agent lying on a legal application question. Likely lands as a new `pkg/submitter` function invoked from within the multi-step flow (item 11), so sequencing after item 11 is natural but not strictly required if a screening page can appear standalone.

**Shipped 2026-07-24 (user-requested, asked whether the agent could see and answer custom screening questions with a naturally-written response inferred from the resume):** `ExtractFormMapping` (`pkg/mcp/client.go`) now proactively detects custom questions during the first fill pass — not just reactively via `SolveValidationErrors` after a validation failure, which was both wasteful (contributed to bugs.md #52's oversized retry payloads) and incomplete (a non-required custom question was previously just left blank). Generates a grounded, naturally-written first-person answer per question, reusing the same never-invent-EEO-answers safety rule already established for `SolveValidationErrors`. `handleDynamic` fills each one via the existing label-fallback chain, best-effort. 2 new tests, `go build/vet/test ./...` all pass. See bugs.md's Details section is not needed — filed here since this was a feature gap, not a defect.

**Not covered (this is why the row says "partial"):** this generalizes to any single-page ATS form reached via the generic Learner Module (SmartRecruiters, Ashby, Homerun, Pinpoint, BambooHR, applytojob.com, recruitee.com, etc.) — but the original item's specific Workday multi-step pre-screening scenario is still unreachable. Workday is routed straight to `MANUAL_REQUIRED` before any form is ever seen (bugs.md #18/#50, account-gated pre-auth), and item 11's multi-step "Next"-button state machine, which a Workday screening flow would need, still does not exist. Re-open or file a fresh item if item 11 gets built and Workday-specific screening needs revisiting.


---

## 15. [Email/portal conversion-rate analytics](#15-emailportal-conversion-rate-analytics)

**Table rationale cell (original):** Small-medium; re-scoped down from a full tracker build since `pkg/tracker` already exists — just needs the analytics layer on top. **2026-07-29 groom correction:** the analytics themselves are intact — `by_source`, `by_variant` and `interview_rate_pct` are still computed and served on every `/api/metrics` poll — but #426's rewrite deleted the only view that rendered them, so this feature is currently reachable only by reading raw JSON

### 15. Email/portal conversion-rate analytics (Done 2026-07-24)
The original backlog entry called for building `pkg/tracker` from scratch; verified 2026-07-19 that it already exists (`pkg/tracker/imap.go`, wired up via `cmd/tracker/main.go`) and polls IMAP for rejection/interview signals per the README's "Email Tracker" feature. What's missing is the analytics layer: no code computes or surfaces a conversion rate (interviews or offers ÷ applications sent, broken down by role/source/ATS/tone). Add a query/report path — a `pkg/tracker` or `pkg/storage` function that aggregates `applications.db` outcomes, surfaced via the existing `cmd/dashboard` web UI or a new CLI report command. This is a prerequisite for item 13 (A/B testing needs a signal to compare variants against).

**Re-scoped before implementing:** dropped "role" (job_title is free text, not categorical — grouping by it is noise) and "tone" (`CoverLetterTone` in `pkg/config/profile.go` is a single global config value today, not a per-application variant, so there's nothing to break a rate down *by* yet — meaningful only once item 13 exists) from the original description. Kept source/ATS, since it's cleanly derivable from the URL.

**Shipped 2026-07-24:** `pkg/tracker/imap.go` only ever moves a `job_funnel` row from `APPLIED` to `REJECTED` or `INTERVIEW_REQUESTED` (never a distinct `OFFER` status), so "ever applied" = `status IN ('APPLIED','REJECTED','INTERVIEW_REQUESTED')`. Added `GetConversionStats()` (overall) and `GetConversionStatsBySource()` (grouped by an ATS-label CASE expression: Greenhouse/Lever/Workday/SmartRecruiters/Ashby/Other) to `pkg/storage/manager.go`, mirroring the existing `SourceOutcomeBreakdown` pattern. `cmd/dashboard` queries the same logic directly against its own local DB connection (it never imports `pkg/storage`, matching every other query already in `serveMetrics`) and surfaces a new "Interview Rate" stat card plus a "Conversion by Platform" table in `cmd/dashboard/index.html`. 4 new tests in `pkg/storage/manager_test.go`. `go build/vet/test ./...` all pass. Verified live: the real DB currently has zero tracked rows (the 82-job re-verification run reset all prior `APPLIED` rows back to `DISCOVERED`), confirmed via `/api/metrics` that the empty state serializes correctly (fields omitted/zeroed, no divide-by-zero); populated-state rendering verified via a throwaway DB copy with synthetic `INTERVIEW_REQUESTED`/`REJECTED` rows, screenshotted through Firefox (this box's Chromium build silently zeroes all text layout — see `bugs.md`'s Operational Trap notes) — card and table both render correctly.

**Delegation note:** first attempted via `agy --model gemini-3.1-pro-high`; all three Gemini tiers were quota-exhausted ("Resets in 94h"). Stepped to `gpt-oss-120b-medium` per protocol, but that model only echoed a restated plan and asked clarifying questions already answered in the brief, then returned nothing (headless mode has no way to answer back). Implemented directly instead rather than spend a third delegation round on an already-fully-scoped small task.


---

## 13. [Adaptive resume A/B testing](#13-adaptive-resume-ab-testing)

**Table rationale cell (original):** Shipped variant generation/tagging/reporting; the "adaptive" weighted-selection policy deferred — no real outcome data exists yet to base it on

### 13. Adaptive resume A/B testing
`pkg/config/profile.go` has a single `CoverLetterTone` string field — one tone per run, no variant generation or outcome tracking. Building real A/B testing requires: (1) generating 2+ resume/cover-letter tone variants per application, (2) tagging which variant was sent in `applications.db`, (3) joining that against interview/rejection outcomes once item 15's conversion analytics exist, and (4) a selection policy that shifts weight toward the higher-performing variant. Do item 15 first — without conversion data, "A/B testing" has no signal to pivot on. Verified 2026-07-19: confirmed `CoverLetterTone` is a single scalar config value with no variant or outcome-tracking code.

**Groom note 2026-07-24:** item 15 (its stated prerequisite) shipped 2026-07-24 — `GetConversionStats()`/`GetConversionStatsBySource()` exist and are live on the dashboard. Re-verified `CoverLetterTone` is still a single scalar with zero variant-tracking code (unchanged). But the underlying blocker this item cares about — real outcome data to pivot on — was still absent at that time: the live DB had zero `APPLIED`/`REJECTED`/`INTERVIEW_REQUESTED` rows. The later verification history and current state are consolidated in `documentation/task_journals/2026-07-25_monitor-live-run-and-fix-bugs.md`.

**Shipped 2026-07-24 (user-requested, "we need to improve this to get more results"), partial:** the "no outcome data to pivot on" blocker above is still genuinely true — so this ships everything buildable *without* real outcome data (steps 1-3 of this item's own description: generate variants, tag which was sent, and a reporting path to join against outcomes once they exist) and explicitly does not build step 4 (a selection policy that shifts weight toward the better-performing variant — there is nothing to weight toward yet).

New `config.Profile.CoverLetterTones []string` (plural, optional — `cover_letter_tones` in `profile.yaml`) holds 2+ tone variants to A/B test; `config.SelectToneVariant` picks one at random per application (random rather than round-robin, so the split doesn't correlate with batch-order/time-of-day effects that would confound a later comparison) and returns a stable label (`variant_0`, `variant_1`, ...). `job_funnel.tone_variant TEXT` (new column, idempotent migration) records which variant actually got used; `storage.UpdateToneVariant` writes it right after `SaveApplication` succeeds in `cmd/agent`'s worker loop, and `storage.GetConversionStatsByVariant` (mirrors item 15's `GetConversionStatsBySource` exactly) reports interview/rejection/pending counts per variant, grouping any untagged row under `"unspecified"` so the total always reconciles with `GetConversionStats`. Dashboard: new "Conversion by Cover-Letter Tone Variant" table (`cmd/dashboard`), same pattern as item 15's "Conversion by Platform" table.

**Deliberately left `cover_letter_tones` unpopulated in the user's live `profile.yaml`:** inventing personal-brand tone variants (the actual wording sent to real employers) is not something to decide unilaterally on the user's behalf — that's a content/voice decision for the user to make deliberately whenever they want to actually turn A/B testing on. Until then this ships as inert, opt-in infrastructure: every application keeps using the existing singular `CoverLetterTone`, unchanged.

Tests: `TestLoadProfile_CoverLetterTones`, `TestSelectToneVariant`, `TestMigrateJobFunnelToneVariant`, `TestUpdateToneVariant`, `TestGetConversionStatsByVariant`. `go build/vet/test ./...` all pass. Verified live: rebuilt and restarted the dashboard (`/tmp/career_dashboard_v4`), confirmed `/api/metrics` correctly omits `by_variant` in the current zero-tracked-rows state (no error, no divide-by-zero) — same empty-state handling item 15 already established for `by_source`.


---

## 12. [Niche data source scrapers](#12-niche-data-source-scrapers)

**Table rationale cell (original):** Shipped HN "Who is Hiring" (confirmed live: 228 real postings from the current thread alone); Otta/Wellfound/YC deferred with live evidence, not assumption — see Details

### 12. Niche data source scrapers
`pkg/scraper/funnel.go`'s `TargetATS`/`atsDomains` lists cover 16 traditional ATS platforms (Greenhouse, Lever, Workday, etc.) reached via SerpApi/DuckDuckGo dorking. YCombinator "Work at a Startup", Otta, Wellfound, and HackerNews "Who is Hiring" are not ATS-hosted job posts — they need their own scraper implementations (likely direct API/HTML parsing per source, not the dork-and-match pattern `funnel.go` uses for ATS domains), wired into `FunnelEngine` as additional sources. Verified 2026-07-19: no references to any of these four sources exist anywhere in `pkg/`.

**Correction 2026-07-24 (`/groom_backlogs` pass):** the 2026-07-19 verification is now slightly stale — `pkg/scraper/scraper.go:162-165` has three `log.Println` stub lines ("Scraping We Work Remotely (Implementation pending)", "...Wellfound...", "...Built In (Remote)...") that fire unconditionally on every scrape pass. They are pure log noise, not implementations — no HTTP fetch, no parsing, nothing appended to the job list — so the item's actual scope and effort are unchanged, but "no references... exist anywhere in `pkg/`" is no longer literally true. Worth knowing before starting: real work replaces these three log lines, doesn't add to an empty file. (Those three log lines live in `scraper.go`'s `Engine.FetchJobs`, confirmed dead code — zero live call sites, `cmd/agent` only ever uses `FunnelEngine.DiscoverJobs` — so they were never actually reachable in production regardless.)

**Shipped 2026-07-24 (user-requested, "we need to improve this to get more results"), partial:** live-checked all four named sources via `WebFetch` before writing any code, per this repo's own established rigor (build against verified live structure, not assumption):
- **HN "Who is Hiring": shipped.** The Algolia HN Search API (`hn.algolia.com/api`) is public, unauthenticated, stable, and well-documented. New `pkg/scraper/hackernews.go`: `discoverWithHackerNews` (wired into `FunnelEngine.DiscoverJobs` alongside the existing `discoverWithRemoteOK`), finds the current month's thread via `latestWhoIsHiringStoryID` (excluding the sibling "who wants to be hired"/"freelancer" threads the same author posts), fetches all comments via paginated `fetchHNThreadComments`, and `parseHNJobPosting` extracts the first http(s) link as the posting URL plus a best-effort company/title from the thread's loose "Company | Title | Location" convention (never enforced, so this is cosmetic-best-effort only — same acceptable-quality bar as bug #19's Workday locale-segment company names — the real job description still gets fetched from the URL itself once it reaches `cmd/agent`'s worker loop, same as every other source). Skips email-only postings (no apply-by-email pathway exists in this pipeline) and HN-internal links.
- **Wellfound: deferred, not built.** Live-fetched `wellfound.com/jobs` → **HTTP 403 Forbidden**, bot-gated. Attempting this would mean real Playwright browser automation, with the same class of anti-bot risk already fought at length elsewhere in this codebase (bugs #45/#46, SmartRecruiters DataDome) — high effort, uncertain payoff, not attempted blind.
- **Otta: deferred, not built.** Live-fetched `otta.com/jobs` → **301 permanent redirect to `uk.welcometothejungle.com/jobs`**. Otta no longer exists as an independent product — merged into/rebranded as Welcome to the Jungle, an unverified different site. The original item description is stale for this source specifically; re-scope or drop rather than build against a URL that no longer serves the intended product.
- **YC "Work at a Startup": deferred, not built.** Live-fetch of `workatastartup.com/jobs` was inconclusive on whether listings are server-rendered or gated behind a YC login. Deferred rather than guessing at selectors against unverified structure.

Tests: `TestParseHNJobPosting` (6 cases), `TestLatestWhoIsHiringStoryID`/`_NotFound`, `TestFetchHNThreadComments_Pagination`, `TestDiscoverWithHackerNews` (confirms nested replies are correctly excluded from top-level postings). `go build/vet/test ./...` all pass. **Live-verified against the real API** (temporary, uncommitted test, deleted after): found the real current thread, fetched 437 real comments, correctly parsed 228 top-level postings with a usable URL — including at least one Greenhouse short-link (`grnh.se/...`) that resolves into the existing dedicated Greenhouse handler. A genuinely productive new source, not just a theoretical one.

**If Wellfound/Otta/YC are wanted later:** Wellfound and the Welcome-to-the-Jungle replacement for Otta would need real Playwright-driven scraping (open item, would need to reuse `pkg/submitter/browser.go`'s existing anti-bot-aware navigation rather than a bare HTTP client); YC would need someone with an actual YC/Work-at-a-Startup account to confirm the login-gate question live before any code gets written against it.


---

## 11. [Multi-step form logic (Workday-style)](#11-multi-step-form-logic-workday-style)

**Table rationale cell (original):** Closed at the user's explicit confirmation: Workday (this item's original motivating platform) is already routed to `MANUAL_REQUIRED` before any form is ever reached (bugs.md #18/#50), and Taleo (the only other named platform) has never once appeared across this project's entire live history — no live path exists that would ever exercise this code

### 11. Multi-step form logic (Workday-style)
`pkg/submitter/dynamic.go` maps `workday.com` and `taleo.net` to template names (`WorkdayTemplate`, `TaleoTemplate`), but nothing in the codebase drives a multi-page "Next" button flow — the submitter pipeline (`TwoStepVerification`, `AttemptSubmit`, `AttemptVisionSubmit`) operates on a single page load. Workday applications are frequently 4-8 pages (personal info, work history, EEO, review). Build a state machine that: fills the current page's mapped fields, detects and clicks the "Next"/"Continue" control, re-extracts and re-prunes the DOM on the new page, and repeats until a terminal "Submit"/"Review" state, persisting `ExecutionState` (already defined in `dynamic.go`) between steps so a crash mid-flow can resume instead of restarting. Verified 2026-07-19: confirmed via `grep` that only the ATS-domain-to-template-name mapping exists; no page-transition logic was found in `pkg/submitter/` or `pkg/scraper/funnel.go`.

**Re-scored 2026-07-24 (`/groom_backlogs` pass):** re-verified `ExecutionState{` has zero construction sites anywhere in the repo — still genuinely unbuilt, description accurate. But the ROI rationale has aged badly: bugs.md #18 (Resolved 2026-07-21) and #50 (Resolved 2026-07-23) route both Workday and Workable straight to `MANUAL_REQUIRED` via an `authGatedATSHosts` check that fires immediately after doc generation, *before* the Learner Module or any form-filling logic ever runs — this item's own multi-step machinery would never even be reached for Workday, since there's no pre-auth form of any kind (single- or multi-page) to drive. That leaves Taleo as the sole remaining justification, and a `grep -c taleo career_agent.log` plus a `job_funnel` URL-pattern query both came back zero — no Taleo posting has ever entered this project's discovery funnel, live or historical. Value dropped from 6 to 2 (a single, never-yet-observed platform) against unchanged Effort 5, landing at 0.4 — below the 0.5 floor. Flagged ⚠️; needs explicit user confirmation (and ideally a real Taleo sighting) before working.

**Re-verified again 2026-07-24 (later `/groom_backlogs` pass, same day):** `grep -c taleo career_agent.log` and a `job_funnel` URL LIKE query both still return zero — no change since the note above; recommendation to the user stands as close (Taleo has never once appeared across this project's entire live history) or leave open only if the user specifically wants Taleo coverage speculatively. **Model reassessed:** stepped up from Sonnet 5 to Opus 5 — if ever worked, this is a genuine state-machine architecture task (multi-page flow detection, DOM re-extraction per step, crash-resumable `ExecutionState` persistence), a better fit for Opus 5's deeper reasoning than Sonnet 5's general-purpose tier, independent of this item's current below-floor status.

**Closed 2026-07-24 (user confirmed):** asked the user directly whether "work #11" meant closing it (this item's own standing recommendation) or building the state machine anyway despite the below-floor score — user chose close. No code exists to remove (`ExecutionState{}` was never constructed anywhere, confirmed twice this session). Re-open only if a real Taleo posting is ever observed live, or another multi-step (non-auth-gated) ATS platform shows up in discovery.


---

## 10. Cron-Driven Drip Campaigns

**Table rationale cell (original):** The earlier “Done” assessment was wrong: `--daemon` only changes a log line and exits after one ordinary batch. The broken documented behavior is now ranked in `bugs.md` rather than duplicated here

---

## 9. Metrics Dashboard (Web)

**Table rationale cell (original):** Found already implemented (`cmd/dashboard`) during the 2026-07-19 backlog rebuild; stale in the old list

---

## 8. Visual Reasoning (VLM) for Form Submissions

**Table rationale cell (original):** Found already implemented (`pkg/submitter/vision.go`) during the 2026-07-19 backlog rebuild; stale in the old list

---

## 7. Playwright Scraper Fallback (DuckDuckGo)

**Table rationale cell (original):** Shipped before this backlog restructure

---

## 6. Stealth & Proxies (webdriver overrides)

**Table rationale cell (original):** Shipped before this backlog restructure

---

## 5. Graceful DB Degradation (self-healing cache)

**Table rationale cell (original):** Shipped before this backlog restructure

---

## 4. DOM Pruning/Minification

**Table rationale cell (original):** Shipped before this backlog restructure

---

## 3. Playwright Dynamic Generation (self-learning DOM mapper)

**Table rationale cell (original):** Shipped before this backlog restructure

---

## 2. Dynamic Source Discovery (FunnelEngine)

**Table rationale cell (original):** Shipped before this backlog restructure

---

## 1. V2 Architecture Blueprint

**Table rationale cell (original):** Shipped before this backlog restructure

---
# 498. Company and role-family duplicate/cooldown protection

**Completed 2026-08-01.** Added an opt-in `duplicate_cooldown_days` profile setting. At a positive value, the pipeline records each discovered job's location and remote metadata, then skips it only when a recent confirmed application has the same normalized company, role family, seniority, location, and remote classification. The matcher intentionally declines incomplete metadata and does not use fuzzy title or company matching, so a distinct role or location is never silently suppressed. Existing profiles default to `0`, which keeps the prior behavior and acts as the explicit override.

SQLite receives idempotent `job_location` and `is_remote` columns without backfilling legacy rows, because that historical information cannot be reconstructed truthfully. The dashboard labels the persisted `duplicate_cooldown` skip reason. Regression tests cover same-identity suppression, different seniority/role/location/remote cases, incomplete and expired history, idempotent migration, profile validation, pipeline behavior before expensive network work, and dashboard text. Full `go build ./...`, `go vet ./...`, `go test ./...`, and `gofmt -l ./cmd ./pkg ./internal` passed. Live verification is intentionally limited to synthetic fixtures: the real database has too little confirmed-application volume to establish an observed duplicate cohort.

---

# 483. Confirm whether zombie `career_agent_bin` processes need reaping

**Closed 2026-08-01.** A fresh `ps` scan found no live or zombie `career_agent_bin` process. The previously observed zombies therefore did not persist as a process-table leak. No code or operational-documentation change is warranted from this one observation; reopen only if a future scan finds a zombie with a still-live parent that does not reap it.
## 486. Safe local-model delegation harness

**Completed 2026-08-02.** Added `cmd/localdelegate` and `internal/delegation`, a framework-independent, local-Ollama-only boundary for bounded repository work. Phase one accepts a sanitized brief up to 32 KiB and returns a strict proposal JSON document containing the finding, root cause, planned paths, implementation summary, success and failure tests, risks, questions, and readiness. Unknown fields, malformed or oversized responses, missing tests, unsafe paths, and obvious credential markers are rejected.

Phase two requires the SHA-256 digest of the exact reviewed proposal plus a reviewer identifier. It can write only a candidate unified-diff artifact, validates that every path is in the reviewed proposal, and never applies that diff. The command contains no shell, Git, browser, email, production-database, application-data, or credential capability. It also refuses whenever the production agent lock is held, so background delegation yields to application work without a force override. Focused contract and command lock tests plus the full Go build, vet, test, formatting, and diff checks pass. Documentation in `README.md` and `documentation/local_delegation.md` records the operating contract and reviewer responsibilities.

---

### 512. Application Mode selector and configurable fit threshold
Closed — full account archived in `documentation/task_journals/2026-08-04_application_modes.md`

## 509. Make an empty application queue explainable before it silently stalls applications

**Completed 2026-08-02.** Filing brief and design, preserved here by the 2026-08-06 groom pass: the live backlog row pointed at this archive entry, but the entry had never actually been written, so the live file held the only copy.

**Found 2026-08-02** during a safe live mission-status check. The running dashboard's read-only `/api/metrics` reported `eligible_queue: 0`, while `/api/agent/status` reported `running: false`. That proves no application can currently progress, but not whether the agent was intentionally stopped, discovery failed, all results were duplicates or excluded, or filters yielded no eligible jobs. The dashboard's “last skipped” record is historical and cannot answer that question. #495 deliberately suppresses its no-progress alert when the eligible queue is empty, so it cannot provide this diagnosis either.

**Proposed direction:** persist a privacy-safe aggregate result for each discovery refresh (started/finished time; source-level attempted/new/duplicate/excluded/error counts; sanitized error class), expose the latest result in `/api/metrics`, and show an actionable dashboard state when the agent is stopped or the most recent refresh produced no eligible jobs. Do not retain job descriptions, URLs, resumes, application answers, raw errors, or credentials; do not start the agent, relax filters, requeue jobs, or submit applications automatically.

**Acceptance criteria:** a deterministic discovery fixture can produce new eligible jobs, zero results, all-filtered or duplicate results, and a source error; the API/dashboard state distinguishes each case without leaking job or personal content. An intentionally stopped agent is visibly distinct from a running agent awaiting its next refresh.

**Automated tests:** table-driven storage/API tests for the aggregate outcomes and a dashboard-handler test for the stopped/empty state.

**Safe live verification:** query only the dashboard's aggregate endpoint after one controlled discovery refresh; confirm the displayed explanation agrees with the persisted aggregate without inspecting raw job data.

**Boundaries:** this is observability and diagnosis, not a discovery-source rewrite (#508), queue-admission change (#492), or autonomous application-start authority. Its theme has one shipped precursor (#495), hence Decay 0.5; Value 6 and Effort 3 yield `1.0`.

---
## 497. User-approved application-answer memory

**Completed 2026-08-13.** `pkg/answers` is the Approved Answer Vault: `approved_answers` and `answer_aliases` in the existing SQLite database, created through an additive `EnsureSchema` in the same migration chain as `EnsureAssistedSchema`, with no destructive migration and no data loss on upgrade.

Resolution is deterministic and has no model in it: an operator-approved alias for the exact phrasing, then an approved answer under the question's own normalized key with scope precedence (company → ATS → global), then a curated pattern table over the operator's `pii.yaml` facts, and otherwise unresolved. Approving an answer writes an alias for the wording the employer actually used, which is the mechanism behind the acceptance criterion: "Are you legally authorized to work in the United States?", "Are you authorized to work in the US?" and "Do you currently have authorization to work in the United States?" all reach the same answer with no LLM call, no similarity threshold, and no stemming.

**The safety rule is enforced in the store rather than in any caller.** `answers.Store.Save` refuses a sensitive answer that does not carry both an explicit operator provenance and an explicit reuse decision, returning `ErrSensitiveNeedsApproval`; a caller may raise a question's classification but never lower it, because the store classifies the question itself, so a client that labels an attestation "routine" is not believed. A `generate_per_job` answer ("Why this company?") is refused outright as reusable. `pii.yaml` seeding writes suggestions with reuse withheld and never overwrites an operator approval. Curated sensitive patterns resolve to a *pre-filled proposal* the operator confirms and never to an auto-fill — `Resolution.Resolved` and `Resolution.AutoFill` are separate fields precisely so that offering a suggestion does not imply permission to type it onto an employer's legal attestation.

The learning loop runs through the dashboard: a refill that leaves unanswered questions moves the application to the new `needs_answers` state, the operator answers them on an exception-only card, and an optional "save this as my approved answer" control — unchecked by default, and requiring a second, separate acknowledgement for any declaration — records the approval with provenance and timestamps. A refusal is counted and reported back rather than swallowed, so an operator who ticks one box but not the other is told the answer was sent but not remembered.

**Tests:** `pkg/answers` covers exact/alias/pattern resolution, scope precedence, alias-on-approval, revocation, provenance, the four ways a sensitive save can be under-authorized, the classifier's downgrade refusal, per-job answers never being stored, seeding idempotence, and a table-driven assertion that *no* curated sensitive pattern can auto-fill — which is the safe live verification this row asked for, run as a unit test over the pattern table so a pattern added later cannot quietly opt out. `cmd/dashboard` covers the endpoint end to end, including that an answer to a question the application never asked is refused, and that the assisted queue projection never serves back an answer the operator typed.

Scored 3×1.0÷6 = 0.5 when filed, on 26 observed `MANUAL_REQUIRED` rows. It was worked as part of a larger operator-directed effort to reduce human seconds per submitted application, not selected by ROI rank.

---
## 537. Apply-session auto-advance is only exercised by unit tests, never by a live multi-application run

**Completed 2026-08-13.** Run against a copy of `applications.db` from inside the `career-agent` container, on two real Greenhouse postings, on a dashboard bound to `127.0.0.1:8099` while the production instance on `:8080` was left untouched. Nothing was submitted to any employer: auto-advance was driven by Skip rather than Confirm, by explicit agreement with the user before the run.

**Observed timeline, second pass (after the fixes below):**

```
10:08:35  application 1 opened automatically  (no click; "Verified destination: application")
10:09:48  refill: 6 fields filled, 1 approved answer reused, 17 questions surfaced
10:10:04  2 operator answers entered into the real form
          "Review the form and submit it yourself; Career Agent will not click Submit."
10:10:16  Skip -> "This application's place in the apply session reached an outcome; closing the browser."
10:10:38  application 2 opened automatically   <- the claim this row exists for
10:18:38  browser 2 closed with no confirmation -> session paused,
          reason browser_closed_without_outcome, item returned to pending, nothing counted
```

Every claim is now observed rather than inferred: the migration on an existing database, session-driven browser launch, the fill report and its question list, the answer round trip through `pending_answers` into a real employer form, auto-advance on a terminal outcome, and the pause-on-closed-browser rule. The 6 filled fields were First Name, Last Name, Email, Phone, LinkedIn Profile and Current Company; the sensitivity classifier correctly marked all nine of that form's immigration and protected-class questions sensitive and offered suggestions only where `pii.yaml` had configured one, leaving the EEO questions blank.

**Not verified, and deliberately so:** a real employer submission followed by Confirm writing an `APPLIED` row. The confirm path's session advance is unit-tested and shares `advanceApplySessionItemTx` with the skip path that was exercised live, but no application was actually sent.

> **Follow-up 2026-08-13, after #543 shipped — this gap is now closed.** The operator submitted a real application they wanted, closed the assisted browser, and clicked Confirm. Exactly one `APPLIED` record was written: one `job_funnel` row, one `applied_jobs` row, one `assisted_applications` row (`completed`, `manual_user_confirmation`), all three sharing an identical nanosecond timestamp, with `pending_answers` left empty and no other `APPLIED` row created. Two caveats that run did *not* close: the operator submitted directly rather than using Continue, so the refill/answer path never ran (`assisted_fill_summary`, `application_questions` and `approved_answers` are all still empty); and it was a one-off Assisted Apply rather than a session, so **auto-advance after a confirmation** remains verified only by code and tests. The original paragraph above is left as written because it is an accurate record of what *this* run did.

**The run found five defects, all of which passed `go test`.** Two before a browser was ever opened (bugs.md #538, #539), three from the live form (#541, #542, #540), plus one filed for later (#543). The two that mattered:

- **#542** — auto-advance did not work on the skip path at all. Nothing told `cmd/assist` its work was done, so the skipped application's browser held the only visible-browser lease forever, and the resulting lease conflict was misread as an unknown outcome and paused the session. The first pass ended with the operator pressing Skip and nothing happening.
- **#541** — the Answer Vault's two-checkbox guarantee failed in the unsafe direction on a real form: an answer the card showed as routine was stored as a sensitive declaration with reuse permission the operator was never asked for. The trigger was **#540**, the classifier treating the employer's own name ("Affirm") as attestation vocabulary.

That is the value this row was filed to obtain. None of the five was reachable from the test suite: three depended on a real employer's DOM, one on a database created by an earlier release, one on a non-default port.

**Live PII scan of `dashboard.log`:** no email addresses, no phone numbers, no question prompt text, and no operator answer. The only page-derived content was Playwright's own locator-retry diagnostics, filed as #543 because the same diagnostic on a filled control would print a typed value.

---

## 557. `distinctiveRoleWords`'s broadest single-word entries admit clearly off-track titles that happen to share one generic word with a configured role

**Completed 2026-08-17.** Reproduced with a failing test first: `Senior Business Systems Analyst, Merchandising Systems`, `Sr. Strategy & Operations Analyst, Deal Desk`, `Senior Technical Customer Support Engineer`, `Senior Operations Specialist`, `GTM Operations Lead`, `Product Specialist: Platform`, `Technical Recruiter — Production Engineering`, `GTM Business Operations Analyst`, and `Cloud Support Administrator` all reached `FitAdjacent` against the real (2026-08-17 re-targeted) profile.yaml role list, purely via one shared generic word (`systems`/`operations`/`support`/`production`/`cloud`/`platform`).

**Fix (`pkg/config/eligibility.go`, `pkg/config/title_policy.go`):** `distinctiveRoleWords` dropped `systems`, `operations`, `support`, `network`, `security`, `api`, `production`, and `cloud` entirely rather than narrowing them with a threshold. Every configured role that legitimately needs one of those words (`Cloud Platform Engineer`, `Production Support Engineer`, `Network Automation Engineer`, `Cloud Systems Administrator`, ...) is itself a full phrase in profile.yaml and already matches via the phrase check in `matchesConfiguredRole` — the single-word fallback was never actually load-bearing for those words, only for the other (genuinely distinctive) word in the same phrase. `platform` was kept in the fallback (needed for adjacent matches like "Platform Architect" against a "Platform Engineer" role list), but given a new structural rule instead of deletion: `platformFollowedByOccupationNoun` only counts a bare "platform" match when it is immediately followed by an IC-occupation noun (engineer/architect/specialist/developer/administrator/consultant) somewhere in the title. That is what separates "Platform Architect" (accepted: platform modifies the following role noun) from "Product Specialist: Platform" (rejected: platform is a trailing qualifier after the noun, not a modifier of one).

Regression coverage: `TestClassifyTitle_GenericSharedWordFalsePositives` pins all nine reproduced false positives to `FitReject`; `TestClassifyTitle_GenericSharedWordFix_PreservesRealMatches` pins the real must-accept matrix (every literal profile.yaml role phrase, plus the fallback-dependent "Platform Architect", "DevOps Consultant", and "Network Reliability Engineer" via the still-strong "reliability" word) to non-reject. `pkg/scraper`'s pre-existing `TestTitleLooksRelevant` (the same canonical `ClassifyTitle` path, used as a cheap pre-scoring gate) had "Cloud Security Engineer" in its `keep` list relying on the now-removed generic "cloud"/"security" words; moved to `drop` with a comment, since that behavior was exactly the bug this row fixes and the same canonical policy backs both call sites by design.

Verified `ManagementTrackTitle`/#556's classifier, `stretchSeniorityTitle`, and geography policy were untouched; separately confirmed the two production examples that don't map to a shared-word mechanism ("Senior Technical Consultant", "generic Staff/Senior Software Engineer") already reject under the current profile.yaml with no code change — they were relics of the pre-2026-08-17 broader 121-entry role list, already resolved by that list's narrowing. `go build ./...`, `go vet ./...`, `go test ./...`, and `gofmt -l ./cmd ./pkg ./internal` all passed.

---
