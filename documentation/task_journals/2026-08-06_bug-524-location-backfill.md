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
- 2026-08-06 — Delegated to `agy` / `gemini-3.6-flash-high` from a clean tree. It produced `pkg/scraper/locationbackfill.go`, `pkg/scraper/locationbackfill_test.go`, `cmd/backfill-location/main.go`, the `countryNamesToCodes` additions (correctly omitting a bare `georgia`, which would read "Atlanta, Georgia" as the country), and one field added to `atsfeeds.go`'s `greenhouseJobsResponse`. Build, vet, tests and gofmt all clean on its output.
- 2026-08-06 — **Reviewed the diff and made three changes of my own**, the first substantive:
  1. **The delegate terminalized any posting absent from its account feed.** Absence is not proof of death — a board can carry postings reachable by direct link but not listed publicly, and writing those off is precisely the failure mode `bugs.md`'s CAPTCHA pre-skip decision forbids. Added `JobFeedURL`/`ParseJobFeed` (per-posting endpoints, all three confirmed live) so a row absent from the feed is asked directly; only an HTTP 404, or Workable's explicit non-`published` state, terminalizes it, and a live-but-unlisted posting resolves its location instead of being lost. This also handles Workable's per-job shape, which differs from the widget listing's (`remote` vs `telecommuting`, nested `location` object vs flat fields).
  2. **Reverted the `atsfeeds.go` edit.** Declared a named `greenhouseFeedJob` in the new file instead, so shipped discovery code is untouched and one type serves both the listing and per-job parse. Also replaced the Lever branch's verbatim copy of `parseLeverBoard` with a call to it, so the two cannot drift.
  3. The Workable no-`locations[]` fallback was pushing a country *name* into `CountryCodes`, which is a field of ISO codes; `CountryCodesFor` discarded it silently, so the free-text path happened to save it. Fixed to leave codes empty and let the free text do the work, with a test asserting `"Toronto, Ontario, Canada"` still resolves to `CA`. Also fixed counters that reported writes during a dry run and counted failed writes as successes, and made a profile that fails to load say so instead of silently reporting zero rejections.
- 2026-08-06 — `go build`, `go vet`, `go test ./...`, `gofmt -l ./cmd ./pkg ./internal` all clean. Database backed up to the session scratchpad. Live dry run started against the real `applications.db` (264 distinct board accounts to fetch).

- 2026-08-07 — **Workable rate-limited the whole host.** The `-confirm` run was killed at the user's request after 25 minutes with no writes applied (verified: 0 locations, 0 expiries — the tool fetches everything before writing anything, so a mid-run kill leaves no partial state). Probing then showed `apply.workable.com` returning 429 on *every* path with `Retry-After: 84643` (23.5 h). Greenhouse and Lever were unaffected (200 on both account and per-job endpoints).
  - **My earlier retry change was the wrong direction and I said so.** I had made 429 retry *harder* (4 attempts, 5s/10s/20s), which is exactly wrong against a host-wide block: it cannot succeed and keeps hitting a host that said stop. That is what turned a 13-minute run into an hour.
  - Fixed properly: `pkg/scraper.FeedHTTPError` now carries the parsed `Retry-After`, and the backfill gives a host up for the rest of the run once it exceeds two minutes, reporting those rows as rate-limited. **Run time went from ~60 min to 2 min**, and the summary now separates `HTTP 429 361` from `HTTP 404 3` instead of reporting one opaque "unresolved" count.
- 2026-08-07 — **Applied live.** `-confirm` wrote 67 locations and 18 `INVALID_URL`/`expired` rows in 63 seconds. Verified in the database, and end to end through a current dashboard build on `127.0.0.1:8099` serving all 67 with real locations; production `:8080` was read but never touched. Note `:8080` is running a binary that predates the location column in the queue projection and serves 0 — **it needs a restart to show this**.
- 2026-08-07 — Filed #530 (dead postings keep their queue card; none of the 18 left the queue) and added a Workable rate-limit operational trap to `bugs.md`. #524 left **Pending** rather than Done: 361 of 524 rows are still unfilled.

## Next Step

After **2026-08-08 ~03:20 UTC**, when Workable's block expires, run `go run ./cmd/backfill-location -confirm` (2 minutes, re-runnable, skips rows already filled) to backfill the remaining 361 Workable rows, then close #524. The 75 rows on feedless boards (Workday, SmartRecruiters, Ashby, Jobvite, BambooHR, applytojob, recruitee, pinpointhq) are out of scope for this row and stay empty.
