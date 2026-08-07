# 💳 Improvement Backlog — Paywalled Items

This file holds improvements that need a **paid** signup, subscription, or API key to implement (e.g. `2captcha`/`capsolver`). It exists so `improvements.md` stays 100% free to work autonomously: `/work_next_item` skips this file by default, while `/groom_backlogs` re-verifies and re-scores it without implementing anything. No item here may be implemented unless the user explicitly names it in the current session.

Everything else — format, scoring model, Working Protocol — is identical to `improvements.md`; this file only separates out the items that need a paid service. See `improvements.md` for the full Working Protocol and the `Score = (Value × Decay) ÷ Effort` model. When an item here becomes free to build (a free-tier API appears, the user provisions a key, etc.), move its row and Details section back into `improvements.md` in the same commit that starts the work.

## Ranked Backlog (best ROI first)

**2026-08-06 groom pass.** All three rows remain correctly paywalled and keep their scores: **#424 2.0**, **#17 1.75**, **#14 0.43 ⚠️ below floor**. Re-verified against current code and the live host: the tree still has no CAPTCHA solver client, solve-and-continue path, or cloud DOM-routing path, and no paid key is configured; the only GPU is still an integrated Vega APU (`PCI_ID=1002:15D8`). #14's stacked re-verification history and #17's were collapsed to one paragraph each this pass under the Working Protocol's step 8. #14 remains a user decision: close it, or re-scope it to the preference-label collection precursor. Prior status paragraphs archived in `documentation/backlog_history/paywall_groom_history.md`.

| # | Improvement | Status | Score (V×D÷E) | Tier | ROI rationale |
|---|---|---|---|---|---|
| 424 | [Cloud-Offloading for DOM Parsing](#424-cloud-offloading-for-dom-parsing) | Pending — needs paid key | 2.0 = 8×1.0÷4 | standard | Hybrid mode using cheap/fast cloud models for heavy DOM mapping, saving local CPU cycles. Moved here from `improvements.md` on 2026-07-29: its premise is routing work to cloud APIs, which needs a paid key |
| 17 | [CAPTCHA & anti-bot solving](#17-captcha--anti-bot-solving) | Pending — needs paid key | 1.75 = 7×1.0÷4 | standard | The monitoring cohort found 6 of 7 fully filled forms blocked after submit. High potential value, but it still needs a paid 2captcha/capsolver key and explicit user approval |
| 14 | [Paid-compute LoRA fine-tuning experiment](#14-paid-compute-lora-fine-tuning-experiment) | Pending ⚠️ below floor — needs paid compute | 0.43 = 3×1.0÷7 | deep-reasoning | This host has only an integrated Radeon Vega GPU, so a useful fine-tuning experiment requires paid cloud compute. 58 confirmed applications now exist, but the schema still has no table that stores an interview or rejection outcome, so there is no preference-labeled training or evaluation set. Recommend deferring until labels exist |

## Details

### 424. Cloud-Offloading for DOM Parsing

Hybrid mode that routes heavy DOM mapping to cheap, fast cloud models (the original row named Gemini Flash and Claude Haiku) instead of the local CPU-only Ollama instance, saving local cycles on the pipeline's dominant cost.

**Moved here from `improvements.md` on 2026-07-29**, during the #423 run, unchanged and at its existing score of **2.0 = 8×1.0÷4**. The item was mis-filed: `improvements.md` is reserved for work that is 100% free to build autonomously, and `improvements_paywall.md` exists precisely so `/work_next_item` can never pick up key-gated work by accident. This item's entire premise is calling a paid cloud API, so it belongs here. It stays available on the user's explicit request, which is the only way anything in this file is ever worked.

**Worth noting before it is picked up:** this repo already has multi-provider LLM plumbing (`pkg/mcp`, `LLM_PROVIDER` in `.env`), so the work is provider routing per call type rather than a new architecture — closer to improvement #24's per-call-type model selection than to a ground-up build. Re-scope the Effort when the user actually authorizes it.

### 17. CAPTCHA & anti-bot solving
`pkg/submitter/browser.go` currently only comments that a CAPTCHA will cause the 45s page timeout to fire — there is no solving integration. Integrating `2captcha` or `capsolver` requires a paid API key, which is not currently in `.env.example` and must be discussed with the user before implementation per this repo's constraint on paid services (see `AGENTS.md`). Scope once approved: add the provider client, wire a CAPTCHA-detection check into `AttemptSubmit`/`AttemptVisionSubmit`, and handle the solve-then-continue flow without blocking the worker pool indefinitely.

**Re-verified 2026-08-06 (`/groom_backlogs`).** Still no solver client, provider dependency, solve-and-continue path, or configured solver key anywhere in the tree: `pkg/submitter` detects CAPTCHAs (`isCaptchaBlocked`) and routes to `BLOCKED_CAPTCHA`, and nothing attempts a solve. Value 7, Decay 1.0, Effort 4, score **1.75** — above the floor on merit, but paywalled and out of autonomous scope until the user chooses a provider and supplies a paid key. *(Eight prior dated re-verification paragraphs, 2026-07-24 through 2026-08-02, archived in `documentation/backlog_history/paywall_groom_history.md`; each recorded the same unchanged finding.)*

### 14. Paid-compute LoRA fine-tuning experiment

Collect scored jobs (fit score, outcome) into a training dataset and periodically fine-tune a model on the user's specific preferences, reducing reliance on prompting alone. Genuinely large scope: dataset export from `applications.db`, a training pipeline (likely Python/PEFT, shelled out to from Go), an eval harness to confirm the fine-tuned model does not regress scoring quality, and a rollout/rollback mechanism.

**Three findings, each independently disqualifying, first established 2026-07-25 and re-confirmed every pass since:**

1. **The hardware cannot do it.** The only GPU is an integrated AMD Radeon Vega (Picasso/Raven2 APU) — re-confirmed live 2026-08-06 via `/sys/class/drm`: `DRIVER=amdgpu`, `PCI_ID=1002:15D8`, no discrete card. Ollama runs CPU-only, and that iGPU class is outside ROCm's practical support. Useful LoRA fine-tuning therefore needs paid cloud GPU, which is what puts this item in this file.
2. **There is still no preference-labeled dataset.** 58 confirmed applications now exist in `applied_jobs` (up from 1 when this was last scored), but the database has no table that stores an interview or rejection outcome at all — verified 2026-08-06 against the live schema. Volume of applications is not the blocker; the absence of any outcome or preference label is.
3. **What data does exist is distillation data, not preference data.** `fit_score` is generated by `qwen3:30b-instruct`, not labeled by the user. Training on it teaches a small model to imitate the large one — that is improvement #24's goal (reached, and its model swap was rejected on evidence), not "learn the user's preferences," which needs human labels.

**Value 3, Decay 1.0, Effort 7, score 0.43 — ⚠️ below floor, re-confirmed 2026-08-06.** The standing recommendation is unchanged and now eleven passes old: **close it, or re-scope it to the "start capturing preference labels" precursor** that would make it viable later. It stays open per the never-close-unilaterally rule. *(Eight prior dated re-verification paragraphs archived in `documentation/backlog_history/paywall_groom_history.md`.)*
