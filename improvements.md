# Improvements & Feature Requests

This file tracks potential enhancements, architectural improvements, and new feature ideas for the Career_Agent_Core application.

## Planned Improvements (Phase 2 & V3)
- **Multi-Step Form Logic:** Enhance the JSON mapping to handle "Next" buttons and multi-page application flows (like Workday).
- **Email & Portal Application Tracking:** Build a `pkg/tracker` module to sync with Gmail/IMAP and ATS portals to track interviews and rejections, calculating conversion rates.
- **Visual Reasoning (VLM) for Form Submissions:** Pass Playwright screenshots to Gemini-1.5-Pro's Vision model to visually locate buttons and inputs, making the agent immune to HTML obfuscation and Shadow DOMs.
- **Adaptive Resume A/B Testing:** Generate and track multiple resume tones (e.g. Technical, Leadership) and automatically pivot to the version yielding the highest interview callback rate.
- **Expansion to Niche Data Sources:** Build custom scrapers for YCombinator "Work at a Startup", Otta, Wellfound, and HackerNews "Who is Hiring".
- **Local LoRA Fine-Tuning Loop:** Collect scored jobs into a dataset and run periodic fine-tuning on a local LLM to learn the user's specific preferences, eliminating API costs.
- **Automated Assessment/Screening Solver:** Use Gemini to dynamically read pre-screening multiple-choice questions on applications and extract the correct numeric/radio answers strictly from `USER_PROFILE.md`.
- **CAPTCHA & Anti-Bot Solving:** Integrate `2captcha` or `capsolver` APIs to autonomously bypass hard visual/audio challenges when Playwright stealth fails.
- **Metrics Dashboard (TUI/Web):** Build a sleek Terminal UI or local React dashboard to visually track the application funnel (Discovered -> Scored -> Applied -> Interview) rather than manually querying SQLite.
- **Cron-Driven Drip Campaigns:** Shift from massive batch processing to a persistent background daemon that drips 10-15 applications every 6 hours to simulate organic human behavior and avoid ATS IP flagging.

## Completed Improvements
- **V2 Architecture Blueprint** implemented.
- **Dynamic Source Discovery:** FunnelEngine dynamically finds ATS URLs instead of static API calls.
- **Playwright Dynamic Generation:** Self-learning DOM mapper implemented. LLM caches Playwright CSS mappings to SQLite for zero-token subsequent submissions.
- **DOM Pruning/Minification:** Implemented `PruneDOM` utility to strip scripts, SVGs, and hidden elements from HTML, vastly reducing token usage.
- **Graceful DB Degradation (Self-Healing):** Execution loop instantly invalidates and deletes stale SQLite cache mappings if Playwright fails, forcing a live LLM regeneration.
- **Stealth & Proxies:** Injected `navigator.webdriver` overrides and AutomationControlled flag bypasses into Chromium to evade anti-bot checks.
- **Playwright Scraper Fallback:** Implemented DuckDuckGo Playwright scraper to flawlessly handle SerpApi quota limits without dropping queries.
