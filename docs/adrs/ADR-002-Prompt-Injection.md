# ADR-002: Prompt Injection Quarantine & SSRF Filters

## Status
Accepted

## Context
When interacting with untrusted third-party job boards, the application dynamically parses HTML (DOM) and feeds it into the Gemini API to extract form mappings. Adversaries could embed prompt injections (e.g., `<!-- Ignore previous instructions -->`) into fake job postings. Additionally, headless browsers used for scraping are susceptible to Server-Side Request Forgery (SSRF) if a URL redirects to an internal network block (e.g., AWS Metadata IP `169.254.169.254`).

## Decision
1. **SSRF Filter:** Implemented a Playwright-level route interceptor (`page.Route("**/*")`) that aborts any network requests matching `localhost`, `127.0.0.1`, `169.254.169.254`, or `0.0.0.0`.
2. **Prompt Injection Quarantine:** Integrated a `security.QuarantineLayer` that filters and validates the raw DOM text *before* passing it to the LLM backend for mapping generation.

## Consequences
**Positive:**
- Prevents headless browser from being weaponized against local internal infrastructure.
- Neutralizes prompt injection payloads embedded in raw DOM.

**Negative:**
- Adds slight latency to the page evaluation process.
- Strict string matching on the SSRF filter may need periodic updates to encompass all private CIDR blocks safely.
