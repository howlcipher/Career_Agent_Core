package main

import (
	"log"
	"os"

	"github.com/howlcipher/Career_Agent_Core/pkg/config"
	"github.com/howlcipher/Career_Agent_Core/pkg/mcp"
	"github.com/howlcipher/Career_Agent_Core/pkg/parser"
	"github.com/howlcipher/Career_Agent_Core/pkg/scraper"
	"github.com/howlcipher/Career_Agent_Core/pkg/security"
	"github.com/howlcipher/Career_Agent_Core/pkg/storage"
	"github.com/howlcipher/Career_Agent_Core/pkg/submitter"
)

func main() {
	log.Println("Initializing Career Agent Core...")

	if err := storage.InitDB(); err != nil {
		log.Fatalf("Failed to initialize SQLite database: %v", err)
	}

	prof, err := config.LoadProfile("profile.yaml")
	if err != nil {
		log.Fatalf("Configuration error: %v", err)
	}
	log.Printf("Loaded profile: role=%s, salary_floor=%d", prof.Role, prof.SalaryFloor)

	piiData, err := config.LoadPII("pii.yaml")
	if err != nil {
		log.Printf("PII warning (defaulting to empty fields): %v", err)
		piiData = &config.PII{}
	}

	filter := security.NewQuarantineLayer()
	
	scrapeEngine := scraper.NewEngine(prof.SalaryFloor)
	jobs, err := scrapeEngine.FetchJobs()
	if err != nil {
		log.Fatalf("Scraper error: %v", err)
	}

	docContext := ""
	mdContent, err := parser.ReadMarkdown("/run/media/system/tallgeese/dev/ai_knowledge_library/USER_PROFILE.md")
	if err != nil {
		log.Println("USER_PROFILE.md not found in knowledge library, trying fallback PDF...")
		pdfContent, pdfErr := parser.ExtractFromFallbackPDF("__William_Elias_Resume__.pdf")
		if pdfErr != nil {
			log.Println("PDF fallback also failed. Proceeding without local document context.")
		} else {
			docContext = pdfContent
		}
	} else {
		docContext = mdContent
	}

	if err := filter.CheckPayload(docContext); err != nil {
		log.Fatalf("Security quarantine triggered: %v", err)
	}

	client := mcp.NewClient(os.Getenv("GEMINI_API_KEY"))

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

		score, err := client.ScoreJob(scrapedData, docContext)
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
			"salary_floor":      prof.SalaryFloor,
			"remote_only":       prof.RemoteOnly,
			"cover_letter_tone": prof.CoverLetterTone,
		}

		resume, coverLetter, interviewPrep, err := client.ProcessJobApplication(scrapedData, profileConstraints, docContext)
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
			if err := submitter.AttemptSubmit(job.CompanyName, job.URL, resumePath, coverLetterPath, piiData); err != nil {
				log.Printf("Auto-Submit failed for %s: %v", job.CompanyName, err)
				if logErr := storage.LogFailedSubmission(job.CompanyName, job.Title, job.URL); logErr != nil {
					log.Printf("Also failed to log manual submission for %s: %v", job.CompanyName, logErr)
				}
			}
		}
	}
}
