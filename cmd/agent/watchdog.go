package main

import (
	"fmt"

	"github.com/howlcipher/Career_Agent_Core/pkg/storage"
)

const (
	watchdogConsecutiveCycles = 3
	watchdogDominantFraction  = 0.75
)

// watchdogSnapshot contains aggregate, non-sensitive cycle outcomes. It never
// carries job text, URLs, or profile data into the diagnostic alert.
type watchdogSnapshot struct {
	EligibleQueue int
	Confirmed     int
	Terminal      map[string]int
}

type daemonWatchdog struct {
	previousConfirmed int
	stalledCycles     int
	lastAlert         string
}

func watchdogSnapshotFromStorage(snapshot storage.DaemonWatchdogSnapshot) watchdogSnapshot {
	return watchdogSnapshot{
		EligibleQueue: snapshot.EligibleQueue,
		Confirmed:     snapshot.Confirmed,
		Terminal:      snapshot.Terminal,
	}
}

// Observe returns one actionable alert when a nonempty queue continues to be
// processed without a confirmed application and a terminal status dominates.
// Returning an empty string deduplicates the condition across later cycles.
func (w *daemonWatchdog) Observe(snapshot watchdogSnapshot) string {
	if snapshot.EligibleQueue == 0 {
		w.stalledCycles = 0
		w.previousConfirmed = snapshot.Confirmed
		w.lastAlert = ""
		return ""
	}
	if snapshot.Confirmed > w.previousConfirmed {
		w.stalledCycles = 0
		w.lastAlert = ""
	} else {
		w.stalledCycles++
	}
	w.previousConfirmed = snapshot.Confirmed
	if w.stalledCycles < watchdogConsecutiveCycles {
		return ""
	}

	status, count, total := dominantTerminalStatus(snapshot.Terminal)
	if total == 0 || float64(count)/float64(total) < watchdogDominantFraction {
		w.lastAlert = ""
		return ""
	}
	if w.lastAlert != "" {
		return ""
	}
	alert := fmt.Sprintf(
		"no confirmed applications in %d consecutive nonempty cycles; %s dominates terminal outcomes (%d/%d)",
		w.stalledCycles, status, count, total,
	)
	w.lastAlert = alert
	return alert
}

func dominantTerminalStatus(counts map[string]int) (string, int, int) {
	status := ""
	count, total := 0, 0
	for candidate, value := range counts {
		if value <= 0 {
			continue
		}
		total += value
		if value > count || (value == count && candidate < status) {
			status, count = candidate, value
		}
	}
	return status, count, total
}
