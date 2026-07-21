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

## Next Step

Keep monitoring `career_agent.log` in the `career-agent` container for a genuine `APPLIED` status (verifies #4) or a new iframe-related failure. If the run stalls or exits without reaching one, decide whether to relaunch or extend.
