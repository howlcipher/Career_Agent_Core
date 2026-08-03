package main

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func TestRunRefusesWhileAgentLockHeld(t *testing.T) {
	dir := t.TempDir()
	briefPath := filepath.Join(dir, "brief.md")
	if err := os.WriteFile(briefPath, []byte("Investigate a synthetic test only."), 0600); err != nil {
		t.Fatal(err)
	}
	lockPath := filepath.Join(dir, "agent.lock")
	lock, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		t.Fatal(err)
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)

	out, err := os.CreateTemp(t.TempDir(), "stdout")
	if err != nil {
		t.Fatal(err)
	}
	defer out.Close()
	errOut, err := os.CreateTemp(t.TempDir(), "stderr")
	if err != nil {
		t.Fatal(err)
	}
	defer errOut.Close()

	code := run([]string{"-phase", "propose", "-model", "test-model", "-brief-file", briefPath, "-proposal-file", filepath.Join(dir, "proposal.json"), "-lock-path", lockPath}, out, errOut)
	if code != 1 {
		t.Fatalf("run() code = %d, want 1", code)
	}
	if _, err := errOut.Seek(0, 0); err != nil {
		t.Fatal(err)
	}
	message, err := os.ReadFile(errOut.Name())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(message), "refuses to compete") {
		t.Fatalf("stderr = %q, want lock refusal", message)
	}
}
