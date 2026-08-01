package modelbench

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"
)

// AgentLockPath is cmd/agent's single-instance lock (bugs.md #414), reused
// read-only here to warn a benchmark run away from a live production attempt
// instead of racing it for CPU and Ollama's single loaded-model slot.
const AgentLockPath = "applications/career_agent.lock"

// IsAgentRunning reports whether the production agent currently holds its
// single-instance lock, mirroring cmd/dashboard's agentPIDAt (bug #449) --
// duplicated rather than imported because that package is `main` and not
// importable, and this check is a handful of lines. It never blocks or
// mutates: acquiring the lock to test it and releasing it immediately is the
// same non-destructive check the dashboard already performs for its own
// status endpoint.
func IsAgentRunning(lockPath string) (running bool, pid int, err error) {
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0666)
	if err != nil {
		return false, 0, fmt.Errorf("open agent lock file %s: %w", lockPath, err)
	}
	defer f.Close()

	if flockErr := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); flockErr != nil {
		data, readErr := os.ReadFile(lockPath)
		if readErr == nil {
			if parsed, parseErr := strconv.Atoi(strings.TrimSpace(string(data))); parseErr == nil {
				pid = parsed
			}
		}
		return true, pid, nil
	}
	syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	return false, 0, nil
}
