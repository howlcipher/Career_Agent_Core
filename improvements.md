# 🚀 Improvement Backlog

This document is the authoritative, ranked backlog for Career Agent Core enhancements. It is designed so a fresh session (Claude Code, Gemini CLI, or Antigravity) can pick up the next item with zero prior chat context. It mirrors the format used by the AI Knowledge Library (`../ai_knowledge_library/improvements.md`), this project's sibling repo, and shares its Working Protocol with `bugs.md`.

**Everything in this file is explicitly nice-to-have.** `bugs.md` opens with a Usability Gate defining what "100% usable" means for this project; while that gate is unmet, closing bugs outranks working any Pending row here, regardless of score. Check the gate status before picking an item from this file.

**Every item here is free to build** — no paid signup, subscription, or API key required. Items that need a paid service live in `improvements_paywall.md` instead, kept separate on purpose so `/work_next_item` only ever autonomously picks free work; a paywalled item is only worked when the user explicitly names it in the current session.

## Working Protocol

This protocol applies to every worked task — improvements and bug fixes alike. Bugs are tracked in `bugs.md`, which mirrors this file's format and shares this protocol; a hotfix without a backlog row still gets steps 1 through 4 and 7.

1. **Check the Usability Gate first.** Read the gate checklist at the top of `bugs.md`. If it is not yet met, work the highest-priority open bug instead of anything in this file, unless the user explicitly asks for a specific improvement.
2. **Open a task journal.** Copy `documentation/task_journals/TEMPLATE.md` to `documentation/task_journals/YYYY-MM-DD_<slug>.md`. The journal is the resume point after a session limit or interruption: update it and commit at every milestone, and always keep its "Next step" line current so a fresh session can continue from the journal alone. It is a resume artifact, not permanent documentation — it gets deleted in step 7; anything durable (findings, decisions, verification evidence) must land in the item's Done note or a proper doc before the task closes.
3. **Pick a model for the item's Tier (every run, before starting).** The table's `Tier` column names a capability level, not a specific model: *mechanical* (one-line/one-file change, verification is obvious), *standard* (ordinary implementation work with a clear spec), or *deep-reasoning* (the difficulty is deciding what correct looks like, not typing it). Check what is actually available right now and pick a model that matches the tier: Claude Pro subscription (Claude Code), Google subscription via Antigravity CLI (`agy models` lists live names; quota is per model and shared across Gemini tiers), and local Ollama (`curl localhost:11434/api/tags` — models change). `documentation/model_allowlist.md` lists IDs last confirmed to exist as a starting reference, but re-check live availability rather than trusting it — it is not mechanically re-verified. To preserve Claude session limits, the default is to orchestrate from Claude Code but delegate implementation to a non-Claude model headlessly (`agy -p "<brief>" --model "<model>" --mode accept-edits --print-timeout 30m` from the repo root) for standard/deep-reasoning tiers, or to local Ollama for mechanical, well-bounded subtasks. Claude Code subagents bill the same Claude plan and do not save limits — never delegate to them for this purpose. Record the choice and one line of reasoning in the journal.
4. **Route the matching skills.** This repo has no local skills directory; skills come from the library at `../ai_knowledge_library/.agents/skills/<skill_name>/SKILL.md` (see `AGENTS.md`). Read the matching SKILL.md file(s) before planning — `devops_sre`, `cyber_security`, and `software_development` are common fits here given the Go/Playwright/SQLite stack.
5. **Re-verify the item against the current code**, not just the backlog row — this backlog was rebuilt from a stale, unscored list on 2026-07-19 and code had already drifted from it (see the Details section for examples). If it is stale, update the row, merge it, or close it with a dated note explaining why. A well-documented closure counts as completing this run.
6. **Read the detail section** for the item (linked from the table) before coding.
7. **Finish the loop:** every code change ships with relevant tests in the same task (`go test ./...` covering the new behavior's success and failure paths). Verify the change works end to end, run `go build ./...`, `go vet ./...`, and `go test ./...` before committing. If the change is user-visible (a fix, a feature, a behavior change — not an internal refactor, a backlog-only edit, or a `//go:build ignore` script), add a dated `CHANGELOG.md` entry in the same commit. **Check `docs/adrs/` too:** if the change touches a package or behavior an existing ADR describes (grep the touched file/package names against `docs/adrs/*.md`), correct the ADR in the same commit rather than leaving it to describe a decision, gap, or constraint that no longer matches the code — a stale ADR reads as "known issue, not yet fixed" long after it was fixed, and can send a future session chasing a defect that no longer exists (improvement #474). Commit with `<type>(<scope>): <description>`, set the item's Status to `Done (YYYY-MM-DD)` in the table with a Done note, delete the task journal in the final commit, and push. Committing and pushing verified work is the default and needs no per-task approval; only destructive or history-rewriting git operations require asking first.
8. **Keep the backlog itself small — this is a hard rule, not a suggestion.** These three files were restructured 2026-08-01 (a dedicated session, not a numbered item) after growing to 3411/1333/107 lines with 152/79/0 closed rows still carrying full inline fix narratives and ~40 stacked "Prior —"/"Groom pass" status paragraphs nobody ever pruned — see `documentation/backlog_history/` for the full record moved out that day. Do not let it regrow the same way:
   - **A closed row's `ROI rationale` cell gets one line**, of the shape `See `documentation/backlog_history/<file>_done_details.md` item #N for the full account.` — write the full fix narrative into that history file (append it), not into the table or a `### N.` Details subsection.
   - **A closed item's `### N.` Details section gets one line** the same way (`Closed — full account archived in ...`), not the full diagnostic story. Only a currently-**Pending** row keeps its full Details section inline — that is the one a future session actually needs to read before picking it up.
   - **Only one status paragraph lives in the Ranked Backlog prose zone at a time** (the Usability Gate zone in `bugs.md` works the same way). When a groom pass or a closed item produces a new one, move the previous paragraph to that file's `_groom_history.md` (append, newest additions at the top of the archive is fine, don't reorder what's already there) rather than leaving both in place. A checklist box's "Prior note:" chain follows the same rule.
   - When trimming a table row, if its `ROI rationale` cell contains a literal `|` (quoted code, a table-shaped example), do not naively split the line on `|` — count columns from the header and split with a bounded `maxsplit` so the embedded pipe stays inside the last cell instead of shifting every column after it (this actually happened during the 2026-08-01 restructure, to bug #465 and improvement #455's rows, and silently corrupts the row if not handled).

## Ranked Backlog (best ROI first)

Pending rows are ranked by a diminishing-returns score:

**Score = (Value × Decay) ÷ Effort**

- **Value (1–8):** pain or capability gained if the item ships.
- **Decay:** geometric halving per already-shipped item in the same theme (1.0 → 0.5 → 0.25 …). New-capability items that open a new curve keep Decay 1.0.
- **Effort (1–8):** roughly log-scale; 1 = minutes, 8 = weeks.
- **ROI floor = 0.5:** items scoring below the floor stay open but are flagged ⚠️ and must not be worked without explicit user confirmation. At selection time, skip past them to the highest-scoring above-floor item and ask the user to confirm, re-scope, or close.

**Tier and Effort are two different axes and are not meant to agree.** Effort measures *how much work* an item takes; Tier (improvement #456) measures *what kind of reasoning capability* it needs. A large, well-specified refactor can be Effort 6 and still `standard` tier — the spec removes the ambiguity, it just takes a while to execute. A one-line change can be Effort 1 and still `deep-reasoning` — #456 itself is Effort 2 and `deep-reasoning`, because the hard part was deciding the schema, not typing it. Do not derive one from the other or flag a mismatch as an error; only reconsider the tier if a groom pass finds the row's actual difficulty was misjudged.

Scores apply to Pending rows only; Done and Closed rows show `—`.

**2026-08-02 local-delegation run and groom.** #486 is Done: a framework-independent local-Ollama contract now produces strict, bounded investigation proposals and reviewed candidate patch artifacts without any apply or tool authority. The six remaining free Pending rows were re-verified against current code and retain their scores: #492, #487, #472, and #473 are 0.75; #485 is 0.67; and #497 is exactly 0.5. There are no Pending bugs. Paywalled #424 and #17 remain key-gated; #14 remains the only below-floor user decision. `go build ./...`, `go vet ./...`, `go test ./...`, and `gofmt -l ./cmd ./pkg ./internal` pass. Prior status paragraph archived in `documentation/backlog_history/improvements_groom_history.md`.

| # | Improvement | Status | Score (V×D÷E) | Tier | ROI rationale |
|---|---|---|---|---|---|
| 506 | [`/work_next_item`'s selection rule never returns to `bugs.md` once the gate is MET, starving Minor Pending bugs indefinitely](#506-work_next_items-selection-rule-never-returns-to-bugsmd-once-the-gate-is-met-starving-minor-pending-bugs-indefinitely) | Done (2026-08-01) | — | standard | See `documentation/backlog_history/improvements_done_details.md` item #506 for the full account. |
| 500 | [Add a missing index on `job_funnel(discovered_at)`](#500-add-a-missing-index-on-job_funneldiscovered_at) | Closed (2026-08-01) | — | mechanical | See `documentation/backlog_history/improvements_done_details.md` item #500 for the full account. |
| 505 | [`storedPromptInjectionThreats` and `toStoredThreats` are the same field-for-field conversion, written twice](#505-storedpromptinjectionthreats-and-tostoredthreats-are-the-same-field-for-field-conversion-written-twice) | Done (2026-08-01) | — | mechanical | See `documentation/backlog_history/improvements_done_details.md` item #505 for the full account. |
| 491 | [Define authoritative mission metrics and surface them on the dashboard](#491-define-authoritative-mission-metrics-and-surface-them-on-the-dashboard) | Done (2026-08-01) | — | standard | See `documentation/backlog_history/improvements_done_details.md` item #491 for the full account. |
| 499 | [Persist `discovery_source` at `AddToFunnel` time](#499-persist-discovery_source-at-addtofunnel-time) | Done (2026-08-01) | — | standard | See `documentation/backlog_history/improvements_done_details.md` item #499 for the full account. |
| 495 | [No-progress / dominant-failure-reason watchdog](#495-no-progress--dominant-failure-reason-watchdog) | Done (2026-08-01) | — | standard | See `documentation/backlog_history/improvements_done_details.md` item #495 for the full account. |
| 496 | [ATS capability and automation-success registry](#496-ats-capability-and-automation-success-registry) | Done (2026-08-01) | — | standard | See `documentation/backlog_history/improvements_done_details.md` item #496 for the full account. |
| 494 | [Append-only funnel/attempt stage ledger](#494-append-only-funnelattempt-stage-ledger) | Done (2026-08-01) | — | standard | See `documentation/backlog_history/improvements_done_details.md` item #494 for the full account. |
| 448 | [`npm run lint` lints the dashboard's own committed build output](#448-npm-run-lint-lints-the-dashboards-own-committed-build-output) | Done (2026-08-02) | — | standard | See `documentation/backlog_history/improvements_done_details.md` item #448 for the full account. |
| 442 | [Measure whether the NLP offload is worth keeping](#442-measure-whether-the-nlp-offload-is-worth-keeping) | Done (2026-08-02) | — | standard | See `documentation/backlog_history/improvements_done_details.md` item #442 for the full account. |
| 509 | [Make an empty application queue explainable before it silently stalls applications](#509-make-an-empty-application-queue-explainable-before-it-silently-stalls-applications) | Done (2026-08-02) | — | standard | See `documentation/backlog_history/improvements_done_details.md` item #509 for the full account. |
| 510 | [Record discovery-source request failures and circuit-open skips in refresh health](#510-record-discovery-source-request-failures-and-circuit-open-skips-in-refresh-health) | Done (2026-08-02) | — | standard | See `documentation/backlog_history/improvements_done_details.md` item #510 for the full account. |
| 511 | [Assisted Apply Queue with resumable human handoff and legacy-job backfill](#511-assisted-apply-queue-with-resumable-human-handoff-and-legacy-job-backfill) | Done (2026-08-02) | — | deep-reasoning | See `documentation/backlog_history/improvements_done_details.md` item #511 for the full account. |
| 479 | [A permanent DNS failure spends the full retry/backoff budget instead of failing fast to a terminal status](#479-a-permanent-dns-failure-spends-the-full-retrybackoff-budget-instead-of-failing-fast-to-a-terminal-status) | Done (2026-08-02) | — | standard | See `documentation/backlog_history/improvements_done_details.md` item #479 for the full account. |
| 486 | [Safe local-model delegation harness](#486-safe-local-model-delegation-harness) | Done (2026-08-02) | — | deep-reasoning | See `documentation/backlog_history/improvements_done_details.md` item #486 for the full account. |
| 492 | [Explicit first-attempt SLA and bounded fresh-queue admission](#492-explicit-first-attempt-sla-and-bounded-fresh-queue-admission) | Pending | 0.75 = 6×0.5÷4 | standard | #481/#482 already shipped; no admission cap or proactive first-attempt sweep exists. |
| 487 | [Lightweight 4B log triage and context compression](#487-lightweight-4b-log-triage-and-context-compression) | Pending | 0.75 = 3×1.0÷4 | standard | Deterministic classification still covers part of the proposed surface; no safe 4B evaluation has been run. |
| 472 | [Extend bug #467's target-closed browser recovery to the security-code resubmit click](#472-extend-bug-467s-target-closed-browser-recovery-to-the-security-code-resubmit-click) | Pending | 0.75 = 3×0.5÷2 | standard | The one-time-code resubmit path still bypasses target-closed recovery. |
| 473 | [Extend bug #467's target-closed browser recovery to the Vision submission paths](#473-extend-bug-467s-target-closed-browser-recovery-to-the-vision-submission-paths) | Pending | 0.75 = 3×0.5÷2 | standard | Vision fallback still has no target-closed recovery of its own. |
| 485 | [Resource-aware local inference admission control](#485-resource-aware-local-inference-admission-control) | Pending | 0.67 = 4×1.0÷6 | deep-reasoning | Existing one-model limit prevents OOM; contention remains an unobserved risk. |
| 497 | [User-approved application-answer memory](#497-user-approved-application-answer-memory) | Pending | 0.5 = 3×1.0÷6 | deep-reasoning | No data-driven approved-answer store exists; historic observed volume remains low. |
| 493 | [Rank by expected confirmed-application yield](#493-rank-by-expected-confirmed-application-yield) | Pending ⚠️ below floor — insufficient outcome data | 0.33 = 2×1.0÷6 | deep-reasoning | The live metrics endpoint is readable, but only 2 applied rows and zero outcomes exist; that cannot validate a yield-ranking change. |
| 488 | [OpenClaw read-only sidecar evaluation](#488-openclaw-read-only-sidecar-evaluation) | Pending ⚠️ below floor — user decision | 0.4 = 2×1.0÷5 | deep-reasoning | Optional sidecar remains below the floor and requires explicit user confirmation. |
| 498 | [Company and role-family duplicate/cooldown protection](#498-company-and-role-family-duplicatecooldown-protection) | Done (2026-08-01) | — | standard | See `documentation/backlog_history/improvements_done_details.md` item #498 for the full account. |
| 484 | [Local-model benchmark and routing-evidence harness](#484-local-model-benchmark-and-routing-evidence-harness) | Done (2026-08-01) | 2.33 = 7×1.0÷3 | standard | See `documentation/backlog_history/improvements_done_details.md` item #484 for the full account. |
| 477 | [Yahoo fallback requests carry no realistic browser headers or cookie jar](#477-yahoo-fallback-requests-carry-no-realistic-browser-headers-or-cookie-jar) | Done (2026-08-01) | — | standard | See `documentation/backlog_history/improvements_done_details.md` item #477 for the full account. |
| 483 | [Confirm whether zombie career_agent_bin processes need reaping, or just documenting as harmless](#483-confirm-whether-zombie-career_agent_bin-processes-need-reaping-or-just-documenting-as-harmless) | Closed (2026-08-01) | — | mechanical | See `documentation/backlog_history/improvements_done_details.md` item #483 for the full account. |
| 474 | [ADRs have no process ensuring they get updated when the decision they describe changes](#474-adrs-have-no-process-ensuring-they-get-updated-when-the-decision-they-describe-changes) | Done (2026-08-01) | — | standard | See `documentation/backlog_history/improvements_done_details.md` item #474 for the full account. |
| 470 | [`cmd/requeue`'s `countForStatus` is fully written and tested but never called](#470-cmdrequeues-countforstatus-is-fully-written-and-tested-but-never-called) | Done (2026-07-31) | — | mechanical | See `documentation/backlog_history/improvements_done_details.md` item #470 for the full account. |
| 469 | [Add per-domain circuit breakers for repeated fetch and pre-flight timeouts](#469-add-per-domain-circuit-breakers-for-repeated-fetch-and-pre-flight-timeouts) | Done (2026-07-31) | — | standard | See `documentation/backlog_history/improvements_done_details.md` item #469 for the full account. |
| 468 | [Filter weak discovery URLs and expose deferred queue state in the dashboard](#468-filter-weak-discovery-urls-and-expose-deferred-queue-state-in-the-dashboard) | Done (2026-07-31) | — | standard | See `documentation/backlog_history/improvements_done_details.md` item #468 for the full account. |
| 459 | [`serveMetrics`'s by-source/by-variant breakdowns never check `rows.Err()` after their scan loops](#459-servemetricss-by-sourceby-variant-breakdowns-never-check-rowserr-after-their-scan-loops) | Done (2026-07-30) | — | standard | See `documentation/backlog_history/improvements_done_details.md` item #459 for the full account. |
| 458 | [The Gemini and OpenAI model columns have never been checked against a vendor catalogue](#458-the-gemini-and-openai-model-columns-have-never-been-checked-against-a-vendor-catalogue) | Done (2026-07-30) | — | standard | See `documentation/backlog_history/improvements_done_details.md` item #458 for the full account. |
| 457 | [Enforce the model columns with a test instead of an instruction](#457-enforce-the-model-columns-with-a-test-instead-of-an-instruction) | Done (2026-07-30) | — | deep-reasoning | See `documentation/backlog_history/improvements_done_details.md` item #457 for the full account. |
| 456 | [Replace the concrete model columns with capability tiers that cannot expire](#456-replace-the-concrete-model-columns-with-capability-tiers-that-cannot-expire) | Done (2026-07-31) | — | deep-reasoning | See `documentation/backlog_history/improvements_done_details.md` item #456 for the full account. |
| 455 | [The Claude model column named one ID that does not exist and one that is a generation behind](#455-the-claude-model-column-named-one-id-that-does-not-exist-and-one-that-is-a-generation-behind) | Done (2026-07-30) | — | deep-reasoning | See `documentation/backlog_history/improvements_done_details.md` item #455 for the full account. |
| 454 | [Nothing in the Working Protocol updates `CHANGELOG.md`, and it drifted a full day](#454-nothing-in-the-working-protocol-updates-changelogmd-and-it-drifted-a-full-day) | Done (2026-07-30) | — | standard | See `documentation/backlog_history/improvements_done_details.md` item #454 for the full account. |
| 450 | [The shared DSN's `journal_mode(WAL)` can fail a connection outright, and `busy_timeout` does not cover it](#450-the-shared-dsns-journal_modewal-can-fail-a-connection-outright-and-busy_timeout-does-not-cover-it) | Done (2026-07-31) | — | standard | See `documentation/backlog_history/improvements_done_details.md` item #450 for the full account. |
| 443 | [Eight Go files are not `gofmt`-clean, and nothing in the verification loop notices](#443-eight-go-files-are-not-gofmt-clean-and-nothing-in-the-verification-loop-notices) | Done (2026-07-30) | 2.0 = 2×1.0÷1 | standard | See `documentation/backlog_history/improvements_done_details.md` item #443 for the full account. |
| 460 | [The dashboard UI shows no visible sign that a metrics poll failed](#460-the-dashboard-ui-shows-no-visible-sign-that-a-metrics-poll-failed) | Done (2026-07-30) | — | standard | See `documentation/backlog_history/improvements_done_details.md` item #460 for the full account. |
| 428 | [Expand usage of Zero transpiler for analytics and tooling](#428-expand-usage-of-zero-transpiler-for-analytics-and-tooling) | Done (2026-07-29) | — | standard | See `documentation/backlog_history/improvements_done_details.md` item #428 for the full account. |
| 429 | [Rewrite data ingestion CLI tools in Zero](#429-rewrite-data-ingestion-cli-tools-in-zero) | Done (2026-07-29) | — | standard | See `documentation/backlog_history/improvements_done_details.md` item #429 for the full account. |
| 425 | [Memory Profiling & sync.Pool Implementation](#425-memory-profiling--syncpool-implementation) | Done (2026-07-29) | — | standard | See `documentation/backlog_history/improvements_done_details.md` item #425 for the full account. |
| 426 | [TypeScript React/Vue Dashboard Rewrite](#426-typescript-reactvue-dashboard-rewrite) | Done (2026-07-29) — **all three regressions now repaired (bugs #436/#437/#438), 2026-07-30** | — | standard | See `documentation/backlog_history/improvements_done_details.md` item #426 for the full account. |
| 463 | [`cmd/dashboard/ui` has no test framework, so frontend logic bugs are only ever caught by manual live verification](#463-cmddashboardui-has-no-test-framework-so-frontend-logic-bugs-are-only-ever-caught-by-manual-live-verification) | Done (2026-07-30) | 1.5 = 3×1.0÷2 | standard | See `documentation/backlog_history/improvements_done_details.md` item #463 for the full account. |
| 464 | [`scripts/server.go`'s transpiled body is not `gofmt`-clean, and the documented `gofmt -l` loop can't catch it](#464-scriptsservergos-transpiled-body-is-not-gofmt-clean-and-the-documented-gofmt--l-loop-cant-catch-it) | Done (2026-08-01) | — | standard | See `documentation/backlog_history/improvements_done_details.md` item #464 for the full account. |
| 462 | [The RAG embedding retry loop duplicates `classifyGenerationError`'s logic instead of reusing it](#462-the-rag-embedding-retry-loop-duplicates-classifygenerationerrors-logic-instead-of-reusing-it) | Done (2026-07-31) | — | standard | See `documentation/backlog_history/improvements_done_details.md` item #462 for the full account. |
| 427 | [Python NLP Microservice for Resume Tailoring](#427-python-nlp-microservice-for-resume-tailoring) | Done (2026-07-29) — **⚠️ shipped as a regression, see bug #439** | — | standard | See `documentation/backlog_history/improvements_done_details.md` item #427 for the full account. |
| 420 | [Pre-Mapped ATS Selectors (Accuracy & Speed)](#420-pre-mapped-ats-selectors) | Done (2026-07-29) | — | standard | See `documentation/backlog_history/improvements_done_details.md` item #420 for the full account. |
| 421 | [Pre-Submission Keyword Gap Analysis](#421-pre-submission-keyword-gap-analysis) | Done (2026-07-29) | — | standard | See `documentation/backlog_history/improvements_done_details.md` item #421 for the full account. |
| 422 | [Vector-Based Job Matchmaking](#422-vector-based-job-matchmaking) | Done (2026-07-29) | — | standard | See `documentation/backlog_history/improvements_done_details.md` item #422 for the full account. |
| 423 | [Human-in-the-Loop "Copilot" Mode](#423-human-in-the-loop-copilot-mode) | Done (2026-07-29) | — | standard | See `documentation/backlog_history/improvements_done_details.md` item #423 for the full account. |
| 424 | Cloud-Offloading for DOM Parsing | Moved to `improvements_paywall.md` (2026-07-29) | — | — | Requires paid cloud API access (the row's own premise names Gemini Flash / Claude Haiku), so it does not belong in the free backlog. Moved unchanged at its existing 2.0 score |
| 418 | [Parallelize DiscoverJobs queries in funnel.go](#418-parallelize-discoverjobs-queries-in-funnelgo) | Done (2026-07-29) | — | standard | See `documentation/backlog_history/improvements_done_details.md` item #418 for the full account. |
| 419 | [Use defer for resp.Body.Close() in scraper.go](#419-use-defer-for-respbodyclose-in-scrapergo) | Done (2026-07-29) | — | standard | See `documentation/backlog_history/improvements_done_details.md` item #419 for the full account. |
| 415 | [UI Overhaul & Agent Start/Stop Controls](#415-ui-overhaul--agent-startstop-controls) | Done (2026-07-29) | — | standard | See `documentation/backlog_history/improvements_done_details.md` item #415 for the full account. |
| 412 | [Implement Exponential Backoff with Jitter for LLM API Retries](#412-implement-exponential-backoff-with-jitter-for-llm-api-retries) | Done (2026-07-29) | — | standard | See `documentation/backlog_history/improvements_done_details.md` item #412 for the full account. |
| 411 | [Implement Stateful Graph-Based Pipeline Architecture](#411-implement-stateful-graph-based-pipeline-architecture) | Done (2026-07-28) | — | standard | See `documentation/backlog_history/improvements_done_details.md` item #411 for the full account. |
| 410 | [AI Processing Optimizations (Concurrency & Context Limits)](#410-ai-processing-optimizations-concurrency--context-limits) | Done (2026-07-28) | — | standard | See `documentation/backlog_history/improvements_done_details.md` item #410 for the full account. |
| 409 | [Frontend Rendering Speed Optimizations](#409-frontend-rendering-speed-optimizations) | Done (2026-07-28) | — | standard | See `documentation/backlog_history/improvements_done_details.md` item #409 for the full account. |
| 402 | [Migrate from go-sqlite3 CGO driver to pure Go modernc.org/sqlite](#402-migrate-from-go-sqlite3-cgo-driver-to-pure-go-moderncorgsqlite) | Done (2026-07-28) | — | standard | See `documentation/backlog_history/improvements_done_details.md` item #402 for the full account. |
| 403 | [Evaluate and prototype go-rod as a lightweight replacement for playwright-go](#403-evaluate-and-prototype-go-rod-as-a-lightweight-replacement-for-playwright-go) | Done (2026-07-28) | — | standard | See `documentation/backlog_history/improvements_done_details.md` item #403 for the full account. |
| 404 | [Batch SQLite writes into explicit transactions for performance](#404-batch-sqlite-writes-into-explicit-transactions-for-performance) | Done (2026-07-28) | — | standard | See `documentation/backlog_history/improvements_done_details.md` item #404 for the full account. |
| 405 | [Refactor DOM parsing pipeline to use single AST pass and IO streams](#405-refactor-dom-parsing-pipeline-to-use-single-ast-pass-and-io-streams) | Done (2026-07-28) | — | standard | See `documentation/backlog_history/improvements_done_details.md` item #405 for the full account. |
| 406 | [Implement concurrent HTTP execution for ATS and job-board discovery scrapers](#406-implement-concurrent-http-execution-for-ats-and-job-board-discovery-scrapers) | Done (2026-07-29) | 2.6 = 4×1.0÷1.5 | standard | See `documentation/backlog_history/improvements_done_details.md` item #406 for the full account. |
| 407 | [Utilize goroutines and bounded worker pools for dashboard queries and embedding inference](#407-utilize-goroutines-and-bounded-worker-pools-for-dashboard-queries-and-embedding-inference) | Done (2026-07-28) | — | standard | See `documentation/backlog_history/improvements_done_details.md` item #407 for the full account. |
| 397 | [Add SkipScoring configuration to bypass local LLM fit evaluation bottlenecks](#397-add-skipscoring-configuration-to-bypass-local-llm-fit-evaluation-bottlenecks) | Done (2026-07-28) | — | standard | See `documentation/backlog_history/improvements_done_details.md` item #397 for the full account. |
| 398 | [Enhance Playwright stealth capabilities to bypass basic ATS bot detection](#398-enhance-playwright-stealth-capabilities-to-bypass-basic-ats-bot-detection) | Done (2026-07-28) | — | standard | See `documentation/backlog_history/improvements_done_details.md` item #398 for the full account. |
| 39 | [Track recent source health and application-attempt cost](#39-track-recent-source-health-and-application-attempt-cost) | Done (2026-07-28) | — | standard | See `documentation/backlog_history/improvements_done_details.md` item #39 for the full account. |
| 96 | [Filter out dead or expired job postings early](#96-filter-out-dead-or-expired-job-postings-early) | Done (2026-07-28) | 2.5 = 5×1.0÷2 | standard | See `documentation/backlog_history/improvements_done_details.md` item #96 for the full account. |
| 36 | [Reconcile setup and feature documentation with executable behavior](#36-reconcile-setup-and-feature-documentation-with-executable-behavior) | Done (2026-07-27) | — | standard | See `documentation/backlog_history/improvements_done_details.md` item #36 for the full account. |
| 38 | [Produce a dry-run queue plan before requeueing or reprioritizing jobs](#38-produce-a-dry-run-queue-plan-before-requeueing-or-reprioritizing-jobs) | Done (2026-07-28) | — | standard | See `documentation/backlog_history/improvements_done_details.md` item #38 for the full account. |
| 37 | [Revalidate posting freshness before expensive document generation](#37-revalidate-posting-freshness-before-expensive-document-generation) | Done (2026-07-28) | — | standard | See `documentation/backlog_history/improvements_done_details.md` item #37 for the full account. |
| 34 | [Make the local dashboard accessible and self-contained](#34-make-the-local-dashboard-accessible-and-self-contained) | Done (2026-07-28) — **partly regressed by #426, tracked as bug #437** | — | standard | See `documentation/backlog_history/improvements_done_details.md` item #34 for the full account. |
| 35 | [Rank the queue from observed outcomes while preserving exploration](#35-rank-the-queue-from-observed-outcomes-while-preserving-exploration) | Done (2026-07-28) | — | standard | See `documentation/backlog_history/improvements_done_details.md` item #35 for the full account. |
| 27 | [Local MCP server for career context](#27-local-mcp-server-for-career-context) | Closed (2026-07-28) | — | standard | See `documentation/backlog_history/improvements_done_details.md` item #27 for the full account. |
| 30 | [Detect unanswerable attestations before fit-scoring, not after](#30-detect-unanswerable-attestations-before-fit-scoring-not-after) | Closed (2026-07-28) | — | deep-reasoning | See `documentation/backlog_history/improvements_done_details.md` item #30 for the full account. |
| 1 | V2 Architecture Blueprint | Done | — | — | See `documentation/backlog_history/improvements_done_details.md` item #1 for the full account. |
| 2 | Dynamic Source Discovery (FunnelEngine) | Done | — | — | See `documentation/backlog_history/improvements_done_details.md` item #2 for the full account. |
| 3 | Playwright Dynamic Generation (self-learning DOM mapper) | Done | — | — | See `documentation/backlog_history/improvements_done_details.md` item #3 for the full account. |
| 4 | DOM Pruning/Minification | Done | — | — | See `documentation/backlog_history/improvements_done_details.md` item #4 for the full account. |
| 5 | Graceful DB Degradation (self-healing cache) | Done | — | — | See `documentation/backlog_history/improvements_done_details.md` item #5 for the full account. |
| 6 | Stealth & Proxies (webdriver overrides) | Done | — | — | See `documentation/backlog_history/improvements_done_details.md` item #6 for the full account. |
| 7 | Playwright Scraper Fallback (DuckDuckGo) | Done | — | — | See `documentation/backlog_history/improvements_done_details.md` item #7 for the full account. |
| 8 | Visual Reasoning (VLM) for Form Submissions | Done | — | — | See `documentation/backlog_history/improvements_done_details.md` item #8 for the full account. |
| 9 | Metrics Dashboard (Web) | Done | — | — | See `documentation/backlog_history/improvements_done_details.md` item #9 for the full account. |
| 10 | Cron-Driven Drip Campaigns | Closed as bug #120 (2026-07-26 audit) | — | — | See `documentation/backlog_history/improvements_done_details.md` item #10 for the full account. |
| 20 | [Dashboard tile for the MANUAL_REQUIRED queue](#20-dashboard-tile-for-the-manual_required-queue) | Done (2026-07-22) | — | mechanical | See `documentation/backlog_history/improvements_done_details.md` item #20 for the full account. |
| 21 | [Separate the actionable manual-apply queue from historical failure noise](#21-separate-the-actionable-manual-apply-queue-from-historical-failure-noise) | Done (2026-07-22) | — | mechanical | See `documentation/backlog_history/improvements_done_details.md` item #21 for the full account. |
| 15 | [Email/portal conversion-rate analytics](#15-emailportal-conversion-rate-analytics) | Done (2026-07-24) — **⚠️ UI surface deleted by #426, tracked as bug #437** | — | standard | See `documentation/backlog_history/improvements_done_details.md` item #15 for the full account. |
| 16 | [Automated assessment/screening solver](#16-automated-assessmentscreening-solver) | Done (2026-07-24, partial — see note) | — | standard | See `documentation/backlog_history/improvements_done_details.md` item #16 for the full account. |
| 18 | [Configurable worker concurrency](#18-configurable-worker-concurrency) | Done | — | — | See `documentation/backlog_history/improvements_done_details.md` item #18 for the full account. |
| 11 | [Multi-step form logic (Workday-style)](#11-multi-step-form-logic-workday-style) | Closed (2026-07-24, user-confirmed) | — | — | See `documentation/backlog_history/improvements_done_details.md` item #11 for the full account. |
| 12 | [Niche data source scrapers](#12-niche-data-source-scrapers) | Done (2026-07-24, partial — see note) | — | mechanical | See `documentation/backlog_history/improvements_done_details.md` item #12 for the full account. |
| 13 | [Adaptive resume A/B testing](#13-adaptive-resume-ab-testing) | Done (2026-07-24, partial — see note) | — | standard | See `documentation/backlog_history/improvements_done_details.md` item #13 for the full account. |
| 26 | [Discover via Greenhouse/Lever board feeds instead of search dorking](#26-discover-via-greenhouselever-board-feeds-instead-of-search-dorking) | Done (2026-07-25) | — | deep-reasoning | See `documentation/backlog_history/improvements_done_details.md` item #26 for the full account. |
| 25 | [Trim over-long job descriptions before scoring](#25-trim-over-long-job-descriptions-before-scoring) | Done (2026-07-25) | — | deep-reasoning | See `documentation/backlog_history/improvements_done_details.md` item #25 for the full account. |
| 24 | [Per-call-type model selection — run ScoreJob on a smaller, faster model](#24-per-call-type-model-selection--run-scorejob-on-a-smaller-faster-model) | Done (2026-07-25 — routing shipped, model swap **rejected on evidence**) | — | deep-reasoning | See `documentation/backlog_history/improvements_done_details.md` item #24 for the full account. |
| 29 | [Hard-code every repeatable application fact so the model stops guessing](#29-hard-code-every-repeatable-application-fact-so-the-model-stops-guessing) | Done (2026-07-25) | — | deep-reasoning | See `documentation/backlog_history/improvements_done_details.md` item #29 for the full account. |
| 28 | [Fill Greenhouse's required Location/Country comboboxes on the first pass](#28-fill-greenhouses-required-locationcountry-comboboxes-on-the-first-pass) | Done (2026-07-25) | — | deep-reasoning | See `documentation/backlog_history/improvements_done_details.md` item #28 for the full account. |
| 31 | [Fill Lever's required location on the initial pass, and generalise the combobox helper](#31-fill-levers-required-location-on-the-initial-pass-and-generalise-the-combobox-helper) | Done (2026-07-25) | — | deep-reasoning | See `documentation/backlog_history/improvements_done_details.md` item #31 for the full account. |
| 33 | [Make the configured location resolvable on every geocoder, and compute the start date](#33-make-the-configured-location-resolvable-on-every-geocoder-and-compute-the-start-date) | Done (2026-07-25) | — | deep-reasoning | See `documentation/backlog_history/improvements_done_details.md` item #33 for the full account. |
| 32 | [Retrieve emailed one-time codes so verification gates can be completed](#32-retrieve-emailed-one-time-codes-so-verification-gates-can-be-completed) | Done (2026-07-25, user approved) | — | deep-reasoning | See `documentation/backlog_history/improvements_done_details.md` item #32 for the full account. |
| 19 | [Prompt-injection/hidden-content detection CSV log](#19-prompt-injection-hidden-content-detection-csv-log) | Done | — | — | See `documentation/backlog_history/improvements_done_details.md` item #19 for the full account. |
| 23 | [Static master cover letter, reused across every application](#23-static-master-cover-letter-reused-across-every-application) | Done (2026-07-24) | — | deep-reasoning | See `documentation/backlog_history/improvements_done_details.md` item #23 for the full account. |

## Details

### 510. Record discovery-source request failures and circuit-open skips in refresh health

Done — full account archived in `documentation/backlog_history/improvements_done_details.md` item #510.

### 509. Make an empty application queue explainable before it silently stalls applications

Done — full account archived in `documentation/backlog_history/improvements_done_details.md` item #509.

**Found 2026-08-02** during a safe live mission-status check. The running dashboard's read-only `/api/metrics` reported `eligible_queue: 0`, while `/api/agent/status` reported `running: false`. That proves no application can currently progress, but not whether the agent was intentionally stopped, discovery failed, all results were duplicates or excluded, or filters yielded no eligible jobs. The dashboard's “last skipped” record is historical and cannot answer that question. #495 deliberately suppresses its no-progress alert when the eligible queue is empty, so it cannot provide this diagnosis either.

**Proposed direction:** persist a privacy-safe aggregate result for each discovery refresh (started/finished time; source-level attempted/new/duplicate/excluded/error counts; sanitized error class), expose the latest result in `/api/metrics`, and show an actionable dashboard state when the agent is stopped or the most recent refresh produced no eligible jobs. Do not retain job descriptions, URLs, resumes, application answers, raw errors, or credentials; do not start the agent, relax filters, requeue jobs, or submit applications automatically.

**Acceptance criteria:** a deterministic discovery fixture can produce new eligible jobs, zero results, all-filtered or duplicate results, and a source error; the API/dashboard state distinguishes each case without leaking job or personal content. An intentionally stopped agent is visibly distinct from a running agent awaiting its next refresh.

**Automated tests:** table-driven storage/API tests for the aggregate outcomes and a dashboard-handler test for the stopped/empty state.

**Safe live verification:** query only the dashboard's aggregate endpoint after one controlled discovery refresh; confirm the displayed explanation agrees with the persisted aggregate without inspecting raw job data.

**Boundaries:** this is observability and diagnosis, not a discovery-source rewrite (#508), queue-admission change (#492), or autonomous application-start authority. Its theme has one shipped precursor (#495), hence Decay 0.5; Value 6 and Effort 3 yield `1.0`.

### 506. `/work_next_item`'s selection rule never returns to `bugs.md` once the gate is MET, starving Minor Pending bugs indefinitely

Done — full account archived in `documentation/backlog_history/improvements_done_details.md` item #506.

### 500. Add a missing index on `job_funnel(discovered_at)`

Closed — full account archived in `documentation/backlog_history/improvements_done_details.md`.

### 505. `storedPromptInjectionThreats` and `toStoredThreats` are the same field-for-field conversion, written twice

Done — full account archived in `documentation/backlog_history/improvements_done_details.md`.

**Boundaries:** pure refactor — no behavior change, no new fields.

### 491. Define authoritative mission metrics and surface them on the dashboard

Done — full account archived in `documentation/backlog_history/improvements_done_details.md` item #491.

### 499. Persist `discovery_source` at `AddToFunnel` time

Done — full account archived in `documentation/backlog_history/improvements_done_details.md` item #499.

### 495. No-progress / dominant-failure-reason watchdog

**Filed 2026-08-01**, mission-alignment audit (seeded candidate E).

Confirmed no mechanism detects "the daemon is alive and processing but producing no confirmed applications" or "one failure reason has dominated several cycles." `runAgentSchedule` (`cmd/agent/main.go:518-593`) only distinguishes "cycle had work" from "no eligible jobs" for scheduling purposes; `runDaemonDiscoveryLoop` (`:598-631`) just logs per-refresh errors. The existing poll-failure banner (#447/#460) and per-domain circuit breakers (#469/#475) are both narrower, failure-triggered mechanisms — neither tracks time-since-last-confirmed-application or a dominant status/reason across cycles.

**This audit is itself the evidence for this item's value:** bugs.md #489 (51% of the entire funnel quarantined, mission-critical) was only found because a human ran a manual multi-hour database audit — exactly the shape of condition ("a high percentage of jobs terminate at one stage," per the seeded brief) a watchdog should have surfaced automatically, and much sooner than a week after #394's incomplete fix.

**Proposed direction, per the brief:** track eligible-fresh-queue-nonempty + cycles-continuing + no-confirmed-application-for-N-hours; a dominant terminal status or (post-bugs.md #480 broadening / #494) dominant `status_reason` across recent cycles. Emit one deduplicated actionable alert (log line + dashboard status, no email/SMS infrastructure exists to page through); create a sanitized diagnostic snapshot; never auto-relax user constraints; never auto-requeue at volume. A coarser first version can ship using only existing status counts (no dependency on #480/#494), with reason-level detail added once those land.

**Acceptance criteria:** a seeded test fixture with N cycles of a dominant failure status triggers exactly one alert, not one per cycle; a fixture with healthy variety triggers none; a fixture with an empty eligible queue (nothing to attempt) triggers none.

**Automated tests:** table-driven tests over the trigger conditions above.

**Safe live verification:** run the watchdog against a read-only copy of the real `applications.db`'s history and confirm it would have flagged the #489 condition (QUARANTINED_PROMPT_INJECTION dominant on 2026-08-01) had it existed.

**Boundaries:** detection and alerting only — no automatic recovery action (source suppression, requeue, constraint relaxation) is in this item's scope; evaluate those separately per the brief's own caution against unlimited watchdog authority.

### 496. ATS capability and automation-success registry

Done — full account archived in `documentation/backlog_history/improvements_done_details.md` item #496.

### 494. Append-only funnel/attempt stage ledger

Done — full account archived in `documentation/backlog_history/improvements_done_details.md` item #494.

### 493. Rank by expected confirmed-application yield

**Filed 2026-08-01**, mission-alignment audit (seeded candidate C).

`RankJobs` (`pkg/storage/ranking.go:110-178`) already combines Bayesian-smoothed per-hostname source health (`ComputeSourceScores`), a bad-outcome `PenaltyFactor`, `fit_similarity`, the confirmed `exp(-0.02×ageDays)` freshness multiplier, bugs.md #481's 10-day urgency override, and a 20% exploration floor with sparse-source reserved slots — a real, working approximation of the brief's conceptual objective already. **Re-verified 2026-08-01:** #499 persists `discovery_source`, #496 records outcome-derived ATS capability evidence, and #494 now records future attempt-stage transitions. The ledger begins empty by design and no sufficiently large confirmation/outcome sample exists, so changing rank weights now would still be speculation. Interview-outcome data correctly is **not** used anywhere in ranking today — with only 2 `APPLIED` rows and 0 observed interview outcomes, that's trivially satisfying the brief's own "no interview outcomes until sample size is sufficient" constraint, not a gap.

**Proposed direction:** use #499's `discovery_source` and #496's capability evidence in `RankJobs` to weight estimated form reach and confirmation probability, alongside the existing fit/freshness/source-health signals, while preserving Bayesian smoothing, the exploration floor, and deterministic tie-breaking. **Explicitly sequenced after #494**: the newly available source and site evidence is still incomplete until attempt stages are reconstructable.

**Acceptance criteria:** a new ranking formula preserves strict user constraints (salary/remote/etc.) and the existing exploration floor; a regression test corpus of synthetic jobs across discovery sources/ATS providers/outcomes shows the new ordering favoring higher-yield combinations without starving any single source below the floor.

**Automated tests:** extend the existing `RankJobs`/`ranking.go` test suite with cases for the new signals; a starvation-prevention test mirroring whatever already guards the exploration floor.

**Safe live verification:** compare the top-N of a live `GetQueuePlan` dry run before/after against the current baseline, checking no eligible source drops to zero share.

**Boundaries:** does not touch strict eligibility filtering (salary/remote/blocklist), which stays exactly as it is today.

**Groomed 2026-08-01:** the current checkout has no readable live outcome database, so no evidence can justify changing rank weights or validate a new formula. Value is reduced to 1 while that dependency is absent; the item is below the ROI floor until fresh outcome evidence becomes available.

**Groomed 2026-08-02:** the dashboard's live `/api/metrics` endpoint is readable again and reports 2 applied rows, 0 interviews, and 0 rejections. That resolves the prior checkout-access limitation, but the sample remains far too small to support a causal yield estimate or validate a ranking rewrite. Value rises conservatively to 2; `2×1.0÷6 = 0.33` remains below the ROI floor. Keep the item deferred until outcome evidence exists.

### 498. Company and role-family duplicate/cooldown protection

Done — full account archived in `documentation/backlog_history/improvements_done_details.md` item #498.

### 497. User-approved application-answer memory

**Filed 2026-08-01**, mission-alignment audit (seeded candidate G).

Confirmed no unanswered-question logging, answer map, or per-question memory exists anywhere in the codebase. Improvement #29 ("Hard-code every repeatable application fact," Done) implemented a fixed Go struct (`config.PII`, `pkg/config/pii.go`) — adding a new fact type requires a code change, not a data-driven registration; there is no mechanism today that logs a question the model couldn't answer for later review.

**Value held to 3, not the brief's implied higher priority**, on real observed volume: only 26 `MANUAL_REQUIRED` rows exist in the live database today (0.2% of the funnel) — dwarfed by bugs.md #489's 5,983 quarantined rows and this file's own #492's multi-day first-attempt latency. The underlying capability gap is real, but the current bottleneck on confirmed applications is overwhelmingly upstream of where this item would help.

**Proposed direction, per the brief:** a local, user-approved answer store — normalized question identity, original example wording, explicit user-approved answer, answer type, scope (global/ATS-specific/company-specific/role-specific), sensitivity classification, timestamps, revocation, source of approval, deterministic reuse before any LLM call. **Never** auto-infer or auto-approve legal attestations, protected-class questions, salary expectations, relocation commitments, or anything absent from user-controlled data — stop and request approval for anything sensitive or ambiguous.

**Acceptance criteria:** a previously-approved question is answered deterministically from the store without an LLM call on a repeat encounter; a novel or sensitive question always stops for explicit approval, never auto-answered.

**Automated tests:** tests covering exact-match reuse, scope resolution (global vs. company-specific), revocation, and the sensitive-question refusal path (mirroring the existing legal-attestation refusal behavior from improvement #30, Done).

**Safe live verification:** confirm no sensitive-category answer is ever auto-approved by feeding the store's own protected-question list through the refusal path.

**Boundaries:** must never fabricate an answer to a legal attestation or protected-class question; must never bypass the existing refusal behavior improvement #30 already established.

### 492. Explicit first-attempt SLA and bounded fresh-queue admission

**Filed 2026-08-01**, mission-alignment audit (seeded candidate B).

Bug #481 (Done) stops an aged `DISCOVERED` row from being outscored forever by forcing it to the front past a 10-day urgency threshold — but sets no proactive first-attempt deadline, and confirmed no periodic sweep of stale rows or per-source/per-provider admission cap exists anywhere in the codebase (`AddToFunnel` inserts unconditionally with `discovered_at = CURRENT_TIMESTAMP`; ranking/expiry logic only ever runs synchronously when the queue is read, never on a schedule).

**Live-measured evidence this is a real, separate gap from #481:** a `job_funnel`↔`application_attempts` join (538 rows) found discovery-to-first-attempt timing of min = 8.1h, p50 = 4.8 days, p90 = 11.7 days, max = 18.4 days — **nothing under 6 hours**. #481 guarantees a stale row eventually wins a ranking comparison; it does not guarantee a *fast* first attempt.

**Proposed direction, per the brief:** an explicit first-attempt deadline (e.g. a configurable SLA measured against `discovered_at`); a periodic sweep of stale `DISCOVERED` rows rather than only on-read ranking (would also give bugs.md #482's breezy.hr rows, and any future excluded-source rows, a real terminal path instead of accumulating forever); a bounded number of pending jobs per source once #499 exists to identify one. Test with deterministic job ages at 0/1/7/14/30 days, and both a small queue and one exceeding the per-cycle processing cap, per the brief's own acceptance-criteria list.

**Acceptance criteria:** a synthetic queue with jobs at each of the five ages above demonstrates the SLA/sweep correctly prioritizing or expiring rows at the intended boundaries; a queue larger than the per-cycle cap does not let admission outpace processing.

**Automated tests:** deterministic age-bucket tests as above; a test proving the periodic sweep terminates already-excluded-source rows (e.g. breezy.hr) instead of leaving them in `DISCOVERED` forever.

**Safe live verification:** after shipping, re-run this row's own discovery-to-first-attempt query against a read-only copy of the real database and confirm the p50/p90 figures have materially improved from this audit's baseline (4.8/11.7 days).

**Boundaries:** does not change the ranking formula itself (that's #493); scoped to admission/sweep/deadline mechanics on top of the existing ranker.

### 484. Local-model benchmark and routing-evidence harness

Closed — full account archived in `documentation/backlog_history/improvements_done_details.md`.

### 486. Safe local-model delegation harness

Closed — full account archived in `documentation/backlog_history/improvements_done_details.md`.

### 487. Lightweight 4B log triage and context compression

**Filed 2026-08-01**, from the same session as #484.

Evaluate using `qwen3:4b-instruct` — the smallest installed capable text model — as a frequent, low-cost worker for: redacting and classifying sanitized log events, normalizing error signatures, grouping repeated failures, detecting novel error classes, summarizing daemon cycles, and producing compact context packets for Claude. Required safeguards if built: deterministic redaction before any inference call, no raw PII ever reaching the model, bounded input/output, schema validation with a deterministic fallback on invalid output, a confidence field, deduplicated alerts, and explicitly no backlog edits, no Git writes, no email access, no submission authority — a read/summarize-only worker that yields to application-critical work.

**Value 3, Effort 4, Decay 1.0, score 0.75.** Value is moderate rather than high because — per the brief's own instruction to check this first — **several existing mechanisms in this codebase already do deterministic classification of exactly this shape without an LLM call**: `classifyGenerationError` (`cmd/agent/pipeline.go`, bug #444) classifies generation errors by substring match; the domain/source circuit breakers (bugs.md #469/#475) already track and summarize repeated-failure streaks without a model in the loop. Before this item is built, **the actual next step is confirming how much log-triage surface deterministic parsing does *not* already cover** — the brief's own instruction ("first determine whether deterministic parsing already solves most of this more reliably") is the first unit of work here, not the model-calling implementation.

**Deliberately not implemented this session.** Depends on #484's harness for any claim that `qwen3:4b-instruct` is schema-reliable enough for this use — #484's `classify_error` task is a structurally similar (if narrower) probe of exactly that question, so re-run it as a starting data point before building this rather than assuming.

### 485. Resource-aware local inference admission control

**Filed 2026-08-01**, from the same session as #484.

Whether document generation, embeddings, vision inference, the optional NLP microservice offload, and any future local-model delegation (#486) can submit overlapping Ollama workloads that compete for this laptop's limited RAM, memory bandwidth, and CPU time is a **credible, currently-unobserved risk**, not a confirmed defect — filed here rather than in `bugs.md` per this item's own brief ("do not file this as a bug without evidence of currently broken behavior"). `OLLAMA_MAX_LOADED_MODELS=1` (set after bugs.md #13's kernel-OOM-kill incident) already provides a blunt, global mitigation — it prevents two models being resident at once, but does nothing about CPU contention between concurrent callers targeting the *same* resident model, or about a queue/fairness/deadline story for what happens when two request classes (e.g. a live document-generation call and a background benchmark or delegation run) want the one model slot at the same time.

**Potential future design**, per the brief: one shared local-inference scheduler; request classes (lightweight, heavyweight, vision, embedding); one heavyweight request at a time; no 30B-and-vision overlap unless benchmarks prove it safe; production application processing takes priority over background analysis; bounded queue length; cancellation and deadlines; fairness; memory-pressure admission checks (this item's evaluation should use `internal/modelbench`'s `TakeHostSnapshot` as a starting point rather than a new implementation); metrics for queue delay, model loads, execution time, and rejections; background delegation (#486) yielding to browser/submission work.

**Value 4, Effort 6, Decay 1.0, score 0.67.** Value is moderate rather than high because the existing blunt mitigation already prevents the worst confirmed failure mode (OOM-kill); this item is about the next-order risk (contention and starvation, not crashes) which has not been observed live. Effort 6: a real shared scheduler with request classes and admission checks is substantial design and implementation work, the same order of magnitude as #486.

**Deliberately not implemented this session.** `cmd/modelbench`'s own runner (#484) is already a candidate *client* of a future admission-control scheduler — it currently benchmarks strictly sequentially and refuses to run alongside the production agent specifically because no shared scheduler exists yet; this item, if built, is what would let that refusal become a queued wait instead.

### 488. OpenClaw read-only sidecar evaluation

**Filed 2026-08-01**, from the same session as #484, at the position the user's own brief specified: **ranked below #484/#485/#486/#487** absent evidence of an immediate operational need, and marked as a user-decision item.

Evaluate OpenClaw only as an optional, read-only interface or notifier (sanitized daily health summary, novel-error notification, manual-review-queue notification, read-only daemon status from another device, invocation of a tiny fixed allowlist of diagnostic wrappers) — explicitly not to be installed as part of this evaluation, and not to be given `.env`, `pii.yaml`, database, application-document, home-directory, shell, browser, email, Git-write, or submission access under any pilot design. Any future pilot would need to start from an isolated process/container with a dedicated narrow workspace and none of the above access.

**Value 2, Effort 5, Decay 1.0, score 0.4 — below the 0.5 ROI floor, flagged ⚠️.** Per this backlog's floor rule, it stays open (never closed unilaterally) but must not be worked without explicit user confirmation. Recommendation: defer behind #484-#487; evaluate against a smaller custom notifier or a dashboard enhancement (`cmd/dashboard` already exists and already has a metrics API) before considering an external framework dependency at all.

### 477. Yahoo fallback requests carry no realistic browser headers or cookie jar

Closed — full account archived in `documentation/backlog_history/improvements_done_details.md`.

### 483. Confirm whether zombie career_agent_bin processes need reaping, or just documenting as harmless

Closed — full account archived in `documentation/backlog_history/improvements_done_details.md` item #483.

### 479. A permanent DNS failure spends the full retry/backoff budget instead of failing fast to a terminal status

Done — full account archived in `documentation/backlog_history/improvements_done_details.md` item #479.

### 474. ADRs have no process ensuring they get updated when the decision they describe changes

Closed — full account archived in `documentation/backlog_history/improvements_done_details.md`.

### 471. Extend bug #467's target-closed browser recovery to the cached-form-mapping fast path

**Found 2026-07-31** while fixing bug #467 (browser target closure). The independent review pass that checked #467's fix confirmed its scope boundary was reasonable for that Minor/Effort-2 bug, but flagged this as the one gap worth a follow-up: `AttemptSubmit`'s cached-mapping fast path (`mappingJSON, err := storage.GetFormMapping(domain)`, hit whenever an ATS has already been mapped once by the Learner Module) calls `handleDynamic` directly and, on any error including a target-closed one, deletes the cached mapping (`storage.DeleteFormMapping(domain)`) and falls back to Vision or returns a bare wrapped error — never reaching #467's new `isTargetClosedErr`/`newSubmitPage` recovery, which only lives inside the generic/dedicated-handler retry loop below it. In steady state this is likely the *most* common submit path, since most traffic to a given ATS domain hits an already-learned mapping rather than the Learner Module cold-start — so a transient browser crash here both wastes the attempt (as #467 fixed elsewhere) and discards a good learned mapping for no reason.

**Done (2026-07-31).** `AttemptSubmit`'s cached-mapping block (`pkg/submitter/browser.go`, right after the `dynErr := handleDynamic(...)` call) now runs a bounded recovery loop before reaching the existing `isSubmitGated`/cache-invalidation checks: on `isTargetClosedErr(dynErr)`, it closes the crashed page/session, recreates them via `newSubmitPage`, redoes the cheap `DeadRedirectReason`/`isDeadJobPage`/`isCaptchaBlocked` checks against the fresh page, re-resolves the fill target (`resolveFillTarget`), and retries `handleDynamic` against the same cached `mappingJSON` — capped at `maxTargetRecoveryAttempts` (1), the same constant and cap #467 uses, via a loop-local counter (`cachedMappingRecoveries`) rather than sharing the main retry loop's `targetRecoveries`, since this block runs before that loop's counter is even declared. If recovery succeeds, execution falls through to the unchanged `confirmOrError` call with the mapping intact; if it doesn't, the existing cache-invalidation/Vision-fallback path runs exactly as before.

Two new tests in `pkg/submitter/browser_test.go` cover both branches, seeding a real cached mapping via `storage.SaveFormMapping`/`InitDBWithPath` rather than mocking storage: `TestAttemptSubmit_CachedMappingRecoversFromTargetClosedOnce` (first page's submit click fails with a target-closed error, the recreated second page's click succeeds and the page's content carries a confirmation phrase — asserts the mapping still reads back unchanged via `storage.GetFormMapping` and `err` is nil) and `TestAttemptSubmit_CachedMappingTargetClosedRecoveryIsBounded` (both pages fail with the same target-closed error — asserts exactly 2 browser contexts were created, `err` wraps the target-closed sentinel, and the mapping was invalidated, matching pre-#471 behavior once the recovery budget is exhausted). `go build ./...`, `go vet ./...`, `go test ./...`, and `gofmt -l ./cmd ./pkg ./internal` all pass clean.

### 472. Extend bug #467's target-closed browser recovery to the security-code resubmit click

**Found 2026-07-31** while fixing bug #467. The post-security-code resubmit click inside `AttemptSubmit`'s `parser.DetectSecurityCodeChallenge`/`emailedCode` branch clicks the submit control after filling an emailed one-time code, and on failure returns directly (`fmt.Errorf("%w: %s (code entered, no confirmation)", ErrNeedsEmailVerification, ...)`) rather than setting `execErr` — so it never reaches #467's recovery check, which only guards the loop's own `execErr`-setting sites. Narrower value than #471: this path only exists on ATS platforms that gate submission behind an emailed code, a smaller slice of traffic, and by this point in the flow the code itself has already been consumed (`waitForSecurityCode`'s 90-second budget is not free to repeat).

**Fix direction:** either route this click's error through the same `execErr`/recovery machinery (requires restructuring this branch to fall through to the common post-loop check rather than returning early), or accept it as a smaller, standalone recovery given the code-reuse constraint — recovering here cannot simply re-click, since the code field must be re-entered against a fresh page and the code may no longer be valid for reuse depending on the ATS.

### 473. Extend bug #467's target-closed browser recovery to the Vision submission paths

**Found 2026-07-31** while fixing bug #467. `pkg/submitter/vision.go`'s `attemptQuarantinedVisionSubmit`/`AttemptVisionSubmit` are called from three places in `AttemptSubmit` (cached-mapping fallback, Learner Module fallback, unmapped-form fallback) and operate on the same `page`/`session` `AttemptSubmit` created, but confirm their own submission internally and return their result directly rather than passing through the retry loop's `execErr` handling — so a target-closed failure inside the Vision path has no recovery at all. Only reached when DOM-based mapping has already failed, so it is the fallback of a fallback: narrower blast radius than #471, comparable to #472.

**Fix direction:** either have the Vision entry points accept an already-created page/session and expose their own target-closed classification with a bounded single retry (recreating the page and re-taking the screenshot Vision needs), or leave this as the accepted floor of #467's scope — a browser crash mid-Vision-fallback is already a rare compound failure (DOM mapping failed *and* the browser crashed), and every job still gets a fresh context on its *next* attempt regardless.

### 470. `cmd/requeue`'s `countForStatus` is fully written and tested but never called

Closed — full account archived in `documentation/backlog_history/improvements_done_details.md`.

### 469. Add per-domain circuit breakers for repeated fetch and pre-flight timeouts

Closed — full account archived in `documentation/backlog_history/improvements_done_details.md`.

### 468. Filter weak discovery URLs and expose deferred queue state in the dashboard

Closed — full account archived in `documentation/backlog_history/improvements_done_details.md`.

### 464. `scripts/server.go`'s transpiled body is not `gofmt`-clean, and the documented `gofmt -l` loop can't catch it

Closed — full account archived in `documentation/backlog_history/improvements_done_details.md`.

### 463. `cmd/dashboard/ui` has no test framework, so frontend logic bugs are only ever caught by manual live verification

Closed — full account archived in `documentation/backlog_history/improvements_done_details.md`.

### 462. The RAG embedding retry loop duplicates `classifyGenerationError`'s logic instead of reusing it

Closed — full account archived in `documentation/backlog_history/improvements_done_details.md`.

### 460. The dashboard UI shows no visible sign that a metrics poll failed

Closed — full account archived in `documentation/backlog_history/improvements_done_details.md`.

### 459. `serveMetrics`'s by-source/by-variant breakdowns never check `rows.Err()` after their scan loops

Closed — full account archived in `documentation/backlog_history/improvements_done_details.md`.

### 458. The Gemini and OpenAI model columns have never been checked against a vendor catalogue

Closed — full account archived in `documentation/backlog_history/improvements_done_details.md`.

### 457. Enforce the model columns with a test instead of an instruction

Closed — full account archived in `documentation/backlog_history/improvements_done_details.md`.

### 456. Replace the concrete model columns with capability tiers that cannot expire

Closed — full account archived in `documentation/backlog_history/improvements_done_details.md`.

### 455. The Claude model column named one ID that does not exist and one that is a generation behind

Closed — full account archived in `documentation/backlog_history/improvements_done_details.md`.

### 454. Nothing in the Working Protocol updates `CHANGELOG.md`, and it drifted a full day

Closed — full account archived in `documentation/backlog_history/improvements_done_details.md`.

### 450. The shared DSN's `journal_mode(WAL)` can fail a connection outright, and `busy_timeout` does not cover it

Closed — full account archived in `documentation/backlog_history/improvements_done_details.md`.

### 448. `npm run lint` lints the dashboard's own committed build output

Done — full account archived in `documentation/backlog_history/improvements_done_details.md` item #448.

### 443. Eight Go files are not `gofmt`-clean, and nothing in the verification loop notices

Closed — full account archived in `documentation/backlog_history/improvements_done_details.md`.

### 442. Measure whether the NLP offload is worth keeping

Done — full account archived in `documentation/backlog_history/improvements_done_details.md` item #442.

### 423. Human-in-the-Loop "Copilot" Mode

Closed — full account archived in `documentation/backlog_history/improvements_done_details.md`.

### 418. Parallelize DiscoverJobs queries in funnel.go

Closed — full account archived in `documentation/backlog_history/improvements_done_details.md`.

### 419. Use defer for resp.Body.Close() in scraper.go

Closed — full account archived in `documentation/backlog_history/improvements_done_details.md`.

### 36. Reconcile setup and feature documentation with executable behavior

Closed — full account archived in `documentation/backlog_history/improvements_done_details.md`.

### 34. Make the local dashboard accessible and self-contained

Closed — full account archived in `documentation/backlog_history/improvements_done_details.md`.

### 35. Rank the queue from observed outcomes while preserving exploration

Closed — full account archived in `documentation/backlog_history/improvements_done_details.md`.

### 38. Produce a dry-run queue plan before requeueing or reprioritizing jobs

Closed — full account archived in `documentation/backlog_history/improvements_done_details.md`.

### 39. Track recent source health and application-attempt cost

Closed — full account archived in `documentation/backlog_history/improvements_done_details.md`.

### 29. Hard-code every repeatable application fact so the model stops guessing

Closed — full account archived in `documentation/backlog_history/improvements_done_details.md`.

### 28. Fill Greenhouse's required Location/Country comboboxes on the first pass

Closed — full account archived in `documentation/backlog_history/improvements_done_details.md`.

### 27. Local MCP server for career context

Closed — full account archived in `documentation/backlog_history/improvements_done_details.md`.

### 26. Discover via Greenhouse/Lever board feeds instead of search dorking (Done 2026-07-25)

Closed — full account archived in `documentation/backlog_history/improvements_done_details.md`.

### 25. Trim over-long job descriptions before scoring (Done 2026-07-25)

Closed — full account archived in `documentation/backlog_history/improvements_done_details.md`.

### 24. Per-call-type model selection — run ScoreJob on a smaller, faster model

Closed — full account archived in `documentation/backlog_history/improvements_done_details.md`.

## VALIDATED 2026-07-25 — and the model swap FAILED. `OLLAMA_FAST_MODEL` is deliberately left unset.

`qwen3:4b-instruct` was pulled and benchmarked against `qwen3:30b-instruct` on three identical real postings (byte-identical cached prompts, production's own `ScoreJob` path). It failed on **both** counts.

**It is not more accurate — it is dangerously more generous.**

| description | 4B | 30B | same decision? |
| --- | --- | --- | --- |
| 5,614 ch | 95 | 80 | yes |
| 6,624 ch | **85** | **0** | **NO** |
| 8,003 ch | 95 | 85 | yes |

Threshold agreement **2/3**, and the disagreement is the worst possible kind. The 30B's `0` was **correct**: that posting (`job-boards.greenhouse.io/edo/jobs/5132798007`) is a hybrid role stating *"we have a hybrid work policy of three days in the office"* and asking *"are you available to work on-site three days per week"*. With `remote_only: true`, rubric rule 2 deducts 80 from the baseline 80 — exactly 0. The 30B applied the rubric perfectly. **The 4B missed the hybrid requirement entirely and scored it 85**, which would have submitted a real application to a job the candidate is explicitly ineligible for — the same failure class as bugs.md #25. Across all three the 4B scored higher every time (+15, +85, +10): it is systematically lenient, and leniency here means false-positive applications.

**It is also not faster, for a structural reason worth remembering.** Cold-prompt timings: 30B **421s / 358s / 420s**; the 4B's one uncontaminated cold measurement **367s** — statistically indistinguishable. The cause is in the incumbent's own config: `qwen3moe.expert_count = 128`, `qwen3moe.expert_used_count = 8`. **The "30B" is a Mixture-of-Experts model activating only 8 of 128 experts per token — roughly 3B active parameters.** A 4B *dense* model therefore does *more* compute per token than the incumbent. This invalidates the item's founding premise: on this hardware there is no smaller-and-faster text model to swap to, because the model already in use is effectively a 3B-active model wearing a 30B badge.

**What was kept:** the routing itself (`genRequest.fast`, `OLLAMA_FAST_MODEL`) stays in the codebase, inert. It costs nothing unset, is covered by tests, and is exactly the plumbing needed if a genuinely faster model appears. **What was rejected:** enabling it. `qwen3:4b-instruct` remains pulled on disk but unused.

**Read this before proposing a smaller model again:** the lever on this box is *prompt length*, not parameter count — see improvements.md #25.

### 23. Static master cover letter, reused across every application (Done 2026-07-24)

Closed — full account archived in `documentation/backlog_history/improvements_done_details.md`.

### 22. Rank the discovery queue by resume-fit similarity, not just source-priority
**Request:** 2026-07-23 — user asked whether, in addition to `sourcePriorityCASE`'s success-likelihood ordering (`pkg/storage/manager.go`, added for bugs.md #45 verification), the queue could also push jobs most likely to result in a hire (i.e., resume/description match) toward the front.

**Investigation:** `job_funnel.fit_score` already is exactly this signal, but it's computed lazily — only via a full generative `ScoreJob` LLM call (`pkg/mcp/client.go`), which only runs *after* a job is dequeued by the live worker. Confirmed live 2026-07-23: 0 of 2,870 currently-`DISCOVERED` rows have a non-null `fit_score`. `ScoreJob` took roughly 8-9 minutes for one real job this same session on this machine's CPU-only Ollama — running it across the full backlog just to establish an order isn't viable. `GetEmbedding` (also in `pkg/mcp/client.go`) is comparatively fast (seconds, per the same session's log) and could serve as a cheap proxy signal for bulk pre-ranking instead. Also confirmed: `job_funnel` stores no description text for `DISCOVERED` rows (only company/title/URL), so a bulk embedding pass would need either a fresh per-job HTTP fetch (~1-2 hours of network time for 2,870 jobs) or a weaker title-only embedding. Separately confirmed `config.Profile.ValidateJob` (a cheap structured remote/salary check) exists but is dead code, never called anywhere live — remote/salary filtering today happens only inside `ScoreJob`'s rubric, at dequeue time.

**Proposed approach (not yet built):** a one-time/periodic embedding-similarity backfill (resume text vs. job description or title) blended into `GetDiscoveredJobs`'s `ORDER BY` alongside `sourcePriorityCASE`. Remote/salary enforcement would stay exactly as it is today (`ScoreJob`-only, at dequeue time) — a topically-similar but ineligible job could still occasionally consume a queue slot until `ScoreJob` catches it, the same risk class that already exists for source-priority ordering today.

**Deprioritized at the user's own request 2026-07-23:** building this now would compete for the same single local Ollama instance the Usability Gate's three remaining Major bugs (#8, #10, #14 in `bugs.md`) need to close organically, and `bugs.md`'s gate rule already outranks every `improvements.md` row regardless of score while the gate is open. Revisit once the gate reads `MET`.

**Un-deprioritized 2026-07-24 (`/groom_backlogs` pass):** the gate reached `MET` 2026-07-24 (`bugs.md`'s #8/#10/#14 closed via direct test). The condition this note said to wait for has been met — restored to plain `Pending`, fair game for normal ROI-ranked selection. Investigation and proposed approach above are unchanged and still accurate (re-checked: `job_funnel.fit_score` is still only populated at dequeue time, `config.Profile.ValidateJob` is still dead code).

**Shipped 2026-07-24 (user-requested, "we need to improve this to get more results"):** built the proposed approach directly — `job_funnel.fit_similarity REAL` (new column, idempotent migration mirroring `last_updated`'s pattern), `storage.GetJobsMissingFitSimilarity`/`UpdateFitSimilarity`, and `parser.BestSimilarity` (max cosine similarity between a job's title/company embedding and the resume's existing `career_chunks`). `GetDiscoveredJobs`'s `ORDER BY` now blends `COALESCE(fit_similarity, -1) DESC` in as a secondary sort key after `sourcePriorityCASE` — platform reachability stays primary (a topically perfect job through an auth-gated ATS is still worthless), fit similarity breaks ties within a tier, and a not-yet-backfilled `NULL` row sorts last within its tier, so the change is additive and never regresses a row `cmd/rankjobs` hasn't reached yet. New `cmd/rankjobs` CLI backfills the score out-of-band (bounded `-limit`, default 200, since embedding calls share the same local Ollama instance a live `cmd/agent` run may be using) rather than computing it inline per query. Title-only (not full description) embedding, per the investigation's own finding that `job_funnel` stores no description text for `DISCOVERED` rows and a full re-fetch would cost 1-2 hours across the backlog.

**Found and fixed a real, separate, higher-impact bug along the way:** a live 3-job test of `cmd/rankjobs` returned `fit_similarity = 0.0` for every job — traced to `career_chunks` storing 3072-dimension embeddings while the configured `nomic-embed-text` model actually produces 768-dimension vectors, silently zeroing every `CosineSimilarity` comparison (its length-mismatch guard, never meant to be load-bearing). This wasn't just breaking the new feature — `parser.RetrieveTopK`, used live in `cmd/agent`'s per-job resume/cover-letter tailoring, has been affected by the same mismatch for its entire live history, silently returning an arbitrary (not semantically relevant) set of resume chunks for every application. Filed and fixed as `bugs.md` #58: extracted `parser.IngestResumeChunks`/`CareerChunksNeedReingest`, `cmd/agent` now self-heals a dimension mismatch on restart, and a live fix was applied immediately (new `cmd/reingest` CLI) against the running database without disrupting the in-progress 82-job re-verification. See #58's Details section for the full diagnostic chain.

Tests: `TestMigrateJobFunnelFitSimilarity`, `TestGetJobsMissingFitSimilarity`, `TestGetDiscoveredJobsOrdersByFitSimilarityWithinTier`, `TestBestSimilarity`/`TestBestSimilarityEmptyChunks` (plus bug #58's own `TestCareerChunksNeedReingest`/`TestIngestResumeChunks*`). `go build/vet/test ./...` all pass. **Verified live end to end**: a 3-job `cmd/rankjobs -limit 3` run against the real database completed in ~12s and did not disrupt the concurrently-running 82-job re-verification (SQLite WAL mode tolerates the concurrent writer) — surfaced bug #58 before the dimension fix landed (every score came back 0.0). After #58's live fix (`cmd/reingest`) completed, re-ran the same 3-job `cmd/rankjobs -limit 3` against the same jobs: real, non-zero, non-uniform scores landed (`0.610`, `0.600`, `0.586`).

### 19. Prompt-injection/hidden-content detection CSV log (Done 2026-07-21)

Closed — full account archived in `documentation/backlog_history/improvements_done_details.md`.

### 11. Multi-step form logic (Workday-style)

Closed — full account archived in `documentation/backlog_history/improvements_done_details.md`.

### 12. Niche data source scrapers

Closed — full account archived in `documentation/backlog_history/improvements_done_details.md`.

### 13. Adaptive resume A/B testing

Closed — full account archived in `documentation/backlog_history/improvements_done_details.md`.

### 31. Fill Lever's required location on the initial pass, and generalise the combobox helper

Closed — full account archived in `documentation/backlog_history/improvements_done_details.md`.

### 33. Make the configured location resolvable on every geocoder, and compute the start date

Closed — full account archived in `documentation/backlog_history/improvements_done_details.md`.

### 32. Retrieve emailed one-time codes so verification gates can be completed

Closed — full account archived in `documentation/backlog_history/improvements_done_details.md`.

### 30. Detect unanswerable attestations before fit-scoring, not after

Closed — full account archived in `documentation/backlog_history/improvements_done_details.md`.

### 15. Email/portal conversion-rate analytics (Done 2026-07-24)

Closed — full account archived in `documentation/backlog_history/improvements_done_details.md`.

### 16. Automated assessment/screening solver (Done 2026-07-24, partial)

Closed — full account archived in `documentation/backlog_history/improvements_done_details.md`.

### 20. Dashboard tile for the MANUAL_REQUIRED queue

Closed — full account archived in `documentation/backlog_history/improvements_done_details.md`.

### 21. Separate the actionable manual-apply queue from historical failure noise

Closed — full account archived in `documentation/backlog_history/improvements_done_details.md`.

### 18. Configurable worker concurrency (Done 2026-07-20)

Closed — full account archived in `documentation/backlog_history/improvements_done_details.md`.

### 37. Revalidate posting freshness before expensive document generation

Closed — full account archived in `documentation/backlog_history/improvements_done_details.md`.

## 96 Filter out dead or expired job postings early
**Symptom:** Logs show 230+ `job posting is dead or expired` errors redirecting to error pages.
**Impact:** Wasted time scoring and attempting to submit to expired jobs.
**Fix:** Maintain a cache of dead URLs and pre-flight fetch the URL before full processing.

**Status:** Done (2026-07-28). Exported `DeadRedirectReason` from the submitter package to reuse its logic in a pre-flight `checkJobAlive` check at the start of the `cmd/agent` worker loop. A lightweight HTTP GET is issued; if it hits an error-page redirect or an off-domain migration (signaling the ATS board is gone), it halts immediately, marks the URL `INVALID_URL` in storage, and skips document generation and LLM scoring entirely.

### 402. Migrate from go-sqlite3 CGO driver to pure Go modernc.org/sqlite

Closed — full account archived in `documentation/backlog_history/improvements_done_details.md`.

### 403. Evaluate and prototype go-rod as a lightweight replacement for playwright-go

Closed — full account archived in `documentation/backlog_history/improvements_done_details.md`.

### 404. Batch SQLite writes into explicit transactions for performance

Closed — full account archived in `documentation/backlog_history/improvements_done_details.md`.

### 405. Refactor DOM parsing pipeline to use single AST pass and IO streams

Closed — full account archived in `documentation/backlog_history/improvements_done_details.md`.

### 406. Implement concurrent HTTP execution for ATS and job-board discovery scrapers

Closed — full account archived in `documentation/backlog_history/improvements_done_details.md`.

### 407. Utilize goroutines and bounded worker pools for dashboard queries and embedding inference

Closed — full account archived in `documentation/backlog_history/improvements_done_details.md`.

### 409. Frontend Rendering Speed Optimizations

Closed — full account archived in `documentation/backlog_history/improvements_done_details.md`.

### 410. AI Processing Optimizations (Concurrency & Context Limits)

Closed — full account archived in `documentation/backlog_history/improvements_done_details.md`.

### 411. Implement Stateful Graph-Based Pipeline Architecture

Closed — full account archived in `documentation/backlog_history/improvements_done_details.md`.

### 412. Implement Exponential Backoff with Jitter for LLM API Retries

Closed — full account archived in `documentation/backlog_history/improvements_done_details.md`.

### 429. Rewrite data ingestion CLI tools in Zero

Closed — full account archived in `documentation/backlog_history/improvements_done_details.md`.
