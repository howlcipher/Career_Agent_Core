package main

import (
	"database/sql"
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
	html := `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Career Agent Dashboard</title>
    <style>
        body { font-family: 'Inter', sans-serif; background-color: #121212; color: #fff; padding: 20px; }
        .grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(200px, 1fr)); gap: 20px; }
        .card { background: #1e1e1e; padding: 20px; border-radius: 8px; text-align: center; border-top: 4px solid #00ff88; }
        h2 { font-size: 2rem; margin: 0; color: #00ff88; }
        p { color: #aaa; margin-top: 5px; }
    </style>
</head>
<body>
    <h1>🚀 Career Agent Live Metrics</h1>
    <div class="grid">
        <div class="card"><h2 id="discovered">0</h2><p>In Queue</p></div>
        <div class="card"><h2 id="processing">0</h2><p>Processing</p></div>
        <div class="card"><h2 id="applied">0</h2><p>Applied</p></div>
        <div class="card"><h2 id="skipped">0</h2><p>Skipped</p></div>
        <div class="card"><h2 id="failed">0</h2><p>Failed</p></div>
    </div>
    <script>
        async function fetchMetrics() {
            const res = await fetch('/api/metrics');
            const data = await res.json();
            document.getElementById('discovered').innerText = data.discovered;
            document.getElementById('processing').innerText = data.processing;
            document.getElementById('applied').innerText = data.applied;
            document.getElementById('skipped').innerText = data.skipped;
            document.getElementById('failed').innerText = data.failed;
        }
        setInterval(fetchMetrics, 3000);
        fetchMetrics();
    </script>
</body>
</html>`
	w.Write([]byte(html))
}
