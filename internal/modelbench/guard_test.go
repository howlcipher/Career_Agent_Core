package modelbench

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestIsAgentRunning_NoLockHeld(t *testing.T) {
	path := filepath.Join(t.TempDir(), "career_agent.lock")
	running, _, err := IsAgentRunning(path)
	if err != nil {
		t.Fatalf("IsAgentRunning: %v", err)
	}
	if running {
		t.Fatalf("expected running=false when nothing holds the lock")
	}
}

func TestIsAgentRunning_LockHeldWithPID(t *testing.T) {
	path := filepath.Join(t.TempDir(), "career_agent.lock")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0666)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer f.Close()
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		t.Fatalf("flock: %v", err)
	}
	if _, err := f.WriteString("4242"); err != nil {
		t.Fatalf("write pid: %v", err)
	}

	running, pid, err := IsAgentRunning(path)
	if err != nil {
		t.Fatalf("IsAgentRunning: %v", err)
	}
	if !running {
		t.Fatalf("expected running=true while the lock is held")
	}
	if pid != 4242 {
		t.Errorf("pid = %d, want 4242", pid)
	}
}

func TestIsAgentRunning_CheckReleasesItsOwnProbeLock(t *testing.T) {
	path := filepath.Join(t.TempDir(), "career_agent.lock")
	if running, _, err := IsAgentRunning(path); err != nil || running {
		t.Fatalf("first check: running=%v err=%v", running, err)
	}
	// A second, independent check must also see it as free -- the first
	// call must not have left the lock held.
	if running, _, err := IsAgentRunning(path); err != nil || running {
		t.Fatalf("second check: running=%v err=%v", running, err)
	}
}
