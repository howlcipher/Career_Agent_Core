# Task Journal: Portable career profile resolution

## Summary

- **Task:** Bug #129, remove the developer-specific career-profile path and fail closed when grounded context is unavailable.
- **Status:** In progress
- **Started:** 2026-07-27
- **Agent and model:** OpenAI Codex / current GPT-5 coding model

## Pre-Flight Re-Evaluation

- **Usability Gate check:** UNMET. Six Major or Blocker bugs remain, and #129 is the highest-ranked open row.
- **Model choice:** The row recommends medium-capability Claude or Gemini models, but the user reports both providers at their session limits. The Working Protocol's fallback ladder therefore selects this session's current OpenAI GPT-5 coding model for the bounded Go/configuration change.
- **Skills routed:** `hallucination_guardrails`, `systems_logic`, `software_development`, `quality_assurance`, `test_and_verify`, `cyber_security`, and `commit_and_changelog`.
- **Code re-verified:** `cmd/agent/main.go` still hard-codes `/var/home/howlcipher/dev/ai_knowledge_library/USER_PROFILE.md`; `cmd/reingest/main.go` repeats it as the `-profile` default. Existing chunks can continue after path/probe failures, and no explicit no-RAG mode exists.

## Plan

- [x] Add failing tests for shared path precedence/readability and agent startup behavior with missing, configured, stale-cache, and explicit no-RAG inputs.
- [x] Implement shared portable profile resolution, fail-closed RAG initialization, and no-RAG retrieval bypass.
- [ ] Update user-facing configuration documentation and the bug record.
- [x] Run the full build, vet, test, and focused race verification.

## Progress Log

- 2026-07-27 — Re-verified the journal resume point, clean branch, backlog ranking, acceptance criteria, affected commands, and current RAG retrieval path.
- 2026-07-27 — Baseline `go build ./...`, `go vet ./...`, and `go test ./...` all pass before implementation.
- 2026-07-27 — Added red tests, then implemented the shared resolver and fail-closed startup. Focused `go test ./pkg/config ./cmd/agent ./cmd/reingest` passes.
- 2026-07-27 — Full build, vet, test, and focused agent/config race gates pass. Built command binaries expose the new flags and both exit nonzero on a missing configured profile before model or browser startup; output contains no profile content or removed machine-specific path.

## Next Step

Commit the reviewed implementation, then close bug #129 and remove this journal.
