package main

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/howlcipher/Career_Agent_Core/pkg/answers"
	"github.com/howlcipher/Career_Agent_Core/pkg/config"
	"github.com/howlcipher/Career_Agent_Core/pkg/knowledge"
	"github.com/howlcipher/Career_Agent_Core/pkg/storage"
)

// maxKnowledgeRequestBytes bounds an approval. A screening question can
// legitimately want a few paragraphs, so this matches the assisted answers
// endpoint rather than the 1 KiB default.
const maxKnowledgeRequestBytes = 64 * 1024

// maxProfileRequestBytes bounds a profile save. The whole of pii.yaml is small;
// this is generous for it and still a hard cap.
const maxProfileRequestBytes = 32 * 1024

// piiPath is where the operator's configured details live. It is a variable
// only so the tests can point it at a temporary file; nothing changes it at
// runtime, and every other caller in this repository uses the same literal.
var piiPath = "pii.yaml"

// openKnowledgeService builds the service for one request.
//
// pii.yaml is read per request rather than cached, deliberately: the operator
// can edit it through this same API, and a cached copy would mean the readiness
// summary went on reporting a question as unresolved after they had just
// supplied the fact that resolves it.
func openKnowledgeService() (*knowledge.Service, error) {
	pii, err := config.LoadPII(piiPath)
	if err != nil {
		// A missing pii.yaml is not fatal here. Without it the vault's pattern
		// table resolves nothing, so every question is simply unresolved --
		// which is a worse summary but an honest one, and better than a section
		// that refuses to render.
		log.Printf("Application knowledge: configured details unavailable: %v", err)
		pii = &config.PII{}
	}
	return knowledge.Open(db, pii)
}

// serveKnowledge returns the whole Application Knowledge view: the readiness
// summary, the deduplicated inbox, and the preflight verdicts.
//
// One endpoint rather than three because the three are views of one queue and
// must agree. Fetched separately they would be three snapshots taken at three
// moments, and the operator would eventually read "8 questions need your input"
// above a list of seven.
//
// It is deliberately not folded into /api/assisted, which the UI polls at 2 Hz
// and which already does per-row summary, question and document lookups.
func serveKnowledge(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	service, err := openKnowledgeService()
	if err != nil {
		log.Printf("serveKnowledge: %v", err)
		http.Error(w, "application knowledge is unavailable", http.StatusInternalServerError)
		return
	}
	snapshot, err := service.Snapshot(time.Now().UTC())
	if err != nil {
		log.Printf("serveKnowledge: %v", err)
		http.Error(w, "could not read what your applications need", http.StatusInternalServerError)
		return
	}
	writeJSON(w, snapshot)
}

// openKnowledgeServiceOrRespond loads the knowledge service; on failure it
// writes a 500 and returns false.
func openKnowledgeServiceOrRespond(w http.ResponseWriter, label string) (*knowledge.Service, bool) {
	service, err := openKnowledgeService()
	if err != nil {
		log.Printf("%s: %v", label, err)
		http.Error(w, "application knowledge is unavailable", http.StatusInternalServerError)
		return nil, false
	}
	return service, true
}

// knowledgeApproveRequest is the POST body for approving a value answer.
type knowledgeApproveRequest struct {
	GroupKey            string `json:"group_key"`
	Answer              string `json:"answer"`
	SaveForReuse        bool   `json:"save_for_reuse"`
	AllowSensitiveReuse bool   `json:"allow_sensitive_reuse"`
	ConfirmedEquivalent bool   `json:"confirmed_equivalent"`
	Scope               string `json:"scope"`
}

// knowledgeAbsenceRequest is the POST body for declaring an intentional absence.
type knowledgeAbsenceRequest struct {
	GroupKey            string `json:"group_key"`
	Reason              string `json:"reason"`
	ConfirmedEquivalent bool   `json:"confirmed_equivalent"`
	Scope               string `json:"scope"`
}

// decodeKnowledgeRequest decodes a bounded JSON body into dst, writing a 400 on failure.
func decodeKnowledgeRequest(w http.ResponseWriter, r *http.Request, dst interface{}) bool {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxKnowledgeRequestBytes))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		http.Error(w, "request could not be decoded", http.StatusBadRequest)
		return false
	}
	return true
}

