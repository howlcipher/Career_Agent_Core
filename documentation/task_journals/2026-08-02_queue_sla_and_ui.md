# Task Journal: Queue SLA and retro-mecha interface redesign

## Summary

- **Task:** Improvement #492 plus user-requested dashboard and GitHub Pages UI redesign
- **Status:** In progress
- **Started:** 2026-08-02
- **Agent and model:** Codex / GPT-5.6

## Pre-Flight Re-Evaluation

- **Usability Gate check:** MET (2026-08-02); #492 is the top free Pending item.
- **Model choice:** Codex retains the standard-tier queue lifecycle and UI decisions because both touch operational safety and user-facing controls.
- **Skills routed:** software_development, architectural_guardrails, quality_assurance, test_and_verify, ui_ux, visual_design, frontend_engineering, accessibility, color_theory, technical_writing, commit_and_changelog, hallucination_guardrails.
- **Code re-verified:** `GetDiscoveredJobs` ranks only when read, `AddToFunnel` admits identified sources without a pending cap, and no scheduled first-attempt lifecycle sweep exists. The dashboard React app and GitHub Pages site (`docs/`) both use heavy neon/glass styling.

## Plan

- [ ] Add deterministic first-attempt SLA priority, source admission cap, and stale-row sweep with tests.
- [ ] Replace dashboard styling with a responsive, accessible retro-mecha engineering design system.
- [ ] Replace GitHub Pages styling and copy with the same design language.
- [ ] Build, test, inspect, document, close the backlog item, commit, push, and restart the dashboard.

## Progress Log

- 2026-08-02 21:25 EDT — Confirmed #492 remains unimplemented. Selected 7-day priority, 30-day terminal sweep, and 25-row source admission cap; mapped `cmd/dashboard/ui` and `docs/` as the two requested UI surfaces.

## Next Step

Implement the storage lifecycle policy and wire its sweep into every queue cycle.
