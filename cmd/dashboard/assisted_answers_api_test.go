package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/howlcipher/Career_Agent_Core/pkg/answers"
	"github.com/howlcipher/Career_Agent_Core/pkg/storage"
)

// seedAnswerableJob puts one application into needs_answers with a live lease,
// which is the only state the answers endpoint accepts.
func seedAnswerableJob(t *testing.T, questions []storage.ApplicationQuestion) {
	t.Helper()
	now := time.Now().UTC()
	if _, err := db.Exec(`INSERT INTO job_funnel (url, id, company_name, job_title, status, last_updated, discovered_at)
		VALUES ('https://boards.greenhouse.io/example/jobs/1', 1, 'Grafana Labs', 'Senior Platform Engineer', 'AWAITING_REVIEW', ?, ?)`, now, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO assisted_applications (job_id, original_status, next_action_code, interruption_reason,
		assisted_state, lease_owner, lease_expires_at, created_at, updated_at)
		VALUES (1, 'AWAITING_REVIEW', 'review_and_submit', '', 'needs_answers', 'owner', ?, ?, ?)`,
		now.Add(10*time.Minute), now, now); err != nil {
		t.Fatal(err)
	}
	if err := storage.ReplaceApplicationQuestions(db, "1", questions, storage.AssistedFillSummary{JobID: "1"}); err != nil {
		t.Fatal(err)
	}
}

func postAnswers(t *testing.T, body any) *httptest.ResponseRecorder {
	t.Helper()
	encoded, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/assisted/answers", bytes.NewReader(encoded))
	recorder := httptest.NewRecorder()
	serveAssistedAnswers(recorder, request)
	return recorder
}

func TestServeAssistedAnswers_SendsAnswersAndSavesOnlyWhatWasApproved(t *testing.T) {
	setupTestDB(t)
	seedAnswerableJob(t, []storage.ApplicationQuestion{
		{JobID: "1", Key: "github", Prompt: "GitHub profile URL", ControlType: "text", Sensitivity: "routine"},
		{JobID: "1", Key: "backstage", Prompt: "Have you used Backstage professionally?", ControlType: "radio", Sensitivity: "routine"},
	})

	recorder := postAnswers(t, map[string]any{
		"job_id": "1",
		"answers": []map[string]any{
			{"key": "github", "answer": "https://example.invalid/someone", "save_for_reuse": true, "allow_sensitive_reuse": false, "scope": "global"},
			{"key": "backstage", "answer": "Yes", "save_for_reuse": false, "allow_sensitive_reuse": false, "scope": "global"},
		},
	})
	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%q", recorder.Code, recorder.Body.String())
	}

	// Both answers reach the browser; only the one the operator asked to keep
	// is stored.
	values, err := storage.TakePendingAnswers(db, "1")
	if err != nil {
		t.Fatal(err)
	}
	if len(values) != 2 || values["backstage"] != "Yes" {
		t.Fatalf("both answers should have been queued for the browser: %+v", values)
	}
	vault, err := answers.OpenStore(db)
	if err != nil {
		t.Fatal(err)
	}
	stored, err := vault.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(stored) != 1 || stored[0].CanonicalQuestion != "GitHub profile URL" {
		t.Fatalf("only the approved answer should be stored: %+v", stored)
	}
	if !stored[0].ReuseAllowed {
		t.Error("a routine answer the operator chose to save should be reusable")
	}
}

// The endpoint must not be able to store a declaration on the strength of one
// checkbox. The vault refuses it, and the operator is told.
func TestServeAssistedAnswers_RefusesToRememberADeclarationWithoutTheSecondAcknowledgement(t *testing.T) {
	setupTestDB(t)
	seedAnswerableJob(t, []storage.ApplicationQuestion{
		{JobID: "1", Key: "auth", Prompt: "Are you legally authorized to work in the United States?", ControlType: "radio", Sensitivity: "sensitive"},
	})

	recorder := postAnswers(t, map[string]any{
		"job_id": "1",
		"answers": []map[string]any{
			{"key": "auth", "answer": "Yes", "save_for_reuse": true, "allow_sensitive_reuse": false, "scope": "global"},
		},
	})
	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%q", recorder.Code, recorder.Body.String())
	}
	var response struct {
		AnswersSaved   int `json:"answers_saved"`
		AnswersRefused int `json:"answers_refused"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.AnswersSaved != 0 || response.AnswersRefused != 1 {
		t.Fatalf("the declaration should have been refused and reported: %+v", response)
	}

	vault, _ := answers.OpenStore(db)
	stored, _ := vault.List()
	if len(stored) != 0 {
		t.Fatalf("no declaration may be stored without the reuse acknowledgement: %+v", stored)
	}
	// The answer still reached the application — refusing to *remember* it is
	// not the same as refusing to send it.
	values, _ := storage.TakePendingAnswers(db, "1")
	if values["auth"] != "Yes" {
		t.Fatalf("the operator's answer should still reach the browser: %+v", values)
	}
}

func TestServeAssistedAnswers_StoresADeclarationWhenBothAcknowledgementsAreGiven(t *testing.T) {
	setupTestDB(t)
	seedAnswerableJob(t, []storage.ApplicationQuestion{
		{JobID: "1", Key: "auth", Prompt: "Are you legally authorized to work in the United States?", ControlType: "radio", Sensitivity: "sensitive"},
	})
	recorder := postAnswers(t, map[string]any{
		"job_id": "1",
		"answers": []map[string]any{
			{"key": "auth", "answer": "Yes", "save_for_reuse": true, "allow_sensitive_reuse": true, "scope": "global"},
		},
	})
	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%q", recorder.Code, recorder.Body.String())
	}
	vault, _ := answers.OpenStore(db)
	stored, _ := vault.List()
	if len(stored) != 1 || !stored[0].ReuseAllowed {
		t.Fatalf("an explicitly acknowledged declaration should be stored and reusable: %+v", stored)
	}
	if stored[0].Provenance != answers.OperatorEdited && stored[0].Provenance != answers.OperatorApproved {
		t.Fatalf("provenance must record that a human approved it, got %q", stored[0].Provenance)
	}
}

