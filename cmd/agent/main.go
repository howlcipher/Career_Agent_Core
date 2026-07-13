package main

import (
	"log"

	"github.com/howlcipher/Career_Agent_Core/pkg/config"
	"github.com/howlcipher/Career_Agent_Core/pkg/mcp"
	"github.com/howlcipher/Career_Agent_Core/pkg/parser"
	"github.com/howlcipher/Career_Agent_Core/pkg/scraper"
	"github.com/howlcipher/Career_Agent_Core/pkg/security"
	"github.com/howlcipher/Career_Agent_Core/pkg/storage"
)

func main() {
	log.Println("Initializing Career Agent Core...")

	prof, err := config.LoadProfile("profile.yaml")
	if err != nil {
		log.Fatalf("Configuration error: %v", err)
	}
	log.Printf("Loaded profile: role=%s, salary_floor=%d", prof.Role, prof.SalaryFloor)

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

	client := mcp.NewClient("http://localhost:8080")

	for _, job := range jobs {
		if !prof.ValidateJob(job.Salary, job.Remote) {
			continue
		}

		scrapedData := map[string]string{
			"title": job.Title,
			"desc":  job.Description,
		}

		profileConstraints := map[string]interface{}{
			"salary_floor": prof.SalaryFloor,
			"remote_only":  prof.RemoteOnly,
		}

		resume, coverLetter, err := client.ProcessJobApplication(scrapedData, profileConstraints, docContext)
		if err != nil {
			log.Printf("Failed to process job for %s: %v", job.CompanyName, err)
			continue
		}

		if err := storage.SaveApplication(job.CompanyName, job.Title, job.Location, job.URL, resume, coverLetter); err != nil {
			log.Printf("Failed to save application for %s: %v", job.CompanyName, err)
			continue
		}

		log.Printf("Successfully generated and saved application for %s", job.CompanyName)
	}
}
