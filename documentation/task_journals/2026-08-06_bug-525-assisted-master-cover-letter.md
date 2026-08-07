# Task Journal: bugs.md #525 — Assisted Apply attaches a .txt extraction where the automatic path uploads the master cover letter PDF

## Summary

- **Task:** bugs.md #525 — Assisted Apply resolves the cover letter to the per-job `coverletter.txt` even when `use_master_cover_letter` is on, so a file-upload form receives an unformatted `.txt` where `cmd/agent` uploads the designed PDF.
- **Status:** In progress
- **Started:** 2026-08-06
- **Agent and model:** Claude Code / Opus 5 orchestrating; implementation delegated to Antigravity CLI / `gemini-3.1-pro-high`

## Pre-Flight Re-Evaluation

- **Usability Gate check:** `MET (2026-08-06)`. Bugs and improvements are one ranked queue. #525 scores 2.0, the highest open row anywhere — `improvements.md`'s best Pending row is #485 at 0.67. No `⚠️ below floor` row was skipped above it.
- **Model choice:** Tier `standard`. Live delegates checked this session: `agy models` lists `gemini-3.1-pro-high` and `gemini-3.6-flash-high`; `curl localhost:11434/api/tags` lists `qwen3:30b-instruct`. Picked `gemini-3.1-pro-high` — the fix is small but spans three packages and the codebase's comment conventions are load-bearing, so the stronger available Antigravity model is worth the quota.
- **Skills routed:** `../ai_knowledge_library/.agents/skills/software_development/SKILL.md` (Go/Effective Go style, explain the "why" in comments, validate inputs at boundaries).
- **Code re-verified:** Yes, the row's claims still match the code.
  - `cmd/agent/pipeline.go:515-521` resolves the upload to `Profile.MasterCoverLetterPath` (falling back to `defaultMasterCoverLetterPath`) when `UseMasterCoverLetter` and `ShouldSendCoverLetter()` are both true, then returns that path to the submitter at `:542`.
  - `pkg/storage/assisted.go:241` `GetAssistedDocument` unconditionally resolves `cover_letter` to `applications/<dir>/coverletter.txt`, with the comment "Only the cover letter is a genuine per-job artifact" — true before `use_master_cover_letter` existed, false now.
  - Live `profile.yaml` has `use_master_cover_letter: true`, `master_cover_letter_path: "Omni_CoverLetter.pdf"`, `send_cover_letter: true`, so the divergence is active on this host.
  - Both consumers reach the same fill handlers: `cmd/assist/main.go:607-618` passes `cover.Path` into `submitter.FillAssistedMappedPage`, which hands it to `dedicatedATSHandler`/`handleDynamic` — the same functions the automatic path uses. So only the path differs, exactly as filed.

## Plan

1. Add `Profile.ResolvedMasterCoverLetterPath()` plus an exported default constant to `pkg/config`, holding the single definition of "which cover letter does this profile actually send".
2. Have `cmd/agent/pipeline.go` call it instead of open-coding the same branch, so the two paths cannot drift apart again — that drift is the whole bug class (#515, #517, #525).
3. Have `pkg/storage.GetAssistedDocument` resolve `cover_letter` through the same helper, validated the way `validateMasterResume` validates the master résumé.
4. Resolve once per `GetAssistedQueue` call rather than per row: the queue projection calls `assistedDocumentExists` twice for every one of ~520 rows.
5. Tests for both directions — master letter served when the profile enables it, per-job artifact still served when it does not.

## Progress

- **2026-08-06 — selection and re-evaluation complete.** Journal opened; no code written yet.

## Next Step

Write the delegation brief and run it through `agy -p ... --model gemini-3.1-pro-high --mode accept-edits`.
