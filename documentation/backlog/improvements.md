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

**2026-08-06 groom pass.** #513 was filed this pass and is now the highest-scoring Pending row here at 1.5. Every other Pending row re-verified against current code and live state. **#485 holds at 0.67:** no shared inference scheduler exists anywhere in the tree, and its premise was checked live rather than assumed — `OLLAMA_MAX_LOADED_MODELS=1` is genuinely in force (`systemctl --user show ollama` on the running MainPID, not just the unit file on disk). **#497 holds at 0.50**, exactly at the floor: no answer store, and the 26 `MANUAL_REQUIRED` rows its Value rests on are still 26. **#493 stays 0.33 ⚠️** — its "2 APPLIED rows" was stale and is corrected in its Details section (7 confirmed applications now exist — **not** the 58 `applied_jobs` rows this paragraph originally cited; 51 of those predate bug #94's 2026-07-25 fix and record document generation, not submission, per bugs.md #529's close), but the correction does not move the score: the binding constraint was always zero *outcomes*, and that is still zero against 49 processed emails, which this pass filed as bugs.md #529. **#473 stays 0.38 ⚠️** — all three `attemptQuarantinedVisionSubmit` call sites confirmed present with no target-closed handling. **#488 stays 0.40 ⚠️**, unchanged user decision. Three closed rows still carrying inline narratives were archived under the Working Protocol's step 8, and **#509's row pointed at an archive entry that had never been written** — the live file held the only copy, now rescued. *(Prior status paragraph archived in `documentation/backlog_history/improvements_groom_history.md`.)*

