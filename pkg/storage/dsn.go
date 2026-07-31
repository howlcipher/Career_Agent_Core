package storage

import "strings"

// DefaultDatabasePath is the SQLite file every command in this repo opens by
// default, relative to the working directory the binary was launched from.
const DefaultDatabasePath = "./applications.db"

// sqlitePragmas is the query string appended to a bare database path by DSN.
//
// The syntax matters: this project uses the pure Go modernc.org/sqlite driver,
// which reads pragmas as `_pragma=name(value)`. The mattn/go-sqlite3 spelling
// (`_journal_mode=WAL`, `_busy_timeout=5000`) is not recognised by this driver
// and is silently ignored rather than rejected, so a connection carrying it
// runs on driver defaults with no error anywhere. That is bug #416, and bug
// #446 is the same defect surviving in cmd/dashboard because #416 named two
// files and only one of them was changed.
//
// busy_timeout is the load-bearing one for any second reader. journal_mode is
// persisted in the database file itself, so WAL survives whichever connection
// set it, but busy_timeout is per connection: without it a query that meets a
// writer's lock fails immediately instead of waiting.
const sqlitePragmas = "_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=busy_timeout(5000)&_pragma=cache_size(-20000)&_pragma=temp_store(MEMORY)"

// readerPragmas is sqlitePragmas without journal_mode. Every other pragma
// here is a per-connection runtime setting that SQLite applies unconditionally;
// journal_mode is a database-wide property change, and SQLite refuses to
// change it while another connection holds an open write transaction -
// busy_timeout does not cover that refusal, because it is not a lock wait,
// it is an outright "cannot change mode right now" error (bug #450). A
// connection that only ever reads has no business asking to change it: on an
// already-WAL database the pragma is a no-op anyway, since journal_mode
// persists in the database file itself and survives whichever connection set
// it first.
const readerPragmas = "_pragma=synchronous(NORMAL)&_pragma=busy_timeout(5000)&_pragma=cache_size(-20000)&_pragma=temp_store(MEMORY)"

// DSN returns the connection string to hand sql.Open("sqlite", ...) for the
// database at path, for the connection that owns writes and schema setup
// (storage.InitDBWithPath, and therefore every command that calls it). Every
// writer caller in this repo must go through it so its connections cannot
// drift apart again.
//
// A path that already carries a query string is returned untouched, so a
// caller that has deliberately chosen its own pragmas keeps them.
func DSN(path string) string {
	if strings.Contains(path, "?") {
		return path
	}
	return path + "?" + sqlitePragmas
}

// ReaderDSN returns the connection string for a connection that only ever
// queries the database and never owns schema setup - currently cmd/dashboard,
// which opens its own handle rather than going through storage.InitDB. It
// carries every pragma DSN does except journal_mode, so it cannot be the
// connection that fails bug #450's SQLITE_BUSY on a genuinely fresh database:
// a designated writer using DSN is expected to reach the database first (or
// at least without a reader holding it open at the same moment) and set
// journal_mode once, and every later connection - reader or writer - then
// finds it already WAL.
//
// The same query-string escape hatch as DSN applies: a path that already
// carries a query string is returned untouched.
func ReaderDSN(path string) string {
	if strings.Contains(path, "?") {
		return path
	}
	return path + "?" + readerPragmas
}
