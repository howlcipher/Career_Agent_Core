package main

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/howlcipher/Career_Agent_Core/pkg/storage"
)

// seedQueuedQuestion puts one queued application with one pending question into
// the test database, with no lease -- the state the knowledge inbox reads.
func seedQueuedQuestion(t *testing.T, jobID int, company string, questions ...storage.ApplicationQuestion) {
	t.Helper()
	now := time.Now().UTC()
	if _, err := db.Exec(`INSERT INTO job_funnel (url, id, company_name, job_title, status, last_updated, discovered_at)
		VALUES (?, ?, ?, 'Senior Platform Engineer', 'AWAITING_REVIEW', ?, ?)`,
		"https://boards.greenhouse.io/example/jobs/"+company, jobID, company, now, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO assisted_applications (job_id, original_status, next_action_code, interruption_reason,
		assisted_state, created_at, updated_at)
		VALUES (?, 'AWAITING_REVIEW', 'review_and_submit', '', 'waiting_human', ?, ?)`, jobID, now, now); err != nil {
		t.Fatal(err)
	}
	id := strconv.Itoa(jobID)
	for i := range questions {
		questions[i].JobID = id
	}
	if err := storage.ReplaceApplicationQuestions(db, id, questions, storage.AssistedFillSummary{JobID: id}); err != nil {
		t.Fatal(err)
	}
}

// captureLog redirects the standard logger for one test and returns what was
// written. Used to prove a value never reaches it.
func captureLog(t *testing.T) func() string {
	t.Helper()
	var captured bytes.Buffer
	previousWriter := log.Writer()
	previousFlags := log.Flags()
	log.SetOutput(&captured)
	t.Cleanup(func() {
		log.SetOutput(previousWriter)
		log.SetFlags(previousFlags)
	})
	return captured.String
}

// usePrivatePII points the profile endpoint at a temporary file, so no test can
// read or write the operator's real details.
func usePrivatePII(t *testing.T, contents string) string {
	t.Helper()
	original := piiPath
	t.Cleanup(func() { piiPath = original })
	path := filepath.Join(t.TempDir(), "pii.yaml")
	if contents != "" {
		if err := os.WriteFile(path, []byte(contents), 0600); err != nil {
			t.Fatal(err)
		}
	}
	piiPath = path
	return path
}

func getJSON(t *testing.T, handler http.HandlerFunc, target string) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	recorder := httptest.NewRecorder()
	handler(recorder, httptest.NewRequest(http.MethodGet, target, nil))
	payload := map[string]any{}
	if recorder.Code == http.StatusOK {
		if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
			t.Fatalf("decode %s: %v (body %q)", target, err, recorder.Body.String())
		}
	}
	return recorder, payload
}

func postJSON(t *testing.T, handler http.HandlerFunc, target string, body any) *httptest.ResponseRecorder {
	t.Helper()
	encoded, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	handler(recorder, httptest.NewRequest(http.MethodPost, target, bytes.NewReader(encoded)))
	return recorder
}

func TestServeKnowledge_ReportsTheQueuesOwnDemand(t *testing.T) {
	setupTestDB(t)
	usePrivatePII(t, "first_name: Test\nlinks:\n  linkedin: https://example.com/in/test\n")

	seedQueuedQuestion(t, 1, "Acme",
		storage.ApplicationQuestion{Key: "a", Prompt: "How many years of Kubernetes experience do you have?", ControlType: "text"},
		storage.ApplicationQuestion{Key: "b", Prompt: "LinkedIn profile URL", ControlType: "text"},
	)
	seedQueuedQuestion(t, 2, "Globex",
		storage.ApplicationQuestion{Key: "c", Prompt: "Years of Kubernetes experience", ControlType: "text"},
	)

	recorder, payload := getJSON(t, serveKnowledge, "/api/knowledge")
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body %q", recorder.Code, recorder.Body.String())
	}
	if recorder.Header().Get("Cache-Control") != "no-store" {
		t.Error("a response carrying the operator's answers must not be cached")
	}

	groups, _ := payload["groups"].([]any)
	if len(groups) != 1 {
		t.Fatalf("two phrasings of one question must be one group, got %d", len(groups))
	}
	group := groups[0].(map[string]any)
	if group["applications"].(float64) != 2 {
		t.Fatalf("applications = %v, want 2", group["applications"])
	}
	readiness := payload["readiness"].(map[string]any)
	// The LinkedIn field is answerable from configured details and is not work.
	if readiness["resolved"].(float64) != 1 {
		t.Fatalf("resolved = %v, want 1", readiness["resolved"])
	}
	if readiness["unique_questions"].(float64) != 1 {
		t.Fatalf("unique questions = %v, want 1", readiness["unique_questions"])
	}
}

func TestServeKnowledgeApprove_ResolvesEveryApplicationWaitingOnIt(t *testing.T) {
	setupTestDB(t)
	usePrivatePII(t, "first_name: Test\n")
	seedQueuedQuestion(t, 1, "Acme",
		storage.ApplicationQuestion{Key: "a", Prompt: "How many years of Terraform experience do you have?", ControlType: "text"})
	seedQueuedQuestion(t, 2, "Globex",
		storage.ApplicationQuestion{Key: "b", Prompt: "Years of Terraform experience", ControlType: "text"})

	_, payload := getJSON(t, serveKnowledge, "/api/knowledge")
	group := payload["groups"].([]any)[0].(map[string]any)

	recorder := postJSON(t, serveKnowledgeApprove, "/api/knowledge/approve", map[string]any{
		"group_key": group["key"], "answer": "4", "save_for_reuse": true, "scope": "global",
	})
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body %q", recorder.Code, recorder.Body.String())
	}
	result := map[string]any{}
	if err := json.Unmarshal(recorder.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result["questions_resolved"].(float64) != 2 {
		t.Fatalf("questions resolved = %v, want 2", result["questions_resolved"])
	}
	if result["applications_helped"].(float64) != 2 {
		t.Fatalf("applications helped = %v, want 2", result["applications_helped"])
	}

	// And the inbox is now empty, which is the operator-visible half of the claim.
	_, after := getJSON(t, serveKnowledge, "/api/knowledge")
	if groups := after["groups"].([]any); len(groups) != 0 {
		t.Fatalf("expected an empty inbox after approval, got %+v", groups)
	}
}

func TestServeKnowledgeApprove_RefusesADeclarationWithoutTheSecondAcknowledgement(t *testing.T) {
	setupTestDB(t)
	usePrivatePII(t, "first_name: Test\n")
	seedQueuedQuestion(t, 1, "Acme", storage.ApplicationQuestion{
		Key: "a", Prompt: "Have you ever been convicted of a felony?", ControlType: "radio",
		Options: []string{"Yes", "No"},
	})
	_, payload := getJSON(t, serveKnowledge, "/api/knowledge")
	group := payload["groups"].([]any)[0].(map[string]any)

	recorder := postJSON(t, serveKnowledgeApprove, "/api/knowledge/approve", map[string]any{
		"group_key": group["key"], "answer": "No", "save_for_reuse": true,
	})
	if recorder.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409; body %q", recorder.Code, recorder.Body.String())
	}
	// The message has to tell the operator what to do, not just refuse.
	if !strings.Contains(recorder.Body.String(), "declaration") {
		t.Errorf("the refusal should explain itself, got %q", recorder.Body.String())
	}
}

func TestServeKnowledgeApprove_RejectsUnknownFieldsAndForeignGroups(t *testing.T) {
	setupTestDB(t)
	usePrivatePII(t, "first_name: Test\n")
	seedQueuedQuestion(t, 1, "Acme",
		storage.ApplicationQuestion{Key: "a", Prompt: "What is your T-shirt size?", ControlType: "text"})

	// An answer to a question that is not in the queue is refused rather than
	// stored, the same way the assisted answers endpoint refuses a key the
	// application never asked.
	foreign := postJSON(t, serveKnowledgeApprove, "/api/knowledge/approve", map[string]any{
		"group_key": "q:something nobody asked", "answer": "L", "save_for_reuse": true,
	})
	if foreign.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409", foreign.Code)
	}

	body := bytes.NewReader([]byte(`{"group_key":"q:x","answer":"L","surprise":true}`))
	recorder := httptest.NewRecorder()
	serveKnowledgeApprove(recorder, httptest.NewRequest(http.MethodPost, "/api/knowledge/approve", body))
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("an unknown field must be rejected, got %d", recorder.Code)
	}
}

func TestServeKnowledgeField_AnswersOnlyWhatItMayFill(t *testing.T) {
	setupTestDB(t)
	usePrivatePII(t, "links:\n  linkedin: https://example.com/in/test\n")

	_, fillable := getJSON(t, serveKnowledgeField, "/api/knowledge/field?prompt=LinkedIn+profile+URL&control_type=text")
	if fillable["policy"] != "safe_auto_fill" || fillable["requires_human"] != false {
		t.Fatalf("LinkedIn should be safe to fill: %+v", fillable)
	}
	if fillable["answer"] == nil || fillable["answer"] == "" {
		t.Fatal("a fillable field must carry its answer")
	}

	_, declaration := getJSON(t, serveKnowledgeField,
		"/api/knowledge/field?prompt=Are+you+legally+authorized+to+work+in+the+United+States%3F&control_type=radio")
	if declaration["requires_human"] != true {
		t.Fatalf("a work-authorization question must require a human: %+v", declaration)
	}
	if answer, present := declaration["answer"]; present && answer != "" {
		t.Fatalf("a declaration must never be returned as a fillable answer: %v", answer)
	}
}

func TestServeKnowledgeField_RefusesAnEmptyOrOversizedPrompt(t *testing.T) {
	setupTestDB(t)
	usePrivatePII(t, "")
	for _, target := range []string{
		"/api/knowledge/field",
		"/api/knowledge/field?prompt=" + strings.Repeat("a", 2100),
	} {
		recorder, _ := getJSON(t, serveKnowledgeField, target)
		if recorder.Code != http.StatusBadRequest {
			t.Errorf("%s: status = %d, want 400", target, recorder.Code)
		}
	}
}

func TestServePreflight_RefusesWhileAnAssistedBrowserIsOpen(t *testing.T) {
	setupTestDB(t)
	usePrivatePII(t, "")
	now := time.Now().UTC()
	if _, err := db.Exec(`INSERT INTO job_funnel (url, id, company_name, job_title, status, last_updated, discovered_at)
		VALUES ('https://boards.greenhouse.io/example/jobs/1', 1, 'Acme', 'Engineer', 'AWAITING_REVIEW', ?, ?)`, now, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO assisted_applications (job_id, original_status, next_action_code, interruption_reason,
		assisted_state, lease_owner, lease_expires_at, created_at, updated_at)
		VALUES (1, 'AWAITING_REVIEW', 'review_and_submit', '', 'waiting_human', 'owner', ?, ?, ?)`,
		now.Add(10*time.Minute), now, now); err != nil {
		t.Fatal(err)
	}

	// Two Chromium instances plus local inference is how this machine falls
	// over, and the operator is mid-application anyway.
	recorder := postJSON(t, servePreflight, "/api/knowledge/preflight", map[string]any{"job_ids": []string{"1"}})
	if recorder.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409; body %q", recorder.Code, recorder.Body.String())
	}
}

