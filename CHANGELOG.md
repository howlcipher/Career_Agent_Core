# Career Agent Core - Changelog

## 2026-07-27 — Loopback-only dashboard default

* **Security:** The unauthenticated dashboard now defaults to `127.0.0.1:8080` instead of every network interface.
* **Changed:** Added an explicit `-addr` option. Non-loopback addresses remain available for intentional use but print a warning that private application data has no authentication boundary.
* **Hardening:** Replaced the package-level default server with a dedicated `http.Server` using read-header, read, write, and idle timeouts.
* **Tests:** Added coverage for default and configured addresses, invalid ports, IPv4 and IPv6 loopback detection, remote-bind warnings, handler selection, and every server timeout.

## 2026-07-27 — Post-transaction backlog grooming

* **Changed:** Re-verified and re-scored every Pending row across the bug, free-improvement, and paywalled-improvement backlogs against current code and live environment evidence.
* **Changed:** Re-scoped bug #125 from a Major bulk-update corruption risk to a Minor durable-manual-correlation gap after bug #124's multi-row rollback safeguard, reducing its score from 1.75 to 0.75.
* **Documentation:** Refreshed the Usability Gate, current monitoring journal, paywalled-grooming scope, and next autonomous item. Nine open Major or Blocker bugs remain; bug #126 is next.
* **Verification:** Re-ran `go build ./...`, `go vet ./...`, and `go test ./...`; all pass.

## 2026-07-27 — Durable email tracker outcomes

* **Fixed:** Rejection and interview emails are acknowledged only when their funnel update commits successfully in the same SQLite transaction.
* **Safety:** Unmatched, no-op, and updated outcomes are reported separately. A company match affecting more than one active application rolls back for manual correlation instead of applying a bulk status change.
* **Tests:** Added transaction coverage for success, no-op and unmatched outcomes, invalid statuses, ambiguous company matches, database locks, acknowledgement failures, and successful retries.

## 2026-07-26 — Post-fix backlog grooming

* **Changed:** Re-verified and re-ranked every Pending bug and improvement against current code and documented live-run evidence; bug #124 remains the next autonomous gate item.
* **Changed:** Reduced improvement #30 to 0.20 after its motivating attestations were configured, kept #14/#27 below the ROI floor with explicit recommendations, and raised paywalled CAPTCHA solving to 1.75 while preserving its paid-key gate.
* **Documentation:** Consolidated the oversized live-monitoring journal to its durable conclusions, unresolved decisions, operating hazards, and current resume point.
* **Verification:** Re-ran `go build ./...`, `go vet ./...`, and `go test ./...`; all pass.

## 2026-07-26 — Free discovery without SerpApi

* **Fixed:** RemoteOK, Hacker News, and public Greenhouse/Lever feed discovery now run whether or not `SERPAPI_API_KEY` is configured.
* **Changed:** Missing SerpApi configuration routes role/ATS search queries directly through the existing Yahoo HTML fallback instead of aborting discovery.
* **Tests:** Added an isolated no-key regression proving free-source results are emitted, Yahoo is used, and SerpApi receives no request.

## 2026-07-26 — Resume upload fallback

* **Fixed:** Resume attachment now resolves a real upload control before reading the source file, so forms without an optional resume field no longer fail on an irrelevant empty or missing path.
* **Changed:** Dynamic and Vision mappings fall back from a bad mapped selector to resume/CV-named inputs, then to a sole non-cover-letter file input. A found control still fails closed on unreadable, empty, or failed uploads.
* **Tests:** Added five focused resume-upload cases and restored the six submitter scenarios regressed by the initial #118 work. The full build, vet, and test loop passes.

## 2026-07-26 — Application sweep (documentation only)

* **Backlog:** Added 12 code- and environment-grounded defects (#118-#129), reopened the unresolved funnel-row half of #112, and moved the Usability Gate back to `UNMET`.
* **Improvements:** Added accessibility/self-containment, outcome-aware queue ranking, and documentation-reconciliation items; refreshed model recommendations against the models currently installed.
* **Journal:** Updated the current resume plan so #118 and the red submitter suite precede another live cohort; removed the superseded 2026-07-21 verification journal after consolidating its remaining context.
* **Verification:** `go build ./...` and `go vet ./...` pass. `go test ./...` has six `pkg/submitter` failures caused by the pre-existing uncommitted #118 resume-upload work; this audit deliberately did not alter that implementation.

## 2026-07-16
* **Security: SSRF Remediation:** Implemented strict route interception (`page.Route("**/*")`) within the Playwright headless browser to categorically block the resolution of `localhost`, local loopback IPs, and AWS Metadata endpoints (`169.254.169.254`).
* **Security: Prompt Injection Blockers:** Integrated the `QuarantineLayer` payload filter into all submission pathways (including the fallback `AttemptSubmit` routine) to neutralize malicious `<!-- Ignore instructions -->` strings hidden in raw DOM before routing to the Gemini API.
* **Architecture: Playwright Concurrency Pool:** Eliminated race-condition crashes and massive CPU overhead by refactoring the pipeline to initialize a single headless Chromium `Browser` instance in `main.go`. All 10 concurrent worker threads now securely spawn lightweight `BrowserContext` sessions from the shared driver pool.
* **Architecture: Encapsulated SQLite Operations:** Removed leaky abstraction layers by refactoring the orchestration pipeline to use strict Repository Pattern methods from `pkg/storage` rather than executing raw SQL queries (`db.Exec`).
* **SRE: Circuit Breaker for Rate Limits:** Integrated global graceful context cancellation (`context.CancelFunc`) so that if the Gemini API encounters a `429 Quota Exceeded` error, all workers are gracefully paused and safely spun down, instead of halting system resources with infinite sleep loops.
* **SRE: Concurrency Control:** Implemented strict connection pooling for SQLite (`SetMaxOpenConns(10)`, `_busy_timeout=5000`) utilizing WAL journal mode, significantly improving database throughput and mitigating `database is locked` panics under parallel scraping loads.
* **SRE: Daemon Mode Memory Fix:** Rewrote the daemon loop architecture to eliminate a dangerous recursive call to `main()` which had been resulting in severe memory leaks and abandoned `defer` statements. Contexts are now properly propagated through OS interrupts.

## Iteration 2 Audit Fixes (2026-07-16)
* **UI/UX & Accessibility:** Rewrote the terminal dashboard (`cmd/dashboard/main.go`). It no longer uses destructive ANSI clear-screen loops (which broke screen readers) and now hosts a clean, modern HTML web interface via standard library `net/http`.
* **Security (Path Traversal):** Hardened `SaveApplication` in `pkg/storage/manager.go` to aggressively strip malicious characters and path separators from `companyName` before allocating file paths.
* **Security (SSRF Upgrade):** Replaced simplistic string-matching anti-SSRF filters in Playwright with true IP resolution via `net.ParseIP`, blocking advanced edge cases like IPv6 `::1`, `0177.0.0.1`, and RFC1918 subnets.
* **Resilience (Race Condition):** Added a `sync.Mutex` lock to `LogFailedSubmission` to prevent interleaved or corrupted data when 10 goroutines write to `manual_submissions.md` concurrently.
* **Resilience (File Deletion):** Fixed an accidental destructive cleanup bug in `AttemptSubmit` where workers would delete the master resume/cover letter from disk instead of generating a copy.
* **Documentation:** Authored comprehensive Architecture Decision Records (`ADR-001`, `ADR-002`, `ADR-003`) detailing our Playwright pool, Prompt Injection, and SQLite logic. Added a `CONTRIBUTING.md` and Mermaid architecture diagram.
