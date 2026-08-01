package modelbench

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTakeHostSnapshot_ParsesRealMeminfo(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "meminfo")
	content := "MemTotal:       32899548 kB\n" +
		"MemAvailable:   23920128 kB\n" +
		"SwapTotal:      15000000 kB\n" +
		"SwapFree:       12000000 kB\n" +
		"Cached:          5000000 kB\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	snap := takeHostSnapshotAt(path)
	if snap.MemTotalKB == nil || *snap.MemTotalKB != 32899548 {
		t.Errorf("MemTotalKB = %v, want 32899548", snap.MemTotalKB)
	}
	if snap.MemAvailableKB == nil || *snap.MemAvailableKB != 23920128 {
		t.Errorf("MemAvailableKB = %v, want 23920128", snap.MemAvailableKB)
	}
	if snap.SwapTotalKB == nil || *snap.SwapTotalKB != 15000000 {
		t.Errorf("SwapTotalKB = %v, want 15000000", snap.SwapTotalKB)
	}
	if snap.SwapFreeKB == nil || *snap.SwapFreeKB != 12000000 {
		t.Errorf("SwapFreeKB = %v, want 12000000", snap.SwapFreeKB)
	}
}

// TestTakeHostSnapshot_GracefulWhenUnavailable is the "optional Linux memory
// metrics missing" case: a non-Linux host, a sandboxed environment, or
// missing permissions must never fail a benchmark run -- it must just come
// back with every field nil.
func TestTakeHostSnapshot_GracefulWhenUnavailable(t *testing.T) {
	snap := takeHostSnapshotAt(filepath.Join(t.TempDir(), "does-not-exist"))
	if snap.MemAvailableKB != nil || snap.MemTotalKB != nil || snap.SwapTotalKB != nil || snap.SwapFreeKB != nil {
		t.Errorf("expected an all-nil snapshot for a missing meminfo file, got %+v", snap)
	}
}

func TestTakeHostSnapshot_MissingFieldsStayNilNotZero(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "meminfo")
	// No SwapTotal/SwapFree lines at all -- a host with swap disabled.
	if err := os.WriteFile(path, []byte("MemTotal: 1000 kB\nMemAvailable: 500 kB\n"), 0644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	snap := takeHostSnapshotAt(path)
	if snap.SwapTotalKB != nil || snap.SwapFreeKB != nil {
		t.Errorf("expected nil swap fields when absent from meminfo, got %+v / %+v", snap.SwapTotalKB, snap.SwapFreeKB)
	}
	if snap.MemTotalKB == nil || *snap.MemTotalKB != 1000 {
		t.Errorf("MemTotalKB = %v, want 1000", snap.MemTotalKB)
	}
}
