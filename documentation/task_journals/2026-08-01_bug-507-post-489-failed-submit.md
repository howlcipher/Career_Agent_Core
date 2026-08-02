# Task Journal: Bug #507 post-#489 FAILED_SUBMIT cohort

## Scope

Diagnose the first cohort processed after bug #489's prompt-injection false-positive fix, where all eight observed rows reached `FAILED_SUBMIT` despite a lower quarantine rate. Use sanitized aggregates only; do not record URLs, company names, titles, posting text, or audit matched text.

## Selection

- Usability Gate: MET (2026-08-01).
- Selected bug #507 from the 1.33-score tie because it directly measures whether the application pipeline is converting after the recent security fix.
- Tier: standard. The current Codex session will implement and review the work directly; no non-Claude delegate is needed for this bounded diagnosis.

## Evidence so far

- `applications.db` and `career_agent.log` exist locally and are owner-readable only.
- The backlog reports the initial post-#489 cohort as 8 `FAILED_SUBMIT`, 0 quarantine, and 0 CAPTCHA outcomes.
- Live daemon/process and safe aggregate evidence are next.

## Next step

Run sanitized, read-only database aggregates and inspect daemon logs to identify the dominant terminal category before proposing a minimal remediation.
