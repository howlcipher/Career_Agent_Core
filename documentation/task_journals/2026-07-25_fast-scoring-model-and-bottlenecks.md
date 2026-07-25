# Task Journal: Fast scoring model (#24), result quality, and bottleneck triage

## Summary

- **Task:** User goal 2026-07-25, three parts: (1) recommend and actually deploy a model for `improvements.md` #24, (2) find anything that improves real-world results, (3) log the emerging bottleneck, file bugs/improvements from it, and work those too.
- **Status:** Ongoing — goal items complete; #64 unmasked bug #65, now fixed and under live observation
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

- Current status spread: 3133 `DISCOVERED`, 301 `INVALID_URL`, 264 `FAILED_SUBMIT`, 115 `SKIPPED`, 36 `BLOCKED_CAPTCHA`, 22 `FAILED_SCORE`, 12 `MANUAL_REQUIRED`, **0 `APPLIED`**.
  - **Correction, made before this claim went any further:** the 0 is *expected and self-inflicted*, not evidence that nothing ever worked. All 82 historically-`APPLIED` rows were deliberately reset to `DISCOVERED` at the start of this re-verification effort — that is the entire cohort in `applied_urls_verify82.txt`. A genuine, confirmed `APPLIED` was also produced 2026-07-23 (a real Lever posting at `jobs.lever.co/smarsh/...`, which is what flipped the Usability Gate's live-batch box). The honest statement is: **82 applications were recorded historically but on evidence bug #53 later showed was unreliable, one application is confirmed genuine, and the re-verification of the 82 is still unanswered.** Do not repeat "nothing has ever applied" — it is wrong.
- Historical failure reasons (rotated log, 622 scored jobs) are dominated by **119× "form failed to render in time"** and **60× "could not launch browser: target closed"**. Both predate most of this week's fixes, so the counts are not current evidence — but "could not launch browser" is a resource/stability class that no bug so far has addressed and is worth watching for recurrence.

## ~~Open lead~~ **DISPROVEN 2026-07-25 ~01:40 — do not chase this again**

The reasoning-token hypothesis below is **wrong**. Probed directly once Ollama was free:
```
content        : '42'
thinking field : ''
eval_count     : 3      <- tokens generated
prompt_eval    : 17
```
Three generated tokens, no `thinking` content, clean output. The `-instruct` variants are non-reasoning, and Ollama is not defaulting thinking on. **No tokens are being wasted on chain-of-thought, and setting `"think": false` would change nothing.** (The probe's 255s `total_duration` was queue wait behind the benchmark process being killed, not inference — do not read it as a latency measurement.)

**What the cost actually is: prompt processing, and it is CPU-bound.** The 30B did ~3,900 prompt tokens in 9m38s (**~6.8 tok/s**).

> **Superseded — this paragraph originally estimated the 4B at "~17 tok/s, about 2.5x faster". That was wrong**, computed from warm-cache timings before the trap below was understood. Clean cold measurements later showed the 4B at **367s** against the 30B's **358-421s** — no speed advantage whatsoever. See the RESULT block above for why (the incumbent is an MoE with ~3B active parameters, so a 4B dense model is not smaller in compute terms).

**The corollary worth acting on:** since cost scales with prompt length, the cheapest remaining lever is sending `ScoreJob` a shorter prompt. It currently receives the full job description (5.5k-14k chars in the sampled set) plus the full résumé text. Trimming the description for scoring purposes is likely a larger, cheaper win than any further model change — but it must be measured against score agreement first, since salary and location details sometimes appear late in a posting.

### Original hypothesis (kept for the record, now falsified)

**Hypothesis, strongly grounded but not yet confirmed.** `pkg/mcp/provider_ollama.go`'s `ollamaChatRequest` has **no `think` field**, and a repo-wide grep confirms `think` is never set on any request. Both configured models advertise the `thinking` capability (`ollama show qwen3:30b-instruct` → `Capabilities: tools, thinking, completion`). If Ollama defaults thinking **on** for these models, then every call in the pipeline generates a chain-of-thought block before its actual answer.

Why that would matter enormously here: generation on this CPU-only host was measured at roughly **1.6-1.8 tokens/sec** (see `defaultOllamaTimeoutMinutes`' comment). A few hundred reasoning tokens is therefore **several minutes of pure waste per call** — for `ScoreJob`, whose entire useful output is one integer.

Two pieces of circumstantial support:
- `ScoreJob` already carries a salvage path (`firstInt`) with the comment "Smaller local models sometimes wrap the number in prose instead of returning a bare integer" — exactly what leaked reasoning output looks like.
- The 4B benchmark is taking ~3.8 min/job for 5.5-14k-char prompts, far slower than a 4B dense model should need on 8 cores if it were emitting only a handful of tokens.

**If confirmed, this is likely a bigger win than the model swap, and it helps every call and both models.** Ollama accepts `"think": false` per request. Prime candidate for `ScoreJob` (bare integer), and worth measuring for `ExtractFormMapping`/`SolveValidationErrors` (JSON out) too.

**Verification blocked so far** only because `OLLAMA_MAX_LOADED_MODELS=1` plus a single-slot server (`-np 1`) means the benchmark monopolizes Ollama; a probe request queues behind it and timed out at 300s. Re-run the probe the moment the benchmark exits:
```bash
curl -s http://localhost:11434/api/chat -d '{"model":"qwen3:4b-instruct",
  "messages":[{"role":"user","content":"Reply with only the number 42."}],"stream":false}' \
| python3 -c "import json,sys; d=json.load(sys.stdin); m=d['message']; \
print('content:',repr(m.get('content','')[:200])); print('thinking:',repr(str(m.get('thinking',''))[:200])); \
print('eval_count:',d.get('eval_count'))"
```
`eval_count` is the decisive number: a bare "42" should be ~1-5 tokens. Anything in the hundreds means reasoning tokens are being generated and paid for.

## Benchmark status and a measurement trap worth remembering

**4B scores on the 3-job set (5,594 / 6,624 / 8,003 chars): 95, 85, 95.**

**The timings from that run are contaminated — do not cite them.** The same run reported 1s, 367s, 2s. A 1-second result for a ~1,400-token prompt is impossible on this CPU; those two jobs had been scored by the earlier (killed) 6-job run, so Ollama served them from a warm prompt cache. Only the 367s figure reflects real cold work. **Any future benchmark here must either use jobs never scored before or restart the Ollama server between runs**, or it will measure the cache instead of the model.

**Benchmark fidelity caveat:** `scorebench` passes the **full résumé text** as `parsedDocument`, whereas production passes `tailoredContext` (RAG top-5 chunks). So benchmark prompts run somewhat larger than live ones — the smallest job (5,594-char description) still produced a **13,388-char payload**, meaning roughly 7.8k chars of that is rubric + résumé + constraints, i.e. **fixed overhead independent of the posting**. Both models receive identical input, so the *score comparison* is unaffected; only absolute timings skew slightly pessimistic. Worth noting that this fixed overhead limits how much improvements.md #25's description trimming can save on short postings.

**RESULT (2026-07-25 ~02:05): the model swap FAILED validation and was rejected. `OLLAMA_FAST_MODEL` is deliberately left unset.**

| description | 4B | 30B | same decision? |
| --- | --- | --- | --- |
| 5,614 ch | 95 | 80 | yes |
| 6,624 ch | **85** | **0** | **NO** |
| 8,003 ch | 95 | 85 | yes |

- **Accuracy: 2/3 threshold agreement, failing in the worst direction.** The 30B's `0` was right — that posting (EDO, `job-boards.greenhouse.io/edo/jobs/5132798007`) states *"hybrid work policy of three days in the office"* and asks *"are you available to work on-site three days per week"*. With `remote_only: true`, rubric rule 2 takes 80 from the 80 baseline = 0. **The 4B missed it and said 85**, which would have applied to a job the candidate cannot take. It scored higher on all three (+15/+85/+10) — systematically lenient, and leniency means false-positive applications.
- **Speed: no gain at all.** Cold timings — 30B `421/358/420s`, 4B's one clean cold figure `367s`. Indistinguishable. **Reason:** `qwen3moe.expert_count = 128`, `expert_used_count = 8` — the incumbent is an MoE activating ~3B parameters per token, so a 4B *dense* model does *more* work per token, not less. **The premise of #24 does not hold on this hardware: there is no smaller-and-faster text model to swap to, because the model in use is already effectively 3B-active.**

Full write-up lives in `improvements.md` #24. The `fast` routing stays in the codebase, inert and tested, ready if a genuinely faster model ever appears.

## Work completed under this goal

- **`bugs.md` #64** (filed, fixed, pushed `6f0b8a5`) — the validation-retry bottleneck. See its Details section.
- **`improvements.md` #25** (filed, shipped, pushed `41cab70`) — trim over-long descriptions before scoring, middle-out so the rubric's salary/location rules keep their evidence.
- **Ruled out, with evidence, rather than filed as noise:**
  - *Reasoning tokens* — disproven by direct probe (`eval_count: 3`).
  - *"could not launch browser" (60 occurrences)* — all 60 fall inside a single two-hour window on 2026-07-16 (28 in hour 11, 32 in hour 12). That is the already-documented environment incident, not an ongoing defect.
  - *"Zero APPLIED ever"* — corrected above; it is the deliberate reset of the 82 cohort.

## 2026-07-25 ~10:30 — #64's fix worked, and immediately unmasked the real blocker (bugs.md #65)

The restarted run (PID `3520054`) ran **8h37m** and processed 19 jobs. Two things came out of it.

**#64 is confirmed working live:** `Narrowed validation retry to the rejected fields only (53366 -> 5363 chars)` — a 90% cut, and elsewhere `43033 -> 1200`. No job hit the 30-minute timeout again.

**But `FAILED_SUBMIT` jumped 5 → 23, and 18 of those shared one new cause:** "failed to submit application after 3 validation error attempts". Removing the timeout did not create this — it revealed what had always been happening *after* the fixes were applied, which no one could see while the call was dying first.

**Root cause, filed and fixed as bugs.md #65 (`3c2ac38`).** #64's own logging made it provable rather than speculative: payload sizes were **byte-identical between attempts 2 and 3** (Ethos `43033 -> 1200` twice, Point Wild `3057` twice), so the same fields were still invalid after each "fix". Two compounding defects — the loop **discarded `safeFill`'s error** (every failure silent), and `safeFill` is **`Fill()`-only**, which Playwright refuses on a `<select>`. Greenhouse forms make dropdowns required (work authorization, EEO, sponsorship), so those fields were unsatisfiable by construction. New `applyValidationFix` dispatches on the control's real shape (select → `SelectOption` by visible label then value; checkbox/radio → `Check`, with an explicit "No" mapping to `Uncheck` so a decline never becomes consent; else `Fill`), logs every failure, and fails fast when nothing could be applied.

**Run restarted 2026-07-25 10:34 as PID `3716166`** (`/tmp/career_agent_bin_verify82h`, HEAD `3c2ac38`). Requeued **17 rows** that failed purely on the now-fixed validation path (identified from the log; `applied_jobs` dedup rows cleared first or `HasApplied` would skip them) plus the orphaned `PROCESSING` row. The **6 remaining `FAILED_SUBMIT` were deliberately left alone** — 5 are the confirmed dead/expired postings, 1 is a non-validation Playwright timeout. Monitor `b6jugkzde` armed.

## Next Step

**Watch for the first genuine `APPLIED` under #65's fix.** That is the question the entire 82-job effort exists to answer, and nothing before now could have answered it: every large-form job was previously lost either to a timeout (#64) or to an unsatisfiable required dropdown (#65), one hidden behind the other.

If failures continue, read the **specific** new reason out of the log before concluding anything — this run has already produced two distinct dominant causes in sequence, each masked by the previous.

**Standing warnings:**
- **Monitor liveness — both existing checks can lie, so use the output file.** The task list has reported zero for monitors that were demonstrably alive (documented in the sibling journal), and today `ps aux | grep applied_urls_verify82` *also* returned nothing for monitor `b6jugkzde` while it was actively emitting status changes. **The authoritative check is reading the monitor's own output file** (`/tmp/.../tasks/<id>.output`): if it contains recent `STATUS CHANGE` lines, the monitor is alive regardless of what `ps` or the task list claim. Only re-arm when that file is stale *and* the run has genuinely moved on. Three monitors were also killed out from under this session today, so expect to re-arm after any interruption.
- **Benchmarking:** Ollama serves **warm prompt caches**. Use unseen jobs or restart the server, or you measure the cache — that produced 1s/2s readings here and a wrong "2.5x faster" conclusion.
- **`OLLAMA_FAST_MODEL` is intentionally unset** — see improvements.md #24 for the evidence the swap was rejected.

### Earlier close-out (goal items), superseded by the section above

**All four goal items are complete and pushed.**

- #24 validated and **rejected on evidence** — `OLLAMA_FAST_MODEL` intentionally unset; routing kept, inert and tested.
- bugs.md **#64** filed and fixed; improvements.md **#25** filed and shipped.
- 82-job run **restarted 2026-07-25 01:53 as PID `3520054`** (`/tmp/career_agent_bin_verify82g`, built from HEAD `e1013d5`) — the first run carrying #63 (fit scores persist), #64 (validation retries narrowed) and #25 (shorter scoring prompts). Confirmed healthy: "loaded 72 matching job(s)", RAG found 9 chunks. Monitor `btylz9pe4` armed. Reddit was requeued once more (orphaned `PROCESSING` from the benchmark kill; its `applied_jobs` dedup row was cleared first or `HasApplied` would have skipped it).
- Ongoing tracking of the 82-job effort itself continues in `2026-07-21_verify-bug4-iframe-fill-live-batch.md`, which is the journal to consult for that task. **This journal can be deleted once the restarted run is confirmed progressing.**

**Standing warning for the next benchmark:** Ollama serves warm prompt caches. Use unseen jobs or restart the server between runs, or you will measure the cache — it produced 1s/2s readings here and led to a wrong "2.5x faster" conclusion that cold measurements later overturned.