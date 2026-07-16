//go:build ignore

package main

import (
	"database/sql"
	"fmt"
	_ "github.com/mattn/go-sqlite3"
)

func main() {
	db, err := sql.Open("sqlite3", "./applications.db")
	if err != nil {
		fmt.Println("Error opening db:", err)
		return
	}
	defer db.Close()

	// Get latest applied_at from applied_jobs
	var latestApplied sql.NullString
	err = db.QueryRow("SELECT MAX(applied_at) FROM applied_jobs").Scan(&latestApplied)
	if err == nil && latestApplied.Valid {
		fmt.Println("Latest Successful Application:", latestApplied.String)
	} else {
		fmt.Println("Latest Successful Application: None")
	}

	// Get counts of current job funnel
	rows, err := db.Query("SELECT status, count(*) FROM job_funnel GROUP BY status;")
	if err == nil {
		defer rows.Close()
		fmt.Println("\n--- CURRENT FUNNEL COUNTS ---")
		for rows.Next() {
			var status string
			var count int
			rows.Scan(&status, &count)
			fmt.Printf("%s: %d\n", status, count)
		}
	}
}
