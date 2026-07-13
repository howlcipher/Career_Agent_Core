# Career Agent Core

Career Agent Core is an autonomous AI-driven job application engine written in Go. It discovers remote jobs, filters them against your strict salary and career requirements, and utilizes Gemini Pro to write highly tailored resumes and cover letters using your central AI Knowledge Library.

## Features
- **Live Scraping**: Aggregates 100% remote jobs directly from the RemoteOK API.
- **AI Tailoring**: Connects to the Gemini 1.5 Pro API to analyze job descriptions and synthesize them with your `USER_PROFILE.md`.
- **Strict Rule Enforcement**: Dynamically discards jobs that don't meet your salary floor or remote requirements defined in `profile.yaml`.
- **Security Quarantine**: Implements a prompt injection quarantine layer via `promptsec` to prevent hostile job postings from manipulating the AI.
- **Blocklist**: Automatically skips current and past employers to prevent awkward application scenarios.
- **Auto-Submit Framework**: Architecture in place to integrate Playwright for headless browser submission.
- **Manual Backlog**: Jobs that fail auto-submission are gracefully logged to an actionable markdown checklist.

## Getting Started

1. **Configure your profile**
Modify the `profile.yaml` file to set your salary floor, target role, and blocklisted companies.

2. **Authenticate Gemini**
The agent uses Gemini Pro for generation. You must export your API key before running:
```bash
export GEMINI_API_KEY="your_api_key_here"
```

3. **Run the Agent**
Fire up the CLI:
```bash
go run cmd/agent/main.go
```

The agent will populate the `applications/` folder with a customized Markdown resume, plain text cover letter, and a `metadata.json` for every matching job.

## Managing Submissions
If `auto_submit: true` is enabled in your config but the agent cannot parse the employer's Applicant Tracking System (ATS), the job will be gracefully added to `applications/manual_submissions.md` as a checklist for you to submit manually using the generated documents.
