# Career Agent Core: Engineering Overview

This document is intended for technical recruiters, hiring managers, AI platform engineering teams, and automation engineers. It answers what Career Agent Core is, why it was built, and what it demonstrates.

## The Problem

Most job application bots focus entirely on quantity: bypassing CAPTCHAs, spamming APIs, and fabricating "optimized" resume text that ultimately sounds generic. This approach doesn't work, and it isn't interesting from a software engineering perspective.

Instead, I asked a different question:

> What does a trustworthy agentic workflow look like when an AI system must discover external information, reason about it, interact with unreliable third-party systems, maintain durable state, recover from failure, and know when to hand control back to a human?

## The Architecture

Career Agent Core is an agentic workflow and automation platform built primarily in **Go**.

It implements a pipeline where unstructured data is discovered from the web, secured, evaluated by pinned local embedding models, processed by an LLM (Ollama, Claude, or Gemini) for context-aware tailoring, and finally pushed to third-party Applicant Tracking Systems (ATS) via Playwright browser orchestration.

### Major Engineering Challenges Solved

#### 1. Security & Untrusted Input
Web content (job descriptions, HTML forms) is inherently untrusted.
- **Prompt Injection Defense:** Text and DOM structures are quarantined and passed through a deterministic security boundary. If the payload is flagged, the job enters a terminal failure state.
- **Network Boundaries (SSRF Prevention):** All outbound network requests resolve hostnames upfront. If any IPv4 or IPv6 address maps to internal/private IP space, the request is rejected before a socket is dialed. Browser instances are piped through an authenticated loopback proxy to prevent DNS rebinding attacks.

#### 2. Human-in-the-Loop & Failure Escalation
Full headless automation is fragile on the modern web due to bot protections (Cloudflare) and unpredictable UI changes.
- Instead of pretending the bot can bypass everything, the architecture **fails gracefully**.
- When the agent encounters a CAPTCHA, complex authentication, or operates in its default **Assisted Apply** mode, it completes all data entry and pauses.
- The state transitions to `AWAITING_REVIEW`, moving the job to a queue for the operator to physically review and click submit.

#### 3. Stateful Workflow Orchestration
Application flows require multi-step navigation (e.g., Next -> Next -> Submit).
- The pipeline processes these forms using a state-machine driven graph architecture.
- SQLite is used in WAL mode for durable persistence, ensuring that if the agent crashes during an expensive generation or submission step, the state is protected and gracefully recovered on restart.

#### 4. Resilient & Cost-Efficient AI Engineering
LLM calls are slow and expensive (or computationally heavy locally).
- **Early Rejection:** Missing job descriptions undergo pre-flight validation. Jobs that don't meet hard criteria (e.g., missing remote tags) are rejected via quick heuristics before any tokens are spent.
- **Dynamic Context Pruning:** Playwright strips out scripts, SVGs, and CSS rules from the DOM before sending it to the LLM for schema mapping.
- **Microservice Fallback:** An optional Python FastAPI NLP service is provided. The Go agent can offload generation to free up the primary worker pool, falling back to in-process execution if the microservice is down.

## What this demonstrates professionally

1. **System Design:** Creating resilient boundaries around third-party dependencies, scaling throughput with worker pools, and optimizing for error recovery.
2. **AI Integration:** Moving past trivial API wrappers. It showcases real LLM orchestration—dealing with context sizes, model fallback, vision models for GUI mapping, and vector-based semantic search.
3. **Observability:** Building an operational React/TypeScript dashboard to surface funnel metrics, failure rates, and live system status using structured SQLite data.
4. **Pragmatism:** Understanding that AI doesn't solve every problem magically. Knowing when to use deterministic Go code, and when to let the LLM handle fuzziness. Knowing when full automation is appropriate, and when a human should be in the loop.

## Technologies Used
- **Backend:** Go (1.26+), `modernc.org/sqlite`
- **Frontend:** TypeScript, React, Vite
- **AI/ML:** Local Ollama (llama3.1, llava, nomic-embed-text), Anthropic Claude, Google Gemini APIs
- **Automation:** Playwright (headless and visible)
- **Optional External Services:** Python (FastAPI)

## Safe Demo

If you are evaluating this repository locally, you do not need to configure real credentials to explore its capabilities. You can seed synthetic test data by running:

```bash
go run ./cmd/demo
```

Then explore the dashboard with:

```bash
go run ./cmd/dashboard
```
