package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/howlcipher/Career_Agent_Core/pkg/config"
	"github.com/howlcipher/Career_Agent_Core/pkg/security"
	"github.com/howlcipher/Career_Agent_Core/pkg/storage"
	"io"
	"log"
	"net/http"
	"os/exec"
	"time"
)

const operatorSettingsPath = "applications/operator_settings.yaml"

func decodeBoundedJSON(w http.ResponseWriter, r *http.Request, req interface{}) error {
	origin := r.Header.Get("Origin")
	if origin != "" && origin != "http://localhost:8080" && origin != "http://127.0.0.1:8080" {
		http.Error(w, "Forbidden origin", http.StatusForbidden)
		return fmt.Errorf("forbidden origin")
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1024)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return err
	}
	var dummy interface{}
	if err := dec.Decode(&dummy); err != io.EOF {
		http.Error(w, "Invalid request body: trailing data", http.StatusBadRequest)
		return fmt.Errorf("trailing data")
	}
	return nil
}

func serveOperatorSettings(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		effective, err := config.GetEffectiveSettings("profile.yaml", operatorSettingsPath)
		if err != nil {
			log.Printf("Failed to resolve effective settings: %v", err)
			http.Error(w, "Failed to resolve effective settings", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(effective)
		return
	}

	if r.Method == http.MethodPost {
		var req config.OperatorSettings
		if err := decodeBoundedJSON(w, r, &req); err != nil {
			return
		}
		if err := req.Validate(); err != nil {
			http.Error(w, fmt.Sprintf("invalid settings: %v", err), http.StatusBadRequest)
			return
		}

		if err := config.SaveOperatorSettings(operatorSettingsPath, &req); err != nil {
			log.Printf("Failed to save operator settings: %v", err)
			http.Error(w, "Failed to save operator settings", http.StatusInternalServerError)
			return
		}

		effective, err := config.GetEffectiveSettings("profile.yaml", operatorSettingsPath)
		if err != nil {
			log.Printf("Failed to resolve effective settings after save: %v", err)
			http.Error(w, "Failed to resolve effective settings", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(effective)
		return
	}

	http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
}

type QualifiedJob struct {
	ID           int64  `json:"id"`
	Company      string `json:"company"`
	Title        string `json:"title"`
	FitScore     int    `json:"fit_score"`
	Provider     string `json:"provider"`
	DiscoveredAt string `json:"discovered_at"`
	LastUpdated  string `json:"last_updated"`
	Location     string `json:"location"`
	Remote       bool   `json:"remote"`
	Reason       string `json:"reason"`
}

func serveQualifiedJobs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	rows, err := db.Query(`
		SELECT id, company_name, job_title, fit_score, discovery_source,
			discovered_at, last_updated, job_location, is_remote, status_reason
		FROM job_funnel
		WHERE status = 'PROCESSED_MANUAL' AND status_reason = 'find_only_threshold_met'
		ORDER BY fit_score DESC, discovered_at DESC
	`)
	if err != nil {
		log.Printf("serveQualifiedJobs query error: %v", err)
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var jobs []QualifiedJob
	for rows.Next() {
		var j QualifiedJob
		var discoveredAt, updatedAt sql.NullTime
		var location, reason, source sql.NullString
		var fitScore sql.NullInt64
		var remote sql.NullBool

		if err := rows.Scan(&j.ID, &j.Company, &j.Title, &fitScore, &source,
			&discoveredAt, &updatedAt, &location, &remote, &reason); err != nil {
			log.Printf("serveQualifiedJobs scan error: %v", err)
			continue
		}

		if fitScore.Valid {
			j.FitScore = int(fitScore.Int64)
		}
		if source.Valid {
			j.Provider = source.String
		}
		if discoveredAt.Valid {
			j.DiscoveredAt = discoveredAt.Time.Format(time.RFC3339)
		}
		if updatedAt.Valid {
			j.LastUpdated = updatedAt.Time.Format(time.RFC3339)
		}
		if location.Valid {
			j.Location = location.String
		}
		if remote.Valid {
			j.Remote = remote.Bool
		}
		if reason.Valid {
			j.Reason = reason.String
		}

		jobs = append(jobs, j)
	}

	if err := rows.Err(); err != nil {
		log.Printf("serveQualifiedJobs iteration error: %v", err)
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}

	if jobs == nil {
		jobs = []QualifiedJob{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(jobs)
}

func serveQualifiedJobsOpen(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		JobID int64 `json:"job_id"`
	}
	if err := decodeBoundedJSON(w, r, &req); err != nil {
		return
	}

	var u string
	if err := db.QueryRow("SELECT url FROM job_funnel WHERE id = ? AND status = 'PROCESSED_MANUAL'", req.JobID).Scan(&u); err != nil {
		http.Error(w, "Job not found", http.StatusNotFound)
		return
	}

	guard := security.NewNetworkGuard()
	if err := guard.ValidateURL(r.Context(), u); err != nil {
		http.Error(w, "Unsafe URL", http.StatusBadRequest)
		return
	}

	cmd := exec.Command("xdg-open", u)
	if err := cmd.Start(); err != nil {
		http.Error(w, "Failed to launch browser", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func serveQualifiedJobsPromote(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		JobID int64 `json:"job_id"`
	}
	if err := decodeBoundedJSON(w, r, &req); err != nil {
		return
	}

	eff, err := config.GetEffectiveSettings("profile.yaml", operatorSettingsPath)
	if err != nil {
		http.Error(w, "Internal configuration error", http.StatusInternalServerError)
		return
	}

	if err := storage.PromoteJobToAssisted(db, req.JobID, eff.MinimumFitScore); err != nil {
		http.Error(w, fmt.Sprintf("Failed to promote job: %v", err), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func serveQualifiedJobsSkip(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		JobID int64 `json:"job_id"`
	}
	if err := decodeBoundedJSON(w, r, &req); err != nil {
		return
	}

	if err := storage.SkipQualifiedJob(db, req.JobID); err != nil {
		http.Error(w, fmt.Sprintf("Failed to skip job: %v", err), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func serveQualifiedJobsConfirm(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		JobID             int64 `json:"job_id"`
		ConfirmedReceived bool  `json:"confirmed_received"`
	}
	if err := decodeBoundedJSON(w, r, &req); err != nil {
		return
	}

	if !req.ConfirmedReceived {
		http.Error(w, "Missing manual confirmation acknowledgement", http.StatusBadRequest)
		return
	}

	if err := storage.MarkJobAppliedManually(db, req.JobID); err != nil {
		http.Error(w, fmt.Sprintf("Failed to confirm job: %v", err), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}
