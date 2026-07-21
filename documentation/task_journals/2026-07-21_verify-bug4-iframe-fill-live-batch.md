# Task Journal: Verify bug #4 (iframe form-fill fix) via live cmd/agent batch run

## Summary

- **Task:** bugs.md #4 — AttemptSubmit form-fill logic never looked inside iframes. Fix applied 2026-07-20 (`resolveFillTarget` in `pkg/submitter/browser.go`), unverified live. Working this run to reach a genuine `APPLIED` status and close the Usability Gate's live-batch checkbox.
- **Status:** In progress
- **Started:** 2026-07-21
- **Agent and model:** Claude Code / Sonnet 5 (orchestrator only — no code changes expected unless the live run surfaces a new defect)

## Pre-Flight Re-Evaluation

- **Usability Gate check:** NOT MET (bugs.md). This is exactly the gate's own "Next action" item, so it's in scope ahead of anything in improvements.md.
- **Model choice:** No delegation needed — this task is running the existing built binary live and observing behavior, not writing new code. If the run surfaces a genuine new defect requiring a code fix, will delegate that fix per the Working Protocol (agy headless first, Ollama for small bits).
- **Skills routed:** none needed yet (no code changes planned); will route `defensive_debugging` / `quality_assurance` if a new failure needs diagnosis.
- **Code re-verified:** confirmed `resolveFillTarget` fix is present in `pkg/submitter/browser.go` per bugs.md #4 detail section (not re-read line by line yet — will do so if a new form-fill failure appears).

## Pre-existing state found at session start

- Found a long-running Claude session (PID 79952, alive since 2026-07-19) with background jobs (dashboard on :8080, `podman exec ... tail -f career_agent.log`) still attached to this repo. Investigated: no `cmd/agent`/`go run` process actually running inside the `career-agent` podman container, `ollama ps` showed zero loaded models, and `career_agent.log` inside the container hadn't advanced since 2026-07-20 22:49:35 — confirmed idle, not a live conflict. User confirmed "proceed anyway" after this check.
- Untracked `agent_run_batch.log`/`agent_run_batch.pid` in repo root: leftover from a run attempted directly on the **host** (not the `career-agent` container), which hit the already-documented/Resolved Playwright missing-library issue (`libicudata.so.74` etc.) and never got past the Playwright install-check. Not a new bug — just confirms the fix for that resolved bug is container-only and someone (or an earlier attempt) ran on host by mistake. Left the files in place (untracked, harmless); will not commit them.
- `auto_submit_click: true` and `headless_browser: true` in `profile.yaml` — confirmed with user via AskUserQuestion that a real live run (real submissions to real job postings) is authorized for this session before proceeding.
- `.env` has no `LLM_PROVIDER` or `OLLAMA_TIMEOUT_MINUTES` set, so both fall back to their fixed defaults (`ollama` / 1 worker, 45-minute timeout) from bugs.md #6's fix — correct, no override needed.

## Plan

- [ ] Launch `cmd/agent` live inside the `career-agent` podman container (correct env per the resolved host-vs-container bug), capturing full log output
- [ ] Monitor for the run to reach `AttemptSubmit` on a real job and specifically watch for iframe-embedded-form cases (SmartRecruiters-style)
- [ ] Confirm at least one job reaches genuine `APPLIED` status in `applications.db` / dashboard metrics
- [ ] If a new failure appears, diagnose root cause before assuming it's bug #4 again (per this file's own history, "looks like #4/#3 again" has been a wrong diagnosis more than once)
- [ ] Update bugs.md: close #4 if verified, update Usability Gate checklist, file any new findings
- [ ] Run `go build/vet/test ./...` if any code changed
- [ ] Commit, delete this journal, push

## Progress Log

