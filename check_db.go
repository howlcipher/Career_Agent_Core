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

	rows, err := db.Query("SELECT url, status, fit_score FROM job_funnel WHERE status IN ('FAILED_SCORE', 'FAILED_SUBMIT') LIMIT 20;")
	if err != nil {
		fmt.Println("Query error:", err)
		return
	}
	defer rows.Close()

	fmt.Println("--- Actionable Errors ---")
	count := 0
	for rows.Next() {
		var url, status string
		var fitScore int
		rows.Scan(&url, &status, &fitScore)
		fmt.Printf("Status: %s | URL: %s\n", status, url)
		count++
	}
	fmt.Printf("Total retrieved: %d\n", count)
}
