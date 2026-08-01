package modelbench

import (
	"bufio"
	"os"
	"strconv"
	"strings"
)

// HostSnapshot is a best-effort point-in-time read of host memory pressure.
// Every field is a pointer so "not available on this platform" (nil) is
// distinguishable from "measured as zero". meminfoPath is only ever
// overridden by tests.
type HostSnapshot struct {
	MemAvailableKB *uint64
	MemTotalKB     *uint64
	SwapTotalKB    *uint64
	SwapFreeKB     *uint64
}

const defaultMeminfoPath = "/proc/meminfo"

// TakeHostSnapshot reads /proc/meminfo. On any platform or condition where
// that file is unavailable (non-Linux, a sandboxed environment, permissions),
// it returns a zero-value snapshot with every field nil and no error --
// optional host metrics must never fail a benchmark run.
func TakeHostSnapshot() HostSnapshot {
	return takeHostSnapshotAt(defaultMeminfoPath)
}

func takeHostSnapshotAt(path string) HostSnapshot {
	f, err := os.Open(path)
	if err != nil {
		return HostSnapshot{}
	}
	defer f.Close()

	values := make(map[string]uint64)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}
		key := strings.TrimSuffix(parts[0], ":")
		n, err := strconv.ParseUint(parts[1], 10, 64)
		if err != nil {
			continue
		}
		values[key] = n
	}

	snap := HostSnapshot{}
	if v, ok := values["MemAvailable"]; ok {
		snap.MemAvailableKB = &v
	}
	if v, ok := values["MemTotal"]; ok {
		snap.MemTotalKB = &v
	}
	if v, ok := values["SwapTotal"]; ok {
		snap.SwapTotalKB = &v
	}
	if v, ok := values["SwapFree"]; ok {
		snap.SwapFreeKB = &v
	}
	return snap
}
