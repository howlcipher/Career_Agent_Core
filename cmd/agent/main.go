package main

import (
	"errors"
	"flag"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"context"
	"fmt"
	"github.com/howlcipher/Career_Agent_Core/pkg/config"
	"github.com/howlcipher/Career_Agent_Core/pkg/mcp"
	"github.com/howlcipher/Career_Agent_Core/pkg/parser"
	"github.com/howlcipher/Career_Agent_Core/pkg/scraper"
	"github.com/howlcipher/Career_Agent_Core/pkg/security"
	"github.com/howlcipher/Career_Agent_Core/pkg/storage"
	"github.com/howlcipher/Career_Agent_Core/pkg/submitter"
	"github.com/howlcipher/Career_Agent_Core/pkg/tracker"
	"github.com/joho/godotenv"
	"github.com/mxschmitt/playwright-go"
	"gopkg.in/natefinch/lumberjack.v2"
	"io"
	"net/http"
	"net/url"
	"os/signal"
	"sync"
	"syscall"
)

const (
	// masterResumePath is the file actually uploaded to every ATS. The
	// pipeline's per-job "tailored" resume is a saved reference document, not
	// the upload payload — this static PDF is what employers receive.
	masterResumePath = "master_resume.pdf"
	// defaultMasterCoverLetterPath is the fallback for the single job-agnostic
	// cover letter reused for every application when profile.yaml sets
	// use_master_cover_letter, applied only when master_cover_letter_path is
	// left unset. Gitignored alongside master_resume.pdf, as any real letter
	// carries contact details.
	defaultMasterCoverLetterPath = "master_cover_letter.txt"
)

// parseTargetJobURLs splits TARGET_JOB_URL's comma-separated value into a
// membership set. Returns nil (not an empty map) for an empty/unset input,
// so callers can distinguish "no filter" from "filter matches nothing" with
// a plain len() check.
func parseTargetJobURLs(raw string) map[string]bool {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	urls := make(map[string]bool)
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			urls[part] = true
		}
	}
	return urls
}

// clientRenderedSPAHosts lists ATS platforms whose job content is rendered
// entirely client-side, so a bare (non-JS-executing) net/http fetch always
// sees an empty shell regardless of whether the posting is actually
// reachable — confirmed live 2026-07-24 on Ashby (see the captcha-check
// comment below). Matched as host suffixes, same convention as
// authGatedATSHosts in pkg/submitter/browser.go.
var clientRenderedSPAHosts = []string{
	"ashbyhq.com",
}

func isClientRenderedSPAHost(rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	host := strings.ToLower(u.Hostname())
	for _, spa := range clientRenderedSPAHosts {
		if host == spa || strings.HasSuffix(host, "."+spa) {
			return true
		}
	}
	return false
}

