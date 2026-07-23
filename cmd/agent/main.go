package main

import (
	"errors"
	"flag"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/howlcipher/Career_Agent_Core/pkg/config"
	"github.com/howlcipher/Career_Agent_Core/pkg/mcp"
	"github.com/howlcipher/Career_Agent_Core/pkg/parser"
	"context"
	"os/signal"
	"syscall"
	"io"
	"fmt"
	"net/http"
	"net/url"
	"github.com/howlcipher/Career_Agent_Core/pkg/scraper"
	"github.com/howlcipher/Career_Agent_Core/pkg/security"
	"github.com/howlcipher/Career_Agent_Core/pkg/storage"
	"github.com/howlcipher/Career_Agent_Core/pkg/submitter"
	"github.com/mxschmitt/playwright-go"
	"github.com/joho/godotenv"
	"gopkg.in/natefinch/lumberjack.v2"
	"sync"
)

func main() {
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

	filter := security.NewQuarantineLayer()
	jobChan := make(chan scraper.Job, 2000)

	discoveredJobs, err := storage.GetDiscoveredJobs()
	var producerWg sync.WaitGroup

	if err == nil {
		producerWg.Add(1)
		go func() {
			defer producerWg.Done()
			for _, dj := range discoveredJobs {
				jobChan <- scraper.Job{
					CompanyName: dj.CompanyName,
					Title:       dj.JobTitle,
					URL:         dj.URL,
					Salary:      prof.TargetComp,
					Remote:      true,
				}
			}
			log.Printf("[Agent] Loaded %d previously discovered jobs from backlog into the queue.", len(discoveredJobs))
		}()
	}

	funnelEngine := scraper.NewFunnelEngine(prof.Roles)
	producerWg.Add(1)
	go func() {
		defer producerWg.Done()
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
	existingChunks, err := storage.GetAllCareerChunks()
	if err != nil {
		log.Printf("[Agent] [RAG] Failed to get career chunks from storage: %v", err)
	}
	if len(existingChunks) == 0 {
		log.Println("[RAG] Knowledge Library cache empty. Ingesting USER_PROFILE.md into local SQLite Vector DB...")
		mdContent, err := parser.ReadMarkdown("/var/home/howlcipher/dev/ai_knowledge_library/USER_PROFILE.md")
		if err == nil {
			chunks := parser.ChunkMarkdown(mdContent)
			for _, text := range chunks {
				if strings.TrimSpace(text) == "" { continue }
				emb, err := client.GetEmbedding(text)
				if err == nil {
					storage.SaveCareerChunk(text, emb)
				}
			}
			log.Printf("[RAG] Successfully embedded and cached %d career chunks.", len(chunks))
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
				continue
			}
			req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
			resp, err := httpClient.Do(req)
			if err == nil {
				defer resp.Body.Close()
				b, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
				if err != nil {
					log.Printf("[Worker-%d] Failed to read response body for %s: %v", workerID, job.CompanyName, err)
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
				widgetOnlyPhrasing := strings.Contains(lowerHTML, "recaptcha") || strings.Contains(lowerHTML, "cf-turnstile")
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

		profileConstraints := map[string]interface{}{
			"salary_floor":        prof.SalaryFloor,
			"target_compensation": prof.TargetComp,
			"remote_only":         prof.RemoteOnly,
			"cover_letter_tone":   prof.CoverLetterTone,
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

		if score < 50 {
			log.Printf("[Worker-%d] Fit Score Pipeline: %s scored %d. Skipping because it is under 50.", workerID, job.CompanyName, score)
			storage.UpdateFunnelStatus(job.URL, "SKIPPED")
			time.Sleep(1 * time.Second)
			continue
		}
		log.Printf("[Worker-%d] Fit Score Pipeline: %s scored %d! Proceeding with application.", workerID, job.CompanyName, score)

		if prof.AutoSubmit {
			if err := pipeline.SaveCheckpoint(job.CompanyName, job.URL, "INITIATED"); err != nil {
				log.Printf("[Worker-%d] Failed to checkpoint: %v", workerID, err)
			}

			generateDocsFunc := func() (string, string, error) {
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

				log.Printf("[Worker-%d] Successfully generated and saved application for %s", workerID, job.CompanyName)

				masterResumePath := "master_resume.pdf"
				coverLetterPath := "applications/" + job.CompanyName + "/coverletter.txt"
				return masterResumePath, coverLetterPath, nil
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
			} else if errors.Is(err, submitter.ErrCaptchaBlocked) {
				// Bug #23: not a submit failure — the site is bot-walled.
				log.Printf("[Worker-%d] %s is behind a bot-protection challenge — marked BLOCKED_CAPTCHA: %v", workerID, job.CompanyName, err)
				pipeline.SaveCheckpoint(job.CompanyName, job.URL, "BLOCKED_CAPTCHA")
				storage.UpdateFunnelStatus(job.URL, "BLOCKED_CAPTCHA")
			} else if err != nil {
				log.Printf("[Worker-%d] Auto-Submit failed for %s: %v", workerID, job.CompanyName, err)
				pipeline.SaveCheckpoint(job.CompanyName, job.URL, "FAILED")
				storage.UpdateFunnelStatus(job.URL, "FAILED_SUBMIT")
				if logErr := storage.LogFailedSubmission(job.CompanyName, job.Title, job.URL); logErr != nil {
					log.Printf("[Worker-%d] Also failed to log manual submission for %s: %v", workerID, job.CompanyName, logErr)
				}
			} else {
				pipeline.SaveCheckpoint(job.CompanyName, job.URL, "COMPLETED")
				storage.UpdateFunnelStatus(job.URL, "APPLIED")
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
