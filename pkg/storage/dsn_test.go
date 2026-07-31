package storage

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

func TestDSNAppendsPragmasToBarePath(t *testing.T) {
	got := DSN("./applications.db")

	path, query, found := strings.Cut(got, "?")
	if !found {
		t.Fatalf("DSN(%q) = %q, want a query string", "./applications.db", got)
	}
	if path != "./applications.db" {
		t.Errorf("DSN kept path %q, want %q", path, "./applications.db")
	}

	for _, want := range []string{
		"_pragma=journal_mode(WAL)",
		"_pragma=synchronous(NORMAL)",
		"_pragma=busy_timeout(5000)",
		"_pragma=cache_size(-20000)",
		"_pragma=temp_store(MEMORY)",
	} {
		if !strings.Contains(query, want) {
			t.Errorf("DSN query %q is missing %q", query, want)
		}
	}
}

// The whole point of bug #416 and its unfixed half #446: the mattn/go-sqlite3
// spelling is accepted by modernc.org/sqlite without complaint and then
// ignored, so a DSN carrying it looks configured and is not. No DSN this
// builder returns may use it.
func TestDSNNeverUsesTheGoSqlite3PragmaSpelling(t *testing.T) {
	for _, path := range []string{DefaultDatabasePath, ":memory:", "/tmp/x.db"} {
		got := DSN(path)
		for _, forbidden := range []string{"_journal_mode=", "_busy_timeout=", "_synchronous=", "_cache_size="} {
			if strings.Contains(got, forbidden) {
				t.Errorf("DSN(%q) = %q contains go-sqlite3 spelling %q", path, got, forbidden)
			}
		}
	}
}

func TestDSNLeavesAnExplicitQueryStringAlone(t *testing.T) {
	custom := "./applications.db?_pragma=busy_timeout(1)"
	if got := DSN(custom); got != custom {
		t.Errorf("DSN(%q) = %q, want it returned unchanged", custom, got)
	}
}

// A string assertion only proves the string. This opens a real connection with
// the real driver and reads the pragmas back, which is the check that would
// have caught #446 and, run against cmd/dashboard's old literal, would have
// failed on busy_timeout.
func TestDSNPragmasTakeEffectOnALiveConnection(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pragma.db")

	handle, err := sql.Open("sqlite", DSN(path))
	if err != nil {
		t.Fatalf("open with DSN: %v", err)
	}
	defer handle.Close()
	handle.SetMaxOpenConns(1)

	var busyTimeout int
	if err := handle.QueryRow("PRAGMA busy_timeout").Scan(&busyTimeout); err != nil {
		t.Fatalf("read back busy_timeout: %v", err)
	}
	if busyTimeout != 5000 {
		t.Errorf("busy_timeout = %d, want 5000", busyTimeout)
	}

	var journalMode string
	if err := handle.QueryRow("PRAGMA journal_mode").Scan(&journalMode); err != nil {
		t.Fatalf("read back journal_mode: %v", err)
	}
	if !strings.EqualFold(journalMode, "wal") {
		t.Errorf("journal_mode = %q, want wal", journalMode)
	}
}

// The negative control for the test above. Without it, the assertion that the
// correct DSN yields busy_timeout=5000 proves nothing about whether the wrong
// one would have failed — and 5000 could have been a driver default. This
// pins the actual defect: the old spelling silently leaves the timeout at 0.
func TestGoSqlite3PragmaSpellingIsSilentlyIgnored(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")

	handle, err := sql.Open("sqlite", path+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		t.Fatalf("open with legacy DSN: %v", err)
	}
	defer handle.Close()
	handle.SetMaxOpenConns(1)

	var busyTimeout int
	if err := handle.QueryRow("PRAGMA busy_timeout").Scan(&busyTimeout); err != nil {
		t.Fatalf("read back busy_timeout: %v", err)
	}
	if busyTimeout == 5000 {
		t.Fatal("the go-sqlite3 pragma spelling now sets busy_timeout; " +
			"if the driver started honouring it, bugs #416/#446 need re-reading, " +
			"and the live-effect test above no longer distinguishes the two DSNs")
	}
	if busyTimeout != 0 {
		t.Errorf("busy_timeout = %d, want 0 (the driver default, i.e. the parameter ignored)", busyTimeout)
	}
}

