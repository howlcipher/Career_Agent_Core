# Task Journal: bug #439 — the tailored-document path ignores LLM configuration

## Summary

- **Task:** `bugs.md` #439 (Major, 4.0) — `ProcessJobApplication` hardcodes `"provider": "ollama"` and `"model": "llama3"`, hardcodes `http://localhost:8000/process`, and has no preflight readiness check
- **Status:** In progress
- **Started:** 2026-07-29
- **Agent and model:** Claude Code / Opus 5 orchestrating, Claude Sonnet 5 subagents for bounded implementation (user directed "prioritize claude models on this run" and "utilize multiple agents", which overrides the Working Protocol's default of delegating to non-Claude models to save limits)

## Pre-Flight Re-Evaluation

- **Usability Gate check:** UNMET — two open Majors (#439, #437). #439 is a bug fix and the gate's recommended next item, so it correctly outranks everything in `improvements.md` (which has zero Pending rows anyway).
- **Model choice:** as above; the user's explicit instruction this session supersedes the protocol's delegation default.
- **Skills routed:** `software_development`, `python`, `defensive_debugging`, `technical_writing`.
- **Code re-verified:** yes, all three claims hold. `pkg/mcp/client.go:236-237` still carries both hardcoded literals with their self-incriminating comments; `client.go:246` still hardcodes the port; `nlp_service/main.py:16-19` still takes provider/model/host off the request. Live `localhost:11434/api/tags` returns `qwen3:4b-instruct`, `qwen3:30b-instruct`, `qwen2.5vl:7b`, `nomic-embed-text:latest` — **`llama3` is not installed**, so the path cannot succeed. `.env` does set `OLLAMA_MODEL` (key checked without reading its value), so provider-level config on this host is correct and only this path is wrong.

## Finding that expands the item: #427 was a regression, not just a miswired call

A regression audit of `40767ca` (the commit that introduced the microservice) against `40767ca^` found that replacing the in-process Go implementation dropped considerably more than configuration:

1. **Circuit breaker and API metrics gone.** The old path called `incrementAndLogAPICall` four times (gap/resume/cover/prep); the new one never calls it. The 50k payload safety limit does not apply to the tailoring path at all any more, and the path emits no `[API Metrics]` line.
2. **Dynamic `num_ctx` gone.** Old: `(totalChars/3)+2000` clamped to `[8192, 64000]`. New: never sent, so Ollama silently falls back to its own small default and truncates context.
3. **Provider abstraction gone.** `c.provider` is never consulted. `nlp_service` accepts `provider` and `api_key` and reads neither — it only implements Ollama. `LLM_PROVIDER=claude`/`gemini` silently gets a local Ollama call.
4. **Bug #6 reintroduced.** `provider_ollama.go` defaults to a **120-minute** timeout precisely because measured CPU-only generation of this shape takes 25-35+ minutes. The new path uses a hardcoded 10-minute Go `http.Client` timeout and a hardcoded **5-minute** `requests.post` timeout in Python. Even with the right model installed, this path could not finish on this host.
5. **Prompt fidelity lost.** "anomaly detection", "CCNA foundation", "without extra commentary", "and talking points based on my profile", and "or de-emphasize their necessity" were all dropped when the prompts were retyped in Python.
6. **Error semantics worse.** Gap-analysis failure was non-fatal before; now it raises `HTTPException` and aborts the whole request before the other three calls start. On the Go side the response body is never read, so `{"detail": "Ollama generation failed: ..."}` is discarded and replaced with a bare status code.

So the fix is not "pass the right model": it is to make the in-process Go path authoritative again and reduce the microservice to a correctly-configured, opt-in offload.

## Design decided before implementation (the contract both sides build to)

**Go is the default and authoritative implementation.** The microservice is used only when `NLP_SERVICE_URL` is set, and only when it passes a preflight health check; any offload failure falls back to in-process generation rather than failing the job.

- `NLP_SERVICE_URL` — unset (default) means never call the service. Set (e.g. `http://localhost:8000`) means try it. Base URL, no path.
- Preflight: `GET {NLP_SERVICE_URL}/health`, 5-second timeout, must return 200.
- Offload only when the resolved provider can describe itself for offload (Ollama today). `LLM_PROVIDER=claude`/`gemini` always runs in-process, because the service is Ollama-only.
- **Go owns the prompts.** They are sent over the wire, so there is exactly one copy of each prompt in the repo and the drift in finding 5 cannot recur.

`POST {NLP_SERVICE_URL}/generate`:

```json
{
  "host": "http://localhost:11434",
  "model": "qwen3:30b-instruct",
  "keep_alive": "30m",
  "num_ctx": 12000,
  "timeout_seconds": 7200,
  "calls": [{"key": "resume", "system": "...", "prompt": "..."}]
}
```

Response, always HTTP 200 when the request itself is well-formed:

```json
{"results": {"resume": "..."}, "errors": {"cover_letter": "ollama returned HTTP 404: model not found"}}
```

Per-call errors rather than a whole-request abort, so the caller keeps the old non-fatal gap-analysis semantics. Go calls `/generate` twice: once for the single gap-analysis call, then once for the three concurrent generations, which preserves the old sequential gap-then-inject ordering.

## Plan

- [x] Re-verify #439 against live code and the live Ollama tags endpoint
- [x] Audit `40767ca` for regressions (agent 1)
- [ ] Inventory every doc mention that the change invalidates (agent 2)
- [ ] Go: restore in-process generation, add offload-when-configured with preflight and fallback (this session)
- [ ] Python: rewrite `nlp_service/main.py` as a generic concurrent executor with `/health` and `/generate` (agent 3)
- [ ] Go tests: in-process default, offload path carries the real model/host/num_ctx, unhealthy-service fallback, error-body propagation, non-Ollama provider never offloads
- [ ] `go build ./...`, `go vet ./...`, `go test ./...`
- [ ] Live verification: run the service, drive one real tailoring call end to end on this host
- [ ] Docs: README + `docs/` GitHub Pages + `.env.example` + CHANGELOG
- [ ] Close #439, file new findings, update the Usability Gate, groom backlogs, delete this journal, commit and push

## Progress Log

- 2026-07-29 20:40 — Journal opened. #439 re-verified live. Regression audit of `40767ca` complete (six regressions beyond the filed configuration defect). Contract above fixed before any code was written so the Go and Python halves can be built in parallel.

## Next Step

Implement the Go half of the contract in `pkg/mcp/client.go` while the Python rewrite is delegated.
