package main

import (
	"strings"
	"testing"

	"github.com/howlcipher/Career_Agent_Core/pkg/storage"
)

// Bug #446: cmd/dashboard opened "./applications.db?_journal_mode=WAL" — the
// go-sqlite3 spelling modernc.org/sqlite ignores — long after pkg/storage had
// been corrected, because the two connections each carried their own literal.
// This test fails the moment anyone reintroduces a literal here.
func TestDashboardDSNMatchesStorage(t *testing.T) {
	want := storage.DSN(storage.DefaultDatabasePath)
	if dashboardDSN != want {
		t.Errorf("dashboardDSN = %q, want %q (it must be derived from pkg/storage, not written out here)", dashboardDSN, want)
	}
}

func TestDashboardDSNCarriesABusyTimeout(t *testing.T) {
	// The pragma that actually matters for this command. It reads while
	// cmd/agent writes, and without a busy timeout a query that meets a write
	// lock fails immediately; serveMetrics logs that error and returns the
	// field's zero value, so the failure renders as a plausible 0.
	if !strings.Contains(dashboardDSN, "_pragma=busy_timeout(") {
		t.Errorf("dashboardDSN = %q, want a _pragma=busy_timeout(...) entry", dashboardDSN)
	}
	for _, forbidden := range []string{"_journal_mode=", "_busy_timeout="} {
		if strings.Contains(dashboardDSN, forbidden) {
			t.Errorf("dashboardDSN = %q uses the go-sqlite3 spelling %q, which this driver ignores", dashboardDSN, forbidden)
		}
	}
}
