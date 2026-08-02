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

## Next Step

Implement a visible single-job assisted browser command using the existing guarded Playwright boundary, then wire the primary dashboard action to it.
