package scraper

import (
	"sync"
	"time"
)

// bugs.md #475: discoverWithYahooHTML's per-query retry (bug #130) handles a
// transient blip but cannot out-wait a sustained block -- a live-log audit
// found 77.8% of Yahoo fallback queries failing at a steady ~45-49/minute
// for a full 8+ hour run, not in bursts. yahooCircuitBreaker is the
// source-level counterpart to #469's domainCircuitBreaker (cmd/agent):
// once a run of consecutive final failures crosses a threshold, further
// queries are skipped for a cooldown window instead of spending the 5-way
// concurrent request budget and the 3x retry cost on a source that is
// currently not viable. Unlike #469's breaker, this tracks exactly one
// source (Yahoo), not a map keyed by domain, and is a package-level
// singleton so its state survives across the repeated FunnelEngine
// instances the daemon's discovery loop constructs per refresh.

type circuitState uint8

const (
	circuitClosed circuitState = iota
	circuitOpen
	circuitHalfOpen
)

const (
	// yahooCircuitFailureThreshold is how many consecutive final failures
	// open the circuit.
	yahooCircuitFailureThreshold = 5
	// yahooCircuitCooldown is how long an open circuit stays closed to new
	// queries before allowing one half-open probe through.
	yahooCircuitCooldown = 2 * time.Minute
)

// sourceCircuitBreaker is safe for concurrent use by multiple workers.
type sourceCircuitBreaker struct {
	mu                  sync.Mutex
	state               circuitState
	consecutiveFailures int
	openedAt            time.Time
	now                 func() time.Time
}

func newSourceCircuitBreaker() *sourceCircuitBreaker {
	return &sourceCircuitBreaker{now: time.Now}
}

// allow reports whether a query may proceed right now. An open circuit past
// its cooldown transitions to half-open and allows exactly one probe
// through; further calls are refused until that probe resolves via
// recordSuccess or recordFailure.
func (b *sourceCircuitBreaker) allow() bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	switch b.state {
	case circuitOpen:
		if b.now().Sub(b.openedAt) < yahooCircuitCooldown {
			return false
		}
		b.state = circuitHalfOpen
		return true
	case circuitHalfOpen:
		return false
	default:
		return true
	}
}

// recordSuccess closes the circuit, whether it was previously closed,
// half-open (a probe resolved cleanly), or briefly open.
func (b *sourceCircuitBreaker) recordSuccess() {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.state = circuitClosed
	b.consecutiveFailures = 0
}

// recordFailure counts a final (retry-exhausted) failure. A closed circuit
// opens once consecutiveFailures reaches the threshold; a half-open probe
// that fails re-opens immediately and restarts the cooldown.
func (b *sourceCircuitBreaker) recordFailure() {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.state == circuitHalfOpen {
		b.state = circuitOpen
		b.openedAt = b.now()
		return
	}

	b.consecutiveFailures++
	// Only the closed->open transition sets openedAt. Without the
	// b.state != circuitOpen guard, a straggler query that started before
	// the circuit opened (concurrent workers can have several in flight)
	// would still report its failure afterward and re-stamp openedAt,
	// repeatedly pushing the cooldown out past yahooCircuitCooldown for as
	// long as stragglers trickle in.
	if b.consecutiveFailures >= yahooCircuitFailureThreshold && b.state != circuitOpen {
		b.state = circuitOpen
		b.openedAt = b.now()
	}
}

// yahooBreaker is a package-level singleton rather than a FunnelEngine field
// because a new FunnelEngine is constructed on every daemon discovery
// refresh (cmd/agent/main.go's runDaemonDiscoveryLoop), and the whole point
// of this breaker is to remember a failure streak across those refreshes.
var yahooBreaker = newSourceCircuitBreaker()
