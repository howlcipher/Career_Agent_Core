# Task Journal: Bug #482 excluded Breezy rows never terminalize

## Summary

- **Task:** Bug #482 — Breezy postings excluded from automation accumulate in `DISCOVERED` forever.
- **Status:** In progress
- **Started:** 2026-08-01
- **Agent and model:** Codex direct implementation. The item is standard tier; live Antigravity and local Ollama models are available, but this is a small storage and startup-boundary change that remains under direct review.

## Pre-Flight Re-Evaluation

- **Usability Gate check:** MET; this is the highest remaining Pending bug at score 1.33.
- **Skills routed:** `software_development`, `database_management`, `test_and_verify`, and `hallucination_guardrails`.
- **Code re-verified:** `GetDiscoveredJobs` and `GetJobsMissingFitSimilarity` exclude `breezy.hr` in SQL, leaving excluded rows at `DISCOVERED`. The live dashboard reports 185 discovered and zero eligible rows.

## Plan

- [ ] Terminalize newly discovered excluded-source rows without routing them to workers.
- [ ] Sweep existing excluded `DISCOVERED` rows during agent startup.
- [ ] Add storage and dashboard-reason regression coverage, verify, rebuild/restart, and push.

## Progress Log

- 2026-08-01 21:35 EDT — Selected option: a terminal boundary plus startup sweep. It clears existing rows and prevents future queue pollution; a future-only insertion filter would leave the live backlog intact.

## Next Step

Implement and test the storage boundary and agent-startup sweep.
