package scraper

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFetchJobs(t *testing.T) {
	mockJobs := []interface{}{
		map[string]interface{}{"legal": "notice"},
		RemoteOkJob{
			Company:     "Test Company",
			Position:    "Backend Engineer",
			Location:    "Remote",
			URL:         "https://test.com/job/1",
			SalaryMin:   100000,
			SalaryMax:   150000,
			Description: "Test Description",
		},
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(mockJobs)
	}))
	defer ts.Close()

	originalURL := remoteOKBaseURL
	remoteOKBaseURL = ts.URL
	defer func() { remoteOKBaseURL = originalURL }()

	engine := NewEngine(100000, []string{"backend"})
	jobs, err := engine.FetchJobs()
	if err != nil {
		t.Fatalf("FetchJobs failed: %v", err)
	}

	if len(jobs) != 1 {
		t.Fatalf("expected 1 job, got %d", len(jobs))
	}

	if jobs[0].CompanyName != "Test Company" {
		t.Errorf("expected Test Company, got %s", jobs[0].CompanyName)
	}
	if !jobs[0].Remote {
		t.Errorf("expected remote job")
	}
	if jobs[0].Salary != 150000 {
		t.Errorf("expected salary 150000, got %d", jobs[0].Salary)
	}
}
