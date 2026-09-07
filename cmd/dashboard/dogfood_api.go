package main

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/howlcipher/Career_Agent_Core/pkg/storage"
)

// serveDogfoodStart opens a new five-application evidence run. It refuses
// with a conflict when one is already active, the same way every other
// single-active-thing endpoint in this dashboard does.
func serveDogfoodStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	cohort, err := storage.StartDogfoodCohort(db)
	if err != nil {
		log.Printf("serveDogfoodStart: %v", err)
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"cohort": cohort})
}

// serveDogfoodActive reports the open cohort, or null when none is active.
func serveDogfoodActive(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	cohort, err := storage.GetActiveDogfoodCohort(db)
	if err != nil {
		log.Printf("serveDogfoodActive: %v", err)
		http.Error(w, "could not load the active dogfood cohort", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"cohort": cohort})
}

// serveDogfoodCohorts lists every cohort, most recent first, for the
// read-only history view.
func serveDogfoodCohorts(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	cohorts, err := storage.ListDogfoodCohorts(db)
	if err != nil {
		log.Printf("serveDogfoodCohorts: %v", err)
		http.Error(w, "could not load dogfood cohorts", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"cohorts": cohorts})
}

// serveDogfoodFeedback records the one thing this feature cannot derive from
// existing storage: what the operator says slowed them down on one cohort
// application, in a fixed closed vocabulary. It is always optional and is
// never required for a cohort to progress or complete.
func serveDogfoodFeedback(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var request struct {
		JobID       string `json:"job_id"`
		Category    string `json:"category"`
		ManualCount *int   `json:"manual_count"`
		Note        string `json:"note"`
	}
	if err := decodeBoundedJSON(w, r, &request); err != nil {
		return
	}
	if strings.TrimSpace(request.JobID) == "" {
		http.Error(w, "a job identifier is required", http.StatusBadRequest)
		return
	}
	// note is a short, operator-authored aside about their own experience --
	// never employer content or an approved answer. Bounded generously but
	// firmly so a pasted job posting cannot end up here.
	if len(request.Note) > 500 {
		http.Error(w, "note is too long", http.StatusBadRequest)
		return
	}
	if err := storage.RecordDogfoodFeedback(db, request.JobID, request.Category, request.ManualCount, request.Note); err != nil {
		log.Printf("serveDogfoodFeedback: %v", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"status":"recorded"}`))
}

// serveDogfoodReport returns the sanitized report for one cohort, computed
// fresh from durable storage every time -- nothing about it is cached or
// precomputed. A ?cohort_id= query parameter selects a specific past cohort
// for the read-only history view; omitting it returns the most recent one,
// which is what the dashboard polls right after a cohort completes.
func serveDogfoodReport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	cohortID, err := dogfoodReportCohortID(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if cohortID == 0 {
		cohorts, err := storage.ListDogfoodCohorts(db)
		if err != nil {
			log.Printf("serveDogfoodReport: %v", err)
			http.Error(w, "could not load dogfood cohorts", http.StatusInternalServerError)
			return
		}
		if len(cohorts) == 0 {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{"report": nil})
			return
		}
		cohortID = cohorts[0].ID
	}
	report, err := storage.GetDogfoodReport(db, cohortID)
	if err != nil {
		log.Printf("serveDogfoodReport: %v", err)
		http.Error(w, "could not load the dogfood report", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"report": report})
}

func dogfoodReportCohortID(r *http.Request) (int64, error) {
	raw := strings.TrimSpace(r.URL.Query().Get("cohort_id"))
	if raw == "" {
		return 0, nil
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		return 0, errors.New("invalid cohort id")
	}
	return id, nil
}
