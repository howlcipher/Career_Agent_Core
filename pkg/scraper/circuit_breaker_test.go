package scraper

import (
	"sync"
	"testing"
	"time"
)

func TestSourceCircuitBreaker_AllowsFreshBreaker(t *testing.T) {
	b := newSourceCircuitBreaker()
	if !b.allow() {
		t.Fatal("a fresh breaker must allow the first query")
	}
}

func TestSourceCircuitBreaker_OpensAfterConsecutiveFailures(t *testing.T) {
	b := newSourceCircuitBreaker()

	for i := 0; i < yahooCircuitFailureThreshold-1; i++ {
		b.recordFailure()
		if !b.allow() {
			t.Fatalf("circuit opened too early, after %d failures", i+1)
		}
	}

	b.recordFailure()
	if b.allow() {
		t.Fatal("circuit should be open after reaching the failure threshold")
	}
}

func TestSourceCircuitBreaker_SuccessResetsFailureCount(t *testing.T) {
	b := newSourceCircuitBreaker()

	for i := 0; i < yahooCircuitFailureThreshold-1; i++ {
		b.recordFailure()
	}
	b.recordSuccess()

	for i := 0; i < yahooCircuitFailureThreshold-1; i++ {
		b.recordFailure()
		if !b.allow() {
			t.Fatalf("circuit opened too early after reset, at failure %d", i+1)
		}
	}
}

func TestSourceCircuitBreaker_HalfOpenAllowsExactlyOneProbe(t *testing.T) {
	b := newSourceCircuitBreaker()
	now := time.Now()
	b.now = func() time.Time { return now }

	for i := 0; i < yahooCircuitFailureThreshold; i++ {
		b.recordFailure()
	}
	if b.allow() {
		t.Fatal("circuit should be open immediately after opening")
	}

	now = now.Add(yahooCircuitCooldown)
	if !b.allow() {
		t.Fatal("circuit should allow exactly one probe once the cooldown elapses")
	}
	if b.allow() {
		t.Fatal("a second concurrent call must not get a probe while one is already in flight")
	}
}

func TestSourceCircuitBreaker_FailedProbeReopensAndRestartsCooldown(t *testing.T) {
	b := newSourceCircuitBreaker()
	now := time.Now()
	b.now = func() time.Time { return now }

	for i := 0; i < yahooCircuitFailureThreshold; i++ {
		b.recordFailure()
	}
	now = now.Add(yahooCircuitCooldown)
	if !b.allow() {
		t.Fatal("probe should be allowed once cooldown elapses")
	}
	b.recordFailure() // the probe itself failed

	if b.allow() {
		t.Fatal("a failed probe must re-open the circuit immediately")
	}

	now = now.Add(yahooCircuitCooldown - time.Second)
	if b.allow() {
		t.Fatal("cooldown should not have elapsed yet after the probe failure restarted it")
	}
	now = now.Add(2 * time.Second)
	if !b.allow() {
		t.Fatal("cooldown should have elapsed after the full duration since the probe failure")
	}
}

func TestSourceCircuitBreaker_SuccessfulProbeCloses(t *testing.T) {
	b := newSourceCircuitBreaker()
	now := time.Now()
	b.now = func() time.Time { return now }

	for i := 0; i < yahooCircuitFailureThreshold; i++ {
		b.recordFailure()
	}
	now = now.Add(yahooCircuitCooldown)
	if !b.allow() {
		t.Fatal("probe should be allowed once cooldown elapses")
	}
	b.recordSuccess()

	for i := 0; i < yahooCircuitFailureThreshold; i++ {
		if !b.allow() {
			t.Fatalf("circuit should be fully closed after a successful probe, blocked at call %d", i+1)
		}
	}
}

// TestSourceCircuitBreaker_StragglerFailureDoesNotExtendCooldown mirrors the
// review-pass finding recorded against #469's domainCircuitBreaker: a query
// that started before the circuit opened can still report its failure
// afterward, and that straggler must not re-stamp openedAt.
func TestSourceCircuitBreaker_StragglerFailureDoesNotExtendCooldown(t *testing.T) {
	b := newSourceCircuitBreaker()
	now := time.Now()
	b.now = func() time.Time { return now }

	for i := 0; i < yahooCircuitFailureThreshold; i++ {
		b.recordFailure()
	}
	if b.allow() {
		t.Fatal("circuit should be open immediately after opening")
	}

	now = now.Add(yahooCircuitCooldown / 2)
	b.recordFailure() // straggler, reported late

	now = now.Add(yahooCircuitCooldown/2 + time.Second)
	if !b.allow() {
		t.Fatal("cooldown should have elapsed from the original open time; a straggler failure must not have extended it")
	}
}

// TestSourceCircuitBreaker_ConcurrentAccess exercises allow/recordSuccess/
// recordFailure from many goroutines so `go test -race` can catch any data
// race in the mutex usage, matching the 5-way concurrent callers
// discoverWithYahooHTML actually has (funnel.go's eg.SetLimit(5)).
func TestSourceCircuitBreaker_ConcurrentAccess(t *testing.T) {
	b := newSourceCircuitBreaker()
	const goroutines = 50
	const opsPerGoroutine = 200

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func(id int) {
			defer wg.Done()
			for i := 0; i < opsPerGoroutine; i++ {
				if b.allow() {
					if i%2 == 0 {
						b.recordFailure()
					} else {
						b.recordSuccess()
					}
				}
				b.allow()
			}
		}(g)
	}
	wg.Wait()
}
