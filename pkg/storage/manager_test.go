package storage

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func setupTestDB(t *testing.T) {
	err := InitDBWithPath(":memory:")
	if err != nil {
		t.Fatalf("Failed to open test database: %v", err)
	}
}

func teardownTestDB() {
	if db != nil {
		db.Close()
		db = nil
	}
}

func TestJobFunnelCRUD(t *testing.T) {
	setupTestDB(t)
	defer teardownTestDB()

	// 1. Create a job in funnel
	isNew, err := AddToFunnel("TestCorp", "Software Engineer", "http://testcorp.com/job1", "DISCOVERED")
	if err != nil {
		t.Fatalf("Failed to add to funnel: %v", err)
	}
	if !isNew {
		t.Fatalf("Expected AddToFunnel to report a new insert for a fresh URL")
	}

	// 2. Read discovered jobs
	jobs, err := GetDiscoveredJobs()
	if err != nil {
		t.Fatalf("Failed to get discovered jobs: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("Expected 1 discovered job, got %d", len(jobs))
	}
	if jobs[0].CompanyName != "TestCorp" || jobs[0].URL != "http://testcorp.com/job1" {
		t.Errorf("Job details mismatch: %+v", jobs[0])
	}

	// 3. Update status
	err = UpdateFunnelStatus("http://testcorp.com/job1", "APPLIED")
	if err != nil {
		t.Fatalf("Failed to update funnel status: %v", err)
	}

	// Verify it's no longer in discovered
	jobs, _ = GetDiscoveredJobs()
	if len(jobs) != 0 {
		t.Fatalf("Expected 0 discovered jobs after update, got %d", len(jobs))
	}

	// 4. Update with score
	err = UpdateFunnelStatusWithScore("http://testcorp.com/job1", "INTERVIEW", 95)
	if err != nil {
		t.Fatalf("Failed to update funnel status with score: %v", err)
	}

	var score int
	err = db.QueryRow("SELECT fit_score FROM job_funnel WHERE url = ?", "http://testcorp.com/job1").Scan(&score)
	if err != nil {
		t.Fatalf("Failed to query score: %v", err)
	}
	if score != 95 {
		t.Errorf("Expected score 95, got %d", score)
	}

	// 5. Re-discovering the same URL later (FunnelEngine re-encountering it in
	// a later search pass) must be a no-op: it must not report a new insert,
	// and it must not reset the job's progress back to DISCOVERED. Confirmed
	// live 2026-07-21 as the root cause of the same job being reprocessed
	// multiple times and eventually hitting the applied_jobs UNIQUE
	// constraint - see bugs.md #12.
	isNewAgain, err := AddToFunnel("TestCorp", "Software Engineer", "http://testcorp.com/job1", "DISCOVERED")
	if err != nil {
		t.Fatalf("Failed to re-add existing URL to funnel: %v", err)
	}
	if isNewAgain {
		t.Errorf("Expected AddToFunnel to report no new insert for an already-known URL")
	}

	var statusAfterRediscovery string
	if err := db.QueryRow("SELECT status FROM job_funnel WHERE url = ?", "http://testcorp.com/job1").Scan(&statusAfterRediscovery); err != nil {
		t.Fatalf("Failed to query status: %v", err)
	}
	if statusAfterRediscovery != "INTERVIEW" {
		t.Errorf("Re-discovering an existing URL must not reset its status; expected %q, got %q", "INTERVIEW", statusAfterRediscovery)
	}
}

func TestApplicationsAndDuplicates(t *testing.T) {
	setupTestDB(t)
	defer teardownTestDB()

	url := "http://example.com/apply"

	// Initially, should not have applied
	if HasApplied(url) {
		t.Fatalf("HasApplied returned true for a new URL")
	}

	// Record application
	err := RecordApplicationInDB("Example Inc", "Tester", url)
	if err != nil {
		t.Fatalf("Failed to record application: %v", err)
	}

	// Now it should return true
	if !HasApplied(url) {
		t.Fatalf("HasApplied returned false after recording")
	}

	// Try inserting duplicate URL - should fail due to UNIQUE constraint
	err = RecordApplicationInDB("Example Inc", "Tester 2", url)
	if err == nil {
		t.Fatalf("Expected error when inserting duplicate URL, got nil")
	}
}

func TestFormMappingCRUD(t *testing.T) {
	setupTestDB(t)
	defer teardownTestDB()

	domain := "example-ats.com"
	mapping := `{"first_name": "input[name='fname']"}`

	// Create
	err := SaveFormMapping(domain, mapping)
	if err != nil {
		t.Fatalf("Failed to save form mapping: %v", err)
	}

	// Read
	readMapping, err := GetFormMapping(domain)
	if err != nil {
		t.Fatalf("Failed to get form mapping: %v", err)
	}
	if readMapping != mapping {
		t.Errorf("Mapping mismatch. Expected %s, got %s", mapping, readMapping)
	}

	// Update (upsert)
	newMapping := `{"last_name": "input[name='lname']"}`
	err = SaveFormMapping(domain, newMapping)
	if err != nil {
		t.Fatalf("Failed to update form mapping: %v", err)
	}

	readMapping, _ = GetFormMapping(domain)
	if readMapping != newMapping {
		t.Errorf("Updated mapping mismatch. Expected %s, got %s", newMapping, readMapping)
	}

	// Delete
	err = DeleteFormMapping(domain)
	if err != nil {
		t.Fatalf("Failed to delete form mapping: %v", err)
	}

	_, err = GetFormMapping(domain)
	if err == nil {
		t.Fatalf("Expected error when getting deleted mapping, got nil")
	}
}

func TestExecutionLogs(t *testing.T) {
	setupTestDB(t)
	defer teardownTestDB()

	err := LogExecution("job123", "http://job123.com", "SUCCESS", 1500)
	if err != nil {
		t.Fatalf("Failed to log execution: %v", err)
	}

	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM execution_logs WHERE job_id = 'job123'").Scan(&count)
	if err != nil || count != 1 {
		t.Fatalf("Failed to verify execution log insertion, count=%d, err=%v", count, err)
	}
}

func TestCareerChunks(t *testing.T) {
	setupTestDB(t)
	defer teardownTestDB()

	embedding := []float32{0.1, 0.2, 0.3}

	err := SaveCareerChunk("Test Chunk", embedding)
	if err != nil {
		t.Fatalf("Failed to save career chunk: %v", err)
	}

	chunks, err := GetAllCareerChunks()
	if err != nil {
		t.Fatalf("Failed to get career chunks: %v", err)
	}
	if len(chunks) != 1 {
		t.Fatalf("Expected 1 chunk, got %d", len(chunks))
	}
	if chunks[0].Text != "Test Chunk" {
		t.Errorf("Chunk text mismatch: %s", chunks[0].Text)
	}
	if len(chunks[0].Embedding) != 3 {
		t.Errorf("Chunk embedding length mismatch: %d", len(chunks[0].Embedding))
	}

	err = ClearCareerChunks()
	if err != nil {
		t.Fatalf("Failed to clear career chunks: %v", err)
	}

	chunks, _ = GetAllCareerChunks()
	if len(chunks) != 0 {
		t.Fatalf("Expected 0 chunks after clear, got %d", len(chunks))
	}
}

func TestSaveApplication(t *testing.T) {
	setupTestDB(t)
	defer teardownTestDB()

	// Change current working directory or create applications dir to prevent polluting real tree if needed
	// Actually SaveApplication uses "applications/Company_Name" so we can clean it up
	companyName := "Test_Save_Company"
	defer os.RemoveAll(filepath.Join("applications", companyName))

	err := SaveApplication(
		companyName,
		"Test Role",
		"Remote",
		"http://test.com",
		"# Resume",
		"Dear hiring manager",
		"Prep notes",
	)
	if err != nil {
		t.Fatalf("Failed to save application: %v", err)
	}

	// Check if directory and files exist
	companyDir := filepath.Join("applications", companyName)
	if _, err := os.Stat(companyDir); os.IsNotExist(err) {
		t.Fatalf("Expected directory %s to be created", companyDir)
	}
	
	files := []string{"resume.md", "coverletter.txt", "interview_prep.md", "metadata.json"}
	for _, f := range files {
		if _, err := os.Stat(filepath.Join(companyDir, f)); os.IsNotExist(err) {
			t.Errorf("Expected file %s to be created", f)
		}
	}

	resumeBytes, err := os.ReadFile(filepath.Join(companyDir, "resume.md"))
	if err != nil || string(resumeBytes) != "# Resume" {
		t.Errorf("resume.md content mismatch or error: %v", err)
	}

	coverBytes, err := os.ReadFile(filepath.Join(companyDir, "coverletter.txt"))
	if err != nil || string(coverBytes) != "Dear hiring manager" {
		t.Errorf("coverletter.txt content mismatch or error: %v", err)
	}

	prepBytes, err := os.ReadFile(filepath.Join(companyDir, "interview_prep.md"))
	if err != nil || string(prepBytes) != "Prep notes" {
		t.Errorf("interview_prep.md content mismatch or error: %v", err)
	}

	// Verify DB record
	if !HasApplied("http://test.com") {
		t.Errorf("Expected URL to be marked as applied in DB")
	}
}

func TestLogFailedSubmission(t *testing.T) {
	reportPath := filepath.Join("applications", "manual_submissions.md")
	// Make sure we clean up
	os.MkdirAll("applications", 0755)
	defer os.Remove(reportPath)

	err := LogFailedSubmission("FailCorp", "Engineer", "http://fail.com")
	if err != nil {
		t.Fatalf("Failed to log failed submission: %v", err)
	}

	data, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatalf("Failed to read report file: %v", err)
	}
	
	content := string(data)
	if !strings.Contains(content, "FailCorp") || !strings.Contains(content, "http://fail.com") {
		t.Errorf("Report content mismatch: %s", content)
	}
	if !strings.Contains(content, "# Manual Submission Backlog") {
		t.Errorf("Missing markdown header in report")
	}
}
