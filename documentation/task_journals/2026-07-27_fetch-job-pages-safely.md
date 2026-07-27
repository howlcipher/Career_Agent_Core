# Task Journal: Fetch job pages safely before scoring

## Summary

- **Task:** Bug #123, failed and non-2xx job-page fetches still proceed to expensive fit scoring.
- **Status:** In progress
- **Started:** 2026-07-27
- **Agent and model:** Codex / GPT-5, working inline because the fetch policy, worker integration, tests, and backlog closure are tightly coupled.

## Pre-Flight Re-Evaluation

- **Usability Gate check:** UNMET. Seven Major or Blocker bugs remain, so bug #123 outranks every free improvement.
- **Model choice:** `agy models` confirms the recommended `gemini-3.6-flash-high` is available, and the local Ollama endpoint lists all required models. The current Codex session is handling this medium, cohesive refactor inline.
- **Skills routed:** `hallucination_guardrails`, `software_development`, `quality_assurance`, `test_and_verify`, `systems_logic`, `technical_writing`, `cyber_security`, and `commit_and_changelog`.
- **Code re-verified:** `cmd/agent/main.go` ignores transport failures after logging, accepts every HTTP status as job content, defers response closure inside the worker loop, and can call embedding and scoring with only a title.

## Plan

- [ ] Add failing tests for usable 2xx content, weak 2xx content, terminal 404 and 410 responses, retryable transport, 429, and 5xx failures, bounded waits, response closure, and cancellation.
- [ ] Extract an injected, context-aware fetch helper and integrate its dispositions into the worker.
- [ ] Check every affected funnel status write and preserve the existing CAPTCHA handling.
- [ ] Update README, CHANGELOG, bug #123, and the monitoring journal with durable behavior and verification evidence.
- [ ] Run focused tests, the required full Go verification loop, security and PII review, then delete this journal in the final signed commit.

## Progress Log

- 2026-07-27 — Confirmed the worktree is clean and synchronized with `origin/main`. Re-verified the bug against `cmd/agent/main.go`, checked live model availability, and selected a focused helper over a broader scraper-package API.

## Next Step

Write the focused failing fetch-policy tests in `cmd/agent/main_test.go`.
