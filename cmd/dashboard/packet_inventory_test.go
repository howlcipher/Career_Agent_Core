package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/howlcipher/Career_Agent_Core/pkg/storage"
)

// packetInventory reads just the form-inventory object out of a packet
// response, which is the part bugs.md #547 added.
type packetInventory struct {
	State         string `json:"state"`
	QuestionCount int    `json:"question_count"`
	FieldCount    int    `json:"field_count"`
	Reason        string `json:"reason"`
	InspectedAt   string `json:"inspected_at"`
	Source        string `json:"source"`
	Preparable    bool   `json:"preparable"`
	Stale         bool   `json:"stale"`
}

func fetchPacket(t *testing.T, jobID string) (packetInventory, string) {
	t.Helper()
	recorder := httptest.NewRecorder()
	serveApplicationPacket(recorder,
		httptest.NewRequest(http.MethodGet, "/api/assisted/packet?job_id="+jobID, nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body %q", recorder.Code, recorder.Body.String())
	}
	var payload struct {
		Inventory packetInventory `json:"form_inventory"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	return payload.Inventory, recorder.Body.String()
}

// seedUnpreparedJob is the state bugs.md #547 was found in: a queued Lever
// application with no verdict and no questions.
func seedUnpreparedJob(t *testing.T, jobID int, company string) {
	t.Helper()
	now := time.Now().UTC()
	if _, err := db.Exec(`INSERT INTO job_funnel (url, id, company_name, job_title, status, last_updated, discovered_at)
		VALUES (?, ?, ?, 'Senior Platform Engineer', 'AWAITING_REVIEW', ?, ?)`,
		"https://jobs.lever.co/"+company+"/abc123", jobID, company, now, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO assisted_applications (job_id, original_status, next_action_code, interruption_reason,
		assisted_state, created_at, updated_at)
		VALUES (?, 'AWAITING_REVIEW', 'review_and_submit', '', 'waiting_human', ?, ?)`, jobID, now, now); err != nil {
		t.Fatal(err)
	}
}

// The packet must always carry the object. An absent field would leave the UI
// inferring preparation from the entry list again, which is the defect.
func TestServeApplicationPacket_AlwaysCarriesFormInventory(t *testing.T) {
	setupTestDB(t)
	usePrivatePII(t, "first_name: Ada\nlast_name: Lovelace\n")
	seedUnpreparedJob(t, 1, "acme")

	inventory, body := fetchPacket(t, "1")
	if !strings.Contains(body, `"form_inventory"`) {
		t.Fatal("the packet must always carry a form_inventory object")
	}
	if inventory.State != storage.FormInventoryNotPrepared {
		t.Fatalf("state = %q, want %q", inventory.State, storage.FormInventoryNotPrepared)
	}
	if !inventory.Preparable {
		t.Fatal("a queued Lever application must be offerable for preparation")
	}
}

// A never-prepared packet still lists the operator's stored details -- the
// operator came for those -- but it must not be describable as complete.
func TestServeApplicationPacket_NeverPreparedStillServesDetailsWithoutClaimingCompleteness(t *testing.T) {
	setupTestDB(t)
	usePrivatePII(t, "first_name: Ada\nlast_name: Lovelace\nemail: ada@example.com\n")
	seedUnpreparedJob(t, 1, "acme")

	inventory, body := fetchPacket(t, "1")
	if !strings.Contains(body, "Ada Lovelace") {
		t.Fatal("the stored details must still be served")
	}
	if inventory.State != storage.FormInventoryNotPrepared {
		t.Fatalf("state = %q, want not_prepared", inventory.State)
	}
	if inventory.QuestionCount != 0 || inventory.Source != "" {
		t.Fatalf("an uninspected form has no inventory: %+v", inventory)
	}
}

// The two zero-question states, through the API this time.
func TestServeApplicationPacket_ZeroQuestionsReadyIsNotNeverPrepared(t *testing.T) {
	setupTestDB(t)
	usePrivatePII(t, "first_name: Ada\n")
	seedUnpreparedJob(t, 1, "read")
	seedUnpreparedJob(t, 2, "unread")
	if err := storage.RecordPreflight(db, storage.PreflightResult{
		JobID: "1", State: storage.PreflightInspected, ATS: "Lever", ControlCount: 14,
	}, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}

	ready, _ := fetchPacket(t, "1")
	never, _ := fetchPacket(t, "2")
	if ready.QuestionCount != never.QuestionCount {
		t.Fatal("this test is meaningless unless both render zero questions")
	}
	if ready.State != storage.FormInventoryReady {
		t.Fatalf("inspected state = %q, want ready", ready.State)
	}
	if never.State != storage.FormInventoryNotPrepared {
		t.Fatalf("uninspected state = %q, want not_prepared", never.State)
	}
	if ready.FieldCount != 14 {
		t.Fatalf("field count = %d, want 14", ready.FieldCount)
	}
}

func TestServeApplicationPacket_ReadyWithQuestionsReportsThem(t *testing.T) {
	setupTestDB(t)
	usePrivatePII(t, "first_name: Ada\n")
	seedQueuedQuestion(t, 1, "LeverCo",
		storage.ApplicationQuestion{Key: "a", Prompt: "What is your notice period?", ControlType: "text"},
		storage.ApplicationQuestion{Key: "b", Prompt: "Pronouns", ControlType: "text"},
	)
	if err := storage.RecordPreflight(db, storage.PreflightResult{
		JobID: "1", State: storage.PreflightInspected, ATS: "Greenhouse", ControlCount: 21,
	}, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}

	inventory, body := fetchPacket(t, "1")
	if inventory.State != storage.FormInventoryReady {
		t.Fatalf("state = %q, want ready", inventory.State)
	}
	if inventory.QuestionCount != 2 {
		t.Fatalf("question count = %d, want 2", inventory.QuestionCount)
	}
	// The questions themselves still travel as entries, unchanged by #547.
	if !strings.Contains(body, "What is your notice period?") {
		t.Fatal("the employer's own questions must still be in the packet")
	}
}

// A failed inspection must not be reported as either ready or never-prepared,
// and its reason must stay inside the closed vocabulary.
func TestServeApplicationPacket_FailedInspectionIsItsOwnStateWithABoundedReason(t *testing.T) {
	setupTestDB(t)
	usePrivatePII(t, "first_name: Ada\n")
	seedUnpreparedJob(t, 1, "gone")
	if err := storage.RecordPreflight(db, storage.PreflightResult{
		JobID: "1", State: storage.PreflightUnavailable, Reason: "posting_dead",
	}, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}

	inventory, body := fetchPacket(t, "1")
	if inventory.State != storage.FormInventoryFailed {
		t.Fatalf("state = %q, want failed", inventory.State)
	}
	if inventory.Reason != "posting_dead" {
		t.Fatalf("reason = %q", inventory.Reason)
	}
	// Nothing resembling a driver message may reach the browser: those quote
	// page content (ADR-006).
	for _, leak := range []string{"playwright", "Timeout", "net::ERR", "Target closed"} {
		if strings.Contains(strings.ToLower(body), strings.ToLower(leak)) {
			t.Fatalf("a driver diagnostic reached the packet: %q", leak)
		}
	}
}

// While a run is in flight the packet says so, rather than reporting the state
// the run is about to replace.
func TestServeApplicationPacket_ReportsPreparingWhileARunHoldsThisJob(t *testing.T) {
	setupTestDB(t)
	usePrivatePII(t, "first_name: Ada\n")
	seedUnpreparedJob(t, 1, "running")
	seedUnpreparedJob(t, 2, "bystander")

	currentPreflight.mutex.Lock()
	currentPreflight.running = true
	currentPreflight.jobs = 1
	currentPreflight.jobIDs = map[string]bool{"1": true}
	currentPreflight.mutex.Unlock()
	t.Cleanup(finishPreflight)

	preparing, _ := fetchPacket(t, "1")
	if preparing.State != storage.FormInventoryPreparing {
		t.Fatalf("state = %q, want preparing", preparing.State)
	}
	// An application that is not in the run is unaffected. A run-wide "busy"
	// flag would have claimed every open packet was being prepared.
	bystander, _ := fetchPacket(t, "2")
	if bystander.State != storage.FormInventoryNotPrepared {
		t.Fatalf("an application outside the run reported %q", bystander.State)
	}
}

// A run in flight outranks a stale verdict: a form being re-read right now is
// neither ready nor failed yet.
func TestServeApplicationPacket_PreparingOutranksAnEarlierVerdict(t *testing.T) {
	setupTestDB(t)
	usePrivatePII(t, "first_name: Ada\n")
	seedUnpreparedJob(t, 1, "retry")
	if err := storage.RecordPreflight(db, storage.PreflightResult{
		JobID: "1", State: storage.PreflightUnavailable, Reason: "navigation_failed",
	}, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}

	currentPreflight.mutex.Lock()
	currentPreflight.running = true
	currentPreflight.jobIDs = map[string]bool{"1": true}
	currentPreflight.mutex.Unlock()
	t.Cleanup(finishPreflight)

	inventory, _ := fetchPacket(t, "1")
	if inventory.State != storage.FormInventoryPreparing {
		t.Fatalf("state = %q, want preparing", inventory.State)
	}
	if inventory.Reason != "" {
		t.Fatalf("a run in flight carries no failure reason, got %q", inventory.Reason)
	}
}

// Completing a run flips the packet to ready without any further operator
// action, which is what makes the one-action repair actually finish.
func TestServeApplicationPacket_BecomesReadyWhenPreparationCompletes(t *testing.T) {
	setupTestDB(t)
	usePrivatePII(t, "first_name: Ada\n")
	seedUnpreparedJob(t, 1, "converges")

	before, _ := fetchPacket(t, "1")
	if before.State != storage.FormInventoryNotPrepared {
		t.Fatalf("state before = %q", before.State)
	}

	// Exactly what a successful run writes.
	if err := storage.ReplaceApplicationQuestions(db, "1", []storage.ApplicationQuestion{
		{JobID: "1", Key: "notice", Prompt: "What is your notice period?", ControlType: "text"},
	}, storage.AssistedFillSummary{JobID: "1"}); err != nil {
		t.Fatal(err)
	}
	if err := storage.RecordPreflight(db, storage.PreflightResult{
		JobID: "1", State: storage.PreflightInspected, ATS: "Lever", ControlCount: 11,
	}, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}

	after, body := fetchPacket(t, "1")
	if after.State != storage.FormInventoryReady {
		t.Fatalf("state after = %q, want ready", after.State)
	}
	if after.QuestionCount != 1 {
		t.Fatalf("question count = %d, want 1", after.QuestionCount)
	}
	if !strings.Contains(body, "What is your notice period?") {
		t.Fatal("the real question must appear once preparation completes")
	}
}

// An already-submitted application is truthful about having nothing to prepare,
// and is never offered the action.
func TestServeApplicationPacket_CompletedApplicationIsNotOfferedPreparation(t *testing.T) {
	setupTestDB(t)
	usePrivatePII(t, "first_name: Ada\n")
	seedUnpreparedJob(t, 1, "applied")
	if _, err := db.Exec(`UPDATE assisted_applications SET assisted_state = 'completed' WHERE job_id = 1`); err != nil {
		t.Fatal(err)
	}

	inventory, _ := fetchPacket(t, "1")
	if inventory.Preparable {
		t.Fatal("a completed application must not be offered for preparation")
	}
	if inventory.Reason != "already_applied" {
		t.Fatalf("reason = %q, want already_applied", inventory.Reason)
	}
}

// A second Prepare while one is running is refused rather than started, so a
// double click cannot produce two inspections of the same form.
func TestStartPreflight_RefusesASecondConcurrentRun(t *testing.T) {
	currentPreflight.mutex.Lock()
	currentPreflight.running = true
	currentPreflight.jobIDs = map[string]bool{"1": true}
	currentPreflight.mutex.Unlock()
	t.Cleanup(finishPreflight)

	if err := startPreflight([]string{"1"}); err != errPreflightBusy {
		t.Fatalf("err = %v, want errPreflightBusy", err)
	}
}

// The packet's Prepare action posts to the endpoint the Prepare panel already
// uses, so a job asked for from the packet is genuinely in the run.
func TestServePreflight_PacketSideRequestEntersTheSameRun(t *testing.T) {
	setupTestDB(t)
	usePrivatePII(t, "first_name: Ada\n")
	seedUnpreparedJob(t, 7, "single")

	original := startPreflight
	var requested []string
	startPreflight = func(jobIDs []string) error {
		requested = jobIDs
		currentPreflight.mutex.Lock()
		currentPreflight.running = true
		currentPreflight.jobIDs = map[string]bool{}
		for _, id := range jobIDs {
			currentPreflight.jobIDs[id] = true
		}
		currentPreflight.mutex.Unlock()
		return nil
	}
	t.Cleanup(func() { startPreflight = original; finishPreflight() })

	recorder := postJSON(t, servePreflight, "/api/knowledge/preflight", map[string]any{"job_ids": []string{"7"}})
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body %q", recorder.Code, recorder.Body.String())
	}
	if len(requested) != 1 || requested[0] != "7" {
		t.Fatalf("the run was asked for %v, want just the packet's own job", requested)
	}
	inventory, _ := fetchPacket(t, "7")
	if inventory.State != storage.FormInventoryPreparing {
		t.Fatalf("state = %q, want preparing right after the request", inventory.State)
	}
}

