# Task Journal: Mission metrics dashboard

## Summary

- **Task:** Improvement #491 — Define authoritative mission metrics and surface them on the dashboard
- **Status:** In progress
- **Started:** 2026-08-01
- **Agent and model:** Codex / GPT-5.6 Terra

## Pre-Flight Re-Evaluation

- **Usability Gate check:** MET (2026-08-01); #491 (score 1.5) is the highest-ranked eligible item across the open bug and improvement queues.
- **Model choice:** Codex handled this standard-tier dashboard feature directly. Live alternatives are Gemini 3.6 Flash High and local qwen3:30b-instruct; no independent delegation is needed for the bounded API/UI/test change.
- **Skills routed:** `software_development`, `frontend_engineering`, `quality_assurance`, `test_and_verify`; `defensive_debugging` and `system_administration` for the live daemon report.
- **Code re-verified:** `serveMetrics` has only raw funnel counts, last-row snapshots, and conversion tables. `job_funnel.applied_at` and `application_attempts` provide the required authoritative sources.

## Plan

- [ ] Add a small, authoritative set of mission aggregates to `/api/metrics` with seeded Go tests.
- [ ] Render the metrics with clear unavailable states and add Vitest coverage.
- [ ] Build UI assets, run the full verification suite, check live aggregates, update backlog/history/changelog, restart services if needed, commit, and push.

## Progress Log

- 2026-08-01 21:17 EDT — Selected #491 after confirming the gate is MET, no journal is in flight, and #491's 1.5 score exceeds bug #507's 1.33. Live daemon investigation found it healthy but idle: it drained its three eligible rows and now excludes all 186 `DISCOVERED` rows via the documented breezy.hr filter (bug #482); SerpAPI quota is exhausted and Yahoo fallback has repeated EOFs. No requeue or source-policy change made.

## Next Step

Inspect the dashboard metrics query and tests, then implement the selected mission aggregate set.
