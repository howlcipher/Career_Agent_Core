# Career Agent Core

Career Agent Core is an autonomous AI-driven job application engine written in Go. It discovers remote jobs, filters them against your strict salary and career requirements, and utilizes Gemini Pro to write highly tailored resumes and cover letters using your central AI Knowledge Library.

## Features
- **Massive Discovery Engine**: Scrapes Google/Yahoo dorks targeting 16 major Applicant Tracking Systems (Greenhouse, Workday, Lever, Jobvite, BambooHR, etc) using fuzzy keyword matching.
- **Tech-Stack Agnostic Fit Score**: Uses Gemini to evaluate job descriptions against your profile and constraints (Salary/Remote). Only proceeds if the fit score is 50 or higher. Evaluates based on core competencies, not strict language matching.
- **AI Tailoring**: Connects to the Gemini 1.5 Pro API to analyze job descriptions and synthesize them with your `USER_PROFILE.md`.
- **Stealth Writer**: The system prompt is engineered with strict humanizing constraints (banning words like "delve", "tapestry", "synergy") and high burstiness to completely bypass AI detection.
- **Interview Cheat Sheet**: Automatically generates an `interview_prep.md` alongside your resume containing likely interview questions and tailored talking points.
- **SQLite Application Tracking**: Locally tracks applied jobs in `applications.db` to ensure you never accidentally apply to the same job twice.
- **Strict Rule Enforcement**: Dynamically discards jobs that don't meet your salary floor or remote requirements defined in `profile.yaml`.
- **Security Quarantine**: Implements a prompt injection quarantine layer via `promptsec` to prevent hostile job postings from manipulating the AI.
- **Blocklist**: Automatically skips current and past employers to prevent awkward application scenarios.
- **Auto-Submit Framework**: Architecture in place to integrate Playwright for headless browser submission (currently targets LinkedIn Easy Apply).
- **Email Tracker**: Actively scans your IMAP Gmail inbox for rejections and interview requests, updating your funnels automatically.
- **Live Metrics Dashboard**: Ships with a beautifully formatted zero-dependency Terminal UI (TUI) to track your live conversion rates.
- **Cron-Driven Daemon Mode**: Avoids ATS IP bans by continuously dripping 10-15 applications out every 6 hours in the background.
- **Playwright Fallback Scraper**: Bypasses SerpApi limits by deploying an undetectable headless DuckDuckGo scraper with `navigator.webdriver` evasion when API credits run out.
- **Cost & Token Optimization**: Drastically prunes DOM footprints (removing CSS, SVGs, scripts) before interacting with Gemini, ensuring payloads remain under ~1,500 characters to prevent budget blowouts and quota exhaustion.
- **Dynamic Learner Module**: When the agent encounters an unknown Applicant Tracking System (like Workday or Breezy), it automatically sends the raw form to Gemini to mathematically map the input selectors. It caches this learned blueprint in SQLite and successfully submits the application, theoretically supporting any job board on the internet.
- **Strict ATS URL Validation**: Implements strict `net/url` parsing and hostname whitelist validation to guarantee search engine redirects, spam, and recruiter blogs never make it into the evaluation pipeline, saving 100% of LLM token spend on junk URLs.
- **Resilient Networking**: All LLM API calls are wrapped in strict 60-second context timeouts to prevent workers from hanging indefinitely during network blips or silent connection drops.
- **Self-Healing DOM Cache**: Instantly clears stale Playwright CSS mappings if a website updates its UI, forcing the LLM to learn the new layout on the next run.

## Requirements
- **Go 1.21+**
- **Playwright System Dependencies**: Headless auto-submission requires specific system libraries (like `libicu`). On standard Linux, run the following to install the necessary dependencies before starting the agent:
  ```bash
  go run github.com/playwright-community/playwright-go/cmd/playwright@latest install --with-deps
  ```

### Running on Immutable OS (Bazzite / Fedora Silverblue / SteamOS)
If you are running an immutable atomic OS where the root filesystem is read-only, you cannot natively install Playwright's system C libraries (like `libX11` or `libicu`). Instead, use **Distrobox** to safely run the agent in a container while maintaining full access to your host filesystem:

1. Open a terminal and create an Ubuntu container:
   ```bash
   distrobox create --name career-agent --image ubuntu:22.04
   distrobox enter career-agent
   ```
2. Once inside the container, install the base dependencies:
   ```bash
   sudo apt-get update && sudo apt-get install -y golang-go nodejs npm
   ```
3. Run the Playwright installer and the agent from inside the container:
   ```bash
   cd ~/dev/Career_Agent_Core
   npx playwright install-deps
   go run cmd/agent/main.go
   ```
*(Note: Because Distrobox perfectly mirrors your home folder, you can run the TUI Dashboard `go run cmd/dashboard/main.go` natively on your host OS and it will instantly read the database updates being written by the containerized agent!)*


## Getting Started (How to Use)

Follow these steps immediately after cloning the repository:

### 1. Set Up Your Personal Identifiable Information (PII)
To protect your sensitive data from version control, your email, phone, and address are handled locally:
1. Copy the template: `cp pii.yaml.template pii.yaml`
2. Open `pii.yaml` and fill in your actual contact details. 
*(Note: `pii.yaml` is intentionally tracked in `.gitignore` so your personal data is never pushed to GitHub).*

### 2. Configure Your Profile & Toggles
Open `profile.yaml` to customize your search parameters:
- **`salary_floor`**: Your absolute lowest acceptable base pay.
- **`target_compensation`**: The ideal number the AI will negotiate or enter into application fields.
- **`roles`**: An array of explicit job titles the system will actively scrape for.
- **`auto_submit_click`**: Set to `true` to have the bot physically click "Submit Application" on Greenhouse/Lever ATS platforms. Set to `false` to have it fill out the form and wait for you to review it.
- **`headless_browser`**: Set to `true` to run the bot silently in the background, or `false` to watch it operate visibly.

### 3. Ensure Your Context Exists
The AI relies on a base resume or profile to tailor against job descriptions. Ensure you have your base markdown profile (e.g., `USER_PROFILE.md`) accessible to the system or a fallback `__William_Elias_Resume__.pdf` in the root directory.

### 4. Authenticate APIs & Mail
The agent requires Gemini and IMAP credentials. Edit your `.env` file (never commit this to Git):
```bash
GEMINI_API_KEY="your_api_key_here"
SERPAPI_API_KEY="your_serpapi_key"
IMAP_SERVER="imap.gmail.com:993"
IMAP_USER="your_email@gmail.com"
IMAP_APP_PASSWORD="your_16_digit_app_password"
```

### 5. Launch the Suite
The Core Agent can be run in batch or daemon mode:
```bash
# Run one massive batch and exit
go run cmd/agent/main.go

# Run continuously as a background service (drip mode)
go run cmd/agent/main.go --daemon
```

*Note: On its very first run, Playwright will automatically download the necessary Chromium browser binaries.*

While the agent runs, open a new terminal window to view your live stats:
```bash
go run cmd/dashboard/main.go
```

To enable auto-tracking of employer rejections and interview requests, launch the Email Tracker in the background:
```bash
go run cmd/tracker/main.go
```

## Managing Submissions
If `auto_submit: true` is enabled in your config but the agent encounters a non-standard Applicant Tracking System (ATS), it will intelligently fall back to dynamic Playwright generation or gracefully add the job to `applications/manual_submissions.md` as a checklist for you to submit manually using the generated documents.