func TestServePreflight_StartsOnlyOneRunAtATime(t *testing.T) {
	setupTestDB(t)
	usePrivatePII(t, "")
	original := startPreflight
	t.Cleanup(func() {
		startPreflight = original
		finishPreflight()
	})
	// Stand in for the child process: claim the run and never finish it.
	startPreflight = func(jobIDs []string) error {
		currentPreflight.mutex.Lock()
		defer currentPreflight.mutex.Unlock()
		if currentPreflight.running {
			return errPreflightBusy
		}
		currentPreflight.running = true
		currentPreflight.jobs = len(jobIDs)
		return nil
	}

	first := postJSON(t, servePreflight, "/api/knowledge/preflight", map[string]any{"job_ids": []string{"1", "2"}})
	if first.Code != http.StatusOK {
		t.Fatalf("first run status = %d, body %q", first.Code, first.Body.String())
	}
	second := postJSON(t, servePreflight, "/api/knowledge/preflight", map[string]any{"job_ids": []string{"3"}})
	if second.Code != http.StatusConflict {
		t.Fatalf("a second concurrent run must be refused, got %d", second.Code)
	}

	_, status := getJSON(t, servePreflight, "/api/knowledge/preflight")
	if status["running"] != true || status["applications"].(float64) != 2 {
		t.Fatalf("status should report the run in progress: %+v", status)
	}
}

