# Task Journal: #477 Yahoo fallback requests carry no realistic browser headers or cookie jar

Copy of TEMPLATE.md for this task. Delete on close per Working Protocol step 7.

## Summary

- **Task:** improvements.md #477 — add `Accept`/`Accept-Language` headers and a shared `http.CookieJar` to `discoverWithYahooHTML` in `pkg/scraper/funnel.go`
- **Status:** In progress
- **Started:** 2026-08-01
- **Agent and model:** Claude Code orchestrator (Sonnet 5) + Claude subagent(s) for implementation/review — user's session goal explicitly asked to prioritize Claude models for subagents/review passes this run, overriding the Working Protocol's default of delegating to a non-Claude model to save session limits.

## Pre-Flight Re-Evaluation

- **Usability Gate check:** MET as of 2026-08-01 (bugs.md #476 closed it, zero Pending rows in bugs.md). improvements.md is fair game.
- **Model choice:** Claude subagents (per user's explicit session-goal instruction to prioritize Claude this run). Tier is `standard`.
- **Skills routed:** `software_development` (../ai_knowledge_library/.agents/skills/software_development/SKILL.md) — standard Go implementation, no other skill fits better.
- **Code re-verified 2026-08-01:** Confirmed via Explore agent — `discoverWithYahooHTML` (`pkg/scraper/funnel.go:209-316`) sets only `User-Agent` (line 228), no `Accept`/`Accept-Language`/`Referer`. `client := newHTTPClient(10 * time.Second)` (line 217) creates a fresh client per call via `security.NewSafeHTTPClient` → `NewNetworkGuard().HTTPClient(timeout)` (`pkg/security/network.go:314-339`), which sets no `Jar`. Circuit breaker (`sourceCircuitBreaker`/`yahooBreaker`, bug #475) is present and unaffected by this change. Concurrency is bounded by `eg.SetLimit(5)` in `DiscoverJobs` (`funnel.go:69`). Item's claims match current code exactly — not stale.

## Plan

- [x] Add `Accept`/`Accept-Language` headers to the Yahoo request in `discoverWithYahooHTML`
- [x] Share one `http.CookieJar` across all Yahoo queries within a single `DiscoverJobs` run (not a fresh client per call)
- [x] Add/adjust tests covering the new headers and jar-sharing behavior
- [x] Run `go build ./...`, `go vet ./...`, `go test ./...`, `gofmt -l ./cmd ./pkg ./internal`
- [x] Independent Claude review pass (background subagent) — mutation-checked the new tests itself, found no bugs
- [ ] Attempt live verification against the running daemon's log — in progress, gathering a larger post-fix sample
- [ ] Update improvements.md row #477 to Done with a Done note; add CHANGELOG.md entry
- [ ] File the newly-found live bug (stuck-DISCOVERED job spins the daemon) as a new bugs.md row
- [ ] Commit, delete this journal, push

## Progress Log

- 2026-08-01 09:00 — Journal opened. Code re-verified against current `pkg/scraper/funnel.go` and `pkg/security/network.go`; item is accurate, not stale.
- 2026-08-01 09:03 — Implemented directly (design was fully nailed down from the re-verification, so no delegation brief needed): `newYahooHTTPClient()` builds an `http.Client` with `Jar: cookiejar.New(nil)`; `FunnelEngine` gained `yahooClientOnce sync.Once` + `yahooClient *http.Client` fields so one client/jar is shared per engine instance (== per `DiscoverJobs` run, since `cmd/agent/main.go:1108` constructs a fresh `FunnelEngine` every call). Added `Accept`/`Accept-Language` headers next to the existing `User-Agent` line. Added 3 new tests to `yahoo_test.go` (headers arrive at mock server; second query on same engine carries first query's `Set-Cookie`, fresh engine does not; 5-way concurrent calls race-clean under `-race`). Mutation-checked all three myself by reverting each piece and confirming the corresponding test fails, then restored. Full suite + `-race` on `pkg/scraper` clean, `gofmt` clean.
- 2026-08-01 09:05 — Rebuilt `career_agent_bin` with the fix, gracefully stopped the live daemon (SIGTERM, clean "Shutdown complete", ~1s), swapped the binary, restarted with identical flags (`-daemon -cycle-limit 15 -cycle-interval 1m`) so `career_agent.log` continues the same file for a clean before/after comparison. New PID confirmed holding the lock file.
- 2026-08-01 09:06 — Launched a background Claude subagent for an independent review (did not just re-read the diff — it mechanically reverted each piece of the fix and reran the new tests to confirm they actually catch the regression, same technique I used). Verdict: no bugs found, scoping and concurrency safety both confirmed correct.
- 2026-08-01 09:06 — **New finding, not part of this task, logged for the backlog and not fixed:** the live daemon was spinning at ~1 cycle/sec (83 cycles in 85s) retrying one job whose URL has an unresolvable hostname (`wwww.raileurope.com`, note the 4 w's — dead/typo'd domain), because `cmd/agent/pipeline.go:101-110`'s `StateInit` node only marks a job `InvalidURLReasonMalformed` when `ValidateURL` returns `security.ErrUnsafeNetworkTarget`; a plain DNS resolution failure (`pkg/security/network.go:168`, wrapped as `"resolve network target: %w"`, NOT wrapped in `ErrUnsafeNetworkTarget`) falls into the `else` branch, which only logs and returns `StateEnd` — never updating `job_funnel.status` away from `DISCOVERED`. The job is reloaded and retried every single cycle forever, with `cycleInterval` never applying because "completed with work" triggers an immediate next cycle. This blocked the independent discovery goroutine's throughput early on but did not stop it (discovery runs on its own loop per bug #475's daemon-cadence notes) — Yahoo fallback traffic was still observed shortly after restart. Will file as a new bugs.md row in the groom pass, not fixed in this task (out of scope for #477).
- 2026-08-01 09:07 — Baseline comparison from the log (both windows post-#475's breaker, apples to apples): pre-fix window (2026-07-31 20:24 to 2026-08-01 09:04, ~12.6h) was 678 final failures / 2690 attempted (breaker-allowed) queries = 25.2%. Post-fix, first ~100s window was 10/60 = 16.7%. Sample too small to call yet — gathering a larger post-fix window before writing the Done note (SerpAPI is exhausted for the billing period, "Your account has run out of searches", so every query goes through Yahoo right now, giving a fast sample).

## Next Step

Read back the larger post-fix Yahoo sample (background wait `b6doq2sb2` was started to let ~5 more minutes accumulate), compute the final failure-rate comparison, write the Done note either way per the item's own allowance, then close out: CHANGELOG entry, improvements.md row to Done, file the new DNS-retry-storm bug in bugs.md, commit, delete this journal, push.
