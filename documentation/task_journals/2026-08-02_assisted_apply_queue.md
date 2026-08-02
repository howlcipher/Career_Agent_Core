# Task Journal: Assisted Apply Queue

## Summary

- **Task:** #511 Assisted Apply Queue with resumable human handoff and legacy-job backfill
- **Status:** In progress
- **Started:** 2026-08-02
- **Agent and model:** Codex / gpt-5.6-terra. Deep-reasoning architecture review requested from the live approved Gemini Pro delegate before implementation.

## Pre-Flight Re-Evaluation

- **Usability Gate check:** `bugs.md`'s gate is met. The user explicitly selected this improvement regardless of normal ranking.
- **Model choice:** Deep-reasoning. Live availability includes Gemini 3.1 Pro High and local qwen3:30b-instruct. The required bounded Gemini Pro review was attempted but the live CLI reported its individual quota exhausted (reset in about 48 hours), so Codex retains the review and implementation rather than delegating to a weaker model.
- **Skills routed:** `hallucination_guardrails`, `software_development`, `test_and_verify`, and `quality_assurance` from `../ai_knowledge_library/.agents/skills/`.
- **Code re-verified:** The dashboard currently exposes metrics and only start/stop mutations. SQLite already has status/history primitives, but no assisted-plan, lease, or safe action model. Existing handoff statuses are `MANUAL_REQUIRED` and `AWAITING_REVIEW`; `BLOCKED_CAPTCHA` is also present in the funnel.

## Plan

- [x] Create the journal and record the selected backlog item.
- [x] Implement private, resumable assisted-plan storage plus a dry-run-first migration command.
- [ ] Add safe dashboard queue/detail APIs, actions, and document lookup.
- [ ] Add the Assisted Apply dashboard workflow and documentation.
- [ ] Add synthetic tests, execute the authorized production backfill safely, verify, commit, push, and merge.

## Progress Log

- 2026-08-02 — Read the supplied requirement brief, repository rulebook, safety guidance, model routing reference, SQLite and Playwright ADRs, and current dashboard/storage implementation. Confirmed the working tree was clean on `main` tracking `origin/main`.
- 2026-08-02 — `agy -p` architecture/security review with `gemini-3.1-pro-high` was rejected before execution because its individual quota is exhausted. No delegated work or external mutation occurred.
- 2026-08-02 — Added `assisted_applications` as an additive, privacy-safe SQLite plan table; it retains only stable job IDs, status/action metadata, attempt/lease/confirmation fields, and timestamps. Added `cmd/assist-migrate`: dry-run by default, `-confirm` required, idempotent, filterable, transaction-backed, and aggregate-only reporting. The dashboard has `/api/assisted` and an explicit Assisted Apply entry point that lists individual plans with translated human instructions. `go test ./pkg/storage ./cmd/dashboard ./cmd/assist-migrate`, UI tests, and the UI build pass.
- 2026-08-02 — Added atomic manual confirmation: only a selected assisted plan still in its original eligible status can become `APPLIED`; the canonical dedup record and `manual_user_confirmation` provenance commit in the same transaction. Added same-origin `POST /api/assisted/confirm` with bounded strict JSON and the dashboard confirmation dialog. Storage/dashboard/UI tests and UI build pass.
- 2026-08-02 — Added a 20-minute atomic assisted-browser lease. The dashboard can request continuation only for an active lease; it cannot mark completion or release another process’s lease. Queue rows now identify a safe ATS provider category instead of exposing a URL. Storage/dashboard/UI tests and UI build pass.
- 2026-08-02 — Added `cmd/assist` and its same-origin dashboard launch endpoint. A validated stable ID is the only browser-launch input; the command rechecks the plan, claims it atomically, uses a private dedicated persistent profile, and routes navigation through an authenticated loopback proxy plus the existing public-network validator. The command deliberately opens one visible browser only and does not solve CAPTCHA, collect credentials, infer answers, or submit. Focused storage/dashboard/UI tests and UI build pass.
- 2026-08-02 — Added server-side assisted document lookup and `View Résumé`/`View Cover Letter` controls. The resolver derives only known document names from the selected job’s canonical application directory, rejects root escape and symlinks at every component, and the dashboard serves the validated file with private no-store headers. Focused storage/dashboard/UI tests and UI build pass.
- 2026-08-02 — Added the user-facing progress stepper and sequential batch selection. Job cards now show provider, original status, freshness, document/mapping readiness, legacy provenance, attempt count, one translated human instruction, and only the action controls relevant to the next action. Batch mode reports `Application N of M`, opens a single selected session at a time, and offers `Stop After This Application`.
- 2026-08-02 — Documented the exact Assisted Apply workflow and intentional human-only boundaries in `README.md`, and added a user-visible changelog entry. Began the authorized production migration protocol: confirmed `applications.db` is present and the dashboard-started agent lock is held by its recorded `career_agent_bin` PID. Sent that exact process `SIGTERM`; after a safe 10-second grace period it still held the lock, so no backup or production database mutation was performed. Do not force-kill it; retry the normal graceful pause/check first.
- 2026-08-02 — Rechecked the lock: the agent had stopped and released it (its old PID was a zombie). Created and opened a timestamped private ignored SQLite backup under `applications/assisted-backups/`; then ran the native migration sequence without logging URLs or application data. Dry-run: eligible 493, imported 0, already queued 0, exclusions none. Confirmed migration: eligible 493, imported 493, already queued 0, exclusions none. Second confirmed run: eligible 0, imported 0, already queued 493, proving idempotency. Restored the original `career_agent_bin -daemon -cycle-limit 15 -cycle-interval 1m` process and verified it holds the lock again.
- 2026-08-02 — Added an Assisted Apply UI regression test for the waiting count, visible queue, human-readable CAPTCHA instruction, legacy provenance, state-relevant controls, and disabled Continue before a live browser exists. UI tests, UI build, and focused Go tests pass.

## Next Step

Audit every requirement against the current implementation, reconcile any remaining legacy eligibility concern safely, then decide whether the final backlog close is justified before journal removal, push, and merge.
