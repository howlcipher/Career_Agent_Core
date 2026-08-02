# Task Journal: Measure whether the NLP offload is worth keeping

## Summary

- **Task:** Improvement #442, Measure whether the NLP offload is worth keeping
- **Status:** In progress
- **Started:** 2026-08-02
- **Agent and model:** Codex orchestrator; Gemini 3.6 Flash High review delegate; live Ollama qwen3:4b-instruct for route measurements

## Pre-Flight Re-Evaluation

- **Usability Gate check:** Met. `bugs.md` has no Pending rows, and its current gate note records clean static checks.
- **Model choice:** Gemini 3.6 Flash High is live through Antigravity and matches this standard tier review. Ollama has qwen3:4b-instruct and qwen3:30b-instruct available for the live routing measurements.
- **Skills routed:** `hallucination_guardrails`, `work_next_item`, `groom_backlogs`, `software_development`, `test_and_verify`, and `commit_and_changelog`.
- **Code re-verified:** `NLP_SERVICE_URL` still enables only the health-checked Ollama microservice path in `pkg/mcp/client.go`; the service and fallback still exist. The current verifier exercises one synthetic job only, so repeated route measurements are required before deciding whether the extra service is justified.

## Plan

- [x] Confirm selection, current code, and available models.
- [ ] Collect comparable repeated in-process and offloaded measurements.
- [ ] Decide, implement the justified outcome, verify, and close the backlog item.

## Progress Log

- 2026-08-02 — Selected #442 after confirming a clean tree, no resume journal or worktree, no running peer agent process, a met Usability Gate, and no higher-priority free Pending item.

## Next Step

Obtain a delegate review of the current benchmark plan, then run repeated live measurements on both routes.
