# Task Journal: Improvement #448 dashboard UI lint excludes committed build output

## Summary

- **Task:** #448 `npm run lint` lints the dashboard's own committed build output
- **Status:** In progress
- **Started:** 2026-08-02
- **Agent and model:** Codex / GPT-5.6

## Pre-Flight Re-Evaluation

- **Usability Gate check:** Met. `bugs.md` has no Pending rows.
- **Model choice:** Antigravity `gemini-3.6-flash-medium` is live and fits the standard-tier, one-file configuration change.
- **Skills routed:** `hallucination_guardrails`, `quality_assurance`, `test_and_verify`, and `commit_and_changelog`.
- **Code re-verified:** `cmd/dashboard/ui/.oxlintrc.json` has no ignored build-output directory; `package.json` runs bare `oxlint`; committed `dist/` exists. The backlog's diagnosis remains current.

## Plan

- [ ] Add the lint exclusion and review the delegate diff.
- [ ] Run UI and full Go verification.
- [ ] Close #448, archive its detail, and remove this journal.

## Progress Log

- 2026-08-02 — Selected #448 as the first highest-ranked eligible Pending row after confirming no journals, concurrent agent processes, or unmerged worktree branches apply.

## Next Step

Delegate the one-file oxlint configuration change, then review and verify it.
