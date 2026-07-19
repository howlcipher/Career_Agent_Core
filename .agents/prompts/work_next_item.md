# Work the Next Backlog Item

Work exactly one item end to end, leaving the repository in a state where this chat can be cleared and the next session starts with zero context.

## 1. Select

- Check `documentation/task_journals/` first (ignore `TEMPLATE.md`). If a journal for an in-flight item exists, resume that item instead of starting a new one (or run `/resume_task` directly).
- **Check the Usability Gate** at the top of `bugs.md`. If it is not yet `MET`, work the highest-priority open row in `bugs.md`'s Ranked Backlog instead of anything in `improvements.md`, unless the user explicitly names an improvement to work on anyway.
- If the gate is met, or `bugs.md` has no open rows, read the ranked table in `improvements.md` and pick the single highest-priority open item.
- **Below-floor gate:** never silently pick an item flagged `⚠️ below floor`. Skip past it to the highest-scoring above-floor item, and tell the user which flagged items were skipped so they can confirm one, re-scope it, or close it. Work a below-floor item only on the user's explicit confirmation in the current session.
- **Live-run gate checks:** if every listed item is done but the Usability Gate's four live end-to-end checkboxes are still unchecked, that IS the next item — attempt the relevant live run (fresh Ollama install, a real `cmd/agent` batch run, `cmd/dashboard`, or `cmd/tracker`) and either check the box with a dated verification note or file the bug it surfaces.

## 2. Re-evaluate before implementing

- Confirm the item is still worth doing and that its stated requirements still match the current code — this backlog has drifted from the code before (see `improvements.md` items 8-10, found already implemented during the 2026-07-19 rebuild). Grep for the relevant package/function before trusting the backlog row's description.
- If it is stale, update the row and detail section, merge it into another item, or close it with a dated note explaining why. A well-documented closure counts as completing this run.

## 3. Execute, delegating heavy implementation where sensible

This session is the orchestrator. To preserve Claude session limits, keep selection, re-evaluation, backlog/journal edits, verification, and the commit in this session; consider delegating the implementation itself to a non-Claude model for well-scoped work.

- Follow the Working Protocol in `improvements.md`: open a task journal from `documentation/task_journals/TEMPLATE.md`, route the matching library skill (`../ai_knowledge_library/.agents/skills/`), and read the item's detail section before writing any delegation brief.
- If delegating, write a self-contained brief: the item's detail section, the specific files involved, the tests to add, and any constraint that applies (e.g. item 17's paid-API-key gate). The delegate starts with zero repository context.
- Pick the delegate from what is live right now, starting from the item's Gemini model column:
  - **Antigravity CLI (headless):** `agy -p "<brief>" --model "<model>" --mode accept-edits --print-timeout 30m` from the repo root. List live model names with `agy models`; quota is shared across Gemini tiers, so on a quota error step to another tier or provider rather than giving up.
  - **Local Ollama:** for small, well-bounded subtasks. Check live tags with `curl localhost:11434/api/tags`.
  - Never delegate to Claude Code subagents (the Agent tool) for limit-saving; they bill the same Claude plan as this session.
- Require a clean `git status` before launching a delegate so its diff is exactly attributable. Afterward, review the full `git diff` yourself, run `go build ./...`, `go vet ./...`, `go test ./...` yourself, and either fix small gaps directly or re-delegate with concrete feedback. Never commit a delegate's work unreviewed.
- Update the journal and commit it at every milestone, recording each delegation and keeping its Next Step line current.

## 4. Close the loop

- Verify the change end to end, run `go build ./...`, `go vet ./...`, `go test ./...`, commit as `<type>(<scope>): <description>`, set the item's Status to `Done (YYYY-MM-DD)` with a Done note, delete this task's journal in the final commit, and push.
- Record findings discovered during the work as new rows plus detail sections in `improvements.md` or `bugs.md`.
- If this run checked a Usability Gate box, or all boxes are now checked, update the gate's Status line in `bugs.md` (including flipping it to `MET (YYYY-MM-DD)` with a verification note if every box is now checked).
- Housekeeping: delete any journals whose items are no longer outstanding.

Done means: clean `git status`, work pushed, no journal left for the finished item, and new findings filed where the next session will see them.
