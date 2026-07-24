# Task Journal: improvements.md #15 — Email/portal conversion-rate analytics

## Summary

- **Task:** improvements.md #15 — surface an interview conversion rate (interviews ÷ applications sent), overall and broken down by ATS platform, computed from `applications.db`. `pkg/tracker/imap.go` already writes `REJECTED`/`INTERVIEW_REQUESTED` transitions on top of `APPLIED` job_funnel rows; nothing aggregates them yet.
- **Status:** In progress
- **Started:** 2026-07-24
- **Agent and model:** Claude Code / Sonnet 5 (orchestrator), delegating implementation to Antigravity CLI / `gemini-3.1-pro-high`

## Pre-Flight Re-Evaluation

- **Usability Gate check:** MET (2026-07-24, see `bugs.md`). `improvements.md` rows are fair game for normal ROI-ranked selection. Only open bug is #48 (Minor, not yet root-caused, doesn't block this).
- **Model choice:** `agy models` confirms `gemini-3.1-pro-high` is live (matches item #15's Gemini-model column). Delegating implementation to preserve Claude session limits per the Working Protocol; this session (Sonnet 5) retains selection, re-evaluation, review of the diff, and verification/commit.
- **Skills routed:** `software_development` (general Go feature work), `database_management` (new aggregate query design) from `../ai_knowledge_library/.agents/skills/`.
- **Code re-verified 2026-07-24:** confirmed no existing conversion-rate code anywhere (`grep -rn "ConversionRate\|conversion_rate"` → zero hits). `pkg/tracker/imap.go` only ever writes `REJECTED` or `INTERVIEW_REQUESTED` (never a distinct `OFFER` status) and only forwards rows currently `APPLIED` (`UPDATE job_funnel SET status = ? WHERE company_name = ? AND status = 'APPLIED'`), so "ever applied" = `status IN ('APPLIED','REJECTED','INTERVIEW_REQUESTED')`.

## Re-scoping (per Working Protocol step 5)

The original item description says "broken down by role/source/ATS/tone." Re-scoped down before delegating:
- **Source/ATS**: kept — derivable from the URL via a CASE expression, same pattern as `sourcePriorityCASE` in `pkg/storage/manager.go`.
- **Role**: dropped — `job_title` is free text, not a categorical field; grouping by it would be noise, not signal.
- **Tone**: dropped — `CoverLetterTone` (`pkg/config/profile.go`) is a single global config value, not a per-application variant, so there is currently nothing to break a rate down *by*. This becomes meaningful once item #13 (A/B testing) exists, which explicitly depends on this item shipping first.

## Plan

- [ ] `pkg/storage/manager.go`: add `ConversionStats` struct (TotalApplied, Interviews, Rejections, Pending, InterviewRatePct) + `GetConversionStats()` (overall) and `SourceConversionStat` + `GetConversionStatsBySource()` (grouped by an ATS-label CASE: Greenhouse, Lever, Workday, SmartRecruiters, Ashby, Other).
- [ ] `pkg/storage/manager_test.go`: tests for both, mirroring `TestSourceOutcomeBreakdown`'s style (build rows via `AddToFunnel`/`UpdateFunnelStatus`, assert counts).
- [ ] `cmd/dashboard/main.go`: extend `Metrics` struct and `serveMetrics` with the new overall stats + a `BySource []storage.SourceConversionStat` field.
- [ ] `cmd/dashboard/index.html`: new stat card ("Interview Rate") in the existing `.grid`, plus a small "Conversion by Platform" table below it.
- [ ] `go build ./...`, `go vet ./...`, `go test ./...` all clean.
- [ ] Rebuild dashboard binary, verify live against the real `applications.db`, screenshot via Firefox (Chromium is broken on this box — see `bugs.md`'s Operational Trap notes).
- [ ] Update `improvements.md`: mark #15 Done with a note, including the role/tone re-scoping above.
- [ ] Commit, push, delete this journal.

## Progress Log

- 2026-07-24 ~16:10 — Journal opened, re-scoping decided, delegation brief written, handing off to `agy`.
- 2026-07-24 ~16:15 — All three Gemini tiers (`gemini-3.1-pro-high`, `gemini-3.1-pro-low`, `gemini-3.6-flash-high`) returned "Individual quota reached... Resets in 94h" — quota is fully exhausted for ~4 days, not per-tier as the Working Protocol assumed. Stepped to a different provider per protocol ("on a quota error step to another tier or provider"): `gpt-oss-120b-medium` responds live. Launched the same brief (unchanged) against it as a background task (`be8bphmsf`, `agy ... --model gpt-oss-120b-medium --mode accept-edits --print-timeout 30m`), running in the `/var/home/howlcipher/dev/Career_Agent_Core` working tree directly (not a worktree) — git status was clean (only the known untracked `applied_urls_verify82.txt`) before launch, so its diff is exactly attributable. **Record for future sessions: Gemini quota (all tiers) exhausted 2026-07-24 ~16:15, resets ~2026-07-28 ~14:30. Don't retry Gemini tiers before then — go straight to gpt-oss or local Ollama.**

## Next Step

Delegate `be8bphmsf` is running in the background. When it completes: review the full `git diff` against the Task 1-4 spec above (especially Task 3's DB-handle nil-mismatch risk — confirm it actually verified live data, not just that it compiled), run `go build ./...`, `go vet ./...`, `go test ./...` myself, fix small gaps directly or re-delegate with concrete feedback, then verify the dashboard live (rebuild binary, screenshot via Firefox per the Chromium-text-rendering note in `bugs.md`'s Operational Trap section), update `improvements.md` row #15 to Done with the re-scoping note above, commit, push, delete this journal.
