# Task Journal: Bug #441 — a clean setup ends up configured for models the installer never pulled

## Summary

- **Task:** bugs.md #441 (Major, score 7.0) — the documented setup path (`./scripts/install_ollama.sh`, then copy `.env.example`) leaves a fresh install configured for two models the installer never downloaded.
- **Status:** In progress
- **Started:** 2026-07-29
- **Agent and model:** Claude Code / Opus 5 as orchestrator, with Claude subagents for implementation (the user asked to prioritise Claude models and use multiple agents on this run)

## Pre-Flight Re-Evaluation

- **Usability Gate check:** UNMET — the "zero open Blocker/Major" box is open with #441 and #437 outstanding. #441 is the highest-scoring open bug (7.0) and the backlog's own recommended next item, so this is correct gate work.
- **Model choice:** Claude throughout, per the user's explicit instruction this run. Implementation is split across two Claude subagents working on disjoint file sets (Go preflight vs. installer scripts + docs); orchestration, review, verification and commits stay in this session.
- **Skills routed:** `automation` (fault tolerance, observability, and config-via-environment parity for the installer scripts), `technical_writing` (README/setup docs).
- **Code re-verified (live, not from the backlog's prose):**
  - `scripts/install_ollama.sh:25-27` — `TEXT_MODEL="${OLLAMA_MODEL:-llama3.1}"`, `VISION_MODEL="${OLLAMA_VISION_MODEL:-llava}"`, `EMBED_MODEL="${OLLAMA_EMBED_MODEL:-nomic-embed-text}"`. Confirmed. Nothing in the script reads `.env`.
  - `.env.example:10,21` — `OLLAMA_MODEL="qwen3:30b-instruct"` and `OLLAMA_VISION_MODEL="qwen2.5vl:7b"`, both **uncommented**. Confirmed.
  - `README.md:230` documents the installer as pulling `llama3.1` / `llava` / `nomic-embed-text`, and `README.md:221` tells the user to copy `.env.example` — in a later section than the installer, so the documented order is installer-first.
  - Live `localhost:11434/api/tags` on this host returns `qwen3:4b-instruct`, `qwen3:30b-instruct`, `qwen2.5vl:7b`, `nomic-embed-text:latest`. `llama3.1` and `llava` are **absent**, so the mismatch is real in both directions: a fresh user gets the two models their `.env` does not name, and this host would waste multi-GB pulls on two models its `.env` does not name either.
  - `pkg/mcp/provider_ollama.go:76-86` reads all four model names from the environment with those same fallbacks; nothing anywhere checks that the configured model actually exists before the first per-job call.

## Plan

- [ ] Make `.env.example`'s Ollama defaults agree with what the installer pulls (comment the two 32 GB recommendations, keep them as documented opt-ins).
- [ ] Make `scripts/install_ollama.sh` and `scripts/install_ollama.ps1` source `.env` when one exists, so the installer pulls what the agent is configured to use.
- [ ] Verify after pulling against `/api/tags` and fail loudly, naming the exact `ollama pull` needed.
- [ ] Add a startup model preflight in `pkg/mcp` wired into `cmd/agent`, so a misconfigured model is a startup error instead of a per-job failure discovered hours in. This is the part that closes the class rather than this instance.
- [ ] Update README (setup ordering, model paragraph, preflight behaviour) and the `docs/` GitHub Pages content if it duplicates any of it.
- [ ] `go build ./...`, `go vet ./...`, `go test ./...`, plus a live installer run and a live preflight check.
- [ ] File any new findings, groom all three backlogs, delete this journal, commit and push.

## Progress Log

- 2026-07-29 — Journal opened. Bug #441 re-verified live on all three of its halves (installer defaults, `.env.example` values, live tags endpoint). Confirmed still open and still correctly described.

## Next Step

Delegate the Go preflight and the installer/docs work to two Claude subagents on disjoint file sets, then review the combined diff.
