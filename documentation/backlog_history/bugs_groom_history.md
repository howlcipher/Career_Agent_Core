# bugs.md — Archived Groom-Pass / Status History

Superseded dated status paragraphs moved out of `bugs.md` during the 2026-08-01 backlog-size restructure (see `documentation/task_journals/2026-08-01_optimize-backlog-access.md`). `bugs.md` itself now carries only the single current status paragraph in each section; this file is the full historical record, kept for audit, not read on a normal work session.

## Usability Gate — prior status paragraphs (newest first, as originally written)

**Prior — Re-MET 2026-08-01 (thirty-seventh session, #481 run).** #481 (Major, 2.0) is Done — see its row for the full fix, mutation-check, and live-restart account. `RankJobs` now forces any `DISCOVERED` job at or past 10 days old to the front of the queue regardless of score, fixing the mechanism that let a low-scoring row keep losing to fresher jobs indefinitely until it expired unattempted. **This closes the last open Major/Blocker — the zero-Blocker/Major box re-closes.** **#480** (Minor, 1.67, diagnostic-only) remains the sole open Pending row in this file and does not gate this box. `go build ./...`, `go vet ./...`, `go test ./...` and `gofmt -l ./cmd ./pkg ./internal` all re-run clean. *(Reopened the same day by a mission-alignment audit that found bug #489 — see the current Gate paragraph in `bugs.md`.)*

**Prior — UNMET 2026-08-01 (thirty-sixth session, live-database audit).** No bug was fixed this session — asked to explain why `applications.db` shows no new `APPLIED` rows in a while, a direct query found exactly 2 `APPLIED` rows total, both from 2026-07-29, against 8,140 more `job_funnel` rows processed on 2026-08-01 alone with zero reaching `APPLIED`. Filed two new Pending rows from that investigation: **#481** (Major, 2.0 — the ranking algorithm's freshness decay is too weak to keep aged `DISCOVERED` postings from expiring before they're ever attempted, the measured direct cause of the 3+-day `APPLIED` stall) and **#480** (Minor, 1.67 — `UpdateFunnelStatusRetryable` never records why a job exhausted its retries, unlike its `UpdateFunnelStatusInvalid` sibling). **#481 reopens the zero-Blocker/Major box below and is now the outright top of the combined free queue.** `go build ./...`, `go vet ./...`, `go test ./...` and `gofmt -l ./cmd ./pkg ./internal` all re-run clean — both findings are live-behavior/data gaps, not compile or test regressions.



**Prior — Still MET 2026-08-01 (thirty-fifth session, groom pass after #464).** No bug was fixed or filed this session — the work item, `improvements.md` #464 (`scripts/server.go`'s transpiled body was not `gofmt`-clean), touched only `scripts/server.go` (whitespace/alignment) and `improvements.md`, and surfaced no new defect. This file remains empty of Pending rows of any severity; the three static boxes (`go build`, `go vet`, `go test ./...`, `gofmt -l ./cmd ./pkg ./internal`) all re-ran clean this pass, and the four live end-to-end boxes were not re-run (no live batch, dashboard, or tracker run happened this session) — they rest on their existing dated evidence below. `improvements.md`'s free queue is now a two-way tie at 1.0 (**#448, #442**), ahead of #472/#473 (0.75) and #479 (0.5) — see that file's own groom note.



**Prior — Still MET 2026-08-01 (thirty-fourth session, `/groom_backlogs` pass after #474).** No bug was fixed or filed this session — the work item, `improvements.md` #474 (extending the Working Protocol's close-the-loop step to require an ADR check alongside the existing `CHANGELOG.md` one), touched only `improvements.md` and surfaced no new defect. This file remains empty of Pending rows of any severity; the three static boxes (`go build`, `go vet`, `go test ./...`, `gofmt -l ./cmd ./pkg ./internal`) all re-ran clean this pass, and the four live end-to-end boxes were not re-run (no live batch, dashboard, or tracker run happened this session) — they rest on their existing dated evidence below. `improvements.md`'s free queue is a three-way tie at 1.0 (**#448, #442, #464**), ahead of #472/#473 (0.75) and #479 (0.5) — see that file's own groom note for the exact tie.



**Prior — MET 2026-08-01 (thirty-third session, #478 run).** #478 (Major, 3.0) is Done — `cmd/agent/pipeline.go`'s `StateInit` DNS-failure branch now calls `storage.UpdateFunnelStatusRetryable`, matching the file's other 5 retryable sites. Mutation-checked (reverting the fix alone reproduces the exact `retry_count = 0` / zero `next_eligible_at` symptom) and **live-verified end to end**: rebuilt `career_agent_bin`, gracefully restarted the running production daemon, and confirmed the previously DNS-failing job (`wwww.raileurope.com`) failed exactly once then dropped out of `GetDiscoveredJobs`, with cycle 2 → cycle 3 timed a clean 60 seconds apart — the documented cadence, not the ~1/sec spin the bug reported. **This closes the last open Pending row in this file — zero bugs remain open here of any severity.** `go build ./...`, `go vet ./...`, `go test ./...` and `gofmt -l ./cmd ./pkg ./internal` all re-run clean. With this file empty, the next session goes straight to `improvements.md`'s ranked table.



**Prior — UNMET 2026-08-01 (thirty-second session, live-verifying #477).** No bug was fixed this session — the work item was `improvements.md` #477, and while live-verifying its fix against the real running daemon, a new Major, **#478**, surfaced (a DNS resolution failure never moves a job out of `DISCOVERED`, so one bad hostname spins the daemon at ~1 cycle/sec instead of the documented 1-minute cadence — see its row for the full account). Not yet worked. `go build ./...`, `go vet ./...`, `go test ./...` and `gofmt -l ./cmd ./pkg ./internal` all re-run clean; the finding is a live-behavior gap, not a compile/test regression, same shape as #475's own discovery.



**Prior — Re-MET 2026-07-31 (thirtieth session, #475 run).** #475 is Done — see its row for the full account. `pkg/scraper/discoverWithYahooHTML` now gates on a source-level circuit breaker (`sourceCircuitBreaker`, same shape as improvement #469's `domainCircuitBreaker`) so a sustained failure streak stops burning the 5-way concurrent request budget and 3x retry cost instead of repeating the same doomed request pattern for hours. Mutation-checked and independently reviewed by a second Claude session, which found and this session closed one real gap (the transport-error and body-read-error failure branches weren't proven to feed the breaker). This was the sole thing holding the gate open — **the gate is MET again**, and the zero-Blocker/Major box below closes with it since the only remaining Pending row in this file, **#476**, is Minor. `go build ./...`, `go vet ./...`, `go test ./...` (including `-race` on `pkg/scraper`) and `gofmt -l ./cmd ./pkg ./internal` all re-run clean. `improvements.md`'s Pending rows are fair game for normal ROI-ranked selection again.



**Prior — Still UNMET 2026-07-31 (twenty-ninth session, full `/groom_backlogs` audit, no fix made).** Re-verified #475 against current code (`pkg/scraper/funnel.go`'s `discoverWithYahooHTML` and its caller still match the row's evidence exactly: 3-attempt retry, no source-level breaker, 5-way concurrency, `User-Agent`-only headers) — unchanged, still the sole thing holding this gate open. This pass's fresh audit found two new items, neither of which touches the gate: a corrected documentation claim (the "discovery queue is a one-time snapshot" Operational Trap note below was true when written but has been false for **daemon mode** since the 2026-07-30 continuous-daemon cadence rework — `runAgentQueueCycle` now calls `storage.GetDiscoveredJobs` fresh on every cycle rather than once at process start; correction added inline where the trap is documented, dated and scoped to daemon mode only), and a new Minor bug, **#476** (`GetQueuePlan` has no `rows.Err()` check, so a cursor fault can silently truncate `cmd/requeue`'s dry-run preview — same defect class as #452/#459, different file, does not affect the actual `-confirm` mutation). All seeded findings from this audit's brief (`bugs.md` #475; `improvements.md` #474/#448/#442/#464/#472/#473; `improvements_paywall.md` #424/#17/#14) were re-verified against current code and are unchanged from the prior groom pass. `go build ./...`, `go vet ./...`, `go test ./...` and `gofmt -l ./cmd ./pkg ./internal` all re-run clean.



**Prior — UNMET 2026-07-31 (twenty-eighth session, live-log audit).** No fix was made this session — a scheduled scan of the continuously running daemon's live `career_agent.log` (05:06–15:20, ~10h14m, the full log window) surfaced one new Major, **#475** (Yahoo fallback still fails 77.8% of discovery queries — 4,269 of 5,490 attempts — despite bug #130's retry/backoff fix; the failure rate held steady at ~45–49/minute for the full 8+ hour stretch, so it reads as a sustained block rather than transient blips #130's per-query retry can out-wait). This is the sole open Pending row in this file; it does not block a fresh build/vet/test pass (`go build ./...`, `go vet ./...`, `go test ./...` and `gofmt -l ./cmd ./pkg ./internal` all still re-run clean), but it does reopen the zero-Blocker/Major box below.



**Prior — Still MET 2026-07-31 (twenty-seventh session, post-#450 run).** No bug was fixed or filed this session — the work item, `improvements.md` #450 (splitting the shared SQLite DSN so `cmd/dashboard`'s read-only connection never asks to change `journal_mode`, which SQLite can refuse outright while another connection holds a write transaction), touched `pkg/storage/dsn.go` and `cmd/dashboard/main.go` and surfaced no new defect — the fix was mutation-checked and live-verified against the real `applications.db` with the production agent daemon running concurrently. `go build ./...`, `go vet ./...`, `go test ./...` (including `-race` on `pkg/storage`) and `gofmt -l ./cmd ./pkg ./internal` all re-run clean. This file remains empty of Pending rows.



**Prior — Still MET 2026-07-31 (twenty-sixth session, post-#462 run).** No bug was fixed or filed this session — the work item, `improvements.md` #462 (deduplicating the RAG embedding retry loop's inline four-condition check into a call to `classifyGenerationError`), touched only `cmd/agent/pipeline.go` and was a pure internal refactor with no behavior change, verified by re-running `TestClassifyGenerationError` uncached against the new code. It surfaced no new defect. `go build ./...`, `go vet ./...`, `go test ./...` and `gofmt -l ./cmd ./pkg ./internal` all re-run clean. This file remains empty of Pending rows.



**Prior — Still MET 2026-07-31 (twenty-fifth session, post-#470 run).** No bug was fixed or filed this session — the work item, `improvements.md` #470 (wiring `cmd/requeue`'s `countForStatus` in as real `-status` pre-flight validation), touched `pkg/storage/manager.go` and `cmd/requeue/main.go`, neither of which surfaced a new defect. `go build ./...`, `go vet ./...`, `go test ./...` and `gofmt -l ./cmd ./pkg ./internal` all re-run clean. This file remains empty of Pending rows.



**Prior — Still MET 2026-07-31 (twenty-fourth session, groom pass after #468 run).** No bug was fixed or filed this session — the work item, `improvements.md` #468, touched `pkg/storage`, `cmd/agent/pipeline.go`, and `cmd/dashboard`, none of which surfaced a new defect. `go build ./...`, `go vet ./...`, `go test ./...` and `gofmt -l ./cmd ./pkg ./internal` all re-run clean. This file remains empty of Pending rows.



**Prior — Still MET 2026-07-31 (twenty-third session, #467 run).** #467 (Minor, browser target closure) is Done — `AttemptSubmit` recreates the browser context once, capped, on a target-closed Playwright error, mutation-checked and independently reviewed. **This closes the last open Pending row in this file — zero bugs remain open here of any severity**, same as the eighteenth session's #440 close. `go build ./...`, `go vet ./...`, `go test ./...` and `gofmt -l ./cmd ./pkg ./internal` all re-run clean. With this file empty, the next session goes straight to `improvements.md`'s ranked table.



**Prior — Re-MET 2026-07-31 (twenty-first session, #466 run).** #466 is Done — the queue starvation the 2026-07-30 live-daemon audit found is fixed and verified with mutation-checked tests. That was the sole open Major; #467 (browser target closure, Minor) remained open but did not gate this box. `improvements.md`'s Pending rows are fair game for normal ROI-ranked selection again.



**Prior — 2026-07-30 live-daemon log audit:** the gate went **UNMET** because the continuously running service exposed one new Major defect, #466 (retryable queue rows are immediately reselected and can starve the backlog). #467 (browser target closure can abort an individual application attempt) is Minor. These were the highest-priority bug items ahead of feature improvements.

## Usability Gate checklist — archived "Prior note:" tails

- Prior note: **2026-07-28:** the uncached suite is green; focused race suites for the security, scraper, submitter, and agent packages also pass.
- Prior note: **re-verified 2026-07-27 after #122:** the root page and `/api/metrics` both return HTTP 200 without printing response data. The post-#126 rebuild established the loopback-only listener and refused non-loopback connection.
- Prior note: **UNMET 2026-08-01 (thirty-sixth session, live-database audit):** one new Major, **#481** (the ranking algorithm's freshness decay is too weak to keep aged `DISCOVERED` postings from expiring before they're ever attempted — see the gate note above and its own row for the full account), found while explaining the funnel's 3+-day `APPLIED` stall. Not yet worked. Prior note: **MET 2026-08-01 (thirty-third session, #478 run):** #478 (Major, 3.0) is Done — fixed, mutation-checked, and live-verified against the real running daemon (see gate note above). **This closes the last open Pending row in this file — zero bugs remain open here of any severity.** `go build ./...`, `go vet ./...`, `go test ./...` and `gofmt -l ./cmd ./pkg ./internal` all re-run clean. Prior note: **UNMET 2026-08-01 (thirty-second session, live-verifying #477):** one new Major, **#478** (a DNS resolution failure never moves a job out of `DISCOVERED`, spinning the daemon at ~1 cycle/sec instead of the documented 1-minute cadence), found live-verifying #477's fix against the real running daemon. Not yet worked. Prior note: **Still MET 2026-08-01 (thirty-first session, #476 run):** #476 (Minor, 2.0) is Done — mutation-checked, closing the last open Pending row of any severity in this file (it was Minor, so it never gated this box). `go build ./...`, `go vet ./...`, `go test ./...` and `gofmt -l ./cmd ./pkg ./internal` all re-run clean. Prior note: **Re-MET 2026-07-31 (thirtieth session, #475 run):** #475 (Major, 0.83) is Done — mutation-checked and independently reviewed, closing the last open Major. **#476** (Minor, 2.0) remains open and does not gate this box. `go build ./...`, `go vet ./...`, `go test ./...` and `gofmt -l ./cmd ./pkg ./internal` all re-run clean. Prior note: **UNMET 2026-07-31 (twenty-eighth session, live-log audit):** one new Major, **#475** (Yahoo fallback still fails 77.8% of discovery queries despite bug #130's retry/backoff fix), found via a scheduled scan of the live daemon log. Not yet worked this session. Prior note: **MET 2026-07-31 (twenty-third session, #467 run):** #467 (Minor, 2.5) is Done — mutation-checked and independently reviewed, closing the last Pending row of any severity in this file. `go build ./...`, `go vet ./...`, `go test ./...` and `gofmt -l ./cmd ./pkg ./internal` all re-run clean. Prior note: **MET 2026-07-31 (twenty-first session, #466 run):** #466 (Major, 4.0) is Done — fixed and mutation-checked, closing the one open Major found by the 2026-07-30 live-daemon audit. **#467** (Minor, 2.5) remained open and did not gate this box. Prior note: **MET 2026-07-30 (eighteenth session, #440 run):** #440 (Minor, 1.5 — `scripts/server.go` was the only file in `scripts/` without `//go:build ignore`) is Done: added the tag, matching all 17 siblings; `go build ./...` and `go vet ./...` no longer compile it. **This closes the last open Pending row in this file — zero bugs remain open here of any severity.** The fix surfaced one new low-value finding, filed as `improvements.md` **#464** (1.0) rather than fixed in the same pass: the file's transpiled body still isn't `gofmt`-clean, and the documented `gofmt -l` loop is scoped to `./cmd ./pkg ./internal` on purpose so it can't see `scripts/`. Build, vet, the full test suite, and `gofmt -l ./cmd ./pkg ./internal` all re-run clean. With this file empty, the next session goes straight to `improvements.md`'s ranked table. Prior note: **MET 2026-07-30 (seventeenth session, groom pass after #458):** no bug was fixed or filed this pass — the session's work item, `improvements.md` #458 (verifying the Gemini/OpenAI model-allowlist columns against `agy models`), touched only `documentation/model_allowlist.md` and `improvements.md`, no Go code. This file's one remaining open Pending row, **#440** (Minor, 1.5), was re-verified against current code and is unchanged (`scripts/` still 18 `.go` files, `server.go` still the one exception to `//go:build ignore`, still no caller). Build, vet, the full test suite, and `gofmt -l ./cmd ./pkg ./internal` were all re-run clean. Prior note: **MET 2026-07-30 (ninth session):** #452 is Done, mutation-checked and independently reviewed by a second Claude session. Four open bugs remain, all Minor: **#449** (2.5), **#444** (2.5), **#447** (2.0), **#440** (1.5). Two follow-ups the review surfaced were filed rather than folded in — see `improvements.md`. Prior note: **MET 2026-07-30 (eighth session):** unchanged from the seventh session — no bug was fixed or filed this pass (the session's work item, `improvements.md` #454, was a Working Protocol/CHANGELOG process fix). Five open bugs remain, all Minor, same as last pass: **#452** (2.5), **#449** (2.5), **#444** (2.5), **#447** (2.0), **#440** (1.5). Two of them (#449, #452) had stale line-number citations in this file, corrected this pass without changing their substance or score — see their detail sections. Prior note: **MET 2026-07-30 (seventh session):** #453 is Done, mutation-checked (reverting the fix alone makes its new test fail with the exact scan error the bug predicted). Five open bugs remain, all Minor and none gates this box: **#452** (2.5), **#449** (2.5), **#444** (2.5), **#447** (2.0), **#440** (1.5). `improvements.md`'s top Pending row, **#454** (3.0), is now the outright top of the combined free queue — #453 tied it before this session and is now closed. Prior note: **MET 2026-07-30 (sixth session):** #451 is Done and verified live against the real `applications.db` (Failed tile split 22 `FAILED_SCORE` + 35 `FAILED_SUBMIT`, confirming the old caption was wrong for 22 of 57 jobs). Six open bugs remain, all Minor and none gates this box: **#453** (3.0), **#452** (2.5), **#449** (2.5), **#444** (2.5), **#447** (2.0), **#440** (1.5). `improvements.md`'s top Pending row, **#454** (3.0), ties #453 for the new top of the combined free queue. Prior note: **MET 2026-07-30 (third session):** #445 is Done and verified live against a running dashboard, which closes the last open Major. The four remaining open bugs are all Minor and none gates this box: **#446** (4.0), **#449** (2.5, new this session), **#444** (2.5), **#447** (2.0), **#440** (1.5). Unlike the last four sessions, this fix surfaced no new Major — #449 came out of the live verification rather than the code review, and it is Minor. Prior note: **still UNMET 2026-07-30 (second session):** #437 is Done and verified live, which closed the last Major inherited from previous sessions — but the review pass that closed it found a new one, **#445** (any web page open in the user's browser can `POST` to the dashboard and start or stop the agent; the handlers check only the HTTP method, and bug #126's loopback bind is no defense against a request that originates on the same machine). So the open-Major count is unchanged at **one**, and #445 is now the sole item holding this box open. Also newly open and not gating: **#446** (Minor, 4.0 — the dashboard half of bug #416 was never fixed, though #416 reads Resolved and names both files) and **#447** (Minor, 2.0 — two small `App.tsx` defects). Prior note: **UNMET 2026-07-30:** #441 is Done and verified live, which drops the open-Major count from two to **one**: **#437** (the dashboard rewrite deleted improvement #15's conversion analytics and improvement #34's accessibility markup, both still served by the API and rendering nowhere) at score 3.0. It is now the recommended next item and the only thing holding this box open; the other two open bugs, #444 (Minor, 2.5) and #440 (Minor, 1.5), do not. Unlike the previous three sessions, closing #441 surfaced **no new Major** — its findings were a `gofmt` drift row in `improvements.md`, a note on #437 about the project page advertising the surface #437 deleted, and Minor bug #444 (a transient 429 cancels an entire run, plus seven log lines naming Gemini for provider-generic calls). Prior note: **UNMET 2026-07-29 (late evening):** #439 is resolved and verified live on all three of its routes, but the fix surfaced **#441** (Major — the documented setup path leaves a clean install configured for two models the installer never pulled, which is #439's failure mode reached through the README instead of through hardcoded literals), so the count of open Majors is unchanged at two: **#441** and **#437**. #441 is the recommended next item: it scores 7.0, it is a one-file fix at its narrowest, and it is the only open bug that breaks a brand-new user's very first run. Prior note: **UNMET 2026-07-29 (evening):** two open Majors, **#439** (the tailored-document path hardcodes `llama3`, a model this host does not have, and ignores `LLM_PROVIDER`/`OLLAMA_MODEL` entirely) and **#437** (improvement #426's rewrite silently deleted improvement #15's conversion analytics and improvement #34's accessibility markup, both still served by the API and rendering nowhere). #436, also Major and found the same session, is resolved. Prior note: **re-MET 2026-07-29.** Briefly re-opened the same day by #434 (hand-off statuses were permanent dead ends), which was resolved with `cmd/reconcile` and the tracker eligibility widening, verified live end to end. The two remaining open bugs (#433, #435) are both Minor.

## Post-checklist historical milestones and groom-pass notes

**✅ 2026-07-26 08:31 — FIRST GENUINELY CONFIRMED APPLICATION.** Akuity (Greenhouse, fit 85) completed end to end: form filled to `invalid fields: 0`, submit accepted, security-code challenge detected, the real code retrieved over IMAP, entered across Greenhouse's **eight single-character boxes**, resubmitted, and confirmed — URL moved to `/confirmation`, document collapsed 126,557 → 14,250 chars, confirmation phrase present. `job_funnel.status = APPLIED` with the `applied_jobs` row timestamped `08:31:02.600`, matching the confirmation to the millisecond (#94: the row is written **only** on confirmed submission). **This is the first `APPLIED` row in the database's history** — the count was 0 across 3,884 rows at the start of this session, and the log's `Submission confirmed` count went 0 → 1.



**This resolved the open question from the earlier 82-job verification journal, which has since been consolidated into the current monitoring journal and this backlog.** The historical `APPLIED` rows were not genuine, and the cause was never an inability to fill forms: the pipeline submitted without being able to detect that it had (#95, #102, #111), and could not complete the out-of-band code challenge (#113, #115, #116).



**Re-verified MET 2026-07-25 (`/groom_backlogs`):** `go build ./...` and `go vet ./...` both clean, `go test ./...` green across all 10 test-bearing packages with 0 failures, re-run after the day's #23/#61/#62 commits. Zero Pending rows remain in the Ranked Backlog (bugs #61 and #62 were both filed and resolved the same day). Gate still holds on every box.



**Status after #120 resolution: UNMET.** Build, vet and the full test suite are clean, but 3 open Blocker/Major defects remain. Bug work continues to outrank `improvements.md`.



**Status after #112 resolution: MET.** URL scheme deduplication and migration is verified, resolving the final open Major defect. The gate is clear, and the agent may now proceed to features in `improvements.md`.



**Re-verified MET 2026-07-28 (`/groom_backlogs`):** `go build ./...`, `go vet ./...`, and `go test ./...` are all clean without CGO enabled following the pure Go SQLite migration. Zero Pending rows remain in the Ranked Bug Backlog. The gate is clear.



**Re-verified MET 2026-07-29 (`/groom_backlogs`):** `go build ./...`, `go vet ./...`, and `go test ./...` pass clean. Zero Pending rows remain in the Ranked Bug Backlog. The gate is clear.



**Groom pass, 2026-07-30 (twentieth session, post-#460 run):** `improvements.md`'s #460 is Done — the dashboard now shows a non-alarming `role="status"` notice after two consecutive `/api/metrics` poll failures, clearing again on recovery, mutation-checked and verified live against a second dashboard instance on `127.0.0.1:8099` (the production instance on `:8080` untouched). Closing #460 surfaced a new bug in this file, fixed the same session: **#465** (Minor) — `internal/backlog`'s "guard the guard" floor in `TestPendingBacklogRowsNameRealModels` was a hardcoded historical snapshot (`checked < 20`) rather than a live measurement, and marking #460 Done dropped the real count to 18, turning `go test ./...` red for a reason that had nothing to do with model IDs or parsing. Fixed by deriving the floor independently (a plain substring count, sharing no code path with the table parser), mutation-checked to confirm it still catches a genuine parser regression. `bugs.md` otherwise remains at zero other Pending rows. `go build ./...`, `go vet ./...`, `go test ./...` and `gofmt -l ./cmd ./pkg ./internal` all re-run clean. **`improvements.md`'s free queue is now #456 (1.5, user decision, not autonomous) at the top, then the five-way 1.0 tier (#450, #448, #442, #464, #462)** — see that file's own note for the exact tie.



**Groom pass, 2026-07-30 (nineteenth session, post-#463 run):** #463 (`improvements.md`) is Done — `cmd/dashboard/ui` now has `vitest`/`@testing-library/react`/`@testing-library/jest-dom`/`jsdom` and six real tests covering the poll sequence guard and the start/stop `actionError` states, mutation-checked live in this session (reverting the metrics sequence guard alone reproduces the exact stale-data symptom and fails the new test). `bugs.md` remains empty — re-verified, zero Pending rows, all four static/live checks and the zero-Blocker/Major box still hold. This pass's finding: **#462** had a full detail section in `improvements.md` but no row in the Ranked Backlog table — invisible to `/work_next_item`, which reads only the table — added as a table row this pass rather than left for a future session to rediscover. Re-verified every other open Pending row against current code: **#456** (1.5, user decision) unchanged; **#450** (1.0) unchanged — `pkg/storage/dsn.go:23` still orders `journal_mode(WAL)` ahead of `busy_timeout(5000)`; **#448** (1.0) unchanged — `.oxlintrc.json` still has no `ignorePatterns`; **#460** (1.33) unchanged — `fetchMetrics` (`App.tsx:143`) still only calls `setMetrics` inside `if (res.ok)`, with no visible poll-failure indicator; **#442** (1.0) unchanged — `NLP_SERVICE_URL` (`pkg/mcp/client.go:251`) still gates the same unmeasured opt-in offload; **#464** (1.0) unchanged — `gofmt -l scripts/server.go` still names the file. `improvements_paywall.md`'s **#424**, **#17**, and below-floor **#14** re-checked structurally (no cloud-offload DOM path, no `2captcha`/`capsolver` integration, `lspci` still shows only the integrated Radeon Vega) and unchanged. No row crossed the 0.5 floor in either direction. No journals were outstanding to clean (the #463 journal was deleted in its own closing commit). `go build ./...`, `go vet ./...`, `go test ./...` and `gofmt -l ./cmd ./pkg ./internal` all re-run clean. **`improvements.md`'s #456/#463 tie is now resolved** since #463 is Done — **#456** (1.5) is the outright top of the table but is a user decision and not autonomous work; **#460** (1.33) is the highest-scoring autonomous item and is the next recommended pick, ahead of the five-way 1.0 tier (**#450, #448, #442, #464, #462**).



**Groom pass, 2026-07-30 (eighteenth session, post-#440 run):** #440 (`bugs.md`) is Done — `scripts/server.go` now carries `//go:build ignore`, matching all 17 siblings; it no longer compiles into `go build ./...`/`go vet ./...`. **`bugs.md` now holds zero Pending rows of any severity** — the first time since this file began that its Ranked Backlog is genuinely empty rather than merely gate-clear. The fix surfaced one new low-value finding, filed as `improvements.md` **#464** (1.0) rather than folded into #440's own fix: `scripts/server.go`'s transpiled body still isn't `gofmt`-clean, invisible to the documented `gofmt -l ./cmd ./pkg ./internal` loop because `scripts/` is deliberately out of its scope. Re-verified every other remaining open Pending row against current code this pass, all unchanged: `improvements.md`'s **#456** (1.5, user decision), **#463** (1.5) — `package.json` still has no `vitest`/`@testing-library` dependency, **#460** (1.33) — `fetchMetrics` (`App.tsx:138-148`) still only calls `setMetrics` inside `if (res.ok)`, with the new `actionError` state (from #447) covering only `handleStart`/`handleStop`, not the poll loop, **#450** (1.0) — `pkg/storage/dsn.go:23` still orders `journal_mode(WAL)` ahead of `busy_timeout(5000)`, **#448** (1.0) — `.oxlintrc.json` still has no `ignorePatterns`, **#442** (1.0) — `NLP_SERVICE_URL` (`pkg/mcp/client.go:251`) still gates the same unmeasured opt-in offload. `improvements_paywall.md`'s **#424**, **#17**, and below-floor **#14** re-checked structurally (no dedicated DOM-offload hybrid path distinct from the general provider abstraction, no `2captcha`/`capsolver` integration, `lspci` still shows only the integrated Radeon Vega) and unchanged. No row crossed the 0.5 floor in either direction. No journals were outstanding to clean (the #440 journal was deleted in its own closing commit). `go build ./...`, `go vet ./...`, `go test ./...` and `gofmt -l ./cmd ./pkg ./internal` all re-run clean. With `bugs.md` empty, **`improvements.md`'s #456/#463 (tied at 1.5) are now the outright top of the combined free queue** — #456 is a user decision and cannot be taken autonomously, so **#463 is the next autonomous item**.



**Groom pass, 2026-07-30 (seventeenth session, post-#458 run):** #458 (`improvements.md`) is Done — `agy models` confirmed `gemini-3.6-flash-high`, `gemini-3.1-pro-high`, and `gpt-oss-120b-medium` live, and `documentation/model_allowlist.md`'s provenance for all three now reads "Confirmed live via `agy models`, 2026-07-30". The other three OpenAI values (`gpt-5.6-terra`/`gpt-5.6-sol`/`gpt-5.6-luna`) could not be closed the same way — no `OPENAI_API_KEY` exists on this machine and none of the three appear in `agy models`' output — so their provenance now says exactly that instead of a blanket "not vendor-verified", and no currently Pending row names an OpenAI-column model, so the gap has zero live blast radius. Re-verified every remaining open Pending row against current code this pass, all unchanged: `bugs.md`'s **#440** (1.5) — `scripts/` still 18 `.go` files, `server.go` still the one exception to `//go:build ignore`, no caller. `improvements.md`'s **#456** (1.5, user decision), **#463** (1.5), **#460** (1.33) — `fetchMetrics` still only calls `setMetrics` inside `if (res.ok)`, **#450** (1.0) — the shared DSN pragma string still orders `journal_mode(WAL)` ahead of `busy_timeout(5000)`, **#448** (1.0) — `.oxlintrc.json` still has no `ignorePatterns`, **#442** (1.0) — `NLP_SERVICE_URL` still gates the same opt-in offload (line numbers drifted slightly from earlier notes' `:244/:302/:532` to `:251/:314/:539`-ish; substance unchanged). `improvements_paywall.md`'s **#424**, **#17**, and below-floor **#14** re-checked structurally (no cloud-offload DOM path, no CAPTCHA-solving integration, GPU still integrated-only) and unchanged. No row crossed the 0.5 floor in either direction. No journals were outstanding to clean (the #458 journal was deleted in its own closing commit). `go build ./...`, `go vet ./...`, `go test ./...` and `gofmt -l ./cmd ./pkg ./internal` all re-run clean. `bugs.md`'s **#440** (1.5) is now the outright top of the combined free queue — the first time a bug, not an improvement, has held that spot since #443/#458 overtook it.



**Groom pass, 2026-07-30 (sixteenth session, post-#443 run):** re-verified every open Pending row against current code rather than its own prose. `bugs.md`: **#440** (1.5) unchanged — `scripts/` still holds 18 `.go` files, `server.go` still the one exception to `//go:build ignore`, still no caller. `improvements.md`: **#458** (2.0) unchanged — `documentation/model_allowlist.md`'s `google`/`openai` sections are still explicitly marked "not vendor-verified"; **#443** (2.0) is now Done (see its row) and drops out of the queue, correcting a stale diff-content claim in the process; **#463** (1.5, filed this session) unchanged; **#456** (1.5) unchanged, still a user decision; **#460** (1.33) unchanged — `fetchMetrics` still only calls `setMetrics` on `res.ok`, with no persistent-failure banner (distinct from #447's `actionError`, which only covers start/stop clicks); **#450** (1.0) unchanged — `pkg/storage/dsn.go` still asks every connection to set `journal_mode(WAL)`; **#448** (1.0) unchanged — `.oxlintrc.json` still has no `ignorePatterns`, confirmed live via `npm run lint` still walking into `dist/`; **#442** (1.0) unchanged. `improvements_paywall.md`: **#424**, **#17**, and below-floor **#14** all re-checked structurally (no cloud-offload DOM path, no CAPTCHA-solving integration, GPU still integrated-only) and unchanged. No row crossed the 0.5 floor in either direction. No journals were outstanding to clean. **#458 is now the outright top of the combined free queue** at 2.0, unchallenged since #443 closed.



**Status after the fifteenth 2026-07-30 session (#447 run): still MET.** #447 (the dashboard UI silently swallowed failed start/stop clicks, and a slow poll could overwrite fresher metrics with stale data) is Done — outright top of the combined free queue at 2.0, tied with `improvements.md` #458 and #443, picked as the sharper bug per this file's own "bugs generally outrank improvements of similar effort" rule. `handleStart`/`handleStop` now catch and surface failures via a new `actionError` state; the poll loop gained a sequence-number guard so a late-resolving response can no longer land after a fresher one. **Verified live**, not just by type-check: a second dashboard instance on `127.0.0.1:8099` returned a real `500` from `/api/agent/start`, and the built bundle was confirmed to catch it and render the new error message; the production dashboard on `:8080` was untouched throughout. `go build ./...`, `go vet ./...`, `go test ./...`, `tsc -b` and `oxlint src` all clean. Remaining open bugs: **#440** (1.5) — Minor, so the zero-Blocker/Major box still holds. No new bug was filed this session.



**Status after the fourteenth 2026-07-30 session (#444 run): still MET.** #444 (a bare 429 was classified as a fatal daily-quota condition on every provider, and the shutdown log always blamed "Gemini" regardless of `LLM_PROVIDER`) is Done — outright top of the free queue at 2.5, no longer tied with #449 since #449 shipped last session. Fixed with a `classifyGenerationError` helper: only Gemini's own `"Quota exceeded"` wording is treated as fatal now; a bare `429` joins the existing network-error retry branch on every provider, including Gemini itself, whose SDK also returns 429 for its per-minute limit. The two call sites that could call `deps.Cancel()` now name the live provider via a new `Client.ProviderName()` instead of a literal, and `pkg/submitter/vision.go`'s five Gemini-branded log lines were fixed the same way through a soft type-assertion so the `FormMapper` interface did not need to change. **Mutation-checked**: reverting the classification alone reproduces the exact bug and fails the new test by name. `go build ./...`, `go vet ./...`, `go test ./...` and `gofmt -l` all clean. Remaining open bugs: **#447** (2.0), **#440** (1.5) — neither Blocker nor Major, so the zero-Blocker/Major box still holds. No new bug was filed this session.



**Status after the thirteenth 2026-07-30 session (#449 run): still MET.** #449 (`pgrep -f`/`pkill -f career_agent_bin` misidentifying the agent by command-line substring) is Done — tied for the top of the combined free queue at 2.5 with #444, picked as the sharper of the two. Fixed by having the dashboard and agent share bug #414's existing single-instance lock file instead of matching process command lines: `cmd/agent` now writes its own PID into `applications/career_agent.lock` once it holds the flock, and `cmd/dashboard` determines "is the agent running" by probing that same lock rather than shelling out to `pgrep`/`pkill`. **Verified live** against the real dashboard binary — a decoy process carrying the old trigger string no longer produces a false `running: true`, and `/api/agent/stop` now signals only the genuine lock-holder's PID, leaving a concurrently-running decoy untouched. `go build ./...`, `go vet ./...` and `go test ./...` all clean, including 8 new tests (2 mutation-checked). Remaining open bugs: **#444** (2.5), **#447** (2.0), **#440** (1.5) — none Blocker or Major, so the zero-Blocker/Major box still holds. No new bug was filed this session.



**Status after the eleventh 2026-07-30 session (#461 run): still MET.** #461 (git-tracked `career_agent_bin` growing the repo on every rebuild) is Done — outright top of the combined free queue at 3.0, ahead of every remaining open bug and improvement row. `git rm --cached career_agent_bin` plus two new `.gitignore` entries (`career_agent_bin`, `dashboard_bin`); **verified live** by rebuilding both binaries locally and confirming `git status` shows neither as untracked or modified afterward. `go build ./...`, `go vet ./...` and `go test ./...` all clean. Remaining open bugs unchanged from the prior pass: **#449** (2.5), **#444** (2.5), **#447** (2.0), **#440** (1.5) — none Blocker or Major, so the zero-Blocker/Major box still holds. No new bug was filed this session; this was a small, self-contained hygiene fix with no review pass.



**Status after the third 2026-07-30 session (#445 run): MET (2026-07-30).** Every box is now checked. `go build ./...`, `go vet ./...` and `go test ./...` are clean across all 12 test-bearing packages including the 12 new dashboard tests; `gofmt -l cmd/dashboard/` is empty. The zero-Blocker/Major box closed because #445 shipped and was **verified live against a running dashboard rather than by tests alone** — six checks on `127.0.0.1:8099`, each of which would have succeeded before the fix. The Ollama, tracker and live-batch boxes were not re-verified this session and rest on their existing dated evidence.



From this point `improvements.md`'s Pending rows are fair game for normal ROI-ranked selection again.



**Updated after the fifth 2026-07-30 session (model-accuracy groom pass): the gate is still MET.** `go build ./...`, `go vet ./...` and `go test ./...` all exit 0 across all 12 test-bearing packages. The three static boxes were re-verified directly this pass; the Ollama, dashboard, tracker and live-batch boxes were not re-run and rest on their existing dated evidence. **No bug was fixed or filed this session** — all seven open Minor bugs (**#451** 4.0, **#453** 3.0, **#452** 2.5, **#449** 2.5, **#444** 2.5, **#447** 2.0, **#440** 1.5) were re-verified against current code and every one still reproduces, so the ranking is unchanged and **#451 remains the next item**. None is Blocker or Major, so the zero-Blocker/Major box holds. The improvement that tied #451 last pass, **#455**, shipped during this pass and is Done; its successor **#456** scores 1.5 and is a user decision, so #451 no longer ties anything and is the outright top of the free queue.



Two rows carry **line-number drift** found while re-verifying, recorded here rather than silently corrected because it is a reminder that cited anchors age faster than the defects they point at: **#449** cites `:615/:637/:643` but the actual `pgrep`/`pkill -f career_agent_bin` call sites are `:628/:635/:650/:656` — **four, not three** — and **#452** cites nine line numbers that have all moved 2-18 lines (actual: 405, 425, 442, 465, 488, 515, 534, 558, 575, 592, 609). Both defects are otherwise exactly as described; #452's central claim was re-counted and holds — 9 `g.Go` closures, result still discarded at `:612`.



**Prior — updated after the fourth 2026-07-30 session (#446 run):** the gate is still MET — build, vet and the full suite are clean at exit 0 across all 12 test-bearing packages, including 7 new DSN tests. #446 is Done. Seven Minor bugs are now open (**#451** 4.0, **#453** 3.0, **#452** 2.5, **#449** 2.5, **#444** 2.5, **#447** 2.0, **#440** 1.5) — three more than before, because the fix's review and verification each produced findings. **None is Blocker or Major, so the zero-Blocker/Major box holds.** The next session should start with **#451**. Note that for the first time an improvement row now ties the top bug: **#455** (4.0, the model columns name models that no longer exist).



**The session's finding, and it is a new shape for this backlog.** Every previous session's lesson has been about *not trusting prose* — a Done note, a bug report, a code comment, a row's arithmetic. This one came from a value nobody was looking at. The live verification script asked six questions about the new guard; the answer that mattered was in a **seventh, uninteresting line** printed only as a control: `/api/agent/status` returned `{"running":true}` on a host with no agent. That is #449, a defect older than #445 and unrelated to it, and no test would ever have found it because the test suite does not run `pgrep` against a real process table. Stated as a rule: **run the control case, and read it.** A verification that only checks the thing you changed can only ever confirm or deny the thing you changed.



There is a second, sharper edge to it. The control line was a false positive produced *by the verification script itself* — `pgrep -f` matched the shell command that was doing the testing, because that command mentioned the binary's name. The measurement perturbed the thing it measured, and that artifact was the bug.



**Status after the second 2026-07-30 session (#437 run): UNMET, still one open Major — but a different one.** Every static check is clean and the dashboard's conversion analytics are visible again for the first time since `0028b2f`, verified live against the real database rather than by tests alone. #437's closure retires the last Major carried over from earlier sessions; the parallel review pass that ran alongside it filed **#445** (Major, 3.5 — unauthenticated cross-origin `POST` can start or stop the agent), which takes its place as the only thing holding the zero-Major box open. Open: **#445** (Major, 3.5), **#446** (Minor, 4.0), **#444** (Minor, 2.5), **#447** (Minor, 2.0), **#440** (Minor, 1.5). **#446 outscores #445 but does not gate the box**; #445 is the recommended next item on severity and kind. `improvements.md` gained **#448** (1.0 — the UI linter lints its own committed build output). Note the pattern break: three sessions running had said "this session's fix surfaced a new Major", the #441 session was the first that did not, and this one resumes it — but from a *review pass*, not from the fix itself.



**Status after the 2026-07-30 session (#441 run): UNMET, but one open Major rather than two.** Every static check is clean, `.env.example`/`install_ollama.sh`/`install_ollama.ps1`/`cmd/agent` now agree on which models exist, and the Ollama gate box is verified by the product itself instead of by hand. Open: **#437** (Major, 3.0), **#444** (Minor, 2.5) and **#440** (Minor, 1.5). **#437 is the recommended next item** and is the last thing holding the zero-Major box open. `improvements.md` gained one Pending row (**#443**, 2.0 — eight files are not `gofmt`-clean). This is the first session in four whose fix did not surface a new Major.



**Status after the 2026-07-29 late-evening session (#439 run): UNMET.** Every static check is clean and the tailored-document path now works end to end on this host for the first time, verified by three real generations rather than by tests alone. The open-Major count is unchanged at two, because fixing #439 surfaced **#441** — the documented setup leaves a fresh install configured for models the installer never pulled, the same failure as #439 one layer out. Open: **#441** (Major, 7.0), **#437** (Major, 3.0), **#440** (Minor, 1.5). **#441 is the recommended next item.**



**Status after the 2026-07-29 evening session (#435 run): UNMET.** Build, vet and the full suite are clean *and now clean from a fresh clone too* (#436), but two open Majors remain: **#439** (the tailored-document path is configured for a model that is not installed) and **#437** (the dashboard rewrite deleted two shipped improvements' entire user-facing surface). Bug work outranks `improvements.md` again — though `improvements.md` has zero Pending rows anyway, so #439, #437 and #433 were the whole queue. **#439 is the recommended next item**: it is the only one of the three that stops a documented feature from working at all.



**All three of this session's new Majors were found by opening a file for an unrelated Minor fix, and none was reachable from the test suite.** #436 (fresh clone cannot build) needed an actual `git clone`; #437 (deleted analytics) needed reading a rewrite's diff rather than its Done note; #439 (hardcoded `llama3`) needed querying the live Ollama tags endpoint rather than trusting a code comment that said `// Can be read from environment`. Three generalisations, each cheap and each of which would have caught its bug earlier:



- **`go build ./...` in a dirty working tree is not the same check as `go build ./...` in a clone.** Untracked artifacts make the local check strictly weaker than a user's.

- **A rewrite's Done note describes what was added, never what was dropped.** Diff a rewrite against what it replaced before trusting it; for an API-backed surface, the response struct is a ready-made checklist of what the old view consumed.

- **A `// TODO`-shaped comment next to a hardcoded literal is a bug report, not a note.** `// Can be read from environment` sat directly above `"model": "llama3"` and marked the exact defect. Grep for the honest comments; they are load-bearing.



**Historical status as of 2026-07-24 ~15:50: MET.** At that point all six gate boxes were checked: static checks, Ollama, dashboard, tracker crash-safety, a live end-to-end application and zero known open Blocker/Major bugs were confirmed. The later first genuinely confirmed application resolved the old 82-job journal's core question; that superseded journal was removed in the 2026-07-26 sweep, with current live-run context retained in `documentation/task_journals/2026-07-25_monitor-live-run-and-fix-bugs.md`. The current gate status is the newer UNMET line above.



Every session — Claude Code, Gemini CLI, or manual — that touches this repo should glance at this checklist. When the last box is checked, change the Status line to `MET (YYYY-MM-DD)` and add a one-line note on what was verified; from that point on, `improvements.md`'s Pending rows become fair game for normal ROI-ranked selection instead of being blocked behind this gate.

## Ranked Backlog — prior groom-pass paragraphs (newest first, as originally written)

**Prior — Groom pass, 2026-08-01 (thirty-seventh session, post-#481 run):** #481 is Done — see its row and the Usability Gate note above for the full fix, mutation-check, and live-restart account. Live-verifying it surfaced one new Minor, **#482** (1.33 — every remaining `DISCOVERED` row on the live database is a `breezy.hr` posting, a source `GetDiscoveredJobs` excludes entirely, so those rows never reach any terminal status and accumulate forever), filed rather than fixed this pass — low urgency at 185 rows today. **#480** (1.67) was re-verified against current code and is unchanged — `UpdateFunnelStatusRetryable` (`pkg/storage/manager.go:1246-1272`) still takes only a URL, still no `status_reason` parameter, all 6 `cmd/agent/pipeline.go` call sites unchanged. **#480 (1.67) is now the outright top of this file** and, since `improvements.md`'s own top row (#448/#442, tied at 1.0) scores lower, **the top of the combined free queue** — the first time in several sessions the next recommended item is a Minor bug rather than a Major one or an improvement. `go build ./...`, `go vet ./...`, `go test ./...` and `gofmt -l ./cmd ./pkg ./internal` all re-run clean. *(Superseded the same day by a mission-alignment audit that filed bug #489 — see the current paragraph in `bugs.md`.)*

**Session thirty-six, 2026-08-01 (live-database audit, no fix made):** asked to explain why `applications.db` shows no new `APPLIED` rows in a while. `job_funnel` has exactly 2 `APPLIED` rows total, both from 2026-07-29 08:33; 2026-08-01 alone processed 8,140 more rows and none reached `APPLIED` — 4,657 `QUARANTINED_PROMPT_INJECTION`, 1,513 `INVALID_URL` (1,319 `expired`), 1,284 `RETRY_EXHAUSTED`. Filed **#481** (Major, 2.0 — `RankJobs`'s freshness decay is too gentle to keep aged `DISCOVERED` rows from expiring before their first processing attempt; ten spot-checked `expired` rows today were all discovered 2026-07-13, ~19 days earlier) and **#480** (Minor, 1.67 — `UpdateFunnelStatusRetryable` never persists a `status_reason`, so every `RETRY_EXHAUSTED` row is diagnostically blank). **#481 is now the sole Pending row in this file and the outright top of the combined free queue**, reopening the zero-Blocker/Major Usability Gate box. `go build ./...`, `go vet ./...`, `go test ./...` and `gofmt -l ./cmd ./pkg ./internal` all re-run clean.



**Prior — Session thirty-two, 2026-08-01 (#477 run, live-verification finding):** `improvements.md` #477 (Yahoo fallback headers/cookie jar) is Done — see its own row and detail section. This session's work item was in `improvements.md`, not this file, but live-verifying it against the real running daemon (rebuild, graceful restart, before/after log comparison) surfaced a new Major, **#478** (a DNS resolution failure never moves a job out of `DISCOVERED`, so one bad hostname spins the daemon at ~1 cycle/sec instead of the documented 1-minute cadence). Filed, not fixed this session — out of scope for #477's own work. **#478 is now the sole Pending row in this file and the outright top of the combined free queue**, reopening the zero-Blocker/Major Usability Gate box. `go build ./...`, `go vet ./...`, `go test ./...` and `gofmt -l ./cmd ./pkg ./internal` all re-run clean.



**Prior — Session thirty-one, 2026-08-01 (#476 run):** #476 is Done — see its row for the full fix and mutation-check account. **This file now holds zero Pending rows of any severity again** — the third time that has happened, after the eighteenth session's #440 close and the twenty-third session's #467 close. The Usability Gate stays **MET**; none of the four live end-to-end boxes were re-run this session (no live batch, dashboard, or tracker run happened). With this file empty, the next session goes straight to `improvements.md`'s ranked table — its current top Pending row is **#477** (1.5). `go build ./...`, `go vet ./...`, `go test ./...` and `gofmt -l ./cmd ./pkg ./internal` all re-run clean.



**Prior — Session thirty, 2026-07-31 (#475 run):** #475 is Done — see its row for the full fix, mutation-check, and independent-review account. **#476** (Minor, 2.0) is now the sole open Pending row and the outright top of the combined free queue, since `improvements.md`'s Pending rows are fair game again with the gate re-MET. `go build ./...`, `go vet ./...`, `go test ./...` (including `-race` on `pkg/scraper`) and `gofmt -l ./cmd ./pkg ./internal` all re-run clean.



**Prior — Full `/groom_backlogs` audit, 2026-07-31 (twenty-ninth session):** re-verified **#475** (0.83, unchanged — see the Usability Gate note above) and filed **#476** (Minor, 2.0 = 4×1.0÷2, standard tier — `GetQueuePlan` never checks `rows.Err()` after its scan loop, so a cursor fault silently truncates `cmd/requeue`'s dry-run preview; see its detail section). Both are ranked above the improvements queue while the gate is unmet, and **#475 is ranked first despite #476's higher raw score** because a Major outranks a Minor regardless of score while the gate is open — #476 is next once #475 is resolved. `go build ./...`, `go vet ./...`, `go test ./...` and `gofmt -l ./cmd ./pkg ./internal` all re-run clean.



**Prior — Live-log audit, 2026-07-31 (twenty-eighth session):** filed **#475** (Major, 0.83 = 5×0.5÷3, standard tier) — Yahoo fallback discovery still fails 77.8% of its queries (4,269 of 5,490) after bug #130's retry/backoff fix, sustained at a steady rate for the full 8+ hour log window rather than in bursts, which is the evidence that this needs a source-level circuit breaker (reusing improvement #469's pattern) rather than more per-query retrying. Not yet worked. This is the only Pending row in the file, so it is the outright top of the free queue and now outranks everything in `improvements.md` per this file's own header rule. `go build ./...`, `go vet ./...`, `go test ./...` and `gofmt -l ./cmd ./pkg ./internal` all still re-run clean — the finding is a live-behavior gap, not a compile/test regression.



**Prior — Groom pass, 2026-07-31 (twenty-third session, post-#467 run):** #467 is Done — see its row and detail section for the full fix, test, mutation-check, and independent-review account. **This file now holds zero Pending rows of any severity — the second time that has happened, after the eighteenth session's #440 close.** `go build ./...`, `go vet ./...`, `go test ./...` and `gofmt -l ./cmd ./pkg ./internal` all re-run clean. The Usability Gate stays **MET**; none of the four live end-to-end boxes were re-run this pass (no live batch, dashboard, or tracker run happened this session), so they rest on their existing dated evidence. With this file empty, the next session goes straight to `improvements.md`'s ranked table — its current top row is **#468** (1.67); this session's fix also filed three new follow-up rows there (see `improvements.md`'s own groom note) from scope boundaries the independent review confirmed were reasonable to leave for later: the cached-form-mapping fast path, the emailed-security-code resubmit click, and the Vision submission paths do not share #467's new target-closed recovery.



**Prior — Groom pass, 2026-07-31 (twenty-second session, post-#469 run):** `improvements.md`'s #469 is Done — per-domain circuit breakers now gate `checkJobAlive`/`fetchJobPage`, shared across workers and daemon cycles, with a same-session independent review catching and fixing three real defects before commit (a retry-budget-corruption path, a cooldown-reset race, and a fetch-classification miscategorization). This file's own one Pending row, **#467** (Minor, 2.5, standard tier — Playwright target closure aborts an application attempt without bounded browser recovery), was re-verified against current code and is unchanged: no bounded context/browser recreation path exists in `pkg/submitter/browser.go` for a closed target, and this session's diff never touched `pkg/submitter`. Not above-floor-flagged, just the sole remaining open bug. `go build ./...`, `go vet ./...`, `go test ./...` (including `-race` on `cmd/agent` and `pkg/storage`) and `gofmt -l ./cmd ./pkg ./internal` all re-run clean. The Usability Gate stays **MET** — none of the four live end-to-end boxes were re-run this pass (no live batch, dashboard, or tracker run happened), so they rest on their existing dated evidence; the zero-Blocker/Major box holds since #467 is Minor.



**2026-07-23 groom-pass note:** this Score column had never actually been populated for any bug row before this pass (every row showed `—`). Filled in for the 5 currently-Pending rows (#4, #8, #10, #14, #39): for the four "fix applied, verified firing live" Major bugs, Effort is scored 1 (the code is already written and shipped — the only remaining work is a clean observation window, not new engineering), with Value 6-7 reflecting that each gates the Usability Gate's live-batch checkbox. #39 (Minor, root cause not yet found) scores Effort 3 to reflect the standalone-diagnostic-script work still needed. All land well above the 0.5 floor, consistent with the gate's existing blanket rule that these outrank every `improvements.md` row regardless of exact score.



**2026-07-25 groom-pass note (live-monitoring session, updated at close of the #70-#76 run):** every bug row is Resolved. Seven bugs (#70-#76) were filed and fixed in this session, **all on the terminal submit path, none reachable from the test suite** — they fire only against a real ATS form that has already bounced once. Two structural lessons came out of it, both now pinned by tests:

1. **A fix in the validation-retry path is not a fix in the initial-fill path.** Seen twice: #65/#66 → #67, and #74 → #75. Check both before calling any control-setting change done.

2. **A shipped fix is not a working fix until something observes it firing.** #76 was a defect in #74 that made #74 *and* #75 completely inert, with passing tests and no error anywhere; it was caught only by noticing an expected log line was **absent**. After any fix whose purpose is to fire on a specific condition, confirm at runtime that it announces itself.



Ranking is otherwise unchanged and no re-scoring was warranted. `improvements.md` gained **#28** (score 2.33) from this session's findings — filling Greenhouse's required Location/Country on the first pass, which would remove a guaranteed ~12-minute retry cycle per Greenhouse posting; it is blocked on a `pii.yaml` schema change only the user can populate, so it is not autonomous work. The other two Pending improvement rows (#14, #27) remain below the ROI floor.



**2026-07-26 groom-pass note (overnight monitoring session, at close of #108):** all **96** bug rows are Resolved; **zero** Pending. `improvements.md` still holds three Pending rows (#30, #14, #27), all ⚠️ below the 0.5 ROI floor and correctly scored; `improvements_paywall.md` holds one. No re-ranking was warranted — with nothing Pending in this file, row order carries no scheduling weight.



**Fifteen bugs were filed and fixed in this session (#94-#108), and the shape of them is the finding.** Only the first few were about *filling forms*. From #95 onward, nearly every defect was the pipeline **misreading its own outcome**:



| # | what was misread | what it really was |

| --- | --- | --- |

| #95 | DOM read the instant the click returned | the submission had not happened yet |

| #102 | `aria-invalid` flags | the **previous** attempt's leftovers, on an *accepted* submission |

| #103 | the option list shown to the model | react-select's internal `id\|label` pairs |

| #107 | "control holds no value" | a checkbox the model had **correctly declined** |

| #108 | "form too large for the local model" | a complete form whose submit went nowhere; the size check merely touched it last |



**The single most valuable habit, and it is now evidence-backed: ship the diagnostic before the root cause is known.** #80, #96, #97 and #100 each paid for themselves within *one* cycle, and #100 caught a defect in #98 within one cycle of #100 itself shipping. Three of this session's fixes were defects in earlier fixes *from the same session* (#95→#102, #98→#103, #104→its own follow-up), and two claims had to be publicly retracted after measurement (#103's causation, #104's "every Greenhouse page carries reCAPTCHA"). Care up front did not prevent those; measuring afterwards caught them.



**Standing check earned this session, and it is the generalisation of #76/#81/#102/#103/#107:** *a check that reads only current state, without the intent behind it, will eventually mistake residue for evidence.* Give the verification access to what was **asked for**, not just what is **present**.



**The backlog's centre of gravity has moved.** The fill path now reaches `invalid fields: 0` routinely on both Greenhouse and Lever. The two live ceilings are **bot protection** (4 boards confirmed blocked across both platforms; needs the user's decision on `improvements_paywall.md` #17, out of scope here) and **local-model throughput on large forms** (#105, now failing fast with documents preserved). Further fill-path work has diminishing value until the first is settled.



**2026-07-25 groom-pass note (second monitoring session, at close of #94):** every bug row is Resolved, #94 included. No re-ranking was warranted — nothing is Pending here, so the table's order carries no scheduling weight.



**Both remaining backlogs re-checked, no changes needed.** `improvements.md` holds three Pending rows (#30, #14, #27), all ⚠️ below the 0.5 ROI floor and all correctly scored; `improvements_paywall.md` holds one, out of scope under this session's no-monetary-cost constraint. One row is worth flagging as *further* devalued rather than re-scored: **#30 (detect unanswerable attestations before fit-scoring)** was worth 0.4 when `MissingAttestations` routinely refused jobs after ~10 minutes of scoring. The user has since populated every attestation, so that function now returns empty and the refusal path it optimises is unreachable. Left Pending at 0.4 rather than closed — the guard will matter again the moment a form asks something `pii.yaml` does not cover — but it should not be picked up on ROI grounds.



**Standing check earned by #94, added to the two already recorded above:** *a benign-looking log line is not evidence of a benign event.* The complement of the existing "an absent signal is not evidence of an absent event" (#77/#84/#81). #94 announced itself for hours as `Duplicate check: Already applied to X. Skipping.` — indistinguishable from dedup working correctly, and visible as a defect only when cross-referenced against the same log showing X failing an hour earlier. That line was also **not in the log monitor's filter**; it is now.



**2026-07-26 application-sweep note:** re-ran the gate, reviewed the current journal, traced the agent, discovery, submission, tracker, storage, dashboard and security boundaries, and inspected permissions without reading or printing protected file contents. The sweep initially found six `pkg/submitter` failures against the uncommitted #118 work; #118 was resolved later the same day and the full gate is green again. The model columns below use models confirmed available on this machine today: `claude-opus-4-6-thinking` / `claude-sonnet-4-6` and `gemini-3.1-pro-high` / `gemini-3.6-flash-high`. The available `gpt-oss-120b-medium` and local `qwen3:30b-instruct` remain valid bounded-task alternatives, but this backlog's established schema has only Claude and Gemini recommendation columns.



> **Correction (2026-07-30, improvement #455):** this note's claim that `claude-opus-4-6-thinking` was "confirmed available on this machine today" was never true — **no `-thinking` suffix exists in any Anthropic model ID**, then or now. Extended thinking is a request parameter (`thinking: {type: "adaptive"}`), never part of a model name, so that string could not have resolved against any endpoint. `claude-sonnet-4-6` in the same sentence *was* and still is real. The note is left standing as the dated record it is; the item rows below now name `claude-opus-5` and `claude-sonnet-5`. Worth noting what produced the error: a plausible-looking ID went unchallenged for four days across a dozen groom passes, several of which re-verified every row in this file without ever checking that the model column's values resolve to anything.



**2026-07-26 post-#119 groom-pass note:** the Usability Gate's build, vet, and test checks are green, but its zero-open-Major/Blocker box remains open with 11 Pending rows. Every one was re-verified against current code: #124/#125 still share the tracker write/correlation boundary; #126 still calls `ListenAndServe(":8080", nil)`; #127's sensitive inputs and generated artifacts remain `0644` with generated directories `0755`; #123 still accepts empty/non-2xx fetch results and defers body closure inside the worker; #129 still embeds the developer-specific profile path twice; #121 still embeds and scores job text before checking only trusted RAG output; #120 still exits after `wg.Wait`; #122 still validates literal hostnames/IPs without DNS-bound dialing; #128 still stores fixed filenames under company alone; and #112 still normalizes only the outward `HasApplied` check while funnel insertion and updates use raw URLs. Values, decay, efforts, model choices, and rank therefore remain unchanged. The next autonomous item is #124.



**2026-07-27 post-#124 groom-pass note:** all 10 Pending rows were re-verified against current code, and every score was recomputed. #126 is independently confirmed by the live `*:8080` listener and remains 6×1.0÷1 = 6.0. #127 remains 7×1.0÷2 = 3.5 after metadata-only checks found `.env`, `pii.yaml`, and `applications.db` at `0644`, plus 1,688 generated files at `0644` and 443 directories at `0755`. #123 and #129 remain 6×1.0÷2 = 3.0; #121 remains 8×1.0÷3 = 2.67; #120 remains 7×1.0÷3 = 2.33; #122 and #128 remain 8×1.0÷4 = 2.0 and 6×1.0÷3 = 2.0; #112 remains 6×1.0÷4 = 1.5. #124 changed #125 materially: multi-row company matches now roll back and remain unacknowledged, so the original bulk-corruption risk is gone. The remaining durable-manual-review/correlation gap is re-scoped Minor at 3×1.0÷4 = 0.75. No bug falls below the 0.5 floor. The next autonomous item is #126.



**2026-07-27 post-#126 groom-pass note:** all 9 remaining Pending rows were re-verified against the current tree and every score was recomputed. #127 remains 7×1.0÷2 = 3.5: metadata-only checks again found `.env`, `pii.yaml`, and `applications.db` at `0644`, 1,688 generated files at `0644`, and 443 generated directories at `0755`; `pkg/storage` still creates those artifacts with `0644`/`0755`. #123 and #129 remain 6×1.0÷2 = 3.0; #121 remains 8×1.0÷3 = 2.67; #120 remains 7×1.0÷3 = 2.33; #122 and #128 remain 8×1.0÷4 = 2.0 and 6×1.0÷3 = 2.0; #112 remains 6×1.0÷4 = 1.5; and #125 remains 3×1.0÷4 = 0.75. Dashboard hardening did not reduce any of those defects. No bug falls below the 0.5 floor, 8 Major/Blocker rows still hold the Usability Gate open, and #127 is the next autonomous item.



**2026-07-27 post-#127 groom-pass note:** all 8 remaining Pending rows were re-verified against current code and every score was recomputed. #123 and #129 remain 6×1.0÷2 = 3.0; #121 remains 8×1.0÷3 = 2.67; #120 remains 7×1.0÷3 = 2.33; #122 and #128 remain 8×1.0÷4 = 2.0 and 6×1.0÷3 = 2.0; #112 remains 6×1.0÷4 = 1.5; and #125 remains 3×1.0÷4 = 0.75. Bug #127's permission changes do not alter those defects. The static gate is green, both dashboard routes still return HTTP 200 on loopback, and no Career Agent batch process is running. No bug falls below the 0.5 floor, 7 Major/Blocker rows still hold the Usability Gate open, and #123 is the next autonomous item.



**2026-07-27 post-#123 groom-pass note:** all 7 remaining Pending rows were re-verified and recomputed. #129 remains 6×1.0÷2 = 3.0 because both maintained ingestion commands still name the same developer-specific profile path. #121 remains 8×1.0÷3 = 2.67 because fetched job text still reaches embedding and scoring before the only pre-score quarantine check scans trusted RAG context. #120 remains 7×1.0÷3 = 2.33 because `--daemon` still changes one log line before the ordinary one-batch path reaches `wg.Wait()` and exits. #122 remains 8×1.0÷4 = 2.0 because HTTP and browser boundaries still compare literal hosts or call `net.ParseIP` without DNS-bound dialing. #128 remains 6×1.0÷3 = 2.0 because `SaveApplication` still writes fixed filenames below company alone. #112 remains 6×1.0÷4 = 1.5 because only outward `HasApplied` dedup normalizes schemes; funnel insertion, updates, queueing and reporting still use raw URLs. #125 remains 3×1.0÷4 = 0.75 because ambiguous company matches still roll back and retry without durable manual routing or role correlation. No bug falls below the 0.5 floor. The static gate is green; the live dashboard returns HTTP 200 on both loopback routes, refuses the non-loopback address, and no Career Agent batch process is running. Six Major/Blocker rows keep the Usability Gate open, and #129 is the next autonomous item.



**2026-07-27 post-#129 groom-pass note:** all 6 remaining Pending bugs were re-verified against current code and every score was recomputed. #121 remains 8×1.0÷3 = 2.67: fetched job text still reaches embedding and scoring before the only main-worker quarantine check scans trusted RAG output. #120 remains 7×1.0÷3 = 2.33: `--daemon` still changes one log line before the one-batch path exits. #122 remains 8×1.0÷4 = 2.0: HTTP and browser boundaries still validate literal hosts without DNS-bound dialing. #128 remains 6×1.0÷3 = 2.0: artifacts are still fixed filenames under company-only directories. #112 remains 6×1.0÷4 = 1.5: only `HasApplied` normalizes schemes; a read-only live recount still finds 20 duplicate pairs, now 15 with divergent statuses rather than the stale documented count of 11. #125 remains 3×1.0÷4 = 0.75: ambiguous tracker outcomes still roll back and retry without durable manual handoff or role correlation. No bug is below the 0.5 floor. Build, vet and test are green; required Ollama models remain installed; both dashboard routes return HTTP 200 on `127.0.0.1:8080`; and no batch agent is running. Five Major/Blocker rows keep the Usability Gate open, and #121 is next. This pass ran inline on the current OpenAI GPT-5 coding model because the user reported the Claude and Gemini sessions at their limits; the table columns remain future task-fit recommendations.



**2026-07-27 post-#121 groom-pass note:** all 5 remaining Pending bugs were re-verified and recomputed. #120 remains 7×1.0÷3 = 2.33 because `--daemon` still changes only its startup log before the ordinary batch reaches `wg.Wait()` and exits. #122 remains 8×1.0÷4 = 2.0 because URL and Playwright checks still do not bind public-address validation to dialing. #128 remains 6×1.0÷3 = 2.0 because `SaveApplication` still writes fixed artifact names below company alone. #112 remains 6×1.0÷4 = 1.5: a scheme-specific read-only recount still finds 20 HTTP/HTTPS pairs, 15 divergent; a broader lowercase count also found one separate same-scheme case variant, which is not included here because URL paths may be case-sensitive. #125 remains 3×1.0÷4 = 0.75 because ambiguous tracker matches still roll back and retry without durable manual handoff. No bug is below the 0.5 floor. The uncached build, vet, test, and focused race gates pass; required Ollama models remain installed; both dashboard routes return HTTP 200 on loopback; and no batch agent is running. Four Major/Blocker rows keep the Usability Gate open, and #120 is next. This pass ran inline on the current OpenAI coding model because the user reported Claude and Gemini session limits.



**2026-07-27 post-#120 groom-pass note:** all 4 remaining Pending bugs were re-verified and recomputed. #122 remains 8×1.0÷4 = 2.0 because the worker, redirects, Hacker News ingestion, scraper fetches, and Playwright routes still validate literal hosts without a resolver-bound dial. #128 remains 6×1.0÷3 = 2.0 because `SaveApplication` still writes fixed artifact names below a company-only directory. #112 remains 6×1.0÷4 = 1.5: the read-only scheme-specific recount still finds 20 HTTP/HTTPS pairs, 15 with divergent statuses, while only outward `HasApplied` dedup normalizes the scheme. #125 remains 3×1.0÷4 = 0.75 because ambiguous company-level tracker matches still roll back and retry without durable manual handoff or role correlation. No bug is below the 0.5 floor. The uncached build, vet, full test suite, and focused agent race suite pass; required Ollama models remain installed; both dashboard routes return HTTP 200 on `127.0.0.1:8080`; and no batch agent is running. Three Major/Blocker rows keep the Usability Gate open, and #122 is next. This pass ran inline on the current OpenAI GPT-5 coding model because the user reported Claude and Gemini session limits.



**2026-07-27 post-#122 groom-pass note:** all 3 remaining Pending bugs were re-verified and recomputed. #128 remains 6×1.0÷3 = 2.0 because `SaveApplication` still writes fixed artifacts under a sanitized company-only directory, reconstructs that source path from company name, and can interleave concurrent same-company saves. #112 remains 6×1.0÷4 = 1.5: a read-only aggregate still finds 20 HTTP/HTTPS pairs, 15 with divergent statuses, while funnel insertion, updates, queueing, and reporting retain independently mutable raw-URL rows. #125 remains 3×1.0÷4 = 0.75 because ambiguous company-level outcome matches still roll back and retry without role correlation or a durable manual-review handoff. No bug is below the 0.5 floor. Build, vet, the uncached full suite, and focused race suites are green; required Ollama models remain installed; both dashboard routes return HTTP 200 on loopback; and no batch agent is running. Two Major rows keep the Usability Gate open, and #128 is next. This pass ran inline on OpenAI `gpt-5.6-sol` because the user reported Claude and Gemini session limits.



**2026-07-27 live-log audit note:** the continuously running agent log was reviewed without recording personal data. The current run contains 148 Yahoo fallback requests ending in `unexpected EOF`, seven truncated ATS board-feed payloads (`unexpected end of JSON input`) across four known boards, and five high-scoring postings that were dead or redirected by the time submission began. The first two paths currently log and discard the failed source response without a retry or per-source health state; the stale-posting pattern is filed as an improvement because the fetch can be valid when scoring begins but stale after several minutes of local inference.



**2026-07-28 groom-pass note:** all 4 remaining Pending bugs were re-verified against current code, and every score was recomputed. #130 remains 6×1.0÷3 = 2.0 because `discoverWithYahooHTML` still drops discovery on EOF. #131 remains 5×1.0÷3 = 1.67 because ATS feeds discard JSON without retry. #112 remains 6×1.0÷4 = 1.5 because only outward `HasApplied` dedup normalizes schemes. #125 remains 3×1.0÷4 = 0.75 because ambiguous tracker matches retry without durable manual review. No bug is below the 0.5 floor. The static gate is green (build, vet, uncached tests pass); required Ollama models remain installed. Two Major rows keep the Usability Gate open, and #130 is the next autonomous item.



**2026-07-28 groom-pass note (session 2):** all remaining Pending bugs were re-verified. Scores and severity remain unchanged. The static gate passes (build, vet, test) and models are installed. The Usability Gate remains open due to remaining Major bugs. The next autonomous item is #394.



**2026-07-28 groom-pass note (session 3):** all remaining Pending bugs (#131, #112, #125) were re-verified. Scores and severity remain unchanged. The static gate passes (build, vet, test) and models are installed. The Usability Gate remains open due to the remaining Major bug #112. The next autonomous item is #112, orchestrated via Gemini models per user request.



**2026-07-28 groom-pass note (session 4):** all remaining Pending bugs (#131, #125) were re-verified against current code, and every score was recomputed. No bug falls below the 0.5 floor. The static gate is green (build, vet, uncached tests pass); required Ollama models remain installed. The Usability Gate is MET since #112 was resolved. The agent will proceed to the top item in `improvements.md`.



**2026-07-29 groom-pass note (post-#423):** the gate remains **MET** — `go build ./...`, `go vet ./...` and `go test ./...` all pass clean after the Copilot Mode work. Two bugs were filed this session, both found by *reading* the submit path while implementing improvement #423 rather than by any test or live run:



- **#432 (Major, fixed the same day):** `auto_submit_click: false` recorded a false `APPLIED` plus a permanent dedup row for forms that were never submitted. Fixed with the sentinel mechanism #423 needed. It does not reopen the gate's zero-open-Major box because it was resolved in the same commit that filed it.

- **#433 (Minor, Pending):** four real statuses rank 0 in `mergeStatuses`, so the scheme-dedup migration can revive a job that was deliberately closed.

- **#434 (Major, Pending) and #435 (Minor, Pending):** filed from a review pass over this session's own Copilot Mode work. #434 is the significant one — no path exists to move a job out of `AWAITING_REVIEW` or `MANUAL_REQUIRED`, so every application the user completes by hand is invisible to the funnel and its outcome email matches no tracked application. It is inherited from `MANUAL_REQUIRED` rather than introduced, but Copilot Mode routes *every* job through it.



**Three Pending rows (#434, #435, #433) briefly held the gate's zero-open-Major box open again via #434.**



**2026-07-29 post-#434 note — gate back to MET.** #434 is resolved and verified live; build, vet and the full suite are green. Two Minor rows remain Pending (#435, #433), neither of which gates anything. `improvements.md` has zero Pending rows, so **#435 (1.5) is the next autonomous item**.



**A delegation note worth keeping, because it will recur.** This item was delegated per the Working Protocol and the delegation *failed twice*: `gemini-3.1-pro-high` returned `Individual quota reached ... resets in 3h8m`, and `gemini-3.6-flash-high` returned the same, confirming the protocol's warning that quota is shared across Gemini tiers. Stepping to `gpt-oss-120b-medium` (verified live first) produced **zero file edits in ten minutes**, so it was stopped and the work was done inline instead. The protocol's "step to another tier or provider rather than giving up" is right, but it needs a second half: **set a bound on the replacement, and take the work back when it is not producing.** A delegate that has touched no files after several minutes is not thinking, and waiting three hours for quota is worse than spending the session's own budget.



**The pattern is worth recording, because it is the third session in a row to produce it:** both bugs were invisible to the test suite and to the logs. #432 in particular logged an ordinary success for every job it corrupted. This is the standing check from #94 — *a benign-looking log line is not evidence of a benign event* — and the way it was actually caught was by tracing what a return value **meant** to its caller, not by observing behaviour. The generalisation earned here: **when a function's contract changes from "returns nil on success" to "returns nil on success or on a deliberate no-op", every caller that branches on nil is now wrong.** Grep the callers before, not after.



A third defect was caught in review of this session's *own* delegated implementation and fixed before commit: the new gate sentinels were being processed by the recovery paths as fill failures, which deleted learned form mappings and fell back to Vision, and could rewrap a sentinel as `ErrCaptchaBlocked` and misfile a healthy board. The delegate's tests passed and the build was clean while both behaviours were live — the #76 lesson again, now with a fourth data point.



**2026-07-28 groom-pass note (session 5):** all remaining Pending bugs (#131, #125) were re-verified against current code, and every score was recomputed. No bug falls below the 0.5 floor. The static gate is green (build, vet, uncached tests pass). The Usability Gate is MET. The agent will proceed to the top item in `improvements.md`.



**2026-07-28 groom-pass note (session 6):** all remaining Pending bugs (#131, #125) were re-verified against current code, and every score was recomputed. No bug falls below the 0.5 floor. The static gate is green (build, vet, uncached tests pass). The Usability Gate is MET. The agent will proceed to the top item in `improvements.md`.



**2026-07-28 groom-pass note (session 7):** all remaining Pending bugs (#131, #125) were re-verified against current code, and every score was recomputed. No bug falls below the 0.5 floor. The static gate is green (build, vet, uncached tests pass). The Usability Gate is MET. The agent will proceed to the top item in `improvements.md`.



**2026-07-28 groom-pass note (session 8):** all remaining Pending bugs (#131, #125) were re-verified against current code, and every score was recomputed. No bug falls below the 0.5 floor. The static gate is green (build, vet, uncached tests pass). The Usability Gate is MET. The agent will proceed to work on bug #131 using a Gemini model, as there are no free above-floor items remaining in `improvements.md`.



**2026-07-28 groom-pass note (session 9):** bug #131 was verified as completed. The only remaining Pending bug (#125) was re-verified against current code, and its score was recomputed. No bug falls below the 0.5 floor. The static gate is green (build, vet, uncached tests pass). The Usability Gate is MET. The agent will proceed to work on bug #125 using a Gemini model.



**2026-07-28 groom-pass note (session 10):** bug #125 was verified as completed. There are no remaining Pending bugs. The static gate is green (build, vet, uncached tests pass). The Usability Gate is MET. The agent will proceed to `improvements.md`.



**2026-07-28 groom-pass note (session 11):** Backlogs groomed. Requeued blocked jobs and fixed a parsing bug in the zero transpiler (`/home/howlcipher/dev/zero/`). All bugs remain Resolved. The static gate is green (build, vet, tests pass). The Usability Gate is MET. There are no pending bugs.



**2026-07-28 groom-pass note (session 12):** Evaluated pipeline for applications filled and submitted. Discovered that the ATS feed truncation bug (#131) was only partially addressed by retries; the true cause was an 8MB `LimitReader` artificially truncating large Lever JSON feeds (e.g., jobgether at 37MB+). Increased the limit to 128MB. Added bug #396 and marked it Done. Backlogs groomed. Usability Gate is MET.

**2026-07-30 groom-pass note (twelfth session, post-#461 run, full re-verification):** #461 is Done (see the status line above). `go build ./...`, `go vet ./...` and `go test ./...` are all clean. The four remaining Pending bug rows were each re-verified against current code this pass; no rank or score change, and #449's cited lines were corrected for line drift (see its row). No row is below the 0.5 floor:



| # | Severity | Score | Re-verified how |

| --- | --- | --- | --- |

| #449 | Minor | **2.5** = 5×1.0÷2 | Unchanged as a defect. Line numbers drifted a third time (`:691`/`:721`/`:727`, corrected in the row itself) because improvement #459's `rows.Err()` fix added code above them in the same file; the mechanism (`pgrep -f`/`pkill -f career_agent_bin` matching any process whose command line merely contains the string) is untouched |

| #444 | Minor | **2.5** = 5×1.0÷2 | Unchanged; untouched this session. Fatal-quota branches still `cmd/agent/pipeline.go:268` and `:405`, exact same lines as last pass |

| #447 | Minor | **2.0** = 4×1.0÷2 | Unchanged. `App.tsx:167`/`:172` (`handleStart`/`handleStop`) still bare `await fetch(...)`; fetchers at `:132`/`:144` both open with `try {`; `setInterval` at `:160` still has no `AbortController` |

| #440 | Minor | **1.5** = 3×1.0÷2 | Unchanged. `scripts/` still holds 18 `.go` files with `server.go` the one exception to `//go:build ignore`, still no caller |



**2026-07-30 groom-pass note (seventh session, post-#453 run):** #453 is Done, fixed by a delegated Sonnet 5 subagent (per its Claude model column) and mutation-checked — reverting the fix alone makes its new test fail with the exact scan error the bug predicted. `go build ./...`, `go vet ./...` and `go test ./...` are all clean across all 12 test-bearing packages. The Usability Gate stays **MET**; the four live boxes were not re-verified this pass and rest on their existing dated evidence.



Five Pending bug rows remain, each re-verified against current code:



| # | Severity | Score | Re-verified how | Above floor |

| --- | --- | --- | --- | --- |

| #452 | Minor | **2.5** = 5×1.0÷2 | Unchanged and recounted: all nine `g.Go` closures in `serveMetrics` still log-and-`return nil` (`main.go:398`, `:424`, `:444`, `:461`, `:484`, `:507`, `:534`, `:553`, `:594`), and `:628` is `_ = g.Wait()`. Line numbers unchanged from the last pass — this session's fix touched only `pkg/storage`, not `cmd/dashboard` |

| #449 | Minor | **2.5** = 5×1.0÷2 | Unchanged. A literal-string grep for `"pgrep -f"` against `main.go` returns nothing because the call is `exec.Command("pgrep", "-f", "career_agent_bin")` — three separate arguments, not one substring — which is itself worth recording: the same grep-vs-parse trap #457 fixed mechanically for model columns has no test here, so it still has to be checked by reading the call, not by grepping for a phrase. All three identification sites are unchanged: `main.go:644` (`serveAgentStart`), `:666` (`serveAgentStop`), `:672` (`serveAgentStatus`) |

| #444 | Minor | **2.5** = 5×1.0÷2 | Unchanged; untouched this session. Fatal-quota branches still `cmd/agent/pipeline.go:268` and `:405` |

| #447 | Minor | **2.0** = 4×1.0÷2 | Unchanged as a defect, line numbers drifted again (unrelated to this session's own work — inherited from prior sessions' edits to the file). `App.tsx:167`/`:172` (`handleStart`/`handleStop`) are still bare `await fetch(...)` while the fetchers at `:131`/`:143` both open with `try {`; the `setInterval` at `:160` still has no `AbortController` |

| #440 | Minor | **1.5** = 3×1.0÷2 | Unchanged. `scripts/` still holds 18 `.go` files with `server.go` the one exception to `//go:build ignore`, still no caller |



Nothing is below the 0.5 floor. The free queue is now **#454, #452, #449, #444, #447, #443, #440, then #450/#448/#442**, with `improvements.md`'s **#454** (3.0) now the outright top of the combined queue — #453 was the only bug tying it, and it is now closed. **#454 is the recommended next item.**



**Prior — 2026-07-30 groom-pass note (post-#446 run):** #446 is Done and verified live against real binaries. The Usability Gate stays **MET** — `go build ./...`, `go vet ./...` and `go test ./...` are all clean, exit 0 across all 12 test-bearing packages including the 7 new DSN tests, and `gofmt -l cmd/dashboard/ pkg/storage/` is empty. The four live boxes were not re-verified this pass and rest on their existing dated evidence, except the dashboard box, which this session exercised directly: two dashboards were served against copies of the real database and both returned correct live metrics.



Seven Pending bug rows, each re-verified against current code rather than against its own prose. Three are new this pass, filed from the parallel Claude review agent's findings **after re-reading every line it cited** — one of its claims about severity was downgraded here, and none of its three findings was taken on trust:



| # | Severity | Score | Re-verified how | Above floor |

| --- | --- | --- | --- | --- |

| #451 | Minor | **4.0** = 4×1.0÷1 | New this pass. Confirmed at all three sites: `cmd/dashboard/main.go:394` sums `FAILED_SCORE` and `FAILED_SUBMIT` into one field, `:395` sums `MANUAL_REQUIRED` and `AWAITING_REVIEW`, while `App.tsx:211` and `:216` caption those tiles with `explain('FAILED_SUBMIT')` and `explain('MANUAL_REQUIRED')` — one member each. `statusReason` at `main.go:132-151` gives all four genuinely different text. **Filed Minor, not the Major the review agent proposed:** the counts are correct and only the explanatory line is wrong, which is a smaller failure than #437's missing analytics. Value 4, Effort 1. Now the top of the free queue | yes |

| #453 | Minor | **3.0** = 3×1.0÷1 | New this pass. `pkg/storage/manager.go:75` declares `discovered_at DATETIME` with no `NOT NULL`; `manager.go:1206` reads it into `sql.NullTime` with a `time.Now()` fallback and `continue`s past a bad row, while `pkg/storage/queue_plan.go:64` reads the same column into a bare `time.Time` and `:67` returns the error, discarding the whole plan. Unreachable today because `AddToFunnel` always writes the column, which is what holds it to Value 3 | yes |

| #452 | Minor | **2.5** = 5×1.0÷2 | New this pass. Counted rather than sampled: all **nine** `g.Go` closures in `serveMetrics` log and `return nil` (`main.go:403`, `:417`, `:435`, `:452`, `:475`, `:502`, `:527`, `:557`, `:591`), and `:612` is `_ = g.Wait()`. Note this is the mechanism **#446's own report described** and nobody filed; #446 was the DSN, so this survived its closure. Effort 2 because `sql.ErrNoRows` is a legitimate empty state that must keep rendering as 0 | yes |

| #449 | Minor | **2.5** = 5×1.0÷2 | Unchanged. Line numbers drifted again — now `cmd/dashboard/main.go:628`, `:650`, `:656`, not `:615`/`:637`/`:643`, because #446 added lines above them. All three still identify the agent with `pgrep -f`/`pkill -f career_agent_bin`. Of the three rows tied at 2.5 this is the sharpest, because `pkill` terminating an unrelated process is the only one of the three that damages something outside the dashboard | yes |

| #444 | Minor | **2.5** = 5×1.0÷2 | Unchanged; untouched this session. Re-read: the two fatal-quota branches are still `cmd/agent/pipeline.go:268` and `:405`, both cancelling on any error containing `429` or `Quota exceeded`. Noted in passing that `:200` handles `429` differently, as one of several transient conditions — the same string, two opposite policies, in one file | yes |

| #447 | Minor | **2.0** = 4×1.0÷2 | Unchanged. `App.tsx:163` and `:168` are still bare `await fetch(...)` while the fetchers at `:128` and `:140` both open with `try {`; `setInterval` at `:156` still has no `AbortController`. Note it now shares a file with #451, so the two are worth taking together | yes |

| #440 | Minor | **1.5** = 3×1.0÷2 | Unchanged. Re-counted rather than copied: `scripts/` holds 18 `.go` files and `grep -L "go:build ignore" scripts/*.go` names exactly one, `server.go`. Still no caller anywhere in the tree | yes |



Nothing is below the 0.5 floor in either free backlog, so no `⚠️` flags and nothing needs user confirmation. **#451 is the recommended next item** — highest score in the free queue, a caption derived from data instead of a literal, and it sits in the same twenty lines as #447 so the two can be taken in one pass. The free queue is **#451, #453, #454, #449, #452, #444, #447, #443, #440, then #450/#448/#442**, with #453 placed ahead of #454 at the same 3.0 on the file's usual convention that a bug outranks an improvement at equal score and effort.



**Prior pass — 2026-07-30 groom-pass note (post-#445 run):** #445 is Done and verified live, which closes the Usability Gate's last open box. Five Pending rows, each re-verified against current code rather than against its own prose, and every score recomputed:



| # | Severity | Score | Re-verified how | Above floor |

| --- | --- | --- | --- | --- |

| #446 | Minor | **4.0** = 4×1.0÷1 | Unchanged, and **not** fixed as a side effect of this session's dashboard work. `cmd/dashboard/main.go:333` still reads `sql.Open("sqlite", "./applications.db?_journal_mode=WAL")` while `pkg/storage/manager.go:44` opens the `_pragma=` DSN built at `:42`. Note the line moved from `:247` to `:333` because #445 added code above it — the row's line number was stale, the defect was not. Highest score of the open set and now the top of the whole free queue, but still Minor, so it never gated the box | yes |

| #449 | Minor | **2.5** = 5×1.0÷2 | New this pass, from #445's live verification rather than from any code review. `cmd/dashboard/main.go:615`, `:637` and `:643` all identify the agent with `pgrep -f`/`pkill -f career_agent_bin`. Reproduced live: a decoy process carrying the string in its command line made `pgrep -f` match with no agent running. Value 5 — half of it is a lying status indicator, but the other half is `pkill` terminating an unrelated process. Effort 2: doing it properly means a PID file, which also touches #414's single-instance lock | yes |

| #444 | Minor | **2.5** = 5×1.0÷2 | Unchanged; untouched by this session. Line numbers drifted, as with #446: the two fatal-quota branches are now `cmd/agent/pipeline.go:268` and `:405`, not `:269`/`:406`. Both still call `deps.Cancel()` on any error containing `429` or `Quota exceeded` | yes |

| #447 | Minor | **2.0** = 4×1.0÷2 | Unchanged, and re-read rather than assumed. `App.tsx:163-171` still shows `handleStart`/`handleStop` as bare `await fetch(...)` with no `try`, while the two fetchers at `:128` and `:140` both open with one. `setInterval` at `:156` still has no `AbortController` and no in-flight guard. **Worth noting for whoever takes it:** #445 gives these two handlers a new way to fail — a 403 — which is exactly the failure this row says the UI swallows silently | yes |

| #440 | Minor | **1.5** = 3×1.0÷2 | Unchanged. Re-counted rather than copied: `ls scripts/*.go` is 18 files and `grep -c "go:build ignore" scripts/server.go` is 0, so it remains the one exception of eighteen. Still no caller | yes |



Nothing is below the 0.5 floor, so no `⚠️` flags and nothing needs user confirmation. **#446 is the recommended next item** — it is now the highest-scoring open row anywhere in the free queue, it is a one-line change with the correct line already written next door, and with the gate met there is no longer a Major outranking it on kind. The free queue is **#446, #449, #444, #447, #443, #440, then #448/#442**.



`improvements_paywall.md` was re-checked in the same pass and is unchanged: **#424** (2.0) and **#17** (1.75) still need a paid key, and **#14** remains `⚠️ below floor` at 0.4. None is autonomous work.



One row was checked for staleness and found **correct**, which is worth recording because these passes usually record the opposite: `improvements.md` #443's title says "Eight Go files" while `gofmt -l .` returns 16. That is not a drifted count — the row means eight *compiled* files, and its detail section already breaks out 8 under `cmd/`/`pkg/` plus 8 build-ignored scripts. Re-running it this pass reproduced exactly that split. Left alone.



**2026-07-30 groom-pass note (post-#437 run):** #437 is Done and verified live. Five Pending rows, each re-verified against current code rather than against its own prose, and every score recomputed:



| # | Severity | Score | Re-verified how | Above floor |

| --- | --- | --- | --- | --- |

| #446 | Minor | **4.0** = 4×1.0÷1 | New this pass. `cmd/dashboard/main.go:247` still opens `"./applications.db?_journal_mode=WAL"` while `pkg/storage/manager.go:42` builds the correct `_pragma=` DSN — confirmed by reading both. Value 4: transient, self-correcting wrong numbers, not corruption. Effort 1: one line, and the correct line already exists to copy. Highest score of the open set, but it does not gate the Usability Gate box because it is Minor | yes |

| #445 | Major | **3.5** = 7×1.0÷2 | New this pass. `grep -c "Origin\|Sec-Fetch\|csrf\|CSRF" cmd/dashboard/main.go` returns **0**, and `serveAgentStart` at `:514` checks only `r.Method` before running `exec.Command("./career_agent_bin", ...)`. Value 7: it launches real applications with real PII without the user asking. Effort 2: the check is small, but it needs tests for three handlers that currently have none | yes |

| #444 | Minor | **2.5** = 5×1.0÷2 | Unchanged this pass; not touched by #437's work. `pipeline.go:269` and `:406` still cancel the whole run on any error containing `429` | yes |

| #447 | Minor | **2.0** = 4×1.0÷2 | New this pass. `App.tsx`'s `handleStart`/`handleStop` still have no `try/catch` while the two fetchers beside them do, and the 2s `setInterval` still has no `AbortController` or in-flight guard. Value 4 for two defects; Effort 2 because the ordering fix needs care | yes |

| #440 | Minor | **1.5** = 3×1.0÷2 | Unchanged. `scripts/server.go` is still the one file of eighteen in `scripts/` without `//go:build ignore`, and still has no caller | yes |



Nothing is below the 0.5 floor, so no `⚠️` flags and nothing needs user confirmation. **#445 is the recommended next item** despite #446 outscoring it: it is the only open Major, it is the only thing holding the zero-Blocker/Major box open, and a one-line DSN fix will still be a one-line DSN fix afterwards. The free queue is **#445, #446, #444, #447, #443, #440, then #442**.



Static gate re-verified this pass: `go build ./...`, `go vet ./...` and `go test ./...` all clean across all 12 test-bearing packages, including the seven new dashboard tests. The dashboard box was re-verified live against the real `applications.db` (200s on both routes, real conversion data rendering). The Ollama, tracker and live-batch boxes were **not** re-verified this session and rest on their existing dated evidence.



**What this session found by not trusting prose — the same lesson for the fourth session running, arriving from a new direction:**



- **A bug that names N files is not closed until N files are checked.** #416 named `pkg/storage/manager.go` *and* `cmd/dashboard/main.go`, was fixed in one of them, and has read `Resolved` through two groom passes since (#446). The previous three sessions learned that a *Done note* describes what was added rather than what was dropped; this is the sibling failure, where the *bug report itself* was the checklist nobody re-read.

- **An assertion that cannot fail is worse than no assertion.** #437's accessibility guard initially checked for the substrings `"col"` and `"row"`, which appear in every React bundle ever built. It passed against the broken bundle. It was caught only by deliberately running the new tests against the pre-fix build — which is the practice this backlog already records as "confirm at runtime that a fix announces itself", applied to a test instead of to a fix.

- **A security boundary that was correct for the threat it was designed against can be no defense at all against the next one.** #126 bound the dashboard to loopback and closed the remote-attacker path completely. #445 walks in through the user's own browser, where loopback is not a boundary of any kind.



**2026-07-30 groom-pass note (post-#441 run):** #441 is Done. Three Pending rows, each re-verified against current code rather than against its own prose, and every score recomputed:



| # | Severity | Score | Re-verified how | Above floor |

| --- | --- | --- | --- | --- |

| #437 | Major | **3.0** = 6×1.0÷2 | Both halves still hold. Go side still serves all three fields (`cmd/dashboard/main.go:72-74` declares them; `:424`, `:466`, `:500` populate them). `App.tsx` mentions `interview_rate_pct`, `by_source` and `by_variant` **only** in its `Metrics` interface at lines 41-43 and renders none of them; a whole-file grep finds zero `<caption>` and zero `scope=`, so improvement #34's markup is still absent too. Untouched by #441's work | yes |

| #444 | Minor | **2.5** = 5×1.0÷2 | New this pass. `pipeline.go:269` and `:406` still cancel the entire run on any error containing `429`, which on Anthropic is a transient per-minute limit the adjacent backoff branch already handles. Value 5 rather than higher because it cannot fire on the default local provider; Effort 2 because deciding what counts as a fatal quota is a real decision, not a wording change | yes |

| #440 | Minor | **1.5** = 3×1.0÷2 | Unchanged as a defect, but its **counts were wrong**: `scripts/` holds 18 `.go` files, 17 tagged, so `server.go` is one exception out of eighteen — the earlier rows said "out of 21" and "all 20 siblings". Confirmed again that nothing imports or invokes it | yes |



Nothing is below the 0.5 floor, so no `⚠️` flags and nothing needs user confirmation. **#437 outranks both** and is the only open Major, making it the sole item holding the zero-Blocker/Major box open. `improvements.md` gained **#443** (2.0, `gofmt` drift) from this session, so the free queue is **#437, #444, #443, #440, then #442**.



Static gate re-verified this pass, and the build box was re-verified **the way the box demands**: a fresh `git clone` of the repository at `d69edb0` builds, vets, and passes the entire test suite with no `.env` and no untracked artifacts present — which also confirms the two new setup-consistency guards work from a clean checkout. The Ollama box is now verified by the product itself: `cmd/agent` preflights `/api/tags` at startup, and the real binary logged a pass against `qwen3:30b-instruct`, `qwen2.5vl:7b` and `nomic-embed-text`. The tracker and live-batch boxes were **not** re-verified this session and rest on their existing dated evidence.



**Two things this pass found by not trusting prose, which is the same lesson three sessions running:**



- **A backlog row's arithmetic is prose too.** #440 carried two mutually inconsistent counts ("out of 21", "20 siblings") and the real number was 18, through two groom passes. The defect was real the whole time, which is exactly why nobody re-counted.

- **A log line is documentation that ships.** Seven references to Gemini survive in code paths that route to whichever provider `LLM_PROVIDER` selects; on this host the log promises "Gemini-1.5-Pro" and a local `qwen2.5vl:7b` answers. #439 was diagnosed by taking an honest comment seriously; #444 is the inverse — a dishonest log line that would misdirect the next person to debug vision.



**2026-07-29 groom-pass note (late evening, post-#439 run):** #439 is Resolved. Three Pending rows remain and each was re-verified against live state, not against its own prose:



| # | Severity | Score | Re-verified how | Above floor |

| --- | --- | --- | --- | --- |

| #441 | Major | **7.0** = 7×1.0÷1 | Read both halves live: `install_ollama.sh:25-27` pulls `llama3.1`/`llava` by default, `.env.example:10,21` ships uncommented `qwen3:30b-instruct`/`qwen2.5vl:7b`. Value 7: it breaks a new user's first run on the documented happy path. Effort 1 at its narrowest (comment two lines and note what the installer pulls), higher if the class is closed properly with a startup `/api/tags` preflight | yes |

| #437 | Major | **3.0** = 6×1.0÷2 | Unchanged this pass — `App.tsx` still renders no `by_source`, `by_variant` or `interview_rate_pct`, all three of which the API still serves on every poll. Not touched by #439's work | yes |

| #440 | Minor | **1.5** = 3×1.0÷2 | `grep -c "go:build ignore" scripts/*.go` returns 0 for exactly one file, `server.go`, out of 21. Confirmed it has no caller. Value 3: nothing is broken, but the directory now has two contradictory conventions | yes |



Nothing is below the 0.5 floor, so no `⚠️` flags and nothing needs user confirmation. **#441 outranks #437** on score and on kind: a bug that stops a new user's first run beats a bug that hides analytics from an existing one. `improvements.md` gained one Pending row from this session (**#442**, 1.0 — measure whether the offload #439 preserved is worth keeping at all), so the free queue is #441, #437, #440, then #442.



Static gate re-verified this pass: `go build ./...`, `go vet ./...` and `go test ./...` all clean, including the 16 new Go tests; `nlp_service`'s 12 Python tests pass and `flake8` is clean on both Python files. The Ollama box was re-verified live via `/api/tags` **and** by three real end-to-end generations, which is stronger evidence than the tags endpoint alone — that box has never before been backed by an actual completed generation on all of its routes.



**The session's finding, stated as a rule, because it is now two-for-two in one day:** *a rewrite's Done note describes what was added and never what was dropped, so the only safe review of a rewrite is a diff against what it replaced.* #437 lost a UI surface that way; #439 turned out to have lost six behaviours the same way — a circuit breaker, a context-size calculation, an entire provider abstraction, a timeout that an earlier bug had been filed to fix, four prompt instructions, and best-effort error handling. Both were shipped, tested, and green. Neither loss was reachable from any test, because the tests were written against the new code.



**2026-07-29 groom-pass note (evening, post-#435 run):** two Pending rows, both Major, both filed this session and both re-verified against live state rather than against their own prose:



| # | Score | Re-verified how | Above floor |

| --- | --- | --- | --- |

| #439 | **4.0** = 8×1.0÷2 | `localhost:11434/api/tags` queried live — `llama3` is absent, and `pkg/mcp/client.go:230-238` still hardcodes it. Value 8: a documented flagship feature cannot succeed at all. Effort 2: the code change is small, but verification needs a real multi-call generation on a CPU-only host | yes |

| #437 | **3.0** = 6×1.0÷2 | `git show 0028b2f -- cmd/dashboard/index.html` still shows the deleted `by_source`/`by_variant`/`interview_rate` markup, and `App.tsx` still renders none of it. Effort re-scored 1.5 → **2** this pass: restoring two conversion tables in React with their `<caption>`/`scope="col"` semantics plus tests is more than the 1.5 first assigned | yes |



Neither is below the 0.5 floor, so no `⚠️` flags and nothing needs user confirmation to work. **#439 outranks #437** on score and on kind — a feature that cannot run beats a feature that cannot be seen. `improvements.md` has zero Pending rows, so these two are the entire free queue.



**One more instance of the session's recurring theme, caught by the test suite itself:** fixing the README's `go run cmd/agent/main.go` → `go run ./cmd/agent` broke `TestREADMENamesCurrentSetupEntrypoints` (`pkg/config/pii_template_test.go`), because that test asserted the README *contained the broken form*. Improvement #36 added it to keep the README's entrypoints current, and it had been actively holding a non-compiling command in place. It now asserts the package form **and** that the three single-file forms are absent — a presence assertion alone is satisfiable while a stale broken copy still sits elsewhere in the file. Third variant this session of *the test observed the wrong thing* (#435, #438, this).



Also re-verified this pass: the three static gate boxes (`go build`/`go vet`/`go test`, all clean, and the build box now additionally clean **from a fresh clone**), the Ollama box (tags queried live), and the dashboard box (binary run against the real `applications.db`, serving 200s and correct payload fields). The tracker box and the live-batch box were **not** re-verified this session and rest on their existing dated evidence.
