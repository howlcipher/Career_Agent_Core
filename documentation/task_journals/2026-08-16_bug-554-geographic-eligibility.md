# Task Journal: bugs.md #554 — Geographic eligibility end to end

**Date:** 2026-08-16
**Branch:** fix/554-geographic-eligibility
**Status:** Implementation complete, tests pass, live DB cleanup applied, ready for PR.

## Handoff recovery

Claude Code had an in-flight branch `fix/554-geographic-eligibility` based on `main` at `54417a8`. The branch contained the bulk of the geography fix but had no `bugs.md` row and the live DB cleanup had not been run. Work retained; only minor report-math and defense-in-depth fixes were added.

## Root cause

- `config.IsEligibleJob` combined title + remote eligibility but did not include geography.
- `ReconcileAssistedQueueEligibility` only re-checked role + remote.
- `GetAssistedLaunchInfo` and `PromoteJobToAssisted` did the same.
- `EnsureAssistedPlanForURL` created actionable assisted rows for interrupted pipeline jobs without re-checking the current policy.
- A missing `profile.yaml` was treated as permission at launch (`if err == nil`).
- Unknown geography was admitted rather than held.

## Fix summary

- Added `config.GeographyEligible` / `config.ScreenJob` with three outcomes: allowed, outside, unknown.
- Canonical rule: `title eligible AND remote eligible AND geography eligible`.
- Wired the gate into discovery, reconciliation, launch, manual promotion, and `EnsureAssistedPlanForURL`.
- Added dashboard geography selector with explicit ISO-code persistence and named presets.
- Made every policy load failure a refusal, not a pass.

## Live verification

- Dry-run of 211 actionable assisted rows:
  - US eligible: 76
  - Canada eligible: 9
  - Outside scope: 106
  - Unknown geography: 20
  - Remaining: 85
- Applied with `go run ./cmd/pruneassisted`.
- `webook` / "Agentic AI Engineer" / Amman / B73794CB19 is now `SKIPPED` / `outside_allowed_countries`, assisted row removed.
- Zero APPLIED rows touched.

## Tests

- `go test ./... -count=1` passes.
- `gofmt -l ./cmd ./pkg ./internal` clean.
- Frontend lint, test and build pass.
- `internal/backlog` validates the new bugs.md row.

## Merge

- PR #29 opened and merged at `cd549d1b9d6cc6df2dcd531ca7d5092c6c517a30`.
- `main` fast-forwarded to the merge commit.
- bugs.md #554 moved to Done; full account archived in `bugs_done_details.md`.
