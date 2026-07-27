# Career Agent Core - Changelog

## 2026-07-27 — Capped recurring daemon cycles

* **Fixed:** `cmd/agent --daemon` now runs a fresh database-backlog and discovery cycle every six hours instead of exiting after one batch.
* **Control:** Each daemon cycle processes at most 15 jobs by default. `-cycle-limit` accepts a different positive cap, while ordinary batch mode remains unlimited and exits after one cycle.
* **Shutdown:** The inter-cycle clock listens to the existing signal context, so `SIGINT` and `SIGTERM` stop a waiting daemon promptly.
* **Tests:** Added deterministic injected-cycle and injected-clock coverage for one-shot batches, repeated daemon cycles, per-cycle refresh and caps, invalid configuration, and cancellation.

## 2026-07-27 — Post-quarantine backlog grooming

* **Backlog:** Re-verified and recomputed all 5 Pending bugs, 5 free improvements, and 2 paywalled improvements after bug #121.
* **Live correction:** Boolean-only configuration checks show the authorization and sponsorship answers are blank again, superseding stale backlog notes. Improvement #30 returns from 0.20 to 0.40 and stays below the ROI floor.
* **Priority:** Four Major or Blocker bugs still hold the Usability Gate open. Broken daemon behavior, bug #120, is next.
* **Recommendations:** Re-scope #30 or configure the answers, close #27 absent a real MCP client, defer paid-compute #14 until preference labels exist, and keep CAPTCHA solving gated on a user-selected paid provider and key.
* **Live checks:** Required Ollama models remain installed, both dashboard routes return HTTP 200 on loopback, and no Career Agent batch process is running.
* **Verification:** The uncached build, vet, full test suite, and race checks for the security, agent, and submitter packages pass.

## 2026-07-27 — Deterministic pre-model quarantine

* **Security:** Fetched posting text and relevant browser DOM now cross one deterministic quarantine boundary before embedding, scoring, form mapping, validation solving, or visual mapping.
* **Fail closed:** Prompt-injection detections never reach an LLM safety judge. Their error text omits matched attacker content, while structured findings continue to append to the private CSV audit.
* **Durability:** Blocked jobs move from `PROCESSING` to the terminal `QUARANTINED_PROMPT_INJECTION` status. Browser-time detections receive the same checked checkpoint and funnel update.
* **Coverage:** Spy regressions prove malicious posting payloads cause zero embedding or scoring calls, and malicious initial or dynamically revealed generic, Greenhouse, and Lever DOM causes zero mapper, Vision, solver, or judge calls. Initial detections also occur before document generation.
* **Verification:** The focused race suite and the full build, vet, and test loop pass.

## 2026-07-27 — Post-profile backlog grooming

* **Backlog:** Re-verified and re-scored all 6 Pending bugs, 5 free improvements, and 2 paywalled improvements after bug #129; no score or rank changed.
* **Live correction:** A read-only database recount still finds 20 HTTP/HTTPS duplicate funnel pairs, but 15 now have divergent statuses rather than the stale documented count of 11. Bug #112 and the monitoring journal now use the live count.
* **Priority:** Five Major or Blocker bugs still hold the Usability Gate open. Pre-model quarantine bug #121 is the next autonomous item.
* **Recommendations:** Close free improvements #27 and #30 absent new evidence, defer paid-compute #14 until preference labels exist, and keep CAPTCHA solving gated on a user-selected paid provider and key.
* **Live checks:** Required Ollama models remain installed. Both dashboard routes return HTTP 200 on `127.0.0.1:8080`, and no Career Agent batch process is running.
* **Verification:** `go build ./...`, `go vet ./...`, and `go test ./...` pass.

## 2026-07-27 — Portable, fail-closed career context

* **Fixed:** Removed the developer-specific career-profile path from `cmd/agent` and `cmd/reingest`. Both commands now share flag, environment, repository-local, and sibling-library resolution.
* **Safety:** Agent startup validates the selected profile before consulting cached chunks, rejects an empty or unverifiable RAG rebuild, and no longer falls back to empty context after per-job retrieval failures.
* **Control:** Added explicit `-no-rag` mode for intentional operation without career context. It bypasses both startup ingestion and per-job retrieval instead of silently reusing old chunks.
* **Configuration:** Documented `CAREER_PROFILE_PATH`, `-profile`, default lookup order, and the fail-closed behavior.
* **Tests:** Added path-precedence, missing-file, sibling-layout, non-regular-file, stale-cache, cache-probe, empty-ingestion, and explicit no-RAG coverage.

