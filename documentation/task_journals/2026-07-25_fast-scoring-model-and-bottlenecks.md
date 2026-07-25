# Task Journal: Fast scoring model (#24), result quality, and bottleneck triage

## Summary

- **Task:** User goal 2026-07-25, three parts: (1) recommend and actually deploy a model for `improvements.md` #24, (2) find anything that improves real-world results, (3) log the emerging bottleneck, file bugs/improvements from it, and work those too.
- **Status:** In progress
- **Started:** 2026-07-25
- **Agent and model:** Claude Code / Opus 5

## Pre-Flight Re-Evaluation

- **Usability Gate:** MET. Zero open bugs in `bugs.md`. Free backlog was empty of actionable items before this goal.
- **Model choice:** Opus 5 inline (user set it as session default). Antigravity was at a session limit earlier today.
- **Authorization note:** the user's goal explicitly says "recommend a model for #24 and use that" — that is the explicit approval `AGENTS.md` requires before a new install (`ollama pull`). No other install is covered by it.

## Key constraint found before recommending anything

`~/.config/systemd/user/ollama.service.d/override.conf` sets **`OLLAMA_MAX_LOADED_MODELS=1`** (bug #13's OOM mitigation, after two real kernel OOM-kills). Live memory: **29.9GB total, 26.3GB used, ~3.6GB available**, with the 30B at **19.3GB RSS**.

This rules out simply running two models concurrently — raising the limit to 2 would put total usage around 28GB of 29.9GB, and bug #13 is a documented history of genuine OOM-kills at this exact ceiling, not a theoretical risk.

**Why a fast scoring model still wins despite the single-model limit** — because of what the pipeline actually calls now:
- `ScoreJob` — every job.
- `ProcessJobApplication` — **no longer called at all** (improvements.md #23).
- `ExtractFormMapping` — only for ATS platforms without a dedicated handler.
- `SolveValidationErrors` — only after a validation failure.

So for a **Greenhouse or Lever job that fills cleanly — the bulk of the queue — `ScoreJob` is now the only LLM call in the entire job.** Putting it on a small model means those jobs never touch the 30B, so there is no swap to pay for. Only unknown-ATS or validation-failing jobs incur a model swap.

## Progress Log

- 2026-07-25 ~00:45 — Verified the constraint above. Decided the approach: keep `OLLAMA_MAX_LOADED_MODELS=1`, put only `ScoreJob` on a small model, measure before trusting it.
- 2026-07-25 ~00:53 — **Model chosen and pulled: `qwen3:4b-instruct`** (4.0B dense, Q4_K_M, 2.5GB, 262k context). Rationale: same family as the incumbent `qwen3:30b-instruct`, so it interprets the same scoring rubric with the least behavioral drift — the goal is to reproduce the 30B's judgment cheaply, not to introduce a different opinion. Dense 4B rather than a smaller 1.7B because scoring needs real reading comprehension over a full job description. First `ollama pull` returned a 503; a retry succeeded (worth knowing — treat a 503 here as transient, not as a bad tag).
- 2026-07-25 ~00:52 — **Stopped the live 82-job run (PID `3486446`).** Three reasons, not just the benchmark: (a) it had been inside one `SolveValidationErrors` call for 22 minutes, the same step that had already failed this exact job once; (b) its binary predates bug #63, so it was still discarding every fit score; (c) `OLLAMA_MAX_LOADED_MODELS=1` means benchmarking necessarily evicts the 30B, so the two cannot run concurrently. It gets restarted once the model question is settled, with #63 and #64 included.
- 2026-07-25 ~01:00 — Built `scorebench` (scratchpad, not committed — throwaway harness): pass 1 fetches and **caches** each job description, pass 2 scores from that cache, so both models see byte-identical prompts. Uses the repo's own `mcp.Client.ScoreJob`, so the prompt is production's, not a reconstruction. 6 real jobs from the 82 cohort cached, 5,594-13,990 chars.
- 2026-07-25 ~01:05 — **Found and fixed the bottleneck the goal asked about: `bugs.md` #64** (committed `6f0b8a5`). `SolveValidationErrors` re-sent the *entire* form on every retry (~55k chars) when only a few fields had failed; at the measured ~7 tok/s that is >30 min against a 45-min timeout, so large forms failed on time rather than logic. Compounded by `StripPresentationalAttrs` stripping `aria-invalid` — the very signal identifying which field was rejected — which made narrowing impossible. Both fixed, with a deliberate fall back to the full form when no invalid marker can be read.

## Findings on result quality (goal item 2)

- **Zero `APPLIED` rows exist in the entire database** (all-time: 3133 `DISCOVERED`, 301 `INVALID_URL`, 264 `FAILED_SUBMIT`, 115 `SKIPPED`, 36 `BLOCKED_CAPTCHA`, 22 `FAILED_SCORE`, 12 `MANUAL_REQUIRED`). Not one confirmed application has ever completed.
- Historical failure reasons (rotated log, 622 scored jobs) are dominated by **119× "form failed to render in time"** and **60× "could not launch browser: target closed"**. Both predate most of this week's fixes, so the counts are not current evidence — but "could not launch browser" is a resource/stability class that no bug so far has addressed and is worth watching for recurrence.

## Next Step

Awaiting the 4B benchmark (`results_4b.json`); then run the identical cached set against `qwen3:30b-instruct` for the baseline, compare agreement — **especially across the `<50` skip threshold, which is what actually changes behavior** — and set `OLLAMA_FAST_MODEL` only if agreement holds. Then rebuild and restart the 82-job run with #63/#64 and whatever model decision follows.
