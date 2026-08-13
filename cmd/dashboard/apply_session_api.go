package main

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/howlcipher/Career_Agent_Core/pkg/storage"
)

// maxSessionJobs bounds one session. Beyond this the operator is not running a
// focused apply session, and the auto-advance loop would be opening browsers
// for hours.
const maxSessionJobs = 50

// serveApplySession returns the open session, or null when none is open.
func serveApplySession(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	session, err := storage.GetApplySession(db)
	if err != nil {
		log.Printf("serveApplySession: %v", err)
		http.Error(w, "could not read the apply session", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"session": session})
}

// serveApplySessionStart opens a session over the selected applications and
// immediately opens the first one.
func serveApplySessionStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var request struct {
		JobIDs []string `json:"job_ids"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8*1024))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		http.Error(w, "a list of assisted job identifiers is required", http.StatusBadRequest)
		return
	}
	if len(request.JobIDs) == 0 || len(request.JobIDs) > maxSessionJobs {
		http.Error(w, "select between 1 and 50 applications for a session", http.StatusBadRequest)
		return
	}
	for _, jobID := range request.JobIDs {
		if _, err := strconv.ParseInt(strings.TrimSpace(jobID), 10, 64); err != nil {
			http.Error(w, "invalid assisted job identifier", http.StatusBadRequest)
			return
		}
	}
	session, err := storage.StartApplySession(db, request.JobIDs, true)
	if err != nil {
		log.Printf("serveApplySessionStart: %v", err)
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	go advanceApplySession()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"session": session})
}

// serveApplySessionControl handles pause, resume, stop-after-current, stop and
// skip. One handler because they are one decision surface, and splitting them
// would mean five near-identical bodies.
func serveApplySessionControl(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var request struct {
		Action string `json:"action"`
		JobID  string `json:"job_id"`
	}
	if err := decodeBoundedJSON(w, r, &request); err != nil {
		return
	}
	var err error
	resume := false
	switch request.Action {
	case "pause":
		err = storage.SetApplySessionState(db, storage.SessionPaused)
	case "resume":
		err = storage.SetApplySessionState(db, storage.SessionRunning)
		resume = true
	case "stop_after_current":
		err = storage.SetApplySessionStopAfterCurrent(db)
	case "stop":
		err = storage.StopApplySession(db)
	case "skip":
		if _, parseErr := strconv.ParseInt(strings.TrimSpace(request.JobID), 10, 64); parseErr != nil {
			http.Error(w, "invalid assisted job identifier", http.StatusBadRequest)
			return
		}
		// Skipping is an explicit operator decision about this application, so
		// it is a terminal outcome and the session may advance past it. That is
		// the difference between this and a browser that merely closed.
		err = storage.AdvanceApplySession(db, request.JobID, storage.ItemSkipped, "operator skipped this application")
		resume = true
	default:
		http.Error(w, "unsupported apply session action", http.StatusBadRequest)
		return
	}
	if err != nil {
		log.Printf("serveApplySessionControl(%s): %v", request.Action, err)
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	if resume {
		go advanceApplySession()
	}
	session, err := storage.GetApplySession(db)
	if err != nil {
		log.Printf("serveApplySessionControl: %v", err)
		http.Error(w, "could not read the apply session", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"session": session})
}

// applySessionAdvance serializes auto-advance. Two triggers can fire at once —
// a confirmation and the background tick — and both would otherwise try to open
// a browser for the same next application.
var applySessionAdvance sync.Mutex

// advanceApplySession opens the session's next application, if there is one and
// it is safe to open.
//
// It never decides that an application is finished; storage.NextApplySessionJob
// only offers a job when no item is in progress, the session is running, and
// stop-after-current is not set. And the launch itself still passes through
// GetAssistedLaunchInfo, which refuses an ineligible, unrevalidated, or
// ATS-rejecting row exactly as it does for a manual click — so auto-advance can
// never open something the operator could not have opened themselves.
func advanceApplySession() {
	applySessionAdvance.Lock()
	defer applySessionAdvance.Unlock()

	jobID, ok, err := storage.NextApplySessionJob(db)
	if err != nil {
		log.Printf("advanceApplySession: %v", err)
		return
	}
	if !ok {
		return
	}
	if _, err := storage.GetAssistedLaunchInfo(db, jobID); err != nil {
		// This application cannot be opened right now. It is recorded as
		// deferred with the reason, and the session moves on rather than
		// stalling on it or pretending it was completed.
		reason := "could not be opened"
		if errors.Is(err, storage.ErrAssistedBrowserRejected) {
			reason = "this ATS rejects the assisted browser; finish it in your own"
		}
		if deferErr := storage.AdvanceApplySession(db, jobID, storage.ItemBlocked, reason); deferErr != nil {
			log.Printf("advanceApplySession: could not defer an unopenable application: %v", deferErr)
			return
		}
		log.Printf("advanceApplySession: deferred job %s (%s)", jobID, reason)
		go advanceApplySession()
		return
	}
	if err := storage.MarkApplySessionItemOpen(db, jobID); err != nil {
		log.Printf("advanceApplySession: %v", err)
		return
	}
	if err := launchAssistedApplication(jobID); err != nil {
		log.Printf("advanceApplySession: could not open the next application: %v", err)
		// The browser did not open, so nothing about this application is
		// known. It goes back to pending and the session pauses, the same as
		// any other outcome-less end.
		//
		// NextApplySessionJob has already refused to offer a job while another
		// assisted browser is live, so this path no longer fires for the
		// transient lease conflict that used to pause a session every time an
		// application was skipped with its browser still open (bugs.md #542).
		if pauseErr := storage.PauseApplySessionForClosedBrowser(db, jobID); pauseErr != nil {
			log.Printf("advanceApplySession: %v", pauseErr)
		}
	}
}

// applySessionTicker is the safety net behind auto-advance.
//
// The confirm, skip and resume paths all advance the session directly, so this
// exists for the cases they cannot cover: an assisted process that exits
// without a clean handoff, a lease that expires, or a dashboard that was
// restarted mid-session. It is deliberately slow — this is a backstop, not the
// mechanism.
func startApplySessionTicker(interval time.Duration, stop <-chan struct{}) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				advanceApplySession()
			}
		}
	}()
}
