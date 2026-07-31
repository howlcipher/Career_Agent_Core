package main

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
)

// Bug #449: cmd/dashboard identified a running agent with `pgrep -f
// career_agent_bin`, a substring match against every process's full command
// line, which false-positived on a `go build`, a `tail -f`, or an editor with
// the file open. The fix has the agent write its own PID into the
// single-instance lock file (bug #414) it already holds for the whole run, so
// the dashboard can read back a real PID instead of guessing from a process
// list. These tests cover the write side; cmd/dashboard/agent_lock_test.go
// covers the read side against the same file format.

func TestAcquireSingleInstanceLockWritesOwnPID(t *testing.T) {
	lockPath := filepath.Join(t.TempDir(), "career_agent.lock")

	f, err := acquireSingleInstanceLock(lockPath)
	if err != nil {
		t.Fatalf("acquireSingleInstanceLock returned error: %v", err)
	}
	defer func() {
		syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		f.Close()
	}()

	data, err := os.ReadFile(lockPath)
	if err != nil {
		t.Fatalf("failed to read lock file: %v", err)
	}
	gotPID, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		t.Fatalf("lock file content %q did not parse as a PID: %v", data, err)
	}
	if gotPID != os.Getpid() {
		t.Errorf("lock file PID = %d, want this process's PID %d", gotPID, os.Getpid())
	}
}

func TestAcquireSingleInstanceLockFailsWhenAlreadyHeld(t *testing.T) {
	lockPath := filepath.Join(t.TempDir(), "career_agent.lock")

	first, err := acquireSingleInstanceLock(lockPath)
	if err != nil {
		t.Fatalf("first acquireSingleInstanceLock returned error: %v", err)
	}
	defer func() {
		syscall.Flock(int(first.Fd()), syscall.LOCK_UN)
		first.Close()
	}()

	if _, err := acquireSingleInstanceLock(lockPath); err == nil {
		t.Fatal("second acquireSingleInstanceLock succeeded while the first instance still held the lock")
	}
}

func TestAcquireSingleInstanceLockTruncatesAPreviousLongerPID(t *testing.T) {
	lockPath := filepath.Join(t.TempDir(), "career_agent.lock")

	// Simulate a prior holder whose PID happened to have more digits than
	// this process's, so a naive overwrite-without-truncate would leave
	// trailing digits from the old value glued onto the new one.
	if err := os.WriteFile(lockPath, []byte("999999999999"), 0666); err != nil {
		t.Fatalf("failed to seed lock file: %v", err)
	}

	f, err := acquireSingleInstanceLock(lockPath)
	if err != nil {
		t.Fatalf("acquireSingleInstanceLock returned error: %v", err)
	}
	defer func() {
		syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		f.Close()
	}()

	data, err := os.ReadFile(lockPath)
	if err != nil {
		t.Fatalf("failed to read lock file: %v", err)
	}
	if strconv.Itoa(os.Getpid()) != string(data) {
		t.Errorf("lock file content = %q, want exactly %q (no trailing digits from the previous holder)", data, strconv.Itoa(os.Getpid()))
	}
}
