# Improvements & Feature Requests

This file tracks potential enhancements, architectural improvements, and new feature ideas for the Career_Agent_Core application.

## Planned Improvements
- **DOM Pruning/Minification:** Strip scripts, SVGs, and hidden elements from HTML before sending to Gemini to vastly reduce token usage and improve reasoning quality.
- **Multi-Step Form Logic:** Enhance the JSON mapping to handle "Next" buttons and multi-page application flows (like Workday).
- **Graceful DB Degradation:** If Playwright fails on a cached CSS selector, mark the DB entry as stale/failed so it forces a re-learn on the next attempt instead of infinitely failing.
- **Stealth & Proxies:** Implement `playwright-stealth` or Cloudflare-bypass logic (for Wellfound, Built In, etc.) to prevent headless browsers from being blocked.

## Completed Improvements
- **V2 Architecture Blueprint** implemented.
- **Dynamic Source Discovery:** FunnelEngine dynamically finds ATS URLs instead of static API calls.
- **Playwright Dynamic Generation:** Self-learning DOM mapper implemented. LLM caches Playwright CSS mappings to SQLite for zero-token subsequent submissions.
