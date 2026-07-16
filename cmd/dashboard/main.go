package main

import (
	"database/sql"
	"fmt"
	"log"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

func clearScreen() {
	fmt.Print("\033[H\033[2J")
}

func main() {
	db, err := sql.Open("sqlite3", "./applications.db")
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	for {
		clearScreen()
		fmt.Println("==========================================================")
		fmt.Println("🚀 CAREER AGENT: LIVE METRICS DASHBOARD")
		fmt.Println("==========================================================")

		var totalDiscovered, totalProcessing, totalSkipped, totalApplied, totalFailed int

		db.QueryRow("SELECT COUNT(*) FROM job_funnel WHERE status = 'DISCOVERED' OR status = 'NEW'").Scan(&totalDiscovered)
		db.QueryRow("SELECT COUNT(*) FROM job_funnel WHERE status = 'PROCESSING'").Scan(&totalProcessing)
		db.QueryRow("SELECT COUNT(*) FROM job_funnel WHERE status = 'SKIPPED'").Scan(&totalSkipped)
		db.QueryRow("SELECT COUNT(*) FROM job_funnel WHERE status IN ('APPLIED', 'PROCESSED_MANUAL')").Scan(&totalApplied)
		db.QueryRow("SELECT COUNT(*) FROM job_funnel WHERE status IN ('FAILED_SCORE', 'FAILED_SUBMIT')").Scan(&totalFailed)

		totalJobs := totalDiscovered + totalProcessing + totalSkipped + totalApplied + totalFailed
		if totalJobs == 0 {
			totalJobs = 1 // prevent div by zero
		}

		fmt.Printf("🔍 In Queue (Waiting)    : %-5d [%.1f%%]\n", totalDiscovered, float64(totalDiscovered)/float64(totalJobs)*100)
		fmt.Printf("⚙️  Actively Processing   : %-5d [%.1f%%]\n", totalProcessing, float64(totalProcessing)/float64(totalJobs)*100)
		fmt.Printf("📝 Successfully Applied  : %-5d [%.1f%%]\n", totalApplied, float64(totalApplied)/float64(totalJobs)*100)
		fmt.Printf("⏭️  Rejected (Low Fit)    : %-5d [%.1f%%]\n", totalSkipped, float64(totalSkipped)/float64(totalJobs)*100)
		fmt.Printf("⚠️  Actionable Errors     : %-5d [%.1f%%]\n", totalFailed, float64(totalFailed)/float64(totalJobs)*100)
		fmt.Println("----------------------------------------------------------")

		// Recent applications
		fmt.Println("Recent Applications:")
		rows, err := db.Query("SELECT company_name, job_title FROM applied_jobs ORDER BY applied_at DESC LIMIT 5")
		if err == nil {
			count := 0
			for rows.Next() {
				var comp, title string
				if rows.Scan(&comp, &title) == nil {
					fmt.Printf("   ✅ %s - %s\n", comp, title)
					count++
				}
			}
			rows.Close()
			if count == 0 {
				fmt.Println("   (No applications submitted yet...)")
			}
		}

		fmt.Println("==========================================================")
		fmt.Println("Press Ctrl+C to exit. Refreshing every 3 seconds...")
		time.Sleep(3 * time.Second)
	}
}