func main() {
	if err := security.PreparePrivateWorkspace(".", os.Stderr); err != nil {
		log.Fatalf("Startup aborted because private paths could not be secured: %v", err)
	}

	daemonMode := flag.Bool("daemon", false, "Run in persistent background drip mode")
	flag.Parse()

	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found or error loading it. Relying on system environment variables.")
	}

	// Setup rotating logs
	log.SetOutput(&lumberjack.Logger{
		Filename:   "career_agent.log",
		MaxSize:    10, // megabytes
		MaxBackups: 3,
		MaxAge:     28, // days
	})

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	log.Println("[Agent] Initializing Career Agent Core...")
	if *daemonMode {
		log.Println("[Agent] [DAEMON MODE] Agent will drip applications every 6 hours to evade ATS IP bans.")
	}

	if err := storage.InitDB(); err != nil {
		log.Fatalf("Failed to initialize SQLite database: %v", err)
	}
	defer storage.CloseDB()

	// Any row still PROCESSING at startup can only be orphaned from a
	// previous run being killed mid-job (confirmed live 2026-07-24: 235
	// rows accumulated over three days) -- this fresh process hasn't
	// touched anything yet, so none of them can be its own.
	if reaped, err := storage.ReapStaleProcessingJobs(); err != nil {
		log.Printf("[Agent] Failed to reap stale PROCESSING rows: %v", err)
	} else if reaped > 0 {
		log.Printf("[Agent] Reset %d stale PROCESSING row(s) (orphaned by a previous run) back to DISCOVERED.", reaped)
	}
	if err := playwright.Install(); err != nil {
		log.Fatalf("Failed to install Playwright: %v", err)
	}

	pw, err := playwright.Run()
	if err != nil {
		log.Fatalf("could not start playwright: %v", err)
	}
	defer pw.Stop()

	prof, err := config.LoadProfile("profile.yaml")
	if err != nil {
		log.Fatalf("Configuration error: %v", err)
	}

	browser, err := pw.Chromium.Launch(playwright.BrowserTypeLaunchOptions{
		Headless: playwright.Bool(prof.HeadlessBrowser),
		Args: []string{
			"--disable-blink-features=AutomationControlled",
			"--disable-infobars",
			"--disable-dev-shm-usage",
			"--no-sandbox",
		},
	})
	if err != nil {
		log.Fatalf("could not launch browser: %v", err)
	}
	defer browser.Close()

	log.Printf("[Agent] Loaded profile: roles=%v, salary_floor=%d", prof.Roles, prof.SalaryFloor)

	piiData, err := config.LoadPII("pii.yaml")
	if err != nil {
		log.Printf("[Agent] PII warning (defaulting to empty fields): %v", err)
		piiData = &config.PII{}
	}

	// improvements.md #32: let the submitter retrieve one-time codes an ATS
	// emails mid-application (bugs.md #93). Reuses the IMAP credentials the
	// email tracker already uses -- no OAuth, no extra setup. Left unset when
	// they are absent, in which case a code gate routes to MANUAL_REQUIRED
	// exactly as before.
	if u, pw, srv := os.Getenv("IMAP_USER"), os.Getenv("IMAP_APP_PASSWORD"), os.Getenv("IMAP_SERVER"); u != "" && pw != "" && srv != "" {
		imapCfg := tracker.IMAPConfig{Server: srv, Username: u, Password: pw}
		submitter.SecurityCodeFetcher = func(notBefore time.Time) (string, error) {
			code, subject, err := tracker.FetchSecurityCode(imapCfg, notBefore)
			if err != nil {
				return "", err
			}
			if code != "" {
				// Never log the code itself -- it is a live credential for an
				// application in flight.
				log.Printf("[Agent] Retrieved a security code from email (subject: %q)", subject)
			}
			return code, nil
		}
		log.Println("[Agent] Security-code retrieval enabled (IMAP).")
	} else {
		log.Println("[Agent] Security-code retrieval disabled (no IMAP credentials); code-gated forms will route to manual submission.")
	}

	filter := security.NewQuarantineLayer()
	jobChan := make(chan scraper.Job, 2000)

	// TARGET_JOB_URL restricts this run to a specific set of already-
	// DISCOVERED jobs and skips fresh FunnelEngine discovery, for verifying
	// a fix end-to-end in minutes instead of waiting on normal queue order
	// or disturbing a separately-running full batch (bugs.md's Operational
	// Trap section — used live 2026-07-23 to verify bugs #46-#49). Accepts
	// a comma-separated list (2026-07-24: generalized from a single URL to
	// re-verify a whole batch of requeued jobs, e.g. bug #53's affected
	// APPLIED rows, without waiting on normal source-priority ordering).
	// Every target job must already be in DISCOVERED status (see
	// cmd/requeue), and if it previously reached document generation, its
	// applied_jobs dedup row must be cleared too or HasApplied will skip it
	// as a duplicate.
	targetJobURLs := parseTargetJobURLs(os.Getenv("TARGET_JOB_URL"))

	discoveredJobs, err := storage.GetDiscoveredJobs()
	var producerWg sync.WaitGroup

	if err == nil {
		producerWg.Add(1)
		go func() {
			defer producerWg.Done()
			matched := 0
			for _, dj := range discoveredJobs {
				if len(targetJobURLs) > 0 && !targetJobURLs[dj.URL] {
					continue
				}
				matched++
				jobChan <- scraper.Job{
					CompanyName: dj.CompanyName,
					Title:       dj.JobTitle,
					URL:         dj.URL,
					Salary:      prof.TargetComp,
					Remote:      true,
				}
			}
			if len(targetJobURLs) > 0 {
				log.Printf("[Agent] TARGET_JOB_URL set: loaded %d matching job(s) (of %d discovered, %d targeted) into the queue.", matched, len(discoveredJobs), len(targetJobURLs))
			} else {
				log.Printf("[Agent] Loaded %d previously discovered jobs from backlog into the queue.", len(discoveredJobs))
			}
		}()
	}

	funnelEngine := scraper.NewFunnelEngine(prof.Roles)
	producerWg.Add(1)
	go func() {
		defer producerWg.Done()
		if len(targetJobURLs) > 0 {
			return
		}
		if err := funnelEngine.DiscoverJobs(jobChan); err != nil {
			log.Printf("[Agent] Funnel discovery error: %v", err)
		}
	}()

	go func() {
		producerWg.Wait()
		close(jobChan)
	}()

	client := mcp.NewClient(os.Getenv("GEMINI_API_KEY"))
	pipeline := submitter.NewPipeline(filter, client, client, browser)

	// Local Embedded RAG Ingestion
	const profilePath = "/var/home/howlcipher/dev/ai_knowledge_library/USER_PROFILE.md"
	existingChunks, err := storage.GetAllCareerChunks()
	if err != nil {
		log.Printf("[Agent] [RAG] Failed to get career chunks from storage: %v", err)
	}
	needsIngest := len(existingChunks) == 0
	if !needsIngest {
		// A changed OLLAMA_EMBED_MODEL (or provider switch) silently breaks
		// CosineSimilarity-based retrieval otherwise — its own mismatched-size
		// guard returns 0 for every comparison with no error surfaced. Probe
		// the currently configured model's actual dimension and compare
		// against what's already stored (bugs.md: "stale RAG chunk dimension").
		if probeEmb, err := client.GetEmbedding("dimension probe"); err != nil {
			log.Printf("[Agent] [RAG] Failed to probe embedding dimension: %v", err)
		} else if parser.CareerChunksNeedReingest(existingChunks, len(probeEmb)) {
			log.Printf("[RAG] Stored career chunks use a different embedding dimension than the current model (%d-dim) — re-ingesting.", len(probeEmb))
			needsIngest = true
		}
	}
	if needsIngest {
		log.Println("[RAG] Knowledge Library cache empty or stale. Ingesting USER_PROFILE.md into local SQLite Vector DB...")
		n, err := parser.IngestResumeChunks(client.GetEmbedding, profilePath)
		if err != nil {
			log.Printf("[RAG] Failed to ingest resume chunks: %v", err)
		} else {
			log.Printf("[RAG] Successfully embedded and cached %d career chunks.", n)
		}
	} else {
		log.Printf("[RAG] Found %d career chunks in local SQLite Vector DB.", len(existingChunks))
	}

	var wg sync.WaitGroup
	numWorkers := defaultWorkerCount()
	if raw := os.Getenv("WORKER_COUNT"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			numWorkers = n
		} else {
			log.Printf("[Config] Ignoring invalid WORKER_COUNT=%q, using %d", raw, numWorkers)
		}
	}
	log.Printf("[Config] Using %d worker(s)", numWorkers)

	for w := 1; w <= numWorkers; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for job := range jobChan {
				select {
				case <-ctx.Done():
					log.Printf("[Worker-%d] Shutting down gracefully...", workerID)
					return
				default:
				}
				// Stale backlog rows predate the discovery filters (bugs.md #22), so
				// known-junk URLs must be caught again at intake or they burn full
				// scoring/tailoring/Vision cycles on every restart.
				if scraper.IsKnownJunkJobURL(job.URL) {
					log.Printf("[Worker-%d] Skipping known-junk URL (never a posting): %s", workerID, job.URL)
					storage.UpdateFunnelStatus(job.URL, "INVALID_URL")
					continue
				}
				storage.UpdateFunnelStatus(job.URL, "PROCESSING")
				// The LLM will perform the real analysis of fit, salary, and remote status based on the job description.
				// We only need to enforce the hard blocklist here.
				nameLower := strings.ToLower(job.CompanyName)
				excluded := false
				for _, ex := range prof.ExcludeCompanies {
					if strings.Contains(nameLower, strings.ToLower(ex)) {
						log.Printf("[Worker-%d] Security Block: Skipping %s (Found in ExcludeCompanies blocklist)", workerID, job.CompanyName)
						excluded = true
						break
					}
				}
				if excluded {
					storage.UpdateFunnelStatus(job.URL, "SKIPPED")
					continue
				}

				// Fetch the job description if it's missing (which is the case for all Yahoo/SerpApi funnel jobs)
				if job.Description == "" {
					log.Printf("[Worker-%d] Fetching job description for %s...", workerID, job.CompanyName)
					u, err := url.Parse(job.URL)
					if err != nil || u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1" || u.Hostname() == "169.254.169.254" {
						log.Printf("[Worker-%d] Invalid or unsafe URL blocked: %s", workerID, job.URL)
						// bugs.md #85: give the row a terminal status. A bare continue
						// left it PROCESSING forever, invisible to GetDiscoveredJobs.
						storage.UpdateFunnelStatus(job.URL, "INVALID_URL")
						continue
					}

					httpClient := &http.Client{
						Timeout: 10 * time.Second,
						CheckRedirect: func(req *http.Request, via []*http.Request) error {
							if req.URL.Hostname() == "localhost" || req.URL.Hostname() == "127.0.0.1" || req.URL.Hostname() == "169.254.169.254" {
								return fmt.Errorf("redirect to internal IP blocked")
							}
							if len(via) >= 10 {
								return fmt.Errorf("stopped after 10 redirects")
							}
							return nil
						},
					}
					req, err := http.NewRequest("GET", job.URL, nil)
					if err != nil {
						log.Printf("[Worker-%d] Failed to create request for %s: %v", workerID, job.CompanyName, err)
						// bugs.md #85: transient — undo the PROCESSING claim so a later
						// run can pick it up, rather than stranding it.
						storage.UpdateFunnelStatus(job.URL, "DISCOVERED")
						continue
					}
					req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
					resp, err := httpClient.Do(req)
					if err == nil {
						defer resp.Body.Close()
						b, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
						if err != nil {
							log.Printf("[Worker-%d] Failed to read response body for %s: %v", workerID, job.CompanyName, err)
							// bugs.md #85: transient — undo the PROCESSING claim.
							storage.UpdateFunnelStatus(job.URL, "DISCOVERED")
							continue
						}
						htmlStr := string(b)

						pruned, err := parser.PruneDOMToText(htmlStr)
						if err != nil {
							log.Printf("[Worker-%d] Failed to prune DOM for %s: %v", workerID, job.CompanyName, err)
						}

						// Captcha / Bot protection check. A bare "recaptcha"/
						// "cf-turnstile" substring match is not reliable proof of an
						// actual block on its own (bug #46, same class as bug #45's
						// fix to pkg/submitter/browser.go's isCaptchaBlocked): these
						// anti-spam widgets are standard on legitimate Greenhouse/
						// Lever/Ashby/Workable job pages, and this check was killing
						// the large majority of real postings on those platforms
						// before they ever reached fit-scoring. A genuine
						// interstitial instead replaces the real page content,
						// leaving little real text behind once pruned to plain text
						// — require that corroborating signal for the widget-only
						// phrases too, same as the explicit Cloudflare phrasing.
						lowerHTML := strings.ToLower(htmlStr)
						genuineBlockPhrasing := strings.Contains(lowerHTML, "cloudflare") && (strings.Contains(lowerHTML, "verify you are human") || strings.Contains(lowerHTML, "attention required"))
						// Bug: the "little real text behind" corroborating signal
						// assumes a legitimate page has server-rendered visible text
						// in a bare (non-JS-executing) fetch. Confirmed live
						// 2026-07-24 on two real, unblocked, currently-open Ashby
						// postings: raw HTML ~42KB, 0 chars of visible text after
						// pruning, "recaptcha" present in its script bundle — Ashby
						// renders everything client-side, so this exact shape is
						// what *every* Ashby posting looks like to a non-JS fetch,
						// genuinely blocked or not. This check cannot tell the two
						// apart without executing JavaScript, so for known
						// client-rendered platforms, only the explicit block
						// phrasing above is trusted — same reasoning as
						// authGatedATSHosts in pkg/submitter/browser.go.
						widgetOnlyPhrasing := !isClientRenderedSPAHost(job.URL) && (strings.Contains(lowerHTML, "recaptcha") || strings.Contains(lowerHTML, "cf-turnstile"))
						if genuineBlockPhrasing || (widgetOnlyPhrasing && len(strings.TrimSpace(pruned)) < 200) {
							log.Printf("[Worker-%d] Security/Captcha block detected for %s. Skipping job to save API tokens.", workerID, job.CompanyName)
							storage.UpdateFunnelStatus(job.URL, "BLOCKED_CAPTCHA")
							continue
						}

						job.Description = pruned
					} else {
						log.Printf("[Worker-%d] Failed to fetch job description for %s: %v", workerID, job.CompanyName, err)
					}
				}

				if storage.HasApplied(job.URL) {
					log.Printf("[Worker-%d] Duplicate check: Already applied to %s. Skipping.", workerID, job.CompanyName)
					// bugs.md #85: undo the PROCESSING claim rather than stranding the
					// row. Deliberately DISCOVERED and not APPLIED: the applied_jobs
					// record is written at document generation, not at confirmed
					// submission (the very falsehood this 82-job re-verification
					// exists to audit -- see #53), so asserting APPLIED here would
					// manufacture exactly the claim under investigation. This restores
					// the pre-PROCESSING state, matching what the startup reaper
					// already does, and makes no new claim about the job.
					storage.UpdateFunnelStatus(job.URL, "DISCOVERED")
					continue
				}

				scrapedData := map[string]string{
					"title": job.Title,
					"desc":  job.Description,
				}

				// RAG Retrieval: Dynamically build tailored context
				jobDescText := job.Title + "\n" + job.Description

				var jobEmb []float32
				var embErr error
				for attempt := 1; attempt <= 3; attempt++ {
					jobEmb, embErr = client.GetEmbedding(jobDescText)
					if embErr == nil {
						break
					}
					if strings.Contains(embErr.Error(), "connect:") || strings.Contains(embErr.Error(), "no route to host") || strings.Contains(embErr.Error(), "429") || strings.Contains(embErr.Error(), "deadline exceeded") {
						log.Printf("[Worker-%d] Network or Rate Limit error getting embedding (attempt %d/3). Sleeping 60s...", workerID, attempt)
						time.Sleep(60 * time.Second)
					} else {
						break
					}
				}

				var tailoredContext string
				if embErr == nil {
					topChunks, _ := parser.RetrieveTopK(jobEmb, 5)
					var sb strings.Builder
					sb.WriteString("Highly Relevant Career Context (Retrieved via RAG):\n\n")
					for _, tc := range topChunks {
						sb.WriteString(tc.Text + "\n\n")
					}
					tailoredContext = sb.String()
				} else {
					log.Printf("[RAG] Embedding failed after retries, falling back to empty context: %v", embErr)
				}

				if err := filter.CheckPayload(tailoredContext); err != nil {
					log.Printf("[Worker-%d] Security quarantine triggered on RAG output: %v", workerID, err)
					continue
				}

				toneVariantLabel, selectedTone, hasToneVariant := config.SelectToneVariant(prof.CoverLetterTones)
				coverLetterTone := prof.CoverLetterTone
				if hasToneVariant {
					coverLetterTone = selectedTone
				}

				profileConstraints := map[string]interface{}{
					"salary_floor":        prof.SalaryFloor,
					"target_compensation": prof.TargetComp,
					"remote_only":         prof.RemoteOnly,
					"cover_letter_tone":   coverLetterTone,
					"location":            piiData.Address,
				}

				var score int
				var scoreErr error
				for attempt := 1; attempt <= 3; attempt++ {
					score, scoreErr = client.ScoreJob(scrapedData, profileConstraints, tailoredContext)
					if scoreErr == nil {
						break
					}
					if strings.Contains(scoreErr.Error(), "429") || strings.Contains(scoreErr.Error(), "Quota exceeded") {
						log.Printf("[Worker-%d] CRITICAL: Gemini API Daily Quota Exceeded scoring job %s. Shutting down agent...", workerID, job.CompanyName)
						cancel()
						return
					} else if strings.Contains(scoreErr.Error(), "connect:") || strings.Contains(scoreErr.Error(), "no route to host") || strings.Contains(scoreErr.Error(), "deadline exceeded") {
						log.Printf("[Worker-%d] Network error scoring job %s (attempt %d/3). Sleeping 60s...", workerID, job.CompanyName, attempt)
						time.Sleep(60 * time.Second)
					} else {
						break
					}
				}

				if scoreErr != nil {
					log.Printf("[Worker-%d] Failed to score job for %s after retries: %v", workerID, job.CompanyName, scoreErr)
					storage.UpdateFunnelStatus(job.URL, "FAILED_SCORE")
					time.Sleep(1 * time.Second)
					continue
				}

				// bugs.md #63: persist the score. ScoreJob is the most expensive step
				// in the pipeline (~9m49s/job measured live after #23 removed
				// tailoring), and its result used to be read once for the threshold
				// check below and then thrown away — UpdateFunnelStatusWithScore, the
				// only writer of fit_score, had zero callers.
				if score < 50 {
					log.Printf("[Worker-%d] Fit Score Pipeline: %s scored %d. Skipping because it is under 50.", workerID, job.CompanyName, score)
					if err := storage.UpdateFunnelStatusWithScore(job.URL, "SKIPPED", score); err != nil {
						log.Printf("[Worker-%d] Failed to record fit score for %s: %v", workerID, job.CompanyName, err)
					}
					time.Sleep(1 * time.Second)
					continue
				}
				log.Printf("[Worker-%d] Fit Score Pipeline: %s scored %d! Proceeding with application.", workerID, job.CompanyName, score)
				if err := storage.UpdateFunnelStatusWithScore(job.URL, "PROCESSING", score); err != nil {
					log.Printf("[Worker-%d] Failed to record fit score for %s: %v", workerID, job.CompanyName, err)
				}

				if prof.AutoSubmit {
					if err := pipeline.SaveCheckpoint(job.CompanyName, job.URL, "INITIATED"); err != nil {
						log.Printf("[Worker-%d] Failed to checkpoint: %v", workerID, err)
					}

					generateDocsFunc := func() (string, string, error) {
						// One static, job-agnostic cover letter for every application
						// (profile.yaml's use_master_cover_letter). Skips the
						// ProcessJobApplication LLM call entirely rather than
						// generating documents and discarding them: that call is the
						// most expensive step in the pipeline by a wide margin, and
						// its per-job resume never reaches the employer anyway since
						// the uploaded file is always masterResumePath below.
						//
						// SaveApplication still runs, because the folder it creates is
						// what MoveToManualApply archives for MANUAL_REQUIRED jobs and
						// what the dashboard reads. It no longer writes the dedup row
						// (bugs.md #94) -- that happens only on confirmed submission.
						if prof.UseMasterCoverLetter {
							// An empty coverPath is the documented "no cover letter"
							// signal: fillCoverLetterIfPresent returns immediately on
							// it, so send_cover_letter: false disables the attachment
							// without disturbing any of the machinery around it.
							coverPath := ""
							letterText := "Cover letters are disabled (send_cover_letter: false); none was sent with this application."
							if prof.ShouldSendCoverLetter() {
								coverPath = prof.MasterCoverLetterPath
								if coverPath == "" {
									coverPath = defaultMasterCoverLetterPath
								}
								// Extracted text, not raw bytes: the letter may be a
								// PDF, and the saved record is meant to be readable.
								text, readErr := parser.ExtractDocumentText(coverPath)
								if readErr != nil {
									log.Printf("[Worker-%d] Failed to read master cover letter %s: %v", workerID, coverPath, readErr)
									return "", "", fmt.Errorf("failed to read master cover letter: %w", readErr)
								}
								letterText = text
							}
							const untailoredNote = "Master documents used for this application (use_master_cover_letter is enabled); no per-job tailoring was generated."
							if err := storage.SaveApplication(job.CompanyName, job.Title, job.Location, job.URL, untailoredNote, letterText, untailoredNote); err != nil {
								log.Printf("[Worker-%d] Failed to save application for %s: %v", workerID, job.CompanyName, err)
								return "", "", err
							}
							if coverPath == "" {
								log.Printf("[Worker-%d] Using master resume for %s (no per-job tailoring, cover letter disabled)", workerID, job.CompanyName)
							} else {
								log.Printf("[Worker-%d] Using master resume and master cover letter (%s) for %s (no per-job tailoring)", workerID, coverPath, job.CompanyName)
							}
							return masterResumePath, coverPath, nil
						}

						var resume, coverLetter, interviewPrep string
						var processErr error
						for attempt := 1; attempt <= 3; attempt++ {
							resume, coverLetter, interviewPrep, processErr = client.ProcessJobApplication(scrapedData, profileConstraints, tailoredContext)
							if processErr == nil {
								break
							}
							if strings.Contains(processErr.Error(), "429") || strings.Contains(processErr.Error(), "Quota exceeded") {
								log.Printf("[Worker-%d] CRITICAL: Gemini API Daily Quota Exceeded processing job %s. Shutting down agent...", workerID, job.CompanyName)
								cancel()
								return "", "", fmt.Errorf("quota exceeded")
							} else if strings.Contains(processErr.Error(), "connect:") || strings.Contains(processErr.Error(), "no route to host") || strings.Contains(processErr.Error(), "deadline exceeded") {
								log.Printf("[Worker-%d] Network error processing application %s (attempt %d/3). Sleeping 60s...", workerID, job.CompanyName, attempt)
								time.Sleep(60 * time.Second)
							} else {
								break
							}
						}

						if processErr != nil {
							log.Printf("[Worker-%d] Failed to process job for %s after retries: %v", workerID, job.CompanyName, processErr)
							return "", "", processErr
						}

						if err := storage.SaveApplication(job.CompanyName, job.Title, job.Location, job.URL, resume, coverLetter, interviewPrep); err != nil {
							log.Printf("[Worker-%d] Failed to save application for %s: %v", workerID, job.CompanyName, err)
							return "", "", err
						}
						if hasToneVariant {
							if err := storage.UpdateToneVariant(job.URL, toneVariantLabel); err != nil {
								log.Printf("[Worker-%d] Failed to record tone variant for %s: %v", workerID, job.CompanyName, err)
							}
						}

						log.Printf("[Worker-%d] Successfully generated and saved application for %s", workerID, job.CompanyName)

						// The tailored letter is still generated above (it comes out of
						// the same combined call as the resume and interview prep) and
						// still saved to the application folder, but an empty path
						// keeps it from being attached when cover letters are off.
						if !prof.ShouldSendCoverLetter() {
							return masterResumePath, "", nil
						}
						// bugs.md #62: this used to concatenate the raw company name,
						// while SaveApplication writes under the sanitized one — so
						// for any company whose name isn't already sanitize-stable
						// ("Backend Software Engineer" -> "Backend_Software_Engineer")
						// the path pointed at a file that did not exist.
						return masterResumePath, storage.CoverLetterPath(job.CompanyName), nil
					}

					if err := submitter.AttemptSubmit(browser, filter, client, client, job.CompanyName, job.URL, generateDocsFunc, piiData, tailoredContext, prof.HeadlessBrowser, prof.AutoSubmitClick); errors.Is(err, submitter.ErrAuthWall) {
						// Bug #18: not an automation failure — the ATS gates its form
						// behind an account. Tailored docs are already saved; queue
						// the job for a manual application instead.
						log.Printf("[Worker-%d] %s requires an account to apply — queued for manual submission: %v", workerID, job.CompanyName, err)
						pipeline.SaveCheckpoint(job.CompanyName, job.URL, "MANUAL_REQUIRED")
						storage.UpdateFunnelStatus(job.URL, "MANUAL_REQUIRED")
						docsDir, mvErr := storage.MoveToManualApply(job.CompanyName)
						if mvErr != nil {
							log.Printf("[Worker-%d] Failed to move %s docs to the manual-apply folder: %v", workerID, job.CompanyName, mvErr)
						}
						if logErr := storage.LogManualRequired(job.CompanyName, job.Title, job.URL, docsDir); logErr != nil {
							log.Printf("[Worker-%d] Also failed to log manual-apply queue entry for %s: %v", workerID, job.CompanyName, logErr)
						}
					} else if errors.Is(err, submitter.ErrNeedsUnprovidedAttestation) {
						// bugs.md #82/#84: this form asks the applicant to declare
						// something not set in pii.yaml -- work authorization,
						// sponsorship, clearance or criminal history -- and those
						// questions offer no decline option. Guessing would submit a
						// false legal statement under the user's name, so the job goes
						// to manual review with its tailored documents saved. Fill the
						// matching key in pii.yaml to unblock it; the error names the
						// missing category.
						log.Printf("[Worker-%d] %s needs a legal attestation not set in pii.yaml — queued for manual submission: %v", workerID, job.CompanyName, err)
						pipeline.SaveCheckpoint(job.CompanyName, job.URL, "MANUAL_REQUIRED")
						storage.UpdateFunnelStatus(job.URL, "MANUAL_REQUIRED")
						docsDir, mvErr := storage.MoveToManualApply(job.CompanyName)
						if mvErr != nil {
							log.Printf("[Worker-%d] Failed to move %s docs to the manual-apply folder: %v", workerID, job.CompanyName, mvErr)
						}
						if logErr := storage.LogManualRequired(job.CompanyName, job.Title, job.URL, docsDir); logErr != nil {
							log.Printf("[Worker-%d] Also failed to log manual-apply queue entry for %s: %v", workerID, job.CompanyName, logErr)
						}
					} else if errors.Is(err, submitter.ErrFormTooLargeForModel) {
						// Bug #52's later recurrences: this form's content would
						// exceed the local model's context window regardless of how
						// much the DOM gets trimmed. Same manual-routing outcome as
						// ErrAuthWall -- tailored docs are already saved -- rather
						// than burning a doomed LLM call.
						log.Printf("[Worker-%d] %s's form is too large for the local model — queued for manual submission: %v", workerID, job.CompanyName, err)
						pipeline.SaveCheckpoint(job.CompanyName, job.URL, "MANUAL_REQUIRED")
						storage.UpdateFunnelStatus(job.URL, "MANUAL_REQUIRED")
						docsDir, mvErr := storage.MoveToManualApply(job.CompanyName)
						if mvErr != nil {
							log.Printf("[Worker-%d] Failed to move %s docs to the manual-apply folder: %v", workerID, job.CompanyName, mvErr)
						}
						if logErr := storage.LogManualRequired(job.CompanyName, job.Title, job.URL, docsDir); logErr != nil {
							log.Printf("[Worker-%d] Also failed to log manual-apply queue entry for %s: %v", workerID, job.CompanyName, logErr)
						}
					} else if errors.Is(err, submitter.ErrCaptchaBlocked) {
						// Bug #23: not a submit failure — the site is bot-walled.
						log.Printf("[Worker-%d] %s is behind a bot-protection challenge — marked BLOCKED_CAPTCHA: %v", workerID, job.CompanyName, err)
						pipeline.SaveCheckpoint(job.CompanyName, job.URL, "BLOCKED_CAPTCHA")
						storage.UpdateFunnelStatus(job.URL, "BLOCKED_CAPTCHA")
					} else if err != nil {
						if submitter.IsManualReviewError(err) {
							// bugs.md #84: a manual-review sentinel that no branch above
							// claimed. Never let one fall through to FAILED_SUBMIT --
							// the job is fine, it just needs a human, and its tailored
							// documents must be preserved.
							log.Printf("[Worker-%d] %s needs manual completion — queued for manual submission: %v", workerID, job.CompanyName, err)
							pipeline.SaveCheckpoint(job.CompanyName, job.URL, "MANUAL_REQUIRED")
							storage.UpdateFunnelStatus(job.URL, "MANUAL_REQUIRED")
							docsDir, mvErr := storage.MoveToManualApply(job.CompanyName)
							if mvErr != nil {
								log.Printf("[Worker-%d] Failed to move %s docs to the manual-apply folder: %v", workerID, job.CompanyName, mvErr)
							}
							if logErr := storage.LogManualRequired(job.CompanyName, job.Title, job.URL, docsDir); logErr != nil {
								log.Printf("[Worker-%d] Also failed to log manual-apply queue entry for %s: %v", workerID, job.CompanyName, logErr)
							}
							continue
						}
						log.Printf("[Worker-%d] Auto-Submit failed for %s: %v", workerID, job.CompanyName, err)
						pipeline.SaveCheckpoint(job.CompanyName, job.URL, "FAILED")
						storage.UpdateFunnelStatus(job.URL, "FAILED_SUBMIT")
						if logErr := storage.LogFailedSubmission(job.CompanyName, job.Title, job.URL); logErr != nil {
							log.Printf("[Worker-%d] Also failed to log manual submission for %s: %v", workerID, job.CompanyName, logErr)
						}
					} else {
						pipeline.SaveCheckpoint(job.CompanyName, job.URL, "COMPLETED")
						storage.UpdateFunnelStatus(job.URL, "APPLIED")
						// bugs.md #94: the dedup row is written HERE, on confirmed
						// submission, and nowhere else. SaveApplication used to write
						// it at document-generation time, which marked jobs as applied
						// that had never been submitted -- they were then skipped on
						// every subsequent run and could never be retried.
						if err := storage.RecordApplicationInDB(job.CompanyName, job.Title, job.URL); err != nil {
							log.Printf("[Worker-%d] Submitted %s but failed to record the dedup row: %v", workerID, job.CompanyName, err)
						}
					}
				} else {
					// If not auto-submitting, we still consider the pipeline processing done
					storage.UpdateFunnelStatus(job.URL, "PROCESSED_MANUAL")
				}

				// Sleep for 15 seconds to ensure we never hit the 5 RPM rate limit
				time.Sleep(1 * time.Second)
			} // close for job := range jobChan
		}(w)
	}

	wg.Wait()
	log.Println("[Agent] Batch execution complete!")
}

// defaultWorkerCount picks a starting concurrency: local Ollama serves one
// request at a time (single slot), so piling on workers just queues them and
// starves the shared context window; paid API backends can parallelize.
func defaultWorkerCount() int {
	provider := strings.ToLower(strings.TrimSpace(os.Getenv("LLM_PROVIDER")))
	if provider == "" || provider == "ollama" {
		// Local Ollama serves one request at a time (-np 1): a second
		// concurrent worker just queues behind the first and, on slow
		// CPU inference, can blow past the client's own request timeout
		// before ever being served. One worker matches the server's
		// actual capacity and avoids that queuing/timeout churn.
		return 1
	}
	return 10
}
