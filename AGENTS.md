# Career Agent Core — Agent Rulebook

This file is the canonical local rulebook for every AI agent working in this repository (Claude Code, Gemini CLI, Antigravity, or any other assistant). Per-agent entry points (`CLAUDE.md`, `GEMINI.md`) import this file; edit `AGENTS.md` only.

## Global Rules

Formatting, safety, skills, and the general grounding/epistemic-humility protocol come from the AI Knowledge Library, this project's sibling repo:

@../ai_knowledge_library/AGENTS.md

If your runtime does not resolve that import, read `../ai_knowledge_library/AGENTS.md` now and obey it fully before doing anything else — in particular its Skills Manifest (`.agents/skills/<skill_name>/SKILL.md`) and the Grounding Protocol (library/live-data/ask, in that order). Claude Code users on this machine get the library's `AGENTS.md` auto-loaded from `~/.claude/CLAUDE.md` regardless of working directory, so it is already active even without this import; Gemini CLI and Antigravity have no equivalent auto-load, so this import is load-bearing for them.

## Project-Specific Backlog

This repo tracks its own two backlogs, mirroring the library's format and Working Protocol:

- **`bugs.md`** — ranked defect backlog. Open with the **Usability Gate** at the top, which defines what "100% usable" means for this project. While the gate is unmet, closing bugs here outranks all Pending rows in `improvements.md`.
- **`improvements.md`** — ranked feature/enhancement backlog, plus the full Working Protocol (model selection, delegation, testing, commit/push) that both backlogs share.

Read the Working Protocol in `improvements.md` before working any item from either file.

## Project-Local Prompts

Reusable task prompts live in `.agents/prompts/`; its `README.md` is the index. Invoke them via Claude Code slash commands (`.claude/commands/`) or Gemini CLI commands (`.gemini/commands/`) — both are thin wrappers that point at the canonical prompt file. Edit the canonical prompt only.

- `/work_next_item` — work the single highest-priority open item across `bugs.md` and `improvements.md`.
- `/resume_task` — resume an interrupted task from its journal in `documentation/task_journals/`.
- `/groom_backlogs` — re-evaluate, re-rank, and clean both backlogs without implementing anything.

## Test Commands

This is a Go project. Standard verification loop, in order:

```bash
go build ./...
go vet ./...
go test ./...
```

There is no Makefile; run these directly from the repo root. `go test ./...` is fast enough to run in full every time — there is no "changed tests only" fast path here the way the library has.

## Constraints

- No paid API keys are assumed present. `LLM_PROVIDER` defaults to local Ollama (`.env.example`); Claude and Gemini providers require keys the user must supply and are not assumed available for autonomous agent work.
- `pii.yaml`, `.env`, `applications.db`, and `career_agent.log` hold real personal data and credentials — never print their contents into a commit, journal, or backlog entry.
- Anything free and already installed may be used autonomously (a linter, an existing CLI, a local model). Anything paid, requiring signup, or needing a new install must be discussed with the user first (e.g. `2captcha`/`capsolver` for CAPTCHA solving, mentioned in `improvements.md`).
