# Task Journal: Throughput audit, feed discovery, and discovery-quality bugs

## Summary

- **Task:** User asked (a) to fix any free bugs/improvements and push, and (b) to consider two proposals: XML/feed-based discovery, and a local Model Context Protocol server. Answering "is anything free actually worth doing?" turned into a throughput audit that produced two more real bugs.
- **Status:** Complete — all work committed and pushed. Nothing in flight except the live run.
- **Started / finished:** 2026-07-25
- **Agent and model:** Claude Code / Opus 5

## The governing fact found in this session

**3,131 jobs sit in `DISCOVERED` at ~10 minutes each on a single worker — roughly 22 days of continuous compute to clear once.** Throughput, not any individual defect, is now the binding constraint on getting real applications out. Worth keeping in view when judging what is worth building: anything that removes wasted scoring cycles beats almost anything that adds capability.

Note that raising `WORKER_COUNT` does **not** obviously help: `llama-server` runs with `-np 1` (a single slot) and `OLLAMA_MAX_LOADED_MODELS=1`, so every LLM call serialises regardless of how many workers exist. Extra workers would only parallelise browser/IO work, not the scoring that dominates. Do not assume concurrency is the fix without measuring.

## Shipped this session

- **improvements.md #26 — Greenhouse/Lever board-feed discovery** (`4134b3d`). Live-verified before building: 238 postings for `remotecom`, 287 for `palantir`, from one unauthenticated call each. 605 company slugs were already recoverable from URLs the funnel had collected. Includes `titleLooksRelevant`, without which a feed's full board (accountants included) would have been a denial-of-service on the queue at ~10 min per score.
- **bugs.md #69 — discovery stored the searched role as `job_title`** (`2972797`). 55 distinct titles across 3,131 rows. Also degraded #22's queue ranking, which embeds title+company.
- Backlog table re-ordered so Pending rows are genuinely score-ranked (`10ec721`).

## Assessment of the two proposals (do not re-litigate without reading this)

**Feed discovery — built, but one premise was wrong.** Discovery here never used browser automation; it is search dorking over plain HTTP, and Playwright is used only for submission. So the proposed compute saving did not apply, and no Python component was needed (this is Go with `playwright-go`). The idea was still worth doing for a *different* reason — feed completeness and quality, against 301 `INVALID_URL` rows out of 3,884.

**Local MCP server — filed as #27, below the ROI floor (0.33), not built.** The capability described already ships: grounded screening-question answers via improvements #16, and résumé-fact retrieval via the `career_chunks` RAG layer. MCP would add an interop layer over something already working in-process. **`pkg/mcp` is not Model Context Protocol** — it is this repo's LLM-provider abstraction and predates the name clash; there is no JSON-RPC or MCP wire format anywhere in the repo. Reconsider only if the user starts driving this data from an external MCP client.

## Backlog state at close

Zero open bugs. Three Pending improvements, **none of them free and actionable**:
- #17 CAPTCHA solving (1.25) — needs a paid key.
- #14 LoRA fine-tuning (0.43 ⚠️) — no discrete GPU; would need paid cloud compute.
- #27 MCP server (0.33 ⚠️) — duplicates shipped capability.

## Next Step

**Watch the live run for the first genuine `APPLIED`.** PID `3755906` (`/tmp/career_agent_bin_verify82j`, HEAD `81792b0`), monitor `b03wmuerf`. It carries bugs #63-#68 and is the first run with none of #64/#65/#66/#67 in the way.

**When it next needs a restart**, rebuild from current HEAD to pick up #69 and #26 as well.

**Feed discovery has not run live yet.** The current run is an isolated `TARGET_JOB_URL` job that skips `FunnelEngine.DiscoverJobs` entirely, so `discoverWithATSFeeds` only executes on the next *normal* batch start. Watch its first pass for the log line `ATS board feeds contributed N new posting(s)` and sanity-check that the title gate is not over-filtering.

**Standing warnings (learned the hard way today):**
- **Monitor liveness:** both the task list *and* `ps aux | grep applied_urls_verify82` have given false negatives. The authoritative check is reading the monitor's own `tasks/<id>.output` file for recent `STATUS CHANGE` lines.
- **Benchmarking:** Ollama serves warm prompt caches. Use unseen jobs or restart the server, or you measure the cache — it produced 1s/2s readings and a wrong "2.5x faster" conclusion earlier today.
- **`OLLAMA_FAST_MODEL` is intentionally unset.** See improvements.md #24: the 4B measured *no faster* and scored an on-site/hybrid role 85 where the 30B correctly gave 0.
