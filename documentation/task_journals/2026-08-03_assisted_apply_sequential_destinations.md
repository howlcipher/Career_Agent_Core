# Task Journal: Assisted Apply sequential launches and exact destinations

## Summary

- **Task:** Fix Assisted Apply so every selected job opens in sequence and lands on the correct CAPTCHA or application page.
- **Status:** In progress
- **Started:** 2026-08-03
- **Agent and model:** Codex / GPT-5

## Pre-Flight Re-Evaluation

- **Usability Gate check:** This is a user-directed production defect in the Assisted Apply workflow.
- **Model choice:** Codex / GPT-5; the failure spans the React batch controller, dashboard process lifecycle, Playwright navigation, and live SQLite lease state.
- **Skills routed:** `hallucination_guardrails`, `defensive_debugging`, `software_development`, `quality_assurance`, `test_and_verify`, `technical_writing`, `commit_and_changelog`, and `environment_doctor`.
- **Code re-verified:** `main` and `origin/main` are at signed merge `5128bd1`. The rebuilt dashboard and daemon are running from the repository. The queue has three browser-capable rows, no active assisted browser, and 490 rows requiring revalidation.

## Plan

- [x] Reproduce the first and subsequent launch paths through the production UI/API.
- [x] Capture the final browser destination and page classification for each launch without submitting anything.
- [x] Add failing lifecycle, batch, and destination tests before implementation.
- [x] Implement exact application-entry navigation and reliable sequential handoff.
- [x] Run the full Go/frontend verification and live multi-job proof.
- [ ] Commit, close this journal, merge, rebuild, restart, and verify production.

## Progress Log

- 2026-08-03 09:25 — The production dashboard log records one browser launch at 09:25:32 and a user close at 09:26:16, but no subsequent launch. The queue now has one `manual_review` row and two `open_verified_application` rows, all with `live_browser=false`.
- 2026-08-03 09:28 — Source inspection found that the batch controller advances its index and calls launch immediately without waiting for the prior browser lease to be released or refreshing the queue. The UI also disables every browser action whenever its last queue snapshot contains any live browser; Assisted Apply has no background queue poll while open.
- 2026-08-03 09:28 — The assist command navigates only to the stored discovery URL and validates only that the document title contains the role. It does not prove the visible page contains a CAPTCHA or application form, and it never follows an Apply entry point. This matches the report that opened pages are not the correct CAPTCHA/application destination.
- 2026-08-03 09:30 — Reproduced the lifecycle race through the live API: job 225138 launched successfully, but an immediate job 224951 launch received HTTP 500 while the first lease was active. After the first process closed and the lease cleared, 224951 launched. The backend lease is working; the React batch controller was advancing against a stale queue snapshot.
- 2026-08-03 09:34 — Added a frontend regression test for a two-job batch. It initially failed because Next remained actionable during the live lease. The UI now polls Assisted Apply every two seconds, refuses Start/Next while any browser is active, advances only after a successful launch, and labels the blocked action “Close Current Application First.” All 18 frontend tests pass.
- 2026-08-03 09:39 — Added destination tests and implementation for visible Apply controls, anchor/ancestor hrefs, guarded fresh tabs, popups, application forms, account gates, and CAPTCHA surfaces. Workable posting URLs now derive their stable `/apply/` route. Targeted `pkg/submitter` and `cmd/assist` tests pass.
- 2026-08-03 09:45 — Live Playwright navigation reached the exact Workable `/apply/` route but the controlled tab closed after roughly seven seconds. A direct Chromium diagnostic remained alive, identifying Workable anti-automation behavior rather than a route or lease failure.
- 2026-08-03 09:55 — Built a direct-browser fallback around the existing authenticated `NetworkGuard` proxy. Initial process-alive checks exposed two false positives: the Playwright browser bundle launched outside its runtime had no usable font environment, and branded Chrome no longer loaded the unpacked proxy-auth extension. Screenshots and browser state were used to reject both attempts.
- 2026-08-03 10:03 — The current guarded design runs Chrome for Testing inside the installed Chrome Flatpak runtime, retaining Flatpak isolation while granting only the browser bundle read-only. A private Manifest V3 extension registers proxy authentication before navigating from `about:blank`. Live screenshot proof shows job 224951 at `apply.workable.com/koin-limited/j/0BA83F6D6F/apply/`, titled “AI Product Engineer,” with the Application tab and Personal information form visible. No fields were filled and nothing was submitted.
- 2026-08-03 10:07 — Replaced the timing-based direct-browser acknowledgement with a private loopback readiness URL. The extension signals only after the exact top-level application route completes and its normalized browser title contains the expected role. The dashboard returned HTTP 200 only after that signal; job 224951 remained live on the verified application form.
- 2026-08-03 10:08 — Closed job 224951, observed `live_browser=false`, then launched job 146891. The second launch returned HTTP 200 and a live screenshot showed `apply.workable.com/webook/j/B73794CB19/apply/`, title “Agentic AI Engineer,” the Application tab, and visible Personal information fields. This proves the close/release/next-launch sequence without filling or submitting anything.
- 2026-08-03 10:11 — Full verification passed: `go build ./...`, `go vet ./...`, `go test ./...`, clean `gofmt -l ./cmd ./pkg ./internal`, frontend lint, all 18 frontend tests, and the production frontend build. README, CHANGELOG, and ADR-002 now describe the exact-destination, sequential lease, and guarded Workable compatibility behavior.

## Next Step

Create the signed implementation milestone commit, delete this resume-only journal in the closing commit, merge to `main`, rebuild/restart production, and repeat the live health and sequential-launch checks against the production binaries.
