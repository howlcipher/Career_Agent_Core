# Task Journal: Monitor live runs and fix surfaced defects

## Summary

- **Task:** Monitor live application runs, fix defects as they surface, record durable findings in the backlogs, and avoid any action that adds monetary cost.
- **Status:** Bug #120 is complete; the requested backlog groom is next. No live application agent or monitor process is running.
- **Started:** 2026-07-25.
- **Agent and model:** Codex orchestration with OpenAI `gpt-5.6-sol` at high reasoning for the bounded security implementation.
- **Durable history:** resolved implementation details live in `bugs.md` #70-#129 and `improvements.md` #28-#33. This journal intentionally keeps only live-run conclusions, unresolved decisions, operating hazards, and the resume point.

## Bug #120 pre-flight

- **Usability Gate:** UNMET; #120 is the highest-ranked open gate bug.
- **Model choice:** Current OpenAI GPT-5 coding model, working inline. The user reports that the Claude and Gemini sessions are limited, and this medium-complexity Go lifecycle refactor needs deterministic scheduling and cancellation tests rather than a separate provider.
- **Skills routed:** `hallucination_guardrails`, `systems_logic`, `software_development`, `architectural_guardrails`, `automation`, `quality_assurance`, `test_and_verify`, `technical_writing`, `commit_and_changelog`, `cyber_security`, and `devops`.
- **Code re-verified:** `cmd/agent/main.go` still reads `--daemon` only for its startup log. Queue creation, `GetDiscoveredJobs`, FunnelEngine discovery, worker startup, and `wg.Wait()` run once before the process exits. No batch cap or cancellable inter-cycle wait exists.

## Bug #120 plan

- [x] Add failing deterministic tests for batch, repeated daemon cycles, queue refresh, per-cycle caps, and cancellation.
- [x] Extract the queue/discovery work into one injected cycle and add a context-cancellable daemon coordinator.
- [x] Document the daemon flags and lifecycle in `README.md` and `CHANGELOG.md`.
- [ ] Groom all three backlogs, refresh the journal resume point, commit, and push.

## Current authoritative state

- Bug #127 is resolved in signed implementation commit `e4e48e1`: maintained commands enforce owner-only creation and repair existing private workspace paths without following symlinks.
- `go build ./...`, `go vet ./...`, and `go test ./...` all pass after #129. The focused agent and shared-configuration race suite also passes.
- The live permission repair completed. Named private root files and generated files are `0600`, generated directories are `0700`, and the application tree has no symlinks.
- Bug #123 is resolved in signed implementation commit `13c5a35`: failed, non-2xx, and weak job-page fetches can no longer reach embedding or fit scoring; response bodies close per attempt and transient failures use bounded retries before returning to `DISCOVERED`.
- Bug #129 is resolved in signed implementation commit `653f320`: both ingestion commands share portable path resolution, startup fails closed before stale chunks can be used, and `-no-rag` is an explicit retrieval bypass.
- The post-#121 groom re-verified and re-scored all 12 Pending rows across the three backlogs. Bug ranks are unchanged and #120 remains next. Free improvement #30 rose from 0.20 to 0.40 after live boolean-only inspection showed its motivating answers are blank again; it remains below floor.
- Bug #121 is resolved: one typed deterministic boundary protects posting embedding/scoring and every model-facing browser path; detections keep the CSV audit, receive a terminal funnel status, and never reach an LLM judge. The full build, vet, test, and focused race gates pass.
- Bug #120 is resolved: batch mode runs one unlimited cycle, while daemon mode refreshes database and discovery work every six hours, admits at most 15 jobs by default, and cancels its inter-cycle wait through the signal context. The configurable cap and deterministic lifecycle are covered by injected-cycle and injected-clock tests.
- The Usability Gate remains **UNMET** because 3 Major/Blocker bugs are open. `bugs.md` is authoritative; #122 is the next autonomous item before grooming.
- The old 82-job cohort tally is approximate because bug #112 leaves scheme-duplicate funnel rows independently mutable. Do not use it as exact status evidence until those rows are merged.
- No live agent, cohort watcher, or log-tail monitor survived into this resume point. The rebuilt container dashboard is running, both routes return HTTP 200, `ss` reports `127.0.0.1:8080`, and the host's non-loopback address cannot connect.

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
2. **Bug #112 merge policy:** 20 `http`/`https` duplicate pairs were re-measured on 2026-07-27, 15 with different statuses. Outward application dedup is fixed, but funnel insertion, updates, queueing, and reporting remain split. The merge must preserve strong evidence and must not silently promote an ambiguous row to `APPLIED`.
3. **Historical dedup rows:** older `applied_jobs` entries predate confirmation-only recording. Do not bulk-clear them: a false row suppresses a valid retry, but clearing a true row can file a duplicate.
4. **Paid CAPTCHA solving:** improvement #17 is worthwhile on evidence but remains out of scope without the user's provider choice and paid key.
5. **Low-ROI improvements:** #14 remains a deferred paywalled experiment; #27 should close unless a real external MCP client is named; #30 should be re-scoped to safe control-level detection or bypassed by configuring the currently blank answers.

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

Run the requested `/groom_backlogs` pass across all three backlogs, then commit and push. Only after all Major and Blocker rows close should a clean monitored cohort restart from the latest green build.
