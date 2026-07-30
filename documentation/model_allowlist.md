# Backlog Model Allowlist

Every model ID that may appear in a model-recommendation column of `bugs.md`, `improvements.md`, or `improvements_paywall.md`. `internal/backlog/models_test.go` enforces this list under `go test ./...`; a backlog row naming anything absent from here fails the build.

## Why this file exists

Improvement #455 (2026-07-30) found that **`claude-opus-4-6-thinking` had been sitting in seven backlog rows for four days**. It is not a model ID and never was — no `-thinking` suffix exists in any Anthropic model name, because extended thinking is a request parameter (`thinking: {type: "adaptive"}`), not part of an ID. That string could not have resolved against any endpoint.

Roughly a dozen groom passes ran in those four days, each asserting it had re-verified every row. They missed it because **re-verification meant re-running a count, never testing the claim the count supported**. #455's own report is the clearest illustration: it counted occurrences precisely (73 / 7 / 4) and drew three conclusions from them, two of which were wrong —

- it called `claude-sonnet-4-6` obsolete; that model is current and still served,
- it reported a row whose Gemini column "reads literally `gemini-whatever-provider-is-configured`"; no such cell exists — the string is part of bug #444's *anchor link*, invented by grepping raw file text instead of parsing the column,
- it put the blast radius at 80 rows; the real figure was 7.

The precision made the conclusions feel audited. They were not. So the fix is not another instruction telling future sessions to check carefully — a dozen passes already believed they were. The fix is a check that runs whether anyone remembers it or not, and that parses the **table column** rather than grepping the file.

## What the check does and does not guarantee

It answers exactly one question: **does this string exist as a model ID?** That is mechanical, and it is the question that was being silently answered wrong.

It deliberately does *not* judge whether a row names the *best* or *newest* model. `claude-sonnet-4-6` is on this list and will pass, because it is real and served — it is simply a generation behind `claude-sonnet-5`. Currency is a judgement call that belongs to a human in a groom pass; existence is not, and should never have depended on one.

## Maintaining this file

Add an entry only with real provenance in the second column — where the value came from and when it was checked. The test rejects an entry with an empty provenance cell, because an allowlist that accepts unsourced additions reproduces the original bug one level up.

To re-verify the Anthropic section, read the current model catalogue (the `claude-api` skill's Current Models table, or `GET /v1/models`). **Do not re-verify from memory** — that is precisely how `claude-opus-4-6-thinking` was born.

## anthropic

Verified 2026-07-30 against the Anthropic model catalogue. Excludes `claude-mythos-5` (Project Glasswing participants only) and every retired ID.

| model ID | provenance |
| --- | --- |
| `claude-fable-5` | Anthropic model catalogue, checked 2026-07-30 |
| `claude-opus-5` | Anthropic model catalogue, checked 2026-07-30 |
| `claude-opus-4-8` | Anthropic model catalogue, checked 2026-07-30 |
| `claude-opus-4-7` | Anthropic model catalogue, checked 2026-07-30 |
| `claude-opus-4-6` | Anthropic model catalogue, checked 2026-07-30 |
| `claude-sonnet-5` | Anthropic model catalogue, checked 2026-07-30 |
| `claude-sonnet-4-6` | Anthropic model catalogue, checked 2026-07-30 — current but a generation behind `claude-sonnet-5` |
| `claude-haiku-4-5` | Anthropic model catalogue, checked 2026-07-30 |

## google

**Not verified against a vendor catalogue.** These are the values already in use across the backlogs; they are listed so the check can distinguish "a value this project has been using" from "a value someone just invented", which is the failure #455 actually represents. Confirming them against `agy models` is open work — see `improvements.md` #458.

| model ID | provenance |
| --- | --- |
| `gemini-3.6-flash-high` | In use in backlog rows since 2026-07-26; not vendor-verified (#458) |
| `gemini-3.1-pro-high` | In use in backlog rows since 2026-07-26; not vendor-verified (#458) |

## openai

**Not verified against a vendor catalogue**, same caveat as above. Recorded from the dated groom notes that introduced each one. See `improvements.md` #458.

| model ID | provenance |
| --- | --- |
| `gpt-5.6-terra` | In use in backlog rows since 2026-07-26; not vendor-verified (#458) |
| `gpt-5.6-sol` | Named by the 2026-07-27 post-#122 groom note as the model that ran that pass; not vendor-verified (#458) |
| `gpt-5.6-luna` | In use in backlog rows since 2026-07-26; not vendor-verified (#458) |
| `gpt-oss-120b-medium` | Named by the 2026-07-26 application-sweep note as locally available; not vendor-verified (#458) |
