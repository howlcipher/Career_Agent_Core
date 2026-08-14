# Task Journal: Application Knowledge — Easy-Apply-like preparation for external applications

## Summary

- **Task:** New capability (improvements #538–#542 to be filed) plus bug #544, found while planning.
  Make the Approved Answer Vault's knowledge demand-driven and cumulative: aggregate unresolved
  questions across the queue, deduplicate them deterministically, let the operator resolve each once,
  and re-evaluate every queued application when an answer is approved.
- **Status:** In progress
- **Started:** 2026-08-13
- **Agent and model:** Claude Code / Opus 5

## Pre-Flight Re-Evaluation

- **Usability Gate check:** MET 2026-08-11 (`bugs.md`). Improvement work is fair game. Bug #544,
  filed by this task, is graded Major and is fixed first regardless.
- **Model choice:** Claude Code / Opus 5 throughout. The Working Protocol's default is to delegate
  implementation headlessly to preserve Claude limits; the user was asked and explicitly chose
  direct implementation for this task, because the change is cross-cutting (Go + SQL + React + docs)
  and coherence across the slices matters more here than limit preservation.
- **Skills routed:** `software_development`, `cyber_security` (read 2026-08-13 — the `pii.yaml` write
  path is the reason), and `frontend_engineering`, `technical_writing`, `quality_assurance` for the
  UI, ADR and test slices.
- **Code re-verified:** Yes, against the live tree and the live database, not against the backlog:
  - `approved_answers` 0 rows, `answer_aliases` 0 rows, `application_questions` 0 rows.
  - `answers.Store.SeedFromPII` is never called outside `pkg/answers/answers_test.go`.
  - No query anywhere groups questions by anything but `job_id`; no index on any question key.
  - `assisted_applications`: 372 rows — 329 `solve_captcha`, 26 `review_and_submit`, 5
    `login_or_create_account`.
  - ADR-006's log-confidentiality fix is shipped and closed; nothing to redo there.
  - **Bug #544 confirmed by reading, not assumed:** `patterns.go` `years_experience` requires only
    `{years|year}` + `{experience}`, is `Routine`, and `resolveFromPattern` sets
    `AutoFill: Sensitivity == Routine` — so a Kubernetes-scoped question auto-fills total career
    years. `answers_test.go:341` currently pins this as intended.

## Plan

- [x] 1. `fix(answers)` — skill-scoped experience questions must not resolve to total years (#544)
- [x] 2. `feat(storage)` — canonical key + auto_fillable on the question inventory, preflight table
- [x] 3. `feat(knowledge)` — `pkg/knowledge`: inbox, layered dedup, approve, re-evaluate
- [x] 4. `feat(submitter)` — discovery-only preflight, `cmd/preflight`
- [x] 5. `feat(dashboard)` — the Application Knowledge HTTP API, including the `pii.yaml` write path
- [x] 6. `feat(dashboard)` — the Application Knowledge UI
- [x] 7. `feat(dashboard)` — apply-session integration
- [x] 8. `docs` — ADR-007, ADR-005 update, ARCHITECTURE, README, CHANGELOG, backlog rows
- [ ] 9. Full verification (Go + frontend), live check, PR

## Progress Log

- 2026-08-13 — Branch `feat/application-knowledge` cut from `main` at `67a1d94`. Baseline confirmed
  clean before any edit: `go build ./...` and `go test ./...` both pass.
- 2026-08-13 — Slices 1-8 committed (`b263bf2`, `546a233`, `13428da`, `5bcfdfb`, `35fa806`,
  `38b0126`, docs). Go and frontend suites both green throughout. Bug #544 confirmed real by
  disabling the fix and watching six of eight phrasings auto-fill the career total.

## Next Step

Slice 9: live verification inside the `career-agent` distrobox, then push and open the PR.
