# Task Journal: Bug #446 — the dashboard's own DB connection still uses the pragma syntax #416 was closed for fixing

## Summary

- **Task:** bug #446 — `cmd/dashboard/main.go` opens `"./applications.db?_journal_mode=WAL"`, the `go-sqlite3` syntax `modernc.org/sqlite` silently ignores, while `pkg/storage/manager.go` builds the correct `_pragma=` DSN. #416 named both files and only one was fixed.
- **Status:** In progress
- **Started:** 2026-07-30
- **Agent and model:** Claude Code / Opus 5 (orchestrator), with parallel Claude review agents for the data-path review and the documentation audit

## Pre-Flight Re-Evaluation

- **Usability Gate check:** MET (2026-07-30, third session). With the gate met, selection is normal ROI ranking across `bugs.md` and `improvements.md`. #446 scores 4.0, the highest open row anywhere in the free queue, so it is the correct pick regardless of the gate.
- **Model choice:** Claude Opus 5 for orchestration per the user's explicit "prioritize claude models on this run". The fix itself is one line plus tests — smaller than the cost of writing a self-contained delegation brief — so it is done in-session rather than delegated. The two parallel agents are where the delegation budget went.
- **Skills routed:** `database_management` (DSN/pragma correctness, busy timeout), `defensive_debugging`, `test_and_verify`.
- **Code re-verified:** yes, both halves read live before writing anything. `cmd/dashboard/main.go:333` is `db, err = sql.Open("sqlite", "./applications.db?_journal_mode=WAL")`. `pkg/storage/manager.go:42` builds `?_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=busy_timeout(5000)&_pragma=cache_size(-20000)&_pragma=temp_store(MEMORY)` and opens it at `:44`. The row's stale line number (`:247`) was the only thing that had drifted; the defect is exactly as filed.

## Plan

- [ ] Extract the DSN construction into an exported, tested builder in `pkg/storage` so the two call sites cannot drift again (the bug's own stated fix direction).
- [ ] Point `pkg/storage/manager.go` at it, unchanged in behaviour.
- [ ] Point `cmd/dashboard/main.go` at it, which is the actual fix.
- [ ] Tests: unit tests for the builder, plus a test that pins the dashboard's DSN to the shared builder so a future edit cannot silently re-fork it.
- [ ] Verify the pragma actually takes effect on a live connection (`PRAGMA busy_timeout` read back), not just that the string changed.
- [ ] `go build ./...`, `go vet ./...`, `go test ./...`, `gofmt -l`.

## Progress Log

- 2026-07-30 — Journal opened. Item selected and re-verified against live code. Two parallel Claude review agents launched: one read-only defect review of the dashboard/storage data path, one read-only README/GitHub Pages accuracy audit.

## Next Step

Add the shared DSN builder to `pkg/storage` and repoint both call sites.
