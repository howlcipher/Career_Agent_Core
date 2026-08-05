package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"time"

	"github.com/howlcipher/Career_Agent_Core/pkg/config"
	"github.com/howlcipher/Career_Agent_Core/pkg/storage"
)

const operatorSettingsPath = "applications/operator_settings.yaml"

func serveOperatorSettings(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		settings, err := config.LoadOperatorSettings(operatorSettingsPath)
		if err != nil {
			log.Printf("Failed to load operator settings: %v", err)
			http.Error(w, "Failed to load operator settings", http.StatusInternalServerError)
			return
		}

		prof, _ := config.LoadProfile("profile.yaml")

		if settings == nil {
			settings = &config.OperatorSettings{
				MinimumFitScore: 50,
			}
			if prof != nil {
				if prof.AutoSubmit && !prof.AutoSubmitClick && prof.CopilotMode {
					settings.ApplicationMode = config.ApplicationModeAssisted
				} else if prof.AutoSubmit && prof.AutoSubmitClick && !prof.CopilotMode {
					settings.ApplicationMode = config.ApplicationModeAutomatic
				} else {
					settings.ApplicationMode = config.ApplicationModeFindOnly
				}
			} else {
				settings.ApplicationMode = config.ApplicationModeFindOnly
			}
		}

		response := map[string]interface{}{
			"application_mode":  settings.ApplicationMode,
			"minimum_fit_score": settings.MinimumFitScore,
		}

		if prof != nil {
			response["scoring_active"] = !prof.SkipScoring
		} else {
			response["scoring_active"] = true
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
		return
	}

	if r.Method == http.MethodPost {
		var req config.OperatorSettings
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&req); err != nil {
			http.Error(w, fmt.Sprintf("invalid request: %v", err), http.StatusBadRequest)
			return
		}
		if err := req.Validate(); err != nil {
			http.Error(w, fmt.Sprintf("invalid settings: %v", err), http.StatusBadRequest)
			return
		}

		pid, running, _ := agentPID()
		if running && pid > 0 {
			if proc, findErr := os.FindProcess(pid); findErr == nil {
				if sigErr := proc.Signal(os.Interrupt); sigErr != nil {
					log.Printf("serveOperatorSettings: failed to signal pid %d: %v", pid, sigErr)
				}
			}
			// Wait for lock to be released
			for i := 0; i < 50; i++ {
				time.Sleep(100 * time.Millisecond)
				_, stillRunning, _ := agentPID()
				if !stillRunning {
					break
				}
			}
			_, stillRunning, _ := agentPID()
			if stillRunning {
				http.Error(w, "failed to stop running agent", http.StatusInternalServerError)
				return
			}
		}

		if err := config.SaveOperatorSettings(operatorSettingsPath, &req); err != nil {
			log.Printf("Failed to save operator settings: %v", err)
			http.Error(w, "Failed to save operator settings", http.StatusInternalServerError)
			return
		}

		if running {
			// Restart agent
			cmd := exec.Command(
				"./career_agent_bin",
				"-daemon",
				"-cycle-limit", "15",
				"-cycle-interval", "1m",
			)
			if err := cmd.Start(); err != nil {
				log.Printf("Failed to restart agent: %v", err)
				http.Error(w, "Settings saved, but failed to restart agent", http.StatusInternalServerError)
				return
			}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(req)
		return
	}

	http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
}

type QualifiedJob struct {
	ID           int64  `json:"id"`
	Company      string `json:"company"`
	Title        string `json:"title"`
	URL          string `json:"url"`
	FitScore     int    `json:"fit_score"`
	Provider     string `json:"provider"`
	DiscoveredAt string `json:"discovered_at"`
	LastUpdated  string `json:"last_updated"`
	SalaryDesc   string `json:"salary_desc"`
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
		SELECT rowid, company_name, title, url, fit_score, discovery_source,
			discovered_at, updated_at, salary_desc, location, remote, status_reason
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
		var salary, location, reason, source sql.NullString
		var fitScore sql.NullInt64
		var remote sql.NullBool

		if err := rows.Scan(&j.ID, &j.Company, &j.Title, &j.URL, &fitScore, &source,
			&discoveredAt, &updatedAt, &salary, &location, &remote, &reason); err != nil {
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
		if salary.Valid {
			j.SalaryDesc = salary.String
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
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	var u string
	if err := db.QueryRow("SELECT url FROM job_funnel WHERE rowid = ? AND status = 'PROCESSED_MANUAL'", req.JobID).Scan(&u); err != nil {
		http.Error(w, "Job not found", http.StatusNotFound)
		return
	}

	// Server-side lookup, verified.
	cmd := exec.Command("xdg-open", u)
	cmd.Start()
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
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	var u string
	if err := db.QueryRow("SELECT url FROM job_funnel WHERE rowid = ? AND status = 'PROCESSED_MANUAL'", req.JobID).Scan(&u); err != nil {
		http.Error(w, "Job not found", http.StatusNotFound)
		return
	}

	if err := storage.UpdateFunnelStatus(u, "PROCESSING"); err != nil {
		http.Error(w, "Failed to promote job", http.StatusInternalServerError)
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
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	var u string
	if err := db.QueryRow("SELECT url FROM job_funnel WHERE rowid = ?", req.JobID).Scan(&u); err != nil {
		http.Error(w, "Job not found", http.StatusNotFound)
		return
	}

	if err := storage.UpdateFunnelStatusWithReason(u, "SKIPPED", "manual_skip"); err != nil {
		http.Error(w, "Failed to skip job", http.StatusInternalServerError)
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
		JobID int64 `json:"job_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	var u string
	var company, title string
	if err := db.QueryRow("SELECT url, company_name, title FROM job_funnel WHERE rowid = ?", req.JobID).Scan(&u, &company, &title); err != nil {
		http.Error(w, "Job not found", http.StatusNotFound)
		return
	}

	if err := storage.UpdateFunnelStatus(u, "APPLIED"); err != nil {
		http.Error(w, "Failed to confirm job", http.StatusInternalServerError)
		return
	}
	storage.RecordApplicationInDB(company, title, u)

	w.WriteHeader(http.StatusOK)
}
