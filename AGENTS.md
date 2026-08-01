# Career Agent Core — Agent Rulebook

This file is the canonical local rulebook for every AI agent working in this repository (Claude Code, Gemini CLI, Antigravity, or any other assistant). Per-agent entry points (`CLAUDE.md`, `GEMINI.md`) import this file; edit `AGENTS.md` only.

## Global Rules

Formatting, safety, skills, and the general grounding/epistemic-humility protocol come from the AI Knowledge Library, this project's sibling repo:

@../ai_knowledge_library/AGENTS.md

If your runtime does not resolve that import, read `../ai_knowledge_library/AGENTS.md` now and obey it fully before doing anything else — in particular its Skills Manifest (`.agents/skills/<skill_name>/SKILL.md`) and the Grounding Protocol (library/live-data/ask, in that order). Claude Code users on this machine get the library's `AGENTS.md` auto-loaded from `~/.claude/CLAUDE.md` regardless of working directory, so it is already active even without this import; Gemini CLI and Antigravity have no equivalent auto-load, so this import is load-bearing for them.

## Project-Specific Backlog

This repo tracks its own three backlogs, mirroring the library's format and Working Protocol:

- **`bugs.md`** — ranked defect backlog. Open with the **Usability Gate** at the top, which defines what "100% usable" means for this project. While the gate is unmet, closing bugs here outranks all Pending rows in `improvements.md`.
- **`improvements.md`** — ranked feature/enhancement backlog, plus the full Working Protocol (model selection, delegation, testing, commit/push) that all three backlogs share. Every item in it is free to build.
- **`improvements_paywall.md`** — sibling backlog, same format, for improvements that need a paid signup/subscription/API key. Kept separate so `/work_next_item` only ever autonomously picks free work; an item here is only worked on the user's explicit request.

Read the Working Protocol in `improvements.md` before working any item from any of the three files — its step 8 (added 2026-08-01) is the size-discipline rule below and applies to all three.

**`documentation/backlog_history/`** holds the full historical record for all three backlogs: `<file>_groom_history.md` (superseded dated status paragraphs — each backlog keeps only its single current one inline) and `<file>_done_details.md` (full fix/implementation accounts for closed rows — each backlog keeps only a one-line pointer inline, in both the table's rationale cell and the `### N.` Details subsection). This split exists because a 2026-08-01 session found `bugs.md`/`improvements.md`/`improvements_paywall.md` had grown to 3411/1333/107 lines with 152/79/0 closed rows still carrying full inline narratives and dozens of stacked status paragraphs nobody ever archived — too large to read in full on every run, which is exactly what these files need to support (`/work_next_item` re-reads the live one every session). Only a **Pending** row's full Details section stays inline; that is the one a future session actually needs before picking it up. Never read `<file>_done_details.md`/`<file>_groom_history.md` as part of normal item selection — they are audit trail, not working state.

## Project-Local Prompts

Reusable task prompts live in `.agents/prompts/`; its `README.md` is the index. Invoke them via Claude Code slash commands (`.claude/commands/`) or Gemini CLI commands (`.gemini/commands/`) — both are thin wrappers that point at the canonical prompt file. Edit the canonical prompt only.

- `/work_next_item` — work the single highest-priority open item across `bugs.md` and `improvements.md` (free items only; `improvements_paywall.md` is out of scope unless the user names an item from it).
- `/resume_task` — resume an interrupted task from its journal in `documentation/task_journals/`.
- `/groom_backlogs` — re-evaluate, re-rank, and clean all three backlogs without implementing anything.

## Test Commands

This is a Go project. Standard verification loop, in order:

```bash
go build ./...
go vet ./...
go test ./...
gofmt -l ./cmd ./pkg ./internal
```

There is no Makefile; run these directly from the repo root. `go test ./...` is fast enough to run in full every time — there is no "changed tests only" fast path here the way the library has. `gofmt -l` should print nothing; any file it names is not gofmt-clean, and none of the first three commands will catch that (improvement #443 — eight files sat un-formatted for days because nothing in the loop read formatting). Run `gofmt -w <file>...` on anything it lists before committing.

**`go test ./...` also checks the backlog documents, not just the code.** `internal/backlog` validates that every **Pending** row of `bugs.md`, `improvements.md`, or `improvements_paywall.md` names a real Tier (`mechanical`/`standard`/`deep-reasoning`, improvement #456) rather than a stale model ID or an empty cell — the predecessor check validated concrete model IDs against `documentation/model_allowlist.md`, but concrete IDs expire (improvement #455 found one that had sat broken for four days) while a tier does not, so that mechanism was replaced rather than kept alongside it. `pkg/config` separately asserts that `.env.example` and `scripts/install_ollama.sh` agree on which Ollama models exist. Both exist because the facts they check were previously "true by convention" — and both conventions had already been silently broken for days before anyone noticed (improvement #455, bug #441). If a documentation edit turns the suite red, that is the check working; fix the document, and do not weaken the test to make it pass.

## Constraints

- No paid API keys are assumed present. `LLM_PROVIDER` defaults to local Ollama (`.env.example`); Claude and Gemini providers require keys the user must supply and are not assumed available for autonomous agent work.
- `pii.yaml`, `.env`, `applications.db`, and `career_agent.log` hold real personal data and credentials — never print their contents into a commit, journal, or backlog entry.
- Anything free and already installed may be used autonomously (a linter, an existing CLI, a local model). Anything paid, requiring signup, or needing a new install must be discussed with the user first (e.g. `2captcha`/`capsolver` for CAPTCHA solving, tracked in `improvements_paywall.md`).