- 2026-07-21 — Journal opened. Pre-flight checks complete (concurrent-session check, host secrets noted but not touched, auto-submit risk confirmed with user). About to launch the live batch.
- 2026-07-21 ~10:15 — Launched `go run cmd/agent/main.go` inside the `career-agent` podman container (correct Go 1.26.5 via `/usr/local/go/bin` prepend). Initial monitor watched the wrong file (stdout redirect only captures `fmt.Printf` lines; all SRE `log.Printf` output goes to a lumberjack-rotated `career_agent.log` set via `log.SetOutput` in `cmd/agent/main.go:40`) — corrected to tail `career_agent.log` instead.
- 2026-07-21 ~11:20-11:47 — Live run active and healthy: 24+ API calls, multiple jobs reaching `AttemptSubmit`. Several jobs correctly blocked by the prompt-injection security guardrail (working as intended, not a bug). Observed 5 consecutive `developer.workday.com` doc pages being scored/tailored/attempted — live reproduction of bugs.md #5.
- 2026-07-21 ~11:47-12:10 — Used the wait time productively: diagnosed #5's root cause in `pkg/scraper/funnel.go` (`isValidATSUrl` bare `"workday.com"` suffix match), delegated the fix to Gemini 3.1 Pro via `agy`, verified the real diff against `git diff` (matched brief exactly, no fabrication), ran `go build/vet/test ./...` (all pass), updated `bugs.md` (moved #5's Workday/Workable portion to Resolved, filed remaining Greenhouse case as new #7), committed as `069ed98`. This did not touch the still-running live batch (separately compiled `go run` process in the container, unaffected by source edits on host).
- Still watching for bug #4: no iframe-embedded SmartRecruiters case or genuine `APPLIED` status reached yet in this run.

## Progress Log (continued)

