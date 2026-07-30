package storage

import (
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
