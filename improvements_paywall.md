# 💳 Improvement Backlog — Paywalled Items

This file holds improvements that need a **paid** signup, subscription, or API key to implement (e.g. `2captcha`/`capsolver`). It exists so `improvements.md` stays 100% free to work autonomously: `/work_next_item` skips this file by default, while `/groom_backlogs` re-verifies and re-scores it without implementing anything. No item here may be implemented unless the user explicitly names it in the current session.

Everything else — format, scoring model, Working Protocol — is identical to `improvements.md`; this file only separates out the items that need a paid service. See `improvements.md` for the full Working Protocol and the `Score = (Value × Decay) ÷ Effort` model. When an item here becomes free to build (a free-tier API appears, the user provisions a key, etc.), move its row and Details section back into `improvements.md` in the same commit that starts the work.

## Ranked Backlog (best ROI first)

| # | Improvement | Status | Score (V×D÷E) | Claude model | Gemini model | ROI rationale |
| --- | --- | --- | --- | --- | --- | --- |
| 17 | [CAPTCHA & anti-bot solving](#17-captcha--anti-bot-solving) | Pending — needs paid key | 1.75 = 7×1.0÷4 | claude-sonnet-4-6 | gemini-3.1-pro-high | The monitoring cohort found 6 of 7 fully filled forms blocked after submit. High potential value, but it still needs a paid 2captcha/capsolver key and explicit user approval |

## Details

### 17. CAPTCHA & anti-bot solving
`pkg/submitter/browser.go` currently only comments that a CAPTCHA will cause the 45s page timeout to fire — there is no solving integration. Integrating `2captcha` or `capsolver` requires a paid API key, which is not currently in `.env.example` and must be discussed with the user before implementation per this repo's constraint on paid services (see `AGENTS.md`). Scope once approved: add the provider client, wire a CAPTCHA-detection check into `AttemptSubmit`/`AttemptVisionSubmit`, and handle the solve-then-continue flow without blocking the worker pool indefinitely.

**Moved here 2026-07-24** from `improvements.md` (was item 17 there too) at the user's request, to keep paid-service items out of `/work_next_item`'s default scope. No change to the item itself.

**Re-scored 2026-07-25 (`/groom_backlogs`):** score unchanged at 1.25 (Value 5 × Decay 1.0 ÷ Effort 4) — above the ROI floor on merit, but **still gated on a paid key and therefore still out of scope for autonomous work**, per this file's whole reason for existing. Re-confirmed against current code that no solving integration exists: `pkg/submitter/browser.go` detects CAPTCHAs (`isCaptchaBlocked`, bugs #23/#45/#46) and routes to `BLOCKED_CAPTCHA`, but nothing attempts a solve. Unchanged otherwise; only work it if the user provisions a key and names the item.

**Model reassessed 2026-07-26 (application sweep):** live model discovery reports `claude-sonnet-4-6` and `gemini-3.1-pro-high`; those replace the stale future-model labels. This remains a medium-complexity API integration (a new provider client plus a detect-and-solve flow wired into two existing submit paths), not a reason to use the largest reasoning model.

**Re-scored 2026-07-26 (`/groom_backlogs`):** the monitoring cohort measured 6 of 7 completed fills blocked after submit, making bot protection the strongest documented live constraint. Value rises from 5 to 7; Decay 1.0 and Effort 4 remain, producing **1.75**. Repository-wide checks still find detection and `BLOCKED_CAPTCHA` routing but no solver client or provider dependency. The item remains paywalled and must not be worked until the user explicitly chooses a provider and supplies a paid key.

**Re-verified and re-scored 2026-07-27 (`/groom_backlogs`):** current code still detects and routes CAPTCHA blocks but has no solver client, provider dependency, or solve-and-continue path. None of the candidate solver environment variable names is present in the live process environment, and `.env.example` still defines no solver key. Value 7, Decay 1.0, Effort 4, and score **1.75** remain unchanged. The item stays paywalled until the user chooses a provider, explicitly requests the work, and supplies a paid key.
