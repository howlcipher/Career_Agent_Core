# ADR-003: SQLite Concurrency and WAL Mode

## Status
Accepted

## Context
The application utilizes SQLite (`applications.db`) for tracking execution states, managing job funnels, and caching learned ATS form mappings. With up to 10 concurrent worker goroutines reading and writing to the database simultaneously, the application consistently threw `database is locked` panics.

## Decision
1. Configured the SQLite connection string to use Write-Ahead Logging (WAL) journal mode, plus `synchronous(NORMAL)`, a `busy_timeout(5000)`, a `cache_size(-20000)`, and `temp_store(MEMORY)`: `?_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=busy_timeout(5000)&_pragma=cache_size(-20000)&_pragma=temp_store(MEMORY)` (`pkg/storage/manager.go:42`). The `_pragma=name(value)` form is required by `modernc.org/sqlite`, the pure Go driver this project uses; it replaces the `?_journal_mode=WAL`-style query parameters the older `go-sqlite3` driver accepted.
2. Allowed a bounded connection pool rather than a single connection: `db.SetMaxOpenConns(10)` and `db.SetMaxIdleConns(5)` (`pkg/storage/manager.go:49-50`). `cmd/dashboard/main.go:338` opens its own independent connection to the same database file and likewise calls `db.SetMaxOpenConns(10)`. Lock contention between connections is absorbed by the `busy_timeout(5000)` pragma above: a connection that finds the database locked waits up to 5 seconds for the lock to clear instead of failing immediately.
3. Refactored the architecture to use a formal Repository Pattern (`pkg/storage`), ensuring raw SQL queries (`db.Exec`) are not scattered across concurrent worker files.

## Consequences
**Positive:**
- WAL mode drastically improves read performance and allows concurrent reads while a write transaction is occurring.
- The `busy_timeout(5000)` pragma lets concurrent connections queue for a lock instead of failing with `database is locked`, so a bounded pool of up to 10 connections (`pkg/storage`) can share the same file without serializing through a single connection.

**Known gap, not a decision:** the pragmas above are per-connection, and only `pkg/storage` applies them. `cmd/dashboard/main.go` opens the same file with the older `?_journal_mode=WAL` query-parameter form, which `modernc.org/sqlite` silently ignores, so the dashboard's connection runs on driver defaults with **no busy timeout** and fails immediately on a lock rather than waiting. Journal mode itself is unaffected, because WAL is a property persisted in the database file and `pkg/storage` has already set it. This is tracked as bug #446 and is a defect to fix, not part of this decision.

**Negative:**
- Concurrent writers can still queue for up to the 5 second `busy_timeout` under sustained contention; a write that cannot acquire the lock within that window still fails, though this has not been observed at this application's scale.
- Two independent connection pools now exist against the same file (`pkg/storage` and `cmd/dashboard`), each with its own `SetMaxOpenConns`. Nothing coordinates their combined connection count, so the total number of open connections to `applications.db` is the sum of both.

## Superseded Decisions
- **2026-07-30:** Item 2 originally read "enforced a strict single-connection limit (`db.SetMaxOpenConns(1)`)." That is no longer what the code does. `pkg/storage/manager.go` sets `SetMaxOpenConns(10)` and `SetMaxIdleConns(5)`, and `cmd/dashboard/main.go` opens its own separate connection with `SetMaxOpenConns(10)`. Concurrency safety today comes from WAL mode plus the `busy_timeout(5000)` pragma absorbing lock contention, not from serializing all access through one connection. This section is left in place, rather than deleted, to record that the decision changed and why.
