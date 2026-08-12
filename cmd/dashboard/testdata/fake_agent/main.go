// fake_agent is a test double for career_agent_bin. It acquires the agent
// single-instance lock, writes its PID, and then waits for SIGTERM. Behaviour
// is controlled by environment variables so the dashboard lifecycle tests can
// exercise slow shutdown, natural exit, ignored signals, and readiness.
package main

import (
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"
)

func main() {
	lockPath := os.Getenv("FAKE_AGENT_LOCK_PATH")
	if lockPath == "" {
		lockPath = "applications/career_agent.lock"
	}

	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0666)
	if err != nil {
		fmt.Fprintf(os.Stderr, "fake_agent: open lock: %v\n", err)
		os.Exit(1)
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		fmt.Fprintf(os.Stderr, "fake_agent: lock held: %v\n", err)
		os.Exit(1)
	}
	_ = f.Truncate(0)
	_, _ = f.WriteAt([]byte(strconv.Itoa(os.Getpid())), 0)
	fmt.Fprintf(os.Stderr, "fake_agent: ready pid=%d\n", os.Getpid())

	if os.Getenv("FAKE_AGENT_EXIT_IMMEDIATELY") == "1" {
		return
	}

	if d := os.Getenv("FAKE_AGENT_EXIT_AFTER"); d != "" {
		if dur, err := time.ParseDuration(d); err == nil {
			<-time.After(dur)
			return
		}
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM)
	<-sigCh

	if os.Getenv("FAKE_AGENT_IGNORE_SIGTERM") == "1" {
		select {}
	}

	if d := os.Getenv("FAKE_AGENT_STOP_DELAY"); d != "" {
		if dur, err := time.ParseDuration(d); err == nil {
			time.Sleep(dur)
		}
	}
}
