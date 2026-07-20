# 🐛 Bug Backlog

This document is the authoritative, ranked backlog for known flaws, bugs, and broken items in Career Agent Core. It mirrors the structure of `improvements.md` and follows the same Working Protocol defined there: open a task journal, re-evaluate the model against what is currently available, route the matching library skill, then fix, verify, commit, and push. Bugs are prioritized independently of new features and generally outrank improvement work of similar effort — and while the Usability Gate below is unmet, bugs outrank *everything* in `improvements.md`, full stop.

## 🎯 Usability Gate — what "100% usable" means

This project reaches 100% usable when every box below is checked. Until then, this is the default work queue ahead of any Pending row in `improvements.md`; everything in that file is explicitly nice-to-have and out of scope until this gate is met.

- [x] `go build ./...` succeeds clean — verified 2026-07-19
- [x] `go vet ./...` reports no issues — verified 2026-07-19
- [x] `go test ./...` passes for every package that has tests (`config`, `parser`, `scraper`, `storage`, `submitter`) — verified 2026-07-19
- [x] A working local Ollama install with the models `cmd/agent` needs — verified 2026-07-20. Note: this box's original wording (`llama3.1`, `llava`, `nomic-embed-text`) is stale; `.env.example` now recommends `qwen3:30b-instruct` / `qwen2.5vl:7b` / `nomic-embed-text` for 32GB-RAM machines, and those are the models actually pulled and confirmed present on this dev machine.
- [ ] `cmd/agent` completes one full batch run against live job boards end to end — discover → score → tailor (resume + cover letter) → submit or log to `applications/manual_submissions.md` → row written to `applications.db` — with zero crashes. **Still NOT met after extensive 2026-07-20 testing** — see bugs #1-#4 below. Three real bugs blocking this were found and fixed (#1, #2 resolved), but no run ever produced a genuine `APPLIED` result; the current blocker is #3 (Blocker, open).
- [x] `cmd/dashboard` serves and displays live, correct data from a populated `applications.db` — verified 2026-07-20: built and ran `cmd/dashboard`, confirmed `/api/metrics` returns live counts matching `applications.db` contents (e.g. `{"discovered":1626,"processing":114,"skipped":56,"applied":36,"failed":104}`).
- [ ] `cmd/tracker` runs against real IMAP credentials for at least one poll cycle without crashing (or no-ops cleanly per its existing missing-credentials guard) — still unverified, not attempted 2026-07-20.
- [ ] Zero open bugs below tagged `Blocker` or `Major` in the Ranked Backlog — **not met**, bug #3 is an open Blocker as of 2026-07-20.

