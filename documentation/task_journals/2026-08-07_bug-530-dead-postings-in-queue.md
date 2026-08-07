# Task Journal: bugs.md #530 — dead postings still occupy a queue card

## Summary

- **Task:** bugs.md #530 — "A posting that has died still occupies a queue card, because the assisted queue never reads the funnel status it selects" (Minor, score 1.5, Tier `standard`)
- **Status:** In progress
- **Started:** 2026-08-07
- **Agent and model:** Claude Code / Opus 5

## Pre-Flight Re-Evaluation

- **Usability Gate check:** gate is `MET (2026-08-06)`. #530 is the highest-scoring open item that is not blocked (#524 ties at 1.5 but cannot proceed until Workable's rate-limit block expires ~2026-08-08 03:20 UTC). User named #530 explicitly.
- **Model choice:** kept in this session. The change is small, but it sits on the projection that decides what the operator is invited to click, next to the confirm path that writes an irreversible `APPLIED` record — #518 and #521 are both bugs in exactly this area. Not delegated.
- **Skills routed:** `software_development`, `database_management`, `quality_assurance`, `defensive_debugging`.
- **Code re-verified (2026-08-07):** every claim in the row still holds.
  - `GetAssistedQueue` (`pkg/storage/assisted.go:682`) filters on `aa.assisted_state != 'completed'` and nothing else.
  - It selects `COALESCE(jf.status, '')` and scans it into `currentStatus` (`:715`), which is never read again.
  - `serveAssistedQueue` (`cmd/dashboard/main.go:927`) is the only caller and applies no further filter.
  - Live: 18 queued rows are `INVALID_URL`/`expired` and all 18 are still served by `/api/assisted`.

### What re-evaluation added, beyond confirming the row

1. **The confirm path already enforces the rule the queue is missing.** `ConfirmAssistedSubmission` (`:800`) refuses when `status != original || !isAssistedEligibleStatus(status)`. So clicking Confirm on one of these dead cards already fails safely with "refusing to overwrite newer job status" — there is no corruption risk, only wasted operator effort. **This means the fix should reuse `eligibleAssistedStatuses` rather than invent a new list of terminal statuses.** The queue simply does not apply the rule the write path already applies.
2. **The invariant holds during assisted work.** The only `job_funnel.status` write anywhere on the assisted path is the `APPLIED` update inside `ConfirmAssistedSubmission`, which is in the same transaction as `assisted_state = 'completed'`. So `jf.status` stays equal to `original_status` — one of the eligible three — for the whole life of an in-flight row. Filtering on the eligible set therefore cannot hide work that is genuinely in progress.
3. **A second dead read is present in the same scan.** `job.Interruption` is scanned twice — position 7 (`COALESCE(jf.status_reason, '')`) and position 11 (`aa.interruption_reason`) — so the funnel's status reason is read and then silently overwritten. Same smell as the one #530 is about. Noted, and in scope only so far as not making it worse.

## Plan

- [ ] Filter `GetAssistedQueue` on `isAssistedEligibleStatus`, deriving the SQL `IN` list from `eligibleAssistedStatuses` so there is one source of truth.
- [ ] Remove the now-provably-dead `jf.status` select/scan, carefully, keeping the scan aligned.
- [ ] Tests: a dead row is excluded, an eligible row is kept, and the exclusion is driven by funnel status rather than assisted state.
- [ ] Mutation-check the new tests.
- [ ] `go build` / `go vet` / `go test ./...` / `gofmt -l`.
- [ ] Verify live against the real `applications.db`: the 18 `INVALID_URL` rows disappear from `/api/assisted` and the other 506 remain.

## Progress Log

- 2026-08-07 — Re-verified all of #530's claims against current code and the live database; all hold. Found the confirm path already enforces the missing rule, which settles the design question the row left open (reuse `eligibleAssistedStatuses`, do not invent a terminal-status list).

## Next Step

Implement the filter in `GetAssistedQueue`.
