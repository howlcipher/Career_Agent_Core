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

## Next Step

Confirm an available small qwen3 tag, pull it, benchmark it against the 30B on identical real jobs (agreement around the `<50` skip threshold is what matters, not raw speed), and only then set `OLLAMA_FAST_MODEL`.
