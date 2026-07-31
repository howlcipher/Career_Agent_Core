package main

import (
	"sync"
	"testing"
	"time"
)

func TestDomainCircuitBreaker_AllowsUnseenDomain(t *testing.T) {
	b := newDomainCircuitBreaker()
	if !b.allow("example.com") {
		t.Fatal("an unseen domain must be allowed")
	}
}

func TestDomainCircuitBreaker_OpensAfterConsecutiveFailures(t *testing.T) {
	b := newDomainCircuitBreaker()
	domain := "flaky.example.com"

	for i := 0; i < circuitBreakerFailureThreshold-1; i++ {
		b.recordFailure(domain)
		if !b.allow(domain) {
			t.Fatalf("circuit opened too early, after %d failures", i+1)
		}
	}

	b.recordFailure(domain)
	if b.allow(domain) {
		t.Fatal("circuit should be open after reaching the failure threshold")
	}
}

func TestDomainCircuitBreaker_SuccessResetsFailureCount(t *testing.T) {
	b := newDomainCircuitBreaker()
	domain := "recovering.example.com"

	for i := 0; i < circuitBreakerFailureThreshold-1; i++ {
		b.recordFailure(domain)
	}
	b.recordSuccess(domain)

	// A success below the threshold should reset the counter, so the
	// circuit needs a fresh full run of failures to open again.
	for i := 0; i < circuitBreakerFailureThreshold-1; i++ {
		b.recordFailure(domain)
		if !b.allow(domain) {
			t.Fatalf("circuit opened too early after reset, at failure %d", i+1)
		}
	}
}

func TestDomainCircuitBreaker_HalfOpenAllowsExactlyOneProbe(t *testing.T) {
	b := newDomainCircuitBreaker()
	domain := "cooling-down.example.com"
	now := time.Now()
	b.now = func() time.Time { return now }

	for i := 0; i < circuitBreakerFailureThreshold; i++ {
		b.recordFailure(domain)
	}
	if b.allow(domain) {
		t.Fatal("circuit should be open immediately after opening")
	}

	now = now.Add(circuitBreakerCooldown)
	if !b.allow(domain) {
		t.Fatal("circuit should allow exactly one probe once the cooldown elapses")
	}
	if b.allow(domain) {
		t.Fatal("a second concurrent call must not get a probe while one is already in flight")
	}
}

func TestDomainCircuitBreaker_FailedProbeReopensAndRestartsCooldown(t *testing.T) {
	b := newDomainCircuitBreaker()
	domain := "still-down.example.com"
	now := time.Now()
	b.now = func() time.Time { return now }

	for i := 0; i < circuitBreakerFailureThreshold; i++ {
		b.recordFailure(domain)
	}
	now = now.Add(circuitBreakerCooldown)
	if !b.allow(domain) {
		t.Fatal("probe should be allowed once cooldown elapses")
	}
	b.recordFailure(domain) // the probe itself failed

	if b.allow(domain) {
		t.Fatal("a failed probe must re-open the circuit immediately")
	}

	// Cooldown must have restarted from the probe's failure, not the
	// original open time.
	now = now.Add(circuitBreakerCooldown - time.Second)
	if b.allow(domain) {
		t.Fatal("cooldown should not have elapsed yet after the probe failure restarted it")
	}
	now = now.Add(2 * time.Second)
	if !b.allow(domain) {
		t.Fatal("cooldown should have elapsed after the full duration since the probe failure")
	}
}

func TestDomainCircuitBreaker_SuccessfulProbeCloses(t *testing.T) {
	b := newDomainCircuitBreaker()
	domain := "recovered.example.com"
	now := time.Now()
	b.now = func() time.Time { return now }

	for i := 0; i < circuitBreakerFailureThreshold; i++ {
		b.recordFailure(domain)
	}
	now = now.Add(circuitBreakerCooldown)
	if !b.allow(domain) {
		t.Fatal("probe should be allowed once cooldown elapses")
	}
	b.recordSuccess(domain)

	for i := 0; i < circuitBreakerFailureThreshold; i++ {
		if !b.allow(domain) {
			t.Fatalf("circuit should be fully closed after a successful probe, blocked at call %d", i+1)
		}
	}
}

// TestDomainCircuitBreaker_OneFailingDomainDoesNotGateAnother is the
// improvements.md #469 acceptance criterion stated verbatim: "test that
// one failing domain does not delay unrelated queue entries."
func TestDomainCircuitBreaker_OneFailingDomainDoesNotGateAnother(t *testing.T) {
	b := newDomainCircuitBreaker()
	failing := "always-times-out.example.com"
	healthy := "healthy.example.com"

	for i := 0; i < circuitBreakerFailureThreshold; i++ {
		b.recordFailure(failing)
	}
	if b.allow(failing) {
		t.Fatal("the failing domain's circuit should be open")
	}
	if !b.allow(healthy) {
		t.Fatal("an unrelated healthy domain must not be gated by another domain's open circuit")
	}
}

// TestDomainCircuitBreaker_StragglerFailureDoesNotExtendCooldown is a
// review-pass finding: a request that started before the circuit opened
// (concurrent workers can have several in flight against the same domain)
// can still report its failure afterward. That straggler must not re-stamp
// openedAt and push the cooldown out further, or a domain with enough
// concurrent in-flight requests at the moment it trips could keep its own
// circuit open indefinitely.
func TestDomainCircuitBreaker_StragglerFailureDoesNotExtendCooldown(t *testing.T) {
	b := newDomainCircuitBreaker()
	domain := "straggler.example.com"
	now := time.Now()
	b.now = func() time.Time { return now }

	for i := 0; i < circuitBreakerFailureThreshold; i++ {
		b.recordFailure(domain)
	}
	if b.allow(domain) {
		t.Fatal("circuit should be open immediately after opening")
	}

	// A straggler request, already in flight before the circuit opened,
	// reports its own failure late.
	now = now.Add(circuitBreakerCooldown / 2)
	b.recordFailure(domain)

	// The cooldown must still be measured from the original open time, not
	// from the straggler's late report.
	now = now.Add(circuitBreakerCooldown/2 + time.Second)
	if !b.allow(domain) {
		t.Fatal("cooldown should have elapsed from the original open time; a straggler failure must not have extended it")
	}
}

// TestDomainCircuitBreaker_ConcurrentAccess exercises allow/recordSuccess/
// recordFailure from many goroutines across both a shared domain and
// per-goroutine unique domains, so `go test -race` can catch any data race
// in the mutex usage. It only asserts the run completes cleanly under the
// race detector -- the interesting assertions are the single-goroutine
// state-machine tests above.
func TestDomainCircuitBreaker_ConcurrentAccess(t *testing.T) {
	b := newDomainCircuitBreaker()
	const goroutines = 50
	const opsPerGoroutine = 200

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func(id int) {
			defer wg.Done()
			shared := "contended.example.com"
			for i := 0; i < opsPerGoroutine; i++ {
				if b.allow(shared) {
					if i%2 == 0 {
						b.recordFailure(shared)
					} else {
						b.recordSuccess(shared)
					}
				}
				b.allow(shared)
			}
		}(g)
	}
	wg.Wait()
}
