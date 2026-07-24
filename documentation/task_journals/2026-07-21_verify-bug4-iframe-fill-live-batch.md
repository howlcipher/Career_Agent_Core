# Task Journal: Verify bug #4 (iframe form-fill fix) via live cmd/agent batch run

## Summary

- **Task:** bugs.md #4 — `AttemptSubmit` form-fill logic never looked inside iframes. Fix applied 2026-07-20 (`resolveFillTarget` in `pkg/submitter/browser.go`), closed 2026-07-23 via unit tests (see bugs.md #4's Details — live traffic can no longer organically exercise this path). Working live batches since 2026-07-21 toward the Usability Gate's remaining condition: zero open Major bugs. That work fanned out into ~50 other real, independently-diagnosed bugs (#5 through #57), all detailed in `bugs.md` directly rather than here — this journal only tracks what's needed to resume the live-run operation itself.
- **Status:** In progress
- **Started:** 2026-07-21
- **Agent and model:** Claude Code / Sonnet 5 (orchestrator; delegates real fixes per the Working Protocol when well-scoped)

## Pre-Flight Re-Evaluation (last done 2026-07-24 ~15:20)

- **Usability Gate check:** NOT MET. Static checks, Ollama install, `cmd/dashboard`, and `cmd/tracker` crash-safety are all verified (see `bugs.md`'s gate checklist for the authoritative state). The two remaining gaps are the **82-job re-verification** (this task, still 0/82 resolved) and zero open Blocker/Major bugs — currently #8, #10, #14 (all Major, fix applied and confirmed firing live, just no path-specific fresh `APPLIED` yet) and #48 (Minor, not yet root-caused) are still Pending — see `bugs.md`'s Ranked Backlog.
- **Model choice:** No delegation needed for observation; code fixes so far have been split between direct edits (well-understood, small) and `agy` delegation (larger, clearly-scoped diffs, always verified against `git diff` before trusting).
- **Code re-verified 2026-07-24 ~15:20:** `go build/vet/test ./...` all pass clean after today's dashboard change (commit `f43cc6c`, pushed to `origin/main`).

## Progress Log (condensed — full blow-by-blow lives in bugs.md's Detail sections, not duplicated here)

- 2026-07-21 — Long live-triage session. Diagnosed and fixed #5 through #17. Five orphaned `go run` child processes found contaminating one observation window (Operational Trap, documented at the top of `bugs.md`) — cleaned up.
- 2026-07-22 — Further live triage (~6 hours): found and fixed a real environment regression (agent running on the bare Bazzite host instead of the `career-agent` distrobox), then a chain of real interaction bugs (#34-#38, #41-#44).
- 2026-07-23 — Priority-queue + requeue tooling shipped (`cmd/requeue`, source-priority `ORDER BY`). Two independent CAPTCHA-detection false positives found and fixed (#45 Blocker, #46 Blocker) — these had been killing the large majority of Greenhouse/Lever/Ashby/Workable jobs before fit-scoring or fill ever ran. Once cleared, #47 (dedicated Lever/Greenhouse handlers missing the click-to-reveal step) surfaced and was fixed. **First fresh, real `APPLIED` produced since this effort began**, via a targeted `TARGET_JOB_URL` test — Usability Gate's live-batch checkbox checked. Bug #4 closed via unit tests (structurally unreachable live — see above). #48-#51 found and fixed the same day (Greenhouse submit-selector fallback, Workable account-gating, and — most significant — #51: the post-submit success check trusted any URL change, not actual confirmation evidence).
- 2026-07-24 (overnight into today) — User asked to requeue all 82 previously-`APPLIED` jobs and re-verify them for real, after #53 found `isSubmissionConfirmed` (#51's fix) was structurally unreachable for every ATS except Lever/Greenhouse/LinkedIn — meaning most of the 82's original `APPLIED` status had zero confirmation evidence, ever. `TARGET_JOB_URL` extended to accept a comma-separated list, `cmd/requeue -status APPLIED` support added, both shipped before executing. All 82 reset `APPLIED` → `DISCOVERED`, isolated `TARGET_JOB_URL` run launched (self-terminating, does not pick up normal discovery — **the normal full-backlog batch must be manually restarted once this finishes**, see `bugs.md`'s Operational Trap notes for the restart procedure). Chain of real bugs found and fixed while watching this run: #52 (payload-size circuit breaker, recurred 3× with genuinely different root causes each time), #53 (confirmation logic unreachable for most ATS, Blocker), #54 (Ashby SPA false-positive), #55 (orphaned PROCESSING rows never retried), #56 (dashboard missing two status tiles), #57 (forms too large for Ollama's context window, routed to `MANUAL_REQUIRED` instead). 4 restarts total, each picking up one new fix. **Still 0/82 resolved as of this writing** — every real outcome so far has been a new bug found and fixed, not an answer to "were the original 82 real."
- 2026-07-24 ~15:20-15:30 (this session, after a `/clear`) — Resumed from journal, confirmed PID 3137654 alive and the 82-job breakdown unchanged (0 `APPLIED`), re-armed a monitor. User asked to add "time to process" (discovered_at → terminal-status duration) to the dashboard's Last Applied/Skipped/Failed/Manual cards, doubting the Manual Queue tile's accuracy. Implemented (`cmd/dashboard/main.go`: `formatDuration` helper + 4 new `*ProcessingTime` fields; `cmd/dashboard/index.html`: new "Time to process: …" line per card; `cmd/dashboard/main_test.go`: new tests including a NULL-`discovered_at` guard). **Confirms the user's suspicion**: the Manual Queue's driving row took 7d19h from discovery to `MANUAL_REQUIRED`, Last Skipped/Failed over 10d — the tiles are raw current-status counts, not "recently flagged," and this single-worker queue is genuinely that far behind on old backlog. Verified live via a rebuilt dashboard binary + a real screenshot (had to fall back to Firefox — this environment's Chromium build is missing `libicudata`/`libicui18n`/`libicuuc`/`libavif`, per the Playwright warning already logged at every `cmd/agent` startup, which silently zeroes out all text layout/`getBoundingClientRect()` results without erroring; Firefox has no such dependency gap here). `go build/vet/test ./...` all pass. Committed (`f43cc6c`) and pushed at the user's explicit request.

## Next Step (accurate as of 2026-07-24 ~15:30, right before another planned session clear)

**Core unanswered question (why this task is still open):** user asked to requeue all 82 previously-`APPLIED` jobs (39 Lever, 32 Greenhouse, 7 Workday, 2 SmartRecruiters, 2 Pinpoint) and re-verify them for real now that bug #53's confirmation-logic fix is live. **As of this writing, 0 of the 82 have reached a confirmed `APPLIED` or a genuine duplicate-block result.** Don't declare this task done just because bugs stop surfacing — it's done when the 82 (or as many as reach a terminal state) show real evidence one way or the other. See bugs.md's #52/#53 Details sections for the full diagnostic history if more context is needed; it is not repeated here.

**Live process running right now, independent of any chat session:** PID `3137654` (`/tmp/career_agent_bin_verify82d`, has every fix through bug #57), started 2026-07-24 ~15:10, running inside the `career-agent` distrobox. Confirm alive: `distrobox enter career-agent -- ps -p 3137654`. Tail: `distrobox enter career-agent -- tail -f career_agent.log`.

**Outcome-breakdown query** (82-URL list at `applied_urls_verify82.txt`, repo root, untracked — don't delete, needed for this query):
```bash
urls_file="/var/home/howlcipher/dev/Career_Agent_Core/applied_urls_verify82.txt"
in_clause=$(awk '{printf "%s%s%s", (NR>1?",":""), "\x27", $0"\x27"}' "$urls_file")
distrobox enter career-agent -- sqlite3 /var/home/howlcipher/dev/Career_Agent_Core/applications.db \
  "SELECT status, COUNT(*) FROM job_funnel WHERE url IN ($in_clause) GROUP BY status ORDER BY status;"
```
Last read (15:30): `DISCOVERED=77 FAILED_SUBMIT=1 MANUAL_REQUIRED=1 PROCESSING=1 SKIPPED=2`. The `MANUAL_REQUIRED=1` is Reddit — **first live confirmation of bug #57's fix working as designed**: log shows it filled/retried normally, then `"Reddit's form is too large for the local model — queued for manual submission"` instead of crashing on the Ollama 400. Now on Akuity (same form-size class, expect the same routing).

**Operational note:** a second monitor (task `b1ojsq6q2`, description referencing "bug #53 re-verification run") also fired a notification this session even though it wasn't armed by this session's own tool calls — it may be a stray survivor from an earlier session despite the "monitors don't survive a clear" assumption in this file's own notes. Its data matched a fresh direct query, so it's trustworthy, but if two monitors are both alive and firing, consider consolidating to avoid duplicate/confusing notifications.

**A fresh session's own Monitor tool does NOT survive a clear — re-arm one.** Track completion by process liveness, not by grepping the shared log (the log has many historical "Batch execution complete" lines from earlier restarts tonight, and multiple processes have written to it — a naive grep false-positives instantly). Last-armed monitor used this pattern, polling every 90s:
```bash
urls_file="/var/home/howlcipher/dev/Career_Agent_Core/applied_urls_verify82.txt"
in_clause=$(awk '{printf "%s%s%s", (NR>1?",":""), "\x27", $0"\x27"}' "$urls_file")
pid=3137654
prev=""
while true; do
  if ! distrobox enter career-agent -- kill -0 "$pid" 2>/dev/null; then
    echo "PROCESS $pid IS NO LONGER RUNNING -- run may have completed or crashed."
    break
  fi
  cur=$(distrobox enter career-agent -- sqlite3 /var/home/howlcipher/dev/Career_Agent_Core/applications.db \
    "SELECT status || '=' || COUNT(*) FROM job_funnel WHERE url IN ($in_clause) GROUP BY status ORDER BY status;" 2>/dev/null | tr '\n' ' ')
  if [ "$cur" != "$prev" ]; then echo "STATUS CHANGE: $cur"; prev="$cur"; fi
  sleep 90
done
```
Don't kill/restart PID 3137654 just to attach a fresh monitor — that only loses progress.

**IMPORTANT — this is NOT the normal full-backlog batch.** It's a dedicated, isolated `TARGET_JOB_URL` run restricted to exactly the 82 URLs. It is self-terminating: once all 82 resolve, the process exits on its own and does **not** pick up fresh discovery or the rest of the ~3000-job backlog. **After this run finishes (or if you decide to abandon it), the normal batch must be manually restarted** — build `cmd/agent` fresh, launch WITHOUT `TARGET_JOB_URL` set, confirm sole instance, re-arm a normal `APPLIED`-count monitor. See the Operational Trap notes at the top of `bugs.md` for the full restart procedure.

**A duplicate-application nuance to remember:** the user explicitly does not want "the ATS correctly rejected a re-submission as a duplicate" treated as a bug — that's actually confirming the original was genuine. Check the real failure reason/page content (not just "FAILED_SUBMIT") before concluding anything's wrong.

**Also running, separately:** `cmd/dashboard` on port 8080, now PID `3148044` (`/tmp/career_dashboard_v2`, rebuilt 2026-07-24 ~15:22 with today's "time to process" feature). Check first with `ss -tlnp | grep 8080` before rebuilding — a stray earlier build can squat the port under an unrelated-looking process name, as happened once already this week. If it needs restarting: `distrobox enter career-agent -- bash -c "cd /var/home/howlcipher/dev/Career_Agent_Core && /usr/local/go/bin/go build -o /tmp/career_dashboard_v2 ./cmd/dashboard && nohup /tmp/career_dashboard_v2 > /tmp/dashboard.log 2>&1 & disown"`.

**Environment note for any future visual/Playwright verification:** this machine's Chromium (via `mxschmitt/playwright-go`) is missing `libicudata.so.74`/`libicui18n.so.74`/`libicuuc.so.74`/`libavif.so.16` — pages load and JS runs fine, but **all text has a zero-size layout box** (`getBoundingClientRect()` returns 0×0, `innerText` is empty) with no error surfaced anywhere. A screenshot taken with Chromium will look visually broken/blank of text even when the page is correct. Use `pw.Firefox.Launch()` instead (confirmed working 2026-07-24) until/unless those system libs get installed.

**Once the 82-job run resolves (or is abandoned) and #48 (the only remaining Pending bug) is closed or explicitly deprioritized, flip the Usability Gate's Status line in `bugs.md` to `MET` and this task is done — delete this journal in that final commit.**
