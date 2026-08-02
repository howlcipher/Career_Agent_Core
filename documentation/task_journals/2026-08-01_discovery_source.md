# Task Journal: Persist discovery source

## Summary

- **Task:** Improvement #499 — Persist `discovery_source` at `AddToFunnel` time
- **Status:** In progress
- **Started:** 2026-08-01
- **Agent and model:** Codex / GPT-5.6 Terra

## Pre-Flight Re-Evaluation

- **Usability Gate check:** MET (2026-08-01); improvement work is eligible.
- **Model choice:** GPT-5.6 Terra orchestrates this standard-tier schema and plumbing task. Live alternatives include Antigravity Gemini and local Ollama (`qwen3:30b-instruct`); delegation is unnecessary for this bounded change.
- **Skills routed:** `hallucination_guardrails`, `software_development`, `database_management`, `quality_assurance`, `test_and_verify`, `architectural_guardrails`, and `technical_writing`.
- **Code re-verified:** `job_funnel` lacks `discovery_source`; five discovery paths write through `storage.AddToFunnel`, confirming the backlog item remains current.

## Plan

- [x] Add an idempotent schema migration and source-aware storage insertion.
- [x] Thread validated source values through every discovery path with focused tests.
- [ ] Rebuild/restart the running agent, verify fresh source data, update backlog, commit, and push.

## Progress Log

- 2026-08-01 — Selected highest-ranked eligible free item #499 (score 1.33). The live agent daemon is running and has no current eligible-count evidence because `sqlite3` is not installed; the dashboard remains available on `127.0.0.1:8080`.
- 2026-08-01 — Added the nullable `job_funnel.discovery_source` migration and threaded `remoteok`, `hackernews`, `atsfeed:greenhouse`, `atsfeed:lever`, `serpapi`, and `yahoo` through live discovery. Focused storage/scraper tests and the full build, vet, test, and formatting loop pass.
- 2026-08-01 — Rebuilt the dashboard-managed `career_agent_bin` and restarted it through `/api/agent/start`; PID 1684288 is supervised by the dashboard. The first daemon queue cycle confirmed zero eligible rows while independent discovery began.

## Next Step

Allow the first discovery refresh to complete, record the live queue result, then close #499 and push the final commit.
