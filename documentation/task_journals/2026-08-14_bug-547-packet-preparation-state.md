# Task Journal: bugs.md #547 — the packet omits the form's questions silently

## Summary

- **Task:** bugs.md #547 — The Copy Application Packet omits the form's questions silently, so an application nobody prepared looks fully described.
- **Status:** In progress
- **Started:** 2026-08-14
- **Agent and model:** Claude Code / Opus 5

## Pre-Flight Re-Evaluation

- **Usability Gate check:** MET (2026-08-11). #547 is a Major bug and the highest-ranked open row, so it is the correct pick regardless.
- **Model choice:** Claude Code / Opus 5, at the user's explicit instruction (the user asked for Claude subagents with the coordinator owning all edits). This overrides the Working Protocol's default of delegating standard-tier work to a non-Claude model to preserve session limits. Recorded because the four planned read-only analysis subagents (A–D) all failed immediately on a plan session limit, and the analysis was therefore done on the main thread.
- **Skills routed:** `software_development`, `frontend_engineering`, `accessibility` (packet-side action must be keyboard-operable), `quality_assurance`.
- **Code re-verified:** Yes, against the live database. 24 AWAITING_REVIEW rows, 19 on Lever, 15 of those with no `application_preflight` row. Job 308177 (the operator's real Lever application) has 0 preflight rows and 0 question rows — exactly as the row claims.

## Findings

The state model needed already existed and was simply never read on the packet path.
`application_preflight` records `state` (`inspected`/`unavailable`), a bounded `reason` from
pkg/submitter's closed vocabulary, `control_count` and `inspected_at`. Its own schema comment says
it exists to tell "we looked and found nothing" from "we could not look". `serveApplicationPacket`
never queried it.

Two traps found while designing:

1. **Inverse regression.** `pkg/storage/assisted.go:430` writes `application_questions` from a live
   assisted session with no preflight verdict. Mapping "no verdict → not prepared" would label a
   genuinely-read form as never-read.
2. **#548 conflation.** `assisted_fill_summary.recorded_at` is stamped by preparation as well as by
   filling. It must not become load-bearing for a third meaning, so `DeriveFormInventory` does not
   read that table at all.

## Plan

- [x] Trace the current architecture end to end
- [x] `storage.DeriveFormInventory` + states + staleness helper
- [x] `preflightRun` tracks job identifiers, not just a count
- [x] `form_inventory` object on the packet response
- [x] CopyPacket renders all five states, one-action Prepare
- [x] Backend tests (11 storage + 16 dashboard)
- [x] Frontend tests (15)
- [x] ADR-007 amendment, CHANGELOG
- [x] Live verification: unprepared Lever, prepared Lever, Greenhouse reference
- [x] Collateral-write check
- [ ] Independent reviews
- [x] Backlog rows and PR #26; [ ] merge, production restart

## Progress Log

- 2026-08-14 20:29 — PR #25 confirmed merged (`4727c9f`). Branch `fix/547-packet-preparation-state` cut from it.
- 2026-08-14 20:35 — Agents A–D all failed on a plan session limit; analysis done on the main thread instead.
- 2026-08-14 20:45 — Backend + frontend complete. `go build`/`vet`/`test` clean, `gofmt -l` empty, `npm run lint`/`test` (53)/`build` clean.

- 2026-08-14 20:44 — Live verification complete. Case A (293710) not_prepared -> preparing -> ready with 5 real Veeva questions; second Prepare click 409'd; bystander 293749 unaffected. Case B (293593) and Case C (303990) ready immediately. Job 228 renders `failed`/`posting_dead`. Collateral diff was exactly one preparation's writes.
- 2026-08-14 20:52 — Commits `a39492c` (fix) and `89c32a7` (docs/backlog) pushed; PR #26 opened. Three read-only reviewers running.

## Next Step

Read the three reviewer reports, fix or document every concrete finding, then merge PR #26 and restart the production dashboard on :8080 from merged main.
