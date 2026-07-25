# 💳 Improvement Backlog — Paywalled Items

This file holds improvements that need a **paid** signup, subscription, or API key to implement (e.g. `2captcha`/`capsolver`). It exists so `improvements.md` stays 100% free to work autonomously: `/work_next_item` and `/groom_backlogs` only read `bugs.md` and `improvements.md` by default, and must **not** pull from this file or implement anything here without the user explicitly naming the item in the current session.

Everything else — format, scoring model, Working Protocol — is identical to `improvements.md`; this file only separates out the items that need a paid service. See `improvements.md` for the full Working Protocol and the `Score = (Value × Decay) ÷ Effort` model. When an item here becomes free to build (a free-tier API appears, the user provisions a key, etc.), move its row and Details section back into `improvements.md` in the same commit that starts the work.

## Ranked Backlog (best ROI first)

| # | Improvement | Status | Score (V×D÷E) | Claude model | Gemini model | ROI rationale |
| --- | --- | --- | --- | --- | --- | --- |
| 17 | [CAPTCHA & anti-bot solving](#17-captcha--anti-bot-solving) | Pending — needs paid key | 1.25 = 5×1.0÷4 | Sonnet 5 | Gemini 3 Pro | Medium effort; needs a paid 2captcha/capsolver key — confirm with user before implementing |

## Details

### 17. CAPTCHA & anti-bot solving
`pkg/submitter/browser.go` currently only comments that a CAPTCHA will cause the 45s page timeout to fire — there is no solving integration. Integrating `2captcha` or `capsolver` requires a paid API key, which is not currently in `.env.example` and must be discussed with the user before implementation per this repo's constraint on paid services (see `AGENTS.md`). Scope once approved: add the provider client, wire a CAPTCHA-detection check into `AttemptSubmit`/`AttemptVisionSubmit`, and handle the solve-then-continue flow without blocking the worker pool indefinitely.

**Moved here 2026-07-24** from `improvements.md` (was item 17 there too) at the user's request, to keep paid-service items out of `/work_next_item`'s default scope. No change to the item itself.

**Model reassessed 2026-07-24 (`/groom_backlogs` pass):** checked against the current Claude lineup (Opus 5, Sonnet 5, Fable 5, Haiku 4.5) — Sonnet 5 stays. This is a medium-complexity API integration (a new provider client plus a detect-and-solve flow wired into two existing submit paths), not the kind of deep architectural work that would justify Opus 5, and not trivial enough for Haiku 4.5.