// The safety boundary this feature must not widen. The packet's Prepare action
// reaches cmd/preflight and nothing else, and cmd/preflight has no fill or
// submit call in it.
//
// Asserted by reading the source because a behavioural test cannot prove a
// negative over every employer form -- the same argument improvements.md #541
// made when the no-submit boundary was first drawn.
func TestPreflightSourceHasNoFillOrSubmitPath(t *testing.T) {
	source, err := os.ReadFile("../preflight/main.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(source)
	for _, forbidden := range []string{
		"AttemptSubmit",
		"FillForm",
		"fillField",
		"SetInputFiles",
		".Fill(",
		".Click(",
		".Press(",
		"TakePendingAnswers",
	} {
		if strings.Contains(text, forbidden) {
			t.Errorf("cmd/preflight must not be able to reach %q: preparation reads a form and nothing else", forbidden)
		}
	}
	// And it must still be doing the reading, or this test would pass on an
	// empty file.
	if !strings.Contains(text, "InspectApplication") {
		t.Fatal("cmd/preflight no longer inspects anything; this assertion is measuring the wrong file")
	}
}

// The packet handler itself must not have grown a write path. It answers a GET
// and is the entry point the new action sits next to.
func TestServeApplicationPacket_RemainsReadOnly(t *testing.T) {
	setupTestDB(t)
	usePrivatePII(t, "first_name: Ada\n")
	seedUnpreparedJob(t, 1, "readonly")

	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch} {
		recorder := httptest.NewRecorder()
		serveApplicationPacket(recorder,
			httptest.NewRequest(method, "/api/assisted/packet?job_id=1", nil))
		if recorder.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s returned %d, want 405", method, recorder.Code)
		}
	}
}
