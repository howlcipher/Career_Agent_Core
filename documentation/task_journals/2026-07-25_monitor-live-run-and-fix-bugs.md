# Task Journal: Monitor the 82-job live run, fix bugs as they surface

## Summary

- **Task:** User asked me to sit and monitor the live 82-job re-verification run, fix any bugs that arise, log them in `bugs.md`, groom the backlog when doing so, and keep this journal against a session limit/outage. Explicit standing authority: *"If a choice arises, do what you recommend, don't feel you need to ask, do not do anything that adds a monetary cost."*
- **Status:** In progress
- **Started:** 2026-07-25 ~12:24
- **Agent and model:** Claude Code / Opus 5

## Standing constraints for this session

- **No monetary cost.** Local Ollama only. Do not build anything needing a paid key/signup — that rules out `improvements_paywall.md` entirely (#17 CAPTCHA solving) and improvements #14 (LoRA, needs paid cloud compute — no discrete GPU here).
- **Do not kill/restart PID `3755906`** casually. Restarting loses in-flight progress and re-snapshots the queue (the queue is a *startup snapshot* — see the standing note in `bugs.md`'s Operational Trap section). Only restart when there is a fix worth picking up, and rebuild from current HEAD when doing so.

## Live state at session start (2026-07-25 12:24)

- Agent PID `3755906` (`/tmp/career_agent_bin_verify82j`, built at HEAD `81792b0`) — alive, started 12:04.
  **Note it predates `2972797` (#69) and `4134b3d` (#26)** — it does not carry the real-title fix or ATS feed discovery.
- 82-cohort breakdown: `DISCOVERED=69 FAILED_SUBMIT=6 PROCESSING=1 SKIPPED=6` → moved to `FAILED_SUBMIT=7 DISCOVERED=68` at ~12:32.
- **Still 0 of 82 confirmed `APPLIED`.** That remains the open question of the parent journal (`2026-07-21_verify-bug4-iframe-fill-live-batch.md`).

## Monitors armed in THIS session (previous session's were orphaned and killed)

- `bcivm59k9` — polls the 82-cohort status breakdown every 120s, emits on change, and on PID `3755906` death. Script: `scratchpad/watch_82.sh`.
- `bpsfomclo` — `tail -F career_agent.log` filtered to outcome/failure signatures.
- Killed PID `3756779`, the previous session's monitor `b03wmuerf` — it was alive but its notifications could never reach this session.

## Progress Log

- **12:24** — Resumed. Verified journal claims against the tree: working tree clean, HEAD `c9e270c`, agent process genuinely alive. The most-recently-modified journal (`2026-07-25_throughput-and-discovery-quality.md`) is **Complete**; the genuinely in-flight task is the parent 82-job re-verification.
- **12:32** — First bug surfaced live: **Reddit (`job-boards.greenhouse.io/reddit/jobs/8044767`), fit score 90 — the best-scoring job in the cohort — failed with `failed to submit application after 3 validation error attempts`.** Burned ~17.5 min of LLM time (12:14:55 → 12:32:28) for nothing. Under investigation; see below.

## Shipped this session

- **bugs.md #70 (Blocker) — `d68ce61`.** The validation-retry loop stripped the page's own error text. `aria-describedby` was in `presentationalAttrs`, so `StripPresentationalAttrs` severed the WCAG link from a rejected control to its error message *before* `PruneDOMToInvalidFields` ran; the pruner then dropped the message element as neither control nor label. The model was told a field was invalid but never what would make it valid. Plus an empty fix map fell through to re-submitting a byte-identical form. Fixed all three; 2 new regression tests, verified failing first.
- **bugs.md #71 (Major) — `4234fba`.** `firstVisibleLocator` falls back to `loc.First()` when nothing is visible, and the submit-click site then clicked an element it had just proven invisible — hanging the full 30s action timeout and misreporting "no visible submit button" as a generic Playwright timeout. Added `firstVisibleSubmit` returning `(locator, ok)`; fill call sites keep the old fallback. 2 new tests.
- **Backlog groomed:** dated groom-pass note added to `bugs.md`. No re-ranking was warranted — all bug rows Resolved, both remaining `improvements.md` Pending rows below the ROI floor.
- **Cleaned up 5 stale leftover monitor shells** (PIDs 2166044/2295635/2368407/2476472/2543238) from long-disconnected sessions, all targeting confirmed-dead PIDs 2165142-2542429. The 07-21 journal predicted they would self-terminate; they had not, and each held a `tail -F` on the log. Verified every target PID dead before killing.

## Run restarted onto the fixes

Killed PID `3755906`, confirmed dead, confirmed zero other agent binaries running. Audited the cohort rather than blanket-resetting: of 7 `FAILED_SUBMIT`, 5 are the known-dead postings (Netcraft, NABIS, Postscript, Sphinx Defense, chownow) — left untouched. Requeued exactly **Reddit** (#70's victim) and **Zimperium** (#71's victim), both with `-clear-dedup`, or `HasApplied` would have silently skipped the retry.

Rebuilt from HEAD to `/tmp/career_agent_bin_verify82k`. **New PID `3778859`**, confirmed healthy at 12:43: reaped the 1 orphaned `PROCESSING` row, `loaded 71 matching job(s)`, RAG found 9 career chunks (bug #58's fix persists), and started on **Reddit** — the exact job #70 killed. Cohort at relaunch: `DISCOVERED=70 FAILED_SUBMIT=5 PROCESSING=1 SKIPPED=6` (82 total).

Monitors re-armed for the new PID: `b8wgiddyn` (cohort status + PID death). The log monitor `bpsfomclo` survived the restart since it tails the file, not the process.

## Open investigation: the validation-retry loop does not converge (RESOLVED — this became #70)

Evidence from `career_agent.log`:

```
12:15:03 Submission failed validation. Retrying...
12:15:03 Attempt 2: Solving validation errors...
12:15:03 Narrowed validation retry to the rejected fields only (53366 -> 5363 chars)
12:27:16 Submission failed validation. Retrying...
12:27:16 Attempt 3: Solving validation errors...
12:27:16 Narrowed validation retry to the rejected fields only (53228 -> 5439 chars)
12:32:28 Submission failed validation. Retrying...
12:32:28 Auto-Submit failed for Reddit: failed to submit application after 3 validation error attempts
```

Signals that this is a real defect, not just a hard form:
1. The page DOM barely changed between attempts (53366 → 53228 chars) and the narrowed slice *grew* (5363 → 5439) — consistent with the same fields being rejected each time, i.e. the LLM's fix is not landing.
2. **Nothing logs *which* fields were rejected or what the model tried to set them to.** That is itself a gap — it makes this class of failure undiagnosable from logs alone, and this is the single most expensive failure mode in the pipeline (~6 min per wasted attempt).

## Next Step

**Keep monitoring PID `3778859` and keep fixing what surfaces.** That is the standing instruction for this session; it does not complete until the user says so.

The immediate question is now sharply posed: **Reddit (90) and Zimperium (85) are both back in the queue running against #70's and #71's fixes.** Reddit is being processed first (started 12:43:52). Watch for:
- `Attempt N applied X/Y validation fix(es) to: <selectors>` — new in #70. If the *same* selectors recur across attempts, the fix still is not landing and the diagnosis was incomplete.
- `found N submit control(s) but none visible` — new in #71. If Zimperium produces this, the real question ("why does Lever show no visible submit control?") is finally askable, and that becomes the next bug.
- `Submission confirmed for ...` — the first genuine `APPLIED` of the whole 82-job effort, still 0 as of this writing.

**Do not restart PID `3778859`** unless there is a new fix worth picking up. It carries everything through `4234fba`, including #69 and #26, which the previous run did not.

**Note #26 (ATS feed discovery) still has not run live** — this is an isolated `TARGET_JOB_URL` run, which skips `FunnelEngine.DiscoverJobs` entirely. `discoverWithATSFeeds` only executes on the next *normal* batch start. Watch then for `ATS board feeds contributed N new posting(s)` and check the title gate is not over-filtering.

**Standing warnings carried forward:** monitor liveness is best checked by reading the monitor's own output file, not `TaskList`; Ollama serves warm prompt caches so benchmark only on unseen jobs; `OLLAMA_FAST_MODEL` is intentionally unset (improvements #24).
