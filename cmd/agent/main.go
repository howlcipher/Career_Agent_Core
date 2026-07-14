package main

import (
	"log"
	"os"
	"strings"

	"github.com/howlcipher/Career_Agent_Core/pkg/config"
	"github.com/joho/godotenv"
	"github.com/howlcipher/Career_Agent_Core/pkg/mcp"
	"github.com/howlcipher/Career_Agent_Core/pkg/parser"
	"github.com/howlcipher/Career_Agent_Core/pkg/scraper"
	"github.com/howlcipher/Career_Agent_Core/pkg/security"
	"github.com/howlcipher/Career_Agent_Core/pkg/storage"
	"github.com/howlcipher/Career_Agent_Core/pkg/submitter"
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found or error loading it. Relying on system environment variables.")
	}

	log.Println("Initializing Career Agent Core...")

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
	
	funnelEngine := scraper.NewFunnelEngine(prof.Roles)
	if err := funnelEngine.DiscoverJobs(); err != nil {
		log.Printf("Funnel discovery error: %v", err)
	}

	discoveredJobs, err := storage.GetDiscoveredJobs()
	if err != nil {
		log.Fatalf("Failed to fetch discovered jobs: %v", err)
	}

	var jobs []scraper.Job
	for _, dj := range discoveredJobs {
		jobs = append(jobs, scraper.Job{
			CompanyName: dj.CompanyName,
			Title:       dj.JobTitle,
			URL:         dj.URL,
			Salary:      prof.TargetComp, 
			Remote:      true,            
		})
	}



	client := mcp.NewClient(os.Getenv("GEMINI_API_KEY"))
	pipeline := submitter.NewPipeline(storage.GetDB(), filter, client)

	// Local Embedded RAG Ingestion
	existingChunks, _ := storage.GetAllCareerChunks()
	if len(existingChunks) == 0 {
		log.Println("[RAG] Knowledge Library cache empty. Ingesting USER_PROFILE.md into local SQLite Vector DB...")
		mdContent, err := parser.ReadMarkdown("/run/media/system/tallgeese/dev/ai_knowledge_library/USER_PROFILE.md")
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

	for _, job := range jobs {
		if !prof.ValidateJob(job.CompanyName, job.Salary, job.Remote) {
			continue
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
		jobEmb, err := client.GetEmbedding(jobDescText)
		
		var tailoredContext string
		if err == nil {
			topChunks, _ := parser.RetrieveTopK(jobEmb, 5)
			var sb strings.Builder
			sb.WriteString("Highly Relevant Career Context (Retrieved via RAG):\n\n")
			for _, tc := range topChunks {
				sb.WriteString(tc.Text + "\n\n")
			}
			tailoredContext = sb.String()
		} else {
			log.Printf("[RAG] Embedding failed, falling back to empty context: %v", err)
		}

		if err := filter.CheckPayload(tailoredContext); err != nil {
			log.Printf("Security quarantine triggered on RAG output: %v", err)
			continue
		}

		score, err := client.ScoreJob(scrapedData, tailoredContext)
		if err != nil {
			log.Printf("Failed to score job for %s: %v", job.CompanyName, err)
			continue
		}

		if score < 80 {
			log.Printf("Fit Score Pipeline: %s scored %d. Skipping because it is under 80.", job.CompanyName, score)
			continue
		}
		log.Printf("Fit Score Pipeline: %s scored an excellent %d! Proceeding with application.", job.CompanyName, score)

		profileConstraints := map[string]interface{}{
			"salary_floor":        prof.SalaryFloor,
			"target_compensation": prof.TargetComp,
			"remote_only":         prof.RemoteOnly,
			"cover_letter_tone":   prof.CoverLetterTone,
		}

		resume, coverLetter, interviewPrep, err := client.ProcessJobApplication(scrapedData, profileConstraints, tailoredContext)
		if err != nil {
			log.Printf("Failed to process job for %s: %v", job.CompanyName, err)
			continue
		}

		if err := storage.SaveApplication(job.CompanyName, job.Title, job.Location, job.URL, resume, coverLetter, interviewPrep); err != nil {
			log.Printf("Failed to save application for %s: %v", job.CompanyName, err)
			continue
		}

		log.Printf("Successfully generated and saved application for %s", job.CompanyName)

		if prof.AutoSubmit {
			resumePath := "applications/" + job.CompanyName + "/resume.md"
			coverLetterPath := "applications/" + job.CompanyName + "/coverletter.txt"

			// Use the new dynamic pipeline to verify and match ATS templates
			// This demonstrates the Two-Step Verification and State Checkpointing architecture
			if err := pipeline.SaveCheckpoint(job.CompanyName, job.URL, "INITIATED"); err != nil {
				log.Printf("Failed to checkpoint: %v", err)
			}

			if err := submitter.AttemptSubmit(job.CompanyName, job.URL, resumePath, coverLetterPath, piiData, prof.HeadlessBrowser, prof.AutoSubmitClick); err != nil {
				log.Printf("Auto-Submit failed for %s: %v", job.CompanyName, err)
				pipeline.SaveCheckpoint(job.CompanyName, job.URL, "FAILED")
				if logErr := storage.LogFailedSubmission(job.CompanyName, job.Title, job.URL); logErr != nil {
					log.Printf("Also failed to log manual submission for %s: %v", job.CompanyName, logErr)
				}
			} else {
				pipeline.SaveCheckpoint(job.CompanyName, job.URL, "COMPLETED")
			}
		}
	}
}
