# Task Journal: Safe local-model delegation harness

## Summary

- **Task:** Improvement #486: Safe local-model delegation harness
- **Status:** In progress
- **Started:** 2026-08-02
- **Agent and model:** Codex / GPT-5.6; local Ollama inventory checked, no local delegation used for this architecture task

## Pre-Flight Re-Evaluation

- **Usability Gate check:** MET (2026-08-02), so the highest-scoring free Pending item is eligible.
- **Model choice:** The item is deep-reasoning and defines the conditions under which local models may act. Codex retains the architectural and security decisions; live local inventory is qwen3:4b-instruct, qwen3:30b-instruct, qwen2.5vl:7b, and nomic-embed-text:latest.
- **Skills routed:** software_development, architectural_guardrails, cyber_security, quality_assurance, test_and_verify, technical_writing, commit_and_changelog, hallucination_guardrails.
- **Code re-verified:** No repository-owned delegation contract or execution command exists. #484 provides a separate read-only model benchmark harness and confirms the agent-lock safety pattern to reuse conceptually.

## Plan

- [ ] Define strict, framework-independent proposal and patch contracts.
- [ ] Implement a local-only CLI with explicit review approval and no patch application capability.
- [ ] Add isolated tests, documentation, backlog closure, full verification, commit, merge, rebuild, and rerun.

## Progress Log

- 2026-08-02 20:04 EDT — Selected #486 (0.83), confirmed it remains unimplemented, chose a default-deny artifact-only design, and created branch `agent/local-delegation-harness`.

## Next Step

Implement and test the contract package and `cmd/localdelegate` command.
