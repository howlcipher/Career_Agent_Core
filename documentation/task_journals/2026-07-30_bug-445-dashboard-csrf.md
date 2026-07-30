# Task Journal: Bug #445 — any web page open in your browser can start or stop the agent

## Summary

- **Task:** bugs.md #445 (Major, score 3.5) — `serveAgentStart`/`serveAgentStop` validate only `r.Method`, so a cross-origin CORS simple request from any tab can launch or kill the agent.
- **Status:** In progress
- **Started:** 2026-07-30
- **Agent and model:** Claude Code / Opus 5 orchestrating; implementation delegated to Claude subagents at the user's explicit instruction ("utilize multiple agents… prioritize claude models on this run"), which overrides the Working Protocol's default non-Claude delegation.

## Pre-Flight Re-Evaluation

- **Usability Gate check:** UNMET. #445 is the sole open Major and the only item holding the zero-Blocker/Major box open, so it correctly outranks everything in `improvements.md`.
- **Model choice:** Claude, per the user's explicit session instruction. Multiple Claude subagents in parallel: one implementing the fix, one auditing README/GitHub Pages for stale information.
- **Skills routed:** `cyber_security` (zero-trust: never trust by default regardless of whether a request originates internally or externally — exactly this defect), `software_development`, `quality_assurance`.
- **Code re-verified:** Confirmed against current code, not the row's prose. `grep -n "Origin\|Sec-Fetch\|csrf\|CSRF" cmd/dashboard/main.go` → **zero matches**. `serveAgentStart` (`cmd/dashboard/main.go:514`) checks `r.Method != http.MethodPost` and nothing else before `exec.Command("./career_agent_bin", "-daemon", "-cycle-limit", "5")`. `serveAgentStop` (`:536`) same shape before `pkill -f career_agent_bin`. `serveAgentStatus` (`:547`) checks nothing at all. Routes registered at `:265-267`. The row is accurate.

## Plan

- [ ] Add a same-origin guard for the state-changing agent endpoints (`Sec-Fetch-Site` primary, `Origin`/`Referer` allowlist fallback, host-matched against the listen address).
- [ ] Tests covering: same-origin allowed, cross-site rejected, missing headers on a legacy client, `GET` still rejected, and the status endpoint's read-only behaviour.
- [ ] Review the full diff, run `go build ./...`, `go vet ./...`, `go test ./...`.
- [ ] Update README / `docs/` GitHub Pages where the dashboard's security posture is documented.
- [ ] Groom all three backlogs, commit, push.

## Progress Log

- 2026-07-30 — Journal opened. #445 selected as the highest-priority open item (sole open Major; #446 outscores it at 4.0 but is Minor and does not gate). Re-verified the defect against live code before writing any brief.

## Next Step

Dispatch the implementation subagent with a self-contained brief for the same-origin guard plus tests, and in parallel a read-only docs-audit subagent over README.md and docs/.
