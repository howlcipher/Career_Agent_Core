package storage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type Metadata struct {
	CompanyName        string    `json:"company_name"`
	JobTitle           string    `json:"job_title"`
	Location           string    `json:"location"`
	OriginalPostingURL string    `json:"original_posting_url"`
	ApplicationDate    time.Time `json:"application_date"`
}

func SaveApplication(companyName, jobTitle, location, url, resumeContent, coverLetterContent string) error {
	baseDir := "applications"
	companyDir := filepath.Join(baseDir, companyName)

	if err := os.MkdirAll(companyDir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	resumePath := filepath.Join(companyDir, "resume.md")
	if err := os.WriteFile(resumePath, []byte(resumeContent), 0644); err != nil {
		return fmt.Errorf("failed to write resume: %w", err)
	}

	coverLetterPath := filepath.Join(companyDir, "coverletter.txt")
	if err := os.WriteFile(coverLetterPath, []byte(coverLetterContent), 0644); err != nil {
		return fmt.Errorf("failed to write cover letter: %w", err)
	}

	metadata := Metadata{
		CompanyName:        companyName,
		JobTitle:           jobTitle,
		Location:           location,
		OriginalPostingURL: url,
		ApplicationDate:    time.Now(),
	}

	metadataBytes, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	metadataPath := filepath.Join(companyDir, "metadata.json")
	if err := os.WriteFile(metadataPath, metadataBytes, 0644); err != nil {
		return fmt.Errorf("failed to write metadata: %w", err)
	}

	return nil
}
