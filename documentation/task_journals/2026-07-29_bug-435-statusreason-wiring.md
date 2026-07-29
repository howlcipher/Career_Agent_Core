# Task Journal: bug #435 — `statusReason` is dead code for the statuses that need it

## Summary

- **Task:** `bugs.md` #435 (Minor, score 1.5) — `statusReason` in `cmd/dashboard/main.go` handles seven statuses but is called from only two queries, so four arms never render. Follow-on in the same run: #433 (Minor, 1.25) — `mergeStatuses` ranks four real statuses at 0.
- **Status:** In progress
- **Started:** 2026-07-29
- **Agent and model:** Claude Code / Opus 5 (orchestrator). User directive for this run: **prioritize Claude models** and use multiple agents, so implementation is delegated to Claude subagents rather than Antigravity/Ollama.

## Pre-Flight Re-Evaluation

- **Usability Gate check:** MET (re-verified 2026-07-29). `improvements.md` has **zero** Pending rows, so the two Minor Pending bug rows are the whole open queue. #435 is the highest-scoring above-floor item.
- **Model choice:** Claude, per explicit user instruction this run.
- **Skills routed:** `defensive_debugging`, `quality_assurance`, `frontend_engineering` (the dashboard UI is now a Vite/React app after #426), `technical_writing` (README / GitHub Pages pass).
- **Code re-verified:** yes, claims hold as written.
  - `statusReason` is at `cmd/dashboard/main.go:122` with 7 arms (`SKIPPED`, `BLOCKED_CAPTCHA`, `FAILED_SCORE`, `FAILED_SUBMIT`, `MANUAL_REQUIRED`, `AWAITING_REVIEW`, `INVALID_URL`).
  - Only two call sites: `:315` (the `SKIPPED` query) and `:338` (the `FAILED_SCORE`/`FAILED_SUBMIT` query).
  - The manual query at `:349-367` selects `company_name, job_title, last_updated, discovered_at` and **not** `status`, even though it matches `IN ('MANUAL_REQUIRED', 'AWAITING_REVIEW')` — so the two statuses are rendered identically, confirming the bug.
  - `Metrics` has no `LastManualReason` field, and `BlockedCaptcha` / `InvalidURL` exist only as counts with no per-status explanation anywhere in the payload.
  - `TestStatusReason_KnownAndUnknownCodes` (`main_test.go:268`) does test the function in isolation, as the bug says. Handler-level infra (`setupTestDB` / `fetchMetricsFromTestServer`) already exists, so wiring-level tests are cheap.

## Plan

- [ ] #435: select `status` in the manual query and surface `LastManualReason`, so `MANUAL_REQUIRED` and `AWAITING_REVIEW` read differently
- [ ] #435: give `BLOCKED_CAPTCHA` and `INVALID_URL` a rendering path so no arm stays unreachable
- [ ] #435: render the new reasons in the Vite/React UI (`cmd/dashboard/ui/src/App.tsx`) and rebuild `ui/dist` (it is `go:embed`-ed)
- [ ] #435: add wiring tests (handler-level, asserting the feature not the function)
- [ ] #433: rank the four unranked statuses in `mergeStatuses`, quarantine + `INVALID_URL` above `DISCOVERED` (delegated, disjoint files)
- [ ] README / `docs/index.html` accuracy pass
- [ ] `/groom_backlogs`, commit, push

## Progress Log

- 2026-07-29 — Journal opened. Selection and re-evaluation done in-session; both bug rows confirmed against current code before any delegation.

## Next Step

Launch the delegated agents (#433 fix, docs audit) and implement #435's Go + UI changes here.