**Status as of 2026-07-20: NOT MET.** Static checks and Ollama install now confirmed; `cmd/dashboard` newly confirmed working. The live `cmd/agent` end-to-end check remains unmet — extensive real-run testing found and fixed two real infrastructure bugs (dead Playwright CDN, host/container library mismatch) but surfaced a new Blocker (#3: Ollama context-window overflow causing the model server to hang) that has prevented any run from reaching a genuine `APPLIED` result so far. **Next action toward the gate:** fix bug #3 (increase Ollama's context window), then re-run `cmd/agent` and confirm at least one job reaches `APPLIED` status with zero crashes.

Every session — Claude Code, Gemini CLI, or manual — that touches this repo should glance at this checklist. When the last box is checked, change the Status line to `MET (YYYY-MM-DD)` and add a one-line note on what was verified; from that point on, `improvements.md`'s Pending rows become fair game for normal ROI-ranked selection instead of being blocked behind this gate.

## Ranked Backlog (best ROI first)

Pending bugs carry the same diminishing-returns score defined in `improvements.md` (Score = Value × Decay ÷ Effort, ROI floor 0.5). Bugs rarely decay — a defect's cost does not shrink because other defects were fixed — so Decay is normally 1.0. A bug below the floor stays open, flagged ⚠️, and needs explicit user confirmation before being worked. When a new bug is found (including one surfaced while checking the Usability Gate above), add a row here with a Severity (`Blocker` | `Major` | `Minor`) and a matching detail section, then work the table top down.

| # | Bug | Severity | Status | Score (V×D÷E) | Claude model | Gemini model | ROI rationale |
| --- | --- | --- | --- | --- | --- | --- | --- |
| 3 | [Ollama context window overflow hangs the model server](#3-ollama-context-window-overflow-hangs-the-model-server) | Blocker | Pending | 6.4 = 8×1.0÷1.25 | Sonnet 5 | Gemini 3 Pro | Blocks the entire Usability Gate; fix is a single config/flag change (start Ollama with a larger `-c`/`OLLAMA_CONTEXT_LENGTH`), effort near-minimal once diagnosed |
| 4 | [AttemptSubmit form-fill reliability is inconsistent across ATS platforms](#4-attemptsubmit-form-fill-reliability-is-inconsistent-across-ats-platforms) | Minor ⚠️ below floor | Pending | 0.4 = 2×1.0÷5 | Sonnet 5 | Gemini 3 Pro | Value is low until #3 is fixed and a real sample size of post-fix failures exists; effort to diagnose per-ATS timing issues without reproducible cases is high. Flagged ⚠️, needs user confirmation before working |

## Details

### 3. Ollama context window overflow hangs the model server
**Symptom:** During a 2026-07-20 live `cmd/agent` run, every `AttemptSubmit` call began failing with `failed to reach ollama at http://localhost:11434 ... context deadline exceeded`. `ollama ps` showed `qwen3:30b-instruct` stuck in `Stopping...` for 15+ minutes, unresponsive to `ollama stop`, while still holding ~20GB RAM (system had only ~360MB available at the worst point). A direct `curl` to `/api/chat` timed out completely with no response. The only fix was `kill -9` on the underlying `llama-server` process.

**Root cause:** confirmed via the log line that appeared right as the server died: `ollama returned HTTP 500: {"error":"...request (4237 tokens) exceeds the available context size (4096 tokens)..."}`. The local Ollama server is started with a 4096-token context window (`-c 4096` on the `llama-server` process), but `pkg/mcp`'s `ProcessJobApplication` prompts (job description + RAG career-chunk context + profile constraints, generating resume + cover letter + interview prep in one call) routinely exceed that — observed at 4237 and 4271 tokens. Instead of failing that one request cleanly, llama.cpp's server hangs in a broken/zombie state under context overflow, blocking all subsequent requests and consuming full model RAM until manually killed.

**Impact:** this is the current hard blocker on the Usability Gate — no `cmd/agent` run has reached a genuine `APPLIED` result because of it. It's a systemic issue (will recur on every run using the recommended `qwen3:30b-instruct` config from `.env.example`), not a one-off flake.

**Suggested fix:** increase Ollama's context window for this model — either set `OLLAMA_CONTEXT_LENGTH` (e.g. `8192` or higher) in the environment before `ollama serve` starts, or create a custom Modelfile for `qwen3:30b-instruct` with `PARAMETER num_ctx 8192` and use that tag. Consider also adding a client-side prompt-size guard in `pkg/mcp` (truncate RAG context or job description before sending) as defense in depth, so an oversized request fails fast with a retryable error instead of relying solely on server-side capacity.

### 4. AttemptSubmit form-fill reliability is inconsistent across ATS platforms
**Symptom:** across three live `cmd/agent` runs on 2026-07-20 (after the Playwright/container fixes below), `AttemptSubmit` reached a live job page and began tailoring successfully several times, but failed at the form-fill stage for different reasons each time: `form failed to render in time: playwright: timeout: Timeout 15000ms exceeded` (Lever), `failed to fill first_name: playwright: timeout: Timeout 5000ms exceeded`, and one case correctly blocked by the security layer (`malicious prompt injection detected on career page` — this one is a guardrail working as intended, not a bug).

**Assessment:** not enough data yet to tell whether this is a real, fixable timing bug (form-fill/render timeouts too short for slower ATS pages) or normal real-world flakiness (different site, different network conditions each time). Re-evaluate once bug #3 is fixed and a clean run can accumulate several `AttemptSubmit` attempts without the Ollama hang masking the signal.

**Suggested fix (if it recurs post-#3):** consider increasing the 15s form-render wait and 5s field-fill timeout in `pkg/submitter/browser.go`'s `handleLever`/`handleDynamic` handlers, and/or adding a retry-with-backoff around individual field fills before failing the whole attempt.

## ✅ Resolved

### Playwright driver download fails — dead Azure CDN (Resolved 2026-07-20)
**Symptom:** `cmd/agent` failed immediately on startup with `Failed to install Playwright: could not install driver: ... got non 200 status code: 404 (404 Not Found) from https://playwright.azureedge.net/builds/driver/playwright-*-linux.zip` (tried against driver versions 1.42.1 and 1.60.0, both 404).

**Root cause:** `go.mod` pinned `github.com/playwright-community/playwright-go v0.4201.1`, an old version that downloads its Node driver from `playwright.azureedge.net` — a CDN Microsoft has since retired. The fork's newest tag (`v0.6100.0`) switches to an npm/Node-based installer instead, but that tag's `go.mod` still declares its module path as the original `github.com/mxschmitt/playwright-go`, so `go get github.com/playwright-community/playwright-go@v0.6100.0` fails with a module-path mismatch error.

**Fix:** changed all four import sites (`cmd/agent/main.go`, `pkg/submitter/{browser,browser_test,dynamic,vision}.go`) from `github.com/playwright-community/playwright-go` to `github.com/mxschmitt/playwright-go`, then `go get github.com/mxschmitt/playwright-go@v0.6100.0 && go mod tidy`. Verified: `go build/vet/test ./...` all pass; the driver now installs successfully via npm instead of the dead CDN.

### cmd/agent crashes/hangs when run on the Bazzite host instead of the documented distrobox container (Resolved 2026-07-20)
**Symptom:** even after the Playwright driver fix above, the shared Chromium `browser` instance would die partway through a live run (`playwright: target closed: Target page, context or browser has been closed`) on every `AttemptSubmit` call after the first few, on this dev machine specifically.

**Root cause:** this machine runs Bazzite (Fedora Atomic/Kinoite), not Ubuntu. Playwright's bundled Chromium expects Ubuntu-native shared libraries (`libicu74`, `libjpeg-turbo8`, `libwoff1`); Fedora ships different, ABI-mismatched versions. The README already documents the correct setup (`distrobox create --name career-agent --image ubuntu:22.04`) and a matching container existed on this machine, but it was never actually being used for live runs, and even inside it, apt's `golang-go` package is a stale Go 1.18.1 that can't parse this project's `go 1.26.5` directive in `go.mod`.

**Fix:** installed Go 1.26.5 manually inside the `career-agent` distrobox container (apt's version is too old), confirmed `libjpeg-turbo8`/`libwoff1` already matched exactly inside the Ubuntu 22.04 container. Running `cmd/agent` from inside the container (with `/usr/local/go/bin` prepended to `PATH`) resolved the browser-crash pattern — subsequent runs got well past the point where it used to die (multiple jobs reached "Generating tailored documents" and beyond, whereas host runs died after 2-4 jobs). **Operator note:** `export PATH=$PATH:/usr/local/go/bin` (append) does *not* work if the container already has an older `go` earlier in `PATH` — must be `export PATH=/usr/local/go/bin:$PATH` (prepend) so the new Go wins.