- 2026-07-21 ~12:30-13:00 — SmartRecruiters case reached in the live run (`sosi1`), the exact ATS bug #4 was diagnosed against. Failed the same `failed to fill first_name` way. Investigated: no `using embedded iframe` log line fired, and `resolveFillTarget` runs *minutes* after page load (after the 5-10 min doc-generation call), ruling out a slow-render race. Direct `curl` inspection of both this SmartRecruiters posting and the earlier Breezy case both showed zero `<form>`/`<iframe>` tags and one unrelated stray `<input>`. Root-caused as a new, distinct bug: forms gated behind a click-to-reveal "Apply" interaction that nothing in the code handles. Filed as bugs.md #8 (commit `510e0a9`), broadened with the second confirmed case (commit `a2c4e55`). Bug #4 remains genuinely unverified — no case has exercised the iframe-fallback path at all this session, but nothing contradicts the fix either.
- 2026-07-21 ~13:00 — User confirmed pivoting to fix #8 now (best-evidenced path to an actual `APPLIED`). Wrote delegation brief, sent to agy/Gemini 3.1 Pro — hit an account-wide quota error before writing anything (git status stayed clean). Retried with agy/GPT-OSS 120B — returned exit 0 with a plausible summary, but `git diff` showed a broken edit (duplicated `resolveFillTarget` body, stray extra `}`, would not compile). Reverted (`git checkout --`) and applied the fix directly: added `clickApplyIfPresent(page)` in `pkg/submitter/browser.go`, called once in the Learner Module branch before DOM capture/fill. Added two unit tests in `browser_test.go`, working around a real Go embedding gotcha (`playwright.Locator`'s own `Locator(...)` method collides with an anonymously-embedded field of the same name — fixed via a type-alias embed). `go build/vet/test ./...` all pass. Committed `5afa230`, docs update `0023132`.
- 2026-07-21 ~13:15 — Killed the old (pre-fix) `go run cmd/agent/main.go` process in the `career-agent` container and relaunched with the fix compiled in (new PID). Re-armed the log monitor (the first one was collaterally killed by the `pkill -f` pattern matching its own script text — used a safer process-check pattern this time). Watching for the fix's effect on the next Learner-Module-routed job.
- Also noted but not yet investigated: a `jobs.smartrecruiters.com?keyword=remote` bare-domain search-query URL slipped through as a "job" (another FunnelEngine false positive, SmartRecruiters this time — candidate for #7's scope or a new row), and one Learner Module `failed to parse mapping json: invalid character 'T'` error (LLM returned non-JSON prose instead of the expected mapping JSON — a different, not-yet-filed issue). Worth filing before this session ends if time allows.

## Progress Log (continued 2)

- 2026-07-21 ~13:18-14:15 — Post-#8-fix run's next Learner Module case (Jobvite, `dwt`) still failed the same way, and `clickApplyIfPresent` never fired (no "Clicked an Apply" log line) — meaning it wasn't a #8 case at all. User confirmed writing a proper standalone diagnostic script (same method that cracked #4 originally). Wrote `tmp_diag/main.go` (temporary, deleted before commit, never tracked) — headless Playwright, same launch args/version as the app, navigates the exact failing URL, waits past networkidle + a settle period, dumps input/form/iframe/Apply-text-element counts + a screenshot. Ran it inside the `career-agent` container against `https://jobs.jobvite.com/dwt/job/o79Qzfwp/apply`: the job had expired and redirected to a `?error=404` page. Root-caused as a third, distinct, simple bug: the existing dead-job-phrase guard's exact-string match didn't cover this ATS's wording. Fixed directly (extracted `isDeadJobPage`, widened phrase list, added `TestIsDeadJobPage`), committed `ac12630`. Filed and resolved as bugs.md #9 (commit `797923d`), which also corrected #8's entry — the Jobvite case had been wrongly attributed to #8; #8's actual click-to-reveal fix still has no live evidence either way beyond the two pre-fix cases that motivated it.
- 2026-07-21 ~14:20 — Relaunched the live batch a third time with both #8 and #9 fixes compiled in (killed PID 876014's predecessor via `podman exec --user root ... kill -9`, needed `--user root` since the default exec session lacked permission to signal the root-owned process). Re-armed log monitor.

## Progress Log (continued 3)

- 2026-07-21 ~14:24-15:00 — With #8+#9 live: `clickApplyIfPresent` fired correctly on two real cases (Pinpoint/`hazelcast`, Breezy/`mind-computing`) — both then hit the prompt-injection guardrail on the revealed content, an expected security block, not a bug. Confirms the click mechanism itself works. Also hit: a Jobvite `/search` listing-page false positive (`cloudone-digital`, filed as #11) and a recurring `UNIQUE constraint failed: applied_jobs.url` on the same URL processed twice, seen on two different jobs now (filed as #12).
- 2026-07-21 ~15:05 — User asked for other approaches given the whack-a-mole pattern (3 distinct root causes behind the same symptom this session). Recommended and got approval for a structural fix: extend the existing but under-triggered Vision fallback (`AttemptVisionSubmit`, screenshot + `qwen2.5vl:7b` visual reasoning, already fully wired) to also fire on a DOM-mapped *fill* failure, not just an outright mapping-generation failure — since every failure diagnosed this session was the former. Implemented in `pkg/submitter/browser.go` (both the cached-mapping and fresh-Learner-Module-mapping paths). `go build/vet/test ./...` pass. Filed as bugs.md #10. Committed `9e8139b` (code) and `fae67b1` (docs, also filed #11/#12).
- 2026-07-21 ~15:08 — Relaunched the batch a fourth time (PID 922666) with #8+#9+#10 all compiled in. Re-armed monitor watching additionally for `Vision-Submit`/`Falling back to Vision`/`UNIQUE constraint` lines.

## Next Step

Watch for a `[Vision-Submit]`/`Falling back to Vision` log line firing for the first time (would verify #10 actually engages) and whether it leads to a better outcome than the plain DOM-mapping path did. Still ultimately looking for a genuine `APPLIED` status, which would also incidentally verify #4 (any successful fill through the Learner Module path exercises `resolveFillTarget`). #11 (Jobvite search pages) and #12 (duplicate-processing race) remain open, not yet root-caused with full rigor — good next candidates if this run stalls again without a new dominant pattern.