// The key selects which control gets typed into, so an unrecognized one would
// let a request fill a field the operator was never shown.
func TestServeAssistedAnswers_RefusesAnAnswerToAQuestionThisApplicationNeverAsked(t *testing.T) {
	setupTestDB(t)
	seedAnswerableJob(t, []storage.ApplicationQuestion{
		{JobID: "1", Key: "github", Prompt: "GitHub profile URL", ControlType: "text", Sensitivity: "routine"},
	})
	recorder := postAnswers(t, map[string]any{
		"job_id": "1",
		"answers": []map[string]any{
			{"key": "ssn", "answer": "123-45-6789", "save_for_reuse": false, "allow_sensitive_reuse": false, "scope": "global"},
		},
	})
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected the request to be refused, got %d %q", recorder.Code, recorder.Body.String())
	}
	values, _ := storage.TakePendingAnswers(db, "1")
	if len(values) != 0 {
		t.Fatalf("nothing should have been queued: %+v", values)
	}
}

// The queue projection carries prompts and Career Agent's own proposals. It
// must never carry back what the operator typed.
func TestAssistedQueue_NeverServesTheOperatorsAnswers(t *testing.T) {
	setupTestDB(t)
	seedAnswerableJob(t, []storage.ApplicationQuestion{
		{JobID: "1", Key: "comp", Prompt: "Desired compensation", ControlType: "text", Sensitivity: "sensitive"},
	})
	const secret = "$165,432-unique-token"
	if recorder := postAnswers(t, map[string]any{
		"job_id":  "1",
		"answers": []map[string]any{{"key": "comp", "answer": secret, "save_for_reuse": false, "allow_sensitive_reuse": false, "scope": "global"}},
	}); recorder.Code != http.StatusOK {
		t.Fatalf("status=%d", recorder.Code)
	}

	request := httptest.NewRequest(http.MethodGet, "/api/assisted", nil)
	recorder := httptest.NewRecorder()
	serveAssistedQueue(recorder, request)
	if bytes.Contains(recorder.Body.Bytes(), []byte(secret)) {
		t.Fatal("the assisted queue served an answer the operator typed onto a real application")
	}
}

func TestServeApplySessionControl_RejectsAnUnknownAction(t *testing.T) {
	setupTestDB(t)
	body, _ := json.Marshal(map[string]string{"action": "delete_everything", "job_id": ""})
	request := httptest.NewRequest(http.MethodPost, "/api/apply-session/control", bytes.NewReader(body))
	recorder := httptest.NewRecorder()
	serveApplySessionControl(recorder, request)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected an unknown action to be refused, got %d", recorder.Code)
	}
}

func TestServeApplySessionStart_ValidatesTheSelection(t *testing.T) {
	setupTestDB(t)
	for _, testCase := range []struct {
		name   string
		jobIDs []string
	}{
		{"empty", nil},
		{"non-numeric", []string{"'; DROP TABLE job_funnel; --"}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			body, _ := json.Marshal(map[string]any{"job_ids": testCase.jobIDs})
			request := httptest.NewRequest(http.MethodPost, "/api/apply-session/start", bytes.NewReader(body))
			recorder := httptest.NewRecorder()
			serveApplySessionStart(recorder, request)
			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("expected a bad request, got %d", recorder.Code)
			}
		})
	}
}