// serveKnowledgeApprove stores one answer for every application waiting on it.
func serveKnowledgeApprove(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var request knowledgeApproveRequest
	if !decodeKnowledgeRequest(w, r, &request) {
		return
	}
	if strings.TrimSpace(request.GroupKey) == "" {
		http.Error(w, "a question and an answer are required", http.StatusBadRequest)
		return
	}
	service, ok := openKnowledgeServiceOrRespond(w, "serveKnowledgeApprove")
	if !ok {
		return
	}
	result, err := service.Approve(knowledge.ApproveRequest{
		GroupKey:            request.GroupKey,
		Answer:              request.Answer,
		SaveForReuse:        request.SaveForReuse,
		AllowSensitiveReuse: request.AllowSensitiveReuse,
		ConfirmedEquivalent: request.ConfirmedEquivalent,
		Scope:               normalizeRequestedScope(request.Scope),
	}, time.Now().UTC())
	if err != nil {
		// The vault's refusals are the operator's answer, not a server fault,
		// and each one has a specific thing they can do about it.
		switch {
		case errors.Is(err, answers.ErrSensitiveNeedsApproval):
			http.Error(w, "this is a declaration. Confirm separately that Career Agent may reuse it automatically before it can be remembered.", http.StatusConflict)
		case errors.Is(err, answers.ErrNotReusable):
			http.Error(w, "this answer is written for one employer and is never reused, so it cannot be saved here. Answer it on each application.", http.StatusConflict)
		default:
			http.Error(w, err.Error(), http.StatusConflict)
		}
		return
	}
	writeJSON(w, result)
}

// serveKnowledgeAbsence stores an intentional-absence decision for a group.
func serveKnowledgeAbsence(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var request knowledgeAbsenceRequest
	if !decodeKnowledgeRequest(w, r, &request) {
		return
	}
	if strings.TrimSpace(request.GroupKey) == "" {
		http.Error(w, "a question group and a reason are required", http.StatusBadRequest)
		return
	}
	service, ok := openKnowledgeServiceOrRespond(w, "serveKnowledgeAbsence")
	if !ok {
		return
	}
	result, err := service.ApproveAbsence(knowledge.ApproveAbsenceRequest{
		GroupKey:            request.GroupKey,
		Reason:              request.Reason,
		SaveForReuse:        true,
		ConfirmedEquivalent: request.ConfirmedEquivalent,
		Scope:               normalizeRequestedScope(request.Scope),
	}, time.Now().UTC())
	if err != nil {
		switch {
		case errors.Is(err, answers.ErrNotReusable):
			http.Error(w, "this answer is written for one employer and cannot be marked absent", http.StatusConflict)
		default:
			http.Error(w, err.Error(), http.StatusConflict)
		}
		return
	}
	writeJSON(w, result)
}

// serveKnowledgeField answers a single-field query.
//
// This is the endpoint ADR-005's normal-browser companion needs, so that
// companion never implements an answer system of its own. It is a GET with the
// question in the query string because the caller has structure, not state:
// what the field is called, what kind of control it is, and who is asking.
func serveKnowledgeField(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	prompt := strings.TrimSpace(r.URL.Query().Get("prompt"))
	if prompt == "" {
		http.Error(w, "a field query needs the question's text", http.StatusBadRequest)
		return
	}
	if len(prompt) > 2000 {
		http.Error(w, "that question is too long to be a field label", http.StatusBadRequest)
		return
	}
	service, err := openKnowledgeService()
	if err != nil {
		log.Printf("serveKnowledgeField: %v", err)
		http.Error(w, "application knowledge is unavailable", http.StatusInternalServerError)
		return
	}
	var options []string
	if raw := r.URL.Query().Get("options"); raw != "" {
		for _, option := range strings.Split(raw, "\n") {
			if option = strings.TrimSpace(option); option != "" {
				options = append(options, option)
			}
		}
	}
	reply, err := service.Field(knowledge.FieldQuery{
		Prompt:      prompt,
		ControlType: r.URL.Query().Get("control_type"),
		Options:     options,
		Company:     r.URL.Query().Get("company"),
		ATS:         r.URL.Query().Get("ats"),
	})
	if err != nil {
		http.Error(w, "could not answer that field query", http.StatusInternalServerError)
		return
	}
	writeJSON(w, reply)
}

// preflightRun tracks the one preflight run this process allows at a time.
//
// It records *which* applications are in flight, not just how many. A count is
// enough for the Prepare panel, which describes the run as a whole, but the
// Copy Application Packet describes one application and has to answer a
// narrower question: is this form being read right now? Without the
// identifiers, a packet opened during a run of some other application would
// have to choose between claiming "preparing" (wrong for this job) and
// "not prepared" (about to become wrong for the other one).
type preflightRun struct {
	mutex   sync.Mutex
	running bool
	started time.Time
	jobs    int
	jobIDs  map[string]bool
}

var currentPreflight preflightRun

// preparingJob reports whether an inspection of this application is in flight
// right now.
//
// This is process state, not durable state: it is true only inside the
// dashboard that spawned the run. A dashboard restarted mid-run reverts to
// reporting whatever the database holds, which is the honest answer -- it no
// longer knows the run exists, and claiming otherwise would be a guess.
// It reports the run's start time alongside, because membership alone is not
// enough: a batch holds every requested application for the whole run, so a job
// the run has already finished would keep claiming to be in progress until the
// last one completed. The caller compares against its recorded verdict.
func (p *preflightRun) preparingJob(jobID string) (bool, time.Time) {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	return p.running && p.jobIDs[jobID], p.started
}