## 2026-07-27 — Post-fetch backlog grooming

* **Backlog:** Re-verified and re-scored all seven remaining bugs, five free improvements, and two paywalled improvements after bug #123; no score or rank changed.
* **Priority:** Six Major or Blocker bugs still hold the Usability Gate open. Bug #129, portable career-profile path resolution, is the next autonomous item.
* **Recommendations:** Close free improvements #27 and #30 absent new evidence, defer paid-compute #14 until preference labels exist, and keep CAPTCHA solving gated on a user-selected paid provider and key.
* **Live checks:** Required Ollama models remain installed. Both dashboard routes return HTTP 200 on `127.0.0.1:8080`, the non-loopback connection is refused, and no Career Agent batch process is running.
* **Verification:** `go build ./...`, `go vet ./...`, `go test ./...`, an uncached full test run, and the focused agent race suite pass.

## 2026-07-27 — Safe pre-score job fetching

* **Fixed:** Jobs with missing descriptions no longer reach embedding or fit scoring after transport failures, non-success HTTP responses, or pages with too little visible posting content.
* **Retry policy:** Transport errors, response-read failures, HTTP 429, and HTTP 5xx responses receive at most three attempts with one-second and two-second context-cancellable waits. Exhausted failures return to `DISCOVERED`.
* **Terminal policy:** HTTP 404 and 410 responses move to `INVALID_URL`; other non-success responses remain retryable rather than being mistaken for job text.
* **Resource safety:** Every response body closes in its own fetch attempt before retrying or returning. All affected funnel-status writes now report failures.
* **Tests:** Added injected server and HTTP-client coverage for usable and weak 2xx content, terminal and retryable statuses, transport and body-read failures, response closure, bounded waits, cancellation, and CAPTCHA classification. The focused race test and full build, vet, and test loop pass.

## 2026-07-27 — Post-permission backlog grooming

* **Backlog:** Re-verified and re-scored all 8 remaining bugs, all 5 free improvements, and both paywalled improvements against current code and live metadata; no score or rank changed.
* **Classification:** Moved the below-floor LoRA experiment from the free backlog to the paywalled backlog because this host still has only an integrated GPU and useful training requires paid cloud compute.
* **Priority:** Seven Major or Blocker defects still hold the Usability Gate open. Bug #123 is the next autonomous item.
* **Recommendations:** Close free improvements #27 and #30 absent new evidence; defer paid-compute #14 until preference labels exist; CAPTCHA solving remains gated on a user-selected paid provider and key.
* **Verification:** `go build ./...`, `go vet ./...`, and `go test ./...` pass. Both dashboard routes return HTTP 200 on loopback, and the required Ollama models remain installed.

## 2026-07-27 — Owner-only private workspace

* **Security:** Maintained commands now start under an owner-only umask and fail closed with a clear warning if existing private paths cannot be secured.
* **Hardening:** Credentials, SQLite files, logs, source resumes and letters, generated documents, and their directories now use `0600` and `0700` modes. Permission repair opens changed paths without following symbolic links.
* **Operations:** Added the idempotent `go run ./cmd/securefiles` maintenance command and applied it to the live workspace.
* **Tests:** Added coverage for process defaults, recursive repair, repeat runs, symbolic-link refusal, warning propagation, private database creation, and generated artifact modes. The full build, vet, test, and focused race gates pass.

## 2026-07-27 — Post-dashboard backlog grooming

* **Backlog:** Re-verified and re-scored all 9 Pending bugs, all 6 free improvements, and the paywalled CAPTCHA item after dashboard hardening; no rank or score changed.
* **Priority:** Eight Major or Blocker defects still hold the Usability Gate open. Bug #127 remains the next autonomous item at 3.5.
* **Recommendations:** Kept improvements #14, #27, and #30 below the ROI floor with explicit defer or close guidance; CAPTCHA solving remains at 1.75 but still requires a user-selected paid provider and key.
* **Verification:** `go build ./...`, `go vet ./...`, and `go test ./...` pass. The live dashboard still returns HTTP 200 on both routes, listens only on `127.0.0.1:8080`, and refuses the host's non-loopback address.

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