func TestReaderDSNAppendsPragmasToBarePathWithoutJournalMode(t *testing.T) {
	got := ReaderDSN("./applications.db")

	path, query, found := strings.Cut(got, "?")
	if !found {
		t.Fatalf("ReaderDSN(%q) = %q, want a query string", "./applications.db", got)
	}
	if path != "./applications.db" {
		t.Errorf("ReaderDSN kept path %q, want %q", path, "./applications.db")
	}

	for _, want := range []string{
		"_pragma=synchronous(NORMAL)",
		"_pragma=busy_timeout(5000)",
		"_pragma=cache_size(-20000)",
		"_pragma=temp_store(MEMORY)",
	} {
		if !strings.Contains(query, want) {
			t.Errorf("ReaderDSN query %q is missing %q", query, want)
		}
	}

	// The one pragma a reader must never carry. Bug #450: asking to change
	// journal_mode from a connection that never owns schema setup is what
	// let a genuinely fresh database refuse a second connection outright
	// while a writer's transaction was open, and busy_timeout does not cover
	// that refusal.
	if strings.Contains(query, "_pragma=journal_mode(") {
		t.Errorf("ReaderDSN query %q must not set journal_mode", query)
	}
}

func TestReaderDSNNeverUsesTheGoSqlite3PragmaSpelling(t *testing.T) {
	for _, path := range []string{DefaultDatabasePath, ":memory:", "/tmp/x.db"} {
		got := ReaderDSN(path)
		for _, forbidden := range []string{"_journal_mode=", "_busy_timeout=", "_synchronous=", "_cache_size="} {
			if strings.Contains(got, forbidden) {
				t.Errorf("ReaderDSN(%q) = %q contains go-sqlite3 spelling %q", path, got, forbidden)
			}
		}
	}
}

func TestReaderDSNLeavesAnExplicitQueryStringAlone(t *testing.T) {
	custom := "./applications.db?_pragma=busy_timeout(1)"
	if got := ReaderDSN(custom); got != custom {
		t.Errorf("ReaderDSN(%q) = %q, want it returned unchanged", custom, got)
	}
}

// This reproduces bug #450's own experiment: a genuinely fresh, default
// (non-WAL) database with another connection holding an open write
// transaction. A second connection using the full writer DSN (which asks to
// change journal_mode) is refused outright — not merely delayed past
// busy_timeout — while ReaderDSN, which never asks to change journal_mode,
// succeeds against the exact same locked file.
//
// This is mutation-checkable: point the "reader" half at DSN instead of
// ReaderDSN and it fails with the same busy error the writer half produces.
func TestReaderDSNOpensAgainstAFreshDatabaseWithAnActiveWriterTransaction(t *testing.T) {
	path := filepath.Join(t.TempDir(), "contended.db")
	ctx := context.Background()

	// Create the file in SQLite's default (non-WAL) journal mode by opening
	// it with no pragmas at all, then hold an open write transaction on a
	// single reserved connection - the same shape as "another process
	// holding an open write transaction" in the original report.
	writerHandle, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open writer handle: %v", err)
	}
	defer writerHandle.Close()

	writerConn, err := writerHandle.Conn(ctx)
	if err != nil {
		t.Fatalf("reserve writer connection: %v", err)
	}
	defer writerConn.Close()

	if _, err := writerConn.ExecContext(ctx, "CREATE TABLE t (x INTEGER)"); err != nil {
		t.Fatalf("create table: %v", err)
	}
	if _, err := writerConn.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		t.Fatalf("begin immediate: %v", err)
	}
	defer writerConn.ExecContext(ctx, "ROLLBACK")

	// A second connection using the full writer DSN asks SQLite to change
	// journal_mode while writerConn's transaction is open, and that request
	// is refused outright.
	contendedHandle, err := sql.Open("sqlite", DSN(path))
	if err != nil {
		t.Fatalf("sql.Open with DSN: %v", err)
	}
	defer contendedHandle.Close()
	contendedHandle.SetMaxOpenConns(1)
	if err := contendedHandle.PingContext(ctx); err == nil {
		t.Fatal("DSN connection succeeded against a locked fresh database; " +
			"if SQLite (or the driver) stopped refusing a journal_mode change " +
			"here, bug #450's premise needs re-reading and this negative " +
			"control no longer proves ReaderDSN's fix does anything")
	}

	// ReaderDSN never asks to change journal_mode, so it is not exposed to
	// that refusal and opens cleanly against the same locked file.
	readerHandle, err := sql.Open("sqlite", ReaderDSN(path))
	if err != nil {
		t.Fatalf("sql.Open with ReaderDSN: %v", err)
	}
	defer readerHandle.Close()
	readerHandle.SetMaxOpenConns(1)
	if err := readerHandle.PingContext(ctx); err != nil {
		t.Fatalf("ReaderDSN connection failed against a locked fresh database: %v", err)
	}
}