func TestServeKnowledgeProfile_SavesAtomicallyAndKeepsTheFilePrivate(t *testing.T) {
	setupTestDB(t)
	path := usePrivatePII(t, "first_name: Existing\nphone: \"555-0100\"\n")

	recorder := postJSON(t, serveKnowledgeProfile, "/api/knowledge/profile", map[string]any{
		"fields": map[string]string{"work.notice_period": "Two weeks", "city": "Denver"},
	})
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body %q", recorder.Code, recorder.Body.String())
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("pii.yaml permissions = %v, want 0600", info.Mode().Perm())
	}
	saved, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	// A patch must not erase what it did not mention.
	for _, expected := range []string{"Existing", "555-0100", "Two weeks", "Denver"} {
		if !strings.Contains(string(saved), expected) {
			t.Errorf("saved file lost %q:\n%s", expected, saved)
		}
	}

	// A backup of the previous contents exists, at the same permissions.
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	backups := 0
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".bak") {
			backups++
			backupInfo, err := entry.Info()
			if err != nil {
				t.Fatal(err)
			}
			if backupInfo.Mode().Perm() != 0600 {
				t.Errorf("backup permissions = %v, want 0600", backupInfo.Mode().Perm())
			}
		}
		// Nothing may be left behind mid-write.
		if strings.HasSuffix(entry.Name(), ".tmp") {
			t.Errorf("a temporary file survived the save: %s", entry.Name())
		}
	}
	if backups != 1 {
		t.Errorf("expected exactly one backup, found %d", backups)
	}
}

