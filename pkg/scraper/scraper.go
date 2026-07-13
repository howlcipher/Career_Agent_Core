package scraper

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
	"math/rand"
)

type Job struct {
	CompanyName string
	Title       string
	Location    string
	URL         string
	Salary      int
	Remote      bool
	Description string
}

type Engine struct {
	SalaryFloor int
}

func NewEngine(salaryFloor int) *Engine {
	return &Engine{
		SalaryFloor: salaryFloor,
	}
}

type RemoteOkJob struct {
	Company     string   `json:"company"`
	Position    string   `json:"position"`
	Location    string   `json:"location"`
	URL         string   `json:"url"`
	SalaryMin   int      `json:"salary_min"`
	SalaryMax   int      `json:"salary_max"`
	Tags        []string `json:"tags"`
	Description string   `json:"description"`
}

func (e *Engine) FetchJobs() ([]Job, error) {
	fmt.Println("Scraping RemoteOK API for backend roles...")

	// Sleep for a random jitter (1-3 seconds) to seem human
	time.Sleep(time.Duration(rand.Intn(2000)+1000) * time.Millisecond)

	req, err := http.NewRequest("GET", "https://remoteok.com/api?tag=backend", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	
	// Humanize the headers to bypass basic bot protection
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.5")
	req.Header.Set("Connection", "keep-alive")
	req.Header.Set("Upgrade-Insecure-Requests", "1")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API returned non-200 status: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// RemoteOK returns a legal notice as the first element, followed by the job objects
	var rawJobs []json.RawMessage
	if err := json.Unmarshal(body, &rawJobs); err != nil {
		return nil, fmt.Errorf("failed to unmarshal JSON array: %w", err)
	}

	if len(rawJobs) <= 1 {
		return nil, fmt.Errorf("no jobs found in API response")
	}

	var jobs []Job
	for i := 1; i < len(rawJobs); i++ {
		var roJob RemoteOkJob
		if err := json.Unmarshal(rawJobs[i], &roJob); err != nil {
			continue // Skip malformed entries
		}

		// Ensure it is a remote role (RemoteOK is usually 100% remote, but we can verify)
		isRemote := true
		if strings.Contains(strings.ToLower(roJob.Location), "hybrid") || strings.Contains(strings.ToLower(roJob.Location), "onsite") {
			isRemote = false
		}

		// RemoteOK uses max salary or min salary
		estimatedSalary := roJob.SalaryMax
		if roJob.SalaryMin > 0 && roJob.SalaryMax == 0 {
			estimatedSalary = roJob.SalaryMin
		}

		jobs = append(jobs, Job{
			CompanyName: roJob.Company,
			Title:       roJob.Position,
			Location:    roJob.Location,
			URL:         roJob.URL,
			Salary:      estimatedSalary,
			Remote:      isRemote,
			Description: roJob.Description,
		})
	}

	fmt.Printf("Successfully fetched and parsed %d jobs.\n", len(jobs))
	return jobs, nil
}
