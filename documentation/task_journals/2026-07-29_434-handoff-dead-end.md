# Task Journal: Hand-off statuses are permanent dead ends (bug #434)

## Summary

- **Task:** bugs.md 434. No path moves a job out of `AWAITING_REVIEW` or `MANUAL_REQUIRED`
- **Status:** In progress
- **Started:** 2026-07-29
- **Agent and model:** Claude Code (orchestrator) / Antigravity `gemini-3.1-pro-high` (implementation)

## Pre-Flight Re-Evaluation

- **Usability Gate check:** UNMET — this bug is the reason. #434 is the open Major row holding the zero-open-Major box; working it is exactly what the gate demands.
- **User direction:** the user explicitly asked for #434 next.
- **Model choice:** `agy` live with `gemini-3.1-pro-high`. Orchestrated from Claude Code per the Working Protocol's limit-preservation rule.
- **Skills routed:** `software_development`, `database_management`, `quality_assurance`.

## Code re-verified 2026-07-29 (both halves confirmed against current code)

1. **`GetTrackedCompanies`** (`pkg/storage/manager.go:425`) selects `status IN ('APPLIED', 'INTERVIEW_REQUESTED', 'MANUAL_REQUIRED')` — `AWAITING_REVIEW` is absent, so a copilot-filled company is not in the tracker's match set at all.
2. **`updateDBWithTrackerResult`** (`pkg/tracker/imap.go:312`) queries `WHERE company_name = ? AND status = 'APPLIED'`. So even for `MANUAL_REQUIRED`, which *is* in the match set, the candidate query returns zero rows and the result is `trackerUpdateNoop`. Half-tracked is fully broken.
3. **No writer anywhere** moves a row out of either hand-off status. Confirmed by grepping every `UpdateFunnelStatus` / `UPDATE job_funnel SET status` call site.

**Net effect:** an application the user completes by hand is recorded as un-submitted forever, and the rejection or interview email it eventually produces correlates to nothing and is dropped.

## Design

Two independent halves; both are needed, neither subsumes the other.

**A. Reconcile ticked checkboxes back into the funnel.** All three queue files already use one uniform entry format, written by `LogFailedSubmission` (`manager.go:640`), `LogManualRequired` (`:716`) and `LogCopilotReview` (`:755`):

```
- [ ] **Company** - Title: [Apply Here](URL)
- [ ] **Company** - Title: [Apply Here](URL) — docs in `path/`
```

A ticked `- [x]` box is the user stating they submitted it. New `cmd/reconcile` parses the three files, and for each ticked entry promotes that URL's funnel row to `APPLIED` and records the `applied_jobs` row. **Dry-run by default, `-confirm` to write**, matching `cmd/requeue`'s established convention for anything that mutates the database.

**B. Let the tracker see hand-off rows.** Add `AWAITING_REVIEW` to `GetTrackedCompanies`, and widen the candidate query in `updateDBWithTrackerResult` to include both hand-off statuses. A real outcome email from a company is strong evidence the user submitted; recording it is more accurate than discarding the most valuable signal the system collects. Existing multi-row ambiguity handling (bugs #124/#125) is unchanged and still rolls back rather than guessing.

**Safety constraint:** the promotion must only ever move a row *out of* a hand-off status. It must never overwrite `REJECTED` / `INTERVIEW_REQUESTED` / an existing `APPLIED`, or re-open a terminal outcome.

## Plan

- [x] Re-verify both halves against current code
- [ ] Open journal, commit clean
- [ ] Delegate implementation to `gemini-3.1-pro-high`
- [ ] Review the full diff; run `go build ./...`, `go vet ./...`, `go test ./...`
- [ ] Independent review pass over the result before push
- [ ] Close #434, update the Usability Gate box, document `cmd/reconcile` in the README
- [ ] Commit, push, delete this journal

## Progress Log

- 2026-07-29 — Opened. Both halves of #434 re-verified against current code; finding 2 (the tracker's candidate query defeating even the statuses that *are* tracked) was not in the original bug write-up and is added above.

## Next Step

Delegate the two-part implementation to `gemini-3.1-pro-high`.
