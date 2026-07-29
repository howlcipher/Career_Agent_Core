package mcp

import (
	"testing"
)

func TestExponentialBackoff(t *testing.T) {
	tests := []struct {
		name        string
		attempt     int
		minExpected float64 // in seconds
		maxExpected float64 // in seconds
	}{
		{
			name:        "attempt 1",
			attempt:     1,
			minExpected: 10.0 * 0.8, // 8s
			maxExpected: 10.0 * 1.2, // 12s
		},
		{
			name:        "attempt 2",
			attempt:     2,
			minExpected: 20.0 * 0.8, // 16s
			maxExpected: 20.0 * 1.2, // 24s
		},
		{
			name:        "attempt 3",
			attempt:     3,
			minExpected: 40.0 * 0.8, // 32s
			maxExpected: 40.0 * 1.2, // 48s
		},
		{
			name:        "attempt 4",
			attempt:     4,
			minExpected: 80.0 * 0.8, // 64s
			maxExpected: 80.0 * 1.2, // 96s
		},
		{
			name:        "attempt 5 (capped)",
			attempt:     5,
			minExpected: 120.0 * 0.8, // 96s
			maxExpected: 120.0 * 1.2, // 144s
		},
		{
			name:        "attempt 10 (capped)",
			attempt:     10,
			minExpected: 120.0 * 0.8,
			maxExpected: 120.0 * 1.2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for i := 0; i < 100; i++ {
				d := ExponentialBackoff(tt.attempt)
				sec := d.Seconds()
				if sec < tt.minExpected || sec > tt.maxExpected {
					t.Errorf("attempt %d: expected between %.2f and %.2f seconds, got %.2f", tt.attempt, tt.minExpected, tt.maxExpected, sec)
				}
			}
		})
	}
}
