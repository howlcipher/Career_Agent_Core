# ADR-003: SQLite Concurrency and WAL Mode

## Status
Accepted

## Context
The application utilizes SQLite (`applications.db`) for tracking execution states, managing job funnels, and caching learned ATS form mappings. With up to 10 concurrent worker goroutines reading and writing to the database simultaneously, the application consistently threw `database is locked` panics.

## Decision
1. Configured the SQLite connection string to use Write-Ahead Logging (WAL) journal mode (`?_journal_mode=WAL`).
2. Enforced a strict single-connection limit (`db.SetMaxOpenConns(1)`).
3. Refactored the architecture to use a formal Repository Pattern (`pkg/storage`), ensuring raw SQL queries (`db.Exec`) are not scattered across concurrent worker files.

## Consequences
**Positive:**
- WAL mode drastically improves read performance and allows concurrent reads while a write transaction is occurring.
- `SetMaxOpenConns(1)` ensures that Go's connection pool does not attempt to open multiple blocking write connections to the local SQLite file, entirely eliminating `database is locked` panics.

**Negative:**
- Write operations are strictly serialized, meaning high-throughput writes may experience slight queueing, though negligible for this application's scale.
