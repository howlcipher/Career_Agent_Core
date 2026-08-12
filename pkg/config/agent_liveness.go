package config

import (
	"os"
	"strconv"
	"strings"
	"syscall"
)

// AgentLockPath is the single-instance flock used by cmd/agent to prove it is
// the only running agent. cmd/dashboard reuses the same file as the source of
// truth for liveness instead of inspecting command lines or PID registries.
var AgentLockPath = "applications/career_agent.lock"

// IsAgentAlive returns whether the agent's single-instance lock is currently
// held, and the PID written into the lock file when it can be read. A held
// lock means an agent process exists; a free lock means no agent is running.
// If the file contents are unreadable or unparseable, the lock is still reported
// as held because the existence of a holder is the authoritative fact; the
// caller just has no safe PID to signal.
func IsAgentAlive(lockPath string) (pid int, running bool, err error) {
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0666)
	if err != nil {
		return 0, false, err
	}
	defer f.Close()

	// Use a shared lock for the status check. A shared lock is compatible with
	// another shared lock but incompatible with the agent's exclusive lock, so
	// this detects a running agent without racing against the agent's own single
	// acquisition attempt.
	if flockErr := syscall.Flock(int(f.Fd()), syscall.LOCK_SH|syscall.LOCK_NB); flockErr != nil {
		// Held by another process: that is the agent. Its PID is whatever it
		// wrote when it acquired the lock; treat unreadable or unparsed
		// content as "running, PID unknown" rather than failing the check.
		data, readErr := os.ReadFile(lockPath)
		if readErr == nil {
			if parsed, parseErr := strconv.Atoi(strings.TrimSpace(string(data))); parseErr == nil {
				pid = parsed
			}
		}
		return pid, true, nil
	}
	syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	return 0, false, nil
}
