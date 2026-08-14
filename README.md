# Career Agent Core

[![CI](https://github.com/howlcipher/Career_Agent_Core/actions/workflows/ci.yml/badge.svg)](https://github.com/howlcipher/Career_Agent_Core/actions/workflows/ci.yml)
[![Go Version](https://img.shields.io/badge/go-1.26+-blue.svg)](https://go.dev)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

Career Agent Core is an agentic workflow and automation platform written primarily in Go. It demonstrates how to combine semantic matching, multi-model LLM orchestration, secure processing of untrusted web content, resilient browser automation, human-in-the-loop controls, stateful workflow execution, outcome tracking, observability, and an operational dashboard.

<div align="center">
  <img src="docs/images/og-image.jpg" alt="Career Agent Core Dashboard" width="100%">
</div>

### Why this exists

This project explores a fundamental engineering question:

> What does a trustworthy agentic workflow look like when an AI system must discover external information, reason about it, interact with unreliable third-party systems, maintain durable state, recover from failure, and know when to hand control back to a human?

It is built around the domain of career and job-search automation, but the architecture serves as a blueprint for safe, stateful, and observable AI-driven automation.

### What it demonstrates

- **Go backend architecture**: Concurrency, worker pools, bounded execution, and pure Go SQLite persistence.
- **Agentic workflows**: Stateful graphs orchestrating LLM calls, web scraping, and browser interaction.
- **LLM orchestration**: Support for Ollama, Claude, and Gemini with dynamic context limits and fallback capabilities.
- **Semantic matching & Embeddings**: Vector-based evaluation using pinned local models.
- **Secure handling of untrusted content**: Prompt-injection quarantine, strict network resolver policies (SSRF protections).
- **Playwright browser orchestration**: Headless validation and fallback human-in-the-loop handoffs.
- **Observability**: Live metrics, structured logging, and an operational UI dashboard (React/TypeScript).

### Architecture

```mermaid
graph TD
    A[Discovery Sources] -->|Feed URL| N[pkg/security: Resolver-Bound Network Guard]
    N -->|Validated Fetch| B(pkg/scraper: Funnel Engine)
    B -->|Raw URL| C{cmd/agent: Main Loop}
    
    C -->|Untrusted Posting Text| F[pkg/security: Deterministic Quarantine]
    F -->|Verified Payload| D[pkg/mcp: LLM Client - Ollama/Claude/Gemini]
    D -->|Fit Score > 50| C
    D -->|"Tailored Docs (default: in-process)"| C
    D -.->|"Tailored Docs (only if NLP_SERVICE_URL is set and healthy)"| P[nlp_service: optional FastAPI executor]
    P -.->|Concurrent Ollama Calls| D

    C -->|Auto-Submit Request| E[pkg/submitter: Playwright Pool]
    C -->|Posting Fetch| N
    E -->|Authenticated Loopback Proxy| N
    N -->|Public IP-Bound Dial| J[Public Job Sites]
    E -->|DOM HTML| F
    F -->|Verified Form DOM| D
    D -->|ATS Mapping| E
    
    E -->|Write Status| G[(pkg/storage: SQLite DB w/ WAL & Pooling)]
    C -->|Update Funnel| G
    
    H[HTTP Web Dashboard] -->|Reads Metrics| G
    I[SRE Logging] -.->|Observability| C
```

### Core workflow

**Discover** → **Validate** → **Secure** → **Match** → **Assist** → **Track** → **Observe**

The system operates across three increasingly autonomous modes:
1. **Discover / Find**: Scrapes feeds and discovers potential matches, scoring them against the profile.
2. **Assisted Apply (Recommended)**: Fills in known information and generates authenticity-aware documents, pausing execution for a human to review, solve CAPTCHAs, and submit.
3. **Automatic / Experimental**: Fully headless submission on supported ATS platforms (requires explicit opt-in).

Assisted Apply optimizes for **human seconds per submitted application**, not for
the number of applications submitted automatically. Career Agent does everything
it can safely do — discovery, ranking, documents, form filling, reusing answers
you have approved — and interrupts you only for judgement, unknown information,
authentication, CAPTCHA, legal attestations, final review, and the employer's own
Submit button. An apply session then opens the next application by itself once
you confirm the last one was received.

* **Approved Answer Vault**: answers you explicitly approve are reused
  automatically on equivalent questions, matched deterministically with no model
  in the path. Legal and protected-class declarations are never learned
  silently: Career Agent may suggest, but reuse requires two separate,
  explicit acknowledgements. See `docs/ARCHITECTURE.md`.
* **Application Knowledge**: every unresolved question across your queue,
  deduplicated. Nine applications asking for your Kubernetes experience in nine
  wordings is one thing to answer, and answering it reports what it resolved
  across the rest of the queue. Questions are only grouped by rules you can
  inspect — a curated question family, a skill reduction, or text that differs
  only in presentation — never by similarity, and never by a model.
* **Prepare applications**: Career Agent opens a selected batch once, reads what
  each form asks, and closes it. It fills nothing and submits nothing, and it
  names what it could not inspect and why — a CAPTCHA, a sign-in gate, an expired
  posting — rather than working around it.
* **Exception-only review**: each card leads with what Career Agent completed and
  the short list that needs you. Full diagnostics remain available behind
  expandable details.
* **Apply sessions**: durable in SQLite, so a dashboard refresh resumes rather
  than restarts. A session never advances past an application unless Career
  Agent knows what happened to it — a browser closing without a confirmation
  pauses the session instead of being guessed either way.
* **Application effort**: a band and a range (`Low · ~1–2 min`), never a false
  precise number, capped at ±4% influence on ranking so ease can break ties but
  never outrank fit.
* **Local effort metrics**: median human interaction time, auto-fill rate,
  approved-answer reuse, and unresolved questions per application, computed
  locally and never transmitted.

### Safety model

- **Untrusted web content quarantine**: All fetched text and DOM structures pass through a deterministic prompt-security boundary before any embedding, scoring, or visual model calls.
- **Network restrictions**: Resolver-bound outbound networking prevents SSRF and DNS rebinding attacks by validating all targets against public IP space before dialing.
- **Explicit operator modes**: The system operates as a "fail-closed" architecture. Missing or misconfigured settings default to requiring manual human intervention.
- **Human approval**: Automation naturally halts and hands off to the user when it encounters CAPTCHAs, unfamiliar authentications, or when running in the recommended Copilot mode.
- **Private local storage defaults**: Databases, credentials, and generated PII artifacts are locked down with owner-only (0600) permissions inside protected directories.

### Engineering highlights

* **Fail-closed autonomous actions**: Automatic external actions require explicit operator configuration.
* **Deterministic prompt-security boundary**: Untrusted content is screened before entering model workflows.
* **Resolver-bound networking**: External network access is validated to reduce SSRF / DNS rebinding risk.
* **Stateful workflow orchestration**: Multi-step application flows maintain explicit state and recovery paths.
* **Durable outcome tracking**: IMAP UID checkpoints survive downtime and recover missed events for application outcome tracking.
* **Human-in-the-loop escalation**: Automation stops when human judgment, CAPTCHA, authentication, or review is required.
* **Fail-closed answer memory**: reusable application answers require explicit operator approval, enforced in the store rather than in any caller, so no code path can learn a legal attestation implicitly.
* **Resilient ATS abstraction**: Known ATS handlers coexist with dynamic fallback mapping.
* **Observability**: Operational dashboard and structured logs expose what the agent is doing and why.

### Quick start

Ensure you have Go (1.26.5+) installed.

```bash
# Clone the repository
git clone https://github.com/howlcipher/Career_Agent_Core.git
cd Career_Agent_Core

# Setup synthetic demo profile
cp profile.example.yaml profile.yaml

# Download browser automation dependencies
go run github.com/mxschmitt/playwright-go/cmd/playwright@latest install --with-deps
```

### Configuration & Execution

To fully configure the agent, ensure you copy `pii.yaml.template` to `pii.yaml` and populate it with your personal information. You must also set `CAREER_PROFILE_PATH` in your `.env` or run the agent so it can find your base profile. 

If running locally on limited hardware, consider adjusting `OLLAMA_TIMEOUT_MINUTES` in your `.env` to allow large generation queries to complete.

Run the system in daemon mode to process the discovery queue continuously:
```bash
go run ./cmd/agent --daemon
```

### Demo mode

You can explore the system's logic and the dashboard interface without connecting personal accounts or executing real job applications. The demo mode populates the database with synthetic discovered jobs, scores, and mock outcomes — every company, role, and URL it inserts is fictional, and no network or LLM calls are made.

> **Warning — destructive to an existing database.** The seeder runs `DELETE FROM job_funnel` against `./applications.db` (`pkg/storage.DefaultDatabasePath`) before inserting its synthetic rows, and the path is not configurable. Run it in a fresh clone, or move an existing `applications.db` aside first. It is safe on a first run, where there is nothing to clear.

```bash
# Start the demo seeder
go run ./cmd/demo

# In another terminal, start the dashboard
go run ./cmd/dashboard
```
*Visit `http://127.0.0.1:8080` to explore the dashboard.*

### Technology

- **Backend**: Go (1.26+), SQLite (`modernc.org/sqlite`)
- **AI/LLM**: Local Ollama (default), Anthropic Claude API, Google Gemini API
- **Frontend Dashboard**: TypeScript, React, Vite
- **Automation**: Playwright (Go bindings)
- **Optional Services**: Python (FastAPI) for external NLP tasks

### Project status

Career Agent Core is an **active engineering demonstration and portfolio project**.
- **Production-tested**: Core discovery, validation, RAG matching, state machine tracking, and human-in-the-loop dashboard orchestration are stable and tested.
- **Experimental**: Full headless "Auto-Submit" functionality is experimental due to the constantly evolving nature of third-party platforms and bot protections.

### Documentation

- [Engineering & Portfolio Overview](docs/ENGINEERING_OVERVIEW.md)
- [Deep Architecture Dive](docs/ARCHITECTURE.md)
- [Architecture Decision Records (ADRs)](docs/adrs/)
- [Contributing](CONTRIBUTING.md)
- [Changelog](CHANGELOG.md)
