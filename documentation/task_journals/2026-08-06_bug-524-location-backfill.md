# Task Journal: bugs.md #524 — backfill location onto existing queue rows

## Summary

- **Task:** bugs.md #524 — "Existing queue rows carry no location, so #516's gate cannot screen them" (Minor, score 1.5, Tier `standard`)
- **Status:** In progress
- **Started:** 2026-08-06
- **Agent and model:** Claude Code / Opus 5 orchestrating; implementation delegated to Antigravity CLI (`gemini-3.6-flash-high`, standard tier)

## Pre-Flight Re-Evaluation

- **Usability Gate check:** gate is `MET (2026-08-06)`. With the gate met, `bugs.md` and `improvements.md` are one queue. #524 scores 1.5, tying improvements #513 (1.5); `bugs.md`'s stated tiebreak — bugs outrank improvement work of similar effort — selects #524. Nothing below it is above-floor and higher.
- **Model choice:** `standard` tier → `gemini-3.6-flash-high` via `agy` (confirmed live in `agy models`). Local Ollama has `qwen3:30b-instruct` as fallback. Not delegated to Claude subagents (bills the same plan).
- **Skills routed:** `software_development`, `defensive_debugging`, `quality_assurance`, `database_management`.
- **Code re-verified (2026-08-06, live):**
  - `applications.db` has 12,980 `job_funnel` rows; **28** carry a `job_location`.
  - The assisted queue (`assisted_applications` joined to `job_funnel`, `assisted_state != 'completed'`) holds **524** rows and **0 of 524** carry a location. The row's claim is exactly true, unchanged.
  - `pkg/scraper/atsfeeds.go:186-208` gates and records location **only for newly discovered feed postings** (`if isNew`), so the row's "forward-only" framing is accurate.
  - `pkg/storage/assisted.go:693` already selects `jf.job_location` into `AssistedJob.Location`, and `cmd/dashboard/ui/src/App.tsx:161,733` already renders it on the card. **The display path is fully wired; the column is simply empty.** Backfilling the column is therefore the whole visible fix.
  - Queue board mix (live): 361 Workable, 68 Lever, 20 Greenhouse, 75 other (Ashby / SmartRecruiters / Workday / Jobvite / BambooHR / applytojob). Three boards cover **449 of 524 = 86%**.

## Live API research (done in this session, before any code)

All three account-level feeds confirmed live 2026-08-06, real accounts from the live queue:

| Board | Endpoint | Location fields | Match key |
|---|---|---|---|
| Greenhouse | `https://boards-api.greenhouse.io/v1/boards/<slug>/jobs` | `location.name` (free text, e.g. `"Remote Estonia"`) | numeric `id` vs `/jobs/<id>` in URL |
| Lever | `https://api.lever.co/v0/postings/<slug>?mode=json` | `country` (ISO-3166 alpha-2), `categories.location`, `categories.allLocations` | `hostedUrl` |
| Workable | `https://apply.workable.com/api/v1/widget/accounts/<slug>?details=true` | `country`, `locations[].countryCode`, `locations[].city/region` | `shortcode` vs `/j/<SHORTCODE>` in URL |

Notes proven, not assumed:
- Workable returns **HTTP 403 without a User-Agent header** and 200 with one. `fetchATSFeed` already sets a UA.
- Expired postings are detectable: `boards-api.greenhouse.io/v1/boards/pointwild/jobs` returns 200 with a live list, while job `5240015008` from the queue is absent from it and its per-job endpoint 404s — that row is a genuinely dead posting, not a dead board.
- Multi-country postings are real and are exactly the #516 hazard: queued Workable job `action1/950A91C0A2` is Cyprus/Serbia/Montenegro, and `maxana/9EFCA28551` is US/Brazil/Argentina.

## Plan

- [ ] `pkg/scraper/locationbackfill.go`: `ParseBoardRef` (URL → board/account/job id), `AccountFeedURL`, and per-board feed parsers returning `map[jobID]feedJob`.
- [ ] `cmd/backfill-location`: group rows by account, one feed fetch per account, write `UpdateFunnelIdentity`; terminalize postings absent from a feed that fetched cleanly as `INVALID_URL` / `InvalidURLReasonExpired`; report which rows `LocationAllowed` would reject against `profile.yaml`'s allowlist. Report-only by default, mutates on `-confirm`.
- [ ] Unit tests over recorded feed fixtures (no network in tests).
- [ ] `go build` / `go vet` / `go test` / `gofmt -l`.
- [ ] Live dry run, then live `-confirm` run against the real `applications.db`, then re-query the queue to confirm coverage.

## Progress Log

- 2026-08-06 — Selected #524 over improvements #513 on the bugs-win-ties rule. Re-verified every claim against live code and the live database (see above); all hold. Researched and confirmed all three board APIs live before writing the delegation brief.

## Next Step

Write the delegation brief and launch `agy` with a clean `git status`.
