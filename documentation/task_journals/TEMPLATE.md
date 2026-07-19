# Task Journal: <task title>

Copy this file to `YYYY-MM-DD_<slug>.md` before starting a task (see the Working Protocol in `improvements.md`). Update and commit it at every milestone. A fresh session resumes by reading Status, the last Progress entry, and Next Step. When the task completes, move anything durable (findings, decisions, verification evidence) into the backlog Done note or a proper doc, then delete the journal in the task's final commit — only in-flight tasks have a journal here.

## Summary

- **Task:** <backlog item number and title, or bug description>
- **Status:** In progress | Blocked | Complete
- **Started:** YYYY-MM-DD
- **Agent and model:** <e.g. Claude Code / Sonnet 5, Gemini CLI / Gemini 3 Flash, local Ollama / qwen3>

## Pre-Flight Re-Evaluation

- **Usability Gate check:** <is the gate in bugs.md met? if not, confirm this task is a bug fix or the user explicitly asked for an improvement anyway>
- **Model choice:** <chosen model and one line of reasoning; note what was available: Claude Pro, Google subscription via Antigravity, local Ollama>
- **Skills routed:** <SKILL.md files read from ../ai_knowledge_library/.agents/skills/, e.g. `devops_sre`, `cyber_security`>
- **Code re-verified:** <confirm the item's claims still match the current code — this backlog has drifted from the code before>

## Plan

- [ ] <step 1>
- [ ] <step 2>

## Progress Log

- YYYY-MM-DD HH:MM — <what happened, what was verified, commit hash if any>

## Next Step

<one line: the single next action a resuming session should take>
