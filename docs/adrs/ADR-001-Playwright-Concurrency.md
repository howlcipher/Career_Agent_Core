# ADR-001: Playwright Concurrency Model

## Status
Accepted

## Context
The orchestration loop (`cmd/agent/main.go`) leverages a worker pool (e.g., 10 concurrent workers) to process job applications rapidly. Initially, each worker invoked `playwright.Run()` or launched a separate `Browser` instance for every submission attempt. This resulted in extreme CPU/memory overhead and Node.js WebSocket race conditions, often crashing the Playwright driver entirely.

## Decision
We opted to decouple the driver and browser initialization from the worker loop.
1. A single `playwright.Playwright` driver and a single headless `Browser` instance are initialized synchronously at the start of `main.go`.
2. The `Browser` reference is injected into the pipeline.
3. Each worker thread spawns a lightweight, isolated `BrowserContext` (`bCtx, err := p.Browser.NewContext(...)`) for its specific job submission, and cleans it up via `defer bCtx.Close()`.

## Consequences
**Positive:**
- Memory footprint is significantly reduced, as multiple Chromium processes are not spawned simultaneously.
- Playwright WebSocket race conditions are mitigated, ensuring stability during parallel runs.
- Startup time per job submission is reduced, as context creation is vastly faster than browser launch.

**Negative:**
- If the single shared `Browser` instance crashes, all worker contexts will fail.
- Cross-contamination between contexts is theoretically possible if Playwright's isolation boundaries fail, though highly unlikely.
