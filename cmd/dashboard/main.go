package main

import (
	"database/sql"
	"embed"
	"encoding/json"
	"log"
	"net/http"

	_ "github.com/mattn/go-sqlite3"
)

type Metrics struct {
	Discovered int `json:"discovered"`
	Processing int `json:"processing"`
	Skipped    int `json:"skipped"`
	Applied    int `json:"applied"`
	Failed     int `json:"failed"`
}

//go:embed index.html
var indexHTML embed.FS

var db *sql.DB

func main() {
	var err error
	db, err = sql.Open("sqlite3", "./applications.db?_journal_mode=WAL")
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)

	http.HandleFunc("/", serveDashboard)
	http.HandleFunc("/api/metrics", serveMetrics)

	log.Println("🚀 Career Agent Web Dashboard running at http://localhost:8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

func serveMetrics(w http.ResponseWriter, r *http.Request) {
	var m Metrics
	db.QueryRow("SELECT COUNT(*) FROM job_funnel WHERE status = 'DISCOVERED' OR status = 'NEW'").Scan(&m.Discovered)
	db.QueryRow("SELECT COUNT(*) FROM job_funnel WHERE status = 'PROCESSING'").Scan(&m.Processing)
	db.QueryRow("SELECT COUNT(*) FROM job_funnel WHERE status = 'SKIPPED'").Scan(&m.Skipped)
	db.QueryRow("SELECT COUNT(*) FROM job_funnel WHERE status IN ('APPLIED', 'PROCESSED_MANUAL')").Scan(&m.Applied)
	db.QueryRow("SELECT COUNT(*) FROM job_funnel WHERE status IN ('FAILED_SCORE', 'FAILED_SUBMIT')").Scan(&m.Failed)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(m)
}

func serveDashboard(w http.ResponseWriter, r *http.Request) {
	content, err := indexHTML.ReadFile("index.html")
	if err != nil {
		http.Error(w, "Could not load dashboard", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(content)
}
