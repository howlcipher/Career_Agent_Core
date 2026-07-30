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

- 2026-07-30 — Journal opened. Item selected and re-verified against live code. Two parallel Claude review agents launched: one read-only defect review of the dashboard/storage data path, one read-only README/GitHub Pages accuracy audit. **Both died on an Anthropic session limit before producing output**, so their work is being done in this session instead.
- 2026-07-30 — Fix shipped. `pkg/storage/dsn.go` adds `DSN(path)` and `DefaultDatabasePath`; `manager.go` and `cmd/dashboard/main.go` both go through it, and `cmd/dashboard` holds the result in a package-level `dashboardDSN` so a test can pin it. 7 new tests. Build, vet, and all 12 test-bearing packages green; `gofmt -l` clean on both touched dirs.
- 2026-07-30 — Mutation check: reverting `dashboardDSN` to the old literal fails all three assertions in `cmd/dashboard/dsn_test.go` with the expected messages. The tests are load-bearing.

### Live verification, and what it actually showed

Verified against real binaries, not tests alone. **The first probe disproved the bug report's stated mechanism**, which is the more useful half of this run.

1. **Contention on the real database: no difference.** A copy of the real `applications.db` (9,710 discovered rows) served by a pre-fix binary and a post-fix binary on `127.0.0.1:8097`/`:8098`, with a separate process holding an open write transaction. Both returned byte-identical metrics, and neither logged a single lock or busy error. **The reason is that the database is already in WAL mode, where readers never block on a writer at all.** The bug's claim that "a query that meets a write lock fails immediately" does not hold for the deployed configuration.

2. **The fresh-database case is where the two binaries genuinely differ.** Each binary was run in an empty directory and asked for `/api/metrics`, so it created `applications.db` itself:
   - pre-fix: `journal_mode=delete`, no `-wal`/`-shm` files.
   - post-fix: `journal_mode=wal`, with `-wal` and `-shm` present.

   So the real consequence of #446 was never a lost read on this host — it was that a dashboard which reached a new database first left it in rollback-journal mode, silently downgrading the durability/concurrency setting that `cmd/agent` then had to convert.

3. **A reader with no busy timeout is still the wrong default**, confirmed separately: a live connection opened with `storage.DSN` reads back `busy_timeout=5000`, and one opened with the old `?_journal_mode=WAL&_busy_timeout=5000` spelling reads back `0`. That pair is now `TestDSNPragmasTakeEffectOnALiveConnection` and its negative control.

4. **Finding, filed rather than fixed here:** in a `delete`-mode database with a writer holding a lock, a *new* connection using the shared DSN fails outright with `SQLITE_BUSY`, because SQLite refuses a `journal_mode` change while another connection is active — and `busy_timeout` does **not** cover it. Tested both pragma orderings (`journal_mode` first and `busy_timeout` first); both fail identically, so ordering is not the answer. This is pre-existing behaviour of `pkg/storage`'s DSN, not something this change introduced, and it is unreachable once the database is WAL. Filed as an improvement row rather than expanded into this fix.

## Next Step

Update the backlogs (close #446, file the finding above), audit README/GitHub Pages, groom, commit and push.
