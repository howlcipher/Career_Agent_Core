# Task Journal: Restore post-gate bug selection

## Summary

- **Task:** Improvement #506 — `/work_next_item` must select Minor bugs after the Usability Gate is met
- **Status:** In progress
- **Started:** 2026-08-01
- **Agent and model:** Codex / GPT-5.6; a bounded local documentation change is being completed directly. Live Antigravity and Ollama discovery were unavailable in this sandbox.

## Pre-Flight Re-Evaluation

- **Usability Gate check:** MET (2026-08-01), so Minor bugs and improvements should compete by their shared ROI score.
- **Model choice:** Codex directly; this standard-tier item changes one local workflow document, and neither optional delegate path was reachable from the sandbox.
- **Skills routed:** `technical_writing`, `test_and_verify`, `commit_and_changelog`, and continuously applicable `hallucination_guardrails`.
- **Code re-verified:** `.agents/prompts/work_next_item.md` explicitly routes a met gate straight to `improvements.md`; this confirms the starvation defect described in #506.

## Plan

- [ ] Update the canonical selection rule to merge open Minor bugs and improvements once the gate is met.
- [ ] Verify the revised prompt and backlog records, then close #506.

## Progress Log

- 2026-08-01 19:53 — Selected #506 after confirming no in-flight journal and a MET Usability Gate. Re-verified the faulty selection branch in the canonical prompt.

## Next Step

Revise the canonical selection rule, then validate and close the backlog item.
