# 🚀 Improvement Backlog

This document is the authoritative, ranked backlog for Career Agent Core enhancements. It is designed so a fresh session (Claude Code, Gemini CLI, or Antigravity) can pick up the next item with zero prior chat context. It mirrors the format used by the AI Knowledge Library (`../ai_knowledge_library/improvements.md`), this project's sibling repo, and shares its Working Protocol with `bugs.md`.

**Everything in this file is explicitly nice-to-have.** `bugs.md` opens with a Usability Gate defining what "100% usable" means for this project; while that gate is unmet, closing bugs outranks working any Pending row here, regardless of score. Check the gate status before picking an item from this file.

## Working Protocol

This protocol applies to every worked task — improvements and bug fixes alike. Bugs are tracked in `bugs.md`, which mirrors this file's format and shares this protocol; a hotfix without a backlog row still gets steps 1 through 4 and 7.

1. **Check the Usability Gate first.** Read the gate checklist at the top of `bugs.md`. If it is not yet met, work the highest-priority open bug instead of anything in this file, unless the user explicitly asks for a specific improvement.
2. **Open a task journal.** Copy `documentation/task_journals/TEMPLATE.md` to `documentation/task_journals/YYYY-MM-DD_<slug>.md`. The journal is the resume point after a session limit or interruption: update it and commit at every milestone, and always keep its "Next step" line current so a fresh session can continue from the journal alone. It is a resume artifact, not permanent documentation — it gets deleted in step 7; anything durable (findings, decisions, verification evidence) must land in the item's Done note or a proper doc before the task closes.
3. **Re-evaluate the model (every run, before starting).** The table's model columns are starting suggestions, not commitments. Check what is actually available right now: Claude Pro subscription (Claude Code), Google subscription via Antigravity CLI (`agy models` lists live names; quota is per model and shared across Gemini tiers), and local Ollama (`curl localhost:11434/api/tags` — models change). To preserve Claude session limits, the default is to orchestrate from Claude Code but delegate implementation to a non-Claude model headlessly (`agy -p "<brief>" --model "<model>" --mode accept-edits --print-timeout 30m` from the repo root), or to local Ollama for small, well-bounded subtasks. Claude Code subagents bill the same Claude plan and do not save limits — never delegate to them for this purpose. Record the choice and one line of reasoning in the journal.
4. **Route the matching skills.** This repo has no local skills directory; skills come from the library at `../ai_knowledge_library/.agents/skills/<skill_name>/SKILL.md` (see `AGENTS.md`). Read the matching SKILL.md file(s) before planning — `devops_sre`, `cyber_security`, and `software_development` are common fits here given the Go/Playwright/SQLite stack.
5. **Re-verify the item against the current code**, not just the backlog row — this backlog was rebuilt from a stale, unscored list on 2026-07-19 and code had already drifted from it (see the Details section for examples). If it is stale, update the row, merge it, or close it with a dated note explaining why. A well-documented closure counts as completing this run.
6. **Read the detail section** for the item (linked from the table) before coding.
7. **Finish the loop:** every code change ships with relevant tests in the same task (`go test ./...` covering the new behavior's success and failure paths). Verify the change works end to end, run `go build ./...`, `go vet ./...`, and `go test ./...` before committing, commit with `<type>(<scope>): <description>`, set the item's Status to `Done (YYYY-MM-DD)` in the table with a Done note, delete the task journal in the final commit, and push. Committing and pushing verified work is the default and needs no per-task approval; only destructive or history-rewriting git operations require asking first.

## Ranked Backlog (best ROI first)

Pending rows are ranked by a diminishing-returns score:

**Score = (Value × Decay) ÷ Effort**

- **Value (1–8):** pain or capability gained if the item ships.
- **Decay:** geometric halving per already-shipped item in the same theme (1.0 → 0.5 → 0.25 …). New-capability items that open a new curve keep Decay 1.0.
- **Effort (1–8):** roughly log-scale; 1 = minutes, 8 = weeks.
- **ROI floor = 0.5:** items scoring below the floor stay open but are flagged ⚠️ and must not be worked without explicit user confirmation. At selection time, skip past them to the highest-scoring above-floor item and ask the user to confirm, re-scope, or close.

Scores apply to Pending rows only; Done and Closed rows show `—`.

| # | Improvement | Status | Score (V×D÷E) | Claude model | Gemini model | ROI rationale |
| --- | --- | --- | --- | --- | --- | --- |
| 1 | V2 Architecture Blueprint | Done | — | — | — | Shipped before this backlog restructure |
| 2 | Dynamic Source Discovery (FunnelEngine) | Done | — | — | — | Shipped before this backlog restructure |
| 3 | Playwright Dynamic Generation (self-learning DOM mapper) | Done | — | — | — | Shipped before this backlog restructure |
| 4 | DOM Pruning/Minification | Done | — | — | — | Shipped before this backlog restructure |
| 5 | Graceful DB Degradation (self-healing cache) | Done | — | — | — | Shipped before this backlog restructure |
| 6 | Stealth & Proxies (webdriver overrides) | Done | — | — | — | Shipped before this backlog restructure |
| 7 | Playwright Scraper Fallback (DuckDuckGo) | Done | — | — | — | Shipped before this backlog restructure |
| 8 | Visual Reasoning (VLM) for Form Submissions | Done | — | — | — | Found already implemented (`pkg/submitter/vision.go`) during the 2026-07-19 backlog rebuild; stale in the old list |
| 9 | Metrics Dashboard (Web) | Done | — | — | — | Found already implemented (`cmd/dashboard`) during the 2026-07-19 backlog rebuild; stale in the old list |
| 10 | Cron-Driven Drip Campaigns | Done | — | — | — | Found already implemented (`cmd/agent --daemon`) during the 2026-07-19 backlog rebuild; stale in the old list |
| 15 | [Email/portal conversion-rate analytics](#15-emailportal-conversion-rate-analytics) | Pending | 1.33 = 4×1.0÷3 | Sonnet 5 | Gemini 3 Pro | Small-medium; re-scoped down from a full tracker build since `pkg/tracker` already exists — just needs the analytics layer on top |
| 16 | [Automated assessment/screening solver](#16-automated-assessmentscreening-solver) | Pending | 1.25 = 5×1.0÷4 | Sonnet 5 | Gemini 3 Pro | Medium effort; unblocks any ATS that gates the application behind pre-screening questions |
| 17 | [CAPTCHA & anti-bot solving](#17-captcha--anti-bot-solving) | Pending | 1.25 = 5×1.0÷4 | Sonnet 5 | Gemini 3 Pro | Medium effort; needs a paid 2captcha/capsolver key — confirm with user before implementing |
| 18 | [Configurable worker concurrency](#18-configurable-worker-concurrency) | Pending | 2.0 = 4×1.0÷2 | Sonnet 5 | Gemini 3 Pro | Small effort, directly motivated by 2026-07-20 live-run findings (see `bugs.md` #3) — current hardcoded value assumes a paid-API backend and overloads a local single-slot Ollama server |
| 11 | [Multi-step form logic (Workday-style)](#11-multi-step-form-logic-workday-style) | Pending | 1.2 = 6×1.0÷5 | Sonnet 5 | Gemini 3 Pro | Medium-high effort; Workday and Taleo are common ATS platforms currently only reachable via single-page assumptions |
| 12 | [Niche data source scrapers](#12-niche-data-source-scrapers) | Pending | 1.0 = 4×1.0÷4 | Fable 5 | Gemini 3 Pro | Medium effort; new source class (YC, Otta, Wellfound, HN) distinct from the existing 16 ATS domains |
| 13 | [Adaptive resume A/B testing](#13-adaptive-resume-ab-testing) | Pending | 1.0 = 5×1.0÷5 | Sonnet 5 | Gemini 3 Pro | Medium-high effort; needs the conversion analytics from item 15 first to know which variant is winning |
| 14 | [Local LoRA fine-tuning loop](#14-local-lora-fine-tuning-loop) | Pending ⚠️ below floor | 0.43 = 3×1.0÷7 | Fable 5 | Gemini 3 Pro | Large effort (dataset collection, training loop, eval); real payoff is speculative until items 13/15 establish what "good" looks like — confirm before working |

## Details

### 11. Multi-step form logic (Workday-style)
`pkg/submitter/dynamic.go` maps `workday.com` and `taleo.net` to template names (`WorkdayTemplate`, `TaleoTemplate`), but nothing in the codebase drives a multi-page "Next" button flow — the submitter pipeline (`TwoStepVerification`, `AttemptSubmit`, `AttemptVisionSubmit`) operates on a single page load. Workday applications are frequently 4-8 pages (personal info, work history, EEO, review). Build a state machine that: fills the current page's mapped fields, detects and clicks the "Next"/"Continue" control, re-extracts and re-prunes the DOM on the new page, and repeats until a terminal "Submit"/"Review" state, persisting `ExecutionState` (already defined in `dynamic.go`) between steps so a crash mid-flow can resume instead of restarting. Verified 2026-07-19: confirmed via `grep` that only the ATS-domain-to-template-name mapping exists; no page-transition logic was found in `pkg/submitter/` or `pkg/scraper/funnel.go`.

### 12. Niche data source scrapers
`pkg/scraper/funnel.go`'s `TargetATS`/`atsDomains` lists cover 16 traditional ATS platforms (Greenhouse, Lever, Workday, etc.) reached via SerpApi/DuckDuckGo dorking. YCombinator "Work at a Startup", Otta, Wellfound, and HackerNews "Who is Hiring" are not ATS-hosted job posts — they need their own scraper implementations (likely direct API/HTML parsing per source, not the dork-and-match pattern `funnel.go` uses for ATS domains), wired into `FunnelEngine` as additional sources. Verified 2026-07-19: no references to any of these four sources exist anywhere in `pkg/`.

### 13. Adaptive resume A/B testing
`pkg/config/profile.go` has a single `CoverLetterTone` string field — one tone per run, no variant generation or outcome tracking. Building real A/B testing requires: (1) generating 2+ resume/cover-letter tone variants per application, (2) tagging which variant was sent in `applications.db`, (3) joining that against interview/rejection outcomes once item 15's conversion analytics exist, and (4) a selection policy that shifts weight toward the higher-performing variant. Do item 15 first — without conversion data, "A/B testing" has no signal to pivot on. Verified 2026-07-19: confirmed `CoverLetterTone` is a single scalar config value with no variant or outcome-tracking code.

### 14. Local LoRA fine-tuning loop
Collect scored jobs (fit score, outcome) into a training dataset and periodically fine-tune a local model on the user's specific preferences, reducing reliance on prompting alone. Genuinely large scope: dataset export from `applications.db`, a training pipeline (likely outside Go — this would shell out to a Python/PEFT toolchain), eval harness to confirm the fine-tuned model doesn't regress scoring quality, and a rollout/rollback mechanism. Real payoff depends on having enough labeled outcome data, which items 13 and 15 would start accumulating. **2026-07-19 scoring note:** below the 0.5 ROI floor at Value 3 (speculative payoff, no labeled data yet) ÷ Effort 7 (large, cross-language build). Flagged ⚠️; do not work without explicit user confirmation. Recommend deferring until items 13/15 ship and real outcome data exists to justify the effort.

### 15. Email/portal conversion-rate analytics
The original backlog entry called for building `pkg/tracker` from scratch; verified 2026-07-19 that it already exists (`pkg/tracker/imap.go`, wired up via `cmd/tracker/main.go`) and polls IMAP for rejection/interview signals per the README's "Email Tracker" feature. What's missing is the analytics layer: no code computes or surfaces a conversion rate (interviews or offers ÷ applications sent, broken down by role/source/ATS/tone). Add a query/report path — a `pkg/tracker` or `pkg/storage` function that aggregates `applications.db` outcomes, surfaced via the existing `cmd/dashboard` web UI or a new CLI report command. This is a prerequisite for item 13 (A/B testing needs a signal to compare variants against).

### 16. Automated assessment/screening solver
Some ATS platforms (Workday in particular, see item 11) insert pre-screening multiple-choice or numeric questions before the standard form fields. Use the existing LLM provider abstraction (`pkg/mcp`) to read the question text extracted from the pruned DOM and answer strictly from `USER_PROFILE.md`/`profile.yaml` facts — never invent an answer not grounded in those files, to avoid the agent lying on a legal application question. Likely lands as a new `pkg/submitter` function invoked from within the multi-step flow (item 11), so sequencing after item 11 is natural but not strictly required if a screening page can appear standalone.

### 17. CAPTCHA & anti-bot solving
`pkg/submitter/browser.go` currently only comments that a CAPTCHA will cause the 45s page timeout to fire — there is no solving integration. Integrating `2captcha` or `capsolver` requires a paid API key, which is not currently in `.env.example` and must be discussed with the user before implementation per this repo's constraint on paid services (see `AGENTS.md`). Scope once approved: add the provider client, wire a CAPTCHA-detection check into `AttemptSubmit`/`AttemptVisionSubmit`, and handle the solve-then-continue flow without blocking the worker pool indefinitely.

### 18. Configurable worker concurrency
`cmd/agent/main.go` hardcodes `numWorkers := 10` with the comment "Increased to 10 workers for massive concurrency on Paid Tier" — tuned for a paid API backend (Gemini/Claude), not the local-Ollama-by-default setup `.env.example` now recommends. On 2026-07-20, running with `LLM_PROVIDER=ollama` and 10 concurrent workers against a single-slot local `llama-server` (`-np 1`) caused severe request queuing and contributed to the conditions behind `bugs.md` #3 (context-window overflow hang) — all 10 workers hammer the same model server simultaneously regardless of backend. Make worker count configurable via `profile.yaml` or an env var (e.g. `WORKER_COUNT`), with a sensible lower default (2-3) when `LLM_PROVIDER=ollama` is detected, since local single-instance inference can't usefully parallelize the way a paid API can. Verified 2026-07-20: confirmed via `grep` that `numWorkers` has no config wiring anywhere in `pkg/config` or `cmd/agent`.
