# Task Journal: bugs.md #548 — preparing an application stamps a fill outcome

## Summary

- **Task:** bugs.md #548 — "Preparing an application stamps a fill outcome, so the card reports a fill that never ran"
- **Status:** In progress
- **Started:** 2026-08-14
- **Agent and model:** Claude Code / Opus 5

## Pre-Flight Re-Evaluation

- **Usability Gate check:** Not met. The "Zero open Blocker or Major bugs" box is unchecked precisely
  because of #548, which is the only Pending **Major** row (the other five Pending rows — #549, #524,
  #532, #526, #528 — are all Minor). A bug fix is therefore the correct work.
- **Model choice:** Claude Code / Opus 5. Tier is `standard`, but the user explicitly directed this
  session to use Claude with multiple read-only auditor agents, which overrides the Working
  Protocol's default delegation-to-a-non-Claude-model guidance for this task.
- **Skills routed:** `software_development`, `database_management` (additive SQLite migration),
  `quality_assurance` (the 15 required backend cases), `frontend_engineering` (the card states).
- **Code re-verified:** Yes — every claim in the row was checked against the current tree and the
  live database. All confirmed, plus two the row did not know about (below).

## Findings beyond the filed row

Four read-only audits (write-provenance, state model/migration, historical adversary, UX) ran before
any edit. What they added to the row:

1. **A fill that errors writes nothing at all.** `cmd/assist/main.go:684-689` records `manual_review`
   and returns — no summary row. A real fill attempt is currently invisible when it fails.
2. **Preparation does not merely stamp fill state, it erases it.** The upsert at
   `pkg/storage/questions.go:322` sets `filled_count = excluded.filled_count`, and `cmd/preflight`
   passes a zero-value summary — so re-preparing an already-filled application resets its real
   `filled_count` to 0. No test calls `ReplaceApplicationQuestions` twice with different summaries,
   which is why it went unseen. This is in scope: without it, the new marker plus zeroed counts would
   make the card say "attempted but could not fill any fields" about a fill that filled eight.
3. **A real fill can legitimately record `filled_count = 0`** — a fill that resolves nothing, or one
   whose post-pass `SnapshotControls` fails (`pkg/submitter/browser.go:4250-4256`). Such a row is
   byte-identical to preflight's. `filled_count > 0` therefore cannot become the attempt marker.
4. **Automatic Apply never writes this table at all.** It shares the ATS handlers but not the
   reporting layer, and records its own evidence in `application_attempts` (keyed by URL).
5. **Not every Continue runs a fill** — the Workable direct-browser path
   (`cmd/assist/main.go:308-311`) records `manual_review` and never fills.

## Live database evidence (read-only)

- 11 `assisted_fill_summary` rows; **every one** `filled_count=0, reused_answers=0, documents='',
  filled_labels=''`.
- `application_preflight` holds 11 `inspected` verdicts; the job set is **identical, 1:1**, to the 11
  summary rows, with timestamps in two batch clusters ~8s apart — the signature of `cmd/preflight`'s
  loop. Every existing row is fully explained by preparation.
- `human_interactions`: 0 rows. `application_questions`: 95 rows, all `pending`.
- Job 310026: `filled_count=0`, `recorded_at=2026-08-14 01:33:58`, paired with its own preflight
  verdict (`inspected`, `control_count=21`) at `01:33:50`.

**No durable evidence that any fill has ever been attempted exists in this database.**

## Plan

- [ ] Storage: `fill_attempted_at` column + migration; split `RecordPreparedQuestions` from
      `ReplaceApplicationQuestions`; `MarkFillAttempted`; extend `AssistedFillSummary`/`GetFillSummary`
- [ ] Wire `cmd/preflight` and `cmd/assist`
- [ ] Backend tests (15 required cases)
- [ ] Frontend: four card states + `CompletedSummary.test.tsx`; rebuild `ui/dist`
- [ ] Verification: synthetic real fill, read-only job 310026 regression, full loop
- [ ] Three independent read-only reviewers
- [ ] Backlog/docs/ADR-007/Usability Gate, PR, merge

## Progress Log

- 2026-08-14 — Branch `fix/548-fill-attempt-provenance` cut from `main` at `2d4ad91`. Four read-only
  audits complete; plan approved. Operator chose the minimal card copy (no form-inventory plumbing
  onto the card) and declined the optional live employer run — synthetic proof plus the read-only
  310026 regression are the evidence of record.

## Next Step

Implement the storage layer in `pkg/storage/questions.go`.
