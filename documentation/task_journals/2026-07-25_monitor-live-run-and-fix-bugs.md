# Task Journal: Monitor live runs and fix surfaced defects

## Summary

- **Task:** Monitor live application runs, fix defects as they surface, record durable findings in the backlogs, and avoid any action that adds monetary cost.
- **Status:** Paused behind the Usability Gate. No agent or monitor process is running as of the 2026-07-27 groom.
- **Started:** 2026-07-25.
- **Durable history:** resolved implementation details live in `bugs.md` #70-#124 and `improvements.md` #28-#33. This journal intentionally keeps only live-run conclusions, unresolved decisions, operating hazards, and the resume point.

## Current authoritative state

- Bug #124 is resolved and pushed in `e553070`: tracker outcome writes and Message-ID acknowledgements now commit atomically, with database failures and ambiguous multi-row matches left retryable.
- `go build ./...`, `go vet ./...`, and `go test ./...` all pass after #124. The focused tracker race suite also passes.
- The Usability Gate remains **UNMET** because 9 Major/Blocker bugs are open. `bugs.md` is authoritative; #126 is the next autonomous item.
- The old 82-job cohort tally is approximate because bug #112 leaves scheme-duplicate funnel rows independently mutable. Do not use it as exact status evidence until those rows are merged.
- No live agent, cohort watcher, or log-tail monitor survived into this resume point. The dashboard is running and returned HTTP 200 on both the root and metrics routes during the 2026-07-27 groom; its `*:8080` listener reconfirmed bug #126.

## Confirmed live conclusions

### One genuinely confirmed application

The pipeline completed a real Greenhouse application end to end for Akuity:

1. The form reached zero invalid fields.
2. The submit was accepted.
3. The mailbox signaled the security-code challenge.
4. The code was retrieved without logging it.
5. Greenhouse's eight single-character inputs were filled.
6. The resubmit reached a confirmation URL and page.
7. `job_funnel` moved to `APPLIED`, and the `applied_jobs` dedup row was written only after confirmation.

This answered the original live audit: the fill path can work. The historical problem was mainly that the pipeline misread delayed outcomes and could not finish the out-of-band code flow. Bugs #94-#117 contain the complete diagnostic and test history.

### Duplicate-application disclosure

Akuity received two applications for the same role. The first completed but the pre-#116 code could not recognize the success; a later requeue filed another. The duplicate cannot be undone. Before requeuing any job that reached code entry, check the inbox for a completion message.

### Bot protection is the current live ceiling

The monitored cohort found 6 of 7 completely filled forms blocked after submit, including all four attempted Lever forms. This raises paywalled improvement #17's value, but it still requires a paid solver key and explicit user approval.

Do not pre-skip a posting merely because a CAPTCHA widget is present. Akuity carried reCAPTCHA and still succeeded; presence is not proof of a block. Only post-submit outcome evidence is safe.

### The inbox is the strongest acceptance signal

Across the live investigation, the browser DOM repeatedly lagged the real outcome. Mailbox evidence exposed accepted submissions, delayed gates, delayed inputs, and delayed message indexing that one-shot page reads missed. Any future outcome check needs a bounded wait; never read an asynchronous signal only once.

## Unresolved decisions

1. **ClickHouse manual completion or requeue:** a prior submit was accepted and reached an email-code gate, but the old run did not complete it. Any old code is session-bound; the user should choose a fresh manual completion or a deliberate requeue.
2. **Bug #112 merge policy:** 20 `http`/`https` duplicate pairs were measured, 11 with different statuses. Outward application dedup is fixed, but funnel insertion, updates, queueing, and reporting remain split. The merge must preserve strong evidence and must not silently promote an ambiguous row to `APPLIED`.
3. **Historical dedup rows:** older `applied_jobs` entries predate confirmation-only recording. Do not bulk-clear them: a false row suppresses a valid retry, but clearing a true row can file a duplicate.
4. **Paid CAPTCHA solving:** improvement #17 is worthwhile on evidence but remains out of scope without the user's provider choice and paid key.
5. **Low-ROI improvements:** #14 should stay deferred or move to a separately approved paid-compute experiment; #27 should close unless a real external MCP client is named; #30 should close because its two motivating attestations are now configured.

## Operating procedure and hazards

- **Build on the host; run browser automation inside the `career-agent` distrobox.** The host has the current Go toolchain. The distrobox is required for the working Playwright/runtime environment.
- **Build a binary directly for live runs.** `go run` leaves a wrapper and child process; killing only the wrapper can leave the agent running. Verify the actual binary PID is gone before restarting.
- **The queue is a startup snapshot.** A running process does not see code fixes, newly discovered rows, or requeued statuses. Restart only when a meaningful fix needs exercising.
- **Requeue narrowly and verify the write.** Bug #112 means the log's URL scheme may not match the stored row. Compare on a scheme-normalized key and verify affected rows immediately.
- **Clear `applied_jobs` only when the retry is deliberate and safe.** `HasApplied` otherwise suppresses the requeued row. Never clear after code entry without checking the inbox first.
- **Re-arm monitors in every new session.** Confirm there are no orphaned agent/watch/tail processes, then watch both cohort status changes and outcome diagnostics.
- **Verify that a fix is present in the launched binary.** A green build does not prove the running executable was rebuilt from the intended source state.
- **Probe real pages without clicking submit.** A read-only browser probe gives faster structural feedback than a full model cycle, but must never file an application.
- **Do not log or journal one-time codes, credentials, or personal profile values.**
- **`TARGET_JOB_URL` skips normal discovery.** ATS-feed discovery is exercised only by a normal batch start; watch for its contribution when live runs resume.

## Next Step

Run `/work_next_item` for bug #126, the top-ranked open gate item. Continue down the bug backlog until the Usability Gate is met. Only then restart a clean monitored cohort from the latest green build; do not launch the historical cohort ahead of unresolved privacy, fetch, quarantine, and SSRF defects.
