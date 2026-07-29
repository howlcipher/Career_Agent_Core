package mcp

import (
	"math"
	"math/rand"
	"time"
)

// ExponentialBackoff calculates an exponential backoff duration with jitter.
// attempt is 1-indexed (e.g. 1 for first retry).
// Base duration is 10 seconds, maximum is 120 seconds.
// A +/- 20% jitter is applied to prevent thundering herd.
func ExponentialBackoff(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}

	base := 10.0         // seconds
	maxDuration := 120.0 // seconds

	// Calculate base exponential backoff
	backoff := base * math.Pow(2.0, float64(attempt-1))
	if backoff > maxDuration {
		backoff = maxDuration
	}

	// Calculate +/- 20% jitter
	jitter := backoff * 0.2
	
	// r will be between -jitter and +jitter
	r := (rand.Float64() * 2 * jitter) - jitter

	finalDuration := backoff + r
	
	return time.Duration(finalDuration * float64(time.Second))
}
