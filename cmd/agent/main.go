package main

import (
	"flag"
	"log"
	"os"
	"strings"
	"time"

	"github.com/howlcipher/Career_Agent_Core/pkg/config"
	"github.com/howlcipher/Career_Agent_Core/pkg/mcp"
	"github.com/howlcipher/Career_Agent_Core/pkg/parser"
	"io"
	"net/http"
	"github.com/howlcipher/Career_Agent_Core/pkg/scraper"
	"github.com/howlcipher/Career_Agent_Core/pkg/security"
	"github.com/howlcipher/Career_Agent_Core/pkg/storage"
	"github.com/howlcipher/Career_Agent_Core/pkg/submitter"
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

	log.Println("Initializing Career Agent Core...")
	if *daemonMode {
		log.Println("[DAEMON MODE] Agent will drip applications every 6 hours to evade ATS IP bans.")
	}

	if err := storage.InitDB(); err != nil {
		log.Fatalf("Failed to initialize SQLite database: %v", err)
	}

	prof, err := config.LoadProfile("profile.yaml")
	if err != nil {
		log.Fatalf("Configuration error: %v", err)
	}
	log.Printf("Loaded profile: roles=%v, salary_floor=%d", prof.Roles, prof.SalaryFloor)

	piiData, err := config.LoadPII("pii.yaml")
	if err != nil {
		log.Printf("PII warning (defaulting to empty fields): %v", err)
		piiData = &config.PII{}
	}

	filter := security.NewQuarantineLayer()
	jobChan := make(chan scraper.Job, 2000)

	discoveredJobs, err := storage.GetDiscoveredJobs()
	if err == nil {
		go func() {
			for _, dj := range discoveredJobs {
				jobChan <- scraper.Job{
					CompanyName: dj.CompanyName,
					Title:       dj.JobTitle,
					URL:         dj.URL,
					Salary:      prof.TargetComp, 
					Remote:      true,            
				}
			}
			log.Printf("Loaded %d previously discovered jobs from backlog into the queue.", len(discoveredJobs))
		}()
	}

	funnelEngine := scraper.NewFunnelEngine(prof.Roles)
	go func() {
		if err := funnelEngine.DiscoverJobs(jobChan); err != nil {
			log.Printf("Funnel discovery error: %v", err)
		}
	}()

	client := mcp.NewClient(os.Getenv("GEMINI_API_KEY"))
	pipeline := submitter.NewPipeline(storage.GetDB(), filter, client)

	// Local Embedded RAG Ingestion
	existingChunks, _ := storage.GetAllCareerChunks()
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
	numWorkers := 10 // Increased to 10 workers for massive concurrency on Paid Tier
	
	for w := 1; w <= numWorkers; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for job := range jobChan {
		storage.UpdateFunnelStatus(job.URL, "PROCESSING")
		// The LLM will perform the real analysis of fit, salary, and remote status based on the job description.
		// We only need to enforce the hard blocklist here.
		nameLower := strings.ToLower(job.CompanyName)
		excluded := false
		for _, ex := range prof.ExcludeCompanies {
			if strings.Contains(nameLower, strings.ToLower(ex)) {
				log.Printf("Security Block: Skipping %s (Found in ExcludeCompanies blocklist)\n", job.CompanyName)
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
			log.Printf("Fetching job description for %s...", job.CompanyName)
			httpClient := &http.Client{Timeout: 10 * time.Second}
			req, _ := http.NewRequest("GET", job.URL, nil)
			req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
			resp, err := httpClient.Do(req)
			if err == nil {
				b, _ := io.ReadAll(resp.Body)
				htmlStr := string(b)
				resp.Body.Close()
				
				// Captcha / Bot protection check
				lowerHTML := strings.ToLower(htmlStr)
				if strings.Contains(lowerHTML, "cloudflare") && (strings.Contains(lowerHTML, "verify you are human") || strings.Contains(lowerHTML, "attention required")) || strings.Contains(lowerHTML, "recaptcha") || strings.Contains(lowerHTML, "cf-turnstile") {
					log.Printf("Security/Captcha block detected for %s. Skipping job to save API tokens.", job.CompanyName)
					storage.UpdateFunnelStatus(job.URL, "BLOCKED_CAPTCHA")
					continue
				}

				pruned, _ := parser.PruneDOMToText(htmlStr)
				job.Description = pruned
			} else {
				log.Printf("Failed to fetch job description for %s: %v", job.CompanyName, err)
			}
		}

		if storage.HasApplied(job.URL) {
			log.Printf("Duplicate check: Already applied to %s. Skipping.", job.CompanyName)
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
				log.Printf("Network or Rate Limit error getting embedding (attempt %d/3). Sleeping 60s...", attempt)
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
			log.Printf("Security quarantine triggered on RAG output: %v", err)
			continue
		}

		profileConstraints := map[string]interface{}{
			"salary_floor":        prof.SalaryFloor,
			"target_compensation": prof.TargetComp,
			"remote_only":         prof.RemoteOnly,
			"cover_letter_tone":   prof.CoverLetterTone,
		}

		var score int
		var scoreErr error
		for attempt := 1; attempt <= 3; attempt++ {
			score, scoreErr = client.ScoreJob(scrapedData, profileConstraints, tailoredContext)
			if scoreErr == nil {
				break
			}
			if strings.Contains(scoreErr.Error(), "429") || strings.Contains(scoreErr.Error(), "Quota exceeded") {
				log.Printf("CRITICAL: Gemini API Daily Quota Exceeded scoring job %s. Sleeping for 1 hour before next attempt...", job.CompanyName)
				time.Sleep(1 * time.Hour)
			} else if strings.Contains(scoreErr.Error(), "connect:") || strings.Contains(scoreErr.Error(), "no route to host") || strings.Contains(scoreErr.Error(), "deadline exceeded") {
				log.Printf("Network error scoring job %s (attempt %d/3). Sleeping 60s...", job.CompanyName, attempt)
				time.Sleep(60 * time.Second)
			} else {
				break
			}
		}

		if scoreErr != nil {
			log.Printf("Failed to score job for %s after retries: %v", job.CompanyName, scoreErr)
			storage.UpdateFunnelStatus(job.URL, "FAILED_SCORE")
			time.Sleep(1 * time.Second)
			continue
		}

		if score < 50 {
			log.Printf("Fit Score Pipeline: %s scored %d. Skipping because it is under 50.", job.CompanyName, score)
			storage.UpdateFunnelStatus(job.URL, "SKIPPED")
			time.Sleep(1 * time.Second)
			continue
		}
		log.Printf("Fit Score Pipeline: %s scored %d! Proceeding with application.", job.CompanyName, score)

		var resume, coverLetter, interviewPrep string
		var processErr error
		for attempt := 1; attempt <= 3; attempt++ {
			resume, coverLetter, interviewPrep, processErr = client.ProcessJobApplication(scrapedData, profileConstraints, tailoredContext)
			if processErr == nil {
				break
			}
			if strings.Contains(processErr.Error(), "429") || strings.Contains(processErr.Error(), "Quota exceeded") {
				log.Printf("CRITICAL: Gemini API Daily Quota Exceeded processing job %s. Sleeping for 1 hour before next attempt...", job.CompanyName)
				time.Sleep(1 * time.Hour)
			} else if strings.Contains(processErr.Error(), "connect:") || strings.Contains(processErr.Error(), "no route to host") || strings.Contains(processErr.Error(), "deadline exceeded") {
				log.Printf("Network error processing application %s (attempt %d/3). Sleeping 60s...", job.CompanyName, attempt)
				time.Sleep(60 * time.Second)
			} else {
				break
			}
		}

		if processErr != nil {
			log.Printf("Failed to process job for %s after retries: %v", job.CompanyName, processErr)
			time.Sleep(1 * time.Second)
			continue
		}

		if err := storage.SaveApplication(job.CompanyName, job.Title, job.Location, job.URL, resume, coverLetter, interviewPrep); err != nil {
			log.Printf("Failed to save application for %s: %v", job.CompanyName, err)
			time.Sleep(1 * time.Second)
			continue
		}

		log.Printf("Successfully generated and saved application for %s", job.CompanyName)

		if prof.AutoSubmit {
			// We still save the LLM-generated resume to the application folder for your records,
			// but we upload the beautifully formatted master PDF to the actual ATS to ensure it parses correctly.
			masterResumePath := "master_resume.pdf"
			coverLetterPath := "applications/" + job.CompanyName + "/coverletter.txt"

			if err := pipeline.SaveCheckpoint(job.CompanyName, job.URL, "INITIATED"); err != nil {
				log.Printf("Failed to checkpoint: %v", err)
			}

			if err := submitter.AttemptSubmit(client, job.CompanyName, job.URL, masterResumePath, coverLetterPath, piiData, prof.HeadlessBrowser, prof.AutoSubmitClick); err != nil {
				log.Printf("Auto-Submit failed for %s: %v", job.CompanyName, err)
				pipeline.SaveCheckpoint(job.CompanyName, job.URL, "FAILED")
				storage.UpdateFunnelStatus(job.URL, "FAILED_SUBMIT")
				if logErr := storage.LogFailedSubmission(job.CompanyName, job.Title, job.URL); logErr != nil {
					log.Printf("Also failed to log manual submission for %s: %v", job.CompanyName, logErr)
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
	log.Println("Batch execution complete!")

	if *daemonMode {
		log.Println("[DAEMON MODE] Sleeping for 6 hours before next drip campaign...")
		time.Sleep(6 * time.Hour)
		main() // recursive loop for daemon
	}
}
