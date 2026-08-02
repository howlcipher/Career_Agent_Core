# Task Journal: Bug #502 encoding-attack evidence

## Summary

- **Task:** Bug #502 — `encoding.go`'s homoglyph branch is a third zero-evidence heuristic threat source outside #489's fix window.
- **Status:** In progress
- **Started:** 2026-08-01
- **Agent and model:** Codex orchestrator; planned implementation delegate: Antigravity Gemini 3.6 Flash High.

## Pre-Flight Re-Evaluation

- **Usability Gate check:** Met (2026-08-01); #502 is the highest-ranked eligible pending bug (score 1.0).
- **Model choice:** Gemini 3.6 Flash High is live through Antigravity and suits this standard-tier, security-sensitive change; local Ollama is also live for bounded review.
- **Skills routed:** `hallucination_guardrails`, `software_development`, `cyber_security`, `quality_assurance`, and `test_and_verify`.
- **Code re-verified:** `pkg/security/filter.go` deliberately keeps `ThreatEncodingAttack` outside #489's override; its 0.7 heuristic homoglyph threat is unlocated. The audit log and current corpus exist and must be measured before extending the override.

## Plan

- [ ] Measure encoding-attack audit rows and correlate only sanitized aggregate characteristics.
- [ ] Decide whether the evidence supports a narrow release rule; delegate any implementation and regression tests.
- [ ] Review the diff, run the full Go verification loop, safely rebuild/restart if needed, update backlog history, commit, and push.

## Progress Log

- 2026-08-01 — Selected #502 after the gate check. `git fetch` found `origin/main` at the current branch tip; two stale Antigravity research worktrees were clean and unrelated.

## Next Step

Measure the current prompt-injection audit data without printing raw payload content, then decide whether implementation is warranted.