// servePreflight starts a run, or reports the current one.
func servePreflight(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		currentPreflight.mutex.Lock()
		running, started, jobs := currentPreflight.running, currentPreflight.started, currentPreflight.jobs
		currentPreflight.mutex.Unlock()
		results, err := storage.PreflightResults(db, nil)
		if err != nil {
			log.Printf("servePreflight: %v", err)
			results = []storage.PreflightResult{}
		}
		payload := map[string]any{"running": running, "applications": jobs, "results": results}
		if running {
			payload["started_at"] = started.Format(time.RFC3339)
		}
		writeJSON(w, payload)

	case http.MethodPost:
		var request struct {
			JobIDs []string `json:"job_ids"`
		}
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8*1024))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&request); err != nil || len(request.JobIDs) == 0 {
			http.Error(w, "select at least one application to prepare", http.StatusBadRequest)
			return
		}
		// The operator has an application open. A second Chromium plus local
		// inference is how this machine falls over, and they are mid-application
		// anyway.
		if live, err := storage.AssistedBrowserIsLive(db); err == nil && live {
			http.Error(w, "finish the open application first; preparing opens its own browser", http.StatusConflict)
			return
		}
		if err := startPreflight(request.JobIDs); err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		writeJSON(w, map[string]any{"status": "preparing", "applications": len(request.JobIDs)})

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// startPreflight spawns the run and returns as soon as it has started, so the
// dashboard stays responsive while a batch of page loads proceeds. Progress is
// read from the database, which is the same channel every other assisted
// transition already uses.
var startPreflight = func(jobIDs []string) error {
	currentPreflight.mutex.Lock()
	if currentPreflight.running {
		currentPreflight.mutex.Unlock()
		return errPreflightBusy
	}
	currentPreflight.running = true
	currentPreflight.started = time.Now().UTC()
	currentPreflight.jobs = len(jobIDs)
	currentPreflight.jobIDs = make(map[string]bool, len(jobIDs))
	for _, id := range jobIDs {
		currentPreflight.jobIDs[id] = true
	}
	currentPreflight.mutex.Unlock()

	cmd, err := preflightCommand(jobIDs)
	if err != nil {
		finishPreflight()
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		finishPreflight()
		return err
	}
	if err := cmd.Start(); err != nil {
		finishPreflight()
		return err
	}
	go func() {
		defer finishPreflight()
		// The same filter cmd/assist's stderr goes through, for the same reason:
		// what a child writes is decided by its dependencies, and Playwright's
		// diagnostics quote page content (ADR-006).
		readAssistedStderr(preflightLogLabel, stderr, func() {})
		if err := cmd.Wait(); err != nil {
			log.Printf("Preparation run ended with an error: %v", err)
		}
	}()
	return nil
}

func finishPreflight() {
	currentPreflight.mutex.Lock()
	currentPreflight.running = false
	currentPreflight.jobs = 0
	currentPreflight.jobIDs = nil
	currentPreflight.mutex.Unlock()
}

// preflightCommand resolves the preflight binary, mirroring
// assistedApplicationCommand's resolution order so an operator who has one
// working layout has both.
func preflightCommand(jobIDs []string) (*exec.Cmd, error) {
	args := []string{"-jobs", strings.Join(jobIDs, ",")}
	if envBin := os.Getenv("CAREER_PREFLIGHT_BIN"); envBin != "" {
		if info, err := os.Stat(envBin); err == nil && !info.IsDir() && info.Mode()&0111 != 0 {
			return exec.Command(envBin, args...), nil
		}
	}
	if executable, err := os.Executable(); err == nil {
		directory := filepath.Dir(executable)
		for _, name := range []string{"career_preflight_bin", "preflight"} {
			candidate := filepath.Join(directory, name)
			if info, statErr := os.Stat(candidate); statErr == nil && !info.IsDir() && info.Mode()&0111 != 0 {
				return exec.Command(candidate, args...), nil
			}
		}
	}
	if root := findGoModuleRoot(); root != "" {
		candidate := filepath.Join(root, "career_preflight_bin")
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() && info.Mode()&0111 != 0 {
			return exec.Command(candidate, args...), nil
		}
		cmd := exec.Command("go", append([]string{"run", "./cmd/preflight"}, args...)...)
		cmd.Dir = root
		return cmd, nil
	}
	return nil, errors.New("cannot locate the preparation command: start the dashboard from the repository checkout or place career_preflight_bin beside the dashboard binary")
}

func writeJSON(w http.ResponseWriter, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		log.Printf("write response: %v", err)
	}
}

// errPreflightBusy is returned when a preparation run is already in progress.
// One at a time is a resource decision, not a correctness one: each run drives
// a browser and a batch of page loads.
var errPreflightBusy = errors.New("a preparation run is already in progress")