func TestServeKnowledgeProfile_RefusesAnythingOutsideTheKnownSchema(t *testing.T) {
	setupTestDB(t)
	path := usePrivatePII(t, "first_name: Existing\n")

	// An arbitrary YAML key must not be introducible over HTTP.
	recorder := postJSON(t, serveKnowledgeProfile, "/api/knowledge/profile", map[string]any{
		"fields": map[string]string{"eeo.race_ethnicity": "..."},
	})
	if recorder.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409", recorder.Code)
	}
	saved, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(saved), "race_ethnicity") {
		t.Fatal("an unknown key reached pii.yaml")
	}
	// A refused save must not have rewritten the file at all.
	if strings.TrimSpace(string(saved)) != "first_name: Existing" {
		t.Fatalf("a refused save modified the file:\n%s", saved)
	}
}

func TestServeKnowledgeProfile_NeverLogsAValue(t *testing.T) {
	setupTestDB(t)
	usePrivatePII(t, "")

	captured := captureLog(t)
	recorder := postJSON(t, serveKnowledgeProfile, "/api/knowledge/profile", map[string]any{
		"fields": map[string]string{"phone": "555-0199", "city": "Reykjavik"},
	})
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body %q", recorder.Code, recorder.Body.String())
	}
	logged := captured()
	for _, secret := range []string{"555-0199", "Reykjavik"} {
		if strings.Contains(logged, secret) {
			t.Errorf("a configured detail reached the log: %q in %q", secret, logged)
		}
	}
	// The response is a count too, not an echo.
	if strings.Contains(recorder.Body.String(), "555-0199") {
		t.Error("the response echoed a value back")
	}
}

func TestServeKnowledgeMethods_AreGated(t *testing.T) {
	setupTestDB(t)
	usePrivatePII(t, "")
	cases := []struct {
		name    string
		handler http.HandlerFunc
		method  string
		target  string
	}{
		{"knowledge is read-only", serveKnowledge, http.MethodPost, "/api/knowledge"},
		{"approve is write-only", serveKnowledgeApprove, http.MethodGet, "/api/knowledge/approve"},
		{"field is read-only", serveKnowledgeField, http.MethodPost, "/api/knowledge/field"},
		{"profile refuses delete", serveKnowledgeProfile, http.MethodDelete, "/api/knowledge/profile"},
		{"preflight refuses delete", servePreflight, http.MethodDelete, "/api/knowledge/preflight"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			testCase.handler(recorder, httptest.NewRequest(testCase.method, testCase.target, nil))
			if recorder.Code != http.StatusMethodNotAllowed {
				t.Errorf("status = %d, want 405", recorder.Code)
			}
		})
	}
}
