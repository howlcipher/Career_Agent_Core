package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"
)

// Bug #449: serveAgentStart/Stop/Status identified the agent by running
// `pgrep -f career_agent_bin`, which matches any process whose full command
// line merely contains that substring - a `go build -o career_agent_bin`, a
// `tail -f career_agent_bin.log`, an editor with the file open - and whose
// `pkill -f` counterpart then killed whichever unrelated process it hit. The
// fix reuses bug #414's single-instance flock on applications/career_agent.lock
// instead: agentPIDAt is exercised here directly against a temp lock path so
// these tests never touch the repo's real applications/ directory.
//
// All tests below open the lock file themselves to simulate an agent process
// holding it, rather than forking a real career_agent_bin - flock locks are
// per open-file-description, not per-process, so a second os.OpenFile call in
// this same test process (exactly what agentPIDAt does) genuinely contends
// for the lock the same way a separate process would.

func TestAgentPIDAt_LockFreeReportsNotRunning(t *testing.T) {
	lockPath := filepath.Join(t.TempDir(), "career_agent.lock")

	pid, running, err := agentPIDAt(lockPath)
	if err != nil {
		t.Fatalf("agentPIDAt returned error: %v", err)
	}
	if running {
		t.Fatal("agentPIDAt reported running with no holder")
	}
	if pid != 0 {
		t.Errorf("pid = %d, want 0", pid)
	}
}

func TestAgentPIDAt_LockHeldReportsRunningWithPID(t *testing.T) {
	lockPath := filepath.Join(t.TempDir(), "career_agent.lock")

	holder, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0666)
	if err != nil {
		t.Fatalf("failed to create lock file: %v", err)
	}
	defer holder.Close()
	if err := syscall.Flock(int(holder.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		t.Fatalf("failed to acquire lock: %v", err)
	}
	defer syscall.Flock(int(holder.Fd()), syscall.LOCK_UN)

	const wantPID = 424242
	if _, err := holder.WriteAt([]byte(strconv.Itoa(wantPID)), 0); err != nil {
		t.Fatalf("failed to write pid: %v", err)
	}

	pid, running, err := agentPIDAt(lockPath)
	if err != nil {
		t.Fatalf("agentPIDAt returned error: %v", err)
	}
	if !running {
		t.Fatal("agentPIDAt reported not running while the lock was held")
	}
	if pid != wantPID {
		t.Errorf("pid = %d, want %d", pid, wantPID)
	}
}

func TestAgentPIDAt_UnparsablePIDStillReportsRunning(t *testing.T) {
	lockPath := filepath.Join(t.TempDir(), "career_agent.lock")

	// A lock file held with no readable PID (e.g. a pre-fix agent binary that
	// never wrote one) must still report running - the caller just has
	// nothing safe to signal.
	holder, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0666)
	if err != nil {
		t.Fatalf("failed to create lock file: %v", err)
	}
	defer holder.Close()
	if err := syscall.Flock(int(holder.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		t.Fatalf("failed to acquire lock: %v", err)
	}
	defer syscall.Flock(int(holder.Fd()), syscall.LOCK_UN)

	pid, running, err := agentPIDAt(lockPath)
	if err != nil {
		t.Fatalf("agentPIDAt returned error: %v", err)
	}
	if !running {
		t.Fatal("agentPIDAt reported not running while the lock was held")
	}
	if pid != 0 {
		t.Errorf("pid = %d, want 0 (unknown) for an empty lock file", pid)
	}
}

func TestAgentPIDAt_StatusCheckDoesNotHoldTheLock(t *testing.T) {
	lockPath := filepath.Join(t.TempDir(), "career_agent.lock")

	if _, running, err := agentPIDAt(lockPath); err != nil || running {
		t.Fatalf("unexpected initial state: running=%v err=%v", running, err)
	}

	// A status check must release the lock it briefly acquired to test for a
	// holder - otherwise the very first status poll would permanently block a
	// real agent from ever starting.
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0666)
	if err != nil {
		t.Fatalf("failed to open lock file: %v", err)
	}
	defer f.Close()
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		t.Fatalf("a real agent could not acquire the lock after a status check: %v", err)
	}
}

// TestAgentPIDAt_DecoyProcessWithMatchingCommandLineIsIgnored reproduces the
// bug's own live repro directly: a process whose command line contains
// "career_agent_bin" but which never touches the lock file. The old `pgrep -f`
// implementation matched this and reported the agent running; agentPIDAt has
// no way to be fooled by it because it never inspects command lines at all.
func TestAgentPIDAt_DecoyProcessWithMatchingCommandLineIsIgnored(t *testing.T) {
	lockPath := filepath.Join(t.TempDir(), "career_agent.lock")

	decoy := exec.Command("sleep", "2")
	decoy.Args = []string{"career_agent_bin_decoy", "2"}
	if err := decoy.Start(); err != nil {
		t.Skipf("could not start decoy process: %v", err)
	}
	defer func() {
		_ = decoy.Process.Kill()
		_ = decoy.Wait()
	}()

	_, running, err := agentPIDAt(lockPath)
	if err != nil {
		t.Fatalf("agentPIDAt returned error: %v", err)
	}
	if running {
		t.Fatal("agentPIDAt reported running because of an unrelated decoy process's command line - the exact bug #449 failure mode")
	}
}
