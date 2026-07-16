# Contributing to Career Agent Core

Welcome! We appreciate your interest in extending the Career Agent Core. This guide covers how you can contribute effectively.

## Directory Structure
- `cmd/agent/`: The main orchestrator loop, handling concurrency, LLM API calls, and batch executions.
- `pkg/scraper/`: Modular packages for querying remote APIs and job boards.
- `pkg/submitter/`: Contains ATS mapping logic and headless browser (Playwright) submission engines.
- `pkg/storage/`: Encapsulates SQLite database access via the Repository Pattern.
- `pkg/security/`: Defensive filters and prompt injection quarantine layers.
- `docs/adrs/`: Architecture Decision Records explaining major engineering choices.

## Adding a New Job Source (Scraper)
To integrate a new job source API:
1. Navigate to `pkg/scraper/`.
2. Implement your scraping logic. Ensure it takes a `context.Context` for proper cancellation and rate-limit handling.
3. Wire the new source into the `FunnelEngine`.

## Adding a New ATS Handler (Submitter)
To add static support for a new Applicant Tracking System:
1. Navigate to `pkg/submitter/`.
2. Update `dynamic.go` to recognize the new ATS domain footprint.
3. (Optional) Provide a static mapping template if the ATS uses a highly structured, unchanging DOM layout.

## Documentation Standards
- **Godoc:** All exported functions and structs must be documented with Godoc standard comments.
- **ADRs:** Significant architectural changes must be proposed with a new Architecture Decision Record in `docs/adrs/`.

## Running Tests
Ensure all unit tests pass before submitting a Pull Request.

```bash
go test ./...
```
