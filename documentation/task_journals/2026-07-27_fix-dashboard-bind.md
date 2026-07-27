# Task Journal: Restrict and harden the dashboard listener

## Summary

- **Task:** Bug 126, the unauthenticated dashboard binds every network interface while announcing localhost.
- **Status:** In progress
- **Started:** 2026-07-27
- **Agent and model:** Codex / GPT-5 session

## Pre-Flight Re-Evaluation

- **Usability Gate check:** Unmet. Bug 126 is the highest-ranked open gate item.
- **Model choice:** Antigravity offers the recommended `gemini-3.6-flash-high`, and local Ollama offers `qwen3:4b-instruct` and `qwen3:30b-instruct`. This bounded standard-library hardening change stays in the orchestrating Codex session.
- **Skills routed:** `hallucination_guardrails`, `systems_logic`, `cyber_security`, `network_engineering`, `defensive_debugging`, `software_development`, `quality_assurance`, `test_and_verify`, `technical_writing`, and `commit_and_changelog`.
- **Code re-verified:** `cmd/dashboard/main.go` still logs `localhost:8080` while calling `http.ListenAndServe(":8080", nil)`. The live process independently confirms the listener is `*:8080`.

## Plan

- [ ] Add failing tests for the loopback default, configured address, non-loopback warning, validation, and server timeouts.
- [ ] Add an explicit `http.Server`, a `-addr` option defaulting to `127.0.0.1:8080`, and a prominent warning for non-loopback binds.
- [ ] Run focused tests, full build/vet/tests, and a loopback-only end-to-end probe on an unused port.
- [ ] Update README, changelog, bug 126, and the monitoring journal; remove this journal; create signed commits and push.

## Progress Log

- 2026-07-27 00:43 EDT: Re-verified the code and live listener. Chose an explicit CLI address over environment-only configuration because it requires no hidden `.env` loading and makes intentional remote exposure visible at startup.

## Next Step

Add focused failing tests in `cmd/dashboard/main_test.go` before changing production code.
