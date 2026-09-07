package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/howlcipher/Career_Agent_Core/pkg/storage"
)

func TestDogfoodHandlers_RejectWrongMethod(t *testing.T) {
	setupTestDB(t)
	tests := []struct {
		name    string
		handler http.HandlerFunc
		method  string
		path    string
	}{
		{"start", serveDogfoodStart, http.MethodGet, "/api/dogfood/start"},
		{"active", serveDogfoodActive, http.MethodPost, "/api/dogfood/active"},
		{"feedback", serveDogfoodFeedback, http.MethodGet, "/api/dogfood/feedback"},
		{"cohorts", serveDogfoodCohorts, http.MethodPost, "/api/dogfood/cohorts"},
		{"report", serveDogfoodReport, http.MethodPost, "/api/dogfood/report"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			rec := httptest.NewRecorder()
			tt.handler(rec, req)
			if rec.Code != http.StatusMethodNotAllowed {
				t.Errorf("%s: status=%d, want 405", tt.name, rec.Code)
			}
		})
	}
}

func TestServeDogfoodActive_NullWhenNoneStarted(t *testing.T) {
	setupTestDB(t)
	req := httptest.NewRequest(http.MethodGet, "/api/dogfood/active", nil)
	rec := httptest.NewRecorder()
	serveDogfoodActive(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
	}
	var body struct {
		Cohort *storage.DogfoodCohort `json:"cohort"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Cohort != nil {
		t.Fatalf("expected a null cohort before any run starts, got %+v", body.Cohort)
	}
}

func TestServeDogfoodStart_CreatesCohortThenConflictsOnASecondCall(t *testing.T) {
	setupTestDB(t)
	req := httptest.NewRequest(http.MethodPost, "/api/dogfood/start", nil)
	rec := httptest.NewRecorder()
	serveDogfoodStart(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
	}
	var body struct {
		Cohort storage.DogfoodCohort `json:"cohort"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Cohort.TargetCount != storage.DogfoodCohortTarget {
		t.Fatalf("target_count=%d, want %d", body.Cohort.TargetCount, storage.DogfoodCohortTarget)
	}

	req2 := httptest.NewRequest(http.MethodPost, "/api/dogfood/start", nil)
	rec2 := httptest.NewRecorder()
	serveDogfoodStart(rec2, req2)
	if rec2.Code != http.StatusConflict {
		t.Fatalf("second start: status=%d, want 409", rec2.Code)
	}
}

func TestServeDogfoodFeedback_RequiresAJobID(t *testing.T) {
	setupTestDB(t)
	req := httptest.NewRequest(http.MethodPost, "/api/dogfood/feedback", bytes.NewBufferString(`{"category":"nothing"}`))
	rec := httptest.NewRecorder()
	serveDogfoodFeedback(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d, want 400", rec.Code)
	}
}

func TestServeDogfoodFeedback_RejectsUnknownCategory(t *testing.T) {
	setupTestDB(t)
	req := httptest.NewRequest(http.MethodPost, "/api/dogfood/feedback", bytes.NewBufferString(`{"job_id":"1","category":"not_a_real_category"}`))
	rec := httptest.NewRecorder()
	serveDogfoodFeedback(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d, want 400", rec.Code)
	}
}

func TestServeDogfoodReport_NullWhenNoCohortsExist(t *testing.T) {
	setupTestDB(t)
	req := httptest.NewRequest(http.MethodGet, "/api/dogfood/report", nil)
	rec := httptest.NewRecorder()
	serveDogfoodReport(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
	}
	var body struct {
		Report *storage.DogfoodReport `json:"report"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Report != nil {
		t.Fatalf("expected a null report with no cohorts yet, got %+v", body.Report)
	}
}

// TestDogfoodRun_EndToEnd exercises the whole cohort lifecycle through this
// package's own handlers plus the storage layer's confirmation path, the same
// way a real five-application dogfood run would: start, confirm five
// applications, leave feedback on one, then read the automatically completed
// report back out.
func TestDogfoodRun_EndToEnd(t *testing.T) {
	setupTestDB(t)

	startReq := httptest.NewRequest(http.MethodPost, "/api/dogfood/start", nil)
	startRec := httptest.NewRecorder()
	serveDogfoodStart(startRec, startReq)
	if startRec.Code != http.StatusOK {
		t.Fatalf("start: status=%d body=%q", startRec.Code, startRec.Body.String())
	}

	now := time.Now().UTC()
	for i := 1; i <= 5; i++ {
		if _, err := db.Exec(`INSERT INTO job_funnel (url, id, company_name, job_title, status, discovered_at)
			VALUES (?, ?, ?, 'Engineer', 'AWAITING_REVIEW', ?)`,
			"https://boards.greenhouse.io/example/jobs/dogfood-"+string(rune('0'+i)), i, "Company", now); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`INSERT INTO assisted_applications
			(job_id, original_status, next_action_code, assisted_state, revalidation_state, revalidation_version, created_at, updated_at)
			VALUES (?, 'AWAITING_REVIEW', 'review_and_submit', 'waiting_human', 'required', 3, ?, ?)`,
			i, now, now); err != nil {
			t.Fatal(err)
		}
		jobID := string(rune('0' + i))
		ordinal, err := storage.ConfirmAssistedSubmission(db, jobID)
		if err != nil {
			t.Fatalf("confirm job %d: %v", i, err)
		}
		if ordinal != i {
			t.Fatalf("job %d: expected ordinal %d, got %d", i, i, ordinal)
		}
	}

	feedbackReq := httptest.NewRequest(http.MethodPost, "/api/dogfood/feedback",
		bytes.NewBufferString(`{"job_id":"3","category":"bad_match"}`))
	feedbackRec := httptest.NewRecorder()
	serveDogfoodFeedback(feedbackRec, feedbackReq)
	if feedbackRec.Code != http.StatusOK {
		t.Fatalf("feedback: status=%d body=%q", feedbackRec.Code, feedbackRec.Body.String())
	}

	activeReq := httptest.NewRequest(http.MethodGet, "/api/dogfood/active", nil)
	activeRec := httptest.NewRecorder()
	serveDogfoodActive(activeRec, activeReq)
	var activeBody struct {
		Cohort *storage.DogfoodCohort `json:"cohort"`
	}
	if err := json.Unmarshal(activeRec.Body.Bytes(), &activeBody); err != nil {
		t.Fatal(err)
	}
	if activeBody.Cohort != nil {
		t.Fatalf("expected the cohort to have auto-completed after the fifth confirmation, got %+v", activeBody.Cohort)
	}

	reportReq := httptest.NewRequest(http.MethodGet, "/api/dogfood/report", nil)
	reportRec := httptest.NewRecorder()
	serveDogfoodReport(reportRec, reportReq)
	if reportRec.Code != http.StatusOK {
		t.Fatalf("report: status=%d body=%q", reportRec.Code, reportRec.Body.String())
	}
	var reportBody struct {
		Report *storage.DogfoodReport `json:"report"`
	}
	if err := json.Unmarshal(reportRec.Body.Bytes(), &reportBody); err != nil {
		t.Fatal(err)
	}
	if reportBody.Report == nil || len(reportBody.Report.Applications) != 5 {
		t.Fatalf("expected a completed report with five applications, got %+v", reportBody.Report)
	}
	if reportBody.Report.BadMatches != 1 {
		t.Fatalf("expected the one bad_match feedback to be counted, got %d", reportBody.Report.BadMatches)
	}
	if reportBody.Report.Verdict == "" {
		t.Fatal("expected a non-empty verdict on a completed cohort report")
	}
}
