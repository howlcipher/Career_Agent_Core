# 🐛 Bug Backlog

This document is the authoritative, ranked backlog for known flaws, bugs, and broken items in Career Agent Core. It mirrors the structure of `improvements.md` and follows the same Working Protocol defined there: open a task journal, re-evaluate the model against what is currently available, route the matching library skill, then fix, verify, commit, and push. Bugs are prioritized independently of new features and generally outrank improvement work of similar effort — and while the Usability Gate below is unmet, bugs outrank *everything* in `improvements.md`, full stop.

## 🎯 Usability Gate — what "100% usable" means

**MET 2026-08-06 (Assisted Apply acceptance trial complete, last Major closed).** The trial finished: **five real applications submitted and confirmed** — Grafana Labs, Affirm, Smartsheet, and Temporal Technologies via Greenhouse, and Veeva via Lever — each with exactly one dedup record, `manual_user_confirmation` provenance, a cleared lease, and removal from the actionable queue. `applied_jobs` moved 53 → 58, one row per application. Four Blockers/Majors were found and closed during the run (#515 résumé placeholder, #516 missing location gate, #517 cover letters unreachable, #518 unconfirmable applications), plus the jobgether aggregator exclusion, and #519/#520/#521 were closed in the days after it.

**The gate is still met, and #534 now closes the last known way a real outcome email could be lost.** Re-verified 2026-08-08: zero open Blocker or Major rows. **#534** (`StartTracker` only ever fetched the newest ~51 messages, so downtime longer than ~4 hours of this inbox's traffic put outcome mail permanently out of reach) is fixed: the tracker now tracks a durable IMAP UID checkpoint (`tracker_cursor`) with an independent, bounded historical catch-up range seeded from live evidence rather than a hardcoded lookback — see ADR-004 and `documentation/backlog_history/bugs_done_details.md` item #534 for the full design and production validation. **A second defect was found and fixed during that same validation, before it ever shipped**: a gap in IMAP UID numbering (deleted mail) could stall catch-up forever under the first version of the fix — confirmed live, then fixed and re-verified live before the task was considered done. The fixed binary is deployed and running in production as of this close: bootstrap found a real ~2009-UID historical catch-up window, three deliberate restarts all resumed correctly from the persisted checkpoint, `processed_emails`/`unmatched_outcomes` counts stayed idempotent across repeated passes over already-covered mail, and a single tracker process remained running throughout. No matched (REJECTED/INTERVIEW_REQUESTED) outcomes were recovered within this session's observation window — catch-up had progressed roughly 30% through its historical span by the end of the session, entirely over territory the pre-#534 tracker had already covered — so usable matched-outcome data is still **0**; the fix guarantees eventual coverage across future scans, not instant coverage in one sitting, and the tracker was left running to keep draining on its own 15-minute cadence. `go build ./...`, `go vet ./...`, `go test ./...` (including 10 new `pkg/tracker` tests run against a real in-process IMAP server, plus all 22 pre-existing ones), and `gofmt -l ./cmd ./pkg ./internal` all pass. *(prior status paragraph archived in `documentation/backlog_history/bugs_groom_history.md`.)*

This project reaches 100% usable when every box below is checked. Until then, this is the default work queue ahead of any Pending row in `improvements.md`; everything in that file is explicitly nice-to-have and out of scope until this gate is met.

- [x] `go build ./...` succeeds clean — re-verified 2026-07-30 **in a fresh `git clone`, not just in this working tree** (clone of `d69edb0`: build, vet and the full test suite all clean with no `.env` and no untracked artifacts). That distinction is now part of the box: bug #436 found this had been silently false since #426 shipped, because `//go:embed ui/dist` depended on a gitignored build artifact that only existed locally. Verify this box the way a new user would.
- [x] `go vet ./...` reports no issues — re-verified 2026-07-30 (working tree and fresh clone)
- [x] `go test ./...` passes for every package that has tests — **re-verified 2026-07-30** in both the working tree and a fresh clone, including the 22 tests added for #441. *(prior verification history archived in `documentation/backlog_history/bugs_groom_history.md`.)*
- [x] A working local Ollama install with the models `cmd/agent` needs — **re-verified 2026-07-30 by the agent itself**, which is the point of #441's fix: `cmd/agent` now preflights `/api/tags` at startup and refuses to run against an absent model, and the real binary logged `Model preflight passed against http://localhost:11434: qwen3:30b-instruct, qwen2.5vl:7b, nomic-embed-text` on this host. `qwen3:4b-instruct` is also present. This box no longer needs a human to check it by hand, and it can no longer be silently false. Note on the box's original wording (`llama3.1`, `llava`, `nomic-embed-text`): those are the installer's defaults and, after #441, also what `.env.example` resolves to for a fresh user; this machine deliberately runs the larger qwen models via its own `.env`, and the installer now follows that file rather than contradicting it.
- [x] `cmd/agent` completes one full batch run against live job boards end to end — discover → score → tailor (resume + cover letter) → submit or log to `applications/manual_submissions.md` → row written to `applications.db` — with zero crashes. **MET 2026-07-23 12:00:** a real Lever posting (`jobs.lever.co/smarsh/...`) went discover → score (85) → tailor (real resume + cover letter generated) → submit (`handleLever`, real form fill + click-submit) → `job_funnel.status = APPLIED` in `applications.db`, zero crashes, in a single clean run. This was the culmination of bugs #45/#46 (two independent CAPTCHA-detection false positives that had been killing the large majority of Greenhouse/Lever/Ashby/Workable jobs before they ever reached fit-scoring or the fill stage) and #47 (the dedicated Lever/Greenhouse handlers' own missing click-to-reveal step, only exposed once #45/#46 stopped killing the job earlier). See #45/#46/#47's Details sections for the full diagnostic chain. **Progress 2026-07-20 (extended live-testing session, ~6 hours):** bug #3 (Ollama context/concurrency) fixed and verified — a real job completed discover → score → tailor cleanly with zero Ollama errors and reached the actual Playwright submit step for the first time. Bug #4 (form-fill) was re-diagnosed from a wrong "timeout" theory to the real cause (forms embedded in iframes were never searched) and fixed in code, but **could not be verified live** — a new Blocker (#6, Ollama generation throughput collapsing mid-request) emerged and prevented most attempts from even reaching the fill step. **Progress 2026-07-21:** bug #6 resolved — root cause was a too-short hardcoded 10-minute client timeout racing against genuinely slow (but honest) CPU generation at long context, not context-shift thrashing; timeout is now configurable and defaults to 45 minutes. Bug #4's fix is still unverified live — that requires an actual full batch run, not yet attempted. **Progress 2026-07-22 (Claude Code session, ~6 hours, live triage with the user watching):** found and fixed a chain of real, independently-verified blockers, most reached only after the previous ones were cleared: (1) three duplicate/orphaned agent processes running simultaneously (same class as the Operational Trap below, recurred despite being documented) fighting over one Ollama instance — killed down to one clean process; (2) three files/dirs under `applications/` silently failing to write (`permission denied`) because they were owned by a stale UID `524288` left over from an earlier containerized run — the manual-apply queue and manual-submissions log had been dropping entries with no record at all; (3) **the dominant root cause of the whole session's "First Name" timeout pattern turned out to be environmental, not code**: `cmd/agent` was running on the bare Bazzite host again (see the Resolved-but-regressed entry below) where Chromium renders pages completely blank while reporting navigation success — moved everything back into the `career-agent` distrobox and confirmed real page content renders again; (4) a cookie-consent banner's backdrop `<div>` intercepts every click site-wide until dismissed, silently defeating `clickApplyIfPresent` (bug #34, new); (5) SmartRecruiters uses "I'm interested" instead of any "Apply" wording, and clicking it can reveal a fresh DataDome challenge the earlier captcha check never saw since it ran before that click (bug #35, new); (6) Jobvite gates the real form behind a "Data Consent" `<select>` — zero fields exist in the DOM until an option is chosen (bug #36, new); (7) `fillActionTimeoutMs` bumped 15000→30000: confirmed live that even a single clean instance can still lose the fill race to a co-located Ollama generation burst (bug #37, new). Also excluded `breezy.hr` from discovery (0 real applies across 212 attempts) and deprioritized Workday in the backlog query (bug #38, new) so platforms that can actually reach `APPLIED` stop being crowded out. **Still not verified:** despite all of the above, no fresh `APPLIED` was produced this session — the last real fill attempt (`brightvisiontechnologies.applytojob.com`) hit a *different*, smaller bug in the Vision-fallback path (bug #39, new, open) rather than any of the issues just fixed. Next session should resume with a clean single instance already running in the container and watch for the first real post-fix `APPLIED`.
- [x] `cmd/dashboard` serves and displays live, correct data from a populated `applications.db` — **re-verified 2026-07-30 (groom pass after #443):** a second instance built from the current tree and run against the real `applications.db` on `127.0.0.1:8099` returned HTTP 200 with real, correctly-typed field data from `/api/metrics` (this was also #447's live verification). The production dashboard on `:8080` was left untouched throughout. *(prior verification history archived in `documentation/backlog_history/bugs_groom_history.md`.)*
- [x] `cmd/tracker` runs against real IMAP credentials for at least one poll cycle without crashing (or no-ops cleanly per its existing missing-credentials guard) — **re-verified 2026-08-07 in the first full production validation since #529 and #533 shipped:** nine consecutive scans against the real inbox, zero errors of any kind, 120 messages acknowledged, 19 outcome-shaped emails durably recorded in `unmatched_outcomes`, and two scans observed end to end that correctly wrote nothing. The box has outgrown "does not crash": it now also records that the outcome path persists real evidence and that its acknowledgement and its unmatched-outcome record are atomic on live data. What this run did *not* cover, and the next session should not assume: no email matched any application, so the matched-outcome path and #529's ledger stamping remain unexercised in production. See the gate status paragraph above for the full evidence, and #534 for why nothing matched. *(prior note — the 2026-07-22 crash-safety verification and the 2026-07-21 malformed-app-password attempt — archived in `documentation/backlog_history/bugs_groom_history.md`.)*
- [x] Zero open bugs below tagged `Blocker` or `Major` in the Ranked Backlog — **Re-checked 2026-08-08 after closing #534:** still zero. The five open rows are #522, #524, #526, #528 and #532, all Minor. **Prior — Checked 2026-08-06:** the acceptance trial's last Major closed. #521 (indistinguishable duplicate cards, which caused a false APPLIED record) closed 2026-08-06; #520 (Lever submissions fail in the assisted browser) and #519 (assisted prefill non-functional on Greenhouse/Lever) closed the same day. Only Minor rows (#522-#525) remain open. *(prior verification history archived in `documentation/backlog_history/bugs_groom_history.md`.)*

Every session — Claude Code, Gemini CLI, or manual — that touches this repo should glance at this checklist. When the last box is checked, change the Status line to `MET (YYYY-MM-DD)` and add a one-line note on what was verified; from that point on, `improvements.md`'s Pending rows become fair game for normal ROI-ranked selection instead of being blocked behind this gate.

**⚠️ Operational trap when running `cmd/agent` live via `go run`:** `go run` does not exec into its compiled binary — it stays alive as a wrapper process around a separately-spawned child (visible as `/tmp/go-build.../b001/exe/main`). Killing only the `go run cmd/agent/main.go` PID (e.g. `kill -9 <pid>` or `pkill -f`) does **not** kill that child, which keeps running orphaned in the background indefinitely. Confirmed live 2026-07-21: five separate `go run` "relaunches" over one session left **five concurrent orphaned agent processes** running simultaneously for hours, all sharing the same `applications.db` and log file — this was the real cause of several confusing symptoms that session initially (wrongly) suspected were code bugs, including apparent duplicate job processing that persisted even after fixing #12, and Ollama OOM crashes worse than a single instance would cause. **To restart a live run cleanly: `go build -o /tmp/career_agent_bin ./cmd/agent` and run that binary directly**, so the PID you launch is the PID doing the work — no wrapper/child ambiguity, and `kill -9` on it is final.

**Recurred 2026-07-22** despite being documented: found three agent processes running simultaneously at the start of the session (two stale pre-fix binaries plus one current one), all sharing `applications.db` and the log file. One of the stale ones survived a plain `kill` and needed `kill -9` — a plain `SIGTERM` is not reliably sufficient for these orphans, always verify with `ps aux` after killing, not just trust the exit code.

**⚠️ Operational trap: the discovery queue is a one-time snapshot, not a live view.** `cmd/agent` calls `storage.GetDiscoveredJobs()` exactly once at startup and pushes the whole result into an in-memory channel — a running process never re-queries the database. Confirmed live 2026-07-23: neither a code fix nor a direct DB status change (e.g. resetting `BLOCKED_CAPTCHA` rows back to `DISCOVERED`) has any effect on an already-running instance; the only way to make either change "visible" is to kill the process and launch a freshly-built binary. This bit twice in the same session — once for a code fix (bugs #45/#46), once for a DB-only status change (the 830-row requeue below) — before being written down here.

**Lesson: a fix does nothing for jobs already marked with a bad status.** `GetDiscoveredJobs` only pulls `status = 'DISCOVERED'`; once something is `BLOCKED_CAPTCHA`/`FAILED_SUBMIT`, it sits there forever regardless of what code ships later. Confirmed live 2026-07-23: bugs #45/#46's CAPTCHA-detection fix produced zero new `APPLIED` results until 830 stale `BLOCKED_CAPTCHA` rows (Greenhouse/Lever/Ashby/Workable) were manually reset — the fix was correct the whole time, but nothing was ever going to exercise it. **After any fix that changes whether a class of job can succeed, run `go run ./cmd/requeue -stats` to see current per-source outcome counts, then requeue the ones the fix should now unblock** (`-source <name> -status BLOCKED_CAPTCHA|FAILED_SUBMIT -confirm`, add `-clear-dedup` if documents were already generated for those jobs before they failed — see `ClearApplicationRecordsByURLPattern`'s doc comment in `pkg/storage/manager.go` for why). Don't requeue an entire source blindly if only one specific failure mode is understood to be fixed — some of that source's failures may have unrelated causes (done deliberately for bug #49: only the one diagnosed Greenhouse job was requeued, not all 14 of that source's `FAILED_SUBMIT` rows).

**Correction, 2026-07-31 (`/groom_backlogs`, this session): the "one-time snapshot" trap above is resolved for daemon mode as of the 2026-07-30 continuous-daemon cadence rework (`7beda37`/`38c3d41`/`47ecd7e`) and this documentation had drifted from the code without anyone noting it — filed under this file's own evidence rule for a doc claim contradicted by code.** `cmd/agent/main.go`'s `runAgentQueueCycle` (`:636`) now calls `deps.loadDiscovered()` — `storage.GetDiscoveredJobs` — itself, and `runAgentSchedule` (`:518`) calls `runAgentQueueCycle` fresh on every daemon cycle (immediately on success, after `cycleInterval` on `errNoEligibleJobs`), so a live daemon now re-queries the database every cycle rather than once at process start; discovery also runs in its own independent goroutine (`runDaemonDiscoveryLoop`, `:598`) writing straight to the DB. **A DB-only status change (e.g. resetting stale rows via `cmd/requeue`) is now visible to a running daemon within one cycle, with no restart required.** This is confirmed by reading the call graph, not yet by a fresh live requeue-while-running observation — the next session that requeues rows against a live daemon should confirm the pickup and can then drop this correction paragraph entirely. The trap's second half is untouched: `GetDiscoveredJobs` still only pulls `status = 'DISCOVERED'`, so a fix still does nothing for jobs already marked `BLOCKED_CAPTCHA`/`FAILED_SUBMIT` until requeued, and a **one-shot non-daemon batch run** (`runAgentCycle`, no `-daemon`) still loads its backlog exactly once at startup and exits, exactly as documented — this correction is scoped to daemon mode only.

**⚠️ Operational trap: polling Workable's board feeds in bulk gets the whole host blocked for a day.** Confirmed live 2026-08-07 while running #524's backfill: after fetching ~210 `apply.workable.com` account feeds at concurrency 6, the host returned **HTTP 429 for every path** — account feeds, per-job endpoints, everything — with `Retry-After: 84643`, i.e. **23.5 hours**. This is a Cloudflare host-wide block, not a per-request rate limit, so no pacing or backoff *within* a run recovers from it; the only cure is waiting. Greenhouse and Lever were unaffected throughout and kept returning 200.

Two consequences worth remembering. First, **retrying a 429 harder makes this worse, not better** — an earlier version of the backfill treated 429 as "be more patient" and spent 35 seconds of backoff per request hammering a host that had already said stop, turning a 2-minute run into an hour. `pkg/scraper`'s `FeedHTTPError` now carries `Retry-After`, and `cmd/backfill-location` gives a host up for the rest of the run when it exceeds two minutes. Second, `discoverWithATSFeeds` polls up to `maxBoardsPerSource` (60) boards per source per pass and has no such protection; it has not been observed tripping this, but it is the same shape of traffic against the same host, so treat a sudden collapse in Workable discovery yield as a suspected block rather than a code bug, and check `Retry-After` before changing anything.

**Technique: targeted single-job verification via `TARGET_JOB_URL`.** `cmd/agent` reads this env var at startup; when set, it restricts the run to exactly that one already-`DISCOVERED` job and skips fresh `FunnelEngine` discovery entirely (`TARGET_JOB_URL="https://..." /path/to/binary`). This is how bugs #46-#49 were verified end-to-end in minutes instead of waiting on normal queue order, and without disturbing a separately-running full batch. The job must actually be in `DISCOVERED` status first (use `cmd/requeue` or direct SQL if it's currently in some other status), and if it previously reached document generation, clear its `applied_jobs` dedup row too or `HasApplied` will skip it as a duplicate.

**⚠️ Deliberately NOT built, and it must stay that way: pre-emptively skipping jobs whose page carries a bot-protection widget.**

Each captcha-blocked job costs ~10 minutes of fit-scoring before the block is discovered — measured 2026-07-26 across 8 blocked boards, roughly 7+ hours of compute over a cohort this size. The obvious optimisation is to detect the provider frame at page load and skip before scoring. **Do not do it.**

**Presence is not blocking.** These systems are score-based. Measured the same night: `greenhouse.io/akuity` carries a reCAPTCHA Enterprise frame **and its submit was accepted** (security-code email timestamped 23:40:07), while `greenhouse.io/clickhouse` carries no frame at all and also succeeded. A presence-based pre-skip would have discarded Akuity — a job that genuinely submitted.

That is exactly **#45/#46**, where captcha false positives killed the large majority of Greenhouse/Lever/Ashby/Workable jobs before they ever reached fit-scoring, and produced zero applications until 830 rows were manually reset. Skipping a job that would have submitted is strictly worse than wasting inference, because the goal is applications, not throughput.

The narrow detections that **are** safe — #99, #101, #104 — all fire only **after** a submit has already produced no outcome, so they cannot pre-empt a working job. Any future optimisation here must preserve that property. A per-board history rule might eventually justify skipping, but 3 Lever data points are not a basis for excluding 39 jobs.

## Ranked Backlog (best ROI first)

Pending bugs carry the same diminishing-returns score defined in `improvements.md` (Score = Value × Decay ÷ Effort, ROI floor 0.5). Bugs rarely decay — a defect's cost does not shrink because other defects were fixed — so Decay is normally 1.0. A bug below the floor stays open, flagged ⚠️, and needs explicit user confirmation before being worked. When a new bug is found (including one surfaced while checking the Usability Gate above), add a row here with a Severity (`Blocker` | `Major` | `Minor`) and a matching detail section, then work the table top down.

**2026-08-06 groom pass.** Every open row re-verified against current code, not its own prose. #522's four claims all still hold (`serveAgentStop` writes `{"status": "stopped"}` immediately after `SIGTERM` at `cmd/dashboard/main.go:1379`; `cmd.Start()` at `:1348` is never `Wait()`ed; `daemon_active` is a pure settings comparison with no liveness component at `pkg/config/effective_settings.go:61`; `go run ./cmd/assist` at `:1208` still wins over `career_assist_bin` at `:1217`). #526 confirmed at `pkg/submitter/network.go:94-100`, including its fix direction — `security.NetworkRejectionReason` is exported, `safeAssistedHost` is not. #524 confirmed against the live database: **0 of 524** queued rows carry a location, while 28 of 12,980 funnel rows now do, so #516's gate is working forward-only exactly as the row states. #528 was re-scored down from 1.5 to 1.0 (see its Details section — it was filed hours earlier with an Effort its own fix direction contradicts). #529 was filed this pass. Six legacy `## N` narrative sections were moved out of this file under the Working Protocol's step 8; three had no table row at all. *(Prior status paragraph archived in `documentation/backlog_history/bugs_groom_history.md`.)*

| # | Bug | Severity | Status | Score (V×D÷E) | Tier | ROI rationale |
|---|---|---|---|---|---|---|
| 521 | [Indistinguishable duplicate cards let one click mark a job applied that was never submitted](#521-indistinguishable-duplicate-cards-let-one-click-mark-a-job-applied-that-was-never-submitted) | Major | Done (2026-08-06) | — | standard | See `documentation/backlog_history/bugs_done_details.md` item #521 for the full account. |
| 523 | [The assisted browser's network guard aborts requests silently](#523-the-assisted-browsers-network-guard-aborts-requests-silently) | Minor | Done (2026-08-06) | — | mechanical | See `documentation/backlog_history/bugs_done_details.md` item #523 for the full account. |
| 519 | [Assisted Apply cannot prefill on Greenhouse or Lever, the only two ATSes it is used with](#519-assisted-apply-cannot-prefill-on-greenhouse-or-lever-the-only-two-atses-it-is-used-with) | Major | Done (2026-08-06) | — | deep-reasoning | See `documentation/backlog_history/bugs_done_details.md` item #519 for the full account. |
| 525 | [Assisted Apply attaches a .txt extraction where the automatic path uploads the master cover letter PDF](#525-assisted-apply-attaches-a-txt-extraction-where-the-automatic-path-uploads-the-master-cover-letter-pdf) | Minor | Done (2026-08-06) | — | standard | See `documentation/backlog_history/bugs_done_details.md` item #525 for the full account. |
| 520 | [Lever submissions fail inside the assisted browser but succeed in an ordinary one](#520-lever-submissions-fail-inside-the-assisted-browser-but-succeed-in-an-ordinary-one) | Major | Done (2026-08-06) | — | deep-reasoning | See `documentation/backlog_history/bugs_done_details.md` item #520 for the full account. |
| 524 | [Existing queue rows carry no location, so #516's gate cannot screen them](#524-existing-queue-rows-carry-no-location-so-516s-gate-cannot-screen-them) | Minor | Pending — 290/524 backfilled, 144 confirmed dead, 161 in-scope rows still unresolved | 1.5 = 3×1.0÷2 | standard | Workable's host-wide block cleared; re-run on 2026-08-08 resolved 223 more rows (all Workable) and terminalized 144. 138 Workable + 23 Greenhouse/Lever rows remain unresolved after this run drew fresh transient Workable 429s partway through — re-run again after a cooldown rather than immediately. 75 rows remain out of scope (no readable public feed). |
| 534 | [`StartTracker` only ever reads the newest ~51 messages, so any outage longer than the fetch window loses outcomes permanently](#534-starttracker-only-ever-reads-the-newest-51-messages-so-any-outage-longer-than-the-fetch-window-loses-outcomes-permanently) | Minor | Done (2026-08-08) | — | standard | See `documentation/backlog_history/bugs_done_details.md` item #534 for the full account. |
| 530 | [A posting that has died still occupies a queue card, because the assisted queue never reads the funnel status it selects](#530-a-posting-that-has-died-still-occupies-a-queue-card-because-the-assisted-queue-never-reads-the-funnel-status-it-selects) | Minor | Done (2026-08-07) | — | standard | See `documentation/backlog_history/bugs_done_details.md` item #530 for the full account. |
| 531 | [Every queue card whose timestamp was written by Go reports `last_updated` as year 0001](#531-every-queue-card-whose-timestamp-was-written-by-go-reports-last_updated-as-year-0001) | Minor | Done (2026-08-07) | — | mechanical | See `documentation/backlog_history/bugs_done_details.md` item #531 for the full account. |
| 522 | [Agent lifecycle and liveness reporting are unreliable in four distinct ways](#522-agent-lifecycle-and-liveness-reporting-are-unreliable-in-four-distinct-ways) | Minor | Pending | 1.3 = 2×1.0÷1.5 | standard | None of the four blocks work, but together they make "is the agent running?" unanswerable without checking /proc by hand, which cost time repeatedly during the trial. |
| 527 | [The `:memory:` test database silently fails every nested query, so the assisted queue's readiness fields cannot be covered](#527-the-memory-test-database-silently-fails-every-nested-query-so-the-assisted-queues-readiness-fields-cannot-be-covered) | Minor | Done (2026-08-07) | — | standard | See `documentation/backlog_history/bugs_done_details.md` item #527 for the full account. |
| 529 | [49 emails processed, zero outcomes recorded — the tracker's detections may never be reaching `job_funnel`](#529-49-emails-processed-zero-outcomes-recorded--the-trackers-detections-may-never-be-reaching-job_funnel) | Minor | Done (2026-08-07) | — | standard | See `documentation/backlog_history/bugs_done_details.md` item #529 for the full account. |
| 533 | [An outcome email that matches no application is acknowledged and discarded, so the evidence is unrecoverable](#533-an-outcome-email-that-matches-no-application-is-acknowledged-and-discarded-so-the-evidence-is-unrecoverable) | Minor | Done (2026-08-07) | — | standard | See `documentation/backlog_history/bugs_done_details.md` item #533 for the full account. |
| 532 | [Every stage-ledger event has a NULL duration, because the trigger measures time with `julianday()` over a format SQLite cannot parse](#532-every-stage-ledger-event-has-a-null-duration-because-the-trigger-measures-time-with-julianday-over-a-format-sqlite-cannot-parse) | Minor | Pending | 0.7 = 2×1.0÷3 | standard | Found while closing #529. All 1,385 live ledger rows have `stage_duration_ms` NULL — the column has never once been populated. ADR-003 decision 5 claims the ledger records elapsed duration; decision 6 explains why it cannot. Fixing it needs a `DROP`/`CREATE TRIGGER` migration, which is why it is not a one-liner. |
| 526 | [The automatic submitter's network guard aborts requests silently](#526-the-automatic-submitters-network-guard-aborts-requests-silently) | Minor | Pending | 1.0 = 2×0.5÷1 | mechanical | Same defect as #523, one layer over: the automatic path's guard still blocks in silence, and its two existing log lines print Playwright errors verbatim. The classifier #523 built is already there to reuse. |
| 528 | [Assisted Apply attaches the "cover letters are disabled" note when send_cover_letter is false](#528-assisted-apply-attaches-the-cover-letters-are-disabled-note-when-send_cover_letter-is-false) | Minor | Pending | 1.0 = 2×1.0÷2 | standard | Last member of the #515/#517/#525 divergence family. Re-scored 2026-08-06: latent, not live (send_cover_letter is true), so Value 2; and it needs a "not applicable" state threaded through storage, the queue projection and cmd/assist, so Effort 2 and standard tier — not the mechanical one-liner it was filed as. |
| 518 | [A revalidated, already-submitted application cannot be confirmed from the dashboard](#518-a-revalidated-already-submitted-application-cannot-be-confirmed-from-the-dashboard) | Major | Done (2026-08-06) | — | mechanical | See `documentation/backlog_history/bugs_done_details.md` item #518 for the full account. |
| 517 | [Assisted Apply serves 404 for every cover letter once documents move to needs_manual_apply](#517-assisted-apply-serves-404-for-every-cover-letter-once-documents-move-to-needs_manual_apply) | Major | Done (2026-08-06) | — | standard | See `documentation/backlog_history/bugs_done_details.md` item #517 for the full account. |
| 516 | [Discovery has no geographic gate, so an India-only role reached a live application attempt](#516-discovery-has-no-geographic-gate-so-an-india-only-role-reached-a-live-application-attempt) | Blocker | Done (2026-08-05) | — | deep-reasoning | See `documentation/backlog_history/bugs_done_details.md` item #516 for the full account. |
| 515 | [Assisted Apply uploaded the saved reference note in place of the résumé](#515-assisted-apply-uploaded-the-saved-reference-note-in-place-of-the-résumé) | Blocker | Done (2026-08-05) | — | standard | See `documentation/backlog_history/bugs_done_details.md` item #515 for the full account. |
| 513 | [Application Mode hardening is incomplete: Qualified Jobs mutations and settings activation are not safely verified](#513-application-mode-hardening-is-incomplete-qualified-jobs-mutations-and-settings-activation-are-not-safely-verified) | Major | Done (2026-08-05) | — | standard | See `documentation/backlog_history/bugs_done_details.md` item #513 for the full account. |
| 514 | [Qualified Jobs and operator settings contain post-hardening runtime regressions](#514-qualified-jobs-and-operator-settings-contain-post-hardening-runtime-regressions) | Major | Done (2026-08-05) | — | standard | See `documentation/backlog_history/bugs_done_details.md` item #514 for the full account. |
| 512 | [Assisted Apply presents stale, mismatched, and blank employer pages as actionable human handoffs](#512-assisted-apply-presents-stale-mismatched-and-blank-employer-pages-as-actionable-human-handoffs) | Major | Done (2026-08-02) | — | standard | See `documentation/backlog_history/bugs_done_details.md` item #512 for the full account. |
| 508 | [Discovery has no independent current-listings fallback when SerpApi quota is exhausted and Yahoo search fails](#508-discovery-has-no-independent-current-listings-fallback-when-serpapi-quota-is-exhausted-and-yahoo-search-fails) | Major | Done (2026-08-01) | — | standard | See `documentation/backlog_history/bugs_done_details.md` item #508 for the full account. |
| 502 | [`encoding.go`'s homoglyph branch is a third zero-evidence heuristic threat source, outside #489's fix window](#502-encodinggos-homoglyph-branch-is-a-third-zero-evidence-heuristic-threat-source-outside-489s-fix-window) | Minor | Done (2026-08-01) | — | standard | See `documentation/backlog_history/bugs_done_details.md` item #502 for the full account. |
| 501 | [Re-run #489's aggregate quarantine-rate queries against fresh data once the fix has been live for a batch cycle](#501-re-run-489s-aggregate-quarantine-rate-queries-against-fresh-data-once-the-fix-has-been-live-for-a-batch-cycle) | Minor | Done (2026-08-01) | — | mechanical | See `documentation/backlog_history/bugs_done_details.md` item #501 for the full account. |
| 490 | [`job_funnel.applied_at` is declared in the schema but no code path ever writes it](#490-job_funnelapplied_at-is-declared-in-the-schema-but-no-code-path-ever-writes-it) | Minor | Done (2026-08-01) | — | mechanical | See `documentation/backlog_history/bugs_done_details.md` item #490 for the full account. |
| 489 | [`promptsec.Moderate()` still quarantines roughly half of everything discovered, disproportionately on Lever and Greenhouse](#489-promptsecmoderate-still-quarantines-roughly-half-of-everything-discovered-disproportionately-on-lever-and-greenhouse) | Major | Done (2026-08-01) | — | deep-reasoning | See `documentation/backlog_history/bugs_done_details.md` item #489 for the full fix account. |
| 482 | [breezy.hr postings are excluded from GetDiscoveredJobs entirely, so they accumulate in DISCOVERED forever with no terminal status](#482-breezyhr-postings-are-excluded-from-getdiscoveredjobs-entirely-so-they-accumulate-in-discovered-forever-with-no-terminal-status) | Minor | Done (2026-08-01) | — | standard | See `documentation/backlog_history/bugs_done_details.md` item #482 for the full account. |
| 507 | [The first post-#489 cohort reaches FAILED_SUBMIT 100% of the time, so its lower quarantine rate has not yet improved outcomes](#507-the-first-post-489-cohort-reaches-failed_submit-100-of-the-time-so-its-lower-quarantine-rate-has-not-yet-improved-outcomes) | Minor | Done (2026-08-01) | — | standard | See `documentation/backlog_history/bugs_done_details.md` item #507 for the full account. |
| 481 | [Aged DISCOVERED postings expire before the ranking algorithm's freshness decay ever surfaces them, starving the funnel](#481-aged-discovered-postings-expire-before-the-ranking-algorithms-freshness-decay-ever-surfaces-them-starving-the-funnel) | Major | Done (2026-08-01) | — | standard | See `documentation/backlog_history/bugs_done_details.md` item #481 for the full fix account. |
| 480 | [UpdateFunnelStatusRetryable never records a status_reason, so every RETRY_EXHAUSTED row loses its own root cause](#480-updatefunnelstatusretryable-never-records-a-status_reason-so-every-retry_exhausted-row-loses-its-own-root-cause) | Minor | Done (2026-08-01) | — | standard | See `documentation/backlog_history/bugs_done_details.md` item #480 for the full account. |
| 504 | [`state.TailoredContext` (our own RAG-generated content) can trip the same zero-evidence quarantine as #489, via a dedicated but unverified `QUARANTINED_RAG_CONTEXT` status](#504-statetailoredcontext-our-own-rag-generated-content-can-trip-the-same-zero-evidence-quarantine-as-489-via-a-dedicated-but-unverified-quarantined_rag_context-status) | Minor | Done (2026-08-01) | — | standard | See `documentation/backlog_history/bugs_done_details.md` item #504 for the full account. |
| 503 | [`TwoStepVerification`'s quarantine check never logs to the prompt-injection audit trail, undercounting the CSV total](#503-twostepverifications-quarantine-check-never-logs-to-the-prompt-injection-audit-trail-undercounting-the-csv-total) | Minor | Done (2026-08-01) | — | mechanical | See `documentation/backlog_history/bugs_done_details.md` item #503 for the full account. |
| 478 | [A DNS resolution failure never moves a job out of DISCOVERED, so one bad hostname spins the daemon forever](#478-a-dns-resolution-failure-never-moves-a-job-out-of-discovered-so-one-bad-hostname-spins-the-daemon-forever) | Major | Done (2026-08-01) | 3.0 = 6×1.0÷2 | standard | See `documentation/backlog_history/bugs_done_details.md` item #478 for the full fix account. |
| 475 | [Yahoo fallback still fails most discovery queries despite bug 130's retry and backoff fix](#475-yahoo-fallback-still-fails-most-discovery-queries-despite-bug-130s-retry-and-backoff-fix) | Major | Done (2026-07-31) | 0.83 = 5×0.5÷3 | standard | See `documentation/backlog_history/bugs_done_details.md` item #475 for the full fix account. |
| 476 | [`GetQueuePlan` has no `rows.Err()` check, so a cursor error silently truncates the requeue dry-run preview](#476-getqueueplan-has-no-rowserr-check-so-a-cursor-error-silently-truncates-the-requeue-dry-run-preview) | Minor | Done (2026-08-01) | **2.0** = 4×1.0÷2 | standard | See `documentation/backlog_history/bugs_done_details.md` item #476 for the full fix account. |
| 466 | [Retryable queue rows are reset to DISCOVERED and immediately selected again](#466-retryable-queue-rows-are-reset-to-discovered-and-immediately-selected-again) | Major | Done (2026-07-31) | 4.0 = 8×1.0÷2 | standard | See `documentation/backlog_history/bugs_done_details.md` item #466 for the full fix account. |
| 467 | [Playwright target closure aborts an application attempt without bounded browser recovery](#467-playwright-target-closure-aborts-an-application-attempt-without-bounded-browser-recovery) | Minor | Done (2026-07-31) | — | standard | See `documentation/backlog_history/bugs_done_details.md` item #467 for the full fix account. |
| 465 | [`internal/backlog`'s Pending-cell floor was a historical snapshot, and ordinary backlog progress tripped it](#465-internalbacklogs-pending-cell-floor-was-a-historical-snapshot-and-ordinary-backlog-progress-tripped-it) | Minor | Done (2026-07-30) | — | standard | See `documentation/backlog_history/bugs_done_details.md` item #465 for the full fix account. |
| 461 | [The runtime binary `career_agent_bin` is tracked in git and grows the repo on every rebuild](#461-the-runtime-binary-career_agent_bin-is-tracked-in-git-and-grows-the-repo-on-every-rebuild) | Minor | Done (2026-07-30) | 3.0 = 3×1.0÷1 | standard | See `documentation/backlog_history/bugs_done_details.md` item #461 for the full fix account. |
| 451 | [Two summary tiles caption a two-status count with the reason for only one of them](#451-two-summary-tiles-caption-a-two-status-count-with-the-reason-for-only-one-of-them) | Minor | Done (2026-07-30) | 4.0 = 4×1.0÷1 | standard | See `documentation/backlog_history/bugs_done_details.md` item #451 for the full fix account. |
| 452 | [`serveMetrics` swallows all nine of its query errors and answers 200 with zeros](#452-servemetrics-swallows-all-nine-of-its-query-errors-and-answers-200-with-zeros) | Minor | Done (2026-07-30) | 2.5 = 5×1.0÷2 | standard | See `documentation/backlog_history/bugs_done_details.md` item #452 for the full fix account. |
| 453 | [`GetQueuePlan` scans `discovered_at` into a non-nullable `time.Time` while its sibling uses `sql.NullTime`](#453-getqueueplan-scans-discovered_at-into-a-non-nullable-timetime-while-its-sibling-uses-sqlnulltime) | Minor | Done (2026-07-30) | 3.0 = 3×1.0÷1 | standard | See `documentation/backlog_history/bugs_done_details.md` item #453 for the full fix account. |
| 449 | [`pgrep -f career_agent_bin` matches any process whose command line merely contains that string](#449-pgrep--f-career_agent_bin-matches-any-process-whose-command-line-merely-contains-that-string) | Minor | Done (2026-07-30) | 2.5 = 5×1.0÷2 | standard | See `documentation/backlog_history/bugs_done_details.md` item #449 for the full fix account. |
| 445 | [Any web page open in your browser can start or stop the agent](#445-any-web-page-open-in-your-browser-can-start-or-stop-the-agent) | Major | Done (2026-07-30) | 3.5 = 7×1.0÷2 | deep-reasoning | See `documentation/backlog_history/bugs_done_details.md` item #445 for the full fix account. |
| 446 | [The dashboard's own database connection still uses the pragma syntax bug #416 was closed for fixing](#446-the-dashboards-own-database-connection-still-uses-the-pragma-syntax-bug-416-was-closed-for-fixing) | Minor | Done (2026-07-30) | 4.0 = 4×1.0÷1 | deep-reasoning | See `documentation/backlog_history/bugs_done_details.md` item #446 for the full fix account. |
| 447 | [The dashboard UI silently swallows failed start/stop clicks, and slow polls can render stale metrics](#447-the-dashboard-ui-silently-swallows-failed-startstop-clicks-and-slow-polls-can-render-stale-metrics) | Minor | Done (2026-07-30) | 2.0 = 4×1.0÷2 | standard | See `documentation/backlog_history/bugs_done_details.md` item #447 for the full fix account. |
| 434 | [No path moves a job out of `AWAITING_REVIEW` or `MANUAL_REQUIRED`, so hand-off statuses are permanent dead ends](#434-no-path-moves-a-job-out-of-awaiting_review-or-manual_required-so-hand-off-statuses-are-permanent-dead-ends) | Major | Done (2026-07-29) | — | standard | See `documentation/backlog_history/bugs_done_details.md` item #434 for the full fix account. |
| 444 | [A single 429 shuts the whole agent down, and the log blames Gemini whatever provider is configured](#444-a-single-429-shuts-the-whole-agent-down-and-the-log-blames-gemini-whatever-provider-is-configured) | Minor | Done (2026-07-30) | 2.5 = 5×1.0÷2 | standard | See `documentation/backlog_history/bugs_done_details.md` item #444 for the full fix account. |
| 441 | [A clean setup ends up configured for models the installer never pulled](#441-a-clean-setup-ends-up-configured-for-models-the-installer-never-pulled) | Major | Done (2026-07-30) | 7.0 = 7×1.0÷1 | deep-reasoning | See `documentation/backlog_history/bugs_done_details.md` item #441 for the full fix account. |
| 439 | [The NLP microservice call ignores your LLM configuration and hardcodes a model this host does not have](#439-the-nlp-microservice-call-ignores-your-llm-configuration-and-hardcodes-a-model-this-host-does-not-have) | Major | Resolved (2026-07-29) | — | standard | See `documentation/backlog_history/bugs_done_details.md` item #439 for the full fix account. |
| 437 | [The React rewrite deleted the dashboard's conversion analytics and accessibility semantics without replacing them](#437-the-react-rewrite-deleted-the-dashboards-conversion-analytics-and-accessibility-semantics-without-replacing-them) | Major | Done (2026-07-30) | 3.0 = 6×1.0÷2 | deep-reasoning | See `documentation/backlog_history/bugs_done_details.md` item #437 for the full fix account. |
| 440 | [`scripts/server.go` is the only script without `//go:build ignore`, so `go build ./...` compiles a dead generated file](#440-scriptsservergo-is-the-only-script-without-gobuild-ignore-so-go-build--compiles-a-dead-generated-file) | Minor | Done (2026-07-30) | 1.5 = 3×1.0÷2 | standard | See `documentation/backlog_history/bugs_done_details.md` item #440 for the full fix account. |
| 438 | [The React rewrite also deleted four tests, including both guards on the dashboard's loopback-only listener](#438-the-react-rewrite-also-deleted-four-tests-including-both-guards-on-the-dashboards-loopback-only-listener) | Minor | Resolved (2026-07-29) | 3.0 = 3×1.0÷1 | standard | See `documentation/backlog_history/bugs_done_details.md` item #438 for the full fix account. |
| 436 | [A fresh clone cannot build: the `go:embed`-ed UI bundle is gitignored](#436-a-fresh-clone-cannot-build-the-goembed-ed-ui-bundle-is-gitignored) | Major | Resolved (2026-07-29) | 8.0 = 8×1.0÷1 | standard | See `documentation/backlog_history/bugs_done_details.md` item #436 for the full fix account. |
| 435 | [`statusReason` is dead code for every status the dashboard actually needs it for](#435-statusreason-is-dead-code-for-every-status-the-dashboard-actually-needs-it-for) | Minor | Resolved (2026-07-29) | 1.5 = 3×1.0÷2 | standard | See `documentation/backlog_history/bugs_done_details.md` item #435 for the full fix account. |
| 433 | [`mergeStatuses` ranks four real statuses at 0, so scheme dedup can revive terminal jobs](#433-mergestatuses-ranks-four-real-statuses-at-0-so-scheme-dedup-can-revive-terminal-jobs) | Minor | Resolved (2026-07-29) | 1.25 = 5×1.0÷4 | standard | See `documentation/backlog_history/bugs_done_details.md` item #433 for the full fix account. |
| 432 | [`auto_submit_click: false` records a false `APPLIED` for a form that was never submitted](#432-auto_submit_click-false-records-a-false-applied-for-a-form-that-was-never-submitted) | Major | Done (2026-07-29) | 7.0 = 7×1.0÷1 | standard | See `documentation/backlog_history/bugs_done_details.md` item #432 for the full fix account. |
| 414 | [Enforce single-instance execution to prevent DB corruption and stuck jobs](#414-enforce-single-instance-execution-to-prevent-db-corruption-and-stuck-jobs) | Blocker | Done (2026-07-29) | 8.0 = 8×1.0÷1 | standard | See `documentation/backlog_history/bugs_done_details.md` item #414 for the full fix account. |
| 412 | [Duplicate check in pipeline.go resets APPLIED jobs back to DISCOVERED](#412-duplicate-check-in-pipelinego-resets-applied-jobs-back-to-discovered) | Blocker | Done (2026-07-28) | — | standard | See `documentation/backlog_history/bugs_done_details.md` item #412 for the full fix account. |
| 413 | [Enhance Greenhouse validation error resolver for <fieldset> and radio groups](#413-enhance-greenhouse-validation-error-resolver-for-fieldset-and-radio-groups) | Major | Resolved (2026-07-29, root-caused and fixed) | — | standard | See `documentation/backlog_history/bugs_done_details.md` item #413 for the full fix account. |
| 396 | [ATS board feed truncates large JSON feeds (30MB+)](#396-ats-board-feed-truncates-large-json-feeds) | Major | Done (2026-07-28) | — | standard | See `documentation/backlog_history/bugs_done_details.md` item #396 for the full fix account. |
| 393 | [Playwright Host missing dependencies to run browsers](#393-playwright-host-missing-dependencies-to-run-browsers) | Blocker | Done (2026-07-28) | — | standard | See `documentation/backlog_history/bugs_done_details.md` item #393 for the full fix account. |
| 394 | [QUARANTINED_PROMPT_INJECTION has massive false positive rate on legitimate jobs](#394-quarantined_prompt_injection-has-massive-false-positive-rate-on-legitimate-jobs) | Major | Done (2026-07-28) | — | deep-reasoning | See `documentation/backlog_history/bugs_done_details.md` item #394 for the full fix account. |
| 395 | [Validation loop times out waiting for Ollama context deadline](#395-validation-loop-times-out-waiting-for-ollama-context-deadline) | Major | Done (2026-07-28) | — | standard | See `documentation/backlog_history/bugs_done_details.md` item #395 for the full fix account. |
| 127 | [Sensitive credentials application data and generated documents are world-readable](#127-sensitive-credentials-application-data-and-generated-documents-are-world-readable) | Major | Resolved (2026-07-27) | — | standard | See `documentation/backlog_history/bugs_done_details.md` item #127 for the full fix account. |
| 123 | [Failed and non-2xx job-page fetches still proceed to expensive fit scoring](#123-failed-and-non-2xx-job-page-fetches-still-proceed-to-expensive-fit-scoring) | Major | Resolved (2026-07-27) | — | standard | See `documentation/backlog_history/bugs_done_details.md` item #123 for the full fix account. |
| 129 | [The agent hard-codes one developer-specific career-profile path](#129-the-agent-hard-codes-one-developer-specific-career-profile-path) | Major | Resolved (2026-07-27) | — | standard | See `documentation/backlog_history/bugs_done_details.md` item #129 for the full fix account. |
| 130 | [Yahoo fallback drops discovery on transient unexpected EOF responses](#130-yahoo-fallback-drops-discovery-on-transient-unexpected-eof-responses) | Major | Done (2026-07-28) | — | standard | See `documentation/backlog_history/bugs_done_details.md` item #130 for the full fix account. |
| 131 | [ATS board polling discards truncated JSON without retry](#131-ats-board-polling-discards-truncated-json-without-retry) | Minor | Done (2026-07-28) | — | standard | See `documentation/backlog_history/bugs_done_details.md` item #131 for the full fix account. |
| 121 | [Untrusted job text reaches embedding and scoring models before quarantine](#121-untrusted-job-text-reaches-embedding-and-scoring-models-before-quarantine) | Blocker | Resolved (2026-07-27) | — | deep-reasoning | See `documentation/backlog_history/bugs_done_details.md` item #121 for the full fix account. |
| 120 | [`--daemon` logs a six-hour drip mode but exits after one batch](#120---daemon-logs-a-six-hour-drip-mode-but-exits-after-one-batch) | Major | Done (2026-07-27) | — | standard | See `documentation/backlog_history/bugs_done_details.md` item #120 for the full fix account. |
| 122 | [SSRF defenses block literal private IPs but not hostnames that resolve to them](#122-ssrf-defenses-block-literal-private-ips-but-not-hostnames-that-resolve-to-them) | Blocker | Resolved (2026-07-27) | — | deep-reasoning | See `documentation/backlog_history/bugs_done_details.md` item #122 for the full fix account. |
| 128 | [Saving a second role at the same company overwrites the first role's documents](#128-saving-a-second-role-at-the-same-company-overwrites-the-first-roles-documents) | Major | Done (2026-07-27) | — | standard | See `documentation/backlog_history/bugs_done_details.md` item #128 for the full fix account. |
| 112 | [The same posting exists twice, once per URL scheme, and their statuses have diverged](#112-the-same-posting-exists-twice-once-per-url-scheme-and-their-statuses-have-diverged) | Major | Resolved 2026-07-28 | — | deep-reasoning | See `documentation/backlog_history/bugs_done_details.md` item #112 for the full fix account. |
| 125 | [Ambiguous outcome emails retry forever instead of entering manual review](#125-ambiguous-outcome-emails-retry-forever-instead-of-entering-manual-review) | Minor | Done (2026-07-28) | — | standard | See `documentation/backlog_history/bugs_done_details.md` item #125 for the full fix account. |
| 126 | [The unauthenticated dashboard binds every network interface while announcing localhost](#126-the-unauthenticated-dashboard-binds-every-network-interface-while-announcing-localhost) | Major | Resolved (2026-07-27) | — | standard | See `documentation/backlog_history/bugs_done_details.md` item #126 for the full fix account. |
| 124 | [The email tracker acknowledges a message even when its database update fails](#124-the-email-tracker-acknowledges-a-message-even-when-its-database-update-fails) | Major | Resolved (2026-07-27) | — | standard | See `documentation/backlog_history/bugs_done_details.md` item #124 for the full fix account. |
| 119 | [Free discovery sources are disabled when the SerpApi key is absent](#119-free-discovery-sources-are-disabled-when-the-serpapi-key-is-absent) | Major | Resolved (2026-07-26) | — | standard | See `documentation/backlog_history/bugs_done_details.md` item #119 for the full fix account. |
| 118 | [Resume-selector fallback work breaks every submitter path without a readable resume](#118-resume-selector-fallback-work-breaks-every-submitter-path-without-a-readable-resume) | Major | Resolved (2026-07-26) | — | standard | See `documentation/backlog_history/bugs_done_details.md` item #118 for the full fix account. |
| 117 | [A single mailbox fetch misses a code that IMAP has not indexed yet](#117-a-single-mailbox-fetch-misses-a-code-that-imap-has-not-indexed-yet) | Major | Resolved (2026-07-26, root-caused and fixed) | — | deep-reasoning | See `documentation/backlog_history/bugs_done_details.md` item #117 for the full fix account. |
| 116 | [The post-security-code resubmit still judged the page in one instantaneous read](#116-the-post-security-code-resubmit-still-judged-the-page-in-one-instantaneous-read) | Blocker | Resolved (2026-07-26, root-caused and fixed) | — | deep-reasoning | See `documentation/backlog_history/bugs_done_details.md` item #116 for the full fix account. |
| 115 | [Greenhouse splits the one-time code across eight single-character inputs](#115-greenhouse-splits-the-one-time-code-across-eight-single-character-inputs) | Blocker | Resolved (2026-07-26, root-caused and fixed) | — | deep-reasoning | See `documentation/backlog_history/bugs_done_details.md` item #115 for the full fix account. |
| 114 | [When the emailed code cannot be entered, nothing records what IS on the page](#114-when-the-emailed-code-cannot-be-entered-nothing-records-what-is-on-the-page) | Major | Resolved (2026-07-26, diagnostic added) | — | deep-reasoning | See `documentation/backlog_history/bugs_done_details.md` item #114 for the full fix account. |
| 113 | [The emailed code was retrieved and then discarded, because the code field had not rendered yet](#113-the-emailed-code-was-retrieved-and-then-discarded-because-the-code-field-had-not-rendered-yet) | Blocker | Resolved (2026-07-26, root-caused and fixed) | — | deep-reasoning | See `documentation/backlog_history/bugs_done_details.md` item #113 for the full fix account. |
| 111 | [#104 labelled an ACCEPTED application captcha-blocked, because the DOM lags the acceptance](#111-104-labelled-an-accepted-application-captcha-blocked-because-the-dom-lags-the-acceptance) | Blocker | Resolved (2026-07-26, root-caused and fixed) | — | deep-reasoning | See `documentation/backlog_history/bugs_done_details.md` item #111 for the full fix account. |
| 110 | [A short option label could hijack a longer answer — "Prefer not to say" selected "No"](#110-a-short-option-label-could-hijack-a-longer-answer--prefer-not-to-say-selected-no) | Blocker | Resolved (2026-07-26, root-caused and fixed) | — | deep-reasoning | See `documentation/backlog_history/bugs_done_details.md` item #110 for the full fix account. |
| 109 | [A single-choice question rendered as a checkbox group was read as one box to untick](#109-a-single-choice-question-rendered-as-a-checkbox-group-was-read-as-one-box-to-untick) | Major | Resolved (2026-07-26, root-caused and fixed) | — | deep-reasoning | See `documentation/backlog_history/bugs_done_details.md` item #109 for the full fix account. |
| 108 | [A submit that went nowhere was reported as "form too large for the local model"](#108-a-submit-that-went-nowhere-was-reported-as-form-too-large-for-the-local-model) | Major | Resolved (2026-07-26, root-caused and fixed) | — | deep-reasoning | See `documentation/backlog_history/bugs_done_details.md` item #108 for the full fix account. |
| 107 | [A checkbox the model deliberately declined was recorded as uncommittable](#107-a-checkbox-the-model-deliberately-declined-was-recorded-as-uncommittable) | Major | Resolved (2026-07-26, root-caused and fixed) | — | deep-reasoning | See `documentation/backlog_history/bugs_done_details.md` item #107 for the full fix account. |
| 106 | [A bare bracketed checkbox-group id got no fallbacks at all — the third shape of #73](#106-a-bare-bracketed-checkbox-group-id-got-no-fallbacks-at-all--the-third-shape-of-73) | Major | Resolved (2026-07-26, root-caused and fixed) | — | deep-reasoning | See `documentation/backlog_history/bugs_done_details.md` item #106 for the full fix account. |
| 105 | [The 45-minute time budget counted bytes to read, not answers to generate](#105-the-45-minute-time-budget-counted-bytes-to-read-not-answers-to-generate) | Major | Resolved (2026-07-26, root-caused and fixed) | — | deep-reasoning | See `documentation/backlog_history/bugs_done_details.md` item #105 for the full fix account. |
| 104 | [A captcha-swallowed submit hid behind stale invalid flags, so #99 never fired](#104-a-captcha-swallowed-submit-hid-behind-stale-invalid-flags-so-99-never-fired) | Major | Resolved (2026-07-26, root-caused and fixed) | — | deep-reasoning | See `documentation/backlog_history/bugs_done_details.md` item #104 for the full fix account. |
| 103 | [#98 showed the model react-select's internal option ids, and it answered with them](#103-98-showed-the-model-react-selects-internal-option-ids-and-it-answered-with-them) | Blocker | Resolved (2026-07-26, root-caused and fixed) | — | deep-reasoning | See `documentation/backlog_history/bugs_done_details.md` item #103 for the full fix account. |
| 102 | [#95's early exit read stale invalid flags and called four accepted submissions failures](#102-95s-early-exit-read-stale-invalid-flags-and-called-four-accepted-submissions-failures) | Blocker | Resolved (2026-07-26, root-caused and fixed) | — | deep-reasoning | See `documentation/backlog_history/bugs_done_details.md` item #102 for the full fix account. |
| 101 | [A submit click that timed out reported nothing about what blocked it](#101-a-submit-click-that-timed-out-reported-nothing-about-what-blocked-it) | Major | Resolved (2026-07-25, diagnostic added) | — | deep-reasoning | See `documentation/backlog_history/bugs_done_details.md` item #101 for the full fix account. |
| 100 | [A field that lands and is rejected anyway had no diagnostic at all](#100-a-field-that-lands-and-is-rejected-anyway-had-no-diagnostic-at-all) | Major | Resolved (2026-07-25, diagnostic added) | — | deep-reasoning | See `documentation/backlog_history/bugs_done_details.md` item #100 for the full fix account. |
| 99 | [A submit silently swallowed by reCAPTCHA was reported as an ordinary validation bounce](#99-a-submit-silently-swallowed-by-recaptcha-was-reported-as-an-ordinary-validation-bounce) | Major | Resolved (2026-07-25, root-caused and fixed) | — | deep-reasoning | See `documentation/backlog_history/bugs_done_details.md` item #99 for the full fix account. |
| 98 | [The model was never shown a dropdown's permitted values, so it guessed the wording](#98-the-model-was-never-shown-a-dropdowns-permitted-values-so-it-guessed-the-wording) | Blocker | Resolved (2026-07-25, root-caused and fixed) | — | deep-reasoning | See `documentation/backlog_history/bugs_done_details.md` item #98 for the full fix account. |
| 97 | [An uncommittable field named the control but never the value that was tried](#97-an-uncommittable-field-named-the-control-but-never-the-value-that-was-tried) | Major | Resolved (2026-07-25, diagnostic added) | — | deep-reasoning | See `documentation/backlog_history/bugs_done_details.md` item #97 for the full fix account. |
| 96 | [Nothing recorded what a submit verdict was actually decided on](#96-nothing-recorded-what-a-submit-verdict-was-actually-decided-on) | Major | Resolved (2026-07-25, diagnostic added) | — | deep-reasoning | See `documentation/backlog_history/bugs_done_details.md` item #96 for the full fix account. |
| 95 | [The submit verdict was read from the DOM the instant the click returned, racing the submission itself](#95-the-submit-verdict-was-read-from-the-dom-the-instant-the-click-returned-racing-the-submission-itself) | Blocker | Resolved (2026-07-25, fix shipped; race inferred, not directly observed) | — | deep-reasoning | See `documentation/backlog_history/bugs_done_details.md` item #95 for the full fix account. |
| 94 | [The dedup row was written at document generation, so a job that never submitted was skipped forever](#94-the-dedup-row-was-written-at-document-generation-so-a-job-that-never-submitted-was-skipped-forever) | Blocker | Resolved (2026-07-25, root-caused and fixed) | — | deep-reasoning | See `documentation/backlog_history/bugs_done_details.md` item #94 for the full fix account. |
| 93 | [Greenhouse's emailed security-code gate read as a validation error, burning the full 45-minute timeout](#93-greenhouses-emailed-security-code-gate-read-as-a-validation-error-burning-the-full-45-minute-timeout) | Major | Resolved (2026-07-25, root-caused and fixed) | — | deep-reasoning | See `documentation/backlog_history/bugs_done_details.md` item #93 for the full fix account. |
| 92 | [Checkbox-group ids contain brackets, which are CSS attribute syntax, so they resolved to nothing](#92-checkbox-group-ids-contain-brackets-which-are-css-attribute-syntax-so-they-resolved-to-nothing) | Major | Resolved (2026-07-25, root-caused and fixed) | — | deep-reasoning | See `documentation/backlog_history/bugs_done_details.md` item #92 for the full fix account. |
| 91 | [#90's single-option rule could never fire, because typing filters the sole option out](#91-90s-single-option-rule-could-never-fire-because-typing-filters-the-sole-option-out) | Major | Resolved (2026-07-25, root-caused and fixed) | — | deep-reasoning | See `documentation/backlog_history/bugs_done_details.md` item #91 for the full fix account. |
| 90 | [A required control with exactly one option was refused, sending a job to manual review one click from completion](#90-a-required-control-with-exactly-one-option-was-refused-sending-a-job-to-manual-review-one-click-from-completion) | Major | Resolved (2026-07-25, root-caused and fixed) | — | deep-reasoning | See `documentation/backlog_history/bugs_done_details.md` item #90 for the full fix account. |
| 89 | [A late-rendering confirmation page is missed, so a successful submit is retried — filing duplicates](#89-a-late-rendering-confirmation-page-is-missed-so-a-successful-submit-is-retried--filing-duplicates) | Blocker | Resolved (2026-07-25, root-caused and fixed) | — | deep-reasoning | See `documentation/backlog_history/bugs_done_details.md` item #89 for the full fix account. |
| 88 | [A required widget that cannot accept the configured value was written off as a submit failure](#88-a-required-widget-that-cannot-accept-the-configured-value-was-written-off-as-a-submit-failure) | Major | Resolved (2026-07-25, root-caused and fixed) | — | deep-reasoning | See `documentation/backlog_history/bugs_done_details.md` item #88 for the full fix account. |
| 87 | [The submit locator clicked the click-to-reveal "Apply" button, so no retry ever actually submitted](#87-the-submit-locator-clicked-the-click-to-reveal-apply-button-so-no-retry-ever-actually-submitted) | Blocker | Resolved (2026-07-25, root-caused and fixed) | — | deep-reasoning | See `documentation/backlog_history/bugs_done_details.md` item #87 for the full fix account. |
| 86 | [Lever's location typeahead was invisible to combobox detection, so every Lever application failed](#86-levers-location-typeahead-was-invisible-to-combobox-detection-so-every-lever-application-failed) | Blocker | Resolved (2026-07-25, root-caused, fixed, verified against the live form) | — | deep-reasoning | See `documentation/backlog_history/bugs_done_details.md` item #86 for the full fix account. |
| 85 | [Four early-exit paths left rows stranded in PROCESSING, invisible to every future queue](#85-four-early-exit-paths-left-rows-stranded-in-processing-invisible-to-every-future-queue) | Major | Resolved (2026-07-25, root-caused and fixed) | — | deep-reasoning | See `documentation/backlog_history/bugs_done_details.md` item #85 for the full fix account. |
| 84 | [#82's manual-routing branch was never applied, so refused jobs were written off as FAILED_SUBMIT](#84-82s-manual-routing-branch-was-never-applied-so-refused-jobs-were-written-off-as-failed_submit) | Major | Resolved (2026-07-25, confirmed live 18:10 — clean A/B on the same job) | — | deep-reasoning | See `documentation/backlog_history/bugs_done_details.md` item #84 for the full fix account. |
| 83 | [The payload breaker guarded the context window but not the time budget, burning the full 45-minute timeout](#83-the-payload-breaker-guarded-the-context-window-but-not-the-time-budget-burning-the-full-45-minute-timeout) | Major | Resolved (2026-07-25, root-caused and fixed) | — | deep-reasoning | See `documentation/backlog_history/bugs_done_details.md` item #83 for the full fix account. |
| 82 | [Once the commit worked, an unanswerable legal attestation would have been guessed and really submitted](#82-once-the-commit-worked-an-unanswerable-legal-attestation-would-have-been-guessed-and-really-submitted) | Blocker | Resolved (2026-07-25, confirmed live 17:52 — refused in 0 seconds) | — | deep-reasoning | See `documentation/backlog_history/bugs_done_details.md` item #82 for the full fix account. |
| 81 | [data-value mirrors the typed search text, so every react-select falsely reported "landed"](#81-data-value-mirrors-the-typed-search-text-so-every-react-select-falsely-reported-landed) | Blocker | Resolved (2026-07-25, root-caused and fixed) | — | deep-reasoning | See `documentation/backlog_history/bugs_done_details.md` item #81 for the full fix account. |
| 80 | [The retry loop logged the payload size but never which fields were still invalid](#80-the-retry-loop-logged-the-payload-size-but-never-which-fields-were-still-invalid) | Major | Resolved (2026-07-25, root-caused and fixed) | — | deep-reasoning | See `documentation/backlog_history/bugs_done_details.md` item #80 for the full fix account. |
| 79 | [The option wait watched an unrelated widget, and committing option-0 filed the wrong location](#79-the-option-wait-watched-an-unrelated-widget-and-committing-option-0-filed-the-wrong-location) | Blocker | Resolved (2026-07-25, confirmed live in the agent 16:02) | — | deep-reasoning | See `documentation/backlog_history/bugs_done_details.md` item #79 for the full fix account. |
| 78 | [Fill() never opens a react-select menu, and the read-back matched the input itself](#78-fill-never-opens-a-react-select-menu-and-the-read-back-matched-the-input-itself) | Blocker | Resolved (2026-07-25, confirmed live in the agent 16:02) | — | deep-reasoning | See `documentation/backlog_history/bugs_done_details.md` item #78 for the full fix account. |
| 77 | [Enter was pressed before react-select had loaded any option, so the commit selected nothing](#77-enter-was-pressed-before-react-select-had-loaded-any-option-so-the-commit-selected-nothing) | Major | Resolved (2026-07-25, confirmed live in the agent 16:02) | — | deep-reasoning | See `documentation/backlog_history/bugs_done_details.md` item #77 for the full fix account. |
| 76 | [#74's own read-back checked el.value first, silently disabling the combobox commit it had just added](#76-74s-own-read-back-checked-elvalue-first-silently-disabling-the-combobox-commit-it-had-just-added) | Blocker | Resolved (2026-07-25, confirmed live in the agent 16:02) | — | deep-reasoning | See `documentation/backlog_history/bugs_done_details.md` item #76 for the full fix account. |
| 75 | [#74's combobox commit was wired into the retry path but not the initial fill, guaranteeing a wasted retry cycle](#75-74s-combobox-commit-was-wired-into-the-retry-path-but-not-the-initial-fill-guaranteeing-a-wasted-retry-cycle) | Major | Resolved (2026-07-25, confirmed live in the agent 16:02) | — | deep-reasoning | See `documentation/backlog_history/bugs_done_details.md` item #75 for the full fix account. |
| 74 | [react-select comboboxes were filled but never committed, so their validated value stayed empty](#74-react-select-comboboxes-were-filled-but-never-committed-so-their-validated-value-stayed-empty) | Major | Resolved (2026-07-25, confirmed live in the agent 16:02) | — | deep-reasoning | See `documentation/backlog_history/bugs_done_details.md` item #74 for the full fix account. |
| 73 | [A CSS id selector cannot start with a digit, so Greenhouse's numeric custom-question ids were unfillable half the time](#73-a-css-id-selector-cannot-start-with-a-digit-so-greenhouses-numeric-custom-question-ids-were-unfillable-half-the-time) | Major | Resolved (2026-07-25, root-caused and fixed) | — | deep-reasoning | See `documentation/backlog_history/bugs_done_details.md` item #73 for the full fix account. |
| 72 | [The retry loop counts empty-valued and non-landing fixes as applied, reporting progress it is not making](#72-the-retry-loop-counts-empty-valued-and-non-landing-fixes-as-applied-reporting-progress-it-is-not-making) | Major | Resolved (2026-07-25, accounting fixed; root cause of the underlying non-convergence still under live investigation) | — | deep-reasoning | See `documentation/backlog_history/bugs_done_details.md` item #72 for the full fix account. |
| 71 | [firstVisibleLocator's .First() fallback reintroduces the very hang it was written to prevent, at the submit click](#71-firstvisiblelocators-first-fallback-reintroduces-the-very-hang-it-was-written-to-prevent-at-the-submit-click) | Major | Resolved (2026-07-25, root-caused and fixed) | — | deep-reasoning | See `documentation/backlog_history/bugs_done_details.md` item #71 for the full fix account. |
| 70 | [The validation-retry loop strips the page's own error text, so the model never learns why a field bounced](#70-the-validation-retry-loop-strips-the-pages-own-error-text-so-the-model-never-learns-why-a-field-bounced) | Blocker | Resolved (2026-07-25, root-caused and fixed) | — | deep-reasoning | See `documentation/backlog_history/bugs_done_details.md` item #70 for the full fix account. |
| 69 | [Discovery stored the searched role as job_title and discarded the real headline](#69-discovery-stored-the-searched-role-as-job_title-and-discarded-the-real-headline) | Major | Resolved (2026-07-25, root-caused and fixed) | — | deep-reasoning | See `documentation/backlog_history/bugs_done_details.md` item #69 for the full fix account. |
| 68 | [SaveFormMapping cached semantically-empty mappings, burning a Learner Module call per visit](#68-saveformmapping-cached-semantically-empty-mappings-burning-a-learner-module-call-per-visit) | Major | Resolved (2026-07-25, root-caused and fixed) | — | deep-reasoning | See `documentation/backlog_history/bugs_done_details.md` item #68 for the full fix account. |
| 67 | [The initial fill path never received #65/#66's fixes, so required dropdowns always failed the first pass](#67-the-initial-fill-path-never-received-6566s-fixes-so-required-dropdowns-always-failed-the-first-pass) | Major | Resolved (2026-07-25, root-caused and fixed) | — | deep-reasoning | See `documentation/backlog_history/bugs_done_details.md` item #67 for the full fix account. |
| 66 | [SolveValidationErrors returns bare id/name values, not CSS selectors, so every proposed fix matched nothing](#66-solvevalidationerrors-returns-bare-idname-values-not-css-selectors-so-every-proposed-fix-matched-nothing) | Blocker | Resolved (2026-07-25, root-caused and fixed) | — | deep-reasoning | See `documentation/backlog_history/bugs_done_details.md` item #66 for the full fix account. |
| 65 | [Validation fixes were applied with Fill() only and their errors discarded, so required dropdowns could never be satisfied](#65-validation-fixes-were-applied-with-fill-only-and-their-errors-discarded-so-required-dropdowns-could-never-be-satisfied) | Blocker | Resolved (2026-07-25, root-caused and fixed) | — | deep-reasoning | See `documentation/backlog_history/bugs_done_details.md` item #65 for the full fix account. |
| 64 | [SolveValidationErrors re-sends the entire form instead of just the fields that failed, timing out on large forms](#64-solvevalidationerrors-re-sends-the-entire-form-instead-of-just-the-fields-that-failed-timing-out-on-large-forms) | Major | Resolved (2026-07-25, root-caused and fixed) | — | deep-reasoning | See `documentation/backlog_history/bugs_done_details.md` item #64 for the full fix account. |
| 63 | [Every fit score was computed and thrown away — the only writer of fit_score had zero callers](#63-every-fit-score-was-computed-and-thrown-away--the-only-writer-of-fit_score-had-zero-callers) | Major | Resolved (2026-07-25, root-caused and fixed) | — | deep-reasoning | See `documentation/backlog_history/bugs_done_details.md` item #63 for the full fix account. |
| 61 | [The cover letter was never sent to any employer — no handler ever filled it](#61-the-cover-letter-was-never-sent-to-any-employer--no-handler-ever-filled-it) | Major | Resolved (2026-07-24, root-caused and fixed) | — | deep-reasoning | See `documentation/backlog_history/bugs_done_details.md` item #61 for the full fix account. |
| 62 | [The saved cover letter was deleted from the application folder, stripping the manual-apply queue](#62-the-saved-cover-letter-was-deleted-from-the-application-folder-stripping-the-manual-apply-queue) | Major | Resolved (2026-07-24, root-caused and fixed) | — | deep-reasoning | See `documentation/backlog_history/bugs_done_details.md` item #62 for the full fix account. |
| 60 | [Ollama server pinned to an unnecessarily conservative 6,144-token context window — the dominant cause of MANUAL_REQUIRED outcomes](#60-ollama-server-pinned-to-an-unnecessarily-conservative-6144-token-context-window--the-dominant-cause-of-manual_required-outcomes) | Major | Resolved (2026-07-24, root-caused and fixed) | — | standard | See `documentation/backlog_history/bugs_done_details.md` item #60 for the full fix account. |
| 59 | [Generic submit-button selector could click a hidden anti-spam-widget button instead of the real submit control](#59-generic-submit-button-selector-could-click-a-hidden-anti-spam-widget-button-instead-of-the-real-submit-control) | Major | Resolved (2026-07-24, root-caused and fixed) | — | standard | See `documentation/backlog_history/bugs_done_details.md` item #59 for the full fix account. |
| 58 | [Stale career_chunks embedding dimension silently zeroed out all live RAG resume-context retrieval](#58-stale-career_chunks-embedding-dimension-silently-zeroed-out-all-live-rag-resume-context-retrieval) | Major | Resolved (2026-07-24, root-caused and fixed) | — | standard | See `documentation/backlog_history/bugs_done_details.md` item #58 for the full fix account. |
| 57 | [Forms too large for Ollama's context window burned a full doc-gen cycle before failing with an ugly HTTP 400](#57-forms-too-large-for-ollamas-context-window-burned-a-full-doc-gen-cycle-before-failing-with-an-ugly-http-400) | Major | Resolved (2026-07-24, root-caused and fixed) | — | standard | See `documentation/backlog_history/bugs_done_details.md` item #57 for the full fix account. |
| 56 | [Dashboard has no tile for BLOCKED_CAPTCHA or INVALID_URL, silently omitting 9% of all job_funnel rows](#56-dashboard-has-no-tile-for-blocked_captcha-or-invalid_url-silently-omitting-9-of-all-job_funnel-rows) | Minor | Resolved (2026-07-24, root-caused and fixed) | — | standard | See `documentation/backlog_history/bugs_done_details.md` item #56 for the full fix account. |
| 55 | [Jobs killed mid-flight get permanently stuck in PROCESSING, never retried, inflating the dashboard's live count](#55-jobs-killed-mid-flight-get-permanently-stuck-in-processing-never-retried-inflating-the-dashboards-live-count) | Major | Resolved (2026-07-24, root-caused and fixed) | — | standard | See `documentation/backlog_history/bugs_done_details.md` item #55 for the full fix account. |
| 54 | [Raw-HTML captcha pre-check misclassifies Ashby's client-rendered SPA shell as a block](#54-raw-html-captcha-pre-check-misclassifies-ashbys-client-rendered-spa-shell-as-a-block) | Major | Resolved (2026-07-24, root-caused and fixed) | — | standard | See `documentation/backlog_history/bugs_done_details.md` item #54 for the full fix account. |
| 53 | [isSubmissionConfirmed only ever ran for Lever/Greenhouse/LinkedIn — every other ATS platform's APPLIED had zero confirmation evidence](#53-issubmissionconfirmed-only-ever-ran-for-levergreenhouselinkedin--every-other-ats-platforms-applied-had-zero-confirmation-evidence) | Blocker | Resolved (2026-07-24, root-caused and fixed) | — | standard | See `documentation/backlog_history/bugs_done_details.md` item #53 for the full fix account. |
| 52 | [SolveValidationErrors sends the whole page's DOM, tripping the LLM-cost circuit breaker and losing otherwise-successful applications](#52-solvevalidationerrors-sends-the-whole-pages-dom-tripping-the-llm-cost-circuit-breaker-and-losing-otherwise-successful-applications) | Major | Resolved (2026-07-23, root-caused and fixed) | — | standard | See `documentation/backlog_history/bugs_done_details.md` item #52 for the full fix account. |
| 51 | [Post-submit success check trusted any URL change, not proof of an actual successful submission](#51-post-submit-success-check-trusted-any-url-change-not-proof-of-an-actual-successful-submission) | Major | Resolved (2026-07-23, root-caused and fixed) | — | standard | See `documentation/backlog_history/bugs_done_details.md` item #51 for the full fix account. |
| 50 | [Workable requires account sign-in on every posting — same structural class as Workday](#50-workable-requires-account-sign-in-on-every-posting-same-structural-class-as-workday) | Major | Resolved (2026-07-23, root-caused and fixed) | — | standard | See `documentation/backlog_history/bugs_done_details.md` item #50 for the full fix account. |
| 49 | [handleGreenhouse's hardcoded submit selector doesn't exist on modern-board postings](#49-handlegreenhouses-hardcoded-submit-selector-doesnt-exist-on-modern-board-postings) | Major | Resolved (2026-07-23, root-caused and fixed) | — | standard | See `documentation/backlog_history/bugs_done_details.md` item #49 for the full fix account. |
| 48 | [Lever click-to-reveal (bug #47's fix) doesn't fire on a second real posting — possible page staleness after the long doc-gen wait](#48-lever-click-to-reveal-bug-47s-fix-doesnt-fire-on-a-second-real-posting-possible-page-staleness-after-the-long-doc-gen-wait) | Minor | Resolved (2026-07-24, not reproduced despite 11+ subsequent live opportunities under the same precondition — see groom note) | — | standard | See `documentation/backlog_history/bugs_done_details.md` item #48 for the full fix account. |
| 47 | [Dedicated Greenhouse/Lever handlers never click "Apply" to reveal the form, only the generic Learner Module path does](#47-dedicated-greenhouselever-handlers-never-click-apply-to-reveal-the-form-only-the-generic-learner-module-path-does) | Major | Resolved (2026-07-23, root-caused and fixed) | — | standard | See `documentation/backlog_history/bugs_done_details.md` item #47 for the full fix account. |
| 46 | [Raw-HTML job-description fetch also misdetects reCAPTCHA/Turnstile widgets as a block, before fit-scoring even runs](#46-raw-html-job-description-fetch-also-misdetects-recaptchaturnstile-widgets-as-a-block-before-fit-scoring-even-runs) | Blocker | Resolved (2026-07-23, root-caused and fixed) | — | standard | See `documentation/backlog_history/bugs_done_details.md` item #46 for the full fix account. |
| 45 | [isCaptchaBlocked misdetects standard reCAPTCHA/hCaptcha anti-spam widgets on real forms as a full block](#45-iscaptchablocked-misdetects-standard-recaptchahcaptcha-anti-spam-widgets-on-real-forms-as-a-full-block) | Blocker | Resolved (2026-07-23, root-caused and fixed) | — | standard | See `documentation/backlog_history/bugs_done_details.md` item #45 for the full fix account. |
| 4 | [AttemptSubmit form-fill logic never looked inside iframes](#4-attemptsubmit-form-fill-logic-never-looked-inside-iframes) | Major | Resolved (2026-07-23, verified via targeted unit test in lieu of unreachable live confirmation) | — | standard | See `documentation/backlog_history/bugs_done_details.md` item #4 for the full fix account. |
| 6 | [Ollama generation throughput collapses mid-request, likely context-shift thrashing](#6-ollama-generation-throughput-collapses-mid-request-likely-context-shift-thrashing) | Blocker | Resolved (2026-07-21) | — | standard | See `documentation/backlog_history/bugs_done_details.md` item #6 for the full fix account. |
| 7 | [FunnelEngine still lets Greenhouse job-search/listing pages into the pipeline](#7-funnelengine-still-lets-greenhouse-job-searchlisting-pages-into-the-pipeline) | Minor | Resolved (2026-07-21, via #15) | — | standard | See `documentation/backlog_history/bugs_done_details.md` item #7 for the full fix account. |
| 9 | [Dead-job-posting detection missed common phrasings, wasting cycles on expired listings](#9-dead-job-posting-detection-missed-common-phrasings-wasting-cycles-on-expired-listings) | Minor | Resolved (2026-07-21) | — | standard | See `documentation/backlog_history/bugs_done_details.md` item #9 for the full fix account. |
| 10 | [DOM-mapped fill failures never fell back to the Vision module, only outright mapping failures did](#10-dom-mapped-fill-failures-never-fell-back-to-the-vision-module-only-outright-mapping-failures-did) | Major | Resolved (2026-07-24, closed via direct end-to-end test, live confirmation structurally unreachable so far) | — | standard | See `documentation/backlog_history/bugs_done_details.md` item #10 for the full fix account. |
| 11 | [FunnelEngine lets Jobvite `/search` listing pages into the pipeline](#11-funnelengine-lets-jobvite-search-listing-pages-into-the-pipeline) | Minor | Resolved (2026-07-21) | — | standard | See `documentation/backlog_history/bugs_done_details.md` item #11 for the full fix account. |
| 12 | [Same job URL reprocessed repeatedly, hitting a UNIQUE constraint on applied_jobs.url](#12-same-job-url-reprocessed-repeatedly-hitting-a-unique-constraint-on-applied_jobsurl) | Major | Resolved (2026-07-21) | — | standard | See `documentation/backlog_history/bugs_done_details.md` item #12 for the full fix account. |
| 13 | [Ollama gets kernel OOM-killed under this machine's real RAM ceiling](#13-ollama-gets-kernel-oom-killed-under-this-machines-real-ram-ceiling) | Major | Resolved (2026-07-21, mitigated) | — | standard | See `documentation/backlog_history/bugs_done_details.md` item #13 for the full fix account. |
| 14 | [No accessible-label fallback for form-field filling, only CSS selector guessing](#14-no-accessible-label-fallback-for-form-field-filling-only-css-selector-guessing) | Major | Resolved (2026-07-24, closed via direct end-to-end test, live confirmation structurally unreachable so far) | — | standard | See `documentation/backlog_history/bugs_done_details.md` item #14 for the full fix account. |
| 15 | [Dedicated Greenhouse/Lever handlers timing out waiting for the form to render](#15-dedicated-greenhouselever-handlers-timing-out-waiting-for-the-form-to-render) | Major | Resolved (2026-07-22, verified live) | — | standard | See `documentation/backlog_history/bugs_done_details.md` item #15 for the full fix account. |
| 16 | [#14's label fallback needs a GetByPlaceholder tier too](#16-14s-label-fallback-needs-a-getbyplaceholder-tier-too) | Minor | Resolved (2026-07-21) | — | standard | See `documentation/backlog_history/bugs_done_details.md` item #16 for the full fix account. |
| 17 | [ORDER BY last_updated DESC picked a stale row over a genuinely newer one](#17-order-by-last_updated-desc-picked-a-stale-row-over-a-genuinely-newer-one) | Major | Resolved (2026-07-21) | — | standard | See `documentation/backlog_history/bugs_done_details.md` item #17 for the full fix account. |
| 8 | [Dynamic/Learner Module fill path never clicks an "Apply" button to reveal click-to-reveal application forms](#8-dynamiclearner-module-fill-path-never-clicks-an-apply-button-to-reveal-click-to-reveal-application-forms) | Major | Resolved (2026-07-24, closed via direct end-to-end test, live confirmation structurally unreachable so far) | — | standard | See `documentation/backlog_history/bugs_done_details.md` item #8 for the full fix account. |
| 18 | [Workday postings burn full Learner+Vision cycles against an auth-gated application flow with no fillable form](#18-workday-postings-burn-full-learnervision-cycles-against-an-auth-gated-application-flow-with-no-fillable-form) | Major | Resolved (2026-07-21, verified live) | — | standard | See `documentation/backlog_history/bugs_done_details.md` item #18 for the full fix account. |
| 20 | [Email tracker classifies unrelated emails as INTERVIEW_REQUESTED and writes them to the DB](#20-email-tracker-classifies-unrelated-emails-as-interview_requested-and-writes-them-to-the-db) | Major | Resolved (2026-07-22, verified live) | — | standard | See `documentation/backlog_history/bugs_done_details.md` item #20 for the full fix account. |
| 25 | [Fit scoring ignores geographic eligibility restrictions](#25-fit-scoring-ignores-geographic-eligibility-restrictions) | Minor | Resolved (2026-07-22, verified locally) | — | standard | See `documentation/backlog_history/bugs_done_details.md` item #25 for the full fix account. |
| 24 | [Prompt-injection quarantine may false-positive on ordinary job-page copy ("you are a...")](#24-prompt-injection-quarantine-may-false-positive-on-ordinary-job-page-copy-you-are-a) | Minor | Resolved (2026-07-22) | — | standard | See `documentation/backlog_history/bugs_done_details.md` item #24 for the full fix account. |
| 23 | [Bot-protection interstitials (DataDome) aren't detected, burning full cycles and feeding the Learner captcha pages](#23-bot-protection-interstitials-datadome-arent-detected-burning-full-cycles-and-feeding-the-learner-captcha-pages) | Major | Resolved (2026-07-22) | — | standard | See `documentation/backlog_history/bugs_done_details.md` item #23 for the full fix account. |
| 21 | [SaveFormMapping caches non-JSON LLM output, poisoning every future visit to the domain](#21-saveformmapping-caches-non-json-llm-output-poisoning-every-future-visit-to-the-domain) | Minor | Resolved (2026-07-22) | — | standard | See `documentation/backlog_history/bugs_done_details.md` item #21 for the full fix account. |
| 22 | [Stale pre-filter backlog rows and error-redirect URLs bypass every discovery filter](#22-stale-pre-filter-backlog-rows-and-error-redirect-urls-bypass-every-discovery-filter) | Minor | Resolved (2026-07-22, mitigated) | — | standard | See `documentation/backlog_history/bugs_done_details.md` item #22 for the full fix account. |
| 34 | [A cookie-consent banner's backdrop silently intercepts every click, defeating clickApplyIfPresent](#34-a-cookie-consent-banners-backdrop-silently-intercepts-every-click-defeating-clickapplyifpresent) | Major | Resolved (2026-07-22, verified live) | — | standard | See `documentation/backlog_history/bugs_done_details.md` item #34 for the full fix account. |
| 35 | [SmartRecruiters' "I'm interested" button and post-click CAPTCHA reveals both went undetected](#35-smartrecruiters-im-interested-button-and-post-click-captcha-reveals-both-went-undetected) | Major | Resolved (2026-07-22, verified live) | — | standard | See `documentation/backlog_history/bugs_done_details.md` item #35 for the full fix account. |
| 36 | [Jobvite's "Data Consent" step means the application form doesn't exist until a location/language <select> is chosen](#36-jobvites-data-consent-step-means-the-application-form-doesnt-exist-until-a-locationlanguage-select-is-chosen) | Major | Resolved (2026-07-22, verified live) | — | standard | See `documentation/backlog_history/bugs_done_details.md` item #36 for the full fix account. |
| 37 | [fillActionTimeoutMs (15000ms) too tight for genuine CPU contention from the co-located Ollama model](#37-fillactiontimeoutms-15000ms-too-tight-for-genuine-cpu-contention-from-the-co-located-ollama-model) | Major | Resolved (2026-07-22, verified live) | — | standard | See `documentation/backlog_history/bugs_done_details.md` item #37 for the full fix account. |
| 38 | [FunnelEngine kept sending Learner+doc-gen cycles at a 0%-success source and let Workday monopolize the worker queue](#38-funnelengine-kept-sending-learnerdoc-gen-cycles-at-a-0-success-source-and-let-workday-monopolize-the-worker-queue) | Major | Resolved (2026-07-22, mitigated) | — | standard | See `documentation/backlog_history/bugs_done_details.md` item #38 for the full fix account. |
| 39 | [Vision-fallback fill fails with "empty selector provided for form filling"](#39-vision-fallback-fill-fails-with-empty-selector-provided-for-form-filling) | Minor | Resolved (2026-07-23, root-caused and fixed) | — | standard | See `documentation/backlog_history/bugs_done_details.md` item #39 for the full fix account. |
| 40 | [~200+ files/dirs under applications/ are still owned by a stale UID from an earlier containerized run](#40-200-filesdirs-under-applications-are-still-owned-by-a-stale-uid-from-an-earlier-containerized-run) | Minor | Resolved (2026-07-22, full sweep) | — | standard | See `documentation/backlog_history/bugs_done_details.md` item #40 for the full fix account. |
| 41 | [applytojob.com and recruitee.com board-index/landing pages scored and processed as real postings](#41-applytojobcom-and-recruiteecom-board-indexlanding-pages-scored-and-processed-as-real-postings) | Major | Resolved (2026-07-22, verified live) | — | standard | See `documentation/backlog_history/bugs_done_details.md` item #41 for the full fix account. |
| 42 | [www.bamboohr.com and app.bamboohr.com pages (marketing site, shared login portal) scored as postings](#42-wwwbamboohrcom-and-appbamboohrcom-pages-marketing-site-shared-login-portal-scored-as-postings) | Minor | Resolved (2026-07-22, verified live) | — | standard | See `documentation/backlog_history/bugs_done_details.md` item #42 for the full fix account. |
| 43 | [getByLabel/getByPlaceholder threw a Playwright strict-mode violation when a label matched more than one element](#43-getbylabelgetbyplaceholder-threw-a-playwright-strict-mode-violation-when-a-label-matched-more-than-one-element) | Major | Resolved (2026-07-22, verified live) | — | standard | See `documentation/backlog_history/bugs_done_details.md` item #43 for the full fix account. |
| 44 | [BambooHR corporate subdomains kept slipping past a growing denylist](#44-bamboohr-corporate-subdomains-kept-slipping-past-a-growing-denylist-resolved-2026-07-23) | Minor | Resolved (2026-07-23) | — | standard | See `documentation/backlog_history/bugs_done_details.md` item #44 for the full fix account. |
| 19 | [Workday URL parsing takes the locale/site segment as the company name](#19-workday-url-parsing-takes-the-localesite-segment-as-the-company-name) | Minor | Resolved (2026-07-21) | — | standard | See `documentation/backlog_history/bugs_done_details.md` item #19 for the full fix account. |

## Details

### 512. Assisted Apply presents stale, mismatched, and blank employer pages as actionable human handoffs

Done — full account archived in `documentation/backlog_history/bugs_done_details.md` item #512.

### 489. `promptsec.Moderate()` still quarantines roughly half of everything discovered, disproportionately on Lever and Greenhouse

See `documentation/backlog_history/bugs_done_details.md` item #489 for the full fix account.

### 501. Re-run #489's aggregate quarantine-rate queries against fresh data once the fix has been live for a batch cycle

Done — full account archived in `documentation/backlog_history/bugs_done_details.md` item #501.

### 508. Discovery has no independent current-listings fallback when SerpApi quota is exhausted and Yahoo search fails

Done — full account archived in `documentation/backlog_history/bugs_done_details.md` item #508.

### 507. The first post-#489 cohort reaches FAILED_SUBMIT 100% of the time, so its lower quarantine rate has not yet improved outcomes

Done — full account archived in `documentation/backlog_history/bugs_done_details.md` item #507.

### 502. `encoding.go`'s homoglyph branch is a third zero-evidence heuristic threat source, outside #489's fix window

Done — full account archived in `documentation/backlog_history/bugs_done_details.md` item #502.

### 503. `TwoStepVerification`'s quarantine check never logs to the prompt-injection audit trail, undercounting the CSV total

Done — full account archived in `documentation/backlog_history/bugs_done_details.md` item #503.

### 504. `state.TailoredContext` (our own RAG-generated content) can trip the same zero-evidence quarantine as #489, via a dedicated but unverified `QUARANTINED_RAG_CONTEXT` status

Done — full account archived in `documentation/backlog_history/bugs_done_details.md` item #504.

### 490. `job_funnel.applied_at` is declared in the schema but no code path ever writes it

Done — full account archived in `documentation/backlog_history/bugs_done_details.md` item #490.

### 482. breezy.hr postings are excluded from GetDiscoveredJobs entirely, so they accumulate in DISCOVERED forever with no terminal status

Done — full account archived in `documentation/backlog_history/bugs_done_details.md` item #482.

### 481. Aged DISCOVERED postings expire before the ranking algorithm's freshness decay ever surfaces them, starving the funnel

Closed — full account archived in `documentation/backlog_history/bugs_done_details.md`.

### 480. UpdateFunnelStatusRetryable never records a status_reason, so every RETRY_EXHAUSTED row loses its own root cause

Done — full account archived in `documentation/backlog_history/bugs_done_details.md` item #480.

### 478. A DNS resolution failure never moves a job out of DISCOVERED, so one bad hostname spins the daemon forever

Closed — full account archived in `documentation/backlog_history/bugs_done_details.md`.

### 476. `GetQueuePlan` has no `rows.Err()` check, so a cursor error silently truncates the requeue dry-run preview

Closed — full account archived in `documentation/backlog_history/bugs_done_details.md`.

### 475. Yahoo fallback still fails most discovery queries despite bug 130's retry and backoff fix

Closed — full account archived in `documentation/backlog_history/bugs_done_details.md`.

### 466. Retryable queue rows are reset to DISCOVERED and immediately selected again

Closed — full account archived in `documentation/backlog_history/bugs_done_details.md`.

### 467. Playwright target closure aborts an application attempt without bounded browser recovery

Closed — full account archived in `documentation/backlog_history/bugs_done_details.md`.

### 461. The runtime binary `career_agent_bin` is tracked in git and grows the repo on every rebuild

Closed — full account archived in `documentation/backlog_history/bugs_done_details.md`.

### 449. `pgrep -f career_agent_bin` matches any process whose command line merely contains that string

Closed — full account archived in `documentation/backlog_history/bugs_done_details.md`.

### 445. Any web page open in your browser can start or stop the agent

Closed — full account archived in `documentation/backlog_history/bugs_done_details.md`.

### 446. The dashboard's own database connection still uses the pragma syntax bug #416 was closed for fixing

Closed — full account archived in `documentation/backlog_history/bugs_done_details.md`.

### 451. Two summary tiles caption a two-status count with the reason for only one of them

Closed — full account archived in `documentation/backlog_history/bugs_done_details.md`.

### 452. `serveMetrics` swallows all nine of its query errors and answers 200 with zeros

Closed — full account archived in `documentation/backlog_history/bugs_done_details.md`.

### 453. `GetQueuePlan` scans `discovered_at` into a non-nullable `time.Time` while its sibling uses `sql.NullTime`

Closed — full account archived in `documentation/backlog_history/bugs_done_details.md`.

### 447. The dashboard UI silently swallows failed start/stop clicks, and slow polls can render stale metrics

Closed — full account archived in `documentation/backlog_history/bugs_done_details.md`.

### 434. No path moves a job out of `AWAITING_REVIEW` or `MANUAL_REQUIRED`, so hand-off statuses are permanent dead ends

Closed — full account archived in `documentation/backlog_history/bugs_done_details.md`.

### 439. The NLP microservice call ignores your LLM configuration and hardcodes a model this host does not have

Closed — full account archived in `documentation/backlog_history/bugs_done_details.md`.

### 444. A single 429 shuts the whole agent down, and the log blames Gemini whatever provider is configured

Closed — full account archived in `documentation/backlog_history/bugs_done_details.md`.

### 441. A clean setup ends up configured for models the installer never pulled

Closed — full account archived in `documentation/backlog_history/bugs_done_details.md`.

### 440. `scripts/server.go` is the only script without `//go:build ignore`, so `go build ./...` compiles a dead generated file

Closed — full account archived in `documentation/backlog_history/bugs_done_details.md`.

### 437. The React rewrite deleted the dashboard's conversion analytics and accessibility semantics without replacing them

Closed — full account archived in `documentation/backlog_history/bugs_done_details.md`.

### 438. The React rewrite also deleted four tests, including both guards on the dashboard's loopback-only listener

Closed — full account archived in `documentation/backlog_history/bugs_done_details.md`.

### 436. A fresh clone cannot build: the `go:embed`-ed UI bundle is gitignored

Closed — full account archived in `documentation/backlog_history/bugs_done_details.md`.

### 435. `statusReason` is dead code for every status the dashboard actually needs it for

Closed — full account archived in `documentation/backlog_history/bugs_done_details.md`.

### 433. `mergeStatuses` ranks four real statuses at 0, so scheme dedup can revive terminal jobs

Closed — full account archived in `documentation/backlog_history/bugs_done_details.md`.

### 432. `auto_submit_click: false` records a false `APPLIED` for a form that was never submitted

Closed — full account archived in `documentation/backlog_history/bugs_done_details.md`.

### 118. Resume-selector fallback work breaks every submitter path without a readable resume (Resolved 2026-07-26)

Closed — full account archived in `documentation/backlog_history/bugs_done_details.md`.

### 119. Free discovery sources are disabled when the SerpApi key is absent (Resolved 2026-07-26)

Closed — full account archived in `documentation/backlog_history/bugs_done_details.md`.

### 120. `--daemon` logs a six-hour drip mode but exits after one batch

Closed — full account archived in `documentation/backlog_history/bugs_done_details.md`.

### 121. Untrusted job text reaches embedding and scoring models before quarantine

Closed — full account archived in `documentation/backlog_history/bugs_done_details.md`.

### 122. SSRF defenses block literal private IPs but not hostnames that resolve to them

Closed — full account archived in `documentation/backlog_history/bugs_done_details.md`.

### 123. Failed and non-2xx job-page fetches still proceed to expensive fit scoring

Closed — full account archived in `documentation/backlog_history/bugs_done_details.md`.

### 124. The email tracker acknowledges a message even when its database update fails

Closed — full account archived in `documentation/backlog_history/bugs_done_details.md`.

### 125. Ambiguous outcome emails retry forever instead of entering manual review

Closed — full account archived in `documentation/backlog_history/bugs_done_details.md`.

### 126. The unauthenticated dashboard binds every network interface while announcing localhost

Closed — full account archived in `documentation/backlog_history/bugs_done_details.md`.

### 127. Sensitive credentials application data and generated documents are world-readable

Closed — full account archived in `documentation/backlog_history/bugs_done_details.md`.

### 128. Saving a second role at the same company overwrites the first role's documents

Closed — full account archived in `documentation/backlog_history/bugs_done_details.md`.

### 129. The agent hard-codes one developer-specific career-profile path

Closed — full account archived in `documentation/backlog_history/bugs_done_details.md`.

### 117. A single mailbox fetch misses a code that IMAP has not indexed yet (Resolved 2026-07-26)

Closed — full account archived in `documentation/backlog_history/bugs_done_details.md`.

### 116. The post-security-code resubmit still judged the page in one instantaneous read (Resolved 2026-07-26)

Closed — full account archived in `documentation/backlog_history/bugs_done_details.md`.

### 115. Greenhouse splits the one-time code across eight single-character inputs (Resolved 2026-07-26)

Closed — full account archived in `documentation/backlog_history/bugs_done_details.md`.

### 114. When the emailed code cannot be entered, nothing records what IS on the page (Resolved 2026-07-26)

Closed — full account archived in `documentation/backlog_history/bugs_done_details.md`.

### 113. The emailed code was retrieved and then discarded, because the code field had not rendered yet (Resolved 2026-07-26)

Closed — full account archived in `documentation/backlog_history/bugs_done_details.md`.

### 112. The same posting exists twice, once per URL scheme, and their statuses have diverged (Resolved 2026-07-28)

Closed — full account archived in `documentation/backlog_history/bugs_done_details.md`.

### 111. #104 labelled an ACCEPTED application captcha-blocked, because the DOM lags the acceptance (Resolved 2026-07-26)

Closed — full account archived in `documentation/backlog_history/bugs_done_details.md`.

### 110. A short option label could hijack a longer answer — "Prefer not to say" selected "No" (Resolved 2026-07-26)

Closed — full account archived in `documentation/backlog_history/bugs_done_details.md`.

### 109. A single-choice question rendered as a checkbox group was read as one box to untick (Resolved 2026-07-26)

Closed — full account archived in `documentation/backlog_history/bugs_done_details.md`.

### 108. A submit that went nowhere was reported as "form too large for the local model" (Resolved 2026-07-26)

Closed — full account archived in `documentation/backlog_history/bugs_done_details.md`.

### 107. A checkbox the model deliberately declined was recorded as uncommittable (Resolved 2026-07-26)

Closed — full account archived in `documentation/backlog_history/bugs_done_details.md`.

### 106. A bare bracketed checkbox-group id got no fallbacks at all — the third shape of #73 (Resolved 2026-07-26)

Closed — full account archived in `documentation/backlog_history/bugs_done_details.md`.

### 105. The 45-minute time budget counted bytes to read, not answers to generate (Resolved 2026-07-26)

Closed — full account archived in `documentation/backlog_history/bugs_done_details.md`.

### 104. A captcha-swallowed submit hid behind stale invalid flags, so #99 never fired (Resolved 2026-07-26)

Closed — full account archived in `documentation/backlog_history/bugs_done_details.md`.

### 103. #98 showed the model react-select's internal option ids, and it answered with them (Resolved 2026-07-26)

Closed — full account archived in `documentation/backlog_history/bugs_done_details.md`.

### 102. #95's early exit read stale invalid flags and called four accepted submissions failures (Resolved 2026-07-26)

Closed — full account archived in `documentation/backlog_history/bugs_done_details.md`.

### 101. A submit click that timed out reported nothing about what blocked it (Resolved 2026-07-25)

Closed — full account archived in `documentation/backlog_history/bugs_done_details.md`.

### 100. A field that lands and is rejected anyway had no diagnostic at all (Resolved 2026-07-25)

Closed — full account archived in `documentation/backlog_history/bugs_done_details.md`.

### 99. A submit silently swallowed by reCAPTCHA was reported as an ordinary validation bounce (Resolved 2026-07-25)

Closed — full account archived in `documentation/backlog_history/bugs_done_details.md`.

### 98. The model was never shown a dropdown's permitted values, so it guessed the wording (Resolved 2026-07-25)

Closed — full account archived in `documentation/backlog_history/bugs_done_details.md`.

### 97. An uncommittable field named the control but never the value that was tried (Resolved 2026-07-25)

Closed — full account archived in `documentation/backlog_history/bugs_done_details.md`.

### 96. Nothing recorded what a submit verdict was actually decided on (Resolved 2026-07-25)

Closed — full account archived in `documentation/backlog_history/bugs_done_details.md`.

### 95. The submit verdict was read from the DOM the instant the click returned, racing the submission itself (Resolved 2026-07-25)

Closed — full account archived in `documentation/backlog_history/bugs_done_details.md`.

### 94. The dedup row was written at document generation, so a job that never submitted was skipped forever (Resolved 2026-07-25)

Closed — full account archived in `documentation/backlog_history/bugs_done_details.md`.

### 93. Greenhouse's emailed security-code gate read as a validation error, burning the full 45-minute timeout (Resolved 2026-07-25)

Closed — full account archived in `documentation/backlog_history/bugs_done_details.md`.

### 92. Checkbox-group ids contain brackets, which are CSS attribute syntax, so they resolved to nothing (Resolved 2026-07-25)

Closed — full account archived in `documentation/backlog_history/bugs_done_details.md`.

### 91. #90's single-option rule could never fire, because typing filters the sole option out (Resolved 2026-07-25)

Closed — full account archived in `documentation/backlog_history/bugs_done_details.md`.

### 90. A required control with exactly one option was refused, sending a job to manual review one click from completion (Resolved 2026-07-25)

Closed — full account archived in `documentation/backlog_history/bugs_done_details.md`.

### 89. A late-rendering confirmation page is missed, so a successful submit is retried — filing duplicates (Resolved 2026-07-25)

Closed — full account archived in `documentation/backlog_history/bugs_done_details.md`.

### 88. A required widget that cannot accept the configured value was written off as a submit failure (Resolved 2026-07-25)

Closed — full account archived in `documentation/backlog_history/bugs_done_details.md`.

### 87. The submit locator clicked the click-to-reveal "Apply" button, so no retry ever actually submitted (Resolved 2026-07-25)

Closed — full account archived in `documentation/backlog_history/bugs_done_details.md`.

### 86. Lever's location typeahead was invisible to combobox detection, so every Lever application failed (Resolved 2026-07-25, verified against the live form)

Closed — full account archived in `documentation/backlog_history/bugs_done_details.md`.

### 85. Four early-exit paths left rows stranded in PROCESSING, invisible to every future queue (Resolved 2026-07-25)

Closed — full account archived in `documentation/backlog_history/bugs_done_details.md`.

### 84. #82's manual-routing branch was never applied, so refused jobs were written off as FAILED_SUBMIT (Resolved 2026-07-25)

Closed — full account archived in `documentation/backlog_history/bugs_done_details.md`.

### 83. The payload breaker guarded the context window but not the time budget, burning the full 45-minute timeout (Resolved 2026-07-25)

Closed — full account archived in `documentation/backlog_history/bugs_done_details.md`.

### 82. Once the commit worked, an unanswerable legal attestation would have been guessed and really submitted (Resolved 2026-07-25)

Closed — full account archived in `documentation/backlog_history/bugs_done_details.md`.

### 81. data-value mirrors the typed search text, so every react-select falsely reported "landed" (Resolved 2026-07-25)

Closed — full account archived in `documentation/backlog_history/bugs_done_details.md`.

### 80. The retry loop logged the payload size but never which fields were still invalid (Resolved 2026-07-25)

Closed — full account archived in `documentation/backlog_history/bugs_done_details.md`.

### 79. The option wait watched an unrelated widget, and committing option-0 filed the wrong location (Resolved 2026-07-25, verified against the live form)

Closed — full account archived in `documentation/backlog_history/bugs_done_details.md`.

### 78. Fill() never opens a react-select menu, and the read-back matched the input itself (Resolved 2026-07-25, verified against the live form)

Closed — full account archived in `documentation/backlog_history/bugs_done_details.md`.

### 77. Enter was pressed before react-select had loaded any option, so the commit selected nothing (Resolved 2026-07-25, live confirmation pending)

Closed — full account archived in `documentation/backlog_history/bugs_done_details.md`.

### 76. #74's own read-back checked el.value first, silently disabling the combobox commit it had just added (Resolved 2026-07-25, live confirmation pending)

Closed — full account archived in `documentation/backlog_history/bugs_done_details.md`.

### 75. #74's combobox commit was wired into the retry path but not the initial fill, guaranteeing a wasted retry cycle (Resolved 2026-07-25, live confirmation pending)

Closed — full account archived in `documentation/backlog_history/bugs_done_details.md`.

### 74. react-select comboboxes were filled but never committed, so their validated value stayed empty (Resolved 2026-07-25, live confirmation pending)

Closed — full account archived in `documentation/backlog_history/bugs_done_details.md`.

### 73. A CSS id selector cannot start with a digit, so Greenhouse's numeric custom-question ids were unfillable half the time (Resolved 2026-07-25)

Closed — full account archived in `documentation/backlog_history/bugs_done_details.md`.

### 72. The retry loop counts empty-valued and non-landing fixes as applied, reporting progress it is not making (Resolved 2026-07-25, accounting fixed; underlying non-convergence still under live investigation)

Closed — full account archived in `documentation/backlog_history/bugs_done_details.md`.

### 71. firstVisibleLocator's .First() fallback reintroduces the very hang it was written to prevent, at the submit click (Resolved 2026-07-25)

Closed — full account archived in `documentation/backlog_history/bugs_done_details.md`.

### 70. The validation-retry loop strips the page's own error text, so the model never learns why a field bounced (Resolved 2026-07-25)

Closed — full account archived in `documentation/backlog_history/bugs_done_details.md`.

### 69. Discovery stored the searched role as job_title and discarded the real headline (Resolved 2026-07-25)

Closed — full account archived in `documentation/backlog_history/bugs_done_details.md`.

### 68. SaveFormMapping cached semantically-empty mappings, burning a Learner Module call per visit (Resolved 2026-07-25)

Closed — full account archived in `documentation/backlog_history/bugs_done_details.md`.

### 67. The initial fill path never received #65/#66's fixes, so required dropdowns always failed the first pass (Resolved 2026-07-25)

Closed — full account archived in `documentation/backlog_history/bugs_done_details.md`.

### 66. SolveValidationErrors returns bare id/name values, not CSS selectors, so every proposed fix matched nothing (Resolved 2026-07-25)

Closed — full account archived in `documentation/backlog_history/bugs_done_details.md`.

### 65. Validation fixes were applied with Fill() only and their errors discarded, so required dropdowns could never be satisfied (Resolved 2026-07-25)

Closed — full account archived in `documentation/backlog_history/bugs_done_details.md`.

### 64. SolveValidationErrors re-sends the entire form instead of just the fields that failed, timing out on large forms (Resolved 2026-07-25)

Closed — full account archived in `documentation/backlog_history/bugs_done_details.md`.

### 63. Every fit score was computed and thrown away — the only writer of fit_score had zero callers (Resolved 2026-07-25)

Closed — full account archived in `documentation/backlog_history/bugs_done_details.md`.

### 61. The cover letter was never sent to any employer — no handler ever filled it (Resolved 2026-07-24)

Closed — full account archived in `documentation/backlog_history/bugs_done_details.md`.

### 62. The saved cover letter was deleted from the application folder, stripping the manual-apply queue (Resolved 2026-07-24)

Closed — full account archived in `documentation/backlog_history/bugs_done_details.md`.

### 60. Ollama server pinned to an unnecessarily conservative 6,144-token context window — the dominant cause of MANUAL_REQUIRED outcomes (Resolved 2026-07-24)

Closed — full account archived in `documentation/backlog_history/bugs_done_details.md`.

### 59. Generic submit-button selector could click a hidden anti-spam-widget button instead of the real submit control (Resolved 2026-07-24)

Closed — full account archived in `documentation/backlog_history/bugs_done_details.md`.

### 58. Stale career_chunks embedding dimension silently zeroed out all live RAG resume-context retrieval (Resolved 2026-07-24)

Closed — full account archived in `documentation/backlog_history/bugs_done_details.md`.

### 57. Forms too large for Ollama's context window burned a full doc-gen cycle before failing with an ugly HTTP 400 (Resolved 2026-07-24)

Closed — full account archived in `documentation/backlog_history/bugs_done_details.md`.

### 56. Dashboard has no tile for BLOCKED_CAPTCHA or INVALID_URL, silently omitting 9% of all job_funnel rows (Resolved 2026-07-24)

Closed — full account archived in `documentation/backlog_history/bugs_done_details.md`.

### 55. Jobs killed mid-flight get permanently stuck in PROCESSING, never retried, inflating the dashboard's live count (Resolved 2026-07-24)

Closed — full account archived in `documentation/backlog_history/bugs_done_details.md`.

### 54. Raw-HTML captcha pre-check misclassifies Ashby's client-rendered SPA shell as a block (Resolved 2026-07-24)

Closed — full account archived in `documentation/backlog_history/bugs_done_details.md`.

### 53. isSubmissionConfirmed only ever ran for Lever/Greenhouse/LinkedIn — every other ATS platform's APPLIED had zero confirmation evidence (Resolved 2026-07-24)

Closed — full account archived in `documentation/backlog_history/bugs_done_details.md`.

### 52. SolveValidationErrors sends the whole page's DOM, tripping the LLM-cost circuit breaker and losing otherwise-successful applications (Resolved 2026-07-23)

Closed — full account archived in `documentation/backlog_history/bugs_done_details.md`.

### 51. Post-submit success check trusted any URL change, not proof of an actual successful submission (Resolved 2026-07-23)

Closed — full account archived in `documentation/backlog_history/bugs_done_details.md`.

### 50. Workable requires account sign-in on every posting — same structural class as Workday (Resolved 2026-07-23)

Closed — full account archived in `documentation/backlog_history/bugs_done_details.md`.

### 49. handleGreenhouse's hardcoded submit selector doesn't exist on modern-board postings (Resolved 2026-07-23)

Closed — full account archived in `documentation/backlog_history/bugs_done_details.md`.

### 48. Lever click-to-reveal (bug #47's fix) doesn't fire on a second real posting — possible page staleness after the long doc-gen wait (Resolved 2026-07-24, not reproduced)

Closed — full account archived in `documentation/backlog_history/bugs_done_details.md`.

### 47. Dedicated Greenhouse/Lever handlers never click "Apply" to reveal the form, only the generic Learner Module path does (Resolved 2026-07-23)

Closed — full account archived in `documentation/backlog_history/bugs_done_details.md`.

### 46. Raw-HTML job-description fetch also misdetects reCAPTCHA/Turnstile widgets as a block, before fit-scoring even runs (Resolved 2026-07-23)

Closed — full account archived in `documentation/backlog_history/bugs_done_details.md`.

### 45. isCaptchaBlocked misdetects standard reCAPTCHA/hCaptcha anti-spam widgets on real forms as a full block (Resolved 2026-07-23)

Closed — full account archived in `documentation/backlog_history/bugs_done_details.md`.

### 4. AttemptSubmit form-fill logic never looked inside iframes (Resolved 2026-07-23)

Closed — full account archived in `documentation/backlog_history/bugs_done_details.md`.

### 10. DOM-mapped fill failures never fell back to the Vision module, only outright mapping failures did (Resolved 2026-07-24)

Closed — full account archived in `documentation/backlog_history/bugs_done_details.md`.

### 11. FunnelEngine lets Jobvite `/search` listing pages into the pipeline

Closed — full account archived in `documentation/backlog_history/bugs_done_details.md`.

### 14. No accessible-label fallback for form-field filling, only CSS selector guessing (Resolved 2026-07-24)

Closed — full account archived in `documentation/backlog_history/bugs_done_details.md`.

### 18. Workday postings burn full Learner+Vision cycles against an auth-gated application flow with no fillable form

Closed — full account archived in `documentation/backlog_history/bugs_done_details.md`.

### 19. Workday URL parsing takes the locale/site segment as the company name

Closed — full account archived in `documentation/backlog_history/bugs_done_details.md`.

### 25. Fit scoring ignores geographic eligibility restrictions

Closed — full account archived in `documentation/backlog_history/bugs_done_details.md`.

### 24. Prompt-injection quarantine may false-positive on ordinary job-page copy ("you are a...")

Closed — full account archived in `documentation/backlog_history/bugs_done_details.md`.

### 20. Email tracker classifies unrelated emails as INTERVIEW_REQUESTED and writes them to the DB

Closed — full account archived in `documentation/backlog_history/bugs_done_details.md`.

### 15. Dedicated Greenhouse/Lever handlers timing out waiting for the form to render

Closed — full account archived in `documentation/backlog_history/bugs_done_details.md`.

### 7. FunnelEngine still lets Greenhouse job-search/listing pages into the pipeline

Closed — full account archived in `documentation/backlog_history/bugs_done_details.md`.

### 8. Dynamic/Learner Module fill path never clicks an "Apply" button to reveal click-to-reveal application forms (Resolved 2026-07-24)

Closed — full account archived in `documentation/backlog_history/bugs_done_details.md`.

## ✅ Resolved

### 23. Bot-protection interstitials (DataDome) aren't detected, burning full cycles and feeding the Learner captcha pages (Resolved 2026-07-22)

Closed — full account archived in `documentation/backlog_history/bugs_done_details.md`.

### 21. SaveFormMapping caches non-JSON LLM output, poisoning every future visit to the domain (Resolved 2026-07-22)

Closed — full account archived in `documentation/backlog_history/bugs_done_details.md`.

### 22. Stale pre-filter backlog rows and error-redirect URLs bypass every discovery filter (Resolved 2026-07-22, mitigated)

Closed — full account archived in `documentation/backlog_history/bugs_done_details.md`.

### 16. #14's label fallback needs a GetByPlaceholder tier too (Resolved 2026-07-21)

Closed — full account archived in `documentation/backlog_history/bugs_done_details.md`.

### 17. ORDER BY last_updated DESC picked a stale row over a genuinely newer one (Resolved 2026-07-21)

Closed — full account archived in `documentation/backlog_history/bugs_done_details.md`.

### 12. Same job URL reprocessed repeatedly, hitting a UNIQUE constraint on applied_jobs.url (Resolved 2026-07-21)

Closed — full account archived in `documentation/backlog_history/bugs_done_details.md`.

### 13. Ollama gets kernel OOM-killed under this machine's real RAM ceiling (Resolved 2026-07-21, mitigated not eliminated)

Closed — full account archived in `documentation/backlog_history/bugs_done_details.md`.

### 9. Dead-job-posting detection missed common phrasings, wasting cycles on expired listings (Resolved 2026-07-21)

Closed — full account archived in `documentation/backlog_history/bugs_done_details.md`.

### 5. FunnelEngine let Workday-docs and Workable-search pages into the pipeline (Resolved 2026-07-21)
**Symptom:** found 2026-07-20 while diagnosing #4. Confirmed cases: `https://jobs.workable.com/search/global/remote-software-engineer-jobs` (a search-results page, not a posting) and `developer.workday.com/welcome`, `/documentation`, `/api-overview`, `/rest-api-explorer` (Workday's own developer-docs site, not a `myworkdayjobs.com` job posting) were all discovered and scored as candidate jobs, then went through a full score → tailor → `AttemptSubmit` cycle before failing to fill a nonexistent `first_name` field. Re-confirmed live during this session's 2026-07-21 batch run: **5 consecutive `developer.workday.com` doc pages in a row** (`welcome`, `api-overview`, `rest-api-explored`, `documentation`, `welcome` again) each burned a full ~5-10 minute local-LLM generation cycle plus a Learner Module DOM-mapping attempt before failing — direct evidence this bug could dominate a run's compute budget, not just an occasional nuisance.

**Root cause:** `isValidATSUrl` in `pkg/scraper/funnel.go` (used by the Yahoo-fallback discovery path) had `"workday.com"` as a bare entry in its `atsDomains` list, matched via a suffix check (`host == domain || strings.HasSuffix(host, "."+domain)`) — this matches *every* subdomain of `workday.com`, including `developer.workday.com`, not just the actual job-posting subdomain pattern `*.myworkdayjobs.com` (which was already separately present in the list). Similarly `"workable.com"` matched its own `/search/` listing-page path with no path-level check.

**Fix:** removed the bare `"workday.com"` entry from `atsDomains` (keeping `"myworkdayjobs.com"`), and added a path check that rejects any `workable.com`/`*.workable.com` URL whose path contains `/search/`. Added `TestIsValidATSUrl` in `pkg/scraper/funnel_test.go` covering both fixes plus regression cases for real Workday/Workable/Lever postings. `go build/vet/test ./...` all pass. Delegated to Gemini 3.1 Pro via `agy`, diff verified against `git diff` before commit.

**Not done (tracked separately as #7):** the Greenhouse false positive from the same original finding (`job-boards.greenhouse.io/remotecom/jobs/7778860003`) — its URL shape looks like a real posting, so no safe fix was obvious without a fresh live repro.

### 6. Ollama generation throughput collapses mid-request, likely context-shift thrashing (Resolved 2026-07-21)

Closed — full account archived in `documentation/backlog_history/bugs_done_details.md`.

### 3. Ollama context window overflow hangs the model server (Resolved 2026-07-20)
**Symptom:** During a 2026-07-20 live `cmd/agent` run, every `AttemptSubmit` call began failing with `failed to reach ollama at http://localhost:11434 ... context deadline exceeded`. `ollama ps` showed `qwen3:30b-instruct` stuck in `Stopping...` for 15+ minutes, unresponsive to `ollama stop`, while still holding ~20GB RAM (system had only ~360MB available at the worst point). The only fix at the time was `kill -9` on the underlying `llama-server` process.

**Root cause:** two compounding issues. (1) The local Ollama server was started with a 4096-token context window (`-c 4096`), but `pkg/mcp`'s `ProcessJobApplication` prompts (job description + RAG career-chunk context + profile constraints, generating resume + cover letter + interview prep in one call) routinely exceed that — observed at 4237 and 4271 tokens — and llama.cpp's server hangs in a broken state under overflow instead of failing that one request cleanly. (2) `cmd/agent/main.go` hardcoded `numWorkers := 10` ("for massive concurrency on Paid Tier"), but local Ollama serves one request at a time (`-np 1`); 10 workers piling onto one slot caused severe queuing, and any worker's request that queued behind others could blow past its own 10-minute client-side timeout even once increasing the context window fixed the overflow itself — confirmed live: a real job's doc-gen call timed out at ~10 minutes with 2 workers before dropping to 1 fixed it.

**Fix:** (1) set `OLLAMA_CONTEXT_LENGTH=6144` via a systemd user-service drop-in (`~/.config/systemd/user/ollama.service.d/override.conf`) so `llama-server` launches with `-c 6144` — verified via `ps` after restart, and confirmed a real ~2700-token application prompt no longer errors. (2) added `defaultWorkerCount()` to `cmd/agent/main.go`, returning `1` when `LLM_PROVIDER` is unset/`ollama` (matches the single-slot server exactly, eliminating queuing) and `10` for paid backends, overridable via a new `WORKER_COUNT` env var. Verified live: a real SmartRecruiters job (`SigmaSoftware2`) completed tailoring in ~5 minutes with zero Ollama errors and reached the actual Playwright submit step (see bug #4) — the first clean end-to-end run of the session.

**Operator note:** while diagnosing this, found and killed two long-lived orphaned `go run cmd/agent` child processes from earlier restarts in the same session (their compiled binaries live under `~/.cache/go-build/<hash>-d/main`, a path shape that does **not** match the more obvious `go-build.*exe/main` grep pattern used elsewhere in this file's fix notes — a bare `kill <parent-pid>` does not reliably kill `go run`'s child; verify with `ps aux | grep -E "go run cmd/agent|go-build"` *and* a `/proc/*/cwd` scan for the project directory, not just the narrower pattern, before assuming a restart is clean). These zombies were silently consuming the single Ollama slot and worker-numbering in logs, which is what made this bug look unresolved for longer than it actually was.

### EEO/demographic form fields could be answered with fabricated data (Resolved 2026-07-20)
**Symptom:** not yet observed in a real submission, but found by code inspection while reviewing full-auto (`auto_submit_click: true`) behavior: `pkg/mcp/client.go`'s `SolveValidationErrors` (used by `AttemptSubmit`'s validation-error retry path to fill any required field a form rejects) had no guardrail against inventing an answer. Neither `pii.yaml` nor `profile.yaml` carried any EEO/demographic fields, so a required race/gender/veteran/disability question on a real ATS form could have been answered with an LLM-hallucinated value and submitted under the user's identity.

**Fix:** added an `EEO` struct to `pkg/config/pii.go` (gender, race/ethnicity, veteran status, disability status, sexual orientation — all optional) with an `EEO.Summary()` method that renders configured answers as prompt context and explicitly lists any left-blank category as "must decline, do not guess." Wired it into the `SolveValidationErrors` call site in `pkg/submitter/browser.go` (prepended to `profileContext`), and added a `CRITICAL RULE` to `SolveValidationErrors`'s system prompt in `pkg/mcp/client.go` forbidding invented values for any legally-sensitive demographic category, requiring "Decline to answer" instead when the field wasn't explicitly provided. Verified no actual submission had used a fabricated EEO answer before the fix landed (job_funnel's `APPLIED` count was unchanged at 36 throughout the vulnerable window). `pii.yaml` (gitignored, not committed) now carries the user's actual answers.

### Playwright driver download fails — dead Azure CDN (Resolved 2026-07-20)
**Symptom:** `cmd/agent` failed immediately on startup with `Failed to install Playwright: could not install driver: ... got non 200 status code: 404 (404 Not Found) from https://playwright.azureedge.net/builds/driver/playwright-*-linux.zip` (tried against driver versions 1.42.1 and 1.60.0, both 404).

**Root cause:** `go.mod` pinned `github.com/playwright-community/playwright-go v0.4201.1`, an old version that downloads its Node driver from `playwright.azureedge.net` — a CDN Microsoft has since retired. The fork's newest tag (`v0.6100.0`) switches to an npm/Node-based installer instead, but that tag's `go.mod` still declares its module path as the original `github.com/mxschmitt/playwright-go`, so `go get github.com/playwright-community/playwright-go@v0.6100.0` fails with a module-path mismatch error.

**Fix:** changed all four import sites (`cmd/agent/main.go`, `pkg/submitter/{browser,browser_test,dynamic,vision}.go`) from `github.com/playwright-community/playwright-go` to `github.com/mxschmitt/playwright-go`, then `go get github.com/mxschmitt/playwright-go@v0.6100.0 && go mod tidy`. Verified: `go build/vet/test ./...` all pass; the driver now installs successfully via npm instead of the dead CDN.

### cmd/agent crashes/hangs when run on the Bazzite host instead of the documented distrobox container (Resolved 2026-07-20)
**Symptom:** even after the Playwright driver fix above, the shared Chromium `browser` instance would die partway through a live run (`playwright: target closed: Target page, context or browser has been closed`) on every `AttemptSubmit` call after the first few, on this dev machine specifically.

**Root cause:** this machine runs Bazzite (Fedora Atomic/Kinoite), not Ubuntu. Playwright's bundled Chromium expects Ubuntu-native shared libraries (`libicu74`, `libjpeg-turbo8`, `libwoff1`); Fedora ships different, ABI-mismatched versions. The README already documents the correct setup (`distrobox create --name career-agent --image ubuntu:22.04`) and a matching container existed on this machine, but it was never actually being used for live runs, and even inside it, apt's `golang-go` package is a stale Go 1.18.1 that can't parse this project's `go 1.26.5` directive in `go.mod`.

**Fix:** installed Go 1.26.5 manually inside the `career-agent` distrobox container (apt's version is too old), confirmed `libjpeg-turbo8`/`libwoff1` already matched exactly inside the Ubuntu 22.04 container. Running `cmd/agent` from inside the container (with `/usr/local/go/bin` prepended to `PATH`) resolved the browser-crash pattern — subsequent runs got well past the point where it used to die (multiple jobs reached "Generating tailored documents" and beyond, whereas host runs died after 2-4 jobs). **Operator note:** `export PATH=$PATH:/usr/local/go/bin` (append) does *not* work if the container already has an older `go` earlier in `PATH` — must be `export PATH=/usr/local/go/bin:$PATH` (prepend) so the new Go wins.

**Regressed and re-fixed 2026-07-22:** despite being marked Resolved above, this session started with `cmd/agent` running directly on the bare host again — the container's Go toolchain had reverted to (or never kept) 1.18.1, and nothing enforced the container requirement. Symptom was worse than a crash this time and much harder to diagnose: Chromium didn't crash or error, it **silently rendered every page completely blank** (zero DOM content, zero inputs, empty body text) while `page.Goto` still reported success — every downstream fill strategy then timed out for a completely different reason (nothing to find) than the error message suggested, and this consumed most of the session before being caught. Confirmed by loading the exact same failing URL with a standalone script inside the container: real content, real form, real (if separately buggy) failure reason. Re-installed Go 1.26.5 inside the container (same steps as above) and re-verified `go build/vet/test` and a live page-render all pass there. **This is now the single most important thing to check first in any future session**: confirm `ps aux` shows the running `cmd/agent` binary was built via `distrobox enter career-agent -- go build ...`, not a bare host `go build`, before trusting any Playwright-related symptom as a code bug. See also the new memory entry `career-agent-core-runtime-environment` (Claude Code auto-memory, not tracked in this repo) written specifically so this doesn't cost another multi-hour session.

### 34. A cookie-consent banner's backdrop silently intercepts every click, defeating clickApplyIfPresent

Closed — full account archived in `documentation/backlog_history/bugs_done_details.md`.

### 35. SmartRecruiters' "I'm interested" button and post-click CAPTCHA reveals both went undetected

Closed — full account archived in `documentation/backlog_history/bugs_done_details.md`.

### 36. Jobvite's "Data Consent" step means the application form doesn't exist until a location/language <select> is chosen

Closed — full account archived in `documentation/backlog_history/bugs_done_details.md`.

### 37. fillActionTimeoutMs (15000ms) too tight for genuine CPU contention from the co-located Ollama model

Closed — full account archived in `documentation/backlog_history/bugs_done_details.md`.

### 38. FunnelEngine kept sending Learner+doc-gen cycles at a 0%-success source and let Workday monopolize the worker queue

Closed — full account archived in `documentation/backlog_history/bugs_done_details.md`.

### 39. Vision-fallback fill fails with "empty selector provided for form filling" (Resolved 2026-07-23)

Closed — full account archived in `documentation/backlog_history/bugs_done_details.md`.

### 40. ~200+ files/dirs under applications/ are still owned by a stale UID from an earlier containerized run

Closed — full account archived in `documentation/backlog_history/bugs_done_details.md`.

### 41. applytojob.com and recruitee.com board-index/landing pages scored and processed as real postings

Closed — full account archived in `documentation/backlog_history/bugs_done_details.md`.

### 42. www.bamboohr.com and app.bamboohr.com pages (marketing site, shared login portal) scored as postings

Closed — full account archived in `documentation/backlog_history/bugs_done_details.md`.

### 43. getByLabel/getByPlaceholder threw a Playwright strict-mode violation when a label matched more than one element

Closed — full account archived in `documentation/backlog_history/bugs_done_details.md`.

### 44. BambooHR corporate subdomains kept slipping past a growing denylist (Resolved 2026-07-23)

Closed — full account archived in `documentation/backlog_history/bugs_done_details.md`.

### 130. Yahoo fallback drops discovery on transient unexpected EOF responses

Closed — full account archived in `documentation/backlog_history/bugs_done_details.md`.

### 393 Playwright Host missing dependencies to run browsers

Done — full account archived in `documentation/backlog_history/bugs_done_details.md` item #393.

### 394 QUARANTINED_PROMPT_INJECTION has massive false positive rate on legitimate jobs

Done — full account archived in `documentation/backlog_history/bugs_done_details.md` item #394.

### 395 Validation loop times out waiting for Ollama context deadline

Done — full account archived in `documentation/backlog_history/bugs_done_details.md` item #395.

### 131. ATS board polling discards truncated JSON without retry

Closed — full account archived in `documentation/backlog_history/bugs_done_details.md`.

### 412. Duplicate check in pipeline.go resets APPLIED jobs back to DISCOVERED

Closed — full account archived in `documentation/backlog_history/bugs_done_details.md`.

### 413. Enhance Greenhouse validation error resolver for <fieldset> and radio groups (Resolved 2026-07-29)

Closed — full account archived in `documentation/backlog_history/bugs_done_details.md`.

### 416. modernc.org/sqlite uses invalid pragma format, causing DB locks
The database connection strings in `pkg/storage/manager.go` and `cmd/dashboard/main.go` use the old `go-sqlite3` format for pragmas (e.g. `?_journal_mode=WAL&_busy_timeout=5000`). The `modernc.org/sqlite` driver ignores this format, requiring `_pragma=name(value)`. This silently disables WAL mode and busy timeouts, causing `database is locked` errors during concurrent access. Fix: Update all connection strings to the `_pragma=` format.

### 417. go-rod prototype hangs on missing elements without timeouts
The `go-rod` prototype in `cmd/prototype_go_rod/main.go` utilizes `.MustWaitLoad()`, `page.Element()`, and `.MustInput()` without configuring a page timeout. If an element is missing from the page or the network stalls, these methods will block and hang indefinitely. Fix: Apply a timeout context (e.g., `page.Timeout(15 * time.Second)`) before performing DOM selections and waits.

### 414. Enforce single-instance execution to prevent DB corruption and stuck jobs

Closed — full account archived in `documentation/backlog_history/bugs_done_details.md`.

### 513. Application Mode hardening is incomplete: Qualified Jobs mutations and settings activation are not safely verified

Closed — full account archived in `documentation/backlog_history/bugs_done_details.md`.

### 514. Qualified Jobs and operator settings contain post-hardening runtime regressions

Closed — full account archived in `documentation/backlog_history/bugs_done_details.md`.

### 521. Indistinguishable duplicate cards let one click mark a job applied that was never submitted

Closed 2026-08-06 — full account archived in `documentation/backlog_history/bugs_done_details.md` item #521.

### 523. The assisted browser's network guard aborts requests silently

Closed 2026-08-06 — full account archived in `documentation/backlog_history/bugs_done_details.md` item #523.

### 519. Assisted Apply cannot prefill on Greenhouse or Lever, the only two ATSes it is used with

Closed — full account archived in `documentation/backlog_history/bugs_done_details.md`.

### 525. Assisted Apply attaches a .txt extraction where the automatic path uploads the master cover letter PDF

Closed — full account archived in `documentation/backlog_history/bugs_done_details.md` item #525.

### 520. Lever submissions fail inside the assisted browser but succeed in an ordinary one

Closed 2026-08-06 — full account archived in `documentation/backlog_history/bugs_done_details.md` item #520.

### 524. Existing queue rows carry no location, so #516's gate cannot screen them

**Found 2026-08-06.** #516 added country extraction and gating at discovery, but only for postings discovered from that point on. The ~520 rows already in the assisted queue still have empty `job_location` and `is_remote`, so nothing can screen them and the operator must open each posting to check its country by hand.

This is not hypothetical: during the trial, candidate 259800 was an India-scoped role that reached a live application attempt, and jobgether was found publishing the identical role for India, the US, Portugal, and Brazil simultaneously. Both were caught by fetching the ATS feed manually, one candidate at a time.

**Fix direction.** A one-off backfill pass over existing rows that re-fetches each posting from its board feed (Lever's `country`/`categories.location`, Greenhouse's `location.name`) and writes `job_location`/`is_remote` through `UpdateFunnelIdentity`. Expired postings should be terminalized rather than left unscreenable — a meaningful share of trial candidates turned out to be `INVALID_URL`.

**Progress 2026-08-07 — the tool is built and verified; the data is one board short.**

`cmd/backfill-location` ships, with the parsing in `pkg/scraper/locationbackfill.go`. It resolves a stored posting URL back to the board account that can still answer for it, across Greenhouse, Lever and **Workable** (the fix direction named only the first two; Workable turned out to hold 361 of the 524 queued rows, so leaving it out would have addressed 17% of the problem). It is re-runnable: a row that already has a location is skipped without a fetch.

**Applied live against the real `applications.db`, and verified end to end:** 67 queued rows now carry a real advertised location, 18 confirmed-dead postings were marked `INVALID_URL`/`expired`, and a dashboard built from the current tree on `127.0.0.1:8099` served all 67 through `/api/assisted` with locations like `"Chicago, Illinois, United States"` and `"North Carolina - Raleigh"`. The production dashboard on `:8080` was read but never touched.

**What remains: 361 Workable rows.** Running the backfill polled ~210 Workable accounts, and `apply.workable.com` responded by rate-limiting **every path on the host** with `Retry-After: 84643` — a 23.5-hour block, not a per-request throttle. Re-run `go run ./cmd/backfill-location -confirm` any time after **2026-08-08 ~03:20 UTC**; it will pick up exactly the rows still missing a location. See the Workable operational trap above before doing so.

A further 75 rows sit on boards with no readable public feed (Workday, SmartRecruiters, Ashby, Jobvite, BambooHR, applytojob, recruitee, pinpointhq). Those cannot be backfilled this way at all and are out of scope for this row — the tool leaves them untouched and reports them separately.

**Two findings came out of the work,** both filed rather than folded in silently: #530 (a posting marked dead still occupies a queue card — none of the 18 left the queue) and, on the first partial pass, that **56% of everything resolvable was outside the configured US/CA allowlist**, which is the clearest evidence yet that this row was worth doing.

**Progress 2026-08-08 — Workable's block cleared; re-run resolved most of the rest, but 161 in-scope rows remain.**

Live counts confirmed the 2026-08-07 state exactly (67/524 with a location, 361 Workable rows still blocked) before touching anything. A dry run (`-confirm=false`) came back clean — no 429/`Retry-After` from `apply.workable.com`, only 5 ordinary per-posting failures (4×404, 1 timeout) — confirming the host-wide block had expired. The confirmed run (`go run ./cmd/backfill-location -scope queue -confirm`) then wrote **223 newly resolved locations** (290/524 total, all from Workable) and terminalized **144** postings as confirmed dead (up from 18), fully accounted against the 524-row scope (67 + 223 + 144 + 15 + 75). Verified against production data via direct SQL and a temporary dashboard on `127.0.0.1:8099` (production `:8080` untouched): `with_location` count matches the tool's own report exactly, zero dead rows leak into `/api/assisted`, no year-0001 timestamps, no `APPLIED` status changes.

**What remains: 161 in-scope rows, not out of scope.** The confirm run itself drew 11 fresh Workable 429s partway through (vs. zero on the dry run minutes earlier) — not a long-`Retry-After` host block, just per-account throttling — leaving 138 Workable rows still without a location, alongside 23 Greenhouse/Lever rows that were already unresolved before this session and were untouched by this run (need a closer look on the next pass — not yet understood why). Per this row's own closure criteria, these are not "boards the tool cannot resolve," so #524 stays **Pending**. A further re-run after a short cooldown (rather than immediately, given the 429s that appeared mid-run) should resolve most of the remainder; the ~75 no-readable-feed rows remain the only genuinely out-of-scope portion.

### 530. A posting that has died still occupies a queue card, because the assisted queue never reads the funnel status it selects

**Closed 2026-08-07 — full account archived in `documentation/backlog_history/bugs_done_details.md` item #530.**

### 531. Every queue card whose timestamp was written by Go reports `last_updated` as year 0001

**Closed 2026-08-07 — full account archived in `documentation/backlog_history/bugs_done_details.md` item #531.**

### 522. Agent lifecycle and liveness reporting are unreliable in four distinct ways

**Found 2026-08-05/06**, all confirmed live during the acceptance trial. Individually minor; together they make "is the agent running?" unanswerable without inspecting `/proc` by hand.

1. **`/api/agent/stop` returns `{"status":"stopped"}` while the agent is still alive.** It signals and returns without waiting. Observed repeatedly; the process often needed 60+ seconds, and once a `SIGKILL`, before actually exiting.
2. **The stopped agent becomes a zombie.** `serveAgentStart` uses `cmd.Start()` and never `Wait()`s, so the child is never reaped. It lingers as `Z`/`<defunct>` until the dashboard itself exits. `kill -0` succeeds on a zombie, so naive liveness checks report a dead agent as running — the flock check is correct, ad-hoc checks are not.
3. **`daemon_active` reads true with no agent running.** It compares `applications/active_operator_settings.json` against the effective settings and has no liveness component at all, so it reflects "the last daemon agreed with these settings", not "a daemon exists".
4. **The dashboard launches Assisted Apply through `go run`.** `assistedApplicationCommand` prefers `go run ./cmd/assist` whenever a go.mod root exists, so a built `career_assist_bin` is never what executes in a source checkout. It also adds a wrapper process between the dashboard and the real assist process — the classic orphan shape this file already documents for `cmd/agent`.

**Fix direction.** Have stop wait briefly and report truthfully; reap the child; give `daemon_active` a real liveness check (the flock already used by `agentPID` is the obvious source); and either exec the built binary or document the `go run` preference deliberately.

### 529. 49 emails processed, zero outcomes recorded — the tracker's detections may never be reaching `job_funnel`

Closed — full account archived in `documentation/backlog_history/bugs_done_details.md` item #529.

### 533. An outcome email that matches no application is acknowledged and discarded, so the evidence is unrecoverable

Closed — full account archived in `documentation/backlog_history/bugs_done_details.md` item #533.

### 534. `StartTracker` only ever reads the newest ~51 messages, so any outage longer than the fetch window loses outcomes permanently

Closed — full account archived in `documentation/backlog_history/bugs_done_details.md` item #534.

### 532. Every stage-ledger event has a NULL duration, because the trigger measures time with `julianday()` over a format SQLite cannot parse

**Found 2026-08-07** while closing #529, which needed to prove that a tracker outcome lands in the funnel stage ledger with the right timestamp. It does now. Its `stage_duration_ms` does not, and neither does anything else's.

**The observation.** Against the live `applications.db`, **1,385 of 1,385** `funnel_stage_events` rows have `stage_duration_ms IS NULL`. The column has never held a value since the ledger shipped (improvement #494, 2026-08-01). This is not specific to the tracker — it is every writer, on every transition.

**Cause, already half-documented.** `createFunnelStageLedgerTrigger` (`pkg/storage/manager.go:597`) computes the gap between the new event and the previous one for that URL with

```sql
CAST(ROUND((julianday(NEW.last_updated) - julianday((SELECT occurred_at ...))) * 86400000) AS INTEGER)
```

ADR-003 **decision 6** (bug #531, 2026-08-07) already records why that returns nothing: `modernc.org/sqlite` stores a bound `time.Time` as `time.Time.String()` — `2026-08-06 15:52:28.596249998 +0000 UTC` — and SQLite's own date functions return NULL over that shape. Confirmed directly against the live database: `SELECT julianday(occurred_at) FROM funnel_stage_events` returns empty for every row. So the subtraction is `NULL - NULL`, the `CAST` yields NULL, and the trigger's `CASE` guard never even gets to matter.

ADR-003 **decision 5** asserts in the same document that the ledger records "prior and next state, derived pipeline stage, normalized reason code, timestamp, and elapsed duration". Four of those five are true. The fifth has never been true. The two decisions contradict each other and decision 6 is the correct one — decision 5's text has been corrected in place as part of #529's close, but the code behind it has not.

**Second, smaller problem in the same trigger.** The `pipeline_stage` `CASE` has no arm for `REJECTED` or `INTERVIEW_REQUESTED`, so every outcome event the tracker writes falls through to the generic `ELSE 'funnel'` bucket — the same label used for ordinary funnel churn. Verified live 2026-08-07 with a synthetic rejection: `stage=funnel`. Outcomes are the events the feedback loop exists to isolate, and they are currently indistinguishable by stage from a requeue.

**Why it is only Minor.** Nothing reads the column today. `stage_duration_ms` is projected into `storage.FunnelStageEvent.StageDurationMs` (`pkg/storage/attempts.go:49`) and no caller consumes it — no dashboard panel, no ranking input. The cost is latent: improvement #493's expected-yield ranking is the intended consumer of exactly this data, and it will read a column that is silently NULL for its entire history rather than one that is visibly absent.

**Fix direction.** Do **not** change the DSN's time format — ADR-003 decision 6 records why that is off the table (it would split every timestamp column into two encodings and leave 12,046 existing rows unreadable). Two workable options, in preference order:

1. Have the trigger normalise before measuring. SQLite can parse the stored shape if the trailing zone text is stripped and the space is left as-is: `julianday(substr(x, 1, 23))` handles `2026-08-06 15:52:28.596` correctly. Cheap, but it hard-codes an assumption about the driver's output width, and rows written as RFC3339 (which some paths do produce) would need the same treatment.
2. Compute the duration in Go at the one call site that knows both timestamps and pass it in, demoting the trigger to recording what it is given. This contradicts decision 5's stated reason for using a trigger at all ("raw status writers cannot silently skip history"), so it needs an ADR amendment rather than a quiet change.

Either way the change is a `DROP TRIGGER` + `CREATE TRIGGER` migration, not a `CREATE TRIGGER IF NOT EXISTS` edit — the existing trigger is already installed in the live database and `IF NOT EXISTS` will not replace it. There is **no precedent for a trigger migration in this repo** (`DROP TRIGGER` appears nowhere), so the migration helper is genuinely new work and is the bulk of the Effort 3. Backfilling the 1,385 existing NULLs is possible from `occurred_at` alone and should be a separate, explicitly opt-in step.

### 526. The automatic submitter's network guard aborts requests silently

**Found 2026-08-06** while fixing #523, which covered only the assisted browser. `installSafeBrowserRoutes` (`pkg/submitter/network.go:80`) is the *automatic* submission path's own route guard — a separate function from the one #523 fixed — and it has the same gap: `guard.ValidateURL` rejects a request at `:94` and `route.Abort("accessdenied")` runs with **no record of the rejection**. It logs only the two failure cases around it (`:97` if the abort fails, `:105` if a continue fails), so a request that is blocked cleanly is invisible exactly as it was on the assisted path.

Scope confirmed by call chain, not assumed: the function is reached only from `Pipeline.Execute` (`pkg/submitter/dynamic.go:270`) and `newSubmitPage` (`pkg/submitter/browser.go:1364`). `cmd/assist` never calls it — `FillAssistedMappedPage` receives an already-open page — so #523's fix does not cover this path and this row does not overlap it.

Lower value than #523 only because the automatic path is not the one currently in daily use. The cost when it does bite is the same: #523's own history is a full investigation cycle spent ruling the guard out by building a synthetic reproduction, because the log could not answer the question.

**Second, smaller problem in the same function.** Both existing log lines print the Playwright error with `%v`. That error can quote the request it refers to, which is why #523 deliberately reported an abort failure by fact and safe host rather than by message. These two lines have the exposure #523 removed from the assisted path.

**Fix direction.** Reuse what #523 already built rather than writing a second mechanism: `security.NetworkRejectionReason(err)` already maps every reachable rejection to a bounded code and is exported. The host sanitizer is not — it is `safeAssistedHost` in `cmd/assist/main.go`, unexported. Either move it into `pkg/security` beside the classifier and repoint `cmd/assist` at it, or duplicate ~20 lines; moving it is preferred, since two copies of a redaction rule is how one of them ends up wrong. Then log one bounded record at the rejection point and drop the `%v` from the two existing lines. Do not change what the guard permits.

### 527. The `:memory:` test database silently fails every nested query, so the assisted queue's readiness fields cannot be covered

Closed — full account archived in `documentation/backlog_history/bugs_done_details.md`.

### 528. Assisted Apply attaches the "cover letters are disabled" note when send_cover_letter is false

**Found 2026-08-06** while fixing #525, in the branch immediately beside it. This is the last member of the #515/#517/#525 family — the automatic and assisted paths resolving the same document differently.

With `send_cover_letter: false`, `cmd/agent` sends no cover letter at all: `cmd/agent/pipeline.go` returns `""` for the cover path from both its untailored branch and its tailored branch. But `SaveApplication` still writes a per-job `coverletter.txt`, and in the untailored branch its contents are the literal sentence "Cover letters are disabled (send_cover_letter: false); none was sent with this application."

`storage.GetAssistedDocument` resolves `cover_letter` to that file whenever no master letter applies. So an assisted application would attach a one-sentence note explaining that no cover letter was sent — the same shape of defect as #515, where the résumé resolved to a "master documents were used" placeholder.

**Not live on this host:** `profile.yaml` currently sets `send_cover_letter: true`, so this cannot fire today. It becomes an active document-fidelity defect the moment the toggle is turned off, which is a one-line profile edit.

**Re-scored 2026-08-06 (`/groom_backlogs`, the pass after it was filed):** filed the same day at `1.5 = 3×0.5÷1` and `mechanical`; corrected to `1.0 = 2×1.0÷2` and `standard`. Two changes, both against this row's own fix direction rather than against the number: Value drops to 2 because the defect is latent rather than live, and Effort rises to 2 because the fix is not a one-line path swap — it needs a "not applicable" state distinguishable from a load failure, threaded through `GetAssistedDocument`, the queue projection's `cover_letter_ready`, and `cmd/assist`'s manual-review fallback. Decay stays 1.0 per this file's rule that a defect's cost does not shrink because sibling defects were fixed; the 0.5 it was filed with was borrowed from `improvements.md`'s theme-decay rule, which bugs are explicitly exempt from.

Deliberately left out of #525's scope: fixing it means teaching the assisted path to represent "no cover letter" as a distinct state from "the cover letter failed to load". Today `cmd/assist` treats any `GetAssistedDocument` error as a reason to fall back to manual review (`cmd/assist/main.go:607-616`), which would turn a correctly-configured profile into a degraded handoff on every job.

**Fix direction.** Extend `config.Profile` with the same single-source resolution #525 introduced — a method answering "does this profile send a cover letter at all" is already there in `ShouldSendCoverLetter`. Have `GetAssistedDocument` report a not-applicable cover letter distinctly from a failed one, have `GetAssistedQueue` set `cover_letter_ready` false without implying an error, and have `cmd/assist` pass an empty cover path to `FillAssistedMappedPage` rather than entering manual review. Check what the fill handlers already do with an empty `coverPath`, since the automatic path passes exactly that today and works.

### 518. A revalidated, already-submitted application cannot be confirmed from the dashboard

Closed — full account archived in `documentation/backlog_history/bugs_done_details.md`.

### 517. Assisted Apply serves 404 for every cover letter once documents move to needs_manual_apply

Closed — full account archived in `documentation/backlog_history/bugs_done_details.md`.

### 516. Discovery has no geographic gate, so an India-only role reached a live application attempt

Closed — full account archived in `documentation/backlog_history/bugs_done_details.md`.

### 515. Assisted Apply uploaded the saved reference note in place of the résumé

Closed — full account archived in `documentation/backlog_history/bugs_done_details.md`.

