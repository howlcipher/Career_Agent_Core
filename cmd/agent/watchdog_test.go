package main

import "testing"

func TestDaemonWatchdog(t *testing.T) {
	tests := []struct {
		name      string
		snapshots []watchdogSnapshot
		alerts    int
	}{
		{"dominant failure alerts once", []watchdogSnapshot{{4, 0, map[string]int{"QUARANTINED_PROMPT_INJECTION": 8, "FAILED_SUBMIT": 1}}, {4, 0, map[string]int{"QUARANTINED_PROMPT_INJECTION": 8, "FAILED_SUBMIT": 1}}, {4, 0, map[string]int{"QUARANTINED_PROMPT_INJECTION": 8, "FAILED_SUBMIT": 1}}, {4, 0, map[string]int{"QUARANTINED_PROMPT_INJECTION": 8, "FAILED_SUBMIT": 1}}}, 1},
		{"healthy variety stays silent", []watchdogSnapshot{{4, 0, map[string]int{"FAILED_SUBMIT": 2, "SKIPPED": 2}}, {4, 0, map[string]int{"FAILED_SUBMIT": 2, "SKIPPED": 2}}, {4, 0, map[string]int{"FAILED_SUBMIT": 2, "SKIPPED": 2}}}, 0},
		{"empty eligible queue stays silent", []watchdogSnapshot{{0, 0, map[string]int{"FAILED_SUBMIT": 8}}, {0, 0, map[string]int{"FAILED_SUBMIT": 8}}, {0, 0, map[string]int{"FAILED_SUBMIT": 8}}}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			watchdog := daemonWatchdog{}
			alerts := 0
			for _, snapshot := range tt.snapshots {
				if watchdog.Observe(snapshot) != "" {
					alerts++
				}
			}
			if alerts != tt.alerts {
				t.Fatalf("alerts = %d, want %d", alerts, tt.alerts)
			}
		})
	}
}
