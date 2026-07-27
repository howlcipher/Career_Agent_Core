# Task Journal: Make tracker outcome acknowledgement atomic

## Summary

- **Task:** Bug 124, the email tracker acknowledges a message even when its database update fails.
- **Status:** In progress
- **Started:** 2026-07-27
- **Agent and model:** Codex / GPT-5 session

## Pre-Flight Re-Evaluation

- **Usability Gate check:** Unmet. Bug 124 is the highest-ranked open gate item.
- **Model choice:** Antigravity offers the recommended `gemini-3.6-flash-high`, and local Ollama offers `qwen3:4b-instruct` and `qwen3:30b-instruct`. This bounded transaction fix stays in the orchestrating Codex session to avoid unnecessary handoff risk.
- **Skills routed:** `hallucination_guardrails`, `systems_logic`, `defensive_debugging`, `database_management`, `software_development`, `quality_assurance`, `test_and_verify`, `technical_writing`, and `commit_and_changelog`.
- **Code re-verified:** `pkg/tracker/imap.go` still discards `db.Exec` errors in `updateDBWithTrackerResult`, then calls `storage.MarkEmailProcessed` independently. A database lock or write error can therefore leave the outcome unchanged while permanently suppressing the email.

## Plan

- [x] Add failing transaction tests for a successful update, a no-op, an ambiguous multi-row match, and a locked database followed by a successful retry.
- [x] Persist the outcome update and processed-message acknowledgement in one transaction, with explicit update-state reporting and row-count validation.
- [x] Run focused tests and the full build, vet, and test sequence.
- [ ] Update README, changelog, and bug 124 with durable verification evidence; remove this journal; commit and push.

## Progress Log

- 2026-07-27 00:26 EDT: Re-verified the backlog claim against `pkg/tracker/imap.go` and `pkg/storage/manager.go`. Chose an atomic transaction over sequential writes because it closes the crash window and preserves retryability.
- 2026-07-27 00:42 EDT: Added transaction tests first and observed the expected compile failure. Implemented atomic outcome persistence, explicit result states, allowed-status validation, and rollback for multi-row matches. Focused tracker tests pass, including database-lock and acknowledgement-failure retries.
- 2026-07-27 00:47 EDT: `go build ./...`, `go vet ./...`, `go test ./...`, and `go test -race ./pkg/tracker -count=1` all pass.

## Next Step

Review the complete diff, scan the staged files for secrets and PII, then create a signed implementation milestone commit.
