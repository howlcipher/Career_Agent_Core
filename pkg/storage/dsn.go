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

// DSN returns the connection string to hand sql.Open("sqlite", ...) for the
// database at path. Every caller in this repo must go through it so the two
// connections cannot drift apart again.
//
// A path that already carries a query string is returned untouched, so a
// caller that has deliberately chosen its own pragmas keeps them.
func DSN(path string) string {
	if strings.Contains(path, "?") {
		return path
	}
	return path + "?" + sqlitePragmas
}
