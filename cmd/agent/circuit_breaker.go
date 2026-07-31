package main

import (
	"sync"
	"time"
)

// improvements.md #469: retry inside fetchJobPage/checkJobAlive is
// request-local -- each call retries on its own with no memory of a
// domain's recent failure history. A domain that is timing out on every
// request still gets tried again on the very next job from that same
// source, and a live-log audit found several domains repeatedly logging
// "context deadline exceeded" while the worker had no way to spend that
// time on a healthier source instead. domainCircuitBreaker tracks
// consecutive retryable failures per registrable domain (keyed the same
// way getATSProvider already keys application_attempts.source) and opens
// a short cooldown once a domain crosses a failure threshold, so
// subsequent jobs from that domain are returned to the queue immediately
// instead of spending a full request timeout finding out again.

type circuitState uint8

const (
	circuitClosed circuitState = iota
	circuitOpen
	circuitHalfOpen
)

const (
	// circuitBreakerFailureThreshold is how many consecutive retryable
	// failures from one domain open its circuit.
	circuitBreakerFailureThreshold = 5
	// circuitBreakerCooldown is how long an open circuit stays closed to
	// new attempts before allowing one half-open probe through.
	circuitBreakerCooldown = 2 * time.Minute
)

type domainCircuit struct {
	state               circuitState
	consecutiveFailures int
	openedAt            time.Time
}

// domainCircuitBreaker is safe for concurrent use by multiple workers.
// State is kept only for domains that have failed at least once, so it
// stays bounded by the number of distinct ATS domains actually seen
// failing, not by queue size.
type domainCircuitBreaker struct {
	mu      sync.Mutex
	domains map[string]*domainCircuit
	now     func() time.Time
}

func newDomainCircuitBreaker() *domainCircuitBreaker {
	return &domainCircuitBreaker{
		domains: make(map[string]*domainCircuit),
		now:     time.Now,
	}
}

// allow reports whether a request to domain may proceed right now. An
// open circuit past its cooldown transitions to half-open and allows
// exactly one probe through; further calls are refused until that probe
// resolves via recordSuccess or recordFailure.
func (b *domainCircuitBreaker) allow(domain string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	c := b.domains[domain]
	if c == nil {
		return true
	}

	switch c.state {
	case circuitOpen:
		if b.now().Sub(c.openedAt) < circuitBreakerCooldown {
			return false
		}
		c.state = circuitHalfOpen
		return true
	case circuitHalfOpen:
		return false
	default:
		return true
	}
}

// recordSuccess closes domain's circuit, whether it was previously
// closed, half-open (a probe resolved cleanly), or briefly open.
func (b *domainCircuitBreaker) recordSuccess(domain string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	c := b.domains[domain]
	if c == nil {
		return
	}
	c.state = circuitClosed
	c.consecutiveFailures = 0
}

// recordFailure counts a retryable failure for domain. A closed circuit
// opens once consecutiveFailures reaches the threshold; a half-open
// probe that fails re-opens immediately and restarts the cooldown.
func (b *domainCircuitBreaker) recordFailure(domain string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	c := b.domains[domain]
	if c == nil {
		c = &domainCircuit{}
		b.domains[domain] = c
	}

	if c.state == circuitHalfOpen {
		c.state = circuitOpen
		c.openedAt = b.now()
		return
	}

	c.consecutiveFailures++
	// Only the closed->open transition sets openedAt. Without the
	// c.state != circuitOpen guard, a straggler request that started
	// before the circuit opened (concurrent workers can have several
	// in flight against the same domain) would still report its failure
	// afterward and re-stamp openedAt, repeatedly pushing the cooldown
	// out past circuitBreakerCooldown for as long as stragglers trickle in.
	if c.consecutiveFailures >= circuitBreakerFailureThreshold && c.state != circuitOpen {
		c.state = circuitOpen
		c.openedAt = b.now()
	}
}