| # | Improvement | Status | Score (V×D÷E) | Tier | ROI rationale |
|---|---|---|---|---|---|
| 536 | [Normal-browser Career Agent companion](#536-normal-browser-career-agent-companion) | Pending | 0.6 = 3×1.0÷5 | deep-reasoning | Designed in `docs/adrs/ADR-005-Browser-Companion.md`, deliberately not built. The Copy Application Packet already recovers most of the operator time in the handoff case at none of the complexity, so the remaining benefit does not yet justify shipping a browser extension that handles PII. |
| 537 | [Apply-session auto-advance is only exercised by unit tests, never by a live multi-application run](#537-apply-session-auto-advance-is-only-exercised-by-unit-tests-never-by-a-live-multi-application-run) | Pending | 1.0 = 3×1.0÷3 | standard | The state machine and its refusals are unit-tested, but nothing has yet driven two real applications end to end through open → questions → answers → confirm → auto-open. That is the one claim in this feature that only a live run can settle. |
| 512 | [Application Mode selector and configurable fit threshold](#512-application-mode-selector-and-configurable-fit-threshold) | Done (2026-08-04) | — | deep-reasoning | See `documentation/backlog_history/improvements_done_details.md` item #512 for the full account. |
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
| 492 | [Explicit first-attempt SLA and bounded fresh-queue admission](#492-explicit-first-attempt-sla-and-bounded-fresh-queue-admission) | Done (2026-08-02) | — | standard | See `documentation/backlog_history/improvements_done_details.md` item #492 for the full account. |
| 487 | [Lightweight 4B log triage and context compression](#487-lightweight-4b-log-triage-and-context-compression) | Done (2026-08-02) | — | standard | See `documentation/backlog_history/improvements_done_details.md` item #487 for the full account. |
| 472 | [Extend bug #467's target-closed browser recovery to the security-code resubmit click](#472-extend-bug-467s-target-closed-browser-recovery-to-the-security-code-resubmit-click) | Done (2026-08-02) | — | standard | See `documentation/backlog_history/improvements_done_details.md` item #472 for the full account. |
| 473 | [Extend bug #467's target-closed browser recovery to the Vision submission paths](#473-extend-bug-467s-target-closed-browser-recovery-to-the-vision-submission-paths) | Pending ⚠️ below floor — same-theme decay | 0.38 = 3×0.25÷2 | standard | Vision fallback still has no target-closed recovery; work only with explicit user confirmation. |
| 513 | [111 backlog table rows link to detail sections that no longer exist](#513-111-backlog-table-rows-link-to-detail-sections-that-no-longer-exist) | Pending | 1.5 = 3×1.0÷2 | mechanical | Every session navigates these files by clicking a row through to its Details. 92 rows in `bugs.md` and 19 in `improvements.md` land nowhere. Cheap to fix and, unlike the last two conventions that silently broke, cheap to keep fixed with a test. |
| 485 | [Resource-aware local inference admission control](#485-resource-aware-local-inference-admission-control) | Pending | 0.67 = 4×1.0÷6 | deep-reasoning | Existing one-model limit prevents OOM; contention remains an unobserved risk. |
| 497 | [User-approved application-answer memory](#497-user-approved-application-answer-memory) | Done (2026-08-13) | — | deep-reasoning | See `documentation/backlog_history/improvements_done_details.md` item #497 for the full account. |
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

### 506. `/work_next_item`'s selection rule never returns to `bugs.md` once the gate is MET, starving Minor Pending bugs indefinitely

Done — full account archived in `documentation/backlog_history/improvements_done_details.md` item #506.

### 500. Add a missing index on `job_funnel(discovered_at)`

Closed — full account archived in `documentation/backlog_history/improvements_done_details.md`.

### 505. `storedPromptInjectionThreats` and `toStoredThreats` are the same field-for-field conversion, written twice

Done — full account archived in `documentation/backlog_history/improvements_done_details.md`.

### 491. Define authoritative mission metrics and surface them on the dashboard

Done — full account archived in `documentation/backlog_history/improvements_done_details.md` item #491.

### 499. Persist `discovery_source` at `AddToFunnel` time

Done — full account archived in `documentation/backlog_history/improvements_done_details.md` item #499.

### 495. No-progress / dominant-failure-reason watchdog

Done — full account archived in `documentation/backlog_history/improvements_done_details.md` item #495.

### 496. ATS capability and automation-success registry

Done — full account archived in `documentation/backlog_history/improvements_done_details.md` item #496.

### 494. Append-only funnel/attempt stage ledger

Done — full account archived in `documentation/backlog_history/improvements_done_details.md` item #494.

### 493. Rank by expected confirmed-application yield

**Filed 2026-08-01**, mission-alignment audit (seeded candidate C).

`RankJobs` (`pkg/storage/ranking.go:110-178`) already combines Bayesian-smoothed per-hostname source health (`ComputeSourceScores`), a bad-outcome `PenaltyFactor`, `fit_similarity`, the confirmed `exp(-0.02×ageDays)` freshness multiplier, bugs.md #481's 10-day urgency override, and a 20% exploration floor with sparse-source reserved slots — a real, working approximation of the brief's conceptual objective already. **Re-verified 2026-08-01:** #499 persists `discovery_source`, #496 records outcome-derived ATS capability evidence, and #494 now records future attempt-stage transitions. The ledger begins empty by design and no sufficiently large confirmation/outcome sample exists, so changing rank weights now would still be speculation. Interview-outcome data correctly is **not** used anywhere in ranking today, and that is still not a gap.

**Re-verified 2026-08-06 (`/groom_backlogs`) against the live database, and the numbers this row rested on have moved — the conclusion has not.** `applied_jobs` now holds **58** confirmed applications (the row previously said 2) and `job_funnel` holds 7 `APPLIED`; the 2 that remain accurate are `application_attempts` rows with `terminal_class = 'APPLIED'`, which is the ledger a yield-ranking change would actually learn from. The other 56 were confirmed through the assisted path and never went through `RecordAttempt`. **Observed interview and rejection outcomes remain zero.** They do have a home — `cmd/tracker` writes `REJECTED` and `INTERVIEW_REQUESTED` straight into `job_funnel.status` (`pkg/tracker/imap.go:340-347`) — but the live database holds **0 rows in either status** against 49 processed emails. So the constraint this item is sequenced behind is unchanged and remains the binding one: there is still no outcome sample to rank against, and changing rank weights now would be speculation. (The zero-outcome result against 49 processed emails is filed separately as bugs.md #529; it is a discrepancy to check, not an assumption to build on.)

**Proposed direction:** use #499's `discovery_source` and #496's capability evidence in `RankJobs` to weight estimated form reach and confirmation probability, alongside the existing fit/freshness/source-health signals, while preserving Bayesian smoothing, the exploration floor, and deterministic tie-breaking. **Explicitly sequenced after #494**: the newly available source and site evidence is still incomplete until attempt stages are reconstructable.

**Acceptance criteria:** a new ranking formula preserves strict user constraints (salary/remote/etc.) and the existing exploration floor; a regression test corpus of synthetic jobs across discovery sources/ATS providers/outcomes shows the new ordering favoring higher-yield combinations without starving any single source below the floor.

**Automated tests:** extend the existing `RankJobs`/`ranking.go` test suite with cases for the new signals; a starvation-prevention test mirroring whatever already guards the exploration floor.

**Safe live verification:** compare the top-N of a live `GetQueuePlan` dry run before/after against the current baseline, checking no eligible source drops to zero share.

**Boundaries:** does not touch strict eligibility filtering (salary/remote/blocklist), which stays exactly as it is today.

**Groomed 2026-08-01:** the current checkout has no readable live outcome database, so no evidence can justify changing rank weights or validate a new formula. Value is reduced to 1 while that dependency is absent; the item is below the ROI floor until fresh outcome evidence becomes available.

**Groomed 2026-08-02:** the dashboard's live `/api/metrics` endpoint is readable again and reports 2 applied rows, 0 interviews, and 0 rejections. That resolves the prior checkout-access limitation, but the sample remains far too small to support a causal yield estimate or validate a ranking rewrite. Value rises conservatively to 2; `2×1.0÷6 = 0.33` remains below the ROI floor. Keep the item deferred until outcome evidence exists.

### 498. Company and role-family duplicate/cooldown protection

Done — full account archived in `documentation/backlog_history/improvements_done_details.md` item #498.

### 536. Normal-browser Career Agent companion

**Filed 2026-08-13**, alongside the Assisted Apply fast-path work.

Some ATS platforms cannot be served by the assisted browser at all — Lever is the confirmed case (bugs.md #520): applications completed in the guarded Playwright browser are rejected with "There was an error verifying your application", while the identical application submitted from an ordinary browser succeeds. `storage.AssistedBrowserRejectionReason` already routes those postings to the operator's own browser, which is the right answer and stays. But that is also where Career Agent's help currently stops.

**Designed, not built.** `docs/adrs/ADR-005-Browser-Companion.md` records the full contract a companion would have to satisfy: loopback-only communication, a paired per-session token (a localhost port with no authentication is reachable by any page the operator visits, which would expose their PII to any site), origin allowlists on both sides, sanitized field descriptors in place of page content, resolution through the existing `answers.Store` so sensitive categories behave identically, and — the load-bearing one — **no submit verb in the protocol at all**, so no bug, compromised page, or future contributor can reach one.

**Why it is deferred rather than built.** The Copy Application Packet (`/api/assisted/packet`) shipped in the same pass and already gives the operator every prepared value, one click each, for exactly these handoff cases. That recovers most of the time a companion would, at none of the cost of distributing and updating a browser extension that handles personal data. Revisit when the packet is demonstrably insufficient in practice, not before.

**Value 3, Effort 5, Decay 1.0, score 0.6.** Effort is the honest number for an extension plus a second DOM-extraction implementation in JavaScript against the same shapes `pkg/submitter/questions.go` already handles in Go — two implementations of one idea that will drift.

### 537. Apply-session auto-advance is only exercised by unit tests, never by a live multi-application run

**Filed 2026-08-13**, by the session that built it, as an honest statement of what was and was not verified.

The apply-session state machine is unit-tested against every rule that matters: it advances only on a terminal item state, a closed browser pauses it rather than advancing, a confirmation and its session advance commit in one transaction, stop-after-current records the remainder as stopped, and `GetAssistedLaunchInfo` still gates every auto-launch exactly as it gates a manual click. The dashboard handlers and the React session bar are tested too.

**What has not happened is a live run.** Nothing has yet driven two real applications end to end — open → refill → questions → answers → review → confirm → the next application opening by itself — against real employer pages. Everything between `advanceApplySession` calling `launchAssistedApplication` and a visible browser appearing is unexercised outside a unit test, and this repository's own history is emphatic that runtime wiring escapes `go test` (see the memory of an uninitialized package-global `db` and `os.IsNotExist` on a wrapped error, both of which only a live binary run caught).

**How to close it.** Back up `applications.db`, select two queued Greenhouse applications, start a session, and watch for: the first browser opening without a second click; the question list appearing on the card rather than a form to re-read; answers landing in the visible form; the confirmation advancing to application 2 automatically; and `career_agent.log` containing no question text, answer text, or PII. Then close this row with what was observed, including anything that did not work.

**Value 3, Effort 3, Decay 1.0, score 1.0.** Effort is a live session with a real browser and real postings, not a code change — assuming nothing is found.

### 497. User-approved application-answer memory

Closed — full account archived in `documentation/backlog_history/improvements_done_details.md` item #497.

### 492. Explicit first-attempt SLA and bounded fresh-queue admission

Closed — full account archived in `documentation/backlog_history/improvements_done_details.md`.

### 484. Local-model benchmark and routing-evidence harness

Closed — full account archived in `documentation/backlog_history/improvements_done_details.md`.

### 486. Safe local-model delegation harness

Closed — full account archived in `documentation/backlog_history/improvements_done_details.md`.

### 487. Lightweight 4B log triage and context compression

Closed — full account archived in `documentation/backlog_history/improvements_done_details.md`.

### 513. 111 backlog table rows link to detail sections that no longer exist

**Found 2026-08-06** by the `/groom_backlogs` pass, while checking that this pass had not itself broken any anchors (it had broken four, since fixed).

Every row in these tables is a Markdown link into a `### N.` heading in the same file. **92 rows in `bugs.md` and 19 in `improvements.md` point at headings that are not there** — clicking them scrolls nowhere. Verified rather than counted: for example `bugs.md`'s #465 row links to `#465-internalbacklogs-pending-cell-floor-was-a-historical-snapshot-and-ordinary-backlog-progress-tripped-it`, and no `### 465.` heading exists anywhere in the file.

**Cause.** The 2026-08-01 restructure moved closed items' narratives into `documentation/backlog_history/`. Rows that kept a one-line pointer in the table *and* a one-line `### N.` stub survived fine; rows whose whole Details section was deleted kept a link with nothing to land on. Nothing checked, so nothing complained.

**Why it is worth fixing rather than accepting.** These files exist to be navigated by a session with zero context — `/work_next_item` reads the table, picks a row, and follows its link to the Details before writing any code. A dead link means that session either scrolls the whole file or proceeds without the detail. It is also the third convention in this repo to break silently and sit broken (improvement #455's model IDs, bug #441's model list), and the established answer to that class is a test, not a promise.

**Fix direction.** Two parts, and the second is the durable one:

1. Give every row with a missing target a one-line `### N. <title>` stub carrying the pointer at its archive entry — the same shape #131, #24 and #37 already have. Mechanical; derive the heading text from the row's own link text so the generated slug matches by construction.
2. **Add the check to `internal/backlog`**, beside the Tier validation. It is a pure string check over three files with no live dependency, so it is exactly the kind of convention that should be enforced by `go test ./...` rather than by a groom pass remembering. Slugging rule to implement: lowercase, drop backticks/asterisks/apostrophes/periods/slashes, non-alphanumerics that are not `_` or `-` become nothing, spaces become hyphens — note that an em dash surrounded by spaces yields a double hyphen.

Scoped as `mechanical`: the transformation is deterministic and the test that guards it is a string comparison. The only judgement is what title text to use for the stubs, and the row's own link text already supplies it.

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

Done — full account archived in `documentation/backlog_history/improvements_done_details.md` item #472.

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

### 96 Filter out dead or expired job postings early

Done — full account archived in `documentation/backlog_history/improvements_done_details.md` item #96.

### 37. Revalidate posting freshness before expensive document generation

Closed — full account archived in `documentation/backlog_history/improvements_done_details.md`.

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
