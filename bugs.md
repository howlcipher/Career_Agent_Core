# 🐛 Bug Backlog

This document is the authoritative, ranked backlog for known flaws, bugs, and broken items in Career Agent Core. It mirrors the structure of `improvements.md` and follows the same Working Protocol defined there: open a task journal, re-evaluate the model against what is currently available, route the matching library skill, then fix, verify, commit, and push. Bugs are prioritized independently of new features and generally outrank improvement work of similar effort — and while the Usability Gate below is unmet, bugs outrank *everything* in `improvements.md`, full stop.

## 🎯 Usability Gate — what "100% usable" means

This project reaches 100% usable when every box below is checked. Until then, this is the default work queue ahead of any Pending row in `improvements.md`; everything in that file is explicitly nice-to-have and out of scope until this gate is met.

- [x] `go build ./...` succeeds clean — verified 2026-07-19
- [x] `go vet ./...` reports no issues — verified 2026-07-19
- [x] `go test ./...` passes for every package that has tests (`config`, `parser`, `scraper`, `storage`, `submitter`) — verified 2026-07-19
- [ ] A fresh-machine Ollama install (`scripts/install_ollama.sh`) completes and pulls the three required models (`llama3.1`, `llava`, `nomic-embed-text`)
- [ ] `cmd/agent` completes one full batch run against live job boards end to end — discover → score → tailor (resume + cover letter) → submit or log to `applications/manual_submissions.md` → row written to `applications.db` — with zero crashes
- [ ] `cmd/dashboard` serves and displays live, correct data from a populated `applications.db`
- [ ] `cmd/tracker` runs against real IMAP credentials for at least one poll cycle without crashing (or no-ops cleanly per its existing missing-credentials guard)
- [ ] Zero open bugs below tagged `Blocker` or `Major` in the Ranked Backlog

**Status as of 2026-07-19: NOT MET.** The three static checks pass. The four live end-to-end checks are unverified — they require an actual run with real network access and credentials, which this backlog-scaffolding session did not perform (it only built the bug/improvement tracking structure). No bugs are currently known, so nothing is actively blocking except that those four checks have never been run and confirmed. **Next action toward the gate:** run `/work_next_item` (or manually attempt a real `cmd/agent` batch run) and either check off a live-run box with a dated verification note, or file the bug that broke it.

Every session — Claude Code, Gemini CLI, or manual — that touches this repo should glance at this checklist. When the last box is checked, change the Status line to `MET (YYYY-MM-DD)` and add a one-line note on what was verified; from that point on, `improvements.md`'s Pending rows become fair game for normal ROI-ranked selection instead of being blocked behind this gate.

## Ranked Backlog (best ROI first)

Pending bugs carry the same diminishing-returns score defined in `improvements.md` (Score = Value × Decay ÷ Effort, ROI floor 0.5). Bugs rarely decay — a defect's cost does not shrink because other defects were fixed — so Decay is normally 1.0. A bug below the floor stays open, flagged ⚠️, and needs explicit user confirmation before being worked. When a new bug is found (including one surfaced while checking the Usability Gate above), add a row here with a Severity (`Blocker` | `Major` | `Minor`) and a matching detail section, then work the table top down.

| # | Bug | Severity | Status | Score (V×D÷E) | Claude model | Gemini model | ROI rationale |
| --- | --- | --- | --- | --- | --- | --- | --- |
| — | *No open bugs.* | — | — | — | — | — | Static checks (build/vet/test) pass as of 2026-07-19; live end-to-end runs are unverified rather than known-broken — see the Usability Gate above |

## Details

*(none — populate as bugs are found)*

## ✅ Resolved

*(none yet)*
