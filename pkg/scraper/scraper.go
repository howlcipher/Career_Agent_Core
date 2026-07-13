package scraper

import (
	"fmt"
)

type Job struct {
	CompanyName string
	Title       string
	Location    string
	URL         string
	Salary      int
	Remote      bool
}

type Engine struct {
	SalaryFloor int
}

func NewEngine(salaryFloor int) *Engine {
	return &Engine{
		SalaryFloor: salaryFloor,
	}
}

func (e *Engine) FetchJobs() ([]Job, error) {
	fmt.Println("Scraping for remote only roles with a salary floor of", e.SalaryFloor)
	
	jobs := []Job{
		{
			CompanyName: "TechCorp",
			Title:       "Senior Backend Engineer",
			Location:    "Remote",
			URL:         "https://example.com/jobs/1",
			Salary:      150000,
			Remote:      true,
		},
	}
	
	return jobs, nil
}
