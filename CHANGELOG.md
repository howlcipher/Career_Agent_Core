# Career Agent Core - Changelog

## 2026-08-02 — Read-only local log triage has a privacy boundary

* **Improvement (#487):** Added `cmd/logtriage`, an opt-in stdin-to-stdout utility for compact daemon context packets. It redacts common direct identifiers, credential-shaped values, and URL query strings before retaining or optionally sending any event to the local 4B model; caps events, lines, model context, output, and duration; validates model JSON; and falls back deterministically on any failure. It has no application, database, browser, email, filesystem, Git, or submission authority.

## 2026-08-02 — Fresh queue control and an operator-first interface

* **Improvement (#492):** `DISCOVERED` jobs now receive priority at seven days, expire at 30 days through the scheduled agent-cycle sweep, and are capped at 25 pending rows per identified discovery source. Cap and expiry outcomes are recorded explicitly; fit ranking itself is unchanged.
* **Design:** The local dashboard and GitHub Pages presentation now use an accessible, restrained retro-industrial command-deck system: flat steel panels, high-contrast amber/cyan state cues, visible focus rings, responsive controls, and less decorative visual noise.

## 2026-08-02 — Local-model delegation now has a reviewed, default-deny boundary

* **Improvement (#486):** Added `cmd/localdelegate` and a framework-independent contract for bounded local-Ollama delegation. The first phase accepts only a size-limited, sanitized brief and returns a strictly validated JSON proposal; unknown fields, sensitive paths, missing test plans, and obvious credential markers are rejected. A separate patch phase requires an explicit reviewer identifier and the SHA-256 digest of the exact reviewed proposal, accepts changes only for the reviewed file list, and writes a candidate diff artifact without applying it. The command cannot run tools, apply patches, access Git, credentials, browsers, email, application data, or the production database. Documentation records the two-phase workflow and the reviewer responsibilities.

## 2026-08-02 — Assisted Apply preserves safe human handoffs

* **Improvement (#511):** The loopback dashboard now has a visible Assisted Apply queue for interrupted and legacy human handoffs. It uses private resumable plans, plain-language next actions, atomic browser leases, guarded visible Chromium sessions, validated per-job document access, explicit manual receipt confirmation, and sequential batch guidance. CAPTCHA challenges, credentials, legal questions, and employer acceptance remain human actions; nothing is marked applied merely because a browser opened or a button was clicked.
* **Fix:** Historic Assisted Apply rows now require an explicit, guarded current-page check before they can be opened. The check does not launch Chromium, fill a form, or retain page content; it labels only a direct current challenge as a CAPTCHA and otherwise leaves the handoff non-launchable for a safe recheck.
* **Fix (bug #512):** Assisted Apply now opens a revalidated employer page only for a direct CAPTCHA or when the page matches the stored role and exposes an application entry point. Blank, stale, mismatched, and unhydrated pages remain non-launchable and can be safely checked again.

## 2026-08-02 — Discovery degradation and dead DNS records no longer hide capacity loss

* **Improvement (#510):** Discovery refresh health now persists Yahoo request attempts, final request failures, and circuit-open skips as numeric, source-level aggregates. The dashboard reports those conditions alongside the existing refresh summary without storing URLs, postings, or raw errors.
* **Improvement (#479):** A resolver response that authoritatively says a hostname does not exist now enters `RETRY_EXHAUSTED` immediately with the normalized `dns_not_found` reason. Temporary DNS failures still use the existing bounded retry and backoff path.

## 2026-08-02 — Discovery stalls now have an explanation

* **Improvement (#509):** The daemon persists privacy-safe refresh timing and aggregate source outcomes. The dashboard distinguishes a stopped agent, a pending first refresh, a source error, no-new-job refreshes, and new eligible work without displaying job content or raw provider errors.

## 2026-08-02 — Optional NLP offload has measured local value

* **Improvement (#442):** Matched local tailoring runs retained the opt-in FastAPI executor: 5m18s offloaded versus 7m6s in-process, with verifier maximum resident memory reduced from 423 MB to 43 MB. It remains health checked and falls back safely.

## 2026-08-01 — Equivalent recent applications can be skipped conservatively

* **Improvement (#498):** An optional `duplicate_cooldown_days` profile setting now prevents repeat applications only when a recently confirmed application has the same normalized company, role family and seniority, location, and remote classification. It defaults to `0`, preserving existing behavior; incomplete or different metadata never causes a skip. The dashboard identifies these skips explicitly, and setting the value back to `0` is the user-controlled override.

## 2026-08-01 — Funnel transitions and submission failures are now traceable

* **Improvement (#494):** SQLite now keeps an append-only, indexed record of every `job_funnel` status transition, including its prior state, pipeline stage, normalized reason code, time, and measured duration since the preceding state. The ledger is enforced by a database trigger, so status writers cannot accidentally bypass it. `application_attempts` also preserves a normalized failure reason: prompt-injection quarantine, exhausted browser-crash recovery, and generic fill failure are distinguishable even though legacy dashboard aggregates retain the compatible `OTHER_FAILURE` class. Neither store contains job content, prompts, answers, generated documents, DOM, or screenshots; historical rows are intentionally not invented.

## 2026-08-01 — Site capabilities now learn from discovery and submission outcomes

* **Improvement (#496):** The formerly unused `career_sites` table now records every newly discovered or attempted domain, its observed provider, form-reach confirmation strategy, account-gate evidence, and mapping health. Cached form mappings now retain outcome counters and validation timestamps; a recently successful mapping is preferred, while a stale or failure-dominated one yields to the established provider-specific fallback rather than becoming a permanent blacklist. Existing databases receive idempotent schema upgrades without inventing historical capability evidence.

## 2026-08-01 — The daemon now reports stalled, one-sided outcomes

* **Improvement (#495):** After three nonempty cycles without a new confirmation, the agent emits one aggregate-only alert when a recent terminal status accounts for at least 75% of outcomes. The dashboard displays the current alert. Detection cannot requeue jobs, suppress sources, relax constraints, or change submission behavior.

## 2026-08-01 — Discovery retains a free, current remote-job source when search is unavailable

* **Fix (bug #508):** The live eligible queue emptied after SerpApi exhausted its search quota and the Yahoo fallback began returning transport EOFs; RemoteOK, Hacker News, and the existing known-board sweep supplied no new postings in that refresh. The daemon now also reads Jobicy's public structured remote-job feed, title-filters it using the established role gate, and stores the actual title, company, URL, and `jobicy` discovery source. The provider is polled at most once per hour across daemon refreshes, respecting its documented fair-use cadence. This adds an independent, no-key feed without relaxing job constraints or changing submission behavior.

## 2026-08-01 — Discovery channels are now visible in the funnel

* **Improvement (#499):** New job-funnel records retain their actual discovery channel (`remoteok`, `hackernews`, `atsfeed:<board>`, `serpapi`, or `yahoo`) instead of only the eventual ATS hostname. Existing records intentionally remain unknown, so no historical channel is fabricated. This makes the live empty queue and source-quality investigations attributable to the channel that found a posting.

## 2026-08-01 — Excluded ATS postings no longer clog the discovered queue

* **Fix (bug #482):** `breezy.hr` postings are deliberately not auto-submitted, but the old queue filter left them permanently marked `DISCOVERED`. New excluded postings now enter as `SKIPPED` with an explicit source-exclusion reason and are never handed to a batch worker; agent startup also terminalizes legacy excluded rows. The dashboard identifies that reason instead of describing the row as a low-fit skip.

## 2026-08-01 — A browser crash during initial page setup gets one safe recovery

* **Fix (bug #507):** all eight initial post-#489 submission failures were the same Playwright `target closed` condition, and each occurred before page validation or document generation. `AttemptSubmit` now recreates the browser context once when that failure occurs during initial setup, matching the later fill-path recovery while remaining bounded. A regression test verifies the crashed page is released, the replacement page proceeds, and document generation is not repeated.

## 2026-08-01 — The dashboard now shows whether the agent can make mission progress

* **Improvement (#491):** The dashboard previously showed raw funnel counts and last-row snapshots, but not whether there were any jobs the daemon could actually process or whether confirmed applications were happening. It now reports confirmed applications today and in the last seven days, median discovery-to-first-attempt latency, time since the last confirmation, and the eligible queue with its never-attempted subset. The eligibility calculation deliberately mirrors the agent's queue filter, so a visible `DISCOVERED` count can no longer imply that work is available when every row is excluded or delayed. Missing time-based values render as an explicit unavailable state rather than a fabricated zero.

## 2026-08-01 — Every pre-scrape prompt-injection quarantine now reaches the audit trail

* **Fix (bug #503):** `TwoStepVerification` rejected unsafe career-page DOM before any model use, but it called the basic quarantine API directly and discarded the structured threat details. Those detections never reached `applications/prompt_injection_detections.csv`, undercounting the security audit used to monitor the filter. The pre-scrape boundary now uses the shared quarantine-and-audit helper, preserving the existing rejection behavior while writing one CSV row per detected threat. The pipeline has no company name at this boundary, so rows accurately retain an empty company field rather than fabricating attribution. Safe and unsafe DOM regression tests cover the audit behavior.

## 2026-08-01 — Confirmed applications now have an authoritative funnel timestamp

* **Fix (bug #490):** `job_funnel.applied_at` existed in the schema but was never set, leaving the funnel unable to answer when a confirmed application happened. Both automatic confirmation and manual handoff promotion now record a canonical UTC timestamp; later status changes retain the original confirmation time. Existing historical rows are intentionally not backfilled because no reliable source exists.

## 2026-08-01 — Exhausted retries retain their diagnostic cause

* **Fix (bug #480):** retryable pipeline failures already logged their causal error, but a job that exhausted its five-attempt budget entered `RETRY_EXHAUSTED` with a blank `status_reason`, making the terminal queue impossible to group by cause without reconstructing logs. `UpdateFunnelStatusRetryable` now accepts that reason from all six retryable paths and writes the final one when the row becomes terminal. Regression coverage verifies both the existing exponential backoff and the retained terminal reason.

## 2026-08-01 — The prompt-injection quarantine no longer discards ~half of everything discovered on the strength of a bare keyword count

* **Fix (bug #489, reopened the Usability Gate):** a mission-alignment audit found `QUARANTINED_PROMPT_INJECTION` was 51.0% of every `job_funnel` row ever written — the single largest status, larger than every success and every other failure combined — concentrated on `jobs.lever.co` and the `greenhouse.io` family, the two ATS platforms this project has working auto-submit handlers for. Root cause, traced through the vendored `promptsec` library's source: two of its heuristic checks (`instruction_override`, `system_prompt_leak`) run unconditionally, ungoverned by the severity dial bug #394 previously turned down, and fire on bare keyword co-occurrence with no located matched text — ordinary EEO statements, background-check disclosures, and application-process instructions every real Lever/Greenhouse posting carries. `pkg/security/filter.go`'s `QuarantineLayer` now runs a conservative local second pass before quarantining: it only releases a payload when every decisive threat is one of those two zero-evidence categories, the text carries none of a curated set of injection-marker phrases, and it matches at least two of a curated set of known-benign ATS-boilerplate signatures — a real located match, a hit from any other guard, or any other threat category still quarantines exactly as before. 79 new test assertions cover both the released false positives (verified against the raw library, not assumed) and an adversarial case where a genuine injection is camouflaged inside real EEO boilerplate, which must and does stay quarantined. See `documentation/backlog_history/bugs_done_details.md` #489 for the full account, including the exact library code paths and why a live before/after quarantine-rate measurement (filed as new bug #501) could not be done retroactively. Also filed #502 (a third, distinct zero-evidence threat source this fix's narrow scope correctly leaves untouched), #503 (one quarantine call site never reaches the audit CSV, undercounting it), #504 (our own RAG-generated content can trip the same mechanism via an existing but unmeasured status), and improvement #505 (a small code duplication noticed while auditing every call site) — none block the gate. **This closes the last open Major/Blocker bug; the Usability Gate is MET again.**

## 2026-08-01 — A local-model benchmark harness measures which installed model actually suits a task

* **Improvement (#484):** nothing in this repository had ever measured whether the largest installed local model (`qwen3:30b-instruct`) is actually the right choice for a given bounded task on this CPU-only, single-Ollama-instance laptop, versus the smallest (`qwen3:4b-instruct`). New `cmd/modelbench` + `internal/modelbench` benchmark three representative, synthetic task classes (structured classification, bounded summarization, structured implementation/test planning) against any installed model, capturing Ollama's own timing/token counters (cold vs. warm load time, prompt/generation throughput) plus host memory/swap snapshots, and validating every response against an objective schema rather than a subjective "looks good". Refuses to run against an uninstalled model, and refuses to run at all while the production agent's single-instance lock is held (verified live against the real lock file and the real running daemon) unless `-force` is passed, since benchmarking unloads and reloads models on the same Ollama instance the agent depends on. See `documentation/model_benchmark.md` for usage and the routing hypothesis this measurement infrastructure is meant to test. Nothing is written to disk by default; `/benchmark_results/` is gitignored for the documented `-out` convention. Backlog: also added `improvements.md` #485-#488 (resource-aware admission control, a safe local-model delegation contract, lightweight 4B log triage, and an OpenClaw sidecar evaluation), all Pending and none built this session — see their Details sections.

## 2026-08-01 — Aging job postings can no longer be starved out of the queue until they expire

* **Fix (bug #481):** `RankJobs`'s freshness signal was a soft multiplicative decay that only ever pushed a job's score *down* as it aged — it had no way to push a job *up*. Under sustained backlog pressure, a job from a weak source or with a weak fit score could keep losing to fresher, higher-scoring jobs indefinitely, aging straight through to expiry with zero processing attempts; a live audit found ten `expired` postings that had all sat in `DISCOVERED` for ~19 days before their first (and fatal) attempt. `RankJobs` now forces any job at or past 10 days old (`urgentAgeDays`) to the front of the returned queue regardless of score, ahead of the existing exploration/exploit split — `cmd/agent`'s per-cycle backlog is truncated straight from that order with no re-sorting, so this guarantees aging jobs get attempted before the cap is hit. Mutation-checked and live-verified via a graceful production-daemon rebuild/restart; see the bug's Done note for why a live before/after ordering measurement wasn't observable this session (the aged backlog it measured had already drained).

## 2026-08-01 — A job with an unresolvable hostname no longer spins the daemon forever

* **Fix (bug #478):** a DNS resolution failure (a "no such host" error, distinct from a rejected private/loopback target) in `cmd/agent/pipeline.go`'s `StateInit` node only logged and left the job at `DISCOVERED` with no backoff, so `GetDiscoveredJobs` reloaded it on every single cycle — observed live at ~1 cycle/sec instead of the documented ~1-minute `cycleInterval`, with unbounded log growth. Now calls `storage.UpdateFunnelStatusRetryable`, the same exponential-backoff/`RETRY_EXHAUSTED` machinery bug #466 built for every other retryable failure in the file. Mutation-checked and live-verified end to end: rebuilt and restarted the real running daemon, confirmed the previously spinning job failed once and the cycle cadence returned to a clean 60 seconds.

## 2026-08-01 — The Yahoo discovery fallback looks a little more like a browser

* **Improvement (#477):** `discoverWithYahooHTML`'s fallback requests (used whenever `SERPAPI_API_KEY` is absent or exhausted) previously sent only a `User-Agent` header and used a fresh, cookie-less HTTP client for every single query. Now sends `Accept`/`Accept-Language` headers too, and shares one `http.CookieJar` across every query made by a single discovery run instead of starting cold each time. Live-verified against the real daemon: failure rate among breaker-allowed attempts dropped from 25.2% to 19.7% in a short post-restart sample — directionally positive, not statistically conclusive given the sample size difference, closed either way per the item's own note.

## 2026-08-01 — A broken database cursor no longer hides behind a shorter-than-expected requeue preview

* **Fix (bug #476):** `cmd/requeue`'s dry-run preview (`GetQueuePlan`) never checked `rows.Err()` after its scan loop, so a driver-level cursor fault partway through iteration looked identical to "this pattern only matched N rows" — the same defect class bug #452 fixed for the dashboard's metrics queries, in a different file. The scan loop is now `scanQueuePlanCandidates`, and a genuine cursor fault surfaces as an error instead of a silently truncated candidate list. Does not affect `-confirm`, which requeues via an independent bulk `UPDATE`.

## 2026-07-31 — The dashboard can no longer be refused a connection outright by a fresh, contended database

* **Improvement (#450):** `cmd/dashboard` opened with the same DSN as the writer (`storage.DSN`, used by `cmd/agent` and every other command via `storage.InitDBWithPath`), which asks SQLite to set `journal_mode(WAL)` on every connect. Against a genuinely fresh database with another connection holding an open write transaction, that request is refused outright (`SQLITE_BUSY`) rather than merely delayed — `busy_timeout` does not cover a mode-change refusal, only a lock wait. New `storage.ReaderDSN` carries every other pragma (`synchronous`, `busy_timeout`, `cache_size`, `temp_store`) but drops `journal_mode`, since a read-only connection has no business changing it and finds it already WAL once the writer has opened once; `cmd/dashboard` now opens with it. The writer DSN is unchanged.

## 2026-07-31 — `cmd/requeue -status` now rejects a typo instead of silently requeuing nothing

* **Improvement (#470):** a typo'd `-status` value (e.g. `-status BLOKCED_CAPTCHA -confirm`) used to sail past `RequeueByURLPattern`'s raw SQL `WHERE` clause, match zero rows, and print `requeued 0 row(s)` as if the operation had succeeded — indistinguishable from "this source genuinely has nothing left in that status." `main()` now validates `-status` against the known set (`BLOCKED_CAPTCHA`, `FAILED_SUBMIT`, `APPLIED`, `RETRY_EXHAUSTED`) before touching the database, failing loudly with the exact bad value on a typo, and prints each source's current count for the requested status before acting — closing a gap in the `-confirm`-without-`-plan` path that previously gave no feedback at all before writing.

## 2026-07-31 — The dashboard's "Not A Posting" caption stops misdescribing most of its own count, and RETRY_EXHAUSTED finally has a tile

* **Improvement (#468):** every `INVALID_URL` job_funnel row collapsed to one status with no persisted reason, even though the code already distinguishes two structurally different causes — a URL that was never a real posting (known-junk pattern, unsafe network target) versus one that was a real posting and has since expired (caught by `checkJobAlive` or a terminal fetch status). A live measurement against `applications.db` found ~88% of the bucket was the latter, so the single hardcoded caption ("Not a real posting…") misdescribed most of what it counted. A new `job_funnel.status_reason` column and `storage.UpdateFunnelStatusInvalid` now record which case applied at write time, and the dashboard's "Not A Posting" tile captions whichever reason(s) actually contributed, the same way bug #451 split the Failed and Manual Queue tiles. Separately, `RETRY_EXHAUSTED` (bug #466's terminal status for a job that spent its whole retry budget) had zero dashboard presence at all — absent from every count and the legend — and now has its own tile.

## 2026-07-31 — The most common submit path now also survives a crashed browser tab

* **Improvement (#471):** bug #467 taught `AttemptSubmit` to recover from a `target closed` Playwright failure by recreating the browser context once, but that recovery only lived in the generic/dedicated-handler retry loop. The cached-mapping fast path (`storage.GetFormMapping` hits — the dominant path once an ATS domain has been mapped once by the Learner Module) still treated the same failure as an ordinary fill failure: it deleted the cached mapping and fell back to Vision or a bare error. That path now recovers the same way — recreate the page/context, redo the cheap dead/CAPTCHA checks, and retry the cached mapping against the fresh page, capped at the same single recovery attempt — before falling back to cache invalidation. Verified with two tests: one where the recovered page succeeds (mapping survives, submission confirms), one where it fails again (recovery budget exhausted, cache invalidated exactly as before).

## 2026-07-31 — A crashed browser tab no longer discards an already-tailored application

* **Fix (bug #467):** a Playwright submit or fill action that failed with `target closed` (the wording Playwright uses when the browser tab/context it was operating on has crashed or been torn down — the live log also recorded a Chromium headless crash the same session) had no recovery path: the whole job attempt was written off, discarding the already-generated tailored resume and cover letter along with it. `AttemptSubmit` now detects this specific failure and recreates the browser context once (capped, so a posting that crashes deterministically still fails cleanly), re-navigates, and retries only the fill/submit step — not the expensive document-generation step that already ran. Verified end to end with a test double that fails a first browser tab and succeeds a recreated second one, plus a bounded-retry test proving a second crash gives up rather than looping.

## 2026-07-31 — A domain that keeps timing out stops eating every worker's next attempt too

* **Improvement (#469):** `checkJobAlive` (pre-flight liveness) and `fetchJobPage` (job description fetch) each retried independently per call, with no memory of a domain's recent failure history — so a domain that was timing out on every request still got tried again on the very next job from that domain, spending a full request timeout to find out again. A new per-domain circuit breaker now tracks consecutive failures by ATS domain, shared across every worker and daemon cycle: after 5 consecutive failures a domain's circuit opens for a 2-minute cooldown, deferring further jobs from that domain straight back to `DISCOVERED` (logged as `Circuit open for <domain>; ...`) instead of retrying them individually, then allows one probe through before closing again. Deferred jobs do **not** spend their own retry budget — only a genuine observed failure counts toward `RETRY_EXHAUSTED` — via a new `storage.DeferFunnelStatus` distinct from the existing `UpdateFunnelStatusRetryable`.

## 2026-07-31 — The queue no longer gets stuck retrying the same handful of jobs forever

* **Fix (bug #466):** a live continuously-running daemon repeatedly processed the same ~15 job_funnel rows across many cycles, starving a 10k-row backlog. Retryable failures (a preflight network check, a job-page fetch, the RAG embedding/retrieval call, a post-score freshness re-check) simply reset a job's status back to `DISCOVERED`, and the scheduler pulls every `DISCOVERED` row with no attempt count or cooldown — so a transient failure was retried on the very next cycle and could dominate the worker indefinitely. Retryable failures now back off exponentially (2, 4, 8, 16 minutes) before becoming eligible again, and after 5 attempts a job moves to a new terminal `RETRY_EXHAUSTED` status instead of retrying forever. `cmd/requeue -status RETRY_EXHAUSTED -confirm` gives such a job a fresh retry budget once you believe the cause is fixed, same as it already does for `BLOCKED_CAPTCHA`/`FAILED_SUBMIT`/`APPLIED`.

## 2026-07-30 — The dashboard now warns you when it can't tell if its numbers are current

* **Improvement (#460):** `App.tsx`'s metrics poll silently kept the last-good numbers on screen when `/api/metrics` failed, with no banner, timestamp, or other cue that the data might be stale — a real `500` (visible in the network tab since bug #452) was otherwise invisible to anyone just watching the dashboard. A single missed poll now stays silent (expected noise), but two consecutive failures show a non-alarming `role="status"` message ("Metrics may be out of date — the last N polls failed"), which clears again the moment a poll succeeds.

## 2026-07-30 — The dashboard UI gets a test framework, and its two trickiest state-machine bugs get real coverage

* **Improvement (#463):** `cmd/dashboard/ui` had no test runner at all — no `vitest`, no `@testing-library/react`, nothing beyond `tsc`/`oxlint`/`vite build`. The poll sequence-number guard (bug #447) and the start/stop `actionError` states could only ever be checked by hand against a live running instance. Added `vitest` + `@testing-library/react` + `@testing-library/jest-dom` + `jsdom`, a `test` script, and six real tests in `src/App.test.tsx` covering: a stale, slower `/api/metrics` and `/api/agent/status` response resolving after a fresher one must not overwrite it; a failed or thrown start/stop `fetch` surfaces the expected `role="alert"` message; a subsequent successful click clears a prior error. Mutation-checked — reverting the metrics sequence guard alone makes the corresponding test fail with the exact stale-data symptom it exists to catch.

## 2026-07-30 — The dashboard now tells you when a start/stop click fails, and a slow poll can't overwrite fresher data

* **Fix (bug #447):** `handleStart`/`handleStop` in the dashboard UI (`cmd/dashboard/ui/src/App.tsx`) had no `try/catch` around their `fetch` calls, so a failed POST (a rejected promise or a non-2xx response) looked identical to a successful one — no error, no button change, nothing but an unhandled rejection in the browser console. Both handlers now catch failures and non-2xx responses and surface a visible `role="alert"` message under the controls. Separately, the 2-second metrics poll had no guard against out-of-order responses: a slow request could resolve after a faster, later one and overwrite fresh state with stale data. Each poll now carries a sequence number, and a response is only applied if it is still the most recent request in flight.

## 2026-07-30 — A transient rate limit no longer cancels the whole batch

* **Fix (bug #444):** `cmd/agent`'s scoring and tailoring retry loops treated any error containing a bare "429" as a fatal daily-quota condition and called `deps.Cancel()`, abandoning every remaining job in the batch. On Anthropic, a 429 is the ordinary per-minute rate limit the adjacent backoff branch already exists to handle — so a Claude-configured agent could lose an entire run to a condition that would have cleared in seconds. Only Gemini's own "Quota exceeded" wording (its genuine hard-quota signal) is now treated as fatal; a bare 429 is retried with backoff on every provider, including Gemini itself, since its own SDK returns 429 for the per-minute limit too. The shutdown log line, and five log lines in `pkg/submitter/vision.go`, also named "Gemini" unconditionally regardless of the configured `LLM_PROVIDER`; both now name the active provider.

## 2026-07-30 — The metrics API's per-row breakdowns stop swallowing cursor faults

* **Fix (improvement #459):** `serveMetrics`'s by-source and by-variant breakdowns each looped over their query's rows without checking `rows.Err()` afterward. `Next()` returning false can mean either "the result set is exhausted" or "an error occurred while advancing the cursor," and the two are indistinguishable without an explicit check — so a fault partway through either stream (a dropped connection, a corrupted page) rendered a truncated breakdown as if it were complete, with no signal anything was missing. Found by the independent review pass on bug #452's fix, which deliberately scoped #452 to the top-level query call rather than per-row iteration. The scan loops are now `scanSourceConversions`/`scanVariantConversions`, both returning an error on `rows.Err()` that flows into #452's existing 500 path.

## 2026-07-30 — The metrics API stops lying on a real query failure

* **Fix (bug #452):** `cmd/dashboard`'s `/api/metrics` handler ran nine independent queries in parallel and logged each one's error but always answered `200 OK` with whatever zero/stale values the failed queries left behind — a genuine database failure (a locked file, a dropped table, a dead connection) looked identical to "nothing has happened yet." The handler now returns a real `500` when a query genuinely fails, while a legitimately empty table (`sql.ErrNoRows`) still renders as zero exactly as before.

## 2026-07-30 — The Working Protocol now keeps this changelog current

* **Fix (improvement #454):** the Working Protocol's close-the-loop step (step 7 in `improvements.md`, shared by `bugs.md`) named the backlog row, the task journal, the build/vet/test run, the commit, and the push — but never this file. A session could follow the protocol exactly and still leave the changelog stale; five bug fixes shipped on 2026-07-30 alone (#436, #437, #441, #445, #446) before this file's most recent entry was updated by hand rather than by the protocol. Step 7 now requires a dated entry here in the same commit for any user-visible change, explicitly excluding internal refactors, backlog-only edits, and ignored/unused scripts.

## 2026-07-30 — Both database connections finally agree, and the setup path stops lying

* **Fix (bug #446):** `cmd/dashboard` opened `"./applications.db?_journal_mode=WAL"` — the `mattn/go-sqlite3` pragma spelling, which the pure Go `modernc.org/sqlite` driver accepts without complaint and then ignores. Bug #416 had corrected exactly this and named both `pkg/storage/manager.go` and `cmd/dashboard/main.go`; only the first was ever changed. Both now build their DSN from one `storage.DSN` helper, and a test pins the dashboard's to it so the two cannot fork again.
* **What that actually repaired, which was not what the bug report claimed.** Verified against real binaries rather than tests: on a database already in WAL mode, readers never block on the writer, so the reported "dashboard reads 0 while the agent writes" symptom did not reproduce at all. The genuine consequence was that a dashboard reaching a *new* database first created it in rollback-journal mode (`journal_mode=delete`, no `-wal` file) instead of WAL. That is fixed and confirmed by running both binaries in an empty directory.
* **Fix (bug #445):** any page open in any tab of your browser could `POST` to `/api/agent/start` or `/api/agent/stop` and launch or kill a real application run, because the handlers checked only the HTTP method. A cross-origin `POST` with no custom headers is a CORS "simple request" and is never preflighted, and the loopback-only bind is no defense against traffic that originates on the same machine. Both endpoints now require same-origin, trusting `Sec-Fetch-Site` first and falling back to host-matching `Origin` then `Referer`. Requests carrying none of the three are still allowed, so `curl` and scripts keep working — a deliberate tradeoff, documented in the README.
* **Fix (bug #437):** the TypeScript dashboard rewrite (#426) had silently deleted improvement #15's conversion analytics and improvement #34's table accessibility markup. The API had been serving `by_source`, `by_variant` and `interview_rate_pct` on every poll to a UI that rendered none of them. Both are restored, with `<caption>` and `scope` semantics.
* **Fix (bug #441):** a clean install ended up configured for models the installer had never pulled — `.env.example` shipped one set, `install_ollama.sh`/`.ps1` pulled another. They now agree, and `cmd/agent` preflights Ollama's `/api/tags` at startup and refuses to run against an absent model instead of failing per-job much later.
* **Fix (bug #436):** `//go:embed ui/dist` pointed at a gitignored build artifact, so `go build ./...` succeeded only in a working tree that happened to have it. No fresh clone could build. `dist/` is now committed on purpose.
* **Docs:** the README documents the same-origin guard, the shared DSN helper, and the driver's pragma syntax; two stale ADRs were corrected.
* **Fix (bug #451):** the Failed and Manual Queue tiles each count two statuses with unrelated meanings (`FAILED_SCORE`/`FAILED_SUBMIT`, `MANUAL_REQUIRED`/`AWAITING_REVIEW`) but captioned only one member of each pair, so a run whose failures were all scoring failures still reported "reached the form but failed to submit." The API now reports each status's count separately and the UI captions whichever one(s) actually contributed.

## 2026-07-29 — Document tailoring works again, and no longer needs a companion service

* **Fix (bug #439):** the per-job tailoring path hardcoded `"provider": "ollama"` and `"model": "llama3"` into its request to the NLP microservice, ignoring `LLM_PROVIDER` and `OLLAMA_MODEL` entirely. `llama3` is not installed by any setup step in this project, so with `use_master_cover_letter: false` — the toggle that turns on the project's headline feature — every generation asked for a model that did not exist and the job failed after three retries. Provider, model, host, context size and timeout are now threaded from the resolved configuration.
* **Generation is in-process again by default.** Improvement #427 had made the Python microservice a hard, manually-started dependency of tailoring. It is now an optional offload behind `NLP_SERVICE_URL`, health-checked before use, with a fallback to in-process generation if it is unreachable or fails mid-job. Nothing external is required to generate documents.
* **Restored what #427 dropped:** the per-call payload circuit breaker and `[API Metrics]` logging (absent from this path entirely), the dynamic `num_ctx` that stops Ollama truncating long postings to its own default window, the provider abstraction (a Claude or Gemini user was silently getting a local Ollama call), the 120-minute provider timeout in place of a hardcoded 10-minute Go and 5-minute Python deadline that could not finish a real CPU generation, and four prompt instructions lost when the prompts were retyped in Python.
* **The prompts now live in Go only** and are sent over the wire, so the service owns no prompt text and the copies cannot drift again.
* **`nlp_service` rewritten** as a generic concurrent Ollama executor with `GET /health` and `POST /generate`, per-call error isolation (one failed call no longer aborts the batch), and 12 tests.
* **New:** an empty document from the model is now an error instead of a saved empty resume, and `scripts/verify_tailoring.go` drives one real generation through either route so the path can be checked without waiting on a batch run.
* **Docs:** corrected the Ollama timeout default, documented `NLP_SERVICE_URL`, and removed the README's known-defect warning box for this bug.

## 2026-07-29 — Hand-off applications can now be recorded and tracked

* **Fix (bug #434):** nothing ever moved a job out of `MANUAL_REQUIRED` or `AWAITING_REVIEW`, so every application the user completed by hand stayed recorded as un-submitted and its rejection or interview email correlated to nothing.
* **New command `cmd/reconcile`:** reads the hand-off checklists and records every ticked entry as applied, including the deduplication row so the agent never re-applies to a job the user sent. Dry run by default; `-confirm` to write. It refuses any row that has already moved past the hand-off stage, so a stale tick cannot overwrite a recorded outcome.
* **Tracker:** outcome emails now correlate against hand-off rows, not just `APPLIED` ones. `AWAITING_REVIEW` was also missing from the tracked-company set entirely. The rollback-rather-than-guess behaviour for ambiguous multi-row matches is unchanged.

## 2026-07-29 — Human-in-the-Loop Copilot Mode

* **Feature:** Added `copilot_mode` to `profile.yaml`. The agent performs every step of an application — discovery, scoring, tailoring, and confirming the form is reachable and fillable — then stops before the final submit click. Jobs are recorded as `AWAITING_REVIEW`, their tailored documents move to `applications/needs_manual_apply/`, and each is queued in `copilot_queue.md` with its apply URL. The agent's own form fill happens in an ephemeral automated browser session and does not carry into the user's browser; the documents and the vetting are what carry over.
* **Ranking:** Attempts that stop before the submit click are excluded from source-health success rates and penalties. Counting them would have collapsed every board's score during a copilot run and kept doing so for the full 30-day lookback afterwards.
* **Fix:** `auto_submit_click: false` previously returned no error, which the pipeline read as success and recorded as `APPLIED` with a permanent dedup row, for forms that were never submitted. It now routes to `AWAITING_REVIEW` and never reaches the application record.
* **Structure:** All six submit-click sites and the outcome-confirmation path now consult one shared `submitGate` helper instead of each testing the flags themselves, so a submission gate cannot be wired into one ATS handler and missed in another.
* **Dashboard:** `AWAITING_REVIEW` is reported as a needs-action outcome and counted in the metrics API.

## 2026-07-27 — Documentation drift correction

* **Setup:** Added a fake-data-only `pii.yaml.template` and a parser test so clean-checkout setup remains safe and schema-valid.
* **README:** Corrected payload limits, provider-specific LLM timeouts, and CPU performance guidance; documented `OLLAMA_TIMEOUT_MINUTES` and the template link.
* **History:** Marked superseded daemon and `net.ParseIP` security wording as historical instead of describing it as current behavior.
* **Checks:** Added lightweight tests for the template and required README setup entrypoints.

## 2026-07-27 — OS-specific run documentation

* **Documentation:** Replaced the Linux-only setup path with concrete run instructions for Windows, macOS, mainstream Linux distributions, immutable Linux, and WSL 2.
* **Dashboard:** Added separate UI-dashboard launch, browser-access, loopback-address, and shared-database guidance.
* **Setup:** Corrected the obsolete reference to a nonexistent `pii.yaml.template`; the README now gives a minimal local PII file and preserves the user-controlled legal-attestation boundary.

## 2026-07-27 — Post-SSRF backlog grooming

* **Backlog:** Re-verified and recomputed all 3 Pending bugs, 5 free improvements, and 2 paywalled improvements after bug #122.
* **Priority:** Bugs #128 and #112 remain Major, so the Usability Gate stays open and company-only artifact storage, bug #128, is next.
* **Scoring:** No score or rank changed. The groom removed the now-fixed live SSRF gap from improvement #36's rationale while retaining its 5.0 score for broken clean setup and false payload, timeout, performance, and historical claims.
* **Recommendations:** Close free improvements #27 and #30 absent new evidence, defer paid-compute #14 until preference labels exist, and keep CAPTCHA solving gated on a user-selected paid provider and key.
* **Live checks:** Required Ollama models remain installed, both dashboard routes return HTTP 200 on loopback, no Career Agent batch process is running, and read-only aggregates reconfirm 20 scheme pairs with 15 divergent statuses.
* **Verification:** The uncached build, vet, full test suite, and focused security, scraper, submitter, and agent race suites pass.

## 2026-07-27 — Resolver-bound outbound network security

* **Security:** Centralized HTTP and HTTPS target validation rejects malformed targets and any DNS answer set containing loopback, private, link-local, multicast, carrier-grade NAT, documentation, benchmarking, transition, or other special-use addresses.
* **Rebinding defense:** Go transports pass validated IP literals to an injectable final dialer. Initial requests, redirects, job-page fetches, RemoteOK, Hacker News, SerpApi, Yahoo, and public ATS feeds all use the guarded transport.
* **Browser boundary:** Every Playwright context uses an ephemeral authenticated loopback proxy backed by the same resolver-bound dialer. The route interceptor independently revalidates requests, and Chromium's implicit loopback bypass is disabled.
* **Tests:** Added isolated IPv4 and IPv6 policy, mixed-answer, redirect, rebinding, proxy authentication, HTTP forwarding, CONNECT tunneling, and browser-context tests. The opt-in Chromium integration passes inside the documented distrobox, proving public proxy traversal and loopback denial.
* **Verification:** `go build ./...`, `go vet ./...`, `go test ./...`, and focused race suites for security, scraper, submitter, and agent packages pass.

## 2026-07-27 — Post-daemon backlog grooming

* **Backlog:** Re-verified and recomputed all 4 Pending bugs, 5 free improvements, and 2 paywalled improvements after bug #120.
* **Priority:** Three Major or Blocker bugs still hold the Usability Gate open. Resolver-bound SSRF protection, bug #122, is next.
* **Scoring:** Raised outcome-aware queue ranking from 1.0 to 1.2 because capped daemon cycles make queue order more consequential. Correct live schema-key checks show the motivating work attestations configured, so early attestation detection returns from 0.40 to 0.20.
* **Recommendations:** Close free improvements #27 and #30 absent new evidence, defer paid-compute #14 until preference labels exist, and keep CAPTCHA solving gated on a user-selected paid provider and key.
* **Live checks:** Required Ollama models remain installed, both dashboard routes return HTTP 200 on loopback, and no Career Agent batch process is running.
* **Verification:** The uncached build, vet, full test suite, and focused agent race suite pass.

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
* **Historical note:** This entry described an earlier daemon implementation and is retained for release history. The current daemon lifecycle is documented and tested in the 2026-07-27 capped-cycle entry above.

## Iteration 2 Audit Fixes (2026-07-16)
* **UI/UX & Accessibility:** Rewrote the terminal dashboard (`cmd/dashboard/main.go`). It no longer uses destructive ANSI clear-screen loops (which broke screen readers) and now hosts a clean, modern HTML web interface via standard library `net/http`.
* **Security (Path Traversal):** Hardened `SaveApplication` in `pkg/storage/manager.go` to aggressively strip malicious characters and path separators from `companyName` before allocating file paths.
* **Historical note:** This entry described an earlier string and `net.ParseIP` check. The current resolver-bound, DNS-rebinding-resistant policy is documented in the 2026-07-27 resolver-bound networking entry above.
* **Resilience (Race Condition):** Added a `sync.Mutex` lock to `LogFailedSubmission` to prevent interleaved or corrupted data when 10 goroutines write to `manual_submissions.md` concurrently.
* **Resilience (File Deletion):** Fixed an accidental destructive cleanup bug in `AttemptSubmit` where workers would delete the master resume/cover letter from disk instead of generating a copy.
* **Documentation:** Authored comprehensive Architecture Decision Records (`ADR-001`, `ADR-002`, `ADR-003`) detailing our Playwright pool, Prompt Injection, and SQLite logic. Added a `CONTRIBUTING.md` and Mermaid architecture diagram.
