# Task Journal: Lever custom-question labels (bugs.md #545)

## Summary

- **Task:** `bugs.md` #545 — Lever's custom question cards extract a placeholder, an option, or a raw name attribute instead of the question
- **Status:** In progress
- **Started:** 2026-08-13
- **Agent and model:** Claude Code / Opus 5

> Note on numbering: `bugs.md` and `improvements.md` keep independent number spaces and both contain 543, 544 and 545. Every reference below is written in full as `bugs.md #545` or `improvements.md #545`.

## Pre-Flight Re-Evaluation

- **Usability Gate check:** MET (2026-08-11). Gate being met means bugs no longer automatically outrank improvements; the ranking is interleaved per improvements.md #506. This is a bug fix either way.
- **Model choice:** Claude Code / Opus 5, working directly rather than delegating headlessly. The Working Protocol's default is to delegate standard/deep-reasoning tiers to a non-Claude model to preserve session limits. Overridden deliberately here: the reproduction evidence (real Lever and Greenhouse DOM captured this session) lives in this session's context, and the hard part of a `deep-reasoning` item is deciding what correct looks like against that DOM. Handing a brief to a cold model would mean re-deriving the evidence and risking a word-list "fix" the bug row explicitly forbids.
- **Skills routed:** `accessibility` (the fix is about accessible-name resolution order — `aria-label`, `aria-labelledby`, `label[for]`, wrapping `<label>` — and the skill's directive to prefer native semantic relationships is exactly the design constraint), `test_and_verify` (zero-trust: prove via execution, not claim), `quality_assurance`, `software_development`.
- **Code re-verified:** Yes, and the row was partly wrong.
  - Confirmed: `labelFor` in `pkg/submitter/questions.go:63-88` falls through to `placeholder`/`name`, and the group branch at `:139` falls back to an option's own label.
  - Confirmed: the suspected cause is the real cause — `closest('fieldset, .field, .form-group, li, div')` at `:82` stops at `div.application-field`, a wrapper containing only the control.
  - **Corrected:** the row says "the page renders client-side, so fetching the HTML is not enough." False. Three real Lever `/apply` pages returned HTTP 200 to an anonymous `curl` with the complete server-rendered form markup. This removes the live-browser investigation the row's Effort 3 was budgeted for.
  - **Corrected:** `pkg/storage/knowledge.go:277-280` justifies refusing preflight on a rejected ATS with "Career Agent cannot see that application without an operator signed in". False for Lever — the apply form is public.
  - Re-scored: Value 4 → 6 on a live queue measurement (`AWAITING_REVIEW` = 20 Lever + 6 Greenhouse; Lever is 77% of the actionable queue, and Lever is precisely the handoff case the Copy Application Packet from PR #23 exists for).

## Plan

- [ ] Failing real-Chromium fixture tests reproducing all three bad label shapes
- [ ] Ancestor-walk label resolution in `controlInventoryJS`; group question separated from option text
- [ ] Greenhouse fixture pinning labels unchanged
- [ ] Split "rejects submissions" from "cannot be read": `blocksPreflight` + `storage.PreflightRefusalReason`
- [ ] gofmt / build / vet / test, plus the gated Playwright run
- [ ] Live no-submit Prepare run against real Lever postings; read the Copy Application Packet
- [ ] Backlog + backlog history + CHANGELOG + ADR-007 amendment; PR, review, merge

## Progress Log

- 2026-08-13 — Branch `fix/lever-question-labels` opened off `aa1cd28`. Reproduction evidence captured statically from three real Lever apply pages and one real Greenhouse form. Greenhouse regression risk established as near-zero: every real Greenhouse control carries `aria-label` or `aria-labelledby`, so Greenhouse never reaches the fallback this change replaces; the only unlabelled inputs are `aria-hidden="true"` and already dropped by `visible()`.

## Next Step

Write `pkg/submitter/questions_browser_test.go` and confirm it fails against the current extractor.
