# Career Agent Core - Changelog

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
