# 🐛 Bug Backlog

This document is the authoritative, ranked backlog for known flaws, bugs, and broken items in Career Agent Core. It mirrors the structure of `improvements.md` and follows the same Working Protocol defined there: open a task journal, re-evaluate the model against what is currently available, route the matching library skill, then fix, verify, commit, and push. Bugs are prioritized independently of new features and generally outrank improvement work of similar effort — and while the Usability Gate below is unmet, bugs outrank *everything* in `improvements.md`, full stop.

## 🎯 Usability Gate — what "100% usable" means

This project reaches 100% usable when every box below is checked. Until then, this is the default work queue ahead of any Pending row in `improvements.md`; everything in that file is explicitly nice-to-have and out of scope until this gate is met.

- [x] `go build ./...` succeeds clean — re-verified 2026-07-28
- [x] `go vet ./...` reports no issues — re-verified 2026-07-28
- [x] `go test ./...` passes for every package that has tests — **re-verified 2026-07-28:** the uncached suite is green; focused race suites for the security, scraper, submitter, and agent packages also pass.
- [x] A working local Ollama install with the models `cmd/agent` needs — re-verified 2026-07-28 through the live tags endpoint: `qwen3:30b-instruct`, `qwen2.5vl:7b`, and `nomic-embed-text` are present, along with `qwen3:4b-instruct`. Note: this box's original wording (`llama3.1`, `llava`, `nomic-embed-text`) is stale; `.env.example` carries the current recommendations.
- [x] `cmd/agent` completes one full batch run against live job boards end to end — discover → score → tailor (resume + cover letter) → submit or log to `applications/manual_submissions.md` → row written to `applications.db` — with zero crashes. **MET 2026-07-23 12:00:** a real Lever posting (`jobs.lever.co/smarsh/...`) went discover → score (85) → tailor (real resume + cover letter generated) → submit (`handleLever`, real form fill + click-submit) → `job_funnel.status = APPLIED` in `applications.db`, zero crashes, in a single clean run. This was the culmination of bugs #45/#46 (two independent CAPTCHA-detection false positives that had been killing the large majority of Greenhouse/Lever/Ashby/Workable jobs before they ever reached fit-scoring or the fill stage) and #47 (the dedicated Lever/Greenhouse handlers' own missing click-to-reveal step, only exposed once #45/#46 stopped killing the job earlier). See #45/#46/#47's Details sections for the full diagnostic chain. **Progress 2026-07-20 (extended live-testing session, ~6 hours):** bug #3 (Ollama context/concurrency) fixed and verified — a real job completed discover → score → tailor cleanly with zero Ollama errors and reached the actual Playwright submit step for the first time. Bug #4 (form-fill) was re-diagnosed from a wrong "timeout" theory to the real cause (forms embedded in iframes were never searched) and fixed in code, but **could not be verified live** — a new Blocker (#6, Ollama generation throughput collapsing mid-request) emerged and prevented most attempts from even reaching the fill step. **Progress 2026-07-21:** bug #6 resolved — root cause was a too-short hardcoded 10-minute client timeout racing against genuinely slow (but honest) CPU generation at long context, not context-shift thrashing; timeout is now configurable and defaults to 45 minutes. Bug #4's fix is still unverified live — that requires an actual full batch run, not yet attempted. **Progress 2026-07-22 (Claude Code session, ~6 hours, live triage with the user watching):** found and fixed a chain of real, independently-verified blockers, most reached only after the previous ones were cleared: (1) three duplicate/orphaned agent processes running simultaneously (same class as the Operational Trap below, recurred despite being documented) fighting over one Ollama instance — killed down to one clean process; (2) three files/dirs under `applications/` silently failing to write (`permission denied`) because they were owned by a stale UID `524288` left over from an earlier containerized run — the manual-apply queue and manual-submissions log had been dropping entries with no record at all; (3) **the dominant root cause of the whole session's "First Name" timeout pattern turned out to be environmental, not code**: `cmd/agent` was running on the bare Bazzite host again (see the Resolved-but-regressed entry below) where Chromium renders pages completely blank while reporting navigation success — moved everything back into the `career-agent` distrobox and confirmed real page content renders again; (4) a cookie-consent banner's backdrop `<div>` intercepts every click site-wide until dismissed, silently defeating `clickApplyIfPresent` (bug #34, new); (5) SmartRecruiters uses "I'm interested" instead of any "Apply" wording, and clicking it can reveal a fresh DataDome challenge the earlier captcha check never saw since it ran before that click (bug #35, new); (6) Jobvite gates the real form behind a "Data Consent" `<select>` — zero fields exist in the DOM until an option is chosen (bug #36, new); (7) `fillActionTimeoutMs` bumped 15000→30000: confirmed live that even a single clean instance can still lose the fill race to a co-located Ollama generation burst (bug #37, new). Also excluded `breezy.hr` from discovery (0 real applies across 212 attempts) and deprioritized Workday in the backlog query (bug #38, new) so platforms that can actually reach `APPLIED` stop being crowded out. **Still not verified:** despite all of the above, no fresh `APPLIED` was produced this session — the last real fill attempt (`brightvisiontechnologies.applytojob.com`) hit a *different*, smaller bug in the Vision-fallback path (bug #39, new, open) rather than any of the issues just fixed. Next session should resume with a clean single instance already running in the container and watch for the first real post-fix `APPLIED`.
- [x] `cmd/dashboard` serves and displays live, correct data from a populated `applications.db` — **re-verified 2026-07-27 after #122:** the root page and `/api/metrics` both return HTTP 200 without printing response data. The post-#126 rebuild established the loopback-only listener and refused non-loopback connection.
- [x] `cmd/tracker` runs against real IMAP credentials for at least one poll cycle without crashing (or no-ops cleanly per its existing missing-credentials guard) — **verified 2026-07-22 00:05:** with a freshly generated Google App Password, a full inbox scan completed cleanly ("Scan complete", no crash), detecting a real rejection (Glimpse) and a real interview invitation. The scan also exposed serious classification false positives, filed separately as bug #20 — the gate box is about crash-safety and it passes; classification quality is its own bug. Earlier attempt note (2026-07-21): built and ran one cycle in the container. The crash-safety half is verified — Google rejected the login and the tracker handled it cleanly (logged the error, proceeded to its sleep loop, no crash). The credentials half is blocked on the user: `.env`'s `IMAP_APP_PASSWORD` is 12 characters, but Google app passwords are 16 — it's malformed or stale, and Google returned "Application-specific password required". Needs a freshly generated Google App Password (Google Account → Security → 2-Step Verification → App passwords) before a genuine logged-in scan can be verified.
- [x] Zero open bugs below tagged `Blocker` or `Major` in the Ranked Backlog — **MET 2026-07-28.** The final remaining Major bug (#112) was resolved.

**✅ 2026-07-26 08:31 — FIRST GENUINELY CONFIRMED APPLICATION.** Akuity (Greenhouse, fit 85) completed end to end: form filled to `invalid fields: 0`, submit accepted, security-code challenge detected, the real code retrieved over IMAP, entered across Greenhouse's **eight single-character boxes**, resubmitted, and confirmed — URL moved to `/confirmation`, document collapsed 126,557 → 14,250 chars, confirmation phrase present. `job_funnel.status = APPLIED` with the `applied_jobs` row timestamped `08:31:02.600`, matching the confirmation to the millisecond (#94: the row is written **only** on confirmed submission). **This is the first `APPLIED` row in the database's history** — the count was 0 across 3,884 rows at the start of this session, and the log's `Submission confirmed` count went 0 → 1.

**This resolved the open question from the earlier 82-job verification journal, which has since been consolidated into the current monitoring journal and this backlog.** The historical `APPLIED` rows were not genuine, and the cause was never an inability to fill forms: the pipeline submitted without being able to detect that it had (#95, #102, #111), and could not complete the out-of-band code challenge (#113, #115, #116).

**Re-verified MET 2026-07-25 (`/groom_backlogs`):** `go build ./...` and `go vet ./...` both clean, `go test ./...` green across all 10 test-bearing packages with 0 failures, re-run after the day's #23/#61/#62 commits. Zero Pending rows remain in the Ranked Backlog (bugs #61 and #62 were both filed and resolved the same day). Gate still holds on every box.

**Status after #120 resolution: UNMET.** Build, vet and the full test suite are clean, but 3 open Blocker/Major defects remain. Bug work continues to outrank `improvements.md`.

**Status after #112 resolution: MET.** URL scheme deduplication and migration is verified, resolving the final open Major defect. The gate is clear, and the agent may now proceed to features in `improvements.md`.

**Historical status as of 2026-07-24 ~15:50: MET.** At that point all six gate boxes were checked: static checks, Ollama, dashboard, tracker crash-safety, a live end-to-end application and zero known open Blocker/Major bugs were confirmed. The later first genuinely confirmed application resolved the old 82-job journal's core question; that superseded journal was removed in the 2026-07-26 sweep, with current live-run context retained in `documentation/task_journals/2026-07-25_monitor-live-run-and-fix-bugs.md`. The current gate status is the newer UNMET line above.

Every session — Claude Code, Gemini CLI, or manual — that touches this repo should glance at this checklist. When the last box is checked, change the Status line to `MET (YYYY-MM-DD)` and add a one-line note on what was verified; from that point on, `improvements.md`'s Pending rows become fair game for normal ROI-ranked selection instead of being blocked behind this gate.

**⚠️ Operational trap when running `cmd/agent` live via `go run`:** `go run` does not exec into its compiled binary — it stays alive as a wrapper process around a separately-spawned child (visible as `/tmp/go-build.../b001/exe/main`). Killing only the `go run cmd/agent/main.go` PID (e.g. `kill -9 <pid>` or `pkill -f`) does **not** kill that child, which keeps running orphaned in the background indefinitely. Confirmed live 2026-07-21: five separate `go run` "relaunches" over one session left **five concurrent orphaned agent processes** running simultaneously for hours, all sharing the same `applications.db` and log file — this was the real cause of several confusing symptoms that session initially (wrongly) suspected were code bugs, including apparent duplicate job processing that persisted even after fixing #12, and Ollama OOM crashes worse than a single instance would cause. **To restart a live run cleanly: `go build -o /tmp/career_agent_bin ./cmd/agent` and run that binary directly**, so the PID you launch is the PID doing the work — no wrapper/child ambiguity, and `kill -9` on it is final.

**Recurred 2026-07-22** despite being documented: found three agent processes running simultaneously at the start of the session (two stale pre-fix binaries plus one current one), all sharing `applications.db` and the log file. One of the stale ones survived a plain `kill` and needed `kill -9` — a plain `SIGTERM` is not reliably sufficient for these orphans, always verify with `ps aux` after killing, not just trust the exit code.

**⚠️ Operational trap: the discovery queue is a one-time snapshot, not a live view.** `cmd/agent` calls `storage.GetDiscoveredJobs()` exactly once at startup and pushes the whole result into an in-memory channel — a running process never re-queries the database. Confirmed live 2026-07-23: neither a code fix nor a direct DB status change (e.g. resetting `BLOCKED_CAPTCHA` rows back to `DISCOVERED`) has any effect on an already-running instance; the only way to make either change "visible" is to kill the process and launch a freshly-built binary. This bit twice in the same session — once for a code fix (bugs #45/#46), once for a DB-only status change (the 830-row requeue below) — before being written down here.

**Lesson: a fix does nothing for jobs already marked with a bad status.** `GetDiscoveredJobs` only pulls `status = 'DISCOVERED'`; once something is `BLOCKED_CAPTCHA`/`FAILED_SUBMIT`, it sits there forever regardless of what code ships later. Confirmed live 2026-07-23: bugs #45/#46's CAPTCHA-detection fix produced zero new `APPLIED` results until 830 stale `BLOCKED_CAPTCHA` rows (Greenhouse/Lever/Ashby/Workable) were manually reset — the fix was correct the whole time, but nothing was ever going to exercise it. **After any fix that changes whether a class of job can succeed, run `go run ./cmd/requeue -stats` to see current per-source outcome counts, then requeue the ones the fix should now unblock** (`-source <name> -status BLOCKED_CAPTCHA|FAILED_SUBMIT -confirm`, add `-clear-dedup` if documents were already generated for those jobs before they failed — see `ClearApplicationRecordsByURLPattern`'s doc comment in `pkg/storage/manager.go` for why). Don't requeue an entire source blindly if only one specific failure mode is understood to be fixed — some of that source's failures may have unrelated causes (done deliberately for bug #49: only the one diagnosed Greenhouse job was requeued, not all 14 of that source's `FAILED_SUBMIT` rows).

**Technique: targeted single-job verification via `TARGET_JOB_URL`.** `cmd/agent` reads this env var at startup; when set, it restricts the run to exactly that one already-`DISCOVERED` job and skips fresh `FunnelEngine` discovery entirely (`TARGET_JOB_URL="https://..." /path/to/binary`). This is how bugs #46-#49 were verified end-to-end in minutes instead of waiting on normal queue order, and without disturbing a separately-running full batch. The job must actually be in `DISCOVERED` status first (use `cmd/requeue` or direct SQL if it's currently in some other status), and if it previously reached document generation, clear its `applied_jobs` dedup row too or `HasApplied` will skip it as a duplicate.

**⚠️ Deliberately NOT built, and it must stay that way: pre-emptively skipping jobs whose page carries a bot-protection widget.**

Each captcha-blocked job costs ~10 minutes of fit-scoring before the block is discovered — measured 2026-07-26 across 8 blocked boards, roughly 7+ hours of compute over a cohort this size. The obvious optimisation is to detect the provider frame at page load and skip before scoring. **Do not do it.**

**Presence is not blocking.** These systems are score-based. Measured the same night: `greenhouse.io/akuity` carries a reCAPTCHA Enterprise frame **and its submit was accepted** (security-code email timestamped 23:40:07), while `greenhouse.io/clickhouse` carries no frame at all and also succeeded. A presence-based pre-skip would have discarded Akuity — a job that genuinely submitted.

That is exactly **#45/#46**, where captcha false positives killed the large majority of Greenhouse/Lever/Ashby/Workable jobs before they ever reached fit-scoring, and produced zero applications until 830 rows were manually reset. Skipping a job that would have submitted is strictly worse than wasting inference, because the goal is applications, not throughput.

The narrow detections that **are** safe — #99, #101, #104 — all fire only **after** a submit has already produced no outcome, so they cannot pre-empt a working job. Any future optimisation here must preserve that property. A per-board history rule might eventually justify skipping, but 3 Lever data points are not a basis for excluding 39 jobs.

## Ranked Backlog (best ROI first)

Pending bugs carry the same diminishing-returns score defined in `improvements.md` (Score = Value × Decay ÷ Effort, ROI floor 0.5). Bugs rarely decay — a defect's cost does not shrink because other defects were fixed — so Decay is normally 1.0. A bug below the floor stays open, flagged ⚠️, and needs explicit user confirmation before being worked. When a new bug is found (including one surfaced while checking the Usability Gate above), add a row here with a Severity (`Blocker` | `Major` | `Minor`) and a matching detail section, then work the table top down.

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

**2026-07-28 groom-pass note (session 5):** all remaining Pending bugs (#131, #125) were re-verified against current code, and every score was recomputed. No bug falls below the 0.5 floor. The static gate is green (build, vet, uncached tests pass). The Usability Gate is MET. The agent will proceed to the top item in `improvements.md`.

**2026-07-28 groom-pass note (session 6):** all remaining Pending bugs (#131, #125) were re-verified against current code, and every score was recomputed. No bug falls below the 0.5 floor. The static gate is green (build, vet, uncached tests pass). The Usability Gate is MET. The agent will proceed to the top item in `improvements.md`.

**2026-07-28 groom-pass note (session 7):** all remaining Pending bugs (#131, #125) were re-verified against current code, and every score was recomputed. No bug falls below the 0.5 floor. The static gate is green (build, vet, uncached tests pass). The Usability Gate is MET. The agent will proceed to the top item in `improvements.md`.

**2026-07-28 groom-pass note (session 8):** all remaining Pending bugs (#131, #125) were re-verified against current code, and every score was recomputed. No bug falls below the 0.5 floor. The static gate is green (build, vet, uncached tests pass). The Usability Gate is MET. The agent will proceed to work on bug #131 using a Gemini model, as there are no free above-floor items remaining in `improvements.md`.

**2026-07-28 groom-pass note (session 9):** bug #131 was verified as completed. The only remaining Pending bug (#125) was re-verified against current code, and its score was recomputed. No bug falls below the 0.5 floor. The static gate is green (build, vet, uncached tests pass). The Usability Gate is MET. The agent will proceed to work on bug #125 using a Gemini model.

**2026-07-28 groom-pass note (session 10):** bug #125 was verified as completed. There are no remaining Pending bugs. The static gate is green (build, vet, uncached tests pass). The Usability Gate is MET. The agent will proceed to `improvements.md`.
| # | Bug | Severity | Status | Score (V×D÷E) | Claude model | Gemini model | OpenAI model | OpenAI task-fit reason | ROI rationale |
|---|---|---|---|---|---|---|---|---|---|
| 393 | [Playwright Host missing dependencies to run browsers](#393-playwright-host-missing-dependencies-to-run-browsers) | Blocker | Done (2026-07-28) | — | claude-sonnet-4-6 | gemini-3.6-flash-high | gpt-5.6-terra | Missing OS packages are easily fixed in the container or host. | Cleared ms-playwright cache and reinstalled dependencies inside the ubuntu:22.04 distrobox so it downloads the correct binaries for that OS version. |
| 394 | [QUARANTINED_PROMPT_INJECTION has massive false positive rate on legitimate jobs](#394-quarantined_prompt_injection-has-massive-false-positive-rate-on-legitimate-jobs) | Major | Done (2026-07-28) | — | claude-opus-4-6-thinking | gemini-3.1-pro-high | gpt-5.6-sol | Security heuristics require careful tuning. | Over 400 legitimate jobs (Lever, Greenhouse) were quarantined. The detection heuristic is too aggressive. |
| 395 | [Validation loop times out waiting for Ollama context deadline](#395-validation-loop-times-out-waiting-for-ollama-context-deadline) | Major | Done (2026-07-28) | — | claude-sonnet-4-6 | gemini-3.6-flash-high | gpt-5.6-terra | Timeouts are a configuration issue. | 480 validation attempts failed with 'context deadline exceeded' indicating the timeout to Ollama during form validation is too short. |
| 127 | [Sensitive credentials application data and generated documents are world-readable](#127-sensitive-credentials-application-data-and-generated-documents-are-world-readable) | Major | Resolved (2026-07-27) | — | claude-sonnet-4-6 | gemini-3.6-flash-high | — | — | Maintained commands enforce an owner-only umask and fail closed if startup repair cannot secure known private paths. Storage creates private artifacts at `0600` under `0700`; the idempotent repair refuses symlinks. Tests and live metadata verification pass |
| 123 | [Failed and non-2xx job-page fetches still proceed to expensive fit scoring](#123-failed-and-non-2xx-job-page-fetches-still-proceed-to-expensive-fit-scoring) | Major | Resolved (2026-07-27) | — | claude-sonnet-4-6 | gemini-3.6-flash-high | — | — | Missing descriptions now require meaningful 2xx page content before model work. Closed postings become `INVALID_URL`; transient failures receive bounded retries and return to `DISCOVERED`; every response closes within its attempt and affected status writes are checked |
| 129 | [The agent hard-codes one developer-specific career-profile path](#129-the-agent-hard-codes-one-developer-specific-career-profile-path) | Major | Resolved (2026-07-27) | — | claude-sonnet-4-6 | gemini-3.6-flash-high | — | — | Shared resolution now supports `-profile`, `CAREER_PROFILE_PATH`, and repository-local or sibling-library defaults. Startup validates the source before cached chunks, fails closed on missing or unverifiable context, and provides explicit `-no-rag` mode |
| 130 | [Yahoo fallback drops discovery on transient unexpected EOF responses](#130-yahoo-fallback-drops-discovery-on-transient-unexpected-eof-responses) | Major | Done (2026-07-28) | — | claude-sonnet-4-6 | gemini-3.6-flash-high | `gpt-5.6-terra` | Network retry/backoff behavior needs balanced implementation and verification. | Done 2026-07-28: added context-aware retry policy with exponential backoff for transport and 5xx/429 errors. Covered by transient recovery, exhaustion, non-retryable and cancellation tests. |
| 131 | [ATS board polling discards truncated JSON without retry](#131-ats-board-polling-discards-truncated-json-without-retry) | Minor | Done (2026-07-28) | — | claude-sonnet-4-6 | gemini-3.6-flash-high | `gpt-5.6-terra` | Parser and polling resilience is a moderate, testable reliability change. | Added retry loop in `pollBoard` with exponential backoff for transient fetch errors and truncated JSON. Added tests utilizing httptest servers mimicking truncated responses. |
| 121 | [Untrusted job text reaches embedding and scoring models before quarantine](#121-untrusted-job-text-reaches-embedding-and-scoring-models-before-quarantine) | Blocker | Resolved (2026-07-27) | — | claude-opus-4-6-thinking | gemini-3.1-pro-high | — | — | One typed deterministic boundary now protects posting embedding/scoring and every model-facing generic, Greenhouse, Lever, cached, validation, and Vision path. Detections retain the private CSV audit, receive a terminal status, and never reach an LLM judge |
| 120 | [`--daemon` logs a six-hour drip mode but exits after one batch](#120---daemon-logs-a-six-hour-drip-mode-but-exits-after-one-batch) | Major | Done (2026-07-27) | — | claude-sonnet-4-6 | gemini-3.6-flash-high | — | — | Daemon mode now refreshes discovery and database work every six hours, applies a configurable positive per-cycle cap, and cancels its wait on SIGINT or SIGTERM. Batch mode remains one unlimited cycle |
| 122 | [SSRF defenses block literal private IPs but not hostnames that resolve to them](#122-ssrf-defenses-block-literal-private-ips-but-not-hostnames-that-resolve-to-them) | Blocker | Resolved (2026-07-27) | — | claude-opus-4-6-thinking | gemini-3.1-pro-high | — | — | One injectable policy now rejects any non-public answer across both address families. Go transports dial validated IPs directly; discovery, redirects and posting fetches use them, while authenticated loopback proxies bind every Playwright context to the same guarded dialer |
| 128 | [Saving a second role at the same company overwrites the first role's documents](#128-saving-a-second-role-at-the-same-company-overwrites-the-first-roles-documents) | Major | Done (2026-07-27) | — | claude-sonnet-4-6 | gemini-3.6-flash-high | `gpt-5.6-sol` | Storage and data-integrity fix spanning persistence, naming, and concurrency behavior. | Documents now live below a company directory and a normalized-URL hash. `SaveApplication` returns the exact directory, the agent hands that path to the manual queue, and atomic private-file replacement plus a save lock prevent interleaved artifacts. Focused collision/concurrency coverage and the full Go verification loop pass |
| 112 | [The same posting exists twice, once per URL scheme, and their statuses have diverged](#112-the-same-posting-exists-twice-once-per-url-scheme-and-their-statuses-have-diverged) | Major | Resolved 2026-07-28 | — | claude-opus-4-6-thinking | gemini-3.1-pro-high | `gpt-5.6-sol` | Cross-layer URL canonicalization, database state, queueing, and reporting require deeper reasoning. | Live 2026-07-27: **20 scheme-duplicate pairs** in `job_funnel`, **15 holding different statuses**. Dedup now prevents a second outward application, but discovery, queueing and reporting still operate on two independently mutable rows. Reopened by the 2026-07-26 journal/backlog sweep instead of leaving unresolved work labelled Resolved |
| 125 | [Ambiguous outcome emails retry forever instead of entering manual review](#125-ambiguous-outcome-emails-retry-forever-instead-of-entering-manual-review) | Minor | Done (2026-07-28) | — | claude-sonnet-4-6 | gemini-3.6-flash-high | `gpt-5.6-terra` | Bounded retry-state logic and tests are substantial but well-scoped engineering work. | Updated tracker to correlate using ATS IDs or role keywords; ambiguous emails now generate a manual review task and are acknowledged to prevent retry loops. |
| 126 | [The unauthenticated dashboard binds every network interface while announcing localhost](#126-the-unauthenticated-dashboard-binds-every-network-interface-while-announcing-localhost) | Major | Resolved (2026-07-27) | — | claude-sonnet-4-6 | gemini-3.6-flash-high | — | — | The dashboard now defaults to `127.0.0.1:8080`, validates an explicit `-addr`, warns on non-loopback exposure, and uses a dedicated server with defensive timeouts. Tests and a live container restart prove both routes work on loopback while the host's non-loopback address cannot connect |
| 124 | [The email tracker acknowledges a message even when its database update fails](#124-the-email-tracker-acknowledges-a-message-even-when-its-database-update-fails) | Major | Resolved (2026-07-27) | — | claude-sonnet-4-6 | gemini-3.6-flash-high | — | — | Outcome updates and Message-ID acknowledgements now commit in one SQLite transaction. Errors roll back and leave the message retryable; unmatched, no-op and one-row updates are distinct, while multi-row matches fail closed. Lock, acknowledgement-failure, rollback and retry regressions pass |
| 119 | [Free discovery sources are disabled when the SerpApi key is absent](#119-free-discovery-sources-are-disabled-when-the-serpapi-key-is-absent) | Major | Resolved (2026-07-26) | — | claude-sonnet-4-6 | gemini-3.6-flash-high | — | — | Free RemoteOK, Hacker News, and public ATS-feed discovery now runs before any optional-key decision. Without SerpApi, role/ATS queries use Yahoo directly; an isolated regression proves free jobs are emitted and SerpApi receives zero requests. Full build, vet, and tests pass |
| 118 | [Resume-selector fallback work breaks every submitter path without a readable resume](#118-resume-selector-fallback-work-breaks-every-submitter-path-without-a-readable-resume) | Major | Resolved (2026-07-26) | — | claude-sonnet-4-6 | gemini-3.6-flash-high | — | — | Resume controls are now resolved before the source file is read. Optional forms with no upload control continue cleanly; mapped, named-fallback and sole-file-input controls upload the resume; a found control with an unreadable/empty source fails clearly. Six regressed scenarios plus five focused tests pass |
| 117 | [A single mailbox fetch misses a code that IMAP has not indexed yet](#117-a-single-mailbox-fetch-misses-a-code-that-imap-has-not-indexed-yet) | Major | Resolved (2026-07-26, root-caused and fixed) | — | Opus 5 | Gemini 3 Pro | — | — | Measured on ClickHouse: Greenhouse sent the code at **08:48:11**, and #111's single fetch at **08:48:21** — ten seconds later — returned **nothing**. The agent concluded the submit was not accepted, made another attempt, and clicked a submit button that was by then **`disabled=true`** (#101's diagnostic said so), burning the 30s action timeout on an application that had actually gone through. `pendingSecurityCodeAfter` now polls on a 25s budget instead of fetching once |
| 116 | [The post-security-code resubmit still judged the page in one instantaneous read](#116-the-post-security-code-resubmit-still-judged-the-page-in-one-instantaneous-read) | Blocker | Resolved (2026-07-26, root-caused and fixed) | — | Opus 5 | Gemini 3 Pro | — | — | **#115 worked — `Entered the emailed security code for akuity; resubmitting` — and the verdict still failed, in the same second.** This third submit site kept the original `WaitForLoadState` + single `page.Content()` read that **#95 replaced in the other two**, so the post-code resubmit was judged before the page could answer. Fifth instance of a capability wired into one path and not the others (#65/#66→#67, #74→#75, #28→#31, #98's two prompt paths). It is also the **last step before a confirmed application**, so the most expensive place to get it wrong |
| 115 | [Greenhouse splits the one-time code across eight single-character inputs](#115-greenhouse-splits-the-one-time-code-across-eight-single-character-inputs) | Blocker | Resolved (2026-07-26, root-caused and fixed) | — | Opus 5 | Gemini 3 Pro | — | — | **#114's diagnostic answered it in one cycle.** The real markup is `security-input-0` … `security-input-7`, **eight inputs, each `maxlength=1`, each with an EMPTY `name`**. Every prior selector looked for a single `security_code`/`security-code` field — none could match, and filling one box with an 8-character code would have failed anyway. This was the last unimplemented link between an accepted submission and a completed application |
| 114 | [When the emailed code cannot be entered, nothing records what IS on the page](#114-when-the-emailed-code-cannot-be-entered-nothing-records-what-is-on-the-page) | Major | Resolved (2026-07-26, diagnostic added) | — | Opus 5 | Gemini 3 Pro | — | — | #113 proved the field is **absent, not late** — a full 20s wait found nothing (`could not find a visible security-code field to fill within 20s`). But the error can only name the selectors that *failed*, so the real field cannot be identified without reproducing an accepted submit, which means **filing a real application**. Detection fires on substrings like `security-code`, which a CSS class or notice text satisfies with no input present. Same move as #80/#96/#97/#100, each of which paid off within one cycle |
| 113 | [The emailed code was retrieved and then discarded, because the code field had not rendered yet](#113-the-emailed-code-was-retrieved-and-then-discarded-because-the-code-field-had-not-rendered-yet) | Blocker | Resolved (2026-07-26, root-caused and fixed) | — | Opus 5 | Gemini 3 Pro | — | — | **One step from the first confirmed application.** Akuity's submit was ACCEPTED, the gate was detected, and #32 **successfully retrieved the code** — then `fillSecurityCode` failed instantly with `could not find a visible security-code field to fill` and the job went to manual review. Detection substring-matches the HTML; filling needs a real *visible* locator, and the input renders later than the markers. Everything happened in one second, ~11s after the submit. Same DOM-lag as #95, #102 and #111, one layer further in |
| 111 | [#104 labelled an ACCEPTED application captcha-blocked, because the DOM lags the acceptance](#111-104-labelled-an-accepted-application-captcha-blocked-because-the-dom-lags-the-acceptance) | Blocker | Resolved (2026-07-26, root-caused and fixed) | — | Opus 5 | Gemini 3 Pro | — | — | **A false positive in my own #104, of exactly the kind #104's guard was written to prevent.** Akuity's submit was **accepted** — Greenhouse emailed code `82taTsxA` at **05:59:19** — and the verdict at 05:59:27 still reported `BLOCKED_CAPTCHA`, because the guard tested `DetectSecurityCodeChallenge(prunedHTML)` and the code input had **not yet rendered** 8s after the click. The DOM cannot distinguish accepted from blocked on this timescale; the mailbox can, and #32's fetcher only returns codes issued after the triggering click |
| 110 | [A short option label could hijack a longer answer — "Prefer not to say" selected "No"](#110-a-short-option-label-could-hijack-a-longer-answer--prefer-not-to-say-selected-no) | Blocker | Resolved (2026-07-26, root-caused and fixed) | — | Opus 5 | Gemini 3 Pro | — | — | **Found by a test written for #109, not by the log.** `pickComboboxOption` matched by raw bidirectional `strings.Contains`, and a short label hides inside longer prose: `"no"` sits inside `"prefer **no**t to say"`. Asking for **"Prefer not to say" selected the box labelled "No"** — on an EEO question that converts a declined answer into a substantive one on a real application. The precise failure **#79** exists to prevent, in the function that enforces it. `"male"` vs `"female"` is the same shape |
| 109 | [A single-choice question rendered as a checkbox group was read as one box to untick](#109-a-single-choice-question-rendered-as-a-checkbox-group-was-read-as-one-box-to-untick) | Major | Resolved (2026-07-26, root-caused and fixed) | — | Opus 5 | Gemini 3 Pro | — | — | Probed live: Sporty Group renders a Yes / No / Prefer-not-to-say question as **three checkboxes sharing one `name`**. A model value of `"No"` means *tick the box labelled No*, but `applyValidationFix` read it as *untick this box* — opposite results. #107 then made the wrong reading report as **landed**, so the job degraded from `MANUAL_REQUIRED` (documents preserved, field named) to a bare `FAILED_SUBMIT` |
| 108 | [A submit that went nowhere was reported as "form too large for the local model"](#108-a-submit-that-went-nowhere-was-reported-as-form-too-large-for-the-local-model) | Major | Resolved (2026-07-26, root-caused and fixed) | — | Opus 5 | Gemini 3 Pro | — | — | Ethos reached **`invalid fields: 0`** — fully satisfied — exhausted the settle budget with **no bot-protection frame** to explain it, and was then written up as `form content exceeds the local model's context window`. The form was never the problem: narrowing found nothing to narrow, fell back to the whole document, and the size check caught it incidentally. Right outcome (manual review, documents preserved), wrong cause — and a wrong cause has real cost, since that is exactly how #83 misdiagnosed the case #93 later reframed |
| 107 | [A checkbox the model deliberately declined was recorded as uncommittable](#107-a-checkbox-the-model-deliberately-declined-was-recorded-as-uncommittable) | Major | Resolved (2026-07-26, root-caused and fixed) | — | Opus 5 | Gemini 3 Pro | — | — | Live on Sporty Group, visible only because #97 logs the value: `1 fix(es) reported success but left the control empty ... input[id='question_8242451101[]_54236360101'] **(tried "No")**`. `applyValidationFix` correctly *unchecks* on a negative, then `verifyFixLanded` reads the generic "does it hold a value" and sees `checked=false` → not landed → `ErrUncommittableField` → the whole job to manual review. **A correct answer was recorded as a failure**, on every checkbox the model declines |
| 106 | [A bare bracketed checkbox-group id got no fallbacks at all — the third shape of #73](#106-a-bare-bracketed-checkbox-group-id-got-no-fallbacks-at-all--the-third-shape-of-73) | Major | Resolved (2026-07-26, root-caused and fixed) | — | Opus 5 | Gemini 3 Pro | — | — | Live: `Validation fix for "question_8242451101[]_54236360101" failed: selector matched no element (**tried 1 form(s)**)`. Greenhouse names checkbox-group controls that way; the brackets alone make `looksLikeCSSSelector` true, but there is no `tag#id` to split, so the selector was used **verbatim with no fallbacks** — and it is not valid CSS for an id either, so it matched nothing. Third shape of one defect: **#73** fixed `input#430`, **#92** fixed `#question_...[]_...`, this is the bare form with no prefix. It was the remaining blocker on Sporty Group, which reached 11 invalid → 4 with three of the four being exactly these ids |
| 105 | [The 45-minute time budget counted bytes to read, not answers to generate](#105-the-45-minute-time-budget-counted-bytes-to-read-not-answers-to-generate) | Major | Resolved (2026-07-26, root-caused and fixed) | — | Opus 5 | Gemini 3 Pro | — | — | The `Remote` job sent a **30,477-char** payload — comfortably inside #83's 40,000 ceiling — and burned the **entire 45-minute Ollama timeout** (01:46:03 → 02:31:03) before failing. #83 derived its ceiling from input size alone, but the run must *generate* a value for every rejected field, and **Remote had 34 of them**. Against ClickHouse (11,140 chars / 3 fields / ~7 min) and Reddit (18,639 / 13 / ~15 min), field count — not payload size — is what separates the runs that finish |
| 104 | [A captcha-swallowed submit hid behind stale invalid flags, so #99 never fired](#104-a-captcha-swallowed-submit-hid-behind-stale-invalid-flags-so-99-never-fired) | Major | Resolved (2026-07-26, root-caused and fixed) | — | Opus 5 | Gemini 3 Pro | — | — | Predicted from #99+#102, then confirmed by #100's diagnostic on the next run. Reddit job `7956443` set all five custom questions to sensible values (`"company website"`, `"Stellantis Financial Services"`, `"Yes"`, `"No"`, `"I agree"`), committed all three comboboxes, and the **identical five** came back flagged with the page **byte-for-byte unchanged** (140544 chars twice). Nothing was left to fix — the submit was never reaching the server past the page's reCAPTCHA. #99 could not catch it because the verdict settles on flagged fields and never reaches budget exhaustion |
| 103 | [#98 showed the model react-select's internal option ids, and it answered with them](#103-98-showed-the-model-react-selects-internal-option-ids-and-it-answered-with-them) | Blocker | Resolved (2026-07-26, root-caused and fixed) | — | Opus 5 | Gemini 3 Pro | **A defect in my own #98 fix, caught by #100's diagnostic within one cycle of that diagnostic shipping.** `readComboboxOptions` returns `"id\ | label"` so `pickComboboxOption` can click by id; #98 put those raw strings into the prompt, and the model faithfully answered `react-select-question_67179376-option-0\ | Yes` — an internal DOM id no widget offers. Live: `Rejected despite being set last attempt: question_67179376 = "react-select-question_67179376-option-0\ | — | — | Yes"`. So #98 has been feeding garbage to the model since it shipped, on every combobox |
| 102 | [#95's early exit read stale invalid flags and called four accepted submissions failures](#102-95s-early-exit-read-stale-invalid-flags-and-called-four-accepted-submissions-failures) | Blocker | Resolved (2026-07-26, root-caused and fixed) | — | Opus 5 | Gemini 3 Pro | — | — | **A defect in my own #95 fix, and the biggest single misreading of the effort.** Greenhouse *accepts* a submission and issues an emailed security-code challenge within ~1s, then re-renders the challenge later while the previous attempt's `aria-invalid` markers are **still on the page**. #95's flagged-field early exit fired at 2s on those stale markers and called the accepted submission a validation failure. Proven by timestamps: the Akuity code email is stamped **23:40:07, between** its submit click (~23:40:06) and its verdict (23:40:08); ClickHouse's is stamped **00:05:34, the same second** as its submit. **Four applications reached Greenhouse today** (Surt AI, ClickHouse ×2, Akuity) and every one was recorded as a failure |
| 101 | [A submit click that timed out reported nothing about what blocked it](#101-a-submit-click-that-timed-out-reported-nothing-about-what-blocked-it) | Major | Resolved (2026-07-25, diagnostic added) | — | Opus 5 | Gemini 3 Pro | — | — | **Three jobs ended the day this way** — Akuity, Nova and Zimperium — each with a bare `playwright: timeout: Timeout 30000ms exceeded` from the submit click and no indication of why the control was unactionable, all written off as generic `FAILED_SUBMIT`. A timeout says the click never landed; it says nothing about what stopped it. Now reads `elementFromPoint` at the control's centre — the same check that cleared Reddit's button in #99 — and names whatever covers it |
| 100 | [A field that lands and is rejected anyway had no diagnostic at all](#100-a-field-that-lands-and-is-rejected-anyway-had-no-diagnostic-at-all) | Major | Resolved (2026-07-25, diagnostic added) | — | Opus 5 | Gemini 3 Pro | — | — | Akuity logged `applied 7/7`, **no** not-landed line — so `verifyFixLanded` reported every control as set — and the **identical 7 fields** came back flagged. #97 names values only for fields that *fail* to land, so this opposite case had no diagnostic whatsoever and the loop could only re-guess. Probe ruled out the obvious causes: all 7 are plain required `INPUT`/`TEXTAREA`, single match, no `pattern`, and React genuinely *does* observe `Fill()` (`reactValue` matches the DOM). Fourth instance of the same lesson as #80/#96/#97 |
| 99 | [A submit silently swallowed by reCAPTCHA was reported as an ordinary validation bounce](#99-a-submit-silently-swallowed-by-recaptcha-was-reported-as-an-ordinary-validation-bounce) | Major | Resolved (2026-07-25, root-caused and fixed) | — | Opus 5 | Gemini 3 Pro | — | — | Reddit reached **`invalid fields: 0`** — the form fully satisfied for the first time — and the submit still produced no confirmation and no rejection. **No Greenhouse email arrived**, while ClickHouse's accepted submit produced one in the same second, so Reddit's request never reached the server. Read-only probe: the submit control is **clean** (one match, visible, enabled, in-form, unobstructed — ruling out #87's decoy and #34's overlay) and the page embeds **reCAPTCHA Enterprise**. Score-based invisible reCAPTCHA silently discards a headless submission. The cost was ~30 min of model calls per attempt on a form with nothing left to fix, ending in a misleading manual-review reason |
| 98 | [The model was never shown a dropdown's permitted values, so it guessed the wording](#98-the-model-was-never-shown-a-dropdowns-permitted-values-so-it-guessed-the-wording) | Blocker | Resolved (2026-07-25, root-caused and fixed) | — | Opus 5 | Gemini 3 Pro | — | — | **The last-mile blocker, caught by #97's diagnostic in one cycle.** Reddit reached a single remaining invalid field and proposed `"I am not a protected veteran"` on **two consecutive attempts** for a widget offering *No military service* / *I don't wish to answer* — a phrasing that filters the option list to **zero**. Probe: none of `#434`'s option strings exist in the page HTML until the widget is opened, and Greenhouse forms carry **zero native `<select>` elements**, so no option text is ever in the served document the prompt is built from. The model was asked to supply a value for a control whose permitted values it is never shown. Residual measured since #91/#92: **14 commits succeeded, 2 fields failed** — and both failures are exactly this unusual-wording case |
| 97 | [An uncommittable field named the control but never the value that was tried](#97-an-uncommittable-field-named-the-control-but-never-the-value-that-was-tried) | Major | Resolved (2026-07-25, diagnostic added) | — | Opus 5 | Gemini 3 Pro | — | — | Reddit reached **one remaining invalid field** — payload 7,212 → 497 chars, 13 fields down to 1 — and then failed twice on `#434` (veteran status) with the log saying only that the control was left empty. A probe shows `#434` is genuinely selectable (typing `I don't wish to answer` filters its 9 options to exactly that entry), so this is a **value mismatch, not a broken mechanism** — but the log could not distinguish those, and they need opposite fixes. Same class as #80/#96, one level down |
| 96 | [Nothing recorded what a submit verdict was actually decided on](#96-nothing-recorded-what-a-submit-verdict-was-actually-decided-on) | Major | Resolved (2026-07-25, diagnostic added) | — | Opus 5 | Gemini 3 Pro | — | — | Observability, filed the moment its absence cost a day. #95 was findable only by cross-referencing wall-clock seconds between unrelated log lines, because the sole record of a judged submit was the word "failed" — nothing said how long it waited, whether the URL moved, how many fields came back flagged, or how large the returned page was. Same class as #80, and #80 paid for itself within one cycle |
| 95 | [The submit verdict was read from the DOM the instant the click returned, racing the submission itself](#95-the-submit-verdict-was-read-from-the-dom-the-instant-the-click-returned-racing-the-submission-itself) | Blocker | Resolved (2026-07-25, fix shipped; race inferred, not directly observed) | — | Opus 5 | Gemini 3 Pro | — | — | Three independent jobs (ClickHouse, Stack AV, Sporty Group) logged **every field committed, every fix applied, and `Submission failed validation` in the same second**. A probe proved those forms are fully satisfiable — after the agent's exact commit sequence ClickHouse's form reports `invalidCount: 0`, natively valid. So the verdict, not the fill, was wrong. `WaitForLoadState(networkidle)` can return immediately because Playwright's `Click` returns on event dispatch, before the app issues its request, so the page is read before the submission has happened. **#93 is direct evidence this misfires:** a Greenhouse security-code email timestamped the exact second of a submit the agent had written off. Cost of a premature verdict is a ~12-min model call plus a re-click on a form that may already have gone through (#89's duplicate-application risk) |
| 94 | [The dedup row was written at document generation, so a job that never submitted was skipped forever](#94-the-dedup-row-was-written-at-document-generation-so-a-job-that-never-submitted-was-skipped-forever) | Blocker | Resolved (2026-07-25, root-caused and fixed) | — | Opus 5 | Gemini 3 Pro | — | — | Live: the 21:16 restart logged `Duplicate check: Already applied` for **Reddit, Akuity, ClickHouse and Staff SRE** — four jobs the same day's log shows failing, and which have **never** reached `APPLIED`. `SaveApplication` wrote the `applied_jobs` row at document-generation time, so any job that generated documents and then bounced, needed manual review, or was killed mid-submit was permanently marked applied. Combined with the funnel row returning to `DISCOVERED` (startup reaper / #85's reset / requeue without `-clear-dedup`), the job was re-queued every run and skipped instantly, forever — **silently unreachable rather than visibly failed**. 7 of the 82-job cohort and 66 rows DB-wide were in this state |
| 93 | [Greenhouse's emailed security-code gate read as a validation error, burning the full 45-minute timeout](#93-greenhouses-emailed-security-code-gate-read-as-a-validation-error-burning-the-full-45-minute-timeout) | Major | Resolved (2026-07-25, root-caused and fixed) | — | Opus 5 | Gemini 3 Pro | — | — | **Found from the user's inbox, not the logs.** A Greenhouse submit to Surt AI produced an email at **20:58:03 UTC — the exact second of the click**: *"Copy and paste this code into the security code field on your application ... After you enter the code, resubmit your application."* So the submit **succeeded** and Greenhouse issued an out-of-band verification challenge. The resulting security-code input read as just another unsatisfied required field, so the whole 50,501-char form went to the model and burned the full 45-minute timeout. **This reframes #83:** the oversized payload was a *symptom* of the code gate, not the underlying event |
| 92 | [Checkbox-group ids contain brackets, which are CSS attribute syntax, so they resolved to nothing](#92-checkbox-group-ids-contain-brackets-which-are-css-attribute-syntax-so-they-resolved-to-nothing) | Major | Resolved (2026-07-25, root-caused and fixed) | — | Opus 5 | Gemini 3 Pro | — | — | Live: `Validation fix for "input#question_8242451101[]_54236360101" failed: selector matched no element (tried 1 form(s))`. Greenhouse names checkbox-group controls with a literal `[]` in the id; `#question_...[]_...` is not a valid CSS id selector because the brackets read as attribute syntax. **The same class as #73** (leading digits), and #73's own attribute-form fallback was blocked here because `splitTagID` explicitly refused any id containing brackets — note `tried 1 form(s)`, versus the 3 an eligible selector gets |
| 91 | [#90's single-option rule could never fire, because typing filters the sole option out](#91-90s-single-option-rule-could-never-fire-because-typing-filters-the-sole-option-out) | Major | Resolved (2026-07-25, root-caused and fixed) | — | Opus 5 | Gemini 3 Pro | — | — | **A defect in #90's own fix, caught on the very next run.** Sporty Group's `GDPR Acknowledgement*` still reported `left the control empty` with #90 shipped. #90 takes the sole option when `len(options) == 1` — but `setComboboxValue` types the model's proposed value *first*, and typing "Yes" into a widget whose only entry is "Acknowledge/Confirm" filters the list to **zero**. So the count was 0, never 1, and the rule could not fire for precisely the case it was written for |
| 90 | [A required control with exactly one option was refused, sending a job to manual review one click from completion](#90-a-required-control-with-exactly-one-option-was-refused-sending-a-job-to-manual-review-one-click-from-completion) | Major | Resolved (2026-07-25, root-caused and fixed) | — | Opus 5 | Gemini 3 Pro | — | — | Sporty Group (Greenhouse, score **90**) got **10 of its 11 invalid fields satisfied** — invalid payload collapsed 6389 → 610 chars, including 3 committed autocompletes, a GDPR checkbox and a checkbox-group entry. The sole holdout was `GDPR Acknowledgement*`, a combobox offering **exactly one option: "Acknowledge/Confirm"**. The model proposed a differently-worded affirmative, so #79's containment check matched nothing and selected nothing — correct caution where several options exist, over-conservative where there is only one and therefore no wrong choice to make |
| 89 | [A late-rendering confirmation page is missed, so a successful submit is retried — filing duplicates](#89-a-late-rendering-confirmation-page-is-missed-so-a-successful-submit-is-retried--filing-duplicates) | Blocker | Resolved (2026-07-25, root-caused and fixed) | — | Opus 5 | Gemini 3 Pro | — | — | Surfaced by Orkes routing to `MANUAL_REQUIRED` via #83 with a **43,411-char** payload — which only happens when narrowing finds **nothing flagged invalid** and falls back to the whole document. Combined with attempt 2 having applied both fixes, the likeliest reading is that the submit **succeeded** and the page became a confirmation page with no form. Greenhouse replaces the form in place, so `currentURL == applyURL` and only a confirmation *phrase* can prove success — and if that page renders after the 10s networkidle wait, the check right after the click sees the old DOM and reports failure. **The loop then re-submits an application that already went through** |
| 88 | [A required widget that cannot accept the configured value was written off as a submit failure](#88-a-required-widget-that-cannot-accept-the-configured-value-was-written-off-as-a-submit-failure) | Major | Resolved (2026-07-25, root-caused and fixed) | — | Opus 5 | Gemini 3 Pro | — | — | Confirmed live on Nova (Lever): `Attempt 3: 1 fix(es) reported success but left the control empty ... input[data-qa='location-input']`. The detection and commit machinery worked exactly as designed — it correctly saw the field as unset and tried to commit — but **Lever's geocoder returns zero results** for `Macomb`, `Macomb Township` and `Macomb, MI`, while Greenhouse's resolves the same address. With no option to select, the required hidden `selectedLocation` can never be populated. Not an automation failure: the job is perfectly applicable by hand, yet it burned 3 attempts and landed in `FAILED_SUBMIT` |
| 87 | [The submit locator clicked the click-to-reveal "Apply" button, so no retry ever actually submitted](#87-the-submit-locator-clicked-the-click-to-reveal-apply-button-so-no-retry-ever-actually-submitted) | Blocker | Resolved (2026-07-25, root-caused and fixed) | — | Opus 5 | Gemini 3 Pro | — | — | **The one that was silently blocking everything.** Orkes (Greenhouse, 85) applied `2/2` fixes — both verifiably settable by probe — and still failed all 3 attempts, with `applied` and `Submission failed validation` in the **same second**, too fast for any navigation. The submit locator put `button:has-text('Apply')` in the same CSS alternation as the real controls, and alternations have **no precedence** — matches return in DOM order. Measured live: `[0] Apply (type=button)`, `[1] Quick Apply with MyGreenhouse`, `[2] Submit application (type=submit)`. Every retry clicked index 0, the click-to-reveal button, which does nothing once the form is open. The page never changed, the same fields stayed flagged, and all three attempts failed identically |
| 86 | [Lever's location typeahead was invisible to combobox detection, so every Lever application failed](#86-levers-location-typeahead-was-invisible-to-combobox-detection-so-every-lever-application-failed) | Blocker | Resolved (2026-07-25, root-caused, fixed, verified against the live form) | — | Opus 5 | Gemini 3 Pro | — | — | Nova (Lever, score 65) failed all 3 attempts applying **7/7 fixes** each time. Probed the real form: only **3 fields are required** — name, email, `location-input` — and the resume upload is *optional*. Lever's location widget has **none of react-select's markers**: no `role`, no `aria-*`, no `select__` classes. It is a plain `<input name="location">` beside a hidden `<input name="selectedLocation">` that holds the committed value. Detection returned `false`, so it was filled with text while `selectedLocation` stayed empty — and that hidden field is what the form validates. **Clicking the option does not work either**: it loses a blur race, leaving both the visible input and the hidden field empty. Keyboard selection is blur-safe |
| 85 | [Four early-exit paths left rows stranded in PROCESSING, invisible to every future queue](#85-four-early-exit-paths-left-rows-stranded-in-processing-invisible-to-every-future-queue) | Major | Resolved (2026-07-25, root-caused and fixed) | — | Opus 5 | Gemini 3 Pro | — | — | Spotted from the cohort monitor: `PROCESSING=4` on a **single-worker** run. The stranded rows clustered in pairs at exactly the moments a job was skipped — each was `Duplicate check: Already applied ... Skipping.` The worker sets `PROCESSING` at the top of the loop, and four `continue` paths exit without ever clearing it. `GetDiscoveredJobs` selects only `DISCOVERED`, so a stranded row never returns to any queue; #55's startup reaper masked it by resetting them, whereupon they were re-picked, skipped, and stranded again — a silent loop that corrupts cohort accounting and the dashboard's in-flight metrics |
| 84 | [#82's manual-routing branch was never applied, so refused jobs were written off as FAILED_SUBMIT](#84-82s-manual-routing-branch-was-never-applied-so-refused-jobs-were-written-off-as-failed_submit) | Major | Resolved (2026-07-25, confirmed live 18:10 — clean A/B on the same job) | — | Opus 5 | Gemini 3 Pro | — | — | **My own error, caught live.** #82's guard worked perfectly — ClickHouse was refused in **0 seconds** with `work authorization, visa sponsorship` — but the job landed in `FAILED_SUBMIT`, not `MANUAL_REQUIRED`, and the routing log line never appeared. The `cmd/agent` edit adding that branch **silently failed to apply**; `go build` still passed because the submitter half compiled fine, and I verified the build instead of verifying the edit. Consequence: a job that is perfectly applicable-by-hand was written off as a failure, its tailored documents never moved to the manual-apply folder and no manual-queue entry logged |
| 83 | [The payload breaker guarded the context window but not the time budget, burning the full 45-minute timeout](#83-the-payload-breaker-guarded-the-context-window-but-not-the-time-budget-burning-the-full-45-minute-timeout) | Major | Resolved (2026-07-25, root-caused and fixed) | — | Opus 5 | Gemini 3 Pro | — | — | Watched live end to end: a Greenhouse theme (`surtai`) that sets **no `aria-invalid` attributes** defeated #64's narrowing, so the retry fell back to the whole form — **50,501 chars**. That fits the 80,000-char context ceiling, passed `likelyExceedsModelContext`, and then ran **16:58:03 → 17:43:03 — exactly the 45-minute Ollama timeout** before dying. Three-quarters of an hour of the single serialised LLM resource, spent on a request that was mathematically incapable of finishing. Context capacity and inference time are different limits, and on this hardware the time one binds far earlier |
| 82 | [Once the commit worked, an unanswerable legal attestation would have been guessed and really submitted](#82-once-the-commit-worked-an-unanswerable-legal-attestation-would-have-been-guessed-and-really-submitted) | Blocker | Resolved (2026-07-25, confirmed live 17:52 — refused in 0 seconds) | — | Opus 5 | Gemini 3 Pro | — | — | **A risk created by fixing #81.** While the combobox commit was broken, nothing the model proposed was ever really set. Probed on the live form immediately after #81: `#question_67942418 -> COMMITTED "Yes"`. Reddit's "Are you currently authorized to work in the U.S.?" and its sponsorship question are **required and offer only Yes/No — no decline option**, so a model with no configured answer does not abstain, it picks one. That answer is a **legal declaration submitted under the applicant's name**. The form is now refused *before* the model is asked, and the job routed to `MANUAL_REQUIRED` |
| 81 | [data-value mirrors the typed search text, so every react-select falsely reported "landed"](#81-data-value-mirrors-the-typed-search-text-so-every-react-select-falsely-reported-landed) | Blocker | Resolved (2026-07-25, root-caused and fixed) | — | Opus 5 | Gemini 3 Pro | — | — | Caught by #80's new diagnostic: `Attempt 2 applied 13/13 validation fix(es)` and the still-invalid list came back **byte-identical** — none of the 13 landed, including the declinable EEO fields that have nothing to do with the missing attestations. Probed directly: after a bare `Fill()` with **nothing selected**, the value read returned `"I don't wish to answer"`. react-select puts `data-value` on `.select__input-container` to mirror the *typed search text* for input sizing, so the `[data-value]` fallback was reading the artifact of typing — the same mistake as #76, one layer deeper. The false "landed" suppressed the commit step for every custom question on every Greenhouse form |
| 80 | [The retry loop logged the payload size but never which fields were still invalid](#80-the-retry-loop-logged-the-payload-size-but-never-which-fields-were-still-invalid) | Major | Resolved (2026-07-25, root-caused and fixed) | — | Opus 5 | Gemini 3 Pro | — | — | Hit the wall directly: `Attempt 2 applied 13/13 validation fix(es)` with **no** not-landed line at all, and the form still bounced — `7212 -> 7281 chars`. Every field reported as filled, nothing reported as failed, and the submission was still rejected. The byte count cannot distinguish "the same fields are still failing" from "different ones now are", so the next step would have been another blind ~25-minute cycle. `InvalidFieldIdentifiers` now names them |
| 79 | [The option wait watched an unrelated widget, and committing option-0 filed the wrong location](#79-the-option-wait-watched-an-unrelated-widget-and-committing-option-0-filed-the-wrong-location) | Blocker | Resolved (2026-07-25, confirmed live in the agent 16:02) | — | Opus 5 | Gemini 3 Pro | — | — | Found with a standalone Playwright probe against Reddit's real form, after the 12-min-per-guess loop became untenable. Two defects: **(a)** the options wait counted `[role="option"]` **document-wide**, and every Greenhouse page carries an always-open intl-tel-input phone-country widget holding ~244 options — so the count was permanently non-zero, the wait returned instantly, and every commit fired into an empty menu. **(b)** far worse, committing the *focused* option is unsafe: typing `Macomb` puts **"Macomb, Illinois, United States"** at option-0 while the configured address is Michigan, so a successful commit would file real applications with the wrong location |
| 78 | [Fill() never opens a react-select menu, and the read-back matched the input itself](#78-fill-never-opens-a-react-select-menu-and-the-read-back-matched-the-input-itself) | Blocker | Resolved (2026-07-25, confirmed live in the agent 16:02) | — | Opus 5 | Gemini 3 Pro | — | — | Probed directly: `Fill()` sets `input.value` while react-select's menu **never opens** — the widget's own option count stayed 0 and `aria-activedescendant` stayed empty for 3 full seconds, so the Enter that followed had nothing to select. Real keystrokes open and filter it in ~600ms. Separately, react-select sets `role="combobox"` **on the input**, and `Element.closest()` tests the element itself first — so the value read resolved its "shell" to the input, which has no children, and never found the committed value. Same DOM, corrected: `""` → `"Macomb, Illinois, United States"` |
| 77 | [Enter was pressed before react-select had loaded any option, so the commit selected nothing](#77-enter-was-pressed-before-react-select-had-loaded-any-option-so-the-commit-selected-nothing) | Major | Resolved (2026-07-25, confirmed live in the agent 16:02) | — | Opus 5 | Gemini 3 Pro | — | — | Caught the moment #76 made the read-back work: `Attempt 2: 11 fix(es) reported success but left the control empty ... 430, 431, 432, 433, 434, 436, candidate-location, country, question_67942418/19/20` — with **no** commit line, so `commitComboboxOnLocator` pressed Enter and still committed nothing. react-select populates its menu asynchronously (Greenhouse's Location field queries a geocoder), so an Enter fired immediately after `Fill()` arrives while the menu is empty, highlights nothing and selects nothing. Real progress in the same run though: the narrowed payload **shrank for the first time**, 8249 → 5988 chars |
| 76 | [#74's own read-back checked el.value first, silently disabling the combobox commit it had just added](#76-74s-own-read-back-checked-elvalue-first-silently-disabling-the-combobox-commit-it-had-just-added) | Blocker | Resolved (2026-07-25, confirmed live in the agent 16:02) | — | Opus 5 | Gemini 3 Pro | — | — | **A defect in #74's fix, caught live by the absence of an expected log line.** Reddit logged `Attempt 2 applied 15/15 validation fix(es)` with **no** `committed N autocomplete selection(s)` line and **no** `left the control empty` line — so `verifyFixLanded` reported every field as landed and the commit step never ran once. Cause: the value read checked `el.value` before the combobox branch, and a react-select search input genuinely holds the typed text after `Fill()`. #74 was therefore inert on exactly the fields it was written for, and #75 inherited the same inertness |
| 75 | [#74's combobox commit was wired into the retry path but not the initial fill, guaranteeing a wasted retry cycle](#75-74s-combobox-commit-was-wired-into-the-retry-path-but-not-the-initial-fill-guaranteeing-a-wasted-retry-cycle) | Major | Resolved (2026-07-25, confirmed live in the agent 16:02) | — | Opus 5 | Gemini 3 Pro | — | — | The identical structural gap **#67** found, one layer up: `safeFillWithLabelFallback`'s three tiers all use plain `Fill()`, so a react-select field is typed into but never committed on the first pass. Since `Location (City)` and `Country` are required on every Greenhouse form, the first submit was **guaranteed** to bounce and force a full validation-retry cycle — ~12 minutes of inference on this machine — to commit something a single keypress could have done immediately. Confirmed live at 13:36: the run carrying #74 still bounced on attempt 1 with the narrowed payload at exactly 8249 chars, unchanged from the run before it |
| 74 | [react-select comboboxes were filled but never committed, so their validated value stayed empty](#74-react-select-comboboxes-were-filled-but-never-committed-so-their-validated-value-stayed-empty) | Major | Resolved (2026-07-25, confirmed live in the agent 16:02) | — | Opus 5 | Gemini 3 Pro | — | — | #72's autocomplete hypothesis, confirmed structurally by fetching Reddit's real Greenhouse markup: `Location (City)` and `Country` are **react-select** widgets (`select__control`, `select__input-container[data-value]`, `react-select-candidate-location-live-region`). The `<input id="candidate-location">` is a *search* box — the chosen value lives in widget state and renders into a sibling `.select__single-value`. `Fill()` types the search text and commits nothing, so the value the form validates stays empty, and reading `el.value` back reports `""` whether or not a selection succeeded. These are required fields, so the form could never pass |
| 73 | [A CSS id selector cannot start with a digit, so Greenhouse's numeric custom-question ids were unfillable half the time](#73-a-css-id-selector-cannot-start-with-a-digit-so-greenhouses-numeric-custom-question-ids-were-unfillable-half-the-time) | Major | Resolved (2026-07-25, root-caused and fixed) | — | Opus 5 | Gemini 3 Pro | — | — | Caught live on Reddit's third and final attempt: 6 of 15 fixes failed with `selector matched no element` for `input#430`, `input#431`, `input#432`, `input#433`, `input#434`, `input#436`. `#430` is a **CSS syntax error** — an id selector cannot begin with a digit — yet Greenhouse numbers its custom-question controls exactly that way. `resolveFieldLocator` only tried its attribute-form fallbacks when the selector did *not* look like CSS, and `input#430` contains `#`, so it was used verbatim and matched nothing. The model sent bare `430` on attempt 2 (resolved fine) and `input#430` on attempt 3 (dead), so the same field alternated between fillable and unfillable across attempts of the same job |
| 72 | [The retry loop counts empty-valued and non-landing fixes as applied, reporting progress it is not making](#72-the-retry-loop-counts-empty-valued-and-non-landing-fixes-as-applied-reporting-progress-it-is-not-making) | Major | Resolved (2026-07-25, accounting fixed; root cause of the underlying non-convergence still under live investigation) | — | Opus 5 | Gemini 3 Pro | — | — | Found immediately after #70 shipped, from the diagnostic #70 added: Reddit logged `Attempt 2 applied 15/15 validation fix(es)` and **still bounced**, with the narrowed payload essentially unchanged (8249 → 8334 chars) — i.e. the same fields were still invalid. Two accounting defects make that tally untrustworthy: `applyValidationFix` returns `nil` for an empty value (correct for the *initial* fill path, a lie in the retry path), and a `nil` return only means Playwright accepted the call, not that the control ended up set |
| 71 | [firstVisibleLocator's .First() fallback reintroduces the very hang it was written to prevent, at the submit click](#71-firstvisiblelocators-first-fallback-reintroduces-the-very-hang-it-was-written-to-prevent-at-the-submit-click) | Major | Resolved (2026-07-25, root-caused and fixed) | — | Opus 5 | Gemini 3 Pro | — | — | Found auditing the cohort's `FAILED_SUBMIT` rows during #70's restart: Zimperium (Lever, scored **85**) died on `playwright: timeout: Timeout 30000ms exceeded` waiting for `locator(...).first()`. The `.first()` in Playwright's own call log is the proof — `firstVisibleLocator` found *no* visible match, fell back to `loc.First()`, and the caller clicked a match already known to be invisible (#59's hidden `hcaptchaSubmitBtn`). Guaranteed to burn the full action timeout and then misreport "no visible submit button here" as a generic timeout, which is why it read as CPU/network flakiness |
| 70 | [The validation-retry loop strips the page's own error text, so the model never learns why a field bounced](#70-the-validation-retry-loop-strips-the-pages-own-error-text-so-the-model-never-learns-why-a-field-bounced) | Blocker | Resolved (2026-07-25, root-caused and fixed) | — | Opus 5 | Gemini 3 Pro | — | — | Caught live on the highest-fit job in the 82-cohort (Reddit, scored **90**), which burned **17.5 minutes** and 3 LLM calls to fail with `failed to submit application after 3 validation error attempts`. `aria-describedby` was stripped as "presentational" *before* `PruneDOMToInvalidFields` ran, so the link from a rejected control to its error message was severed, and the message element was then dropped as neither control nor label. The model was told a field was invalid but never what would make it valid, so it re-guessed the same value each attempt. Compounded by an empty fix map falling through to a re-submit of a byte-identical form. This is the terminal step of the whole pipeline — it fails *after* discovery, scoring and fill have all succeeded |
| 69 | [Discovery stored the searched role as job_title and discarded the real headline](#69-discovery-stored-the-searched-role-as-job_title-and-discarded-the-real-headline) | Major | Resolved (2026-07-25, root-caused and fixed) | — | Opus 5 | Gemini 3 Pro | — | — | Found while auditing throughput: **55 distinct titles across 3,131 waiting rows**. The SerpAPI path called `AddToFunnel(company, role, ...)` — storing the *search term* — while `result.Title` (the real headline) was logged one line earlier and thrown away. Beyond misleading logs and dashboard, this degraded improvements.md #22, which ranks the queue by embedding title+company: every job found under the same role got a near-identical embedding |
| 68 | [SaveFormMapping cached semantically-empty mappings, burning a Learner Module call per visit](#68-saveformmapping-cached-semantically-empty-mappings-burning-a-learner-module-call-per-visit) | Major | Resolved (2026-07-25, root-caused and fixed) | — | Opus 5 | Gemini 3 Pro | — | — | Bug #21 guards against non-JSON, but an all-null mapping is *valid* JSON. Found live: **7 of 60** cached mappings had every selector null, including `smartrecruiters.com`, `pinpointhq.com` and `applytojob.com`. Each visit to those domains loaded the useless mapping, failed every fill, invalidated the cache and burned a fresh Learner Module call to regenerate the same nulls |
| 67 | [The initial fill path never received #65/#66's fixes, so required dropdowns always failed the first pass](#67-the-initial-fill-path-never-received-6566s-fixes-so-required-dropdowns-always-failed-the-first-pass) | Major | Resolved (2026-07-25, root-caused and fixed) | — | Opus 5 | Gemini 3 Pro | — | — | #65 (dispatch by control type) and #66 (bare-identifier resolution) were both wired only into the validation-*retry* path. `handleDynamic`'s first pass still used `Fill()`-only `safeFill`, so a required `<select>` could never be set on the first attempt and every such form was forced into an avoidable, expensive retry cycle — and custom screening questions that are dropdowns could not be answered at all |
| 66 | [SolveValidationErrors returns bare id/name values, not CSS selectors, so every proposed fix matched nothing](#66-solvevalidationerrors-returns-bare-idname-values-not-css-selectors-so-every-proposed-fix-matched-nothing) | Blocker | Resolved (2026-07-25, root-caused and fixed) | — | Opus 5 | Gemini 3 Pro | — | — | Exposed immediately by #65's new per-selector logging: **12 of 12** proposed fixes on one Greenhouse form failed with "selector matched no element", every one a bare identifier (`question_9558065008`, `country`, `candidate-location`). Playwright reads a bare word as a *tag name*, so it searched for `<country>` elements. The model was returning the literal `id`/`name` values it saw in the DOM — correct answers in an unusable form |
| 65 | [Validation fixes were applied with Fill() only and their errors discarded, so required dropdowns could never be satisfied](#65-validation-fixes-were-applied-with-fill-only-and-their-errors-discarded-so-required-dropdowns-could-never-be-satisfied) | Blocker | Resolved (2026-07-25, root-caused and fixed) | — | Opus 5 | Gemini 3 Pro | — | — | **The dominant failure mode of the current run**: 18 of 23 `FAILED_SUBMIT` outcomes were "failed to submit application after 3 validation error attempts". The retry loop called `safeFill` (Fill()-only) and **discarded the returned error**. Playwright rejects `Fill()` on a `<select>`, and Greenhouse-style forms make dropdowns required (work authorization, EEO), so those fields could never be satisfied and nothing was logged. Proven by the narrowed payload being byte-identical across attempts 2 and 3 |
| 64 | [SolveValidationErrors re-sends the entire form instead of just the fields that failed, timing out on large forms](#64-solvevalidationerrors-re-sends-the-entire-form-instead-of-just-the-fields-that-failed-timing-out-on-large-forms) | Major | Resolved (2026-07-25, root-caused and fixed) | — | Opus 5 | Gemini 3 Pro | — | — | **Filed and fixed 2026-07-25.** The retry path sends the whole pruned form (~55k chars per bug #52's own measurement) when typically only a few fields failed validation. At the ~7 tok/s prompt processing measured live on the 30B, that is ~33 minutes against a 45-minute timeout — borderline by construction. Confirmed live: the same Reddit posting hit it **twice**, once timing out outright (`context deadline exceeded`) and once running 22+ minutes before the run was stopped. Now the pipeline's dominant bottleneck, since #23 removed tailoring and #24 can cut scoring |
| 63 | [Every fit score was computed and thrown away — the only writer of fit_score had zero callers](#63-every-fit-score-was-computed-and-thrown-away--the-only-writer-of-fit_score-had-zero-callers) | Major | Resolved (2026-07-25, root-caused and fixed) | — | Opus 5 | Gemini 3 Pro | — | — | Found while assessing whether improvements.md #14 (LoRA fine-tuning) was worth working: the DB has **zero** non-null `fit_score` values across ~3000 jobs. `UpdateFunnelStatusWithScore` is the only function that writes the column and had **no callers at all** — every scoring outcome went through `UpdateFunnelStatus`, which doesn't take a score. The pipeline's single most expensive step (~9m49s/job after #23) was read once for the `< 50` threshold check and discarded, so no scoring history could ever accumulate |
| 61 | [The cover letter was never sent to any employer — no handler ever filled it](#61-the-cover-letter-was-never-sent-to-any-employer--no-handler-ever-filled-it) | Major | Resolved (2026-07-24, root-caused and fixed) | — | Opus 5 | Gemini 3 Pro | — | — | Found while scoping the static master cover letter: `handleDynamic`/`handleGreenhouse`/`handleLever` fill name, email, phone, resume and custom questions, but there was zero `cover_letter` handling anywhere in `pkg/submitter/` — despite `ExtractFormMapping` being prompted to map a `cover_letter` selector and `FormMapping` carrying it. `AttemptSubmit` took `coverPath` and used it for exactly one thing: an `os.Remove` deferral. Every application in this project's history went out resume only, while still paying the full LLM cost to write a letter that was then discarded |
| 62 | [The saved cover letter was deleted from the application folder, stripping the manual-apply queue](#62-the-saved-cover-letter-was-deleted-from-the-application-folder-stripping-the-manual-apply-queue) | Major | Resolved (2026-07-24, root-caused and fixed) | — | Opus 5 | Gemini 3 Pro | — | — | `defer os.Remove(coverPath)` fired on every exit path including the `ErrAuthWall` early return, so `MANUAL_REQUIRED` jobs lost their letter before `MoveToManualApply` archived the folder — defeating that queue's entire "tailored docs ready" purpose. Live evidence: 5 of 6 sampled `needs_manual_apply/` folders held `resume.md` + `interview_prep.md` but no `coverletter.txt`. Compounded by a path bug (raw vs. sanitized company name) that made the delete land only on sanitize-stable names |
| 60 | [Ollama server pinned to an unnecessarily conservative 6,144-token context window — the dominant cause of MANUAL_REQUIRED outcomes](#60-ollama-server-pinned-to-an-unnecessarily-conservative-6144-token-context-window--the-dominant-cause-of-manual_required-outcomes) | Major | Resolved (2026-07-24, root-caused and fixed) | — | Sonnet 5 | Gemini 3 Pro | — | — | Found live during the 82-job re-verification: every single `MANUAL_REQUIRED` outcome (13/13) was "form too large for the local model." The model itself supports up to 262,144 tokens; the systemd service was manually pinned to 6,144 on 2026-07-21. Raised to 32,768 with KV-cache quantization (`OLLAMA_KV_CACHE_TYPE=q8_0`) — confirmed live that available memory actually *improved* after the change |
| 59 | [Generic submit-button selector could click a hidden anti-spam-widget button instead of the real submit control](#59-generic-submit-button-selector-could-click-a-hidden-anti-spam-widget-button-instead-of-the-real-submit-control) | Major | Resolved (2026-07-24, root-caused and fixed) | — | Sonnet 5 | Gemini 3 Pro | — | — | Found live during the 82-job re-verification: a real Lever posting ("Nova") failed with `playwright: timeout: Timeout 30000ms exceeded` clicking a locator that resolved to `<button type="submit" class="hidden" id="hcaptchaSubmitBtn">` — the `SolveValidationErrors` retry path's submit-button selector matched a hidden hCaptcha auxiliary button before the real, visible submit control |
| 58 | [Stale career_chunks embedding dimension silently zeroed out all live RAG resume-context retrieval](#58-stale-career_chunks-embedding-dimension-silently-zeroed-out-all-live-rag-resume-context-retrieval) | Major | Resolved (2026-07-24, root-caused and fixed) | — | Sonnet 5 | Gemini 3 Pro | — | — | Discovered while implementing improvements.md #22 (fit-similarity queue ranking): every stored `career_chunks` row is 3072-dimensional but the currently configured `nomic-embed-text` model produces 768-dim vectors — `CosineSimilarity`'s length-mismatch guard silently returns 0 for every comparison, so `RetrieveTopK` (used live in `cmd/agent`'s per-job resume/cover-letter tailoring) has been returning an arbitrary, non-semantic chunk order for every application this whole time, no error ever surfaced |
| 57 | [Forms too large for Ollama's context window burned a full doc-gen cycle before failing with an ugly HTTP 400](#57-forms-too-large-for-ollamas-context-window-burned-a-full-doc-gen-cycle-before-failing-with-an-ugly-http-400) | Major | Resolved (2026-07-24, root-caused and fixed) | — | Sonnet 5 | Gemini 3 Pro | — | — | Reddit and Akuity (both Greenhouse, both real, large screening forms) each hit `ollama returned HTTP 400: ... exceeds the available context size (6144 tokens)` on the exact same forms bug #52's payload-size fixes had already cut down — confirming a genuinely different, deeper constraint (the model's actual context window) that no amount of character-based trimming alone could fully solve |
| 56 | [Dashboard has no tile for BLOCKED_CAPTCHA or INVALID_URL, silently omitting 9% of all job_funnel rows](#56-dashboard-has-no-tile-for-blocked_captcha-or-invalid_url-silently-omitting-9-of-all-job_funnel-rows) | Minor | Resolved (2026-07-24, root-caused and fixed) | — | Sonnet 5 | Gemini 3 Pro | — | — | User asked whether the dashboard's visible counts were accurate; each visible tile's own number checked out, but cross-referencing the full `job_funnel` status breakdown found 337 real rows (301 `INVALID_URL`, 36 `BLOCKED_CAPTCHA`) had no tile anywhere on the dashboard at all |
| 55 | [Jobs killed mid-flight get permanently stuck in PROCESSING, never retried, inflating the dashboard's live count](#55-jobs-killed-mid-flight-get-permanently-stuck-in-processing-never-retried-inflating-the-dashboards-live-count) | Major | Resolved (2026-07-24, root-caused and fixed) | — | Sonnet 5 | Gemini 3 Pro | — | — | User asked why the dashboard UI showed 235 "processing" jobs — found every one was a permanently orphaned row from a run killed mid-job (`kill -9`, the only reliable method documented in this file's own Operational Trap notes), accumulated since 2026-07-21, none of them ever retriable since `GetDiscoveredJobs` only pulls `DISCOVERED` |
| 54 | [Raw-HTML captcha pre-check misclassifies Ashby's client-rendered SPA shell as a block](#54-raw-html-captcha-pre-check-misclassifies-ashbys-client-rendered-spa-shell-as-a-block) | Major | Resolved (2026-07-24, root-caused and fixed) | — | Sonnet 5 | Gemini 3 Pro | — | — | Investigating why 2 Ashby postings hit the raw-HTML captcha check before fit-scoring even ran (while trying to get a fresh confirmed success through the generic Learner Module path for bugs #8/#10/#14): confirmed both were real, currently-open, unblocked postings — the check's "little real text = block" corroborating signal is structurally meaningless for a client-rendered SPA fetched without executing JavaScript |
| 53 | [isSubmissionConfirmed only ever ran for Lever/Greenhouse/LinkedIn — every other ATS platform's APPLIED had zero confirmation evidence](#53-issubmissionconfirmed-only-ever-ran-for-levergreenhouselinkedin--every-other-ats-platforms-applied-had-zero-confirmation-evidence) | Blocker | Resolved (2026-07-24, root-caused and fixed) | — | Sonnet 5 | Gemini 3 Pro | — | — | User asked to verify APPLIED jobs generate email confirmations, worried about false positives. Live-probing the exact evidence-tier logic (added the same night for observability) found a live example landing on the weakest tier, which led to discovering the confirmation check was structurally unreachable for most ATS platforms — the majority of `job_funnel`'s entire APPLIED history for non-Lever/Greenhouse/LinkedIn platforms rested on a bare "handler didn't error" signal, the exact unverified-success pattern bug #51 was written to fix |
| 52 | [SolveValidationErrors sends the whole page's DOM, tripping the LLM-cost circuit breaker and losing otherwise-successful applications](#52-solvevalidationerrors-sends-the-whole-pages-dom-tripping-the-llm-cost-circuit-breaker-and-losing-otherwise-successful-applications) | Major | Resolved (2026-07-23, root-caused and fixed) | — | Sonnet 5 | Gemini 3 Pro | — | — | Caught live resuming a watch session: a real Greenhouse posting (fit score 90) generated real tailored documents, filled every field, then was lost outright at the validation-retry step — `incrementAndLogAPICall`'s 50k-char safety limit aborted a 104,932-char payload that was the *entire* pruned page, not just the form |
| 51 | [Post-submit success check trusted any URL change, not proof of an actual successful submission](#51-post-submit-success-check-trusted-any-url-change-not-proof-of-an-actual-successful-submission) | Major | Resolved (2026-07-23, root-caused and fixed) | — | Sonnet 5 | Gemini 3 Pro | — | — | User asked why no confirmation emails were arriving for today's real `APPLIED` jobs. Live Gmail search confirmed real ATS confirmation/rejection emails do exist for today's applications (a real Lever rejection, a real Workable confirmation+rejection) — auto-submit is genuinely reaching employers — but while investigating found `AttemptSubmit`'s only success signal was `currentURL != applyURL`, true for a validation-error redirect just as much as a real success |
| 50 | [Workable requires account sign-in on every posting — same structural class as Workday](#50-workable-requires-account-sign-in-on-every-posting-same-structural-class-as-workday) | Major | Resolved (2026-07-23, root-caused and fixed) | — | Sonnet 5 | Gemini 3 Pro | — | — | Investigating why Ashby/Workable/Homerun sat at 0 `APPLIED` despite #45/#46 clearing their CAPTCHA false positives: probed several live `jobs.workable.com` postings and found a "log in"/"sign in" gate before any form field on every one — 0 `APPLIED` across 12 real attempts this session, not a fill-selector problem. Ashby and Homerun, by contrast, turned out to have real fillable forms once probed properly (see #50's Details section) |
| 49 | [handleGreenhouse's hardcoded submit selector doesn't exist on modern-board postings](#49-handlegreenhouses-hardcoded-submit-selector-doesnt-exist-on-modern-board-postings) | Major | Resolved (2026-07-23, root-caused and fixed) | — | Sonnet 5 | Gemini 3 Pro | — | — | Live 2026-07-23, right after the priority-queue change put Greenhouse first: a real posting (`job-boards.greenhouse.io/alphasense`) filled every field successfully then failed with `failed to click submit: Timeout 30000ms exceeded` — a probe confirmed `input#submit_app` (the only selector `handleGreenhouse` ever tried) has zero matches on this posting's modern board template; the real control is an unidentified `<button type='submit'>Submit application</button>` |
| 48 | [Lever click-to-reveal (bug #47's fix) doesn't fire on a second real posting — possible page staleness after the long doc-gen wait](#48-lever-click-to-reveal-bug-47s-fix-doesnt-fire-on-a-second-real-posting-possible-page-staleness-after-the-long-doc-gen-wait) | Minor | Resolved (2026-07-24, not reproduced despite 11+ subsequent live opportunities under the same precondition — see groom note) | — | Sonnet 5 | Gemini 3 Pro | — | — | Live 2026-07-23, right after #47 shipped: a second real Lever posting (`jobs.eu.lever.co/pnlfin/...`) failed the same way #47 was supposed to fix, but an isolated probe using the exact same selector against a *fresh* page load found and clicked it instantly with no error — pointing at page staleness during the ~14-16 min doc-gen wait between navigation and the click attempt, not a selector problem. Single occurrence so far, not yet confirmed |
| 47 | [Dedicated Greenhouse/Lever handlers never click "Apply" to reveal the form, only the generic Learner Module path does](#47-dedicated-greenhouselever-handlers-never-click-apply-to-reveal-the-form-only-the-generic-learner-module-path-does) | Major | Resolved (2026-07-23, root-caused and fixed) | — | Sonnet 5 | Gemini 3 Pro | — | — | Discovered live 2026-07-23 while re-verifying #45: a real Lever posting reached `handleLever` for the first time (previously always killed earlier by #45/#46) and failed with `form failed to render in time` — `handleLever`/`handleGreenhouse` were never wired to bug #8's click-to-reveal step, unlike the Learner Module path, because they "weren't implicated" when #8 was fixed. Invisible until #45/#46 stopped killing these jobs before they ever reached this code |
| 46 | [Raw-HTML job-description fetch also misdetects reCAPTCHA/Turnstile widgets as a block, before fit-scoring even runs](#46-raw-html-job-description-fetch-also-misdetects-recaptchaturnstile-widgets-as-a-block-before-fit-scoring-even-runs) | Blocker | Resolved (2026-07-23, root-caused and fixed) | — | Sonnet 5 | Gemini 3 Pro | — | — | Found immediately after fixing #45: a second, earlier, independent CAPTCHA check in `cmd/agent/main.go` (on the raw HTML from a plain `net/http` fetch, before fit-scoring) had the exact same bare-substring false-positive bug — confirmed live, it killed a real Lever posting before #45's fix could even matter |
| 45 | [isCaptchaBlocked misdetects standard reCAPTCHA/hCaptcha anti-spam widgets on real forms as a full block](#45-iscaptchablocked-misdetects-standard-recaptchahcaptcha-anti-spam-widgets-on-real-forms-as-a-full-block) | Blocker | Resolved (2026-07-23, root-caused and fixed) | — | Sonnet 5 | Gemini 3 Pro | — | — | DB analysis 2026-07-23: `BLOCKED_CAPTCHA` accounted for 89% of Greenhouse outcomes, 91% of Lever, 96% of Ashby, 82% of Workable — platforms with dedicated handlers, previously assumed reliable. Live probes of several of these postings found large, genuinely fillable forms (21-40 real fields) being killed purely because a standard invisible reCAPTCHA/hCaptcha anti-spam widget iframe was present. Almost certainly the single largest suppressor of `APPLIED` outcomes in the project's history, dwarfing #4/#8/#10/#14 combined |
| 4 | [AttemptSubmit form-fill logic never looked inside iframes](#4-attemptsubmit-form-fill-logic-never-looked-inside-iframes) | Major | Resolved (2026-07-23, verified via targeted unit test in lieu of unreachable live confirmation) | — | Sonnet 5 | Gemini 3 Pro | — | — | Real root cause of the "timeout" symptom: several ATS platforms (confirmed on SmartRecruiters) embed the application form in an `<iframe>`; every fill selector was scoped to the top-level page only. A first attempt just widened the timeouts (wrong diagnosis, verified live not to help — same failures recurred at the new, longer timeout). Re-diagnosed via a standalone script that loaded a failing page directly and found 0 `<input>` tags on the main page plus 1 iframe |
| 6 | [Ollama generation throughput collapses mid-request, likely context-shift thrashing](#6-ollama-generation-throughput-collapses-mid-request-likely-context-shift-thrashing) | Blocker | Resolved (2026-07-21) | — | Sonnet 5 | Gemini 3 Pro | — | — | Root cause turned out to be different from the leading hypothesis (see Resolved section): the hardcoded 10-minute client timeout, not context-shift, was killing honest-but-slow generations |
| 7 | [FunnelEngine still lets Greenhouse job-search/listing pages into the pipeline](#7-funnelengine-still-lets-greenhouse-job-searchlisting-pages-into-the-pipeline) | Minor | Resolved (2026-07-21, via #15) | — | Sonnet 5 | Gemini 3 Pro | — | — | Remainder of #5 after the Workday/Workable fix (Resolved 2026-07-21): the one Greenhouse false positive seen (`job-boards.greenhouse.io/remotecom/jobs/7778860003`) wasn't re-reproduced live this session, and a bare `/jobs/<id>` path is normally a real posting pattern, so a safe tightening rule wasn't obvious enough to fix opportunistically — needs its own live repro before writing a fix |
| 9 | [Dead-job-posting detection missed common phrasings, wasting cycles on expired listings](#9-dead-job-posting-detection-missed-common-phrasings-wasting-cycles-on-expired-listings) | Minor | Resolved (2026-07-21) | — | Sonnet 5 | Gemini 3 Pro | — | — | Confirmed live 2026-07-21: a Jobvite posting that had expired between discovery and `AttemptSubmit` looked exactly like a #8 click-to-reveal failure until a standalone diagnostic script showed it redirected to a `?error=404` page whose text ("job listing no longer...") didn't match the single hardcoded dead-job phrase the code checked for |
| 10 | [DOM-mapped fill failures never fell back to the Vision module, only outright mapping failures did](#10-dom-mapped-fill-failures-never-fell-back-to-the-vision-module-only-outright-mapping-failures-did) | Major | Resolved (2026-07-24, closed via direct end-to-end test, live confirmation structurally unreachable so far) | — | Sonnet 5 | Gemini 3 Pro | — | — | Structural observation after diagnosing #4, #8, and #9 in one session: every live failure was the Learner Module confidently returning a plausible-but-wrong selector mapping (a fill failure), never an outright mapping-generation failure — yet `AttemptVisionSubmit` (screenshot + visual LLM reasoning, already implemented and wired to a locally-pulled vision model) only triggered on the latter. A fill failure just deleted the cache and gave up |
| 11 | [FunnelEngine lets Jobvite `/search` listing pages into the pipeline](#11-funnelengine-lets-jobvite-search-listing-pages-into-the-pipeline) | Minor | Resolved (2026-07-21) | — | Sonnet 5 | Gemini 3 Pro | — | — | Observed live 2026-07-21: `jobs.jobvite.com/cloudone-digital/search` (a listing page, not a posting) was scored and reached `AttemptSubmit`, same false-positive class as #5/#7 but for Jobvite. Not yet root-caused — `isValidATSUrl` only gates the Yahoo-fallback path; worth checking whether Jobvite needs the same kind of path-based tightening Workable got in #5 |
| 12 | [Same job URL reprocessed repeatedly, hitting a UNIQUE constraint on applied_jobs.url](#12-same-job-url-reprocessed-repeatedly-hitting-a-unique-constraint-on-applied_jobsurl) | Major | Resolved (2026-07-21) | — | Sonnet 5 | Gemini 3 Pro | — | — | Root-caused as `AddToFunnel`'s `ON CONFLICT ... DO UPDATE SET status=excluded.status` silently resetting an in-progress/finished job's status back to `DISCOVERED` on every re-discovery, combined with FunnelEngine unconditionally re-queuing on any successful `AddToFunnel` call. Observed live re-running individual jobs 3-5+ times, 20-30+ min each — very likely the dominant reason `applied` never moved during tonight's ~7 hour session |
| 13 | [Ollama gets kernel OOM-killed under this machine's real RAM ceiling](#13-ollama-gets-kernel-oom-killed-under-this-machines-real-ram-ceiling) | Major | Resolved (2026-07-21, mitigated) | — | Sonnet 5 | Gemini 3 Pro | — | — | Confirmed via `journalctl -k`: two live `llama-server` OOM-kills this session (14:02, 15:57), the second immediately after #10's Vision fallback started loading a second model concurrently with the 30B text model on a 29GB-RAM machine. Not a soft/config limit — the kernel OOM-killer genuinely ran out of memory. Mitigated (not eliminated) by setting `OLLAMA_MAX_LOADED_MODELS=1` |
| 14 | [No accessible-label fallback for form-field filling, only CSS selector guessing](#14-no-accessible-label-fallback-for-form-field-filling-only-css-selector-guessing) | Major | Resolved (2026-07-24, closed via direct end-to-end test, live confirmation structurally unreachable so far) | — | Sonnet 5 | Gemini 3 Pro | — | — | Structural gap behind every fill failure in #4/#8/#9/#10: the Learner Module only ever produces a guessed CSS selector, with no fallback to the field's accessible label (`<label for>`/`aria-label`) that WCAG-compliant enterprise ATS forms reliably expose even when raw name/id attributes are obfuscated. Confirmed against current Playwright best-practice guidance via web search: `GetByLabel`-style "user-first locators" are the recommended primary strategy over raw CSS selectors for exactly this kind of unknown-markup automation |
| 15 | [Dedicated Greenhouse/Lever handlers timing out waiting for the form to render](#15-dedicated-greenhouselever-handlers-timing-out-waiting-for-the-form-to-render) | Major | Resolved (2026-07-22, verified live) | — | Sonnet 5 | Gemini 3 Pro | — | — | Observed live 2026-07-21: two consecutive jobs on the *dedicated* (non-Learner-Module) handler path — `handleLever` (`mistral`) and `handleGreenhouse` (`nebius`) — both failed with `form failed to render in time: playwright: timeout: Timeout 30000ms exceeded.`. These handlers were assumed reliable all session (the confirmed-real `APPLIED` rows are all Greenhouse/Lever); this is the first evidence they can fail too. Not yet root-caused |
| 16 | [#14's label fallback needs a GetByPlaceholder tier too](#16-14s-label-fallback-needs-a-getbyplaceholder-tier-too) | Minor | Resolved (2026-07-21) | — | Sonnet 5 | Gemini 3 Pro | — | — | Observed live 2026-07-21 three times (Jobvite x2, ApplyToJob): `GetByLabel("First Name")` found nothing even with the *correct* label text identified, most likely because those forms use `placeholder="First Name"` with no real `<label>`/`aria-label` association — a form that looks labeled to a human but isn't semantically one. Added a `GetByPlaceholder` tier to the fallback chain |
| 17 | [ORDER BY last_updated DESC picked a stale row over a genuinely newer one](#17-order-by-last_updated-desc-picked-a-stale-row-over-a-genuinely-newer-one) | Major | Resolved (2026-07-21) | — | Sonnet 5 | Gemini 3 Pro | — | — | User caught this live from the dashboard: "Working on X since 1:48:26 AM" while it was actually ~9:59 PM, looking like an 8+ hour stall on a job really only ~10 minutes old. Initially misdiagnosed as a cosmetic container-timezone display issue; actually a real sort-order bug — mixing UTC and local-offset timestamp formats across two relaunches broke `ORDER BY last_updated DESC`'s plain TEXT comparison |
| 8 | [Dynamic/Learner Module fill path never clicks an "Apply" button to reveal click-to-reveal application forms](#8-dynamiclearner-module-fill-path-never-clicks-an-apply-button-to-reveal-click-to-reveal-application-forms) | Major | Resolved (2026-07-24, closed via direct end-to-end test, live confirmation structurally unreachable so far) | — | Sonnet 5 | Gemini 3 Pro | — | — | Confirmed live 2026-07-21 on a Breezy.hr posting: `resolveFillTarget` correctly found no real form on the page or in any iframe (the only main-page input was an unrelated readonly referral-link box), yet `handleDynamic` filled against it anyway instead of first clicking the page's "Apply" button to reveal the actual form (a fancybox/lightbox modal on this legacy Breezy portal theme). Distinct from #4 — not an iframe problem, a click-to-reveal problem the Learner Module's fill path has no step for |
| 18 | [Workday postings burn full Learner+Vision cycles against an auth-gated application flow with no fillable form](#18-workday-postings-burn-full-learnervision-cycles-against-an-auth-gated-application-flow-with-no-fillable-form) | Major | Resolved (2026-07-21, verified live) | — | Sonnet 5 | Gemini 3 Pro | — | — | Observed live 2026-07-21 ~22:51-23:01 (GDIT SRE `RQ219922` on `gdit.wd5.myworkdayjobs.com`): full tailoring (~6 min) + Apply click + Learner mapping (~3 min) + all three fill tiers + Vision fallback, all doomed from the start — Workday's real application form sits behind account-creation/sign-in, so no First Name field exists on any pre-auth page. Workday URLs are a large share of discovered jobs; each one wastes a 10-30 min cycle that should short-circuit to `applications/manual_submissions.md` on login-wall detection |
| 20 | [Email tracker classifies unrelated emails as INTERVIEW_REQUESTED and writes them to the DB](#20-email-tracker-classifies-unrelated-emails-as-interview_requested-and-writes-them-to-the-db) | Major | Resolved (2026-07-22, verified live) | — | Sonnet 5 | Gemini 3 Pro | — | — | Observed live 2026-07-22 00:05 (first real logged-in scan): Google payment receipts ("we've received your payment") and a LinkedIn application-sent confirmation were classified INTERVIEW_REQUESTED, each logging "Updating database" — junk statuses written against fuzzy company matches ("google") can corrupt real application history |
| 25 | [Fit scoring ignores geographic eligibility restrictions](#25-fit-scoring-ignores-geographic-eligibility-restrictions) | Minor | Resolved (2026-07-22, verified locally) | — | Sonnet 5 | Gemini 3 Pro | — | — | Observed live 2026-07-22 02:05: "Site Reliability Engineer — Remote from Romania or Hungary" scored 80 for a US-based candidate. `remote_only` is enforced but *where the remote worker must live* is not — cycles get burned (and applications potentially sent) for roles the candidate is ineligible for. Likely fix is a ScoreJob prompt instruction to hard-fail location-restricted roles outside the candidate's country; needs scoring-quality verification before shipping, not a blind prompt edit |
| 24 | [Prompt-injection quarantine may false-positive on ordinary job-page copy ("you are a...")](#24-prompt-injection-quarantine-may-false-positive-on-ordinary-job-page-copy-you-are-a) | Minor | Resolved (2026-07-22) | — | Sonnet 5 | Gemini 3 Pro | — | — | Observed live 2026-07-22 02:03 (Versant3, SmartRecruiters): the quarantine layer blocked the application on `role_manipulation 0.4` via a "you are a" heuristic plus a 0.65 fuzzy keyword match — phrasing like "you are a passionate engineer" is standard job-ad copy, so this heuristic class may silently block legitimate applications. Needs a false-positive-rate check against BLOCKED/quarantine logs before any tuning: loosening an injection defense requires evidence, not one anecdote |
| 23 | [Bot-protection interstitials (DataDome) aren't detected, burning full cycles and feeding the Learner captcha pages](#23-bot-protection-interstitials-datadome-arent-detected-burning-full-cycles-and-feeding-the-learner-captcha-pages) | Major | Resolved (2026-07-22) | — | Sonnet 5 | Gemini 3 Pro | — | — | Confirmed live 2026-07-22 01:43 (AbbVie on jobs.smartrecruiters.com): DataDome served "Access is temporarily restricted" (12-element page, challenge iframe from geo.captcha-delivery.com), yet the pipeline generated docs, "mapped" the captcha page, failed all fill tiers, and went to Vision. The only captcha detection was Cloudflare/reCAPTCHA phrases in the scraper path. `AttemptSubmit` now checks content phrases + challenge-iframe hosts right after navigation and returns `ErrCaptchaBlocked` → `BLOCKED_CAPTCHA`, before any doc generation |
| 21 | [SaveFormMapping caches non-JSON LLM output, poisoning every future visit to the domain](#21-saveformmapping-caches-non-json-llm-output-poisoning-every-future-visit-to-the-domain) | Minor | Resolved (2026-07-22) | — | Sonnet 5 | Gemini 3 Pro | — | — | Confirmed live 2026-07-22 00:31: cached mapping for `www.workday.com/en-us` began with prose ("invalid character 'T'"), guaranteeing a parse failure and a multi-minute Vision fallback on every reuse until invalidated. `SaveFormMapping` now rejects anything failing `json.Valid` |
| 22 | [Stale pre-filter backlog rows and error-redirect URLs bypass every discovery filter](#22-stale-pre-filter-backlog-rows-and-error-redirect-urls-bypass-every-discovery-filter) | Minor | Resolved (2026-07-22, mitigated) | — | Sonnet 5 | Gemini 3 Pro | — | — | Confirmed live 2026-07-22: `www.workday.com/.../hiring-programs.html` (a marketing page queued before #5's filters existed) burned doc-gen + a 34-minute Vision call, and the funnel "discovered" `remotecom?error=true` as a live job. `isValidATSUrl` now rejects any URL with an `error` query param; 62 known-invalid DISCOVERED rows flipped to `INVALID_URL` by one-time DB pass |
| 34 | [A cookie-consent banner's backdrop silently intercepts every click, defeating clickApplyIfPresent](#34-a-cookie-consent-banners-backdrop-silently-intercepts-every-click-defeating-clickapplyifpresent) | Major | Resolved (2026-07-22, verified live) | — | Sonnet 5 | Gemini 3 Pro | — | — | Confirmed live 2026-07-22 (Workable/European Dynamics): the real "Apply now" button reported visible/enabled/stable yet every click retried and timed out — Playwright's own error log showed a `data-ui="backdrop"` div intercepting pointer events across the click target. No amount of increasing `fillActionTimeoutMs` could have fixed this; it's a genuine interaction blocker. `dismissCookieBanner` now runs right after page load, before any interaction |
| 35 | [SmartRecruiters' "I'm interested" button and post-click CAPTCHA reveals both went undetected](#35-smartrecruiters-im-interested-button-and-post-click-captcha-reveals-both-went-undetected) | Major | Resolved (2026-07-22, verified live) | — | Sonnet 5 | Gemini 3 Pro | — | — | Confirmed live 2026-07-22 (Oteemo, SmartRecruiters): `clickApplyIfPresent`'s selector only matched "Apply" text, so SmartRecruiters' actual button ("I'm interested") was never clicked and the fill logic always targeted the public job-description page, which has no form. Separately, clicking it navigates to a *new* `oneclick-ui` URL that can be gated by a fresh DataDome challenge (`geo.captcha-delivery.com`) the earlier #23 captcha check never saw, since that check only ran once, before this click. Both fixed together: broadened the click-target selector, and re-run `isCaptchaBlocked` immediately after the reveal click |
| 36 | [Jobvite's "Data Consent" step means the application form doesn't exist until a location/language <select> is chosen](#36-jobvites-data-consent-step-means-the-application-form-doesnt-exist-until-a-locationlanguage-select-is-chosen) | Major | Resolved (2026-07-22, verified live) | — | Sonnet 5 | Gemini 3 Pro | — | — | Confirmed live 2026-07-22 (CMG Financial, Jobvite): after the Apply click, the page has zero `<input>` elements anywhere (main page or any frame) — only a single `<select>` labeled "Location of Residence and Language". Confirmed via standalone script that choosing an option alone (no extra click needed) instantly reveals the real form (24 fields). `resolveConsentGateIfPresent` now detects a zero-input page with a `<select>` present and chooses an option matching the candidate's actual state from `pii.yaml` (so the CA-privacy-specific disclosure some tenants show stays honest), falling back to the first non-placeholder option |
| 37 | [fillActionTimeoutMs (15000ms) too tight for genuine CPU contention from the co-located Ollama model](#37-fillactiontimeoutms-15000ms-too-tight-for-genuine-cpu-contention-from-the-co-located-ollama-model) | Major | Resolved (2026-07-22, verified live) | — | Sonnet 5 | Gemini 3 Pro | — | — | Confirmed live 2026-07-22: even after cleaning up duplicate processes down to one clean instance, two different real (non-junk) jobs still hit the fill timeout on all three tiers (label/placeholder/CSS) in the same ~45s window immediately after Ollama finished a heavy generation burst (200%+ CPU). Same failure shape as #6's already-fixed Ollama client timeout — a fixed value too short for genuinely slow-but-honest contention, not a selector bug. Doubled to 30000ms across all 6 call sites via one named constant |
| 38 | [FunnelEngine kept sending Learner+doc-gen cycles at a 0%-success source and let Workday monopolize the worker queue](#38-funnelengine-kept-sending-learnerdoc-gen-cycles-at-a-0-success-source-and-let-workday-monopolize-the-worker-queue) | Major | Resolved (2026-07-22, mitigated) | — | Sonnet 5 | Gemini 3 Pro | — | — | DB analysis 2026-07-22: `breezy.hr` had 0 `APPLIED` across 212 discovered jobs (48 `FAILED_SUBMIT`, the worst ratio of any actively-attempted platform) — excluded from `TargetATS` and `isValidATSUrl` entirely. Separately, `GetDiscoveredJobs` had no `ORDER BY`, so 228 already-queued Workday rows (account-gated, can only ever reach `MANUAL_REQUIRED` per #18) came back clustered together and monopolized every worker cycle — confirmed live as 6 Workday jobs in a row post-cleanup. Query now excludes `breezy.hr` and sorts Workday rows last |
| 39 | [Vision-fallback fill fails with "empty selector provided for form filling"](#39-vision-fallback-fill-fails-with-empty-selector-provided-for-form-filling) | Minor | Resolved (2026-07-23, root-caused and fixed) | — | Sonnet 5 | Gemini 3 Pro | — | — | Observed live 2026-07-22 (`brightvisiontechnologies.applytojob.com`): a stale pre-fix cached mapping timed out, correctly invalidated itself, and fell back to `AttemptVisionSubmit`, which then failed with `ErrEmptySelector` rather than a genuine fill attempt — the Vision LLM's response apparently didn't parse into a usable selector for at least one field. Not yet root-caused; needs the same standalone-script approach used for #34-#37, reproduced against a *fresh* (non-cached) Learner Module attempt so cache staleness doesn't confound the diagnosis |
| 40 | [~200+ files/dirs under applications/ are still owned by a stale UID from an earlier containerized run](#40-200-filesdirs-under-applications-are-still-owned-by-a-stale-uid-from-an-earlier-containerized-run) | Minor | Resolved (2026-07-22, full sweep) | — | Sonnet 5 | Gemini 3 Pro | — | — | Confirmed live 2026-07-22: `manual_queue.md`, `manual_submissions.md`, and two per-job directories (`applications/en/`, then `applications/jobs/`) were owned by UID `524288` and silently failing every write with `permission denied` — the second collision (`applications/jobs/`) cost two fully-generated, otherwise-successful applications in one 20-minute window, which is what justified the full fix instead of continuing to patch one collision at a time. User ran `sudo chown -R $(whoami):$(whoami) applications/`; `find applications -not -user howlcipher` now returns zero paths |
| 41 | [applytojob.com and recruitee.com board-index/landing pages scored and processed as real postings](#41-applytojobcom-and-recruiteecom-board-indexlanding-pages-scored-and-processed-as-real-postings) | Major | Resolved (2026-07-22, verified live) | — | Sonnet 5 | Gemini 3 Pro | — | — | Confirmed live 2026-07-22: `holafly.applytojob.com/apply` (bare path, no job ID) is the company's full 20-role "Current Openings" list, not a posting — a standalone script showed zero form fields and a screenshot showed a generic job board. `greatminds.recruitee.com/homepage` is the same shape, the tenant's landing page. Same root cause and fix pattern as the already-fixed board-index bugs for smartrecruiters/lever/greenhouse/ashbyhq/workable/jobvite, just never extended to these two subdomain-tenant platforms. `IsKnownJunkJobURL` now treats any `*.applytojob.com` or `*.recruitee.com` URL with ≤1 path segment as junk (real postings need `/apply/<id>/<slug>` or `/o/<slug>`). applytojob.com had 0 `APPLIED` across 176 historical attempts — this was very likely the dominant reason |
| 42 | [www.bamboohr.com and app.bamboohr.com pages (marketing site, shared login portal) scored as postings](#42-wwwbamboohrcom-and-appbamboohrcom-pages-marketing-site-shared-login-portal-scored-as-postings) | Minor | Resolved (2026-07-22, verified live) | — | Sonnet 5 | Gemini 3 Pro | — | — | Same bare-domain pattern as the homerun.co fix (#29-class): `www.bamboohr.com/integrations/listings/remote` (BambooHR's own product marketing page) burned a 16-minute doc-gen cycle, and `app.bamboohr.com/login/` (the shared employee login portal every tenant uses) scored 80 and reached `AttemptSubmit`. Real postings are always on a company subdomain, e.g. `cxm.bamboohr.com/jobs/questions?id=169`. Both bare hosts now filtered in `IsKnownJunkJobURL` |
| 43 | [getByLabel/getByPlaceholder threw a Playwright strict-mode violation when a label matched more than one element](#43-getbylabelgetbyplaceholder-threw-a-playwright-strict-mode-violation-when-a-label-matched-more-than-one-element) | Major | Resolved (2026-07-22, verified live) | — | Sonnet 5 | Gemini 3 Pro | — | — | Confirmed live 2026-07-22 (Workable/Dispel posting): after every other fix landed, a real fill attempt got past "First Name" cleanly for the first time all session, then failed on "Phone" with `strict mode violation: getByLabel('Phone') resolved to 2 elements` — a second element (likely a hidden duplicate or a country-code sub-field) shared the same accessible label. `GetByLabelLoc`/`GetByPlaceholderLoc` in `pkg/submitter/browser.go` now call `.First()` — filling isn't order-sensitive, so narrowing to one match beats failing outright |
| 44 | [BambooHR corporate subdomains kept slipping past a growing denylist](#44-bamboohr-corporate-subdomains-kept-slipping-past-a-growing-denylist-resolved-2026-07-23) | Minor | Resolved (2026-07-23) | — | Sonnet 5 | Gemini 3 Pro  |
| 19 | [Workday URL parsing takes the locale/site segment as the company name](#19-workday-url-parsing-takes-the-localesite-segment-as-the-company-name) | Minor | Resolved (2026-07-21) | — | Sonnet 5 | Gemini 3 Pro | — | — | Long-observed cosmetic defect, never filed on its own (referenced in passing in #12 and #17): Workday jobs get company names like `en-US`, `External_Career_Site`, `apply`, `en` from URL path segments instead of the real employer (GDIT, U-Haul, etc.), polluting `job_funnel`/dashboard rows and making log lines ambiguous |

## OpenAI task-fit model assignments

These assignments cover current Pending bugs only. They are task-fit starting points, not a claim that the strongest model is always required. Resolved and historical rows retain their existing model metadata so the backlog structure and history remain intact.

| # | OpenAI model | Task-fit reason |
| --- | --- | --- |
| 128 | `gpt-5.6-sol` | Storage and data-integrity fix spanning persistence, naming, and concurrency behavior. |
| 112 | `gpt-5.6-sol` | Cross-layer URL canonicalization, database state, queueing, and reporting require deeper reasoning. |
| 125 | `gpt-5.6-terra` | Bounded retry-state logic and tests are substantial but well-scoped engineering work. |
| 130 | `gpt-5.6-terra` | Network retry/backoff behavior needs balanced implementation and verification. |
| 131 | `gpt-5.6-terra` | Parser and polling resilience is a moderate, testable reliability change. |

## Details

### 118. Resume-selector fallback work breaks every submitter path without a readable resume (Resolved 2026-07-26)

**Found from the current worktree, not inferred from history.** `pkg/submitter/browser.go` contains uncommitted #118 work adding `resumeFileInputSelectors` and `attachResume` after a live Pinpoint mapping targeted a nonexistent resume selector. Preserving that fallback is important. The regression is that `attachResume` reads `resumePath` before it establishes whether a resume upload is mapped or required.

That changes the contract for every dynamic and Vision test that deliberately omits a resume path while testing another behavior. Full verification now fails in six established `pkg/submitter` tests: both custom-question tests, both end-to-end fallback tests, and both cover-letter tolerance tests. The failures occur before those scenarios reach their intended assertion.

**Acceptance criteria as filed:** keep the mapped-selector-plus-fallback behavior; do not read a file when no resume control is present or required; fail clearly when a required resume exists but its file is unreadable; preserve optional-resume behavior; add focused tests for mapped, fallback, absent and unreadable cases; restore `go test ./...` without weakening the six existing tests.

**Resolution:** split control discovery from file reading. `findResumeUpload` searches the mapped selector, resume/CV-named fallbacks, and the sole non-cover-letter file-input rule first. If an optional form has no upload control it returns cleanly without touching the path; once a real control exists, unreadable or empty content is a hard failure and upload errors retain their selector context. Nil locator guards keep sparse test/browser targets from panicking.

**Tests:** added focused coverage for optional no-control/missing-path behavior, mapped-selector miss with named fallback, unreadable required content, the sole-file-input fallback, and exclusion of cover-letter signals from that last-resort selector. Re-ran the six orchestration tests that originally regressed, then `go build ./...`, `go vet ./...`, and `go test ./...`; all pass.

### 119. Free discovery sources are disabled when the SerpApi key is absent (Resolved 2026-07-26)

`pkg/scraper/funnel.go::DiscoverJobs` checks `SERPAPI_API_KEY` and returns immediately when it is blank. Calls to `discoverWithRemoteOK`, `discoverWithHackerNews`, and `discoverWithATSFeeds` come only after that return.

Those three sources are free and already shipped. The ordering makes them depend on an unrelated keyed source anyway, contradicting the project constraint that external keys are not assumed for autonomous free work. Existing funnel tests always install a fake SerpApi key, so the supported no-key configuration has no regression test.

**Acceptance criteria as filed:** always run free sources; run SerpApi only when a real key is present; retain the free Yahoo fallback where appropriate; return a useful aggregate error only when discovery truly produced nothing or a required source failed; add a no-key test proving free sources are invoked and no SerpApi request is attempted.

**Resolution:** `DiscoverJobs` now runs RemoteOK, Hacker News, and public Greenhouse/Lever feed discovery first. It trims and checks `SERPAPI_API_KEY` only for the subsequent role/ATS search loop: a configured key uses SerpApi, while an absent key starts that loop in the existing Yahoo fallback mode. SerpApi errors still switch the remaining queries to Yahoo as before. Re-evaluation removed the proposed aggregate-error behavior: every source in this best-effort discovery fan-in is optional, and a valid run may produce no jobs, so the function has no required-source failure to aggregate. The free-source helpers continue to log their own failures, and no missing optional credential is treated as a discovery failure.

**Tests and documentation:** `TestDiscoverJobsWithoutSerpAPIKeyRunsFreeSources` uses isolated HTTP servers and a temporary database to prove a RemoteOK result reaches the channel, Yahoo receives the role/ATS query, and SerpApi receives zero requests. Existing SerpApi-success and SerpApi-to-Yahoo tests remain green. README and `.env.example` now describe the key as optional. `go build ./...`, `go vet ./...`, and `go test ./...` all pass.

### 120. `--daemon` logs a six-hour drip mode but exits after one batch

`cmd/agent/main.go` parses `--daemon` and logs that the agent will drip applications every six hours. The flag is never read again. The normal producer and worker goroutines run once, `wg.Wait()` returns, “Batch execution complete!” is logged, and `main` exits.

This also invalidates `improvements.md` #10's “Done” rationale and the README launch instruction. It matters operationally because a user can leave the documented command running believing new jobs will be discovered later when the process has already stopped.

**Acceptance criteria:** extract a testable one-cycle function; in daemon mode refresh discovery/DB work each cycle, enforce a configurable per-cycle cap, wait on a context-cancellable clock, and exit promptly on SIGINT/SIGTERM; non-daemon behavior remains one batch; add deterministic tests with an injected clock rather than real six-hour sleeps.

**Done 2026-07-27:** `runAgentCycle` now loads the current `DISCOVERED` rows and invokes every discovery source on each call, merges both streams through one limit boundary, and starts a fresh worker queue for the accepted jobs. `runAgentSchedule` preserves one unlimited cycle for ordinary batch mode; daemon mode repeats the injected cycle every six hours with a default 15-job cap configurable through `-cycle-limit`. Its timer selects on the signal-backed context, so `SIGINT` and `SIGTERM` cancel the wait without another cycle or a six-hour shutdown delay. The database, browser, and model client remain process-scoped rather than being recursively recreated.

Deterministic tests cover one-shot batch behavior, repeated daemon cycles, refreshed backlog and discovery calls, a cap spanning both job sources, invalid daemon settings, an injected cancellable clock, and the real timer helper's cancellation path. `go build ./...`, `go vet ./...`, `go test ./...`, and `go test -race ./cmd/agent -count=1` pass.

Signed implementation commit: `6453177`.

### 121. Untrusted job text reaches embedding and scoring models before quarantine

The worker fetches and prunes a posting, builds `jobDescText`, sends it to `client.GetEmbedding`, and then passes the scraped data to `client.ScoreJob`. Only between those calls does it invoke `filter.CheckPayload(tailoredContext)`, where `tailoredContext` is trusted résumé/RAG material—not the fetched posting.

There is a later scan inside part of the browser submission path, but that occurs after scoring and is not a centralized guard for dedicated Greenhouse/Lever handlers. `Pipeline.TwoStepVerification` contains a more appropriate boundary but is not the path used by the main worker. The README's security-quarantine claim therefore does not hold for the first model calls.

**Acceptance criteria:** route every fetched description and relevant DOM through one pre-LLM quarantine boundary before embedding, scoring, mapping, or judging; make a block a durable terminal/quarantine status rather than leaving `PROCESSING`; do not expose flagged text to a model as instructions during review; preserve the CSV audit log; add spy-based tests proving malicious input causes zero model calls on generic and dedicated ATS paths.

**Done 2026-07-27:** `security.QuarantinePayload` is now the typed deterministic boundary for untrusted posting text and DOM. The worker wraps its posting-dependent embedding and scoring stage with that boundary, preserves structured detections in `applications/prompt_injection_detections.csv`, and moves blocked rows to `QUARANTINED_PROMPT_INJECTION` before any model callback can run. Browser submission applies the same boundary to initial, cached, dynamically revealed, dedicated Greenhouse/Lever, validation-retry, and pre-Vision DOM. The earlier LLM safety-judge override was removed, so flagged attacker text is never presented to a second model; typed error text also omits the match.

Spy tests cover benign passage and malicious raw HTML, checked status-write failures, initial and dynamically revealed generic/Greenhouse/Lever pages, the private CSV audit, and legacy mapping entry points. They prove zero embedding and scoring calls for malicious posting payloads, plus zero mapper, Vision, validation-solver, or judge calls for quarantined DOM; initial browser detections also precede document generation. `go build ./...`, `go vet ./...`, `go test ./...`, and the race suite across `pkg/security`, `cmd/agent`, and `pkg/submitter` pass.

### 122. SSRF defenses block literal private IPs but not hostnames that resolve to them

The worker and redirect checks block three literal hostnames. Hacker News and scraper fetches use the same shape. Playwright's route filter calls `net.ParseIP(reqURL.Hostname())`, which works only when the URL itself contains an IP literal. It never resolves a normal hostname before deciding.

Consequently, a hostname whose A/AAAA record points to loopback, RFC1918, link-local or unspecified space passes validation. DNS rebinding can also change the result between a validation lookup and the connection. This is broader than one call site because arbitrary posting links enter through public feeds and browser subresources.

**Acceptance criteria:** centralize HTTP/HTTPS URL validation; resolve every address family and reject if any result is non-public; enforce the check on initial URLs, redirects, feed fetches and browser requests; bind validation to dialing or otherwise close the rebinding gap; make the resolver/dialer injectable; test loopback/private/link-local IPv4 and IPv6, redirects, mixed public/private answers, and a legitimate public host.

**Done 2026-07-27:** `security.NetworkGuard` now owns HTTP and HTTPS syntax validation, complete A/AAAA answer-set validation, special-use address classification, and connection-bound dialing. It rejects the entire host when any answer is non-public, then passes only a validated IP literal to the injected final dialer. Its guarded `http.Client` disables environment-configured forwarding proxies and revalidates both initial requests and every redirect. The worker validates all job URLs before claiming them, while every RemoteOK, Hacker News, ATS-feed, SerpApi and Yahoo fetch uses the same client factory.

Playwright now creates each browser context with an ephemeral authenticated proxy listening only on `127.0.0.1`. The proxy applies the guarded dialer to ordinary HTTP and HTTPS `CONNECT` traffic, Chromium's implicit loopback bypass is subtracted, and the existing route layer independently resolves and checks every initial or subresource URL. This binds validation to the actual browser connection rather than trusting a preflight lookup.

Tests cover public and special-use IPv4 and IPv6 ranges, literal and resolved targets, mixed public/private answers, validated-IP dialing, unsafe redirects, a resolution that rebinds between preflight and dial, proxy authentication, HTTP forwarding and `CONNECT` tunneling. The real-Chromium integration test passed in the documented `career-agent` distrobox from a host-built test binary: the public-name request traversed the guarded proxy, while a loopback navigation never reached the local target. `go build ./...`, `go vet ./...`, `go test ./...`, and focused race suites for `pkg/security`, `pkg/scraper`, `pkg/submitter`, and `cmd/agent` pass.

### 123. Failed and non-2xx job-page fetches still proceed to expensive fit scoring

In the main worker, a transport error from `httpClient.Do` has no error branch: the description remains blank and processing continues. Successful responses are read regardless of `StatusCode`, so a 404, 429 or 500 error page becomes job text. `defer resp.Body.Close()` is also inside the long-lived worker loop, retaining bodies until that worker finishes the entire queue.

The next step is the pipeline's slowest operation: embedding plus fit scoring. Apart from wasted runtime, a title-only or error-page score is not a grounded decision about a real posting.

**Acceptance criteria:** accept only usable 2xx content with a conservative minimum signal; close each response in the current iteration; classify 404/410 as terminal invalid/closed postings; return transient network, 429 and 5xx failures to a retryable state with bounded backoff; check status-write errors; cover each branch with an injected HTTP client/server.

**Done 2026-07-27:** `fetchJobPage` now accepts only 2xx responses with at least 200 visible runes before the worker may embed or score a missing description. HTTP 404 and 410 responses move to `INVALID_URL`. Transport errors, response-read failures, HTTP 429, and HTTP 5xx responses receive at most three attempts with one-second and two-second context-cancellable waits; exhausted failures return to `DISCOVERED`. Other non-success responses and weak 2xx pages also remain retryable without a hot loop. Response bodies close inside each attempt before any wait or return, and every affected funnel-status write reports errors. The existing CAPTCHA distinction remains intact: explicit challenges and weak non-SPA widget pages route to `BLOCKED_CAPTCHA`, while widget presence on a real posting does not pre-skip it. Injected servers and HTTP clients cover every disposition, retries, response closure, cancellation, and CAPTCHA classification. `go test -race ./cmd/agent -count=1`, `go build ./...`, `go vet ./...`, and `go test ./...` pass.

### 124. The email tracker acknowledges a message even when its database update fails

`pkg/tracker/imap.go::updateDBWithTrackerResult` returns no value and discards the result of `db.Exec`. Its caller then reaches `storage.MarkEmailProcessed` regardless of whether the update succeeded. The preceding log says “Updating database,” which can look like confirmation even when SQLite rejected the write.

The Message-ID ledger makes the loss durable: later polls skip that email forever. A momentary database lock can therefore erase the only automated rejection/interview signal.

**Acceptance criteria:** return and handle the database error; use a transaction where outcome write and processed-message acknowledgement can succeed together, or acknowledge only after a confirmed durable outcome; distinguish unmatched/no-op/updated states; verify exactly one intended row changed; add tests for locked/erroring DBs and successful retries.

**Done 2026-07-27:** `updateDBWithTrackerResult` now validates the requested outcome and returns an explicit unmatched, no-op, or updated result. For matched outcomes it updates exactly one `APPLIED` row and inserts the Message-ID ledger entry in the same SQLite transaction. Database locks, acknowledgement failures, and matches affecting more than one active application roll back without acknowledging the email, so a later poll can retry safely. Tests cover every result state, invalid status rejection, multi-row rollback, a real SQLite write lock followed by retry, and an acknowledgement-table failure followed by retry. `go build ./...`, `go vet ./...`, `go test ./...`, and `go test -race ./pkg/tracker -count=1` pass. Signed implementation commit: `fabe12c`.

### 125. Ambiguous outcome emails retry forever instead of entering manual review

Before #124, the tracker matched an outcome email at company level and could update every `APPLIED` row for that company. Bug #124 removed that corruption path: the update and Message-ID acknowledgement now share one transaction, and a match affecting more than one row rolls back. Its two-role regression proves both rows remain `APPLIED` and the email remains retryable.

The remaining limitation is fail-closed but incomplete. There is still no role, posting URL, requisition ID, or thread correlation. An ambiguous message now fails on every 15-minute poll, writes only a transient error log, and never enters a durable manual-review queue. The outcome signal is preserved in the inbox, but automated tracking cannot progress and the repeated retries create noise.

**Acceptance criteria:** correlate to one application using stable ATS/requisition metadata and normalized role clues; if more than one candidate remains, persist one explicit ambiguous event for manual review and update none; avoid repeating the same error every poll after durable handoff; add two-role tests for rejection, interview, ambiguous subjects, and idempotent manual routing.

**Re-scored 2026-07-27 after #124:** the high-value bulk-corruption failure is fixed, so this is now Minor. Value drops from 7 to 3; Decay remains 1.0 because defect value does not diminish with unrelated tracker fixes; Effort remains 4 for stable role correlation plus a durable manual-review path. Score: **0.75**, above the ROI floor.

**Done 2026-07-28:** the tracker now resolves multiple APPLIED roles at the same company by extracting stable ATS IDs (Greenhouse/Lever) from the stored job URL or searching for normalized role title keywords in the incoming email's subject and body. If exactly one role matches, it is updated. If the match remains ambiguous (zero or multiple matches), the system explicitly logs a `MANUAL_REQUIRED` task with context and acknowledges the message in `processed_emails` so it does not retry on the next poll. `go test ./pkg/tracker` passes with new coverage for ambiguous resolution, ID matching, and title keyword matching. Signed implementation commit: `9db9322`.

### 126. The unauthenticated dashboard binds every network interface while announcing localhost

`cmd/dashboard/main.go` logs `http://localhost:8080` but calls `http.ListenAndServe(":8080", nil)`. An empty host binds all interfaces. The root page and `/api/metrics` have no authentication and expose application funnel data, recent role/company context and posting URLs. The default server also has no read-header, read, write or idle timeouts.

**Acceptance criteria:** default to `127.0.0.1:8080`; make the address configurable and visibly warn or require an access control when a non-loopback address is selected; use an explicit `http.Server` with defensive timeouts; test default/configured bind selection and preserve dashboard behavior on loopback.

**Done 2026-07-27:** the dashboard now defaults to `127.0.0.1:8080`, accepts an explicit validated `-addr`, and prints a prominent warning whenever the selected host is not provably loopback. It uses a dedicated `http.Server` with 5-second read-header, 15-second read, 30-second write, and 60-second idle timeouts. Tests cover default and configured IPv4/IPv6 addresses, malformed ports, wildcard/LAN warnings, handler selection, and every timeout. `go build ./...`, `go vet ./...`, `go test ./...`, and `go test -race ./cmd/dashboard -count=1` pass. End-to-end probes returned HTTP 200 for both routes on a separate loopback port; the rebuilt existing container dashboard now listens only on `127.0.0.1:8080`, and its non-loopback probe is refused. Signed implementation commit: `b717e53`.

### 127. Sensitive credentials application data and generated documents are world-readable

This audit inspected permission metadata only. `.env`, `pii.yaml`, the SQLite database/WAL files, source résumé/cover-letter files, and generated application artifacts are currently mode `0644`; generated company directories are `0755`. `SaveApplication` explicitly creates `0755` directories and writes résumé, letter, prep and metadata files as `0644`.

On a multi-user machine, another local account can read credentials, personal contact data, application history and tailored documents. `.gitignore` prevents repository disclosure but is not a filesystem access control.

**Acceptance criteria:** use `0700` for private directories and `0600` for credentials, databases, logs and application documents; start with a restrictive process policy/umask; safely repair existing paths at startup or through a documented command; do not follow symlinks during repair; add mode tests and a clear warning when permissions cannot be secured.

**Done 2026-07-27:** every maintained command now applies an owner-only umask before opening databases or logs and fails closed with a clear warning if existing private paths cannot be secured. `pkg/security` repairs the known credential, SQLite, log, source-document, and generated-application paths idempotently. Changed paths are opened with `O_NOFOLLOW` and chmodded through their descriptors; symbolic links and non-regular paths are refused. `pkg/storage` creates application directories at `0700`, writes documents and reports at `0600`, and secures SQLite database, WAL, and shared-memory files before and after initialization. `cmd/securefiles` exposes the same bounded repair as a documented maintenance command.

Tests cover restrictive process defaults, recursive and repeat repair, symbolic-link target preservation, warning propagation, private database creation, and generated artifact modes. `go build ./...`, `go vet ./...`, `go test ./...`, and the focused race suite for security, storage, tracker, agent, and dashboard all pass. The live repair required one exact-path chmod for a legacy container-owned log, then completed successfully: every named private root file is `0600`, all generated directories are `0700`, all generated files are `0600`, and the application tree contains no symlinks. Signed implementation commit: `e4e48e1`.

### 128. Saving a second role at the same company overwrites the first role's documents

`pkg/storage/manager.go::SaveApplication` derives `applications/<safe company>/` and writes fixed `resume.md`, `coverletter.txt`, `interview_prep.md`, and `metadata.json` names. Job title and URL are stored inside metadata but do not participate in the directory key. Sanitization also maps distinct punctuation to underscores, creating extra collisions.

The second role replaces the first role's artifacts. With multiple workers, same-company jobs can interleave those writes; manual-apply movement can then preserve documents for the wrong role. Improvement #21 made the destination collision-safe but did not make the source role-specific.

**Acceptance criteria:** key artifacts by company plus stable job ID or normalized-URL hash; write atomically; return/store the exact artifact directory instead of reconstructing it from company name; make manual-queue links use that path; preserve or migrate existing company-only folders; test two roles, sanitization collisions and concurrent saves.

**Done 2026-07-27:** new artifacts are stored in `applications/<safe-company>/<normalized-url-hash>/`, preserving existing company-only folders without touching them. `SaveApplication` returns the precise directory and uses a process-local save lock with atomic private-file replacements. The agent passes that returned directory to `MoveToManualApply`, so manual-queue links identify the correct role. Tests cover two roles at one company, two labels that sanitize to the same company name, concurrent saves, exact cover-letter paths, and restrictive modes. `go build ./...`, `go vet ./...`, and `go test ./...` pass.

### 129. The agent hard-codes one developer-specific career-profile path

`cmd/agent/main.go` declares a constant path under `/var/home/howlcipher/dev/ai_knowledge_library/USER_PROFILE.md`. `cmd/reingest` repeats the same machine-specific path as its flag default. There is no profile setting or environment override in the normal agent path.

On a different username, checkout layout, container mount or CI run, initial ingestion fails and the worker continues with no grounded career chunks. Because cached chunks may exist from an older profile, the failure can also leave stale personal context in use.

**Acceptance criteria:** resolve the profile through an explicit flag/config/environment setting with a portable repository-relative default where appropriate; validate readability at startup; fail closed or require an explicit no-RAG mode rather than silently using empty/stale context; share resolution code with `cmd/reingest`; test missing, configured and stale-cache cases without logging profile contents.

**Done 2026-07-27:** `pkg/config` now owns one resolver used by both maintained ingestion commands. Precedence is `-profile`, `CAREER_PROFILE_PATH`, repository-local `USER_PROFILE.md`, then the standard sibling knowledge-library checkout; configured paths never silently fall through to another source. `cmd/agent` validates the selected regular file before consulting cached chunks, refuses a failed dimension probe or zero-chunk rebuild, and returns jobs to `DISCOVERED` rather than scoring with empty context after retrieval failures. Explicit `-no-rag` mode bypasses both startup ingestion and per-job retrieval without consuming stale cached chunks.

Tests cover flag/environment/default precedence, sibling fallback, missing and non-regular paths, stale cache with a missing source, matching and mismatched cache dimensions, failed probes, empty rebuilds, and explicit no-RAG selection. Both built commands expose the portable options and exit nonzero before model/browser work when an explicitly configured profile is missing. Their smoke-test output contains neither profile contents nor the removed developer-specific path. `go build ./...`, `go vet ./...`, `go test ./...`, and `go test -race ./cmd/agent ./pkg/config -count=1` pass. Signed implementation commit: `653f320`.

### 117. A single mailbox fetch misses a code that IMAP has not indexed yet (Resolved 2026-07-26)

#111 made the mailbox the acceptance signal, and deliberately used **one** fetch to keep the retry path cheap. That was too tight, and ClickHouse measured it:

| event | time |
| --- | --- |
| Attempt 2 submit | 08:48:11 |
| Greenhouse sent the code (`p5Kqsn22`) | **08:48:11** |
| #111's fetch found nothing | **08:48:21** |
| Attempt 3 clicked a `disabled=true` submit control | 08:50:10 → 30s timeout |

The email existed for ten seconds before the check ran and was still not returned — IMAP had not indexed it. So the agent concluded "not accepted", proceeded to another attempt, and clicked a button that was disabled *because the form had already been accepted*. #101's diagnostic named it exactly: `submit control: disabled=true inViewport=false, nothing covering it`.

**Fix.** `pendingSecurityCodeAfter` polls on a 25-second budget with a 5-second tick, rather than fetching once. That stays well under `waitForSecurityCode`'s 90 seconds because it runs on every retry attempt, while comfortably outlasting the lag actually observed. Tests pin both directions: a code appearing on the fourth poll is found, and a genuinely absent code still gives up inside the budget.

**Wider point worth keeping.** This is the sixth instance of the same underlying mistake in this session — reading a signal before it is available. #95 read the DOM before the submission happened, #102 read the previous attempt's `aria-invalid`, #111 read a gate that had not rendered, #113 read a field that had not rendered, #116 judged a resubmit instantly, and now #117 read a mailbox that had not indexed. **Every signal this pipeline depends on arrives late; none of them should be read once.**

### 116. The post-security-code resubmit still judged the page in one instantaneous read (Resolved 2026-07-26)

**#115 worked. The chain ran end to end for the first time, and then failed on the very last step:**

```
08:00:31 Retrieved a security code from email (subject: "Security code for your application to Akuity")
08:00:31 akuity issued a security code after the last submit — that submission was ACCEPTED
08:00:31 Security-code gate detected for akuity — waiting for the emailed code...
08:00:31 Entered the emailed security code for akuity; resubmitting
08:00:31 akuity needs manual completion ... (code entered, no confirmation)
```

**Every line in the same second**, which is the signature #95 exists to eliminate.

**Root cause.** The post-code resubmit had its own verdict, written before #95 and never updated:

```go
page.WaitForLoadState(networkidle, 10s)
if content, err := page.Content(); err == nil {
    if confirmed, reason := isSubmissionConfirmed(...); confirmed { ... }
}
```

That is exactly the code #95 replaced in `confirmOrError` and in the retry loop. There were **three** post-click verdict sites and #95 fixed two. So the resubmit was judged before the page had a chance to respond — and this is the final step between an accepted application and a confirmed one, which makes it the most expensive site of the three to have missed.

**Fifth instance of one structural pattern.** The backlog already records #65/#66→#67, #74→#75, #28→#31, and #98 needing both prompt paths. Knowing the pattern was not enough to prevent it: when I wrote #95 I checked the two sites I knew about and did not enumerate all callers.

**Fix.** The resubmit now uses `awaitSubmissionOutcome`, like the other two, and logs the reason when it does not confirm so a failure here is diagnosable rather than silent.

**Structural guard, because memory has now failed five times.** `TestNoUnpolledPostClickConfirmationChecks` pins that `isSubmissionConfirmed` appears exactly three times in `browser.go` — its declaration, its use inside `decideSubmissionOutcome`, and #89's opportunistic re-check of already-settled state at the top of a retry. A **new** call site is almost certainly a post-click verdict that should be polling, and the test says so in its failure message. This is the same species of guarantee as #84's `manualReviewErrors` list, added for the same reason: the invariant cannot depend on the next person remembering it.

### 115. Greenhouse splits the one-time code across eight single-character inputs (Resolved 2026-07-26)

**#114 answered this within one cycle of shipping — the sixth consecutive time that "log the evidence before guessing the cause" has paid off** (#80, #96, #97, #100, #114, now this).

The diagnostic dumped every fillable input present at the moment the code could not be entered, and the answer was unmistakable:

```
id:security-input-0  label:Security code  maxlength:1  type:text  visible:true
id:security-input-1  maxlength:1  visible:true
id:security-input-2  maxlength:1  visible:true
...
id:security-input-7  maxlength:1  visible:true
```

**Eight separate inputs, one character each, every `name` attribute empty.**

Three independent reasons the old code could never have worked, and none of them were guessable:

1. The ids are `security-input-N`. Every selector looked for `security_code`, `securityCode`, `verification_code`, or `id*='security-code'` — **`security-input` contains none of those**.
2. `name` is empty on all eight, so the four name-based selectors were dead on arrival regardless.
3. Even a matching selector would have called `Fill("82taTsxA")` on a box with `maxlength=1`.

**This was the last unimplemented link** between an accepted submission and a completed application. Everything either side of it was already confirmed live: the form fills to `invalid fields: 0`, #111 recognises the acceptance from the mailbox, #93 detects the gate, and improvements #32 retrieves the real code.

**Fix.** `fillSplitSecurityCode` tries the split-box layout **first**, since it is what Greenhouse actually uses, and distributes one character per box. It requires **at least as many boxes as characters** and fills exactly `len(code)` of them: fewer boxes means this is not the widget for this code, and a partial fill would submit a **truncated** answer, which is worse than reporting failure. Three tests, including one that reassembles the code from the boxes and asserts it round-trips, and one pinning that a 4-box widget refuses an 8-character code.

The single-field path is retained and still runs when no split widget is present, so other ATS platforms are unaffected.

**Also visible in that dump, worth recording:** `g-recaptcha-response` was present on the page while this submission was **accepted** — direct confirmation of the score-based behaviour inferred in #104's correction, and of why widget presence alone must never be treated as blocking.

### 114. When the emailed code cannot be entered, nothing records what IS on the page (Resolved 2026-07-26)

The full chain fired on Akuity at 07:00, and stopped at the last step:

```
07:00:41 Retrieved a security code from email (subject: "Security code for your application to Akuity")
07:00:41 akuity issued a security code after the last submit — that submission was ACCEPTED
07:00:41 Security-code gate detected for akuity — waiting for the emailed code...
07:01:02 Retrieved a security code for akuity but could not enter it:
         could not find a visible security-code field to fill within 20s
```

**#111 and #32 are confirmed working end to end** — the acceptance was recognised from the mailbox and the real code was retrieved. **#113 established that the field is absent rather than late**: a full 20-second poll found nothing, so the earlier instantaneous failure was not a timing artifact.

**What is still unknown, and why it cannot be guessed.** `DetectSecurityCodeChallenge` fires on a field marker **plus** matching wording, and its markers include bare substrings — `security-code`, `verification-code`. A CSS class such as `security-code-notice`, or the phrase inside an explanatory message, satisfies that with **no input on the page at all**. So the likeliest reading is that detection matched a *message about* the code rather than a field for it. Other possibilities: the input lives outside the resolved fill target's frame, or Greenhouse renders it only after a reload or a click.

Distinguishing those requires seeing the page at that moment — and reproducing it requires an **accepted submit**, which means filing a real application. That is not an acceptable way to gather evidence.

**Fix (diagnostic).** On failure to enter the code, the log now names every fillable input actually present — id, name, type, placeholder, autocomplete, maxlength, label, visibility — and says so explicitly when there are none. `no fillable inputs on the page at all` and `inputs present, none matching` are different diagnoses and the log now separates them. Best-effort: an unevaluable page yields nothing and the original error passes through unchanged.

**This is the fifth time this session the move has been "ship the diagnostic before guessing the cause"** (#80, #96, #97, #100, now #114). The previous four each identified their root cause within one cycle, including #100 catching a defect in #98 within one cycle of #100 shipping. The alternative here — guessing at selectors and re-running — costs a real application per attempt.

**Nothing else changed.** The code is still never logged (improvements #32's rule), and an unenterable code still routes to `ErrNeedsEmailVerification` → manual review with documents preserved, which is correct: the code is in the user's inbox and a human can finish in seconds.

### 113. The emailed code was retrieved and then discarded, because the code field had not rendered yet (Resolved 2026-07-26)

**The closest the pipeline has come to a confirmed application.** Every link in the chain fired, in order, for the first time:

```
06:30:40 Attempt 2 applied 7/7 validation fix(es)              <- form satisfied
06:30:49 Submit verdict after 8.5s: ... invalid fields: 7      <- stale flags, submit was actually ACCEPTED
06:30:51 Security-code gate detected for akuity — waiting for the emailed code...
06:30:51 Retrieved a security code for akuity but could not enter it:
         could not find a visible security-code field to fill
06:30:51 akuity needs manual completion — form is waiting on a one-time code sent by email
```

The submit was accepted. The gate was found. **improvements #32 retrieved the real code from the mailbox.** And then the code was thrown away because the input could not be filled — one step short.

**Root cause.** `DetectSecurityCodeChallenge` substring-matches the page HTML for markers like `name="security_code"` and `security-code`. `fillSecurityCode` needs something stronger: a locator that exists *and* is visible. The markers reach the HTML before the input becomes fillable, and the whole sequence above ran inside **one second**, roughly 11s after the submit. So the fill was attempted against a field that was on its way but not yet there, and it failed immediately with no retry.

**This is the same DOM-lag that produced #95** (read before the submission happened), **#102** (read the previous attempt's `aria-invalid`) **and #111** (read a gate that had not rendered) — now one layer further in, at the field rather than the gate. Fifth instance. The recurring lesson holds: **anything read from this page needs a bounded wait, not an instantaneous read.**

**Fix.** `fillSecurityCode` polls its selector list on a 20-second budget instead of failing on the first pass, and reports the budget in its error so a genuine absence is distinguishable from a slow render. The timing is a `var` so tests can compress it; one test drives a field that appears 600ms late and must still be filled, another pins that a truly absent field still errors.

**Not changed:** the retrieved code is still discarded when the field never appears, and the job still routes to `ErrNeedsEmailVerification` → manual review with documents preserved. That remains correct — the code is in the user's inbox either way, and printing it into the log is explicitly out of bounds (improvements #32 never logs the code, only the subject).

### 112. The same posting exists twice, once per URL scheme, and their statuses have diverged (Resolved 2026-07-28)

**Found while checking a correction I had just made and got wrong.** After #111 I told the user the captcha count was overstated because Akuity was in it. Verifying that claim showed Akuity was *not* among the 9 cohort rows — because its `BLOCKED_CAPTCHA` had landed on a **different row for the same job**.

`AddToFunnel` inserts with `ON CONFLICT(url) DO NOTHING`, keyed on the raw URL. Discovery yields `https://`, while some earlier records and the 82-job verification list hold `http://`. Re-measured read-only against the live database during the 2026-07-27 post-#129 groom:

| `http://` row | `https://` row | pairs |
| --- | --- | --- |
| FAILED_SUBMIT | DISCOVERED | 5 |
| SKIPPED | DISCOVERED | 5 |
| BLOCKED_CAPTCHA | DISCOVERED | 4 |
| FAILED_SUBMIT | APPLIED | 1 |
| *(in agreement)* | | 5 |

**20 pairs, 15 diverged.** The library/backlog previously said 11; live evidence supersedes that stale count and is now the synchronization source for this row.

**Two distinct consequences, of very different severity.**

1. **Outward-facing, now fixed:** `applied_jobs` is keyed the same way, so a job recorded as applied under one scheme was **not deduped under the other** — it could be applied to twice. `HasApplied` now compares on a scheme-normalised key. Only the scheme is normalised: query strings and trailing `/apply` paths genuinely distinguish postings on Lever, and a test pins that `.../aaa-111`, `.../aaa-111/apply` and `.../bbb-222` stay separate jobs.

2. **Reporting, left open deliberately:** the cohort tally used throughout the 2026-07-25/26 session reads whichever scheme the verification file holds, so for up to 15 of 82 jobs it has been showing a **stale status** while the agent worked the other row. Every conclusion drawn from the *log* is unaffected — including the "6 of 7 completed fills were captcha-blocked" figure, which came from log lines — but the funnel status breakdown quoted during that session should be treated as approximate for those rows.

**Reopened 2026-07-26 during the application sweep.** Labelling the row Resolved while one independently mutable half remained open made “zero open bugs” false. The remaining acceptance criteria are: normalize scheme at funnel insertion and lookup boundaries; define a conservative status-merge policy that never silently converts an ambiguous outcome into `APPLIED`; migrate the 20 measured pairs transactionally with before/after reporting; preserve the strongest evidence and queue retryable conflicts for review; add tests for insertion, status updates, queue reads and migration idempotency.

**Fix 2026-07-28.** Introduced `NormalizeURL` which converts `http://` to `https://`. Applied it consistently across all database write and read boundaries in `pkg/storage/manager.go`. Added a startup migration script `migrateURLSchemes` that merges the existing `http://` records into `https://` records, combining their statuses with a conservative resolution policy that promotes `APPLIED` vs `FAILED_SUBMIT` conflicts to `MANUAL_REQUIRED`. Verified idempotency and tested resolution policy via a new test suite.

### 111. #104 labelled an ACCEPTED application captcha-blocked, because the DOM lags the acceptance (Resolved 2026-07-26)

**A false positive in my own #104, of precisely the kind #104's own guard was written to prevent.** I had identified this risk hours earlier, added a guard, claimed a test pinned "both directions", and it still fired.

```
05:59:19  (Greenhouse email) Security code for your application to Akuity: 82taTsxA
05:59:27  Submit verdict after 8.5s: page re-rendered with fields flagged invalid (invalid fields: 7)
05:59:27  akuity is behind a bot-protection challenge — marked BLOCKED_CAPTCHA
          (every rejected field was already set; recaptcha.net/... present)
```

The email is timestamped **eight seconds before** the verdict that called the submission blocked. It was **accepted**.

**Why the guard failed.** #104 skips its captcha verdict when `parser.DetectSecurityCodeChallenge(prunedHTML)` is true. That reads the **DOM**, and Greenhouse emails the code within ~1s while the code *input* does not appear for far longer — certainly not within the 8s settle. So the guard asked a question whose answer had not arrived yet, and the stale answer looked like "no gate".

**The mistake behind the mistake.** My test pinned the logic *given a rendered gate*; it could not pin the case where the gate has not rendered, because that case has nothing to assert against in a mock DOM. I stated the guard "pins both directions" — it pinned both directions of one branch, not both real-world situations. **A test over a mocked signal cannot establish that the signal is available when needed.**

**This is the same error class a fourth time**: #95 read the DOM before the submission happened, #102 read `aria-invalid` left over from the previous attempt, #103 read internal option ids, and now #104's guard read a DOM gate that had not rendered. Each time the fix was to stop asking the page and ask something authoritative.

**Fix.** The **mailbox** is the ground truth. `pendingSecurityCodeAfter` does **one** cheap `SecurityCodeFetcher` call per retry attempt, and that fetcher only returns codes issued *after* the click that triggered them (improvements #32's design) — so a hit means "the server accepted this submission", regardless of what the page shows. It now:

1. **blocks #104's captcha verdict** when a code is pending;
2. **triggers the #93/#32 code-entry path** on the email alone, not only on a rendered gate;
3. **reuses the code already fetched**, skipping `waitForSecurityCode`'s 90-second poll.

Fetch errors and a missing fetcher both read as "no code" rather than as acceptance, and a zero click-time never queries the mailbox — tests pin all three, plus that the query is scoped to codes issued after the click so a stale one can never be reused.

**Consequence for the run:** Akuity currently sits in `BLOCKED_CAPTCHA` while actually holding an accepted, code-pending application. It needs requeuing, and the same is likely true of any other board that reached this branch.

### 110. A short option label could hijack a longer answer — "Prefer not to say" selected "No" (Resolved 2026-07-26)

**Found by a test written for #109, not by the log** — the live symptom was indistinguishable from an ordinary rejection, and would have stayed that way.

`pickComboboxOption` matched an option against the wanted value with raw bidirectional containment:

```go
strings.Contains(text, wantN) || strings.Contains(wantN, text)
```

Normalisation strips punctuation but not word boundaries, so a **short label hides inside longer prose**. Against Sporty Group's real option list:

| want | old rule selected | correct |
| --- | --- | --- |
| `Prefer not to say` | **`No`** — because `"no"` sits inside `"prefer **no**t to say"` | `Prefer not to say` |

`"male"` inside `"female"` is the same shape, and so is `"yes"` inside `"yesterday"`.

**This is the exact failure #79 exists to prevent, occurring inside the function that enforces it.** #79's guarantee is *never commit the wrong entry* — written after an earlier probe committed "Macomb, Illinois" for a Michigan address. Here the consequence is worse in kind than in size: on an EEO question, a declined answer silently becomes a **substantive** one, submitted under the user's name.

**Fix.** `optionTextMatches` compares **whole words in sequence**: exact equality, else one side's words appearing as a contiguous run inside the other's. Every match the old rule was written for survives, and there are tests for each — `"United States"` still matches `"United States of America"` (#79), and `"Macomb, MI, USA"` still matches `"Macomb, MI"` (improvements #33). Out-of-order and non-contiguous word sets are rejected, so `"states united"` and `"united america"` no longer match.

All six pre-existing `pickComboboxOption` tests pass unchanged, which is the evidence that the loosening this replaces was never actually needed.

**Retroactive exposure, stated because it affects work already done.** The loose matcher was live for the whole 2026-07-25/26 session and every prior one. Any single-choice question where a short option label is a substring of the intended answer could have been committed wrongly, and several of those commits happened on forms that genuinely reached Greenhouse.

**Which specific answers were affected cannot be determined.** Values were not logged until #97 shipped, and the browser state is gone. What *is* established: the four applications that reached Greenhouse (Surt AI, ClickHouse ×2, Akuity) are all **incomplete**, held at an emailed security-code challenge, so nothing has been finally filed carrying a wrong answer. That is fortunate rather than by design.

**Action for the user:** if any of those pending applications is completed by hand, check the EEO / self-identification answers rather than trusting what the agent selected.

### 109. A single-choice question rendered as a checkbox group was read as one box to untick (Resolved 2026-07-26)

Probed Sporty Group's real form after #106/#107 made its last fields settable and it *still* rejected them:

| id | label | name |
| --- | --- | --- |
| `question_8242451101[]_54236359101` | **Yes** | `question_8242451101[]` |
| `question_8242451101[]_54236360101` | **No** | `question_8242451101[]` |
| `question_8242451101[]_54236362101` | **Prefer not to say** | `question_8242451101[]` |

It is a **single-choice question rendered as a checkbox group** — three boxes sharing one `name`, each marked required. A model value of `"No"` means *tick the box labelled No*. `applyValidationFix` read it through the standalone-checkbox rule and **unticked** the box instead: the opposite outcome, and the group stayed empty so the form could never validate.

**#107 made the reporting worse while making the check more correct.** Before it, the unticked box read as not-landed and the job reached `MANUAL_REQUIRED` with documents preserved and the field named. After it, the same wrong action reported as **landed**, `lastNotLanded` was empty, and Sporty Group degraded to a bare `FAILED_SUBMIT`. Worth recording plainly: a fix that made one thing more accurate made an adjacent outcome less useful, and only the live re-run showed it.

**Fix.** When a checkbox shares its `name` with siblings, the value names *which option to tick*, resolved through the same `pickComboboxOption` path comboboxes use — so #79's never-commit-the-wrong-entry rule now covers checkbox groups too, and an unmatched value ticks **nothing** rather than guessing. `verifyFixLanded` checks the *matched* option, which is usually a different element than the selector resolved to. Standalone checkboxes keep #107's behaviour exactly, pinned by a test.

### 108. A submit that went nowhere was reported as "form too large for the local model" (Resolved 2026-07-26)

```
03:29:26 Attempt 2 applied 3/3 validation fix(es) to: #preferred_name, #question_6122095009, #question_6122097009
03:29:42 Submit verdict after 15.3s: no confirmation and no rejection evidence within the settle budget
         (url moved: false, invalid fields: 0, page 105262 chars)
03:29:42 Ethos's form is too large for the local model — queued for manual submission
```

**Every part of that last line is misleading.** The form was *fully satisfied* — `invalid fields: 0`. It was not too large; its narrowed payload had been 1,491 chars. What actually happened is that the submit produced no outcome at all, narrowing then found nothing to narrow, the code fell back to sending the whole 43,672-char document, and #105's size ceiling refused it. The size check was the last thing to touch the job, so it named the outcome.

**Distinct from #99 and #104**, which cover the same "no outcome" state *when a bot-protection frame is present*. Here the inbox showed **no Greenhouse email** and the page carried **no provider frame**, so neither explanation applies and the true cause is still unknown. That is precisely why it needs its own name rather than borrowing one that fits badly.

**Why a wrong reason is worth fixing on its own.** #83 diagnosed an oversized payload and was correct about the size while being wrong about the cause; #93 later established the payload was a *symptom* of a security-code gate. A manual-review entry is something a human acts on, and one that says "too large for the local model" invites exactly the wrong follow-up — tuning context limits — for a job whose form was already complete.

**Fix.** `ErrSubmitProducedNoOutcome` is returned when nothing is flagged invalid **and** the previous verdict was budget exhaustion, before the whole-form fallback runs. That also saves the wasted cycle: there is nothing for the model to fix, so sending the entire document could only burn inference and then be refused.

Registered in `manualReviewErrors`, which is the structural guarantee **#84** added after a sentinel shipped with no routing branch and stranded a job's documents. A test pins that it routes to manual review, that the wrapped form still does, that it is **not** conflated with `ErrFormTooLargeForModel`, and that the message states what actually happened.

### 107. A checkbox the model deliberately declined was recorded as uncommittable (Resolved 2026-07-26)

**Visible only because #97 logs the attempted value.** Without it this reads as an ordinary uncommittable field:

```
03:12:53 Attempt 3: 1 fix(es) reported success but left the control empty (autocomplete/combobox suspected):
         input[id='question_8242451101[]_54236360101'] (tried "No")
03:13:01 Sporty Group needs manual completion — queued for manual submission:
         a required field could not be committed with the configured value: ... (tried "No")
```

**The two halves disagree.** `applyValidationFix` handles a checkbox correctly: a value of `false`/`no`/`0`/`unchecked` calls `Uncheck`, everything else calls `Check` — with a comment that an explicit negative "must not silently tick it". So `"No"` did exactly the right thing.

Then `verifyFixLanded` re-reads the control with the generic *does it hold a value* check, sees `checked=false`, and reports **not landed**. That routes to the combobox-commit fallback (it is not a combobox), fails, and lands in `ErrUncommittableField` → `MANUAL_REQUIRED`.

**So a correct answer is recorded as a failure, on every checkbox the model declines.** The verification never knew what was asked for; it only knew what the control now holds, and for a deliberately-unticked box those are the same thing but mean opposite outcomes. Third instance this session of a check reading state without the intent behind it (#102's stale `aria-invalid`, #103's `id|label`, now this).

**Fix.** `verifyFixLanded` takes the intended value. When that value is negative *and* the control is a checkbox or radio, unchecked **is** the requested state and counts as landed. The negative set is extracted into `isNegativeCheckboxValue`, now the single source of truth for both the action and its verification, so the two halves cannot drift apart again — which is what caused this.

**Guarded against over-matching.** A test pins that `"Nope, I have no objection"`, `"none of the above"` and `"November"` do **not** read as negative. Exact-match on the trimmed, lowercased value only: a substring rule would silently untick real answers, which is worse than the bug being fixed.

**Consequence.** Sporty Group is the clearest case — it reached **11 invalid → 3** and was sent to manual over a checkbox it had answered correctly. Documents were preserved and the field and value named, so the outcome was safe; it was simply wrong.

### 106. A bare bracketed checkbox-group id got no fallbacks at all — the third shape of #73 (Resolved 2026-07-26)

Caught live on Sporty Group, on the one line that names how many selector forms were attempted:

```
03:09:20 Validation fix for "question_8242451101[]_54236360101" failed:
         selector matched no element (tried 1 form(s) of "question_8242451101[]_54236360101")
```

**`tried 1 form(s)`** is the tell. A bare identifier normally gets five candidate forms; a `tag#id` selector gets three. One means the selector was used verbatim and nothing else was attempted.

**Root cause.** `resolveFieldLocator` branches on `looksLikeCSSSelector`. Greenhouse names checkbox-group controls `question_8242451101[]_54236360101`, and the `[`/`]` alone are enough to make that predicate true — so the bare-identifier fallbacks are skipped. It then tries `splitTagID`, which finds no `#` to split on, so **that** branch adds nothing either. The result is a selector that is simultaneously "too CSS-like" for the identifier path and "not CSS enough" for the tag path, and it falls through both with zero fallbacks. It is also not valid CSS for an id, so the verbatim attempt matches nothing.

**Third shape of a single defect.** #73 fixed `input#430` (an id starting with a digit, used verbatim). #92 fixed `#question_...[]_...` (bracketed, with a `#` prefix, blocked because `splitTagID` refused bracketed ids). This is the same control class again with **no prefix at all** — the one arrangement neither previous fix covered.

**Fix.** When the selector looks like CSS but has no `tag#id` to split, append the attribute forms built from the whole selector. Safe for genuine CSS selectors: the candidates are appended *after* the verbatim attempt, and an attribute form built from a real selector simply matches nothing. A test pins that `input[type='email']` still resolves on the first candidate and tries nothing else.

**Consequence.** This was the remaining blocker on Sporty Group, which reached **11 invalid → 4** with three of the four survivors being exactly these bracketed ids. Verified failing against the old code before the fix was kept.

### 105. The 45-minute time budget counted bytes to read, not answers to generate (Resolved 2026-07-26)

The single most expensive failure mode in this pipeline, recurring after #83 was supposed to have closed it:

```
01:46:01 Narrowed validation retry to the rejected fields only (79608 -> 19481 chars); still invalid: <34 fields>
01:46:03 SolveValidationErrors API Call #16 executed. Payload length: 30477 characters.
02:31:03 Auto-Submit failed for Remote: ... context deadline exceeded
```

**45 minutes exactly**, then nothing. And the payload was **30,477 chars against a 40,000 ceiling** — it passed the guard #83 added specifically to prevent this.

**Why #83's model was incomplete.** It derived the ceiling from *reading* cost: ~7 tok/s × ~2.5 chars/token ≈ 17.5 chars/s, so 45 minutes buys ~47,000 chars, and 40,000 was set as the margin. That accounts for the prompt going in. It does not account for the answers coming out — and `SolveValidationErrors` must generate a value for **every** rejected field.

Three live data points on this hardware separate cleanly on field count, not size:

| job | payload | fields | outcome |
| --- | --- | --- | --- |
| ClickHouse | 11,140 | 3 | ~7 min, completed |
| Reddit | 18,639 | 13 | ~15 min, completed |
| **Remote** | **30,477** | **34** | **45 min, timed out** |

Remote's payload is 1.6× Reddit's, but its field count is 2.6× — and it did not merely take longer, it failed to finish at all.

**Fix.** `exceedsRetryTimeBudget` adds a field-count ceiling (**20**) alongside the character ceiling, and the character ceiling drops **40,000 → 28,000**, below the payload that was observed to fail. A retry over either limit routes to `ErrFormTooLargeForModel` → `MANUAL_REQUIRED`, which preserves the tailored documents for a human instead of spending 45 minutes to preserve nothing.

Applied to the **retry** call site only. The initial `ExtractFormMapping` path keeps the size-only guard: it is not answering a list of rejected fields, so the field-count reasoning does not apply to it, and tightening it without evidence would refuse forms that currently work.

A test pins the ceiling below the observed 30,477-char failure, so any future widening has to argue with the measurement rather than silently regress past it — the same treatment #83's own corrected test case got.

**Known limitation, stated rather than hidden: the field ceiling of 20 is interpolated, not measured.** The evidence brackets it loosely — 13 fields completed in ~15 min, 34 fields did not complete in 45 — and there is **no data point between 13 and 34**. So a form with, say, 22 fields might well have succeeded and will now be routed to `MANUAL_REQUIRED` instead. That is the deliberate direction to err: a wrongly-refused form costs one manual completion with its documents intact, while a wrongly-accepted one costs 45 minutes of the machine's only inference capacity and produces nothing. The number should be revisited the first time a refused form's real field count and timing can be measured, and this note is the reason to revisit it rather than treat 20 as established.

### 104. A captcha-swallowed submit hid behind stale invalid flags, so #99 never fired (Resolved 2026-07-26)

**Predicted before it was seen, then confirmed by measurement** — which is worth noting because the previous prediction this session (#103's causal claim) was wrong and had to be retracted.

After #102, the reasoning was: a reCAPTCHA-swallowed submit leaves the page untouched, so the previous attempt's `aria-invalid` markers persist, so the verdict settles as `reasonFieldsFlagged` at the 8s floor and **never reaches budget exhaustion** — which is the only state #99's bot-protection branch tests. Same structure as #102, with a captcha in place of the code gate.

The next run produced exactly that, on Reddit job `7956443`:

```
00:51:48 Attempt 2 committed 3 autocomplete selection(s): #question_67179376, #question_67179377, #question_67179378
00:51:48 Attempt 2 applied 5/5 validation fix(es)
00:51:56 Submit verdict after 8s: page re-rendered with fields flagged invalid (url moved: false, invalid fields: 5, page 140544 chars)
00:51:56 Rejected despite being set last attempt: question_67179374 = "company website";
         question_67179375 = "Stellantis Financial Services"; question_67179376 = "Yes";
         question_67179377 = "No"; question_67179378 = "I agree"
```

Every value is sensible and correctly committed. The identical five come back flagged. And the page is **byte-for-byte unchanged** — 140544 chars on this run and on the previous one, 133352 at both initial submits. A server that had processed and re-rejected the form would not return the same bytes twice.

**This also cleanly confirms #103's fix and its retraction.** The values are now human-readable labels (`"Yes"`, not `react-select-…-option-0|Yes`), so #103 works — and the rejection is *unchanged*, so #103 was correctly retracted as the cause.

**Fix.** When every still-rejected field was successfully written by the previous attempt, there is nothing left for the model to fix; the form is rejecting values it already holds. Combined with a bot-protection widget on the page, that is a swallowed submission, and it now returns `ErrCaptchaBlocked` at the top of the retry rather than spending a third ~10-minute model call to re-answer questions that were already answered correctly.

**Deliberately requires ALL of them, not merely some.** A single genuinely-bad answer among several is an ordinary validation failure and keeps its remaining retry; a test pins that one unset field prevents the captcha verdict.

**Correction, and it strengthens the constraint rather than weakening it.** This entry originally justified that with "every Greenhouse page carries reCAPTCHA". That was an assumption, and it is **wrong**. Measured across three boards the same night:

| board | reCAPTCHA frame | submit outcome |
| --- | --- | --- |
| `greenhouse.io/reddit` | **present** | blocked |
| `greenhouse.io/clickhouse` | **absent** | accepted |
| `greenhouse.io/akuity` | **present** | **accepted** (code email 23:40:07) |

reCAPTCHA Enterprise is score-based, so **Akuity carries the widget and submits fine**. The claim was wrong; the caution it was defending is now *empirically* proven right, which is a better footing than the assumption gave it. Presence of a provider frame means nothing on its own — only the conjunction with "nothing left for the model to fix" is evidence.

**Follow-up defect found by that same measurement, before it caused harm.** Akuity is precisely the case that breaks this: an accepted submission whose post-acceptance click times out (`element is not enabled`) on a page that *does* carry reCAPTCHA. Because #104's check sits **above** #93's security-code handling in the retry loop, it would have labelled an accepted application `BLOCKED_CAPTCHA`. That is #102's rule — acceptance outranks any rejection signal — reintroduced by the very fix written after learning it. The condition now tests `DetectSecurityCodeChallenge` itself rather than relying on ordering, with a test pinning both directions.

Same discipline as #99's iframe-`src` narrowness, for the same reason: #45/#46 were captcha false positives that killed most jobs on this platform.

### 103. #98 showed the model react-select's internal option ids, and it answered with them (Resolved 2026-07-26)

**#100's diagnostic caught this within one cycle of #100 itself shipping** — the fourth time an observability fix in this session paid for itself immediately (#80, #96, #97, now #100).

The first job to run under the new binary produced:

```
00:31:55 Rejected despite being set last attempt:
  question_67179374 = "company website";
  question_67179375 = "Stellantis Financial Services";
  question_67179376 = "react-select-question_67179376-option-0|Yes";
  question_67179377 = "react-select-question_67179377-option-1|No";
  question_67179378 = "react-select-question_67179378-option-0|I agree"
```

The model answered three fields with **react-select's internal DOM option ids**.

**Root cause, and it is mine.** `readComboboxOptions` deliberately returns each entry as `"id|label"` — `pickComboboxOption` needs the id so it can click the right option, which is how #79's "never commit the wrong entry" guarantee is enforced. #98's `enumerateComboboxOptions` reused that helper and rendered its output straight into the prompt, so the block told the model that `react-select-question_67179376-option-0|Yes` was a permitted value. It copied it exactly, as #98 instructed it to ("copied exactly, character for character").

So #98 — the fix whose whole purpose was to stop the model guessing wording it had never been shown — was **showing it wording no human could choose**, on every combobox, from the moment it shipped.

**Fix.** `optionLabel` strips the `id|` prefix, so only the human-readable text reaches the prompt. Entries with no separator (Lever's shape) pass through unchanged, and a label that itself contains a pipe keeps everything after the *first* separator. Two tests: one on the splitting, and an end-to-end one asserting no `react-select-` or `option-N` string can appear in the generated block at all.

**Correction, made the same night: this is a real defect but NOT proven to be the cause of that job's rejection.** After filing, I read `pickComboboxOption` properly. It splits each entry on `|` and matches the model's value against the **label** bidirectionally — `strings.Contains(text, wantN) || strings.Contains(wantN, text)`. Normalisation strips punctuation, so `react-select-question_67179376-option-0|Yes` becomes a string *ending* in `yes`, and the reverse-containment arm still matches the `Yes` option. The same holds for the `No` and `I agree` cases. **So the correct option may well have been committed regardless**, and something else is rejecting those five fields.

What remains unambiguously true: the model was being handed internal identifiers as if they were permitted values, which is wrong on its own terms, defeats the stated purpose of #98, and only worked by accident of a containment rule that was never designed to carry it. The fix stands. The causal claim does not, and the live re-run is what will settle it.

**Why it was invisible before.** #97 names values only for fields that fail to land; these three *did* land (`committed 3 autocomplete selection(s)`), because `setComboboxValue` types the value, gets zero matches, and #91's clear-and-re-read then commits *something*. Only #100 — written for exactly the "reported as set, rejected anyway" case, and shipped before its own root cause was known — could surface the value that was actually written.

### 102. #95's early exit read stale invalid flags and called four accepted submissions failures (Resolved 2026-07-26)

**A defect in my own #95 fix, found the same way #93 and #95 were: by reading the inbox rather than the logs.**

Four Greenhouse security-code emails exist for 2026-07-25/26. Lined up against the log, the timestamps are decisive:

| code email (EDT) | job | log |
| --- | --- | --- |
| 16:58:03 | Surt AI | the original #93 case |
| 21:15:58 | ClickHouse | `21:15:58 applied 3/3` → `Submission failed validation` |
| **23:40:07** | **Akuity** | `23:40:05 applied 7/7` → **`23:40:08 verdict: invalid fields: 7`** |
| **00:05:34** | **ClickHouse** | `00:05:34 applied 3/3` → **`00:05:36 verdict: invalid fields: 3`** |

Akuity's email is timestamped **between** its submit click (~23:40:06) and its verdict (23:40:08). ClickHouse's is stamped the **same second** as its submit. In both cases the server had already accepted the application before the agent declared it failed.

**Root cause, and it is mine.** #95 replaced an instantaneous read with a bounded poll, and treated "fields flagged `aria-invalid` past a 2s floor" as *positive evidence of rejection* — reasoning that a re-rendered form with flagged fields proves the server answered and refused. That reasoning is wrong on Greenhouse: it accepts the submission, issues the code challenge, and **leaves the previous attempt's `aria-invalid` markers in place** while the challenge renders. Both signals are true simultaneously, and #95 was reading the stale one.

**This is the same trap, third time.** #76 read `el.value` that was really the artifact of typing; #81 read `[data-value]` that was really the search text; #102 reads `aria-invalid` that is really the previous attempt's leftover. Each time a signal that looked like evidence was a residue of the prior step. That the pattern recurred *in a fix written specifically to stop misreading post-submit state* is the part worth remembering.

**Why #93's detector never rescued it.** `DetectSecurityCodeChallenge` runs at the top of the *next* attempt. ClickHouse's attempt 3 began 2s after the submit and went straight to `SolveValidationErrors`, so the challenge had not rendered even then. The detector was correct; it was asked too early, twice.

**Fix.**
1. The security-code gate is now tested **inside** the verdict, on every poll, and **before** the flagged-field branch — acceptance beats a stale rejection marker by construction, not by timing luck.
2. `submitOutcomeSettleFloor` raised **2s → 8s**. Akuity's verdict at 2.2s missed a challenge that had already been issued; the floor now leaves room for one that renders late.
3. A gate verdict is routed to the existing #93/#32 path (retrieve the emailed code, enter it, resubmit) and explicitly **not** to #99's bot-protection branch, which would otherwise mislabel an accepted submission as captcha-blocked.

The rejection signal is deliberately preserved when no gate is present — a test pins that, because removing it entirely would leave every genuine validation failure exiting on budget exhaustion, which #99 maps to `BLOCKED_CAPTCHA` on any page carrying reCAPTCHA. That interaction would have traded one misreading for another. 4 new tests.

**Consequence.** The pipeline submitted **four** applications today and recorded all four as failures. "0 confirmed `APPLIED`" was never a submission problem; it was this.

### 101. A submit click that timed out reported nothing about what blocked it (Resolved 2026-07-25)

Three separate jobs ended 2026-07-25 with the identical, uninformative line:

```
23:48:08 [Worker-1] Auto-Submit failed for Akuity: playwright: timeout: Timeout 30000ms exceeded.
```

`grep` confirms the count: **Akuity, Nova and Zimperium**, each after a full set of validation fixes had been applied, each written off as a generic `FAILED_SUBMIT`. Akuity's is the clearest waste — attempt 3 applied 7/7 fixes and then spent the whole 30s action timeout failing to click.

**A timeout says the click never landed. It says nothing about what stopped it** — a disabled control, an off-screen one, a consent banner over the top (#34's shape), or a bot-protection frame (#99's). Those need different responses and were indistinguishable.

The 07-21 journal had already guessed at Nova's: *"most plausibly an hCaptcha overlay after repeated submits."* A guess is what this replaces.

**Fix.** On a failed submit click, read the control's actionability directly via `elementFromPoint` at its centre — the same check a read-only probe used to *clear* Reddit's button in #99 — and log it:

```
[Auto-Submit] Submit click failed for Akuity (timeout ...); submit control: disabled=false inViewport=true, covered by DIV#onetrust-consent-sdk.banner
```

Naming the covering element turns three indistinguishable timeouts into three separable causes. When an iframe covers it, its `src` is included, so a challenge frame identifies itself.

**Routing stays evidence-led.** `ErrCaptchaBlocked` is returned only when a provider frame is genuinely embedded, reusing #99's narrow `src` matcher. #45/#46 were CAPTCHA false positives that killed the large majority of Greenhouse/Lever/Ashby/Workable jobs before fit-scoring, so nothing here infers a captcha from a timeout alone — the timeout and the widget must both be present, and the message states both facts rather than asserting one caused the other. The original Playwright error is preserved in the wrapped message either way.

The probe is best-effort: a page that cannot be evaluated yields no description and the original error is returned unchanged, so a diagnostic can never break the failure path it exists to explain. 5 new sub-cases plus a best-effort test.

### 100. A field that lands and is rejected anyway had no diagnostic at all (Resolved 2026-07-25)

Akuity produced a signature none of the existing diagnostics could speak to:

```
23:40:05 Attempt 2 applied 7/7 validation fix(es) to: input#question_6039579009 ... textarea#question_6051662009
23:40:08 Submit verdict after 2.2s: page re-rendered with fields flagged invalid (url moved: false, invalid fields: 7, page 126557 chars)
23:40:08 Narrowed validation retry ... still invalid: question_6039579009, ... question_6110764009   [the identical 7]
```

**No not-landed line**, so `verifyFixLanded` reported all seven controls as genuinely set — and the form rejected the same seven anyway. #97 names the attempted value only when a fix *fails to land*; the opposite case had no diagnostic whatsoever, so the log could not say what had been written or why it was refused, and the loop could only re-guess.

**Probing ruled out every convenient explanation.** All seven are plain required `INPUT`/`TEXTAREA` (`LinkedIn Profile*`, `Github*`, three free-text questions), each a **single** DOM match, no `pattern`, no `minLength`. The React-controlled-input trap does **not** apply either — reading React's own props alongside the DOM after a plain `Fill()`:

| control | DOM value | React prop value |
| --- | --- | --- |
| `question_6039579009` (input) | `https://linkedin.com/in/probe` | **same** |
| `question_6110764009` (input, real keystrokes) | `https://github.com/probe` | **same** |
| `question_6051659009` (textarea) | set correctly | uncontrolled — DOM is the source of truth |

So React observes `Fill()`, and the values genuinely land. One structural note worth keeping: these controls have **no `name` attribute**, so `FormData` serialises nothing for them — Greenhouse submits from React state, which is why "the DOM says it is set" is not by itself proof the submission carries it.

**What the diagnostic adds.** `rejectedDespiteLanding` pairs each still-rejected id with the value the previous attempt wrote into it:

```
[Auto-Submit] Rejected despite being set last attempt: question_6039579009 = "https://..."; question_6051659009 = "Ran production Kubernetes..."
```

Matching is by suffix because the two sides name controls differently — the model emits `input#question_1`, `#question_1` or `input[id='question_1']`, while `parser.InvalidFieldIdentifiers` reports the bare id (the same selector-shape problem as **#73** and **#92**). A test pins that `question_60395790091` and `question_6039579` do **not** match `question_6039579009`, so a prefix collision cannot mis-attribute a value. Free-text answers are whitespace-collapsed and truncated so one answer stays on one log line.

**Fourth instance of one lesson** (#80, #96, #97, now #100): every expensive failure in this pipeline is *"the mechanism reported success and the outcome was failure"*, and each time the fix has been to log the evidence a decision rested on. The root cause of Akuity's rejection is **still open** — this makes the next occurrence diagnosable rather than guessing now.

### 99. A submit silently swallowed by reCAPTCHA was reported as an ordinary validation bounce (Resolved 2026-07-25)

**Found by an outcome that could not be either of the two things the code knew about.** With #98 shipped, Reddit's form was fully satisfied for the first time in the entire effort:

```
23:19:28 Attempt 2 committed 9 autocomplete selection(s): 430, 431, 432, 433, 434, 436, question_67942418, question_67942419, question_67942420
23:19:28 Attempt 2 applied 13/13 validation fix(es)
23:19:43 Submit verdict after 15.1s: no confirmation and no rejection evidence within the settle budget (url moved: false, invalid fields: 0, page 169636 chars)
```

`invalid fields: 0` with no confirmation and no rejection, after a full 15s wait. **#95's budget-exhaustion branch existed precisely to represent this state honestly instead of guessing**, and #96's line is what made it legible.

**The inbox discriminated the two cases.** ClickHouse's accepted submit produced a Greenhouse security-code email *in the same second*; Reddit's produced **nothing**. So Reddit's request never reached the server, where ClickHouse's plainly did.

**Read-only probe of the submit control** (never clicking it — that files a real application):

| check | result |
| --- | --- |
| matches for `button[type='submit'], input[type='submit']` | **1** |
| identity | `BUTTON type=submit "Submit application"`, 190×40 |
| visible / enabled / inside a `<form>` | yes / yes / yes |
| `document.elementFromPoint` at its centre | **the button itself — nothing covering it** |

That rules out **#87** (a decoy control winning precedence) and **#34** (an overlay intercepting the click). What the probe did find:

```
https://www.recaptcha.net/recaptcha/enterprise/anchor?ar=1&k=6LfmcbcpAAAAAChNTbhUShzUOAMj_wY9LQIvLFX0
```

**reCAPTCHA Enterprise, score-based and invisible** — no `.g-recaptcha` marker, just the anchor frame. A headless Chromium scores badly and the submission is discarded client-side: no error, no navigation, no request, no email. Every observation follows from that, including why ClickHouse (no such widget) went through.

**Fix, and what it deliberately is not.** Solving CAPTCHAs is `improvements_paywall.md` #17 — paid, user-gated, and out of scope. What is free is *reporting the truth*: when a submit exhausts the settle budget with no outcome **and** the page carries a live bot-protection widget, that is `ErrCaptchaBlocked` (already wired to `BLOCKED_CAPTCHA`), not a validation bounce. Previously this burned another ~15-minute model call, then fell back to the whole form and landed in manual review via #83's size ceiling — with a reason that named the wrong cause entirely.

**The detection is narrow on purpose, and the narrowness is the point.** It matches the **iframe `src`** of known providers, never page wording. Bugs **#45/#46** were CAPTCHA false positives from phrase matching, and between them they killed the large majority of Greenhouse/Lever/Ashby/Workable jobs before they ever reached fit-scoring. A false positive here costs a real application, so this only reports what it can point at, and it only runs *after* a submit has already produced no outcome — it can never pre-empt a working job. The provider pattern is a single Go constant interpolated into the browser check **and** compiled in the test, so the test exercises the real pattern rather than a copy that can drift. 9 sub-cases, including a `google.com` URL that is not reCAPTCHA.

**Consequence worth stating plainly:** Reddit is not completable by this pipeline for free. It is now correctly labelled `BLOCKED_CAPTCHA` in ~15 seconds instead of ~30 minutes.

### 98. The model was never shown a dropdown's permitted values, so it guessed the wording (Resolved 2026-07-25)

**The last-mile blocker, and #97's diagnostic produced it in exactly one cycle** — the same way #80 paid for itself.

Reddit got to a single remaining invalid field and then stalled on it twice with the value now visible:

```
22:41:35 Attempt 2: 1 fix(es) ... left the control empty: 434 (tried "I am not a protected veteran")
22:42:39 Attempt 3: 1 fix(es) ... left the control empty: #434 (tried "I am not a protected veteran")
```

The **identical** wrong value on both attempts — the model is deterministic here and would never have converged, however many retries ran.

**Root cause, established by probe.** `#434` is *Are you a veteran/have you served in the military?* and offers:

> Active Reserve · Inactive Reserve · Other Protected Veteran · Retired · Unspecified Veteran · Vietnam Era Veteran · Vietnam Veteran and Other Protected Veteran · No military service · I don't wish to answer

None of those strings appear in the page HTML until the widget is opened:

| string | in page HTML before opening | after opening |
| --- | --- | --- |
| `Other Protected Veteran` | **false** | true |
| `No military service` | **false** | true |
| `I don't wish to answer` | **false** | true |

Opening one widget grows the document 144,506 → 146,812 chars, and the form contains **zero native `<select>` elements** — Greenhouse is react-select throughout, so `<option>` text is never in the served markup. The narrowed validation payload is built from that markup. **The model was being asked to supply a value for a control whose permitted values it is never shown**, leaving it to invent plausible wording. `"I am not a protected veteran"` is a perfectly reasonable guess; it just is not on the menu, and typing it filters the list to nothing.

**This explains the shape of the entire day.** Yes/No fields committed reliably because they are trivially guessable. Measured over the window since #91/#92 shipped: **14 autocomplete commits succeeded and 2 distinct fields failed** — `#434` and Sporty Group's `#question_7849575101` — and both failures are this same unusual-wording case. Nothing else in that window failed to commit.

**Fix.** `enumerateComboboxOptions` opens each invalid control that is genuinely a combobox, reads its real option list with **no query typed** (bugs.md #91: typing filters the list, and an unrecognised query filters it to nothing — the very state this is meant to reveal rather than reproduce), closes it again, and renders a block into the prompt naming the exact permitted values and instructing that they be copied character for character.

**Wired into both prompt paths**, the validation retry *and* `ExtractFormMapping`. That is deliberate: the standing check in this backlog records three prior instances of a capability added to one path and not the other (#65/#66→#67, #74→#75, #28→#31), and this is the same shape.

**The `isComboboxLocator` gate is a correctness requirement, not an optimisation.** The invalid-field list routinely includes checkboxes — Greenhouse's GDPR consent among them — and clicking one would *toggle* it, silently changing the answer this function exists to get right. A test pins that a non-combobox is never clicked.

Bounded at 25 controls and 40 options each, and the block is added *before* `likelyExceedsModelContext` runs, so the #83 time-budget ceiling still accounts for it. 3 new tests.

### 97. An uncommittable field named the control but never the value that was tried (Resolved 2026-07-25)

**Found at the closest the pipeline has ever come to finishing.** Reddit (fit 90, the job that opened this whole investigation) went from 13 invalid fields to **one**, with the narrowed payload collapsing **7,212 → 497 chars**:

```
22:11:40 Attempt 2 committed 8 autocomplete selection(s) ... #430, #431, #432, #433, #436, #question_67942418, #question_67942419, #question_67942420
22:11:40 Attempt 2: 1 fix(es) reported success but left the control empty ... #434
22:11:43 Narrowed validation retry to the rejected fields only (54218 -> 497 chars); still invalid: 434
22:12:39 Reddit needs manual completion ... a required field could not be committed with the configured value: #434
```

Eight autocompletes committed, including both legal attestations. One field stood between this and the first confirmed application of the entire effort, and the log said only that `#434` came back empty.

**That is not enough to act on.** "Left the control empty" is consistent with two opposite situations: the commit machinery is broken for this widget, or the machinery works fine and was handed a value the widget does not offer. The first needs a code fix, the second needs different data or a different prompt. Nothing in the log separated them.

**A probe settled it — the mechanism is fine.** `#434` is *Are you a veteran/have you served in the military?*, a react-select offering:

> Active Reserve · Inactive Reserve · Other Protected Veteran · Retired · Unspecified Veteran · Vietnam Era Veteran · Vietnam Veteran and Other Protected Veteran · No military service · I don't wish to answer

Typing `I don't wish to answer` filters that list to **exactly that one entry**, so the control is genuinely selectable and #90/#91's sole-option path would commit it. Two other plausible phrasings (`Prefer not to say`, `I am not a protected veteran`) filter it to **zero** — the #91 shape, where the typed query eliminates every option. So this is a **value mismatch**, and the missing datum was always the value.

**Fix.** The not-landed entry now carries it: `#434 (tried "…")`. It flows into the `ErrUncommittableField` message too, so the manual-review entry and the log both name the exact string that failed. 1 new test.

Deliberately not done in the same change: making the model's answer converge on the offered wording. That is a real follow-up, but choosing it blind — before a single log line shows what the model actually proposes — is how #90 shipped a rule that #91 proved could never fire. Measure first.

**Third instance of the same lesson in one session** (#80, #96, #97): this pipeline's expensive failures are all *"the mechanism reported success and the outcome was failure"*, and each time the fix has been to log the evidence a decision rested on rather than the decision itself.

### 96. Nothing recorded what a submit verdict was actually decided on (Resolved 2026-07-25)

**Filed the moment its absence had cost a day.** The only trace of a judged submit was `Submission failed validation. Retrying...`. That line says a verdict was reached and nothing about the evidence behind it — not how long the code waited, not whether the URL moved, not how many fields came back flagged, not how large the returned page was. #95 was consequently findable only by noticing that four *unrelated* log lines shared a wall-clock second, then reasoning backwards from that coincidence.

`awaitSubmissionOutcome` now emits one line per submit click at the moment it settles:

```
[Auto-Submit] Submit verdict after 2s: page re-rendered with fields flagged invalid (url moved: false, invalid fields: 13, page 54537 chars)
```

Every field in it distinguishes cases that were previously indistinguishable: elapsed time separates a premature read from a settled one, `url moved` separates an in-place re-render from a navigation, the flagged count separates "the same fields again" from "different fields now", and page size catches the #83/#93 oversized-payload shape.

**Confirmed working within minutes of shipping**, on Reddit: `Submission failed validation (page re-rendered with fields flagged invalid)` at 21:56:22 against a fill completing at 21:56:20 — two seconds, not the same second, and settled on positive rejection evidence rather than an empty read. That is #95's settle floor behaving exactly as designed, and it is visible in the log rather than inferred.

Same class as **#80**, which was also filed the moment the diagnostics ran out and paid for itself within one cycle. The standing lesson is that this pipeline's expensive failures are all "the mechanism reported success and the outcome was failure", and the only defence is logging the evidence a decision was made on, not just the decision.

### 95. The submit verdict was read from the DOM the instant the click returned, racing the submission itself (Resolved 2026-07-25)

**Three independent jobs produced the identical impossible signature.** ClickHouse:

```
21:15:58 Attempt 2 committed 2 autocomplete selection(s) ... #question_15561491004, #question_15653623004
21:15:58 Attempt 2 applied 3/3 validation fix(es) to: ...
21:15:58 Submission failed validation. Retrying...
21:15:58 Narrowed validation retry ... still invalid: <the same three fields>
```

Stack AV reproduced it exactly at 21:36:22 (`committed 4`, `applied 4/4`, `Submission failed validation`, all in one second), and Sporty Group had done the same earlier in the day.

**The fill was ruled out by probe, not by argument.** Against ClickHouse's real form, driving the agent's own commit sequence (click, clear, type, pick the option scoped via `aria-controls`):

| field | kind | result |
| --- | --- | --- |
| `question_15561491004` (sponsorship) | react-select | committed `"No"` |
| `question_15653623004` (AI-evaluation consent) | react-select | committed `"Yes"` |
| `question_15561492004` (current location) | plain text | holds `"Macomb, MI"` |

and then, critically, native constraint validation over the form containing `#first_name`: **`formValid: true`, `invalidCount: 0`**. Before the commits it was `false` with 2 invalid controls — react-select's hidden `input.requiredInput` proxies, which carry **no id and no name**, and which react-select *removes from the DOM* once a value is selected. So the agent's commits fully satisfy these forms client-side. The fill machinery is not the problem; the verdict is.

**Root cause.** Both the initial path (`confirmOrError`) and the retry loop judged the click from a single page read taken immediately after:

```go
page.WaitForLoadState(networkidle, 10s)
currentURL := page.URL()
pageContent, _ := page.Content()
isSubmissionConfirmed(...)
```

Playwright's `Click` returns when the event is dispatched, not when the application reacts. At the moment `WaitForLoadState` is called there is frequently no request in flight yet, so the page already counts as idle and the wait returns at once. The DOM is then read **before the submission has happened**, shows the form still present and unchanged, and is scored as a validation failure. That is exactly why all four log lines share one second: there is no network round-trip in between because none had started.

**#93 is direct live evidence this misfires.** It found a Greenhouse security-code email timestamped *the exact second of a submit the agent had written off as failed* — the submission reached Greenhouse's servers while the agent concluded it had not.

**Fix.** `awaitSubmissionOutcome` replaces the single read with a bounded poll, and the per-poll rules live in a pure `decideSubmissionOutcome` so they are unit-testable — branching inside the driver loop is what let #76 ship inert:

1. Confirmation wins at any elapsed time; a thank-you view rendering in 200ms is as real as one taking 8s.
2. Past a 2s settle floor, **either** fields flagged `aria-invalid` **or** explicit validation-error wording settles it as rejection — both are positive evidence the server answered. Both forms are needed: the theme in #83 sets no `aria-invalid` but still renders error text, and a form flagged only via `aria-invalid` may carry no wording. Before the floor, neither counts: that is just the pre-submit DOM.
3. Otherwise keep waiting, to a 15s budget.

The change is deliberately one-directional: it can turn a premature "failed" into a correct "confirmed", never the reverse. 6 new tests. The timings are `var`s so the genuine no-evidence-either-way case — which now correctly waits out the full budget — does not make the suite sit through 15s.

**Stated plainly: the race is inferred, not directly observed.** It is consistent with every symptom (the same-second timing, forms proven satisfiable, payloads unchanged between attempts, #93's email, #89's late-render finding), but confirming it directly would require clicking submit on a live posting, which files a real application. Treat the mechanism as strongly supported rather than proven, and watch for the first `Submission confirmed` to settle it.

### 94. The dedup row was written at document generation, so a job that never submitted was skipped forever (Resolved 2026-07-25)

**Found in the restart log, in four lines that looked like routine housekeeping.** The 21:16 relaunch reported:

```
21:16:42 [Worker-1] Duplicate check: Already applied to Reddit. Skipping.
21:16:43 [Worker-1] Duplicate check: Already applied to Akuity. Skipping.
21:16:43 [Worker-1] Duplicate check: Already applied to ClickHouse. Skipping.
21:16:43 [Worker-1] Duplicate check: Already applied to Staff Site Reliability Engineer. Skipping.
```

Every one of those four is a job this same day's log shows *failing*. ClickHouse is the cleanest case: its dedup row is timestamped `21:08:15.789`, which is the exact second `SaveApplication` ran for it, and the process was killed at 21:16 while it was still mid-attempt-3. It had never submitted anything.

**Root cause.** `SaveApplication` ended with `return RecordApplicationInDB(...)`, so the `applied_jobs` row — the sole thing `HasApplied` gates on — was written at **document-generation** time, minutes before the first submit click and regardless of its outcome. Generating documents is not applying.

**Why it was permanent rather than merely wrong.** On its own, a spurious dedup row would only matter if the job were re-queued. Three separate mechanisms re-queue it, all added or exercised the same day:

- the startup reaper resetting orphaned `PROCESSING` rows to `DISCOVERED` (#55),
- #85's duplicate-path reset, which deliberately returns the row to `DISCOVERED`,
- `cmd/requeue` run without `-clear-dedup`.

So the job returns to `DISCOVERED`, is loaded into the next run's queue, hits `HasApplied` → true, is skipped in milliseconds, and is reset to `DISCOVERED` again. It is queued every single run, forever, and never progresses. The funnel status reads `DISCOVERED` — indistinguishable from a job genuinely waiting its turn — so nothing in the dashboard or the status breakdown shows it as stuck. **The failure mode is silent unreachability, not visible failure.**

Measured live at the time of the fix: **7 of the 82-job cohort** and **66 rows DB-wide** sat in `DISCOVERED` carrying a dedup row.

**This is the mechanism behind #53's falsehood, seen from the write side.** #53 recorded that `applied_jobs` overstates what was applied to, and `cmd/dashboard` was already patched to count `job_funnel.status = 'APPLIED'` instead. That fixed the *reporting*. The write itself was never corrected, and `HasApplied` still trusted it — so the same bad data kept silently suppressing work. `job_funnel` has **never** contained a single `APPLIED` row across all 3,884 rows, while `applied_jobs` holds 261.

**Fix.** `SaveApplication` no longer writes the dedup row; it remains responsible for the documents folder (`MoveToManualApply` archives it) and the record the dashboard reads. `cmd/agent` writes the row on the confirmed-submission branch, next to `UpdateFunnelStatus(job.URL, "APPLIED")`, and nowhere else. `RecordApplicationInDB` became `ON CONFLICT(url) DO NOTHING`: the row is no longer written on a path guaranteed to run exactly once, and #89's confirmation re-check can legitimately observe success twice for one URL, where the `UNIQUE` constraint would have reported an error against an application that actually worked.

**Two existing tests asserted the old behaviour and were deliberately inverted**, each with the reasoning written into the test body so it reads as a correction rather than a regression: `TestSaveApplication` required `HasApplied` to be true after saving documents, and `TestApplicationsAndDuplicates` required a duplicate insert to error. Three tests total now pin the new contract, including `TestSaveApplicationLeavesJobRetryableUntilConfirmed`, which drives the full failed-then-confirmed sequence. All three were verified failing against the old code before the fix was kept.

**Operational cleanup.** The 7 stuck cohort rows had their dedup rows cleared. That was safe to assert rather than assume: all 7 dedup timestamps fall inside the current log window, and that window contains **zero** `Submission confirmed` lines, so none of them was ever submitted. The remaining DB-wide rows were **not** cleared — their timestamps predate the log, so there is no positive evidence either way, and re-applying to an employer who already has an application is an outward-facing action that is the user's call, not the agent's. Left as an open question for the user rather than silently resolved in either direction.

**Method note, and it is the recurring one.** This was found by reading a log line that announced itself as normal operation. `Duplicate check: Already applied to X. Skipping.` is what a correctly-working dedup looks like; the defect was only visible in the conjunction of that line with a company name the same log had shown failing an hour earlier. Related to the standing warning that *an absent signal is not evidence of an absent event* (#77, #84, #81) — here the inverse: **a present, benign-looking signal is not evidence of a benign event.**

### 93. Greenhouse's emailed security-code gate read as a validation error, burning the full 45-minute timeout (Resolved 2026-07-25)

**Found from the user's inbox, not from the logs** — which is the whole point of this entry. The agent's own telemetry could not see it.

The user forwarded a Greenhouse email:

> **Security code for your application to Surt AI**
> Hi William, Copy and paste this code into the security code field on your application: `uOSBQvRu`
> After you enter the code, resubmit your application.

Timestamp: **2026-07-25 20:58:03 UTC**. The surtai submit clicked at **16:58:03 local — the same second.**

**What actually happened**, versus what the logs said:

| logged | actually |
| --- | --- |
| `Submission failed validation. Retrying...` | the submit reached Greenhouse and **succeeded** |
| retry payload 50,501 chars, no `aria-invalid` | the page now showed a **security-code field** |
| `context deadline exceeded` after 45 min | the model was asked to "fix" a field only an emailed code can satisfy |

**This reframes #83.** That entry blamed a theme with no `aria-invalid` markers forcing a full-form payload. The size ceiling it added is still correct and still worth having — but the *reason* this particular form had nothing flagged invalid was that it was no longer a validation failure at all. It was a verification gate.

**Why no number of retries could ever work:** the code exists only in the applicant's mailbox. The agent has no access to it, so each attempt asked a local model to invent a value that cannot be invented — at ~12 minutes a go, then 45 for the timeout.

**Fix:** `parser.DetectSecurityCodeChallenge` checks for a code input **and** the matching page wording, and the retry loop returns `ErrNeedsEmailVerification` **before any model call** — ahead of even the attestation guard, since a code the agent cannot obtain makes everything downstream pointless. It joins `manualReviewErrors`, so #84's routing preserves the job with its documents.

**Both conditions are required deliberately.** Wording alone would strand real applications — job descriptions mention "security" and "verification" routinely — and a bare field without the wording is not evidence of a gate. Pinned by two negative tests.

**Open question for the user, not decided here:** making the agent retrieve these codes itself would need Gmail API credentials wired into `cmd/agent`. That is free but a genuine new capability with real access implications, and it is the user's call. Filed as improvements #32.

**Tests:** `TestDetectSecurityCodeChallenge_FindsTheGreenhouseCodeGate`, `TestDetectSecurityCodeChallenge_IgnoresWordingWithoutAField`, `TestDetectSecurityCodeChallenge_IgnoresAFieldWithoutTheWording`.

### 92. Checkbox-group ids contain brackets, which are CSS attribute syntax, so they resolved to nothing (Resolved 2026-07-25)

Observed live on Sporty Group:

```
Validation fix for "input#question_8242451101[]_54236360101" failed:
  selector matched no element (tried 1 form(s) of "input#question_8242451101[]_54236360101")
```

Greenhouse names checkbox-group controls with a literal `[]` in the id — `question_8242451101[]_54236360101`. As a CSS selector, `#question_8242451101[]_54236360101` is not an id: the brackets read as attribute syntax, so it matches nothing. The `[id="question_8242451101[]_54236360101"]` attribute form resolves it perfectly.

**This is the same class as #73** (a leading digit making `#430` invalid), and #73's own fix should have caught it — the attribute-form retry exists for exactly this. It did not, because `splitTagID` explicitly refused any id containing `[` or `]`. The `tried 1 form(s)` in the log is the tell: an eligible selector gets three.

**Fix:** brackets no longer disqualify. Combinators and separators still do (`#`, `.`, `>`, `:`, `,`, whitespace) — those indicate a compound selector where rewriting the tail as a single id would change the meaning.

**Tests:** `TestSplitTagID_AllowsBracketsSoCheckboxGroupIDsResolve`, `TestSplitTagID_StillRefusesCompoundSelectors`.

### 91. #90's single-option rule could never fire, because typing filters the sole option out (Resolved 2026-07-25)

**A defect in #90's own fix, caught on the very next run** — the same shape as #76 (a defect in #74) and #81 (a defect in #76's fallback).

Sporty Group re-run with #90 shipped:

```
20:30:50 Attempt 2: 1 fix(es) reported success but left the control empty
         (autocomplete/combobox suspected): input#question_7849575101
```

Unchanged. #90 selects the sole option when `len(options) == 1`, and probing had confirmed `GDPR Acknowledgement*` offers exactly one: `Acknowledge/Confirm`. But `setComboboxValue` **types the model's proposed value before reading the options**, and typing "Yes" into a widget whose only entry is "Acknowledge/Confirm" filters the list to **zero**. So the observed count was 0, never 1 — and the rule could not fire for precisely the case it was written for.

I had verified the option list by clicking the control open with **no query typed**; the agent always types first. The probe and the code were doing different things, and the difference was the whole bug.

**Fix:** when typing yields no options at all, clear the query and re-read. An empty query restores the unfiltered list, which is where the lone option lives.

**Method note:** this is the third time a fix of mine has been inert for a reason only a live run exposed (#76, #81, #91). The pattern is consistent — the probe reproduces *my* mental model of the sequence, not the code's actual sequence. Probing is still what found each one; the lesson is to replicate the code path exactly, including the order of operations, rather than the outcome I expect it to reach.

### 90. A required control with exactly one option was refused, sending a job to manual review one click from completion (Resolved 2026-07-25)

**The closest the pipeline has come to finishing.** Sporty Group (Greenhouse, fit **90**) on the full fix stack:

```
19:54:03 still invalid: gdpr_processing_consent_given_1, question_7849567101, ... (11 fields)
20:07:46 Attempt 2 committed 3 autocomplete selection(s) that Fill() alone had left empty
20:07:46 Attempt 2 applied 9/9 validation fix(es)
20:07:46 Narrowed ... (50136 -> 610 chars); still invalid: question_7849575101
```

**Eleven invalid fields down to one.** The payload collapsed from 6,389 to 610 characters. Three autocompletes committed, a GDPR consent checkbox ticked, a checkbox-group entry set — the whole machinery from #70 through #89 working together on a genuinely hard form.

Probing the survivor:

```
label:   "GDPR Acknowledgement*"
options: [Acknowledge/Confirm]      <- exactly one
```

**Root cause:** the model proposed a reasonable affirmative ("Yes", "I acknowledge", or similar) and `pickComboboxOption` requires the option text and the wanted value to contain one another. "acknowledge confirm" neither contains nor is contained by "yes", so **nothing was selected**. #88 then correctly routed the job to `MANUAL_REQUIRED`.

That caution is right when there are several options — it is exactly what stops "Detroit, ME" being filed instead of "Detroit, MI" (#79). But with **one** option there is no wrong choice available. Refusing it costs a real application on a 90-fit job that was otherwise complete.

**Fix:** when `mustContain` is empty and the control offers exactly one option, take it. **Deliberately not applied when `mustContain` is set** — those tokens exist precisely because the *identity* of the option matters, so a lone option that fails them is a wrong answer rather than an obvious one. A lone `Detroit, ME` must still be refused.

**Tests:** `TestPickComboboxOption_TakesTheSoleOptionWhenThereIsOnlyOne`, `TestPickComboboxOption_StillRefusesALoneOptionThatFailsMustContain` (the #79 guarantee survives), `TestPickComboboxOption_StillRefusesWhenSeveralOptionsAndNoneMatch`.

**Worth noting:** #88 did its job here — the outcome was a preserved manual-review job naming the exact field, not a silent `FAILED_SUBMIT`. That is what made this diagnosable in one probe.

### 89. A late-rendering confirmation page is missed, so a successful submit is retried — filing duplicates (Resolved 2026-07-25)

**Surfaced by an outcome that did not fit.** Orkes routed to `MANUAL_REQUIRED` through #83's time-budget ceiling, with a payload of **43,411 characters**. That only happens when `PruneDOMToInvalidFields` finds **nothing flagged invalid** and falls back to the whole document. But attempt 2 had just applied both outstanding fixes, including committing the Yes/No combobox. A form with nothing invalid, right after both its blocking fields were satisfied, is far more consistent with **the submit having succeeded and the form being gone** than with a form still sitting there.

**Why the success would be missed:** Greenhouse replaces the form in place rather than navigating. So `currentURL == applyURL`, and `isSubmissionConfirmed` can only return true via a confirmation *phrase*:

```go
if currentURL == applyURL { return false, reasonURLUnchanged }
```

The check runs immediately after the click, behind a 10-second `networkidle` wait. If the thank-you view renders after that — entirely plausible on a CPU-bound host running a 30B model alongside the browser — the check reads the **old DOM**, reports failure, and the loop retries.

**Retrying a submit that already succeeded means filing a duplicate application with a real employer.** That is a materially worse failure than not applying at all, and it is invisible in the logs: it looks exactly like a validation bounce.

**Fix:** re-check `isSubmissionConfirmed` at the **top of every retry attempt**, before any DOM work. It is nearly free, and it is the only thing standing between a slow-rendering confirmation and a duplicate submission. On a hit it returns success with `on re-check before attempt N — the previous click had succeeded`.

Also added a diagnostic for the ambiguity that made this unreadable: when narrowing finds nothing invalid, the log now states **whether a `<form>` is still on the page** and the payload size. "Nothing is flagged invalid" means two opposite things — *the form is fine now* or *the form is gone because it submitted* — and the logs could not tell them apart.

**Honest status:** whether Orkes actually submitted is **not established**. The evidence is circumstantial (payload size, nothing invalid, both fields satisfied) and the browser state is gone. This fix ensures the *next* occurrence is detected rather than retried; it does not retroactively prove what happened here. Orkes sits in `MANUAL_REQUIRED`, which is a safe place for it either way — a human can check whether an application already exists.

**Correction to an earlier claim in #87:** I treated "`applied N/N` and the failure in the same second" as proof that nothing was submitted. That inference was too strong — a client-side validation rejection also produces no navigation and no delay. #87 was still a genuine defect (clicking the click-to-reveal Apply button is unambiguously wrong), but that timing signature alone did not prove non-submission.

**Tests:** `TestIsSubmissionConfirmed_ConfirmsOnPhraseWithAnUnchangedURL`, `TestIsSubmissionConfirmed_StillRefusesWithNoEvidence` (bug #51's guarantee must survive).

### 88. A required widget that cannot accept the configured value was written off as a submit failure (Resolved 2026-07-25)

**This one is not a broken mechanism — it is the mechanism working and reaching an honest dead end.** Nova (Lever), re-run with #86 and #87:

```
19:22:15 Attempt 3: 1 fix(es) reported success but left the control empty
         (autocomplete/combobox suspected): input[data-qa='location-input']
```

Exactly the diagnostic #81 was built to produce: the field was correctly identified as unset, and the commit was attempted and failed. The cause is data, not code — measured directly against the live form, **Lever's geocoder returns zero results** for `Macomb`, `Macomb Township` and `Macomb, MI`, while Greenhouse's resolves `Township of Macomb, Michigan, United States` without trouble. No option exists to select, so the required hidden `selectedLocation` can never be populated, so the form can never validate.

**Why that needed a fix anyway:** the outcome was `FAILED_SUBMIT` after three full attempts. That is wrong twice over — it wastes the retries reaching a wall that was already known after the first, and it writes off a job that a human could complete in seconds, discarding its tailored documents.

**The option not taken:** substituting a nearby city the geocoder *does* know (Detroit is 25 miles away and resolves fine) would make the form submit. It would also state a false location on a real job application. Not done, and deliberately so — the same reasoning as #82's attestations.

**Fix:** `ErrUncommittableField`, added to `manualReviewErrors` so #84's catch-all routes it to `MANUAL_REQUIRED` with documents preserved and the offending selectors named in the log. The retry loop remembers the final attempt's uncommitted fields and returns this instead of the generic "failed after 3 validation error attempts" when any remain.

**Note for the user:** this is the concrete cost of a location that some ATS geocoders do not index. Greenhouse handles it; Lever does not. Since 39 of the original 82 jobs are Lever, a `pii.yaml` location the geocoders agree on would unblock a large share — but that is the user's call to make about their own address, not the agent's.

**Tests:** `TestErrUncommittableField_IsAManualReviewOutcome`.

### 87. The submit locator clicked the click-to-reveal "Apply" button, so no retry ever actually submitted (Resolved 2026-07-25)

**This is the defect that was silently invalidating the whole retry path**, and it took the previous eight fixes to expose it — until fields could actually be filled, nothing downstream of them was reachable.

Orkes (Greenhouse, fit 85) applied **`2/2` fixes** on every attempt and failed all three. The two fields were `LinkedIn Profile` (plain required text) and `Are you located in Australia or Europe?` (a Yes/No react-select). Probing proved both were satisfiable, in either order, with no interference:

```
order A: LinkedIn="https://linkedin.com/in/wylelias"  combobox="No"
order B: LinkedIn="https://linkedin.com/in/wylelias"  combobox="No"
```

The tell was in the timestamps: `applied 2/2` and `Submission failed validation` landed in the **same second**. A real submit plus a `networkidle` wait cannot complete that fast — so nothing was being submitted at all.

**Root cause:** the locator was a single CSS alternation —

```
input[type='submit'], button[type='submit'], button:has-text('Submit'), button:has-text('Apply')
```

CSS alternations carry **no precedence**; Playwright returns matches in **DOM order**. Measured on the live form:

```
[0] visible BUTTON type=button  "Apply"                      <- firstVisibleSubmit picked this
[1] visible BUTTON type=button  "Quick Apply with MyGreenhouse"
[2] visible BUTTON type=submit  "Submit application"          <- the real one
```

Every retry "submitted" by clicking the click-to-reveal **Apply** button — a `type=button` that does nothing once the form is already open. The page never changed, so the same fields stayed flagged and each attempt was a byte-for-byte repeat.

**This also re-frames earlier entries.** #70-#81 were all real and all necessary, but their fixes could never have produced a completed application on such a form: the fields were being filled correctly and then never submitted. The recurring "applied N/N and still rejected" signature that drove #72, #80 and #81 had *two* causes, and this was the second.

**Fix:** replaced the flat alternation with `submitControlSelectors`, tried in precedence order — real `type='submit'` controls first, then "Submit application", then "Submit" — with `findSubmitControl` returning the first group that has a visible match. **"Apply" is deliberately absent entirely**: it reveals a form, it never submits one, and keeping it even as a last resort would restore this bug on any form where the reveal button stays in the DOM.

**Tests:** `TestSubmitControlSelectors_PreferRealSubmitControlsAndNeverApply` (asserts the precedence and that "Apply" never appears), `TestFindSubmitControl_SkipsAGroupWithNoVisibleMatch`.

### 86. Lever's location typeahead was invisible to combobox detection, so every Lever application failed (Resolved 2026-07-25, verified against the live form)

**Nova (Lever, `ioconnectservices`) failed all three attempts while reporting `7/7 validation fix(es) applied` each time** — the model even varied its selector syntax between attempts (`input[name='email']` then `input[data-qa='email-input']`), which looked like flailing but was irrelevant: both resolved fine.

Probing the real form settled it immediately. Lever asks for far less than Greenhouse — **only three required fields**, and the resume upload is *optional*:

```
EMPTY  text   name             EMPTY  email  email             EMPTY  text  location-input
file inputs: [optional resume files=0]
```

So the failure was entirely down to `location-input`. Its markup:

```html
<input class="location-input" data-qa="location-input" id="location-input" name="location" required>
<input id="selected-location" type="hidden" name="selectedLocation">
<div class="dropdown-container"><div class="dropdown-results">
```

**Root cause:** none of react-select's markers are present — no `role="combobox"`, no `aria-autocomplete`, no `aria-controls`, no `select__` classes. `isComboboxInputJS` therefore returned **false**, so the field was treated as a plain text input: filled with text, never committed, while the hidden `selectedLocation` — *the value the form actually validates* — stayed empty. Measured directly: `detectedAsCombobox: false`, `selectedLocation=""` after typing.

**A second, independent obstacle:** clicking the chosen option does not commit it. The click blurs the input, the dropdown closes, and the handler never fires — measured, leaving **both** the visible input and `selectedLocation` empty. Keyboard selection is blur-safe and does work.

**Fix:**
- Detection extended to a sibling `.dropdown-results`/`.dropdown-container` or a hidden `input[name^="selected"]`.
- `readHiddenCommitValueJS` reads that hidden field as the committed value — never `el.value`, for #81's reason.
- Option enumeration extended to Lever's `.dropdown-location` results.
- `setComboboxValue` keeps the click (confirmed working for react-select and left undisturbed) and falls back to **index-driven keyboard selection**: arrow to the option `pickComboboxOption` chose, then Enter. Index-driven, not "first option", because #79's guarantee has to hold here too — **"Detroit, ME" sits directly beneath "Detroit, MI" in the same list**.

**Verified against the live form, all parts in one run:**

```
detectedAsCombobox (new logic): true
options seen (4): location-0|Detroit, MI, USA  location-1|Detroit, ME, USA  location-2|Detroit, TX, USA ...
picked id="location-0" index=0 ok=true
AFTER keyboard commit: visible=Detroit, MI, USA
  hidden(selectedLocation)={"name":"Detroit, MI, USA","id":"cf06481e9473fd2cbab9d1db5ddb043a7c4170df"}
```

**Open caveat, deliberately not worked around:** Lever's geocoder returns **zero results** for `Macomb`, `Macomb Township` and `Macomb, MI`, while Greenhouse's resolves `Township of Macomb, Michigan, United States` happily. So this fix makes Lever's location *committable*, but the configured location may still not be findable there. Substituting a nearby city the geocoder does know would be misrepresenting the applicant's location on a real application, so it is not done. If this proves common, the honest outcome is to route such jobs to `MANUAL_REQUIRED` rather than to invent a location.

**Also worth noting:** Lever's geocoder rate-limits repeated queries — the same term returned 4 options, then 0, then 4 again across probe runs. Any live testing here needs a fresh page and a single query.

**Tests:** `TestComboboxJS_DetectsLeverStyleTypeaheads`, plus `TestPickComboboxOption_RejectsTheWrongStateEvenWhenItIsFirst` extended to assert the returned index, which is what drives keyboard selection.

### 85. Four early-exit paths left rows stranded in PROCESSING, invisible to every future queue (Resolved 2026-07-25)

**Found by noticing an impossible number.** The cohort monitor reported `PROCESSING=4` on a run with `Using 1 worker(s)`. One worker cannot have four jobs in flight.

The timestamps clustered in pairs, at startup and again exactly when the previous job resolved:

```
Reddit                            22:01:28
Akuity                            22:01:28
Staff Site Reliability Engineer   22:10:45
Stack AV                          22:10:45
```

Cross-referencing the log gave the cause immediately:

```
18:01:28 Fetching job description for Reddit...
18:01:28 Duplicate check: Already applied to Reddit. Skipping.
```

**Root cause:** the worker sets `UpdateFunnelStatus(job.URL, "PROCESSING")` at the top of the loop, and **four** `continue` paths exit without ever clearing it:

| Path | Was | Now |
| --- | --- | --- |
| Invalid/unsafe URL blocked | stranded | `INVALID_URL` (the status already existed for exactly this) |
| Failed to create HTTP request | stranded | `DISCOVERED` (transient) |
| Failed to read response body | stranded | `DISCOVERED` (transient) |
| Duplicate check skip | stranded | `DISCOVERED` |

`GetDiscoveredJobs` selects only `DISCOVERED`, so a stranded row never reappears in any future queue. **#55's startup reaper masked this**: every restart reset the orphans, they were re-picked, skipped, and stranded again — a silent loop that consumed a queue slot each run, corrupted the cohort accounting, and inflated the dashboard's in-flight figures.

**On the duplicate-check case specifically — `DISCOVERED`, deliberately not `APPLIED`.** The `applied_jobs` record is written at *document generation*, not at confirmed submission. That is precisely the falsehood this entire 82-job re-verification exists to audit (see #53: most historical `APPLIED` rows had no confirmation evidence at all). Marking the row `APPLIED` here would manufacture the very claim under investigation. Resetting to `DISCOVERED` restores the pre-`PROCESSING` state, matches exactly what the startup reaper already does, and asserts nothing new about the job.

**The deeper issue is left open and unchanged:** dedup rows are written before submission is confirmed, so `HasApplied` can skip a job that was never actually submitted. That is pre-existing behaviour, not something this fix should silently redefine.

### 84. #82's manual-routing branch was never applied, so refused jobs were written off as FAILED_SUBMIT (Resolved 2026-07-25)

**My own error, and worth recording as such.** #82's guard behaved exactly as designed — watched live on ClickHouse:

```
17:52:52 Narrowed validation retry ... (53969 -> 1877 chars); still invalid: question_15561491004, ...
17:52:52 Auto-Submit failed for ClickHouse: form requires a legal attestation the applicant has not provided: work authorization, visa sponsorship
```

Same timestamp — refused in **zero seconds**, before the ~12-minute model call, exactly as intended. But then:

```
sqlite> SELECT status FROM job_funnel WHERE company_name='ClickHouse';
FAILED_SUBMIT
$ grep -c 'needs a legal attestation not set in pii.yaml' career_agent.log
0
```

`FAILED_SUBMIT`, not `MANUAL_REQUIRED`, and the routing log line never fired. The `cmd/agent` branch that was supposed to catch `ErrNeedsUnprovidedAttestation` **did not exist in the source at all** — the scripted edit silently failed to match, `go build ./...` passed anyway because the `pkg/submitter` half compiled fine, and I confirmed the build rather than confirming the edit.

**Consequence:** a job that is entirely applicable by hand — high fit score, form reachable, only a personal declaration missing — was recorded as a failure. Its tailored documents were never moved to the manual-apply folder and no manual-queue entry was written, so it would simply have been lost.

**The irony is the point.** This is the same failure mode as #76, #77 and #81: *trusting an absence*. There I reasoned from a log line that never appeared; here I reasoned from a compiler that never complained. A green build proves the code that exists compiles, not that the code you intended exists.

**Fix, in two parts:**
1. The branch, applied properly and verified present with `grep` rather than inferred from a passing build.
2. **A structural guarantee instead of a promise.** `submitter.manualReviewErrors` now lists every sentinel meaning "queue this for a human", with `IsManualReviewError` over it, and `cmd/agent` consults it as a **catch-all immediately before the generic failure log**. A sentinel added in future without its own branch still reaches manual review rather than silently becoming `FAILED_SUBMIT`.

**Confirmed live 2026-07-25 18:10**, as a clean A/B on the same job:

```
17:52:52  Auto-Submit failed for ClickHouse: form requires a legal attestation...   -> FAILED_SUBMIT
18:10:44  ClickHouse needs a legal attestation not set in pii.yaml -- queued for
          manual submission: work authorization, visa sponsorship                  -> MANUAL_REQUIRED
```

**Tests:** `TestIsManualReviewError_CoversEveryManualReviewSentinel` (including wrapped, as call sites actually return them), `TestIsManualReviewError_IgnoresOrdinaryFailures` — a real automation failure must *not* be diverted to manual review, since that would hide genuine bugs behind a queue.

### 83. The payload breaker guarded the context window but not the time budget, burning the full 45-minute timeout (Resolved 2026-07-25)

**Predicted, then watched happen.** A Greenhouse posting on a different tenant (`surtai`) produced no `still invalid:` line at all — its theme sets no `aria-invalid` attributes, so `PruneDOMToInvalidFields` found nothing to narrow to and correctly fell back to the whole form, per #64's deliberate design ("an unreadable theme is a reason to send more, never less").

That fallback payload was **50,501 characters**:

```
16:58:03 Attempt 2: Solving validation errors...
16:58:03 SolveValidationErrors API Call #4 executed. Payload length: 50501 characters.
17:43:03 Auto-Submit failed: ... context deadline exceeded
```

**Exactly 45 minutes.** The full `defaultOllamaTimeoutMinutes`, on the one resource that serialises across the entire pipeline, spent on a request that could never have completed.

**Root cause: two different ceilings, only one of them enforced.** `likelyExceedsModelContext` tested against `maxPromptCharsForModelContext` (80,000) — a limit derived from the llama-server context window. 50,501 fits it comfortably, so the check passed. But fitting the context says nothing about finishing in time.

The arithmetic was already recorded in this file and simply never applied here: prompt processing measured at **~7 tok/s** on this host's 30B model (#64, improvements #25), at ~2.5 chars/token ≈ **17.5 chars/s**. Against a 45-minute timeout that is **~47,000 characters** before a request is doomed. The observed 50,501 sits just past it — the prediction and the measurement agree to within a few percent.

**Fix:** added `maxPromptCharsForTimeBudget` (40,000, leaving headroom for token generation and for CPU contention with the browser) and made `likelyExceedsModelContext` trip on either ceiling. Oversized forms now hit the existing `ErrFormTooLargeForModel` path and route to `MANUAL_REQUIRED` **immediately**, with documents saved, instead of costing 45 minutes first.

**A prior test had to be corrected, deliberately.** `TestLikelyExceedsModelContext` carried a case from #60 asserting that 54,917 chars *should pass*. #60 was right about context capacity and silent about time; today's live evidence settles it. The case now expects `true`, with the derivation written into the test so the change reads as a correction rather than a regression.

**Tests:** `TestLikelyExceedsModelContext_RejectsATimeDoomedPayloadThatFitsTheContext` (uses the exact 50,501 size observed), `TestLikelyExceedsModelContext_StillAllowsNormalPayloads`, plus the corrected `#60` case.

### 82. Once the commit worked, an unanswerable legal attestation would have been guessed and really submitted (Resolved 2026-07-25)

**This is a risk that #81 created rather than one it revealed**, and it is worth being precise about that. For the whole of 2026-07-25 the combobox commit was broken, so whatever the model proposed for a screening question was never actually set. #81 fixed that. Probing the live form immediately afterwards:

```
#question_67942418   want="Yes"
  after Fill(): isCombobox=true readCombobox=""  -> not landed -> commit runs
  commit: prefix="Yes" opts=1 -> COMMITTED "Yes"
```

It commits. Which means from #81 onward, whatever the model answers to *"Are you currently authorized to work in the U.S.?"* is really submitted.

**The earlier safety assumption does not hold for these fields.** `ApplicationFacts` tells the model that anything not listed "was not provided" and to "choose the form's decline option" — and for the EEO questions that works, because every one of them offers *"I don't wish to answer"* (verified: `430`, `433`, `434`). But the option-level audit found:

```
question_67942418 (authorized to work in US):  Yes | No
question_67942419 (requires sponsorship):      Yes | No
```

**Required, binary, no decline option.** There is nothing for the model to decline *to*. Instructed not to fabricate, and given no abstention, it will still pick one — and that answer is a legal declaration made under the user's name to a real employer.

**Fix — refuse before asking, not after.** `parser.DetectAttestationQuestions` scans the form's visible text for the phrasings ATS forms actually use across four categories (work authorization, visa sponsorship, security clearance, criminal history). `PII.MissingAttestations` filters those to the ones with no configured answer. If any remain, the retry loop returns `ErrNeedsUnprovidedAttestation` **before** `SolveValidationErrors` is called, and `cmd/agent` routes the job to `MANUAL_REQUIRED` with its tailored documents saved — the same path already used for auth walls and oversized forms.

Refusing *before* the model call matters twice over: it cannot produce a guess to submit, and it saves the ~12-minute inference that would have produced one.

**False positives cost a real application**, so detection is deliberately phrase-based rather than keyword-based — `TestDetectAttestationQuestions_IgnoresOrdinaryForms` pins that "desired salary" and "why do you want to work here" do not trip it. `visa_status` is accepted as a stand-in for the sponsorship question, since "U.S. Citizen" answers it unambiguously.

**Unblocking is one line of config per category** (`work.authorized_to_work_us`, `work.requires_sponsorship`, `work.security_clearance`, `work.criminal_history`). The log names exactly which category is missing.

**Tests:** `TestDetectAttestationQuestions_FindsTheRealGreenhousePhrasings`, `TestDetectAttestationQuestions_IgnoresOrdinaryForms`, `TestDetectAttestationQuestions_FindsClearanceAndCriminalHistory`, `TestMissingAttestations`.

### 81. data-value mirrors the typed search text, so every react-select falsely reported "landed" (Resolved 2026-07-25)

**Caught immediately by #80's new diagnostic**, which is exactly why #80 was worth filing:

```
16:27:01 Narrowed ... still invalid: 430, 431, 432, 433, 434, 436, gdpr_..., question_67942415 ... 67942420
16:40:11 Attempt 2 applied 13/13 validation fix(es) to: #430 ... #question_67942420
16:40:13 Narrowed ... still invalid: 430, 431, 432, 433, 434, 436, gdpr_..., question_67942415 ... 67942420
```

The list is **byte-identical before and after applying all 13 fixes**. Not a near miss — nothing landed at all, including the EEO fields, which are freely declinable and have nothing to do with the two missing attestations. Before #80 this was invisible: the only observable was a payload size drifting 7212 → 7281.

**Root cause, established by probe rather than inference.** Replicating what `applyValidationFix` does — resolve, `Fill()`, then run the agent's own checks:

```
[id="430"]   Fill()=nil, nothing selected
  isCombobox   = true
  readCombobox = "I don't wish to answer"    <-- reports LANDED
```

react-select sets `data-value` on `.select__input-container` to mirror the **typed search text** (it drives the input's auto-sizing). It is therefore non-empty the instant anything is typed, committed or not. The `[data-value]` fallback in `readComboboxValueJS` was reading it and calling it a committed selection.

So: `applyValidationFix` types the text, the read-back sees `data-value` and reports success, the commit step is skipped, and the field is never actually set. Every custom question on every Greenhouse form, every attempt.

**This is the same mistake as #76 one layer deeper** — reading the artifact of typing and treating it as a committed value. #76 fixed `el.value`; the fallback added alongside it had the identical flaw and went unnoticed because it only fires when `el.value` is empty.

**Why Location/Country still worked:** `fillGreenhouseCombobox` calls `setComboboxValue` directly and clicks a real option, so `.select__single-value` genuinely exists and is checked first. Only the *retry* path, which starts from a bare `Fill()`, was fooled.

**Fix:** `readComboboxValueJS` now reads **only** `.select__single-value` / `.select__multi-value__label` — the widget's rendered selection. Pinned by `TestReadComboboxValue_IgnoresDataValueWhichMirrorsTypedText`, which asserts the JS does not mention `data-value` at all, so the fallback cannot be reintroduced.

**Second defect fixed here:** a verification *error* fell through the retry loop's condition entirely (`vErr == nil && !landed`), recording the field as neither landed nor failed — it vanished from the logs and no commit was attempted. An unverifiable field is now treated as not set and logged.

### 80. The retry loop logged the payload size but never which fields were still invalid (Resolved 2026-07-25)

**Filed the moment the existing diagnostics ran out.** With every earlier fix in place:

```
16:15:33 Attempt 2 applied 13/13 validation fix(es) to: #430 ... #question_67942420
16:15:34 Submission failed validation. Retrying...
16:15:34 Narrowed validation retry to the rejected fields only (54606 -> 7281 chars)
```

Read that carefully: **13 of 13 applied, no "left the control empty" line, no "Validation fix failed" line — and the form still rejected the submission.** Every signal the system had said success, and the outcome was failure. The only remaining number, the payload size, went 7212 → 7281, which is uninterpretable: it cannot distinguish "the same fields are still failing" from "a different set is now failing".

The next step from here would have been another blind ~25-minute cycle. That is the same trap #70, #76 and #77 each sprang in turn, and the same fix applies: **stop inferring, start naming.**

`parser.InvalidFieldIdentifiers` walks the narrowed payload and lists the controls the page flagged, by `id`, falling back to `name` and then the tag so a control can never be silently omitted. The retry loop logs them alongside the size:

```
Narrowed validation retry to the rejected fields only (54606 -> 7281 chars); still invalid: 430, 431, question_67942418, ...
```

**Not a code defect in itself** — it is the missing measurement that was preventing the *next* defect from being found. Filed as a bug rather than an improvement because the absence was actively blocking diagnosis of a Blocker-severity failure.

**Tests:** `TestInvalidFieldIdentifiers_NamesTheRejectedControls`, `TestInvalidFieldIdentifiers_FallsBackToNameThenTag`.

### 79. The option wait watched an unrelated widget, and committing option-0 filed the wrong location (Resolved 2026-07-25, verified against the live form)

**Found by abandoning the guess-and-wait loop.** Each hypothesis was costing ~12 minutes of inference to test through the agent, so I built a standalone Playwright probe against Reddit's real form. Feedback dropped to ~30 seconds and both defects fell out immediately.

**(a) The options wait was watching the wrong widget.** #77 polled `document.querySelectorAll('[role="option"], .select__option')`. Probe output:

```
#candidate-location typed="Macomb Township, MI"
  activedescendant=""   options=[Afghanistan+93, Åland Islands+358, Albania+355, ...]
```

Those are **dial codes**. Every Greenhouse page carries an always-present intl-tel-input phone-country widget whose menu holds ~244 options at all times, so a document-wide count is *permanently* non-zero. The wait returned instantly, every time, and each commit fired into a menu that had not opened. Now resolved through the input's own `aria-controls` (falling back to the listbox implied by `aria-activedescendant`), so it can only ever see the widget being driven.

**(b) Committing the focused option is unsafe.** This is the serious one. Typing `Macomb` returns:

```
option-0  Macomb, Illinois, United States      <-- wrong state
option-2  Township of Macomb, Michigan, ...    <-- the configured address
```

The configured address is in **Michigan**. An earlier probe run pressed Enter and committed *Macomb, Illinois* — meaning that had the commit "worked" at any point today, it would have filed real job applications with the wrong location. A silent wrong answer is worse than the visible failure it replaced.

`pickComboboxOption` now requires every token in `mustContain` (for location: the city's first word and the spelled-out state) and otherwise requires option and configured value to contain one another. **If nothing matches, nothing is selected** — the field is left to the validation-retry loop rather than filled with something wrong.

Also fixed here: the configured value frequently cannot be typed in full, because these widgets filter by substring against their own labels. `"United States of America"` matches nothing against a list whose entry is `"United States"`; `"Macomb Township, MI"` matches nothing at all. `searchPrefixes` shortens the query word by word until the list responds.

**Verified against the live form**, not merely unit-tested:

```
LOCATION: "Macomb" -> 7 options -> picked option-2
  => COMMITTED "Township of Macomb, Michigan, United States"
COUNTRY:  "United States" -> picked option-0
  => COMMITTED "+1"
```

**Tests:** `TestPickComboboxOption_RejectsTheWrongStateEvenWhenItIsFirst` (pins the safety property), `TestPickComboboxOption_SelectsNothingWhenNoOptionMatches`, `TestPickComboboxOption_MatchesAShorterListLabelAgainstALongerConfiguredValue`, `TestSearchPrefixes_ShortensUntilSomethingCanMatch`, `TestNormalizeOptionText_StripsDialCodeAndPunctuation`.

### 78. Fill() never opens a react-select menu, and the read-back matched the input itself (Resolved 2026-07-25, verified against the live form)

**Two independent reasons the combobox commit could never have worked**, both established by direct observation of the live DOM rather than inference.

**(a) `Fill()` does not open the menu.** Probed on Reddit's form: after `Fill()` succeeded, the widget's own option count stayed **0** and `aria-activedescendant` stayed **empty for a full 3 seconds**. react-select opens and filters its menu in response to real key events, not to a programmatic value set. The Enter that followed therefore had nothing to select. Clicking the control and typing produces options and a focused option within ~600ms. `setComboboxValue` now clicks, clears, and types.

**(b) The value read matched the input itself.** react-select sets `role="combobox"` **on the input element**, and `Element.closest()` tests the element before its ancestors. So `el.closest('.select__control, .select-shell, [role="combobox"]')` returned **the input** — which has no children — and the `.select__single-value` lookup inside it found nothing. The read-back reported `""` even when the DOM plainly contained `option Macomb, Illinois, United States, selected.` and a populated `.select__single-value`.

Proven by toggling only that one expression against identical DOM:

```
role attr on the input itself = "combobox"   <-- closest() matches self
before: COMMITTED VALUE = ""
after:  COMMITTED VALUE = "Macomb, Illinois, United States"
```

The shell lookup now prefers the container classes and only considers a `role="combobox"` ancestor **via `el.parentElement`**, so it can never resolve to the input.

**Why this mattered so much:** (b) made #74, #75 and #77 all report failure even where they had succeeded, which is why three consecutive fixes looked inert. It is the same class of error as #76 — a diagnostic lying about its own subject.

### 77. Enter was pressed before react-select had loaded any option, so the commit selected nothing (Resolved 2026-07-25, live confirmation pending)

**Caught the moment #76 made the read-back actually work.** The run carrying #76 produced a line that had never appeared before:

```
14:17:51 Attempt 2 applied 15/15 validation fix(es) to: 430 ... candidate-location, country ...
14:17:51 Attempt 2: 11 fix(es) reported success but left the control empty (autocomplete/combobox suspected):
         430, 431, 432, 433, 434, 436, candidate-location, country, question_67942418, question_67942419, question_67942420
14:17:53 Narrowed validation retry to the rejected fields only (54359 -> 5988 chars)
```

Two things to read off this:

1. **#76 works.** Before it, all 15 fields reported as landed; now 11 are correctly identified as still empty. The diagnostic is finally telling the truth.
2. **The commit still does nothing.** Those 11 went to `notLanded`, not to the committed list — so `commitComboboxOnLocator` ran, pressed Enter, and re-read an *empty* control. Enter is reaching the widget and selecting nothing.

**Root cause:** react-select populates its menu asynchronously. Greenhouse's Location field is geocoder-backed, so options arrive over the network. `Fill()` returns as soon as the text is typed, and the Enter that follows lands while the menu is still empty — nothing is highlighted, so nothing is selected. The custom-question fields (`430`-`436`, `question_679424xx`) are react-select too and fail identically.

**Fix:** `waitForComboboxOptions` polls for `[role="option"], .select__option` before the keypress, on a 5s budget at 250ms intervals. It searches document-wide because react-select renders its menu in a portal as often as inline. Best-effort: on timeout the Enter is still attempted, and the read-back afterwards remains the thing that actually decides whether anything committed — so a slow or genuinely optionless widget degrades to the previous behaviour rather than erroring.

**Genuine progress in the same run, worth recording:** the narrowed invalid-field payload **shrank for the first time in this entire investigation** — 8249 → 5988 chars, −28%. Every previous attempt held flat or grew (8249 → 8334). Four of the 15 fields are now being satisfied. The direction finally reversed.

**Method note, again:** this was very nearly missed. The `left the control empty` line was **not** in the log-monitor's filter, so it never surfaced as a notification — it was found only by grepping the log directly after the payload size dropped unexpectedly. Filter widened. **An absent notification is not evidence of an absent event; check the filter before concluding anything from silence.**

**Tests:** `TestCommitComboboxOnLocator_WaitsForOptionsBeforePressingEnter`.

### 76. #74's own read-back checked el.value first, silently disabling the combobox commit it had just added (Resolved 2026-07-25, live confirmation pending)

**A defect in #74's fix, and it was caught by a log line that did not appear.** The run carrying #74 produced:

```
13:49:18 Attempt 2 applied 15/15 validation fix(es) to: 430 ... candidate-location, country ...
13:49:20 Submission failed validation. Retrying...
```

No `committed N autocomplete selection(s)` line. No `reported success but left the control empty` line. Both are absent, and that combination is only possible one way: `verifyFixLanded` returned **true for all 15 fields**, so the combobox branch was never entered. #74 was inert on precisely the fields it was written for, and #75 — built on the same read-back — inherited the inertness.

**Root cause:** the read script tested `el.value` before the combobox branch:

```js
if (el.value) return String(el.value);          // <-- wrong for a combobox
const shell = el.closest('.select__control, ...');
```

After `Fill()`, a react-select search input **does** hold the typed text. So `el.value` was `"Detroit, MI"`, non-empty, and the control was declared satisfied — while the widget's committed selection was still empty and the form still rejected it. The check that was supposed to detect "typed but not committed" was reading the very artifact of typing.

**Fix:** split into `readInputValueJS` and `readComboboxValueJS` (which never looks at `el.value`), and moved the choice between them into Go, in `locatorHasValue`. Branching inside one JS blob is what made the ordering untestable and let this ship in the first place; the decision is now a plain Go conditional with `isComboboxLocator`, and is unit-tested directly.

**Severity Blocker:** it rendered two other shipped fixes completely inert without any error, on required fields that no retry could otherwise satisfy.

**Lesson worth keeping:** the diagnostic that caught this was an *absent* log line, not a failing one. Both #74 and #75 looked correct in isolation and had passing tests. What exposed the defect was checking whether the fix actually announced itself at runtime — worth doing deliberately after any fix whose whole purpose is to fire on a specific condition.

**Tests:** `TestLocatorHasValue_ReadsAComboboxWithTheWidgetScriptNotElValue`, `TestLocatorHasValue_ReadsAPlainInputWithElValue`.

### 75. #74's combobox commit was wired into the retry path but not the initial fill, guaranteeing a wasted retry cycle (Resolved 2026-07-25, live confirmation pending)

**This is precisely the structural gap bug #67 found, recurring one layer up** — a capability added to the validation-*retry* path and never wired into the initial fill.

`safeFillWithLabelFallback`'s three tiers (accessible label → placeholder → CSS selector) all set values with a plain `Fill()`. Per #74 that types into a react-select's search box and commits nothing. `Location (City)` and `Country` are required on every Greenhouse form, so **the first submit was guaranteed to bounce** — not because anything went wrong, but because the first pass structurally could not satisfy two required fields.

The cost is not the bounce itself, it is what the bounce buys: a full validation-retry cycle is a `SolveValidationErrors` call, which on this machine is **~12 minutes of inference**, to commit something a single keypress could have done in the first pass. Multiplied across every Greenhouse posting in a ~3,100-job backlog where all LLM calls serialise, this is one of the largest avoidable time sinks in the system.

**Confirmed live at 13:36**, on the run already carrying #74: attempt 1 bounced with the narrowed payload at exactly **8249 chars — byte-for-byte the same size as the run before it**. #74 fixed the retry, so the job would eventually recover, but the wasted first pass was untouched.

**Fix:** extracted `locatorHasValue` and `commitComboboxOnLocator` so the label/placeholder tiers — which never have a selector string to re-resolve — can run the same check as the selector tier. `commitFilledCombobox` runs after every successful initial fill and is **best-effort**: it never fails an otherwise-good fill, since the validation-retry path remains as a backstop. It stays as narrow as #74's version, firing only inside `.select__control` / `.select-shell` / `role="combobox"`.

**Tests:** `TestSafeFillWithLabelFallback_CommitsAComboboxOnTheInitialFill`, `TestSafeFillWithLabelFallback_DoesNotPressEnterOnPlainInputs`.

**Worth noting as a pattern:** #65/#66 → #67, and now #74 → #75. Twice, a fill capability has been added to the retry path only. Any future change to how a control is set should be checked against *both* paths before it is called done.

### 74. react-select comboboxes were filled but never committed, so their validated value stayed empty (Resolved 2026-07-25, live confirmation pending)

**This is #72's autocomplete hypothesis, promoted to a root cause by evidence rather than inference.** Fetched Reddit's actual Greenhouse page and read the markup for the two fields that resolved fine on attempt 3 and still bounced:

```html
<label id="candidate-location-label" for="candidate-location">Location (City)<span aria-hidden="true">*</span></label>
<div class="select-shell remix-css-b62m3t-container">
  <span id="react-select-candidate-location-live-region" ...></span>
  <div class="select__control remix-css-13cymwt-control">
    <div class="select__value-container">
      <div class="select__placeholder" id="react-select-candidate-location-placeholder"></div>
      <div class="select__input-container" data-value="">
        <input class="select__input" ...>
```

It is **react-select**. `<input id="candidate-location">` is the widget's *search* box, not its value. The committed selection lives in React state and is rendered into a sibling `.select__single-value`; `.select__input-container[data-value]` mirrors it.

**Two consequences, both fatal:**
1. `Fill()` types search text and commits nothing. The value the form validates stays empty. `Location (City)` and `Country` are both **required** (note the `*`), so the form could never pass, no matter how many retries.
2. `verifyFixLanded` (added in #72) reads `el.value` — which is `""` *whether or not a selection succeeded*. So #72's own read-back would have reported a false negative on a working combobox. Fixed here as part of the same change.

**Fix:** `readControlValueJS` now looks past the input to the widget — `.select__single-value`, `.select__multi-value__label`, then `[data-value]` — before concluding a control is empty. `commitComboboxSelection` presses `Enter` to commit the focused option and re-reads. It is **deliberately narrow**: it fires only when the control is inside `.select__control` / `.select-shell` or carries `role="combobox"`, because a stray `Enter` in an ordinary text input can submit the form before the remaining fixes are applied. Pinned by `TestCommitComboboxSelection_LeavesPlainInputsAlone`.

The retry loop now logs `committed N autocomplete selection(s) that Fill() alone had left empty`.

**Status is "live confirmation pending" deliberately.** The markup evidence is direct and the mechanism is not in doubt, but no live run has yet produced a confirmed `APPLIED` through this path. Do not mark it verified until one does.

**Tests:** `TestCommitComboboxSelection_PressesEnterOnAComboboxAndConfirms`, `TestCommitComboboxSelection_LeavesPlainInputsAlone`.

### 73. A CSS id selector cannot start with a digit, so Greenhouse's numeric custom-question ids were unfillable half the time (Resolved 2026-07-25)

**Caught live on Reddit's third and final attempt**, minutes after #72 shipped:

```
13:13:54 Validation fix for "input#434" failed: selector matched no element (tried 1 form(s) of "input#434")
13:13:54 Validation fix for "input#430" failed: selector matched no element (tried 1 form(s) of "input#430")
13:13:55 Validation fix for "input#431" failed: ...   (same for #432, #433, #436)
13:13:55 Attempt 3 applied 9/15 validation fix(es) to: #gdpr_demographic_data_consent_given_1,
         input#candidate-location, input#country, input#question_67942415 ... question_67942420
13:13:57 Auto-Submit failed for Reddit: failed to submit application after 3 validation error attempts
```

**Root cause:** `#430` is not a valid CSS selector. A CSS identifier may not begin with a digit — `document.querySelector("#430")` throws a `SyntaxError`. Greenhouse nevertheless numbers its custom-question controls exactly that way (`id="430"`). The `[id="430"]` attribute form has no such restriction and matches perfectly.

`resolveFieldLocator` built its attribute-form fallbacks only under `if !looksLikeCSSSelector(selector)`. `input#430` contains `#`, so it "looks like CSS", so the fallbacks were skipped and the invalid selector was used verbatim — note `tried 1 form(s)` in the log, versus the 5 forms a bare identifier gets.

**The tell that makes this unambiguous:** the model sent bare `430` on attempt 2 and `input#430` on attempt 3, for the same field on the same form. Attempt 2 resolved it (via #66's bare-identifier fallbacks); attempt 3 could not. The same control alternated between fillable and unfillable purely on how the model happened to phrase the selector — which also explains why #72's `15/15 applied` on attempt 2 dropped to `9/15` on attempt 3 with no change to the page.

**Fix:** added `splitTagID`, which decomposes a simple `tag#id` / `#id` selector and refuses anything more complex (descendant combinators, attribute filters, class chains, selector lists) where a naive rewrite would change meaning. When the verbatim selector is CSS-shaped, `resolveFieldLocator` now also queues `tag[id="..."]`, `[id="..."]` and `[name="..."]`. The verbatim selector is still tried first, so nothing that worked before changes behaviour.

**Tests:** `TestResolveFieldLocator_FallsBackToAttributeFormForNumericIDs`, `TestSplitTagID` (table-driven, pins the refusal cases too).

**Note on what this does *not* explain:** `input#candidate-location` and `input#country` resolved fine on attempt 3 and the form still bounced. #72's autocomplete/combobox hypothesis remains live and unconfirmed for those two.

### 72. The retry loop counts empty-valued and non-landing fixes as applied, reporting progress it is not making (Resolved 2026-07-25, accounting fixed; underlying non-convergence still under live investigation)

**Found by the diagnostic #70 added, within an hour of it shipping** — which is the point of that diagnostic. Reddit, re-run against #70's fix:

```
12:52:05 Narrowed validation retry to the rejected fields only (54877 -> 8249 chars)
13:04:18 Attempt 2 applied 15/15 validation fix(es) to: 430, 431, 432, 433, 434, 436,
         candidate-location, country, gdpr_demographic_data_consent_given_1,
         question_67942415, question_67942416, question_67942417, question_67942418,
         question_67942419, question_67942420
13:04:19 Submission failed validation. Retrying...
13:04:19 Narrowed validation retry to the rejected fields only (54748 -> 8334 chars)
```

**#70's fix did work** — the narrowed payload grew 5,363 → 8,249 chars on the identical form, which is the error text now reaching the model. But 15/15 fixes "applied" and the form still bounced, with the invalid-field payload essentially unchanged (8249 → 8334). The same fields are still invalid, so the tally is not measuring what it claims.

**Two accounting defects, both proven from code:**

1. **An empty value counts as applied.** `applyValidationFix` returns `nil` when `value == ""`. That is *correct* for the initial-fill path, where it means "the profile has no data for this field, skip it" — `safeFillWithLabelFallback` depends on it. In the retry path the same return is a lie: the field was just rejected, and proposing `""` cannot satisfy it. The contract could not simply be changed, since both paths share the function; fixed at the retry call site instead.
2. **A `nil` return does not mean the control ended up set.** It means Playwright accepted the call. The known gap is ATS autocomplete widgets — `candidate-location` and `country` in the list above are exactly Greenhouse's — where the visible text box is backed by a separate hidden field, so setting it without choosing a suggestion leaves the value the form actually validates completely unset.

**Fix:** the retry loop now skips empty values (logging them separately as `model proposed an empty value for N rejected field(s)`), and `verifyFixLanded` reads each control back after the fix, logging any that report success but left the control empty as `N fix(es) reported success but left the control empty (autocomplete/combobox suspected)`. The read-back test is deliberately "is it non-empty now" rather than strict equality — forms legitimately reformat phone numbers and dates, and a `<select>` set by visible label reports its underlying value, so equality would cry wolf on fixes that did land.

**Deliberately still open:** this fixes the *measurement*, not necessarily the underlying non-convergence. The autocomplete theory is a hypothesis consistent with the selector list, not yet a confirmed root cause. The next Reddit attempt will name the offending selectors outright, and that becomes the next bug — which is exactly the position #70 was in an hour ago, and it was right then.

**Tests:** `TestVerifyFixLanded_DetectsAControlLeftEmpty`, `TestVerifyFixLanded_AcceptsAReformattedValue`, `TestVerifyFixLanded_TreatsAnUncheckedBoxAsNotLanded`.

### 71. firstVisibleLocator's .First() fallback reintroduces the very hang it was written to prevent, at the submit click (Resolved 2026-07-25)

**Found while auditing the 82-cohort's `FAILED_SUBMIT` rows** before restarting the run for #70 — the count had grown 6 → 7 and the newcomer was not one of the five known-dead postings. Zimperium (`jobs.lever.co/zimperium/18699ad3...`), fit score **85**:

```
09:05:51 Detected Lever ATS. Filling out fields...
09:06:02 Submission failed validation. Retrying...
09:06:02 Attempt 2: Solving validation errors...
09:33:28 Auto-Submit failed for Zimperium: playwright: timeout: Timeout 30000ms exceeded.
Call log:
  - waiting for locator('input[type=\'submit\'], button[type=\'submit\'], button:has-text(\'Submit\'), button:has-text(\'Apply\')').first()
```

**Root cause:** the `.first()` in Playwright's own call log is the tell. `firstVisibleLocator` walks the matches looking for a visible one and, finding none, falls back to `return loc.First()`. The caller then clicks that match — which is *known to be invisible*, because the loop just checked every single one. Playwright waits for it to become actionable, it never does, and the click burns the full `fillActionTimeoutMs`.

This is precisely the hang the function's own doc comment says it exists to prevent (bug #59: Lever's `<button type="submit" class="hidden" id="hcaptchaSubmitBtn">" being clicked ahead of the real button). #59 fixed the "picks the wrong match" half and left the "picks a known-bad match anyway" half in the fallback.

Two costs: 30 wasted seconds per occurrence, and — worse — the failure surfaces as a bare `Timeout 30000ms exceeded`, which reads as CPU contention or a slow page. It is nothing of the sort. It means *there is no visible submit control on this form*, which is a completely different and actionable diagnosis.

**Fix:** added `firstVisibleSubmit`, which is `firstVisibleLocator` without the fallback — it returns `(locator, ok)`. The submit-click site now fails immediately with `found N submit control(s) but none visible` instead of clicking a hidden element. `firstVisibleLocator` is reimplemented on top of it and keeps its fallback for the two *fill* call sites, where attempting a hidden element is still worth doing; their existing tests are unchanged and still pass.

**Not yet known, deliberately left open:** *why* Lever presented no visible submit control at that moment. The old error message made that question unaskable. It should now be answerable from the logs the next time it happens — and Zimperium is requeued to find out.

**Tests:** `TestFirstVisibleSubmit_ReportsWhenNoMatchIsVisible`, `TestFirstVisibleSubmit_ReportsTheVisibleMatch`.

### 70. The validation-retry loop strips the page's own error text, so the model never learns why a field bounced (Resolved 2026-07-25)

**Caught live, in the act**, while monitoring the 82-job re-verification run on 2026-07-25. Reddit (`job-boards.greenhouse.io/reddit/jobs/8044767`) scored **90** — the highest-fit job in the entire cohort — and then died at the last step:

```
12:14:55 Fit Score Pipeline: Reddit scored 90! Proceeding with application.
12:15:03 Submission failed validation. Retrying...
12:15:03 Narrowed validation retry to the rejected fields only (53366 -> 5363 chars)
12:27:16 Submission failed validation. Retrying...
12:27:16 Narrowed validation retry to the rejected fields only (53228 -> 5439 chars)
12:32:28 Submission failed validation. Retrying...
12:32:28 Auto-Submit failed for Reddit: failed to submit application after 3 validation error attempts
```

**17.5 minutes and 3 LLM calls for nothing.** The tell is in the numbers: between attempts the form barely changed (53366 → 53228) and the narrowed slice *grew* (5363 → 5439). If the fixes were landing, the invalid-field set should shrink. It did not — the same fields were being rejected every time.

**Root cause — two independent defects on the same path:**

1. **`aria-describedby` was in `presentationalAttrs`** (`pkg/parser/dom.go`), stripped as bloat by `StripPresentationalAttrs`. But that runs at `pkg/submitter/browser.go` *before* `PruneDOMToInvalidFields`. `aria-describedby` is the WCAG-standard pointer from a rejected control to the element holding the page's explanation of the rejection. Stripping it severed the link; the pruner then dropped the error element itself, since it is neither an invalid control nor a `<label>`. Net effect: the model received `<input name="phone" aria-invalid="true">` with the label "Phone" and **no statement of what was wrong with it** — so it re-proposed a plausible value, which bounced identically. #64's narrowing made this strictly worse: before it, the full form at least carried the error text somewhere in the payload.

2. **An empty fix map fell through to a re-submit.** The guard read `if !appliedAny && len(fixesMap) > 0`. When the model proposed *nothing* (a legitimate outcome — `SolveValidationErrors` returns a nil map for a `null`/`{}` response with no error), `len(fixesMap) == 0`, the guard did not fire, and control fell straight through to the submit click — re-submitting a byte-identical form and burning another ~6-12 minutes. Note the comment already sitting above that block from #65 states the exact failure mode ("the next attempt re-sends an identical payload and the loop is guaranteed to exhaust itself"); the guard just did not cover the empty case.

**Fix:**
- Removed `aria-describedby` from `presentationalAttrs`, with the same style of carve-out comment `aria-invalid` already carries.
- `PruneDOMToInvalidFields` now follows `aria-describedby` **and** `aria-errormessage` (both space-separated id lists per WCAG) and emits **label → control → error text grouped per rejected field**, so the model never has to re-associate them by id across a flat list.
- Empty fix map is now a hard failure (`model proposed no fixes for the rejected fields`) instead of a futile re-submit.
- Added an `Attempt N applied X/Y validation fix(es) to: <selectors>` log line. **Selectors only, never values** — the values come from the PII profile and the log is not a place for them. Without this, a non-converging retry loop is undiagnosable from logs, which is exactly why this bug survived until someone watched it happen in real time.

**Tests:** `TestPruneDOMToInvalidFields_KeepsAriaDescribedByErrorText` and `TestPruneDOMToInvalidFields_KeepsAriaErrorMessageText` (both verified failing before the fix — the error text was provably absent from the output). `TestStripPresentationalAttrs_RemovesStylingAndStateAttrs` had its `aria-describedby` assertion removed and replaced with a pointer to the new tests, so the carve-out cannot be silently reverted.

**Severity Blocker, not Major:** this is the terminal step of the entire pipeline. It fails *after* discovery, scoring, document generation and form fill have all succeeded, on the jobs the agent rated highest — and it consumes the single most expensive resource in the system (~6 min of inference per wasted attempt, on a machine where all LLM calls serialise).

### 69. Discovery stored the searched role as job_title and discarded the real headline (Resolved 2026-07-25)
**Found while auditing why throughput is the binding constraint** (3,131 jobs waiting at ~10 min each ≈ 22 days of continuous compute). Checking whether cheap title-based pre-filtering could skip irrelevant rows turned up something odd: `SELECT COUNT(DISTINCT job_title)` over the 3,131 waiting rows returned **55** — suspiciously close to the length of the configured roles list, and the sample was "Senior Backend Engineer" repeated ten times over.

**Root cause:** `pkg/scraper/funnel.go`'s SerpAPI path called `storage.AddToFunnel(company, role, result.Link, ...)`, passing the **searched role** as the job title. The real headline was available the whole time — `result.Title` is logged on the line immediately above and then discarded — and `extractCompanyFromTitle` already parses the company out of that very string.

**Why it matters beyond cosmetics:** improvements.md #22 (shipped 2026-07-24) ranks the discovery queue by embedding `title + company`. If the title is one of 55 role names rather than the posting's real title, every job discovered under the same search role produces a near-identical embedding, and the ranking degenerates toward "order by which role was searched" — quietly undermining a feature that had just shipped. It also made title-based pre-filtering impossible, and made logs and the dashboard misleading about what a row actually is.

**Fix:** new `extractJobTitleFromResult` reads the title from the same headline `extractCompanyFromTitle` reads the company from ("Senior Backend Engineer at Stripe - Lever" → title before " at ", company after), with a secondary "Title - Company - ATS" form, falling back to the searched role when the headline cannot be parsed so a row always carries something meaningful.

**Deliberately not changed — the Yahoo fallback.** That path parses raw anchor hrefs and has no result headline at all, so the searched role genuinely is the only label available. Left as-is with a comment saying why, rather than inventing a title from the URL slug.

**Limitation, stated plainly:** this corrects discovery *going forward*. The 3,131 existing rows keep their role-as-title values, and their `fit_similarity` scores remain computed from those, so queue ranking stays weak for the current backlog. Re-deriving real titles for those would mean re-fetching every posting; not worth it. Expect ranking quality to improve only as new discoveries replace the old backlog.

Tests: `TestExtractJobTitleFromResult` (both headline shapes plus three unparseable cases falling back to the role), `TestTitleAndCompanyExtractorsAgree` (pins that the two extractors keep reading the same headline consistently). `go build/vet/test ./...` all pass, 10 packages, 0 failures.

### 68. SaveFormMapping cached semantically-empty mappings, burning a Learner Module call per visit (Resolved 2026-07-25)
Bug #21 added a `json.Valid` guard after a cached mapping of prose poisoned a domain. That guard is necessary but **not sufficient**: a response shaped correctly but with every selector `null` is perfectly valid JSON.

**Found live** while auditing why platforms were underperforming: **7 of 60** cached mappings were in this state, including three actively-used platforms — `smartrecruiters.com`, `pinpointhq.com`, `applytojob.com` (plus `yahoo.com`, `breezy.hr` and two Workday hosts that are excluded or auth-gated anyway):
```json
{"fields": {"first_name": null, "last_name": null, "email": null, "phone": null, "submit_button": null}}
```
**Cost:** every visit to such a domain loaded the mapping, failed every fill, correctly invalidated the cache, then spent a fresh multi-minute `ExtractFormMapping` call regenerating the same nulls — a permanent tax with no path to improvement. Not a hard block (the self-healing invalidation works), but pure waste on repeat.

**Fix:** `hasUsableSelector` rejects any mapping with no non-empty selector before it can be cached. Deliberately **tolerant of both shapes** — the nested `{"fields": {...}}` form that `ExtractFormMapping` produces today, and a flat top-level map — because the guard's job is to reject worthless mappings, and mistaking an unfamiliar-but-usable shape for a worthless one would discard good work. The 7 poisoned rows were purged from the live database (60 → 53) so those domains re-map cleanly.

Test: `TestSaveFormMapping_RejectsSemanticallyEmptyMappings` (all-null refused, whitespace-only refused, a mapping with one real selector still cached and round-tripping).

### 67. The initial fill path never received #65/#66's fixes, so required dropdowns always failed the first pass (Resolved 2026-07-25)
**Found by asking where else the just-fixed defects applied.** #65 (dispatch by control type) and #66 (bare-identifier resolution) were both wired into `applyValidationFix`, which had exactly **one** call site: the validation-*retry* loop. The initial fill — `handleDynamic`'s standard fields and its custom-screening-question pass — still went through `safeFillWithLabelFallback` → `safeFill`, which is `Fill()`-only and does no selector resolution.

**Consequences on the first pass:**
- A required `<select>` (work authorization, sponsorship, EEO — routine on Greenhouse) **could not be set at all**, so the submission was guaranteed to bounce and enter the expensive validation-retry cycle that #64/#65/#66 exist to survive. The retry was avoidable in the first place.
- Custom screening questions rendered as dropdowns (improvements.md #16 generates real answers for these) could never be applied.
- A bare identifier returned in `mapping.Fields` only worked if the label or placeholder tier happened to match, which masked the problem rather than fixing it.

**Fix:** `safeFillWithLabelFallback`'s CSS tier now routes through `applyValidationFix`, so label → placeholder → **type-aware, resolution-capable** selector fill. The label and placeholder tiers are unchanged and still tried first.

**Behavioural note worth knowing:** `applyValidationFix` resolves the element (checking it exists) before acting, which `safeFill` never did. That is strictly better in production — a missing element now reports "selector matched no element" immediately instead of burning a 30s fill timeout — but it did surface three pre-existing test mocks that had no `countFunc` and therefore reported zero matches. Those mocks were completed rather than the assertion weakened; each test still asserts exactly what it did before.

### 66. SolveValidationErrors returns bare id/name values, not CSS selectors, so every proposed fix matched nothing (Resolved 2026-07-25)
**Found within an hour of #65 shipping, by the logging #65 added.** #65 made every fix failure visible instead of silent, and the very first job to hit the new code path reported `none of the 12 proposed validation fixes could be applied`, followed by twelve lines of:
```
Validation fix for "question_9558065008" failed: selector matched no element
Validation fix for "country" failed: selector matched no element
Validation fix for "candidate-location" failed: selector matched no element
```

**Root cause:** every value is a bare identifier — the literal contents of the element's `id` or `name` attribute — not a CSS selector. Passed to `Loc()`, Playwright interprets a bare word as a **tag name**, so `country` searches for a `<country>` element and `question_9558065008` for a `<question_9558065008>` element. Neither exists, so the match count is always zero. The model was not wrong about *which* fields needed fixing; it returned the right fields in a form the code could not use. This is why #65's dispatch-by-type fix, though correct on its own terms, did not by itself produce a successful submission: the elements were never being found in the first place.

Worth noting *why* the model does this: it is shown a DOM fragment where the attributes literally read `id="question_9558065008"`, and it echoes that value back. Tightening the prompt might help, but prompt wording is a weaker guarantee than accepting both forms in code, so the fix is normalisation rather than instruction.

**Fix:** new `resolveFieldLocator` tries the string exactly as given **first** — so a genuinely correct selector is used unchanged and never mangled into `##foo` — then, only if that matched nothing and the string carries no CSS syntax (`looksLikeCSSSelector`), falls back through `#id`, `[name="..."]`, `[id="..."]`, and `[data-qa="..."]`. `applyValidationFix` now routes through it, so #65's type dispatch operates on an element that was actually found.

Tests: `TestResolveFieldLocator_RecoversBareIdentifiers` (also asserts the raw string is attempted first), `TestResolveFieldLocator_RecoversViaNameAttribute`, `TestResolveFieldLocator_LeavesRealSelectorsAlone`, `TestResolveFieldLocator_ErrorsWhenNothingMatches`, `TestLooksLikeCSSSelector`. `go build/vet/test ./...` all pass, 10 packages, 0 failures.

**Sequence worth remembering:** #64 (timeout) hid #65 (wrong fill method), which hid #66 (unusable selectors). Three distinct blockers stacked on the same code path, each only observable once the one in front of it was cleared.

### 65. Validation fixes were applied with Fill() only and their errors discarded, so required dropdowns could never be satisfied (Resolved 2026-07-25)
**Surfaced by bug #64's own fix.** Before #64, large forms died on a >30-minute timeout inside `SolveValidationErrors`, so nobody ever learned what happened *after* the fixes were applied. Once #64 cut the payload (confirmed live: **53,366 → 5,363 chars**, and elsewhere 43,033 → 1,200), those jobs began completing the LLM call in minutes — and immediately exposed the real blocker. It is now the run's dominant failure: **18 of 23 `FAILED_SUBMIT` outcomes** are "failed to submit application after 3 validation error attempts".

**The proof it was structural, not flaky:** the narrowing log reports payload sizes per attempt, and they were **byte-identical between attempt 2 and attempt 3** on multiple jobs (Ethos `43033 -> 1200` twice; Point Wild `3057` twice). Identical input means the same fields were still flagged invalid after the "fix" — nothing had changed, so the third attempt was arithmetically certain to fail like the second.

**Two compounding defects:**
1. **The outcome was thrown away.** The loop read `for selector, value := range fixesMap { safeFill(target, selector, value) }` — `safeFill` returns an `error` and the call site discarded it. Every failed fix was invisible: no log line, no error, no signal that the retry was unwinnable.
2. **`safeFill` is `Fill()`-only.** Playwright refuses `Fill()` on a `<select>` element. Greenhouse-style application forms routinely make dropdowns **required** — work authorization, visa sponsorship, EEO self-identification, "how did you hear about us". Such a field can therefore *never* be satisfied by this code path, no matter how correct the model's proposed answer is. The codebase already knew this: `resolveConsentGateIfPresent` uses `SelectOption` for bug #36's consent gate. The validation-fix path simply never adopted it.

**Fix:** new `applyValidationFix` dispatches on the control's real shape, resolved via a `tagName`/`type` probe:
- `<select>` → `SelectOption`, trying the **visible option label first** (the model answers as a human would — "Yes", "Decline to answer") and falling back to the underlying `value` attribute.
- `<input type=checkbox|radio>` → `Check`, with an explicit negative ("No"/"false"/"0") mapping to `Uncheck` so a decline is never silently converted into consent.
- everything else → `Fill`, as before.

Errors are now logged per selector, and if **no** proposed fix could be applied the attempt fails immediately with a clear message instead of burning the remaining retries on an unchanged form.

Tests: `TestApplyValidationFix_UsesSelectOptionForDropdowns` (also asserts `Fill()` is never called on a select), `TestApplyValidationFix_ChecksCheckboxes` (including that "No" unchecks rather than checks), `TestApplyValidationFix_ReportsUnmatchedSelector`, `TestApplyValidationFix_FillsPlainTextInputs`. `go build/vet/test ./...` all pass, 10 packages, 0 failures.

### 64. SolveValidationErrors re-sends the entire form instead of just the fields that failed, timing out on large forms (Resolved 2026-07-25)
**Found by asking why the 82-job run kept losing the same job.** The Reddit posting (`job-boards.greenhouse.io/reddit/jobs/8044767`) failed at the identical step twice: once with `failed to solve validation errors: ... context deadline exceeded` reaching Ollama, and once running **22+ minutes** inside a single `SolveValidationErrors` call before the run was stopped. Both times it had already filled the form successfully — this is a job that was *working* and lost at the last step.

**Root cause — a budget problem, not a logic problem.** The retry path sends `PruneDOMToForm(domHTML)`, i.e. **every field on the form**, even though a validation bounce typically involves a handful. Bug #52's own note measured a real 35-field ATS form at **52-55k chars even after both existing reduction passes**. Prompt processing on this machine's 30B was measured live at roughly **7 tokens/sec** (`ScoreJob`: 15,647 chars in 9m38s), so ~55k chars ≈ 13.7k tokens ≈ **over 30 minutes of inference against a 45-minute timeout**. Large forms were therefore failing on *time*, not on reasoning — and any concurrent load pushed them over.

**A second defect made the first one unfixable.** `StripPresentationalAttrs` listed **`aria-invalid`** among the attributes it strips. That is the WCAG-standard signal marking which control a form rejected — so by the time the payload reached the model, the information identifying the failing fields had already been deleted. Re-sending everything wasn't just wasteful, it was the only option left.

**Fix, in two parts:**
1. `aria-invalid` removed from `presentationalAttrs` — it is semantic, not presentational. (It is one short attribute per field; its retention costs essentially nothing against the 66% reduction that pass already achieves.)
2. New `parser.PruneDOMToInvalidFields` narrows the retry payload to only the controls marked invalid (`aria-invalid`, plus `data-invalid`/`data-has-error` for themes that roll their own), **plus any `<label for>` bound to them** — labels are collected separately because a label is usually a sibling or ancestor of its control, not a descendant, and the label text is exactly what tells the model what value the field wants.

**Deliberate fallback:** when no invalid control can be identified, the full form is sent exactly as before. An unreadable theme is a reason to send *more*, never less — narrowing to nothing would guarantee the retry fixes nothing. `narrowed` is returned explicitly so the caller cannot silently mistake "found none" for "narrowed to none", and the reduction is logged per attempt.

Tests: `TestPruneDOMToInvalidFields` (keeps both rejected fields and their label, drops the passing ones, asserts the payload actually shrinks), `TestPruneDOMToInvalidFields_NoMarkersFallsBackToFullForm`, `TestStripPresentationalAttrs_KeepsAriaInvalid`. One pre-existing test (`TestStripPresentationalAttrs_RemovesStylingAndStateAttrs`) asserted the *old* `aria-invalid` behavior and was updated with an inline note explaining the reversal rather than quietly edited. `go build/vet/test ./...` all pass, 10 packages, 0 failures.

### 63. Every fit score was computed and thrown away — the only writer of fit_score had zero callers (Resolved 2026-07-25)
**Found while assessing whether improvements.md #14 (local LoRA fine-tuning) was worth working.** Checking what training data actually existed returned a surprising result: `SELECT COUNT(*) FROM job_funnel WHERE fit_score IS NOT NULL` came back **0**, across a database with ~3000 discovered jobs and months of runs.

**Root cause:** `pkg/storage/manager.go` has exactly one function that writes the column — `UpdateFunnelStatusWithScore(url, status, fitScore)` — and a repo-wide grep found it had **zero call sites**. It was dead code. Every scoring outcome in `cmd/agent/main.go` instead called plain `UpdateFunnelStatus(url, status)`, which never touches `fit_score`. So `ScoreJob`'s result was used for the in-memory `score < 50` threshold check, written to the log line, and then discarded.

**Why this matters more than it looks:** `ScoreJob` is now the single most expensive operation in the entire pipeline — measured live at **9m49s of a ~10m job cycle** once improvements.md #23 removed the tailoring call. The project was spending essentially its whole per-job compute budget producing a number it immediately threw away. Knock-on effects: no scoring history for analytics, nothing for `cmd/dashboard` to chart, and — the reason this surfaced — **no labeled dataset could ever accumulate**, which is a hard structural blocker for improvements.md #14 independent of that item's other problems.

**Fix:** both post-scoring branches in `cmd/agent/main.go` now call `UpdateFunnelStatusWithScore` — the `SKIPPED` path (score under 50) and the proceed path (recording `PROCESSING` with the score) — so a score is persisted whichever way the decision goes. Verified that a subsequent plain `UpdateFunnelStatus` (e.g. the later `APPLIED`/`FAILED_SUBMIT` transition) preserves the stored score rather than nulling it, since those transitions are what every job hits afterward.

Test: `TestUpdateFunnelStatusWithScorePersistsScore` — writes a score, asserts it lands, then applies a later plain status change and asserts the score survives. `go build/vet/test ./...` all pass, 10 packages, 0 failures.

**Note for whoever reads this next:** scores only start accumulating from the next run onward. Every historical job remains `NULL`, and re-deriving those would mean re-scoring at ~10 minutes each — not worth it. Treat the dataset as starting 2026-07-25.

### 61. The cover letter was never sent to any employer — no handler ever filled it (Resolved 2026-07-24)
**Found while scoping the user's request for a static master cover letter** (improvements.md #23), by tracing what `ProcessJobApplication`'s cover letter output actually does downstream.

**Root cause:** none of the three fill handlers had any cover letter step. `handleDynamic` fills `first_name`, `last_name`, `email`, `phone`, `resume`, then custom screening questions, then clicks submit. `handleGreenhouse` and `handleLever` fill their hardcoded equivalents plus the resume upload. A `grep -n "cover_letter\|coverLetter\|CoverLetter" pkg/submitter/*.go` returned **zero matches** in non-test code. Meanwhile `ExtractFormMapping`'s system directive explicitly instructs the LLM to map a `cover_letter` selector, and `FormMapping.Fields` carries it — the mapping was produced and then read by nobody. `AttemptSubmit` receives `coverPath` from `generateDocs()` and its only use was `defer os.Remove(coverPath)` (bug #62).

**Impact:** every application this project has ever auto-submitted went out **resume only**. The pipeline spent the single most expensive step in the run (`ProcessJobApplication`, measured live at 15-20+ minutes per job on this machine's CPU-only Ollama) generating a tailored cover letter that was written to disk, never uploaded, and then deleted.

**Fix:** new `fillCoverLetterIfPresent` in `pkg/submitter/browser.go`, wired into all three handlers (`handleDynamic` via the mapping's `cover_letter` selector/label, `handleGreenhouse` via `input[type='file'][name='cover_letter']`, `handleLever` via `textarea[name='comments']`). It tolerates both shapes real ATS platforms use: a file input (upload, detected via a `tagName`/`type` probe since Playwright rejects `Fill()` on a file input) or a textarea/text input (paste), and falls back through the existing label/placeholder/CSS chain. **Best effort by design** — a cover letter is optional on the large majority of real postings, so a failure to place one logs and continues rather than aborting an otherwise complete, submittable application, matching the contract already established for custom screening questions. `coverPath` is now threaded through `handleDynamic`/`handleGreenhouse`/`handleLever`/`AttemptVisionSubmit`.

Tests: `TestFillCoverLetter_PastesIntoTextarea`, `TestFillCoverLetter_UploadsToFileInput` (asserts the uploaded buffer is the real letter content), `TestFillCoverLetter_FailureDoesNotAbortSubmission`, `TestFillCoverLetter_MissingFileIsTolerated`. `go build/vet/test ./...` all pass.

### 62. The saved cover letter was deleted from the application folder, stripping the manual-apply queue (Resolved 2026-07-24)
**Found alongside #61**, tracing the same `coverPath` value.

**Root cause:** `pkg/submitter/browser.go` ran `defer os.Remove(coverPath)` right after `generateDocs()`, treating the file as a scratch temporary. It is not: `SaveApplication` writes it into `applications/<company>/coverletter.txt` deliberately as the saved record of what was sent, and `MoveToManualApply` archives that entire folder for jobs routed to `MANUAL_REQUIRED` — a queue whose whole value proposition is handing the user ready-made documents to apply with by hand. Because it was a `defer`, it fired on **every** exit path, including the `ErrAuthWall` early return that precedes the manual-apply routing.

**Live evidence:** `applications/needs_manual_apply/` held `Akuity`, `alteryxcareers`, `careers`, `ClickHouse`, and `DexCare` all with `resume.md` + `interview_prep.md` + `metadata.json` but **no** `coverletter.txt`. `Backend_Software_Engineer` was the lone folder that kept its letter, which confirmed the exact mechanism rather than leaving it a theory: `cmd/agent/main.go` built the path as `"applications/" + job.CompanyName + "/coverletter.txt"` from the **raw** company name, while `SaveApplication` writes under `safeCompanyDirName`'s sanitized name. The two agree only for names that are already sanitize-stable ("Reddit" → deleted), and silently diverge for anything with a space or punctuation ("Backend Software Engineer" → `os.Remove` targeted a nonexistent path, unchecked error, letter survived).

**Fix:** dropped the deletion entirely — nothing at that call site needs cleanup, since `resumePath` is the persistent `master_resume.pdf` and `coverPath` is either the persistent `master_cover_letter.txt` or an application-folder file meant to outlive the call. Separately fixed the path divergence with a new exported `storage.CoverLetterPath(companyName)`, which builds the path through the same `safeCompanyDirName` that `SaveApplication` writes with, so the two can no longer disagree. Note the same unsanitized-path class was already fixed once before for the manual-queue links (improvements.md #21's amendment) — this was a second, missed instance of it.

Tests: `TestCoverLetterPath` (including names with spaces and punctuation), `TestCoverLetterPathMatchesSaveApplication` (writes via `SaveApplication`, reads back via `CoverLetterPath` — fails if the two ever diverge again). `go build/vet/test ./...` all pass.

### 60. Ollama server pinned to an unnecessarily conservative 6,144-token context window — the dominant cause of MANUAL_REQUIRED outcomes (Resolved 2026-07-24)
**Found live while watching the 82-job re-verification**, after the user asked "any improvements or bugs we can make from this" given 0/82 had reached `APPLIED`: cross-referencing every `MANUAL_REQUIRED` outcome so far (13 total) against the live log found **all 13, no exceptions**, were "form too large for the local model" (bug #57's circuit breaker). 4 of the run's 6 `FAILED_SUBMIT` outcomes were confirmed genuinely dead/expired postings, 1 was bug #59. `MANUAL_REQUIRED` was by a wide margin the single largest blocker to a real `APPLIED` in this entire run.

**Root cause:** `ps aux` showed the live `llama-server` process launched with `-c 6144` — Ollama's actual runtime context window. Cross-referencing `ollama show`'s `model_info` for the running model (`qwen3:30b-instruct`, `qwen3moe` architecture) found `qwen3moe.context_length = 262144` — the model architecturally supports **262,144 tokens**, 42x the 6,144 it was actually being run with. Found the exact cause: `~/.config/systemd/user/ollama.service.d/override.conf` explicitly set `OLLAMA_CONTEXT_LENGTH=6144` (dated 2026-07-21, likely a conservative first value chosen before this model's actual memory profile was understood).

**Why this was assumed to be a hard constraint rather than just fixed outright:** the machine appeared genuinely memory-tight at the time (only ~496MB free, 4.8GB "available", 3.2GB already in swap, with the `llama-server` process alone at 19.3GB RSS / 63% of 29GB total RAM) — naively raising context risked an OOM on a live, valuable run. Investigated the actual KV-cache cost before assuming that risk was real: this model uses GQA with only 4 KV heads (of 32 attention heads, `qwen3moe.attention.head_count_kv = 4`), across 48 layers — working out to roughly **96KB of KV cache per token at f16**, meaning the observed 19.3GB was almost entirely the 30.5B model's fixed weight size (~17GB), not context-dependent at all. Raising context is comparatively cheap for this specific model.

**Fix:** raised `OLLAMA_CONTEXT_LENGTH` to `32768` (covers both real observed failures — Reddit needed 18,572 tokens, Akuity 16,604 — with ~2x margin) and added `OLLAMA_KV_CACHE_TYPE=q8_0` (quantizes the KV cache, roughly halving its already-small per-token cost, near-lossless in practice) to the same systemd override, then `systemctl --user daemon-reload && systemctl --user restart ollama.service`. Confirmed via the restarted `llama-server` process's own launch flags (`-c 32768 --cache-type-k q8_0 --cache-type-v q8_0`). **Confirmed live that system memory headroom actually improved after the restart** (15GB available vs. 4.8GB before) despite the ~5x larger context — the KV cache was never the dominant memory cost for this model. Correspondingly raised `pkg/submitter/browser.go`'s `maxPromptCharsForModelContext` from `14000` to `80000` (same `(context_tokens - 400) × 2.5 chars/token` conservative formula the original constant used) so the character-based circuit breaker stays in sync with the server's actual new capacity. Updated `TestLikelyExceedsModelContext` to reflect the new threshold, including a case confirming Reddit's real repro size (54,917 chars) now correctly fits under the raised window rather than being routed to manual. `go build/vet/test ./...` all pass.

**Operational cost of the fix itself:** restarting the Ollama service interrupted whatever was mid-generation in the live 82-job run at the time (one job, "Jumio," failed with a connection-reset error and needs a natural retry — not a new bug, direct fallout of the restart, and Ollama's per-request retry logic in `pkg/mcp` already handles transient connection failures on the next job).

**Not yet applied to the in-progress 82-job re-verification's own binary:** `maxPromptCharsForModelContext` is a compile-time constant — PID 3137654 (the isolated run) was built before this change and still enforces the old 14,000-char ceiling even though the Ollama server itself can now handle far more. Whether to rebuild and restart that specific run to benefit the ~55 still-`DISCOVERED` jobs, versus letting this fix apply from the next full-backlog batch onward, is a decision left to whoever is driving that run next (see the task journal) — restarting it also interrupts whatever's currently in-flight and needs the documented isolated-run restart procedure.

### 59. Generic submit-button selector could click a hidden anti-spam-widget button instead of the real submit control (Resolved 2026-07-24)
**Found live during the 82-job re-verification**, while monitoring for status changes: "Nova" (`jobs.lever.co/ioconnectservices.com/...`) reached `SolveValidationErrors`'s retry path and failed with:
```
Auto-Submit failed for Nova: playwright: timeout: Timeout 30000ms exceeded.
Call log:
  - waiting for locator('input[type=\'submit\'], button[type=\'submit\'], button:has-text(\'Submit\'), button:has-text(\'Apply\')').first()
    - locator resolved to <button type="submit" class="hidden" id="hcaptchaSubmitBtn"></button>
  - attempting click action
    - waiting for element to be visible, enabled and stable
    - element is not visible
    (56 retries, 30s total)
```

**Root cause:** `pkg/submitter/browser.go`'s retry-path submit click (line ~874, inside the `SolveValidationErrors` branch) used `submitLocator.First()` on `input[type='submit'], button[type='submit'], button:has-text('Submit'), button:has-text('Apply')`. That broad selector matched a hidden `<button type="submit" id="hcaptchaSubmitBtn">` — an internal control belonging to Lever's hCaptcha anti-spam embed (same widget class as bug #23's iframe, not the applicant-facing form) — before it ever reached the real, visible submit button later in the DOM. `.First()` doesn't consider visibility, so Playwright spent the full click timeout retrying a click on an element it will never consider clickable, surfacing as a generic timeout rather than a clear "wrong element" signal.

**Fix:** new `firstVisibleLocator(loc playwright.Locator, count int) playwright.Locator` in `pkg/submitter/browser.go` — iterates `loc.Nth(i)` for `i` in `[0, count)`, returns the first match where `IsVisible()` reports true, falling back to `loc.First()` if none report visible (better to attempt and get a clear timeout than silently give up when visibility detection itself might be unreliable). The retry-path submit click now calls this instead of `submitLocator.First()`. Only one call site in the codebase used this exact selector pattern, so the fix is self-contained.

**Tests:** `TestFirstVisibleLocator_SkipsHiddenCaptchaButton` (reproduces the exact live shape: index 0 hidden, index 1 visible, confirms index 1 is returned) and `TestFirstVisibleLocator_FallsBackToFirstWhenNoneVisible`. Extended `MockLocator` (`pkg/submitter/browser_test.go`) with `Nth`/`IsVisible` overrides (both default to matching prior behavior — `Nth` returns the receiver, `IsVisible` returns true — so no existing test's behavior changed). `go build/vet/test ./...` all pass.

**Not yet re-verified live against the specific "Nova" posting** — the fix is compiled but the isolated 82-job re-verification's already-running binary (PID 3137654) doesn't pick up code changes without a restart, and this bug was found mid-run rather than before it started. Requeuing "Nova" specifically (`cmd/requeue -pattern '%ioconnectservices%' -status FAILED_SUBMIT -confirm`) and restarting with a freshly-built binary would give a real live confirmation, but that also interrupts whatever the run is currently mid-flight on — left as a decision for whoever is driving that run next rather than done unilaterally here. See `documentation/task_journals/2026-07-25_monitor-live-run-and-fix-bugs.md` for the current consolidated run state.

### 58. Stale career_chunks embedding dimension silently zeroed out all live RAG resume-context retrieval (Resolved 2026-07-24)
**Found while implementing improvements.md #22** (rank the discovery queue by resume-fit similarity): a live 3-job test run of the new `cmd/rankjobs` backfill returned `fit_similarity = 0.0` for every job, which shouldn't happen across genuinely different job titles unless something structural was wrong, not just "genuinely low similarity."

**Root cause:** `pkg/parser/rag.go`'s `CosineSimilarity` has a length-mismatch guard (`if len(a) != len(b) { return 0 }`, added defensively, never meant to be load-bearing). Confirmed live: every one of the 8 rows in `career_chunks` stores a 3072-dimension embedding, but the currently configured `OLLAMA_EMBED_MODEL` (`nomic-embed-text`, confirmed via `/api/show`'s `embedding_length`) actually produces 768-dimension vectors. `cmd/agent`'s RAG ingestion only ever runs when `career_chunks` is empty (`len(existingChunks) == 0`) — once any chunks exist, from however long ago and under whatever model was configured at the time, they are never refreshed. The stored chunks predate the current embed-model configuration (exact prior model/provider not recoverable from `git log` — no code path in this repo has ever called anything but `nomic-embed-text` via the `ollama` provider's `Embed`, so the mismatch most likely predates a change to `.env`/`OLLAMA_EMBED_MODEL` outside version control, or a fresh model pull that changed what `nomic-embed-text` resolved to locally).

**Real-world impact:** `parser.RetrieveTopK`, called live in `cmd/agent`'s main worker loop (`cmd/agent/main.go`, RAG retrieval before every `ScoreJob`/tailoring call) scores every one of the 8 chunks 0 against any real job embedding, because every comparison hits the dimension mismatch. `sort.Slice`'s stable sort then returns the chunks in storage-insertion order regardless of actual relevance — so every tailored resume and cover letter generated by this pipeline, for its entire live history, has been built from an arbitrary fixed set of resume chunks rather than the ones actually most relevant to each job. No error, crash, or log line ever indicated this; `CosineSimilarity`'s guard exists purely to avoid a panic on mismatched slice lengths, not to signal an upstream configuration problem.

**Fix:** `parser.IngestResumeChunks(embed func(string) ([]float32, error), profilePath string) (int, error)` extracts the previously-inline ingestion logic (clear `career_chunks`, re-chunk `USER_PROFILE.md`, re-embed, re-save) into a reusable, independently-testable function. `parser.CareerChunksNeedReingest(existing []storage.CareerChunk, freshDim int) bool` detects a dimension mismatch. `cmd/agent`'s startup RAG-ingestion block now probes the configured embed model's actual current dimension with one cheap `GetEmbedding` call whenever chunks already exist, and re-ingests automatically if it no longer matches what's stored — so this class of drift self-heals on the next `cmd/agent` restart regardless of cause. New `cmd/reingest` CLI exposes the same ingestion for two cases the automatic startup check can't reach without a restart: fixing a live database a `cmd/agent` process is already using (a separate short-lived writer takes effect on that process's very next job — `career_chunks` is read fresh per job via `RetrieveTopK`, not cached — so this required no restart of PID 3137654, the live 82-job re-verification run in progress at the time this was found), and manually refreshing after editing `USER_PROFILE.md`. Tests: `TestCareerChunksNeedReingest`, `TestIngestResumeChunks` (confirms the pre-existing stale chunk gets cleared, not merged), `TestIngestResumeChunksSkipsFailedEmbeddings`. `go build/vet/test ./...` all pass.

**Live remediation:** ran the new `cmd/reingest` against the real `applications.db` while PID 3137654 (the 82-job re-verification) kept running unaffected — queued behind the live run's single-slot Ollama usage (~19 minutes total, several individual embed calls each themselves queued behind separate `ProcessJobApplication` calls), completed cleanly (`Re-ingested 9 career chunk(s)`). Confirmed all 9 `career_chunks` rows now store 768-dimension embeddings, matching `nomic-embed-text`. The 3 `job_funnel` rows the `cmd/rankjobs` test run had written `fit_similarity = 0.0` against the stale chunks were reset to `NULL` beforehand so they'd get correctly re-scored rather than silently kept as false data — re-ran `cmd/rankjobs -limit 3` against the same 3 real jobs and confirmed real, non-zero, non-uniform scores this time (`0.610`, `0.600`, `0.586`). Both this bug and improvements.md #22 (which surfaced it) are now verified end to end, not just unit-tested.

### 57. Forms too large for Ollama's context window burned a full doc-gen cycle before failing with an ugly HTTP 400 (Resolved 2026-07-24)
**Symptom:** live during the 82-job re-verification run, after bug #52's `StripPresentationalAttrs`/75k-limit fixes already shipped: Reddit failed again with `ollama returned HTTP 400: {"error":{"code":400,"message":"request (18572 tokens) exceeds the available context size (6144 tokens)..."}}`. Shortly after, Akuity — a different real Greenhouse posting — hit the identical error (16,604 tokens against the same 6,144 limit), confirming this wasn't a one-off.

**Root cause:** the character-based circuit breaker (`pkg/mcp/client.go`'s `payloadSafetyLimits`) and the local Ollama model's actual context window are two independent constraints. A payload can sit comfortably under the 75k-character safety limit and still overflow the model's real 6,144-token budget, since HTML content runs roughly 3 characters per token — well short of the 1:1 assumption a pure character limit implies. Both real failures happened at the validation-retry stage, meaning a full ~20-40 minute doc-generation cycle had already completed before the form was discovered to be unfillable at all.

**Fix applied 2026-07-24:** added `likelyExceedsModelContext` (`pkg/submitter/browser.go`) — a conservative 14,000-character budget (2.5 chars/token against the observed 6,144-token window, minus ~400 tokens reserved for the system prompt and EEO context) checked against the combined DOM-plus-profile-context length *before* calling either `ExtractFormMapping` or `SolveValidationErrors`, not just the retry path (the same 6,144-token ceiling applies to the very first mapping attempt too, which doesn't benefit from the retry path's extra DOM trimming and so is if anything more exposed). New `ErrFormTooLargeForModel` sentinel error, routed to `MANUAL_REQUIRED` in `cmd/agent/main.go` exactly like the existing `ErrAuthWall` path (bug #18) — the tailored documents are already generated and saved, so nothing is lost, just no longer force-fit into an automated submission the model structurally cannot process. 3 new tests, `go build/vet/test ./...` all pass.

**User's explicit choice (2026-07-24):** raising Ollama's `num_ctx` was considered and rejected for now — this machine has documented OOM history (bug #13) and increasing the context window directly increases per-request RAM usage (KV cache). Routing to manual submission avoids that risk entirely; revisit only if manual-queue volume from this specific reason becomes a real burden.

**Not yet verified live** — needs a fresh oversized form (Reddit or Akuity, once requeued) to confirm it now routes straight to `MANUAL_REQUIRED` instead of attempting a doomed LLM call.

### 56. Dashboard has no tile for BLOCKED_CAPTCHA or INVALID_URL, silently omitting 9% of all job_funnel rows (Resolved 2026-07-24)
**Symptom:** user reported the exact numbers shown on the dashboard UI (3140 in queue, 112 skipped, 282 failed, 12 manual queue) and asked me to verify accuracy.

**Investigation:** cross-checked each tile's exact query (`cmd/dashboard/main.go`'s `serveMetrics`) directly against the live DB — every number matched (the 282 vs 283 "Failed" discrepancy was pure live-data timing, the batch was actively running). But summing the six displayed tiles against `SELECT status, COUNT(*) FROM job_funnel GROUP BY status` left a gap: 337 rows (301 `INVALID_URL`, 36 `BLOCKED_CAPTCHA`) belonged to neither the shown tiles nor any hidden aggregate — they were simply never queried at all. Also found a smaller, related inconsistency while fixing this: the "last skipped" detail widget's own query already included `BLOCKED_CAPTCHA` (`WHERE status IN ('SKIPPED', 'BLOCKED_CAPTCHA')`), while the `Skipped` tile's count never did — the detail widget could show a CAPTCHA-blocked company while the tile total silently excluded it.

**Fix applied 2026-07-24:** added `BlockedCaptcha`/`InvalidURL` fields to `Metrics`, their own count queries, a `statusReason` case for `INVALID_URL`, and two new dashboard tiles (`cmd/dashboard/index.html`, two new neon colors — pink, teal — added to the existing palette, matching every other tile's CSS pattern exactly). Narrowed the "last skipped" query to just `SKIPPED` now that `BLOCKED_CAPTCHA` has its own dedicated tile, fixing the inconsistency found along the way. 3 new/updated tests (`go build/vet/test ./...` all pass), and visually verified via a Playwright screenshot of the real running dashboard against the live DB — both new tiles render correctly with accurate counts (36, 301).

### 55. Jobs killed mid-flight get permanently stuck in PROCESSING, never retried, inflating the dashboard's live count (Resolved 2026-07-24)
**Symptom:** user asked why the dashboard UI showed 235 jobs as "processing." A single-worker run can only ever have one job truly in flight at a time — 235 was never a live figure.

**Root cause:** every `kill -9` used to restart a run tonight (and every other night since 2026-07-21, per this file's own Operational Trap notes on why `kill -9` is necessary in the first place) kills whatever job the worker was mid-way through, and `AttemptSubmit`'s `PROCESSING` status write (`cmd/agent/main.go`, right before dispatch) never gets a chance to be reverted — no signal handler, no cleanup path, nothing. `GetDiscoveredJobs` only ever queries `status = 'DISCOVERED'` (`pkg/storage/manager.go`), so a row stuck at `PROCESSING` is invisible to every future run, forever, regardless of how many fixes ship afterward. Confirmed live: `MIN(last_updated)` among `PROCESSING` rows was 2026-07-21, `MAX` was the current moment — three full days of silent accumulation. Directly re-encountered twice tonight already (Enveritas, Akuity) while manually managing the 82-job re-verification run, each requiring a one-off manual `UPDATE` to recover.

**Fix applied 2026-07-24:** `storage.ReapStaleProcessingJobs()` — a single `UPDATE job_funnel SET status = 'DISCOVERED' WHERE status = 'PROCESSING'`, called once in `cmd/agent/main.go` right after `InitDB`, before any job can be marked `PROCESSING` by the run doing the reaping. Safe by construction: a freshly-started process cannot have produced any `PROCESSING` row itself yet, so anything already in that state at startup is unconditionally orphaned, regardless of worker count. 1 new test (`TestReapStaleProcessingJobs`), `go build/vet/test ./...` all pass. One-time cleanup: reset 234 of the 235 stale rows directly (excluded the one genuinely-active job at the time, confirmed via the running process's own log).

**Verified against real data:** `SELECT COUNT(*) FROM job_funnel WHERE status='PROCESSING'` dropped from 235 to 1 (the genuinely active job) immediately after the cleanup pass. The code fix itself will self-verify on the next restart of any kind — expect a `[Agent] Reset N stale PROCESSING row(s)...` log line whenever N > 0.

### 54. Raw-HTML captcha pre-check misclassifies Ashby's client-rendered SPA shell as a block (Resolved 2026-07-24)
**Symptom:** while trying to get bugs #8/#10/#14 a fresh confirmed success through the generic Learner Module path (needed since neither has been exercised under the new post-#53 confirmation logic yet), two different, currently-`DISCOVERED` Ashby postings both hit `[Worker-%d] Security/Captcha block detected` — the raw-`net/http`-fetch pre-check in `cmd/agent/main.go` (bug #46's area) — before fit-scoring ever ran.

**Investigation:** wrote a standalone probe using the exact same plain `net/http` fetch (no browser, no JS execution) the real check uses, against one of the two flagged URLs. Found: raw HTML 41,996 bytes (a substantial, real response, not a small interstitial), but `parser.PruneDOMToText` extracted **0 characters** of visible text, and the raw HTML does contain a "recaptcha" substring. This is exactly bug #46's `widgetOnlyPhrasing && len(pruned) < 200` corroborating-signal condition — but the underlying assumption (a genuine interstitial replaces the real page, leaving little text behind) doesn't hold for Ashby: it renders all real content client-side via JavaScript, so a non-JS-executing fetch sees an empty shell on *every* posting, genuinely blocked or not. Checked `cmd/requeue -stats -source ashby`: only 9 total attempts (3 `BLOCKED_CAPTCHA`, 6 `FAILED_SUBMIT`, 0 `APPLIED`) — too small a sample to call this a 100%-deterministic block, but a real, reproducible false-positive mechanism regardless. Tried to find a genuine currently-blocked page from another platform to calibrate a general raw-HTML-size threshold instead of an Ashby-specific carve-out, but the other `BLOCKED_CAPTCHA` rows sampled were either stale (no longer reproducible) or a separate, distinct issue (`developers.smartrecruiters.com`/`developers.pinpointhq.com` API-docs pages being scraped as if they were postings).

**Addendum, same investigation:** the `developers.smartrecruiters.com`/`developers.pinpointhq.com` docs-page sighting turned out to be its own real, fixable bug, same class as the already-fixed board-index junk-URL bugs (#41/#42/#44) — confirmed live: `careers.smartrecruiters.com/<company>` tenant board-index pages are already correctly caught by the existing path-segment check, but SmartRecruiters' and Pinpoint's own API-documentation subdomain (`developers.`) was never a company tenant and had no filter at all. Fixed in `pkg/scraper/funnel.go`'s `IsKnownJunkJobURL`: exact-host-match exclusion for `developers.smartrecruiters.com` and `developers.pinpointhq.com`, alongside the existing corporate-subdomain exclusions for Workday/homerun.co/BambooHR. Confirmed a genuine Pinpoint company-tenant subdomain (`sunking.pinpointhq.com/postings/...`) still passes. 4 new test cases. One-time DB pass flipped the 8 existing `BLOCKED_CAPTCHA` rows for these two hosts to `INVALID_URL`.

**Fix applied 2026-07-24:** since this check cannot distinguish "SPA shell for a real posting" from "SPA shell leading to a block" without executing JavaScript, added `clientRenderedSPAHosts`/`isClientRenderedSPAHost` in `cmd/agent/main.go` (host-suffix matching, same convention as `authGatedATSHosts` in `pkg/submitter/browser.go`) — for these hosts, only the explicit block phrasing (`genuineBlockPhrasing`: Cloudflare + "verify you are human"/"attention required") is trusted; the widget-substring fallback is skipped entirely rather than trusting a text-length signal that's meaningless for this platform shape. Currently lists only `ashbyhq.com`. 5 new test cases, `go build/vet/test ./...` all pass.

**Not yet verified live** — needs a fresh Ashby posting to reach fit-scoring instead of being killed at this pre-check.

### 53. isSubmissionConfirmed only ever ran for Lever/Greenhouse/LinkedIn — every other ATS platform's APPLIED had zero confirmation evidence (Resolved 2026-07-24)
**Symptom:** the user asked to verify that `APPLIED` jobs actually generate email confirmations, worried about false positives. While investigating, added logging to `isSubmissionConfirmed` (bug #51's addendum) to expose which of its three evidence tiers fired for each success. The very first live example after shipping that logging (Kobie Marketing, Lever) landed on the weakest tier — "URL changed, no confirmation or error wording found."

**Investigation:** wrote a read-only probe (no submission attempted) against a separate, untouched Lever posting to check whether that weak tier could plausibly be masking a real failure. Found 18 fields with native HTML5 `required` attributes and no `formnovalidate` override on the submit button — meaning a blank required field makes the *browser itself* block the submit client-side, with no navigation and no error text ever rendered into the page's HTML. Tracing why the "URL changed" fallback would fire true in that exact scenario found the real defect: `isSubmissionConfirmed`'s baseline for "did the URL change" was `applyURL`, the *original job-posting URL* captured once at the top of `AttemptSubmit` — but bug #47's click-to-reveal step navigates the page away from that URL (to a `.../apply` sub-page) *before any fill or submit ever happens*, for every Lever/Greenhouse job. So "the URL changed" was trivially true by the time confirmation ran, regardless of whether the submit click itself did anything at all.

**Second, larger gap found while mapping the fix:** tracing every code path that can lead to `AttemptSubmit` returning `nil` (success) found `isSubmissionConfirmed` is only ever reached by the Lever, Greenhouse, and LinkedIn dispatch branches, which are the only ones that fall through to the loop's shared bottom code. Every other path — the cached-mapping fast path (for any domain the Learner Module previously mapped), and both `AttemptVisionSubmit` call sites (used whenever the generic Learner Module handles an unrecognized ATS, i.e. SmartRecruiters, Ashby, Homerun, Pinpoint, Jobvite, BambooHR, applytojob.com, recruitee.com, and any platform without a dedicated handler) — returned success straight from `handleDynamic`'s bare error value, with no confirmation evidence of any kind. This is the exact unverified-success pattern bug #51 fixed; that fix was simply never extended past three of the many ATS paths.

**Fix applied 2026-07-24:** extracted `confirmOrError(page, companyName, urlBeforeClick, autoSubmitClick) error` as a shared helper (wraps the existing wait/URL/content/`isSubmissionConfirmed`/logging sequence). Threaded a new `urlBeforeSubmitClick` variable through every dispatch branch in `AttemptSubmit`'s loop (LinkedIn, Greenhouse, Lever, the generic Learner Module's primary `handleDynamic` call, and the validation-retry branch), captured immediately before each branch's own submit click rather than reusing the stale `applyURL` parameter. Wired `confirmOrError` into the cached-mapping fast path. Made `AttemptVisionSubmit` (`pkg/submitter/vision.go`) self-contained: it now captures its own pre-click URL and calls `confirmOrError` before returning success, so both loop-internal Vision-fallback call sites were changed from `execErr = AttemptVisionSubmit(...)` (which let it re-enter the loop's own confirmation check against a now-stale baseline) to a direct `return AttemptVisionSubmit(...)`, since its result is now already fully verified. 4 new tests (`TestConfirmOrError_*`), including `TestConfirmOrError_CatchesNativeValidationBlock` reproducing the exact live-repro shape. `go build/vet/test ./...` all pass.

**Not yet verified live** — needs a fresh submission on a non-Lever/Greenhouse/LinkedIn platform (e.g. Ashby or Homerun, both confirmed to have genuinely fillable forms per bug #50) to confirm the newly-wired confirmation check doesn't introduce false negatives on a real success.

### 52. SolveValidationErrors sends the whole page's DOM, tripping the LLM-cost circuit breaker and losing otherwise-successful applications (Resolved 2026-07-23)
**Symptom:** caught while resuming a watch session on the live batch (PID 2542429, running bug #51's fix): `career_agent.log` showed a real Greenhouse posting ("Senior GTM Systems & Automation Engineer" at Cobalt.io, fit score 90) go all the way through real document generation and a full field fill, then get lost outright — `Submission failed validation. Retrying...` followed immediately by `CIRCUIT BREAKER TRIGGERED: Payload size 104932 exceeds safety limit (50k chars). Aborting to prevent runaway LLM costs.` `job_funnel.status` for this job was `FAILED_SUBMIT`.

**Root cause:** `pkg/mcp/client.go`'s `incrementAndLogAPICall` aborts any LLM call over 50,000 characters as a blanket safety net against runaway cost. `SolveValidationErrors`'s prompt is `Applicant Profile + Failed Form DOM`, where the DOM comes from `AttemptSubmit`'s validation-retry branch (`pkg/submitter/browser.go`): `target.HTML()` (the *entire* page or frame content, not scoped to the form) run through `parser.PruneDOM`, which only strips `<script>/<style>/<svg>/<path>/<iframe>/<noscript>/<meta>/<link>` — everything else (nav, footer, marketing copy, every class/data/aria attribute on every element) survives. On a modern React-rendered Greenhouse board page, that's well over 100k characters even after pruning; the circuit breaker isn't malfunctioning, it's correctly catching a payload that should never have been that large, since only the form's own fields matter for solving a validation error.

**Fix applied 2026-07-23:** added `parser.PruneDOMToForm` (`pkg/parser/dom.go`) — runs the existing `PruneDOM` pass, then narrows to the first `<form>` element found and renders only that subtree; falls back to the full pruned document when no `<form>` tag exists (covers ATS forms assembled without a real `<form>` element, so this can't regress a page that previously worked). Wired into the validation-retry call site only (`pkg/submitter/browser.go` line ~761); the initial `ExtractFormMapping` call site is untouched since it may need full-page context to find where the form even is. 2 new tests (`TestPruneDOMToForm_ScopesDownToFormWhenPresent`, `TestPruneDOMToForm_FallsBackToFullDocumentWhenNoFormTag`), both passing. `go build/vet/test ./...` all pass.

**Verified against real data:** confirmed via direct `applications.db` query that the diagnosed job (`job-boards.greenhouse.io/cobaltio/jobs/8603198002`) was genuinely `FAILED_SUBMIT` from this exact failure. Requeued it (`cmd/requeue -pattern '%cobaltio/jobs/8603198002%' -status FAILED_SUBMIT -confirm -clear-dedup`) back to `DISCOVERED`. Rebuilt and restarted the batch (PID 2542429 → 2579802) to pick up the fix; confirmed sole instance via `ps aux` and log growth. Live confirmation that this specific job now reaches a genuine `APPLIED` is still pending — the requeue happened after this restart's queue snapshot was already loaded, so it won't be retried until either a future restart or a `TARGET_JOB_URL` targeted run.

**Recurred 2026-07-24, root-caused and fixed for real this time:** the 82-job re-verification run's first outcome (Reddit, `job-boards.greenhouse.io`, fit 90) hit the exact same circuit breaker (102,963 chars) even with `PruneDOMToForm` live. A probe confirmed this posting genuinely has a `<form>` element and `PruneDOMToForm` correctly scoped to it — but that form element alone is 98,255 characters, because modern Greenhouse themes wrap every field in several layers of styling `<div>`s and accessibility attributes (`class`, `aria-describedby`, `aria-hidden`, `role`, `tabindex`, etc.). The fix wasn't wrong, the form is just genuinely that large. Added `parser.StripPresentationalAttrs` (`pkg/parser/dom.go`) — strips attributes that carry no selector-relevant information (styling/state/most `aria-*`), deliberately keeping `aria-label`/`aria-labelledby` since `ExtractFormMapping`/`SolveValidationErrors` both rely on them as a fallback label source. On the real Reddit form: 98,255 → 33,629 characters (66% reduction), comfortably under the 50k limit. Wired into the validation-retry call site alongside `PruneDOMToForm`. 3 new tests, `go build/vet/test ./...` all pass.

**Recurred a third time on the very same Reddit posting**, requeued into the re-verification run with both fixes live: this time 54,917 chars — the two-round trim clearly worked (down from 102,963), but still ~10% over 50k. Probed directly: this specific form genuinely has 35 real input/textarea fields and 24 labels (Reddit's actual screening questionnaire), no `<select>` dropdown bloat, no obviously-strippable fat left — the remaining size is proportional to real field count, not inefficiency. Raised the limit specifically for `SolveValidationErrors` to 75,000 chars (`payloadSafetyLimits` in `pkg/mcp/client.go`, `incrementAndLogAPICall` now looks up a per-call-type override instead of one hardcoded constant) — still far below the ~103-145k this call site saw before any of these three fixes existed, so a real regression back toward sending whole pages would still trip the breaker; every other call type keeps the original 50k. 3 new tests, `go build/vet/test ./...` all pass.

**Not yet verified live** — needs a fresh requeue of this exact Reddit posting into an active run to confirm the raised limit actually lets it through to a genuine `APPLIED`.

### 51. Post-submit success check trusted any URL change, not proof of an actual successful submission (Resolved 2026-07-23)
**Symptom:** the user asked why they weren't getting email confirmation receipts for today's real `APPLIED` jobs, the way they normally would applying manually.

**Investigation:** `cmd/tracker`'s classifier only recognizes REJECTED/INTERVIEW_REQUESTED-shaped emails — it has no "application received" category at all, so a quick tracker scan finding nothing proved nothing either way. Searched the connected Gmail account directly instead and found real, concrete evidence: a genuine Lever rejection email (`no-reply@hire.lever.co`, "Avive Solutions," dated today) and a genuine Workable confirmation-then-rejection pair (ZeroFox) — proof that `AttemptSubmit` is producing real submissions that reach real employer systems, not pure false positives. Nearly every one of these emails (Lever, Workable, and even unrelated LinkedIn Easy Apply confirmations) was sitting in the account's Trash rather than the inbox; `pkg/tracker`'s code has no delete/trash logic anywhere, so that routing is a Gmail-side filter/rule outside this codebase's control — flagged to the user to check their own Gmail filters, not something fixable in this repo. Not every ATS is configured to send an immediate "received" email either, so an absent receipt isn't always evidence of failure on its own.

**Real code issue found along the way:** while reading the post-submit verification logic (`pkg/submitter/browser.go`, the validation-error retry loop), the only success signal was `currentURL != applyURL || urlContains(thank/success/confirmation)`. A validation-error page reached via a redirect, or a bounce back to the company's careers listing, would satisfy `currentURL != applyURL` just as easily as a genuine success — the check never looked at page *content* to distinguish the two, and never handled AJAX-style ATS themes that show a success message without changing the URL at all (which would have been wrongly retried as a failure).

**Fix applied 2026-07-23:** added `isSubmissionConfirmed(applyURL, currentURL, pageContent)` in `pkg/submitter/browser.go`: prefers explicit confirmation wording anywhere on the page content (`submissionConfirmationPhrases` — works even when the URL never changed), falls back to the URL itself looking like a confirmation page, and only falls back further to "URL changed" when the resulting page doesn't show validation-error wording (`submissionErrorPhrases`). 5 new test cases in `TestIsSubmissionConfirmed`, all passing, including the exact false-positive shape this fix targets. `go build/vet/test ./...` all pass.

**Not yet verified live** — needs a fresh submission to confirm the tightened check doesn't introduce false negatives (rejecting a real success it used to accept).

**Addendum 2026-07-24:** user asked whether `APPLIED` jobs actually generate a confirmation email, worried about false positives — Gmail search for the last several real Lever `APPLIED` jobs tonight found zero traceable emails (user believes they may have deleted at least one relevant email, so inconclusive on its own). While investigating, found `isSubmissionConfirmed` had no way to tell, after the fact, which of its three evidence tiers (explicit confirmation phrase / confirmation-looking URL / weak "URL changed, no error text" last resort) actually fired for a given success — the function returned a bare bool. Added a `submissionConfirmationReason` return value and logged it at the call site (`[Auto-Submit] Submission confirmed for %s (%s)`), so future log reads (or a future automated cross-check) can distinguish strong evidence from the weak fallback without depending on email at all. No behavior change — same three tiers, same outcomes, just now observable. Tests updated to assert the reason per case. `go build/vet/test ./...` all pass.

### 50. Workable requires account sign-in on every posting — same structural class as Workday (Resolved 2026-07-23)
**Symptom:** the user asked why Ashby, Workable, and Homerun all sat at 0 `APPLIED` despite #45/#46 clearing the CAPTCHA false positives that were killing them. A per-source `job_funnel` breakdown (`cmd/requeue -stats`) confirmed `BLOCKED_CAPTCHA` had dropped to 0 for all three, but `FAILED_SUBMIT` remained high (12/12 Workable attempts, 6/7 Ashby, 6/6 Homerun) — so something downstream of the CAPTCHA fix was still killing every attempt.

**Investigation:** wrote a standalone probe and checked several real, currently-`FAILED_SUBMIT` postings from each source directly:
- **Workable** (`jobs.workable.com/view/.../at-telestream`, `.../at-callminer`): zero real form fields on load, no "Apply" button found even after a proper wait, and the page's own text contains "log in" — a genuine account-gate, same structural shape as Workday (bug #18). Confirmed real across 2 independent postings.
- **Ashby** (`jobs.ashbyhq.com/xbowcareers/...`): a first, too-fast probe (2s wait, `DOMContentLoaded`) falsely read this as broken (0 inputs, no button found). A proper probe (`NetworkIdle` wait + 3s, then click) found the real "Apply for this Job" button and a genuinely fillable 12-field form behind it. **Not a structural blocker** — Ashby's failures are happening in the Learner Module's mapping/fill quality, the same class as bugs #8/#10/#14 (still open, still gating the Usability Gate).
- **Homerun**: two `FAILED_SUBMIT` rows turned out to be `homerun.co`'s own marketing pages (`/hiring-kits/...`, `/job-description-templates/...`), not real postings — but `IsKnownJunkJobURL` in `pkg/scraper/funnel.go` already filters these; the rows in the DB are just stale, pre-fix data, not a live gap (no code change needed). A real posting (`root-sustainability.homerun.co/senior-software-engineer`) had a genuinely fillable 11-field form once probed properly, same as Ashby — its failures are also Learner Module quality, not structural.

**Fix applied 2026-07-23:** added `workable.com` to `authGatedATSHosts` in `pkg/submitter/browser.go` (the exact list Workday/bug #18 already uses) — `AttemptSubmit` now routes Workable jobs straight to manual submission with tailored documents already generated, instead of burning a full Learner Module + Vision cycle on a form that will never be reachable pre-auth. Also moved `workable.com` from priority tier 1 to tier 3 (alongside Workday) in `sourcePriorityCASE` (`pkg/storage/manager.go`) so it no longer competes for worker cycles ahead of platforms that can actually reach `APPLIED`. Added 4 new cases to `TestIsKnownAuthGatedHost` in `pkg/submitter/browser_test.go`, all passing. `go build/vet/test ./...` all pass.

**Not yet verified live** — needs a fresh Workable posting to confirm it now routes to `MANUAL_REQUIRED` immediately instead of a wasted fill cycle.

### 49. handleGreenhouse's hardcoded submit selector doesn't exist on modern-board postings (Resolved 2026-07-23)
**Symptom:** live 2026-07-23, right after the priority-queue change (this session) put Greenhouse jobs first in the queue and #45/#46/#47's fixes let one actually reach `handleGreenhouse`: `job-boards.greenhouse.io/alphasense/jobs/8420858002` (Staff Site Reliability Engineer) filled every field successfully — no errors on `first_name`/`last_name`/`email`/`phone` — then failed at the very last step: `failed to click submit: playwright: timeout: Timeout 30000ms exceeded`.

**Root cause:** `handleGreenhouse`'s submit step only ever tried one hardcoded selector, `input#submit_app`. A standalone probe against the live posting confirmed that selector has **zero matches** on this page — it's from Greenhouse's legacy embed theme, but this posting uses Greenhouse's modern board template, whose actual submit control is an unidentified `<button type='submit'>Submit application</button>` (also present on the page: an "Apply" button and a "Quick Apply with MyGreenhouse" button, both wrong targets — confirms a naive `:has-text('Submit')` search wasn't safe without narrowing by `type='submit'` too).

**Fix applied 2026-07-23:** in `pkg/submitter/browser.go`'s `handleGreenhouse`, check `input#submit_app`'s count first (preserves legacy-theme postings unchanged) and fall back to `button[type='submit']` only when the legacy selector has zero matches. Added `TestHandleGreenhouse_SubmitFallsBackWhenLegacySelectorMissing` and `TestHandleGreenhouse_SubmitUsesLegacySelectorWhenPresent` in `pkg/submitter/browser_test.go`, both passing. `go build/vet/test ./...` all pass.

**Verified live 2026-07-23 14:47:** requeued the exact same diagnosed job (`job-boards.greenhouse.io/alphasense`) after the fix shipped — it reached `handleGreenhouse` again, and `job_funnel.status` is now `APPLIED`. Third fresh `APPLIED` this session (after two via Lever, `smarsh` and `DexCare`), first via Greenhouse.

### 48. Lever click-to-reveal (bug #47's fix) doesn't fire on a second real posting — possible page staleness after the long doc-gen wait (Resolved 2026-07-24, not reproduced)
**Symptom:** live 2026-07-23, shortly after #47 shipped and was confirmed working on `smarsh`: a second real Lever posting (`jobs.eu.lever.co/pnlfin/024459c9-ba1b-4e36-b173-72b4f46a72d4`, Finom) went through the same code path and failed with the identical `form failed to render in time: playwright: timeout: Timeout 30000ms exceeded` #47 was supposed to have fixed — but with no `Clicked an Apply-labeled element` log line beforehand, meaning `clickApplyIfPresent`'s locator found zero matches this time (not a click failure, a *no-match* result).

**Investigation so far:** wrote a standalone probe against the same URL and confirmed the page genuinely has an "APPLY FOR THIS JOB" button (all-caps, still `:has-text('Apply')`-matchable — Playwright's `has-text` is case-insensitive). Re-ran `clickApplyIfPresent`'s exact selector against a *fresh* page load of the same URL: found 1 match, clicked with no error. So the selector itself is not the problem, and the button genuinely exists.

**Working theory, not yet confirmed:** the real difference between the probe and the live run is time — `AttemptSubmit` navigates to the page, then spends the full doc-generation window (~14-16 minutes on this machine's CPU-only Ollama) before ever attempting the click, while a fresh probe interacts within seconds of navigation. A page left open that long could plausibly go stale in a way a quick probe can't reproduce: a session/idle-timeout reload, lazy-unmounted content, or an anti-bot heuristic reacting to a suspiciously long dwell time with no interaction. Not yet confirmed — would require watching a live ~15 minute cycle end-to-end (page state at click time vs. at nav time) to prove, which wasn't done this session given the cost. Only one occurrence so far; needs a second live repro before treating the staleness theory as confirmed root cause.

**No fix attempted yet** — filed for a future session to either reproduce with direct evidence (e.g. a screenshot taken immediately before the click attempt in the real flow) or downgrade/close if it doesn't recur.

**Closed 2026-07-24, `/groom_backlogs` pass — not reproduced despite ample opportunity under the exact theorized precondition:** the 82-job re-verification run (now consolidated in `documentation/task_journals/2026-07-25_monitor-live-run-and-fix-bugs.md`) processed at least 11 further Lever postings on 2026-07-24 alone (Gateway Automation Engineer, AHEAD, Agentic Engineer, Dijital Team, Celara, Senior Infrastructure Software Engineer, Eneba, Grant Street Group, Instrumentl, Aircall, Kobie Marketing), each one going through the identical ~10-20 minute `generateDocsFunc` doc-gen wait between navigation and the Apply click that this bug's staleness theory blamed. `career_agent.log` shows a clean `Clicked an Apply-labeled element to reveal the application form` immediately followed by `Detected Lever ATS` in every one of them — zero occurrences of `form failed to render in time` for Lever anywhere in the log since the original 2026-07-23 13:17:38 sighting (confirmed via `grep -n 'form failed to render in time' career_agent.log`, last Lever-relevant hit predates this bug's own filing). This is a direct test of the staleness theory under matching conditions, not just absence of a fresh repro attempt — treating the original sighting as a one-off (an unlucky timing/network blip) rather than a systemic defect. Reopen if a genuine second occurrence is ever observed with a fresh timestamp.

### 47. Dedicated Greenhouse/Lever handlers never click "Apply" to reveal the form, only the generic Learner Module path does (Resolved 2026-07-23)
**Symptom:** live 2026-07-23, right after fixing #45/#46, a real Lever posting (`jobs.lever.co/smarsh/...`, one of the exact postings used to diagnose #45) reached `handleLever` for the first time — the earlier CAPTCHA false-positives had been killing every Lever/Greenhouse job before they ever got this far, so this code path had effectively never run against real traffic. It failed immediately: `Auto-Submit failed for smarsh: form failed to render in time: playwright: timeout: Timeout 30000ms exceeded` — `handleLever` waits on `input[name='name']` right away with no earlier step.

**Root cause:** confirmed via the same live-probe technique used for #45 — this exact Lever posting has **zero** real form fields on page load; the fields only appear after clicking "Apply for this job." Bug #8 already solved this exact click-to-reveal problem, but its fix (`clickApplyIfPresent`) was deliberately scoped only to the generic Learner Module branch of `AttemptSubmit` — the comment on that fix explicitly says the dedicated Greenhouse/Lever/LinkedIn handlers "weren't implicated" at the time, because no live case had ever reached them with this failure shape. #45/#46's CAPTCHA-detection fix is what finally let real traffic reach `handleLever`/`handleGreenhouse` and exposed the gap.

**Fix applied 2026-07-23:** added `clickApplyIfPresent(page)` plus the same post-click `isCaptchaBlocked` re-check bug #35 added to the Learner Module branch, to both the Greenhouse and Lever dispatch branches in `AttemptSubmit` (`pkg/submitter/browser.go`). `clickApplyIfPresent` no-ops when no "Apply"-labeled element exists, so postings whose form is already present on load are unaffected. `go build/vet/test ./...` all pass.

**Verified live 2026-07-23 12:00:** re-ran the same Lever posting after the fix — log showed the click firing (`Clicked an Apply-labeled element to reveal the application form`), then `Detected Lever ATS. Filling out fields...` with no `form failed to render in time`, and the job reached a genuine `APPLIED` in `job_funnel` — the first fresh `APPLIED` produced this entire verification effort (since 2026-07-21). See #45's Details section for the full fix chain.

### 46. Raw-HTML job-description fetch also misdetects reCAPTCHA/Turnstile widgets as a block, before fit-scoring even runs (Resolved 2026-07-23)
**Symptom:** discovered immediately after fixing #45. Re-ran the exact same Lever `smarsh` posting standalone (`TARGET_JOB_URL` single-job test harness, temporarily added to `cmd/agent/main.go`) and it was still killed instantly: `[Worker-1] Security/Captcha block detected for smarsh. Skipping job to save API tokens.` — before fit-scoring, before #45's fix could even be reached.

**Root cause:** a second, entirely separate CAPTCHA check lives in `cmd/agent/main.go`, run on the raw HTML from a plain `net/http` fetch of the job-description page (no browser, no rendered DOM, no frames) — completely independent of `isCaptchaBlocked` in `pkg/submitter/browser.go`. Its condition included two bare, uncorroborated substring checks: `strings.Contains(lowerHTML, "recaptcha") || strings.Contains(lowerHTML, "cf-turnstile")`, with no check for whether real page content was also present. Since virtually every Greenhouse/Lever/Ashby/Workable job page's raw HTML references a `recaptcha`-hosted script tag as a standard anti-spam measure, this check alone likely accounts for most of #45's DB-observed false-positive rate — it runs *before* #45's check ever gets a chance to matter, on every single job, not just ones that reach the apply stage.

**Fix applied 2026-07-23:** same principle as #45, adapted for raw HTML (no DOM/frame API available here): compute the pruned plain-text content first, then only treat the bare `recaptcha`/`cf-turnstile` substring match as a block if that pruned text is also unusually short (<200 chars) — a genuine interstitial replaces the real page content and prunes down to almost nothing; a real job posting with a standard widget script tag prunes down to hundreds/thousands of characters of real description text regardless. The explicit two-phrase Cloudflare check (`"cloudflare" && ("verify you are human" || "attention required")`) is untouched and still applies unconditionally. `go build/vet/test ./...` all pass.

**Verified live 2026-07-23:** re-ran the same Lever posting after this fix — it passed the description-fetch stage cleanly for the first time (no more instant `Security/Captcha block detected`), reached fit-scoring, then #45's fix, then #47's fix, then a genuine `APPLIED`.

### 45. isCaptchaBlocked misdetects standard reCAPTCHA/hCaptcha anti-spam widgets on real forms as a full block (Resolved 2026-07-23)
**Discovered while investigating the user's question "is there anything else we can do to increase chances of APPLIED working?"** Ran a per-source breakdown of `job_funnel` outcomes for the first time (this had never been done before — prior source exclusions like breezy.hr and Workday, #38, came from diagnosing individual live failures, not a full statistical pass):

| source | total | applied | captcha | failed | manual |
| --- | --- | --- | --- | --- | --- |
| greenhouse | 392 | 29 | 351 (89%) | 12 | 0 |
| lever | 291 | 6 | 264 (91%) | 21 | 0 |
| ashby | 168 | 0 | 162 (96%) | 6 | 0 |
| workable | 66 | 0 | 54 (82%) | 12 | 0 |
| smartrecruiters | 31 | 2 | 11 (35%) | 18 | 0 |

Greenhouse and Lever have dedicated handlers (`handleGreenhouse`, `handleLever`) and were assumed to be among the most reliable platforms — instead they were being killed by `BLOCKED_CAPTCHA` at an 89-91% rate, similar to or worse than SmartRecruiters (the platform CAPTCHA detection, bug #23, was originally built for).

**Root cause:** wrote a standalone Playwright probe and loaded several live, currently-`DISCOVERED` postings directly. `jobs.lever.co/smarsh/...` and `jobs.lever.co/teramind/...`: 35 and 40 real, fillable `<input>/<textarea>/<select>` fields on the main page — genuinely fillable forms — that also embed a standard `hcaptcha.com` invisible anti-spam widget iframe (2 elements). `job-boards.greenhouse.io/rushdownstudios/...`: 21 real fields plus a `recaptcha.net` invisible enterprise-reCAPTCHA anchor iframe. `isCaptchaBlocked` (`pkg/submitter/browser.go`, bug #23) treated the mere presence of any frame whose URL contains `hcaptcha.com`/`recaptcha`/etc. as proof of a full block, with no regard for whether a real form also exists — so every one of these forms was killed by the very first `isCaptchaBlocked` check (`AttemptSubmit`, before doc generation, before any ATS-specific dispatch), before `handleGreenhouse`/`handleLever` ever ran. Genuine DataDome-style interstitials (the original #23 repro, and the still-current SmartRecruiters case) instead replace the real page content, leaving essentially zero real form fields — confirmed live 2026-07-23: SmartRecruiters postings, post-click, show 0 main-page fields and only the captcha frame's own internal fields.

**Fix applied 2026-07-23:** added a `captchaWidgetFieldThreshold` (5) check to `isCaptchaBlocked` — if the main page already has more than 5 real `input`/`textarea`/`select` fields, the frame-host heuristic is not trusted (only the explicit block-wording text check, `isCaptchaContent`, still applies). Genuine interstitials still have ~0 real fields and are unaffected; forms with a benign anti-spam widget no longer get killed. Added `TestIsCaptchaBlocked_GenuineInterstitialWithFewMainFields`, `TestIsCaptchaBlocked_RealFormWithBenignCaptchaWidget`, and `TestIsCaptchaBlocked_ExplicitBlockWordingStillWins` in `pkg/submitter/browser_test.go`, all passing. `go build/vet/test ./...` all pass.

**Verified live 2026-07-23 12:00:** re-ran the exact Lever posting (`jobs.lever.co/smarsh/...`) used to diagnose this bug, after also fixing #46 (a second, independent CAPTCHA false-positive, found immediately while re-verifying this one) and #47 (the dedicated Lever handler's own missing click-to-reveal step, exposed only once #45/#46 stopped killing the job earlier). Result: `job_funnel.status` went from `BLOCKED_CAPTCHA` to a genuine `APPLIED` — the first fresh, real `APPLIED` produced since this whole verification effort began (2026-07-21). This is also the first live confirmation of the Usability Gate's "one full batch run reaches `APPLIED` end to end" checkbox.

### 4. AttemptSubmit form-fill logic never looked inside iframes (Resolved 2026-07-23)
**Symptom:** across live `cmd/agent` runs on 2026-07-20 (after the Playwright/container fixes below, and again post-#3 fix), `AttemptSubmit` reached a live job page and began tailoring successfully several times, but failed at the form-fill stage for different reasons each time: `form failed to render in time: playwright: timeout: Timeout 15000ms exceeded` (Lever, reproduced twice on two different `jobgether` postings in one 40-min window), `failed to fill first_name: playwright: timeout: Timeout 5000ms exceeded` (seen on both a Learner-Module-mapped SmartRecruiters job and an `applytojob.com` job), and one case correctly blocked by the security layer (`malicious prompt injection detected on career page`, indirect prompt injection score 0.85 — this one is a guardrail working as intended, not a bug).

**First (wrong) diagnosis:** the "ran out of time" shape of every failure looked like a pure timing issue (CPU-bound Chromium sharing a heavily-loaded host with a 30B local LLM), so timeouts were doubled/tripled as a first fix. **Verified live not to help** — a second 40-minute run with the longer timeouts produced the exact same failures, at the exact same field, now correctly waiting the *full* new timeout before failing. Waiting longer changing nothing is itself the signal that it was never a timing problem.

**Real root cause:** wrote a standalone script (`playwright-go`, same version as the app) to load a failing job page (`TechnologyNavigators` on SmartRecruiters) directly and inspect it. Found **zero `<input>` tags on the main page and one `<iframe>`** — the real application form is embedded in an iframe. `handleGreenhouse`, `handleLever`, `handleDynamic` (the Learner Module path), and `safeFill` all searched only `page.Locator(...)`/`page.WaitForSelector(...)` — the top-level document — which will never find a field that lives one frame down, no matter the timeout. Also found, separately, that some URLs entering the pipeline (`remotecom` on Greenhouse, `search` on Workable) are generic job-search/listing pages, not individual postings — those will never have an application form either; that's a distinct FunnelEngine URL-quality issue, not this bug.

**Fix applied 2026-07-20 (unverified):** added a `fillTarget` abstraction (`pageTarget`/`frameTarget`) in `pkg/submitter/browser.go` with a `resolveFillTarget(page)` resolver: checks the main page for form inputs first, falls back to scanning child frames for the first one that has any, and uses whichever is found for every fill/wait/DOM-extraction call downstream (`handleGreenhouse`, `handleLever`, `handleDynamic`, `safeFill`, the Learner Module's DOM capture, and the validation-error-retry path). Not yet confirmed to actually produce an `APPLIED` result — re-run and check `job_funnel` status before closing this out.

**Progress 2026-07-21 (live batch, still in progress):** one `failed to fill first_name` case observed so far (Breezy.hr, `jway-group`) turned out **not** to be this bug — `resolveFillTarget` correctly found no real form on the page or in any iframe and fell back to the page target as designed; the actual cause was a click-to-reveal "Apply" form, filed separately as bug #8. No SmartRecruiters-pattern (form genuinely embedded in an iframe already present in the DOM) case has been reached yet this run to positively verify the fix works end to end; also no counter-evidence that it doesn't. Still Pending verification.

**Structural blocker found 2026-07-23:** wrote a standalone probe (same `resolveFillTarget` logic reimplemented against a real headless browser) and ran it against 6 current `DISCOVERED` SmartRecruiters postings — the *only* platform this bug was ever reproduced on. Every single one now serves a DataDome CAPTCHA iframe (`geo.captcha-delivery.com`, 7 `<input>` fields) immediately after the "I'm interested"/"Apply" click, which bug #35's post-click `isCaptchaBlocked` check (confirmed correctly placed *before* `resolveFillTarget` runs, `pkg/submitter/browser.go:591`) now intercepts every time — so SmartRecruiters can no longer reach the fill stage at all, let alone this bug's iframe-fallback path. Also probed a broader live sample (Ashby, Pinpoint, Homerun, Jobvite) and found every one of those renders its real form directly on the main page — no iframe needed. **Conclusion: no platform in the current live traffic mix can organically exercise this bug's fix anymore**; waiting on a live batch to do it is likely to run indefinitely. Given that, added a direct unit test instead — `TestResolveFillTarget_FallsBackToIframeWithInputs` in `pkg/submitter/browser_test.go` reproduces the exact original repro shape (zero main-page inputs, one child frame with inputs) via mocks and confirms `resolveFillTarget` correctly returns a `frameTarget` pointed at that frame; two sibling tests confirm it prefers the main page when that already has inputs, and falls back to the page (not a frame) when nothing has inputs anywhere — covering the DataDome-iframe-with-inputs-but-not-a-form edge case specifically. `go build/vet/test ./...` all pass.

**Resolved 2026-07-23:** closed on the strength of the above rather than a live-traffic confirmation, per user go-ahead — live verification of this specific path is structurally unreachable (not merely unlucky), since the fix's own reasoning (frame-scan fallback) is directly, deterministically exercised by the unit test above using the exact original repro shape, and no code path between here and a real `APPLIED` remains unverified as a result of closing this one. If a genuine non-SmartRecruiters iframe-embedded-form case ever surfaces live, treat it as a bonus confirmation, not a requirement to reopen.

### 10. DOM-mapped fill failures never fell back to the Vision module, only outright mapping failures did (Resolved 2026-07-24)
**Observation:** across this session's diagnosis of bugs #4, #8, and #9, every single live failure had the same shape: the Learner Module's `ExtractFormMapping` call *succeeded* — it confidently returned a plausible-looking JSON mapping (e.g. `input[name='first_name']`) — but that mapping was simply wrong against the real page (iframe-embedded, click-gated, or a dead/redirected listing). `AttemptSubmit`'s existing fallback chain in `pkg/submitter/browser.go` only invoked `AttemptVisionSubmit` (screenshot -> vision-LLM reasoning -> selectors, `pkg/submitter/vision.go`) when `ExtractFormMapping` returned an outright error or empty string; a fill failure on an otherwise "successful" mapping just deleted the cache and returned an error, never trying the more robust visual path.

**Why this matters:** confirmed the Vision path is fully wired and usable in this environment — `OLLAMA_VISION_MODEL=qwen2.5vl:7b` is set in `.env` and the model is present in `ollama list` — so this wasn't a missing-capability gap, just an under-triggered one. A single structural change here has a chance to subsume an open-ended class of future DOM surprises (new ATS themes, new click-reveal patterns, new field-naming conventions) instead of requiring a dedicated bugfix per pattern as they're discovered one at a time, which is how #4, #8, and #9 all happened this session.

**Fix applied 2026-07-21:** in both places `AttemptSubmit` calls `handleDynamic` (the cached-mapping path and the fresh Learner-Module-mapping path), a fill failure now invalidates the cache and calls `AttemptVisionSubmit` before giving up, instead of returning the error immediately. `go build/vet/test ./...` all pass.

**Not yet verified live:** needs a fresh Learner Module fill failure in a live run to confirm the Vision fallback actually fires and improves the outcome, and to check it doesn't meaningfully slow down the common case (Vision calls are a second LLM round-trip on top of the mapping call, only paid when the mapping-based fill already failed, so worst case is not much worse than today's dead end — but not yet measured).

**Partial live verification 2026-07-21 23:01:** the trigger half is confirmed — on the GDIT Workday job (bug #18), the Learner-mapped fill failed and the log immediately shows `Invalidating cache. Falling back to Vision module` → `Taking a full-page screenshot` → `Transmitting screenshot`, i.e. the fill-failure→Vision path this fix added fired exactly as designed on a live job for the first time. The outcome half remains unverified: the batch was deliberately restarted (~23:05, to load an expanded roles list) while that Vision attempt was still in flight, and the underlying page was an auth-gated Workday form no fill strategy could have succeeded on anyway.

**Resolved 2026-07-24, via a direct end-to-end test:** three more days of live batch traffic (including the dedicated 82-job re-verification run) never produced a case where a genuine Learner Module fill failure landed on a page that could still be filled successfully — every live trigger observed happened to hit a structurally unfillable page (Workday's auth wall), the same "mechanism confirmed firing, outcome unreachable via live traffic" shape bug #4 hit. Closed the same way: `TestAttemptSubmit_VisionFallback_EndToEndSuccess` (`pkg/submitter/browser_test.go`) drives the real `AttemptSubmit` → `AttemptVisionSubmit` orchestration (not just its helpers in isolation) against a mock page where the Learner Module's mapping is deliberately wrong (fill genuinely fails, not just an outright mapping error), confirms `ExtractFormMappingVision` is actually invoked with a real screenshot payload, and confirms the resulting vision-remapped fill carries the submission all the way to `isSubmissionConfirmed` returning true. `go build/vet/test ./...` all pass.

### 11. FunnelEngine lets Jobvite `/search` listing pages into the pipeline
**Symptom:** live 2026-07-21, `jobs.jobvite.com/cloudone-digital/search` scored 80 and reached `AttemptSubmit`/the Learner Module, same as #5's Workday/Workable false positives — a listing/search page, not an individual posting, so the eventual `failed to fill first_name` was never fixable. Not yet root-caused with the same rigor as #5 (no direct DOM inspection done this session, just the URL shape and the eventual failure); needs its own pass to confirm the fix shape (likely a `/search` path rejection for `jobvite.com` similar to Workable's, added to `isValidATSUrl` in `pkg/scraper/funnel.go`).

**Fix 2026-07-21:** exactly the anticipated shape — `isValidATSUrl` now rejects `jobvite.com`/`*.jobvite.com` URLs whose path ends in `/search` or contains `/search/`, mirroring #5's Workable rule. Real Jobvite postings (`/job/<id>` paths) covered by regression cases in `TestIsValidATSUrl`. `go build/vet/test ./...` pass.


### 14. No accessible-label fallback for form-field filling, only CSS selector guessing (Resolved 2026-07-24)
**Symptom:** every single fill failure diagnosed this session (bugs #4, #8, #9, #10) had the same shape — the Learner Module's `ExtractFormMapping`/`ExtractFormMappingVision` confidently returned a plausible-looking CSS selector (e.g. `input[name='first_name']`) that simply didn't match the real page, and there was no second strategy to fall back on beyond re-guessing via Vision.

**Root cause / gap:** `handleDynamic` only ever calls `target.Loc(selector).Fill(...)` with the LLM's raw CSS-selector guess. Enterprise ATS platforms are generally WCAG-compliant (legal/accessibility requirements), meaning their form fields reliably carry a stable, human-readable accessible label — a `<label for="...">` association or an `aria-label`/`aria-labelledby` attribute — even when the underlying `name`/`id`/`class` attributes are auto-generated, obfuscated, or vary wildly by ATS vendor theme (exactly the pattern behind every failure this session). Checked current Playwright best-practice guidance via web search: "user-first locators" like `GetByLabel`/`GetByRole` are the explicitly recommended primary strategy for resilient automation against unknown markup, precisely because they're tied to what a human sees rather than implementation details — ahead of raw CSS selectors, which can also silently match the wrong element without ever raising an error.

**Fix applied 2026-07-21:** added `GetByLabelLoc(text string) playwright.Locator` to the `fillTarget` interface (implemented for both the page and iframe targets), added a `Labels map[string]string` field to `FormMapping` alongside the existing `Fields`, and updated both Learner Module prompts (`pkg/mcp/client.go`) to also return each field's visible accessible label text. Added `safeFillWithLabelFallback` in `pkg/submitter/browser.go`: tries the label first when one was identified, falls back to the CSS selector guess if the label attempt fails or no label was available. Wired into `handleDynamic`'s four PII fields (first_name, last_name, email, phone). Added 4 new tests covering label-first success, selector fallback on label failure, both failing, and no-label-available. `go build/vet/test ./...` all pass.

**Not yet verified live:** needs a fresh batch run to confirm the label fallback actually improves real-world fill success rate.

**Resolved 2026-07-24, via a direct end-to-end test:** live traffic through 2026-07-24 kept confirming the label tier engaging (e.g. bug #18's Workday cases) but never on a page where the underlying form was actually fillable — the auth wall meant no label, placeholder, or selector tier could ever have succeeded there regardless of this fix. Same "mechanism firing, outcome unreachable live" shape as bug #4. Closed via `TestAttemptSubmit_ClickToRevealPlusLabelFallback_EndToEndSuccess` (`pkg/submitter/browser_test.go`): drives real `AttemptSubmit` against a mock page where the Learner Module's mapping has deliberately wrong CSS selectors for every field but correct labels, and confirms the submission still reaches `isSubmissionConfirmed` — proof the label fallback is what carried the fill, not the (broken) selector. `go build/vet/test ./...` all pass.

### 18. Workday postings burn full Learner+Vision cycles against an auth-gated application flow with no fillable form
**Symptom:** live 2026-07-21, the GDIT Site Reliability Engineer posting (`gdit.wd5.myworkdayjobs.com/External_Career_Site/job/Any-Location--Remote/Site-Reliability-Engineer_RQ219922-1`) went through the entire pipeline: scored 80 (22:51:16), tailored documents generated and saved (~6 min, 22:57:22), Learner Module triggered, `clickApplyIfPresent` clicked an Apply-labeled element (22:57:22 — bug #8's fix firing live), `ExtractFormMapping` returned a mapping (23:00:27), then every fill tier failed — `failed to fill first_name: label fill for "First Name" failed: playwright: timeout: Timeout 15000ms exceeded` (23:01:12) — and the Vision fallback fired (bug #10's fix). Total: ~10 minutes of LLM + browser work on a form that was never fillable.

**Why it was never fillable:** Workday's application flow requires creating an account or signing in before the actual application form (with name/email/phone fields) is reachable. The pre-auth job page has a job description and an Apply button that leads to a sign-in/account-creation step — no First Name field exists in any frame, so the label, placeholder, and CSS-selector tiers (bugs #14/#16) all correctly time out, and Vision can only screenshot a login wall. Note this batch's Apply click landed on the fixed binary (built 22:46, includes #8/#10/#14/#16), so this is not any of those bugs recurring — it is a missing capability: nothing detects "this ATS requires an account" and short-circuits.

**Why it matters:** `*.myworkdayjobs.com` is one of the most common domains in the discovery funnel (GDIT, Cisco, U-Haul, Carrier, ABC Financial all seen this session), so this class silently dominates wasted cycle time the same way #5's `developer.workday.com` docs pages did before they were filtered.

**Second confirmed case 2026-07-21 23:15-23:16 (post-restart batch, all current fixes compiled in):** `redhat.wd5.myworkdayjobs.com/en-US` replayed the identical sequence — Learner Module "successfully mapped" (23:15:56), `failed to fill first_name: label fill for "First Name" failed: Timeout 15000ms` (23:16:41), Vision fallback triggered. Two independent Workday tenants (GDIT, Red Hat) with the same shape in one evening confirms this is the platform's auth-gate, not a per-tenant quirk. With Workday's share of the discovery funnel, this is now the leading candidate for the next fix.

**Suggested direction (not attempted):** detect the auth wall early — before document generation, or at latest before the Learner Module — via cheap signals (Workday domain + presence of "Sign In"/"Create Account" markers, or absence of any text input after the Apply click), then log the job to `applications/manual_submissions.md` with the tailored docs path and mark it `MANUAL_REQUIRED` in `job_funnel` instead of `FAILED_SUBMIT`. A full auto-account-creation flow is a much bigger (and riskier) feature and should be its own decision.

**Fix applied 2026-07-21 (unverified live):** two detection tiers in `pkg/submitter/browser.go`, both returning a new `ErrAuthWall` sentinel. (1) Known-host tier: `isKnownAuthGatedHost` suffix-matches `myworkdayjobs.com` (list: `authGatedATSHosts`), checked in `AttemptSubmit` right after document generation — docs are deliberately still generated, since they're the payload the manual application needs — and before the cached-mapping/Learner/Vision chain, cutting the wasted portion (~4+ min of mapping/fill/Vision) per Workday job. (2) Generic tier: in the Learner branch after `clickApplyIfPresent`, a password input present *plus* account-gate phrasing (`looksLikeAuthWallContent`: "sign in to apply", "create account", "returning candidate", etc.) short-circuits the same way, covering future auth-gated platforms beyond the known list. `cmd/agent/main.go` maps `errors.Is(err, submitter.ErrAuthWall)` to a new `MANUAL_REQUIRED` funnel status plus a `manual_submissions.md` entry (via existing `LogFailedSubmission`), distinct from `FAILED_SUBMIT`; the dashboard's `/api/metrics` now reports a `manual_required` count and `statusReason` explains the status (a dashboard UI tile is left for `improvements.md`). Tests: `TestIsKnownAuthGatedHost` (incl. `developer.workday.com` and a host-suffix-spoof URL staying false) and `TestLooksLikeAuthWallContent`. `go build/vet/test ./...` all pass in the `career-agent` container. Live batch restarted 23:26 with the fix compiled in — verification is the next Workday job in the queue short-circuiting to `MANUAL_REQUIRED` in seconds instead of ~10 minutes.

**Verified live 2026-07-21 23:34:36:** the next Workday job in the queue (`healthcatalyst.wd5.myworkdayjobs.com`) logged `is a known account-gated ATS — no pre-auth application form exists. Routing to manual submissions with tailored docs ready.` immediately after doc generation, and the worker recorded it as `queued for manual submission` (`MANUAL_REQUIRED`) — zero Learner/fill/Vision time spent, exactly as designed.

### 19. Workday URL parsing takes the locale/site segment as the company name
**Symptom:** Workday-hosted jobs enter the funnel with company names parsed from a URL path segment rather than the employer: `en-US` (U-Haul job in #12, and again live 2026-07-21 23:05-23:07 — `[Worker-1] Fetching job description for en-US...` / `Fit Score Pipeline: en-US scored 80!`), `External_Career_Site` (GDIT, this session), `ABCFinancialServices`, `apply`/`en` (the applytojob/pinpointhq cases noted in #17). Dashboard rows, log lines, and `job_funnel`/`applied_jobs` records all inherit the garbage name, which makes live debugging ambiguous (two different "en-US" jobs are indistinguishable at a glance) and the dashboard's company column meaningless for these rows.

**Likely shape:** wherever the scraper derives `CompanyName` from the URL, Workday URLs need the tenant subdomain (`gdit` from `gdit.wd5.myworkdayjobs.com`) rather than a path segment, and locale segments (`en-US`, `en`) should never be accepted as a name. Cosmetic, but cheap to fix and improves every future debugging session.

**Fix 2026-07-21:** added `companyFromURL` to `pkg/scraper/funnel.go`, used by the Yahoo-fallback discovery path (the only URL-based extraction — the SerpAPI path derives from the result title, RemoteOK from its API): subdomain-tenant platforms (Workday, ApplyToJob, Breezy, Recruitee, Pinpoint, BambooHR, Homerun) take the tenant host label; path-tenant platforms take the first path segment that isn't a locale (`en-US` regex) or generic section (`jobs`, `careers`, `apply`, ...); empty result falls back to "Unknown Company" instead of the old first-path-segment grab. Known trade-off documented in code: genuine two-letter company slugs get skipped as locale-like. `TestCompanyFromURL` covers every garbage name observed live (`en-US`, `External_Career_Site`, `apply`, `en`). Note: only affects newly discovered jobs — existing `job_funnel`/`applied_jobs` rows keep their old garbled names (a data backfill wasn't attempted; the URL remains the reliable key).

### 25. Fit scoring ignores geographic eligibility restrictions
**Symptom:** live 2026-07-22 02:05, `jobs.smartrecruiters.com/AristaNetworks/744000001578165-site-reliability-engineer-remote-from-romania-or-hungary` scored **80** and proceeded to a full application attempt. The role is explicitly restricted to workers located in Romania or Hungary; the candidate is US-based (Michigan). `profile.yaml`'s `remote_only` gate passed (it *is* remote) — nothing checks whether the candidate is in the required location. Every such job burns a full cycle, and a successful auto-submit would send an application the employer must discard.

**Fix applied 2026-07-22 (shipped as part of the #34-#38 batch, commit `b1709fd`):** added a 7th rule to the `ScoreJob` prompt in `pkg/mcp/client.go`: "If the job explicitly restricts remote candidates to a specific country or region, and my location does not match, deduct 80 points" — same-magnitude penalty as the existing `remote_only` mismatch rule, hard-failing the fit score for a role like the Arista Networks one above. No dedicated test exists (consistent with the rest of `ScoreJob`, which has no test coverage anywhere — it's a prompt-template function with no deterministic output to assert against); "verified locally" means the prompt text was read back and confirmed present, not a live re-score of the original flagged posting. **Groom-pass note (2026-07-23):** this Details section had drifted from the table's Resolved status — confirmed via `grep` that rule 7 is live in the current prompt; updating this section to match.

### 24. Prompt-injection quarantine may false-positive on ordinary job-page copy ("you are a...")
**Symptom:** live 2026-07-22 02:03:59, the Versant3 SmartRecruiters application was hard-blocked: `malicious prompt injection detected on career page: [{role_manipulation 0.4 potential role assignment via 'you are a' heuristic ...} {instruction_override 0.65 fuzzy match detected multiple injection-related keywords (possible typo evasion) ...}]`. "You are a ..." is near-universal job-ad copy ("you are a self-starter who..."), so the role-manipulation heuristic plausibly fires on a large fraction of legitimate postings, and combined-score blocking turns that into silently lost applications (status FAILED_SUBMIT, indistinguishable from real failures in the funnel).

**Fix applied 2026-07-22:** Implemented a secondary LLM check. If the quarantine layer flags a payload, we perform a lightweight inference call to verify if the context is legitimately describing a role (e.g. Prompt Engineer, AI Engineer) or if it is an actual malicious attack trying to hijack the agent. If the LLM classifies it as SAFE, we proceed instead of blocking the job application.

### 20. Email tracker classifies unrelated emails as INTERVIEW_REQUESTED and writes them to the DB
**Symptom:** during the first fully-logged-in `cmd/tracker` scan (2026-07-22 00:05, one cycle, ~2000s wall time — email analysis queues behind the live batch on the single Ollama slot), the tracker correctly detected a Glimpse rejection and a genuine Glimpse interview invitation, but also logged `Detected INTERVIEW_REQUESTED` + "Updating database" for clearly unrelated emails: four copies of a Google payment receipt ("google: we've received your payment for ..."), a LinkedIn "your application was sent to ClearlyAgile" confirmation (an application-*sent* notice, not an interview), and duplicate detections of the same recruiter thread (three identical updates).

**Why it matters:** whatever matching writes these statuses (company-name fuzzy match on "google"/"linkedin"?) is writing junk state transitions into real application history — the same DB the dashboard and conversion analytics (improvements #15) read. Needs: inspection of `pkg/tracker`'s classification prompt/heuristics and its DB-update matching logic; likely fixes are a stricter prompt with an explicit NOT_JOB_RELATED class, sender-domain allowlisting against known applied-to companies, and de-duplication by message ID. Not yet root-caused — filed from live log evidence only.

**Root cause, confirmed by reading `pkg/tracker/imap.go` — four distinct defects, one of them lucky:** (1) classification was pure keyword matching (any body containing "interview"/"next steps"/"availability" ⇒ INTERVIEW_REQUESTED) — the "AnalyzeEmail" metric in the logs is actually `ExtractRejectionReason`, only ever called on the rejection path, so no LLM was involved in classifying; (2) the DB update was `UPDATE ... WHERE company_name LIKE '%<label>%'` with the label being the sender domain's first token ("google", "linkedin"); (3) no de-duplication — the last 50 messages are refetched every 15-minute cycle, re-detecting the same threads; (4) **`cmd/tracker` never called `storage.InitDB`**, so `GetDB()` returned nil and every "Updating database" line in the tracker's entire history was a silent no-op — which is why defect (2) never actually corrupted anything, and also why the tracker has never done its job.

**Fix, verified live 2026-07-22 00:21:** `cmd/tracker` now initializes the DB; classification (`classifyEmail`, extracted pure for tests) short-circuits on not-job phrases (payment receipts, "your application was sent", "automated message"); a detected status only writes if the email matches a company we actually track — `storage.GetTrackedCompanies()` (APPLIED/INTERVIEW_REQUESTED/MANUAL_REQUIRED rows) matched by exact stored value against sender domain or subject, with sub-4-char and pre-#19 garbage labels ("en", "en-US") excluded and updates restricted to `company_name = ? AND status = 'APPLIED'`; processed Message-IDs persist in a new `processed_emails` table so each email is handled once ever. Live rerun of the same inbox: the Google receipts and LinkedIn confirmation produced nothing, the Glimpse emails were correctly held back as "matches no tracked application" (no funnel row exists for Glimpse — nothing to update). The rerun exposed one further hole, fixed in the same pass: tracked company "Remote" (remote.com) matched a recruiter subject merely containing the word "remote" — common-word company names (`commonWordCompanies`) now only match via the sender's domain, and the one bogus `Remote → INTERVIEW_REQUESTED` row the test run wrote was reverted to APPLIED by direct DB correction. Known accepted limitation: several emails in one thread arriving in the same first scan each get detected once (idempotent same-status updates); dedup silences them from the next cycle on. Tests: `TestClassifyEmail` (all live false positives as regression cases), `TestMatchTrackedCompany`, `TestEmailProcessedDedup`, `TestGetTrackedCompanies`.

### 15. Dedicated Greenhouse/Lever handlers timing out waiting for the form to render
**Symptom:** live 2026-07-21, two consecutive jobs — `jobs.lever.co/mistral/...` (`handleLever`) and `boards.eu.greenhouse.io/nebius/jobs/...` (`handleGreenhouse`) — both failed at the exact same error, `form failed to render in time: playwright: timeout: Timeout 30000ms exceeded.` (the shared error string at `pkg/submitter/browser.go:375` and `:431`, one per handler, each behind its own `WaitForSel(..., 30000)` call). Notable because these are the *dedicated*, hand-written handlers, not the Learner Module path this session has spent most of its time on — every confirmed-real `APPLIED` row checked earlier this session was Greenhouse/Lever, so this is the first live evidence that path can fail too, not just the generic fallback.

**Not yet root-caused (superseded below).** Both handlers already call `resolveFillTarget(page)` (bug #4's iframe-fallback resolver) before their `WaitForSel`, and that call happens several minutes after page load (after `generateDocsFunc`'s LLM call), so a simple "hasn't rendered yet" race seems unlikely per the same timing argument that ruled it out for #8/#9 — but this hasn't been directly verified for these two specific postings the way #4/#8/#9 were, via a standalone diagnostic script inspecting the actual live DOM. Needs that same treatment before assuming a fix.

**Root cause, confirmed via standalone diagnostic 2026-07-21 (~23:35): not a rendering/timeout problem at all — every case was a dead or moved posting the code had no way to detect.** A playwright-go script (same launch args/UA/stealth as the app) loaded each failing URL directly:
- `jobs.lever.co/mistral/f76907fd-...` (and its `/apply` variant): renders Lever's expired-posting shell — title `Not found – 404 error`, **zero** inputs/forms — served with HTTP 200, in phrasing `isDeadJobPage` didn't match.
- `boards.eu.greenhouse.io/nebius/jobs/4558243101`: **silently redirects to `careers.nebius.com/`** — the company migrated its board off Greenhouse; the handler then waited 30s for `input#first_name` on a generic careers landing page.
- `job-boards.greenhouse.io/remotecom/jobs/7778860003` (bug #7's URL): **redirects to `job-boards.greenhouse.io/remotecom?error=true`** — Greenhouse's expired-posting redirect, the same class bug #9 caught on Jobvite (`?error=404`).

So "form failed to render in time" was always the same story as #9 — jobs dying between discovery and the worker reaching them (queue latency is hours) — surfacing through a different, misleading error because nothing checked *where navigation actually landed*. The dedicated handlers themselves are likely fine; no genuinely-live posting failure has been observed on them.

**Fix applied 2026-07-21 (unverified live):** two additions in `pkg/submitter/browser.go`, both running before the costly doc generation: (1) `deadRedirectReason(applyURL, finalURL)` — flags an `error` query parameter on the post-navigation URL (Greenhouse `?error=true`, Jobvite `?error=404`) or a redirect to a different registrable domain (`registrableDomain` last-two-labels approximation; within-ATS board migrations like `boards.greenhouse.io` → `job-boards.greenhouse.io` are deliberately allowed through). (2) `"404 error"` added to `deadJobPhrases` for Lever's 404 shell. Tests: `TestDeadRedirectReason` (all three live-confirmed cases plus benign-redirect regressions), `TestRegistrableDomain`, new `TestIsDeadJobPage` case. `go build/vet/test ./...` pass. Live verification: the next expired posting hit by any handler path should log `job posting is dead or expired: redirected...` in seconds instead of a 30s form timeout after minutes of doc generation.

**Verified live 2026-07-22 00:11:** a jobgether job (Lever — the exact company whose postings produced the original 2026-07-20 timeouts) scored 80 at 00:11:22 and was rejected `job posting is dead or expired` at 00:11:25 — three seconds, before document generation, versus the old ~5-minute doc generation plus 30-second timeout path.

### 7. FunnelEngine still lets Greenhouse job-search/listing pages into the pipeline
**Remainder of #5** (Workday/Workable portion resolved 2026-07-21, see Resolved section below). The one Greenhouse false positive seen, `https://job-boards.greenhouse.io/remotecom/jobs/7778860003`, wasn't re-reproduced live during this session's ~1-hour batch run, and unlike Workday/Workable its path shape (`/jobs/<id>`) is normally exactly what a *real* Greenhouse posting URL looks like, so there's no obvious safe tightening rule (path-based or domain-based) without risking false negatives on legitimate postings. Needs a fresh live repro and DOM inspection of the specific false-positive case before a fix is attempted, same approach that diagnosed #4 and the rest of #5.

**Resolved 2026-07-21, via #15's diagnostic:** loading the exact URL directly showed it was never a listing-page false positive — it's a real posting URL whose job expired, and Greenhouse's expired-posting redirect (`→ job-boards.greenhouse.io/remotecom?error=true`) landed it on the tenant's board index, which *looked* like a listing page in the original diagnosis. That's exactly why its `/jobs/<id>` shape resisted a URL-filter fix: the URL was legitimate. #15's `deadRedirectReason` now catches the `?error=` redirect at submit time and bails in seconds. No evidence of a genuine Greenhouse listing-page discovery false positive remains; if one ever appears, file it fresh.

### 8. Dynamic/Learner Module fill path never clicks an "Apply" button to reveal click-to-reveal application forms (Resolved 2026-07-24)
**Symptom:** live 2026-07-21, `AttemptSubmit` for a Breezy.hr posting (`jway-group.breezy.hr/p/419b44576d64-backend-developer-laravel-aws`) reached the Learner Module, successfully "mapped" the page, then failed with `failed to fill first_name: playwright: timeout: Timeout 15000ms exceeded` on the re-attempt.

**Root cause, confirmed by direct inspection:** fetched the raw page (`curl`, same URL) and found **zero `<form>` tags anywhere**, exactly **one `<input>`** on the page (a `readonly` referral-link text box, unrelated to any application form), and **one `<iframe>`** (a Google Maps embed for the office location, also unrelated). The page loads `jquery.fancybox` and other circa-2017 Breezy portal assets and has visible "Apply" button text — this ATS's real application form is a fancybox/lightbox modal that only renders into the DOM after a user clicks "Apply," not something present on page load or embedded in an already-present iframe. `resolveFillTarget` (bug #4's fix) correctly detected this: it found no real form on the main page or in any frame and fell back to the page target, exactly as designed for this case — so this is not a #4 regression. The actual bug is one level up: `handleDynamic` in `pkg/submitter/browser.go:396` goes straight from the Learner Module's field mapping to `safeFill`/`WaitForSel` calls with no step that clicks an "Apply"/"Apply for this Job" trigger first, so it's waiting for fields that will never appear without that click. The Learner Module's `ExtractFormMapping` DOM capture likely has the same blind spot — it should be inspecting the DOM *after* the apply-click, not before.

**Suggested fix:** before invoking the Learner Module / `handleDynamic` fill path, look for a common "Apply"-labeled clickable element (button/link with text matching `apply` case-insensitively, or common ATS-specific selectors) and click it first if present, then re-resolve the fill target and re-capture the DOM for `ExtractFormMapping` if using the Learner Module. Needs design thought on how to detect "form already present" (skip the click) vs. "form behind a click" (do the click) without false-triggering on unrelated "Apply Now" marketing links elsewhere on the page. Not attempted this session — found while investigating #4's live verification, filed for its own dedicated session.

**Second confirmed case 2026-07-21 (same session, SmartRecruiters this time):** `jobs.smartrecruiters.com/sosi1/3743990013881284-cloud-web-developer` — the exact ATS platform bug #4 was originally diagnosed and fixed against — hit the identical failure shape: Learner Module mapped successfully, then `failed to fill first_name` after the full 15s timeout. Direct inspection (`curl`) again found zero `<form>` tags, zero `<iframe>` tags, and one unrelated stray `<input>` (a "copy link to share via WeChat" text box), plus script tags hinting at a client-rendered framework. Critically, `resolveFillTarget`'s check runs several *minutes* after page navigation (after the 5-10 minute `generateDocs()` LLM call completes, see call order in `pkg/submitter/browser.go` around line 140-178) — long enough that a merely slow-rendering SPA should have finished by then. That timing rules out "raced a slow render" as the explanation and makes "form is genuinely gated behind a click, and nothing in the code ever clicks it" the best-supported explanation for both cases. Net effect: in every live case observed this session (2 ATS platforms, 4 total attempts: Breezy x3, SmartRecruiters x1), the failure was this bug, not bug #4's iframe scenario — bug #4's fix has still not been positively exercised (no case has hit the `using embedded iframe` log line), so it remains unverified, though nothing here contradicts it either. This bug (#8) is now the best next candidate for unblocking the Usability Gate's live-batch checkbox, likely more impactful than further #4 investigation.

**Fix applied 2026-07-21:** added `clickApplyIfPresent(page)` in `pkg/submitter/browser.go`, called once in `AttemptSubmit`'s Learner Module branch (`else if mapper != nil`) right before the DOM is captured for `ExtractFormMapping` and before `resolveFillTarget` — so both the Learner Module's mapping and the later fill see the post-click DOM. Looks for `button:has-text('Apply'), a:has-text('Apply')`, clicks the first match if any exist, no-ops otherwise. Scoped only to the Learner Module path (not the dedicated Greenhouse/Lever/LinkedIn handlers, which weren't implicated, and not the validation-error retry branch). Added `TestClickApplyIfPresent_NoApplyButton` and `TestClickApplyIfPresent_ClicksWhenFound` in `pkg/submitter/browser_test.go`. `go build/vet/test ./...` all pass.

**Delegation note:** the first attempt (agy/Gemini 3.1 Pro) hit an account-wide quota error before writing anything (git status stayed clean, correctly caught before assuming success). The retry (agy/GPT-OSS 120B) returned exit 0 and a plausible-sounding diff, but the actual diff (verified via `git diff`, per this repo's own standing rule to never trust a delegate's self-report) had duplicated `resolveFillTarget`'s body with a stray extra `}` — would not have compiled. Reverted and applied the fix directly instead, including working around a genuine Go gotcha along the way: `playwright.Locator`'s own `Locator(...)` chaining method collides with the field name Go gives an anonymously-embedded `playwright.Locator` in a mock struct; fixed by embedding via a local type alias (`type pwLocator = playwright.Locator`) instead, which lets the embedded field take a different name while remaining the identical type.

**Not yet verified live:** the currently-running live batch (started before this fix was written) is a separately compiled `go run` process and does not have this fix — a fresh run is needed to confirm it actually resolves the Breezy/SmartRecruiters cases without breaking ATS platforms that don't need a click.

**Relaunched with the fix 2026-07-21, still unresolved by first look:** killed and relaunched the batch with this fix compiled in. The next Learner-Module-routed job (Jobvite, `jobs.jobvite.com/dwt/job/o79Qzfwp/apply`) still failed with the identical `failed to fill first_name` symptom, `clickApplyIfPresent` did not log a click (confirmed via `grep` — no "Clicked an Apply-labeled element" line). Wrote a standalone headless-Playwright diagnostic script (same browser launch args and `mxschmitt/playwright-go` version as the app) to inspect this specific URL directly. **Turned out to be a different bug entirely, not this one:** the job had expired and redirected to `jobs.jobvite.com/careers/dwt/jobs?error=404`, whose rendered text ("the job listing no longer...") didn't match the app's existing (too narrow) dead-job detection, so it wasn't caught before reaching the Learner Module. Filed and fixed separately as bug #9. This means #8's actual click-to-reveal fix still has zero live evidence either confirming or refuting it beyond the two original cases that motivated it (both observed *before* the fix existed) — genuinely re-verifying #8 needs a fresh Breezy or SmartRecruiters case to recur post-fix, which hasn't happened yet this session.

**Later confirmed firing live 2026-07-22** (GDIT Workday job, bug #18's Details): `clickApplyIfPresent clicked an Apply-labeled element` logged correctly post-fix — the click mechanism itself works. **Resolved 2026-07-24, via a direct end-to-end test:** no live case since has recurred where a click-to-reveal page's form was actually fillable afterward (Workday's cases were auth-gated regardless of the click). Closed via `TestAttemptSubmit_ClickToRevealPlusLabelFallback_EndToEndSuccess` (`pkg/submitter/browser_test.go`), the same test that closes #14: a mock page with zero form fields until an "Apply" element is clicked (the exact original Breezy/SmartRecruiters repro shape), confirming `AttemptSubmit` clicks it, the form fields become visible to `resolveFillTarget`, and the fill+submit that follows reaches a confirmed success. `go build/vet/test ./...` all pass.

## ✅ Resolved

### 23. Bot-protection interstitials (DataDome) aren't detected, burning full cycles and feeding the Learner captcha pages (Resolved 2026-07-22)
**Symptom:** live 2026-07-22 01:37-01:43, a genuine AbbVie posting (`jobs.smartrecruiters.com/oneclick-ui/company/AbbVie/publication/...`) went through the full pipeline — docs generated, Learner Module "successfully mapped" the page, all three fill tiers timed out on `first_name`, Vision fallback launched — exactly the shape of the fill bugs (#8/#14/#16), which is what made it valuable to diagnose properly instead of assuming.

**Root cause, confirmed by standalone diagnostic + screenshot:** the page served to the agent is a DataDome bot-protection interstitial — "Access is temporarily restricted ... Automated (bot) activity on your network" — a 12-element DOM with zero forms and one challenge iframe from `geo.captcha-delivery.com`. The Learner mapped a captcha page; no fill strategy could ever work. The only captcha detection in the codebase was Cloudflare/reCAPTCHA phrase matching in the scraper's description-fetch path (`cmd/agent/main.go`) — nothing guarded `AttemptSubmit`, and DataDome wasn't recognized anywhere.

**Fix:** `AttemptSubmit` now runs `isCaptchaBlocked` right after navigation, before document generation: content phrases ("access is temporarily restricted", "verify you are human", ...) plus a frame scan for challenge hosts (`captcha-delivery.com`, `hcaptcha.com`, `recaptcha`, `challenges.cloudflare.com`, `arkoselabs.com`). Detection returns `ErrCaptchaBlocked`; the worker maps it to the pre-existing `BLOCKED_CAPTCHA` status (already rendered by the dashboard's `statusReason`). Cost per bot-walled job drops from 10-40 minutes to seconds. `TestIsCaptchaContent` covers the live DataDome copy plus Cloudflare/human-check variants.

**Operational note, not fixable in code:** the block message cites the machine's network IP — SmartRecruiters has rate-flagged this network for automated activity, so SmartRecruiters jobs will keep hitting `BLOCKED_CAPTCHA` until the flag ages out (typically hours to days) regardless of this fix. Actually *solving* challenges is `improvements_paywall.md` #17 (needs a paid 2captcha/capsolver key, explicitly user-gated).

### 21. SaveFormMapping caches non-JSON LLM output, poisoning every future visit to the domain (Resolved 2026-07-22)
Observed live 2026-07-22 00:31: `AttemptSubmit` loaded the cached mapping for `www.workday.com/en-us` and immediately failed `failed to parse mapping json: invalid character 'T' looking for beginning of value` — an earlier Learner Module response was prose, got cached verbatim, and every subsequent visit paid a guaranteed parse failure plus the Vision fallback (34 minutes on CPU Ollama this time). Fix: `SaveFormMapping` rejects input failing `json.Valid` with an explicit error, so a bad LLM response costs one failed save instead of a poisoned cache. `TestSaveFormMappingRejectsNonJSON` covers accept/reject/not-cached.

### 22. Stale pre-filter backlog rows and error-redirect URLs bypass every discovery filter (Resolved 2026-07-22, mitigated)
Two related leak paths observed live 2026-07-22: (1) the 2,001-row DISCOVERED backlog predates every FunnelEngine filter added since #5, so long-dead junk (`www.workday.com/en-us/company/careers/hiring-programs.html` — a marketing page, not a posting) still reaches workers and burns full cycles — filters only gate *new* discoveries; (2) Yahoo discovery indexed Greenhouse's own expired-posting redirect (`job-boards.greenhouse.io/remotecom?error=true`) as a "live job". Fixes: `isValidATSUrl` rejects any URL carrying an `error` query parameter (regression cases added to `TestIsValidATSUrl`), and a one-time DB pass flipped 62 known-invalid DISCOVERED rows (32 `www.workday.com`/`developer.workday.com`, 14 `error=` URLs, 16 `/search`/`?workplaceType=` listing pages) to a dedicated `INVALID_URL` status — reversible, distinct from fit-based SKIPPED; agent restarted to shed the already-queued copies (queue: 2001 → 1939). *Mitigated, not eliminated:* other stale-junk shapes may still lurk in the remaining backlog; the submit-time dead-page/redirect checks (#15) remain the backstop.

**Hardened same night (01:17):** ten minutes after the first purge, `digital.workday.com/en-us` — a third Workday corporate subdomain the LIKE patterns didn't cover — reached the Learner Module from the stale backlog. Purge-by-pattern is whack-a-mole, so the junk rules are now code, applied at two layers: exported `scraper.IsKnownJunkJobURL` (any non-`myworkdayjobs` `workday.com` host, `error` query params, `/search` listing paths, `?workplaceType=` board filters — deliberately a blacklist so legitimate non-ATS company-site URLs from RemoteOK still pass) backs both `isValidATSUrl` at discovery and a new worker-intake guard in `cmd/agent` that flips matching queue rows to `INVALID_URL` before any scoring or tailoring. A second DB pass flipped 36 more `workday.com` corporate rows. `TestIsKnownJunkJobURL` covers all live-seen shapes plus must-pass regressions.

**Hardened again (01:33): company board-index pages.** `careers.smartrecruiters.com/aristanetworks` — "Careers at Arista Networks", the tenant's whole job board, confirmed by direct fetch — scored 80 and burned docs + Learner + a Vision fallback. Added the general rule to `IsKnownJunkJobURL`: on path-tenant ATS hosts (SmartRecruiters, Lever, Greenhouse, Ashby, Workable, Jobvite), a path with ≤1 segment is the company's board index, never a posting (real postings always carry `/company/<id-or-slug>` or deeper). The matching DB pass was the biggest cleanup of the night: **181 queued board-index rows** flipped to `INVALID_URL` (~9% of the backlog, each worth a 10-40 minute wasted cycle). Queue after restart: 1826.

### 16. #14's label fallback needs a GetByPlaceholder tier too (Resolved 2026-07-21)
**Symptom:** live 2026-07-21, three separate cases (`techinsights.applytojob.com/apply`, `jobs.jobvite.com/ninjaone`, `brightvisiontechnologies.applytojob.com/apply`) all logged `label fill for "First Name" failed (playwright: timeout: Timeout 15000ms exceeded.` — the label text identified was correct ("First Name", not a garbage value like the earlier phone-number mixup), yet `GetByLabel` still found nothing.

**Explanation:** modern minimalist ATS widgets commonly style an input's `placeholder` attribute to look exactly like a label visually (e.g. "First Name" greyed-out text inside the empty box) without an actual `<label for="...">` or `aria-label` association — visually indistinguishable from a real label to a human or a screenshot-reading vision model, but semantically invisible to Playwright's accessibility-tree-based `GetByLabel`.

**Fix:** added `GetByPlaceholderLoc` to the `fillTarget` interface (page + iframe targets) and a third tier to `safeFillWithLabelFallback`: label → placeholder → CSS-selector-guess, only advancing to the next tier if the previous one failed or wasn't available. Added tests for the new tier and for all three tiers failing together. `go build/vet/test ./...` all pass.

**First post-fix live case 2026-07-21 23:01 — not counter-evidence:** the GDIT Workday job failed `first_name` on a binary that includes this fix (built 22:46, fix committed 22:43), but that page is an auth-gated Workday flow with no name field present pre-login in any form (bug #18) — all three tiers failing there is correct behavior, not a placeholder-tier miss. A genuine verification still needs a placeholder-styled form (applytojob.com/Jobvite pattern) to recur post-fix.

### 17. ORDER BY last_updated DESC picked a stale row over a genuinely newer one (Resolved 2026-07-21)
**Symptom:** the user caught this directly from the dashboard screenshot: "Working on `apply` — Site Reliability Engineer since 1:48:26 AM" while the real time was ~9:59 PM — looked like an 8+ hour stall on a job that was actually only ~10 minutes old.

**First (wrong) diagnosis:** assumed the `career-agent` podman container had no timezone configured and `job_funnel.last_updated` was simply being displayed in UTC without conversion — filed as a "cosmetic" Minor issue. Checking `date` inside the container showed correct EDT already, which didn't fit that theory, so it was checked further instead of left as filed.

**Real root cause, confirmed via direct DB inspection:** two different `applytojob.com` postings (both company-labeled `"apply"` due to the same URL-parsing mislabeling as the pinpointhq `"en"` case) were both `status='PROCESSING'`: one genuinely current (`last_updated="2026-07-21T21:50:47.247518127-04:00"`, written by this session's `UpdateFunnelStatus` fix using a local-offset `time.Now()`), one stuck from ~20 minutes earlier during a brief window when an intermediate build wrote the same column via SQLite's `CURRENT_TIMESTAMP` (always UTC) — `last_updated="2026-07-22T01:48:26Z"`, already rolled over to the next calendar day in UTC terms. `ORDER BY last_updated DESC` in SQLite is a plain **TEXT** comparison, not a real chronological one: comparing `"2026-07-22..."` against `"2026-07-21..."` as strings, `'2' > '1'` makes the OLD stuck row sort first, even though it happened chronologically earlier once both are correctly interpreted with their offsets.

**Fix:** `UpdateFunnelStatus`/`UpdateFunnelStatusWithScore` now bind `time.Now().UTC()` explicitly (canonical, always-comparable `...Z` format) instead of a local-offset `time.Time`, guaranteeing every future write to this column is directly text-comparable regardless of local wall-clock rollovers. The dashboard now calls `.Local()` before formatting any of the three affected timestamp fields (plus `last_applied_at`, for consistency) for display, so storage stays sortable while the viewer still sees their own local time. Added a regression test confirming the canonical-UTC write format, and a dashboard test confirming UTC-stored values are correctly converted to local time in the API response. `go build/vet/test ./...` all pass.

### 12. Same job URL reprocessed repeatedly, hitting a UNIQUE constraint on applied_jobs.url (Resolved 2026-07-21)
**Symptom:** live 2026-07-21, multiple different jobs (`developer.workday.com/rest-api-explorer`, `carrier.wd5.myworkdayjobs.com` "jobs", `abcfinancial.wd5.myworkdayjobs.com` "ABCFinancialServices", `uhaul.wd1.myworkdayjobs.com` "en-US", and more) each got reprocessed 3-5+ times over the course of the session, each full attempt taking 20-30+ minutes (score → tailor → Learner Module/Vision → fill), before eventually failing with `failed to generate application documents: UNIQUE constraint failed: applied_jobs.url`. Confirmed via `ProcessJobApplication API Call #N` counters: three near-simultaneous log lines for the same company showed three *different* call numbers, and independent LLM scoring calls for the same URL produced different scores (80, 85, 80) moments apart — proof these were genuinely separate, independent pipeline runs for the same job, not one run's sequential log lines. This was very likely the dominant reason `applied` never moved during the session's ~7 hour live batch despite continuous activity — misdiagnosed for a while as a multi-worker concurrency bug (ruled out: only `Worker-1` was ever active, confirmed via a properly date-anchored log check after an earlier unanchored `awk` filter gave a false positive).

**Root cause, confirmed by reading `pkg/storage/manager.go` and `pkg/scraper/funnel.go`:** `AddToFunnel` (called by all three FunnelEngine discovery paths, always with `status="DISCOVERED"`) used `INSERT ... ON CONFLICT(url) DO UPDATE SET status=excluded.status`. Since the same URL routinely gets rediscovered across separate search passes (observed literally all session — the same URLs "discovered" repeatedly hours apart), every rediscovery silently reset that job's `job_funnel.status` back to `DISCOVERED`, even while a worker was actively processing it or had already finished it — making it eligible to be picked up again by `GetDiscoveredJobs()`/pushed again to `jobChan`. Compounding this, all three FunnelEngine call sites pushed the job onto `jobChan` unconditionally on any successful `AddToFunnel` call (insert *or* update), so a rediscovery queued a fresh duplicate work item regardless of the status field at all. The existing "Duplicate check" (`storage.HasApplied`) only ever caught the case where a *prior* attempt had already fully succeeded — it did nothing for a job that was currently in-flight or had merely failed previously, so a failing job could be retried indefinitely, each time from scratch, for as long as it kept getting rediscovered.

**Fix:** `AddToFunnel` now uses `ON CONFLICT(url) DO NOTHING` and returns whether a row was genuinely newly inserted (via `RowsAffected()`); all three FunnelEngine call sites (`DiscoverJobs`, `discoverWithYahooHTML`, `discoverWithRemoteOK`) only push to `jobChan` when it reports a new insert. Added a regression test in `pkg/storage/manager_test.go` covering that re-adding an already-known URL neither reports as new nor resets its already-advanced status. `go build/vet/test ./...` all pass.

**Not yet verified live:** needs a fresh batch run to confirm `applied` actually starts moving now that jobs aren't burning repeated 20-30 minute cycles on the same handful of URLs.

**One-time data correction 2026-07-21 (post-relaunch):** confirmed a handful of post-fix `UNIQUE constraint` echoes were harmless one-time artifacts of pre-existing corruption, not a repeat of the bug: exactly 23 `job_funnel` rows had `status='DISCOVERED'` while *also* already having a matching `applied_jobs` row — a combination only possible via this exact bug (a job that had already fully succeeded, whose status was later reset to `DISCOVERED` by an earlier rediscovery, before the fix landed). Corrected those 23 rows' status to `APPLIED` directly via a one-off script (dashboard `applied` count: 35 → 58). Deliberately did **not** touch the much larger set of `job_funnel` rows that have a matching `applied_jobs` entry but a different status (`PROCESSING`: 135, `FAILED_SUBMIT`: 90, `BLOCKED_CAPTCHA`: 52, `FAILED_SCORE`: 3) — `applied_jobs` only records that a tailored resume/cover letter was generated and saved (`SaveApplication`, called early in `AttemptSubmit`), not that the actual browser form submission succeeded, so those other statuses may already correctly reflect a real submission failure (matching bugs #4/#8/#9/#10's exact failure shape: doc generation succeeds, the fill/submit step after it doesn't) and reclassifying them without individual verification would overstate real progress. The honest total is 58 fully-successful applications recorded across this project's history, not the 430 raw `applied_jobs` rows.

**Correction to the correction, 2026-07-21 (~21:30):** the assumption above — "`status='DISCOVERED'` + a matching `applied_jobs` row can only mean a job that had already fully succeeded" — was wrong. It missed a second, equally possible path to that same state: docs generated (→ `applied_jobs` row written) → the actual browser submission then failed → status should have gone to `FAILED_SUBMIT`, but instead got reset to `DISCOVERED` by a later rediscovery (this bug, active until the earlier fix landed). Caught while building the dashboard's "last applied" feature: it surfaced `jobs.jobvite.com/cloudone-digital/search` — the exact FunnelEngine `/search`-listing false positive identified hours earlier (bug #11) — as a "successful application," which is structurally impossible (a search page has no form to submit). Cross-checked all 58 `status='APPLIED'` rows against `execution_logs` (an append-only audit trail populated by `pipeline.SaveCheckpoint`/`storage.LogExecution`, distinct from the unused `execution_state` table) for each URL's most recent logged status: **12 of the 23 corrected rows had a last-known status of `FAILED`, not `COMPLETED`.** Reverted those 12 to `FAILED_SUBMIT` (their true last-known state) via a second one-off script. Corrected honest total: **46**, not 58. This entire episode is itself worth internalizing: a "safe-looking" data correction based on a single necessary-but-not-sufficient condition still produced real errors — worth a second independent signal (here, `execution_logs`) before trusting a bulk correction, not just after.

**Post-fix verification was contaminated by an orphaned-process bug, not this bug:** continued apparent duplicate processing after this fix landed (e.g. the same job "skinspirit" logging 4 near-simultaneous `Initiating submission sequence` lines) was traced to five separate orphaned `go run` child processes left running from the session's own earlier `kill -9`-on-wrapper-only relaunches, not a gap in this fix — see the operational note above the Usability Gate. Once all orphans were killed and the agent relaunched as a single directly-run binary, this needs one more clean observation window to fully confirm.

### 13. Ollama gets kernel OOM-killed under this machine's real RAM ceiling (Resolved 2026-07-21, mitigated not eliminated)
**Symptom:** two `context deadline exceeded`/`EOF`/`connection refused` incidents this session (~14:02, ~15:57), each briefly breaking every in-flight LLM call across the batch until Ollama's systemd unit auto-restarted.

**Root cause, confirmed via `journalctl -k`:** genuine kernel OOM-killer events both times — `Out of memory: Killed process ... (llama-server) ... anon-rss:13-15GB` — not a soft config limit or app-level bug. `free -h` showed this machine has 29GB total RAM with only ~5.7GB "available" at rest. The second kill landed immediately after bug #10's Vision fallback first triggered live, which loads a second model (`qwen2.5vl:7b`) concurrently with the already-loaded 30B text model (`qwen3:30b-instruct`, ~19GB alone) — tonight's own fix increased peak memory pressure.

**Mitigation applied:** added `Environment=OLLAMA_MAX_LOADED_MODELS=1` to `~/.config/systemd/user/ollama.service.d/override.conf` (alongside the existing `OLLAMA_CONTEXT_LENGTH=6144` from bug #3's fix) and restarted the service. This makes Ollama evict one model before loading the other instead of holding both simultaneously, trading a ~1-2 second model-swap delay (per `journalctl` timings, negligible next to the 5-30 minute generation calls) for not exceeding available RAM when text and vision calls interleave.

**Not eliminated:** this reduces peak memory (one model instead of two) but doesn't guarantee headroom against everything else competing for the same 29GB (desktop environment, browser automation, other apps) — a third OOM kill under sufficient combined pressure is still possible. A real fix would need either more RAM, a smaller model, or a dedicated headless environment for long batch runs.

### 9. Dead-job-posting detection missed common phrasings, wasting cycles on expired listings (Resolved 2026-07-21)
**Symptom:** live 2026-07-21 while re-verifying #8's fix, a Jobvite posting (`jobs.jobvite.com/dwt/job/o79Qzfwp/apply`) failed with the same `failed to fill first_name` symptom as bugs #4/#8, but `clickApplyIfPresent` (bug #8's just-applied fix) never logged a click attempt.

**Root cause, confirmed via a standalone diagnostic script:** wrote a small headless-Playwright program (`mxschmitt/playwright-go`, same version and browser launch args as the app) to load the URL directly, wait past network-idle plus an extra settle period, and dump input/form/iframe counts, any "Apply"-text elements, and a screenshot. Found the page had actually redirected to `jobs.jobvite.com/careers/dwt/jobs?error=404` — the job had simply expired between discovery and `AttemptSubmit` (jobs can sit in the funnel queue for hours before a worker reaches them) — with page text reading "the job listing no longer [exists]". `AttemptSubmit`'s existing dead-job guard in `pkg/submitter/browser.go` only checked for the literal substring `"job is no longer available"` (plus two other exact phrases), which this ATS's wording didn't match, so the dead page sailed through the check and wasted a full generation + Learner Module cycle before failing with a misleading fill-timeout error that looked exactly like bugs #4/#8.

**Fix:** extracted the inline check into `isDeadJobPage(content string) bool` and widened the phrase list to also cover `"no longer exists"`, `"no longer accepting applications"`, `"job listing no longer"`, `"posting is no longer active"`, and `"job has been filled"`. Added `TestIsDeadJobPage` (pure string-matching test, no browser needed). `go build/vet/test ./...` all pass.

### 5. FunnelEngine let Workday-docs and Workable-search pages into the pipeline (Resolved 2026-07-21)
**Symptom:** found 2026-07-20 while diagnosing #4. Confirmed cases: `https://jobs.workable.com/search/global/remote-software-engineer-jobs` (a search-results page, not a posting) and `developer.workday.com/welcome`, `/documentation`, `/api-overview`, `/rest-api-explorer` (Workday's own developer-docs site, not a `myworkdayjobs.com` job posting) were all discovered and scored as candidate jobs, then went through a full score → tailor → `AttemptSubmit` cycle before failing to fill a nonexistent `first_name` field. Re-confirmed live during this session's 2026-07-21 batch run: **5 consecutive `developer.workday.com` doc pages in a row** (`welcome`, `api-overview`, `rest-api-explored`, `documentation`, `welcome` again) each burned a full ~5-10 minute local-LLM generation cycle plus a Learner Module DOM-mapping attempt before failing — direct evidence this bug could dominate a run's compute budget, not just an occasional nuisance.

**Root cause:** `isValidATSUrl` in `pkg/scraper/funnel.go` (used by the Yahoo-fallback discovery path) had `"workday.com"` as a bare entry in its `atsDomains` list, matched via a suffix check (`host == domain || strings.HasSuffix(host, "."+domain)`) — this matches *every* subdomain of `workday.com`, including `developer.workday.com`, not just the actual job-posting subdomain pattern `*.myworkdayjobs.com` (which was already separately present in the list). Similarly `"workable.com"` matched its own `/search/` listing-page path with no path-level check.

**Fix:** removed the bare `"workday.com"` entry from `atsDomains` (keeping `"myworkdayjobs.com"`), and added a path check that rejects any `workable.com`/`*.workable.com` URL whose path contains `/search/`. Added `TestIsValidATSUrl` in `pkg/scraper/funnel_test.go` covering both fixes plus regression cases for real Workday/Workable/Lever postings. `go build/vet/test ./...` all pass. Delegated to Gemini 3.1 Pro via `agy`, diff verified against `git diff` before commit.

**Not done (tracked separately as #7):** the Greenhouse false positive from the same original finding (`job-boards.greenhouse.io/remotecom/jobs/7778860003`) — its URL shape looks like a real posting, so no safe fix was obvious without a fresh live repro.

### 6. Ollama generation throughput collapses mid-request, likely context-shift thrashing (Resolved 2026-07-21)
**Symptom:** across two consecutive 40-minute live runs late on 2026-07-20 (after #3 and #4's fixes, and after a clean `systemctl --user restart ollama`), almost every real `AttemptSubmit` attempt died at the document-generation stage with the same `context deadline exceeded` error #3 was supposed to have fixed — but this time each attempt ran a genuinely long time (33 minutes to fail) rather than hanging outright. A direct `journalctl` check mid-request showed `tg` (generation speed) at **1.58 tokens/sec**, `n_decoded = 944` — down from **8.8 tokens/sec** on a tiny throwaway prompt with no context pressure.

**Leading hypothesis going in (disproven):** that `--context-shift` (confirmed present in the live `llama-server` args: `-c 6144 ... --context-shift --keep 4`) was triggering expensive KV-cache recompute/eviction events as generation approached the 6144-token window.

**Actual root cause, confirmed by direct reproduction 2026-07-21:** sent a real ~4042-token prompt to the live `qwen3:30b-instruct` server directly (bypassing the app's client timeout) and watched `journalctl --user -u ollama -f` for the full run. Generation speed was **already** down at ~1.77 tok/s by `n_decoded = 100` (not "fast then collapsing" — just uniformly slow once a ~4000-token prompt is loaded) and declined smoothly to ~1.58-1.62 tok/s by `n_decoded ≈ 1200`, with **zero** `context-shift`/cache-eviction log lines anywhere in the run — the request never got close enough to the 6144 ceiling (peaked at ~5266 total tokens) to trigger a shift at all, yet was already this slow. This is simply attention-decode cost scaling with total context length on CPU-only inference — expected llama.cpp/CPU behavior, not a defect or a discrete "thrashing" event. The request completed cleanly with `HTTP 200` after **15m58s** for 1224 output tokens. Extrapolating that rate to a full `ProcessJobApplication` response (resume + cover letter + interview prep, likely 1500-2500 combined tokens) lands at **~25-35 minutes** — which matches the original incident's "33 minutes to fail" almost exactly. The real bug was `pkg/mcp/provider_ollama.go`'s `ollamaProvider.Timeout()` hardcoded to **10 minutes**: Go's `context.WithTimeout` was cancelling honest, still-progressing generations long before they could finish, and that cancellation surfaces as the same `context deadline exceeded` string bug #3 also produced (from a genuinely different cause — server-side hang on context overflow, already fixed), which is what made this look like a recurrence of #3 rather than a new, distinct problem.

**Ruled out:** thermal throttling (CPU never exceeded 69°C) and a Playwright/Chromium process leak (confirmed to be one browser's normal multi-process architecture).

**Fix:** made the Ollama client timeout configurable (`pkg/mcp/provider_ollama.go`): added `ollamaTimeoutFromEnv()` reading `OLLAMA_TIMEOUT_MINUTES` (falls back to a default on unset/non-numeric/non-positive values), defaulting to **45 minutes** (`defaultOllamaTimeoutMinutes`) — comfortable headroom over the ~25-35 minute measured/extrapolated real generation time. Documented the new var in `.env.example`. Added `pkg/mcp/provider_ollama_test.go` covering the default, a valid override, and invalid-value fallback (non-numeric, zero, negative). `go build/vet/test ./...` all pass.

**Not done (separate, larger scope):** splitting `ProcessJobApplication`'s single call into three smaller generations (resume/cover letter/interview prep) would reduce per-call latency and risk further, but re-sends the full prompt context three times, increasing total wall-clock on this CPU-bound hardware — a real trade-off, not a strict improvement, and out of scope for closing this specific bug. Also not done: a full live `cmd/agent` batch run to a genuine `APPLIED` status — that remains the Usability Gate's own broader unchecked item (blocked on bug #4's fill-step fix still being unverified in practice, and #5's FunnelEngine false positives), not something this bug's fix alone can close.

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
**Symptom:** confirmed live 2026-07-22 on a Workable posting (European Dynamics, "Junior DevSecOps Engineer"): `clickApplyIfPresent` found the real "Apply now" button, and Playwright's own diagnostic log showed it as visible, enabled, and stable — yet every click attempt retried and eventually timed out at both 5000ms (the production code) and 10000ms (a standalone diagnostic run). No amount of increasing the timeout could have helped, since the actionability check itself never passed.

**Root cause:** a screenshot of the page showed a cookie-consent banner ("Workable uses cookies... Accept all / Decline all") sitting at the bottom of the viewport. Playwright's click-retry log named the actual blocker: a `<div data-ui="backdrop" class="styles__backdrop--1TOnJ">` intercepting pointer events across the click target area. The existing consent-detection logic (used elsewhere for a different check) only matched literal `cookie`/`consent` substrings in element `id`/`class`, but this banner's markup uses obfuscated CSS-module class names, so nothing before this ever saw it as a consent banner at all.

**Fix:** added `dismissCookieBanner(page)` in `pkg/submitter/browser.go`, called immediately after the initial page-load wait and before any dead-job/captcha/interaction checks. Prefers a Decline/Reject button when offered, falling back to Accept — the choice only matters for unblocking the click (this is a one-shot headless session with no persistent identity to protect), and Decline is preferred only because it isn't always offered and the project is otherwise privacy-conscious (`pii.yaml`, `scripts/sanitize_jobs.go`). Verified with a standalone script: the same "Apply now" click that timed out for 10+ seconds before the fix succeeded instantly (`err=<nil>`) after adding the decline-click step. `go build/vet/test ./...` all pass.

### 35. SmartRecruiters' "I'm interested" button and post-click CAPTCHA reveals both went undetected
**Symptom:** confirmed live 2026-07-22 (Oteemo, `jobs.smartrecruiters.com`): a real, non-expired job reached the Learner Module, mapped, and still failed to fill "First Name" — a standalone script showed 0 real inputs anywhere on the page, only one unlabeled input and an empty `about:blank` iframe. A screenshot showed the page was a normal job-description page with a button reading **"I'm interested"**, not "Apply".

**Root cause, part one:** `clickApplyIfPresent`'s locator (`button:has-text('Apply'), a:has-text('Apply')`) never matches SmartRecruiters' own wording, so the click-to-reveal step silently no-ops and every downstream fill attempt targets the public job-description page, which has no form on it at all.

**Root cause, part two:** confirmed via the same standalone script that clicking "I'm interested" navigates to a *new* URL (`jobs.smartrecruiters.com/oneclick-ui/company/.../publication/...`) that can itself be gated by a fresh DataDome challenge (`geo.captcha-delivery.com`) — this exact case reproduced live. Bug #23's captcha check runs once, right after the initial navigation, before this reveal click ever happens, so it never sees a challenge that only appears after the click.

**Fix:** broadened `clickApplyIfPresent`'s selector to also match `"I'm interested"` text. Added a second `isCaptchaBlocked` check immediately after the reveal click in `AttemptSubmit`'s "Unknown ATS" branch, before the Learner Module runs — bails with `BLOCKED_CAPTCHA` instead of burning a full mapping + fill + Vision cycle on an unfillable challenge page. Verified live same session: the very next SmartRecruiters job (SynergyMachines) logged `Clicked an Apply-labeled element to reveal the application form` immediately followed by the new check catching a DataDome challenge and marking `BLOCKED_CAPTCHA` in under 2 seconds. `go build/vet/test ./...` all pass.

### 36. Jobvite's "Data Consent" step means the application form doesn't exist until a location/language <select> is chosen
**Symptom:** confirmed live 2026-07-22 (CMG Financial, `jobs.jobvite.com`): Apply click succeeded, Learner Module mapping succeeded, and the fill still failed on "First Name" at the full 30000ms timeout. A standalone script inspecting the post-click page found **zero `<input>` elements anywhere** — main page, all frames, nothing — only a single `<select>` labeled "Location of Residence and Language" with options "Non-California" and "California" (a CA-privacy-disclosure gate, not an actual country picker despite the label wording).

**Root cause:** the real application form (confirmed 24 real fields once revealed) simply does not exist in the DOM until the `<select>` has a value chosen — not a click-to-reveal pattern like #8, a genuinely separate "form doesn't render until this prerequisite step" pattern. Confirmed via the standalone script that selecting an option alone (no extra button click needed) is sufficient to reveal it immediately.

**Fix:** added `resolveConsentGateIfPresent(page, pii)` in `pkg/submitter/browser.go`, called right before `resolveFillTarget` in the "Unknown ATS" branch. Only activates when the page has zero `<input>` elements *and* a `<select>` is present, so it can't false-positive on normal single-page forms (the vast majority of postings). Prefers an option whose text matches the candidate's actual state — "Non-California" here, correctly, since `pii.yaml`'s address is in Michigan — so the CA-specific disclosure some tenants show stays honest, falling back to the first non-placeholder option (skipping the "Select..." placeholder at index 0) when no match is found. `go build/vet/test ./...` all pass.

### 37. fillActionTimeoutMs (15000ms) too tight for genuine CPU contention from the co-located Ollama model
**Symptom:** confirmed live 2026-07-22, *after* cleaning up the duplicate-process contention (bugs.md's Operational Trap) down to a single clean instance: two different real, non-junk jobs on two different platforms (`utilus.homerun.co`, `jobs.workable.com/view`) still hit `Timeout 15000ms exceeded` on all three fill tiers (label, placeholder, CSS selector) in the same ~45-second window, each time immediately after the local Ollama model finished a heavy generation burst (200%+ CPU on an 8-core/29GB machine, per `ps`/`uptime` at the time).

**Root cause:** the exact same failure shape as bug #6's already-fixed Ollama client timeout — a hardcoded value too short for genuinely slow-but-honest work under real, bursty local-LLM contention, not a selector-strategy bug (#4/#14's territory, already fixed). The three consecutive fill-tier failures in the same short window, immediately following an LLM burst, is the signature of CPU starvation, not a missing element.

**Fix:** introduced a named `fillActionTimeoutMs = 30000` constant in `pkg/submitter/browser.go` and replaced all 6 raw `playwright.Float(15000)` call sites (fill, label-fill, placeholder-fill, click, upload, submit-click) with it. Doubling gave real headroom against the observed contention window without being reckless. **Caveat:** confirmed *not* sufficient on its own — the next job tested after this fix still hit `Timeout 30000ms exceeded` on Jobvite, but that specific case was root-caused separately as #36 (no form existed yet, not a timing issue at all), so this fix's actual hit rate against pure CPU-contention timeouts specifically is still unconfirmed by a clean counterfactual. `go build/vet/test ./...` all pass.

### 38. FunnelEngine kept sending Learner+doc-gen cycles at a 0%-success source and let Workday monopolize the worker queue
**Symptom:** two related findings from a DB analysis of `applications.db` on 2026-07-22 (cross-referencing `job_funnel.url` against `job_funnel.status` by ATS platform): `breezy.hr` had **0 `APPLIED` across 212 discovered jobs**, with 48 `FAILED_SUBMIT` — the worst attempted-vs-success ratio of any platform with meaningful volume. Separately, live observation showed **6 Workday jobs discovered and processed in a row** immediately after a clean single-instance restart, crowding out every other platform.

**Root cause, breezy.hr:** no single root cause found (unlike #34-#36) — just a sustained, evidence-backed pattern of failure worth cutting rather than continuing to chase platform-by-platform.

**Root cause, Workday:** `storage.GetDiscoveredJobs()` (`pkg/storage/manager.go`) had no `ORDER BY`, so its `SELECT ... WHERE status = 'DISCOVERED'` came back in raw SQLite rowid order. 228 Workday rows already sitting in the backlog from an earlier discovery run were inserted consecutively, so they came back clustered together — and since Workday postings are account-gated (bug #18, already fixed), every one of those 6 could only ever reach `MANUAL_REQUIRED`, never `APPLIED`, while doing so ahead of platforms that actually can.

**Fix:** removed `"breezy.hr"` from `TargetATS` (`pkg/scraper/funnel.go`, stops new discovery) and from `isValidATSUrl`'s `atsDomains` allowlist (stops any stray breezy.hr URL from validating even via other discovery paths). Changed `GetDiscoveredJobs`'s query to `WHERE status = 'DISCOVERED' AND url NOT LIKE '%breezy.hr%' ORDER BY CASE WHEN url LIKE '%myworkdayjobs.com%' THEN 1 ELSE 0 END, id` — excludes breezy.hr entirely (no future value expected) and deprioritizes (not excludes) Workday, since it still produces useful pre-tailored manual-apply documents. Verified live: backlog dropped from 2021→1917 on restart (matching the ~104 excluded breezy.hr rows almost exactly), and the very first job pulled after restart was a non-Workday platform for the first time all session. `go build/vet/test ./...` all pass.

### 39. Vision-fallback fill fails with "empty selector provided for form filling" (Resolved 2026-07-23)
**Symptom:** observed live 2026-07-22 on `brightvisiontechnologies.applytojob.com`: a cached form-mapping (learned before today's #34-#37 fixes, so potentially stale) timed out on the primary fill attempt, correctly invalidated itself, and fell back to `AttemptVisionSubmit` — which then failed immediately with `ErrEmptySelector` ("empty selector provided for form filling") rather than attempting a real fill.

**Root cause, confirmed via a standalone script (same methodology as #34-#37):** wrote a small program using the app's own `mcp.Client`/Playwright launch config to navigate a *fresh* (non-cached, non-stale) `brightvisiontechnologies.applytojob.com` posting discovered live that same morning, screenshot it, and call `ExtractFormMappingVision` directly. The page had **zero inputs, zero forms, zero iframes, and zero "Apply"-labeled elements** — a screenshot confirmed why: the posting had expired, rendering JazzHR/ApplyToJob's own banner, **"This position is no longer available. Click here to view more opportunities..."**. This exact wording doesn't match any existing entry in `deadJobPhrases` (`"job is no longer available"`, `"no longer exists"`, etc. — all close but not this one, which says "position" instead of "job"), so the dead-job guard at the top of `AttemptSubmit` (`isDeadJobPage`, checked before doc generation) let it straight through. By the time `AttemptVisionSubmit` ran against a screenshot with no real form in it at all, the vision model (`qwen2.5vl:7b`) had nothing grounded to map — in this reproduction it confidently **hallucinated** a fully plausible-but-fake selector set (`#first-name-input`, `#last-name-input`, etc., none of which exist on the page); the original 2026-07-22 report saw the same underlying "no real form" condition instead produce empty fields/labels for at least one field. Both are the same root cause — a dead page reaching Vision with nothing to see — manifesting as two different failure shapes depending on the model's response that run. This is the same bug class as #9/#15 (a dead-job phrasing variant the guard didn't know about yet), not a Vision-module defect at all.

**Fix:** added `"position is no longer available"` to `deadJobPhrases` in `pkg/submitter/browser.go`. A dead posting with this wording now bails in seconds at the pre-existing `isDeadJobPage` check, before document generation, the Learner Module, or Vision ever run. Added a regression case to `TestIsDeadJobPage`. `go build/vet/test ./...` all pass. Diagnostic script was written to a temporary, untracked directory and deleted before commit, per this project's established practice for these live-repro scripts.

### 40. ~200+ files/dirs under applications/ are still owned by a stale UID from an earlier containerized run
**Symptom:** confirmed live 2026-07-22: `applications/needs_manual_apply/manual_queue.md`, `applications/manual_submissions.md`, and `applications/en/` (a per-job output directory) were all owned by UID `524288` — not `howlcipher` — and every write to them failed with `permission denied`, silently. For `manual_queue.md`/`manual_submissions.md` this meant an unknown number of jobs that should have been recorded as `MANUAL_REQUIRED` were dropped with zero record anywhere. For `applications/en/`, it outright killed an otherwise-successful job (`failed to write resume: ... permission denied`) because the company slug happened to collide with a pre-existing stale directory of the same name.

**Root cause:** UID `524288` is a classic rootless-podman/distrobox subuid mapping — these paths were written by an earlier session running inside (or with root inside) a container, and are simply inaccessible to the `howlcipher` (UID 1000) user these paths are normally accessed as, from either the host or the `career-agent` container (see the regressed Bazzite entry above — same container, unrelated cause).

**Fix, partial:** `manual_queue.md` and `manual_submissions.md` were fixable without root — their *parent* directory was owned correctly, so a delete-and-recreate (preserving content via a copy first) worked fine, since Unix permission checks for unlinking a file depend on the containing directory's write bit, not the file's own ownership. `applications/en/` needed the user to run `sudo rm -rf applications/en` directly, since that directory's *own* mode (755, no group/other write) blocked emptying it even though its parent was owned correctly.

**Still open:** `find applications -not -user howlcipher` shows roughly 200+ other paths in the same state. None of them currently block anything (they're historical per-job output directories the app doesn't write back into), but any future job whose company slug collides with one of these names will silently fail exactly like `applications/en/` did. Needs either a one-time `sudo chown -R $(whoami):$(whoami) applications/` sweep, or a code-level fix: the manual-queue path already has collision-avoidance (suffixes like `en_US-5`, `en_US-6` are visible in `manual_queue.md`), but the main `applications/<company>/` doc-writing path used by `AttemptSubmit` does not.

**Resolved 2026-07-22 (later same session):** the predicted collision happened for real, twice, within 20 minutes — two different jobs (both with the generic company slug "jobs", from `opn.bamboohr.com` and `it8.bamboohr.com`) each completed a full 6-17 minute tailoring cycle and then failed at the very last step, `applications/jobs/resume.md: permission denied`, because `applications/jobs/` was one of the stale-owned directories. That real, repeated cost was enough to justify the full sweep instead of continuing to patch collisions one at a time: user ran `sudo chown -R $(whoami):$(whoami) applications/`. Verified: `find applications -not -user howlcipher` now returns nothing. The code-level collision-avoidance gap (the main doc-writing path still has none, unlike the manual-queue path) remains but is now low-priority since the underlying ownership problem is gone.

### 41. applytojob.com and recruitee.com board-index/landing pages scored and processed as real postings
**Symptom:** confirmed live 2026-07-22 on two different platforms in the same session: `holafly.applytojob.com/apply` scored 80, generated a full tailored application, and then failed the fill stage — a standalone script inspecting the page found zero form fields and a screenshot showed a plain list of 20 unrelated open roles ("Current Openings"), not the specific job that had been scored. Separately, `greatminds.recruitee.com/homepage` showed the identical shape: scored, tailored, then no form to fill.

**Root cause:** both platforms are subdomain-tenant ATS hosts (`company.applytojob.com`, `company.recruitee.com`) whose bare board-index/landing path is indistinguishable from a real posting by URL alone unless the path depth is checked — exactly the same class of bug already fixed months earlier for the six *path*-tenant platforms (`pathTenantATS` in `IsKnownJunkJobURL`), just never extended to these two subdomain-tenant ones. Confirmed the real-posting shape by contrast: `brightvisiontechnologies.applytojob.com/apply/z4xS0fd5C5/Senior-Backend-Engineer` (3 path segments, a real job that reached full doc generation earlier the same night) versus `holafly.applytojob.com/apply` (1 segment, junk). Recruitee's convention is `/o/<slug>` for real postings; even a bare `/o` (seen several times in earlier logs this session, e.g. `sensysgatsogroup.recruitee.com/o`) is suspected to be that tenant's job-board index under the `/o` prefix, not a posting.

**Fix:** added two new blocks to `IsKnownJunkJobURL` in `pkg/scraper/funnel.go`, one per platform, each counting path segments and rejecting ≤1 (mirroring the existing `pathTenantATS` loop's logic but applied per-host since these are subdomain- not path-tenant). Caught live immediately after deploying: the very next `bamboohr.com` board-index URL discovered was correctly skipped at worker intake (see #42). `go build/vet/test ./...` all pass. **Why this one matters most:** `applytojob.com` had 0 `APPLIED` across 176 historical attempts in `applications.db` — the worst attempted-vs-success ratio of any platform still actively targeted after #38 excluded `breezy.hr`. This bug, not a fill-strategy problem, is the most likely explanation for that entire 0% record.

### 42. www.bamboohr.com and app.bamboohr.com pages (marketing site, shared login portal) scored as postings
**Symptom:** confirmed live 2026-07-22: `www.bamboohr.com/integrations/listings/remote` (BambooHR's own product-integrations marketing page) scored 80 and burned a 16-minute tailoring cycle before the Learner Module found nothing fillable. Minutes later, `app.bamboohr.com/login/` — BambooHR's shared employee login portal, used by every tenant, not a job posting at all — also scored 80 and reached `AttemptSubmit`.

**Root cause:** identical shape to the already-fixed homerun.co bug: BambooHR is a subdomain-tenant platform where real postings always live on a company subdomain (`cxm.bamboohr.com/jobs/questions?id=169`), but the bare `www.bamboohr.com` (marketing site) and `app.bamboohr.com` (shared app/login shell) hosts were never excluded, so anything discovered on them got treated like a tenant posting.

**Fix:** two more bare-host checks added to `IsKnownJunkJobURL` alongside the homerun.co one: `host == "bamboohr.com" || host == "www.bamboohr.com"` and `host == "app.bamboohr.com"`. Verified live: the very next `www.bamboohr.com/careers/` URL pulled from the backlog was correctly caught at worker intake (`Skipping known-junk URL (never a posting)`), rather than reaching doc generation. `go build/vet/test ./...` all pass.

### 43. getByLabel/getByPlaceholder threw a Playwright strict-mode violation when a label matched more than one element
**Symptom:** confirmed live 2026-07-22 on a Workable/Dispel posting — after every other fix this session had landed (cookie banner, Apply-button matching, consent gates, junk-page filters), a real fill attempt finally got past "First Name" cleanly, the first time all session a fill reached a second field at all. It then failed immediately on "Phone": `playwright: Error: strict mode violation: getByLabel('Phone') resolved to 2 elements`.

**Root cause:** Playwright's `getByLabel`/`getByPlaceholder` throw rather than silently pick one when more than one element on the page shares the same accessible label or placeholder text (e.g. a visible phone field plus a hidden duplicate, or a country-code sub-control also labeled "Phone"). `GetByLabelLoc`/`GetByPlaceholderLoc` in `pkg/submitter/browser.go` called these directly with no disambiguation.

**Fix:** both methods, on both the `pageTarget` and `frameTarget` implementations, now chain `.First()`. Filling a field isn't order-sensitive — any element genuinely carrying that label is an acceptable fill target, so narrowing to the first match is strictly better than failing the whole field (and cascading to a Vision fallback) over an ambiguity that doesn't actually matter for correctness. `go build/vet/test ./...` all pass. **Not yet verified end-to-end** — confirmed the error class and the fix's mechanism, but hasn't yet been observed live producing a full successful submission past this point.

### 44. BambooHR corporate subdomains kept slipping past a growing denylist (Resolved 2026-07-23)
**Symptom:** confirmed live over 2026-07-22 into 2026-07-23: after #42 excluded `www.bamboohr.com` and `app.bamboohr.com` specifically, two *more* BambooHR corporate subdomains were discovered and processed as postings a few hours later during unattended running — `learn.bamboohr.com/introduction-to-bamboohrs-open-api` and `trust.bamboohr.com/controls`, each burning a full doc-gen cycle. A `find`-style grep of the log for every unique `*.bamboohr.com` URL seen turned up two more still unexcluded (`developers.bamboohr.com`, `documentation.bamboohr.com`).

**Root cause:** #42's fix was a denylist of specific known-bad subdomains, which only ever catches subdomains already observed failing — BambooHR evidently has an open-ended family of shared corporate subdomains (login, product docs, compliance, API docs, ...) that a one-at-a-time denylist can never keep ahead of.

**Fix:** replaced the denylist with a positive check instead. A grep across every real BambooHR posting URL seen this session (dozens, across many different tenant subdomains) showed they *all* use exactly one of two path shapes: `/jobs/questions...` or `/careers/<id>`. `IsKnownJunkJobURL` now treats any `*.bamboohr.com` URL whose path doesn't match one of those two shapes as junk, regardless of which subdomain it's on — this catches every subdomain above plus any future one without needing to be told about it individually. `go build/vet/test ./...` all pass.
### 130. Yahoo fallback drops discovery on transient unexpected EOF responses

**Evidence:** the live `career_agent.log` review on 2026-07-27 found **148** lines of `Yahoo fallback failed: ... unexpected EOF` during one continuous run. The failures span many role/ATS queries, so the fallback source is repeatedly losing discovery opportunities rather than failing on one isolated query.

**Root cause:** `pkg/scraper/funnel.go::discoverWithYahooHTML` performs one HTTP request with a 10-second client and returns immediately when `client.Do` returns an error. It has no bounded retry/backoff, no distinction between transient transport failures and permanent responses, and no per-source health signal for the caller or dashboard.

**Acceptance criteria:** add a small context-aware retry policy for retryable transport errors and retryable status codes; preserve the existing per-query rate limit; stop after a bounded attempt budget; log the final failure with query-safe context; add tests for transient recovery, exhausted retries, non-retryable responses and cancellation; do not log API keys or personal data.

### 131. ATS board polling discards truncated JSON without retry

**Evidence:** the same live log contained seven `unexpected end of JSON input` errors across four known boards: `jobgether` twice, `veeva` twice, `weloglobal` twice and `bluelightconsulting` once. Each error occurred in `pollBoard`, which means the complete current posting list for that board was discarded for that discovery pass.

**Root cause:** `pkg/scraper/atsfeeds.go::fetchATSFeed` reads one response and `pollBoard` calls the parser once. A partial or rate-limited response is logged and converted to zero discovered jobs; there is no retry, response-size/content validation, or board-level cooldown to prevent repeated noisy failures on the next continuous pass.

**Acceptance criteria:** retry truncated or otherwise retryable feed responses with a bounded, injectable backoff; validate status, content type and non-empty body before parsing; keep malformed payloads isolated to one board; add tests for transient truncation recovery, persistent malformed JSON, empty bodies and cancellation; retain the existing title and junk-URL gates.

## 393 Playwright Host missing dependencies to run browsers
**Symptom:** Over 1000 `Host system is missing dependencies to run browsers` warnings in `career_agent.log`.
**Impact:** Headless browser actions fail, preventing form submission.
**Fix:** Run `npx playwright install-deps` or add missing libraries (`libicudata.so.74`, etc.) to the environment setup.

## 394 QUARANTINED_PROMPT_INJECTION has massive false positive rate on legitimate jobs
**Symptom:** Over 400 jobs are stuck in `QUARANTINED_PROMPT_INJECTION` status in `job_funnel`.
**Impact:** legitimate jobs (e.g., Senior Backend Engineer at Instrumentl via Lever) are never applied to.
**Fix:** Refine the prompt injection heuristic to distinguish between actual injections and normal ATS text.
**Status:** Done. Switched the promptsec protector from Strict to Moderate, which prevents false positives on benign ATS instructions while still catching actual injection attempts.

## 395 Validation loop times out waiting for Ollama context deadline
**Symptom:** 480 errors in `career_agent.log` with `failed to solve validation errors: ... context deadline exceeded`.
**Impact:** Validation loop fails to resolve form errors because the API call to Ollama times out.
**Fix:** Increase the HTTP client timeout for Ollama calls, especially during the validation phase which may pass large DOM contexts.
**Status:** Done. Increased `defaultOllamaTimeoutMinutes` to 120 (from 45) to allow sufficient time for attention decoding of massive DOM contexts.
