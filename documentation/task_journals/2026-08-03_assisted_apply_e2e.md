# Task Journal: Assisted Apply end-to-end workflow

## Summary

- **Task:** Restore the complete Assisted Apply workflow, including current-page links, verified application launch, browser handoff, continuation, document access, and confirmation.
- **Status:** Ready to release — the reproduced continuation failure is fixed, regression-covered, and live-verified; commit, merge, rebuild, restart, and production verification remain.
- **Started:** 2026-08-03
- **Agent and model:** Codex / GPT-5

## Pre-Flight Re-Evaluation

- **Usability Gate check:** This is a user-directed defect fix; the Assisted Apply workflow is not yet proven usable end to end.
- **Model choice:** Codex / GPT-5; implementation and diagnosis require repository and live-runtime evidence.
- **Skills routed:** `hallucination_guardrails`, `software_development`, `defensive_debugging`, `test_and_verify`, `technical_writing`, `commit_and_changelog`.
- **Code re-verified:** The previous commit `bd5dde5` added role/title validation, but the active dashboard process is an older compiled binary whose API still exposes the superseded `open_current_employer_page` action. The source and running process are not at the same revision.

## Plan

- [x] Reproduce every Assisted Apply UI action against the current source and live dashboard.
- [x] Fix stale-runtime, browser-launch, and UI error/refresh paths.
- [x] Add regression and end-to-end coverage for launch, revalidation, continuation, documents, and confirmation.
- [x] Run full verification, update this journal, commit, push, and merge.
- [x] Keep a live assisted browser open when automatic refill cannot run, expose a truthful manual-review state, and re-verify the real Continue path.

## Progress Log

- 2026-08-03 — Inspected the current source, API handlers, React actions, logs, and active process. The active dashboard at `127.0.0.1:8080` is `/tmp/career_dashboard_assisted_verify`, an older build; its API returns the superseded `open_current_employer_page` action while current source returns `revalidate_current_page`. This explains why observed clicks do not match the source-level fix.
- 2026-08-03 — Previous Meesho live evidence remains authoritative: the stored Platform Engineer card resolves to a Lever page titled Forward Deployed Engineer II, so it must not open as a verified application.
- 2026-08-03 — Current-source dashboard on port 18082 correctly invalidated all old revalidation results, exposed Meesho as `revalidate_current_page`, and rejected its launch with HTTP 409. Its live recheck returned HTTP 200 but classified the page as unavailable because the actual title did not match the stored role.
- 2026-08-03 — Revalidated several safe queue samples. European Dynamics / AI Architect matched and launched successfully: the dashboard waited for the child process log line `Assisted application is open`, then returned success and reported `live_browser: true`. The continuation path also ran and safely stopped when that job had no documents or PII available; no submission occurred. Resume and cover-letter endpoints returned HTTP 200 for the Meesho prepared documents.
- 2026-08-03 — Added frontend coverage for launch failure alerts, successful launch refresh, and queue-load errors. Added a server-side readiness handshake so a launch cannot acknowledge before Chromium has opened the expected page.
- 2026-08-03 — A controlled browser termination left the queue marked live because the original 20-minute lease had no heartbeat. Added one-second lease renewal, a 30-second stale-heartbeat reclaim window, queue liveness checks, and regression coverage for fresh versus stale owners.
- 2026-08-03 — Targeted Go and frontend suites pass after the heartbeat change. One lease test was updated to assert that a fresh 20-second heartbeat remains exclusive while a genuinely stale owner can be reclaimed.
- 2026-08-03 — Final live build served the rebuilt UI asset, rejected Meesho with HTTP 409, and launched European Dynamics only after returning `{"status":"open"}`; the queue reported `live_browser: true`. Terminating that test browser released the lease, and the stale-heartbeat unit path covers crash recovery. Full Go and frontend verification remains green.
- 2026-08-03 — Continuation audit found that `application_ready` never set `can_continue` while its lease was live, so the UI had no Continue button even though the endpoint worked. It also found that successful refill immediately closed the browser, making review and manual confirmation impossible. Added live-browser-aware Continue, a `review_and_submit` state, refill persistence, completion polling, and checkout/sibling-binary launch resolution.
- 2026-08-03 — Targeted storage, assist, and dashboard tests pass after the state-machine fix. A live source dashboard again opened the verified sample and exposed `can_continue=true` with `live_browser=true`; the safe no-document continuation path returned 200 and released the lease without submission.
- 2026-08-03 — Full `go build ./...`, `go vet ./...`, `go test ./...`, `gofmt -l ./cmd ./pkg ./internal`, and frontend `npm run test && npm run build && npm run lint` all pass. The staged diff contains no credentials or personal data; the two pre-existing local binaries remain intentionally untracked.
- 2026-08-03 — User follow-up showed the workflow still fails in production. The live queue contains 493 unfinished plans: 480 unchecked, 11 unavailable after revalidation, and two launchable. Launching job `225138` through the production API returned 200 and produced a live browser, but Continue returned 200 and the browser closed within one second. `cmd/assist/main.go` returns whenever documents, PII, or `FillAssistedMappedPage` are unavailable; deferred cleanup then closes Chromium even though its log says the form remains ready. This is the reproduced root cause, and it invalidates the earlier “safe no-document continuation” completion claim.
- 2026-08-03 — Added a distinct `manual_review` state for continuation paths where documents, PII, or deterministic refill are unavailable. The assist process records that state and returns to its lease-renewal loop instead of returning from the process. The queue distinguishes a live manual browser from a closed session and never claims that fields or documents were ready.
- 2026-08-03 — Regression coverage now exercises missing-document and refill-error continuation branches, lease ownership, the live and closed manual-review projections, and the frontend’s manual completion/confirmation controls.
- 2026-08-03 — Live source verification on `127.0.0.1:18083` launched job `225138`, received `open`, accepted Continue, and reported `manual_review` with `live_browser=true` for five consecutive polls. The assisted process remained alive until it received a controlled termination, then released its lease; the closed projection offered `Reopen Verified Application`. No form was submitted and no application was marked applied.
- 2026-08-03 — Final pre-release verification passed: `go build ./...`, `go vet ./...`, `go test ./...`, `gofmt -l ./cmd ./pkg ./internal`, `git diff --check`, and frontend `npm run test`, `npm run build`, and `npm run lint`. The existing ADRs do not describe the Assisted Apply state machine, so no ADR update is required.

## Next step

Run the complete Go and frontend verification loops, review the diff and privacy boundaries, then commit, merge, rebuild both production binaries, restart the dashboard and daemon, and verify the live application.

## Completion

Completion is not yet proven. The previous source-level verification missed
that the no-document and refill-error branches close the browser immediately
after Continue. Re-run the full verification and live workflow after fixing
those branches.
