# Career Agent Core - Changelog

## 2026-07-16
* **Security: SSRF Remediation:** Implemented strict route interception (`page.Route("**/*")`) within the Playwright headless browser to categorically block the resolution of `localhost`, local loopback IPs, and AWS Metadata endpoints (`169.254.169.254`).
* **Security: Prompt Injection Blockers:** Integrated the `QuarantineLayer` payload filter into all submission pathways (including the fallback `AttemptSubmit` routine) to neutralize malicious `<!-- Ignore instructions -->` strings hidden in raw DOM before routing to the Gemini API.
* **Architecture: Playwright Concurrency Pool:** Eliminated race-condition crashes and massive CPU overhead by refactoring the pipeline to initialize a single headless Chromium `Browser` instance in `main.go`. All 10 concurrent worker threads now securely spawn lightweight `BrowserContext` sessions from the shared driver pool.
* **Architecture: Encapsulated SQLite Operations:** Removed leaky abstraction layers by refactoring the orchestration pipeline to use strict Repository Pattern methods from `pkg/storage` rather than executing raw SQL queries (`db.Exec`).
* **SRE: Circuit Breaker for Rate Limits:** Integrated global graceful context cancellation (`context.CancelFunc`) so that if the Gemini API encounters a `429 Quota Exceeded` error, all workers are gracefully paused and safely spun down, instead of halting system resources with infinite sleep loops.
* **SRE: Concurrency Control:** Implemented strict connection pooling for SQLite (`SetMaxOpenConns(1)`) utilizing WAL journal mode, significantly improving database throughput and mitigating `database is locked` panics under parallel scraping loads.
* **SRE: Daemon Mode Memory Fix:** Rewrote the daemon loop architecture to eliminate a dangerous recursive call to `main()` which had been resulting in severe memory leaks and abandoned `defer` statements. Contexts are now properly propagated through OS interrupts.
