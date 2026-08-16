# Task Journal: Assisted Fill known-field coverage regression after #555

## Summary

- **Task:** Production regression investigation after bugs.md #555 — operator reports two real Assisted applications had résumé attached but ordinary profile fields (name, address, EEO) remained blank.
- **Status:** Checkpointed investigation; implementation not started
- **Started:** 2026-08-16
- **Agent and model:** Claude Code

## Pre-Flight Re-Evaluation

- **Usability Gate check:** The gate in bugs.md is unmet (Major #551 remains open), but this task is explicitly a production regression investigation, not an improvement.
- **Model choice:** Claude Code (orchestration), may delegate mechanical edits to local Ollama if needed.
- **Skills routed:** defensive_debugging, software_development, test_and_verify.
- **Code re-verified:** Current `origin/main` is `b943852` "fix(bugs.md #555): make Assisted Fill reliably load the operator profile (#31)". The running dashboard binary's build info reports module version `v0.0.0-20260816175010-6fd8a2a20f72+dirty`, i.e. built from a dirty tree at commit `6fd8a2a` (docs commit for #554) with uncommitted changes. Symbols `main.findWorkspaceRoot` and `main.findGoModuleRoot` are present, so the binary appears to contain the #555 working-tree code. Binary mtimes are 2026-08-16 13:57:23-25 local; #555 commit was 2026-08-16 14:01:49 local.

## Plan

- [ ] Identify the two real Assisted applications completed after #555.
- [ ] Trace Continue path and fill report for each.
- [ ] Confirm running binary provenance and whether stale binaries explain the failure.
- [ ] Inventory "Your Details" fields and their path to browser fill.
- [ ] Inventory "What Career Agent Core Already Knows" sources and their path to browser fill.
- [ ] Build field-by-field acceptance matrix for both real applications.
- [ ] Audit cross-source precedence and EEO policy behavior.
- [ ] Build synthetic form and reproduce failure through real production code.
- [ ] Determine root cause and file new bugs.md row.
- [ ] Implement fix.
- [ ] Synthetic and real non-submit verification.
- [ ] Independent reviewers.
- [ ] Full verification loop (gofmt, build, vet, tests).
- [ ] Commit, push, PR, merge.

## Progress Log

- 2026-08-16 16:25 — Checked out `origin/main` at `b943852`. Confirmed current workspace matches.
- 2026-08-16 16:25 — Read bugs.md, improvements.md, CHANGELOG, ADR-005/006/007, and full histories for #519, #543, #546, #548, #549, #555.
- 2026-08-16 16:28 — Queried live database. Most recent Assisted applications with `assisted_state='completed'` and `status='APPLIED'`:
  - 307795 (affirm, Senior Product Manager, Agent Enablement, Greenhouse) — APPLIED 2026-08-16 20:19:23 UTC
  - 304813 (veeva, Principal DevOps Engineer, Lever) — APPLIED 2026-08-16 01:14:36 UTC
  - 304777 (thinkahead, Technical Consultant - Enterprise Network, Lever) — APPLIED 2026-08-16 01:12:51 UTC
  Candidate primary reproductions: 307795 and 304813 (the two most recent completed Assisted applications).
- 2026-08-16 16:30 — Running dashboard process: PID 1522828, binary `/var/home/howlcipher/dev/Career_Agent_Core/career_dashboard_bin`, cwd repo root, stdout/stderr to `/tmp/career_dashboard.log`. No `career_assist_bin` currently running. `career_agent_bin` not running (daemon appears absent; stale `/tmp/fake_agent_test` processes present from Aug 11).
- 2026-08-16 16:31 — `/tmp/career_dashboard.log` shows two Assisted browser open/close cycles today at 16:16 and 16:17 local (20:16-20:19 UTC), both opened and closed without Continue. Database shows 307795 updated at 20:19 UTC, matching the second session's close.
- 2026-08-16 16:45 — Rechecked the live database without printing PII. Recent completed rows have no `fill_attempted_at`; candidate rows 307795 and 304813 have zero recorded filled fields and zero reused answers. The dashboard log contains no Continue/refill event for either browser session. Therefore these records do not prove a browser-fill landing defect: the refill path was never reached.
- 2026-08-16 16:47 — Source trace: dashboard Continue writes `assisted_state=continue_requested`; the assist loop then calls `continueAssistedApplication`, loads documents plus the workspace-resolved `pii.yaml`, opens the Answer Vault, and calls `FillAssistedMappedPage`. That function runs the dedicated ATS handler, snapshots remaining controls, and resolves routine configured facts and reusable approved answers through `answers.ResolveAll`; sensitive values remain in `NeedsOperator`. No source change made because the observed records stop before this path.
- 2026-08-16 16:50 — `go test ./pkg/submitter ./cmd/assist ./cmd/dashboard` passed.

## Root Cause / Handoff

- Proven breakpoint: the two inspected browser sessions were closed before Continue; no fill attempt was recorded. This is an execution/workflow or stale-runtime observation, not proof that `FillAssistedMappedPage` failed to populate controls.
- Not disproven globally: a later session that actually reaches Continue could still expose field-coverage or control-matching gaps. The existing source path for ordinary remaining fields and approved reusable answers should be exercised with a synthetic DOM before changing it.
- Stale-runtime risk remains: the running dashboard binary reports a build from `6fd8a2a+dirty`, predating the committed #555 SHA, although the journal indicates the dirty build may have contained the #555 edits. Rebuild both dashboard and assist from current `b943852` before the next real application.
- Safe next action: build/restart the current dashboard and `career_assist_bin`, run one Assisted application, click Continue, and capture only state transitions, counts, labels, and sanitized error reasons. Keep Assisted applications paused until that controlled Continue/refill run produces a nonzero attempt marker.
- No PII, answer values, or secrets are included here.
