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

## Open investigation: the validation-retry loop does not converge

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

Read `pkg/submitter`'s validation-retry path (`SolveValidationErrors` + the 3-attempt loop) and determine whether the fix is (a) not being applied to the DOM, (b) being applied but rejected again, or (c) the rejected-field extraction is wrong. File in `bugs.md` with real evidence, then fix. Do **not** restart PID `3755906` until there is a fix worth picking up.
