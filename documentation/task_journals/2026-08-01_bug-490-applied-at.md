# Task Journal: Record application confirmation timestamps

## Summary

- **Task:** Bug #490 — `job_funnel.applied_at` is declared but never written
- **Status:** In progress
- **Started:** 2026-08-01
- **Agent and model:** Codex / GPT-5.6; mechanical, bounded storage change implemented inline. Live availability checked: Antigravity offers `gemini-3.6-flash-high`; Ollama offers `qwen3:4b-instruct`.

## Pre-Flight Re-Evaluation

- **Usability Gate check:** MET (2026-08-01); #490 is the first table-ordered item tied for the top combined score (1.5).
- **Model choice:** Inline implementation is lower overhead than delegation for the two-statement, test-backed mechanical fix; live tier availability was still verified.
- **Skills routed:** `hallucination_guardrails`, `systems_logic`, `software_development`, and `test_and_verify` from `../ai_knowledge_library/.agents/skills/`.
- **Code re-verified:** `pkg/storage/manager.go` has exactly two `job_funnel` APPLIED writers: `UpdateFunnelStatus` and `MarkHandoffApplied`; neither currently writes `applied_at`.

## Plan

- [x] Update both APPLIED writes to set the same canonical UTC timestamp as `last_updated`.
- [x] Add isolated regression coverage for both APPLIED paths and non-APPLIED transitions.
- [ ] Run the full Go verification loop, update backlog history/status, commit, and push.

## Progress Log

- 2026-08-01 — Selected #490 after checking for in-flight journals, the MET usability gate, queue scores, and live model availability.
- 2026-08-01 — Implemented the two timestamp writes and a focused storage regression test. `go test ./pkg/storage` passes.

## Next Step

Review the focused diff, commit this implementation milestone, then run the full repository verification loop.
